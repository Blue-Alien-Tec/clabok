package structure

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func fixturePaths(t *testing.T, directory string) store.BookPaths {
	t.Helper()
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func targetOf(t *testing.T, stdout string) model.ID {
	t.Helper()
	var envelope struct {
		Data struct {
			Target string `json:"target"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	id, err := model.ParseID(envelope.Data.Target)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAddPartFlag(t *testing.T) {
	directory := prepareProject(t)
	code, stdout, stderr := run(t, directory, "add", "--book", "fixture", "--title", "Two", "--part", "2")
	if code != cli.ExitOK {
		t.Fatalf("add exit %d: %s", code, stderr)
	}
	node, ok := readManifest(t, fixturePaths(t, directory)).FindNode(targetOf(t, stdout))
	if !ok || node.Part != "Part II" {
		t.Fatalf("added chapter %+v", node)
	}
}

func TestRenamePartFlags(t *testing.T) {
	directory := prepareProject(t)

	code, _, stderr := run(t, directory, "rename", "--book", "fixture", "10",
		"--title", "Eins", "--part", "2", "--clear-part")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.invalid-arguments") {
		t.Fatalf("the conflicting flags were accepted: %d %s", code, stderr)
	}

	code, _, stderr = run(t, directory, "rename", "--book", "fixture", "10",
		"--title", "Eins", "--part", "2")
	if code != cli.ExitOK {
		t.Fatalf("rename exit %d: %s", code, stderr)
	}
	paths := fixturePaths(t, directory)
	if node, _ := readManifest(t, paths).FindNode(10); node.Part != "Part II" {
		t.Fatalf("rename did not set the part: %+v", node)
	}

	code, _, stderr = run(t, directory, "rename", "--book", "fixture", "10",
		"--title", "Eins", "--clear-part")
	if code != cli.ExitOK {
		t.Fatalf("clear exit %d: %s", code, stderr)
	}
	if node, _ := readManifest(t, paths).FindNode(10); node.Part != "" {
		t.Fatalf("the clear did not take: %+v", node)
	}
}

func TestAddPartOnSubchapterRejected(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := run(t, directory, "add", "--book", "fixture", "--parent", "10",
		"--title", "Sub", "--part", "1")
	if code != cli.ExitFailure || !strings.Contains(stderr, "structure.part-on-node") {
		t.Fatalf("a subchapter took a part: %d %s", code, stderr)
	}
}
