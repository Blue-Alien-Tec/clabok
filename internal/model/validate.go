package model

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Issue is one validation finding. Validation aggregates instead of raising, so one call reports
// every structural defect.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError carries all findings of one validation run.
type ValidationError struct {
	Issues []Issue
}

// Error returns the human summary with the first finding.
func (err *ValidationError) Error() string {
	if len(err.Issues) == 0 {
		return "invalid book"
	}
	if len(err.Issues) == 1 {
		return "invalid book: " + err.Issues[0].String()
	}
	return fmt.Sprintf("invalid book: %d problems; first: %s", len(err.Issues), err.Issues[0].String())
}

// ErrorCode returns the stable code for the CLI boundary.
func (err *ValidationError) ErrorCode() string { return "model.invalid" }

// Codes returns the finding codes in order, which is what most tests need.
func (err *ValidationError) Codes() []string {
	codes := make([]string, 0, len(err.Issues))
	for _, issue := range err.Issues {
		codes = append(codes, issue.Code)
	}
	return codes
}

// String renders one issue as "code at path: message".
func (issue Issue) String() string {
	return fmt.Sprintf("%s at %s: %s", issue.Code, issue.Path, issue.Message)
}

type manifestValidator struct {
	manifest *Manifest
	issues   []Issue
	ids      map[ID]string
	nodes    map[ID]string
	placed   map[ID]string
}

// Validate checks the structure, the identifiers, the regions and the settings.
func (manifest *Manifest) Validate() *ValidationError {
	if manifest == nil {
		return &ValidationError{Issues: []Issue{{
			Code: "model.missing-manifest", Path: "", Message: "manifest is nil",
		}}}
	}
	validator := &manifestValidator{
		manifest: manifest,
		ids:      map[ID]string{},
		nodes:    map[ID]string{},
		placed:   map[ID]string{},
	}
	if manifest.Version != Version {
		validator.add("model.manifest-version", "version", "expected %d, got %d", Version, manifest.Version)
	}
	if !manifest.ID.Valid() {
		validator.add("model.invalid-book-id", "id", "invalid book id %q", string(manifest.ID))
	}
	if manifest.Root == nil {
		validator.add("model.missing-root", "root", "the manifest has no root node")
	} else {
		validator.walkNode(manifest.Root, 1, "root")
	}
	validator.validateRegions()
	validator.validateProseRefs()
	validator.validateBlockMeta()
	validator.validateStatus()
	validator.validateBadges()
	validator.validateLayout()
	validator.validateSettings()
	if len(validator.issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: validator.issues}
}

// Validate checks the manifest and the body consistency.
func (book *Book) Validate() *ValidationError {
	if book == nil || book.Manifest == nil {
		return &ValidationError{Issues: []Issue{{
			Code: "model.missing-manifest", Path: "", Message: "book has no manifest",
		}}}
	}
	var issues []Issue
	if manifestIssues := book.Manifest.Validate(); manifestIssues != nil {
		issues = append(issues, manifestIssues.Issues...)
	}
	placedIDs := book.Manifest.PlacedBlocks()
	if len(issues) == 0 {
		for _, id := range placedIDs {
			if _, exists := book.Bodies[id]; !exists {
				issues = append(issues, Issue{
					Code:    "model.missing-body",
					Path:    "blocks[" + id.String() + "]",
					Message: "no body for a placed block",
				})
			}
		}
	}
	placement := make(map[ID]struct{}, len(placedIDs))
	for _, id := range placedIDs {
		placement[id] = struct{}{}
	}
	ids := make([]ID, 0, len(book.Bodies))
	for id := range book.Bodies {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, id := range ids {
		if id.IsZero() {
			issues = append(issues, Issue{
				Code: "model.zero-id", Path: "blocks[0]", Message: "a body id must not be zero",
			})
			continue
		}
		if _, exists := placement[id]; !exists {
			issues = append(issues, Issue{
				Code:    "model.orphan-body",
				Path:    "blocks[" + id.String() + "]",
				Message: "body has no placement",
			})
		}
	}
	for _, region := range book.Manifest.Regions {
		if region == nil || region.Kind != RegionProse {
			continue
		}
		for _, id := range region.Blocks {
			if message := ValidateProse(book.Bodies[id]); message != "" {
				issues = append(issues, Issue{
					Code:    "model.invalid-prose",
					Path:    "blocks[" + id.String() + "]",
					Message: message,
				})
			}
		}
	}
	if len(issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: issues}
}

func (validator *manifestValidator) add(code, path, format string, args ...any) {
	validator.issues = append(validator.issues, Issue{
		Code:    code,
		Path:    path,
		Message: fmt.Sprintf(format, args...),
	})
}

func (validator *manifestValidator) claim(id ID, path string) bool {
	if id.IsZero() {
		validator.add("model.zero-id", path, "an id must not be zero")
		return false
	}
	if previous, exists := validator.ids[id]; exists {
		validator.add("model.duplicate-id", path, "id %s is already used at %s", id, previous)
		return false
	}
	validator.ids[id] = path
	return true
}

func (validator *manifestValidator) walkNode(node *Node, depth int, path string) {
	if node == nil {
		validator.add("model.missing-node", path, "node is null")
		return
	}
	if depth > 3 {
		validator.add("model.depth-exceeded", path,
			"structural level %d exceeds the three allowed levels", depth)
		return
	}
	if validator.claim(node.ID, path+".id") {
		validator.nodes[node.ID] = path
	}
	if !node.Kind.Valid() {
		validator.add("model.kind-depth", path+".kind", "unknown kind %q", node.Kind)
	} else {
		switch depth {
		case 1:
			if node.Kind != KindTitle {
				validator.add("model.kind-depth", path+".kind",
					"only a title may be at level 1, got %q", node.Kind)
			}
		case 2:
			if node.Kind != KindChapter && node.Kind != KindLink {
				validator.add("model.kind-depth", path+".kind",
					"only a chapter or a link may be at level 2, got %q", node.Kind)
			}
		case 3:
			if node.Kind != KindSubchapter {
				validator.add("model.kind-depth", path+".kind",
					"only a subchapter may be at level 3, got %q", node.Kind)
			}
		}
	}
	if strings.TrimSpace(node.Title) == "" {
		validator.add("model.empty-title", path+".title", "the title must not be empty")
	}
	if node.Part != "" && node.Kind != KindChapter && node.Kind != KindLink {
		validator.add("model.part-on-node", path+".part",
			"part is allowed on a chapter or a link, not on %q", node.Kind)
	}
	if node.Kind == KindLink {
		if !node.Ref.Valid() {
			validator.add("model.link-missing-ref", path+".ref", "a link needs a reference")
		}
		if len(node.Blocks) > 0 {
			validator.add("model.link-not-leaf", path+".blocks", "a link cannot hold blocks")
		}
		if len(node.Children) > 0 {
			validator.add("model.link-not-leaf", path+".children", "a link cannot hold children")
		}
		return
	}
	if node.Ref != nil {
		validator.add("model.link-with-ref", path+".ref",
			"only a link carries a reference")
	}
	validator.validateBlocks(node.Blocks, path+".blocks")
	for index, child := range node.Children {
		validator.walkNode(child, depth+1, fmt.Sprintf("%s.children[%d]", path, index))
	}
}

func (validator *manifestValidator) validateBlocks(blocks []ID, path string) {
	for index, id := range blocks {
		blockPath := fmt.Sprintf("%s[%d]", path, index)
		validator.claim(id, blockPath)
		if id.IsZero() {
			continue
		}
		if previous, exists := validator.placed[id]; exists {
			validator.add("model.duplicate-placement", blockPath,
				"block %s is already placed at %s", id, previous)
		}
		validator.placed[id] = blockPath
	}
}

func (validator *manifestValidator) validateRegions() {
	footers := 0
	notesTargets := map[string]string{}
	proseTargets := map[string]string{}
	for index, region := range validator.manifest.Regions {
		path := fmt.Sprintf("regions[%d]", index)
		if region == nil {
			validator.add("model.missing-region", path, "region is null")
			continue
		}
		validator.claim(region.ID, path+".id")
		if !region.Kind.Valid() {
			validator.add("model.unknown-region-kind", path+".kind",
				"unknown region kind %q", region.Kind)
		}
		switch region.Target.Kind {
		case TargetBook:
			if !region.Target.ID.IsZero() {
				validator.add("model.invalid-target", path+".target", "the book target carries no id")
			}
			if region.Kind == RegionNotes {
				validator.add("model.invalid-target", path+".target",
					"notes target a node or a block")
			}
		case TargetNode:
			if region.Target.ID.IsZero() {
				validator.add("model.invalid-target", path+".target", "a node target needs an id")
			} else if _, exists := validator.nodes[region.Target.ID]; !exists {
				validator.add("model.invalid-target", path+".target",
					"node %s does not exist", region.Target.ID)
			}
		case TargetBlock:
			if region.Target.ID.IsZero() {
				validator.add("model.invalid-target", path+".target", "a block target needs an id")
			} else if _, exists := validator.placed[region.Target.ID]; !exists {
				validator.add("model.invalid-target", path+".target",
					"block %s is not placed", region.Target.ID)
			}
		default:
			validator.add("model.invalid-target", path+".target",
				"unknown target kind %q", region.Target.Kind)
		}
		if region.Kind == RegionProse && region.Target.Kind != TargetNode {
			validator.add("model.invalid-target", path+".target",
				"index prose targets a node")
		}
		if region.Kind == RegionFooter {
			footers++
			if footers > 1 {
				validator.add("model.footer-count", path, "a book has at most one footer")
			}
			if region.Target.Kind != TargetBook {
				validator.add("model.invalid-target", path+".target",
					"the footer must target the book")
			}
			if len(region.Blocks) > 1 {
				validator.add("model.footer-blocks", path+".blocks",
					"the footer holds at most one block")
			}
		}
		if region.Kind == RegionNotes {
			key := string(region.Target.Kind) + ":" + region.Target.ID.String()
			if previous, exists := notesTargets[key]; exists {
				validator.add("model.note-count", path,
					"a target has at most one notes region (already at %s)", previous)
			} else {
				notesTargets[key] = path
			}
		}
		if region.Kind == RegionProse && region.Target.Kind == TargetNode {
			key := region.Target.ID.String()
			if previous, exists := proseTargets[key]; exists {
				validator.add("model.prose-count", path,
					"a node has at most one index prose region (already at %s)", previous)
			} else {
				proseTargets[key] = path
			}
			if len(region.Blocks) != 1 {
				validator.add("model.prose-blocks", path+".blocks",
					"an index prose region holds exactly one block")
			}
		}
		validator.validateBlocks(region.Blocks, path+".blocks")
	}
}

// validateProseRefs checks the two-way link between Node.ProseBlock and the prose region that owns
// the block. The text itself lives in the body store, never on the node.
func (validator *manifestValidator) validateProseRefs() {
	owners := map[ID]ID{}
	for _, region := range validator.manifest.Regions {
		if region == nil || region.Kind != RegionProse ||
			region.Target.Kind != TargetNode || len(region.Blocks) != 1 {
			continue
		}
		owners[region.Blocks[0]] = region.Target.ID
	}
	validator.manifest.Walk(func(node *Node, depth int, parent *Node) bool {
		if node.ProseBlock.IsZero() {
			return true
		}
		path := validator.nodes[node.ID] + ".proseBlock"
		if _, placed := validator.placed[node.ProseBlock]; !placed {
			validator.add("model.prose-unplaced", path, "block %s is not placed", node.ProseBlock)
			return true
		}
		owner, exists := owners[node.ProseBlock]
		switch {
		case !exists:
			validator.add("model.prose-not-region", path,
				"block %s is not in an index prose region", node.ProseBlock)
		case owner != node.ID:
			validator.add("model.prose-owner", path,
				"block %s belongs to node %s", node.ProseBlock, owner)
		}
		return true
	})
	blocks := make([]ID, 0, len(owners))
	for block := range owners {
		blocks = append(blocks, block)
	}
	sort.Slice(blocks, func(left, right int) bool { return blocks[left] < blocks[right] })
	for _, block := range blocks {
		owner := owners[block]
		node, exists := validator.manifest.FindNode(owner)
		if exists && node.ProseBlock != block {
			validator.add("model.prose-ref-missing", validator.nodes[owner]+".proseBlock",
				"the prose region block %s is not referenced", block)
		}
	}
}

// validateStatus checks assigned states while the book enables the status system. A disabled system
// ignores assignments, so switching it off stays reversible.
func (validator *manifestValidator) validateStatus() {
	manifest := validator.manifest
	if !manifest.StatusEnabled() {
		return
	}
	manifest.Walk(func(node *Node, depth int, parent *Node) bool {
		if node.Status == "" {
			return true
		}
		if _, exists := FindStatus(node.Status); !exists {
			validator.add("model.unknown-status", validator.nodes[node.ID]+".status",
				"unknown status %q", node.Status)
		}
		return true
	})
	blocks := make([]ID, 0, len(manifest.BlockMeta))
	for id := range manifest.BlockMeta {
		blocks = append(blocks, id)
	}
	sort.Slice(blocks, func(left, right int) bool { return blocks[left] < blocks[right] })
	for _, id := range blocks {
		meta := manifest.BlockMeta[id]
		if meta.Status == "" {
			continue
		}
		if _, exists := FindStatus(meta.Status); !exists {
			validator.add("model.unknown-status", "blockMeta["+id.String()+"].status",
				"unknown status %q", meta.Status)
		}
	}
}

// validateBadges checks the declared badge list. The detector value is a repository fact and is
// resolved by the renderer, not by the manifest validator.
func (validator *manifestValidator) validateBadges() {
	seen := map[string]int{}
	for index, badge := range validator.manifest.Badges {
		path := fmt.Sprintf("badges[%d]", index)
		if message := ValidateTypeToken(badge.ID); badge.ID == "" || message != "" {
			validator.add("model.invalid-badge", path+".id",
				"badge id must be a single token of at most 64 characters")
		}
		if previous, exists := seen[badge.ID]; exists {
			validator.add("model.duplicate-badge", path+".id",
				"badge %q is already declared at badges[%d]", badge.ID, previous)
			continue
		}
		seen[badge.ID] = index
	}
}

// validateLayout checks the generated repository-layout configuration and its single anchor.
func (validator *manifestValidator) validateLayout() {
	manifest := validator.manifest
	blocks := make([]ID, 0, 1)
	for _, id := range manifest.PlacedBlocks() {
		if manifest.BlockType(id) == "RepoLayout" {
			blocks = append(blocks, id)
		}
	}
	if len(blocks) > 1 {
		validator.add("model.layout-count", "blockMeta",
			"a book has at most one RepoLayout block")
	}
	if len(blocks) > 0 && manifest.Layout == nil {
		validator.add("model.layout-missing-config", "layout",
			"a RepoLayout block needs a layout config")
	}
	if manifest.Layout == nil {
		return
	}
	config := manifest.Layout
	if !ValidRepoPath(config.Root) {
		validator.add("model.invalid-layout", "layout.root", "invalid repository path %q", config.Root)
	}
	if config.Depth < 0 {
		validator.add("model.invalid-layout", "layout.depth", "depth must not be negative")
	}
	for index, path := range config.Exclude {
		if path == "" || path == "." || !ValidRepoPath(path) {
			validator.add("model.invalid-layout", fmt.Sprintf("layout.exclude[%d]", index),
				"invalid repository path %q", path)
		}
	}
	paths := make([]string, 0, len(config.Notes))
	for path := range config.Notes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if path == "" || path == "." || !ValidRepoPath(path) {
			validator.add("model.invalid-layout", "layout.notes["+path+"]",
				"invalid repository path %q", path)
			continue
		}
		if message := ValidateProse(config.Notes[path]); message != "" {
			validator.add("model.invalid-layout", "layout.notes["+path+"]", "%s", message)
		}
	}
}

func (validator *manifestValidator) validateBlockMeta() {
	ids := make([]ID, 0, len(validator.manifest.BlockMeta))
	for id := range validator.manifest.BlockMeta {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, id := range ids {
		path := "blockMeta[" + id.String() + "]"
		if id.IsZero() {
			validator.add("model.zero-id", path, "a metadata key must not be zero")
			continue
		}
		if _, placed := validator.placed[id]; !placed {
			validator.add("model.meta-orphan", path, "metadata for an unplaced block")
		}
		meta := validator.manifest.BlockMeta[id]
		if meta.Type != "" {
			if message := ValidateTypeToken(meta.Type); message != "" {
				validator.add("model.invalid-type", path+".type", "%s", message)
			}
		}
	}
}

// ValidateProse checks the one-line index prose. An empty value is valid and means "no prose";
// trailing line breaks are tolerated because body files normally end with one.
func ValidateProse(value string) string {
	text := strings.TrimRight(value, "\r\n")
	if text == "" {
		return ""
	}
	if len([]rune(text)) > 300 {
		return "must be at most 300 characters"
	}
	if strings.ContainsAny(text, "\r\n") {
		return "must be a single line"
	}
	return ""
}

// ValidateTypeToken checks the block type format: a single token of at most 64 characters.
func ValidateTypeToken(value string) string {
	if len(value) > 64 {
		return "must be at most 64 characters"
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return "must not contain whitespace or control characters"
		}
	}
	return ""
}

func (validator *manifestValidator) validateSettings() {
	if !validator.manifest.Settings.Output.Valid() {
		validator.add("model.invalid-setting", "settings.output",
			"unknown output mode %q", validator.manifest.Settings.Output)
	}
	if validator.manifest.Settings.IndexDepth < 1 {
		validator.add("model.invalid-setting", "settings.indexDepth",
			"the index depth must be at least 1")
	}
}
