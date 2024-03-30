package repotest

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api"
)

func TestFakeRepo(t *testing.T) {
	pre := "testing/"
	vcs := NewVCS(pre)
	newRepo := func(ctx context.Context, _ *testing.T, dir string) (api.Repository, error) {
		if !strings.HasPrefix(dir, pre) {
			dir = filepath.Join("testing", dir)
		}
		return vcs.Repository(ctx, dir)
	}
	RepoTests(t, newRepo, Options{})
}
