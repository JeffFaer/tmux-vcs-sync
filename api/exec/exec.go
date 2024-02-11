package exec

import (
	"bytes"
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

func (cmd *Command) StdoutPipe() (io.ReadCloser, error) {
	if cmd.Stdout == defaultStdout {
		cmd.Stdout = nil
	}
	return cmd.cmd.StdoutPipe()
}

func (cmd *Command) StderrPipe() (io.ReadCloser, error) {
	if cmd.Stderr == defaultStderr {
		cmd.Stderr = nil
	}
	return cmd.cmd.StderrPipe()
}

func (cmd *Command) Start() error {
	start := time.Now()
	logger := slog.Default().With("command", cmd)
	logger.Debug("Executing external process. (1/2)")
	err := cmd.Start()
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
	if cmd.Stdout != defaultStdout {
		return "", fmt.Errorf("exec: Stdout already set")
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return strings.TrimRight(string(stdout.Bytes()), "\n"), err
}

// RunOutput runs the command and returns both its standard output and standard
// error as a trimmed string.
func (cmd *Command) RunOutput() (string, string, error) {
	if cmd.Stdout != defaultStdout {
		return "", "", fmt.Errorf("exec: Stdout already set")
	}
	if cmd.Stderr != defaultStderr {
		return "", "", fmt.Errorf("exec: Stderr already set")
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimRight(string(stdout.Bytes()), "\n"), strings.TrimRight(string(stderr.Bytes()), "\n"), err
}
