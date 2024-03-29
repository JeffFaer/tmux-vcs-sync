package config

import (
	"fmt"
	"os"

	"github.com/adrg/xdg"
)

func PluginDir() (string, error) {
	pluginDir, err := xdg.ConfigFile("tmux-vcs-sync/vcs")
	if err != nil {
		return "", fmt.Errorf("could not find any VCS: %w", err)
	}
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return "", err
	}
	return pluginDir, nil
}
