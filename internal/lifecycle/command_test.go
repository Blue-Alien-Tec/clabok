package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func runBookCommand(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(directory)
	registry := cli.NewRegistry()
	config.Register(registry)
	Register(registry)
	var stdout, stderr strings.Builder
	code := cli.Run(registry, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func prepareProject(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	saveBook(t, layout, "fixture", 10, 11, "Body one\n")
	return directory
}

func TestBookCommandRoundTrip(t *testing.T) {
	directory := prepareProject(t)

	code, stdout, stderr := runBookCommand(t, directory, "book", "status", "fixture")
	if code != cli.ExitOK {
		t.Fatalf("status exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"closed"`) {
		t.Fatalf("unexpected status %s", stdout)
	}

	code, stdout, stderr = runBookCommand(t, directory, "book", "open", "fixture")
	if code != cli.ExitOK {
		t.Fatalf("open exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"status": "open"`) || !strings.Contains(stdout, `"session"`) {
		t.Fatalf("unexpected open payload %s", stdout)
	}

	code, _, stderr = runBookCommand(t, directory, "book", "close", "fixture")
	if code != cli.ExitOK {
		t.Fatalf("close exit %d: %s", code, stderr)
	}

	code, stdout, _ = runBookCommand(t, directory, "book", "status", "fixture")
	if code != cli.ExitOK || !strings.Contains(stdout, `"closed"`) {
		t.Fatalf("status after close: %d %s", code, stdout)
	}
}

func TestBookOpenDryRun(t *testing.T) {
	directory := prepareProject(t)
	code, stdout, stderr := runBookCommand(t, directory, "book", "open", "fixture", "--dry-run")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"dryRun": true`) || !strings.Contains(stdout, `"action"`) {
		t.Fatalf("unexpected receipt %s", stdout)
	}
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.Dir, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("dry run extracted the book: %v", err)
	}
}

func TestBookCloseRejectsInvalid(t *testing.T) {
	directory := prepareProject(t)
	if code, _, stderr := runBookCommand(t, directory, "book", "open", "fixture"); code != cli.ExitOK {
		t.Fatalf("open failed: %s", stderr)
	}
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.BlocksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.BlocksDir, "99.md"), []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runBookCommand(t, directory, "book", "close", "fixture")
	if code != cli.ExitReject {
		t.Fatalf("exit %d, want %d: %s", code, cli.ExitReject, stderr)
	}
	if !strings.Contains(stderr, `"book.invalid"`) || !strings.Contains(stderr, "model.orphan-body") {
		t.Fatalf("unexpected reject %s", stderr)
	}
}

func TestBookUnknownID(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := runBookCommand(t, directory, "book", "status", "bad id")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.invalid-value") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestBookUsage(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := runBookCommand(t, directory, "book", "open")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.invalid-arguments") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestBookRollbackCommands(t *testing.T) {
	directory := prepareProject(t)
	if code, _, stderr := runBookCommand(t, directory, "book", "open", "fixture"); code != cli.ExitOK {
		t.Fatalf("open failed: %s", stderr)
	}
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := New(layout, locks, Options{UndoLimit: 10, BookPointLimit: 3})
	pkg, err := life.Snapshot("fixture", "test")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.BlocksDir, "11.md"), []byte("Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runBookCommand(t, directory, "rollback", "list", "fixture")
	if code != cli.ExitOK {
		t.Fatalf("list exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, pkg.Name) {
		t.Fatalf("package not listed: %s", stdout)
	}

	code, _, stderr = runBookCommand(t, directory, "rollback", "restore", "fixture", pkg.Name)
	if code != cli.ExitOK {
		t.Fatalf("restore exit %d: %s", code, stderr)
	}
	body, err := os.ReadFile(filepath.Join(paths.BlocksDir, "11.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Body one\n" {
		t.Fatalf("restored body %q", body)
	}
}
