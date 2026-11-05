package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func TestClosedRenderMatchesOpen(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	opened, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scene.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	closed, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(opened.Files) != len(closed.Files) {
		t.Fatalf("open %d files, closed %d files", len(opened.Files), len(closed.Files))
	}
	for index := range opened.Files {
		if opened.Files[index] != closed.Files[index] {
			t.Fatalf("file %s differs between open and closed rendering", opened.Files[index].Path)
		}
	}
}

func TestClosedAndOpenSyncHashesMatch(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	output := t.TempDir()
	if _, err := scene.service.Sync("fixture", output, SyncOptions{Write: true}); err != nil {
		t.Fatal(err)
	}
	openCheck, err := scene.service.Sync("fixture", output, SyncOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if openCheck.Status != store.StatusClean {
		t.Fatalf("open check status %s", openCheck.Status)
	}
	if _, err := scene.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	closedCheck, err := scene.service.Sync("fixture", output, SyncOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if closedCheck.Status != store.StatusClean {
		t.Fatalf("closed check status %s", closedCheck.Status)
	}
	if len(closedCheck.Changed) != 0 {
		t.Fatalf("closed check reports drift: %v", closedCheck.Changed)
	}
}

func TestCheckClosedBookHasNoOpenFinding(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	if _, err := scene.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	findings, err := scene.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if finding.Code == "render.open-book" {
			t.Fatal("a closed book reports render.open-book")
		}
	}
}

func TestCheckDoesNotCreateOutputDir(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	output := filepath.Join(t.TempDir(), "missing", "nested")
	if _, err := scene.service.Sync("fixture", output, SyncOptions{Check: true}); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("a check created the output directory: %v", statErr)
	}
}

func TestSyncPrunesOnlyOwnBookFiles(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	output := t.TempDir()
	foreignBook := Marker + "\n<!-- book other; mode monolith -->\n\n# Other\n"
	if err := os.WriteFile(filepath.Join(output, "other.md"), []byte(foreignBook), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "PLAN.md"), []byte("plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := Marker + "\n<!-- book fixture; mode chapter -->\n\n# Stale\n"
	if err := os.WriteFile(filepath.Join(output, "stale.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := scene.service.Sync("fixture", output, SyncOptions{Write: true, Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pruned) != 1 || result.Pruned[0] != "stale.md" {
		t.Fatalf("pruned %v", result.Pruned)
	}
	for _, name := range []string{"other.md", "PLAN.md"} {
		if _, statErr := os.Stat(filepath.Join(output, name)); statErr != nil {
			t.Fatalf("%s must survive pruning: %v", name, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(output, "stale.md")); !os.IsNotExist(statErr) {
		t.Fatal("the stale file of this book was not pruned")
	}
}

func TestDocumentIndexRendering(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	scene.service.LibraryIndex = []IndexEntry{
		{Title: "Guide", Href: "Docs/guide/README.md"},
		{Title: "A | B", Href: "Docs/ab/README.md"},
	}
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, "## Document Index") {
		t.Fatalf("missing Document Index:\n%s", content)
	}
	if !strings.Contains(content, "| [Guide](Docs/guide/README.md) |") {
		t.Fatalf("missing Guide row:\n%s", content)
	}
	if !strings.Contains(content, `| [A \| B](Docs/ab/README.md) |`) {
		t.Fatalf("a table cell was not escaped:\n%s", content)
	}
	if strings.Index(content, "## Index") > strings.Index(content, "## Document Index") {
		t.Fatal("the Document Index must follow the book index")
	}
}

func TestDocumentIndexOmittedWhenEmpty(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Files[0].Content, "## Document Index") {
		t.Fatal("an empty library index renders a section")
	}
}
