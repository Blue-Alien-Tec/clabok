package library

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestPurgeRemovesTracesAndKeepsOutput(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "unregister", "guide")

	layout := layoutOf(t, directory)
	paths, err := layout.Book("guide")
	if err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(paths.Dir); statErr != nil || !info.IsDir() {
		t.Fatalf("store directory missing before purge: %v", statErr)
	}
	output := filepath.Join(directory, "Docs", "guide", "README.md")
	if _, statErr := os.Stat(output); statErr != nil {
		t.Fatalf("output missing before purge: %v", statErr)
	}

	if mkErr := os.MkdirAll(paths.RollbackDir, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	if writeErr := os.WriteFile(filepath.Join(paths.RollbackDir, "book-orphan.zip"), []byte("x"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	tempDir := filepath.Join(directory, ".temp", "guide")
	if mkErr := os.MkdirAll(tempDir, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	if writeErr := os.WriteFile(filepath.Join(tempDir, "draft.md"), []byte("x"), 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	lockDir := layout.Coordinator()
	if mkErr := os.MkdirAll(lockDir, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	removedLocks := []string{"book-guide.lock", "block-guide-11.lock", "node-guide-12.lock"}
	keptLocks := []string{
		"book-guide-other.lock",
		"block-guide-other-13.lock",
		"node-guide-other-15.lock",
		"block-other-14.lock",
	}
	for _, name := range append(append([]string{}, removedLocks...), keptLocks...) {
		if writeErr := os.WriteFile(filepath.Join(lockDir, name), nil, 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	}

	dryRun := dataOf(t, runOK(t, directory, "book", "purge", "guide", "--dry-run"))
	if dryRun["removed"] != true {
		t.Fatalf("dry-run payload %v", dryRun)
	}
	if _, statErr := os.Stat(paths.Dir); statErr != nil {
		t.Fatal("dry run removed the store directory")
	}
	if _, statErr := os.Stat(tempDir); statErr != nil {
		t.Fatal("dry run removed the temp directory")
	}
	for _, name := range removedLocks {
		if _, statErr := os.Stat(filepath.Join(lockDir, name)); statErr != nil {
			t.Fatalf("dry run removed lock file %s", name)
		}
	}

	payload := dataOf(t, runOK(t, directory, "book", "purge", "guide"))
	if payload["removed"] != true {
		t.Fatalf("purge payload %v", payload)
	}
	if payload["store"] != true || payload["rollback"] != true || payload["temp"] != true {
		t.Fatalf("purge did not report every trace: %v", payload)
	}
	if payload["locks"] != float64(len(removedLocks)) {
		t.Fatalf("purge lock count %v, want %d", payload["locks"], len(removedLocks))
	}
	for _, trace := range []string{paths.Dir, paths.RollbackDir, tempDir} {
		if _, statErr := os.Stat(trace); !os.IsNotExist(statErr) {
			t.Fatalf("trace %s still exists", trace)
		}
	}
	for _, name := range removedLocks {
		if _, statErr := os.Stat(filepath.Join(lockDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("lock file %s still exists", name)
		}
	}
	for _, name := range keptLocks {
		if _, statErr := os.Stat(filepath.Join(lockDir, name)); statErr != nil {
			t.Fatalf("purge removed foreign lock file %s", name)
		}
	}
	if _, statErr := os.Stat(output); statErr != nil {
		t.Fatal("purge removed the rendered output")
	}

	again := dataOf(t, runOK(t, directory, "book", "purge", "guide"))
	if again["removed"] != false {
		t.Fatalf("second purge is not a no-op: %v", again)
	}
	runOK(t, directory, "sync", "--check")
}

func TestPurgeRefusesRegisteredLibraryAndReserved(t *testing.T) {
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
	assertReject([]string{"book", "purge", "guide"}, "library.book-registered")
	assertReject([]string{"book", "purge", "guide", "--dry-run"}, "library.book-registered")
	assertReject([]string{"book", "purge", "library"}, "library.immovable")
	assertReject([]string{"book", "purge", "GUIDE"}, "library.book-registered")
	assertReject([]string{"book", "purge", "LIBRARY"}, "library.immovable")
	assertReject([]string{"book", "purge", "rollback"}, "library.purge-reserved")
	assertReject([]string{"book", "purge", "ROLLBACK"}, "library.purge-reserved")
	assertReject([]string{"book", "purge", "rollback."}, "library.purge-reserved")
	assertReject([]string{"book", "purge", ".Coordinator"}, "library.purge-reserved")
	assertReject([]string{"book", "purge", "guide."}, "library.purge-ambiguous")
	assertReject([]string{"book", "purge", "guide:stream"}, "library.purge-ambiguous")

	layout := layoutOf(t, directory)
	paths, err := layout.Book("guide")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Dir); err != nil {
		t.Fatalf("a refused purge removed the store directory: %v", err)
	}
	if _, err := os.Stat(layout.Registry()); err != nil {
		t.Fatalf("a refused purge removed the registry: %v", err)
	}
}

func TestPurgeMatchesLockCaseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive lock names are a Windows concern")
	}
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "unregister", "guide")

	layout := layoutOf(t, directory)
	lockDir := layout.Coordinator()
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	removed := []string{"book-guide.lock", "block-guide-11.lock", "node-guide-12.lock"}
	kept := []string{"block-guide-other-13.lock"}
	for _, name := range append(append([]string{}, removed...), kept...) {
		if err := os.WriteFile(filepath.Join(lockDir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	payload := dataOf(t, runOK(t, directory, "book", "purge", "GUIDE"))
	if payload["locks"] != float64(len(removed)) {
		t.Fatalf("purge lock count %v, want %d", payload["locks"], len(removed))
	}
	for _, name := range removed {
		if _, err := os.Stat(filepath.Join(lockDir, name)); !os.IsNotExist(err) {
			t.Fatalf("lock file %s still exists", name)
		}
	}
	for _, name := range kept {
		if _, err := os.Stat(filepath.Join(lockDir, name)); err != nil {
			t.Fatalf("purge removed foreign lock file %s", name)
		}
	}
}

func TestPurgeRefusesWithoutLibraryRegistry(t *testing.T) {
	directory := t.TempDir()
	ghost := filepath.Join(directory, "_CLABOK", "ghost")
	if err := os.MkdirAll(ghost, 0o755); err != nil {
		t.Fatal(err)
	}
	exit, _, stderr := run(t, directory, "book", "purge", "ghost")
	if exit != cli.ExitReject || !strings.Contains(stderr, "library.incomplete-registry") {
		t.Fatalf("exit %d\n%s", exit, stderr)
	}
	if _, err := os.Stat(ghost); err != nil {
		t.Fatal("purge removed store data without a library registry")
	}
}
