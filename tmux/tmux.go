package tmux

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
)

var (
	tmux exec.Executable

	errNotTmux = fmt.Errorf("must be called from within a tmux session")
)

func init() {
	var err error
	tmux, err = exec.Lookup("tmux")
	if err != nil {
		panic(err)
	}
}

type Server interface {
	// PID returns the process ID of the server, if it's currently active.
	PID(context.Context) (int, error)

	// ListSessions lists the sessions that exist in this tmux server.
	ListSessions(context.Context) ([]Session, error)
	// ListClients lists all clients currently attached to this tmux server.
	ListClients(context.Context) ([]Client, error)

	// NewSession creates a new session in this tmux server.
	NewSession(context.Context, NewSessionOptions) (Session, error)
	// AttachOrSwitch either attaches the controlling terminal to the given TargetSession or switches the current tmux client to the TargetSession.
	AttachOrSwitch(context.Context, Session) error

	// Kill this tmux server.
	Kill(context.Context) error
}

// NewSessionOptions affects how NewSession creates sessions.
type NewSessionOptions struct {
	// Name is the optional initial name for the session.
	Name string
	// StartDir is the optional initial working directory for the session.
	StartDir string
}

func (opts NewSessionOptions) args() []string {
	var res []string
	if opts.Name != "" {
		res = append(res, []string{"-s", opts.Name}...)
	}
	if opts.StartDir != "" {
		res = append(res, []string{"-c", opts.StartDir}...)
	}
	return res
}

type Session interface {
	// Server returns the tmux server this Session belongs to.
	Server() Server
	// ID returns the ID of this session within its tmux server.
	ID() string

	// Property retrieves the value of the given property key.
	Property(context.Context, SessionProperty) (string, error)
	// Properties retrieves the values of all the given property keys.
	Properties(context.Context, ...SessionProperty) (map[SessionProperty]string, error)

	// Rename this tmux session to have the given name.
	Rename(context.Context, string) error

	// Kill this tmux session.
	Kill(context.Context) error
}

type SessionProperty string

const (
	SessionID   SessionProperty = "#{session_id}"
	SessionName SessionProperty = "#{session_name}"
	SessionPath SessionProperty = "#{session_path}"
)

type Client interface {
	// Property retrieves the value of the given property key.
	Property(context.Context, ClientProperty) (string, error)
	// Properties retrieves the values of all the given property keys.
	Properties(context.Context, ...ClientProperty) (map[ClientProperty]string, error)

	// DisplayMenu displays a menu in this client.
	DisplayMenu(context.Context, []MenuElement) error
}

type ClientProperty string

const (
	ClientTTY ClientProperty = "#{client_tty}"
)

type MenuElement interface {
	args() []string
}

// MenuEntry is an actual entry in the menu that has an executable command.
type MenuEntry struct {
	Name, Key, Command string
}

// MenuSpacer allows you to delineate sections within a menu.
type MenuSpacer struct{}

func (e MenuEntry) args() []string  { return []string{e.Name, e.Key, e.Command} }
func (e MenuSpacer) args() []string { return []string{""} }

type envVar struct {
	socketPath string
	pid        int
	sessionID  string
}

func getenv() (envVar, error) {
	env := os.Getenv("TMUX")
	if env == "" {
		return envVar{}, errNotTmux
	}

	sp := strings.SplitN(env, ",", 3)
	pid, err := strconv.Atoi(sp[1])
	if err != nil {
		return envVar{}, fmt.Errorf("%w: %w", errNotTmux, err)
	}
	return envVar{sp[0], pid, fmt.Sprintf("$%s", sp[2])}, nil
}

func (env envVar) server() *server {
	return &server{serverOptions{socketPath: env.socketPath}}
}

func (env envVar) session() *session {
	srv := env.server()
	return &session{srv, env.sessionID}
}
