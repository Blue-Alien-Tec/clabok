package grammar

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestStopDoesNotKillAReusedPID(t *testing.T) {
	t.Setenv("CLABOK_GRAMMAR_STATE_DIR", t.TempDir())
	endpoint := "http://127.0.0.1:1/v2/check"
	command := sleepingCommand()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() {
		_ = command.Process.Kill()
		<-done
	}()

	if err := writeState(serverState{
		PID: command.Process.Pid, URL: endpoint, Started: "2000-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Stop(endpoint); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		t.Fatal("Stop killed a process whose recorded server is gone")
	case <-time.After(250 * time.Millisecond):
	}
}

func sleepingCommand() *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "ping", "-n", "30", "127.0.0.1")
	}
	return exec.Command("sleep", "30")
}
