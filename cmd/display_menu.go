package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JeffFaer/go-stdlib-ext/morecmp"
	"github.com/JeffFaer/go-stdlib-ext/moremaps"
	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/state"
	"github.com/spf13/cobra"
	"golang.org/x/exp/maps"
)

func init() {
	rootCmd.AddCommand(displayMenuCommand)
}

var displayMenuCommand = &cobra.Command{
	Use:    "display-menu",
	Hidden: true,
	Short:  "Run tmux display-menu to switch to a new session.",
	Args:   cobra.ExactArgs(0),
	RunE: func(*cobra.Command, []string) error {
		return displayMenu()
	},
}

func displayMenu() error {
	curSesh, err := tmux.CurrentSession()
	if err != nil {
		return err
	}
	curClient, err := tmux.CurrentClient()
	if err != nil {
		// This shouldn't be possible since we have a current session.
		return err
	}

	menu, err := createMenu(curSesh, api.Registered)
	if err != nil {
		return err
	}
	return curClient.DisplayMenu(menu)
}

func createMenu(curSesh tmux.Session, vcs api.VersionControlSystems) ([]tmux.MenuElement, error) {
	st, err := state.New(curSesh.Server(), vcs)
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
		name string
		id   string
	}
	var orderedSessions [][]session
	repos := st.Repositories()
	for _, n := range repoNames {
		repo := repos[n]
		sessions := sessionsByRepo[n]
		workUnits := maps.Keys(sessions)
		if err := repo.Sort(workUnits); err != nil {
			return nil, err
		}
		var group []session
		for _, wu := range workUnits {
			sesh := sessions[wu]
			n := state.NewWorkUnitName(repo, wu)
			group = append(group, session{st.SessionName(n), sesh.ID()})
		}
		orderedSessions = append(orderedSessions, group)
	}

	unknownSessions := st.UnknownSessions()
	if len(unknownSessions) > 0 {
		var group []session
		for _, n := range moremaps.SortedKeys(unknownSessions) {
			group = append(group, session{n, unknownSessions[n].ID()})
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
