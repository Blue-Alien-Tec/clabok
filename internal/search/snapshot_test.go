package search

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func TestSnapshotExcludesWriter(t *testing.T) {
	scene := newScene(t)
	release, err := scene.Life.LockBookExclusive("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", Snapshot: true}); errorCode(err) != "search.lock-failed" {
		release()
		t.Fatalf("snapshot under an exclusive lock: %v", err)
	}
	if _, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello"}); err != nil {
		release()
		t.Fatalf("a lock-free search failed: %v", err)
	}
	release()
	if _, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", Snapshot: true}); err != nil {
		t.Fatalf("snapshot after release: %v", err)
	}
	result, err := scene.Service.Find(Options{All: true, Query: "Hello", Snapshot: true})
	if err != nil {
		t.Fatalf("snapshot over all books: %v", err)
	}
	if len(result.Matches) != 4 {
		t.Fatalf("all-books snapshot matches %+v", result.Matches)
	}
}

func TestCombinedFilters(t *testing.T) {
	scene := newScene(t)
	unlocked := false
	result := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{
		Kinds: []string{"chapter"}, Types: []string{"table"}, Statuses: []string{"wip"},
		Part: "Guide", Locked: &unlocked,
	}})
	if len(result.Matches) != 2 || result.Matches[0].Block != "11" {
		t.Fatalf("combined filters %+v", result.Matches)
	}
	excluded := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{
		Kinds: []string{"chapter"}, Types: []string{"table"}, Statuses: []string{"final"},
	}})
	if len(excluded.Matches) != 0 {
		t.Fatalf("a non-matching combination matched %+v", excluded.Matches)
	}
}

func TestOpenClosedSearchParity(t *testing.T) {
	scene := newScene(t)
	closed, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", IndexMode: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scene.Life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	open, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", IndexMode: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if len(open.Matches) != len(closed.Matches) {
		t.Fatalf("open %d vs closed %d matches", len(open.Matches), len(closed.Matches))
	}
	for index := range closed.Matches {
		if closed.Matches[index].Block != open.Matches[index].Block ||
			closed.Matches[index].Line != open.Matches[index].Line ||
			closed.Matches[index].Text != open.Matches[index].Text {
			t.Fatalf("match %d differs: %+v vs %+v", index, closed.Matches[index], open.Matches[index])
		}
	}
	if len(open.Warnings) != len(closed.Warnings) {
		t.Fatalf("warnings differ: %+v vs %+v", closed.Warnings, open.Warnings)
	}
}

func TestRequireMatchOutputForms(t *testing.T) {
	scene := newScene(t)
	code, stdout, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "zzz", "--require-match", "--text")...)
	if code != cli.ExitFailure {
		t.Fatalf("text require-match exit %d", code)
	}
	if !strings.Contains(stdout, "0 match(es)") || !strings.Contains(stderr, "no match") {
		t.Fatalf("text require-match stdout %q stderr %q", stdout, stderr)
	}
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "zzz", "--require-match", "--format", "ndjson")...)
	if code != cli.ExitFailure {
		t.Fatalf("ndjson require-match exit %d", code)
	}
	if !strings.Contains(stdout, `"type":"summary"`) || !strings.Contains(stderr, "search.no-match") {
		t.Fatalf("ndjson require-match stdout %q stderr %q", stdout, stderr)
	}
}

func TestCLIRejectsInvalidRegex(t *testing.T) {
	scene := newScene(t)
	code, _, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "(", "--mode", "regex")...)
	if code != cli.ExitUsage || !strings.Contains(stderr, "search.invalid-regex") {
		t.Fatalf("invalid regex exit %d stderr %s", code, stderr)
	}
}
