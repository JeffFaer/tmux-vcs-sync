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
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var quoted bool
		if strings.HasPrefix(toComplete, `"`) {
			quoted = true
			toComplete = toComplete[1:]
		} else {
			unescaped, err := shellquote.Split(toComplete)
			if err != nil {
				slog.Warn("Couldn't unescape input.", "input", toComplete, "error", err)
			} else {
				toComplete = strings.Join(unescaped, " ")
			}
		}
		suggestions := suggestWorkUnitNames(cmd.Context(), state.ParseSessionNameWithoutKnownRepository(toComplete))
		for i, s := range suggestions {
			if quoted {
				suggestions[i] = `"` + s + `"`
			} else {
				suggestions[i] = shellquote.Join(s)
			}
		}
		return suggestions, 0
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return update(cmd.Context())
		}
		return updateTo(cmd.Context(), state.ParseSessionNameWithoutKnownRepository(args[0]))
	},
}

func suggestWorkUnitNames(ctx context.Context, toComplete state.WorkUnitName) []string {
	curRepo, repos := discoverRepositories(ctx, api.Registered())

	var suggestions []string
	for name, repo := range repos {
		if toComplete.Repo != "" {
			if toComplete.Repo != name.Repo {
				// toComplete looks something like "foo>"
				continue
			}
		} else if repo != curRepo && !strings.HasPrefix(name.Repo, toComplete.WorkUnit) {
			// toComplete looks something like "foo"
			continue
		}

		var prefix string
		if toComplete.Repo != "" || repo == curRepo {
			prefix = toComplete.WorkUnit
		}

		wus, err := repo.List(ctx, prefix)
		if err != nil {
			slog.Warn("Could not list work units.", "repo", name, "error", err)
			continue
		}
		for _, wu := range wus {
			if repo != curRepo {
				wu = state.NewWorkUnitName(repo, wu).RepoString()
			}
			suggestions = append(suggestions, wu)
		}
	}
	suggestions = slices.DeleteFunc(suggestions, func(s string) bool { return !strings.HasPrefix(s, toComplete.String()) })
	return suggestions
}

func discoverRepositories(ctx context.Context, vcs api.VersionControlSystems) (current api.Repository, all map[state.RepoName]api.Repository) {
	all = make(map[state.RepoName]api.Repository)
	var srv tmux.Server
	var curSesh tmux.Session
	if sesh := tmux.MaybeCurrentSession(); sesh != nil {
		srv = sesh.Server()
		curSesh = sesh
	} else {
		srv = tmux.DefaultServer()
	}

	st, err := state.New(ctx, srv, vcs)
	if err != nil {
		slog.Warn("Could not determine repositories from tmux server.", "server", srv, "error", err)
	} else {
		all = st.Repositories()
		if curSesh != nil {
			repo, _, err := st.WorkUnit(ctx, curSesh)
			if err != nil {
				slog.Warn("Could not determine current repository from tmux.", "server", srv, "error", err)
			} else {
				return repo, all
			}
		}
	}

	// If we're not in tmux or weren't able to discover the current repo from
	// tmux, check for it directly.
	cur, err := vcs.MaybeCurrentRepository(ctx)
	if err != nil {
		slog.Warn("Could not determine current repository.", "error", err)
		return nil, all
	}
	all[state.NewRepoName(cur)] = cur
	return cur, all
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

func updateTo(ctx context.Context, sessionName state.WorkUnitName) error {
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

	repo, err := findRepository(ctx, vcs, st, sessionName)
	if err != nil {
		return err
	}

	var update bool

	// Update to the work unit.
	if cur, err := repo.Current(ctx); err != nil {
		return fmt.Errorf("couldn't check repo's current %s: %w", repo.VCS().WorkUnitName(), err)
	} else if cur != sessionName.WorkUnit {
		slog.Info("Updating repository.", "got", cur, "want", sessionName.WorkUnit)
		if err := repo.Update(ctx, sessionName.WorkUnit); err != nil {
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
	} else if cur := tmux.MaybeCurrentSession(); cur == nil || !tmux.SameSession(ctx, cur, sesh) {
		// cur == nil shouldn't be possible. We already know we're attached to tmux.
		needsSwitch = true
	}
	if needsSwitch {
		if err := updateTmux(ctx, st, repo, sessionName.WorkUnit, !hasCurrentServer); err != nil {
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

func findRepository(ctx context.Context, vcs api.VersionControlSystems, st *state.State, n state.WorkUnitName) (api.Repository, error) {
	var err1, err2 error
	if n.RepoName.Zero() {
		cur, err1 := existsInCurrentRepo(ctx, vcs, n.WorkUnit)
		if err1 == nil && cur != nil {
			return cur, nil
		}
	}
	repo, err2 := st.MaybeFindRepository(ctx, n)
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
