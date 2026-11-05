package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiff(t *testing.T) {
	before := New()
	if err := before.WriteText("a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := before.WriteText("b", "2"); err != nil {
		t.Fatal(err)
	}
	if err := before.WriteText("c", "3"); err != nil {
		t.Fatal(err)
	}

	after := New()
	if err := after.WriteText("a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := after.WriteText("b", "22"); err != nil {
		t.Fatal(err)
	}
	if err := after.WriteText("d", "4"); err != nil {
		t.Fatal(err)
	}

	changes := Diff(before, after)
	want := []Change{
		{Name: "b", Kind: ChangeChanged},
		{Name: "c", Kind: ChangeRemoved},
		{Name: "d", Kind: ChangeAdded},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes %+v, want %+v", changes, want)
	}
	if len(Diff(before, before)) != 0 {
		t.Fatal("identical containers differ")
	}
}

func TestExport(t *testing.T) {
	container := fixtureContainer(t)
	directory := filepath.Join(t.TempDir(), "export")
	written, err := container.Export(directory)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"blocks/1.md", "blocks/2.md", "manifest.json"}
	if strings.Join(written, ",") != strings.Join(want, ",") {
		t.Fatalf("written %v, want %v", written, want)
	}
	body, err := os.ReadFile(filepath.Join(directory, "blocks", "1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "First body\n" {
		t.Fatalf("body %q", body)
	}
	empty, err := os.ReadFile(filepath.Join(directory, "blocks", "2.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty body %q", empty)
	}

	if err := container.WriteText("blocks/1.md", "Changed\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := container.Export(directory); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(filepath.Join(directory, "blocks", "1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Changed\n" {
		t.Fatalf("second export did not replace: %q", body)
	}

	entries, err := os.ReadDir(filepath.Join(directory, "blocks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", entry.Name())
		}
	}
}

func TestExportRejectsEmptyDirectory(t *testing.T) {
	_, err := New().Export("   ")
	expectCode(t, err, "store.invalid-root")
}
