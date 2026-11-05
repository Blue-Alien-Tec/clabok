package render

import (
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
	manifest, err := model.NewManifest("fixture", "Book", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children = []*model.Node{{
		ID: 10, Kind: model.KindChapter, Title: "One", Part: "Basics", Blocks: []model.ID{11},
	}}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	_ = container.Write("manifest.json", data)
	_ = container.Write("blocks/11.md", []byte("Body 11\n"))
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, directory, "book", "open", "fixture"); code != cli.ExitOK {
		t.Fatalf("open failed: %s", stderr)
	}
	return directory
}

func TestSyncCommand(t *testing.T) {
	directory := prepareProject(t)
	out := filepath.Join(directory, "out")
	code, stdout, stderr := run(t, directory, "sync", "--book", "fixture", "--out", out)
	if code != cli.ExitOK {
		t.Fatalf("check exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"written": false`) || !strings.Contains(stdout, `"changed"`) {
		t.Fatalf("unexpected check payload %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "README.md")); !os.IsNotExist(err) {
		t.Fatal("a dry sync wrote a file")
	}

	code, stdout, stderr = run(t, directory, "sync", "--book", "fixture", "--out", out, "--write")
	if code != cli.ExitOK {
		t.Fatalf("write exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"written": true`) {
		t.Fatalf("unexpected write payload %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "README.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCommand(t *testing.T) {
	directory := prepareProject(t)
	code, stdout, stderr := run(t, directory, "check", "--book", "fixture")
	if code != cli.ExitOK {
		t.Fatalf("check exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "render.open-book") {
		t.Fatalf("missing open-book finding %s", stdout)
	}

	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.BlocksDir, "11.md"), []byte("## nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = run(t, directory, "check", "--book", "fixture")
	if code != cli.ExitFailure {
		t.Fatalf("check exit %d, want %d: %s", code, cli.ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "render.check-failed") || !strings.Contains(stderr, "body.forbidden-heading") {
		t.Fatalf("unexpected failure %s", stderr)
	}
}

func TestVerifyAlias(t *testing.T) {
	directory := prepareProject(t)
	code, stdout, stderr := run(t, directory, "verify", "--book", "fixture")
	if code != cli.ExitOK {
		t.Fatalf("verify exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "render.open-book") {
		t.Fatalf("unexpected payload %s", stdout)
	}
}
