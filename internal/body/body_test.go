package body

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/rollback"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

type scene struct {
	service *Service
	life    *lifecycle.Lifecycle
	layout  store.Layout
	paths   store.BookPaths
}

func newScene(t *testing.T) scene {
	t.Helper()
	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := lifecycle.New(layout, locks, lifecycle.Options{UndoLimit: 20, BookPointLimit: 3})
	manifest, err := model.NewManifest("fixture", "Fixture", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children = []*model.Node{
		{ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{11, 12}},
		{ID: 20, Kind: model.KindChapter, Title: "Two", Blocks: []model.ID{21}},
	}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	_ = container.Write("manifest.json", data)
	_ = container.Write("blocks/11.md", []byte("Body 11\n"))
	_ = container.Write("blocks/12.md", []byte("Body 12\n"))
	_ = container.Write("blocks/21.md", []byte("Body 21\n"))
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	return scene{service: New(life), life: life, layout: layout, paths: paths}
}

func (s scene) secondService(t *testing.T) *Service {
	t.Helper()
	locks, err := coordinator.NewFile(s.layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := lifecycle.New(s.layout, locks, lifecycle.Options{UndoLimit: 20, BookPointLimit: 3})
	return New(life)
}

func (s scene) setMeta(t *testing.T, id model.ID, meta model.BlockMeta) {
	t.Helper()
	path := filepath.Join(s.paths.Dir, lifecycle.ManifestMember)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest.SetMeta(id, meta)
	encoded, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(path, encoded); err != nil {
		t.Fatal(err)
	}
}

func (s scene) bodyOf(t *testing.T, block model.ID) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.paths.BlocksDir, block.String()+".md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCheckProse(t *testing.T) {
	cases := []struct {
		text  string
		lines int
	}{
		{"# Title", 1},
		{"## Sub", 1},
		{"### Sub", 1},
		{"#### Deep", 0},
		{"##### Deeper", 0},
		{"text\n# Later", 1},
		{"   ## three spaces", 1},
		{"    # four spaces", 0},
		{"#nospace", 0},
		{"```\n# code\n```", 0},
		{"~~~\n## code\n~~~", 0},
	}
	for _, testCase := range cases {
		if issues := CheckProse(testCase.text); len(issues) != testCase.lines {
			t.Fatalf("%q: %d issues, want %d (%v)", testCase.text, len(issues), testCase.lines, issues)
		}
	}
}

func TestSetAndGet(t *testing.T) {
	s := newScene(t)
	result, err := s.service.Set("fixture", 11, 0, "a\nb\nc\n", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Block != 11 || result.Lines != 3 || result.Bytes != len("a\nb\nc\n") {
		t.Fatalf("result %+v", result)
	}
	if s.bodyOf(t, 11) != "a\nb\nc\n" {
		t.Fatal("body not written")
	}

	slice, err := s.service.Get("fixture", 11, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if slice.Text != "a\nb\nc\n" || slice.TotalLines != 3 || slice.Truncated {
		t.Fatalf("slice %+v", slice)
	}

	slice, err = s.service.Get("fixture", 11, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if slice.Text != "a\nb" || !slice.Truncated || slice.From != 1 || slice.To != 2 {
		t.Fatalf("slice %+v", slice)
	}

	slice, err = s.service.Get("fixture", 11, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if slice.Text != "b\nc\n" {
		t.Fatalf("slice %q", slice.Text)
	}
}

func TestAppendAndPrepend(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.Set("fixture", 11, 0, "A\n", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Append("fixture", 11, 0, "B\n", false); err != nil {
		t.Fatal(err)
	}
	if s.bodyOf(t, 11) != "A\nB\n" {
		t.Fatalf("append %q", s.bodyOf(t, 11))
	}
	if _, err := s.service.Prepend("fixture", 11, 0, "C\n", false); err != nil {
		t.Fatal(err)
	}
	if s.bodyOf(t, 11) != "C\nA\nB\n" {
		t.Fatalf("prepend %q", s.bodyOf(t, 11))
	}

	if _, err := s.service.Set("fixture", 11, 0, "A", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Append("fixture", 11, 0, "B", false); err != nil {
		t.Fatal(err)
	}
	if s.bodyOf(t, 11) != "A\nB" {
		t.Fatalf("separator %q", s.bodyOf(t, 11))
	}
}

func TestHeadingGate(t *testing.T) {
	s := newScene(t)
	before := s.bodyOf(t, 11)
	_, err := s.service.Set("fixture", 11, 0, "## nope\n", false)
	if err == nil {
		t.Fatal("expected body.forbidden-heading")
	}
	var bodyErr *Error
	if !errors.As(err, &bodyErr) || bodyErr.Code != "body.forbidden-heading" {
		t.Fatalf("unexpected error %v", err)
	}
	if s.bodyOf(t, 11) != before {
		t.Fatal("a rejected body was written")
	}
	if _, err := s.service.Set("fixture", 11, 0, "```\n# code\n```\n", false); err != nil {
		t.Fatalf("a fenced heading is text: %v", err)
	}
}

func TestEncoding(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.Set("fixture", 11, 0, "\uFEFFx", false); err == nil {
		t.Fatal("expected body.invalid-encoding")
	}
	if _, err := s.service.Set("fixture", 11, 0, "a\r\nb\r\n", false); err != nil {
		t.Fatal(err)
	}
	if s.bodyOf(t, 11) != "a\nb\n" {
		t.Fatalf("line endings %q", s.bodyOf(t, 11))
	}
}

func TestBlockZeroCreatesBlock(t *testing.T) {
	s := newScene(t)
	result, err := s.service.Set("fixture", 0, 10, "New body\n", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Block.IsZero() {
		t.Fatalf("result %+v", result)
	}
	if s.bodyOf(t, result.Block) != "New body\n" {
		t.Fatal("new body not written")
	}
	slice, err := s.service.Get("fixture", result.Block, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if slice.Text != "New body\n" {
		t.Fatalf("slice %q", slice.Text)
	}
}

func TestBlockZeroRequiresParent(t *testing.T) {
	s := newScene(t)
	_, err := s.service.Set("fixture", 0, 0, "x", false)
	if err == nil {
		t.Fatal("expected body.parent-required")
	}
	var bodyErr *Error
	if !errors.As(err, &bodyErr) || bodyErr.Code != "body.parent-required" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestUnknownBlock(t *testing.T) {
	s := newScene(t)
	_, err := s.service.Set("fixture", 99, 0, "x", false)
	if err == nil {
		t.Fatal("expected body.unknown-block")
	}
}

func TestReplace(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.Set("fixture", 11, 0, "alpha beta alpha\n", false); err != nil {
		t.Fatal(err)
	}
	result, err := s.service.Replace("fixture", 11, []Pair{{Find: "alpha", Replace: "ALPHA"}}, ReplaceOptions{Expect: 2}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replacements != 2 || s.bodyOf(t, 11) != "ALPHA beta ALPHA\n" {
		t.Fatalf("replace %+v %q", result, s.bodyOf(t, 11))
	}

	before := s.bodyOf(t, 11)
	_, err = s.service.Replace("fixture", 11, []Pair{{Find: "ALPHA", Replace: "x"}}, ReplaceOptions{Expect: 1}, false)
	if err == nil {
		t.Fatal("expected body.expectation-failed")
	}
	if s.bodyOf(t, 11) != before {
		t.Fatal("a failed expectation wrote the body")
	}
}

func TestReplaceWordAndRegex(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.Set("fixture", 11, 0, "cat concatenate cat\n", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Replace("fixture", 11, []Pair{{Find: "cat", Replace: "dog"}}, ReplaceOptions{Expect: 2, Word: true}, false); err != nil {
		t.Fatal(err)
	}
	if s.bodyOf(t, 11) != "dog concatenate dog\n" {
		t.Fatalf("word replace %q", s.bodyOf(t, 11))
	}

	if _, err := s.service.Set("fixture", 11, 0, "a1 b22\n", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Replace("fixture", 11, []Pair{{Find: "[0-9]+", Replace: "#", Regex: true}}, ReplaceOptions{Expect: 2}, false); err != nil {
		t.Fatal(err)
	}
	if s.bodyOf(t, 11) != "a# b#\n" {
		t.Fatalf("regex replace %q", s.bodyOf(t, 11))
	}
}

func TestList(t *testing.T) {
	s := newScene(t)
	rows, err := s.service.List("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].ID != 11 || rows[0].Title != "One" || rows[0].Depth != 2 || rows[0].Lines != 1 {
		t.Fatalf("row %+v", rows[0])
	}
	if rows[2].ID != 21 || rows[2].Title != "Two" || rows[2].Depth != 2 {
		t.Fatalf("row %+v", rows[2])
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	s := newScene(t)
	before := s.bodyOf(t, 11)
	result, err := s.service.Set("fixture", 11, 0, "Dry\n", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Lines != 1 {
		t.Fatalf("result %+v", result)
	}
	if s.bodyOf(t, 11) != before {
		t.Fatal("a dry run wrote the body")
	}
}

func TestParallelBlockWrites(t *testing.T) {
	s := newScene(t)
	second := s.secondService(t)
	targets := []struct {
		block model.ID
		text  string
	}{
		{11, "A\n"},
		{12, "B\n"},
		{21, "C\n"},
	}
	var wait sync.WaitGroup
	errs := make([]error, len(targets))
	for index, target := range targets {
		wait.Add(1)
		go func(index int, block model.ID, text string) {
			defer wait.Done()
			service := s.service
			if index%2 == 1 {
				service = second
			}
			_, errs[index] = service.Set("fixture", block, 0, text, false)
		}(index, target.block, target.text)
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("write %d: %v", index, err)
		}
	}
	for _, target := range targets {
		if s.bodyOf(t, target.block) != target.text {
			t.Fatalf("block %s: %q", target.block, s.bodyOf(t, target.block))
		}
	}
	rollbacks, err := s.life.RollbackStore("fixture")
	if err != nil {
		t.Fatal(err)
	}
	packages, err := rollbacks.List()
	if err != nil {
		t.Fatal(err)
	}
	bodyPackages := 0
	for _, pkg := range packages {
		if pkg.Kind == rollback.KindBody {
			bodyPackages++
		}
	}
	if bodyPackages != 3 {
		t.Fatalf("body packages %d", bodyPackages)
	}
}

func TestLockedBodyRejected(t *testing.T) {
	s := newScene(t)
	s.setMeta(t, 11, model.BlockMeta{Type: "UILayout", Locked: true})
	before := s.bodyOf(t, 11)
	if _, err := s.service.Set("fixture", 11, 0, "x", false); err == nil {
		t.Fatal("expected body.locked")
	} else if bodyErr, ok := err.(*Error); !ok || bodyErr.Code != "body.locked" {
		t.Fatalf("unexpected error %v", err)
	}
	if _, err := s.service.Set("fixture", 11, 0, "x", true); err == nil {
		t.Fatal("a dry run must reject a locked block as well")
	}
	if _, err := s.service.Append("fixture", 11, 0, "x", false); err == nil {
		t.Fatal("append on a locked block")
	}
	if _, err := s.service.Replace("fixture", 11, []Pair{{Find: "Body", Replace: "X"}}, ReplaceOptions{Expect: 1}, false); err == nil {
		t.Fatal("replace on a locked block")
	}
	if s.bodyOf(t, 11) != before {
		t.Fatal("a locked body was changed")
	}
}

func TestListShowsMeta(t *testing.T) {
	s := newScene(t)
	s.setMeta(t, 12, model.BlockMeta{Type: "Note", Locked: true})
	rows, err := s.service.List("fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		switch row.ID {
		case 11:
			if row.Type != "" || row.Locked {
				t.Fatalf("block 11 meta %+v", row)
			}
		case 12:
			if row.Type != "Note" || !row.Locked {
				t.Fatalf("block 12 meta %+v", row)
			}
		}
	}
}

func TestParseLines(t *testing.T) {
	if from, to, err := ParseLines(""); err != nil || from != 0 || to != 0 {
		t.Fatalf("%d %d %v", from, to, err)
	}
	if from, to, err := ParseLines("3-5"); err != nil || from != 3 || to != 5 {
		t.Fatalf("%d %d %v", from, to, err)
	}
	if from, to, err := ParseLines("3"); err != nil || from != 3 || to != 3 {
		t.Fatalf("single line: %d %d %v", from, to, err)
	}
	for _, value := range []string{"0-1", "5-3", "x"} {
		if _, _, err := ParseLines(value); err == nil {
			t.Fatalf("%q must be rejected", value)
		}
	}
}

func TestAnchorLineReported(t *testing.T) {
	issues := CheckProse("ok\n\n## bad\n")
	if len(issues) != 1 || issues[0].Line != 3 {
		t.Fatalf("issues %+v", issues)
	}
}

func TestCheckProseDoesNotTreatFourHashesAsHeading(t *testing.T) {
	if len(CheckProse("#### text")) != 0 {
		t.Fatal("#### must be text")
	}
}

func TestNormalizeRejectsBOM(t *testing.T) {
	if _, err := normalize("\uFEFFx"); err == nil {
		t.Fatal("a BOM must be rejected")
	}
	if value, err := normalize("a\r\nb\r"); err != nil || value != "a\nb\n" {
		t.Fatalf("%q %v", value, err)
	}
}

func TestAppendSeparator(t *testing.T) {
	if combine("A", "B", modeAppend) != "A\nB" {
		t.Fatal("append separator")
	}
	if combine("A", "B", modePrepend) != "B\nA" {
		t.Fatal("prepend separator")
	}
}

func TestReplaceLiteralSinglePass(t *testing.T) {
	text, count, err := applyPairs("ab", []Pair{{Find: "a", Replace: "b"}, {Find: "b", Replace: "a"}}, ReplaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if text != "ba" || count != 2 {
		t.Fatalf("%q %d", text, count)
	}
}

func TestListKeepsOrder(t *testing.T) {
	s := newScene(t)
	rows, err := s.service.List("fixture")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID.String())
	}
	if strings.Join(ids, ",") != "11,12,21" {
		t.Fatalf("order %v", ids)
	}
}
