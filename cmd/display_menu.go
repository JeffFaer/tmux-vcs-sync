package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/JeffFaer/go-stdlib-ext/morecmp"
	"github.com/JeffFaer/go-stdlib-ext/moremaps"
	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/state"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(displayMenuCommand)
}

var displayMenuCommand = &cobra.Command{
	Use:    "display-menu",
	Hidden: true,
	Short:  "Run tmux display-menu to switch to a new session.",
	Args:   cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, _ []string) error {
		return displayMenu(cmd.Context())
	},
}

func displayMenu(ctx context.Context) error {
	curSesh, err := tmux.CurrentSession()
	if err != nil {
		return err
	}
	curClient, err := tmux.CurrentClient()
	if err != nil {
		// This shouldn't be possible since we have a current session.
		return err
	}

	menu, err := createMenu(ctx, curSesh, api.Registered())
	if err != nil {
		return err
	}
	// Displaying the menu waits for user input, and might take a really long
	// time. Cancel the trace early to prevent the flight recorder from thinking
	// we took to long.
	err = stopTrace()
	return errors.Join(curClient.DisplayMenu(ctx, menu), err)
}

func createMenu(ctx context.Context, curSesh tmux.Session, vcs api.VersionControlSystems) ([]tmux.MenuElement, error) {
	st, err := state.New(ctx, curSesh.Server(), vcs)
	if err != nil {
		return nil, err
	}

	sessions := st.Sessions()
	sessionsByRepo := make(map[state.RepoName]map[string]tmux.Session)
	for n, sesh := range sessions {
		if sessionsByRepo[n.RepoName] == nil {
			sessionsByRepo[n.RepoName] = make(map[string]tmux.Session)
		}
		sessionsByRepo[n.RepoName][n.WorkUnit] = sesh
	}

	repoCmp := morecmp.Comparing(func(n state.RepoName) string { return n.VCS }).
		AndThen(morecmp.Comparing(func(n state.RepoName) string { return n.Repo }))
	repoNames := moremaps.SortedKeysFunc(sessionsByRepo, repoCmp)

	type session struct {
		name          string
		id            string
		unknownToRepo bool
	}
	var orderedSessions [][]session
	repos := st.Repositories()
	for _, n := range repoNames {
		repo := repos[n]
		sessions := sessionsByRepo[n]
		exists := make(map[string]bool)
		if wus, err := repo.List(ctx, ""); err != nil {
			return nil, err
		} else {
			for _, wu := range wus {
				exists[wu] = true
			}
		}
		var workUnits []string
		for wu := range sessions {
			if exists[wu] {
				workUnits = append(workUnits, wu)
			}
		}
		if err := repo.Sort(ctx, workUnits); err != nil {
			return nil, err
		}
		var group []session
		for _, wu := range workUnits {
			sesh := sessions[wu]
			n := state.NewWorkUnitName(repo, wu)
			group = append(group, session{name: st.SessionName(n), id: sesh.ID()})
		}
		for _, wu := range moremaps.SortedKeys(sessions) {
			if !exists[wu] {
				sesh := sessions[wu]
				n := state.NewWorkUnitName(repo, wu)
				group = append(group, session{name: st.SessionName(n), id: sesh.ID(), unknownToRepo: true})
			}
		}
		orderedSessions = append(orderedSessions, group)
	}

	unknownSessions := st.UnknownSessions()
	if len(unknownSessions) > 0 {
		var group []session
		for _, n := range moremaps.SortedKeys(unknownSessions) {
			group = append(group, session{name: n, id: unknownSessions[n].ID()})
		}
		orderedSessions = append(orderedSessions, group)
	}

	hasCurrentSession := func(group []session) bool {
		for _, s := range group {
			if s.id == curSesh.ID() {
				return true
			}
		}
		return false
	}
	slices.SortStableFunc(orderedSessions, morecmp.ComparingFunc(hasCurrentSession, morecmp.TrueFirst()))

	var menu []tmux.MenuElement
	keys := strings.Split("0123456789wertyuiopasdfghjklzxcvbnm", "")
	for i, group := range orderedSessions {
		if i > 0 {
			menu = append(menu, tmux.MenuSpacer{})
		}
		for _, sesh := range group {
			name := sesh.name
			key := keys[0]
			keys = keys[1:]
			if sesh.id == curSesh.ID() {
				key = "q"
				name = "*" + name
			} else if sesh.unknownToRepo {
				name = "?" + name
			} else {
				name = " " + name
			}
			menu = append(menu, tmux.MenuEntry{
				Name: name,
				Key:  key,
				// TODO: Should this be a `run-shell tmux-vcs-sync update` instead?
				Command: fmt.Sprintf("switch-client -t %s", sesh.id),
			})
		}
	}
	return menu, nil
}
