package repotest

import (
	"fmt"
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
)

type Options struct {
	NoopRenameIsOK bool
}

func RepoTests(t *testing.T, repoCtor func(string) (api.Repository, error), opts Options) {
	for _, tc := range []struct {
		name string

		test    func(api.Repository) error
		wantErr bool
	}{
		{
			name: "EmptyRepository_Current",

			test: func(repo api.Repository) error {
				cur, err := repo.Current()
				if err != nil {
					return fmt.Errorf("repo.Current() = _, %v", err)
				}
				if cur == "" {
					return fmt.Errorf("expected empty repository to have a current work unit")
				}
				return checkExists(repo, cur)
			},
			wantErr: false,
		},
		{
			name: "New",
			test: func(repo api.Repository) error {
				workUnit := "abcd"
				if err := repo.New(workUnit); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnit, err)
				}
				if err := checkExists(repo, workUnit); err != nil {
					return err
				}
				if err := checkCurrent(repo, workUnit); err != nil {
					return err
				}
				return nil
			},
			wantErr: false,
		},
		{
			name: "New_DuplicateWorkUnitName",
			test: func(repo api.Repository) error {
				workUnit := "abcd"
				if err := repo.New(workUnit); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnit, err)
				}
				return repo.New(workUnit)
			},
			wantErr: true,
		},
		{
			name: "CommitAfterNew",
			test: func(repo api.Repository) error {
				workUnits := []string{
					"abcd1",
					"abcd2",
				}
				if err := repo.New(workUnits[0]); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnits[0], err)
				}
				if err := checkExists(repo, workUnits[0]); err != nil {
					return err
				}
				if err := checkCurrent(repo, workUnits[0]); err != nil {
					return err
				}
				if err := repo.Commit(workUnits[1]); err != nil {
					return fmt.Errorf("repo.Commit(%q) = %v", workUnits[1], err)
				}
				if err := checkExists(repo, workUnits...); err != nil {
					return err
				}
				if err := checkCurrent(repo, workUnits[1]); err != nil {
					return err
				}
				return nil
			},
			wantErr: false,
		},
		{
			name: "Rename",

			test: func(repo api.Repository) error {
				origName := "abcd"
				newName := "efgh"
				if err := repo.New(origName); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", origName, err)
				}
				if err := repo.Rename(newName); err != nil {
					return fmt.Errorf("repo.Rename(%q) = %v", newName, err)
				}
				if err := checkNotExists(repo, origName); err != nil {
					return err
				}
				if err := checkExists(repo, newName); err != nil {
					return err
				}
				if err := checkCurrent(repo, newName); err != nil {
					return err
				}
				return nil
			},
			wantErr: false,
		},
		{
			name: "Rename_Noop",

			test: func(repo api.Repository) error {
				workUnitName := "abcd"
				if err := repo.New(workUnitName); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnitName, err)
				}
				return repo.Rename(workUnitName)
			},
			wantErr: !opts.NoopRenameIsOK,
		},
		{
			name: "Rename_DuplicateWorkUnitName",

			test: func(repo api.Repository) error {
				workUnitNames := []string{
					"abcd",
					"efgh",
				}
				if err := repo.New(workUnitNames[0]); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnitNames[0], err)
				}
				if err := repo.New(workUnitNames[1]); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnitNames[1], err)
				}
				if err := checkCurrent(repo, workUnitNames[1]); err != nil {
					return err
				}
				return repo.Rename(workUnitNames[0])
			},
			wantErr: true,
		},
		{
			name: "Update",

			test: func(repo api.Repository) error {
				workUnitNames := []string{
					"abcd",
					"efgh",
				}
				if err := repo.New(workUnitNames[0]); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnitNames[0], err)
				}
				if err := repo.New(workUnitNames[1]); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnitNames[1], err)
				}
				if err := checkExists(repo, workUnitNames...); err != nil {
					return err
				}
				if err := checkCurrent(repo, workUnitNames[1]); err != nil {
					return err
				}
				if err := repo.Update(workUnitNames[0]); err != nil {
					return fmt.Errorf("repo.Update(%q) = %v", workUnitNames[0], err)
				}
				if err := checkExists(repo, workUnitNames...); err != nil {
					return err
				}
				if err := checkCurrent(repo, workUnitNames[0]); err != nil {
					return err
				}
				return nil
			},
			wantErr: false,
		},
		{
			name: "Update_OriginalWorkUnitName",

			test: func(repo api.Repository) error {
				cur, err := repo.Current()
				if err != nil {
					return fmt.Errorf("repo.Current() = _, %v", err)
				}
				workUnitName := "abcd"
				if err := repo.New(workUnitName); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnitName, err)
				}
				if err := checkExists(repo, cur, workUnitName); err != nil {
					return err
				}
				if err := checkCurrent(repo, workUnitName); err != nil {
					return err
				}
				if err := repo.Update(cur); err != nil {
					return fmt.Errorf("repo.Update(%q) = %v", cur, err)
				}
				if err := checkExists(repo, cur, workUnitName); err != nil {
					return err
				}
				if err := checkCurrent(repo, cur); err != nil {
					return err
				}
				return nil
			},
			wantErr: false,
		},
		{
			name: "Update_NonExistantWorkUnitName",

			test: func(repo api.Repository) error {
				workUnitName := "abcd"
				if err := repo.New(workUnitName); err != nil {
					return fmt.Errorf("repo.New(%q) = %v", workUnitName, err)
				}
				return repo.Update("efgh")
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := repoCtor(tc.name)
			if err != nil {
				t.Fatalf("Could not create repository: %v", err)
			}

			if err := tc.test(repo); (err != nil) != tc.wantErr {
				t.Errorf("got error %v; wantErr %t", err, tc.wantErr)
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