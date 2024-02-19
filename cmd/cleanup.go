package cmd

import (
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
	RunE: func(*cobra.Command, []string) error {
		return cleanup()
	},
}

func cleanup() error {
	srv, _ := tmux.CurrentServerOrDefault()
	st, err := state.New(srv)
	if err != nil {
		return err
	}
	return st.PruneSessions()
}
