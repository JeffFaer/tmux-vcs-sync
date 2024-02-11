package exec

import (
	"os"
	"testing"
)

func TestDefaultStdout(t *testing.T) {
	if defaultStdout == os.Stdout {
		t.Errorf("defaultStdout == os.Stdout, expected distinct pointers")
	}
}

func TestCommand_StdoutPipe_OverridesDefaultStdout(t *testing.T) {
	echo, err := Lookup("echo")
	if err != nil {
		t.Fatalf("echo doesn't exist: %v", err)
	}
	cmd := echo.Command("abc")
	_, err = cmd.StdoutPipe()
	if err != nil {
		t.Errorf("cmd.StdoutPipe() = _, %v, exepected no error", err)
	}
}

func TestCommand_StdoutPipe_DoesNotOverrideNonDefaultStdout(t *testing.T) {
	echo, err := Lookup("echo")
	if err != nil {
		t.Fatalf("echo doesn't exist: %v", err)
	}
	cmd := echo.Command("abc")
	cmd.Stdout = os.Stdout
	_, err = cmd.StdoutPipe()
	if err == nil {
		t.Errorf("cmd.StdoutPipe() = _, %v, expected error", err)
	}
}
