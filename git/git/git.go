package git

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
)

func init() {
	if exec, err := exec.Lookup("git"); err != nil {
		slog.Warn("Could not find git.", "error", err)
	} else {
		api.Register(git{exec})
	}
}

type commander interface {
	Command(args ...string) *exec.Command
}

type git struct {
	commander
}

func (git) Name() string {
	return "git"
}

func (git) WorkUnitName() string {
	return "branch"
}

func (git git) Repository(dir string) (api.Repository, error) {
	cmd := git.Command("-C", dir, "ls-files", "--error-unmatch")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, nil
	}
	root, err := git.rootDir(dir)
	if err != nil {
		return nil, err
	}
	name := git.repoName(root)
	return gitRepo{git: git, name: name, rootDir: root}, nil
}

func (git git) rootDir(cwd string) (string, error) {
	return git.Command("-C", cwd, "rev-parse", "--show-toplevel").RunStdout()
}

func (git git) repoName(root string) string {
	for _, strat := range []func() (string, bool){
		func() (string, bool) { return git.checkExplicitRepoName(root) },
		func() (string, bool) { return git.parseOriginURL(root) },
	} {
		if n, ok := strat(); ok {
			return n
		}
	}
	return filepath.Base(root)
}

var urlRegexes = []*regexp.Regexp{
	regexp.MustCompile("^git@github.com:[^/]+/(.+).git$"),
}

func (git git) checkExplicitRepoName(root string) (string, bool) {
	n, stderr, err := git.Command("-C", root, "config", "tmux-vcs-sync.name").RunOutput()
	if err != nil {
		if len(stderr) == 0 {
			return "", false
		}
		fmt.Fprint(os.Stderr, stderr)
		return "", false
	}
	return n, true
}

func (git git) parseOriginURL(root string) (string, bool) {
	url, stderr, err := git.Command("-C", root, "remote", "get-url", "origin").RunOutput()
	if err != nil {
		if strings.Contains(stderr, "No such remote") {
			return "", false
		}
		fmt.Fprint(os.Stderr, stderr)
		return "", false
	}
	for _, regex := range urlRegexes {
		m := regex.FindStringSubmatch(url)
		if m != nil {
			return m[1], true
		}
	}
	return "", false
}

type gitRepo struct {
	git
	name    string
	rootDir string
}

func (repo gitRepo) Command(args ...string) *exec.Command {
	args = append([]string{"-C", repo.rootDir}, args...)
	return repo.git.Command(args...)
}

func (repo gitRepo) VCS() api.VersionControlSystem {
	return repo.git
}

func (repo gitRepo) Name() string {
	return repo.name
}

func (repo gitRepo) RootDir() string {
	return repo.rootDir
}

func (repo gitRepo) Current() (string, error) {
	return repo.Command("rev-parse", "--abbrev-ref", "HEAD").RunStdout()
}

func (repo gitRepo) New(workUnitName string) error {
	n, err := repo.defaultBranchName()
	if err != nil {
		return err
	}
	return repo.Command("checkout", "-b", workUnitName, n).Run()
}

// defaultBranch name attempts to determine the default branch name of this repository.
func (repo gitRepo) defaultBranchName() (string, error) {
	def, err := repo.configValue("init.defaultBranch")
	if err != nil {
		slog.Warn("Could not determine init.defaultBranch preference.", "error", err)
		def = "master"
	}
	for _, n := range slices.Compact([]string{def, "master"}) {
		if repo.branchExists(n) {
			return n, nil
		}
	}
	return "", fmt.Errorf("could not determine default branch")
}

func (repo gitRepo) configValue(key string) (string, error) {
	return repo.Command("config", key).RunStdout()
}

// branchExists determines whether a branch exists in the this repository.
func (repo gitRepo) branchExists(name string) bool {
	err := repo.Command("show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", name)).Run()
	return err == nil
}

func (repo gitRepo) Commit(workUnitName string) error {
	return repo.Command("checkout", "-b", workUnitName).Run()
}

func (repo gitRepo) Rename(workUnitName string) error {
	return repo.Command("branch", "-m", workUnitName).Run()
}

func (repo gitRepo) Exists(workUnitName string) (bool, error) {
	return repo.branchExists(workUnitName), nil
}

func (repo gitRepo) Update(workUnitName string) error {
	return repo.Command("checkout", workUnitName).Run()
}
