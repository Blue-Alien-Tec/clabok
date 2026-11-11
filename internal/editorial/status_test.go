package editorial

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestStatusLevelsEndToEnd(t *testing.T) {
	directory := t.TempDir()
	quietGrammar(t, directory)
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)
	runOK(t, directory, "status", "enable", "--book", "guide")

	view := dataOf(t, runOK(t, directory, "status", "list", "--book", "guide"))
	if fallback, _ := view["fallback"].(bool); !fallback {
		t.Fatalf("the empty list must use the fallback: %v", view)
	}
	if states, _ := view["states"].([]any); len(states) != 4 {
		t.Fatalf("fallback states %v", view["states"])
	}

	runOK(t, directory, "status", "level", "add", "--book", "guide",
		"--level-id", "approved", "--label", "Approved", "--color", "green")
	view = dataOf(t, runOK(t, directory, "status", "list", "--book", "guide"))
	if fallback, _ := view["fallback"].(bool); fallback {
		t.Fatalf("the first add must start a custom list: %v", view)
	}
	states, _ := view["states"].([]any)
	if len(states) != 1 {
		t.Fatalf("the custom list must not copy the defaults: %v", view["states"])
	}
	runOK(t, directory, "status", "set", "--book", "guide",
		"--target", "node", "--id", chapter, "--state", "approved")

	runOK(t, directory, "status", "level", "set", "--book", "guide",
		"--level-id", "approved", "--label", "Reviewed", "--color", "blue")
	runOK(t, directory, "status", "level", "set", "--book", "guide",
		"--level-id", "approved", "--description", "Content is reviewed.")
	view = dataOf(t, runOK(t, directory, "status", "list", "--book", "guide"))
	state := view["states"].([]any)[0].(map[string]any)
	if state["label"] != "Reviewed" || state["color"] != "blue" ||
		state["description"] != "Content is reviewed." || state["block"] == "" {
		t.Fatalf("edited level %v", state)
	}

	runOK(t, directory, "status", "level", "set", "--book", "guide",
		"--level-id", "approved", "--description", "Content is signed off.")
	view = dataOf(t, runOK(t, directory, "status", "list", "--book", "guide"))
	state = view["states"].([]any)[0].(map[string]any)
	if state["description"] != "Content is signed off." {
		t.Fatalf("edited description %v", state)
	}
	runOK(t, directory, "status", "level", "set", "--book", "guide",
		"--level-id", "approved", "--clear-description")
	view = dataOf(t, runOK(t, directory, "status", "list", "--book", "guide"))
	state = view["states"].([]any)[0].(map[string]any)
	if state["description"] != "" || state["block"] != nil {
		t.Fatalf("cleared description %v", state)
	}

	code, _, stderr := run(t, directory, "status", "level", "remove", "--book", "guide",
		"--level-id", "approved")
	if code == cli.ExitOK || !strings.Contains(stderr, "structure.status-in-use") {
		t.Fatalf("an assigned level must not be removed: %s", stderr)
	}
	runOK(t, directory, "status", "level", "add", "--book", "guide",
		"--level-id", "frozen", "--label", "Frozen")
	runOK(t, directory, "status", "set", "--book", "guide",
		"--target", "node", "--id", chapter, "--state", "frozen")
	runOK(t, directory, "status", "level", "remove", "--book", "guide",
		"--level-id", "approved")

	for index := 0; index < 7; index++ {
		runOK(t, directory, "status", "level", "add", "--book", "guide",
			"--level-id", "spare"+string(rune('a'+index)), "--label", "Spare")
	}
	code, _, stderr = run(t, directory, "status", "level", "add", "--book", "guide",
		"--level-id", "ninth", "--label", "Ninth")
	if code == cli.ExitOK || !strings.Contains(stderr, "structure.status-level-cap") {
		t.Fatalf("a ninth level must be refused: %s", stderr)
	}

	runOK(t, directory, "book", "close", "guide")
	runOK(t, directory, "sync", "--write")
	readme := readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	if !strings.Contains(readme, "![status: Frozen]") ||
		!strings.Contains(readme, "| Status | Meaning |") {
		t.Fatalf("custom legend missing:\n%s", readme)
	}
	if strings.Contains(readme, "Content is settled") {
		t.Fatalf("the fallback text leaked into a custom list:\n%s", readme)
	}
}

func TestStatusEndToEnd(t *testing.T) {
	directory := t.TempDir()
	quietGrammar(t, directory)
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
