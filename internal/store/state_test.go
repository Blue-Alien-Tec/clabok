package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func TestHashMembers(t *testing.T) {
	first := map[string][]byte{"a": []byte("1"), "b": []byte("2")}
	second := map[string][]byte{"b": []byte("2"), "a": []byte("1")}
	if HashMembers(first) != HashMembers(second) {
		t.Fatal("map order changed the hash")
	}
	if HashMembers(first) == HashMembers(map[string][]byte{"a": []byte("1")}) {
		t.Fatal("a removed member did not change the hash")
	}
	if HashMembers(map[string][]byte{"a": []byte("2"), "b": []byte("1")}) == HashMembers(first) {
		t.Fatal("swapped data did not change the hash")
	}
	if HashMembers(map[string][]byte{"ab": []byte("c")}) ==
		HashMembers(map[string][]byte{"a": []byte("bc")}) {
		t.Fatal("the name/data boundary is ambiguous")
	}
	if HashMembers(map[string][]byte{}) != emptyHash {
		t.Fatalf("empty hash %q", HashMembers(map[string][]byte{}))
	}
	if HashBytes([]byte("x")) == HashBytes([]byte("y")) {
		t.Fatal("different bytes hashed equally")
	}
	if HashBytes(nil) != emptyHash {
		t.Fatal("empty bytes are not the empty hash")
	}
}

func testBookPaths(t *testing.T) BookPaths {
	t.Helper()
	layout, err := NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestStateRoundTrip(t *testing.T) {
	paths := testBookPaths(t)
	state := State{InputHash: "in", OutputHash: "out"}
	if err := paths.WriteState(state); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := paths.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if !exists || loaded != state {
		t.Fatalf("state %+v exists %t", loaded, exists)
	}

	state.OutputHash = "out2"
	if err := paths.WriteState(state); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = paths.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OutputHash != "out2" {
		t.Fatalf("state not replaced: %+v", loaded)
	}

	entries, err := os.ReadDir(paths.Dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", entry.Name())
		}
	}
}

func TestStateDelete(t *testing.T) {
	paths := testBookPaths(t)
	if err := paths.WriteState(State{InputHash: "in", OutputHash: "out"}); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteState(State{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.StateFile); !os.IsNotExist(err) {
		t.Fatalf("state file still present: %v", err)
	}
	if _, exists, err := paths.ReadState(); err != nil || exists {
		t.Fatalf("unexpected state %t %v", exists, err)
	}
	if err := paths.WriteState(State{}); err != nil {
		t.Fatalf("deleting an absent state must succeed: %v", err)
	}
}

func TestStateCorrupt(t *testing.T) {
	paths := testBookPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.StateFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.StateFile, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := paths.ReadState()
	expectCode(t, err, "store.state-corrupt")
}

func TestOpenBookCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "open.json")
	if count, err := OpenBookCount(path); err != nil || count != 0 {
		t.Fatalf("missing file: %d %v", count, err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"books":[{"id":"a"},{"id":"b"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if count, err := OpenBookCount(path); err != nil || count != 2 {
		t.Fatalf("count %d %v", count, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenBookCount(path); err == nil {
		t.Fatal("expected store.state-corrupt")
	} else {
		expectCode(t, err, "store.state-corrupt")
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		name   string
		state  State
		input  string
		output string
		want   Status
	}{
		{"no state", State{}, "in", "out", StatusNoState},
		{"clean", State{InputHash: "in", OutputHash: "out"}, "in", "out", StatusClean},
		{"input", State{InputHash: "old", OutputHash: "out"}, "in", "out", StatusDirtyInput},
		{"output", State{InputHash: "in", OutputHash: "old"}, "in", "out", StatusDirtyOutput},
		{"both", State{InputHash: "a", OutputHash: "b"}, "in", "out", StatusDirtyBoth},
		{"never rendered", State{InputHash: "in"}, "in", "out", StatusDirtyOutput},
	}
	for _, testCase := range cases {
		if got := testCase.state.Compare(testCase.input, testCase.output); got != testCase.want {
			t.Fatalf("%s: %s, want %s", testCase.name, got, testCase.want)
		}
	}
}
