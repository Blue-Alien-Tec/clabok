package library

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func TestNewBookRejectsCaseCollidingIDAndPath(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")

	assertReject := func(args []string, code string) {
		t.Helper()
		exit, _, stderr := run(t, directory, args...)
		if exit != cli.ExitReject || !strings.Contains(stderr, code) {
			t.Fatalf("%v: exit %d\n%s", args, exit, stderr)
		}
	}
	assertReject([]string{"book", "new", "--id", "GUIDE", "--title", "Other", "--path", "Docs/other"},
		"library.id-in-use")
	assertReject([]string{"book", "new", "--id", "other", "--title", "Other", "--path", "Docs/GUIDE"},
		"library.path-in-use")

	layout := layoutOf(t, directory)
	registry, err := store.LoadRegistry(layout.Registry())
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Books) != 2 {
		t.Fatalf("a refused book was registered: %+v", registry.Books)
	}
	if _, exists := registry.FindFold("other"); exists {
		t.Fatal("the refused book left a registry entry")
	}

	derived := dataOf(t, runOK(t, directory, "book", "new", "--title", "GUIDE"))
	if derived["id"] != "guide-2" {
		t.Fatalf("derived id %v, want guide-2", derived["id"])
	}
}

func TestNewBookRejectsReservedIDs(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")

	for _, id := range []string{
		"rollback", "ROLLBACK", "rollback.", ".coordinator", ".Coordinator",
		"library.json", "guide:stream",
	} {
		exit, _, stderr := run(t, directory, "book", "new", "--id", id, "--title", "X")
		if exit != cli.ExitReject || !strings.Contains(stderr, "library.id-invalid") {
			t.Fatalf("id %q: exit %d\n%s", id, exit, stderr)
		}
	}

	exit, _, stderr := run(t, directory, "book", "new", "--title", "Rollback")
	if exit != cli.ExitReject || !strings.Contains(stderr, "library.id-unavailable") {
		t.Fatalf("derived reserved id: exit %d\n%s", exit, stderr)
	}
}

func TestNewBookRejectsStaleStoreDirectory(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "unregister", "guide")

	exit, _, stderr := run(t, directory, "book", "new", "--id", "guide", "--title", "Guide",
		"--path", "Docs/new-guide")
	if exit != cli.ExitReject || !strings.Contains(stderr, "library.store-exists") {
		t.Fatalf("stale store: exit %d\n%s", exit, stderr)
	}

	runOK(t, directory, "book", "purge", "guide")
	runOK(t, directory, "book", "new", "--id", "guide", "--title", "Guide", "--path", "Docs/new-guide")
	runOK(t, directory, "sync", "--check")
}
