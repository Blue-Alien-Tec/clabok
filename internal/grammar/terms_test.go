package grammar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func termsFixture(t *testing.T) (pending, terms string) {
	t.Helper()
	directory := t.TempDir()
	return filepath.Join(directory, "pending-terms.json"), filepath.Join(directory, "terms.txt")
}

func TestTermsLifecycle(t *testing.T) {
	pending, terms := termsFixture(t)
	now := time.Now()
	entry, err := AddPending(pending, "add", "Clabok", "the product name", now)
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID == "" {
		t.Fatalf("entry %+v", entry)
	}
	if _, err := os.Stat(terms); !os.IsNotExist(err) {
		t.Fatalf("a request wrote the term list: %v", err)
	}

	committed, count, err := CommitPending(pending, terms, entry.ID)
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
	second, err := AddPending(pending, "add", "clabok", "duplicate", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, count, err := CommitPending(pending, terms, second.ID); err != nil || count != 1 {
		t.Fatalf("dedupe commit: %d %v", count, err)
	}

	remove, err := AddPending(pending, "remove", "CLABOK", "drop it", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, count, err := CommitPending(pending, terms, remove.ID); err != nil || count != 0 {
		t.Fatalf("remove commit: %d %v", count, err)
	}
}

func TestDiscardPending(t *testing.T) {
	pending, _ := termsFixture(t)
	entry, err := AddPending(pending, "add", "X", "test", time.Now())
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
	_, terms := termsFixture(t)
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
