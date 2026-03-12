package tmuxtest

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/JeffFaer/tmux-vcs-sync/tmux"
)

type Server struct {
	pid int

	nextSessionID int
	sessions      map[string]*Session

	CurrentSession *Session
}

var _ tmux.Server = (*Server)(nil)

// In the real world, different tmux.Server instances would return the same
// state since they're calling out to real tmux. Simulate that by caching
// instances per pid.
var servers = make(map[int]*Server)

func NewServer(pid int) *Server {
	if srv := servers[pid]; srv != nil {
		return srv
	}
	servers[pid] = &Server{pid: pid}
	return servers[pid]
}

func (srv *Server) PID(context.Context) (int, error) { return srv.pid, nil }

func (srv *Server) ListSessions(context.Context) (tmux.Sessions, error) {
	var ret Sessions
	for _, sesh := range srv.sessions {
		if sesh.dead {
			continue
		}
		ret = append(ret, sesh)
	}
	return ret, nil
}

func (srv *Server) ListClients(context.Context) ([]tmux.Client, error) {
	return nil, nil
}

func (srv *Server) NewSession(_ context.Context, opts tmux.NewSessionOptions) (tmux.Session, error) {
	idNum := srv.nextSessionID
	id := fmt.Sprintf("%d#%d", srv.pid, idNum)
	srv.nextSessionID++

	name := strconv.Itoa(idNum)
	if n := opts.Name; n != "" {
		name = n
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Getwd: %w", err)
	}
	if d := opts.StartDir; d != "" {
		dir = d
	}

	if srv.sessions == nil {
		srv.sessions = make(map[string]*Session)
	}
	srv.sessions[id] = &Session{
		srv:   srv,
		id:    id,
		props: tmux.CreateSessionPropertyValues(tmux.SessionID.Value(id), tmux.SessionName.Value(name), tmux.SessionPath.Value(dir)),
	}
	return srv.sessions[id], nil
}

func (srv *Server) AttachOrSwitch(ctx context.Context, sesh tmux.Session) error {
	if !tmux.SameServer(ctx, srv, sesh.Server()) {
		return fmt.Errorf("session %q does not belong to this server", sesh.ID())
	}
	if srv.sessions[sesh.ID()].dead {
		return fmt.Errorf("session %q was killed", sesh.ID())
	}
	srv.CurrentSession = srv.sessions[sesh.ID()]
	return nil
}

func (srv *Server) Kill(context.Context) error {
	srv.sessions = nil
	srv.CurrentSession = nil
	return nil
}

type Sessions []*Session

var _ tmux.Sessions = (Sessions)(nil)

func (s Sessions) Server() tmux.Server {
	return s[0].Server()
}

func (s Sessions) Sessions() []tmux.Session {
	ret := make([]tmux.Session, len(s))
	for i, sesh := range s {
		ret[i] = sesh
	}
	return ret
}

func (s Sessions) Property(ctx context.Context, prop tmux.SessionPropertyName) (map[tmux.Session]tmux.SessionPropertyValue, error) {
	vals, err := s.Properties(ctx, prop)
	if err != nil {
		return nil, err
	}
	ret := make(map[tmux.Session]tmux.SessionPropertyValue, len(vals))
	for sesh, props := range vals {
		ret[sesh] = props[prop]
	}
	return ret, nil
}

func (s Sessions) Properties(ctx context.Context, props ...tmux.SessionPropertyName) (map[tmux.Session]tmux.SessionPropertyValues, error) {
	ret := make(map[tmux.Session]tmux.SessionPropertyValues, len(s))
	for _, sesh := range s {
		var err error
		ret[sesh], err = sesh.Properties(ctx, props...)
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

type Session struct {
	srv *Server
	id  string

	props tmux.SessionPropertyValues
	dead  bool
}

var _ tmux.Session = (*Session)(nil)

func (s *Session) Server() tmux.Server { return s.srv }
func (s *Session) ID() string          { return s.id }

func (s *Session) Property(ctx context.Context, prop tmux.SessionPropertyName) (tmux.SessionPropertyValue, error) {
	vals, err := s.Properties(ctx, prop)
	if err != nil {
		return tmux.SessionPropertyValue{}, err
	}
	return vals[prop], nil
}

func (s *Session) Properties(_ context.Context, props ...tmux.SessionPropertyName) (tmux.SessionPropertyValues, error) {
	if s.dead {
		return nil, fmt.Errorf("session %q was killed", s.id)
	}

	ret := make(tmux.SessionPropertyValues, len(props))
	for _, prop := range props {
		ret[prop] = s.props[prop]
	}
	return ret, nil
}

func (s *Session) setProperty(v tmux.SessionPropertyValue) {
	if s.props == nil {
		s.props = make(tmux.SessionPropertyValues)
	}
	s.props.Set(v)
}

func (s *Session) Rename(_ context.Context, n string) error {
	if s.dead {
		return fmt.Errorf("session %q was killed", s.id)
	}
	s.setProperty(tmux.SessionName.Value(n))
	return nil
}

func (s *Session) Kill(context.Context) error {
	if s.dead {
		return fmt.Errorf("session %q was already killed", s.id)
	}
	s.dead = true
	return nil
}
