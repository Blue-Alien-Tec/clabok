package transfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/body"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/editorial"
	"github.com/Blue-Alien-Tec/clabok/internal/library"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

func newRegistry() *cli.Registry {
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	structure.Register(registry)
	editorial.Register(registry)
	body.Register(registry)
	render.Register(registry, library.IndexEntries)
	library.Register(registry)
	Register(registry)
	return registry
}

func run(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := cli.Run(newRegistry(), append([]string{"--project", directory}, args...), &stdout,
		&stderr)
	return code, stdout.String(), stderr.String()
}

func runOK(t *testing.T, directory string, args ...string) string {
	t.Helper()
	code, stdout, stderr := run(t, directory, args...)
	if code != cli.ExitOK {
		t.Fatalf("%v: exit %d\n%s%s", args, code, stdout, stderr)
	}
	return stdout
}

// prepareBook creates a project with a library and one guide book that has a chapter and a body.
func prepareBook(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")
	chapter := targetOf(t, runOK(t, directory, "add", "--book", "guide", "--title", "Chapter"))
	block := dataString(t, runOK(t, directory, "body", "set", "--book", "guide", "0",
		"--parent", chapter, "--body", "Body text."), "block")
	runOK(t, directory, "body", "set", "--book", "guide", "0", "--parent", chapter,
		"--body", "See [[id:"+block+"]].\n\nSecond paragraph.")
	return directory, block
}

func targetOf(t *testing.T, payload string) string {
	t.Helper()
	return dataString(t, payload, "target")
}

func dataString(t *testing.T, payload, field string) string {
	t.Helper()
	value := fieldOf(t, payload, field)
	if value == "" {
		t.Fatalf("payload has no %q: %s", field, payload)
	}
	return value
}

func fieldOf(t *testing.T, payload, field string) string {
	t.Helper()
	marker := `"` + field + `": "`
	index := strings.Index(payload, marker)
	if index < 0 {
		return ""
	}
	rest := payload[index+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func TestDumpRestoreRoundTrip(t *testing.T) {
	directory, _ := prepareBook(t)
	runOK(t, directory, "sync", "--book", "guide", "--write")
	runOK(t, directory, "book", "close", "guide")
	artifact := filepath.Join(directory, "guide.book")
	runOK(t, directory, "dump", "--book", "guide", "--out", artifact)
	original, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	container := filepath.Join(directory, "_CLABOK", "guide", "guide.book")
	before, err := os.ReadFile(container)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(original) {
		t.Fatal("dump is not byte-identical to the container")
	}
	runOK(t, directory, "book", "unregister", "guide")
	runOK(t, directory, "restore", "--file", artifact, "--as", "guide")
	after, err := os.ReadFile(container)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("restore did not reproduce the container bytes")
	}
	if code, _, stderr := run(t, directory, "sync", "--book", "guide", "--check"); code != cli.ExitOK {
		t.Fatalf("sync check after restore exit %d: %s", code, stderr)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	directory, _ := prepareBook(t)
	runOK(t, directory, "book", "close", "guide")
	exported := filepath.Join(directory, "export")
	runOK(t, directory, "export", "--book", "guide", "--out", exported)
	if _, err := os.Stat(filepath.Join(exported, descriptorMember)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(exported, "manifest.json")); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(runOK(t, directory, "import", "--from", exported, "--as", "copy"),
		`"idCount":`) {
		t.Fatal("the import payload has no id count")
	}
	runOK(t, directory, "book", "open", "copy")
	payload := runOK(t, directory, "check", "--book", "copy")
	if !strings.Contains(payload, `"clean": true`) {
		t.Fatalf("the imported book does not validate cleanly: %s", payload)
	}
	original, err := os.ReadFile(filepath.Join(directory, "_CLABOK", "guide", "guide.book"))
	if err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(filepath.Join(directory, "_CLABOK", "copy", "copy.book"))
	if err != nil {
		t.Fatal(err)
	}
	if string(original) == string(copied) {
		t.Fatal("the imported container must carry fresh IDs")
	}
}

func TestImportRefusesAnUnresolvedReference(t *testing.T) {
	directory, _ := prepareBook(t)
	runOK(t, directory, "book", "close", "guide")
	exported := filepath.Join(directory, "export")
	runOK(t, directory, "export", "--book", "guide", "--out", exported)
	blocks, err := filepath.Glob(filepath.Join(exported, "blocks", "*.md"))
	if err != nil || len(blocks) == 0 {
		t.Fatalf("no exported bodies: %v", err)
	}
	edited := false
	for _, path := range blocks {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(data), "[[id:") {
			replaced := strings.ReplaceAll(string(data), "[[id:", "[[id:999999")
			if writeErr := os.WriteFile(path, []byte(replaced), 0o644); writeErr != nil {
				t.Fatal(writeErr)
			}
			edited = true
			break
		}
	}
	if !edited {
		t.Fatal("the export carries no inline reference")
	}
	code, _, stderr := run(t, directory, "import", "--from", exported, "--as", "copy")
	if code == cli.ExitOK || !strings.Contains(stderr, "import.unresolved-ref") {
		t.Fatalf("dangling import exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(directory, "_CLABOK", "copy")); !os.IsNotExist(err) {
		t.Fatal("a failed import left a container behind")
	}
}

func TestImportRequiresTheSidecar(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	loose := filepath.Join(directory, "loose")
	if err := os.MkdirAll(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loose, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, directory, "import", "--from", loose, "--as", "copy")
	if code == cli.ExitOK || !strings.Contains(stderr, "import.bad-format") {
		t.Fatalf("a file set without a sidecar must be refused: exit %d %s", code, stderr)
	}
}

func TestExportCheckReportsDrift(t *testing.T) {
	directory, _ := prepareBook(t)
	runOK(t, directory, "book", "close", "guide")
	exported := filepath.Join(directory, "export")
	runOK(t, directory, "export", "--book", "guide", "--out", exported)
	runOK(t, directory, "export", "--book", "guide", "--out", exported, "--check")
	body := filepath.Join(exported, "blocks")
	entries, err := os.ReadDir(body)
	if err != nil || len(entries) == 0 {
		t.Fatalf("exported bodies: %v", err)
	}
	target := filepath.Join(body, entries[0].Name())
	if err := os.WriteFile(target, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, directory, "export", "--book", "guide", "--out", exported, "--check")
	if code == cli.ExitOK || !strings.Contains(stderr, "export.drift") {
		t.Fatalf("drift check exit %d: %s", code, stderr)
	}
}
