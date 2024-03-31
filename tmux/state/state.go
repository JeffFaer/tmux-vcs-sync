package state

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"runtime/trace"
	"slices"
	"strings"
	"sync"

	"github.com/JeffFaer/go-stdlib-ext/morecmp"
	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	expmaps "golang.org/x/exp/maps"
)

type State struct {
	srv      tmux.Server
	sessions tmux.Sessions

	// tmux sessions in srv with their associated repositories.
	sessionsByName map[WorkUnitName]tmux.Session
	sessionsByID   map[string]workUnit
	// An index of unqualified repo names that exist in sessions.
	unqualifiedRepos map[string]int
	// Representative examples of each api.Repository in sessions.
	repos map[RepoName]api.Repository

	unknownSessions map[string]tmux.Session
}

func New(ctx context.Context, srv tmux.Server, vcs api.VersionControlSystems) (*State, error) {
	defer trace.StartRegion(ctx, "state.New()").End()

	sessions, err := srv.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	st := &State{
		srv:              srv,
		sessions:         sessions,
		sessionsByName:   make(map[WorkUnitName]tmux.Session),
		sessionsByID:     make(map[string]workUnit),
		unqualifiedRepos: make(map[string]int),
		repos:            make(map[RepoName]api.Repository),
		unknownSessions:  make(map[string]tmux.Session),
	}
	props, err := sessions.Properties(ctx, tmux.SessionName, tmux.SessionPath)
	if err != nil {
		return nil, fmt.Errorf("could not resolve session properties: %w", err)
	}

	sessionsByPath := make(map[string][]tmux.Session)
	for sesh, props := range props {
		// This tool makes tmux sessions with the repository's root dir as the
		// session path. There's a pretty good chance we'll have multiple sessions
		// with the same exact session path.
		sessionsByPath[props[tmux.SessionPath]] = append(sessionsByPath[props[tmux.SessionPath]], sesh)
	}

	type result struct {
		sessions []tmux.Session
		api.Repository
	}
	results := make(chan result, len(sessionsByPath))
	var wg sync.WaitGroup
	for path, sessions := range sessionsByPath {
		wg.Add(1)
		go func(path string, sessions []tmux.Session) {
			defer wg.Done()
			logger := slog.With("directory", path)
			logger.Debug("Checking for repository in directory.")

			repo, err := vcs.MaybeFindRepository(ctx, path)
			if err != nil {
				logger.Warn("Error while checking for repository in directory.", "error", err)
				return
			}
			results <- result{sessions, repo}
		}(path, sessions)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		sessions, repo := result.sessions, result.Repository
		if repo != nil {
			st.repos[NewRepoName(repo)] = repo
		}
		for _, sesh := range sessions {
			name := props[sesh][tmux.SessionName]
			logger := slog.With("id", sesh.ID(), "session_name", name)
			if repo == nil {
				st.unknownSessions[name] = sesh
				logger.Info("Not a repository.")
				continue
			}

			parsed := ParseSessionName(repo, name)
			st.sessionsByName[parsed] = sesh
			st.sessionsByID[sesh.ID()] = workUnit{repo, parsed.WorkUnit}
			st.unqualifiedRepos[parsed.Repo]++
			logger.Info("Found work unit in tmux session.", "name", parsed)
		}
	}
	return st, nil
}

// SessionName returns the string that this State would use for the tmux
// session name if a work unit with the given name were created right now.
func (st *State) SessionName(n WorkUnitName) string {
	if len(st.unqualifiedRepos) > 1 || (len(st.unqualifiedRepos) == 1 && st.unqualifiedRepos[n.Repo] == 0) {
		return n.RepoString()
	}
	return n.WorkUnitString()
}

// Sessions returns all tmux sessions keyed by their work unit.
func (st *State) Sessions() map[WorkUnitName]tmux.Session {
	return maps.Clone(st.sessionsByName)
}

// UnknownSessions returns all tmux sessions that didn't appear to have a work
// unit.
func (st *State) UnknownSessions() map[string]tmux.Session {
	return maps.Clone(st.unknownSessions)
}

// WorkUnit returns work unit metadata for the given session.
func (st *State) WorkUnit(ctx context.Context, sesh tmux.Session) (api.Repository, string, error) {
	defer trace.StartRegion(ctx, "State.WorkUnit()").End()
	if !tmux.SameServer(ctx, sesh.Server(), st.srv) {
		return nil, "", fmt.Errorf("sesh is from a different server")
	}
	n, ok := st.sessionsByID[sesh.ID()]
	if !ok {
		return nil, "", fmt.Errorf("sesh does not have an associated work unit")
	}
	return n.repo, n.workUnitName, nil
}

// Repositories returns a representative example for each known RepoName.
func (st *State) Repositories() map[RepoName]api.Repository {
	return maps.Clone(st.repos)
}

// Session determines if a tmux session for the given work unit exists.
func (st *State) Session(repo api.Repository, workUnitName string) tmux.Session {
	n := NewWorkUnitName(repo, workUnitName)
	ret := st.sessionsByName[n]
	if ret != nil {
		slog.Info("Found existing tmux session for work unit.", "id", ret.ID(), "name", n)
	}
	return ret
}

// NewSession creates a tmux session for the given work unit.
// Returns an error if the session already exists.
func (st *State) NewSession(ctx context.Context, repo api.Repository, workUnitName string) (tmux.Session, error) {
	defer trace.StartRegion(ctx, "State.NewSession()").End()

	name := NewWorkUnitName(repo, workUnitName)
	n := st.SessionName(name)
	if _, ok := st.sessionsByName[name]; ok {
		return nil, fmt.Errorf("tmux session %q already exists", n)
	}

	slog.Info("Creating tmux session.", "name", name, "session_name", n)
	sesh, err := st.srv.NewSession(ctx, tmux.NewSessionOptions{Name: n, StartDir: repo.RootDir()})
	if err != nil {
		return nil, fmt.Errorf("failed to create tmux session %q: %w", n, err)
	}

	st.sessionsByName[name] = sesh
	st.sessionsByID[sesh.ID()] = workUnit{repo, name.WorkUnit}
	st.unqualifiedRepos[name.Repo]++
	st.repos[name.RepoName] = repo
	if err := st.updateSessionNames(ctx); err != nil {
		slog.Warn("Failed to update tmux session names.", "error", err)
	}
	return sesh, nil
}

// RenameSession finds a tmux session for work unit old and then renames that
// session so that it represents work unit new.
// Returns an error if the "old" tmux session doesn't exist or if there's
// already a "new" tmux session.
func (st *State) RenameSession(ctx context.Context, repo api.Repository, old, new string) error {
	defer trace.StartRegion(ctx, "State.RenameSession()").End()

	oldName := ParseSessionName(repo, old)
	sesh, ok := st.sessionsByName[oldName]
	if !ok {
		return fmt.Errorf("tmux session %q does not exist", st.SessionName(oldName))
	}
	newName := NewWorkUnitName(repo, new)
	if _, ok := st.sessionsByName[newName]; ok {
		return fmt.Errorf("tmux session %q already exists", st.SessionName(newName))
	}

	if err := sesh.Rename(ctx, st.SessionName(newName)); err != nil {
		return err
	}

	delete(st.sessionsByName, oldName)
	st.sessionsByName[newName] = sesh
	st.sessionsByID[sesh.ID()] = workUnit{repo, newName.WorkUnit}

	if err := st.updateSessionNames(ctx); err != nil {
		slog.Warn("Failed to update tmux session names.", "error", err)
	}
	return nil
}

func (st *State) PruneSessions(ctx context.Context) error {
	defer trace.StartRegion(ctx, "State.PruneSessions()").End()

	validWorkUnits := make(map[WorkUnitName]bool)
	errRepos := make(map[RepoName]bool)
	for n, repo := range st.repos {
		wus, err := repo.List(ctx, "")
		if err != nil {
			errRepos[n] = true
			slog.Warn("Could not list work units for repository.", "repo", n, "error", err)
			continue
		}
		for _, wu := range wus {
			validWorkUnits[NewWorkUnitName(repo, wu)] = true
		}
	}
	invalidSessions := make(map[tmux.Session]WorkUnitName)
	var toRemove []tmux.Session
	for n, sesh := range st.Sessions() {
		if errRepos[n.RepoName] {
			continue
		}
		if !validWorkUnits[n] {
			invalidSessions[sesh] = n
			toRemove = append(toRemove, sesh)
		}
	}
	if curSesh := tmux.MaybeCurrentSession(); curSesh != nil {
		// Delete the current session last so we don't terminate this command
		// early.
		isCurrent := func(s tmux.Session) bool { return tmux.SameSession(ctx, curSesh, s) }
		slices.SortFunc(toRemove, morecmp.ComparingFunc(isCurrent, morecmp.FalseFirst()))
	}

	for _, sesh := range toRemove {
		n := invalidSessions[sesh]
		slog.Warn("Killing session.", "session_id", sesh.ID(), "name", n)
		if err := sesh.Kill(ctx); err != nil {
			return err
		}
		delete(st.sessionsByName, n)
		delete(st.sessionsByID, sesh.ID())
		st.unqualifiedRepos[n.Repo]--
		if st.unqualifiedRepos[n.Repo] == 0 {
			delete(st.unqualifiedRepos, n.Repo)
			delete(st.repos, n.RepoName)
		}
	}

	if err := st.updateSessionNames(ctx); err != nil {
		slog.Warn("Failed to update tmux session names.", "error", err)
	}
	return nil
}

func (st *State) updateSessionNames(ctx context.Context) error {
	defer trace.StartRegion(ctx, "State.updateSessionNames()").End()

	names, err := st.sessions.Property(ctx, tmux.SessionName)
	if err != nil {
		return fmt.Errorf("could not resolve session names: %w", err)
	}
	var errs []error
	for k, sesh := range st.sessionsByName {
		if got, want := names[sesh], st.SessionName(k); got != want {
			if err := sesh.Rename(ctx, want); err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}
	return errors.Join(errs...)
}

// MaybeFindRepository attempts to find an api.Repository that's currently
// active in tmux and claims that the given work unit exists.
// Returns an error if multiple api.Repositories claim that the given work unit
// exists.
// Returns nil, nil if no such api.Repository exists.
func (st *State) MaybeFindRepository(ctx context.Context, n WorkUnitName) (api.Repository, error) {
	var repos []api.Repository
	switch {
	case n.RepoName.VCS != "":
		if n.RepoName.Repo == "" {
			return nil, fmt.Errorf("WorkUnitName has VCS set, but not Repo: %v", n)
		}

		repo, ok := st.repos[n.RepoName]
		if !ok {
			return nil, nil
		}
		repos = append(repos, repo)
	case n.RepoName.Repo != "":
		for m, repo := range st.repos {
			if n.Repo == m.Repo {
				repos = append(repos, repo)
			}
		}
	default:
		repos = expmaps.Values(st.Repositories())
	}

	repo, err := api.MaybeFindRepository(ctx, repos, func(repo api.Repository) (api.Repository, error) {
		if ok, err := repo.Exists(ctx, n.WorkUnit); err != nil {
			return nil, err
		} else if !ok {
			return nil, nil
		}
		return repo, nil
	})
	if err != nil {
		return nil, fmt.Errorf("work unit %v: %w", n, err)
	}
	return repo, nil
}

type RepoName struct {
	VCS, Repo string
}

func NewRepoName(repo api.Repository) RepoName {
	return RepoName{VCS: repo.VCS().Name(), Repo: repo.Name()}
}

func (n RepoName) Zero() bool {
	return n == RepoName{}
}

func (n RepoName) String() string {
	if n.VCS != "" {
		return fmt.Sprintf("%s>%s", n.VCS, n.Repo)
	}
	return n.Repo
}

func (n RepoName) LogValue() slog.Value {
	return slog.GroupValue(slog.String("vcs", n.VCS), slog.String("repo", n.Repo))
}

type WorkUnitName struct {
	RepoName
	WorkUnit string
}

func ParseSessionName(repo api.Repository, tmuxSessionName string) WorkUnitName {
	n := ParseSessionNameWithoutKnownRepository(tmuxSessionName)
	if m := NewRepoName(repo); n.RepoName != m {
		if (n.RepoName.VCS != "" && n.RepoName.VCS != m.VCS) || (n.RepoName.Repo != "" && n.RepoName.Repo != m.Repo) {
			slog.Warn("Session name does not agree with repository.", "session_name", tmuxSessionName, "repo", m)
		}
		n.RepoName = m
	}
	return n
}

func ParseSessionNameWithoutKnownRepository(tmuxSessionName string) WorkUnitName {
	sp := strings.SplitN(tmuxSessionName, ">", 3)
	switch len(sp) {
	case 1:
		return WorkUnitName{WorkUnit: sp[0]}
	case 2:
		return WorkUnitName{RepoName: RepoName{Repo: sp[0]}, WorkUnit: sp[1]}
	default:
		return WorkUnitName{RepoName: RepoName{VCS: sp[0], Repo: sp[1]}, WorkUnit: sp[2]}
	}
}

func NewWorkUnitName(repo api.Repository, workUnitName string) WorkUnitName {
	return WorkUnitName{NewRepoName(repo), workUnitName}
}

func (n WorkUnitName) Zero() bool {
	return n == WorkUnitName{}
}

func (n WorkUnitName) String() string {
	if n.VCS != "" {
		return fmt.Sprintf("%s>%s>%s", n.VCS, n.Repo, n.WorkUnit)
	} else if n.Repo != "" {
		return n.RepoString()
	}
	return n.WorkUnitString()
}

func (n WorkUnitName) RepoString() string {
	return fmt.Sprintf("%s>%s", n.Repo, n.WorkUnit)
}

func (n WorkUnitName) WorkUnitString() string {
	return n.WorkUnit
}

func (n WorkUnitName) LogValue() slog.Value {
	return slog.GroupValue(slog.String("vcs", n.VCS), slog.String("repo", n.Repo), slog.String("work_unit", n.WorkUnit))
}

type workUnit struct {
	repo         api.Repository
	workUnitName string
}

func (wu workUnit) name() WorkUnitName {
	return NewWorkUnitName(wu.repo, wu.workUnitName)
}
