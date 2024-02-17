package git

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/api/repotest"
)

func TestRepoAPI(t *testing.T) {
	newGitRepo := func(name string) (api.Repository, error) {
		dir := t.TempDir()
		if err := git.Command("-C", dir, "init", name).Run(); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}
		dir = filepath.Join(dir, name)
		if err := git.Command("-C", dir, "commit", "--allow-empty", "--message", "Initial commit.").Run(); err != nil {
			return nil, fmt.Errorf("git initial commit: %v", err)
		}
		return git.Repository(dir)
	}
	repotest.RepoTests(t, newGitRepo, repotest.Options{NoopRenameIsOK: true})
}
