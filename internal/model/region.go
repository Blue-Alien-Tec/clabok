package model

// RegionKind is the purpose of an ordered block collection that is not part of the chapter flow.
type RegionKind string

const (
	// RegionFooter is the book footer; exactly one slot per book.
	RegionFooter RegionKind = "footer"
	// RegionNotes holds notes attached to a node or a block.
	RegionNotes RegionKind = "notes"
	// RegionProse holds the block with the one-line index prose of a node.
	RegionProse RegionKind = "prose"
	// RegionStatus holds the legend meaning of one configured status level.
	RegionStatus RegionKind = "status"
)

// Valid reports whether the region kind is known.
func (kind RegionKind) Valid() bool {
	return kind == RegionFooter || kind == RegionNotes || kind == RegionProse ||
		kind == RegionStatus
}

// TargetKind names what a region collection or a reference points at.
type TargetKind string

const (
	// TargetBook targets the book itself.
	TargetBook TargetKind = "book"
	// TargetNode targets a structural node.
	TargetNode TargetKind = "node"
	// TargetBlock targets a block.
	TargetBlock TargetKind = "block"
)

// Valid reports whether the target kind is known.
func (kind TargetKind) Valid() bool {
	switch kind {
	case TargetBook, TargetNode, TargetBlock:
		return true
	}
	return false
}

// Target is a region target. The ID is empty for the book.
type Target struct {
	Kind TargetKind `json:"kind"`
	ID   ID         `json:"id,omitempty"`
}

// Region is an ordered block collection with a stable ID and a target. Its order is the render
// order; there is no separate region source.
type Region struct {
	ID     ID         `json:"id"`
	Kind   RegionKind `json:"kind"`
	Target Target     `json:"target"`
	Blocks []ID       `json:"blocks,omitempty"`
}

// Clone returns a deep copy.
func (region *Region) Clone() *Region {
	if region == nil {
		return nil
	}
	clone := &Region{ID: region.ID, Kind: region.Kind, Target: region.Target}
	if region.Blocks != nil {
		clone.Blocks = append([]ID(nil), region.Blocks...)
	}
	return clone
}
