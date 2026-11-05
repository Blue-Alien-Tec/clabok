package rollback

import (
	"fmt"
	"testing"
)

func storeIn(t *testing.T, undoLimit, bookLimit int) *Store {
	t.Helper()
	store_, err := New(t.TempDir(), undoLimit, bookLimit)
	if err != nil {
		t.Fatal(err)
	}
	return store_
}

func memberSet(body string) map[string][]byte {
	return map[string][]byte{
		"manifest.json": []byte("{}\n"),
		"blocks/1.md":   []byte(body),
	}
}

func countOf(t *testing.T, store_ *Store, kind Kind) int {
	t.Helper()
	packages, err := store_.List()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, pkg := range packages {
		if pkg.Kind == kind {
			count++
		}
	}
	return count
}

func TestSnapshotListLoad(t *testing.T) {
	store_ := storeIn(t, 10, 3)
	pkg, err := store_.Snapshot(KindUndo, memberSet("one"), "body.set")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Kind != KindUndo || pkg.Reason != "body.set" || pkg.Name == "" {
		t.Fatalf("package %+v", pkg)
	}
	packages, err := store_.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || packages[0].Name != pkg.Name || packages[0].Size == 0 {
		t.Fatalf("list %+v", packages)
	}
	loaded, members, err := store_.Load(pkg.Name)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Kind != KindUndo {
		t.Fatalf("loaded %+v", loaded)
	}
	if string(members["blocks/1.md"]) != "one" {
		t.Fatalf("members %v", members)
	}
}

func TestSnapshotRejectsUnknownKind(t *testing.T) {
	store_ := storeIn(t, 10, 3)
	_, err := store_.Snapshot(Kind("bogus"), memberSet("x"), "x")
	if err == nil {
		t.Fatal("expected rollback.invalid-kind")
	} else if rollbackErr, ok := err.(*Error); !ok || rollbackErr.Code != "rollback.invalid-kind" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestLimits(t *testing.T) {
	store_ := storeIn(t, 2, 1)
	for index := 0; index < 4; index++ {
		if _, err := store_.Snapshot(KindUndo, memberSet(fmt.Sprintf("body-%d", index)), "edit"); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 3; index++ {
		if _, err := store_.Snapshot(KindBook, memberSet("closed"), "close"); err != nil {
			t.Fatal(err)
		}
	}
	if undo := countOf(t, store_, KindUndo); undo != 2 {
		t.Fatalf("undo packages %d", undo)
	}
	if book := countOf(t, store_, KindBook); book != 1 {
		t.Fatalf("book packages %d", book)
	}
}

func TestZeroLimitDisables(t *testing.T) {
	store_ := storeIn(t, 0, 0)
	pkg, err := store_.Snapshot(KindUndo, memberSet("x"), "edit")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "" {
		t.Fatalf("package written with a zero limit: %+v", pkg)
	}
	if countOf(t, store_, KindUndo) != 0 {
		t.Fatal("a zero limit wrote a package")
	}
}

func TestClear(t *testing.T) {
	store_ := storeIn(t, 10, 3)
	if _, err := store_.Snapshot(KindUndo, memberSet("one"), "edit"); err != nil {
		t.Fatal(err)
	}
	if _, err := store_.Snapshot(KindBook, memberSet("two"), "close"); err != nil {
		t.Fatal(err)
	}
	if err := store_.Clear(KindUndo); err != nil {
		t.Fatal(err)
	}
	if countOf(t, store_, KindUndo) != 0 {
		t.Fatal("undo packages survived clear")
	}
	if countOf(t, store_, KindBook) != 1 {
		t.Fatal("clear removed a book package")
	}
}

func TestLoadRejectsInvalidNames(t *testing.T) {
	store_ := storeIn(t, 10, 3)
	for _, name := range []string{"../evil.zip", "nope.zip", "undo-20260101T000000.000000000Z-a.zip/.."} {
		if _, _, err := store_.Load(name); err == nil {
			t.Fatalf("%q must be rejected", name)
		}
	}
}

func TestNamesIncrease(t *testing.T) {
	store_ := storeIn(t, 10, 0)
	first, err := store_.Snapshot(KindUndo, memberSet("a"), "a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store_.Snapshot(KindUndo, memberSet("a"), "a")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name == second.Name || first.Name > second.Name {
		t.Fatalf("names %q and %q", first.Name, second.Name)
	}
}
