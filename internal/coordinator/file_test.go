package coordinator

import (
	"strings"
	"testing"
	"time"
)

func TestAcquireBusy(t *testing.T) {
	directory := t.TempDir()
	first := backendIn(t, directory, "first")
	second := backendIn(t, directory, "second")
	resources := []Resource{{Kind: KindBlock, Book: "clabok", ID: 7}}

	release, err := first.Acquire(resources, Exclusive, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_, busyErr := second.Acquire(resources, Exclusive, 60*time.Millisecond)
	if busyErr == nil {
		t.Fatal("expected a busy error")
	}
	coordinatorErr, ok := busyErr.(*Error)
	if !ok || coordinatorErr.Code != "coordinator.busy" {
		t.Fatalf("unexpected error %v", busyErr)
	}
	if !strings.Contains(coordinatorErr.Message, "pid") {
		t.Fatalf("holder metadata missing: %s", coordinatorErr.Message)
	}
	if !strings.Contains(coordinatorErr.Message, "first") {
		t.Fatalf("holder command missing: %s", coordinatorErr.Message)
	}

	if err := release(); err != nil {
		t.Fatal(err)
	}
	released, err := second.Acquire(resources, Exclusive, time.Second)
	if err != nil {
		t.Fatalf("lock not free after release: %v", err)
	}
	if err := released(); err != nil {
		t.Fatal(err)
	}
}

func TestDifferentResourcesRunConcurrently(t *testing.T) {
	directory := t.TempDir()
	first := backendIn(t, directory, "first")
	second := backendIn(t, directory, "second")

	releaseFirst, err := first.Acquire([]Resource{{Kind: KindBlock, Book: "clabok", ID: 1}}, Exclusive, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseFirst() }()
	releaseSecond, err := second.Acquire([]Resource{{Kind: KindBlock, Book: "clabok", ID: 2}}, Exclusive, time.Second)
	if err != nil {
		t.Fatalf("different blocks must not conflict: %v", err)
	}
	defer func() { _ = releaseSecond() }()
}

func TestCrossingAcquiresDoNotDeadlock(t *testing.T) {
	directory := t.TempDir()
	first := backendIn(t, directory, "first")
	second := backendIn(t, directory, "second")

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

	firstResult := <-results
	secondResult := <-results
	successes := make([]Release, 0, 2)
	for _, outcome := range []result{firstResult, secondResult} {
		if outcome.err == nil {
			successes = append(successes, outcome.release)
		}
	}
	if len(successes) != 1 {
		t.Fatalf("expected exactly one successful acquire, got %d", len(successes))
	}
	for _, release := range successes {
		if err := release(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	backend := newBackend(t)
	release, err := backend.Acquire([]Resource{{Kind: KindBook, Book: "clabok"}}, Exclusive, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatalf("second release failed: %v", err)
	}
}

func TestSharedLocksCoexistAndBlockExclusive(t *testing.T) {
	directory := t.TempDir()
	first := backendIn(t, directory, "first")
	second := backendIn(t, directory, "second")
	third := backendIn(t, directory, "third")
	resources := []Resource{{Kind: KindBook, Book: "clabok"}}

	releaseFirst, err := first.Acquire(resources, Shared, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseFirst() }()
	releaseSecond, err := second.Acquire(resources, Shared, time.Second)
	if err != nil {
		t.Fatalf("shared locks must coexist: %v", err)
	}
	defer func() { _ = releaseSecond() }()

	if _, err := third.Acquire(resources, Exclusive, 60*time.Millisecond); err == nil {
		t.Fatal("exclusive must wait for shared holders")
	}
}

func TestExclusiveBlocksShared(t *testing.T) {
	directory := t.TempDir()
	first := backendIn(t, directory, "first")
	second := backendIn(t, directory, "second")
	resources := []Resource{{Kind: KindBook, Book: "clabok"}}

	release, err := first.Acquire(resources, Exclusive, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	if _, err := second.Acquire(resources, Shared, 60*time.Millisecond); err == nil {
		t.Fatal("shared must wait for an exclusive holder")
	}
}

func TestInvalidMode(t *testing.T) {
	backend := newBackend(t)
	if _, err := backend.Acquire([]Resource{{Kind: KindBook, Book: "clabok"}}, Mode(9), time.Second); err == nil {
		t.Fatal("expected an invalid mode error")
	}
}

func backendIn(t *testing.T, directory, command string) *File {
	t.Helper()
	backend, err := NewFile(directory)
	if err != nil {
		t.Fatal(err)
	}
	backend.Command = command
	backend.Timeout = time.Second
	return backend
}

func newBackend(t *testing.T) *File {
	t.Helper()
	return backendIn(t, t.TempDir(), "test")
}
