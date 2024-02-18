package git

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
	"github.com/JeffFaer/tmux-vcs-sync/api/repotest"
	"github.com/kballard/go-shellquote"
)

type testGit struct {
	git
}

func newGit(t *testing.T) testGit {
	t.Helper()
	exec, err := exec.Lookup("git")
	if err != nil {
		t.Fatal(err)
	}
	return testGit{git{testGitCmd{t, exec}}}
}

func (git testGit) newRepo(dir string, name string) (*gitRepo, error) {
	if err := git.Command("-C", dir, "init", name).Run(); err != nil {
		return nil, fmt.Errorf("git init: %w", err)
	}
	dir = filepath.Join(dir, name)
	if err := git.Command("-C", dir, "commit", "--allow-empty", "--message", "Initial commit.").Run(); err != nil {
		return nil, fmt.Errorf("git initial commit: %v", err)
	}
	repo, err := git.Repository(dir)
	if err != nil {
		return nil, err
	}
	return repo.(*gitRepo), nil
}

type testGitCmd struct {
	t   testing.TB
	git commander
}

func (git testGitCmd) Command(args ...string) *exec.Command {
	args = append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "init.defaultBranch=main"}, args...)
	cmd := git.git.Command(args...)
	cmd.Env = append(cmd.Environ(), "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null")
	w := testingWriter{t: git.t}
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd
}

type testingWriter struct {
	t testing.TB
	exec.OverrideableWriter
}

func (w testingWriter) Write(b []byte) (int, error) {
	w.t.Log(string(b))
	return len(b), nil
}

func TestRepoAPI(t *testing.T) {
	testGit := newGit(t)
	newGitRepo := func(name string) (api.Repository, error) {
		dir := t.TempDir()
		return testGit.newRepo(dir, name)
	}
	repotest.RepoTests(t, newGitRepo, repotest.Options{NoopRenameIsOK: true})
}

func TestRepoName(t *testing.T) {
	configureName := func(name string) []string {
		return []string{"config", "tmux-vcs-sync.name", name}
	}
	setGitHubSSHURL := func(remote, repoName string) []string {
		return []string{"remote", "add", remote, fmt.Sprintf("git@github.com:%s.git", repoName)}
	}
	setGitHubHTTPURL := func(remote, repoName string) []string {
		return []string{"remote", "add", remote, fmt.Sprintf("https://github.com/%s.git", repoName)}
	}
	for _, tc := range []struct {
		name string

		initCommands [][]string

		want string
	}{
		{
			name: "DefaultsToDirectoryName",
			want: "DefaultsToDirectoryName",
		},
		{
			name: "ExplicitConfig",

			initCommands: [][]string{
				configureName("configured-name"),
				setGitHubSSHURL("origin", "JeffFaer/testing"),
			},

			want: "configured-name",
		},
		{
			name: "OriginSSHURL",

			initCommands: [][]string{
				setGitHubSSHURL("origin", "JeffFaer/ssh-url"),
			},

			want: "ssh-url",
		},
		{
			name: "OriginHTTPURL",

			initCommands: [][]string{
				setGitHubHTTPURL("origin", "JeffFaer/https-url"),
			},

			want: "https-url",
		},
		{
			name: "UnrecognizedOriginURL",

			initCommands: [][]string{
				setGitHubHTTPURL("origin", "https-url"),
			},

			want: "UnrecognizedOriginURL",
		},
		{
			name: "OtherRemoteURL",

			initCommands: [][]string{
				setGitHubHTTPURL("upstream", "JeffFaer/https-url"),
			},

			want: "OtherRemoteURL",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testGit := newGit(t)
			repo, err := testGit.newRepo(t.TempDir(), tc.name)
			if err != nil {
				t.Fatalf("Could not create repository: %v", err)
			}

			for i, args := range tc.initCommands {
				cmd := repo.Command(args...)
				if err := cmd.Run(); err != nil {
					t.Fatalf("Init step #%d: %s: %v", i+1, shellquote.Join(cmd.Args...), err)
				}
			}

			if n := repo.Name(); n != tc.want {
				t.Errorf("repo.Name() = %q, want %q", n, tc.want)
			}
		})
	}
}
