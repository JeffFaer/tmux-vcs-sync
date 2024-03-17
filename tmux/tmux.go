package tmux

import (
	"fmt"

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
	PID() (int, error)

	// ListSessions lists the sessions that exist in this tmux server.
	ListSessions() ([]Session, error)
	// ListClients lists all clients currently attached to this tmux server.
	ListClients() ([]Client, error)

	// NewSession creates a new session in this tmux server.
	NewSession(NewSessionOptions) (Session, error)
	// AttachOrSwitch either attaches the controlling terminal to the given TargetSession or switches the current tmux client to the TargetSession.
	AttachOrSwitch(Session) error

	// Kill this tmux server.
	Kill() error
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
	Property(SessionProperty) (string, error)
	// Properties retrieves the values of all the given property keys.
	Properties(...SessionProperty) (map[SessionProperty]string, error)

	// Rename this tmux session to have the given name.
	Rename(string) error

	// Kill this tmux session.
	Kill() error
}

type SessionProperty string

const (
	SessionID   SessionProperty = "#{session_id}"
	SessionName SessionProperty = "#{session_name}"
	SessionPath SessionProperty = "#{session_path}"
)

type Client interface {
	// Property retrieves the value of the given property key.
	Property(ClientProperty) (string, error)
	// Properties retrieves the values of all the given property keys.
	Properties(...ClientProperty) (map[ClientProperty]string, error)
}

type ClientProperty string

const (
	ClientTTY ClientProperty = "#{client_tty}"
)
