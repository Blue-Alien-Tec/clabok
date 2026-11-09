package library

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/apply"
	"github.com/Blue-Alien-Tec/clabok/internal/body"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
	"github.com/Blue-Alien-Tec/clabok/internal/search"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
	"github.com/Blue-Alien-Tec/clabok/internal/temp"
)

func newRegistry() *cli.Registry {
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	structure.Register(registry)
	apply.Register(registry)
	body.Register(registry)
	search.Register(registry)
	temp.Register(registry)
	render.Register(registry, IndexEntries)
	Register(registry)
	return registry
}

func run(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := cli.Run(newRegistry(), append([]string{"--project", directory}, args...), &stdout, &stderr)
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

func dataOf(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("parse %q: %v", stdout, err)
	}
	return envelope.Data
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// quietGrammar points the project at an empty fake server so a body write in an unrelated test
// never starts the real LanguageTool. Production always checks; the fake keeps the suite offline.
func quietGrammar(t *testing.T, directory string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"matches":[]}`)
	}))
	t.Cleanup(server.Close)
	runOK(t, directory, "config", "set", "grammar.endpoint", server.URL)
}

func layoutOf(t *testing.T, directory string) store.Layout {
	t.Helper()
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	return layout
}

func readManifest(t *testing.T, directory, id string) *model.Manifest {
	t.Helper()
	paths, err := layoutOf(t, directory).Book(model.BookID(id))
	if err != nil {
		t.Fatal(err)
	}
	if data, readErr := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember)); readErr == nil {
		manifest, decodeErr := model.Decode(data)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		return manifest
	}
	container, err := store.Load(paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	data, ok := container.Read(lifecycle.ManifestMember)
	if !ok {
		t.Fatalf("%s has no manifest", id)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"CLABOK":      "clabok",
		"Moved Guide": "moved-guide",
		"  A / B  ":   "a-b",
		"Über":        "ber",
		"!!!":         "book",
		"x2 y3":       "x2-y3",
	}
	for input, want := range cases {
		if got := Slug(input); got != want {
			t.Fatalf("Slug(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNewLibraryBookAndDocumentIndex(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	readme := readFile(t, filepath.Join(directory, "README.md"))
	if !strings.HasPrefix(readme, render.Marker) {
		t.Fatalf("the library README is not generated:\n%s", readme)
	}
	if !strings.Contains(readme, "# CLABOK") {
		t.Fatalf("the library title is missing:\n%s", readme)
	}
	if strings.Contains(readme, "## Document Index") {
		t.Fatal("an empty library renders a Document Index")
	}

	runOK(t, directory, "book", "new", "--title", "Guide")
	guide := readFile(t, filepath.Join(directory, "Docs", "guide", "README.md"))
	if !strings.Contains(guide, "# Guide") {
		t.Fatalf("the new book was not rendered:\n%s", guide)
	}
	if !strings.Contains(guide, "[← Library](../../README.md)") {
		t.Fatalf("the new book misses its library navigation:\n%s", guide)
	}
	readme = readFile(t, filepath.Join(directory, "README.md"))
	if strings.Count(readme, "[Document Index](#document-index)") != 2 {
		t.Fatalf("the library navigation is missing:\n%s", readme)
	}
	if !strings.Contains(readme, "## Document Index") ||
		!strings.Contains(readme, "| [Guide](Docs/guide/README.md) |") {
		t.Fatalf("the Document Index is missing:\n%s", readme)
	}
	runOK(t, directory, "sync", "--check")
}

func TestDefaultsApplyAtCreation(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "library", "defaults", "set", "numbering", "true")
	runOK(t, directory, "library", "defaults", "set", "output", "chapter")

	runOK(t, directory, "book", "new", "--title", "Guide")
	manifest := readManifest(t, directory, "guide")
	if !manifest.Settings.Numbering.Enabled {
		t.Fatal("the numbering default was not applied")
	}
	if manifest.Settings.Output != model.OutputChapter {
		t.Fatalf("output = %s, want chapter", manifest.Settings.Output)
	}

	runOK(t, directory, "book", "new", "--title", "Plain",
		"--numbering=false", "--output", "monolith")
	manifest = readManifest(t, directory, "plain")
	if manifest.Settings.Numbering.Enabled {
		t.Fatal("an explicit --numbering=false did not win")
	}
	if manifest.Settings.Output != model.OutputMonolith {
		t.Fatalf("output = %s, want monolith", manifest.Settings.Output)
	}

	runOK(t, directory, "library", "defaults", "unset")
	shown := dataOf(t, runOK(t, directory, "library", "defaults", "show"))
	if shown["source"] != "builtin" {
		t.Fatalf("source = %v, want builtin", shown["source"])
	}
}

func TestNewBookPathGuards(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--title", "Guide")

	code, _, stderr := run(t, directory, "book", "new", "--title", "Outside", "--path", "Elsewhere")
	if code != cli.ExitReject || !strings.Contains(stderr, "library.outside-docs-root") {
		t.Fatalf("outside path: exit %d\n%s", code, stderr)
	}
	runOK(t, directory, "book", "new", "--title", "Outside", "--path", "Elsewhere", "--allow-outside")

	code, _, stderr = run(t, directory, "book", "new", "--title", "Root", "--path", ".")
	if code != cli.ExitReject || !strings.Contains(stderr, "library.path-conflict") {
		t.Fatalf("root path: exit %d\n%s", code, stderr)
	}

	code, _, stderr = run(t, directory, "book", "new", "--title", "Again", "--path", "Docs/guide")
	if code != cli.ExitReject || !strings.Contains(stderr, "library.path-in-use") {
		t.Fatalf("duplicate path: exit %d\n%s", code, stderr)
	}
}

func TestNewBookPathExistsNeedsForce(t *testing.T) {
	directory := t.TempDir()
	guideDir := filepath.Join(directory, "Docs", "guide")
	if err := os.MkdirAll(guideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guideDir, "notes.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, directory, "book", "new", "--title", "Guide")
	if code != cli.ExitReject || !strings.Contains(stderr, "library.path-exists") {
		t.Fatalf("non-empty path: exit %d\n%s", code, stderr)
	}
	runOK(t, directory, "book", "new", "--title", "Guide", "--force")
	if _, err := os.Stat(filepath.Join(guideDir, "notes.md")); err != nil {
		t.Fatal("--force deleted the existing file")
	}
}

func TestNewLibraryBacksUpExistingReadme(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "README.md"), []byte("hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK", "--force")
	readme := readFile(t, filepath.Join(directory, "README.md"))
	if !strings.HasPrefix(readme, render.Marker) {
		t.Fatalf("the library README is not generated:\n%s", readme)
	}
	backup := readFile(t, filepath.Join(directory, "README.bak"))
	if backup != "hand written\n" {
		t.Fatalf("the existing README was not preserved: %q", backup)
	}
}

func TestNewBookIDCollision(t *testing.T) {
	directory := t.TempDir()
	first := dataOf(t, runOK(t, directory, "book", "new", "--title", "Guide"))
	second := dataOf(t, runOK(t, directory, "book", "new", "--title", "Guide"))
	if first["id"] != "guide" || second["id"] != "guide-2" {
		t.Fatalf("ids %v and %v", first["id"], second["id"])
	}
}

func TestLibrarySingleton(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	code, _, stderr := run(t, directory, "book", "new", "--library", "--id", "second", "--title", "Second")
	if code != cli.ExitReject || !strings.Contains(stderr, "library.library-exists") {
		t.Fatalf("second library: exit %d\n%s", code, stderr)
	}
}

func TestMoveRenameUnregister(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "book", "open", "guide")

	for _, args := range [][]string{
		{"book", "move", "guide", "--to", "Docs/moved"},
		{"book", "rename", "guide", "--title", "Moved"},
		{"book", "unregister", "guide"},
	} {
		code, _, stderr := run(t, directory, args...)
		if code != cli.ExitReject || !strings.Contains(stderr, "library.book-open") {
			t.Fatalf("%v while open: exit %d\n%s", args, code, stderr)
		}
	}
	runOK(t, directory, "book", "close", "guide")

	moveData := dataOf(t, runOK(t, directory, "book", "move", "guide", "--to", "Docs/moved"))
	if moveData["to"] != "Docs/moved" {
		t.Fatalf("move payload %v", moveData)
	}
	if _, err := os.Stat(filepath.Join(directory, "Docs", "moved", "README.md")); err != nil {
		t.Fatal("the moved book was not rendered at the new path")
	}

	renameData := dataOf(t, runOK(t, directory, "book", "rename", "guide", "--title", "New Guide"))
	if renameData["to"] != "New Guide" {
		t.Fatalf("rename payload %v", renameData)
	}
	if title := readManifest(t, directory, "guide").Root.Title; title != "New Guide" {
		t.Fatalf("manifest title %q", title)
	}
	readme := readFile(t, filepath.Join(directory, "README.md"))
	if !strings.Contains(readme, "| [New Guide](Docs/moved/README.md) |") {
		t.Fatalf("the Document Index was not refreshed:\n%s", readme)
	}

	unregisterData := dataOf(t, runOK(t, directory, "book", "unregister", "guide"))
	if unregisterData["deleted"] != false {
		t.Fatalf("unregister payload %v", unregisterData)
	}
	layout := layoutOf(t, directory)
	registry, err := store.LoadRegistry(layout.Registry())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := registry.Find("guide"); exists {
		t.Fatal("the book is still registered")
	}
	if _, err := os.Stat(filepath.Join(directory, "Docs", "moved", "README.md")); err != nil {
		t.Fatal("unregister deleted the output")
	}
	runOK(t, directory, "sync", "--check")
}

func TestMoveKeepsIdentity(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Alpha")
	root := readManifest(t, directory, "alpha").Root.ID.String()

	runOK(t, directory, "book", "open", "alpha")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "alpha", "--title", "Chapter"))["target"].(string)
	quietGrammar(t, directory)
	block := dataOf(t, runOK(t, directory, "body", "set", "--book", "alpha", "0",
		"--parent", root, "--body", "Moved body"))["block"].(string)
	reference := dataOf(t, runOK(t, directory, "body", "set", "--book", "alpha", "0",
		"--parent", root, "--body", "See [[id:"+block+"]]."))["block"].(string)

	runOK(t, directory, "move", "--book", "alpha", block, "--child-of", chapter)

	manifest := readManifest(t, directory, "alpha")
	if !manifest.IsPlaced(mustParseID(t, block)) {
		t.Fatal("the move changed the block identity")
	}
	referenceText := dataOf(t, runOK(t, directory, "body", "get", "--book", "alpha", reference))["text"].(string)
	if !strings.Contains(referenceText, "[[id:"+block+"]]") {
		t.Fatalf("the move rewrote a reference: %q", referenceText)
	}
	runOK(t, directory, "check", "--book", "alpha")
}

func TestDuplicateOwnedIDsAreRejected(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Alpha")
	runOK(t, directory, "book", "new", "--title", "Beta")

	runOK(t, directory, "book", "open", "alpha")
	alphaRoot := readManifest(t, directory, "alpha").Root.ID.String()
	quietGrammar(t, directory)
	duplicate := dataOf(t, runOK(t, directory, "body", "set", "--book", "alpha", "0",
		"--parent", alphaRoot, "--body", "Alpha body"))["block"].(string)

	// Simulate external corruption: the same owned ID appears in a second book.
	runOK(t, directory, "book", "open", "beta")
	betaManifest := readManifest(t, directory, "beta")
	betaPaths, err := layoutOf(t, directory).Book("beta")
	if err != nil {
		t.Fatal(err)
	}
	betaManifest.Root.Blocks = append(betaManifest.Root.Blocks, mustParseID(t, duplicate))
	encoded, err := betaManifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(betaPaths.Dir, "manifest.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(betaPaths.BlocksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(betaPaths.BlocksDir, duplicate+".md"), []byte("dup\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runOK(t, directory, "book", "close", "beta")

	code, _, stderr := run(t, directory, "sync", "--check")
	if code != cli.ExitFailure || !strings.Contains(stderr, "library.id-duplicate") {
		t.Fatalf("duplicate check: exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "alpha") || !strings.Contains(stderr, "beta") {
		t.Fatalf("duplicate details miss the books: %s", stderr)
	}
}

func TestCrossBookReferenceResolves(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Alpha")
	runOK(t, directory, "book", "new", "--title", "Beta")

	runOK(t, directory, "book", "open", "beta")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "beta", "--title", "Beta Chapter"))["target"].(string)
	quietGrammar(t, directory)
	block := dataOf(t, runOK(t, directory, "body", "set", "--book", "beta", "0",
		"--parent", chapter, "--body", "Beta body"))["block"].(string)

	alphaRoot := readManifest(t, directory, "alpha").Root.ID.String()
	runOK(t, directory, "book", "open", "alpha")
	runOK(t, directory, "body", "set", "--book", "alpha", "0",
		"--parent", alphaRoot, "--body", "See [[id:"+block+"]].")
	runOK(t, directory, "book", "close", "alpha")
	runOK(t, directory, "book", "close", "beta")
	runOK(t, directory, "sync", "--write")

	readme := readFile(t, filepath.Join(directory, "Docs", "alpha", "README.md"))
	if !strings.Contains(readme, "[Beta Chapter](../beta/README.md#beta-chapter)") {
		t.Fatalf("the cross-book reference did not expand:\n%s", readme)
	}
	if !strings.Contains(readme, "[← Library](../../README.md) · [Beta](../beta/README.md)") {
		t.Fatalf("the sibling book is missing from the navigation:\n%s", readme)
	}
	libraryReadme := readFile(t, filepath.Join(directory, "README.md"))
	if !strings.Contains(libraryReadme,
		"[Alpha](Docs/alpha/README.md) · [Beta](Docs/beta/README.md) · [Document Index](#document-index)") {
		t.Fatalf("the library navigation does not list the books:\n%s", libraryReadme)
	}
	runOK(t, directory, "check", "--book", "alpha")
}

func TestLinkEntryResolvesAcrossBooks(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Alpha")
	runOK(t, directory, "book", "new", "--title", "Beta")

	runOK(t, directory, "book", "open", "beta")
	chapter := dataOf(t, runOK(t, directory, "add", "--book", "beta", "--title", "Beta Chapter"))["target"].(string)
	runOK(t, directory, "book", "open", "alpha")
	runOK(t, directory, "add", "--book", "alpha", "--link",
		"--ref-book", "beta", "--ref-node", chapter, "--title", "Beta Link")
	runOK(t, directory, "add", "--book", "alpha", "--link",
		"--ref-book", "beta", "--title", "Beta Book")
	runOK(t, directory, "book", "close", "alpha")
	runOK(t, directory, "book", "close", "beta")
	runOK(t, directory, "sync", "--write")

	readme := readFile(t, filepath.Join(directory, "Docs", "alpha", "README.md"))
	if !strings.Contains(readme, "- [Beta Link](../beta/README.md#beta-chapter)") {
		t.Fatalf("the cross-book link entry did not render:\n%s", readme)
	}
	if !strings.Contains(readme, "- [Beta Book](../beta/README.md)") {
		t.Fatalf("the book link did not render:\n%s", readme)
	}
	runOK(t, directory, "check", "--book", "alpha")
}

func mustParseID(t *testing.T, raw string) model.ID {
	t.Helper()
	id, err := model.ParseID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestIssuedIDsAreGloballyUnique(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Alpha")
	runOK(t, directory, "book", "new", "--title", "Beta")

	seen := map[model.ID]struct{}{}
	record := func(id model.ID) {
		if id.IsZero() {
			return
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = struct{}{}
	}
	for _, id := range []string{"library", "alpha", "beta"} {
		manifest := readManifest(t, directory, id)
		manifest.Root.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
			record(node.ID)
			for _, block := range node.Blocks {
				record(block)
			}
			return true
		})
		for _, region := range manifest.Regions {
			record(region.ID)
			for _, block := range region.Blocks {
				record(block)
			}
		}
		for block := range manifest.BlockMeta {
			record(block)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no ids were issued")
	}
	if _, err := os.Stat(layoutOf(t, directory).IDs()); err != nil {
		t.Fatalf("the library ID ledger is missing: %v", err)
	}
}

func TestSyncDriftGate(t *testing.T) {
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "sync", "--check")

	root := readManifest(t, directory, "guide").Root.ID.String()
	runOK(t, directory, "book", "open", "guide")
	quietGrammar(t, directory)
	runOK(t, directory, "body", "set", "--book", "guide", "0", "--parent", root, "--body", "Changed")
	runOK(t, directory, "book", "close", "guide")

	code, _, stderr := run(t, directory, "sync", "--check")
	if code != cli.ExitFailure || !strings.Contains(stderr, "render.drift") {
		t.Fatalf("drift check: exit %d\n%s", code, stderr)
	}
	runOK(t, directory, "sync", "--write")
	runOK(t, directory, "sync", "--check")
}

func TestDryRunWritesNothing(t *testing.T) {
	directory := t.TempDir()
	stdout := runOK(t, directory, "book", "new", "--title", "Ghost", "--dry-run")
	if dataOf(t, stdout)["dryRun"] != true {
		t.Fatal("the dry-run receipt is missing")
	}
	if _, err := os.Stat(filepath.Join(directory, "Docs", "ghost")); !os.IsNotExist(err) {
		t.Fatal("a dry run created the output directory")
	}
	if _, err := os.Stat(filepath.Join(directory, "_CLABOK", "ghost")); !os.IsNotExist(err) {
		t.Fatal("a dry run created the container")
	}
	if _, err := os.Stat(filepath.Join(directory, "_CLABOK", "ids.json")); !os.IsNotExist(err) {
		t.Fatal("a dry run wrote the ID ledger")
	}
	runOK(t, directory, "library", "defaults", "set", "numbering", "true", "--dry-run")
	if source := dataOf(t, runOK(t, directory, "library", "defaults", "show"))["source"]; source != "builtin" {
		t.Fatalf("a dry run changed the defaults: %v", source)
	}
}

func TestLegacyDirectoryIgnored(t *testing.T) {
	directory := t.TempDir()
	legacy := filepath.Join(directory, "Assets", "Docs", "legacy")
	if err := os.MkdirAll(filepath.Join(legacy, "_source"), 0o755); err != nil {
		t.Fatal(err)
	}
	containerPath := filepath.Join(legacy, "_source", "legacy.book")
	content := []byte("legacy container bytes")
	if err := os.WriteFile(containerPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")
	runOK(t, directory, "sync", "--check")
	after, err := os.ReadFile(containerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(content) {
		t.Fatal("the legacy container was modified")
	}
}

func TestNoImportCommands(t *testing.T) {
	registry := newRegistry()
	for _, name := range []string{"import", "library import", "book adopt", "rekey", "library rekey"} {
		if _, exists := registry.Get(name); exists {
			t.Fatalf("the import command %q exists", name)
		}
	}
}
