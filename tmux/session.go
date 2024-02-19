package tmux

import (
	"fmt"
	"log/slog"
	"strings"
)

// TargetSession represents the tmux flag value target-session.
type TargetSession struct {
	sesh *Session
}

// Session represents a tmux session on a particular server.
type Session struct {
	Server *Server
	ID     string
}

// CurrentSession returns a Session if this program is being executed inside
// tmux.
func CurrentSession() (*Session, error) {
	sesh, err := MaybeCurrentSession()
	if err != nil {
		return nil, err
	}
	if sesh == nil {
		return nil, errNotTmux
	}
	return sesh, nil
}

// MaybeCurrentSession returns a Session if this program is being executed
// inside tmux. If it's not being executed inside tmux, returns nil, nil.
// An error may occur if we can't determine the session ID from the running
// tmux server.
func MaybeCurrentSession() (*Session, error) {
	srv := maybeCurrentServer()
	if srv == nil {
		return nil, nil
	}

	id, err := srv.command("display-message", "-p", string(SessionID)).RunStdout()
	if err != nil {
		return nil, fmt.Errorf("could not determine session ID: %w", err)
	}

	slog.Info("Found current tmux session.", "server", srv, "session", id)
	return &Session{srv, id}, nil
}

func (s *Session) Target() TargetSession {
	return TargetSession{sesh: s}
}

func (s *Session) Equal(other *Session) bool {
	return s.ID == other.ID && s.Server.Equal(other.Server)
}

func (s *Session) Property(prop SessionProperty) (string, error) {
	props, err := s.Properties(prop)
	if err != nil {
		return "", err
	}
	return props[prop], nil
}

// Properties fetches properties about a session.
func (s *Session) Properties(props ...SessionProperty) (map[SessionProperty]string, error) {
	res, err := properties(props, func(keys []string) ([]string, error) {
		stdout, err := s.DisplayMessage(strings.Join(keys, "\n"))
		if err != nil {
			return nil, err
		}
		return strings.Split(stdout, "\n"), nil
	})
	if err != nil {
		return nil, fmt.Errorf("session %q: %w", s.ID, err)
	}
	return res, nil
}

type SessionProperty string

const (
	SessionID   SessionProperty = "#{session_id}"
	SessionName SessionProperty = "#{session_name}"
	SessionPath SessionProperty = "#{session_path}"
)

func (s *Session) Rename(name string) error {
	err := s.Server.command("rename-session", "-t", s.ID, name).Run()
	if err != nil {
		return fmt.Errorf("could not rename session %q to %q: %w", s.ID, name, err)
	}
	return nil
}

// Kill kills this session.
func (s *Session) Kill() error {
	err := s.Server.command("kill-session", "-t", s.ID).Run()
	if err != nil {
		return fmt.Errorf("could not kill session %q: %w", s.ID, err)
	}
	return nil
}

func (s *Session) SetOption(opt Option, val string) error {
	if err := s.Server.command("set-option", "-t", s.ID, string(opt), val).Run(); err != nil {
		return fmt.Errorf("set-option -t %s %q %q: %w", s.ID, opt, val, err)
	}
	return nil
}

func (s *Session) DisplayMessage(msg string) (string, error) {
	stdout, err := s.Server.command("display-message", "-t", s.ID, "-p", msg).RunStdout()
	if err != nil {
		return "", fmt.Errorf("display-message -t %s %q: %w", s.ID, msg, err)
	}
	return stdout, nil
}
