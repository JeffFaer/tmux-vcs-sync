package git

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/JeffFaer/go-stdlib-ext/morecmp"
	"github.com/JeffFaer/tmux-vcs-sync/api"
	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
)

var errUnstableRepoState = fmt.Errorf("unable to determine branch name (is the repo in an unstable state?)")

func init() {
	if exec, err := exec.Lookup("git"); err != nil {
		slog.Warn("Could not find git.", "error", err)
	} else {
		api.Register(git{exec})
	}
}

type git struct {
	exec.Commander
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
	return &gitRepo{git: git, rootDir: root}, nil
}

func (git git) rootDir(cwd string) (string, error) {
	return git.Command("-C", cwd, "rev-parse", "--show-toplevel").RunStdout()
}

type gitRepo struct {
	git
	rootDir string

	name string
}

func (repo *gitRepo) Command(args ...string) *exec.Command {
	args = append([]string{"-C", repo.rootDir}, args...)
	return repo.git.Command(args...)
}

func (repo *gitRepo) VCS() api.VersionControlSystem {
	return repo.git
}

func (repo *gitRepo) Name() string {
	if repo.name != "" {
		return repo.name
	}
	if n := repo.repoNameOverride(); n != "" {
		repo.name = n
	} else {
		repo.name = filepath.Base(repo.rootDir)
	}
	return repo.name
}

func (repo *gitRepo) repoNameOverride() string {
	for _, strat := range []func() string{
		repo.checkExplicitRepoName,
		repo.parseOriginURL,
	} {
		if n := strat(); n != "" {
			return n
		}
	}
	return ""
}

var urlRegexes = []*regexp.Regexp{
	regexp.MustCompile("^git@github.com:[^/]+/(.+).git$"),
	regexp.MustCompile("^https://github.com/[^/]+/(.+).git$"),
}

func (repo *gitRepo) checkExplicitRepoName() string {
	n, err := repo.configValue("tmux-vcs-sync.name")
	if err != nil {
		return ""
	}
	return n
}

func (repo *gitRepo) parseOriginURL() string {
	url, stderr, err := repo.Command("remote", "get-url", "origin").RunOutput()
	if err != nil {
		if strings.Contains(stderr, "No such remote") {
			return ""
		}
		fmt.Fprintln(os.Stderr, stderr)
		return ""
	}
	for _, regex := range urlRegexes {
		m := regex.FindStringSubmatch(url)
		if m != nil && m[1] != "" {
			return m[1]
		}
	}
	return ""
}

func (repo *gitRepo) RootDir() string {
	return repo.rootDir
}

func (repo *gitRepo) Current() (string, error) {
	cur, err := repo.Command("rev-parse", "--abbrev-ref", "HEAD").RunStdout()
	if err != nil {
		return "", err
	}
	if cur == "HEAD" {
		return "", errUnstableRepoState
	}
	return cur, nil
}

func (repo *gitRepo) List(prefix string) ([]string, error) {
	args := []string{"branch", "--format=%(refname:short)", "--list"}
	if prefix != "" {
		args = append(args, prefix+"*")
	}
	stdout, err := repo.Command(args...).RunStdout()
	if err != nil {
		return nil, err
	}
	return strings.Split(stdout, "\n"), nil
}

func (repo *gitRepo) Sort(workUnits []string) error {
	if len(workUnits) == 0 {
		return nil
	}

	branchesByHash, err := repo.keyBranchByHash(workUnits)
	if err != nil {
		return err
	}
	slog.Debug("Found hashes for branches.", "hashes", branchesByHash)

	args := []string{"rev-list", "--topo-order", "--reverse"}
	// We're reversing the output of rev-list, which will use its command line for
	// tie breakers. So reverse the order of our work units so that they'll be
	// sorted correctly in the output.
	slices.SortFunc(workUnits, morecmp.CmpFunc[string](cmp.Compare[string]).Reversed())
	args = append(args, workUnits...)
	cmd := repo.Command(args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start topological sorting: %w", err)
	}
	var i int
	r := bufio.NewReader(stdout)
	for i < len(workUnits) {
		hash, err := r.ReadString('\n')
		if hash != "" {
			hash = strings.TrimSuffix(hash, "\n")
			for _, b := range branchesByHash[hash] {
				workUnits[i] = b
				i++
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			err = errors.Join(fmt.Errorf("error during topological sorting: %w", err), cmd.Process.Kill())
			return err
		}
	}
	if n := len(workUnits); i != n {
		found := make(map[string]bool)
		for _, wu := range workUnits[:i] {
			found[wu] = true
		}
		var missing []string
		for _, branches := range branchesByHash {
			for _, b := range branches {
				if !found[b] {
					missing = append(missing, b)
				}
			}
		}
		return fmt.Errorf("only able to topologically sort %d of %d branches: unsortable branches: %q", i, n, missing)
	}
	if err := cmd.Process.Kill(); err != nil {
		slog.Warn("Problem killing rev-list command early.", "error", err)
	}

	// Move the default branch up top.
	defaultBranch, err := repo.defaultBranchName()
	if err != nil {
		return err
	}
	isDefault := func(name string) bool { return name == defaultBranch }
	slices.SortStableFunc(workUnits, morecmp.ComparingFunc(isDefault, morecmp.TrueFirst()))

	return nil
}

func (repo *gitRepo) keyBranchByHash(branches []string) (map[string][]string, error) {
	if len(branches) == 0 {
		return nil, nil
	}
	args := []string{"branch", "--list", "--format=%(refname:short) %(objectname)"}
	args = append(args, branches...)
	stdout, err := repo.Command(args...).RunStdout()
	if err != nil {
		return nil, fmt.Errorf("could not get branch hashes: %w", err)
	}
	ret := make(map[string][]string)
	for _, line := range strings.Split(stdout, "\n") {
		if line == "" {
			break
		}
		sp := strings.Split(line, " ")
		ret[sp[1]] = append(ret[sp[1]], sp[0])
	}
	return ret, nil
}

func (repo *gitRepo) New(workUnitName string) error {
	n, err := repo.defaultBranchName()
	if err != nil {
		return err
	}
	return repo.Command("checkout", "-b", workUnitName, n).Run()
}

// defaultBranch name attempts to determine the default branch name of this repository.
func (repo *gitRepo) defaultBranchName() (string, error) {
	def, err := repo.configValue("init.defaultBranch")
	if err != nil {
		slog.Warn("Could not determine init.defaultBranch preference.", "error", err)
		def = "master"
	}
	for _, n := range slices.Compact([]string{def, "master"}) {
		if repo.branchExists(n) {
			slog.Debug("Found default branch name.", "name", n)
			return n, nil
		}
	}
	return "", fmt.Errorf("could not determine default branch")
}

func (repo *gitRepo) configValue(key string) (string, error) {
	stdout, stderr, err := repo.Command("config", key).RunOutput()
	if err != nil {
		if stderr == "" {
			return "", nil
		}
		fmt.Fprintln(os.Stderr, stderr)
		return "", err
	}
	return stdout, nil
}

// branchExists determines whether a branch exists in the this repository.
func (repo *gitRepo) branchExists(name string) bool {
	err := repo.Command("show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", name)).Run()
	return err == nil
}

func (repo *gitRepo) Commit(workUnitName string) error {
	return repo.Command("checkout", "-b", workUnitName).Run()
}

func (repo *gitRepo) Rename(workUnitName string) error {
	return repo.Command("branch", "-m", workUnitName).Run()
}

func (repo *gitRepo) Exists(workUnitName string) (bool, error) {
	return repo.branchExists(workUnitName), nil
}

func (repo *gitRepo) Update(workUnitName string) error {
	return repo.Command("checkout", workUnitName).Run()
}
