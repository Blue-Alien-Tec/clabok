package render

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func editManifest(t *testing.T, scene scene, mutate func(*model.Manifest)) {
	t.Helper()
	path := filepath.Join(scene.paths.Dir, "manifest.json")
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
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBlockReferenceExpands(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	if err := os.WriteFile(filepath.Join(scene.paths.BlocksDir, "11.md"),
		[]byte("See [[id:13]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, "[Sub](#sub)") {
		t.Fatalf("a block reference did not expand:\n%s", content)
	}
	if strings.Contains(content, "[[id:13]]") {
		t.Fatalf("the literal reference survived:\n%s", content)
	}
}

func TestPerFileTOC(t *testing.T) {
	chapterScene := newScene(t, model.OutputChapter)
	editManifest(t, chapterScene, func(manifest *model.Manifest) {
		manifest.Settings.TOC.Enabled = true
	})
	result, err := chapterScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	chapter := chapterScene.file(t, result, "one.md")
	if !strings.Contains(chapter, "**Contents**") || !strings.Contains(chapter, "- [Sub](#sub)") {
		t.Fatalf("chapter ToC missing:\n%s", chapter)
	}

	partScene := newScene(t, model.OutputPart)
	editManifest(t, partScene, func(manifest *model.Manifest) {
		manifest.Settings.TOC.Enabled = true
	})
	partResult, err := partScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	part := partScene.file(t, partResult, "basics.md")
	if !strings.Contains(part, "**Contents**") ||
		!strings.Contains(part, "- [One](#one)") ||
		!strings.Contains(part, "  - [Sub](#sub)") {
		t.Fatalf("part ToC missing:\n%s", part)
	}

	offScene := newScene(t, model.OutputChapter)
	offResult, err := offScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(offScene.file(t, offResult, "one.md"), "**Contents**") {
		t.Fatal("a disabled ToC rendered")
	}
}

func TestPartNameCollisionRejected(t *testing.T) {
	scene := newScene(t, model.OutputPart)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children[0].Part = "A/B"
		manifest.Root.Children[1].Part = "A-B"
	})
	_, err := scene.service.Render("fixture")
	if err == nil {
		t.Fatal("expected render.part-name-collision")
	}
	coded, ok := err.(interface{ ErrorCode() string })
	if !ok || coded.ErrorCode() != "render.part-name-collision" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestIndexListWithoutProse(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	want := "**Basics**\n\n" +
		"- [One](#one)\n" +
		"  - [Sub](#sub)\n" +
		"\n**Advanced**\n\n" +
		"- [Two](#two)\n"
	if !strings.Contains(result.Files[0].Content, want) {
		t.Fatalf("index list mismatch:\n%s", result.Files[0].Content)
	}
	chapterTOC := chapterTOCForTest(t, scene)
	if !strings.Contains(chapterTOC, "- [Sub](#sub)") ||
		strings.Contains(chapterTOC, "|") {
		t.Fatalf("the ToC must stay a list:\n%s", chapterTOC)
	}
}

func TestIndexTableWithProse(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children[0].ProseBlock = 50
		manifest.Regions = append(manifest.Regions, &model.Region{
			ID: 46, Kind: model.RegionProse, Target: model.Target{Kind: model.TargetNode, ID: 10},
			Blocks: []model.ID{50},
		})
	})
	if err := os.WriteFile(filepath.Join(scene.paths.BlocksDir, "50.md"),
		[]byte("Summary of one"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	want := "**Basics**\n\n" +
		"| Document | Description |\n| --- | --- |\n" +
		"| [One](#one) | Summary of one |\n" +
		"| &emsp;[Sub](#sub) |  |\n" +
		"\n**Advanced**\n\n" +
		"| Document | Description |\n| --- | --- |\n" +
		"| [Two](#two) |  |\n"
	if !strings.Contains(result.Files[0].Content, want) {
		t.Fatalf("index table mismatch:\n%s", result.Files[0].Content)
	}
	chapterTOC := chapterTOCForTest(t, scene)
	if !strings.Contains(chapterTOC, "- [Sub](#sub)") ||
		strings.Contains(chapterTOC, "|") {
		t.Fatalf("the ToC must stay a list:\n%s", chapterTOC)
	}
}

func chapterTOCForTest(t *testing.T, scene scene) string {
	t.Helper()
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Settings.Output = model.OutputChapter
		manifest.Settings.TOC.Enabled = true
	})
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	return scene.file(t, result, "one.md")
}

func TestIndexProse(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children[0].ProseBlock = 50
		manifest.Regions = append(manifest.Regions, &model.Region{
			ID: 46, Kind: model.RegionProse, Target: model.Target{Kind: model.TargetNode, ID: 10},
			Blocks: []model.ID{50},
		})
	})
	if err := os.WriteFile(filepath.Join(scene.paths.BlocksDir, "50.md"),
		[]byte("Summary of one"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, "| [One](#one) | Summary of one |") {
		t.Fatalf("index prose missing:\n%s", content)
	}

	chapterScene := newScene(t, model.OutputChapter)
	editManifest(t, chapterScene, func(manifest *model.Manifest) {
		manifest.Settings.TOC.Enabled = true
		manifest.Root.Children[0].Children[0].ProseBlock = 50
		manifest.Regions = append(manifest.Regions, &model.Region{
			ID: 46, Kind: model.RegionProse, Target: model.Target{Kind: model.TargetNode, ID: 12},
			Blocks: []model.ID{50},
		})
	})
	if err := os.WriteFile(filepath.Join(chapterScene.paths.BlocksDir, "50.md"),
		[]byte("Sub summary"), 0o644); err != nil {
		t.Fatal(err)
	}
	chapterResult, err := chapterScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	readme := chapterScene.file(t, chapterResult, "README.md")
	if !strings.Contains(readme, "| &emsp;[Sub](one.md#sub) | Sub summary |") {
		t.Fatalf("chapter index prose missing:\n%s", readme)
	}
	chapter := chapterScene.file(t, chapterResult, "one.md")
	if strings.Contains(chapter, "Sub summary") {
		t.Fatalf("index prose leaked into the chapter ToC:\n%s", chapter)
	}
}

func TestStatusRendering(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Status = &model.StatusConfig{Enabled: true}
		manifest.Root.Children[0].Status = "final"
		manifest.SetMeta(11, model.BlockMeta{Status: "wip"})
	})
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	finalBadge := "![status: Final](https://img.shields.io/badge/status-Final-brightgreen?style=flat-square)"
	wipBadge := "![status: Work in Progress](https://img.shields.io/badge/status-Work%20in%20Progress-orange?style=flat-square)"
	for _, want := range []string{
		"| Document | Status |",
		"| [One](#one) | " + finalBadge + " |",
		"| &emsp;[Sub](#sub) |  |",
		"## One " + finalBadge,
		wipBadge + "\n\nBody 11",
		"**Status**",
		"| " + finalBadge + " | Content is settled; only errata and clarifications remain. |",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in\n%s", want, content)
		}
	}
	if strings.Contains(content, "| Document | Status | Description |") {
		t.Fatalf("an empty description column must not appear:\n%s", content)
	}
	legendAt := strings.Index(content, "**Status**")
	indexAt := strings.Index(content, "## Index")
	if legendAt < 0 || indexAt < 0 || legendAt > indexAt {
		t.Fatalf("the legend must precede the index:\n%s", content)
	}
	chapterTOC := chapterTOCForTest(t, scene)
	if strings.Contains(chapterTOC, "| Document |") {
		t.Fatalf("the ToC must not become a table:\n%s", chapterTOC)
	}
}

func TestStatusAndProseColumns(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Status = &model.StatusConfig{Enabled: true}
		manifest.Root.Children[0].Status = "final"
		manifest.Root.Children[0].ProseBlock = 50
		manifest.Regions = append(manifest.Regions, &model.Region{
			ID: 46, Kind: model.RegionProse, Target: model.Target{Kind: model.TargetNode, ID: 10},
			Blocks: []model.ID{50},
		})
	})
	if err := os.WriteFile(filepath.Join(scene.paths.BlocksDir, "50.md"),
		[]byte("Short summary"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, "| Document | Status | Description |") {
		t.Fatalf("missing the three-column header:\n%s", content)
	}
	if !strings.Contains(content, "| [One](#one) | ![status: Final]") ||
		!strings.Contains(content, "| Short summary |") {
		t.Fatalf("missing the prose/status row:\n%s", content)
	}
}

func TestStatusDisabledRendersNothing(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children[0].Status = "final"
		manifest.SetMeta(11, model.BlockMeta{Status: "wip"})
	})
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	for _, forbidden := range []string{"img.shields.io", "**Status**", "| Document | Status"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("a disabled system rendered %q:\n%s", forbidden, content)
		}
	}
	if !strings.Contains(content, "- [One](#one)") {
		t.Fatalf("the plain index must stay a list:\n%s", content)
	}
}

func TestLibraryBadges(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"),
		[]byte("module github.com/example/project\n\ngo 1.27.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "LICENSE"),
		[]byte("MIT License\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Badges = []model.Badge{
			{ID: "go"}, {ID: "license"}, {ID: "ghost"},
		}
	})
	scene.service.ProjectDir = project
	scene.service.Navigation.IsLibrary = true

	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	for _, want := range []string{
		"![go: 1.27.3](https://img.shields.io/badge/go-1.27.3-00ADD8?style=flat-square)",
		"![license: MIT](https://img.shields.io/badge/license-MIT-blue?style=flat-square)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in\n%s", want, content)
		}
	}
	if strings.Contains(content, "ghost") {
		t.Fatalf("an unresolved badge must be omitted:\n%s", content)
	}
	badgeAt := strings.Index(content, "![go:")
	headingAt := strings.Index(content, "# Book")
	if badgeAt < 0 || headingAt < 0 || badgeAt > headingAt {
		t.Fatalf("badges must sit above the title:\n%s", content)
	}

	findings, err := scene.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range findings {
		if finding.Code == "badge-unresolved" && strings.Contains(finding.Message, "ghost") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing badge-unresolved finding: %+v", findings)
	}

	scene.service.Navigation.IsLibrary = false
	result, err = scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Files[0].Content, "img.shields.io") {
		t.Fatal("a non-library book must not render badges")
	}
}

func TestRepoLayoutBlock(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	project := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", project}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("init", "-q")
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "src", "main.go"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "src/main.go")

	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children[0].Blocks = append(manifest.Root.Children[0].Blocks, 50)
		manifest.SetMeta(50, model.BlockMeta{Type: "RepoLayout", Locked: true})
		manifest.Layout = &model.LayoutConfig{
			Root: ".", Depth: 2, Notes: map[string]string{"src": "The Go sources."},
		}
	})
	if err := os.WriteFile(filepath.Join(scene.paths.BlocksDir, "50.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	scene.service.ProjectDir = project
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	for _, want := range []string{
		"<!-- repolayout:50 -->",
		"```text\nsrc/  The Go sources.\n```",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in\n%s", want, content)
		}
	}

	scene.service.ProjectDir = t.TempDir()
	result, err = scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content = result.Files[0].Content
	if strings.Contains(content, "Repository layout unavailable") || strings.Contains(content, "repolayout:50") {
		t.Fatalf("a layout without git must be omitted:\n%s", content)
	}
	if !strings.Contains(content, "Body 11") {
		t.Fatalf("the authored block of the same chapter must stay:\n%s", content)
	}
}

func TestLayoutOnlyChapterOmittedWithoutGit(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children = append(manifest.Root.Children, &model.Node{
			ID: 40, Kind: model.KindChapter, Title: "Generated Tree", Blocks: []model.ID{50},
		})
		manifest.SetMeta(50, model.BlockMeta{Type: "RepoLayout", Locked: true})
		manifest.Layout = &model.LayoutConfig{Root: ".", Depth: 2}
	})
	if err := os.WriteFile(filepath.Join(scene.paths.BlocksDir, "50.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	scene.service.ProjectDir = t.TempDir()
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Files[0].Content, "Generated Tree") {
		t.Fatalf("a layout-only chapter must be omitted without git:\n%s", result.Files[0].Content)
	}
}

func TestLayoutOnlyChapterRendersWithGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	project := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", project}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("init", "-q")
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children = append(manifest.Root.Children, &model.Node{
			ID: 40, Kind: model.KindChapter, Title: "Generated Tree", Blocks: []model.ID{50},
		})
		manifest.SetMeta(50, model.BlockMeta{Type: "RepoLayout", Locked: true})
		manifest.Layout = &model.LayoutConfig{Root: ".", Depth: 2}
	})
	if err := os.WriteFile(filepath.Join(scene.paths.BlocksDir, "50.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	scene.service.ProjectDir = project
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Files[0].Content, "Generated Tree") {
		t.Fatalf("a layout chapter must render with git:\n%s", result.Files[0].Content)
	}
}

func TestEmptyIndexIsOmitted(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children = nil
	})
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Files[0].Content, "## Index") {
		t.Fatalf("an empty index must not render:\n%s", result.Files[0].Content)
	}
}

func TestIndexProseOnLinkEntry(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children = append(manifest.Root.Children, &model.Node{
			ID: 30, Kind: model.KindLink, Title: "Elsewhere", ProseBlock: 50,
			Ref: &model.Ref{Book: "other"},
		})
		manifest.Regions = append(manifest.Regions, &model.Region{
			ID: 46, Kind: model.RegionProse, Target: model.Target{Kind: model.TargetNode, ID: 30},
			Blocks: []model.ID{50},
		})
	})
	if err := os.WriteFile(filepath.Join(scene.paths.BlocksDir, "50.md"),
		[]byte("Link summary"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Files[0].Content, "| Elsewhere | Link summary |") {
		t.Fatalf("link index prose missing:\n%s", result.Files[0].Content)
	}
}

func TestIndexTitleIsPlainText(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	editManifest(t, scene, func(manifest *model.Manifest) {
		manifest.Root.Children[0].Title = "Coverage of `PLAN.md` §3"
	})
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if !strings.Contains(content, "## Coverage of `PLAN.md` §3") {
		t.Fatalf("the heading must keep its formatting:\n%s", content)
	}
	if !strings.Contains(content, "- [Coverage of PLAN.md §3](#coverage-of-plan-md-3)") {
		t.Fatalf("the index entry must be plain text:\n%s", content)
	}
	if strings.Contains(content, "[Coverage of `PLAN.md`") {
		t.Fatalf("the index entry still carries inline markup:\n%s", content)
	}

	tocScene := newScene(t, model.OutputChapter)
	editManifest(t, tocScene, func(manifest *model.Manifest) {
		manifest.Settings.TOC.Enabled = true
		manifest.Root.Children[0].Children[0].Title = "Uses `PLAN.md`"
	})
	tocResult, err := tocScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	chapter := tocScene.file(t, tocResult, "one.md")
	if !strings.Contains(chapter, "- [Uses PLAN.md](#uses-plan-md)") {
		t.Fatalf("the ToC entry must be plain text:\n%s", chapter)
	}
	if strings.Contains(chapter, "[Uses `PLAN.md`") {
		t.Fatalf("the ToC entry still carries inline markup:\n%s", chapter)
	}
}
