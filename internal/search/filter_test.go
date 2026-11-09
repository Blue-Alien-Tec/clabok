package search

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestNodePathFilter(t *testing.T) {
	scene := newScene(t)
	all := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{NodePath: "Fixture/*"}})
	if len(all.Matches) != 3 {
		t.Fatalf("subtree matches %+v", all.Matches)
	}
	leaf := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{NodePath: "Fixture/One/Two"}})
	if len(leaf.Matches) != 1 || leaf.Matches[0].Block != "21" {
		t.Fatalf("leaf matches %+v", leaf.Matches)
	}
}

func TestBookPathFilter(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{
		All: true, Query: "Hello", Filter: Filter{BookPath: "Docs/fixture"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 3 || len(result.Books) != 1 || result.Books[0] != "fixture" {
		t.Fatalf("result %+v", result)
	}
	none, err := scene.Service.Find(Options{
		All: true, Query: "Hello", Filter: Filter{BookPath: "Docs/other"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Matches) != 0 {
		t.Fatalf("matches %+v", none.Matches)
	}
}

func TestRefFilterOnly(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{
		Book: "fixture", Result: "blocks", Filter: Filter{Ref: model.ID(99)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FilterOnly || len(result.ListBlocks) != 1 || result.ListBlocks[0].Block != "24" {
		t.Fatalf("blocks %+v", result.ListBlocks)
	}
	if result.Ref == nil || result.Ref.Exists {
		t.Fatalf("ref %+v", result.Ref)
	}
	payload := result.Payload(0, 0).(blocksPayload)
	if payload.Count != 1 || payload.Occurrences != 0 || payload.Blocks[0].FirstLine != 0 {
		t.Fatalf("payload %+v", payload)
	}
	owned, err := scene.Service.Find(Options{
		Book: "fixture", Result: "blocks", Filter: Filter{Ref: model.ID(10)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if owned.Ref == nil || !owned.Ref.Exists {
		t.Fatalf("owned ref %+v", owned.Ref)
	}
}

func TestMissingBodyFilterOnly(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{
		Book: "fixture", Result: "blocks", Filter: Filter{MissingBody: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FilterOnly || len(result.ListBlocks) != 1 || result.ListBlocks[0].Block != "23" {
		t.Fatalf("blocks %+v", result.ListBlocks)
	}
	count, err := scene.Service.Find(Options{
		Book: "fixture", Result: "count", Filter: Filter{MissingBody: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := count.Payload(0, 0).(countPayload)
	if payload.Count != 1 || payload.Blocks != 1 || payload.Occurrences != 0 {
		t.Fatalf("count %+v", payload)
	}
}

func TestFilterOnlyRequiresFilterOrQuery(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.Service.Find(Options{Book: "fixture", Result: "blocks"}); errorCode(err) != "search.empty-query" {
		t.Fatalf("empty filter-only: %v", err)
	}
	if _, err := scene.Service.Find(Options{Book: "fixture", Result: "matches", Filter: Filter{Ref: 10}}); errorCode(err) != "search.empty-query" {
		t.Fatalf("matches without query: %v", err)
	}
}

func TestFindCommandPathAndRefFilters(t *testing.T) {
	scene := newScene(t)
	code, stdout, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--node-path", "Fixture/One/Two", "--query", "Hello")...)
	if code != cli.ExitOK {
		t.Fatalf("node-path exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"count": 1`) {
		t.Fatalf("node-path payload %s", stdout)
	}
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--ref", "99", "--result", "blocks")...)
	if code != cli.ExitOK {
		t.Fatalf("ref exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"block": "24"`) || !strings.Contains(stdout, `"exists": false`) {
		t.Fatalf("ref payload %s", stdout)
	}
	code, _, _ = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--book-path", "Docs/*", "--query", "Hello")...)
	if code != cli.ExitReject {
		t.Fatalf("book-path without --all exit %d", code)
	}
}

func TestFindCommandMissingBodyAndRequireMatch(t *testing.T) {
	scene := newScene(t)
	code, stdout, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--missing-body", "--result", "blocks")...)
	if code != cli.ExitOK {
		t.Fatalf("missing-body exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"block": "23"`) {
		t.Fatalf("missing-body payload %s", stdout)
	}
	code, _, _ = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--missing-body", "--query", "x", "--result", "blocks")...)
	if code != cli.ExitUsage {
		t.Fatalf("missing-body with query exit %d", code)
	}
	code, _, _ = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--missing-body")...)
	if code != cli.ExitUsage {
		t.Fatalf("missing-body with matches exit %d", code)
	}
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "zzz", "--require-match")...)
	if code != cli.ExitFailure {
		t.Fatalf("require-match exit %d", code)
	}
	if !strings.Contains(stdout, `"count": 0`) || !strings.Contains(stderr, "search.no-match") {
		t.Fatalf("require-match stdout %s stderr %s", stdout, stderr)
	}
}

func TestFindCommandSnapshot(t *testing.T) {
	scene := newScene(t)
	code, _, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Hello", "--snapshot")...)
	if code != cli.ExitOK {
		t.Fatalf("snapshot exit %d: %s", code, stderr)
	}
}
