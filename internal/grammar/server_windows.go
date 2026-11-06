//go:build windows

package grammar

import (
	"os/exec"
	"syscall"
)

// detach keeps the server alive after the command exits and hides its console window.
func detach(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}
