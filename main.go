package main

import (
	"os"

	"github.com/JeffFaer/tmux-vcs-sync/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
