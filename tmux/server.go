package tmux

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
)

// Equal determines if two servers equivalent, based on PID.
func SameServer(ctx context.Context, a, b Server) bool {
	if a == b {
		return true
	}
	pid1, err := a.PID(ctx)
	if err != nil {
		return false
	}
	pid2, err := b.PID(ctx)
	if err != nil {
		return false
	}
	return pid1 == pid2
}

// Server represents a tmux server that exists at a particular Socket.
// If Socket is unset, we will use the default tmux socket.
type server struct {
	opts serverOptions
}

// NewServer creates a new server for the given socket.
// Note: This doesn't actually create the server yet. You will need to create at
// least one session for the server to be active.
func NewServer(opts ...ServerOption) *server {
	opt := serverOptions{}
	for _, o := range opts {
		o(&opt)
	}
	return &server{opt}
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
func CurrentServer() (Server, error) {
	srv := MaybeCurrentServer()
	if srv == nil {
		return nil, errNotTmux
	}
	return srv, nil
}

// MaybeCurrentServer returns a server if this program is running within a tmux
// server. If it's not, it returns nil.
func MaybeCurrentServer() Server {
	env, err := getenv()
	if err != nil {
		return nil
	}
	srv := env.server()
	slog.Info("Found tmux server.", "server", srv)
	return srv
}

// CurrentServerOrDefault either returns the CurrentServer, or the default server.
func CurrentServerOrDefault() (Server, bool) {
	srv := MaybeCurrentServer()
	if srv == nil {
		return &server{}, false
	}
	return srv, true
}

func (srv *server) LogValue() slog.Value {
	switch {
	case srv.opts.socketPath != "":
		return slog.GroupValue(slog.String("socket", srv.opts.socketPath))
	case srv.opts.socketName != "":
		return slog.GroupValue(slog.String("socket", srv.opts.socketName))
	default:
		return slog.StringValue("<<default server>>")
	}
}

func (srv *server) command(ctx context.Context, args ...string) *exec.Command {
	args = append(srv.opts.args(), args...)
	return tmux.Command(ctx, args...)
}

func (srv *server) PID(ctx context.Context) (int, error) {
	pid, err := srv.command(ctx, "display-message", "-p", "-F", "#{pid}").RunStdout()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(pid)
}

func (srv *server) ListSessions(ctx context.Context) ([]Session, error) {
	stdout, stderr, err := srv.command(ctx, "list-sessions", "-F", string(SessionID)).RunOutput()
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
	var res []Session
	for _, id := range strings.Split(stdout, "\n") {
		res = append(res, &session{srv, id})
	}
	return res, nil
}

func (srv *server) ListClients(ctx context.Context) ([]Client, error) {
	stdout, err := srv.command(ctx, "list-clients", "-F", string(ClientTTY)).RunStdout()
	if err != nil {
		return nil, err
	}
	if stdout == "" {
		return nil, nil
	}
	var res []Client
	for _, tty := range strings.Split(stdout, "\n") {
		res = append(res, &client{srv, tty})
	}
	return res, nil
}

func (srv *server) NewSession(ctx context.Context, opts NewSessionOptions) (Session, error) {
	args := []string{"new-session", "-d", "-P", "-F", string(SessionID)}
	args = append(args, opts.args()...)
	newSession := srv.command(ctx, args...)
	newSession.Stdin = os.Stdin // tmux wants a tty.
	stdout, err := newSession.RunStdout()
	if err != nil {
		return nil, err
	}
	return &session{srv, stdout}, nil
}

func (srv *server) AttachOrSwitch(ctx context.Context, s Session) error {
	if !SameServer(ctx, srv, s.Server()) {
		return fmt.Errorf("target session does not exist in this server")
	}
	var cmd *exec.Command
	var err error
	if os.Getenv("TMUX") != "" {
		cmd, err = srv.switchCommand(ctx, s)
	} else {
		cmd, err = srv.attachCommand(ctx, s)
	}
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (srv *server) attachCommand(ctx context.Context, s Session) (*exec.Command, error) {
	cmd := srv.command(ctx, "attach-session", "-t", s.ID())
	cmd.Stdin = os.Stdin // tmux wants a tty.
	return cmd, nil
}

func (srv *server) switchCommand(ctx context.Context, s Session) (*exec.Command, error) {
	cmd := srv.command(ctx, "switch-client", "-t", s.ID())
	cmd.Stdin = os.Stdin // tmux wants a tty.
	return cmd, nil
}

func (srv *server) Kill(ctx context.Context) error {
	return srv.command(ctx, "kill-server").Run()
}
