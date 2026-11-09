package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestStoreMarker(t *testing.T) {
	layout, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	version, err := PeekStoreVersion(layout.StoreMarkerPath())
	if err != nil || version != 0 {
		t.Fatalf("missing marker %d %v", version, err)
	}
	if err := WriteStoreVersion(layout.StoreMarkerPath(), StoreVersion); err != nil {
		t.Fatal(err)
	}
	version, err = PeekStoreVersion(layout.StoreMarkerPath())
	if err != nil || version != StoreVersion {
		t.Fatalf("written marker %d %v", version, err)
	}
	if err := WriteStoreVersion(layout.StoreMarkerPath(), 99); err != nil {
		t.Fatal(err)
	}
	version, err = PeekStoreVersion(layout.StoreMarkerPath())
	if err != nil || version != 99 {
		t.Fatalf("unknown marker %d %v", version, err)
	}
	if err := os.WriteFile(layout.StoreMarkerPath(), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PeekStoreVersion(layout.StoreMarkerPath()); err == nil {
		t.Fatal("a corrupt marker reads")
	}
}

func TestStoreNamesAreReserved(t *testing.T) {
	for _, id := range []string{"store.json", "migrations", "index", "STORE.JSON"} {
		if err := ValidateBookID(model.BookID(id)); err == nil {
			t.Fatalf("%q is not reserved", id)
		}
	}
}

func TestMigrationsRoot(t *testing.T) {
	root := t.TempDir()
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if layout.MigrationsRoot() != filepath.Join(root, "migrations") {
		t.Fatalf("migrations root %q", layout.MigrationsRoot())
	}
}
