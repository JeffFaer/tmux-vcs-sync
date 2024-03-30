package main

import (
	"context"
	"os"

	"github.com/JeffFaer/tmux-vcs-sync/cmd"
)

func main() {
	if err := cmd.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}
