package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
	"github.com/JeffFaer/tmux-vcs-sync/api/exec/exectest"
	"github.com/JeffFaer/tmux-vcs-sync/api/repotest"
	"github.com/google/go-cmp/cmp"
	"github.com/kballard/go-shellquote"
)

const (
	defaultBranchName = "main"
)

type testGit struct {
	git
}

type testGitRepo struct {
	*gitRepo
}

type testGitCmd struct {
	git exec.Commander
}

func newGit(t *testing.T) testGit {
	t.Helper()
	exec, err := exec.Lookup("git")
	if err != nil {
		t.Fatal(err)
	}
	return testGit{git{testGitCmd{exectest.NewTestCommander(t, exec)}}}
}

func (git testGit) newRepo(dir string, name string) (*testGitRepo, error) {
	if err := git.Command("-C", dir, "init", name).Run(); err != nil {
		return nil, fmt.Errorf("git init: %w", err)
	}
	dir = filepath.Join(dir, name)
	repo, err := git.Repository(dir)
	if err != nil {
		return nil, err
	}
	gitRepo := &testGitRepo{repo.(*gitRepo)}
	gitRepo.gitRepo.git = git.git
	if err := gitRepo.addEmptyCommit("Initial commit."); err != nil {
		return nil, err
	}
	return gitRepo, nil
}

func (repo *testGitRepo) New(workUnitName string) error {
	if err := repo.gitRepo.New(workUnitName); err != nil {
		return err
	}
	return repo.addEmptyCommit("Initial commit for " + workUnitName)
}

func (repo *testGitRepo) Commit(workUnitName string) error {
	if err := repo.gitRepo.Commit(workUnitName); err != nil {
		return err
	}
	return repo.addEmptyCommit("Initial commit for " + workUnitName)
}

func (repo *testGitRepo) addEmptyCommit(msg string) error {
	if err := repo.gitRepo.Command("commit", "--allow-empty", "--message", msg).Run(); err != nil {
		return fmt.Errorf("git empty commit %q: %s", msg, err)
	}
	return nil
}

func (git testGitCmd) Command(args ...string) *exec.Command {
	args = append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", fmt.Sprintf("init.defaultBranch=%s", defaultBranchName)}, args...)
	cmd := git.git.Command(args...)
	cmd.Env = append(cmd.Environ(), "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null")
	return cmd
}

func TestRepoAPI(t *testing.T) {
	newGitRepo := func(t *testing.T, name string) (api.Repository, error) {
		git := newGit(t)
		dir := t.TempDir()
		return git.newRepo(dir, name)
	}
	repotest.RepoTests(t, newGitRepo, repotest.Options{
		NoopRenameIsOK:            true,
		ExtraListWorkUnitNames:    []string{"JeffFaer/slashed-branch-name"},
		ExtraListWorkUnitPrefixes: []repotest.ListWorkUnitTestCase{{Prefix: "JeffFaer*", Want: []string{"JeffFaer/slashed-branch-name"}}},
	})
}

func TestRepoName(t *testing.T) {
	configureName := func(name string) initStep {
		return repoCommand{args: []string{"config", "tmux-vcs-sync.name", name}}
	}
	setGitHubSSHURL := func(remote, repoName string) initStep {
		return repoCommand{args: []string{"remote", "add", remote, fmt.Sprintf("git@github.com:%s.git", repoName)}}
	}
	setGitHubHTTPURL := func(remote, repoName string) initStep {
		return repoCommand{args: []string{"remote", "add", remote, fmt.Sprintf("https://github.com/%s.git", repoName)}}
	}
	for _, tc := range []struct {
		name string

		init []initStep

		want string
	}{
		{
			name: "DefaultsToDirectoryName",
			want: "DefaultsToDirectoryName",
		},
		{
			name: "ExplicitConfig",

			init: []initStep{
				configureName("configured-name"),
				setGitHubSSHURL("origin", "JeffFaer/testing"),
			},

			want: "configured-name",
		},
		{
			name: "OriginSSHURL",

			init: []initStep{
				setGitHubSSHURL("origin", "JeffFaer/ssh-url"),
			},

			want: "ssh-url",
		},
		{
			name: "OriginHTTPURL",

			init: []initStep{
				setGitHubHTTPURL("origin", "JeffFaer/https-url"),
			},

			want: "https-url",
		},
		{
			name: "UnrecognizedOriginURL",

			init: []initStep{
				setGitHubHTTPURL("origin", "https-url"),
			},

			want: "UnrecognizedOriginURL",
		},
		{
			name: "OtherRemoteURL",

			init: []initStep{
				setGitHubHTTPURL("upstream", "JeffFaer/https-url"),
			},

			want: "OtherRemoteURL",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			git := newGit(t)
			repo, err := git.newRepo(t.TempDir(), tc.name)
			if err != nil {
				t.Fatalf("Could not create repo: %v", err)
			}

			initializeRepo(t, repo, tc.init)

			if n := repo.Name(); n != tc.want {
				t.Errorf("repo.Name() = %q, want %q", n, tc.want)
			}
		})
	}
}

func TestCurrent(t *testing.T) {
	checkoutNewBranch := func(name string, parent string) initStep {
		return repoCommand{args: []string{"checkout", "-b", name, parent}}
	}
	add := func(file string) initStep {
		return repoCommand{args: []string{"add", file}}
	}
	commit := func(msg string) initStep {
		return repoCommand{args: []string{"commit", "--message", msg}}
	}
	rebase := func(base string, errorMessage string) initStep {
		return repoCommand{args: []string{"rebase", base}, errorMessage: errorMessage}
	}
	for _, tc := range []struct {
		name string

		init []initStep

		want    string
		wantErr error
	}{
		{
			name: "Initial",
			want: defaultBranchName,
		},
		{
			name: "MidRebase",
			init: []initStep{
				checkoutNewBranch("branch1", defaultBranchName),
				newFile{"README", "abc"},
				add("README"),
				commit("branch1 commit"),
				checkoutNewBranch("branch2", defaultBranchName),
				newFile{"README", "def"},
				add("README"),
				commit("branch2 commit"),
				rebase("branch1", "Resolve all conflicts manually"),
			},
			wantErr: errUnstableRepoState,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			git := newGit(t)
			repo, err := git.newRepo(t.TempDir(), tc.name)
			if err != nil {
				t.Fatalf("Could not create repo: %v", err)
			}
			initializeRepo(t, repo, tc.init)

			got, err := repo.Current()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("repo.Current() = _, %v, wanted %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("repo.Current() = %q, _, wanted %q", got, tc.want)
			}
		})
	}
}

func TestSort_DuplicateBranch(t *testing.T) {
	git := newGit(t)
	repo, err := git.newRepo(t.TempDir(), t.Name())
	if err != nil {
		t.Fatalf("Could not create repo: %v", err)
	}
	addBranch := func(name string) initStep {
		return repoCommand{args: []string{"branch", name}}
	}
	commit := func(name string) initStep {
		return apiCommand{"commit " + name, func(repo *testGitRepo) error { return repo.Commit(name) }}
	}

	initializeRepo(t, repo, []initStep{addBranch("foo"), commit("bar")})

	branches := []string{"foo", defaultBranchName, "bar"}
	got := slices.Clone(branches)
	if err := repo.Sort(got); err != nil {
		t.Errorf("repo.Sort(%q) = %v", branches, err)
	}
	if diff := cmp.Diff([]string{"main", "foo", "bar"}, got); diff != "" {
		t.Errorf("repo.Sort(%q) diff (-want +got)\n%s", branches, diff)
	}
}

type initStep interface {
	Run(*testGitRepo) error
	String() string
}

type repoCommand struct {
	args         []string
	errorMessage string
}

func (cmd repoCommand) Run(repo *testGitRepo) error {
	repoCmd := repo.Command(cmd.args...)
	if cmd.errorMessage == "" {
		return repoCmd.Run()
	}
	orig := repoCmd.Stderr
	stderr, _ := repoCmd.RunStderr()
	if !strings.Contains(stderr, cmd.errorMessage) {
		fmt.Fprintln(orig, stderr)
		return fmt.Errorf("did not observe expected error message containing %q", cmd.errorMessage)
	}
	return nil
}

func (cmd repoCommand) String() string {
	return shellquote.Join(append([]string{"git"}, cmd.args...)...)
}

type newFile struct {
	path    string
	content string
}

func (cmd newFile) Run(repo *testGitRepo) error {
	return os.WriteFile(filepath.Join(repo.rootDir, cmd.path), []byte(cmd.content), 0600)
}

func (cmd newFile) String() string {
	return fmt.Sprintf("echo %q > %q", cmd.content, cmd.path)
}

type apiCommand struct {
	name string
	cmd  func(*testGitRepo) error
}

func (cmd apiCommand) Run(repo *testGitRepo) error {
	return cmd.cmd(repo)
}

func (cmd apiCommand) String() string {
	return cmd.name
}

func initializeRepo(t *testing.T, repo *testGitRepo, init []initStep) {
	t.Helper()
	for i, step := range init {
		if err := step.Run(repo); err != nil {
			t.Fatalf("Init step #%d: %s: %v", i+1, step, err)
		}
	}
}
