package structure

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func run(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(directory)
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	Register(registry)
	var stdout, stderr strings.Builder
	code := cli.Run(registry, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func prepareProject(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.NewManifest("fixture", "Fixture", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children = []*model.Node{{
		ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{11},
	}}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	if err := container.Write("manifest.json", data); err != nil {
		t.Fatal(err)
	}
	if err := container.Write("blocks/11.md", []byte("Body 11\n")); err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, directory, "book", "open", "fixture"); code != cli.ExitOK {
		t.Fatalf("open failed: %s", stderr)
	}
	return directory
}

type envelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Target  string `json:"target"`
		Kind    string `json:"kind"`
		Parent  string `json:"parent"`
		DryRun  bool   `json:"dryRun"`
		Changed bool   `json:"changed"`
	} `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func parseEnvelope(t *testing.T, text string) envelope {
	t.Helper()
	var payload envelope
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("not a JSON envelope: %v\n%s", err, text)
	}
	return payload
}

func TestAddRenameMoveRemove(t *testing.T) {
	directory := prepareProject(t)

	code, stdout, stderr := run(t, directory, "add", "--book", "fixture", "--title", "1. Two")
	if code != cli.ExitOK {
		t.Fatalf("add exit %d: %s", code, stderr)
	}
	added := parseEnvelope(t, stdout)
	if added.Data.Kind != "chapter" || added.Data.Target == "" {
		t.Fatalf("unexpected add receipt %+v", added.Data)
	}

	code, _, stderr = run(t, directory, "rename", "--book", "fixture", added.Data.Target, "--title", "Zwei")
	if code != cli.ExitOK {
		t.Fatalf("rename exit %d: %s", code, stderr)
	}

	code, stdout, stderr = run(t, directory, "move", "--book", "fixture", added.Data.Target, "--child-of", "10")
	if code != cli.ExitOK {
		t.Fatalf("move exit %d: %s", code, stderr)
	}
	moved := parseEnvelope(t, stdout)
	if moved.Data.Parent != "10" || moved.Data.Kind != "subchapter" {
		t.Fatalf("unexpected move receipt %+v", moved.Data)
	}

	code, _, stderr = run(t, directory, "remove", "--book", "fixture", added.Data.Target)
	if code != cli.ExitOK {
		t.Fatalf("remove exit %d: %s", code, stderr)
	}
}

func TestBookSet(t *testing.T) {
	directory := prepareProject(t)
	if code, _, stderr := run(t, directory, "book", "set", "fixture", "numbering", "on"); code != cli.ExitOK {
		t.Fatalf("book set exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, directory, "book", "set", "fixture", "output", "chapter"); code != cli.ExitOK {
		t.Fatalf("book set output exit %d: %s", code, stderr)
	}
	if code, _, _ := run(t, directory, "book", "set", "fixture", "output", "stripe"); code == cli.ExitOK {
		t.Fatal("an invalid output mode must fail")
	}
}

func TestBookSetLanguage(t *testing.T) {
	directory := prepareProject(t)
	if code, _, stderr := run(t, directory, "config", "set", "grammar.endpoint",
		"http://127.0.0.1:1/v2/check"); code != cli.ExitOK {
		t.Fatalf("config exit %d: %s", code, stderr)
	}

	code, stdout, stderr := run(t, directory, "book", "set", "fixture", "language", "de-DE")
	if code != cli.ExitOK {
		t.Fatalf("unverified set exit %d: %s", code, stderr)
	}
	var result struct {
		Data struct {
			Warnings []grammar.Warning `json:"warnings"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("payload %s: %v", stdout, err)
	}
	if len(result.Data.Warnings) != 1 ||
		result.Data.Warnings[0].Code != grammar.LanguageUnverified {
		t.Fatalf("unverified warning %+v", result.Data.Warnings)
	}
	if language := manifestLanguage(t, directory); language != "de-DE" {
		t.Fatalf("unverified language %q", language)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"code":"en-US","longCode":"en-US","name":"English (US)"}]`)
	}))
	defer server.Close()
	if code, _, stderr := run(t, directory, "config", "set", "grammar.endpoint",
		server.URL); code != cli.ExitOK {
		t.Fatalf("config exit %d: %s", code, stderr)
	}
	code, _, stderr = run(t, directory, "book", "set", "fixture", "language", "fr-FR")
	if code == cli.ExitOK || !strings.Contains(stderr, "grammar.language-unsupported") {
		t.Fatalf("unsupported language exit %d: %s", code, stderr)
	}
	if language := manifestLanguage(t, directory); language != "de-DE" {
		t.Fatalf("a refused set changed the language to %q", language)
	}
}

func TestBookSetLibraryLayout(t *testing.T) {
	directory := prepareProject(t)
	for _, args := range [][]string{
		{"book", "set", "fixture", "library.navigation", "off"},
		{"book", "set", "fixture", "library.documentIndex", "off"},
		{"book", "set", "fixture", "library.slots", "content,toc,status"},
		{"book", "set", "fixture", "library.toc.heading", "3"},
		{"book", "set", "fixture", "library.toc.title", "Contents"},
		{"book", "set", "fixture", "library.index.columns", "title,path"},
		{"book", "set", "fixture", "library.index.shape", "table"},
	} {
		if code, _, stderr := run(t, directory, args...); code != cli.ExitOK {
			t.Fatalf("%v exit %d: %s", args, code, stderr)
		}
	}
	settings := manifestSettings(t, directory)
	if settings.Library == nil {
		t.Fatal("the library layout is missing")
	}
	layout := settings.Library
	if layout.Navigation == nil || *layout.Navigation {
		t.Fatalf("navigation %+v", layout.Navigation)
	}
	if len(layout.Slots) != 3 || layout.Slots[0] != "content" || layout.Slots[2] != "status" {
		t.Fatalf("slots %+v", layout.Slots)
	}
	if layout.Toc == nil || layout.Toc.Heading != 3 || layout.Toc.Title != "Contents" {
		t.Fatalf("toc %+v", layout.Toc)
	}
	if layout.Index == nil || layout.Index.Shape != "table" ||
		len(layout.Index.Columns) != 2 || layout.Index.Columns[1] != "path" {
		t.Fatalf("index %+v", layout.Index)
	}

	for _, args := range [][]string{
		{"book", "set", "fixture", "library.slots", "nav"},
		{"book", "set", "fixture", "library.slots", "content,content"},
		{"book", "set", "fixture", "library.index.columns", "description"},
		{"book", "set", "fixture", "library.index.shape", "grid"},
		{"book", "set", "fixture", "library.toc.heading", "9"},
	} {
		if code, _, _ := run(t, directory, args...); code == cli.ExitOK {
			t.Fatalf("%v must be refused", args)
		}
	}
}

func TestBookSetMaterializesTheDefaultSlots(t *testing.T) {
	directory := prepareProject(t)
	if code, _, stderr := run(t, directory, "book", "set", "fixture",
		"library.navigation", "off"); code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	layout := manifestSettings(t, directory).Library
	if layout == nil || len(layout.Slots) != 5 {
		t.Fatalf("the default slots must be materialized: %+v", layout)
	}
}

func manifestSettings(t *testing.T, directory string) model.Settings {
	t.Helper()
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifest.Settings
}

func manifestLanguage(t *testing.T, directory string) string {
	t.Helper()
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifest.Settings.Language
}

func TestAddDryRun(t *testing.T) {
	directory := prepareProject(t)
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(paths.Dir, lifecycle.ManifestMember)
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, directory, "add", "--book", "fixture", "--title", "Dry", "--dry-run")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	receipt := parseEnvelope(t, stdout)
	if !receipt.Data.DryRun || !receipt.Data.Changed {
		t.Fatalf("unexpected receipt %+v", receipt.Data)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a dry run wrote the manifest")
	}
}

func TestAddRequiresBook(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := run(t, directory, "add", "--title", "X")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.missing-argument") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestBlockTypeCommand(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := run(t, directory, "block", "type", "--book", "fixture", "11", "UILayout")
	if code != cli.ExitOK {
		t.Fatalf("type exit %d: %s", code, stderr)
	}
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	manifest := readManifest(t, paths)
	if manifest.BlockType(11) != "UILayout" {
		t.Fatalf("type %q", manifest.BlockType(11))
	}
	code, _, stderr = run(t, directory, "block", "type", "--book", "fixture", "11", "--clear")
	if code != cli.ExitOK {
		t.Fatalf("clear exit %d: %s", code, stderr)
	}
	if readManifest(t, paths).BlockType(11) != "" {
		t.Fatal("type not cleared")
	}

	code, stdout, stderr := run(t, directory, "add", "--book", "fixture", "--parent", "10", "--title", "Sub")
	if code != cli.ExitOK {
		t.Fatalf("sub add exit %d: %s", code, stderr)
	}
	var sub struct {
		Data struct {
			Target string `json:"target"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &sub); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = run(t, directory, "add", "--book", "fixture", "--parent", sub.Data.Target, "--type", "Note")
	if code != cli.ExitOK {
		t.Fatalf("block add exit %d: %s", code, stderr)
	}
	var block struct {
		Data struct {
			Target string `json:"target"`
			Kind   string `json:"kind"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &block); err != nil {
		t.Fatal(err)
	}
	if block.Data.Kind != "block" {
		t.Fatalf("kind %q", block.Data.Kind)
	}
	manifest = readManifest(t, paths)
	blockID, err := model.ParseID(block.Data.Target)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BlockType(blockID) != "Note" {
		t.Fatalf("block type %q", manifest.BlockType(blockID))
	}
}

func readManifest(t *testing.T, paths store.BookPaths) *model.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestMoveRequiresDestination(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := run(t, directory, "move", "--book", "fixture", "10")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.missing-argument") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}
