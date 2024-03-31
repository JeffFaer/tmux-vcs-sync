package exec

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDefaultStdout(t *testing.T) {
	if defaultStdout == os.Stdout {
		t.Errorf("defaultStdout == os.Stdout, expected distinct pointers")
	}
}

func TestCommand_StdoutPipe_OverridesDefaultStdout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	echo, err := Lookup("echo")
	if err != nil {
		t.Fatalf("echo doesn't exist: %v", err)
	}
	cmd := echo.Command(ctx, "abc")
	_, err = cmd.StdoutPipe()
	if err != nil {
		t.Errorf("cmd.StdoutPipe() = _, %v, exepected no error", err)
	}
}

func TestCommand_StdoutPipe_DoesNotOverrideNonDefaultStdout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	echo, err := Lookup("echo")
	if err != nil {
		t.Fatalf("echo doesn't exist: %v", err)
	}
	cmd := echo.Command(ctx, "abc")
	cmd.Stdout = os.Stdout
	_, err = cmd.StdoutPipe()
	if err == nil {
		t.Errorf("cmd.StdoutPipe() = _, %v, expected error", err)
	}
}
