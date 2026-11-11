package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

type scene struct {
	service *Service
	life    *lifecycle.Lifecycle
	paths   store.BookPaths
}

func newScene(t *testing.T, mode model.OutputMode) scene {
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
	manifest, err := model.NewManifest("fixture", "Book", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Settings.Output = mode
	manifest.Root.Blocks = []model.ID{2}
	manifest.Root.Children = []*model.Node{
		{
			ID: 10, Kind: model.KindChapter, Title: "One", Part: "Basics", Blocks: []model.ID{11},
			Children: []*model.Node{
				{ID: 12, Kind: model.KindSubchapter, Title: "Sub", Blocks: []model.ID{13}},
			},
		},
		{ID: 20, Kind: model.KindChapter, Title: "Two", Part: "Advanced", Blocks: []model.ID{21}},
	}
	manifest.Regions = []*model.Region{
		{ID: 40, Kind: model.RegionFooter, Target: model.Target{Kind: model.TargetBook}, Blocks: []model.ID{43}},
		{ID: 41, Kind: model.RegionNotes, Target: model.Target{Kind: model.TargetNode, ID: 10}, Blocks: []model.ID{44}},
		{ID: 42, Kind: model.RegionNotes, Target: model.Target{Kind: model.TargetBlock, ID: 13}, Blocks: []model.ID{45}},
	}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	_ = container.Write("manifest.json", data)
	for id, body := range map[model.ID]string{
		2:  "Intro\n",
		11: "Body 11\n",
		13: "Body 13\n",
		21: "Body 21\n",
		43: "Footer\n",
		44: "Note 10\n",
		45: "Note 13\n",
	} {
		_ = container.Write("blocks/"+id.String()+".md", []byte(body))
	}
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
	return scene{service: New(life), life: life, paths: paths}
}

func (s scene) file(t *testing.T, result Result, name string) string {
	t.Helper()
	for _, file := range result.Files {
		if file.Path == name {
			return file.Content
		}
	}
	t.Fatalf("file %s not rendered; got %v", name, result.Files)
	return ""
}

func TestRenderMonolith(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "README.md" {
		t.Fatalf("files %+v", result.Files)
	}
	content := result.Files[0].Content
	for _, want := range []string{
		Marker, "# Book", "Intro", "## Index", "**Basics**", "[One](#one)",
		"  - [Sub](#sub)", "**Advanced**", "## One", "Body 11", "Note 10",
		"### Sub", "Body 13", "Note 13", "## Two", "Body 21", "Footer",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in\n%s", want, content)
		}
	}
	if strings.Index(content, "## Index") > strings.Index(content, "## One") {
		t.Fatal("index must precede the content")
	}
	if strings.Count(content, "Body 11") != 1 {
		t.Fatal("chapter content rendered twice")
	}
}

func TestRenderProseHeadingIsText(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("Body 11\n\n#### Deep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, "#### Deep") {
		t.Fatal("prose heading not rendered verbatim")
	}
	if strings.Contains(content, "- Deep") {
		t.Fatal("prose heading entered the index")
	}
}

func TestRenderChapters(t *testing.T) {
	s := newScene(t, model.OutputChapter)
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(result.Files))
	for _, file := range result.Files {
		names = append(names, file.Path)
	}
	if strings.Join(names, ",") != "README.md,one.md,two.md" {
		t.Fatalf("files %v", names)
	}
	index := s.file(t, result, "README.md")
	for _, want := range []string{"[One](one.md#one)", "  - [Sub](one.md#sub)", "[Two](two.md#two)", "Footer"} {
		if !strings.Contains(index, want) {
			t.Fatalf("missing %q in index\n%s", want, index)
		}
	}
	chapter := s.file(t, result, "one.md")
	for _, want := range []string{Marker, "[Index](README.md)", "# One", "## Sub", "Body 11", "Note 10"} {
		if !strings.Contains(chapter, want) {
			t.Fatalf("missing %q in chapter\n%s", want, chapter)
		}
	}
	if strings.Contains(chapter, "Footer") {
		t.Fatal("the footer belongs to the README")
	}
}

func TestRenderParts(t *testing.T) {
	s := newScene(t, model.OutputPart)
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(result.Files))
	for _, file := range result.Files {
		names = append(names, file.Path)
	}
	if strings.Join(names, ",") != "README.md,basics.md,advanced.md" {
		t.Fatalf("files %v", names)
	}
	index := s.file(t, result, "README.md")
	for _, want := range []string{"**Basics**", "(basics.md#one)", "**Advanced**", "(advanced.md#two)"} {
		if !strings.Contains(index, want) {
			t.Fatalf("missing %q in index\n%s", want, index)
		}
	}
	part := s.file(t, result, "basics.md")
	for _, want := range []string{"# Basics", "## One", "### Sub", "Body 11", "Body 13"} {
		if !strings.Contains(part, want) {
			t.Fatalf("missing %q in part\n%s", want, part)
		}
	}
}

func TestRenderPartMissing(t *testing.T) {
	s := newScene(t, model.OutputPart)
	data, err := os.ReadFile(filepath.Join(s.paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children[1].Part = ""
	encoded, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(filepath.Join(s.paths.Dir, lifecycle.ManifestMember), encoded); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Render("fixture"); err == nil {
		t.Fatal("expected render.missing-part")
	}
}

func TestSyncCheckWrite(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	output := t.TempDir()
	checked, err := s.service.Sync("fixture", output, SyncOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if checked.Written || len(checked.Changed) != 1 || checked.Status != store.StatusNoState {
		t.Fatalf("check result %+v", checked)
	}
	if _, err := os.Stat(filepath.Join(output, "README.md")); !os.IsNotExist(err) {
		t.Fatal("a check wrote a file")
	}

	written, err := s.service.Sync("fixture", output, SyncOptions{Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if !written.Written {
		t.Fatal("write did not report a write")
	}
	if _, err := os.Stat(filepath.Join(output, "README.md")); err != nil {
		t.Fatal(err)
	}
	clean, err := s.service.Sync("fixture", output, SyncOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if clean.Status != store.StatusClean || len(clean.Changed) != 0 {
		t.Fatalf("clean result %+v", clean)
	}
}

func TestSyncDriftAfterBodyEdit(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	output := t.TempDir()
	if _, err := s.service.Sync("fixture", output, SyncOptions{Write: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := s.service.Sync("fixture", output, SyncOptions{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != store.StatusDirtyInput && result.Status != store.StatusDirtyBoth {
		t.Fatalf("status %s", result.Status)
	}
	if len(result.Changed) != 1 {
		t.Fatalf("changed %v", result.Changed)
	}
}

func TestSyncForeignFileBacksUp(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(output, "README.md"), []byte("hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Sync("fixture", output, SyncOptions{Write: true}); err == nil {
		t.Fatal("expected render.foreign-file")
	}
	unchanged, err := os.ReadFile(filepath.Join(output, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != "hand written\n" {
		t.Fatal("the refusal changed the foreign file")
	}
	result, err := s.service.Sync("fixture", output, SyncOptions{Write: true, Force: true})
	if err != nil {
		t.Fatalf("force: %v", err)
	}
	if len(result.BackedUp) != 1 || result.BackedUp[0] != "README.bak" {
		t.Fatalf("backed up %v", result.BackedUp)
	}
	backup, err := os.ReadFile(filepath.Join(output, "README.bak"))
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "hand written\n" {
		t.Fatalf("backup changed: %q", backup)
	}
	data, err := os.ReadFile(filepath.Join(output, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), Marker) {
		t.Fatal("force did not overwrite")
	}
}

func TestSyncForeignFileBackupCollision(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(output, "README.md"), []byte("hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "README.bak"), []byte("older\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := s.service.Sync("fixture", output, SyncOptions{Write: true, Force: true})
	if err != nil {
		t.Fatalf("force: %v", err)
	}
	if len(result.BackedUp) != 1 || result.BackedUp[0] != "README.bak.1" {
		t.Fatalf("backed up %v", result.BackedUp)
	}
	backup, err := os.ReadFile(filepath.Join(output, "README.bak.1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "hand written\n" {
		t.Fatalf("backup changed: %q", backup)
	}
	old, err := os.ReadFile(filepath.Join(output, "README.bak"))
	if err != nil {
		t.Fatal(err)
	}
	if string(old) != "older\n" {
		t.Fatalf("the existing backup was overwritten: %q", old)
	}
}

func TestSyncPrune(t *testing.T) {
	s := newScene(t, model.OutputChapter)
	output := t.TempDir()
	if _, err := s.service.Sync("fixture", output, SyncOptions{Write: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "one.md")); err != nil {
		t.Fatal(err)
	}
	// Switch the mode to monolith and prune the chapter files.
	data, err := os.ReadFile(filepath.Join(s.paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Settings.Output = model.OutputMonolith
	encoded, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(filepath.Join(s.paths.Dir, lifecycle.ManifestMember), encoded); err != nil {
		t.Fatal(err)
	}
	result, err := s.service.Sync("fixture", output, SyncOptions{Write: true, Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pruned) != 2 {
		t.Fatalf("pruned %v", result.Pruned)
	}
	if _, err := os.Stat(filepath.Join(output, "one.md")); !os.IsNotExist(err) {
		t.Fatal("one.md survived the prune")
	}
	if _, err := os.Stat(filepath.Join(output, "README.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFindsBodyHeading(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	findings, err := s.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if finding.Blocking {
			t.Fatalf("unexpected blocking finding %+v", finding)
		}
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("## nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err = s.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range findings {
		if finding.Code == "body.forbidden-heading" && finding.Blocking {
			found = true
		}
	}
	if !found {
		t.Fatalf("findings %+v", findings)
	}
}

func TestRenderAsciiUIAnchor(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	manifestPath := filepath.Join(s.paths.Dir, lifecycle.ManifestMember)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children[0].Blocks = append(manifest.Root.Children[0].Blocks, 50)
	manifest.SetMeta(50, model.BlockMeta{Type: "UILayout", Locked: true})
	encoded, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(manifestPath, encoded); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "50.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	specDir := filepath.Join(s.paths.Dir, "layouts", "asciiui")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "50.asciiui"), []byte("legend true\nbutton Buy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, "<!-- asciiui:50 -->\n```text\n") || !strings.Contains(content, "[ Buy ]") {
		t.Fatalf("ascii drawing missing\n%s", content)
	}
	if !strings.Contains(content, "\n```\n<!-- /asciiui:50 -->") {
		t.Fatalf("the drawing is not fenced\n%s", content)
	}
	if !strings.Contains(content, "Buy: Button") {
		t.Fatalf("generic legend missing\n%s", content)
	}

	s.service.Overrides = map[string]string{"button": "UIButton"}
	result, err = s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Files[0].Content, "Buy: UIButton") {
		t.Fatalf("override missing\n%s", result.Files[0].Content)
	}

	// Moving a generated block changes only its position; the drawing bytes stay identical.
	before := drawingSection(t, result.Files[0].Content, 50)
	blocks := manifest.Root.Children[0].Blocks
	remaining := make([]model.ID, 0, len(blocks))
	for _, id := range blocks {
		if id != 50 {
			remaining = append(remaining, id)
		}
	}
	manifest.Root.Children[0].Blocks = remaining
	manifest.Root.Blocks = append(manifest.Root.Blocks, 50)
	encoded, err = manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(manifestPath, encoded); err != nil {
		t.Fatal(err)
	}
	result, err = s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if after := drawingSection(t, result.Files[0].Content, 50); after != before {
		t.Fatalf("the generated drawing changed after a move:\n%s\n%s", before, after)
	}
}

func drawingSection(t *testing.T, content string, id model.ID) string {
	t.Helper()
	start := strings.Index(content, "<!-- asciiui:"+id.String()+" -->")
	end := strings.Index(content, "<!-- /asciiui:"+id.String()+" -->")
	if start < 0 || end < start {
		t.Fatalf("drawing %s is missing\n%s", id, content)
	}
	return content[start : end+len("<!-- /asciiui:"+id.String()+" -->")]
}

func TestCheckMissingLayout(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	manifestPath := filepath.Join(s.paths.Dir, lifecycle.ManifestMember)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children[0].Blocks = append(manifest.Root.Children[0].Blocks, 51)
	manifest.SetMeta(51, model.BlockMeta{Type: "UILayout", Locked: true})
	encoded, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(manifestPath, encoded); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "51.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Render("fixture"); err == nil {
		t.Fatal("expected render.missing-layout")
	} else if coded, ok := err.(interface{ ErrorCode() string }); !ok || coded.ErrorCode() != "render.missing-layout" {
		t.Fatalf("unexpected error %v", err)
	}
	findings, err := s.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range findings {
		if finding.Code == "render.missing-layout" && finding.Blocking {
			found = true
		}
	}
	if !found {
		t.Fatalf("findings %+v", findings)
	}
}

func setManifest(t *testing.T, paths store.BookPaths, mutate func(*model.Manifest)) {
	t.Helper()
	path := filepath.Join(paths.Dir, lifecycle.ManifestMember)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	mutate(manifest)
	encoded, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(path, encoded); err != nil {
		t.Fatal(err)
	}
}

func TestNumberingKeepsAnchors(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	setManifest(t, s.paths, func(manifest *model.Manifest) {
		manifest.Settings.Numbering.Enabled = true
	})
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	for _, want := range []string{"## 1. One", "### 1.1 Sub", `<a id="one"></a>`, `<a id="sub"></a>`, "[One](#one)"} {
		if !strings.Contains(content, want) {
			t.Fatalf("numbered render missing %q\n%s", want, content)
		}
	}

	setManifest(t, s.paths, func(manifest *model.Manifest) {
		manifest.Settings.Numbering.Enabled = false
	})
	result, err = s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content = result.Files[0].Content
	for _, want := range []string{"## One", "### Sub", `<a id="one"></a>`, "[One](#one)"} {
		if !strings.Contains(content, want) {
			t.Fatalf("plain render missing %q\n%s", want, content)
		}
	}
}

func TestNumberingFollowsOrder(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	setManifest(t, s.paths, func(manifest *model.Manifest) {
		manifest.Settings.Numbering.Enabled = true
		children := manifest.Root.Children
		children[0], children[1] = children[1], children[0]
	})
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, "## 1. Two") || !strings.Contains(content, "## 2. One") {
		t.Fatalf("numbers do not follow the order\n%s", content)
	}
	if !strings.Contains(content, "[One](#one)") || !strings.Contains(content, "[Two](#two)") {
		t.Fatalf("anchors changed with the order\n%s", content)
	}
}

func TestAnchorCollision(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	setManifest(t, s.paths, func(manifest *model.Manifest) {
		manifest.Root.Children[1].Children = []*model.Node{
			{ID: 22, Kind: model.KindSubchapter, Title: "Sub"},
		}
	})
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, `<a id="sub"></a>`) || !strings.Contains(content, `<a id="sub-2"></a>`) {
		t.Fatalf("collision not resolved\n%s", content)
	}
}

func TestRefsExpand(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("See [[id:20]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Files[0].Content, "[Two](#two)") {
		t.Fatalf("monolith ref not expanded\n%s", result.Files[0].Content)
	}

	setManifest(t, s.paths, func(manifest *model.Manifest) {
		manifest.Settings.Output = model.OutputChapter
	})
	result, err = s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	chapter := s.file(t, result, "one.md")
	if !strings.Contains(chapter, "[Two](two.md#two)") {
		t.Fatalf("cross-file ref not expanded\n%s", chapter)
	}
}

func TestDanglingRefFinding(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("[[id:999]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := s.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range findings {
		if finding.Code == "refs.dangling" && finding.Blocking {
			found = true
		}
	}
	if !found {
		t.Fatalf("findings %+v", findings)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"One":            "one",
		"Part 1: Basics": "part-1-basics",
		"Ärger & Co":     "ärger-co",
		"":               "section",
	}
	for input, want := range cases {
		if got := Slug(input); got != want {
			t.Fatalf("Slug(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPlainTitle(t *testing.T) {
	cases := map[string]string{
		"Coverage of `PLAN.md` §3":           "Coverage of PLAN.md §3",
		"**Bold** and __Strong__":            "Bold and Strong",
		"~~Struck~~":                         "Struck",
		"[Nested](https://example.com) link": "Nested link",
		"![Alt](image.png) image":            "Alt image",
		"plain title":                        "plain title",
		"broken [bracket":                    "broken [bracket",
		"broken [link]":                      "broken [link]",
	}
	for input, want := range cases {
		if got := plainTitle(input); got != want {
			t.Fatalf("plainTitle(%q) = %q, want %q", input, got, want)
		}
	}
}
