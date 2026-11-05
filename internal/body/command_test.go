package body

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
	manifest.Root.Children = []*model.Node{
		{ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{11, 12}},
		{ID: 20, Kind: model.KindChapter, Title: "Two", Blocks: []model.ID{21}},
	}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	_ = container.Write("manifest.json", data)
	_ = container.Write("blocks/11.md", []byte("Body 11\n"))
	_ = container.Write("blocks/12.md", []byte("Body 12\n"))
	_ = container.Write("blocks/21.md", []byte("Body 21\n"))
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, directory, "book", "open", "fixture"); code != cli.ExitOK {
		t.Fatalf("open failed: %s", stderr)
	}
	return directory
}

func decode(t *testing.T, text string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	return payload
}

func TestBodySetGetList(t *testing.T) {
	directory := prepareProject(t)
	if code, _, stderr := run(t, directory, "body", "set", "--book", "fixture", "11", "--body", "Hello\n"); code != cli.ExitOK {
		t.Fatalf("set exit %d: %s", code, stderr)
	}

	code, stdout, stderr := run(t, directory, "body", "get", "--book", "fixture", "11")
	if code != cli.ExitOK {
		t.Fatalf("get exit %d: %s", code, stderr)
	}
	payload := decode(t, stdout)
	data := payload["data"].(map[string]any)
	if data["text"] != "Hello\n" || data["totalLines"] != float64(1) {
		t.Fatalf("unexpected get %+v", data)
	}

	code, stdout, stderr = run(t, directory, "body", "list", "--book", "fixture")
	if code != cli.ExitOK {
		t.Fatalf("list exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"depth": 2`) {
		t.Fatalf("depth missing: %s", stdout)
	}
}

func TestBodyHeadingReject(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := run(t, directory, "body", "set", "--book", "fixture", "11", "--body", "## no\n")
	if code != cli.ExitReject {
		t.Fatalf("exit %d, want %d: %s", code, cli.ExitReject, stderr)
	}
	if !strings.Contains(stderr, "body.forbidden-heading") {
		t.Fatalf("unexpected reject %s", stderr)
	}
}

func TestBodyCreateBlock(t *testing.T) {
	directory := prepareProject(t)
	code, stdout, stderr := run(t, directory, "body", "set", "--book", "fixture", "0", "--parent", "10", "--body", "New\n")
	if code != cli.ExitOK {
		t.Fatalf("create exit %d: %s", code, stderr)
	}
	payload := decode(t, stdout)
	data := payload["data"].(map[string]any)
	if data["created"] != true {
		t.Fatalf("unexpected receipt %+v", data)
	}
	block, _ := data["block"].(string)
	if block == "" || block == "0" {
		t.Fatalf("block %q", block)
	}
	code, stdout, stderr = run(t, directory, "body", "get", "--book", "fixture", block)
	if code != cli.ExitOK {
		t.Fatalf("get new block exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"text": "New\n"`) {
		t.Fatalf("new body missing: %s", stdout)
	}
}

func TestBodyReplaceCommand(t *testing.T) {
	directory := prepareProject(t)
	if code, _, stderr := run(t, directory, "body", "set", "--book", "fixture", "11", "--body", "Hello world\n"); code != cli.ExitOK {
		t.Fatalf("set failed: %s", stderr)
	}
	code, _, stderr := run(t, directory, "body", "replace", "--book", "fixture", "11",
		"--find", "Hello", "--replace", "Hi", "--expect", "1")
	if code != cli.ExitOK {
		t.Fatalf("replace exit %d: %s", code, stderr)
	}
	code, stdout, _ := run(t, directory, "body", "get", "--book", "fixture", "11")
	if code != cli.ExitOK || !strings.Contains(stdout, "Hi world") {
		t.Fatalf("unexpected body %s", stdout)
	}

	code, _, stderr = run(t, directory, "body", "replace", "--book", "fixture", "11",
		"--find", "Hi", "--replace", "X", "--expect", "2")
	if code != cli.ExitReject || !strings.Contains(stderr, "body.expectation-failed") {
		t.Fatalf("expectation exit %d: %s", code, stderr)
	}
}

func TestBodyDryRun(t *testing.T) {
	directory := prepareProject(t)
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(paths.BlocksDir, "11.md"))
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, directory, "body", "set", "--book", "fixture", "11", "--body", "Dry\n", "--dry-run")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"dryRun": true`) {
		t.Fatalf("unexpected receipt %s", stdout)
	}
	after, err := os.ReadFile(filepath.Join(paths.BlocksDir, "11.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a dry run wrote the body")
	}
}

func TestBodyUsage(t *testing.T) {
	directory := prepareProject(t)
	code, _, stderr := run(t, directory, "body", "set", "11", "--body", "x")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.missing-argument") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	code, _, stderr = run(t, directory, "body", "get", "--book", "fixture", "0")
	if code != cli.ExitUsage || !strings.Contains(stderr, "usage.invalid-value") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}
