package editorial

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBadgeEndToEnd(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"),
		[]byte("module github.com/example/clabok\n\ngo 1.27.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "LICENSE"),
		[]byte("MIT License\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "library")
	runOK(t, directory, "badge", "add", "--book", "library", "--id", "go")
	runOK(t, directory, "badge", "add", "--book", "library", "--id", "license")

	view := dataOf(t, runOK(t, directory, "badge", "show", "--book", "library"))
	entries, _ := view["badges"].([]any)
	if len(entries) != 2 {
		t.Fatalf("badge view %v", view)
	}
	first, _ := entries[0].(map[string]any)
	if first["resolved"] != true || first["value"] != "1.27.3" || first["source"] != "go.mod" {
		t.Fatalf("go badge %v", first)
	}

	runOK(t, directory, "book", "close", "library")
	runOK(t, directory, "sync", "--write")
	readme := readFile(t, filepath.Join(directory, "README.md"))
	goBadge := "![go: 1.27.3](https://img.shields.io/badge/go-1.27.3-00ADD8?style=flat-square)"
	licenseBadge := "![license: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)"
	for _, want := range []string{goBadge, licenseBadge} {
		if !strings.Contains(readme, want) {
			t.Fatalf("missing %q in\n%s", want, readme)
		}
	}
	if strings.Index(readme, goBadge) > strings.Index(readme, "# CLABOK") {
		t.Fatalf("badges must sit above the title:\n%s", readme)
	}

	runOK(t, directory, "book", "open", "library")
	runOK(t, directory, "badge", "add", "--book", "library", "--id", "go", "--message", "custom")
	runOK(t, directory, "book", "close", "library")
	runOK(t, directory, "sync", "--write")
	readme = readFile(t, filepath.Join(directory, "README.md"))
	if !strings.Contains(readme, "![go: custom]") {
		t.Fatalf("an explicit message must win:\n%s", readme)
	}

	runOK(t, directory, "book", "open", "library")
	runOK(t, directory, "badge", "add", "--book", "library", "--id", "ghost")
	runOK(t, directory, "book", "close", "library")
	code, _, stderr := run(t, directory, "check", "--book", "library")
	if code != 0 {
		t.Fatalf("check exited %d: %s", code, stderr)
	}

	runOK(t, directory, "book", "open", "library")
	runOK(t, directory, "badge", "remove", "--book", "library", "--id", "go")
	runOK(t, directory, "badge", "remove", "--book", "library", "--id", "ghost")
	runOK(t, directory, "book", "close", "library")
	runOK(t, directory, "sync", "--write")
	readme = readFile(t, filepath.Join(directory, "README.md"))
	if strings.Contains(readme, "![go:") {
		t.Fatalf("the removed badge survived:\n%s", readme)
	}
}
