package editorial

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func writeSpecFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAsciiuiAuthoringEndToEnd(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))["target"].(string)

	specFile := filepath.Join(directory, "layout.asciiui")
	writeSpecFile(t, specFile, "legend true\nbutton Buy\n")
	added := dataOf(t, runOK(t, directory, "asciiui", "add",
		"--book", "guide", "--parent", chapter, "--file", specFile))
	block, _ := added["block"].(string)
	if block == "" || added["parent"] != chapter {
		t.Fatalf("add payload %v", added)
	}
	storeSpec := filepath.Join(directory, "_CLABOK", "guide", "layouts", "asciiui", block+".asciiui")
	if got := readFile(t, storeSpec); got != "legend true\nbutton Buy\n" {
		t.Fatalf("stored spec %q", got)
	}
	blockID, err := model.ParseID(block)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode([]byte(readFile(t, filepath.Join(directory, "_CLABOK", "guide", "manifest.json"))))
	if err != nil {
		t.Fatal(err)
	}
	if meta := manifest.Meta(blockID); !meta.Locked || meta.Type != "UILayout" {
		t.Fatalf("block metadata %+v", meta)
	}

	runOK(t, directory, "sync", "--write")
	readme := readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	if !strings.Contains(readme, "[ Buy ]") || !strings.Contains(readme, "<!-- asciiui:"+block+" -->") {
		t.Fatalf("the drawing is missing:\n%s", readme)
	}

	writeSpecFile(t, specFile, "button Sell\n")
	runOK(t, directory, "asciiui", "set", "--book", "guide", "--block", block, "--file", specFile)
	runOK(t, directory, "sync", "--write")
	readme = readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	if strings.Contains(readme, "[ Buy ]") || !strings.Contains(readme, "[ Sell ]") {
		t.Fatalf("the spec was not replaced:\n%s", readme)
	}

	writeSpecFile(t, specFile, "button Dry\n")
	runOK(t, directory, "asciiui", "set", "--book", "guide", "--block", block, "--file", specFile, "--dry-run")
	if got := readFile(t, storeSpec); got != "button Sell\n" {
		t.Fatalf("a dry run wrote the spec: %q", got)
	}

	runOK(t, directory, "book", "close", "guide")
	if _, err := os.Stat(storeSpec); !os.IsNotExist(err) {
		t.Fatalf("close left the extracted spec: %v", err)
	}
	runOK(t, directory, "sync", "--write")
	if !strings.Contains(readFile(t, filepath.Join(directory, "Docs", "guide", "README.md")), "[ Sell ]") {
		t.Fatal("a closed book lost its layout")
	}

	runOK(t, directory, "book", "open", "guide")
	if got := readFile(t, storeSpec); got != "button Sell\n" {
		t.Fatalf("open did not restore the spec: %q", got)
	}
	runOK(t, directory, "asciiui", "remove", "--book", "guide", "--block", block)
	if _, err := os.Stat(storeSpec); !os.IsNotExist(err) {
		t.Fatalf("the spec survived removal: %v", err)
	}
	runOK(t, directory, "sync", "--write")
	if strings.Contains(readFile(t, filepath.Join(directory, "Docs", "guide", "README.md")), "[ Sell ]") {
		t.Fatal("the drawing survived removal")
	}

	subchapter := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Sub",
		"--parent", chapter))["target"].(string)
	plainBlock := dataOf(t, runOK(t, directory, "add", "--book", "guide", "--parent", subchapter))["target"].(string)
	writeSpecFile(t, specFile, "button X\n")
	code, stdout, stderr := run(t, directory, "asciiui", "set",
		"--book", "guide", "--block", plainBlock, "--file", specFile)
	if code == cli.ExitOK {
		t.Fatalf("a plain block accepted a layout: %s%s", stdout, stderr)
	}
	if !strings.Contains(stderr, `"structure.not-layout"`) {
		t.Fatalf("expected structure.not-layout, got stdout=%q stderr=%q", stdout, stderr)
	}
}
