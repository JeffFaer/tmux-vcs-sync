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
	// tmux sessions in srv with their associated repositories.
	sessions map[SessionName]session
	// An index of unqualified repo names that exist in sessions.
	unqualifiedRepos map[string]bool
	// Representative examples of each api.Repository in sessions.
	repos map[RepoName]api.Repository
}

func New(srv *tmux.Server) (*State, error) {
	sessions, err := srv.ListSessions()
	if err != nil {
		return nil, err
	}

	st := &State{
		srv:              srv,
		sessions:         make(map[SessionName]session),
		unqualifiedRepos: make(map[string]bool),
		repos:            make(map[RepoName]api.Repository),
	}
	// An index from directory to api.Repository.
	// This tool makes tmux sessions with the repository's root dir, so there's a
	// pretty good chance for some cache hits here.
	reposByDir := make(map[string]api.Repository)
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

		repo, ok := reposByDir[path]
		if !ok {
			var err error
			repo, err = api.Registered.MaybeFindRepository(path)
			if err != nil {
				logger.Warn("Error while checking for repository in tmux session.", "error", err)
				continue
			}
			reposByDir[path] = repo
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
		st.unqualifiedRepos[parsed.Repo] = true
		st.repos[parsed.RepoName] = repo
		logger.Info("Found repository in tmux session.", "name", parsed)
	}
	return st, nil
}

type session struct {
	tmux *tmux.Session
	repo api.Repository
}

func (st *State) sessionNameString(n SessionName) string {
	if len(st.unqualifiedRepos) > 1 || (len(st.unqualifiedRepos) == 1 && !st.unqualifiedRepos[n.Repo]) {
		return n.RepoString()
	}
	return n.WorkUnitString()
}

// Repositories returns a representative example for each known RepoName.
func (st *State) Repositories() []api.Repository {
	var repos []api.Repository
	for _, repo := range st.repos {
		repos = append(repos, repo)
	}
	return repos
}

// Session determines if a tmux session for the given work unit exists.
func (st *State) Session(repo api.Repository, workUnitName string) *tmux.Session {
	n := NewSessionName(repo, workUnitName)
	ret := st.sessions[n].tmux
	if ret != nil {
		slog.Info("Found existing tmux session for work unit.", "id", ret.ID, "name", n)
	}
	return ret
}

// NewSession creates a tmux session for the given work unit.
// Returns an error if the session already exists.
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
	st.unqualifiedRepos[name.Repo] = true
	st.repos[name.RepoName] = repo
	if err := st.updateSessionNames(); err != nil {
		slog.Warn("Failed to update tmux session names.", "error", err)
	}
	return sesh, nil
}

// RenameSession finds a tmux session for work unit old and then renames that
// session so that it represents work unit new.
// Returns an error if the "old" tmux session doesn't exist or if there's
// already a "new" tmux session.
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

// MaybeFindRepository attempts to find an api.Repository that claims the given
// work unit exists.
// Returns an error if multiple api.Repositories claim that the given work unit
// exists.
// Returns nil, nil if no such api.Repository exists.
func (st *State) MaybeFindRepository(workUnitName string) (api.Repository, error) {
	repo, err := api.MaybeFindRepository(st.Repositories(), func(repo api.Repository) (api.Repository, error) {
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

type RepoName struct {
	VCS, Repo string
}

func NewRepoName(repo api.Repository) RepoName {
	return RepoName{VCS: repo.VCS().Name(), Repo: repo.Name()}
}

func (n RepoName) LogValue() slog.Value {
	return slog.GroupValue(slog.String("vcs", n.VCS), slog.String("repo", n.Repo))
}

type SessionName struct {
	RepoName
	WorkUnit string
}

func ParseSessionName(repo api.Repository, tmuxSessionName string) SessionName {
	n := SessionName{RepoName: NewRepoName(repo)}

	sp := strings.SplitN(tmuxSessionName, ">", 3)
	switch len(sp) {
	case 1:
		n.WorkUnit = sp[0]
	case 2:
		if n.Repo != sp[0] {
			slog.Warn("Session name does not agree with repository.", "session_name", tmuxSessionName, "repo", n.Repo)
		}
		n.WorkUnit = sp[1]
	default:
		if n.VCS != sp[1] || n.Repo != sp[1] {
			slog.Warn("Session name does not agree with repository.", "session_name", tmuxSessionName, "vcs", n.VCS, "repo", n.Repo)
		}
		n.WorkUnit = sp[2]
	}
	return n
}

func NewSessionName(repo api.Repository, workUnitName string) SessionName {
	return SessionName{NewRepoName(repo), workUnitName}
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
