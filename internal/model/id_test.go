package model

import (
	"encoding/json"
	"testing"
)

func TestIDStringRoundTrip(t *testing.T) {
	id, err := ParseID("18446744073709551615")
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "18446744073709551615" {
		t.Fatalf("unexpected string %q", id.String())
	}
	if ZeroID.String() != "0" || !ZeroID.IsZero() {
		t.Fatal("zero sentinel broken")
	}
	if _, err := ParseID("abc"); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestIDJSONIsString(t *testing.T) {
	data, err := json.Marshal(ID(7))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"7"` {
		t.Fatalf("marshal %s", data)
	}
	var decoded ID
	if err := json.Unmarshal([]byte(`"42"`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != 42 {
		t.Fatalf("decoded %d", decoded)
	}
	if err := json.Unmarshal([]byte(`42`), &decoded); err == nil {
		t.Fatal("a JSON number must be rejected")
	}
}

func TestSequenceAllocator(t *testing.T) {
	allocator := NewSequenceAllocator(5)
	seen := map[ID]bool{}
	for index := 0; index < 100; index++ {
		id, err := allocator.Next()
		if err != nil {
			t.Fatal(err)
		}
		if id.IsZero() || seen[id] {
			t.Fatalf("bad id %d", id)
		}
		seen[id] = true
	}
	if !seen[5] || !seen[104] {
		t.Fatalf("unexpected range %v", seen)
	}
}

func TestRandomAllocator(t *testing.T) {
	allocator := NewRandomAllocator()
	seen := map[ID]bool{}
	for index := 0; index < 1000; index++ {
		id, err := allocator.Next()
		if err != nil {
			t.Fatal(err)
		}
		if id.IsZero() || seen[id] {
			t.Fatalf("bad id %d", id)
		}
		seen[id] = true
	}
}
