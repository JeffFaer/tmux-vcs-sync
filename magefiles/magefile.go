//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"

	"github.com/magefile/mage/sh"
)

var Default = Build

func Build() error {
	fmt.Println("Building...")
	return sh.Run("go", "build", "-o=tmux-vcs-sync", ".")
}

func Test() error {
	fmt.Println("Testing...")
	return sh.Run("go", "test", "./...")
}

func Install() error {
	fmt.Println("Installing...")
	return sh.Run("go", "install", ".")
}

func Clean() {
	fmt.Println("Cleaning...")
	os.RemoveAll("tmux-vcs-sync")
}
