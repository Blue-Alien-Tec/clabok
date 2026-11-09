package lifecycle

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// oldBook builds a closed book whose manifest carries the given version.
func oldBook(t *testing.T, version int) (*Lifecycle, store.Layout, store.BookPaths, []byte) {
	t.Helper()
	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := New(layout, locks, Options{UndoLimit: 10, BookPointLimit: 3})
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
	if version != model.Version {
		data = bytes.Replace(data, []byte(`"version": 3`),
			[]byte(fmt.Sprintf(`"version": %d`, version)), 1)
	}
	container := store.New()
	if err := container.Write(ManifestMember, data); err != nil {
		t.Fatal(err)
	}
	if err := container.Write("blocks/11.md", []byte("Hello\n")); err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	return life, layout, paths, data
}

func codeOf(err error) string {
	coded, ok := err.(interface{ ErrorCode() string })
	if !ok {
		return ""
	}
	return coded.ErrorCode()
}

func TestWritePolicyRefusesAnOldManifest(t *testing.T) {
	life, _, _, _ := oldBook(t, 2)
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := life.Snapshot("fixture", "structure.add"); codeOf(err) != "migrate.required" {
		t.Fatalf("snapshot on v2: %v", err)
	}
	if _, err := life.SnapshotBody("fixture", 11, "body.set"); codeOf(err) != "migrate.required" {
		t.Fatalf("snapshot body on v2: %v", err)
	}
}

func TestCurrentManifestStaysWritable(t *testing.T) {
	life, _, _, _ := oldBook(t, model.Version)
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := life.Snapshot("fixture", "structure.add"); err != nil {
		t.Fatalf("snapshot on v3: %v", err)
	}
}

func TestClosePreservesAnOldManifest(t *testing.T) {
	life, _, paths, source := oldBook(t, 2)
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := life.Close("fixture"); err != nil {
		t.Fatalf("close on v2: %v", err)
	}
	container, err := store.Load(paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	packed, exists := container.Read(ManifestMember)
	if !exists {
		t.Fatal("no manifest after close")
	}
	if !bytes.Equal(packed, source) {
		t.Fatal("close rewrote the v2 manifest")
	}
	if version, err := model.PeekVersion(packed); err != nil || version != 2 {
		t.Fatalf("close wrote version %d %v", version, err)
	}
}
