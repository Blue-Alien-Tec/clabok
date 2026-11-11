package editorial

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestProseFileEncoding(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	proseFile := filepath.Join(directory, "prose.md")
	if err := os.WriteFile(proseFile, []byte("\uFEFFThe chapter summary.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runOK(t, directory, "prose", "set", "--book", "guide", "--node", chapter,
		"--file", proseFile)

	manifestData, err := os.ReadFile(filepath.Join(directory, "_CLABOK", "guide", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(manifestData)
	if err != nil {
		t.Fatal(err)
	}
	chapterID, _ := model.ParseID(chapter)
	node, ok := manifest.FindNode(chapterID)
	if !ok {
		t.Fatalf("chapter %s not found", chapter)
	}
	proseBlock := node.ProseBlock
	body, err := os.ReadFile(filepath.Join(directory, "_CLABOK", "guide", "blocks",
		proseBlock.String()+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "The chapter summary." {
		t.Fatalf("stored prose %q", body)
	}

	if err := os.WriteFile(proseFile, []byte{'a', 0x00, 'b', 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, directory, "prose", "set", "--book", "guide", "--node", chapter,
		"--file", proseFile)
	if code == cli.ExitOK || !strings.Contains(stderr, "text.invalid-encoding") {
		t.Fatalf("a BOM-less UTF-16 prose file must be refused: exit %d %s", code, stderr)
	}
}

func TestAsciiuiSpecEncoding(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	specFile := filepath.Join(directory, "bom.asciiui")
	if err := os.WriteFile(specFile, []byte("\uFEFFlegend true\nbutton Buy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	added := dataOf(t, runOK(t, directory, "asciiui", "add",
		"--book", "guide", "--parent", chapter, "--file", specFile))
	block, _ := added["block"].(string)
	if block == "" {
		t.Fatalf("add payload %v", added)
	}
	stored, err := os.ReadFile(filepath.Join(directory, "_CLABOK", "guide", "layouts",
		"asciiui", block+".asciiui"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != "legend true\nbutton Buy\n" {
		t.Fatalf("stored spec %q", stored)
	}
}
