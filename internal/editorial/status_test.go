package editorial

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestStatusEndToEnd(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	code, _, stderr := run(t, directory, "status", "set",
		"--book", "guide", "--target", "node", "--id", chapter, "--state", "final")
	if code == cli.ExitOK || !strings.Contains(stderr, `"structure.status-disabled"`) {
		t.Fatalf("a disabled book accepted a state: %s", stderr)
	}

	runOK(t, directory, "status", "enable", "--book", "guide")
	runOK(t, directory, "status", "set",
		"--book", "guide", "--target", "node", "--id", chapter, "--state", "final")
	block := dataOf(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "Body text"))["block"].(string)
	runOK(t, directory, "status", "set",
		"--book", "guide", "--target", "block", "--id", block, "--state", "wip")

	view := dataOf(t, runOK(t, directory, "status", "show", "--book", "guide"))
	if view["enabled"] != true {
		t.Fatalf("show payload %v", view)
	}
	if assignments, _ := view["assignments"].([]any); len(assignments) != 2 {
		t.Fatalf("assignments %v", view["assignments"])
	}

	runOK(t, directory, "book", "close", "guide")
	runOK(t, directory, "sync", "--write")
	readme := readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	finalBadge := "![status: Final](https://img.shields.io/badge/status-Final-brightgreen?style=flat-square)"
	wipBadge := "![status: Work in Progress](https://img.shields.io/badge/status-Work%20in%20Progress-orange?style=flat-square)"
	for _, want := range []string{
		"| Document | Status |",
		"| [Chapter](#chapter) | " + finalBadge + " |",
		"## Chapter " + finalBadge,
		wipBadge + "\n\nBody text",
		"**Status**",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("missing %q in\n%s", want, readme)
		}
	}

	runOK(t, directory, "book", "open", "guide")
	runOK(t, directory, "status", "clear",
		"--book", "guide", "--target", "node", "--id", chapter)
	runOK(t, directory, "status", "clear",
		"--book", "guide", "--target", "block", "--id", block)
	runOK(t, directory, "status", "disable", "--book", "guide")
	runOK(t, directory, "book", "close", "guide")
	runOK(t, directory, "sync", "--write")
	readme = readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	for _, forbidden := range []string{"img.shields.io", "**Status**", "| Document | Status"} {
		if strings.Contains(readme, forbidden) {
			t.Fatalf("a disabled book still renders %q:\n%s", forbidden, readme)
		}
	}
	if !strings.Contains(readme, "- [Chapter](#chapter)") {
		t.Fatalf("the plain index must stay a list:\n%s", readme)
	}
}
