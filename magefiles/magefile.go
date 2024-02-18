//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var Default = Build.Build

var Aliases = map[string]interface{}{
	"build":   Build.Build,
	"install": Install.Install,
}

type Build mg.Namespace
type Install mg.Namespace

func (Build) Build() error {
	fmt.Println("Building...")
	return sh.Run("go", "build", "-o=tmux-vcs-sync", ".")
}

func (Build) Completion(shell string) error {
	mg.Deps(Build.Build)
	fmt.Printf("Generating %s completion...\n", shell)
	f, err := os.OpenFile(fmt.Sprintf("tvs_completion.%s", shell), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := sh.Exec(nil, f, os.Stderr, "tmux-vcs-sync", "completion", shell); err != nil {
		return err
	}
	return f.Close()
}

func Test() error {
	fmt.Println("Testing...")
	return sh.Run("go", "test", "./...")
}

func (Install) Install() error {
	fmt.Println("Installing...")
	return sh.Run("go", "install", ".")
}

func (Install) Completion(shell string) error {
	mg.Deps(mg.F(Build.Completion, shell))
	dir, err := determineCompletionDir(shell)
	if err != nil {
		return err
	}
	fmt.Printf("Installing %s completion to %s...\n", shell, dir)
	name := fmt.Sprintf("tvs_completion.%s", shell)
	return sh.Run("cp", name, filepath.Join(dir, name))
}

func determineCompletionDir(shell string) (string, error) {
	if shell != "bash" {
		return "", fmt.Errorf("the author only uses bash so they don't know where other shell completion files are supposed to go")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	for _, dir := range []string{
		filepath.Join(home, "bash_completion.d"),
		filepath.Join(home, "bashrc.d/bash_completion.d"),
		"/etc/bash_completion.d",
	} {
		if stat, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		} else if stat.IsDir() {
			return dir, nil
		}
	}

	return "", fmt.Errorf("did not find any appropriate directory to install %s completions", shell)
}

func Clean() {
	fmt.Println("Cleaning...")
	os.RemoveAll("tmux-vcs-sync")
}
