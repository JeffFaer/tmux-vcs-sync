// repotest implements a fake implementation of api.VersionControlSystem.
package repotest

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api"
)

// NewVCS creates a new, fake VersionControlSystem that requires all
// repositories to be in the given directory.
func NewVCS(dir string) api.VersionControlSystem {
	return fakeVCS{dir}
}

type fakeVCS struct {
	dir string
}

func (vcs fakeVCS) Name() string     { return fmt.Sprintf("fake(%s)", vcs.dir) }
func (fakeVCS) WorkUnitName() string { return "work unit" }
func (vcs fakeVCS) Repository(dir string) (api.Repository, error) {
	if !strings.HasPrefix(dir, vcs.dir) {
		return nil, nil
	}
	return &fakeRepo{
		vcs:       vcs,
		name:      filepath.Base(dir),
		dir:       dir,
		cur:       "root",
		workUnits: map[string]string{"root": ""},
		children:  map[string]map[string]bool{"root": make(map[string]bool)},
	}, nil
}

type fakeRepo struct {
	vcs       api.VersionControlSystem
	name, dir string

	cur       string
	workUnits map[string]string
	children  map[string]map[string]bool
}

func (repo *fakeRepo) VCS() api.VersionControlSystem {
	return repo.vcs
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

func (repo *fakeRepo) List(prefix string) ([]string, error) {
	var ret []string
	for n := range repo.workUnits {
		if strings.HasPrefix(n, prefix) {
			ret = append(ret, n)
		}
	}
	return ret, nil
}

func (repo *fakeRepo) Sort(workUnits []string) error {
	allowed := make(map[string]bool)
	for _, wu := range workUnits {
		if _, ok := repo.workUnits[wu]; !ok {
			return fmt.Errorf("work unit %q does not exist", wu)
		}
		allowed[wu] = true
	}

	var topo []string
	s := map[string]bool{"root": true}
	for len(s) > 0 {
		var n string
		for m := range s {
			if n == "" {
				n = m
			} else {
				n = min(n, m)
			}
		}
		delete(s, n)
		topo = append(topo, n)
		for child := range repo.children[n] {
			s[child] = true
		}
	}

	var i int
	for _, n := range topo {
		if allowed[n] {
			workUnits[i] = n
			i++
		}
	}

	return nil
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
	repo.children[workUnitName] = make(map[string]bool)
	repo.children[parent][workUnitName] = true
	repo.cur = workUnitName
	return nil
}

func (repo *fakeRepo) Rename(workUnitName string) error {
	if _, ok := repo.workUnits[workUnitName]; ok {
		return fmt.Errorf("work unit %q already exists", workUnitName)
	}
	parent := repo.workUnits[repo.cur]
	children := repo.children[repo.cur]
	delete(repo.workUnits, repo.cur)
	delete(repo.children, repo.cur)
	delete(repo.children[parent], repo.cur)
	repo.workUnits[workUnitName] = parent
	repo.children[workUnitName] = children
	repo.children[parent][workUnitName] = true
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
