package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/state"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newCommand)
}

var newCommand = &cobra.Command{
	Use:   "new name",
	Short: "Create a tmux session and work unit based on the current repository's trunk.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return newWorkUnit(cmd.Context(), args[0], api.Repository.New)
	},
}

type workUnitCtor func(api.Repository, context.Context, string) error

func newWorkUnit(ctx context.Context, workUnitName string, ctor workUnitCtor) error {
	vcs := api.Registered()
	repo, err := vcs.CurrentRepository(ctx)
	if err != nil {
		return err
	}
	srv, _ := tmux.CurrentServerOrDefault()
	state, err := state.New(ctx, srv, vcs)
	if err != nil {
		return err
	}

	if state.Session(repo, workUnitName) == nil {
		slog.Info("Creating work unit", "work_unit", workUnitName)
		// If the tmux session doesn't yet exist, try making the work unit first.
		// Otherwise, if the tmux session does exist, we know that state.NewSession
		// will return an error.
		// This logic is kinda ugly.
		if err := ctor(repo, ctx, workUnitName); err != nil {
			return fmt.Errorf("failed to create %s %q: %w", repo.VCS().WorkUnitName(), workUnitName, err)
		}
	}

	sesh, err := state.NewSession(ctx, repo, workUnitName)
	if err != nil {
		return err
	}
	if err := srv.AttachOrSwitch(ctx, sesh); err != nil {
		return fmt.Errorf("failed to attach to newly created session %q: %w", sesh.ID(), err)
	}
	return nil
}

type sessionKey struct {
	repoName, workUnitName string
}
