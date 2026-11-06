package model

import (
	"reflect"
	"strings"
	"testing"
)

func validManifest() *Manifest {
	return &Manifest{
		Version: Version,
		ID:      "fixture",
		Root: &Node{
			ID:     1,
			Kind:   KindTitle,
			Title:  "Fixture",
			Blocks: []ID{10},
			Children: []*Node{
				{
					ID:     2,
					Kind:   KindChapter,
					Title:  "Chapter",
					Part:   "One",
					Blocks: []ID{11},
					Children: []*Node{
						{ID: 3, Kind: KindSubchapter, Title: "Sub", Blocks: []ID{12}},
					},
				},
				{ID: 4, Kind: KindLink, Title: "Elsewhere", Ref: &Ref{Book: "other"}},
			},
		},
		Regions: []*Region{
			{ID: 20, Kind: RegionFooter, Target: Target{Kind: TargetBook}, Blocks: []ID{13}},
			{ID: 21, Kind: RegionNotes, Target: Target{Kind: TargetNode, ID: 2}, Blocks: []ID{14}},
			{ID: 22, Kind: RegionNotes, Target: Target{Kind: TargetBlock, ID: 11}, Blocks: []ID{15}},
		},
		Settings: DefaultSettings(),
	}
}

func hasCode(err *ValidationError, code string) bool {
	if err == nil {
		return false
	}
	for _, issue := range err.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func expectCode(t *testing.T, manifest *Manifest, code string) {
	t.Helper()
	err := manifest.Validate()
	if err == nil {
		t.Fatalf("expected %s, got a valid manifest", code)
	}
	if !hasCode(err, code) {
		t.Fatalf("expected %s, got %v", code, err.Codes())
	}
}

func TestValidManifest(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatalf("unexpected issues: %v", err.Issues)
	}
}

func TestDepthExceeded(t *testing.T) {
	manifest := validManifest()
	subchapter := manifest.Root.Children[0].Children[0]
	subchapter.Children = []*Node{{ID: 99, Kind: KindSubchapter, Title: "Fourth"}}
	expectCode(t, manifest, "model.depth-exceeded")
}

func TestKindDepth(t *testing.T) {
	manifest := validManifest()
	manifest.Root.Children[0].Children[0].Kind = KindChapter
	expectCode(t, manifest, "model.kind-depth")

	manifest = validManifest()
	manifest.Root.Children[0].Kind = KindSubchapter
	expectCode(t, manifest, "model.kind-depth")

	manifest = validManifest()
	manifest.Root.Kind = KindChapter
	expectCode(t, manifest, "model.kind-depth")
}

func TestEmptyTitle(t *testing.T) {
	manifest := validManifest()
	manifest.Root.Children[0].Title = "   "
	expectCode(t, manifest, "model.empty-title")
}

func TestLinkRules(t *testing.T) {
	manifest := validManifest()
	link := manifest.Root.Children[1]
	link.Ref = nil
	expectCode(t, manifest, "model.link-missing-ref")

	manifest = validManifest()
	link = manifest.Root.Children[1]
	link.Blocks = []ID{30}
	expectCode(t, manifest, "model.link-not-leaf")

	manifest = validManifest()
	link = manifest.Root.Children[1]
	link.Children = []*Node{{ID: 31, Kind: KindChapter, Title: "Child"}}
	expectCode(t, manifest, "model.link-not-leaf")
}

func TestIdentifierRules(t *testing.T) {
	manifest := validManifest()
	manifest.Root.Children[0].ID = 0
	expectCode(t, manifest, "model.zero-id")

	manifest = validManifest()
	manifest.Root.Children[0].ID = 1
	expectCode(t, manifest, "model.duplicate-id")

	manifest = validManifest()
	manifest.Regions[0].ID = 2
	expectCode(t, manifest, "model.duplicate-id")

	manifest = validManifest()
	manifest.Root.Children[0].Blocks = []ID{10}
	expectCode(t, manifest, "model.duplicate-placement")

	manifest = validManifest()
	manifest.Root.Children[0].Blocks = []ID{11, 11}
	expectCode(t, manifest, "model.duplicate-placement")
}

func TestPartRules(t *testing.T) {
	manifest := validManifest()
	manifest.Root.Children[0].Children[0].Part = "One"
	expectCode(t, manifest, "model.part-on-node")

	manifest = validManifest()
	manifest.Root.Children[1].Part = "One"
	if err := manifest.Validate(); err != nil {
		t.Fatalf("a part on a link is allowed: %v", err.Issues)
	}
}

func TestRegionRules(t *testing.T) {
	manifest := validManifest()
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 30, Kind: RegionFooter, Target: Target{Kind: TargetBook},
	})
	expectCode(t, manifest, "model.footer-count")

	manifest = validManifest()
	manifest.Regions[0].Target = Target{Kind: TargetNode, ID: 2}
	expectCode(t, manifest, "model.invalid-target")

	manifest = validManifest()
	manifest.Regions[1].Target = Target{Kind: TargetBook}
	expectCode(t, manifest, "model.invalid-target")

	manifest = validManifest()
	manifest.Regions[1].Target = Target{Kind: TargetNode, ID: 404}
	expectCode(t, manifest, "model.invalid-target")

	manifest = validManifest()
	manifest.Regions[2].Target = Target{Kind: TargetBlock, ID: 404}
	expectCode(t, manifest, "model.invalid-target")

	manifest = validManifest()
	manifest.Regions[0].Kind = "sidebar"
	expectCode(t, manifest, "model.unknown-region-kind")
}

func TestProseRules(t *testing.T) {
	manifest := validManifest()
	manifest.Root.Children[0].ProseBlock = 16
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 23, Kind: RegionProse, Target: Target{Kind: TargetNode, ID: 2}, Blocks: []ID{16},
	})
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid prose: %v", err.Issues)
	}

	manifest = validManifest()
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 23, Kind: RegionProse, Target: Target{Kind: TargetBook}, Blocks: []ID{16},
	})
	expectCode(t, manifest, "model.invalid-target")

	manifest = validManifest()
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 23, Kind: RegionProse, Target: Target{Kind: TargetBlock, ID: 11}, Blocks: []ID{16},
	})
	expectCode(t, manifest, "model.invalid-target")

	manifest = validManifest()
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 23, Kind: RegionProse, Target: Target{Kind: TargetNode, ID: 2}, Blocks: []ID{16, 17},
	})
	expectCode(t, manifest, "model.prose-blocks")

	manifest = validManifest()
	manifest.Regions = append(manifest.Regions,
		&Region{ID: 23, Kind: RegionProse, Target: Target{Kind: TargetNode, ID: 2}, Blocks: []ID{16}},
		&Region{ID: 24, Kind: RegionProse, Target: Target{Kind: TargetNode, ID: 2}, Blocks: []ID{17}},
	)
	expectCode(t, manifest, "model.prose-count")

	manifest = validManifest()
	manifest.Root.Children[0].ProseBlock = 11
	expectCode(t, manifest, "model.prose-not-region")

	manifest = validManifest()
	manifest.Root.Children[0].ProseBlock = 16
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 23, Kind: RegionProse, Target: Target{Kind: TargetNode, ID: 3}, Blocks: []ID{16},
	})
	expectCode(t, manifest, "model.prose-owner")

	manifest = validManifest()
	manifest.Root.Children[0].ProseBlock = 16
	manifest.Regions = append(manifest.Regions,
		&Region{ID: 23, Kind: RegionProse, Target: Target{Kind: TargetNode, ID: 2}, Blocks: []ID{16}},
		&Region{ID: 24, Kind: RegionProse, Target: Target{Kind: TargetNode, ID: 3}, Blocks: []ID{17}},
	)
	expectCode(t, manifest, "model.prose-ref-missing")
}

func TestStatusRules(t *testing.T) {
	manifest := validManifest()
	manifest.Status = &StatusConfig{Enabled: true}
	manifest.Root.Children[0].Status = "final"
	manifest.BlockMeta = map[ID]BlockMeta{11: {Status: "wip"}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid status: %v", err.Issues)
	}

	manifest = validManifest()
	manifest.Status = &StatusConfig{Enabled: true}
	manifest.Root.Children[0].Status = "ghost"
	expectCode(t, manifest, "model.unknown-status")

	manifest = validManifest()
	manifest.Status = &StatusConfig{Enabled: true}
	manifest.BlockMeta = map[ID]BlockMeta{11: {Status: "ghost"}}
	expectCode(t, manifest, "model.unknown-status")

	manifest = validManifest()
	manifest.Status = &StatusConfig{Enabled: false}
	manifest.Root.Children[0].Status = "ghost"
	if err := manifest.Validate(); err != nil {
		t.Fatalf("a disabled system must ignore assignments: %v", err.Issues)
	}
}

func TestBadgeRules(t *testing.T) {
	manifest := validManifest()
	manifest.Badges = []Badge{{ID: "go"}, {ID: "license", Message: "MIT"}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid badges: %v", err.Issues)
	}

	manifest = validManifest()
	manifest.Badges = []Badge{{ID: "go"}, {ID: "go"}}
	expectCode(t, manifest, "model.duplicate-badge")

	manifest = validManifest()
	manifest.Badges = []Badge{{ID: "bad id"}}
	expectCode(t, manifest, "model.invalid-badge")

	manifest = validManifest()
	manifest.Badges = []Badge{{ID: ""}}
	expectCode(t, manifest, "model.invalid-badge")
}

func TestLayoutRules(t *testing.T) {
	manifest := validManifest()
	manifest.BlockMeta = map[ID]BlockMeta{11: {Type: "RepoLayout", Locked: true}}
	manifest.Layout = &LayoutConfig{
		Root: ".", Depth: 2, Notes: map[string]string{"internal": "The Go sources."},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid layout: %v", err.Issues)
	}

	manifest = validManifest()
	manifest.BlockMeta = map[ID]BlockMeta{11: {Type: "RepoLayout", Locked: true}}
	expectCode(t, manifest, "model.layout-missing-config")

	manifest = validManifest()
	manifest.BlockMeta = map[ID]BlockMeta{11: {Type: "RepoLayout", Locked: true}}
	manifest.Layout = &LayoutConfig{Root: "../escape", Depth: 2}
	expectCode(t, manifest, "model.invalid-layout")

	manifest = validManifest()
	manifest.BlockMeta = map[ID]BlockMeta{11: {Type: "RepoLayout", Locked: true}}
	manifest.Layout = &LayoutConfig{
		Root: ".", Depth: 2, Notes: map[string]string{"internal": "line one\nline two"},
	}
	expectCode(t, manifest, "model.invalid-layout")

	manifest = validManifest()
	manifest.BlockMeta = map[ID]BlockMeta{
		11: {Type: "RepoLayout", Locked: true},
		12: {Type: "RepoLayout", Locked: true},
	}
	manifest.Layout = &LayoutConfig{Root: ".", Depth: 2}
	expectCode(t, manifest, "model.layout-count")
}

func TestBlockMeta(t *testing.T) {
	manifest := validManifest()
	manifest.SetMeta(11, BlockMeta{Type: "UILayout", Locked: true})
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid metadata: %v", err.Issues)
	}
	if !manifest.IsLocked(11) || manifest.BlockType(11) != "UILayout" {
		t.Fatal("metadata accessors broken")
	}
	manifest.SetMeta(13, BlockMeta{Type: "UILayout", Locked: true})
	if err := manifest.Validate(); err != nil {
		t.Fatalf("a type may be shared by several blocks: %v", err.Issues)
	}
	if manifest.IsLocked(12) {
		t.Fatal("block 12 must not be locked")
	}

	manifest = validManifest()
	manifest.SetMeta(404, BlockMeta{Type: "Note"})
	expectCode(t, manifest, "model.meta-orphan")

	manifest = validManifest()
	manifest.SetMeta(0, BlockMeta{Type: "Note"})
	expectCode(t, manifest, "model.zero-id")

	manifest = validManifest()
	manifest.SetMeta(11, BlockMeta{Type: "UI Layout"})
	expectCode(t, manifest, "model.invalid-type")

	manifest = validManifest()
	manifest.SetMeta(11, BlockMeta{})
	if manifest.BlockMeta != nil {
		t.Fatal("setting the zero meta must remove the entry")
	}
}

func TestSettingsRules(t *testing.T) {
	manifest := validManifest()
	manifest.Settings.Output = "stripe"
	expectCode(t, manifest, "model.invalid-setting")

	manifest = validManifest()
	manifest.Settings.IndexDepth = 0
	expectCode(t, manifest, "model.invalid-setting")
}

func TestBookIDRules(t *testing.T) {
	manifest := validManifest()
	manifest.ID = "with/slash"
	expectCode(t, manifest, "model.invalid-book-id")

	for _, id := range []BookID{"", ".", "..", "with space", "with\\backslash"} {
		if id.Valid() {
			t.Fatalf("%q must be invalid", string(id))
		}
	}
	for _, id := range []BookID{"clabok", "ahadv", "book-1"} {
		if !id.Valid() {
			t.Fatalf("%q must be valid", string(id))
		}
	}
}

func TestPlacedBlocksOrder(t *testing.T) {
	manifest := validManifest()
	placed := manifest.PlacedBlocks()
	want := []ID{10, 11, 12, 13, 14, 15}
	if !reflect.DeepEqual(placed, want) {
		t.Fatalf("placed %v, want %v", placed, want)
	}
}

func TestValidationMessagesAreStable(t *testing.T) {
	manifest := validManifest()
	manifest.Root.Children[0].Children[0].Children = []*Node{{ID: 99, Kind: KindSubchapter, Title: "X"}}
	err := manifest.Validate()
	if err == nil || !strings.Contains(err.Issues[0].String(), "model.depth-exceeded") {
		t.Fatalf("unexpected issues %v", err)
	}
}

func TestGeneratedTreesValidate(t *testing.T) {
	for chapters := 0; chapters <= 3; chapters++ {
		for subs := 0; subs <= 2; subs++ {
			manifest := &Manifest{
				Version:  Version,
				ID:       "generated",
				Root:     &Node{ID: 1, Kind: KindTitle, Title: "Generated"},
				Settings: DefaultSettings(),
			}
			next := ID(100)
			for chapter := 0; chapter < chapters; chapter++ {
				chapterNode := &Node{ID: next, Kind: KindChapter, Title: "Chapter"}
				next++
				for sub := 0; sub < subs; sub++ {
					subNode := &Node{ID: next, Kind: KindSubchapter, Title: "Sub"}
					next++
					for block := 0; block < 2; block++ {
						subNode.Blocks = append(subNode.Blocks, next)
						next++
					}
					chapterNode.Children = append(chapterNode.Children, subNode)
				}
				manifest.Root.Children = append(manifest.Root.Children, chapterNode)
			}
			if err := manifest.Validate(); err != nil {
				t.Fatalf("%d/%d: %v", chapters, subs, err.Issues)
			}
		}
	}
}
