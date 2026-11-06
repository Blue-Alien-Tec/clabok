package editorial

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitFixture(t *testing.T, directory string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", directory}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	run("init", "-q")
	for _, name := range []string{"src/core/main.go", "src/util/util.go"} {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package src\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "src")
}

func TestLayoutEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	directory := t.TempDir()
	quietGrammar(t, directory)
	gitFixture(t, directory)

	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Tree"))["target"].(string)

	added := dataOf(t, runOK(t, directory, "layout", "add",
		"--book", "guide", "--parent", chapter, "--path", "src", "--depth", "2"))
	if anchor, _ := added["anchor"].(string); anchor == "" {
		t.Fatalf("add payload %v", added)
	}
	runOK(t, directory, "layout", "note", "--book", "guide", "--path", "src/core", "--note", "The core package.")
	runOK(t, directory, "layout", "exclude", "--book", "guide", "--path", "src/util")

	view := dataOf(t, runOK(t, directory, "layout", "show", "--book", "guide"))
	tree, _ := view["tree"].(string)
	if !strings.Contains(tree, "core/") || strings.Contains(tree, "util/") {
		t.Fatalf("tree %q", tree)
	}
	if !strings.Contains(tree, "The core package.") {
		t.Fatalf("the note is missing: %q", tree)
	}

	runOK(t, directory, "book", "close", "guide")
	runOK(t, directory, "sync", "--write")
	readme := readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	for _, want := range []string{"<!-- repolayout:", "```text\ncore/"} {
		if !strings.Contains(readme, want) {
			t.Fatalf("missing %q in\n%s", want, readme)
		}
	}
	if strings.Contains(readme, "util/") {
		t.Fatalf("an excluded file rendered:\n%s", readme)
	}

	runOK(t, directory, "book", "open", "guide")
	runOK(t, directory, "layout", "include", "--book", "guide", "--path", "src/util")
	runOK(t, directory, "book", "close", "guide")
	runOK(t, directory, "sync", "--write")
	readme = readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	if !strings.Contains(readme, "util/") {
		t.Fatalf("the included file is missing:\n%s", readme)
	}
}
