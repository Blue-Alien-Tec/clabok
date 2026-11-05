package idspace

import (
	"os"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func newAllocator(t *testing.T) (*Allocator, store.Layout, *lifecycle.Lifecycle) {
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
	return New(life, layout), layout, life
}

func TestNextIsSequentialAndPersistent(t *testing.T) {
	allocator, layout, life := newAllocator(t)
	for want := model.ID(1); want <= 3; want++ {
		id, err := allocator.Next()
		if err != nil {
			t.Fatal(err)
		}
		if id != want {
			t.Fatalf("id %s, want %s", id, want)
		}
	}
	if _, err := os.Stat(layout.IDs()); err != nil {
		t.Fatalf("the ledger was not written: %v", err)
	}
	reopened := New(life, layout)
	id, err := reopened.Next()
	if err != nil {
		t.Fatal(err)
	}
	if id != 4 {
		t.Fatalf("reopened id %s, want 4", id)
	}
}

func TestInitializationScansRegisteredBooks(t *testing.T) {
	allocator, layout, _ := newAllocator(t)
	manifest, err := model.NewManifest("existing", "Existing", model.NewSequenceAllocator(100))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Blocks = []model.ID{101}
	manifest.Regions = []*model.Region{{
		ID: 800, Kind: model.RegionFooter, Target: model.Target{Kind: model.TargetBook},
	}}
	manifest.BlockMeta = map[model.ID]model.BlockMeta{900: {Type: "UILayout"}}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("existing")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	container := store.New()
	if err := container.Write(lifecycle.ManifestMember, data); err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	registry := &store.Registry{Version: store.RegistryVersion, Books: []store.RegistryEntry{
		{ID: "existing", Path: "Docs/existing", Kind: store.KindBook},
	}}
	if err := registry.Save(layout.Registry()); err != nil {
		t.Fatal(err)
	}
	id, err := allocator.Next()
	if err != nil {
		t.Fatal(err)
	}
	if id != 901 {
		t.Fatalf("id %s, want 901", id)
	}
}

func TestLedgerVersionRejected(t *testing.T) {
	allocator, layout, _ := newAllocator(t)
	if err := os.MkdirAll(layout.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.IDs(), []byte(`{"version":2,"next":"5"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := allocator.Next()
	if err == nil {
		t.Fatal("expected an error")
	}
	coded, ok := err.(interface{ ErrorCode() string })
	if !ok || coded.ErrorCode() != "idspace.ledger-version" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestMissingBookContainerIsARealError(t *testing.T) {
	allocator, layout, _ := newAllocator(t)
	registry := &store.Registry{Version: store.RegistryVersion, Books: []store.RegistryEntry{
		{ID: "ghost", Path: "Docs/ghost", Kind: store.KindBook},
	}}
	if err := registry.Save(layout.Registry()); err != nil {
		t.Fatal(err)
	}
	_, err := allocator.Next()
	if err == nil {
		t.Fatal("expected an error")
	}
	coded, ok := err.(interface{ ErrorCode() string })
	if !ok || coded.ErrorCode() != "idspace.book-missing" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestPeekDoesNotWrite(t *testing.T) {
	allocator, layout, _ := newAllocator(t)
	for attempt := 0; attempt < 2; attempt++ {
		id, err := allocator.Peek()
		if err != nil {
			t.Fatal(err)
		}
		if id != 1 {
			t.Fatalf("peek %s, want 1", id)
		}
		if _, err := os.Stat(layout.IDs()); !os.IsNotExist(err) {
			t.Fatalf("peek wrote the ledger: %v", err)
		}
	}
	if id, err := allocator.Next(); err != nil || id != 1 {
		t.Fatalf("next %s, %v", id, err)
	}
}

func TestIDsAreGloballyUniqueAcrossAllocators(t *testing.T) {
	_, layout, life := newAllocator(t)
	seen := map[model.ID]struct{}{}
	for instance := 0; instance < 3; instance++ {
		instanceAllocator := New(life, layout)
		for index := 0; index < 5; index++ {
			id, err := instanceAllocator.Next()
			if err != nil {
				t.Fatal(err)
			}
			if _, duplicate := seen[id]; duplicate {
				t.Fatalf("duplicate id %s", id)
			}
			seen[id] = struct{}{}
		}
	}
	if len(seen) != 15 {
		t.Fatalf("issued %d ids, want 15", len(seen))
	}
}
