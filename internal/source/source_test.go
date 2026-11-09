package source

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// newFixture builds a closed book with two chapters, three placed bodies, one region block and one
// asciiui layout. With missing set it places the block 22 without writing its body.
func newFixture(t *testing.T, missing bool) (*Service, *lifecycle.Lifecycle, store.BookPaths) {
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
	second := []model.ID{21}
	if missing {
		second = append(second, 22)
	}
	manifest.Root.Children = []*model.Node{
		{ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{11, 12}},
		{ID: 20, Kind: model.KindChapter, Title: "Two", Blocks: second},
	}
	manifest.Regions = []*model.Region{{
		ID: 30, Kind: model.RegionNotes,
		Target: model.Target{Kind: model.TargetNode, ID: 10},
		Blocks: []model.ID{31},
	}}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	members := map[string]string{
		"manifest.json":              string(data),
		"blocks/11.md":               "alpha\n",
		"blocks/12.md":               "beta\n",
		"blocks/21.md":               "gamma\n",
		"blocks/31.md":               "note\n",
		"layouts/asciiui/11.asciiui": "panel Main\n",
	}
	for name, text := range members {
		if err := container.Write(name, []byte(text)); err != nil {
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
	return New(life), life, paths
}

func TestLoadOpenAndClosedAgree(t *testing.T) {
	service, life, _ := newFixture(t, false)
	closed, err := service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if closed.Open {
		t.Fatal("a closed book reports open")
	}
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	open, err := service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !open.Open {
		t.Fatal("an extracted book does not report open")
	}
	if !bytes.Equal(closed.ManifestData, open.ManifestData) {
		t.Fatal("manifest bytes differ between open and closed")
	}
	if !reflect.DeepEqual(closed.Bodies, open.Bodies) {
		t.Fatalf("bodies differ: %v vs %v", closed.Bodies, open.Bodies)
	}
	if !reflect.DeepEqual(closed.Layouts, open.Layouts) {
		t.Fatal("layouts differ between open and closed")
	}
	if !reflect.DeepEqual(closed.MissingBodies(), open.MissingBodies()) {
		t.Fatal("missing bodies differ between open and closed")
	}
}

func TestLoadPlacementIndex(t *testing.T) {
	service, _, _ := newFixture(t, false)
	book, err := service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	node, ok := book.NodeForBlock(11)
	if !ok || node.ID != 10 {
		t.Fatalf("block 11 owner %+v, %v", node, ok)
	}
	if _, ok := book.NodeForBlock(31); ok {
		t.Fatal("a region block reports a node owner")
	}
	region, ok := book.RegionForBlock(31)
	if !ok || region.ID != 30 {
		t.Fatalf("block 31 region %+v, %v", region, ok)
	}
	if _, ok := book.RegionForBlock(11); ok {
		t.Fatal("a node block reports a region owner")
	}
	if body, ok := book.Body(12); !ok || body != "beta\n" {
		t.Fatalf("body 12 %q, %v", body, ok)
	}
	if spec, ok := book.Layout(11); !ok || string(spec) != "panel Main\n" {
		t.Fatalf("layout 11 %q, %v", spec, ok)
	}
	members := book.Members()
	if len(members) != 5 {
		t.Fatalf("members %v", members)
	}
	if _, hasLayout := members["layouts/asciiui/11.asciiui"]; hasLayout {
		t.Fatal("layouts belong to the render hash")
	}
}

func TestLoadManifestOnly(t *testing.T) {
	service, life, _ := newFixture(t, false)
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	book, err := service.LoadManifest("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Bodies) != 0 || len(book.Layouts) != 0 {
		t.Fatalf("manifest-only load read bodies %v or layouts %v", book.Bodies, book.Layouts)
	}
	if book.Manifest == nil || book.Manifest.Root == nil {
		t.Fatal("manifest missing")
	}
	if missing := book.MissingBodies(); len(missing) != 4 {
		t.Fatalf("missing bodies %v", missing)
	}
	if node, ok := book.NodeForBlock(21); !ok || node.ID != 20 {
		t.Fatalf("manifest-only index for block 21: %+v, %v", node, ok)
	}
}

func TestLoadBodiesSelective(t *testing.T) {
	service, life, _ := newFixture(t, false)
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	book, err := service.LoadBodies("fixture", []model.ID{11, 21})
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Bodies) != 2 {
		t.Fatalf("selected bodies %v", book.Bodies)
	}
	if body, ok := book.Body(11); !ok || body != "alpha\n" {
		t.Fatalf("body 11 %q, %v", body, ok)
	}
	if _, ok := book.Body(12); ok {
		t.Fatal("a body outside the selection was read")
	}
	if len(book.Layouts) != 0 {
		t.Fatal("a selective body load read layouts")
	}
	if _, ok := book.NodeForBlock(11); !ok {
		t.Fatal("the placement index is missing on a selective load")
	}
}

func TestMissingBodyIsReported(t *testing.T) {
	service, _, _ := newFixture(t, true)
	book, err := service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	missing := book.MissingBodies()
	if !reflect.DeepEqual(missing, []model.ID{22}) {
		t.Fatalf("missing bodies %v", missing)
	}
	if _, ok := book.Body(22); ok {
		t.Fatal("a missing body is present")
	}
}

func TestCorruptContainer(t *testing.T) {
	service, _, paths := newFixture(t, false)
	if err := os.WriteFile(paths.Container, []byte("not a container"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Load("fixture"); err == nil {
		t.Fatal("a corrupt container loads")
	}
}

func TestMissingManifestMember(t *testing.T) {
	service, _, paths := newFixture(t, false)
	if err := os.Remove(paths.Container); err != nil {
		t.Fatal(err)
	}
	container := store.New()
	if err := container.Write("blocks/11.md", []byte("alpha\n")); err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	_, err := service.LoadManifest("fixture")
	if err == nil {
		t.Fatal("a container without a manifest loads")
	}
	coded, ok := err.(interface{ ErrorCode() string })
	if !ok || coded.ErrorCode() != "source.manifest-missing" {
		t.Fatalf("error %v", err)
	}
}

func TestIncompleteExtractFallsBackToContainer(t *testing.T) {
	service, life, paths := newFixture(t, false)
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(paths.Dir, lifecycle.ManifestMember)); err != nil {
		t.Fatal(err)
	}
	book, err := service.Load("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if book.Open {
		t.Fatal("a fallback to the container reports open")
	}
	if len(book.Bodies) != 4 {
		t.Fatalf("fallback bodies %v", book.Bodies)
	}
}
