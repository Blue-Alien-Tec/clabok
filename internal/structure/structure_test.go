package structure

import (
	"os"
	"path/filepath"
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
	locks.Command = "test"
	life := lifecycle.New(layout, locks, lifecycle.Options{UndoLimit: 10, BookPointLimit: 3})

	manifest, err := model.NewManifest("fixture", "Fixture", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children = []*model.Node{
		{
			ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{11},
			Children: []*model.Node{
				{ID: 12, Kind: model.KindSubchapter, Title: "Sub", Blocks: []model.ID{13}},
			},
		},
		{ID: 20, Kind: model.KindChapter, Title: "Two", Blocks: []model.ID{21}},
	}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	_ = container.Write("manifest.json", data)
	_ = container.Write("blocks/11.md", []byte("Body 11\n"))
	_ = container.Write("blocks/13.md", []byte("Body 13\n"))
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
	return scene{service: New(life, model.NewSequenceAllocator(100)), life: life, paths: paths}
}

func (s scene) manifest(t *testing.T) *model.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func (s scene) setMeta(t *testing.T, id model.ID, meta model.BlockMeta) {
	t.Helper()
	paths, err := s.life.RequireOpen("fixture")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(paths.Dir, lifecycle.ManifestMember)
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

func (s scene) manifestBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func expectedIDs(t *testing.T, manifest *model.Manifest, id model.ID) []model.ID {
	t.Helper()
	node, ok := manifest.FindNode(id)
	if !ok {
		t.Fatalf("node %s not found", id)
	}
	ids := make([]model.ID, 0, len(node.Children))
	for _, child := range node.Children {
		ids = append(ids, child.ID)
	}
	return ids
}

func TestAddUnits(t *testing.T) {
	s := newScene(t)
	chapter, err := s.service.Add("fixture", "1. Three", Append(0), false)
	if err != nil {
		t.Fatal(err)
	}
	if chapter.Kind != "chapter" || chapter.Target != 100 || chapter.Parent != 1 {
		t.Fatalf("chapter %+v", chapter)
	}
	if len(chapter.Order) != 3 || chapter.Order[2] != 100 {
		t.Fatalf("order %v", chapter.Order)
	}
	manifest := s.manifest(t)
	node, ok := manifest.FindNode(100)
	if !ok || node.Title != "Three" || node.Kind != model.KindChapter {
		t.Fatalf("node %+v", node)
	}

	subchapter, err := s.service.Add("fixture", "Sub Two", Append(20), false)
	if err != nil {
		t.Fatal(err)
	}
	if subchapter.Kind != "subchapter" || subchapter.Parent != 20 {
		t.Fatalf("subchapter %+v", subchapter)
	}

	block, err := s.service.Add("fixture", "", Append(subchapter.Target), false)
	if err != nil {
		t.Fatal(err)
	}
	if block.Kind != "block" {
		t.Fatalf("block %+v", block)
	}
	if _, err := os.Stat(filepath.Join(s.paths.BlocksDir, block.Target.String()+".md")); err != nil {
		t.Fatalf("block body not created: %v", err)
	}
}

func TestAddUnderBlockFails(t *testing.T) {
	s := newScene(t)
	_, err := s.service.Add("fixture", "X", Append(11), false)
	if err == nil {
		t.Fatal("expected structure.parent-invalid")
	}
	if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.parent-invalid" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestRename(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.Rename("fixture", 10, "1. Eins", false); err != nil {
		t.Fatal(err)
	}
	node, _ := s.manifest(t).FindNode(10)
	if node.Title != "Eins" {
		t.Fatalf("title %q", node.Title)
	}
	_, err := s.service.Rename("fixture", 11, "Body", false)
	if err == nil {
		t.Fatal("expected structure.block-no-title")
	} else if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.block-no-title" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestMoveNode(t *testing.T) {
	s := newScene(t)
	receipt, err := s.service.Move("fixture", 12, Append(20), false)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Parent != 20 || receipt.Kind != "subchapter" {
		t.Fatalf("receipt %+v", receipt)
	}
	manifest := s.manifest(t)
	if _, ok := manifest.FindNode(12); !ok {
		t.Fatal("moved node disappeared")
	}
	if ids := expectedIDs(t, manifest, 20); len(ids) != 1 || ids[0] != 12 {
		t.Fatalf("chapter 20 children %v", ids)
	}
	if ids := expectedIDs(t, manifest, 10); len(ids) != 0 {
		t.Fatalf("chapter 10 children %v", ids)
	}
}

func TestMoveSubchapterToRootRetags(t *testing.T) {
	s := newScene(t)
	receipt, err := s.service.Move("fixture", 12, Append(0), false)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Kind != "chapter" {
		t.Fatalf("receipt %+v", receipt)
	}
	node, _ := s.manifest(t).FindNode(12)
	if node.Kind != model.KindChapter {
		t.Fatalf("kind %q", node.Kind)
	}
}

func TestMoveLevelExceeded(t *testing.T) {
	s := newScene(t)
	_, err := s.service.Move("fixture", 10, Append(20), false)
	if err == nil {
		t.Fatal("expected structure.level-exceeded")
	}
	if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.level-exceeded" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestMoveCycle(t *testing.T) {
	s := newScene(t)
	_, err := s.service.Move("fixture", 10, Append(12), false)
	if err == nil {
		t.Fatal("expected structure.cycle")
	}
	if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.cycle" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestMoveBlockKeepsBody(t *testing.T) {
	s := newScene(t)
	before, err := os.ReadFile(filepath.Join(s.paths.BlocksDir, "21.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Move("fixture", 21, Append(10), false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(s.paths.BlocksDir, "21.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a block move changed the body")
	}
	node, _ := s.manifest(t).FindNode(10)
	if len(node.Blocks) != 2 || node.Blocks[1] != 21 {
		t.Fatalf("blocks %v", node.Blocks)
	}
}

func TestMoveBlockBefore(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.Move("fixture", 21, Placement{Before: 11}, false); err != nil {
		t.Fatal(err)
	}
	node, _ := s.manifest(t).FindNode(10)
	if len(node.Blocks) != 2 || node.Blocks[0] != 21 || node.Blocks[1] != 11 {
		t.Fatalf("blocks %v", node.Blocks)
	}
}

func TestRemoveNode(t *testing.T) {
	s := newScene(t)
	_, err := s.service.Remove("fixture", 10, false, false)
	if err == nil {
		t.Fatal("expected structure.node-not-empty")
	} else if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.node-not-empty" {
		t.Fatalf("unexpected error %v", err)
	}
	if _, err := s.service.Remove("fixture", 10, true, false); err != nil {
		t.Fatal(err)
	}
	manifest := s.manifest(t)
	if _, ok := manifest.FindNode(10); ok {
		t.Fatal("node 10 still present")
	}
	for _, id := range []model.ID{11, 13} {
		if _, err := os.Stat(filepath.Join(s.paths.BlocksDir, id.String()+".md")); !os.IsNotExist(err) {
			t.Fatalf("body %s still present: %v", id, err)
		}
	}
}

func TestRemoveBlock(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.Remove("fixture", 21, false, false); err != nil {
		t.Fatal(err)
	}
	node, _ := s.manifest(t).FindNode(20)
	if len(node.Blocks) != 0 {
		t.Fatalf("blocks %v", node.Blocks)
	}
	if _, err := os.Stat(filepath.Join(s.paths.BlocksDir, "21.md")); !os.IsNotExist(err) {
		t.Fatalf("body still present: %v", err)
	}
}

func TestSettings(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.SetSetting("fixture", "numbering", "on", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.SetSetting("fixture", "index-depth", "3", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.SetSetting("fixture", "output", "part", false); err != nil {
		t.Fatal(err)
	}
	settings := s.manifest(t).Settings
	if !settings.Numbering.Enabled || settings.IndexDepth != 3 || settings.Output != model.OutputPart {
		t.Fatalf("settings %+v", settings)
	}
	if _, err := s.service.SetSetting("fixture", "output", "stripe", false); err == nil {
		t.Fatal("expected structure.invalid-value")
	}
	if _, err := s.service.SetSetting("fixture", "bogus", "on", false); err == nil {
		t.Fatal("expected structure.unknown-setting")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	s := newScene(t)
	before := s.manifestBytes(t)
	receipt, err := s.service.Add("fixture", "Dry", Append(0), true)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Changed {
		t.Fatal("receipt should report the planned change")
	}
	after := s.manifestBytes(t)
	if string(before) != string(after) {
		t.Fatal("a dry run wrote the manifest")
	}
}

func TestMutationsCreateUndoPackages(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.Add("fixture", "Undo", Append(0), false); err != nil {
		t.Fatal(err)
	}
	rollbacks, err := s.life.RollbackStore("fixture")
	if err != nil {
		t.Fatal(err)
	}
	packages, err := rollbacks.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pkg := range packages {
		if pkg.Kind == rollback.KindUndo {
			found = true
		}
	}
	if !found {
		t.Fatalf("no undo package after a mutation: %+v", packages)
	}
}

func TestSetBlockType(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.SetBlockType("fixture", 11, "UILayout", false); err != nil {
		t.Fatal(err)
	}
	manifest := s.manifest(t)
	if manifest.BlockType(11) != "UILayout" || manifest.IsLocked(11) {
		t.Fatalf("meta %+v", manifest.Meta(11))
	}
	if _, err := s.service.SetBlockType("fixture", 11, "", false); err != nil {
		t.Fatal(err)
	}
	if s.manifest(t).BlockType(11) != "" {
		t.Fatal("type not cleared")
	}
	if _, err := s.service.SetBlockType("fixture", 11, "UI Layout", false); err == nil {
		t.Fatal("expected structure.invalid-type")
	} else if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.invalid-type" {
		t.Fatalf("unexpected error %v", err)
	}
	if _, err := s.service.SetBlockType("fixture", 999, "X", false); err == nil {
		t.Fatal("expected structure.unknown-target")
	}
}

func TestLockedRules(t *testing.T) {
	s := newScene(t)
	s.setMeta(t, 11, model.BlockMeta{Type: "UILayout", Locked: true})
	if _, err := s.service.SetBlockType("fixture", 11, "Other", false); err == nil {
		t.Fatal("expected structure.block-locked")
	} else if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.block-locked" {
		t.Fatalf("unexpected error %v", err)
	}

	// A locked block may move: placement is remapping, not content.
	if _, err := s.service.Move("fixture", 11, Append(20), false); err != nil {
		t.Fatalf("a locked block may move: %v", err)
	}
	if s.manifest(t).BlockType(11) != "UILayout" {
		t.Fatal("the move lost the type")
	}

	// A locked block must be removable, and the metadata goes with it.
	if _, err := s.service.Remove("fixture", 11, false, false); err != nil {
		t.Fatalf("a locked block must be removable: %v", err)
	}
	manifest := s.manifest(t)
	if manifest.BlockType(11) != "" {
		t.Fatal("metadata survived removal")
	}
	if _, err := os.Stat(filepath.Join(s.paths.BlocksDir, "11.md")); !os.IsNotExist(err) {
		t.Fatalf("body survived removal: %v", err)
	}

	// Removing a node removes its locked blocks and their metadata.
	s.setMeta(t, 13, model.BlockMeta{Type: "UILayout", Locked: true})
	if _, err := s.service.Remove("fixture", 10, true, false); err != nil {
		t.Fatalf("node removal with a locked block: %v", err)
	}
	if s.manifest(t).BlockType(13) != "" {
		t.Fatal("metadata survived node removal")
	}
}

func TestAddLockedBlock(t *testing.T) {
	s := newScene(t)
	receipt, err := s.service.AddLockedBlock("fixture", 10, "UILayout", false)
	if err != nil {
		t.Fatal(err)
	}
	manifest := s.manifest(t)
	if !manifest.IsLocked(receipt.Target) || manifest.BlockType(receipt.Target) != "UILayout" {
		t.Fatalf("meta %+v", manifest.Meta(receipt.Target))
	}
	if _, err := s.service.SetBlockType("fixture", receipt.Target, "X", false); err == nil {
		t.Fatal("the type of a locked block must not change")
	}
}

func TestAddTypeOnNodeRejected(t *testing.T) {
	s := newScene(t)
	if _, err := s.service.AddTyped("fixture", "Sub", "UILayout", Append(10), false); err == nil {
		t.Fatal("expected structure.type-on-node")
	} else if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.type-on-node" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestUnknownTarget(t *testing.T) {
	s := newScene(t)
	_, err := s.service.Rename("fixture", 999, "X", false)
	if err == nil {
		t.Fatal("expected structure.unknown-target")
	}
	if structureErr, ok := err.(*Error); !ok || structureErr.Code != "structure.unknown-target" {
		t.Fatalf("unexpected error %v", err)
	}
}
