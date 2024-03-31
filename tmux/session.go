package tmux

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// Equal determines if two sessions are equivalent by checking they have the same ID and belong to the same Server.
func SameSession(ctx context.Context, a, b Session) bool {
	return a.ID() == b.ID() && SameServer(ctx, a.Server(), b.Server())
}

type sessions []*session

func (s sessions) Server() Server {
	return s.server()
}

func (s sessions) server() *server {
	return s[0].srv
}

func (s sessions) Sessions() []Session {
	ret := make([]Session, len(s))
	for i, sesh := range s {
		ret[i] = sesh
	}
	return ret
}

func (s sessions) Property(ctx context.Context, prop SessionProperty) (map[Session]string, error) {
	vals, err := s.Properties(ctx, prop)
	if err != nil {
		return nil, err
	}
	ret := make(map[Session]string, len(vals))
	for sesh, props := range vals {
		ret[sesh] = props[prop]
	}
	return ret, nil
}

func (s sessions) Properties(ctx context.Context, props ...SessionProperty) (map[Session]map[SessionProperty]string, error) {
	if len(s) == 0 {
		return nil, nil
	}

	propStrings := make([]string, len(props)+1)
	propStrings[0] = string(SessionID)
	for i, prop := range props {
		propStrings[i+1] = string(prop)
	}

	format := strings.Join(propStrings, "\n")

	seshByID := make(map[string]Session, len(s))
	idFilters := make([]string, len(s))
	for i, sesh := range s {
		seshByID[sesh.id] = sesh
		idFilters[i] = fmt.Sprintf("#{==:%s,%s}", SessionID, sesh.ID())
	}

	filter := idFilters[0]
	for _, idFilter := range idFilters[1:] {
		filter = fmt.Sprintf("#{||:%s,%s}", filter, idFilter)
	}

	stdout, err := s.server().command(ctx, "list-sessions", "-F", format, "-f", filter).RunStdout()
	if err != nil {
		return nil, err
	}

	ret := make(map[Session]map[SessionProperty]string, len(s))
	lines := strings.Split(stdout, "\n")
	for i := 0; i < len(lines); i++ {
		id := lines[i]
		vals := make(map[SessionProperty]string, len(props))
		for _, prop := range props {
			i++
			vals[prop] = lines[i]
		}
		ret[seshByID[id]] = vals
	}
	return ret, nil
}

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

func (s *session) Property(ctx context.Context, prop SessionProperty) (string, error) {
	props, err := s.Properties(ctx, prop)
	if err != nil {
		return "", err
	}
	return props[prop], nil
}

// Properties fetches properties about a session.
func (s *session) Properties(ctx context.Context, props ...SessionProperty) (map[SessionProperty]string, error) {
	vals, err := sessions{s}.Properties(ctx, props...)
	if err != nil {
		return nil, err
	}
	return vals[Session(s)], nil
}

func (s *session) Rename(ctx context.Context, name string) error {
	err := s.srv.command(ctx, "rename-session", "-t", s.id, name).Run()
	if err != nil {
		return fmt.Errorf("could not rename session %q to %q: %w", s.ID(), name, err)
	}
	return nil
}

func (s *session) Kill(ctx context.Context) error {
	err := s.srv.command(ctx, "kill-session", "-t", s.id).Run()
	if err != nil {
		return fmt.Errorf("could not kill session %q: %w", s.ID(), err)
	}
	return nil
}
