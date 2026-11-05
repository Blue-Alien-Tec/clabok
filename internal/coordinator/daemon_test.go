package coordinator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func shortSocket(t *testing.T) string {
	t.Helper()
	path := filepath.Join(os.TempDir(),
		fmt.Sprintf("clabok-test-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func startDaemon(t *testing.T, version string) (*Server, string) {
	t.Helper()
	socket := shortSocket(t)
	server, err := NewServer(socket, version)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Listen(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve() }()
	t.Cleanup(func() { _ = server.Close() })
	return server, socket
}

func daemonClient(t *testing.T, socket, version string) *Daemon {
	t.Helper()
	client, err := NewDaemon(socket, version)
	if err != nil {
		t.Fatal(err)
	}
	client.Timeout = time.Second
	client.Command = "test"
	return client
}

func TestDaemonConformance(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		directory := t.TempDir()
		first, err := NewFile(directory)
		if err != nil {
			t.Fatal(err)
		}
		first.Command = "first"
		first.Timeout = time.Second
		second, err := NewFile(directory)
		if err != nil {
			t.Fatal(err)
		}
		second.Command = "second"
		second.Timeout = time.Second
		runSemantics(t, first, second)
	})
	t.Run("daemon", func(t *testing.T) {
		_, socket := startDaemon(t, "test")
		first := daemonClient(t, socket, "test")
		second := daemonClient(t, socket, "test")
		second.Command = "second"
		runSemantics(t, first, second)
	})
}

func runSemantics(t *testing.T, first, second Coordinator) {
	t.Helper()
	book := []Resource{{Kind: KindBook, Book: "clabok"}}

	release, err := first.Acquire(book, Exclusive, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, busyErr := second.Acquire(book, Exclusive, 60*time.Millisecond); busyErr == nil {
		t.Fatal("expected a busy error")
	} else if coordinatorErr, ok := busyErr.(*Error); !ok || coordinatorErr.Code != "coordinator.busy" {
		t.Fatalf("unexpected error %v", busyErr)
	} else if !strings.Contains(coordinatorErr.Message, "pid") {
		t.Fatalf("holder missing: %s", coordinatorErr.Message)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatalf("release is not idempotent: %v", err)
	}

	sharedFirst, err := first.Acquire(book, Shared, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sharedFirst() }()
	sharedSecond, err := second.Acquire(book, Shared, time.Second)
	if err != nil {
		t.Fatalf("shared locks must coexist: %v", err)
	}
	defer func() { _ = sharedSecond() }()
	if _, err := first.Acquire(book, Exclusive, 60*time.Millisecond); err == nil {
		t.Fatal("exclusive must wait for shared holders")
	}
}

func TestDaemonVersionMismatch(t *testing.T) {
	_, socket := startDaemon(t, "server")
	client := daemonClient(t, socket, "client")
	_, err := client.Acquire([]Resource{{Kind: KindBook, Book: "clabok"}}, Exclusive, time.Second)
	if err == nil {
		t.Fatal("expected a version mismatch")
	}
	if coordinatorErr, ok := err.(*Error); !ok || coordinatorErr.Code != "coordinator.version-mismatch" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestDaemonUnavailable(t *testing.T) {
	client, err := NewDaemon(shortSocket(t), "test")
	if err != nil {
		t.Fatal(err)
	}
	_, acquireErr := client.Acquire([]Resource{{Kind: KindBook, Book: "clabok"}}, Exclusive, 100*time.Millisecond)
	if acquireErr == nil {
		t.Fatal("expected coordinator.daemon-unavailable")
	}
	if coordinatorErr, ok := acquireErr.(*Error); !ok || coordinatorErr.Code != "coordinator.daemon-unavailable" {
		t.Fatalf("unexpected error %v", acquireErr)
	}
}

func TestDaemonAlreadyRunning(t *testing.T) {
	_, socket := startDaemon(t, "test")
	second, err := NewServer(socket, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Listen(); err == nil {
		t.Fatal("expected coordinator.already-running")
	} else if coordinatorErr, ok := err.(*Error); !ok || coordinatorErr.Code != "coordinator.already-running" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestDaemonCrossingAcquiresDoNotDeadlock(t *testing.T) {
	_, socket := startDaemon(t, "test")
	first := daemonClient(t, socket, "test")
	second := daemonClient(t, socket, "test")
	book := Resource{Kind: KindBook, Book: "clabok"}
	node := Resource{Kind: KindNode, Book: "clabok", ID: 9}

	type result struct {
		release Release
		err     error
	}
	results := make(chan result, 2)
	go func() {
		release, err := first.Acquire([]Resource{book, node}, Exclusive, 300*time.Millisecond)
		results <- result{release: release, err: err}
	}()
	go func() {
		release, err := second.Acquire([]Resource{node, book}, Exclusive, 300*time.Millisecond)
		results <- result{release: release, err: err}
	}()
	successes := make([]Release, 0, 2)
	for index := 0; index < 2; index++ {
		outcome := <-results
		if outcome.err == nil {
			successes = append(successes, outcome.release)
		}
	}
	if len(successes) != 1 {
		t.Fatalf("expected exactly one successful acquire, got %d", len(successes))
	}
	for _, release := range successes {
		_ = release()
	}
}

func TestDaemonInvalidMode(t *testing.T) {
	client := daemonClient(t, shortSocket(t), "test")
	if _, err := client.Acquire([]Resource{{Kind: KindBook, Book: "clabok"}}, Mode(9), time.Second); err == nil {
		t.Fatal("expected coordinator.invalid-mode")
	}
}
