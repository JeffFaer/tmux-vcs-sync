package cmd

import (
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/api/repotest"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/tmuxtest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestDisplayMenu(t *testing.T) {
	for i, tc := range []struct {
		name string

		sessions []tmux.NewSessionOptions
		current  tmux.NewSessionOptions
		vcs      api.VersionControlSystems

		want []tmux.MenuElement
	}{
		{
			name: "SingleSession",

			current: tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo"},
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/", repotest.RepoConfig{
					Name: "repo",
					WorkUnits: map[string][]string{
						repotest.DefaultWorkUnitName: {"foo"},
					},
				}),
			},

			want: []tmux.MenuElement{
				tmux.MenuEntry{Name: "*foo", Key: "q"},
			},
		},
		{
			name: "SingleSession_Unknown",

			current: tmux.NewSessionOptions{Name: "foo", StartDir: "someOtherDir"},
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			want: []tmux.MenuElement{
				tmux.MenuEntry{Name: "*foo", Key: "q"},
			},
		},
		{
			name: "SingleRepo_TopologicallySorted",

			sessions: []tmux.NewSessionOptions{
				{Name: "x", StartDir: "testing/repo"},
				{Name: "y", StartDir: "testing/repo"},
				{Name: "z", StartDir: "testing/repo"},
			},
			current: tmux.NewSessionOptions{Name: repotest.DefaultWorkUnitName, StartDir: "testing/repo"},
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/", repotest.RepoConfig{
					Name: "repo",
					WorkUnits: map[string][]string{
						repotest.DefaultWorkUnitName: {"z"},
						"z":                          {"y"},
						"y":                          {"x"},
					},
				}),
			},

			want: []tmux.MenuElement{
				tmux.MenuEntry{Name: "*" + repotest.DefaultWorkUnitName, Key: "q"},
				tmux.MenuEntry{Name: " z", Key: "1"},
				tmux.MenuEntry{Name: " y", Key: "2"},
				tmux.MenuEntry{Name: " x", Key: "3"},
			},
		},
		{
			name: "SingleRepo_WorkUnitDoesNotExist",

			sessions: []tmux.NewSessionOptions{
				{Name: "foo", StartDir: "testing/repo"},
			},
			current: tmux.NewSessionOptions{Name: "bar", StartDir: "testing/repo"},
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/", repotest.RepoConfig{
					Name: "repo",
					WorkUnits: map[string][]string{
						repotest.DefaultWorkUnitName: {"bar"},
					},
				}),
			},

			want: []tmux.MenuElement{
				tmux.MenuEntry{Name: "*bar", Key: "q"},
				tmux.MenuEntry{Name: "?foo", Key: "1"},
			},
		},
		{
			name: "MultipleRepos",

			sessions: []tmux.NewSessionOptions{
				{Name: "bar", StartDir: "testing/repo1"},
				{Name: "baz", StartDir: "testing/repo1"},
				{Name: "foo", StartDir: "testing/repo2"},
				{Name: "bar", StartDir: "testing/repo2"},
				{Name: "baz", StartDir: "testing/repo2"},
				{Name: "foo", StartDir: "someOtherDir"},
				{Name: "bar", StartDir: "someOtherDir"},
				{Name: "baz", StartDir: "someOtherDir"},
			},
			current: tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo1"},
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/", repotest.RepoConfig{
					Name:      "repo1",
					WorkUnits: map[string][]string{repotest.DefaultWorkUnitName: {"foo", "bar", "baz"}},
				}, repotest.RepoConfig{
					Name:      "repo2",
					WorkUnits: map[string][]string{repotest.DefaultWorkUnitName: {"foo", "bar", "baz"}},
				}),
			},

			want: []tmux.MenuElement{
				tmux.MenuEntry{Name: " repo1>bar", Key: "0"},
				tmux.MenuEntry{Name: " repo1>baz", Key: "1"},
				tmux.MenuEntry{Name: "*repo1>foo", Key: "q"},
				tmux.MenuSpacer{},
				tmux.MenuEntry{Name: " repo2>bar", Key: "3"},
				tmux.MenuEntry{Name: " repo2>baz", Key: "4"},
				tmux.MenuEntry{Name: " repo2>foo", Key: "5"},
				tmux.MenuSpacer{},
				tmux.MenuEntry{Name: " bar", Key: "6"},
				tmux.MenuEntry{Name: " baz", Key: "7"},
				tmux.MenuEntry{Name: " foo", Key: "8"},
			},
		},
		{
			name: "MultipleRepos_CurrentIsUnknown",

			sessions: []tmux.NewSessionOptions{
				{Name: "foo", StartDir: "testing/repo1"},
				{Name: "bar", StartDir: "testing/repo1"},
				{Name: "baz", StartDir: "testing/repo1"},
				{Name: "foo", StartDir: "testing/repo2"},
				{Name: "bar", StartDir: "testing/repo2"},
				{Name: "baz", StartDir: "testing/repo2"},
				{Name: "bar", StartDir: "someOtherDir"},
				{Name: "baz", StartDir: "someOtherDir"},
			},
			current: tmux.NewSessionOptions{Name: "foo", StartDir: "someOtherDir"},
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/", repotest.RepoConfig{
					Name:      "repo1",
					WorkUnits: map[string][]string{repotest.DefaultWorkUnitName: {"foo", "bar", "baz"}},
				}, repotest.RepoConfig{
					Name:      "repo2",
					WorkUnits: map[string][]string{repotest.DefaultWorkUnitName: {"foo", "bar", "baz"}},
				}),
			},

			want: []tmux.MenuElement{
				tmux.MenuEntry{Name: " bar", Key: "0"},
				tmux.MenuEntry{Name: " baz", Key: "1"},
				tmux.MenuEntry{Name: "*foo", Key: "q"},
				tmux.MenuSpacer{},
				tmux.MenuEntry{Name: " repo1>bar", Key: "3"},
				tmux.MenuEntry{Name: " repo1>baz", Key: "4"},
				tmux.MenuEntry{Name: " repo1>foo", Key: "5"},
				tmux.MenuSpacer{},
				tmux.MenuEntry{Name: " repo2>bar", Key: "6"},
				tmux.MenuEntry{Name: " repo2>baz", Key: "7"},
				tmux.MenuEntry{Name: " repo2>foo", Key: "8"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := tmuxtest.NewServer(i)
			for _, sesh := range tc.sessions {
				if _, err := srv.NewSession(sesh); err != nil {
					t.Errorf("tmux.NewSession(%#v) = _, %v", sesh, err)
				}
			}

			current, err := srv.NewSession(tc.current)
			if err != nil {
				t.Errorf("tmux.NewSession(%#v) = _, %v", tc.current, err)
			}

			got, err := createMenu(current, tc.vcs)
			if err != nil {
				t.Errorf("createMenu() = _, %v", err)
			}

			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(tmux.MenuEntry{}, "Command")); diff != "" {
				t.Errorf("createMenu() diff (-want +got)\n%s", diff)
			}
		})
	}
}
