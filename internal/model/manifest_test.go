package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestNewManifest(t *testing.T) {
	manifest, err := NewManifest("fixture", "1. Fixture", NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Root == nil || manifest.Root.Kind != KindTitle || manifest.Root.Title != "Fixture" {
		t.Fatalf("unexpected root %+v", manifest.Root)
	}
	if manifest.Root.ID.IsZero() {
		t.Fatal("root id is zero")
	}
	if manifest.Settings.Output != OutputMonolith || manifest.Settings.IndexDepth != DefaultIndexDepth {
		t.Fatalf("unexpected settings %+v", manifest.Settings)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("new manifest must be valid: %v", err.Issues)
	}
}

func TestNewManifestRejectsBadInput(t *testing.T) {
	if _, err := NewManifest("bad id", "T", NewSequenceAllocator(1)); err == nil {
		t.Fatal("expected an invalid book id error")
	}
	if _, err := NewManifest("fixture", "T", nil); err == nil {
		t.Fatal("expected a missing allocator error")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	manifest := validManifest()
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !manifestsEqual(manifest, decoded) {
		t.Fatalf("round trip changed the manifest:\n%s", data)
	}
	second, err := decoded.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, second) {
		t.Fatal("encode is not deterministic")
	}
}

func TestDecodeNormalizesSettings(t *testing.T) {
	document := `{"version":2,"id":"x","root":{"id":"1","kind":"title","title":"X"},"settings":{}}`
	manifest, err := Decode([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Settings.Output != OutputMonolith || manifest.Settings.IndexDepth != DefaultIndexDepth {
		t.Fatalf("settings not normalized: %+v", manifest.Settings)
	}
}

func TestDecodeRejectsInvalidDocuments(t *testing.T) {
	cases := []struct {
		document string
		code     string
	}{
		{`{"version":2,"id":"x","root":{"id":"1","kind":"title","title":"X"},"bogus":1}`, "model.unknown-key"},
		{`{"version":2,"version":2}`, "model.duplicate-key"},
		{`{"version":3,"id":"x"}`, "model.manifest-version"},
		{`{"id":"x"}`, "model.manifest-version"},
		{`{`, "model.invalid-json"},
	}
	for _, testCase := range cases {
		_, err := Decode([]byte(testCase.document))
		if err == nil {
			t.Fatalf("%s: expected an error", testCase.document)
		}
		var modelErr *Error
		if !errors.As(err, &modelErr) || modelErr.Code != testCase.code {
			t.Fatalf("%s: got %v, want %s", testCase.document, err, testCase.code)
		}
	}
}

func TestIDsAreJSONStrings(t *testing.T) {
	manifest := validManifest()
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	root := raw["root"].(map[string]any)
	if root["id"] != "1" {
		t.Fatalf("id is not a string: %v", root["id"])
	}
}

func TestFindNodeAndClone(t *testing.T) {
	manifest := validManifest()
	node, ok := manifest.FindNode(3)
	if !ok || node.Title != "Sub" {
		t.Fatalf("find failed: %+v", node)
	}
	if _, ok := manifest.FindNode(404); ok {
		t.Fatal("found a missing node")
	}
	clone := manifest.Clone()
	clone.Root.Children[0].Title = "Changed"
	if manifest.Root.Children[0].Title != "Chapter" {
		t.Fatal("clone shares state with the original")
	}
}

func manifestsEqual(left, right *Manifest) bool {
	leftData, leftErr := left.Encode()
	rightData, rightErr := right.Encode()
	return leftErr == nil && rightErr == nil && bytes.Equal(leftData, rightData)
}
