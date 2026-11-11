package model

import (
	"bytes"
	"testing"
)

func libraryFixture() *Manifest {
	manifest := validManifest()
	off := false
	manifest.Settings.Library = &LibraryLayout{
		Slots:      []string{SlotContent, SlotTOC},
		Navigation: &off,
		Toc:        &SlotMeta{Heading: 3, Title: "Contents"},
		Index:      &LibraryIndexConfig{Columns: []string{"title", "path"}, Shape: IndexShapeTable},
	}
	return manifest
}

func TestLibraryLayoutRoundTrip(t *testing.T) {
	manifest := libraryFixture()
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"library"`)) {
		t.Fatalf("library layout missing: %s", data)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	layout := decoded.Settings.Library
	if layout == nil || len(layout.Slots) != 2 || layout.Navigation == nil ||
		*layout.Navigation || layout.Toc == nil || layout.Toc.Heading != 3 ||
		layout.Index == nil || layout.Index.Shape != IndexShapeTable {
		t.Fatalf("decoded layout %+v", layout)
	}
}

func TestLibraryLayoutStaysAbsentWhenUnset(t *testing.T) {
	manifest := validManifest()
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"library"`)) {
		t.Fatalf("an unset layout is written: %s", data)
	}
}

func TestLibraryLayoutValidation(t *testing.T) {
	manifest := validManifest()
	manifest.Settings.Library = &LibraryLayout{Slots: DefaultSlotOrder()}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("the default order was rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*LibraryLayout)
	}{
		{"structural", func(layout *LibraryLayout) { layout.Slots = []string{"nav"} }},
		{"unknown", func(layout *LibraryLayout) { layout.Slots = []string{"sidebar"} }},
		{"duplicate", func(layout *LibraryLayout) { layout.Slots = []string{"content", "content"} }},
		{"columns-without-title", func(layout *LibraryLayout) {
			layout.Index = &LibraryIndexConfig{Columns: []string{"path"}}
		}},
		{"unknown-column", func(layout *LibraryLayout) {
			layout.Index = &LibraryIndexConfig{Columns: []string{"title", "date"}}
		}},
		{"duplicate-column", func(layout *LibraryLayout) {
			layout.Index = &LibraryIndexConfig{Columns: []string{"title", "title"}}
		}},
		{"shape", func(layout *LibraryLayout) {
			layout.Index = &LibraryIndexConfig{Shape: "grid"}
		}},
		{"heading-low", func(layout *LibraryLayout) {
			layout.Toc = &SlotMeta{Heading: 1}
		}},
		{"heading-high", func(layout *LibraryLayout) {
			layout.Toc = &SlotMeta{Heading: 7}
		}},
	}
	for _, testCase := range cases {
		manifest := validManifest()
		manifest.Settings.Library = &LibraryLayout{Slots: DefaultSlotOrder()}
		testCase.mutate(manifest.Settings.Library)
		if err := manifest.Validate(); err == nil {
			t.Fatalf("%s: expected a validation error", testCase.name)
		}
	}
}
