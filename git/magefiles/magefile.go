//go:build mage
// +build mage

package main

import (
	//mage:import
	"github.com/JeffFaer/tmux-vcs-sync/api/magefiles/plugin"
)

func init() {
	plugin.Filename = "git.so"
}

var Default = plugin.Build
