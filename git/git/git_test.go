package git

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
	"github.com/JeffFaer/tmux-vcs-sync/api/repotest"
)

type testGitCmd struct {
	git gitCmd
}

func (git testGitCmd) Command(args ...string) *exec.Command {
	args = append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com"}, args...)
	cmd := git.git.Command(args...)
	cmd.Env = append(cmd.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG=/dev/null", "HOME=/dev/null")
	return cmd
}

func TestRepoAPI(t *testing.T) {
	git := testGitCmd{git}
	newGitRepo := func(name string) (api.Repository, error) {
		dir := t.TempDir()
		if err := git.Command("-C", dir, "init", name).Run(); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}
		dir = filepath.Join(dir, name)
		if err := git.Command("-C", dir, "commit", "--allow-empty", "--message", "Initial commit.").Run(); err != nil {
			return nil, fmt.Errorf("git initial commit: %v", err)
		}
		git.Command("-C", dir, "log").Run()
		return git.git.Repository(dir)
	}
	repotest.RepoTests(t, newGitRepo, repotest.Options{NoopRenameIsOK: true})
}
