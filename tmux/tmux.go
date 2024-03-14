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
	NewSession(opts ...NewSessionOption) (Session, error)
	// AttachOrSwitch either attaches the controlling terminal to the given TargetSession or switches the current tmux client to the TargetSession.
	AttachOrSwitch(Session) error

	// Kill this tmux server.
	Kill() error
}

// NewSessionOption affects how NewSession creates sessions.
type NewSessionOption func(*newSessionOptions)

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
