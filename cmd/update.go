package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/state"
	"github.com/kballard/go-shellquote"
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
	Long: `This command has three different forms:

1. If executed in tmux without a work unit name, it will update the VCS to point at the work unit that the current tmux session represents.
2. If executed outside of tmux without a work unit name, it will attach to a tmux session for the current work unit.
3. If given a work unit name, it will attempt to find that work unit in any of the repositories currently active in tmux and update both tmux and that VCS to point at the given work unit. Note: This means that you can update to a work unit that exists in a different repository.`,
	Args: cobra.RangeArgs(0, 1),
	ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return suggestWorkUnitNames(toComplete), 0
	},
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return update()
		}
		return updateTo(state.ParseSessionNameWithoutKnownRepository(args[0]))
	},
}

func suggestWorkUnitNames(toComplete string) []string {
	var repos map[state.RepoName]api.Repository
	if srv := tmux.MaybeCurrentServer(); srv != nil {
		st, err := state.New(srv, api.Registered)
		if err != nil {
			slog.Warn("Could not determine repositories from tmux server.", "server", srv, "error", err)
		} else {
			repos = st.Repositories()
		}
	}
	cur, err := api.Registered.MaybeCurrentRepository()
	if err != nil {
		slog.Warn("Could not determine current repository.", "error", err)
	} else {
		repos[state.NewRepoName(cur)] = cur
	}

	var suggestions []string
	for name, repo := range repos {
		wus, err := repo.List("")
		if err != nil {
			slog.Warn("Could not list work units.", "repo", name, "error", err)
			continue
		}
		for _, wu := range wus {
			if repo != cur {
				wu = state.NewWorkUnitName(repo, wu).RepoString()
			}
			wu = shellquote.Join(wu)
			suggestions = append(suggestions, wu)
		}
	}
	suggestions = slices.DeleteFunc(suggestions, func(s string) bool {
		return !strings.HasPrefix(s, toComplete)
	})
	return suggestions
}

func update() error {
	repo, err := api.Registered.CurrentRepository()
	if err != nil {
		return err
	}
	cur, err := repo.Current()
	if err != nil {
		return fmt.Errorf("couldn't check repo's current %s: %w", repo.VCS().WorkUnitName(), err)
	}
	if sesh := tmux.MaybeCurrentSession(); sesh == nil {
		// Executed outside of tmux. Attach to the proper tmux session.
		srv, _ := tmux.CurrentServerOrDefault()
		state, err := state.New(srv, api.Registered)
		if err != nil {
			return err
		}
		return updateTmux(srv, state, repo, cur)
	} else {
		// Executed within tmux. Update the repo state.
		name, err := sesh.Property(tmux.SessionName)
		if err != nil {
			return err
		}
		parsed := state.ParseSessionName(repo, name)
		if cur != parsed.WorkUnit {
			slog.Info("Updating repository.", "current", cur, "want", parsed.WorkUnit)
			return repo.Update(parsed.WorkUnit)
		}
		slog.Info("No update needed.")
		if failNoop {
			os.Exit(1)
		}
		return nil
	}
}

func updateTmux(srv tmux.Server, st *state.State, repo api.Repository, workUnit string) error {
	sesh := st.Session(repo, workUnit)
	if sesh == nil {
		var err error
		sesh, err = st.NewSession(repo, workUnit)
		if err != nil {
			return err
		}
	}
	return srv.AttachOrSwitch(sesh)
}

func updateTo(sessionName state.WorkUnitName) error {
	srv, hasCurrentServer := tmux.CurrentServerOrDefault()
	st, err := state.New(srv, api.Registered)
	if err != nil {
		return err
	}

	repo, err := findRepository(st, sessionName)
	if err != nil {
		return err
	}

	var update bool

	// Update to the work unit.
	if cur, err := repo.Current(); err != nil {
		return fmt.Errorf("couldn't check repo's current %s: %w", repo.VCS().WorkUnitName(), err)
	} else if cur != sessionName.WorkUnit {
		slog.Info("Updating repository.", "current", cur, "want", sessionName.WorkUnit)
		if err := repo.Update(sessionName.WorkUnit); err != nil {
			return err
		}
		update = true
	}

	// Switch or attach to the tmux session.
	var needsSwitch bool
	if !hasCurrentServer {
		// Not currently attached to tmux.
		needsSwitch = true
	} else if sesh := st.Session(repo, sessionName.WorkUnit); sesh == nil {
		// Session doesn't exist.
		needsSwitch = true
	} else if cur := tmux.MaybeCurrentSession(); cur == nil || !tmux.SameSession(cur, sesh) {
		// cur == nil shouldn't be possible. We already know we're attached to tmux.
		needsSwitch = true
	}
	if needsSwitch {
		if err := updateTmux(srv, st, repo, sessionName.WorkUnit); err != nil {
			return err
		}
		update = true
	}

	if update {
		return nil
	}
	slog.Info("No update needed.")
	if failNoop {
		os.Exit(1)
	}
	return nil
}

func findRepository(st *state.State, n state.WorkUnitName) (api.Repository, error) {
	var err1, err2 error
	if n.RepoName.Zero() {
		cur, err1 := existsInCurrentRepo(n.WorkUnit)
		if err1 == nil && cur != nil {
			return cur, nil
		}
	}
	repo, err2 := st.MaybeFindRepository(n)
	if err2 != nil {
		return nil, errors.Join(err1, err2)
	}
	if repo == nil {
		return nil, errors.Join(err1, fmt.Errorf("could not find repository %v", n))
	}
	if err1 != nil {
		slog.Warn("An error occurred with the current repository.", "error", err1)
	}
	slog.Info("Found repository for requested work unit.", "name", state.NewWorkUnitName(repo, n.WorkUnit))
	return repo, nil
}

func existsInCurrentRepo(workUnitName string) (api.Repository, error) {
	repo, err := api.Registered.MaybeCurrentRepository()
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	if ok, err := repo.Exists(workUnitName); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}
	return repo, nil
}
