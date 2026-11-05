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
	Kind    string
	Parent  model.ID
	Index   int
	Order   []model.ID
	Changed bool
}

// Step is one declarative mutation.
type Step struct {
	Op        string
	Title     string
	Target    model.ID
	Placement Placement
	Recursive bool
	Key       string
	Value     string
}

// mutation is the pure result of one step: the receipt plus the file effects the commit applies.
type mutation struct {
	receipt Receipt
	created []model.ID
	removed []model.ID
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
	receipts, err := service.run(book, "struct.add", []Step{{Op: "add", Title: title, Placement: placement}}, dryRun)
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
	ids := newIDs(manifest, service.Allocator)
	receipts := make([]Receipt, 0, len(steps))
	var created, removed []model.ID
	for _, step := range steps {
		result, stepErr := step.apply(manifest, ids)
		if stepErr != nil {
			return nil, stepErr
		}
		receipts = append(receipts, result.receipt)
		created = append(created, result.created...)
		removed = append(removed, result.removed...)
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
	}
	if err := service.save(paths, manifest); err != nil {
		return nil, err
	}
	if len(created) > 0 {
		if err := os.MkdirAll(paths.BlocksDir, 0o755); err != nil {
			return nil, fail("structure.write-failed", "%v", err)
		}
		for _, id := range created {
			if err := os.WriteFile(bodyPath(paths, id), nil, 0o644); err != nil {
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
	case "add-block":
		return applyAddBlock(manifest, ids, step)
	case "rename":
		return applyRename(manifest, step)
	case "move":
		return applyMove(manifest, step)
	case "remove":
		return applyRemove(manifest, step)
	case "setting":
		return applySetting(manifest, step)
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
	} else {
		node := &model.Node{ID: id, Kind: model.Kind(kind), Title: cleanTitle}
		parent.Children = insertNode(parent.Children, index, node)
		result.receipt.Order = childIDs(parent.Children)
	}
	return result, nil
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
		parent, index, ok := nodeParent(manifest, step.Target)
		if !ok {
			return mutation{}, fail("structure.unknown-target", "node %s has no parent", step.Target)
		}
		parent.Children = removeNodeAt(parent.Children, index)
		result.receipt.Kind = string(node.Kind)
		result.receipt.Parent = parent.ID
		result.receipt.Order = childIDs(parent.Children)
		return result, nil
	}
	if parent, index, isBlock := blockParent(manifest, step.Target); isBlock {
		result.removed = append(result.removed, step.Target)
		parent.Blocks = removeIDAt(parent.Blocks, index)
		result.receipt.Kind = "block"
		result.receipt.Parent = parent.ID
		result.receipt.Order = append([]model.ID(nil), parent.Blocks...)
		return result, nil
	}
	return mutation{}, fail("structure.unknown-target", "unit %s does not exist", step.Target)
}

func applySetting(manifest *model.Manifest, step Step) (mutation, error) {
	switch step.Key {
	case "numbering":
		enabled, err := parseOnOff(step.Key, step.Value)
		if err != nil {
			return mutation{}, err
		}
		manifest.Settings.Numbering.Enabled = enabled
	case "toc":
		enabled, err := parseOnOff(step.Key, step.Value)
		if err != nil {
			return mutation{}, err
		}
		manifest.Settings.TOC.Enabled = enabled
	case "index-depth":
		depth, err := strconv.Atoi(strings.TrimSpace(step.Value))
		if err != nil || depth < 1 {
			return mutation{}, fail("structure.invalid-value",
				"index-depth expects an integer of at least 1, got %q", step.Value)
		}
		manifest.Settings.IndexDepth = depth
	case "output":
		mode := model.OutputMode(strings.TrimSpace(step.Value))
		if !mode.Valid() {
			return mutation{}, fail("structure.invalid-value",
				"output expects monolith, chapter or part, got %q", step.Value)
		}
		manifest.Settings.Output = mode
	default:
		return mutation{}, fail("structure.unknown-setting",
			"unknown book setting %q; use numbering, toc, index-depth or output", step.Key)
	}
	return mutation{receipt: Receipt{Book: manifest.ID, Kind: "settings", Index: -1, Changed: true}}, nil
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
