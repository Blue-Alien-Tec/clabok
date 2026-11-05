package render

import (
	"os"
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
