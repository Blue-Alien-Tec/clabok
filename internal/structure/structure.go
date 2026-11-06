// Package structure implements the uniform structure commands and the book settings commands.
//
// Structure is data: adding, renaming, moving and removing units edits the manifest; a block move
// never touches its body. Every mutation runs under the book's exclusive lock, snapshots an undo
// package first and validates the result before anything is written.
package structure

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Error is a structure failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *Error) ErrorCode() string { return err.Code }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Placement expresses where a unit lands.
type Placement struct {
	// Parent is the destination node. Zero means the book root.
	Parent model.ID
	// Index is the position in the destination list. Negative appends.
	Index int
	// Before and After place the unit relative to an existing sibling.
	Before model.ID
	After  model.ID
}

// Append returns a placement that appends to a node (zero means the root).
func Append(parent model.ID) Placement {
	return Placement{Parent: parent, Index: -1}
}

// Receipt reports one mutation.
type Receipt struct {
	Book    model.BookID
	Target  model.ID
	Block   model.ID
	Kind    string
	Parent  model.ID
	Index   int
	Order   []model.ID
	Changed bool
}

// Step is one declarative mutation.
type Step struct {
	Op         string
	Title      string
	Type       string
	TargetKind model.TargetKind
	Target     model.ID
	Placement  Placement
	Recursive  bool
	Key        string
	Value      string
	Ref        *model.Ref
	Badge      *model.Badge
}

// mutation is the pure result of one step: the receipt plus the file effects the commit applies.
type mutation struct {
	receipt Receipt
	created []model.ID
	removed []model.ID
	bodies  map[model.ID]string
	layouts map[model.ID][]byte
}

// Service applies structure mutations to open books.
type Service struct {
	Lifecycle *lifecycle.Lifecycle
	Allocator model.Allocator
}

// New returns a structure service. A nil allocator falls back to the cooperative random allocator.
func New(life *lifecycle.Lifecycle, allocator model.Allocator) *Service {
	if allocator == nil {
		allocator = model.NewRandomAllocator()
	}
	return &Service{Lifecycle: life, Allocator: allocator}
}

// Execute applies a batch atomically: one lock, one validation, one undo package and one manifest
// write. A failed step leaves the book untouched.
func (service *Service) Execute(book model.BookID, steps []Step, dryRun bool) ([]Receipt, error) {
	return service.run(book, "apply", steps, dryRun)
}

// Add creates a chapter, subchapter or block. The kind follows the destination parent.
func (service *Service) Add(book model.BookID, title string, placement Placement, dryRun bool) (Receipt, error) {
	return service.AddTyped(book, title, "", placement, dryRun)
}

// AddTyped creates a unit and sets the block type when it is a block.
func (service *Service) AddTyped(book model.BookID, title, blockType string, placement Placement, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.add",
		[]Step{{Op: "add", Title: title, Type: blockType, Placement: placement}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// AddLink adds a link entry at the book root. The target stays an ID; local and cross-book links
// are resolved at render time.
func (service *Service) AddLink(book model.BookID, title string, ref *model.Ref, placement Placement, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.add-link",
		[]Step{{Op: "add-link", Title: title, Ref: ref, Placement: placement}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// SetProse sets or clears the one-line index prose of a node. The text lives in a block that the
// node references through ProseBlock; an empty text removes the block.
func (service *Service) SetProse(book model.BookID, node model.ID, prose string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.prose",
		[]Step{{Op: "prose", Target: node, Value: prose}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// SetFooter replaces the book footer with one block. An empty body clears the footer.
func (service *Service) SetFooter(book model.BookID, body string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.footer",
		[]Step{{Op: "footer", Value: body}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// ClearFooter removes the footer region and its block.
func (service *Service) ClearFooter(book model.BookID, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.footer-clear",
		[]Step{{Op: "footer-clear"}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// AddNote appends a note block to the target's notes region.
func (service *Service) AddNote(book model.BookID, targetKind model.TargetKind, target model.ID, body string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.note",
		[]Step{{Op: "note", TargetKind: targetKind, Target: target, Value: body}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// RemoveNote removes the whole notes region of a target.
func (service *Service) RemoveNote(book model.BookID, targetKind model.TargetKind, target model.ID, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.note-remove",
		[]Step{{Op: "note-remove", TargetKind: targetKind, Target: target}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// AddLockedBlock creates a block with a type and locks it. It is the primitive for tool-owned
// content such as the ASCII UI layout anchor.
func (service *Service) AddLockedBlock(book model.BookID, parent model.ID, blockType string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.add-locked",
		[]Step{{Op: "add-locked", Type: blockType, Placement: Append(parent)}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// AddLayoutBlock creates a locked UILayout block and stores its ASCII UI spec in the same atomic
// step, so a layout anchor never exists without its drawing.
func (service *Service) AddLayoutBlock(book model.BookID, parent model.ID, spec string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.add-layout",
		[]Step{{Op: "add-layout", Type: "UILayout", Placement: Append(parent), Value: spec}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// SetLayout replaces the ASCII UI spec of an existing locked UILayout block.
func (service *Service) SetLayout(book model.BookID, block model.ID, spec string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.layout",
		[]Step{{Op: "layout", Target: block, Value: spec}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// RemoveLayout removes a locked UILayout block together with its spec.
func (service *Service) RemoveLayout(book model.BookID, block model.ID, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.layout-remove",
		[]Step{{Op: "layout-remove", Target: block}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// SetStatus assigns an editorial state to a node or a block. An empty state clears it.
func (service *Service) SetStatus(book model.BookID, target model.ID, state string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.status",
		[]Step{{Op: "status", Target: target, Value: state}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// SetStatusEnabled switches the status system of a book on or off. Assignments stay; a disabled
// system renders nothing and ignores them.
func (service *Service) SetStatusEnabled(book model.BookID, enabled, dryRun bool) (Receipt, error) {
	value := "off"
	if enabled {
		value = "on"
	}
	receipts, err := service.run(book, "struct.status-config",
		[]Step{{Op: "status-config", Value: value}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// StatusAssignment is one assigned state.
type StatusAssignment struct {
	ID     model.ID
	Target string
	Status string
}

// StatusSummary is the read-only view of the status system.
type StatusSummary struct {
	Enabled     bool
	Assignments []StatusAssignment
}

// StatusSummary reads the current status configuration. The book must be open.
func (service *Service) StatusSummary(book model.BookID) (StatusSummary, error) {
	_, manifest, err := service.load(book)
	if err != nil {
		return StatusSummary{}, err
	}
	summary := StatusSummary{Enabled: manifest.StatusEnabled()}
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		if node.Status != "" {
			summary.Assignments = append(summary.Assignments, StatusAssignment{
				ID: node.ID, Target: "node", Status: node.Status,
			})
		}
		return true
	})
	blocks := make([]model.ID, 0, len(manifest.BlockMeta))
	for id := range manifest.BlockMeta {
		blocks = append(blocks, id)
	}
	sort.Slice(blocks, func(left, right int) bool { return blocks[left] < blocks[right] })
	for _, id := range blocks {
		if meta := manifest.BlockMeta[id]; meta.Status != "" {
			summary.Assignments = append(summary.Assignments, StatusAssignment{
				ID: id, Target: "block", Status: meta.Status,
			})
		}
	}
	return summary, nil
}

// SetBadge adds or replaces a badge of the library README. The badge id is the key.
func (service *Service) SetBadge(book model.BookID, badge model.Badge, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.badge",
		[]Step{{Op: "badge", Badge: &badge}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// RemoveBadge removes a declared badge by its id.
func (service *Service) RemoveBadge(book model.BookID, id string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.badge-remove",
		[]Step{{Op: "badge-remove", Value: id}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// Badges reads the declared badges. The book must be open.
func (service *Service) Badges(book model.BookID) ([]model.Badge, error) {
	_, manifest, err := service.load(book)
	if err != nil {
		return nil, err
	}
	return append([]model.Badge(nil), manifest.Badges...), nil
}

// SetBlockType sets or clears the optional type of a block. A locked block rejects the change.
func (service *Service) SetBlockType(book model.BookID, block model.ID, value string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "block.type",
		[]Step{{Op: "type", Target: block, Value: value}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// AddBlock appends a new block to a node that can hold blocks. Unlike Add, the parent kind does
// not decide the unit: a chapter can hold blocks next to its subchapters.
func (service *Service) AddBlock(book model.BookID, parent model.ID, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.add-block",
		[]Step{{Op: "add-block", Placement: Append(parent)}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// Rename changes the title of a structural node. Blocks have no title.
func (service *Service) Rename(book model.BookID, target model.ID, title string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.rename", []Step{{Op: "rename", Target: target, Title: title}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// Move reassigns the placement of a node or a block. A block keeps its body.
func (service *Service) Move(book model.BookID, target model.ID, placement Placement, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.move", []Step{{Op: "move", Target: target, Placement: placement}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// Remove deletes a block or a node. A non-empty node requires recursive.
func (service *Service) Remove(book model.BookID, target model.ID, recursive, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.remove", []Step{{Op: "remove", Target: target, Recursive: recursive}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// SetSetting changes one book setting.
func (service *Service) SetSetting(book model.BookID, key, value string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "book.setting", []Step{{Op: "setting", Key: key, Value: value}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

func (service *Service) run(book model.BookID, reason string, steps []Step, dryRun bool) ([]Receipt, error) {
	release, err := service.Lifecycle.LockBookExclusive(book)
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	paths, manifest, err := service.load(book)
	if err != nil {
		return nil, err
	}
	allocator := service.Allocator
	if dryRun {
		// A dry run previews the IDs without touching the library ledger.
		if preview, ok := allocator.(interface{ Peek() (model.ID, error) }); ok {
			start, peekErr := preview.Peek()
			if peekErr != nil {
				return nil, fail("structure.alloc-failed", "%v", peekErr)
			}
			allocator = model.NewSequenceAllocator(start)
		}
	}
	ids := newIDs(manifest, allocator)
	receipts := make([]Receipt, 0, len(steps))
	var created, removed []model.ID
	bodies := make(map[model.ID]string)
	layouts := make(map[model.ID][]byte)
	for _, step := range steps {
		result, stepErr := step.apply(manifest, ids)
		if stepErr != nil {
			return nil, stepErr
		}
		receipts = append(receipts, result.receipt)
		created = append(created, result.created...)
		removed = append(removed, result.removed...)
		for id, body := range result.bodies {
			bodies[id] = body
		}
		for id, spec := range result.layouts {
			layouts[id] = spec
		}
	}
	if err := validate(manifest); err != nil {
		return nil, err
	}
	if dryRun {
		return receipts, nil
	}
	if _, err := service.Lifecycle.Snapshot(book, reason); err != nil {
		return nil, err
	}
	for _, id := range removed {
		if err := os.Remove(bodyPath(paths, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fail("structure.write-failed", "%v", err)
		}
		if err := os.Remove(layoutPath(paths, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fail("structure.write-failed", "%v", err)
		}
	}
	if err := service.save(paths, manifest); err != nil {
		return nil, err
	}
	for _, id := range created {
		if _, exists := bodies[id]; !exists {
			bodies[id] = ""
		}
	}
	if len(bodies) > 0 {
		if err := os.MkdirAll(paths.BlocksDir, 0o755); err != nil {
			return nil, fail("structure.write-failed", "%v", err)
		}
		writeIDs := make([]model.ID, 0, len(bodies))
		for id := range bodies {
			writeIDs = append(writeIDs, id)
		}
		sort.Slice(writeIDs, func(left, right int) bool { return writeIDs[left] < writeIDs[right] })
		for _, id := range writeIDs {
			if err := os.WriteFile(bodyPath(paths, id), []byte(bodies[id]), 0o644); err != nil {
				return nil, fail("structure.write-failed", "%v", err)
			}
		}
	}
	if len(layouts) > 0 {
		if err := os.MkdirAll(filepath.Join(paths.LayoutsDir, "asciiui"), 0o755); err != nil {
			return nil, fail("structure.write-failed", "%v", err)
		}
		layoutIDs := make([]model.ID, 0, len(layouts))
		for id := range layouts {
			layoutIDs = append(layoutIDs, id)
		}
		sort.Slice(layoutIDs, func(left, right int) bool { return layoutIDs[left] < layoutIDs[right] })
		for _, id := range layoutIDs {
			if err := os.WriteFile(layoutPath(paths, id), layouts[id], 0o644); err != nil {
				return nil, fail("structure.write-failed", "%v", err)
			}
		}
	}
	return receipts, nil
}

func (step Step) apply(manifest *model.Manifest, ids *bookIDs) (mutation, error) {
	switch step.Op {
	case "add":
		return applyAdd(manifest, ids, step)
	case "add-link":
		return applyAddLink(manifest, ids, step)
	case "add-block":
		return applyAddBlock(manifest, ids, step)
	case "type":
		return applySetType(manifest, step)
	case "add-locked":
		return applyAddLockedBlock(manifest, ids, step)
	case "add-layout":
		return applyAddLayout(manifest, ids, step)
	case "layout":
		return applySetLayout(manifest, step)
	case "layout-remove":
		return applyRemoveLayout(manifest, step)
	case "status":
		return applyStatus(manifest, step)
	case "status-config":
		return applyStatusConfig(manifest, step)
	case "badge":
		return applyBadge(manifest, step)
	case "badge-remove":
		return applyBadgeRemove(manifest, step)
	case "rename":
		return applyRename(manifest, step)
	case "move":
		return applyMove(manifest, step)
	case "remove":
		return applyRemove(manifest, step)
	case "setting":
		return applySetting(manifest, step)
	case "prose":
		return applyProse(manifest, ids, step)
	case "footer":
		return applyFooter(manifest, ids, step)
	case "footer-clear":
		return applyFooterClear(manifest, step)
	case "note":
		return applyNote(manifest, ids, step)
	case "note-remove":
		return applyNoteRemove(manifest, step)
	default:
		return mutation{}, fail("structure.unknown-op", "unknown step operation %q", step.Op)
	}
}

func applyAdd(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	cleanTitle := model.NormalizeTitle(step.Title)
	parent, index, unitBlock, kind, err := addDestination(manifest, step.Placement)
	if err != nil {
		return mutation{}, err
	}
	if !unitBlock && strings.TrimSpace(cleanTitle) == "" {
		return mutation{}, fail("structure.empty-title", "the title must not be empty")
	}
	if step.Type != "" {
		if !unitBlock {
			return mutation{}, fail("structure.type-on-node", "only a block carries a type")
		}
		if message := model.ValidateTypeToken(step.Type); message != "" {
			return mutation{}, fail("structure.invalid-type", "%s", message)
		}
	}
	id, err := ids.next()
	if err != nil {
		return mutation{}, err
	}
	receipt := Receipt{Book: manifest.ID, Target: id, Kind: kind, Parent: parent.ID, Index: index, Changed: true}
	result := mutation{receipt: receipt}
	if unitBlock {
		parent.Blocks = insertID(parent.Blocks, index, id)
		result.receipt.Order = append([]model.ID(nil), parent.Blocks...)
		result.created = []model.ID{id}
		if step.Type != "" {
			manifest.SetMeta(id, model.BlockMeta{Type: step.Type})
		}
	} else {
		node := &model.Node{ID: id, Kind: model.Kind(kind), Title: cleanTitle}
		parent.Children = insertNode(parent.Children, index, node)
		result.receipt.Order = childIDs(parent.Children)
	}
	return result, nil
}

func applyAddLink(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	cleanTitle := model.NormalizeTitle(step.Title)
	if strings.TrimSpace(cleanTitle) == "" {
		return mutation{}, fail("structure.empty-title", "the title must not be empty")
	}
	if step.Ref == nil || !step.Ref.Valid() {
		return mutation{}, fail("structure.invalid-ref", "a link needs a book, a node or a block")
	}
	if step.Ref.Book != "" && !step.Ref.Book.Valid() {
		return mutation{}, fail("structure.invalid-ref", "invalid target book %q", step.Ref.Book)
	}
	parent, index, unitBlock, _, err := addDestination(manifest, step.Placement)
	if err != nil {
		return mutation{}, err
	}
	if unitBlock || parent != manifest.Root {
		return mutation{}, fail("structure.parent-invalid", "a link belongs to the book root")
	}
	id, err := ids.next()
	if err != nil {
		return mutation{}, err
	}
	node := &model.Node{ID: id, Kind: model.KindLink, Title: cleanTitle, Ref: step.Ref}
	parent.Children = insertNode(parent.Children, index, node)
	return mutation{receipt: Receipt{
		Book: manifest.ID, Target: id, Kind: "link", Parent: parent.ID, Index: index,
		Order: childIDs(parent.Children), Changed: true,
	}}, nil
}

func applyProse(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	node, ok := manifest.FindNode(step.Target)
	if !ok {
		return mutation{}, fail("structure.unknown-target", "node %s does not exist", step.Target)
	}
	text := strings.TrimSpace(step.Value)
	if message := model.ValidateProse(text); message != "" {
		return mutation{}, fail("structure.invalid-prose", "%s", message)
	}
	target := model.Target{Kind: model.TargetNode, ID: node.ID}
	region := findRegion(manifest, model.RegionProse, target)
	if text == "" {
		if region == nil {
			changed := !node.ProseBlock.IsZero()
			node.ProseBlock = model.ZeroID
			return mutation{receipt: Receipt{
				Book: manifest.ID, Target: node.ID, Kind: "prose", Index: -1, Changed: changed,
			}}, nil
		}
		removed := append([]model.ID(nil), region.Blocks...)
		node.ProseBlock = model.ZeroID
		removeRegion(manifest, region)
		return mutation{
			receipt: Receipt{Book: manifest.ID, Target: node.ID, Kind: "prose", Index: -1, Changed: true},
			removed: removed,
		}, nil
	}
	var blockID model.ID
	var created, removed []model.ID
	if region == nil {
		regionID, err := ids.next()
		if err != nil {
			return mutation{}, err
		}
		blockID, err = ids.next()
		if err != nil {
			return mutation{}, err
		}
		region = &model.Region{ID: regionID, Kind: model.RegionProse, Target: target, Blocks: []model.ID{blockID}}
		manifest.Regions = append(manifest.Regions, region)
		created = []model.ID{blockID}
	} else {
		if len(region.Blocks) == 0 {
			nextID, err := ids.next()
			if err != nil {
				return mutation{}, err
			}
			blockID = nextID
			region.Blocks = []model.ID{blockID}
			created = []model.ID{blockID}
		} else {
			blockID = region.Blocks[0]
			removed = append(removed, region.Blocks[1:]...)
			region.Blocks = []model.ID{blockID}
		}
	}
	node.ProseBlock = blockID
	return mutation{
		receipt: Receipt{
			Book: manifest.ID, Target: node.ID, Block: blockID, Kind: "prose",
			Parent: region.ID, Index: 0, Order: []model.ID{blockID}, Changed: true,
		},
		created: created,
		removed: removed,
		bodies:  map[model.ID]string{blockID: text},
	}, nil
}

func applyFooter(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	body := strings.TrimSpace(step.Value)
	if body == "" {
		return applyFooterClear(manifest, step)
	}
	target := model.Target{Kind: model.TargetBook}
	region := findRegion(manifest, model.RegionFooter, target)
	var removed []model.ID
	if region == nil {
		regionID, err := ids.next()
		if err != nil {
			return mutation{}, err
		}
		region = &model.Region{ID: regionID, Kind: model.RegionFooter, Target: target}
		manifest.Regions = append(manifest.Regions, region)
	} else {
		removed = append(removed, region.Blocks...)
		region.Blocks = nil
	}
	blockID, err := ids.next()
	if err != nil {
		return mutation{}, err
	}
	region.Blocks = []model.ID{blockID}
	return mutation{
		receipt: Receipt{
			Book: manifest.ID, Target: blockID, Kind: "footer", Parent: region.ID, Index: 0,
			Order: append([]model.ID(nil), region.Blocks...), Changed: true,
		},
		created: []model.ID{blockID},
		removed: removed,
		bodies:  map[model.ID]string{blockID: body},
	}, nil
}

func applyFooterClear(manifest *model.Manifest, step Step) (mutation, error) {
	region := findRegion(manifest, model.RegionFooter, model.Target{Kind: model.TargetBook})
	if region == nil {
		return mutation{receipt: Receipt{Book: manifest.ID, Kind: "footer", Index: -1, Changed: false}}, nil
	}
	removed := append([]model.ID(nil), region.Blocks...)
	removeRegion(manifest, region)
	return mutation{
		receipt: Receipt{Book: manifest.ID, Target: region.ID, Kind: "footer", Index: -1, Changed: true},
		removed: removed,
	}, nil
}

func applyNote(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	body := strings.TrimSpace(step.Value)
	if body == "" {
		return mutation{}, fail("structure.empty-body", "a note needs a body")
	}
	if err := validateNoteTarget(manifest, step.TargetKind, step.Target); err != nil {
		return mutation{}, err
	}
	target := model.Target{Kind: step.TargetKind, ID: step.Target}
	region := findRegion(manifest, model.RegionNotes, target)
	if region == nil {
		regionID, err := ids.next()
		if err != nil {
			return mutation{}, err
		}
		region = &model.Region{ID: regionID, Kind: model.RegionNotes, Target: target}
		manifest.Regions = append(manifest.Regions, region)
	}
	blockID, err := ids.next()
	if err != nil {
		return mutation{}, err
	}
	region.Blocks = append(region.Blocks, blockID)
	return mutation{
		receipt: Receipt{
			Book: manifest.ID, Target: blockID, Kind: "note", Parent: region.ID,
			Index: len(region.Blocks) - 1, Order: append([]model.ID(nil), region.Blocks...), Changed: true,
		},
		created: []model.ID{blockID},
		bodies:  map[model.ID]string{blockID: body},
	}, nil
}

func applyNoteRemove(manifest *model.Manifest, step Step) (mutation, error) {
	target := model.Target{Kind: step.TargetKind, ID: step.Target}
	region := findRegion(manifest, model.RegionNotes, target)
	if region == nil {
		return mutation{}, fail("structure.unknown-target",
			"no notes for %s %s", step.TargetKind, step.Target)
	}
	removed := append([]model.ID(nil), region.Blocks...)
	removeRegion(manifest, region)
	return mutation{
		receipt: Receipt{Book: manifest.ID, Target: region.ID, Kind: "note", Index: -1, Changed: true},
		removed: removed,
	}, nil
}

func findRegion(manifest *model.Manifest, kind model.RegionKind, target model.Target) *model.Region {
	for _, region := range manifest.Regions {
		if region != nil && region.Kind == kind && region.Target == target {
			return region
		}
	}
	return nil
}

func removeRegion(manifest *model.Manifest, region *model.Region) {
	for index, candidate := range manifest.Regions {
		if candidate == region {
			manifest.Regions = append(manifest.Regions[:index], manifest.Regions[index+1:]...)
			return
		}
	}
}

func validateNoteTarget(manifest *model.Manifest, kind model.TargetKind, id model.ID) error {
	if id.IsZero() {
		return fail("structure.invalid-target", "the target id must not be zero")
	}
	switch kind {
	case model.TargetNode:
		if _, ok := manifest.FindNode(id); !ok {
			return fail("structure.unknown-target", "node %s does not exist", id)
		}
	case model.TargetBlock:
		for _, placed := range manifest.PlacedBlocks() {
			if placed == id {
				return nil
			}
		}
		return fail("structure.unknown-target", "block %s is not placed", id)
	default:
		return fail("structure.invalid-target", "notes target a node or a block, not %q", kind)
	}
	return nil
}

func applyAddLockedBlock(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	if step.Type == "" {
		return mutation{}, fail("structure.invalid-type", "a locked block needs a type")
	}
	if message := model.ValidateTypeToken(step.Type); message != "" {
		return mutation{}, fail("structure.invalid-type", "%s", message)
	}
	result, err := applyAddBlock(manifest, ids, step)
	if err != nil {
		return mutation{}, err
	}
	manifest.SetMeta(result.receipt.Target, model.BlockMeta{Type: step.Type, Locked: true})
	return result, nil
}

func applyAddLayout(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	result, err := applyAddLockedBlock(manifest, ids, step)
	if err != nil {
		return mutation{}, err
	}
	result.receipt.Block = result.receipt.Target
	result.layouts = map[model.ID][]byte{result.receipt.Target: []byte(step.Value)}
	return result, nil
}

func applySetLayout(manifest *model.Manifest, step Step) (mutation, error) {
	if err := requireLayoutBlock(manifest, step.Target); err != nil {
		return mutation{}, err
	}
	return mutation{
		receipt: Receipt{
			Book: manifest.ID, Target: step.Target, Block: step.Target, Kind: "layout",
			Index: -1, Changed: true,
		},
		layouts: map[model.ID][]byte{step.Target: []byte(step.Value)},
	}, nil
}

func applyRemoveLayout(manifest *model.Manifest, step Step) (mutation, error) {
	if err := requireLayoutBlock(manifest, step.Target); err != nil {
		return mutation{}, err
	}
	result, err := applyRemove(manifest, Step{Op: "remove", Target: step.Target})
	if err != nil {
		return mutation{}, err
	}
	result.receipt.Kind = "layout"
	result.receipt.Block = step.Target
	return result, nil
}

func applyStatus(manifest *model.Manifest, step Step) (mutation, error) {
	value := strings.TrimSpace(step.Value)
	if value != "" {
		if !manifest.StatusEnabled() {
			return mutation{}, fail("structure.status-disabled",
				"the book does not enable status")
		}
		if _, ok := model.FindStatus(value); !ok {
			return mutation{}, fail("structure.unknown-status", "unknown status %q", value)
		}
	}
	node, isNode := manifest.FindNode(step.Target)
	_, _, isBlock := blockParent(manifest, step.Target)
	if !isNode && !isBlock {
		return mutation{}, fail("structure.unknown-target", "%s does not exist", step.Target)
	}
	changed := false
	if isNode {
		changed = node.Status != value
		node.Status = value
	} else {
		meta := manifest.Meta(step.Target)
		changed = meta.Status != value
		meta.Status = value
		manifest.SetMeta(step.Target, meta)
	}
	return mutation{receipt: Receipt{
		Book: manifest.ID, Target: step.Target, Kind: "status", Index: -1, Changed: changed,
	}}, nil
}

func applyStatusConfig(manifest *model.Manifest, step Step) (mutation, error) {
	enabled := step.Value == "on"
	changed := manifest.StatusEnabled() != enabled
	if changed {
		if enabled {
			if manifest.Status == nil {
				manifest.Status = &model.StatusConfig{}
			}
			manifest.Status.Enabled = true
		} else {
			manifest.Status.Enabled = false
		}
	}
	return mutation{receipt: Receipt{
		Book: manifest.ID, Kind: "status-config", Index: -1, Changed: changed,
	}}, nil
}

func applyBadge(manifest *model.Manifest, step Step) (mutation, error) {
	if step.Badge == nil {
		return mutation{}, fail("structure.invalid-badge", "a badge is required")
	}
	badge := *step.Badge
	badge.ID = strings.TrimSpace(badge.ID)
	if badge.ID == "" {
		return mutation{}, fail("structure.invalid-badge", "a badge id is required")
	}
	if message := model.ValidateTypeToken(badge.ID); message != "" {
		return mutation{}, fail("structure.invalid-badge", "%s", message)
	}
	changed := true
	if index := badgeIndex(manifest, badge.ID); index >= 0 {
		changed = manifest.Badges[index] != badge
		manifest.Badges[index] = badge
	} else {
		manifest.Badges = append(manifest.Badges, badge)
	}
	return mutation{receipt: Receipt{
		Book: manifest.ID, Kind: "badge", Index: -1, Changed: changed,
	}}, nil
}

func applyBadgeRemove(manifest *model.Manifest, step Step) (mutation, error) {
	id := strings.TrimSpace(step.Value)
	index := badgeIndex(manifest, id)
	if index < 0 {
		return mutation{receipt: Receipt{Book: manifest.ID, Kind: "badge", Index: -1, Changed: false}}, nil
	}
	manifest.Badges = append(manifest.Badges[:index], manifest.Badges[index+1:]...)
	if len(manifest.Badges) == 0 {
		manifest.Badges = nil
	}
	return mutation{receipt: Receipt{Book: manifest.ID, Kind: "badge", Index: -1, Changed: true}}, nil
}

func badgeIndex(manifest *model.Manifest, id string) int {
	for index, badge := range manifest.Badges {
		if badge.ID == id {
			return index
		}
	}
	return -1
}

// requireLayoutBlock rejects a target that is not a placed, locked UILayout block.
func requireLayoutBlock(manifest *model.Manifest, id model.ID) error {
	if id.IsZero() {
		return fail("structure.invalid-target", "a block id is required")
	}
	if _, _, isBlock := blockParent(manifest, id); !isBlock {
		return fail("structure.unknown-target", "block %s does not exist", id)
	}
	meta := manifest.Meta(id)
	if !meta.Locked || meta.Type != "UILayout" {
		return fail("structure.not-layout", "block %s is not a locked UILayout block", id)
	}
	return nil
}

func applySetType(manifest *model.Manifest, step Step) (mutation, error) {
	if step.Target.IsZero() {
		return mutation{}, fail("structure.unknown-target", "a block id is required")
	}
	if _, _, isBlock := blockParent(manifest, step.Target); !isBlock {
		return mutation{}, fail("structure.unknown-target", "block %s does not exist", step.Target)
	}
	if manifest.IsLocked(step.Target) {
		return mutation{}, fail("structure.block-locked",
			"block %s is locked; its type cannot be changed", step.Target)
	}
	value := strings.TrimSpace(step.Value)
	if value != "" {
		if message := model.ValidateTypeToken(value); message != "" {
			return mutation{}, fail("structure.invalid-type", "%s", message)
		}
	}
	meta := manifest.Meta(step.Target)
	meta.Type = value
	manifest.SetMeta(step.Target, meta)
	return mutation{receipt: Receipt{
		Book: manifest.ID, Target: step.Target, Kind: "block", Index: -1, Changed: true,
	}}, nil
}

func applyAddBlock(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	if step.Placement.Parent.IsZero() {
		return mutation{}, fail("structure.parent-invalid", "a block needs a parent node")
	}
	parent, ok := manifest.FindNode(step.Placement.Parent)
	if !ok {
		if _, _, isBlock := blockParent(manifest, step.Placement.Parent); isBlock {
			return mutation{}, fail("structure.parent-invalid", "a block cannot hold units")
		}
		return mutation{}, fail("structure.unknown-target",
			"parent %s does not exist", step.Placement.Parent)
	}
	if parent.Kind == model.KindLink {
		return mutation{}, fail("structure.parent-invalid", "a link cannot hold blocks")
	}
	index, err := destinationIndex(manifest, step.Placement, parent, true)
	if err != nil {
		return mutation{}, err
	}
	id, err := ids.next()
	if err != nil {
		return mutation{}, err
	}
	parent.Blocks = insertID(parent.Blocks, index, id)
	return mutation{
		receipt: Receipt{
			Book: manifest.ID, Target: id, Kind: "block", Parent: parent.ID, Index: index,
			Order: append([]model.ID(nil), parent.Blocks...), Changed: true,
		},
		created: []model.ID{id},
	}, nil
}

func applyRename(manifest *model.Manifest, step Step) (mutation, error) {
	node, ok := manifest.FindNode(step.Target)
	if !ok {
		if _, _, isBlock := blockParent(manifest, step.Target); isBlock {
			return mutation{}, fail("structure.block-no-title", "block %s has no title", step.Target)
		}
		return mutation{}, fail("structure.unknown-target", "unit %s does not exist", step.Target)
	}
	cleanTitle := model.NormalizeTitle(step.Title)
	if strings.TrimSpace(cleanTitle) == "" {
		return mutation{}, fail("structure.empty-title", "the title must not be empty")
	}
	node.Title = cleanTitle
	return mutation{receipt: Receipt{
		Book: manifest.ID, Target: step.Target, Kind: string(node.Kind), Index: -1, Changed: true,
	}}, nil
}

func applyMove(manifest *model.Manifest, step Step) (mutation, error) {
	placement := step.Placement
	if placement.Parent.IsZero() && placement.Before.IsZero() && placement.After.IsZero() && placement.Index == 0 {
		return mutation{}, fail("structure.invalid-placement", "a destination is required")
	}
	receipt := Receipt{Book: manifest.ID, Target: step.Target, Index: -1, Changed: true}
	if node, ok := manifest.FindNode(step.Target); ok {
		if node == manifest.Root {
			return mutation{}, fail("structure.root", "the book root cannot be moved")
		}
		if !placement.Parent.IsZero() {
			if candidate, found := manifest.FindNode(placement.Parent); found {
				if candidate.ID == node.ID || containsNode(node, candidate.ID) {
					return mutation{}, fail("structure.cycle", "cannot move %s into its own subtree", step.Target)
				}
			}
		}
		destination, err := destinationParent(manifest, placement, false)
		if err != nil {
			return mutation{}, err
		}
		if destination.ID == node.ID || containsNode(node, destination.ID) {
			return mutation{}, fail("structure.cycle", "cannot move %s into its own subtree", step.Target)
		}
		newDepth := depthOf(manifest, destination.ID) + 1
		if newDepth+subtreeDepth(node)-1 > 3 {
			return mutation{}, fail("structure.level-exceeded",
				"the move would create a fourth structural level")
		}
		if node.Kind == model.KindLink && destination.ID != manifest.Root.ID {
			return mutation{}, fail("structure.link-parent", "a link can only be a child of the book")
		}
		currentParent, currentIndex, ok := nodeParent(manifest, step.Target)
		if !ok {
			return mutation{}, fail("structure.unknown-target", "node %s has no parent", step.Target)
		}
		currentParent.Children = removeNodeAt(currentParent.Children, currentIndex)
		index, err := destinationIndex(manifest, placement, destination, false)
		if err != nil {
			return mutation{}, err
		}
		if node.Kind != model.KindLink {
			retag(node, newDepth)
		}
		destination.Children = insertNode(destination.Children, index, node)
		receipt.Kind = string(node.Kind)
		receipt.Parent = destination.ID
		receipt.Index = index
		receipt.Order = childIDs(destination.Children)
		return mutation{receipt: receipt}, nil
	}
	if currentParent, currentIndex, isBlock := blockParent(manifest, step.Target); isBlock {
		destination, err := destinationParent(manifest, placement, true)
		if err != nil {
			return mutation{}, err
		}
		if destination.Kind == model.KindLink {
			return mutation{}, fail("structure.parent-invalid", "a link cannot hold blocks")
		}
		currentParent.Blocks = removeIDAt(currentParent.Blocks, currentIndex)
		index, err := destinationIndex(manifest, placement, destination, true)
		if err != nil {
			return mutation{}, err
		}
		destination.Blocks = insertID(destination.Blocks, index, step.Target)
		receipt.Kind = "block"
		receipt.Parent = destination.ID
		receipt.Index = index
		receipt.Order = append([]model.ID(nil), destination.Blocks...)
		return mutation{receipt: receipt}, nil
	}
	return mutation{}, fail("structure.unknown-target", "unit %s does not exist", step.Target)
}

func applyRemove(manifest *model.Manifest, step Step) (mutation, error) {
	receipt := Receipt{Book: manifest.ID, Target: step.Target, Index: -1, Changed: true}
	result := mutation{receipt: receipt}
	if node, ok := manifest.FindNode(step.Target); ok {
		if node == manifest.Root {
			return mutation{}, fail("structure.root", "the book root cannot be removed")
		}
		if !step.Recursive && (len(node.Children) > 0 || len(node.Blocks) > 0) {
			return mutation{}, fail("structure.node-not-empty",
				"node %s still has children or blocks; pass --recursive", step.Target)
		}
		collectBlocks(node, &result.removed)
		removedNodes := make(map[model.ID]struct{})
		node.Walk(func(current *model.Node, depth int, parent *model.Node) bool {
			removedNodes[current.ID] = struct{}{}
			return true
		})
		parent, index, ok := nodeParent(manifest, step.Target)
		if !ok {
			return mutation{}, fail("structure.unknown-target", "node %s has no parent", step.Target)
		}
		parent.Children = removeNodeAt(parent.Children, index)
		result.removed = append(result.removed, cleanupRegions(manifest, removedNodes, result.removed)...)
		for _, id := range result.removed {
			manifest.SetMeta(id, model.BlockMeta{})
		}
		result.receipt.Kind = string(node.Kind)
		result.receipt.Parent = parent.ID
		result.receipt.Order = childIDs(parent.Children)
		return result, nil
	}
	if parent, index, isBlock := blockParent(manifest, step.Target); isBlock {
		result.removed = append(result.removed, step.Target)
		parent.Blocks = removeIDAt(parent.Blocks, index)
		result.removed = append(result.removed, cleanupRegions(manifest, nil, result.removed)...)
		for _, id := range result.removed {
			manifest.SetMeta(id, model.BlockMeta{})
		}
		result.receipt.Kind = "block"
		result.receipt.Parent = parent.ID
		result.receipt.Order = append([]model.ID(nil), parent.Blocks...)
		return result, nil
	}
	return mutation{}, fail("structure.unknown-target", "unit %s does not exist", step.Target)
}

// cleanupRegions drops notes whose target disappears with the removed unit. Their blocks are
// returned so the caller deletes their bodies too.
func cleanupRegions(manifest *model.Manifest, removedNodes map[model.ID]struct{}, removed []model.ID) []model.ID {
	extra := make([]model.ID, 0)
	kept := manifest.Regions[:0]
	for _, region := range manifest.Regions {
		if region == nil {
			continue
		}
		drop := false
		switch region.Target.Kind {
		case model.TargetNode:
			if _, exists := removedNodes[region.Target.ID]; exists {
				drop = true
			}
		case model.TargetBlock:
			if containsID(removed, region.Target.ID) {
				drop = true
			}
		}
		if drop {
			extra = append(extra, region.Blocks...)
			continue
		}
		kept = append(kept, region)
	}
	manifest.Regions = kept
	return extra
}

func containsID(ids []model.ID, id model.ID) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func applySetting(manifest *model.Manifest, step Step) (mutation, error) {
	if err := ApplySettingValue(&manifest.Settings, step.Key, step.Value); err != nil {
		return mutation{}, err
	}
	return mutation{receipt: Receipt{Book: manifest.ID, Kind: "settings", Index: -1, Changed: true}}, nil
}

// ApplySettingValue parses one book setting key and writes it into settings. It is shared by the
// per-book `book set` command and the library defaults.
func ApplySettingValue(settings *model.Settings, key, value string) error {
	switch key {
	case "numbering":
		enabled, err := parseOnOff(key, value)
		if err != nil {
			return err
		}
		settings.Numbering.Enabled = enabled
	case "toc":
		enabled, err := parseOnOff(key, value)
		if err != nil {
			return err
		}
		settings.TOC.Enabled = enabled
	case "index-depth":
		depth, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || depth < 1 {
			return fail("structure.invalid-value",
				"index-depth expects an integer of at least 1, got %q", value)
		}
		settings.IndexDepth = depth
	case "output":
		mode := model.OutputMode(strings.TrimSpace(value))
		if !mode.Valid() {
			return fail("structure.invalid-value",
				"output expects monolith, chapter or part, got %q", value)
		}
		settings.Output = mode
	default:
		return fail("structure.unknown-setting",
			"unknown book setting %q; use numbering, toc, index-depth or output", key)
	}
	return nil
}

func (service *Service) load(book model.BookID) (store.BookPaths, *model.Manifest, error) {
	paths, err := service.Lifecycle.RequireOpen(book)
	if err != nil {
		return store.BookPaths{}, nil, err
	}
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		return store.BookPaths{}, nil, fail("structure.read-failed", "%v", err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		return store.BookPaths{}, nil, err
	}
	return paths, manifest, nil
}

func (service *Service) save(paths store.BookPaths, manifest *model.Manifest) error {
	data, err := manifest.Encode()
	if err != nil {
		return fail("structure.write-failed", "%v", err)
	}
	if err := store.WriteAtomic(filepath.Join(paths.Dir, lifecycle.ManifestMember), data); err != nil {
		return err
	}
	return nil
}

func validate(manifest *model.Manifest) error {
	if issues := manifest.Validate(); issues != nil {
		return issues
	}
	return nil
}

func bodyPath(paths store.BookPaths, id model.ID) string {
	return filepath.Join(paths.BlocksDir, id.String()+".md")
}

func layoutPath(paths store.BookPaths, id model.ID) string {
	return filepath.Join(paths.LayoutsDir, "asciiui", id.String()+".asciiui")
}

// addDestination resolves the parent, index and unit kind for Add.
func addDestination(manifest *model.Manifest, placement Placement) (*model.Node, int, bool, string, error) {
	if !placement.Before.IsZero() || !placement.After.IsZero() {
		if !placement.Before.IsZero() && !placement.After.IsZero() {
			return nil, 0, false, "", fail("structure.invalid-placement", "use --before or --after, not both")
		}
		anchor := placement.Before
		if anchor.IsZero() {
			anchor = placement.After
		}
		parent, _, anchorBlock, ok := locateSibling(manifest, anchor)
		if !ok {
			return nil, 0, false, "", fail("structure.unknown-target", "sibling %s does not exist", anchor)
		}
		kind, err := childKind(parent)
		if err != nil {
			return nil, 0, false, "", err
		}
		if (kind == "block") != anchorBlock {
			return nil, 0, false, "", fail("structure.anchor-mismatch",
				"the anchor and the new unit have different kinds under %s", parent.ID)
		}
		index, err := destinationIndex(manifest, placement, parent, anchorBlock)
		if err != nil {
			return nil, 0, false, "", err
		}
		return parent, index, anchorBlock, kind, nil
	}

	parent := manifest.Root
	if !placement.Parent.IsZero() {
		found, ok := manifest.FindNode(placement.Parent)
		if !ok {
			if _, _, isBlock := blockParent(manifest, placement.Parent); isBlock {
				return nil, 0, false, "", fail("structure.parent-invalid", "a block cannot hold units")
			}
			return nil, 0, false, "", fail("structure.unknown-target",
				"parent %s does not exist", placement.Parent)
		}
		parent = found
	}
	kind, err := childKind(parent)
	if err != nil {
		return nil, 0, false, "", err
	}
	index, err := destinationIndex(manifest, placement, parent, kind == "block")
	if err != nil {
		return nil, 0, false, "", err
	}
	return parent, index, kind == "block", kind, nil
}

// destinationParent resolves the destination node for Move.
func destinationParent(manifest *model.Manifest, placement Placement, unitBlock bool) (*model.Node, error) {
	if !placement.Before.IsZero() || !placement.After.IsZero() {
		if !placement.Before.IsZero() && !placement.After.IsZero() {
			return nil, fail("structure.invalid-placement", "use --before or --after, not both")
		}
		anchor := placement.Before
		if anchor.IsZero() {
			anchor = placement.After
		}
		parent, _, anchorBlock, ok := locateSibling(manifest, anchor)
		if !ok {
			return nil, fail("structure.unknown-target", "sibling %s does not exist", anchor)
		}
		if anchorBlock != unitBlock {
			return nil, fail("structure.anchor-mismatch",
				"the anchor and the moved unit have different kinds")
		}
		return parent, nil
	}
	if placement.Parent.IsZero() {
		return manifest.Root, nil
	}
	parent, ok := manifest.FindNode(placement.Parent)
	if !ok {
		if _, _, isBlock := blockParent(manifest, placement.Parent); isBlock {
			return nil, fail("structure.parent-invalid", "a block cannot hold units")
		}
		return nil, fail("structure.unknown-target", "parent %s does not exist", placement.Parent)
	}
	if parent.Kind == model.KindLink {
		return nil, fail("structure.parent-invalid", "a link cannot hold units")
	}
	if !unitBlock && parent.Kind == model.KindSubchapter {
		return nil, fail("structure.parent-invalid", "a subchapter cannot hold a subchapter")
	}
	return parent, nil
}

func destinationIndex(manifest *model.Manifest, placement Placement, parent *model.Node, unitBlock bool) (int, error) {
	if !placement.Before.IsZero() || !placement.After.IsZero() {
		anchor := placement.Before
		if anchor.IsZero() {
			anchor = placement.After
		}
		var index int
		var ok bool
		if unitBlock {
			_, index, ok = locateBlock(manifest, anchor)
		} else {
			_, index, ok = locateNodeChild(manifest, anchor)
		}
		if !ok {
			return 0, fail("structure.unknown-target", "sibling %s does not exist", anchor)
		}
		if !placement.After.IsZero() {
			index++
		}
		return index, nil
	}
	length := len(parent.Children)
	if unitBlock {
		length = len(parent.Blocks)
	}
	index := placement.Index
	if index < 0 {
		index = length
	}
	if index > length {
		return 0, fail("structure.invalid-index",
			"index %d is beyond the %d entries of %s", placement.Index, length, parent.ID)
	}
	return index, nil
}

func childKind(parent *model.Node) (string, error) {
	switch parent.Kind {
	case model.KindTitle:
		return string(model.KindChapter), nil
	case model.KindChapter:
		return string(model.KindSubchapter), nil
	case model.KindSubchapter:
		return "block", nil
	default:
		return "", fail("structure.parent-invalid", "a %s cannot hold units", parent.Kind)
	}
}

func locateSibling(manifest *model.Manifest, id model.ID) (*model.Node, int, bool, bool) {
	if parent, index, ok := locateNodeChild(manifest, id); ok {
		return parent, index, false, true
	}
	if parent, index, ok := locateBlock(manifest, id); ok {
		return parent, index, true, true
	}
	return nil, 0, false, false
}

func locateNodeChild(manifest *model.Manifest, id model.ID) (*model.Node, int, bool) {
	var parent *model.Node
	index := -1
	found := false
	manifest.Walk(func(node *model.Node, depth int, current *model.Node) bool {
		if found {
			return false
		}
		for position, child := range node.Children {
			if child.ID == id {
				parent = node
				index = position
				found = true
				return false
			}
		}
		return true
	})
	return parent, index, found
}

func locateBlock(manifest *model.Manifest, id model.ID) (*model.Node, int, bool) {
	var parent *model.Node
	index := -1
	found := false
	manifest.Walk(func(node *model.Node, depth int, current *model.Node) bool {
		if found {
			return false
		}
		for position, blockID := range node.Blocks {
			if blockID == id {
				parent = node
				index = position
				found = true
				return false
			}
		}
		return true
	})
	return parent, index, found
}

func nodeParent(manifest *model.Manifest, id model.ID) (*model.Node, int, bool) {
	return locateNodeChild(manifest, id)
}

func blockParent(manifest *model.Manifest, id model.ID) (*model.Node, int, bool) {
	return locateBlock(manifest, id)
}

func depthOf(manifest *model.Manifest, id model.ID) int {
	depth := 0
	manifest.Walk(func(node *model.Node, current int, parent *model.Node) bool {
		if node.ID == id {
			depth = current
			return false
		}
		return depth == 0
	})
	return depth
}

func subtreeDepth(node *model.Node) int {
	depth := 1
	for _, child := range node.Children {
		if childDepth := 1 + subtreeDepth(child); childDepth > depth {
			depth = childDepth
		}
	}
	return depth
}

func containsNode(node *model.Node, id model.ID) bool {
	found := false
	node.Walk(func(current *model.Node, depth int, parent *model.Node) bool {
		if current.ID == id {
			found = true
			return false
		}
		return !found
	})
	return found
}

func retag(node *model.Node, depth int) {
	switch depth {
	case 2:
		node.Kind = model.KindChapter
	case 3:
		node.Kind = model.KindSubchapter
	}
	for _, child := range node.Children {
		retag(child, depth+1)
	}
}

func collectBlocks(node *model.Node, ids *[]model.ID) {
	*ids = append(*ids, node.Blocks...)
	for _, child := range node.Children {
		collectBlocks(child, ids)
	}
}

func insertID(list []model.ID, index int, id model.ID) []model.ID {
	list = append(list, 0)
	copy(list[index+1:], list[index:])
	list[index] = id
	return list
}

func insertNode(list []*model.Node, index int, node *model.Node) []*model.Node {
	list = append(list, nil)
	copy(list[index+1:], list[index:])
	list[index] = node
	return list
}

func removeIDAt(list []model.ID, index int) []model.ID {
	return append(list[:index], list[index+1:]...)
}

func removeNodeAt(list []*model.Node, index int) []*model.Node {
	return append(list[:index], list[index+1:]...)
}

func childIDs(children []*model.Node) []model.ID {
	ids := make([]model.ID, 0, len(children))
	for _, child := range children {
		ids = append(ids, child.ID)
	}
	return ids
}

func parseOnOff(key, value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1":
		return true, nil
	case "off", "false", "0":
		return false, nil
	}
	return false, fail("structure.invalid-value", "%s expects on or off, got %q", key, value)
}

type bookIDs struct {
	base model.Allocator
	used map[model.ID]struct{}
}

func newIDs(manifest *model.Manifest, base model.Allocator) *bookIDs {
	used := make(map[model.ID]struct{})
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		used[node.ID] = struct{}{}
		return true
	})
	for _, region := range manifest.Regions {
		if region != nil {
			used[region.ID] = struct{}{}
		}
	}
	for _, id := range manifest.PlacedBlocks() {
		used[id] = struct{}{}
	}
	return &bookIDs{base: base, used: used}
}

func (ids *bookIDs) next() (model.ID, error) {
	for {
		id, err := ids.base.Next()
		if err != nil {
			return model.ZeroID, fail("structure.alloc-failed", "%v", err)
		}
		if id.IsZero() {
			continue
		}
		if _, exists := ids.used[id]; exists {
			continue
		}
		ids.used[id] = struct{}{}
		return id, nil
	}
}
