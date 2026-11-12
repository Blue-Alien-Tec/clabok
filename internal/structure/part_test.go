package structure

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// partManifest builds an in-memory book with the given number of chapters, numbered from 10 so the
// IDs stay recognisable in a failure.
func partManifest(chapters int) *model.Manifest {
	manifest := &model.Manifest{
		Version: model.Version,
		ID:      "fixture",
		Root:    &model.Node{ID: 1, Kind: model.KindTitle, Title: "Fixture"},
	}
	for index := 0; index < chapters; index++ {
		manifest.Root.Children = append(manifest.Root.Children, &model.Node{
			ID:    model.ID(10 + index*10),
			Kind:  model.KindChapter,
			Title: fmt.Sprintf("Chapter %d", index+1),
		})
	}
	return manifest
}

func requirePartCode(t *testing.T, err error, code string) {
	t.Helper()
	structureErr, ok := err.(*Error)
	if !ok || structureErr.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func TestPartTextNormalization(t *testing.T) {
	scene := newScene(t)
	receipt, err := scene.service.SetPart("fixture", 10, "  Teil   II  ", false)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Part != "Teil II" {
		t.Fatalf("stored %q", receipt.Part)
	}
	if !receipt.Changed || receipt.Kind != "part" {
		t.Fatalf("receipt %+v", receipt)
	}
	slotted, err := scene.service.SetPart("fixture", 20, "3", false)
	if err != nil {
		t.Fatal(err)
	}
	if slotted.Part != "Part III" {
		t.Fatalf("slot did not expand: %q", slotted.Part)
	}
	cleared, err := scene.service.SetPart("fixture", 10, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Part != "" || !cleared.Changed {
		t.Fatalf("clear receipt %+v", cleared)
	}
}

func TestPartSlotOutOfRange(t *testing.T) {
	for _, raw := range []string{"11", "0", "999"} {
		if _, err := normalizePart(raw); err == nil {
			t.Fatalf("%q must be rejected", raw)
		} else {
			requirePartCode(t, err, "part.out-of-range")
		}
	}
}

func TestSetPartOnChapter(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.service.SetPart("fixture", 10, "2", false); err != nil {
		t.Fatal(err)
	}
	manifest := scene.manifest(t)
	if node, ok := manifest.FindNode(10); !ok || node.Part != "Part II" {
		t.Fatalf("chapter part %+v", node)
	}
	if issue := manifest.Validate(); issue != nil {
		t.Fatalf("the written manifest is invalid: %v", issue)
	}
}

func TestSetPartOnSubchapterRejected(t *testing.T) {
	scene := newScene(t)
	_, err := scene.service.SetPart("fixture", 12, "Part I", false)
	requirePartCode(t, err, "structure.part-on-node")
}

func TestPartLimit(t *testing.T) {
	manifest := partManifest(model.PartLimit + 1)
	for index := 0; index < model.PartLimit; index++ {
		target := manifest.Root.Children[index].ID
		if _, err := applyPart(manifest, Step{Op: "part", Target: target,
			Value: strconv.Itoa(index + 1)}); err != nil {
			t.Fatalf("slot %d: %v", index+1, err)
		}
	}
	if len(manifest.Parts()) != model.PartLimit {
		t.Fatalf("parts %d", len(manifest.Parts()))
	}
	eleventh := manifest.Root.Children[model.PartLimit].ID
	_, err := applyPart(manifest, Step{Op: "part", Target: eleventh, Value: "Part XI"})
	requirePartCode(t, err, "part.limit")
	// A group the book already carries never grows the count.
	if _, err := applyPart(manifest, Step{Op: "part", Target: eleventh, Value: "Part I"}); err != nil {
		t.Fatalf("an existing group must pass: %v", err)
	}
}

func TestPartLabelRewritesGroup(t *testing.T) {
	manifest := partManifest(3)
	link := &model.Node{ID: 90, Kind: model.KindLink, Title: "Elsewhere",
		Ref: &model.Ref{Book: "other"}}
	manifest.Root.Children = append(manifest.Root.Children, link)
	for _, node := range manifest.Root.Children {
		node.Part = "Part I"
	}
	result, err := applyPartLabel(manifest, Step{Op: "part-label", Key: "Part I",
		Value: "Teil I — Einstieg"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.receipt.Order) != 4 || !result.receipt.Changed {
		t.Fatalf("receipt %+v", result.receipt)
	}
	for _, node := range manifest.Root.Children {
		if node.Part != "Teil I — Einstieg" {
			t.Fatalf("node %s kept %q", node.ID, node.Part)
		}
	}
	same, err := applyPartLabel(manifest, Step{Op: "part-label", Key: "Teil I — Einstieg",
		Value: "Teil I — Einstieg"})
	if err != nil {
		t.Fatal(err)
	}
	if same.receipt.Changed {
		t.Fatal("an unchanged label must report unchanged")
	}
	_, unknown := applyPartLabel(manifest, Step{Op: "part-label", Key: "Nope", Value: "Part II"})
	requirePartCode(t, unknown, "part.unknown-group")
	_, empty := applyPartLabel(manifest, Step{Op: "part-label", Key: "Teil I — Einstieg",
		Value: ""})
	requirePartCode(t, empty, "part.empty-label")
}

func TestAddWithPart(t *testing.T) {
	scene := newScene(t)
	receipt, err := scene.service.AddTyped("fixture", "Three", "", "2", Append(0), false)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Part != "Part II" {
		t.Fatalf("receipt part %q", receipt.Part)
	}
	node, ok := scene.manifest(t).FindNode(receipt.Target)
	if !ok || node.Part != "Part II" {
		t.Fatalf("added node %+v", node)
	}
	if _, err := scene.service.AddTyped("fixture", "", "", "1", Append(12), false); err == nil {
		t.Fatal("a block must not carry a part")
	} else {
		requirePartCode(t, err, "structure.part-on-node")
	}
	if _, err := scene.service.AddTyped("fixture", "Sub", "", "1", Append(10), false); err == nil {
		t.Fatal("a subchapter must not carry a part")
	} else {
		requirePartCode(t, err, "structure.part-on-node")
	}
}

func TestRenamePartClear(t *testing.T) {
	scene := newScene(t)
	part := "2"
	if _, err := scene.service.Rename("fixture", 10, "One", &part, false); err != nil {
		t.Fatal(err)
	}
	if node, _ := scene.manifest(t).FindNode(10); node.Part != "Part II" {
		t.Fatalf("rename did not set the part: %+v", node)
	}
	untouched, err := scene.service.Rename("fixture", 10, "Eins", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Part != "" {
		t.Fatalf("a nil part must not report a group: %+v", untouched)
	}
	if node, _ := scene.manifest(t).FindNode(10); node.Part != "Part II" {
		t.Fatalf("a nil part changed the group: %+v", node)
	}
	empty := ""
	if _, err := scene.service.Rename("fixture", 10, "Eins", &empty, false); err != nil {
		t.Fatal(err)
	}
	if node, _ := scene.manifest(t).FindNode(10); node.Part != "" {
		t.Fatalf("the clear did not take: %+v", node)
	}
}

func TestPartSummary(t *testing.T) {
	scene := newScene(t)
	if _, err := scene.service.SetPart("fixture", 10, "1", false); err != nil {
		t.Fatal(err)
	}
	if _, err := scene.service.SetPart("fixture", 20, "Teil II — Die Galaxie", false); err != nil {
		t.Fatal(err)
	}
	summary, err := scene.service.PartSummary("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Limit != model.PartLimit || len(summary.Parts) != 2 {
		t.Fatalf("summary %+v", summary)
	}
	if summary.Parts[0].Slot != 1 || summary.Parts[0].Text != "Part I" || summary.Parts[0].Chapters != 1 {
		t.Fatalf("first group %+v", summary.Parts[0])
	}
	if summary.Parts[1].Slot != 2 || summary.Parts[1].Text != "Teil II — Die Galaxie" {
		t.Fatalf("second group %+v", summary.Parts[1])
	}
}

func TestPartDryRunWritesNothing(t *testing.T) {
	scene := newScene(t)
	before := scene.manifestBytes(t)
	receipt, err := scene.service.SetPart("fixture", 10, "Part II", true)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Part != "Part II" || !receipt.Changed {
		t.Fatalf("dry-run receipt %+v", receipt)
	}
	after := scene.manifestBytes(t)
	if string(before) != string(after) {
		t.Fatal("a dry run wrote the manifest")
	}
}
