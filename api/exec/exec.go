package exec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/kballard/go-shellquote"
)

func Lookup(name string) (Executable, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return Executable(path), nil
}

type Executable string

func (exe Executable) Command(args ...string) *Command {
	cmd := exec.Command(string(exe), args...)
	cmd.Stdout = defaultStdout
	cmd.Stderr = defaultStderr
	return &Command{cmd}
}

var (
	// This is how os.Stdout and os.Stderr are defined, except these os.File
	// pointers will be distinct, so they won't be the same according to ==.
	defaultStdout = os.NewFile(uintptr(syscall.Stdout), "/dev/stdout")
	defaultStderr = os.NewFile(uintptr(syscall.Stderr), "/dev/stderr")
)

// OverrideableWriter can be used to indicate that values in Command.Stdout and
// Command.Stderr are safe to override when requested.
type OverrideableWriter interface {
	ConsideredOverridable()
}

// cmd is a type alias that lets us embed *exec.Cmd without
// exporting it.
type cmd = exec.Cmd

type Command struct {
	*cmd
}

func (cmd *Command) LogValue() slog.Value {
	val := slog.StringValue(shellquote.Join(cmd.Args...))
	if cmd.Dir != "" {
		wd, err := os.Getwd()
		if err == nil && cmd.Dir != wd {
			val = slog.GroupValue(slog.Attr{Key: "cmd", Value: val}, slog.String("wd", cmd.Dir))
		}
	}
	return val
}

func (cmd *Command) setStdout(w io.Writer) error {
	if cmd.Stdout == defaultStdout {
		cmd.Stdout = w
		return nil
	}
	if _, ok := cmd.Stdout.(OverrideableWriter); ok {
		cmd.Stdout = w
		return nil
	}
	return fmt.Errorf("exec: Stdout already set")
}

func (cmd *Command) setStderr(w io.Writer) error {
	if cmd.Stderr == defaultStderr {
		cmd.Stderr = w
		return nil
	}
	if _, ok := cmd.Stderr.(OverrideableWriter); ok {
		cmd.Stderr = w
		return nil
	}
	return fmt.Errorf("exec: Stderr already set")
}

func (cmd *Command) StdoutPipe() (io.ReadCloser, error) {
	if err := cmd.setStdout(nil); err != nil {
		return nil, err
	}
	return cmd.cmd.StdoutPipe()
}

func (cmd *Command) StderrPipe() (io.ReadCloser, error) {
	if err := cmd.setStderr(nil); err != nil {
		return nil, err
	}
	return cmd.cmd.StderrPipe()
}

func (cmd *Command) Start() error {
	start := time.Now()
	logger := slog.Default().With("command", cmd)
	logger.Debug("Executing external process. (1/2)")
	err := cmd.cmd.Start()
	go func() {
		cmd.Wait()
		logger.Debug("Executing external process. (2/2)", "elapsed", time.Since(start), "exit_code", cmd.ProcessState.ExitCode())
	}()
	return err
}

func (cmd *Command) Run() error {
	start := time.Now()
	logger := slog.Default().With("command", cmd)
	logger.Debug("Executing external process. (1/2)")
	err := cmd.cmd.Run()
	logger.Debug("Executing external process. (2/2)", "elapsed", time.Since(start), "exit_code", cmd.ProcessState.ExitCode())
	return err
}

// RunStdout runs the command and returns its standard output as a trimmed string.
func (cmd *Command) RunStdout() (string, error) {
	var stdout bytes.Buffer
	if err := cmd.setStdout(&stdout); err != nil {
		return "", err
	}
	err := cmd.Run()
	return strings.TrimRight(string(stdout.Bytes()), "\n"), err
}

// RunOutput runs the command and returns both its standard output and standard
// error as a trimmed string.
func (cmd *Command) RunOutput() (string, string, error) {
	var stdout, stderr bytes.Buffer
	if err := errors.Join(cmd.setStdout(&stdout), cmd.setStderr(&stderr)); err != nil {
		return "", "", err
	}
	err := cmd.Run()
	return strings.TrimRight(string(stdout.Bytes()), "\n"), strings.TrimRight(string(stderr.Bytes()), "\n"), err
}
