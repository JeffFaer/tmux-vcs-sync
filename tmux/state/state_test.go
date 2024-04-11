package state

import (
	stdcmp "cmp"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/JeffFaer/go-stdlib-ext/morecmp"
	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/api/repotest"
	"github.com/JeffFaer/tmux-vcs-sync/tmux"
	"github.com/JeffFaer/tmux-vcs-sync/tmux/tmuxtest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNew(t *testing.T) {
	for _, tc := range []struct {
		name string

		tmux tmux.Server
		vcs  api.VersionControlSystems

		want simplifiedState
	}{
		{
			name: "EmptyServer",
			tmux: newServer(),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			want: simplifiedState{},
		},
		{
			name: "SingleRepo",
			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo"},
				tmux.NewSessionOptions{Name: "bar", StartDir: "testing/repo"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "foo"},
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo"},
				Repos:            []RepoName{{Repo: "repo"}},
			},
		},
		{
			name: "UnknownSessions",
			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo"},
				tmux.NewSessionOptions{Name: "bar", StartDir: "someOtherDir"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "foo"},
				},
				UnqualifiedRepos: []string{"repo"},
				Repos:            []RepoName{{Repo: "repo"}},
				UnknownSessions:  []string{"bar"},
			},
		},
		{
			name: "MultipleRepos_UnqualifiedNames",
			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo1"},
				tmux.NewSessionOptions{Name: "bar", StartDir: "testing/repo2"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo1"}, WorkUnit: "foo"},
					{RepoName: RepoName{Repo: "repo2"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo1", "repo2"},
				Repos:            []RepoName{{Repo: "repo1"}, {Repo: "repo2"}},
			},
		},
		{
			name: "MultipleRepos_QualifiedNames",
			tmux: newServer(
				tmux.NewSessionOptions{Name: "repo1>foo", StartDir: "testing/repo1"},
				tmux.NewSessionOptions{Name: "repo2>bar", StartDir: "testing/repo2"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo1"}, WorkUnit: "foo"},
					{RepoName: RepoName{Repo: "repo2"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo1", "repo2"},
				Repos:            []RepoName{{Repo: "repo1"}, {Repo: "repo2"}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			st, err := New(ctx, tc.tmux, tc.vcs)
			if err != nil {
				t.Errorf("New() = _, %v", err)
			}
			if diff := cmp.Diff(tc.want, simplifyState(t, st), compareSimplifiedStates, cmpopts.IgnoreFields(RepoName{}, "VCS")); diff != "" {
				t.Errorf("State diff (-want +got)\n%s", diff)
			}
		})
	}
}

func TestNewSession(t *testing.T) {
	for _, tc := range []struct {
		name string

		tmux tmux.Server
		vcs  api.VersionControlSystems

		repoDir, workUnitName string

		want     simplifiedState
		wantTmux simplifiedTmuxState
		wantErr  bool
	}{
		{
			name: "Empty",

			tmux: newServer(),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir:      "testing/repo",
			workUnitName: "foo",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "foo"},
				},
				UnqualifiedRepos: []string{"repo"},
				Repos: []RepoName{
					{Repo: "repo"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "foo",
						Dir:  "testing/repo",
					},
				},
			},
		},
		{
			name: "SingleRepo",

			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir:      "testing/repo",
			workUnitName: "bar",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "foo"},
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo"},
				Repos: []RepoName{
					{Repo: "repo"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "foo",
						Dir:  "testing/repo",
					},
					{
						Name: "bar",
						Dir:  "testing/repo",
					},
				},
			},
		},
		{
			name: "MultipleRepos",

			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo1"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir:      "testing/repo2",
			workUnitName: "bar",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo1"}, WorkUnit: "foo"},
					{RepoName: RepoName{Repo: "repo2"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo1", "repo2"},
				Repos: []RepoName{
					{Repo: "repo1"},
					{Repo: "repo2"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "repo1>foo",
						Dir:  "testing/repo1",
					},
					{
						Name: "repo2>bar",
						Dir:  "testing/repo2",
					},
				},
			},
		},
		{
			name: "SessionAlreadyExists_Error",

			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir:      "testing/repo",
			workUnitName: "foo",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "foo"},
				},
				UnqualifiedRepos: []string{"repo"},
				Repos: []RepoName{
					{Repo: "repo"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "foo",
						Dir:  "testing/repo",
					},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			st, err := New(ctx, tc.tmux, tc.vcs)
			if err != nil {
				t.Fatalf("New() = _, %v", err)
			}
			repo, err := tc.vcs.MaybeFindRepository(ctx, tc.repoDir)
			if err != nil {
				t.Fatalf("MaybeFindRepository(%q) = _, %v", tc.repoDir, err)
			}
			if repo == nil {
				t.Fatalf("tc.repoDir did not yield a repository")
			}

			if _, err := st.NewSession(ctx, repo, tc.workUnitName); (err != nil) != tc.wantErr {
				t.Errorf("NewSession(%q, %q) = %v, wantErr %t", tc.repoDir, tc.workUnitName, err, tc.wantErr)
			}

			if diff := cmp.Diff(tc.want, simplifyState(t, st), compareSimplifiedStates, cmpopts.IgnoreFields(RepoName{}, "VCS")); diff != "" {
				t.Errorf("State diff (-want +got)\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantTmux, simplifyTmuxState(ctx, tc.tmux), compareSimplifiedTmuxState); diff != "" {
				t.Errorf("tmux diff (-want +got)\n%s", diff)
			}
		})
	}
}

func TestRename(t *testing.T) {
	for _, tc := range []struct {
		name string

		tmux tmux.Server
		vcs  api.VersionControlSystems

		repoDir, old, new string

		want     simplifiedState
		wantTmux simplifiedTmuxState
		wantErr  bool
	}{
		{
			name: "Simple",

			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir: "testing/repo",
			old:     "foo",
			new:     "bar",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo"},
				Repos: []RepoName{
					{Repo: "repo"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "bar",
						Dir:  "testing/repo",
					},
				},
			},
		},
		{
			name: "MultipleRepos_UnqualifiedNames",

			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo1"},
				tmux.NewSessionOptions{Name: "bar", StartDir: "testing/repo2"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir: "testing/repo1",
			old:     "foo",
			new:     "baz",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo1"}, WorkUnit: "baz"},
					{RepoName: RepoName{Repo: "repo2"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo1", "repo2"},
				Repos: []RepoName{
					{Repo: "repo1"},
					{Repo: "repo2"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "repo1>baz",
						Dir:  "testing/repo1",
					},
					{
						Name: "repo2>bar",
						Dir:  "testing/repo2",
					},
				},
			},
		},
		{
			name: "MultipleRepos_QualifiedNames",

			tmux: newServer(
				tmux.NewSessionOptions{Name: "repo1>foo", StartDir: "testing/repo1"},
				tmux.NewSessionOptions{Name: "repo2>bar", StartDir: "testing/repo2"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir: "testing/repo1",
			old:     "repo1>foo",
			new:     "baz",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo1"}, WorkUnit: "baz"},
					{RepoName: RepoName{Repo: "repo2"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo1", "repo2"},
				Repos: []RepoName{
					{Repo: "repo1"},
					{Repo: "repo2"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "repo1>baz",
						Dir:  "testing/repo1",
					},
					{
						Name: "repo2>bar",
						Dir:  "testing/repo2",
					},
				},
			},
		},
		{
			name: "OldDoesNotExist_Error",

			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir: "testing/repo",
			old:     "bar",
			new:     "foo",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "foo"},
				},
				UnqualifiedRepos: []string{"repo"},
				Repos: []RepoName{
					{Repo: "repo"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "foo",
						Dir:  "testing/repo",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "NewAlreadyExists_Error",

			tmux: newServer(
				tmux.NewSessionOptions{Name: "foo", StartDir: "testing/repo"},
				tmux.NewSessionOptions{Name: "bar", StartDir: "testing/repo"},
			),
			vcs: api.VersionControlSystems{
				repotest.NewVCS("testing/"),
			},

			repoDir: "testing/repo",
			old:     "foo",
			new:     "bar",

			want: simplifiedState{
				WorkUnits: []WorkUnitName{
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "foo"},
					{RepoName: RepoName{Repo: "repo"}, WorkUnit: "bar"},
				},
				UnqualifiedRepos: []string{"repo"},
				Repos: []RepoName{
					{Repo: "repo"},
				},
			},
			wantTmux: simplifiedTmuxState{
				Sessions: []simplifiedSessionState{
					{
						Name: "foo",
						Dir:  "testing/repo",
					},
					{
						Name: "bar",
						Dir:  "testing/repo",
					},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			st, err := New(ctx, tc.tmux, tc.vcs)
			if err != nil {
				t.Fatalf("New() = _, %v", err)
			}
			repo, err := tc.vcs.MaybeFindRepository(ctx, tc.repoDir)
			if err != nil {
				t.Fatalf("MaybeFindRepository(%q) = _, %v", tc.repoDir, err)
			}
			if repo == nil {
				t.Fatalf("tc.repoDir did not yield a repository")
			}

			if err := st.RenameSession(ctx, repo, tc.old, tc.new); (err != nil) != tc.wantErr {
				t.Errorf("RenameSession(%q, %q, %q) = %v, wantErr = %t", tc.repoDir, tc.old, tc.new, err, tc.wantErr)
			}

			if diff := cmp.Diff(tc.want, simplifyState(t, st), compareSimplifiedStates, cmpopts.IgnoreFields(RepoName{}, "VCS")); diff != "" {
				t.Errorf("State diff (-want +got)\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantTmux, simplifyTmuxState(ctx, tc.tmux), compareSimplifiedTmuxState); diff != "" {
				t.Errorf("tmux diff (-want +got)\n%s", diff)
			}
		})
	}
}

type simplifiedState struct {
	WorkUnits        []WorkUnitName
	UnqualifiedRepos []string
	Repos            []RepoName
	UnknownSessions  []string
}

var repoCmp = morecmp.Comparing(func(n RepoName) string { return n.VCS }).
	AndThen(morecmp.Comparing(func(n RepoName) string { return n.Repo }))

var compareSimplifiedStates = cmp.Options{
	cmpopts.SortSlices(morecmp.ComparingFunc(func(n WorkUnitName) RepoName { return n.RepoName }, repoCmp).
		AndThen(morecmp.Comparing(func(n WorkUnitName) string { return n.WorkUnit })).
		LessFunc()),
	cmpopts.SortSlices(stdcmp.Less[string]),
	cmpopts.SortSlices(repoCmp.LessFunc()),
}

func simplifyState(t *testing.T, st *State) simplifiedState {
	t.Helper()
	var ret simplifiedState
	for n, sesh := range st.sessionsByName {
		ret.WorkUnits = append(ret.WorkUnits, n)

		if wu, ok := st.sessionsByID[sesh.sesh.ID()]; !ok || wu.name() != n {
			t.Errorf("sessionsByID[%q] = %q, %t, expected %q", sesh.sesh.ID(), wu.name(), ok, n)
		}
	}
	for id, wu := range st.sessionsByID {
		n := wu.name()
		if _, ok := st.sessionsByName[n]; !ok {
			t.Errorf("sessionsByName[%q] is missing, expected sesh %q", n, id)
		}
	}
	for n := range st.unqualifiedRepos {
		ret.UnqualifiedRepos = append(ret.UnqualifiedRepos, n)
	}
	for n := range st.repos {
		ret.Repos = append(ret.Repos, n)
	}
	for n := range st.unknownSessions {
		ret.UnknownSessions = append(ret.UnknownSessions, n)
	}
	return ret
}

type simplifiedTmuxState struct {
	Sessions []simplifiedSessionState
}

type simplifiedSessionState struct {
	ID   string
	Name string
	Dir  string
}

var compareSimplifiedTmuxState = cmp.Options{
	cmpopts.IgnoreFields(simplifiedSessionState{}, "ID"),
	cmpopts.SortSlices(morecmp.Comparing(func(s simplifiedSessionState) string { return s.ID }).LessFunc()),
}

func simplifyTmuxState(ctx context.Context, srv tmux.Server) simplifiedTmuxState {
	var ret simplifiedTmuxState
	for sesh, props := range must(must(srv.ListSessions(ctx)).Properties(ctx, tmux.SessionName, tmux.SessionPath)) {
		ret.Sessions = append(ret.Sessions, simplifiedSessionState{
			ID:   sesh.ID(),
			Name: props[tmux.SessionName],
			Dir:  props[tmux.SessionPath],
		})
	}
	return ret
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

var pid = 1

func newServer(opts ...tmux.NewSessionOptions) tmux.Server {
	srv := tmuxtest.NewServer(pid)
	pid++
	for _, opts := range opts {
		_, err := srv.NewSession(context.Background(), opts)
		if err != nil {
			panic(fmt.Errorf("srv.NewSession(%#v) = %w", opts, err))
		}
	}
	return srv
}
