//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
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
	return os.WriteFile(fmt.Sprintf("tmux-vcs-sync.%s", shell), []byte(fmt.Sprintf("source <(tmux-vcs-sync completion %q)", shell)), 0644)
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
	name := fmt.Sprintf("tmux-vcs-sync.%s", shell)
	return sh.Run("cp", name, filepath.Join(dir, name))
}

func determineCompletionDir(shell string) (string, error) {
	if shell != "bash" {
		return "", fmt.Errorf("the author only uses bash so they don't know where other shell completion files are supposed to go")
	}
	// /usr/share/bash-completion/bash_completion checks
	// ${BASH_COMPLETION_USER_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/bash-completion}/completions
	dir := os.Getenv("BASH_COMPLETION_USER_DIR")
	if dir == "" {
		dir = filepath.Join(xdg.DataHome, "bash-completion")
	}
	dir = filepath.Join(dir, "completions")

	if err := os.MkdirAll(dir, 0775); err != nil {
		return "", err
	}
	return dir, nil
}

func Clean() {
	fmt.Println("Cleaning...")
	os.RemoveAll("tmux-vcs-sync")
}
