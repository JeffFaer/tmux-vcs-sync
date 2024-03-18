package repotest

import (
	"fmt"
	"slices"
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/google/go-cmp/cmp"
)

type Options struct {
	// It's okay for noop renames to succeed. The CLI command will likely fail
	// because the tmux session already exists.
	NoopRenameIsOK bool
	// i.e. when asked to create a work unit and the name is already used, the
	// VCS will implicitly drop the name on the old work unit and use the new
	// name implicitly.
	// Similar to noop rename, this is okay because the CLI command will likely
	// fail because the tmux session already exists.
	ImplicitlyRenamesWorkUnits bool

	ExtraListWorkUnitNames    []string
	ExtraListWorkUnitPrefixes []ListWorkUnitTestCase

	Parallel bool
}

type ListWorkUnitTestCase struct {
	Prefix string
	Want   []string
}

type repoCtor func(*testing.T) api.Repository

func RepoTests(t *testing.T, ctor func(*testing.T, string) (api.Repository, error), opts Options) {
	for n, tc := range map[string]func(*testing.T, repoCtor, Options){
		"EmptyRepository": testEmptyRepository,
		"New":             testNew,
		"Commit":          testCommit,
		"Rename":          testRename,
		"Update":          testUpdate,
		"List":            testList,
	} {
		t.Run(n, func(t *testing.T) {
			if opts.Parallel {
				t.Parallel()
			}
			ctor := func(t *testing.T) api.Repository {
				repo, err := ctor(t, t.Name())
				if err != nil {
					t.Fatalf("Failed to create repository %q: %v", n, err)
				}
				return repo
			}
			tc(t, ctor, opts)
		})
	}
}

func testEmptyRepository(t *testing.T, ctor repoCtor, opts Options) {
	repo := ctor(t)
	cur, err := repo.Current()
	if err != nil {
		t.Fatalf("repo.Current() = _, %v", err)
	}
	if cur == "" {
		t.Fatalf("empty repository should have a current work unit (likely its trunk / default branch)")
	}
	if err := checkExists(repo, cur); err != nil {
		t.Error(err)
	}
}

func testNew(t *testing.T, ctor repoCtor, opts Options) {
	repo := ctor(t)
	workUnit := "abcd"
	if err := repo.New(workUnit); err != nil {
		t.Errorf("repo.New(%q) = %v", workUnit, err)
	}
	if err := checkExists(repo, workUnit); err != nil {
		t.Error(err)
	}
	if err := checkCurrent(repo, workUnit); err != nil {
		t.Error(err)
	}

	wantErr := !opts.ImplicitlyRenamesWorkUnits
	if err := repo.New(workUnit); (err != nil) != wantErr {
		t.Errorf("Creating duplicate work unit resulted in %v, wantErr %t", err, wantErr)
	}
}

func testCommit(t *testing.T, ctor repoCtor, opts Options) {
	repo := ctor(t)
	workUnits := []string{
		"abcd1",
		"abcd2",
	}
	if err := repo.New(workUnits[0]); err != nil {
		t.Errorf("repo.New(%q) = %v", workUnits[0], err)
	}
	if err := checkExists(repo, workUnits[0]); err != nil {
		t.Error(err)
	}
	if err := checkCurrent(repo, workUnits[0]); err != nil {
		t.Error(err)
	}
	if err := repo.Commit(workUnits[1]); err != nil {
		t.Errorf("repo.Commit(%q) = %v", workUnits[1], err)
	}
	if err := checkExists(repo, workUnits...); err != nil {
		t.Error(err)
	}
	if err := checkCurrent(repo, workUnits[1]); err != nil {
		t.Error(err)
	}
}

func testRename(t *testing.T, ctor repoCtor, opts Options) {
	for _, tc := range []struct {
		name string

		workUnits []string
		newName   string

		wantErr bool
	}{
		{
			name: "Simple",

			workUnits: []string{"abcd"},
			newName:   "efgh",
		},
		{
			name: "NoOp",

			workUnits: []string{"abcd"},
			newName:   "abcd",

			wantErr: !opts.NoopRenameIsOK,
		},
		{
			name: "DuplicateRename",

			workUnits: []string{"abcd", "efgh"},
			newName:   "abcd",

			wantErr: !opts.ImplicitlyRenamesWorkUnits,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := ctor(t)
			for _, wu := range tc.workUnits {
				if err := repo.New(wu); err != nil {
					t.Errorf("repo.New(%q) = %v", wu, err)
				}
			}
			if err := repo.Rename(tc.newName); (err != nil) != tc.wantErr {
				t.Errorf("repo.Rename(%q) = %v, wantErr %t", tc.newName, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if prev := tc.workUnits[len(tc.workUnits)-1]; prev != tc.newName {
				if err := checkNotExists(repo, tc.workUnits[len(tc.workUnits)-1]); err != nil {
					t.Error(err)
				}
			}
			if err := checkExists(repo, tc.newName); err != nil {
				t.Error(err)
			}
			if err := checkCurrent(repo, tc.newName); err != nil {
				t.Error(err)
			}
		})
	}
}

func testUpdate(t *testing.T, ctor repoCtor, opts Options) {
	for _, tc := range []struct {
		name string

		workUnits     []string
		update        string
		updateToTrunk bool

		wantErr bool
	}{
		{
			name: "Simple",

			workUnits: []string{"abcd", "efgh"},
			update:    "abcd",
		},
		{
			name: "UpdateToTrunk",

			workUnits:     []string{"efgh"},
			updateToTrunk: true,
		},
		{
			name: "DoesNotExist",

			workUnits: []string{"efgh"},
			update:    "abcd",

			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := ctor(t)
			if tc.updateToTrunk {
				var err error
				tc.update, err = repo.Current()
				if err != nil {
					t.Errorf("repo.Current() = _, %v", err)
				}
			}
			for _, wu := range tc.workUnits {
				if err := repo.New(wu); err != nil {
					t.Errorf("repo.New(%q) = %v", wu, err)
				}
			}
			if err := checkExists(repo, tc.workUnits...); err != nil {
				t.Error(err)
			}
			if err := checkCurrent(repo, tc.workUnits[len(tc.workUnits)-1]); err != nil {
				t.Error(err)
			}
			if err := repo.Update(tc.update); (err != nil) != tc.wantErr {
				t.Errorf("repo.Update(%q) = %v, wantErr %t", tc.update, err, tc.wantErr)
			}
			if err := checkExists(repo, tc.workUnits...); err != nil {
				t.Error(err)
			}
			if tc.wantErr {
				return
			}
			if err := checkCurrent(repo, tc.update); err != nil {
				t.Error(err)
			}
		})
	}
}

func testList(t *testing.T, ctor repoCtor, opts Options) {
	repo := ctor(t)
	workUnitNames := append([]string{
		"abcd1",
		"abcd2",
		"efgh",
	}, opts.ExtraListWorkUnitNames...)
	cur, err := repo.Current()
	if err != nil {
		t.Errorf("repo.Current() = _, %v", err)
	}
	for _, n := range workUnitNames {
		if err := repo.New(n); err != nil {
			t.Errorf("repo.New(%q) = %v", n, err)
		}
	}

	tcs := append([]ListWorkUnitTestCase{
		{
			Prefix: "",
			Want:   slices.Concat(workUnitNames, []string{cur}),
		},
		{
			Prefix: "abcd",
			Want:   []string{"abcd1", "abcd2"},
		},
	}, opts.ExtraListWorkUnitPrefixes...)
	for _, tc := range tcs {
		t.Run(fmt.Sprintf("prefix=%q", tc.Prefix), func(t *testing.T) {
			got, err := repo.List(tc.Prefix)
			if err != nil {
				t.Errorf("repo.List(%q) = _, %v", tc.Prefix, err)
			}
			slices.Sort(got)
			slices.Sort(tc.Want)
			if diff := cmp.Diff(tc.Want, got); diff != "" {
				t.Errorf("repo.List(%q) diff (-want +got)\n%s", tc.Prefix, diff)
			}
		})
	}
}

func checkExists(repo api.Repository, workUnitNames ...string) error {
	for _, n := range workUnitNames {
		if ok, err := repo.Exists(n); err != nil {
			return fmt.Errorf("repo.Exists(%q) = _, %v", n, err)
		} else if !ok {
			return fmt.Errorf("work unit %q does not exist", n)
		}
	}
	return nil
}

func checkNotExists(repo api.Repository, workUnitNames ...string) error {
	for _, n := range workUnitNames {
		if ok, err := repo.Exists(n); err != nil {
			return fmt.Errorf("repo.Exists(%q) = _, %v", n, err)
		} else if ok {
			return fmt.Errorf("work unit %q exists", n)
		}
	}
	return nil
}

func checkCurrent(repo api.Repository, workUnitName string) error {
	if cur, err := repo.Current(); err != nil {
		return fmt.Errorf("repo.Current() = _, %v", err)
	} else if cur != workUnitName {
		return fmt.Errorf("repo.Current() = %q, want %q", cur, workUnitName)
	}
	return nil
}
