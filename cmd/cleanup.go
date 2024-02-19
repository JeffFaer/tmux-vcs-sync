package cmd

import (
	"log/slog"
	"slices"

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
	validWorkUnits := make(map[state.SessionName]bool)
	errRepos := make(map[state.RepoName]bool)
	for _, repo := range st.Repositories() {
		wus, err := repo.ListWorkUnits("")
		if err != nil {
			n := state.NewRepoName(repo)
			errRepos[n] = true
			slog.Warn("Could not list work units for repository.", "repo", n)
			continue
		}
		for _, wu := range wus {
			validWorkUnits[state.NewSessionName(repo, wu)] = true
		}
	}
	var toRemove []*tmux.Session
	for n, sesh := range st.Sessions() {
		if errRepos[n.RepoName] {
			continue
		}
		if !validWorkUnits[n] {
			toRemove = append(toRemove, sesh)
		}
	}
	if curSesh, err := tmux.MaybeCurrentSession(); err != nil {
		slog.Warn("Could not determine current session.", "error", err)
	} else if curSesh != nil {
		// Delete the current session last so we don't terminate this command
		// early.
		var del bool
		toRemove = slices.DeleteFunc(toRemove, func(other *tmux.Session) bool {
			if curSesh.Equal(other) {
				del = true
				return true
			}
			return false
		})
		if del {
			toRemove = append(toRemove, curSesh)
		}
	}

	for _, sesh := range toRemove {
		if err := sesh.Kill(); err != nil {
			return err
		}
	}
	return nil
}
