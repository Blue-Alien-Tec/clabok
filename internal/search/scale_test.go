package search

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func TestIndexedSearchScaleParity(t *testing.T) {
	const chapters = 20
	const blocksPerChapter = 15
	const linesPerBlock = 8

	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := lifecycle.New(layout, locks, lifecycle.Options{UndoLimit: 10, BookPointLimit: 3})
	manifest, err := model.NewManifest("large", "Large", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	nodeID := 100
	blockID := 1000
	for chapterIndex := 0; chapterIndex < chapters; chapterIndex++ {
		node := &model.Node{
			ID: model.ID(nodeID), Kind: model.KindChapter,
			Title: fmt.Sprintf("Chapter %d", chapterIndex),
		}
		nodeID++
		for blockIndex := 0; blockIndex < blocksPerChapter; blockIndex++ {
			id := model.ID(blockID)
			blockID++
			node.Blocks = append(node.Blocks, id)
			var builder strings.Builder
			for lineIndex := 0; lineIndex < linesPerBlock; lineIndex++ {
				fmt.Fprintf(&builder, "line %d of block %d", lineIndex+1, id)
				if blockIndex%5 == 0 && lineIndex == 3 {
					builder.WriteString(" needle")
				}
				builder.WriteString("\n")
			}
			if err := container.Write(fmt.Sprintf("blocks/%d.md", id), []byte(builder.String())); err != nil {
				t.Fatal(err)
			}
		}
		manifest.Root.Children = append(manifest.Root.Children, node)
	}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Write("manifest.json", data); err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("large")
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	registry := &store.Registry{Version: store.RegistryVersion, Books: []store.RegistryEntry{
		{ID: "large", Path: "Docs/large", Kind: store.KindBook},
	}}
	if err := registry.Save(layout.Registry()); err != nil {
		t.Fatal(err)
	}
	service := New(life, registry, layout.Root)
	if _, err := service.Index.Build("large"); err != nil {
		t.Fatal(err)
	}
	scan, err := service.Find(Options{Book: "large", Query: "needle", IndexMode: "off"})
	if err != nil {
		t.Fatal(err)
	}
	indexed, err := service.Find(Options{Book: "large", Query: "needle", IndexMode: "force"})
	if err != nil {
		t.Fatal(err)
	}
	if !indexed.Index.Used {
		t.Fatalf("index was not used: %+v", indexed.Index)
	}
	want := chapters * (blocksPerChapter / 5)
	if len(scan.Matches) != want {
		t.Fatalf("scan matches %d, want %d", len(scan.Matches), want)
	}
	if !reflect.DeepEqual(scan.Matches, indexed.Matches) {
		t.Fatalf("indexed and scanned results differ")
	}
}
