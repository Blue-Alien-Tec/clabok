package source

import (
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Book is one read-only book: manifest, bodies and asciiui specifications, plus the placement
// indexes that map a block to its node or region.
type Book struct {
	ID           model.BookID
	Paths        store.BookPaths
	Manifest     *model.Manifest
	ManifestData []byte
	Bodies       map[model.ID]string
	Layouts      map[model.ID][]byte
	// Open reports whether the extracted state was the source. A closed book reads its container.
	Open bool

	blockNode   map[model.ID]model.ID
	blockRegion map[model.ID]model.ID
	region      map[model.ID]*model.Region
}

func newBook(paths store.BookPaths, manifestData []byte, manifest *model.Manifest, bodies map[model.ID]string, layouts map[model.ID][]byte, open bool) *Book {
	book := &Book{
		ID:           paths.ID,
		Paths:        paths,
		Manifest:     manifest,
		ManifestData: manifestData,
		Bodies:       bodies,
		Layouts:      layouts,
		Open:         open,
	}
	book.index()
	return book
}

// index builds the block placement maps once per load.
func (book *Book) index() {
	book.blockNode = map[model.ID]model.ID{}
	book.blockRegion = map[model.ID]model.ID{}
	book.region = map[model.ID]*model.Region{}
	if book.Manifest == nil {
		return
	}
	book.Manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		for _, id := range node.Blocks {
			book.blockNode[id] = node.ID
		}
		return true
	})
	for _, region := range book.Manifest.Regions {
		if region == nil {
			continue
		}
		book.region[region.ID] = region
		for _, id := range region.Blocks {
			book.blockRegion[id] = region.ID
		}
	}
}

// Body returns the body of a block and reports whether it exists.
func (book *Book) Body(id model.ID) (string, bool) {
	body, exists := book.Bodies[id]
	return body, exists
}

// Layout returns the asciiui specification of a block and reports whether it exists.
func (book *Book) Layout(id model.ID) ([]byte, bool) {
	spec, exists := book.Layouts[id]
	return spec, exists
}

// NodeForBlock returns the node that places the block, if any.
func (book *Book) NodeForBlock(id model.ID) (*model.Node, bool) {
	nodeID, exists := book.blockNode[id]
	if !exists {
		return nil, false
	}
	return book.Manifest.FindNode(nodeID)
}

// RegionForBlock returns the region that owns the block, if any.
func (book *Book) RegionForBlock(id model.ID) (*model.Region, bool) {
	regionID, exists := book.blockRegion[id]
	if !exists {
		return nil, false
	}
	region, exists := book.region[regionID]
	return region, exists
}

// MissingBodies returns the placed blocks without a body, in placement order.
func (book *Book) MissingBodies() []model.ID {
	missing := make([]model.ID, 0)
	if book.Manifest == nil {
		return missing
	}
	for _, id := range book.Manifest.PlacedBlocks() {
		if _, exists := book.Bodies[id]; !exists {
			missing = append(missing, id)
		}
	}
	return missing
}

// Members returns the drift-hash input: the manifest and the bodies. Layouts are excluded so an
// open and a closed book hash identically.
func (book *Book) Members() map[string][]byte {
	members := map[string][]byte{lifecycle.ManifestMember: book.ManifestData}
	for id, body := range book.Bodies {
		members["blocks/"+id.String()+".md"] = []byte(body)
	}
	return members
}
