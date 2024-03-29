package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JeffFaer/tmux-vcs-sync/api/config"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var Filename string

func Build() error {
	fmt.Println("Building...")
	return sh.Run("go", "build", "-buildmode=plugin", "-o", Filename, ".")
}

func Test() error {
	fmt.Println("Testing...")
	return sh.Run("go", "test", "./...")
}

func Install() error {
	mg.Deps(Build)
	fmt.Println("Installing...")
	dir, err := config.PluginDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, Filename)
	return sh.Run("cp", Filename, dest)
}

func Clean() {
	fmt.Println("Cleaning...")
	os.RemoveAll(Filename)
}
