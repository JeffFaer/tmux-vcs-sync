package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/phsym/console-slog"
	"github.com/spf13/cobra"
)

var (
	verbosity int
	levels    = []slog.Level{
		slog.LevelWarn,
		slog.LevelInfo,
		slog.LevelDebug,
	}
)

var rootCmd = &cobra.Command{
	Use:          "tmux-vcs-sync",
	Short:        "Synchronize VCS state with tmux state.",
	SilenceUsage: true,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if par := cmd.Parent(); par != nil && par.Name() == "completion" {
			return nil
		}
		slog.SetDefault(slog.New(console.NewHandler(os.Stderr, &console.HandlerOptions{
			Level:      levels[min(verbosity, len(levels)-1)],
			TimeFormat: time.RFC3339,
		})))

		pluginDir, err := xdg.ConfigFile("tmux-vcs-sync/vcs")
		if err != nil {
			return fmt.Errorf("could not find any VCS: %w", err)
		}
		if err := os.MkdirAll(pluginDir, 0700); err != nil {
			return err
		}
		des, err := os.ReadDir(pluginDir)
		if err != nil {
			return fmt.Errorf("could not read VCS dir: %w", err)
		}
		var loaded int
		var errs []error
		for _, de := range des {
			if de.IsDir() {
				continue
			}
			if strings.HasSuffix(de.Name(), ".so") {
				path := filepath.Join(pluginDir, de.Name())
				if _, err := plugin.Open(path); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", path, err))
				} else {
					loaded++
				}
			}
		}
		if loaded == 0 {
			if len(errs) == 0 {
				return fmt.Errorf("add VCS libraries to %s", pluginDir)
			}
			return errors.Join(errs...)
		}

		for _, err := range errs {
			slog.Warn("An error occurred loading a VCS.", "error", err)
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "Log more verbosely.")
}

func Execute() error {
	return rootCmd.Execute()
}
