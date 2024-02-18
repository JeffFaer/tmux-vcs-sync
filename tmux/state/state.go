package state

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
)

type State struct {
	srv *tmux.Server
	// sessions is a list of tmux sessions associated with their repositories.
	sessions map[SessionName]session
	// repos is an index of the repo names that exist in sessions.
	repos map[string]bool
}

func New(srv *tmux.Server) (*State, error) {
	sessions, err := srv.ListSessions()
	if err != nil {
		return nil, err
	}

	st := &State{
		srv:      srv,
		sessions: make(map[SessionName]session),
		repos:    make(map[string]bool),
	}
	// repos is an index from directory to api.Repository.
	// This tool makes tmux sessions with the repository's root dir, so there's a
	// pretty good chance for some cache hits here.
	repos := make(map[string]api.Repository)
	for _, sesh := range sessions {
		logger := slog.Default().With("id", sesh.ID)
		logger.Debug("Checking for repository in tmux session.")
		props, err := sesh.Properties(tmux.SessionName, tmux.SessionPath)
		if err != nil {
			return nil, err
		}
		name, path := props[tmux.SessionName], props[tmux.SessionPath]
		logger = logger.With("session_name", name)
		logger.Debug("Resolved tmux session properties.")

		repo, ok := repos[path]
		if !ok {
			var err error
			repo, err = api.Registered.MaybeFindRepository(path)
			if err != nil {
				logger.Warn("Error while checking for repository in tmux session.", "error", err)
				continue
			}
			repos[path] = repo
		}
		if repo == nil {
			logger.Info("Not a repository.")
			continue
		}

		parsed := ParseSessionName(repo, name)
		st.sessions[parsed] = session{
			tmux: sesh,
			repo: repo,
		}
		st.repos[parsed.Repo] = true
		logger.Info("Found repository in tmux session.", "name", parsed)
	}
	return st, nil
}

type session struct {
	tmux *tmux.Session
	repo api.Repository
}

func (st *State) sessionNameString(n SessionName) string {
	if len(st.repos) > 1 || (len(st.repos) == 1 && !st.repos[n.Repo]) {
		return n.RepoString()
	}
	return n.WorkUnitString()
}

func (st *State) Session(repo api.Repository, workUnitName string) *tmux.Session {
	n := NewSessionName(repo, workUnitName)
	ret := st.sessions[n].tmux
	if ret != nil {
		slog.Info("Found existing tmux session for work unit.", "id", ret.ID, "name", n)
	}
	return ret
}

func (st *State) NewSession(repo api.Repository, workUnitName string) (*tmux.Session, error) {
	name := NewSessionName(repo, workUnitName)
	if _, ok := st.sessions[name]; ok {
		return nil, fmt.Errorf("tmux session %q already exists", st.sessionNameString(name))
	}

	n := st.sessionNameString(name)
	slog.Info("Creating tmux session.", "name", name, "session_name", n)
	sesh, err := st.srv.NewSession(tmux.NewSessionName(n), tmux.NewSessionStartDirectory(repo.RootDir()))
	if err != nil {
		return nil, fmt.Errorf("failed to create tmux session %q: %w", n, err)
	}

	st.sessions[name] = session{sesh, repo}
	st.repos[name.Repo] = true
	if err := st.updateSessionNames(); err != nil {
		slog.Warn("Failed to update tmux session names.", "error", err)
	}
	return sesh, nil
}

func (st *State) RenameSession(repo api.Repository, old, new string) error {
	oldName := NewSessionName(repo, old)
	sesh, ok := st.sessions[oldName]
	if !ok {
		return fmt.Errorf("tmux session %q does not exist", st.sessionNameString(oldName))
	}
	newName := NewSessionName(repo, new)
	if _, ok := st.sessions[newName]; ok {
		return fmt.Errorf("tmux session %q already exists", st.sessionNameString(newName))
	}

	if err := sesh.tmux.Rename(st.sessionNameString(newName)); err != nil {
		return err
	}

	delete(st.sessions, oldName)
	st.sessions[newName] = sesh

	if err := st.updateSessionNames(); err != nil {
		slog.Warn("Failed to update tmux session names.", "error", err)
	}
	return nil
}

func (st *State) updateSessionNames() error {
	var errs []error
	for k, sesh := range st.sessions {
		name, err := sesh.tmux.Property(tmux.SessionName)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if want := st.sessionNameString(k); name != want {
			if err := sesh.tmux.Rename(want); err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}
	return errors.Join(errs...)
}

func (st *State) MaybeFindRepository(workUnitName string) (api.Repository, error) {
	seen := make(map[api.Repository]bool)
	var repos []api.Repository
	for _, sesh := range st.sessions {
		if seen[sesh.repo] {
			continue
		}
		seen[sesh.repo] = true
		repos = append(repos, sesh.repo)
	}

	repo, err := api.MaybeFindRepository(repos, func(repo api.Repository) (api.Repository, error) {
		ok, err := repo.Exists(workUnitName)
		if err != nil {
			return nil, err
		} else if ok {
			return repo, nil
		}
		return nil, nil
	})
	if err != nil {
		return nil, fmt.Errorf("work unit %q: %w", workUnitName, err)
	}
	return repo, nil
}

type SessionName struct {
	VCS, Repo, WorkUnit string
}

func ParseSessionName(repo api.Repository, name string) SessionName {
	n := SessionName{VCS: repo.VCS().Name(), Repo: repo.Name()}

	sp := strings.SplitN(name, ">", 3)
	switch len(sp) {
	case 1:
		n.WorkUnit = sp[0]
	case 2:
		if n.Repo != sp[0] {
			slog.Warn("Session name does not agree with repository.", "session_name", name, "repo", n.Repo)
		}
		n.WorkUnit = sp[1]
	default:
		if n.VCS != sp[1] || n.Repo != sp[1] {
			slog.Warn("Session name does not agree with repository.", "session_name", name, "vcs", n.VCS, "repo", n.Repo)
		}
		n.WorkUnit = sp[2]
	}
	return n
}

func NewSessionName(repo api.Repository, workUnitName string) SessionName {
	return SessionName{repo.VCS().Name(), repo.Name(), workUnitName}
}

func (n SessionName) RepoString() string {
	return fmt.Sprintf("%s>%s", n.Repo, n.WorkUnit)
}

func (n SessionName) WorkUnitString() string {
	return n.WorkUnit
}

func (n SessionName) LogValue() slog.Value {
	return slog.GroupValue(slog.String("vcs", n.VCS), slog.String("repo", n.Repo), slog.String("work_unit", n.WorkUnit))
}
