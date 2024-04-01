package cmd

import (
	"context"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/state"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(cleanupCommand)
}

var cleanupCommand = &cobra.Command{
	Use:   "cleanup",
	Short: "Delete tmux sessions which appear to be for work units that no longer exist.",
	Args:  cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cleanup(cmd.Context())
	},
}

func cleanup(ctx context.Context) error {
	srv := tmux.MaybeCurrentServer()
	if srv == nil {
		srv = tmux.DefaultServer()
	}
	st, err := state.New(ctx, srv, api.Registered())
	if err != nil {
		return err
	}
	return st.PruneSessions(ctx)
}
