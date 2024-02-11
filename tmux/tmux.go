package tmux

import (
	"fmt"

	"github.com/JeffFaer/tmux-vcs-sync/api/exec"
)

var (
	tmux exec.Executable

	errNotTmux = fmt.Errorf("must be called from within a tmux session")
)

func init() {
	var err error
	tmux, err = exec.Lookup("tmux")
	if err != nil {
		panic(err)
	}
}
