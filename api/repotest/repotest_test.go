package repotest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
)

func TestFakeRepo(t *testing.T) {
	pre := "testing/"
	vcs := NewVCS(pre)
	newRepo := func(_ *testing.T, dir string) (api.Repository, error) {
		if !strings.HasPrefix(dir, pre) {
			dir = filepath.Join("testing", dir)
		}
		return vcs.Repository(dir)
	}
	RepoTests(t, newRepo, Options{})
}
