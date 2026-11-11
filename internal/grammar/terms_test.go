package grammar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func termsFixture(t *testing.T) (pending, terms, termsDir string) {
	t.Helper()
	directory := t.TempDir()
	return filepath.Join(directory, "pending-terms.json"), filepath.Join(directory, "terms.txt"),
		filepath.Join(directory, "terms")
}

func TestTermsLifecycle(t *testing.T) {
	pending, terms, termsDir := termsFixture(t)
	now := time.Now()
	entry, err := AddPending(pending, "add", "Clabok", "the product name", ScopeGlobal, now)
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID == "" {
		t.Fatalf("entry %+v", entry)
	}
	if _, err := os.Stat(terms); !os.IsNotExist(err) {
		t.Fatalf("a request wrote the term list: %v", err)
	}

	committed, count, err := CommitPending(pending, terms, termsDir, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Term != "Clabok" || count != 1 {
		t.Fatalf("commit %+v (%d)", committed, count)
	}
	data, err := os.ReadFile(terms)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Clabok\n" {
		t.Fatalf("terms %q", data)
	}
	if entries, err := ReadPending(pending); err != nil || len(entries) != 0 {
		t.Fatalf("pending after commit: %v %v", entries, err)
	}

	// A second add of the same term (different case) deduplicates.
	second, err := AddPending(pending, "add", "clabok", "duplicate", ScopeGlobal, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, count, err := CommitPending(pending, terms, termsDir, second.ID); err != nil || count != 1 {
		t.Fatalf("dedupe commit: %d %v", count, err)
	}

	remove, err := AddPending(pending, "remove", "CLABOK", "drop it", ScopeGlobal, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, count, err := CommitPending(pending, terms, termsDir, remove.ID); err != nil || count != 0 {
		t.Fatalf("remove commit: %d %v", count, err)
	}
}

func TestDiscardPending(t *testing.T) {
	pending, _, _ := termsFixture(t)
	entry, err := AddPending(pending, "add", "X", "test", ScopeGlobal, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	discarded, err := DiscardPending(pending, entry.ID)
	if err != nil || discarded.ID != entry.ID {
		t.Fatalf("discard: %+v %v", discarded, err)
	}
	if entries, err := ReadPending(pending); err != nil || len(entries) != 0 {
		t.Fatalf("pending after discard: %v %v", entries, err)
	}
	if _, err := DiscardPending(pending, "missing"); err == nil {
		t.Fatal("an unknown request must be rejected")
	}
}

func TestWriteTermsDedupes(t *testing.T) {
	_, terms, _ := termsFixture(t)
	if err := WriteTerms(terms, []string{"Alpha", " alpha ", "", "Beta"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(terms)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "Alpha") != 1 || !strings.Contains(string(data), "Beta") {
		t.Fatalf("terms %q", data)
	}
}

func TestTermSetWordBoundaryAndSubstring(t *testing.T) {
	set := TermSet{Layers: []TermLayer{
		{Scope: ScopeGlobal, Terms: []string{"ark", "~land"}},
	}}
	entries := set.Suppress("maryLAND", nil)
	if len(entries) != 1 || entries[0].Term != "land" || entries[0].Match != "substring" {
		t.Fatalf("substring marker: %+v", entries)
	}
	entries = set.Suppress("the ark", nil)
	if len(entries) != 1 || entries[0].Term != "ark" || entries[0].Match != "word" {
		t.Fatalf("word boundary: %+v", entries)
	}
	if entries := set.Suppress("ARK", nil); len(entries) != 1 {
		t.Fatalf("case-insensitive: %+v", entries)
	}
	entries = set.Suppress("start", []string{"the ark"})
	if len(entries) != 1 || entries[0].Field != "replacement" {
		t.Fatalf("replacement attribution: %+v", entries)
	}
	if entries := set.Suppress("marker", nil); len(entries) != 0 {
		t.Fatalf("no match expected: %+v", entries)
	}
}

func TestTermSetAttributionAcrossLayers(t *testing.T) {
	set := TermSet{Layers: []TermLayer{
		{Scope: ScopeGlobal, Terms: []string{"Clabok"}},
		{Scope: "de-de", Terms: []string{"Clabok", "Simia"}},
	}}
	entries := set.Suppress("Clabok Simia", nil)
	if len(entries) != 2 {
		t.Fatalf("entries %+v", entries)
	}
	if entries[0].Layer != ScopeGlobal || len(entries[0].AlsoIn) != 1 ||
		entries[0].AlsoIn[0] != "de-de" {
		t.Fatalf("global attribution: %+v", entries[0])
	}
	if entries[1].Layer != "de-de" || entries[1].Term != "Simia" {
		t.Fatalf("language attribution: %+v", entries[1])
	}
}

func TestCommitWritesTheLanguageLayer(t *testing.T) {
	pending, terms, termsDir := termsFixture(t)
	entry, err := AddPending(pending, "add", "Simia", "a ship name", "de-DE", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, count, err := CommitPending(pending, terms, termsDir, entry.ID); err != nil || count != 1 {
		t.Fatalf("commit %d %v", count, err)
	}
	languagePath := filepath.Join(termsDir, "de-de.txt")
	data, err := os.ReadFile(languagePath)
	if err != nil || string(data) != "Simia\n" {
		t.Fatalf("language layer %q %v", data, err)
	}
	if _, err := os.Stat(terms); !os.IsNotExist(err) {
		t.Fatalf("the global layer must stay untouched: %v", err)
	}
	layer := LoadTermSet(terms, termsDir, "de-DE")
	if len(layer.Layers) != 2 || len(layer.Layers[1].Terms) != 1 {
		t.Fatalf("loaded layers %+v", layer.Layers)
	}
}

func TestLegacyPendingEntryIsGlobal(t *testing.T) {
	pending, terms, termsDir := termsFixture(t)
	legacy := `[{"id":"abc","action":"add","term":"Legacy","reason":"old",` +
		`"created":"2026-01-01T00:00:00Z"}]`
	if err := os.WriteFile(pending, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadPending(pending)
	if err != nil || len(entries) != 1 || entries[0].ScopeOf() != ScopeGlobal {
		t.Fatalf("legacy pending %+v %v", entries, err)
	}
	if _, _, err := CommitPending(pending, terms, termsDir, "abc"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(terms); err != nil || string(data) != "Legacy\n" {
		t.Fatalf("global layer %q %v", data, err)
	}
}

func TestParseTermForm(t *testing.T) {
	if form := ParseTermForm("~ark"); form.Term != "ark" || !form.Substring {
		t.Fatalf("substring form %+v", form)
	}
	if form := ParseTermForm("ark"); form.Term != "ark" || form.Substring {
		t.Fatalf("word form %+v", form)
	}
}
