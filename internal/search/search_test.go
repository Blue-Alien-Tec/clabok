package search

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

type scene struct {
	Service *Service
	Life    *lifecycle.Lifecycle
	Layout  store.Layout
	Paths   map[model.BookID]store.BookPaths
}

// newScene builds two closed registered books. The fixture book has a chapter with a subchapter,
// a UILayout block, a generated RepoLayout block, a block without a body, and prose, notes and
// footer regions.
func newScene(t *testing.T) *scene {
	t.Helper()
	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := lifecycle.New(layout, locks, lifecycle.Options{UndoLimit: 10, BookPointLimit: 3})
	fixture := fixtureManifest(t)
	writeBook(t, layout, "fixture", fixture, map[string]string{
		"blocks/11.md":               "Hello world\nHello again\n",
		"blocks/12.md":               "drawn\n",
		"blocks/21.md":               "Hello there\n",
		"blocks/22.md":               "SHOULD NOT MATCH\nHello generated\n",
		"blocks/24.md":               "See [[id:99]] and [[id:10]].\n",
		"blocks/31.md":               "index prose Hello\n",
		"blocks/33.md":               "note Hello\n",
		"blocks/35.md":               "footer Hello\n",
		"layouts/asciiui/12.asciiui": "panel Main\n",
	})
	second, err := model.NewManifest("second", "Second", model.NewSequenceAllocator(100))
	if err != nil {
		t.Fatal(err)
	}
	second.Root.Children = []*model.Node{{
		ID: 40, Kind: model.KindChapter, Title: "Other", Blocks: []model.ID{41},
	}}
	writeBook(t, layout, "second", second, map[string]string{
		"blocks/41.md": "Hello second\n",
	})
	registry := &store.Registry{Version: store.RegistryVersion, Books: []store.RegistryEntry{
		{ID: "fixture", Path: "Docs/fixture", Kind: store.KindBook},
		{ID: "second", Path: "Docs/second", Kind: store.KindBook},
	}}
	if err := registry.Save(layout.Registry()); err != nil {
		t.Fatal(err)
	}
	paths := map[model.BookID]store.BookPaths{}
	for _, id := range []model.BookID{"fixture", "second"} {
		bookPaths, pathErr := layout.Book(id)
		if pathErr != nil {
			t.Fatal(pathErr)
		}
		paths[id] = bookPaths
	}
	return &scene{Service: New(life, registry, layout.Root), Life: life, Layout: layout, Paths: paths}
}

func fixtureManifest(t *testing.T) *model.Manifest {
	t.Helper()
	manifest, err := model.NewManifest("fixture", "Fixture", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Settings.Output = model.OutputChapter
	manifest.Status = &model.StatusConfig{Enabled: true}
	manifest.Root.Status = "wip"
	manifest.Root.Children = []*model.Node{{
		ID: 10, Kind: model.KindChapter, Title: "One", Part: "Guide", Status: "wip",
		Blocks: []model.ID{11, 12},
		Children: []*model.Node{{
			ID: 20, Kind: model.KindSubchapter, Title: "Two", Status: "draft",
			Blocks: []model.ID{21, 22, 23, 24},
		}},
	}}
	manifest.Regions = []*model.Region{
		{ID: 30, Kind: model.RegionProse, Target: model.Target{Kind: model.TargetNode, ID: 10}, Blocks: []model.ID{31}},
		{ID: 32, Kind: model.RegionNotes, Target: model.Target{Kind: model.TargetBlock, ID: 11}, Blocks: []model.ID{33}},
		{ID: 34, Kind: model.RegionFooter, Target: model.Target{Kind: model.TargetBook}, Blocks: []model.ID{35}},
	}
	manifest.BlockMeta = map[model.ID]model.BlockMeta{
		11: {Type: "table", Status: "wip"},
		12: {Type: "UILayout", Locked: true},
		21: {Status: "final"},
		22: {Type: "RepoLayout", Locked: true},
		23: {Type: "text"},
		24: {Type: "text"},
	}
	return manifest
}

func writeBook(t *testing.T, layout store.Layout, id model.BookID, manifest *model.Manifest, members map[string]string) {
	t.Helper()
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	if err := container.Write("manifest.json", data); err != nil {
		t.Fatal(err)
	}
	for name, text := range members {
		if err := container.Write(name, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := layout.Book(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
}

func findBook(t *testing.T, scene *scene, book model.BookID, query string, options Options) Result {
	t.Helper()
	options.Book = book
	options.Query = query
	result, err := scene.Service.Find(options)
	if err != nil {
		t.Fatalf("find %q: %v", query, err)
	}
	return result
}

func TestMatchRenderedFile(t *testing.T) {
	scene := newScene(t)
	result := findBook(t, scene, "fixture", "Hello", Options{})
	for _, match := range result.Matches {
		if match.File != "Docs/fixture/one.md" {
			t.Fatalf("block %s file %q", match.Block, match.File)
		}
	}
	title := findBook(t, scene, "fixture", "Fixture", Options{})
	if len(title.Matches) != 1 || title.Matches[0].File != "Docs/fixture/README.md" {
		t.Fatalf("title %+v", title.Matches)
	}
	footer := findBook(t, scene, "fixture", "footer", Options{Fields: []string{"footer"}})
	if len(footer.Matches) != 1 || footer.Matches[0].File != "Docs/fixture/README.md" {
		t.Fatalf("footer %+v", footer.Matches)
	}
}

func TestFindDefaultFields(t *testing.T) {
	scene := newScene(t)
	result := findBook(t, scene, "fixture", "Hello", Options{})
	if len(result.Matches) != 3 {
		t.Fatalf("matches %+v", result.Matches)
	}
	for _, match := range result.Matches {
		if match.Field != "body" {
			t.Fatalf("default field %q", match.Field)
		}
	}
	first := result.Matches[0]
	if first.Book != "fixture" || first.BookPath != "Docs/fixture" || first.Node != "10" || first.Block != "11" {
		t.Fatalf("first match %+v", first)
	}
	if first.Column != 1 || first.Offset != 0 || first.Length != 5 || first.Occurrences != 1 {
		t.Fatalf("span %+v", first)
	}
}

func TestFindRejectsGeneratedRepoLayoutBody(t *testing.T) {
	scene := newScene(t)
	result := findBook(t, scene, "fixture", "SHOULD NOT MATCH", Options{})
	if len(result.Matches) != 0 {
		t.Fatalf("a generated body was searched: %+v", result.Matches)
	}
}

func TestFindFields(t *testing.T) {
	scene := newScene(t)
	cases := []struct {
		fields []string
		query  string
		field  string
		block  string
	}{
		{[]string{"layout"}, "Main", "layout", "12"},
		{[]string{"prose"}, "index prose", "prose", "31"},
		{[]string{"note"}, "note", "note", "33"},
		{[]string{"footer"}, "footer", "footer", "35"},
	}
	for _, test := range cases {
		result := findBook(t, scene, "fixture", test.query, Options{Fields: test.fields})
		if len(result.Matches) != 1 {
			t.Fatalf("%v: %+v", test.fields, result.Matches)
		}
		match := result.Matches[0]
		if match.Field != test.field || match.Block != test.block {
			t.Fatalf("%v: %+v", test.fields, match)
		}
	}
	all := findBook(t, scene, "fixture", "Hello", Options{Fields: []string{"all"}})
	if len(all.Matches) != 6 {
		t.Fatalf("all fields: %d matches", len(all.Matches))
	}
	for _, match := range all.Matches {
		if match.Field == "title" {
			continue
		}
		if match.Block == "" {
			t.Fatalf("a field match lacks a block: %+v", match)
		}
	}
}

func TestFindKindFilter(t *testing.T) {
	scene := newScene(t)
	chapter := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Kinds: []string{"chapter"}}})
	if len(chapter.Matches) != 2 {
		t.Fatalf("chapter matches %+v", chapter.Matches)
	}
	subchapter := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Kinds: []string{"subchapter"}}})
	if len(subchapter.Matches) != 1 || subchapter.Matches[0].Block != "21" {
		t.Fatalf("subchapter matches %+v", subchapter.Matches)
	}
}

func TestFindTypeAndStatusFilters(t *testing.T) {
	scene := newScene(t)
	table := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Types: []string{"TABLE"}}})
	if len(table.Matches) != 2 {
		t.Fatalf("type matches %+v", table.Matches)
	}
	wip := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Statuses: []string{"wip"}}})
	if len(wip.Matches) != 2 {
		t.Fatalf("wip matches %+v", wip.Matches)
	}
	final := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Statuses: []string{"final"}}})
	if len(final.Matches) != 1 || final.Matches[0].Block != "21" {
		t.Fatalf("final matches %+v", final.Matches)
	}
	unset := findBook(t, scene, "fixture", "drawn", Options{Filter: Filter{Statuses: []string{"unset"}}})
	if len(unset.Matches) != 1 || unset.Matches[0].Block != "12" {
		t.Fatalf("unset matches %+v", unset.Matches)
	}
}

func TestFindLockedAndPartFilters(t *testing.T) {
	scene := newScene(t)
	locked := true
	unlocked := false
	lockedResult := findBook(t, scene, "fixture", "drawn", Options{Filter: Filter{Locked: &locked}})
	if len(lockedResult.Matches) != 1 || lockedResult.Matches[0].Block != "12" {
		t.Fatalf("locked matches %+v", lockedResult.Matches)
	}
	unlockedResult := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Locked: &unlocked}})
	if len(unlockedResult.Matches) != 3 {
		t.Fatalf("unlocked matches %+v", unlockedResult.Matches)
	}
	part := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Part: "Guide"}})
	if len(part.Matches) != 3 {
		t.Fatalf("part matches %+v", part.Matches)
	}
	missingPart := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Part: "Elsewhere"}})
	if len(missingPart.Matches) != 0 {
		t.Fatalf("part filter matched %+v", missingPart.Matches)
	}
}

func TestFindNodeAndBlockFilters(t *testing.T) {
	scene := newScene(t)
	subtree := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Node: 10}})
	if len(subtree.Matches) != 3 {
		t.Fatalf("subtree matches %+v", subtree.Matches)
	}
	leaf := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Node: 20}})
	if len(leaf.Matches) != 1 || leaf.Matches[0].Block != "21" {
		t.Fatalf("leaf matches %+v", leaf.Matches)
	}
	block := findBook(t, scene, "fixture", "Hello", Options{Filter: Filter{Block: 11}})
	if len(block.Matches) != 2 {
		t.Fatalf("block matches %+v", block.Matches)
	}
	note := findBook(t, scene, "fixture", "Hello", Options{Fields: []string{"note"}, Filter: Filter{Block: 11}})
	if len(note.Matches) != 1 || note.Matches[0].Block != "33" {
		t.Fatalf("block note matches %+v", note.Matches)
	}
}

func TestFindResultModes(t *testing.T) {
	scene := newScene(t)
	blocks := findBook(t, scene, "fixture", "Hello", Options{Result: "blocks"})
	payload, ok := blocks.Payload(0, 0).(blocksPayload)
	if !ok {
		t.Fatalf("payload %T", blocks.Payload(0, 0))
	}
	if payload.Count != 2 || payload.Occurrences != 3 || len(payload.Blocks) != 2 {
		t.Fatalf("blocks payload %+v", payload)
	}
	if payload.Blocks[0].Block != "11" || payload.Blocks[0].Count != 2 || payload.Blocks[0].FirstLine != 1 {
		t.Fatalf("first block %+v", payload.Blocks[0])
	}
	nodes := findBook(t, scene, "fixture", "Hello", Options{Result: "nodes"})
	nodePayload := nodes.Payload(0, 0).(nodesPayload)
	if nodePayload.Count != 2 || len(nodePayload.Nodes) != 2 {
		t.Fatalf("nodes payload %+v", nodePayload)
	}
	books := findBook(t, scene, "fixture", "Hello", Options{Result: "books"})
	bookPayload := books.Payload(0, 0).(booksPayload)
	if bookPayload.Count != 1 || bookPayload.Books[0].Count != 3 {
		t.Fatalf("books payload %+v", bookPayload)
	}
	count := findBook(t, scene, "fixture", "Hello", Options{Result: "count"})
	countResult := count.Payload(0, 0).(countPayload)
	if countResult.Count != 3 || countResult.Occurrences != 3 || countResult.Books != 1 ||
		countResult.Blocks != 2 || countResult.Nodes != 2 || countResult.Returned != 0 {
		t.Fatalf("count payload %+v", countResult)
	}
}

func TestFindLimitAndOffset(t *testing.T) {
	scene := newScene(t)
	result := findBook(t, scene, "fixture", "Hello", Options{Limit: 2})
	payload := result.Payload(2, 0).(matchesPayload)
	if payload.Count != 3 || payload.Returned != 2 || !payload.Truncated || len(payload.Matches) != 2 {
		t.Fatalf("limit payload %+v", payload)
	}
	offset := result.Payload(0, 2).(matchesPayload)
	if offset.Returned != 1 || offset.Truncated || len(offset.Matches) != 1 {
		t.Fatalf("offset payload %+v", offset)
	}
}

func TestFindWarnsAboutMissingBody(t *testing.T) {
	scene := newScene(t)
	result := findBook(t, scene, "fixture", "Hello", Options{})
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "search.missing-body" ||
		!strings.Contains(result.Warnings[0].Message, "23") {
		t.Fatalf("warnings %+v", result.Warnings)
	}
	_, err := scene.Service.Find(Options{Book: "fixture", Query: "Hello", Strict: true})
	if code := errorCode(err); code != "search.strict-failed" {
		t.Fatalf("strict error %v", err)
	}
}

func TestFindAllScope(t *testing.T) {
	scene := newScene(t)
	result, err := scene.Service.Find(Options{All: true, Query: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 4 {
		t.Fatalf("matches %+v", result.Matches)
	}
	if len(result.Books) != 2 || result.Books[0] != "fixture" || result.Books[1] != "second" {
		t.Fatalf("books %+v", result.Books)
	}
	payload := result.Payload(0, 0).(matchesPayload)
	if payload.Scope.Mode != "all" || len(payload.Scope.Books) != 2 {
		t.Fatalf("scope %+v", payload.Scope)
	}
}

func TestFindAllWarnsOnUnreadableBook(t *testing.T) {
	scene := newScene(t)
	if err := os.WriteFile(scene.Paths["second"].Container, []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := scene.Service.Find(Options{All: true, Query: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 3 {
		t.Fatalf("matches %+v", result.Matches)
	}
	warnings := 0
	for _, warning := range result.Warnings {
		if warning.Code == "search.read-failed" && warning.Book == "second" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("warnings %+v", result.Warnings)
	}
	if _, err := scene.Service.Find(Options{All: true, Query: "Hello", Strict: true}); errorCode(err) != "search.strict-failed" {
		t.Fatalf("strict error %v", err)
	}
}

func TestFindRejects(t *testing.T) {
	scene := newScene(t)
	cases := []struct {
		options Options
		code    string
	}{
		{Options{Book: "fixture"}, "search.empty-query"},
		{Options{Query: "x"}, "search.scope-required"},
		{Options{Book: "fixture", All: true, Query: "x"}, "search.scope-conflict"},
		{Options{Book: "missing", Query: "x"}, "search.unknown-book"},
		{Options{Book: "fixture", Query: "x", Mode: Mode("bogus")}, "search.invalid-mode"},
		{Options{Book: "fixture", Query: "x", Fields: []string{"bogus"}}, "search.invalid-field"},
		{Options{Book: "fixture", Query: "x", Filter: Filter{Block: 999}}, "search.unknown-id"},
		{Options{Book: "fixture", Query: "x", Filter: Filter{Node: 999}}, "search.unknown-id"},
	}
	for _, test := range cases {
		if _, err := scene.Service.Find(test.options); errorCode(err) != test.code {
			t.Fatalf("%+v: %v", test.options, err)
		}
	}
}

func TestFindNDJSON(t *testing.T) {
	scene := newScene(t)
	result := findBook(t, scene, "fixture", "Hello", Options{})
	data, err := result.NDJSON(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines %d: %s", len(lines), data)
	}
	types := make([]string, 0, len(lines))
	for _, line := range lines {
		var entry struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		types = append(types, entry.Type)
	}
	if types[0] != "meta" || types[1] != "match" || types[2] != "warning" || types[3] != "summary" {
		t.Fatalf("types %v", types)
	}
	var summary struct {
		Count     int  `json:"count"`
		Returned  int  `json:"returned"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(lines[3]), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Count != 3 || summary.Returned != 1 || !summary.Truncated {
		t.Fatalf("summary %+v", summary)
	}
}

func run(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(directory)
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	Register(registry)
	var stdout, stderr strings.Builder
	code := cli.Run(registry, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func commandArgs(scene *scene, args ...string) []string {
	return append(args, "--store-root", scene.Layout.Root, "--project", scene.Layout.Root)
}

func TestFindCommandValidation(t *testing.T) {
	scene := newScene(t)
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"find", "--query", "x"}, cli.ExitReject},
		{[]string{"find", "--book", "fixture", "--all", "--query", "x"}, cli.ExitReject},
		{[]string{"find", "--book", "missing", "--query", "x"}, cli.ExitReject},
		{[]string{"find", "--book", "fixture", "--query", "x", "--mode", "bogus"}, cli.ExitUsage},
		{[]string{"find", "--book", "fixture", "--query", "x", "--format", "ndjson", "--text"}, cli.ExitUsage},
		{[]string{"find", "--book", "fixture", "--query", "x", "--result", "count", "--limit", "2"}, cli.ExitUsage},
	}
	for _, test := range cases {
		code, _, stderr := run(t, scene.Layout.Root, commandArgs(scene, test.args...)...)
		if code != test.code {
			t.Fatalf("%v: exit %d (want %d) %s", test.args, code, test.code, stderr)
		}
	}
}

func TestFindCommandOutput(t *testing.T) {
	scene := newScene(t)
	code, stdout, stderr := run(t, scene.Layout.Root, commandArgs(scene, "find", "--book", "fixture", "--query", "Hello")...)
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var envelope struct {
		Schema int  `json:"schema"`
		OK     bool `json:"ok"`
		Data   struct {
			SchemaVersion int     `json:"schemaVersion"`
			Count         int     `json:"count"`
			Matches       []Match `json:"matches"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("payload %s: %v", stdout, err)
	}
	if envelope.Data.SchemaVersion != SchemaVersion || envelope.Data.Count != 3 {
		t.Fatalf("payload %+v", envelope.Data)
	}
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Hello", "--format", "ndjson")...)
	if code != cli.ExitOK {
		t.Fatalf("ndjson exit %d: %s", code, stderr)
	}
	if !strings.HasPrefix(stdout, `{"type":"meta"`) || !strings.Contains(stdout, `"type":"summary"`) {
		t.Fatalf("ndjson %s", stdout)
	}
	code, stdout, stderr = run(t, scene.Layout.Root,
		commandArgs(scene, "find", "--book", "fixture", "--query", "Hello", "--count")...)
	if code != cli.ExitOK {
		t.Fatalf("count exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"count": 3`) || !strings.Contains(stdout, `"blocks": 2`) {
		t.Fatalf("count payload %s", stdout)
	}
}
