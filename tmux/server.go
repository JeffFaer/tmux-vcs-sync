package tmux

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
)

// Server represents a tmux server that exists at a particular Socket.
// If Socket is unset, we will use the default tmux socket.
type Server struct {
	opts serverOptions
}

// NewServer creates a new server for the given socket.
// Note: This doesn't actually create the server yet. You will need to create at
// least one session for the server to be active.
func NewServer(opts ...ServerOption) *Server {
	opt := serverOptions{}
	for _, o := range opts {
		o(&opt)
	}
	return &Server{opt}
}

type ServerOption func(*serverOptions)

func NamedServerSocket(name string) ServerOption {
	return func(opts *serverOptions) {
		opts.socketName = name
	}
}

func ServerConfigFile(file string) ServerOption {
	return func(opts *serverOptions) {
		opts.configFile = file
	}
}

type serverOptions struct {
	socketPath string
	socketName string
	configFile string
}

func (opts serverOptions) args() []string {
	var args []string
	if opts.socketPath != "" {
		args = append(args, []string{"-S", opts.socketPath}...)
	}
	if opts.socketName != "" {
		args = append(args, []string{"-L", opts.socketName}...)
	}
	if opts.configFile != "" {
		args = append(args, []string{"-f", opts.configFile}...)
	}
	return args
}

// CurrentServer returns a server if this program is running within a tmux
// server.
func CurrentServer() (*Server, error) {
	srv := MaybeCurrentServer()
	if srv == nil {
		return nil, errNotTmux
	}
	return srv, nil
}

// MaybeCurrentServer returns a server if this program is running within a tmux
// server. If it's not, it returns nil.
func MaybeCurrentServer() *Server {
	srv := maybeCurrentServer()
	if srv != nil {
		slog.Info("Found tmux server.", "server", srv)
	}
	return srv
}

// maybeCurrentServer is the same as MaybeCurrentServer, except it doesn't log.
func maybeCurrentServer() *Server {
	sp := strings.SplitN(os.Getenv("TMUX"), ",", 2)
	if socket := sp[0]; socket != "" {
		return &Server{serverOptions{socketPath: socket}}
	}
	return nil
}

// CurrentServerOrDefault either returns the CurrentServer, or the default server.
func CurrentServerOrDefault() (*Server, bool) {
	srv := MaybeCurrentServer()
	if srv == nil {
		return &Server{}, false
	}
	return srv, true
}

func (srv *Server) LogValue() slog.Value {
	switch {
	case srv.opts.socketPath != "":
		return slog.GroupValue(slog.String("socket", srv.opts.socketPath))
	case srv.opts.socketName != "":
		return slog.GroupValue(slog.String("socket", srv.opts.socketName))
	default:
		return slog.StringValue("<<default server>>")
	}
}

func (srv *Server) command(args ...string) *exec.Command {
	args = append(srv.opts.args(), args...)
	return tmux.Command(args...)
}

// PID returns the process ID of the server, if it's currently active.
func (srv *Server) PID() (int, error) {
	pid, err := srv.command("display-message", "-p", "-F", "#{pid}").RunStdout()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(pid)
}

// Equal determines if this Server is the same as the other Server, based on PID.
func (srv *Server) Equal(other *Server) bool {
	if srv == other {
		return true
	}
	pid1, err := srv.PID()
	if err != nil {
		return false
	}
	pid2, err := other.PID()
	if err != nil {
		return false
	}
	return pid1 == pid2
}

// ListSessions lists the sessions that exist in this tmux server.
// This method will also lookup and cache the given properties.
func (srv *Server) ListSessions() ([]*Session, error) {
	stdout, stderr, err := srv.command("list-sessions", "-F", string(SessionID)).RunOutput()
	if err != nil {
		if
		// Socket doesn't yet exists.
		strings.Contains(stderr, "No such file or directory") ||
			// Socket exists, but no server owns it.
			strings.Contains(stderr, "no server running") {
			return nil, nil
		}
		fmt.Fprintln(os.Stderr, stderr)
		return nil, err
	}
	var res []*Session
	for _, id := range strings.Split(stdout, "\n") {
		res = append(res, &Session{srv, id})
	}
	return res, nil
}

func (srv *Server) ListClients() ([]*Client, error) {
	stdout, err := srv.command("list-clients", "-F", string(ClientTTY)).RunStdout()
	if err != nil {
		return nil, err
	}
	if stdout == "" {
		return nil, nil
	}
	var res []*Client
	for _, tty := range strings.Split(stdout, "\n") {
		res = append(res, &Client{srv: srv, TTY: tty})
	}
	return res, nil
}

// NewSession creates a new session in this tmux server.
func (srv *Server) NewSession(opts ...NewSessionOption) (*Session, error) {
	opt := &newSessionOptions{}
	for _, o := range opts {
		o(opt)
	}
	args := []string{"new-session", "-d", "-P", "-F", string(SessionID)}
	args = append(args, opt.args()...)
	newSession := srv.command(args...)
	newSession.Stdin = os.Stdin // tmux wants a tty.
	stdout, err := newSession.RunStdout()
	if err != nil {
		return nil, err
	}
	return &Session{Server: srv, ID: stdout}, nil
}

type newSessionOptions struct {
	name     string
	startDir string
}

func (opts newSessionOptions) args() []string {
	var res []string
	if opts.name != "" {
		res = append(res, []string{"-s", opts.name}...)
	}
	if opts.startDir != "" {
		res = append(res, []string{"-c", opts.startDir}...)
	}
	return res
}

// NewSessionOption affects how NewSession creates sessions.
type NewSessionOption func(*newSessionOptions)

// NewSessionName creates the new session with the given initial name.
func NewSessionName(name string) NewSessionOption {
	return func(opts *newSessionOptions) {
		opts.name = name
	}
}

// NewSessionStartDirectory creates the new session with the given initial start directory.
func NewSessionStartDirectory(dir string) NewSessionOption {
	return func(opts *newSessionOptions) {
		opts.startDir = dir
	}
}

// AttachOrSwitch either attaches a new client connected to the given
// TargetSession, or switches the current client to the TargetSession.
func (srv *Server) AttachOrSwitch(s TargetSession) error {
	var cmd *exec.Command
	var err error
	if os.Getenv("TMUX") != "" {
		cmd, err = srv.switchCommand(s)
	} else {
		cmd, err = srv.attachCommand(s)
	}
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (srv *Server) attachCommand(s TargetSession) (*exec.Command, error) {
	target, err := srv.resolveTargetSession(s)
	if err != nil {
		return nil, err
	}
	cmd := srv.command("attach-session", "-t", target)
	cmd.Stdin = os.Stdin // tmux wants a tty.
	return cmd, nil
}

func (srv *Server) switchCommand(s TargetSession) (*exec.Command, error) {
	target, err := srv.resolveTargetSession(s)
	if err != nil {
		return nil, err
	}
	cmd := srv.command("switch-client", "-t", target)
	cmd.Stdin = os.Stdin // tmux wants a tty.
	return cmd, nil
}

func (srv *Server) resolveTargetSession(s TargetSession) (string, error) {
	switch {
	case s.sesh != nil:
		if !srv.Equal(s.sesh.Server) {
			return "", fmt.Errorf("target session does not exist in this server")
		}
		return s.sesh.ID, nil
	default:
		panic("unhandled TargetSession case")
	}
}

// Kill kills this server.
func (srv *Server) Kill() error {
	return srv.command("kill-server").Run()
}
