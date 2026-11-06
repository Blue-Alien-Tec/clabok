package structure

import (
	"os"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestLayoutLifecycle(t *testing.T) {
	scene := newScene(t)
	added, err := scene.service.AddLayoutBlock("fixture", 10, "legend true\nbutton Buy\n", false)
	if err != nil {
		t.Fatal(err)
	}
	if added.Block.IsZero() {
		t.Fatalf("receipt has no block: %+v", added)
	}
	meta := scene.manifest(t).Meta(added.Block)
	if !meta.Locked || meta.Type != "UILayout" {
		t.Fatalf("block metadata %+v", meta)
	}
	specPath := layoutPath(scene.paths, added.Block)
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(spec) != "legend true\nbutton Buy\n" {
		t.Fatalf("spec %q", spec)
	}

	if _, err := scene.service.SetLayout("fixture", added.Block, "button Dry\n", true); err != nil {
		t.Fatal(err)
	}
	spec, err = os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(spec) != "legend true\nbutton Buy\n" {
		t.Fatalf("a dry run wrote the spec: %q", spec)
	}

	if _, err := scene.service.SetLayout("fixture", added.Block, "button Sell\n", false); err != nil {
		t.Fatal(err)
	}
	spec, err = os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(spec) != "button Sell\n" {
		t.Fatalf("replaced spec %q", spec)
	}

	if _, err := scene.service.SetLayout("fixture", 11, "button X\n", false); err == nil {
		t.Fatal("expected structure.not-layout")
	} else if coded, ok := err.(*Error); !ok || coded.Code != "structure.not-layout" {
		t.Fatalf("unexpected error %v", err)
	}

	removed, err := scene.service.RemoveLayout("fixture", added.Block, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Block != added.Block {
		t.Fatalf("removed %s, want %s", removed.Block, added.Block)
	}
	manifest := scene.manifest(t)
	if manifest.IsPlaced(added.Block) {
		t.Fatal("the layout block is still placed")
	}
	if _, exists := manifest.BlockMeta[added.Block]; exists {
		t.Fatal("the layout block metadata survived")
	}
	if _, err := os.Stat(specPath); !os.IsNotExist(err) {
		t.Fatalf("the spec survived removal: %v", err)
	}
}

func TestLayoutRejectsUnplacedTarget(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.service.SetLayout("fixture", model.ID(404), "button X\n", false); err == nil {
		t.Fatal("expected structure.unknown-target")
	} else if coded, ok := err.(*Error); !ok || coded.Code != "structure.unknown-target" {
		t.Fatalf("unexpected error %v", err)
	}
}
