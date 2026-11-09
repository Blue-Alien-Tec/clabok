package search

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
)

func buildIndex(t *testing.T, scene *scene) {
	t.Helper()
	if _, err := scene.Service.Index.Build("fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestIndexedSearchMatchesScan(t *testing.T) {
	scene := newScene(t)
	buildIndex(t, scene)
	cases := []Options{
		{Query: "Hello"},
		{Query: "Hello", IgnoreCase: true},
		{Fields: []string{"all"}, Queries: []Query{{Text: "Hello"}, {Text: "world"}}, Join: "and"},
		{Filter: Filter{Kinds: []string{"chapter"}}, Query: "Hello"},
		{Filter: Filter{Statuses: []string{"wip"}}, Query: "Hello"},
		{Filter: Filter{NodePath: "Fixture/One/Two"}, Query: "Hello"},
		{Query: "Hel+o", Mode: ModeRegex},
		{Query: "One*", Mode: ModeGlob},
		{Queries: []Query{{Text: "Helo", Mode: ModeFuzzy}}},
		{Queries: []Query{{Text: "Helo", Mode: ModeLiteral, Fuzzy: 1}}},
		{Query: "Hello", Result: "blocks"},
		{Query: "Hello", GroupBy: "type"},
	}
	for _, options := range cases {
		scanned := options
		scanned.Book = "fixture"
		scanned.IndexMode = "off"
		indexed := options
		indexed.Book = "fixture"
		indexed.IndexMode = "auto"
		want, err := scene.Service.Find(scanned)
		if err != nil {
			t.Fatalf("%+v scan: %v", options, err)
		}
		got, err := scene.Service.Find(indexed)
		if err != nil {
			t.Fatalf("%+v index: %v", options, err)
		}
		if !got.Index.Used || !got.Index.Fresh {
			t.Fatalf("%+v: index not used (%+v)", options, got.Index)
		}
		if !reflect.DeepEqual(want.Matches, got.Matches) {
			t.Fatalf("%+v: matches differ\nscan  %+v\nindex %+v", options, want.Matches, got.Matches)
		}
		if !reflect.DeepEqual(want.Warnings, got.Warnings) {
			t.Fatalf("%+v: warnings differ %+v vs %+v", options, want.Warnings, got.Warnings)
		}
	}
}

func TestIndexedSearchFuzzy(t *testing.T) {
	scene := newScene(t)
	buildIndex(t, scene)
	scanned, err := scene.Service.Find(Options{
		Book: "fixture", Queries: []Query{{Text: "Helo", Mode: ModeFuzzy}}, IndexMode: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	indexed, err := scene.Service.Find(Options{
		Book: "fixture", Queries: []Query{{Text: "Helo", Mode: ModeFuzzy}}, IndexMode: "force",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !indexed.Index.Used {
		t.Fatalf("fuzzy did not use the index: %+v", indexed.Index)
	}
	if !reflect.DeepEqual(scanned.Matches, indexed.Matches) {
		t.Fatalf("fuzzy indexed and scanned results differ")
	}
	if len(indexed.Matches) != 3 {
		t.Fatalf("fuzzy matches %+v", indexed.Matches)
	}
}

func TestIndexedSearchFallsBackToScan(t *testing.T) {
	scene := newScene(t)
	buildIndex(t, scene)
	result, err := scene.Service.Find(Options{
		Book: "fixture", Query: ",,", IndexMode: "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Index.Used {
		t.Fatal("a punctuation-only query used the index")
	}
}

func TestIndexForceAndVerify(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", IndexMode: "force"}); errorCode(err) != "index.missing" {
		t.Fatalf("force without index: %v", err)
	}
	buildIndex(t, scene)
	if _, err := scene.Service.Find(Options{
		Book: "fixture", IndexMode: "force", Query: ",,",
	}); errorCode(err) != "index.unsupported" {
		t.Fatalf("force without a usable literal: %v", err)
	}
	path := filepath.Join(scene.Layout.Root, "index", "fixture.lines")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "Hello world", "Hallo world", 1)
	if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	loose, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", IndexMode: "force"})
	if err != nil {
		t.Fatal(err)
	}
	if len(loose.Matches) != 2 {
		t.Fatalf("tampered index matches %+v", loose.Matches)
	}
	verified, err := scene.Service.Find(Options{
		Book: "fixture", Query: "Hello", IndexMode: "force", Verify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(verified.Matches) != 3 {
		t.Fatalf("verified matches %+v", verified.Matches)
	}
	mismatch := false
	for _, warning := range verified.Warnings {
		if warning.Code == "index.verify-mismatch" {
			mismatch = true
		}
	}
	if !mismatch {
		t.Fatalf("warnings %+v", verified.Warnings)
	}
}

func TestIndexStaleUnderForce(t *testing.T) {
	scene := newScene(t)
	buildIndex(t, scene)
	if err := os.WriteFile(scene.Paths["fixture"].Container, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", IndexMode: "force"}); errorCode(err) != "index.stale" {
		t.Fatalf("stale force: %v", err)
	}
	if _, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", IndexMode: "auto"}); errorCode(err) != "search.read-failed" {
		t.Fatalf("a changed container did not fail the scan: %v", err)
	}
}

func TestAllNodeResolutionUsesFreshIndex(t *testing.T) {
	scene := newScene(t)
	buildIndex(t, scene)
	result, err := scene.Service.Find(Options{
		All: true, Query: "Hello", IndexMode: "force", Filter: Filter{Node: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Books) != 1 || result.Books[0] != "fixture" {
		t.Fatalf("books %+v", result.Books)
	}
}

func TestIndexCommandOutput(t *testing.T) {
	scene := newScene(t)
	code, stdout, stderr := run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Hello", "--index", "off")...)
	if code != cli.ExitOK {
		t.Fatalf("off exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"used": false`) {
		t.Fatalf("off payload %s", stdout)
	}
	buildIndex(t, scene)
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Hello", "--index", "force")...)
	if code != cli.ExitOK {
		t.Fatalf("force exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"used": true`) || !strings.Contains(stdout, `"fresh": true`) {
		t.Fatalf("force payload %s", stdout)
	}
}
