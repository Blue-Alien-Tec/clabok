package model

import "testing"

func bookWithBodies() *Book {
	book := NewBook(validManifest())
	for _, id := range []ID{10, 11, 12, 13, 14, 15} {
		book.SetBody(id, "body "+id.String())
	}
	return book
}

func TestBookWithBodiesIsValid(t *testing.T) {
	if err := bookWithBodies().Validate(); err != nil {
		t.Fatalf("unexpected issues: %v", err.Issues)
	}
}

func TestBookMissingBody(t *testing.T) {
	book := bookWithBodies()
	delete(book.Bodies, 12)
	err := book.Validate()
	if err == nil || !hasCode(err, "model.missing-body") {
		t.Fatalf("expected a missing body, got %v", err)
	}
}

func TestBookOrphanBody(t *testing.T) {
	book := bookWithBodies()
	book.SetBody(99, "orphan")
	err := book.Validate()
	if err == nil || !hasCode(err, "model.orphan-body") {
		t.Fatalf("expected an orphan body, got %v", err)
	}
}

func TestBookZeroBodyID(t *testing.T) {
	book := bookWithBodies()
	book.SetBody(0, "zero")
	err := book.Validate()
	if err == nil || !hasCode(err, "model.zero-id") {
		t.Fatalf("expected a zero id issue, got %v", err)
	}
}

func TestBookBlockAndClone(t *testing.T) {
	book := bookWithBodies()
	block, ok := book.Block(11)
	if !ok || block.Body != "body 11" {
		t.Fatalf("unexpected block %+v", block)
	}
	if _, ok := book.Block(404); ok {
		t.Fatal("found a missing block")
	}
	clone := book.Clone()
	clone.SetBody(11, "changed")
	if body, _ := book.Body(11); body != "body 11" {
		t.Fatal("clone shares the body map")
	}
}

func TestBookProseBody(t *testing.T) {
	book := bookWithBodies()
	book.Manifest.Root.Children[0].ProseBlock = 16
	book.Manifest.Regions = append(book.Manifest.Regions, &Region{
		ID: 23, Kind: RegionProse, Target: Target{Kind: TargetNode, ID: 2}, Blocks: []ID{16},
	})
	book.SetBody(16, "A short summary\n")
	if err := book.Validate(); err != nil {
		t.Fatalf("unexpected issues: %v", err.Issues)
	}
	book.SetBody(16, "line one\nline two\n")
	err := book.Validate()
	if err == nil || !hasCode(err, "model.invalid-prose") {
		t.Fatalf("expected invalid prose, got %v", err)
	}
}

func TestBookWithNilManifest(t *testing.T) {
	book := &Book{}
	err := book.Validate()
	if err == nil || !hasCode(err, "model.missing-manifest") {
		t.Fatalf("expected a missing manifest, got %v", err)
	}
}
