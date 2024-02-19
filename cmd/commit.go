package cmd

import (
	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(commitCommand)
}

var commitCommand = &cobra.Command{
	Use:   "commit name",
	Short: "Create a tmux session and work unit based on the current work unit in the repository.",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return newWorkUnit(args[0], api.Repository.Commit)
	},
}
