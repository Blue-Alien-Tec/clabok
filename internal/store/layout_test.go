package store

import (
	"path/filepath"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestNewLayout(t *testing.T) {
	root := t.TempDir()
	layout, err := NewLayout(root + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	if layout.Root != filepath.Clean(root) {
		t.Fatalf("root %q", layout.Root)
	}

	if _, err := NewLayout("   "); err == nil {
		t.Fatal("an empty root must be rejected")
	} else {
		expectCode(t, err, "store.invalid-root")
	}
	if _, err := NewLayout(filepath.Join("relative", "store")); err == nil {
		t.Fatal("a relative root must be rejected")
	} else {
		expectCode(t, err, "store.invalid-root")
	}
}

func TestValidateBookID(t *testing.T) {
	valid := []model.BookID{"guide", "clabok-planning", "library", "a"}
	for _, id := range valid {
		if err := ValidateBookID(id); err != nil {
			t.Fatalf("%q rejected: %v", id, err)
		}
	}
	invalid := []model.BookID{
		"rollback", "ROLLBACK", "rollback.", ".coordinator", ".Coordinator",
		"library.json", "LIBRARY.JSON", "guide.", "guide:stream", "C:", "guide*",
	}
	for _, id := range invalid {
		if err := ValidateBookID(id); err == nil {
			t.Fatalf("%q accepted", id)
		} else {
			expectCode(t, err, "store.reserved-book-id")
		}
	}
	if err := ValidateBookID("a/b"); err == nil {
		t.Fatal("a separator must be rejected")
	} else {
		expectCode(t, err, "store.invalid-book-id")
	}
	if BookIDKey("Guide") != BookIDKey("guide") {
		t.Fatal("book id keys are case-sensitive")
	}
	if PathKey("Docs/Guide") != PathKey("Docs/guide") {
		t.Fatal("path keys are case-sensitive")
	}
	if PathKey("Docs/Guide.") != PathKey("Docs/guide") {
		t.Fatal("a trailing dot is not normalized in a path key")
	}
	if PathKey(".") != "." {
		t.Fatalf("the library path key is %q", PathKey("."))
	}
}

func TestLayoutSharedPaths(t *testing.T) {
	root := t.TempDir()
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Registry() != filepath.Join(root, "library.json") {
		t.Fatalf("registry %q", layout.Registry())
	}
	if layout.Terms() != filepath.Join(root, "terms.txt") {
		t.Fatalf("terms %q", layout.Terms())
	}
	if layout.RollbackRoot() != filepath.Join(root, "rollback") {
		t.Fatalf("rollback root %q", layout.RollbackRoot())
	}
	if layout.Coordinator() != filepath.Join(root, ".coordinator") {
		t.Fatalf("coordinator dir %q", layout.Coordinator())
	}
	if layout.OpenState() != filepath.Join(root, "open.json") {
		t.Fatalf("open state %q", layout.OpenState())
	}
}

func TestBookPaths(t *testing.T) {
	root := t.TempDir()
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("clabok")
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "clabok")
	expected := BookPaths{
		ID:          "clabok",
		Dir:         directory,
		Container:   filepath.Join(directory, "clabok.book"),
		BlocksDir:   filepath.Join(directory, "blocks"),
		LayoutsDir:  filepath.Join(directory, "layouts"),
		RollbackDir: filepath.Join(root, "rollback", "clabok"),
		StateFile:   filepath.Join(directory, "state.json"),
	}
	if paths != expected {
		t.Fatalf("paths %+v, want %+v", paths, expected)
	}
}

func TestBookRejectsInvalidID(t *testing.T) {
	layout, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []model.BookID{"", ".", "..", "a/b", `a\b`, "with space"} {
		_, bookErr := layout.Book(id)
		expectCode(t, bookErr, "store.invalid-book-id")
	}
}
