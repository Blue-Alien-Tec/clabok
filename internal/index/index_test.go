package index

import (
	"bytes"
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

type scene struct {
	Service *Service
	Life    *lifecycle.Lifecycle
	Layout  store.Layout
	Paths   store.BookPaths
}

func newScene(t *testing.T) *scene {
	t.Helper()
	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := lifecycle.New(layout, locks, lifecycle.Options{UndoLimit: 10, BookPointLimit: 3})
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
	if err := container.Write("blocks/11.md", []byte("Hello world\nHello again\n")); err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	registry := &store.Registry{Version: store.RegistryVersion, Books: []store.RegistryEntry{
		{ID: "fixture", Path: "Docs/fixture", Kind: store.KindBook},
	}}
	if err := registry.Save(layout.Registry()); err != nil {
		t.Fatal(err)
	}
	return &scene{Service: New(life, registry, layout.Root), Life: life, Layout: layout, Paths: paths}
}

func TestBuildStatusAndClear(t *testing.T) {
	scene := newScene(t)
	status, err := scene.Service.Status("fixture")
	if err != nil || status.State != "missing" {
		t.Fatalf("initial status %+v %v", status, err)
	}
	built, err := scene.Service.Build("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if built.State != "fresh" || built.Version != Version {
		t.Fatalf("built %+v", built)
	}
	status, err = scene.Service.Status("fixture")
	if err != nil || status.State != "fresh" {
		t.Fatalf("status after build %+v %v", status, err)
	}
	derived, err := scene.Service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(derived.Entries) != 4 {
		t.Fatalf("entries %+v", derived.Entries)
	}
	if len(derived.Tokens["hello"]) != 2 {
		t.Fatalf("token postings %+v", derived.Tokens["hello"])
	}
	if len(derived.Trigrams) == 0 {
		t.Fatal("no trigrams")
	}
	if err := scene.Service.Clear("fixture"); err != nil {
		t.Fatal(err)
	}
	status, err = scene.Service.Status("fixture")
	if err != nil || status.State != "missing" {
		t.Fatalf("status after clear %+v %v", status, err)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.Service.Build("fixture"); err != nil {
		t.Fatal(err)
	}
	firstPostings, err := os.ReadFile(scene.Service.postingsPath("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	firstLines, err := os.ReadFile(scene.Service.linesPath("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scene.Service.Build("fixture"); err != nil {
		t.Fatal(err)
	}
	secondPostings, err := os.ReadFile(scene.Service.postingsPath("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	secondLines, err := os.ReadFile(scene.Service.linesPath("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstPostings, secondPostings) || !bytes.Equal(firstLines, secondLines) {
		t.Fatal("two builds of the same input differ")
	}
}

func TestStaleAndCorrupt(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.Service.Build("fixture"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scene.Paths.Container, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := scene.Service.Status("fixture")
	if err != nil || status.State != "stale" {
		t.Fatalf("stale status %+v %v", status, err)
	}
	if err := os.WriteFile(scene.Service.postingsPath("fixture"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err = scene.Service.Status("fixture")
	if err != nil || status.State != "corrupt" {
		t.Fatalf("corrupt status %+v %v", status, err)
	}
}

func TestOpenBookIsSkipped(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.Life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	status, err := scene.Service.Status("fixture")
	if err != nil || status.State != "open" {
		t.Fatalf("open status %+v %v", status, err)
	}
	if _, err := scene.Service.Build("fixture"); err == nil {
		t.Fatal("an open book was indexed")
	} else if coded, ok := err.(interface{ ErrorCode() string }); !ok || coded.ErrorCode() != "index.open-book" {
		t.Fatalf("build error %v", err)
	}
}

func TestLoadCachesByFingerprint(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.Service.Build("fixture"); err != nil {
		t.Fatal(err)
	}
	first, err := scene.Service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) == 0 {
		t.Fatal("empty index")
	}
	tampered := `{"version":1,"book":"fixture","entries":[],"tokens":{},"trigrams":{}}`
	if err := os.WriteFile(scene.Service.postingsPath("fixture"), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	cached, err := scene.Service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(cached.Entries) != len(first.Entries) {
		t.Fatal("the cache did not serve the unchanged fingerprint")
	}
	if _, err := scene.Service.Build("fixture"); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := scene.Service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.Entries) != len(first.Entries) {
		t.Fatalf("rebuilt entries %d", len(rebuilt.Entries))
	}
}

func TestBuildRespectsTheBookLock(t *testing.T) {
	scene := newScene(t)
	release, err := scene.Life.LockBookExclusive("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scene.Service.Build("fixture"); err == nil {
		release()
		t.Fatal("build ignored the held book lock")
	} else if coded, ok := err.(interface{ ErrorCode() string }); !ok || coded.ErrorCode() != "index.lock-failed" {
		release()
		t.Fatalf("build error %v", err)
	}
	release()
	if _, err := scene.Service.Build("fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestStatusVerifyDetectsAFullHashChange(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.Service.Build("fixture"); err != nil {
		t.Fatal(err)
	}
	manifest, err := scene.Service.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Books) != 1 || manifest.Books[0].FullHash == "" {
		t.Fatalf("manifest %+v", manifest)
	}
	manifest.Books[0].FullHash = "0000"
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scene.Service.manifestPath(), data, 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := scene.Service.Status("fixture")
	if err != nil || status.State != "fresh" {
		t.Fatalf("cheap status %+v %v", status, err)
	}
	verified, err := scene.Service.StatusVerified("fixture")
	if err != nil || verified.State != "stale" {
		t.Fatalf("verified status %+v %v", verified, err)
	}
}

func TestIndexStatusVerifyCommand(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.Service.Build("fixture"); err != nil {
		t.Fatal(err)
	}
	manifest, err := scene.Service.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Books[0].FullHash = "0000"
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scene.Service.manifestPath(), data, 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, scene.Layout.Root, commandArgs(scene, "index", "status", "--book", "fixture")...)
	if code != cli.ExitOK || !strings.Contains(stdout, `"state": "fresh"`) {
		t.Fatalf("plain status %d: %s %s", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "index", "status", "--book", "fixture", "--verify")...)
	if code != cli.ExitOK || !strings.Contains(stdout, `"state": "stale"`) {
		t.Fatalf("verified status %d: %s %s", code, stdout, stderr)
	}
	code, _, _ = run(t, scene.Layout.Root, commandArgs(scene, "index", "build", "--book", "fixture", "--verify")...)
	if code != cli.ExitUsage {
		t.Fatalf("verify on build exit %d", code)
	}
}

func TestIndexIsReserved(t *testing.T) {
	if err := store.ValidateBookID("index"); err == nil {
		t.Fatal("index is not a reserved book id")
	}
}

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

func commandArgs(scene *scene, args ...string) []string {
	return append(args, "--store-root", scene.Layout.Root, "--project", scene.Layout.Root)
}

func TestIndexCommand(t *testing.T) {
	scene := newScene(t)
	code, stdout, stderr := run(t, scene.Layout.Root, commandArgs(scene, "index", "status")...)
	if code != cli.ExitOK {
		t.Fatalf("status exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"state": "missing"`) || !strings.Contains(stdout, `"dryRun": false`) {
		t.Fatalf("status payload %s", stdout)
	}
	code, stdout, stderr = run(t, scene.Layout.Root, commandArgs(scene, "index", "build")...)
	if code != cli.ExitReject {
		t.Fatalf("build without scope exit %d: %s", code, stderr)
	}
	code, stdout, stderr = run(t, scene.Layout.Root, commandArgs(scene, "index", "build", "--book", "fixture")...)
	if code != cli.ExitOK {
		t.Fatalf("dry build exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"dryRun": true`) || strings.Contains(stdout, `"written": true`) {
		t.Fatalf("dry build payload %s", stdout)
	}
	code, stdout, stderr = run(t, scene.Layout.Root, commandArgs(scene, "index", "build", "--book", "fixture", "--write")...)
	if code != cli.ExitOK {
		t.Fatalf("build exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"written": true`) || !strings.Contains(stdout, `"state": "built"`) {
		t.Fatalf("build payload %s", stdout)
	}
	code, stdout, stderr = run(t, scene.Layout.Root, commandArgs(scene, "index", "status", "--book", "fixture")...)
	if code != cli.ExitOK || !strings.Contains(stdout, `"state": "fresh"`) {
		t.Fatalf("status after build %d: %s %s", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, scene.Layout.Root, commandArgs(scene, "index", "clear", "--book", "fixture", "--write")...)
	if code != cli.ExitOK || !strings.Contains(stdout, `"state": "cleared"`) {
		t.Fatalf("clear %d: %s %s", code, stdout, stderr)
	}
	if _, err := os.Stat(scene.Service.postingsPath("fixture")); !os.IsNotExist(err) {
		t.Fatal("clear left the postings file")
	}
	if _, err := os.Stat(scene.Service.linesPath("fixture")); !os.IsNotExist(err) {
		t.Fatal("clear left the lines file")
	}
}

func TestIndexCommandUnknownBook(t *testing.T) {
	scene := newScene(t)
	code, _, _ := run(t, scene.Layout.Root, commandArgs(scene, "index", "build", "--book", "missing")...)
	if code != cli.ExitReject {
		t.Fatalf("unknown book exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(scene.Layout.Root, "index")); !os.IsNotExist(err) {
		t.Fatal("a failed command created the index directory")
	}
}
