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
	"sync"
	"time"

	"github.com/JeffFaer/tmux-vcs-sync/api/config"
	"github.com/carlmjohnson/versioninfo"
	"github.com/kballard/go-shellquote"
	"github.com/phsym/console-slog"
	"github.com/spf13/cobra"
	exptrace "golang.org/x/exp/trace"
)

func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

var (
	version = readVersion()

	verbosity int
	levels    = []slog.Level{
		slog.LevelWarn,
		slog.LevelInfo,
		slog.LevelDebug,
	}

	traceTask *trace.Task

	doTrace   bool
	traceFile *os.File

	recordAfter    = 100 * time.Millisecond
	start          time.Time
	commandName    string
	flightRecorder *exptrace.FlightRecorder
)

var rootCmd = &cobra.Command{
	Use:          "tmux-vcs-sync",
	Short:        "Synchronize VCS state with tmux state.",
	Version:      version,
	SilenceUsage: true,
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cobraBuiltin(cmd) {
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
	PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
		if cobraBuiltin(cmd) {
			return nil
		}
		if err := stopTrace(); err != nil {
			return err
		}
		return nil
	},
}

func readVersion() string {
	var s string
	if v := versioninfo.Version; v != "" && v != "unknown" && v != "(devel)" {
		s = versioninfo.Version
	}
	if r := versioninfo.Revision; r != "" && r != "unknown" {
		s = versioninfo.Revision
		if len(s) > 7 {
			s = s[:7]
		}
	}
	if s == "" {
		s = "devel"
	} else if versioninfo.DirtyBuild {
		s += "-dev"
	}
	if t := versioninfo.LastCommit; !t.IsZero() {
		s += fmt.Sprintf(" (%s)", t.Format(time.RFC3339))
	}
	return s
}

func init() {
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "Log more verbosely.")
	rootCmd.PersistentFlags().BoolVar(&doTrace, "trace", false, "Whether to record an execution trace or not.")
	if err := rootCmd.PersistentFlags().MarkHidden("trace"); err != nil {
		log.Fatal(err)
	}
}

func cobraBuiltin(cmd *cobra.Command) bool {
	if par := cmd.Parent(); par != nil && par.Name() == "completion" {
		return true
	}
	return false
}

func configureLogging() {
	slog.SetDefault(slog.New(console.NewHandler(os.Stderr, &console.HandlerOptions{
		Level:      levels[min(verbosity, len(levels)-1)],
		TimeFormat: time.RFC3339,
	})))
}

func startTrace(cmd *cobra.Command, args []string) (context.Context, error) {
	start = time.Now()
	if cmd.Name() == cobra.ShellCompRequestCmd && slices.Contains(args, "--trace") {
		// cobra.ShellCompRequestCmd disables flag parsing.
		doTrace = true
	}
	if doTrace {
		var err error
		traceFile, err = createTraceFile(fmt.Sprintf("%s_%s.out", cmd.Name(), start.Format(time.RFC3339Nano)))
		if err != nil {
			return nil, err
		}
		if err := trace.Start(traceFile); err != nil {
			return nil, fmt.Errorf("trace.Start(): %w", err)
		}
		slog.Debug("Started tracing.")
	} else {
		commandName = cmd.Name()
		flightRecorder = exptrace.NewFlightRecorder()
		flightRecorder.SetPeriod(5 * time.Second)
		if err := flightRecorder.Start(); err != nil {
			return nil, fmt.Errorf("flightRecorder.Start(): %w", err)
		}
	}

	ctx := cmd.Context()
	var cmds []string
	for ; cmd != nil; cmd = cmd.Parent() {
		cmds = append(cmds, cmd.Name())
	}
	slices.Reverse(cmds)
	cmds = append(cmds, args...)

	ctx, traceTask = trace.NewTask(ctx, shellquote.Join(cmds...))
	return ctx, nil
}

func createTraceFile(filename string) (*os.File, error) {
	dir, err := config.TraceDir()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	return f, nil
}

var stopTrace = sync.OnceValue(func() error {
	traceTask.End()
	if doTrace {
		trace.Stop()
		slog.Warn("Trace recorded.", "file", traceFile.Name())
		if err := traceFile.Close(); err != nil {
			return err
		}
	} else if dur := time.Since(start); dur > recordAfter {
		dur = dur.Truncate(time.Millisecond)
		f, err := createTraceFile(fmt.Sprintf("%s_%v_%s.out", commandName, dur, start.Format(time.RFC3339Nano)))
		if err != nil {
			return err
		}
		if _, err := flightRecorder.WriteTo(f); err != nil {
			return fmt.Errorf("writing trace file: %w", err)
		}
		slog.Warn("Recording trace due to slow execution.", "duration", dur, "file", f.Name())

		if err := flightRecorder.Stop(); err != nil {
			return fmt.Errorf("flightRecorder.Stop(): %w", err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("closing trace file: %w", err)
		}
	} else {
		slog.Debug("Not recording trace.", "duration", dur)
	}
	return nil
})

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
