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
	"github.com/JeffFaer/tmux-vcs-sync/api/exec/exectest"
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

func tmuxCmpOpt(ctx context.Context) cmp.Option {
	return cmp.Options{
		cmp.Comparer(func(a, b Server) bool { return SameServer(ctx, a, b) }),
		cmpopts.IgnoreFields(server{}, "opts"),
		cmp.AllowUnexported(session{}, TestServer{}, TestSession{}, TestClient{}),
		cmpopts.IgnoreFields(TestServer{}, "t"),
		cmpopts.IgnoreFields(TestSession{}, "t"),
		cmpopts.IgnoreFields(TestClient{}, "t"),
	}
}

type TestServer struct {
	*server
	t *testing.T
}

func NewServerForTesting(ctx context.Context, t *testing.T) TestServer {
	n := fmt.Sprintf("%s-%s", t.Name(), randomString())
	srv := NewServer(NamedServerSocket(n), ServerConfigFile("/dev/null"))
	srv.tmux = exectest.NewTestCommander(t, tmux)
	socketPath, err := srv.command(ctx, "start-server", ";", "display-message", "-p", "#{socket_path}").RunStdout()
	if err != nil {
		t.Fatalf("Could not determine tmux server socket path")
	}
	t.Cleanup(func() {
		if err := srv.Kill(context.Background()); err != nil {
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

func (srv TestServer) MustNewSession(ctx context.Context, opts NewSessionOptions) TestSession {
	sesh, err := srv.NewSession(ctx, opts)
	if err != nil {
		srv.t.Fatal(err)
	}
	return TestSession{sesh.(*session), srv.t}
}

func (srv TestServer) MustListSessions(ctx context.Context) TestSessions {
	val, err := srv.ListSessions(ctx)
	if err != nil {
		srv.t.Fatal(err)
	}
	if val == nil {
		return TestSessions{nil, srv.t}
	}
	return TestSessions{val.(sessions), srv.t}
}

func (srv TestServer) mustAttachCommand(ctx context.Context, s Session) *exec.Command {
	cmd, err := srv.attachCommand(ctx, s)
	if err != nil {
		srv.t.Fatal(err)
	}
	return cmd
}

func (srv TestServer) mustSwitchCommand(ctx context.Context, s Session) *exec.Command {
	cmd, err := srv.switchCommand(ctx, s)
	if err != nil {
		srv.t.Fatal(err)
	}
	return cmd
}

func (srv TestServer) MustListClients(ctx context.Context) []TestClient {
	clients, err := srv.ListClients(ctx)
	if err != nil {
		srv.t.Fatal(err)
	}
	res := make([]TestClient, len(clients))
	for i, c := range clients {
		res[i] = TestClient{c.(*client), srv.t}
	}
	return res
}

type TestSessions struct {
	sessions
	t *testing.T
}

type TestSession struct {
	*session
	t *testing.T
}

func (s TestSession) MustKill(ctx context.Context) {
	if err := s.Kill(ctx); err != nil {
		s.t.Fatal(err)
	}
}

type TestClient struct {
	*client
	t *testing.T
}

func (c TestClient) MustProperties(ctx context.Context, props ...ClientProperty) map[ClientProperty]string {
	res, err := c.Properties(ctx, props...)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv := NewServerForTesting(ctx, t)

	sessions := srv.MustListSessions(ctx)
	if n := len(sessions.Sessions()); n != 0 {
		t.Errorf("New tmux server has %d sessions, expected 0", n)
	}

	a := srv.MustNewSession(ctx, NewSessionOptions{Name: "a"})
	b := srv.MustNewSession(ctx, NewSessionOptions{Name: "b"})
	c := srv.MustNewSession(ctx, NewSessionOptions{Name: "c"})

	sessions = srv.MustListSessions(ctx)
	if diff := cmp.Diff([]Session{a.session, b.session, c.session}, sessions.Sessions(), tmuxCmpOpt(ctx)); diff != "" {
		t.Errorf("srv.ListSessions() diff (-want +got)\n%s", diff)
	}

	b.MustKill(ctx)

	sessions = srv.MustListSessions(ctx)
	if diff := cmp.Diff([]Session{a.session, c.session}, sessions.Sessions(), tmuxCmpOpt(ctx)); diff != "" {
		t.Errorf("srv.ListSessions() diff (-want +got)\n%s", diff)
	}
}

func TestServer_AttachOrSwitch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv := NewServerForTesting(ctx, t)
	a := srv.MustNewSession(ctx, NewSessionOptions{Name: "a"})
	b := srv.MustNewSession(ctx, NewSessionOptions{Name: "b"})

	if c := srv.MustListClients(ctx); len(c) != 0 {
		t.Errorf("Server already has %d clients:\n%v", len(c), c)
	}

	pty := RunInTTY(t, srv.mustAttachCommand(ctx, a))

	var client TestClient
	err := retry.Do(func() error {
		clients := srv.MustListClients(ctx)
		if len(clients) != 1 {
			return fmt.Errorf("server has %d clients", len(clients))
		}
		client = clients[0]
		return nil
	}, retry.Delay(10*time.Millisecond), retry.Context(ctx))
	if err != nil {
		t.Fatal(err)
	}
	if id := client.MustProperties(ctx, ClientProperty(SessionID))[ClientProperty(SessionID)]; id != a.ID() {
		t.Errorf("Client is connected to %q, expected %q", id, a.ID())
	}

	switchCmd := srv.mustSwitchCommand(ctx, b)
	sh := shellquote.Join(switchCmd.Args...)
	t.Logf("Running command in tty: %s", sh)
	fmt.Fprintln(pty, sh)

	err = retry.Do(func() error {
		clients := srv.MustListClients(ctx)
		if len(clients) != 1 {
			return fmt.Errorf("server has %d clients", len(clients))
		}
		if id := clients[0].MustProperties(ctx, ClientProperty(SessionID))[ClientProperty(SessionID)]; id != b.ID() {
			return fmt.Errorf("client is connected to %q, expected %q", id, b.ID())
		}
		return nil
	}, retry.Delay(10*time.Millisecond), retry.Context(ctx))
	if err != nil {
		t.Error(err)
	}
}
