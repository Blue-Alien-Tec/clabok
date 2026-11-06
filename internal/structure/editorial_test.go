package structure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestSetProse(t *testing.T) {
	scene := newScene(t)
	receipt, err := scene.service.SetProse("fixture", 10, "Short summary", false)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Kind != "prose" || !receipt.Changed || receipt.Block.IsZero() {
		t.Fatalf("receipt %+v", receipt)
	}
	manifest := scene.manifest(t)
	if manifest.Root.Children[0].ProseBlock != receipt.Block {
		t.Fatalf("node prose block %s, want %s",
			manifest.Root.Children[0].ProseBlock, receipt.Block)
	}
	region := findRegion(manifest, model.RegionProse, model.Target{Kind: model.TargetNode, ID: 10})
	if region == nil || len(region.Blocks) != 1 || region.Blocks[0] != receipt.Block {
		t.Fatalf("prose region %+v", region)
	}
	body, err := os.ReadFile(filepath.Join(scene.paths.BlocksDir, receipt.Block.String()+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Short summary" {
		t.Fatalf("prose body %q", body)
	}

	before := scene.manifestBytes(t)
	if _, err := scene.service.SetProse("fixture", 10, "Other", true); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(scene.manifestBytes(t)) {
		t.Fatal("a dry run wrote the manifest")
	}

	replaced, err := scene.service.SetProse("fixture", 10, "Other", false)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Block != receipt.Block {
		t.Fatalf("replacing prose changed the block: %s -> %s", receipt.Block, replaced.Block)
	}
	body, err = os.ReadFile(filepath.Join(scene.paths.BlocksDir, replaced.Block.String()+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Other" {
		t.Fatalf("replaced prose body %q", body)
	}
	region = findRegion(scene.manifest(t), model.RegionProse,
		model.Target{Kind: model.TargetNode, ID: 10})
	if region == nil || len(region.Blocks) != 1 {
		t.Fatalf("prose region after replace %+v", region)
	}

	if _, err := scene.service.SetProse("fixture", 10, "line one\nline two", false); err == nil {
		t.Fatal("expected structure.invalid-prose")
	} else if coded, ok := err.(*Error); !ok || coded.Code != "structure.invalid-prose" {
		t.Fatalf("unexpected error %v", err)
	}

	if _, err := scene.service.SetProse("fixture", 10, "", false); err != nil {
		t.Fatal(err)
	}
	manifest = scene.manifest(t)
	if !manifest.Root.Children[0].ProseBlock.IsZero() {
		t.Fatalf("prose reference survived clearing: %s", manifest.Root.Children[0].ProseBlock)
	}
	if findRegion(manifest, model.RegionProse, model.Target{Kind: model.TargetNode, ID: 10}) != nil {
		t.Fatal("the prose region survived clearing")
	}
	if _, err := os.Stat(filepath.Join(scene.paths.BlocksDir, receipt.Block.String()+".md")); !os.IsNotExist(err) {
		t.Fatalf("prose body survived clearing: %v", err)
	}
}

func TestFooterLifecycle(t *testing.T) {
	scene := newScene(t)
	first, err := scene.service.SetFooter("fixture", "Footer one", false)
	if err != nil {
		t.Fatal(err)
	}
	manifest := scene.manifest(t)
	region := findRegion(manifest, model.RegionFooter, model.Target{Kind: model.TargetBook})
	if region == nil || len(region.Blocks) != 1 || region.Blocks[0] != first.Target {
		t.Fatalf("footer region %+v", region)
	}
	body, err := os.ReadFile(filepath.Join(scene.paths.BlocksDir, first.Target.String()+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Footer one" {
		t.Fatalf("footer body %q", body)
	}

	second, err := scene.service.SetFooter("fixture", "Footer two", false)
	if err != nil {
		t.Fatal(err)
	}
	manifest = scene.manifest(t)
	region = findRegion(manifest, model.RegionFooter, model.Target{Kind: model.TargetBook})
	if region == nil || len(region.Blocks) != 1 || region.Blocks[0] != second.Target {
		t.Fatalf("footer after replace %+v", region)
	}
	if _, err := os.Stat(filepath.Join(scene.paths.BlocksDir, first.Target.String()+".md")); !os.IsNotExist(err) {
		t.Fatalf("old footer body still present: %v", err)
	}

	if _, err := scene.service.ClearFooter("fixture", false); err != nil {
		t.Fatal(err)
	}
	if findRegion(scene.manifest(t), model.RegionFooter, model.Target{Kind: model.TargetBook}) != nil {
		t.Fatal("the footer region survived clearing")
	}
	if _, err := os.Stat(filepath.Join(scene.paths.BlocksDir, second.Target.String()+".md")); !os.IsNotExist(err) {
		t.Fatalf("footer body survived clearing: %v", err)
	}
}

func TestNotesLifecycle(t *testing.T) {
	scene := newScene(t)
	first, err := scene.service.AddNote("fixture", model.TargetNode, 10, "Note one", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := scene.service.AddNote("fixture", model.TargetNode, 10, "Note two", false)
	if err != nil {
		t.Fatal(err)
	}
	region := findRegion(scene.manifest(t), model.RegionNotes, model.Target{Kind: model.TargetNode, ID: 10})
	if region == nil || len(region.Blocks) != 3 ||
		region.Blocks[1] != first.Target || region.Blocks[2] != second.Target {
		t.Fatalf("notes region %+v", region)
	}
	body, err := os.ReadFile(filepath.Join(scene.paths.BlocksDir, second.Target.String()+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Note two" {
		t.Fatalf("note body %q", body)
	}

	if _, err := scene.service.AddNote("fixture", model.TargetBlock, 11, "Block note", false); err != nil {
		t.Fatal(err)
	}
	blockRegion := findRegion(scene.manifest(t), model.RegionNotes, model.Target{Kind: model.TargetBlock, ID: 11})
	if blockRegion == nil || len(blockRegion.Blocks) != 1 {
		t.Fatalf("block notes region %+v", blockRegion)
	}

	if _, err := scene.service.RemoveNote("fixture", model.TargetNode, 10, false); err != nil {
		t.Fatal(err)
	}
	if findRegion(scene.manifest(t), model.RegionNotes, model.Target{Kind: model.TargetNode, ID: 10}) != nil {
		t.Fatal("the notes region survived removal")
	}
	for _, id := range []model.ID{first.Target, second.Target} {
		if _, err := os.Stat(filepath.Join(scene.paths.BlocksDir, id.String()+".md")); !os.IsNotExist(err) {
			t.Fatalf("note body %s survived removal: %v", id, err)
		}
	}
	if _, err := scene.service.RemoveNote("fixture", model.TargetNode, 10, false); err == nil {
		t.Fatal("expected structure.unknown-target")
	}
}

func TestRegionDryRunWritesNothing(t *testing.T) {
	scene := newScene(t)
	before := scene.manifestBytes(t)
	if _, err := scene.service.SetFooter("fixture", "Dry footer", true); err != nil {
		t.Fatal(err)
	}
	if _, err := scene.service.AddNote("fixture", model.TargetNode, 10, "Dry note", true); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(scene.manifestBytes(t)) {
		t.Fatal("a dry run wrote the manifest")
	}
	entries, err := os.ReadDir(scene.paths.BlocksDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			data, readErr := os.ReadFile(filepath.Join(scene.paths.BlocksDir, entry.Name()))
			if readErr == nil && (strings.Contains(string(data), "Dry footer") || strings.Contains(string(data), "Dry note")) {
				t.Fatalf("a dry run wrote a body: %s", entry.Name())
			}
		}
	}
}
