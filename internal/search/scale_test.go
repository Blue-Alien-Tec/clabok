package search

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// scaleScene builds two closed books. The large book has chapters/blocksPerChapter blocks with
// linesPerBlock lines; with bigLines set, one extra block holds that many lines.
func scaleScene(t *testing.T, chapters, blocksPerChapter, linesPerBlock, bigLines int) (*Service, store.Layout) {
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
		if chapterIndex == 0 && bigLines > 0 {
			id := model.ID(blockID)
			blockID++
			node.Blocks = append(node.Blocks, id)
			var builder strings.Builder
			for lineIndex := 0; lineIndex < bigLines; lineIndex++ {
				fmt.Fprintf(&builder, "big line %d of block %d %s", lineIndex+1, id,
					strings.Repeat("payload ", 10))
				if lineIndex == bigLines/2 {
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
	if err := saveScaleBook(t, layout, "large", manifest, container); err != nil {
		t.Fatal(err)
	}
	corpus, err := model.NewManifest("corpus", "Corpus", model.NewSequenceAllocator(2000))
	if err != nil {
		t.Fatal(err)
	}
	corpusContainer := store.New()
	corpusNode := &model.Node{ID: 2100, Kind: model.KindChapter, Title: "Corpus"}
	for index := 0; index < 50; index++ {
		id := model.ID(2200 + index)
		corpusNode.Blocks = append(corpusNode.Blocks, id)
		body := fmt.Sprintf("corpus line %d\nneedle %d\n", index, index)
		if err := corpusContainer.Write(fmt.Sprintf("blocks/%d.md", id), []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	corpus.Root.Children = []*model.Node{corpusNode}
	if err := saveScaleBook(t, layout, "corpus", corpus, corpusContainer); err != nil {
		t.Fatal(err)
	}
	registry := &store.Registry{Version: store.RegistryVersion, Books: []store.RegistryEntry{
		{ID: "large", Path: "Docs/large", Kind: store.KindBook},
		{ID: "corpus", Path: "Docs/corpus", Kind: store.KindBook},
	}}
	if err := registry.Save(layout.Registry()); err != nil {
		t.Fatal(err)
	}
	return New(life, registry, layout.Root), layout
}

func saveScaleBook(t *testing.T, layout store.Layout, id model.BookID, manifest *model.Manifest, container *store.Container) error {
	t.Helper()
	data, err := manifest.Encode()
	if err != nil {
		return err
	}
	if err := container.Write("manifest.json", data); err != nil {
		return err
	}
	paths, err := layout.Book(id)
	if err != nil {
		return err
	}
	return container.Save(paths.Container)
}

func TestIndexedSearchScaleParity(t *testing.T) {
	service, _ := scaleScene(t, 20, 15, 8, 0)
	if _, err := service.Index.Build("large"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Index.Build("corpus"); err != nil {
		t.Fatal(err)
	}
	scan, err := service.Find(Options{All: true, Query: "needle", IndexMode: "off"})
	if err != nil {
		t.Fatal(err)
	}
	before := service.Sources.BodyReads()
	indexed, err := service.Find(Options{All: true, Query: "needle", IndexMode: "force"})
	if err != nil {
		t.Fatal(err)
	}
	if after := service.Sources.BodyReads(); after != before {
		t.Fatalf("indexed search read %d bodies", after-before)
	}
	if !indexed.Index.Used {
		t.Fatalf("index was not used: %+v", indexed.Index)
	}
	want := 20*3 + 50
	if len(scan.Matches) != want {
		t.Fatalf("scan matches %d, want %d", len(scan.Matches), want)
	}
	if !reflect.DeepEqual(scan.Matches, indexed.Matches) {
		t.Fatalf("indexed and scanned results differ")
	}
}

// TestIndexedSearchLargeFixture is the opt-in scale run: CLABOK_SCALE=1 creates a multi-megabyte
// block next to the regular corpus and repeats the parity and no-body-read checks.
func TestIndexedSearchLargeFixture(t *testing.T) {
	if os.Getenv("CLABOK_SCALE") == "" {
		t.Skip("set CLABOK_SCALE=1 for the large fixture")
	}
	service, _ := scaleScene(t, 40, 20, 10, 40000)
	if _, err := service.Index.Build("large"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Index.Build("corpus"); err != nil {
		t.Fatal(err)
	}
	scan, err := service.Find(Options{All: true, Query: "needle", IndexMode: "off"})
	if err != nil {
		t.Fatal(err)
	}
	before := service.Sources.BodyReads()
	runtime.GC()
	var beforeStats runtime.MemStats
	runtime.ReadMemStats(&beforeStats)
	started := time.Now()
	indexed, err := service.Find(Options{All: true, Query: "needle", IndexMode: "force"})
	elapsed := time.Since(started)
	var afterStats runtime.MemStats
	runtime.ReadMemStats(&afterStats)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("large indexed search: %v, heap delta %d bytes",
		elapsed, int64(afterStats.HeapAlloc)-int64(beforeStats.HeapAlloc))
	if after := service.Sources.BodyReads(); after != before {
		t.Fatalf("indexed search read %d bodies", after-before)
	}
	if !reflect.DeepEqual(scan.Matches, indexed.Matches) {
		t.Fatalf("indexed and scanned results differ on the large fixture")
	}
}
