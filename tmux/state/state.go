package state

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
)

type State struct {
	srv *tmux.Server
	// tmux sessions in srv with their associated repositories.
	sessions map[SessionName]*tmux.Session
	// An index of unqualified repo names that exist in sessions.
	unqualifiedRepos map[string]int
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
		sessions:         make(map[SessionName]*tmux.Session),
		unqualifiedRepos: make(map[string]int),
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
		st.sessions[parsed] = sesh
		st.unqualifiedRepos[parsed.Repo]++
		st.repos[parsed.RepoName] = repo
		logger.Info("Found repository in tmux session.", "name", parsed)
	}
	return st, nil
}

func (st *State) sessionNameString(n SessionName) string {
	if len(st.unqualifiedRepos) > 1 || (len(st.unqualifiedRepos) == 1 && st.unqualifiedRepos[n.Repo] == 0) {
		return n.RepoString()
	}
	return n.WorkUnitString()
}

func (st *State) Sessions() map[SessionName]*tmux.Session {
	return maps.Clone(st.sessions)
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
	ret := st.sessions[n]
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

	st.sessions[name] = sesh
	st.unqualifiedRepos[name.Repo]++
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

	if err := sesh.Rename(st.sessionNameString(newName)); err != nil {
		return err
	}

	delete(st.sessions, oldName)
	st.sessions[newName] = sesh

	if err := st.updateSessionNames(); err != nil {
		slog.Warn("Failed to update tmux session names.", "error", err)
	}
	return nil
}

func (st *State) PruneSessions() error {
	validWorkUnits := make(map[SessionName]bool)
	errRepos := make(map[RepoName]bool)
	for _, repo := range st.Repositories() {
		wus, err := repo.ListWorkUnits("")
		if err != nil {
			n := NewRepoName(repo)
			errRepos[n] = true
			slog.Warn("Could not list work units for repository.", "repo", n)
			continue
		}
		for _, wu := range wus {
			validWorkUnits[NewSessionName(repo, wu)] = true
		}
	}
	invalidSessions := make(map[*tmux.Session]SessionName)
	var toRemove []*tmux.Session
	for n, sesh := range st.Sessions() {
		if errRepos[n.RepoName] {
			continue
		}
		if !validWorkUnits[n] {
			invalidSessions[sesh] = n
			toRemove = append(toRemove, sesh)
		}
	}
	if curSesh, err := tmux.MaybeCurrentSession(); err != nil {
		slog.Warn("Could not determine current session.", "error", err)
	} else if curSesh != nil {
		// Delete the current session last so we don't terminate this command
		// early.
		var del bool
		toRemove = slices.DeleteFunc(toRemove, func(other *tmux.Session) bool {
			if curSesh.Equal(other) {
				del = true
				return true
			}
			return false
		})
		if del {
			toRemove = append(toRemove, curSesh)
		}
	}

	for _, sesh := range toRemove {
		n := invalidSessions[sesh]
		slog.Warn("Killing session.", "session_id", sesh.ID, "name", n)
		if err := sesh.Kill(); err != nil {
			return err
		}
		delete(st.sessions, n)
		st.unqualifiedRepos[n.Repo]--
		if st.unqualifiedRepos[n.Repo] == 0 {
			delete(st.unqualifiedRepos, n.Repo)
			delete(st.repos, n.RepoName)
		}
	}

	if err := st.updateSessionNames(); err != nil {
		slog.Warn("Failed to update tmux session names.", "error", err)
	}
	return nil
}

func (st *State) updateSessionNames() error {
	var errs []error
	for k, sesh := range st.sessions {
		name, err := sesh.Property(tmux.SessionName)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if want := st.sessionNameString(k); name != want {
			if err := sesh.Rename(want); err != nil {
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
