package api

import (
	"context"
	"runtime/trace"
)

// tracingVersionControlSystem wraps the provided VersionControlSystem so that
// it automatically creates trace regions.
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
