package tmux

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
	"github.com/avast/retry-go/v4"
	"github.com/creack/pty"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kballard/go-shellquote"
	"github.com/mr-tron/base58"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

var tmuxCmpOpt = cmp.Options{
	cmp.Comparer(SameServer),
	cmpopts.IgnoreFields(server{}, "opts"),
	cmp.AllowUnexported(session{}, TestServer{}, TestSession{}, TestClient{}),
	cmpopts.IgnoreFields(TestServer{}, "t"),
	cmpopts.IgnoreFields(TestSession{}, "t"),
	cmpopts.IgnoreFields(TestClient{}, "t"),
}

type TestServer struct {
	*server
	t *testing.T
}

func NewServerForTesting(t *testing.T) TestServer {
	n := fmt.Sprintf("%s-%s", t.Name(), randomString())
	srv := NewServer(NamedServerSocket(n), ServerConfigFile("/dev/null"))
	socketPath, err := srv.command("start-server", ";", "display-message", "-p", "#{socket_path}").RunStdout()
	if err != nil {
		t.Fatalf("Could not determine tmux server socket path")
	}
	t.Cleanup(func() {
		if err := srv.Kill(); err != nil {
			t.Logf("Failed to kill tmux server %s: %v", n, err)
		}
		if err := os.Remove(socketPath); err != nil {
			t.Logf("Could not clean up tmux socket %s: %v", socketPath, err)
		}
	})
	return TestServer{srv, t}
}

func randomString() string {
	n := rand.Uint64()
	b := make([]byte, binary.Size(n))
	binary.BigEndian.PutUint64(b, n)
	return base58.Encode(b)
}

func (srv TestServer) MustNewSession(opts NewSessionOptions) TestSession {
	sesh, err := srv.NewSession(opts)
	if err != nil {
		srv.t.Fatal(err)
	}
	return TestSession{sesh.(*session), srv.t}
}

func (srv TestServer) MustListSessions() []TestSession {
	sessions, err := srv.ListSessions()
	if err != nil {
		srv.t.Fatal(err)
	}
	res := make([]TestSession, len(sessions))
	for i, sesh := range sessions {
		res[i] = TestSession{sesh.(*session), srv.t}
	}
	return res
}

func (srv TestServer) mustAttachCommand(s Session) *exec.Command {
	cmd, err := srv.attachCommand(s)
	if err != nil {
		srv.t.Fatal(err)
	}
	return cmd
}

func (srv TestServer) mustSwitchCommand(s Session) *exec.Command {
	cmd, err := srv.switchCommand(s)
	if err != nil {
		srv.t.Fatal(err)
	}
	return cmd
}

func (srv TestServer) MustListClients() []TestClient {
	clients, err := srv.ListClients()
	if err != nil {
		srv.t.Fatal(err)
	}
	res := make([]TestClient, len(clients))
	for i, c := range clients {
		res[i] = TestClient{c.(*client), srv.t}
	}
	return res
}

type TestSession struct {
	*session
	t *testing.T
}

func (s TestSession) MustKill() {
	if err := s.Kill(); err != nil {
		s.t.Fatal(err)
	}
}

type TestClient struct {
	*client
	t *testing.T
}

func (c TestClient) MustProperties(props ...ClientProperty) map[ClientProperty]string {
	res, err := c.Properties(props...)
	if err != nil {
		c.t.Fatal(err)
	}
	return res
}

func RunInTTY(t *testing.T, cmd *exec.Command) *os.File {
	pty, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("Could not open pty: %v", err)
	}
	t.Cleanup(func() {
		if err := tty.Close(); err != nil {
			t.Logf("Could not close tty %s: %v", tty.Name(), err)
		}
		if err := pty.Close(); err != nil {
			t.Logf("Could not close pty %s: %v", pty.Name(), err)
		}
	})

	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	// tmux needs TERM to be set, and it isn't inherited from the parent process
	// in GitHub Actions.
	cmd.Env = append(cmd.Environ(), "TERM=xterm-256color")

	t.Logf("Running command in tty %s: %v", tty.Name(), cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Could not start command: %v", err)
	}
	t.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("Could not kill process %d: %v", cmd.Process.Pid, err)
		}
	})
	return pty
}

func TestServer_Sessions(t *testing.T) {
	srv := NewServerForTesting(t)

	sessions := srv.MustListSessions()
	if len(sessions) != 0 {
		t.Errorf("New tmux server has %d sessions, expected 0", len(sessions))
	}

	a := srv.MustNewSession(NewSessionOptions{Name: "a"})
	b := srv.MustNewSession(NewSessionOptions{Name: "b"})
	c := srv.MustNewSession(NewSessionOptions{Name: "c"})

	sessions = srv.MustListSessions()
	if diff := cmp.Diff([]TestSession{a, b, c}, sessions, tmuxCmpOpt); diff != "" {
		t.Errorf("srv.ListSessions() diff (-want +got)\n%s", diff)
	}

	b.MustKill()

	sessions = srv.MustListSessions()
	if diff := cmp.Diff([]TestSession{a, c}, sessions, tmuxCmpOpt); diff != "" {
		t.Errorf("srv.ListSessions() diff (-want +got)\n%s", diff)
	}
}

func TestServer_AttachOrSwitch(t *testing.T) {
	srv := NewServerForTesting(t)
	a := srv.MustNewSession(NewSessionOptions{Name: "a"})
	b := srv.MustNewSession(NewSessionOptions{Name: "b"})

	if c := srv.MustListClients(); len(c) != 0 {
		t.Errorf("Server already has %d clients:\n%v", len(c), c)
	}

	pty := RunInTTY(t, srv.mustAttachCommand(a))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var client TestClient
	err := retry.Do(func() error {
		clients := srv.MustListClients()
		if len(clients) != 1 {
			return fmt.Errorf("server has %d clients", len(clients))
		}
		client = clients[0]
		return nil
	}, retry.Delay(10*time.Millisecond), retry.Context(ctx))
	if err != nil {
		t.Fatal(err)
	}
	if id := client.MustProperties(ClientProperty(SessionID))[ClientProperty(SessionID)]; id != a.ID() {
		t.Errorf("Client is connected to %q, expected %q", id, a.ID())
	}

	switchCmd := srv.mustSwitchCommand(b)
	sh := shellquote.Join(switchCmd.Args...)
	t.Logf("Running command in tty: %s", sh)
	fmt.Fprintln(pty, sh)

	err = retry.Do(func() error {
		clients := srv.MustListClients()
		if len(clients) != 1 {
			return fmt.Errorf("server has %d clients", len(clients))
		}
		if id := clients[0].MustProperties(ClientProperty(SessionID))[ClientProperty(SessionID)]; id != b.ID() {
			return fmt.Errorf("client is connected to %q, expected %q", id, b.ID())
		}
		return nil
	}, retry.Delay(10*time.Millisecond), retry.Context(ctx))
	if err != nil {
		t.Error(err)
	}
}
