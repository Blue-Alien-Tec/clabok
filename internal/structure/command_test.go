package structure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
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

func TestMoveRequiresDestination(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := run(t, directory, "move", "--book", "fixture", "10")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.missing-argument") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}
