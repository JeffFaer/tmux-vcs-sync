package repotest

import "testing"

func TestFakeRepo(t *testing.T) {
	RepoTests(t, VCS.Repository, Options{})
}
