package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"plugin"
	"runtime/trace"
	"slices"
	"strings"
	"time"

	"github.com/JeffFaer/tmux-vcs-sync/api/config"
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

	doTrace   bool
	traceFile *os.File
	traceTask *trace.Task
)

var rootCmd = &cobra.Command{
	Use:          "tmux-vcs-sync",
	Short:        "Synchronize VCS state with tmux state.",
	SilenceUsage: true,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if par := cmd.Parent(); par != nil && par.Name() == "completion" {
			return nil
		}

		configureLogging()
		ctx, err := startTrace(cmd, args)
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)
		if err := loadPlugins(ctx); err != nil {
			return err
		}
		return nil
	},
	PersistentPostRunE: func(*cobra.Command, []string) error {
		if err := stopTrace(); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "Log more verbosely.")
	rootCmd.PersistentFlags().BoolVar(&doTrace, "trace", false, "Whether to record an execution trace or not.")
	if err := rootCmd.PersistentFlags().MarkHidden("trace"); err != nil {
		log.Fatal(err)
	}
}

func configureLogging() {
	slog.SetDefault(slog.New(console.NewHandler(os.Stderr, &console.HandlerOptions{
		Level:      levels[min(verbosity, len(levels)-1)],
		TimeFormat: time.RFC3339,
	})))
}

func startTrace(cmd *cobra.Command, args []string) (context.Context, error) {
	if !doTrace {
		return cmd.Context(), nil
	}
	dir, err := config.TraceDir()
	if err != nil {
		return nil, err
	}
	traceFile, err = os.OpenFile(filepath.Join(dir, fmt.Sprintf("trace_%s.out", time.Now().Format(time.RFC3339Nano))), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := trace.Start(traceFile); err != nil {
		return nil, fmt.Errorf("trace.Start(): %w", err)
	}
	slog.Debug("Started tracing.")

	ctx := cmd.Context()
	var cmds []string
	for ; cmd != nil; cmd = cmd.Parent() {
		cmds = append(cmds, cmd.Name())
	}
	slices.Reverse(cmds)
	cmds = append(cmds, args...)

	ctx, traceTask = trace.NewTask(ctx, strings.Join(cmds, " "))
	return ctx, nil
}

func stopTrace() error {
	if !doTrace {
		return nil
	}
	traceTask.End()
	trace.Stop()
	slog.Warn("Trace recorded.", "file", traceFile.Name())
	if err := traceFile.Close(); err != nil {
		return err
	}
	return nil
}

func loadPlugins(ctx context.Context) error {
	defer trace.StartRegion(ctx, "loading plugins").End()

	dir, err := config.PluginDir()
	if err != nil {
		return err
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("could not read VCS dir: %w", err)
	}
	var loaded int
	var errs []error
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		if !strings.HasSuffix(de.Name(), ".so") {
			continue
		}

		path := filepath.Join(dir, de.Name())
		if err := loadPlugin(ctx, path); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		} else {
			loaded++
		}
	}
	if loaded == 0 {
		if len(errs) == 0 {
			return fmt.Errorf("add VCS libraries to %s", dir)
		}
		return errors.Join(errs...)
	}

	for _, err := range errs {
		slog.Warn("An error occurred loading a VCS.", "error", err)
	}
	return nil
}

func loadPlugin(ctx context.Context, file string) error {
	defer trace.StartRegion(ctx, filepath.Base(file)).End()
	_, err := plugin.Open(file)
	return err
}

func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}
