package search

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func TestAllCrossBookBlockResolution(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{
		All: true, Query: "Hello", Filter: Filter{Block: 41},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Books) != 1 || result.Books[0] != "second" {
		t.Fatalf("books %+v", result.Books)
	}
	if len(result.Matches) != 1 || result.Matches[0].Block != "41" {
		t.Fatalf("matches %+v", result.Matches)
	}
}

func TestSearchWritesNothingWithoutSnapshot(t *testing.T) {
	scene := newScene(t)
	before := storeTree(t, scene.Layout.Root)
	if _, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello"}); err != nil {
		t.Fatal(err)
	}
	after := storeTree(t, scene.Layout.Root)
	if len(before) != len(after) {
		t.Fatalf("a search changed the store: %d -> %d entries", len(before), len(after))
	}
	for path := range before {
		if _, exists := after[path]; !exists {
			t.Fatalf("a search removed %s", path)
		}
	}
}

func storeTree(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	entries := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries[path] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestDuplicateIDOwnerWarning(t *testing.T) {
	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := lifecycle.New(layout, locks, lifecycle.Options{UndoLimit: 10, BookPointLimit: 3})
	first, err := model.NewManifest("first", "First", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	first.Root.Children = []*model.Node{{ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{11}}}
	writeBook(t, layout, "first", first, map[string]string{"blocks/11.md": "Hello first\n"})
	second, err := model.NewManifest("second", "Second", model.NewSequenceAllocator(100))
	if err != nil {
		t.Fatal(err)
	}
	second.Root.Children = []*model.Node{{ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{101}}}
	writeBook(t, layout, "second", second, map[string]string{"blocks/101.md": "Hello second\n"})
	registry := &store.Registry{Version: store.RegistryVersion, Books: []store.RegistryEntry{
		{ID: "first", Path: "Docs/first", Kind: store.KindBook},
		{ID: "second", Path: "Docs/second", Kind: store.KindBook},
	}}
	if err := registry.Save(layout.Registry()); err != nil {
		t.Fatal(err)
	}
	service := New(life, registry, layout.Root)
	result, err := service.Find(Options{All: true, Query: "Hello", Filter: Filter{Node: 10}})
	if err != nil {
		t.Fatal(err)
	}
	duplicate := false
	for _, warning := range result.Warnings {
		if warning.Code == "search.duplicate-id" {
			duplicate = true
		}
	}
	if !duplicate {
		t.Fatalf("warnings %+v", result.Warnings)
	}
	if len(result.Books) != 2 || len(result.Matches) != 2 {
		t.Fatalf("result %+v", result)
	}
}

func TestUnsupportedSchemaWarning(t *testing.T) {
	scene := newScene(t)
	manifest, err := model.NewManifest("second", "Second", model.NewSequenceAllocator(100))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children = []*model.Node{{ID: 40, Kind: model.KindChapter, Title: "Other", Blocks: []model.ID{41}}}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	newer := bytes.Replace(data, []byte(`"version": `+strconv.Itoa(manifest.Version)),
		[]byte(`"version": 99`), 1)
	container := store.New()
	if err := container.Write("manifest.json", newer); err != nil {
		t.Fatal(err)
	}
	if err := container.Save(scene.Paths["second"].Container); err != nil {
		t.Fatal(err)
	}
	result, err := scene.Service.Find(Options{All: true, Query: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	unsupported := false
	for _, warning := range result.Warnings {
		if warning.Code == "search.unsupported-schema" && warning.Book == "second" {
			unsupported = true
		}
	}
	if !unsupported {
		t.Fatalf("warnings %+v", result.Warnings)
	}
	if len(result.Matches) != 3 {
		t.Fatalf("the surviving book lost matches: %+v", result.Matches)
	}
}
