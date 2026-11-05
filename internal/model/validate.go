package model

import (
	"fmt"
	"sort"
	"strings"
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
		if region.Kind == RegionFooter {
			footers++
			if footers > 1 {
				validator.add("model.footer-count", path, "a book has at most one footer")
			}
			if region.Target.Kind != TargetBook {
				validator.add("model.invalid-target", path+".target",
					"the footer must target the book")
			}
		}
		validator.validateBlocks(region.Blocks, path+".blocks")
	}
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
