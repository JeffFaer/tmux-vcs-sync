package cmd

import (
	"fmt"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/state"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(renameCommand)
}

var renameCommand = &cobra.Command{
	Use:   "rename new-name",
	Short: "Rename both the current tmux session and work unit.",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return rename(args[0])
	},
}

func rename(newName string) error {
	repo, err := api.Registered.CurrentRepository()
	if err != nil {
		return err
	}
	sesh, err := tmux.CurrentSession()
	if err != nil {
		return err
	}
	state, err := state.New(sesh.Server(), api.Registered)
	if err != nil {
		return err
	}
	oldName, err := sesh.Property(tmux.SessionName)
	if err != nil {
		return err
	}
	if state.Session(repo, newName) == nil {
		// If the tmux session doesn't yet exist, try renaming the work unit first.
		// Otherwise, if the tmux session does exist, we know that state.RenameSession will
		// return an error.
		// This logic is kinda ugly.
		if err := repo.Rename(newName); err != nil {
			return fmt.Errorf("could not rename %s %q to %q: %w", repo.VCS().WorkUnitName(), oldName, newName, err)
		}
	}
	if err := state.RenameSession(repo, oldName, newName); err != nil {
		return err
	}
	return nil
}
