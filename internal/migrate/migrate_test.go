package migrate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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

func newScene(t *testing.T, manifestVersion, registryVersion int) *scene {
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
	manifest.Regions = []*model.Region{{
		ID: 20, Kind: model.RegionProse,
		Target: model.Target{Kind: model.TargetNode, ID: 10}, Blocks: []model.ID{21},
	}}
	manifest.BlockMeta = map[model.ID]model.BlockMeta{11: {Type: "text"}}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if manifestVersion != model.Version {
		data = bytes.Replace(data, []byte(fmt.Sprintf(`"version": %d`, model.Version)),
			[]byte(fmt.Sprintf(`"version": %d`, manifestVersion)), 1)
	}
	container := store.New()
	for name, body := range map[string]string{
		"manifest.json": string(data),
		"blocks/11.md":  "Hello world\n",
		"blocks/21.md":  "Index prose\n",
	} {
		if err := container.Write(name, []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	registry := &store.Registry{Version: registryVersion, Books: []store.RegistryEntry{
		{ID: "fixture", Path: "Docs/fixture", Kind: store.KindBook},
	}}
	registryData, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(layout.Registry(), append(registryData, '\n')); err != nil {
		t.Fatal(err)
	}
	return &scene{
		Service: New(life, layout, registry, 3, ""),
		Life:    life, Layout: layout, Paths: paths,
	}
}

func containerVersion(t *testing.T, paths store.BookPaths) int {
	t.Helper()
	container, err := store.Load(paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	manifest, exists := container.Read("manifest.json")
	if !exists {
		t.Fatal("no manifest")
	}
	version, err := model.PeekVersion(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func readMember(t *testing.T, paths store.BookPaths, name string) []byte {
	t.Helper()
	container, err := store.Load(paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	data, exists := container.Read(name)
	if !exists {
		t.Fatalf("%s missing", name)
	}
	return data
}

func TestStatusReportsPendingSteps(t *testing.T) {
	scene := newScene(t, 2, 1)
	report, err := scene.Service.Status("fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Books) != 1 {
		t.Fatalf("books %+v", report.Books)
	}
	book := report.Books[0]
	if book.State != "closed" || book.Manifest.From != 2 || !book.Manifest.Supported {
		t.Fatalf("book %+v", book)
	}
	if len(book.Manifest.Steps) != 2 || book.Manifest.Steps[0] != "manifest-v2-v3" ||
		book.Manifest.Steps[1] != "manifest-v3-v4" {
		t.Fatalf("steps %+v", book.Manifest.Steps)
	}
	if report.Registry.From != 1 || len(report.Registry.Steps) != 1 {
		t.Fatalf("registry %+v", report.Registry)
	}
	if report.Store.From != 0 || len(report.Store.Steps) != 1 {
		t.Fatalf("store %+v", report.Store)
	}
	if _, err := os.Stat(scene.Layout.MigrationsRoot()); !os.IsNotExist(err) {
		t.Fatal("status wrote the migrations directory")
	}
	if _, err := os.Stat(scene.Layout.StoreMarkerPath()); !os.IsNotExist(err) {
		t.Fatal("status wrote the store marker")
	}
}

func TestApplyAllMigratesBookRegistryAndStore(t *testing.T) {
	scene := newScene(t, 2, 1)
	if err := os.WriteFile(scene.Paths.StateFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	bodyBefore := readMember(t, scene.Paths, "blocks/11.md")
	report, err := scene.Service.Apply("", true, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.DryRun {
		t.Fatal("apply reported a dry run")
	}
	if containerVersion(t, scene.Paths) != model.Version {
		t.Fatalf("container version %d", containerVersion(t, scene.Paths))
	}
	if version, err := store.PeekRegistryVersion(scene.Layout.Registry()); err != nil || version != store.RegistryVersion {
		t.Fatalf("registry version %d %v", version, err)
	}
	if version, err := store.PeekStoreVersion(scene.Layout.StoreMarkerPath()); err != nil || version != store.StoreVersion {
		t.Fatalf("store version %d %v", version, err)
	}
	if _, err := os.Stat(scene.Paths.StateFile); !os.IsNotExist(err) {
		t.Fatal("the derived state was not reset")
	}
	bodyAfter := readMember(t, scene.Paths, "blocks/11.md")
	if !bytes.Equal(bodyBefore, bodyAfter) {
		t.Fatal("a body changed during migration")
	}
	entries, err := os.ReadDir(scene.Layout.MigrationsRoot())
	if err != nil || len(entries) != 1 {
		t.Fatalf("migrations %v %v", entries, err)
	}
	directory := filepath.Join(scene.Layout.MigrationsRoot(), entries[0].Name())
	receiptData, err := os.ReadFile(filepath.Join(directory, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt Report
	if err := json.Unmarshal(receiptData, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Books) != 1 || !receipt.Books[0].Changed || receipt.Books[0].Backup == "" {
		t.Fatalf("receipt %+v", receipt)
	}
	if _, err := os.Stat(scene.Paths.RollbackDir); err != nil {
		t.Fatalf("rollback pool missing: %v", err)
	}
}

func TestPlanWritesNothing(t *testing.T) {
	scene := newScene(t, 2, 1)
	report, err := scene.Service.Apply("", true, 0, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun || !report.Books[0].Changed {
		t.Fatalf("plan %+v", report)
	}
	if _, err := os.Stat(scene.Layout.MigrationsRoot()); !os.IsNotExist(err) {
		t.Fatal("plan wrote the migrations directory")
	}
	if _, err := os.Stat(scene.Layout.StoreMarkerPath()); !os.IsNotExist(err) {
		t.Fatal("plan wrote the store marker")
	}
	if containerVersion(t, scene.Paths) != 2 {
		t.Fatal("plan changed the container")
	}
}

func TestOpenBookIsRejected(t *testing.T) {
	scene := newScene(t, 2, 1)
	if _, err := scene.Life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := scene.Service.Apply("fixture", false, 0, true, false); errorCode(err) != "migrate.book-open" {
		t.Fatalf("open apply: %v", err)
	}
}

func TestDowngradeIsRejected(t *testing.T) {
	scene := newScene(t, 3, 2)
	if _, err := scene.Service.Apply("fixture", false, 2, false, false); errorCode(err) != "migrate.downgrade" {
		t.Fatalf("downgrade: %v", err)
	}
}

func TestUnsupportedSource(t *testing.T) {
	scene := newScene(t, 1, 2)
	report, err := scene.Service.Status("fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Books[0].Manifest.Supported {
		t.Fatal("a v1 manifest reports supported")
	}
	if _, err := scene.Service.Apply("fixture", false, 0, false, false); errorCode(err) != "migrate.unsupported-source" {
		t.Fatalf("v1 apply: %v", err)
	}
}

func TestVerifyFailureRestoresTheContainer(t *testing.T) {
	scene := newScene(t, 2, 1)
	scene.Service.afterWrite = func() error { return errors.New("injected") }
	if _, err := scene.Service.Apply("fixture", false, 0, true, false); errorCode(err) != "migrate.verify-failed" {
		t.Fatalf("injected failure: %v", err)
	}
	if containerVersion(t, scene.Paths) != 2 {
		t.Fatal("the container was not restored")
	}
}

func TestCurrentBookIsANoOp(t *testing.T) {
	scene := newScene(t, model.Version, 2)
	report, err := scene.Service.Apply("fixture", false, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Books[0].Changed {
		t.Fatalf("a current book changed: %+v", report.Books[0])
	}
	if _, err := os.Stat(scene.Layout.MigrationsRoot()); !os.IsNotExist(err) {
		t.Fatal("a no-op wrote a receipt")
	}
}

func TestRegistryOnlyMovesOnAll(t *testing.T) {
	scene := newScene(t, model.Version, 1)
	if _, err := scene.Service.Apply("fixture", false, 0, true, false); err != nil {
		t.Fatal(err)
	}
	if version, _ := store.PeekRegistryVersion(scene.Layout.Registry()); version != 1 {
		t.Fatalf("--book migrated the registry to %d", version)
	}
	if _, err := scene.Service.Apply("", true, 0, true, false); err != nil {
		t.Fatal(err)
	}
	if version, _ := store.PeekRegistryVersion(scene.Layout.Registry()); version != store.RegistryVersion {
		t.Fatalf("--all left the registry at %d", version)
	}
}

func errorCode(err error) string {
	coded, ok := err.(interface{ ErrorCode() string })
	if !ok {
		return ""
	}
	return coded.ErrorCode()
}
