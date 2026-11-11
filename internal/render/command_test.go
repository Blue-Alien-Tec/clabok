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
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

func run(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(directory)
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	structure.Register(registry)
	Register(registry, nil)
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

func TestLibraryLayoutSlots(t *testing.T) {
	directory := prepareProject(t)
	out := filepath.Join(directory, "out")
	readREADME := func() string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(out, "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if code, _, stderr := run(t, directory, "sync", "--book", "fixture", "--write", "--out", out); code != cli.ExitOK {
		t.Fatalf("write exit %d: %s", code, stderr)
	}
	readme := readREADME()
	if !strings.Contains(readme, "## Index") || !strings.Contains(readme, "One") {
		t.Fatalf("default README %s", readme)
	}

	for _, setting := range [][2]string{
		{"library.navigation", "off"},
		{"library.badges", "off"},
		{"library.documentIndex", "off"},
		{"library.slots", "content"},
	} {
		if code, _, stderr := run(t, directory, "book", "set", "fixture",
			setting[0], setting[1]); code != cli.ExitOK {
			t.Fatalf("%v exit %d: %s", setting, code, stderr)
		}
	}
	if code, _, stderr := run(t, directory, "sync", "--book", "fixture", "--write", "--out", out); code != cli.ExitOK {
		t.Fatalf("minimal write exit %d: %s", code, stderr)
	}
	readme = readREADME()
	for _, absent := range []string{"## Index", "Document Index", "One"} {
		if strings.Contains(readme, absent) {
			t.Fatalf("minimal README still contains %q: %s", absent, readme)
		}
	}
	if !strings.Contains(readme, "# Book") {
		t.Fatalf("minimal README lost the title: %s", readme)
	}

	for _, setting := range [][2]string{
		{"library.slots", "content,toc"},
		{"library.toc.heading", "3"},
		{"library.toc.title", "Contents"},
		{"library.index.columns", "title,path"},
		{"library.index.shape", "table"},
	} {
		if code, _, stderr := run(t, directory, "book", "set", "fixture",
			setting[0], setting[1]); code != cli.ExitOK {
			t.Fatalf("%v exit %d: %s", setting, code, stderr)
		}
	}
	if code, _, stderr := run(t, directory, "sync", "--book", "fixture", "--write", "--out", out); code != cli.ExitOK {
		t.Fatalf("heading write exit %d: %s", code, stderr)
	}
	readme = readREADME()
	if !strings.Contains(readme, "### Contents") || !strings.Contains(readme, "| Document | Path |") {
		t.Fatalf("heading projection %s", readme)
	}

	if code, _, stderr := run(t, directory, "book", "set", "fixture",
		"library.slots", "chapters,toc"); code != cli.ExitOK {
		t.Fatalf("order exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, directory, "sync", "--book", "fixture", "--write", "--out", out); code != cli.ExitOK {
		t.Fatalf("order write exit %d: %s", code, stderr)
	}
	readme = readREADME()
	chapterAt := strings.Index(readme, "## One")
	tocAt := strings.Index(readme, "### Contents")
	if chapterAt < 0 || tocAt < 0 || chapterAt > tocAt {
		t.Fatalf("chapter must precede the index: %s", readme)
	}
}

func TestLibraryLayoutDriftAndRevert(t *testing.T) {
	directory := prepareProject(t)
	out := filepath.Join(directory, "out")
	if code, _, stderr := run(t, directory, "sync", "--book", "fixture", "--write", "--out", out); code != cli.ExitOK {
		t.Fatalf("write exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, directory, "sync", "--book", "fixture", "--check", "--out", out); code != cli.ExitOK {
		t.Fatalf("clean check exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, directory, "book", "set", "fixture",
		"library.toc.title", "Contents"); code != cli.ExitOK {
		t.Fatalf("set exit %d: %s", code, stderr)
	}
	if code, _, _ := run(t, directory, "sync", "--book", "fixture", "--check", "--out", out); code == cli.ExitOK {
		t.Fatal("a slot change must dirty the README")
	}
	if code, _, stderr := run(t, directory, "book", "set", "fixture",
		"library.toc.title", "Index"); code != cli.ExitOK {
		t.Fatalf("revert exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, directory, "sync", "--book", "fixture", "--check", "--out", out); code != cli.ExitOK {
		t.Fatalf("reverted check exit %d: %s", code, stderr)
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

func TestCheckReportsTermLayers(t *testing.T) {
	directory := prepareProject(t)
	termsDir := filepath.Join(directory, "_CLABOK", "terms")
	if err := os.MkdirAll(termsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(termsDir, "de-de.txt"), []byte("Simia\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, directory, "verify", "--book", "fixture", "--text")
	if code != cli.ExitOK {
		t.Fatalf("verify exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "languages en-US") {
		t.Fatalf("missing language summary %s", stdout)
	}
	if !strings.Contains(stdout, "2 layer(s), 1 with a file") {
		t.Fatalf("missing layer summary %s", stdout)
	}
}
