package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

func mkdir(name string) (string, error) {
	dir, err := xdg.ConfigFile(filepath.Join("tmux-vcs-sync", name))
	if err != nil {
		return "", fmt.Errorf("could not find configuration directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func PluginDir() (string, error) {
	return mkdir("vcs")
}

func TraceDir() (string, error) {
	return mkdir("trace")
}
