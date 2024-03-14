package tmuxtest

import (
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

func (srv *Server) PID() (int, error) { return srv.pid, nil }

func (srv *Server) ListSessions() ([]tmux.Session, error) {
	var ret []tmux.Session
	for _, sesh := range srv.sessions {
		if sesh.dead {
			continue
		}
		ret = append(ret, sesh)
	}
	return ret, nil
}

func (srv *Server) ListClients() ([]tmux.Client, error) {
	return nil, nil
}

func (srv *Server) NewSession(opts tmux.NewSessionOptions) (tmux.Session, error) {
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
		srv: srv,
		id:  id,
		props: map[tmux.SessionProperty]string{
			tmux.SessionID:   id,
			tmux.SessionName: name,
			tmux.SessionPath: dir,
		},
	}
	return srv.sessions[id], nil
}

func (srv *Server) AttachOrSwitch(sesh tmux.Session) error {
	if !tmux.SameServer(srv, sesh.Server()) {
		return fmt.Errorf("session %q does not belong to this server", sesh.ID())
	}
	if srv.sessions[sesh.ID()].dead {
		return fmt.Errorf("session %q was killed", sesh.ID())
	}
	srv.CurrentSession = srv.sessions[sesh.ID()]
	return nil
}

func (srv *Server) Kill() error {
	srv.sessions = nil
	srv.CurrentSession = nil
	return nil
}

type Session struct {
	srv *Server
	id  string

	props map[tmux.SessionProperty]string
	dead  bool
}

var _ tmux.Session = (*Session)(nil)

func (s *Session) Server() tmux.Server { return s.srv }
func (s *Session) ID() string          { return s.id }

func (s *Session) Property(prop tmux.SessionProperty) (string, error) {
	vals, err := s.Properties(prop)
	if err != nil {
		return "", err
	}
	return vals[prop], nil
}

func (s *Session) Properties(props ...tmux.SessionProperty) (map[tmux.SessionProperty]string, error) {
	if s.dead {
		return nil, fmt.Errorf("session %q was killed", s.id)
	}

	ret := make(map[tmux.SessionProperty]string)
	for _, prop := range props {
		ret[prop] = s.props[prop]
	}
	return ret, nil
}

func (s *Session) setProperty(k tmux.SessionProperty, v string) {
	if s.props == nil {
		s.props = make(map[tmux.SessionProperty]string)
	}
	s.props[k] = v
}

func (s *Session) Rename(n string) error {
	if s.dead {
		return fmt.Errorf("session %q was killed", s.id)
	}
	s.setProperty(tmux.SessionName, n)
	return nil
}

func (s *Session) Kill() error {
	if s.dead {
		return fmt.Errorf("session %q was already killed", s.id)
	}
	s.dead = true
	return nil
}
