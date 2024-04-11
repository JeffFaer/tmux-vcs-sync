package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

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
	Long: `This command has three different forms:

1. If executed in tmux without a work unit name, it will update the VCS to point at the work unit that the current tmux session represents.
2. If executed outside of tmux without a work unit name, it will attach to a tmux session for the current work unit.
3. If given a work unit name, it will attempt to find that work unit in any of the repositories currently active in tmux and update both tmux and that VCS to point at the given work unit. Note: This means that you can update to a work unit that exists in a different repository.`,
	Args: cobra.RangeArgs(0, 1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return suggestWorkUnitNames(cmd.Context(), toComplete), 0
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return update(cmd.Context())
		}
		return updateTo(cmd.Context(), args[0])
	},
}

func suggestWorkUnitNames(ctx context.Context, toComplete string) []string {
	vcs := api.Registered()
	repos := make(map[state.RepoName]api.Repository)
	if srv := tmux.MaybeCurrentServer(); srv != nil {
		st, err := state.New(ctx, srv, vcs)
		if err != nil {
			slog.Warn("Could not determine repositories from tmux server.", "server", srv, "error", err)
		} else {
			repos = st.Repositories()
		}
	}
	if repo, err := vcs.MaybeCurrentRepository(ctx); err != nil {
		slog.Warn("Could not determine current repository.", "error", err)
	} else {
		n := state.NewRepoName(repo)
		if _, ok := repos[n]; !ok {
			repos[n] = repo
		}
	}

	var suggestions []string
	for name, repo := range repos {
		wus, err := repo.List(ctx, toComplete)
		if err != nil {
			slog.Warn("Could not list work units.", "repo", name, "error", err)
		}
		suggestions = append(suggestions, wus...)
	}
	suggestions = slices.DeleteFunc(suggestions, func(s string) bool { return !strings.HasPrefix(s, toComplete) })
	return suggestions
}

func update(ctx context.Context) error {
	vcs := api.Registered()
	curRepo, err := vcs.CurrentRepository(ctx)
	if err != nil {
		return err
	}
	curWorkUnit, err := curRepo.Current(ctx)
	if err != nil {
		return fmt.Errorf("couldn't check repo's current %s: %w", curRepo.VCS().WorkUnitName(), err)
	}
	curSesh := tmux.MaybeCurrentSession()
	if curSesh == nil {
		// Executed outside of tmux. Attach to the proper tmux session.
		srv := tmux.DefaultServer()
		state, err := state.New(ctx, srv, vcs)
		if err != nil {
			return err
		}
		return updateTmux(ctx, state, curRepo, curWorkUnit, true)
	}

	// Executed within tmux. Update the repo state.
	name, err := curSesh.Property(ctx, tmux.SessionName)
	if err != nil {
		return err
	}
	parsed := state.ParseSessionName(curRepo, name)
	if curWorkUnit != parsed.WorkUnit {
		slog.Info("Updating repository.", "current", curWorkUnit, "want", parsed.WorkUnit)
		return curRepo.Update(ctx, parsed.WorkUnit)
	}
	slog.Info("No update needed.")
	if failNoop {
		os.Exit(1)
	}
	return nil
}

func updateTmux(ctx context.Context, st *state.State, repo api.Repository, workUnit string, endTrace bool) error {
	sesh := st.Session(repo, workUnit)
	if sesh == nil {
		var err error
		sesh, err = st.NewSession(ctx, repo, workUnit)
		if err != nil {
			return err
		}
	}
	var err error
	if endTrace {
		// Attaching to a session hangs until the client is detached.
		err = stopTrace()
	}
	return errors.Join(sesh.Server().AttachOrSwitch(ctx, sesh), err)
}

func updateTo(ctx context.Context, workUnitName string) error {
	vcs := api.Registered()
	srv := tmux.MaybeCurrentServer()
	hasCurrentServer := srv != nil
	if !hasCurrentServer {
		srv = tmux.DefaultServer()
	}
	st, err := state.New(ctx, srv, vcs)
	if err != nil {
		return err
	}

	var repo api.Repository
	if cur, err1 := existsInCurrentRepo(ctx, vcs, workUnitName); err1 == nil && cur != nil {
		repo = cur
	} else {
		var err2 error
		repo, err2 = st.MaybeFindRepository(ctx, workUnitName)
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
	slog.Info("Found repository for requested work unit.", "name", state.NewWorkUnitName(repo, workUnitName))

	var update bool

	// Update to the work unit.
	if cur, err := repo.Current(ctx); err != nil {
		return fmt.Errorf("couldn't check repo's current %s: %w", repo.VCS().WorkUnitName(), err)
	} else if cur != workUnitName {
		slog.Info("Updating repository.", "got", cur, "want", workUnitName)
		if err := repo.Update(ctx, workUnitName); err != nil {
			return err
		}
		update = true
	}

	// Switch or attach to the tmux session.
	var needsSwitch bool
	if !hasCurrentServer {
		// Not currently attached to tmux.
		needsSwitch = true
	} else if sesh := st.Session(repo, workUnitName); sesh == nil {
		// Session doesn't exist.
		needsSwitch = true
	} else if cur := tmux.MaybeCurrentSession(); cur == nil || !tmux.SameSession(ctx, cur, sesh) {
		// cur == nil shouldn't be possible. We already know we're attached to tmux.
		needsSwitch = true
	}
	if needsSwitch {
		if err := updateTmux(ctx, st, repo, workUnitName, !hasCurrentServer); err != nil {
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

func existsInCurrentRepo(ctx context.Context, vcs api.VersionControlSystems, workUnitName string) (api.Repository, error) {
	repo, err := vcs.MaybeCurrentRepository(ctx)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	ok, err := repo.Exists(ctx, workUnitName)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return repo, nil
}
