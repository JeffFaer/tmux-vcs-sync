// repotest implements a fake implementation of api.VersionControlSystem.
package repotest

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api"
)

type fakeVCS struct{}

var VCS = fakeVCS{}

func (fakeVCS) Name() string         { return "fake" }
func (fakeVCS) WorkUnitName() string { return "work unit" }
func (fakeVCS) Repository(dir string) (api.Repository, error) {
	return &fakeRepo{
		name:      filepath.Base(dir),
		dir:       dir,
		cur:       "root",
		workUnits: map[string]string{"root": ""},
	}, nil
}

type fakeRepo struct {
	name, dir string

	cur       string
	workUnits map[string]string
}

func (*fakeRepo) VCS() api.VersionControlSystem {
	return VCS
}

func (repo *fakeRepo) Name() string {
	return repo.name
}

func (repo *fakeRepo) RootDir() string {
	return repo.dir
}

func (repo *fakeRepo) Current() (string, error) {
	return repo.cur, nil
}

func (repo *fakeRepo) ListWorkUnits(prefix string) ([]string, error) {
	var ret []string
	for n := range repo.workUnits {
		if strings.HasPrefix(n, prefix) {
			ret = append(ret, n)
		}
	}
	return ret, nil
}

func (repo *fakeRepo) New(workUnitName string) error {
	return repo.commit(workUnitName, "root")
}

func (repo *fakeRepo) Commit(workUnitName string) error {
	return repo.commit(workUnitName, repo.cur)
}

func (repo *fakeRepo) commit(workUnitName, parent string) error {
	if _, ok := repo.workUnits[workUnitName]; ok {
		return fmt.Errorf("work unit %q already exists", workUnitName)
	}
	if _, ok := repo.workUnits[parent]; !ok {
		// Maybe someone renamed root?
		return fmt.Errorf("parent work unit %q does not exist", parent)
	}
	repo.workUnits[workUnitName] = parent
	repo.cur = workUnitName
	return nil
}

func (repo *fakeRepo) Rename(workUnitName string) error {
	if _, ok := repo.workUnits[workUnitName]; ok {
		return fmt.Errorf("work unit %q already exists", workUnitName)
	}
	parent := repo.workUnits[repo.cur]
	delete(repo.workUnits, repo.cur)
	repo.workUnits[workUnitName] = parent
	repo.cur = workUnitName
	return nil
}

func (repo *fakeRepo) Exists(workUnitName string) (bool, error) {
	_, ok := repo.workUnits[workUnitName]
	return ok, nil
}

func (repo *fakeRepo) Update(workUnitName string) error {
	if _, ok := repo.workUnits[workUnitName]; !ok {
		return fmt.Errorf("work unit %q does not exist", workUnitName)
	}
	repo.cur = workUnitName
	return nil
}
