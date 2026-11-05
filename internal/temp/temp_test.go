package temp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAndDelete(t *testing.T) {
	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixture", "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other", "b.md"), []byte("bb"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := service.List("fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Book != "fixture" {
		t.Fatalf("entries %+v", entries)
	}
	all, err := service.List("", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("entries %+v", all)
	}

	deleted, err := service.Delete(entries)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted %d", deleted)
	}
	if _, err := os.Stat(filepath.Join(root, "fixture", "a.md")); !os.IsNotExist(err) {
		t.Fatalf("file still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "other", "b.md")); err != nil {
		t.Fatalf("unrelated file removed: %v", err)
	}
}

func TestScopeRequired(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.List("", false); err == nil {
		t.Fatal("expected temp.scope-required")
	}
}

func TestDeleteRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Delete([]Entry{{Path: outside}}); err == nil {
		t.Fatal("expected temp.invalid-path")
	}
}

func TestMissingRootIsEmpty(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := service.List("fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries %+v", entries)
	}
}
