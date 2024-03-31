package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/trace"
	"slices"
	"strings"
)

// A VersionControlSystem/VCS is a tool that tracks changes to files over time.
type VersionControlSystem interface {
	// Name returns the name of the VCS.
	// e.g. git
	Name() string
	// WorkUnitName returns a human-friendly name for work units.
	// e.g. branch
	WorkUnitName() string
	// Repository determines if a repository instance of this VCS exists for the
	// given directory.
	// If no such instance exists, return nil, nil.
	Repository(ctx context.Context, dir string) (Repository, error)
}

// A Repository is a particular instance of a Version Control System over some
// folder hierarchy.
type Repository interface {
	// VCS is a reference to the VCS that owns this repository.
	VCS() VersionControlSystem
	// Name returns the name of the repository.
	// e.g. tmux-vcs-sync
	Name() string
	// RootDir returns the root directory of the repository.
	RootDir() string

	// Current returns the name of the current work unit.
	Current(context.Context) (string, error)
	// List returns all of the work units in this repository that start with the
	// given prefix.
	List(ctx context.Context, prefix string) ([]string, error)
	// Sort orders the given work units topologically.
	Sort(ctx context.Context, workUnits []string) error

	// New creates a new work unit with the given name on top of the repository's
	// trunk.
	// e.g. Create a new branch on main.
	New(ctx context.Context, workUnitName string) error
	// Commit creates a new work unit with the given name on top of the repository's
	// current work unit.
	// e.g. Create a new branch based on the current branch.
	// n.b. Commit is the same as New if the current branch is main.
	Commit(ctx context.Context, workUnitName string) error
	// Rename the current work unit's name to the given name.
	Rename(ctx context.Context, workUnitName string) error
	// Exists determines whether a work unit with the given name exists in this
	// repository.
	Exists(ctx context.Context, workUnitName string) (bool, error)
	// Update the state of this repository so that the given work unit is
	// "active".
	// e.g. Check out the named branch.
	Update(ctx context.Context, workUnitName string) error
}

type VersionControlSystems []VersionControlSystem

var (
	registered VersionControlSystems
)

// Register registers a VCS for use by tmux-vcs-sync.
func Register(vcs VersionControlSystem) {
	registered = append(registered, TracingVersionControlSystem(vcs))
}

// Registered is all of the VersionControlSystems added via Register.
func Registered() VersionControlSystems {
	return slices.Clone(registered)
}

// TracingVersionControlSystem wraps the provided VersionControlSystem so that
// it automatically creates trace regions.
func TracingVersionControlSystem(vcs VersionControlSystem) VersionControlSystem {
	return &tracingVersionControlSystem{vcs}
}

// CurrentRepository returns a Repository for the current working directory, or an error if one cannot be found.
func (all VersionControlSystems) CurrentRepository(ctx context.Context) (Repository, error) {
	repo, err := all.MaybeCurrentRepository(ctx)
	if repo == nil && err == nil {
		var s []string
		for _, vcs := range all {
			s = append(s, vcs.Name())
		}
		return nil, fmt.Errorf("no Repository exists in working directory (registered VCS: %s)", strings.Join(s, ", "))
	}
	return repo, err
}

func (all VersionControlSystems) MaybeCurrentRepository(ctx context.Context) (Repository, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not get working directory: %w", err)
	}
	repo, err := all.MaybeFindRepository(ctx, wd)
	if err == nil && repo != nil {
		slog.Info("Discovered repository for current working directory.", "vcs", repo.VCS().Name(), "repo", repo.Name())
	}
	return repo, err
}

// MaybeFindRepository attempts to find an Repository for the given directory.
// Returns an error if multiple Repositories claim to exist in the given
// directory.
// Returns nil, nil if no such Repository can be found.
func (all VersionControlSystems) MaybeFindRepository(ctx context.Context, dir string) (Repository, error) {
	defer trace.StartRegion(ctx, "VersionControlSystems.MaybeFindRepository()").End()
	trace.Log(ctx, "directory", dir)
	if len(all) == 0 {
		return nil, fmt.Errorf("no registered VCS")
	}
	repo, err := MaybeFindRepository(ctx, all, func(vcs VersionControlSystem) (Repository, error) { return vcs.Repository(ctx, dir) })
	if err != nil {
		return nil, fmt.Errorf("dir %s: %w", dir, err)
	}
	return repo, nil
}

// MaybeFindRepository attempts to find the single Repository that fn yields for
// all elems.
// Returns an error if fn yields a Repository more than once as we test it
// against each element in elems.
// Returns nil, nil if fn never yields a Repository (or an error).
func MaybeFindRepository[T any](ctx context.Context, elems []T, fn func(T) (Repository, error)) (Repository, error) {
	defer trace.StartRegion(ctx, "api.MaybeFindRepository()").End()
	var repos []Repository
	var errs []error
	for _, e := range elems {
		if repo, err := fn(e); err != nil {
			errs = append(errs, err)
		} else if repo != nil {
			repos = append(repos, repo)
		}
	}

	switch len(repos) {
	case 1:
		if err := errors.Join(errs...); err != nil {
			slog.Warn("Some errors occurred.", "error", err)
		}
		return repos[0], nil
	case 0:
		return nil, errors.Join(errs...)
	default:
		if err := errors.Join(errs...); err != nil {
			slog.Warn("Some errors occurred.", "error", err)
		}

		var s []string
		for _, repo := range repos {
			s = append(s, fmt.Sprintf("%s:%s", repo.VCS().Name(), repo.Name()))
		}
		return nil, fmt.Errorf("multiple Repositories match: %s", strings.Join(s, ", "))
	}
}

type tracingVersionControlSystem struct {
	vcs VersionControlSystem
}

func (vcs *tracingVersionControlSystem) Name() string         { return vcs.vcs.Name() }
func (vcs *tracingVersionControlSystem) WorkUnitName() string { return vcs.vcs.WorkUnitName() }

func (vcs *tracingVersionControlSystem) Repository(ctx context.Context, name string) (Repository, error) {
	defer trace.StartRegion(ctx, "VCS:"+vcs.Name()).End()
	repo, err := vcs.vcs.Repository(ctx, name)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	return &tracingRepository{repo}, nil
}

type tracingRepository struct {
	repo Repository
}

func (repo *tracingRepository) VCS() VersionControlSystem { return repo.repo.VCS() }
func (repo *tracingRepository) Name() string              { return repo.repo.Name() }
func (repo *tracingRepository) RootDir() string           { return repo.repo.RootDir() }

func (repo *tracingRepository) startRegions(ctx context.Context) func() {
	r1 := trace.StartRegion(ctx, "VCS:"+repo.VCS().Name())
	r2 := trace.StartRegion(ctx, "Repo:"+repo.Name())
	return func() {
		r2.End()
		r1.End()
	}
}

func (repo *tracingRepository) Current(ctx context.Context) (string, error) {
	defer repo.startRegions(ctx)()
	return repo.repo.Current(ctx)
}
func (repo *tracingRepository) List(ctx context.Context, prefix string) ([]string, error) {
	defer repo.startRegions(ctx)()
	return repo.repo.List(ctx, prefix)
}
func (repo *tracingRepository) Sort(ctx context.Context, workUnits []string) error {
	defer repo.startRegions(ctx)()
	return repo.repo.Sort(ctx, workUnits)
}
func (repo *tracingRepository) New(ctx context.Context, workUnitName string) error {
	defer repo.startRegions(ctx)()
	return repo.repo.New(ctx, workUnitName)
}
func (repo *tracingRepository) Commit(ctx context.Context, workUnitName string) error {
	defer repo.startRegions(ctx)()
	return repo.repo.Commit(ctx, workUnitName)
}
func (repo *tracingRepository) Rename(ctx context.Context, workUnitName string) error {
	defer repo.startRegions(ctx)()
	return repo.repo.Rename(ctx, workUnitName)
}
func (repo *tracingRepository) Exists(ctx context.Context, workUnitName string) (bool, error) {
	defer repo.startRegions(ctx)()
	return repo.repo.Exists(ctx, workUnitName)
}
func (repo *tracingRepository) Update(ctx context.Context, workUnitName string) error {
	defer repo.startRegions(ctx)()
	return repo.repo.Update(ctx, workUnitName)
}
