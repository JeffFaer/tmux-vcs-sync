package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/state"
	"github.com/spf13/cobra"
)

var failNoop bool

func init() {
	updateCommand.Flags().BoolVar(&failNoop, "fail-noop", false, "If update didn't do anything (because both tmux and the repository were already in the correct state), return a non-zero exit code.")
	rootCmd.AddCommand(updateCommand)
}

var updateCommand = &cobra.Command{
	Use:   "update [name]",
	Short: "Update tmux and the corresponding repository to point to the given work unit.",
	Long: `This command has two different forms:

1. If executed without a work unit name, it will update the VCS to point at the work unit that the current tmux session represents.
2. If given a work unit name, it will attempt to find that work unit in any of the repositories currently active in tmux and update both tmux and that VCS to point at the given work unit. Note: This means that you can update to a work unit that exists in a different repository.`,
	Args: cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			repo, err := api.Registered.CurrentRepository()
			if err != nil {
				return err
			}
			sesh, err := tmux.CurrentSession()
			if err != nil {
				return err
			}

			name, err := sesh.Property(tmux.SessionName)
			if err != nil {
				return err
			}
			parsed := state.ParseSessionName(repo, name)
			if cur, err := repo.Current(); err != nil {
				return fmt.Errorf("couldn't check repo's current %s: %w", repo.VCS().WorkUnitName(), err)
			} else if cur != parsed.WorkUnit {
				return repo.Update(parsed.WorkUnit)
			}
			if failNoop {
				os.Exit(1)
			}
			return nil
		}

		return update(args[0])
	},
}

func update(workUnitName string) error {
	srv := tmux.CurrentServerOrDefault()
	state, err := state.New(srv)
	if err != nil {
		return err
	}

	var repo api.Repository
	if cur, err1 := existsInCurrentRepo(workUnitName); err == nil && cur != nil {
		repo = cur
	} else {
		var err2 error
		repo, err2 = state.MaybeFindRepository(workUnitName)
		if err2 != nil {
			return errors.Join(err1, err2)
		}
		if repo == nil {
			return errors.Join(err1, fmt.Errorf("could not find any repository that contains work unit %q", workUnitName))
		}
		if err1 != nil {
			slog.Warn("An error occurred with the current repository.", "error", err1)
		}
	}

	sesh := state.Session(repo, workUnitName)
	var update bool
	if sesh == nil {
		sesh, err = state.NewSession(repo, workUnitName)
		if err != nil {
			return err
		}
		update = true
	}
	if cur, err := repo.Current(); err != nil {
		return fmt.Errorf("couldn't check repo's current %s: %w", repo.VCS().WorkUnitName(), err)
	} else if cur != workUnitName {
		if err := repo.Update(workUnitName); err != nil {
			return err
		}
		update = true
	}
	if cur, err := tmux.MaybeCurrentSession(); err != nil {
		return fmt.Errorf("couldn't check tmux current state: %w", err)
	} else if cur == nil || !cur.Equal(sesh) {
		if err := srv.AttachOrSwitch(sesh.Target()); err != nil {
			return err
		}
		update = true
	}

	if update {
		return nil
	}
	if failNoop {
		os.Exit(1)
	}
	return nil
}

func existsInCurrentRepo(workUnitName string) (api.Repository, error) {
	repo, err := api.Registered.MaybeCurrentRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	ok, err := repo.Exists(workUnitName)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return repo, nil
}
