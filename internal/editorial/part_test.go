package editorial

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestPartCommandEndToEnd(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")

	first := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Vorwort",
		"--part", "1"))["target"].(string)
	second := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Galaxie",
		"--part", "2"))["target"].(string)
	sub := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--parent", first,
		"--title", "Zubehör"))["target"].(string)

	groups := dataOf(t, runOK(t, directory, "part", "list", "--book", "guide"))
	if groups["limit"].(float64) != 10 {
		t.Fatalf("limit %v", groups["limit"])
	}
	parts, _ := groups["parts"].([]any)
	if len(parts) != 2 {
		t.Fatalf("groups %v", groups["parts"])
	}
	firstGroup := parts[0].(map[string]any)
	if firstGroup["text"] != "Part I" || firstGroup["slot"].(float64) != 1 {
		t.Fatalf("first group %v", firstGroup)
	}
	if chapters := parts[0].(map[string]any)["chapters"].(float64); chapters != 1 {
		t.Fatalf("chapter count %v", chapters)
	}

	label := dataOf(t, runOK(t, directory, "part", "label", "--book", "guide",
		"--from", "Part II", "--part", "Teil II — Die Galaxie"))
	if label["part"] != "Teil II — Die Galaxie" {
		t.Fatalf("label payload %v", label)
	}
	if affected, _ := label["affected"].([]any); len(affected) != 1 ||
		affected[0].(string) != second {
		t.Fatalf("affected %v", label["affected"])
	}

	if code, _, stderr := run(t, directory, "part", "set", "--book", "guide",
		"--target", "node", "--id", sub, "--part", "3"); code == cli.ExitOK ||
		!strings.Contains(stderr, "structure.part-on-node") {
		t.Fatalf("a subchapter took a part: %s", stderr)
	}
	if code, _, stderr := run(t, directory, "part", "set", "--book", "guide",
		"--target", "node", "--id", second, "--part", "11"); code == cli.ExitOK ||
		!strings.Contains(stderr, "part.out-of-range") {
		t.Fatalf("an out-of-range slot was accepted: %s", stderr)
	}
	if code, _, stderr := run(t, directory, "part", "set", "--book", "guide",
		"--target", "node", "--id", first); code == cli.ExitOK ||
		!strings.Contains(stderr, "usage.") {
		t.Fatalf("a missing --part was accepted: %s", stderr)
	}

	runOK(t, directory, "book", "close", "guide")
	runOK(t, directory, "sync", "--write")
	readme := readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	for _, want := range []string{"**Part I**", "**Teil II — Die Galaxie**"} {
		if !strings.Contains(readme, want) {
			t.Fatalf("missing %q in\n%s", want, readme)
		}
	}
	if strings.Index(readme, "**Part I**") > strings.Index(readme, "**Teil II — Die Galaxie**") {
		t.Fatalf("the groups render out of order:\n%s", readme)
	}
}
