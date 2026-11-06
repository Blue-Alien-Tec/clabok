//go:build !windows

package grammar

import (
	"os/exec"
	"syscall"
)

// detach starts the server in its own session so it survives the command.
func detach(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
