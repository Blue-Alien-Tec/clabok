// Package refs derives stable anchors and resolves structure references by ID.
//
// Anchors never contain a number: enabling or changing numbering cannot break a reference. Numbers
// exist only in the rendered heading text; the anchor is derived from the stored title.
package refs

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// Pattern matches a prose reference to a stable ID: [[id:42]].
var Pattern = regexp.MustCompile(`\[\[id:([0-9]+)\]\]`)

// Anchor derives a deterministic anchor from a title. Letters and digits are kept (Unicode
// included), every run of other characters becomes one dash, and numbers are never added.
func Anchor(title string) string {
	var builder strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(title) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	anchor := strings.Trim(builder.String(), "-")
	if anchor == "" {
		return "section"
	}
	return anchor
}

// Assigner hands out unique anchors within one rendered document.
type Assigner struct {
	used map[string]struct{}
}

// NewAssigner returns an empty assigner.
func NewAssigner() *Assigner {
	return &Assigner{used: map[string]struct{}{}}
}

// Next returns the anchor for a title, suffixed on collision.
func (assigner *Assigner) Next(title string) string {
	base := Anchor(title)
	anchor := base
	for index := 2; ; index++ {
		if _, exists := assigner.used[anchor]; !exists {
			assigner.used[anchor] = struct{}{}
			return anchor
		}
		anchor = base + "-" + strconv.Itoa(index)
	}
}

// Target is the resolved location of a reference.
type Target struct {
	Kind  string
	ID    model.ID
	Title string
	Path  []*model.Node
}

// Resolve finds a node or a block by ID.
func Resolve(manifest *model.Manifest, id model.ID) (Target, bool) {
	var found *Target
	var walk func(node *model.Node, path []*model.Node) bool
	walk = func(node *model.Node, path []*model.Node) bool {
		if node.ID == id {
			found = &Target{Kind: string(node.Kind), ID: id, Title: node.Title, Path: copyPath(path)}
			return false
		}
		for _, blockID := range node.Blocks {
			if blockID == id {
				found = &Target{Kind: "block", ID: id, Title: node.Title, Path: copyPath(path)}
				return false
			}
		}
		for _, child := range node.Children {
			next := append(copyPath(path), node)
			if !walk(child, next) {
				return false
			}
		}
		return true
	}
	if manifest == nil || manifest.Root == nil {
		return Target{}, false
	}
	walk(manifest.Root, nil)
	if found == nil {
		return Target{}, false
	}
	return *found, true
}

func copyPath(path []*model.Node) []*model.Node {
	if path == nil {
		return nil
	}
	copied := make([]*model.Node, len(path))
	copy(copied, path)
	return copied
}

// ParseID parses the decimal ID captured by Pattern.
func ParseID(text string) (model.ID, bool) {
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil || value == 0 {
		return model.ZeroID, false
	}
	return model.ID(value), true
}
