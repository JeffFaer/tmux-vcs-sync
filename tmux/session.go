package tmux

import (
	"fmt"
	"log/slog"
	"strings"
)

// Equal determines if two sessions are equivalent by checking they have the same ID and belong to the same Server.
func SameSession(a, b Session) bool {
	return a.ID() == b.ID() && SameServer(a.Server(), b.Server())
}

// Session represents a tmux session on a particular server.
type session struct {
	srv *server
	id  string
}

// CurrentSession returns a Session if this program is being executed inside
// tmux.
func CurrentSession() (Session, error) {
	sesh := MaybeCurrentSession()
	if sesh == nil {
		return nil, errNotTmux
	}
	return sesh, nil
}

// MaybeCurrentSession returns a Session if this program is being executed
// inside tmux. If it's not being executed inside tmux, returns nil.
func MaybeCurrentSession() Session {
	env, err := getenv()
	if err != nil {
		return nil
	}
	sesh := env.session()
	slog.Info("Found current tmux session.", "server", sesh.srv, "session", sesh.id)
	return sesh
}

func (s *session) Server() Server {
	return s.srv
}

func (s *session) ID() string {
	return s.id
}

func (s *session) Property(prop SessionProperty) (string, error) {
	props, err := s.Properties(prop)
	if err != nil {
		return "", err
	}
	return props[prop], nil
}

// Properties fetches properties about a session.
func (s *session) Properties(props ...SessionProperty) (map[SessionProperty]string, error) {
	res, err := properties(props, func(keys []string) ([]string, error) {
		stdout, err := s.srv.command("display-message", "-t", s.id, "-p", strings.Join(keys, "\n")).RunStdout()
		if err != nil {
			return nil, err
		}
		return strings.Split(stdout, "\n"), nil
	})
	if err != nil {
		return nil, fmt.Errorf("session %q: %w", s.id, err)
	}
	return res, nil
}

func (s *session) Rename(name string) error {
	err := s.srv.command("rename-session", "-t", s.id, name).Run()
	if err != nil {
		return fmt.Errorf("could not rename session %q to %q: %w", s.ID(), name, err)
	}
	return nil
}

func (s *session) Kill() error {
	err := s.srv.command("kill-session", "-t", s.id).Run()
	if err != nil {
		return fmt.Errorf("could not kill session %q: %w", s.ID(), err)
	}
	return nil
}
