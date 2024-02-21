package exectest

import (
	"testing"

	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
	"github.com/kballard/go-shellquote"
)

// TestCommander wraps a different Commander so that its stdout and stderr are
// recorded in testing.T by default.
type TestCommander struct {
	exec.Commander
	t *testing.T
}

func NewTestCommander(t *testing.T, cmd exec.Commander) TestCommander {
	return TestCommander{cmd, t}
}

func (c TestCommander) Command(args ...string) *exec.Command {
	cmd := c.Commander.Command(args...)
	c.t.Logf("Executing command %s", shellquote.Join(cmd.Args...))
	w := testingWriter{t: c.t}
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd
}

type testingWriter struct {
	t testing.TB
	exec.OverrideableWriter
}

func (w testingWriter) Write(b []byte) (int, error) {
	w.t.Log(string(b))
	return len(b), nil
}
