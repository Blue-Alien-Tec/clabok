package render

import (
	"path"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/refs"
)

// LinkTarget is the rendered location of a unit inside its own book.
type LinkTarget struct {
	Title  string
	File   string
	Anchor string
}

// Href renders the target relative to the book's output directory.
func (target LinkTarget) Href() string {
	if target.Anchor == "" {
		return target.File
	}
	if target.File == "" {
		return "#" + target.Anchor
	}
	return target.File + "#" + target.Anchor
}

// LayoutFor returns the rendered location of every node and block for the book's output mode. It is
// the single source for anchors and file assignment: rendering, reference expansion and cross-book
// links all use it, so they cannot drift apart.
func LayoutFor(manifest *model.Manifest) map[model.ID]LinkTarget {
	layout := make(map[model.ID]LinkTarget)
	if manifest == nil || manifest.Root == nil {
		return layout
	}
	root := LinkTarget{Title: manifest.Root.Title, File: "README.md"}
	layout[manifest.Root.ID] = root
	for _, block := range manifest.Root.Blocks {
		layout[block] = root
	}
	switch manifest.Settings.Output {
	case model.OutputChapter:
		files := chapterFileMap(manifest)
		for _, chapter := range manifest.Root.Children {
			if chapter.Kind != model.KindChapter {
				continue
			}
			walkLayout([]*model.Node{chapter}, refs.NewAssigner(), files[chapter.ID], layout)
		}
	case model.OutputPart:
		files, _ := partFileMap(manifest, false)
		for _, part := range partOrder(manifest) {
			assigner := refs.NewAssigner()
			for _, chapter := range manifest.Root.Children {
				if chapter.Kind == model.KindChapter && chapter.Part == part {
					walkLayout([]*model.Node{chapter}, assigner, files[part], layout)
				}
			}
		}
	default:
		walkLayout(manifest.Root.Children, refs.NewAssigner(), "README.md", layout)
	}
	return layout
}

func walkLayout(nodes []*model.Node, assigner *refs.Assigner, file string, layout map[model.ID]LinkTarget) {
	var walk func(node *model.Node)
	walk = func(node *model.Node) {
		if node.Kind != model.KindLink {
			target := LinkTarget{Title: node.Title, File: file, Anchor: assigner.Next(node.Title)}
			layout[node.ID] = target
			for _, block := range node.Blocks {
				layout[block] = target
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}
}

// chapterFileMap is the file assignment used by chapter-wise rendering.
func chapterFileMap(manifest *model.Manifest) map[model.ID]string {
	files := make(map[model.ID]string)
	used := map[string]struct{}{"readme": {}}
	for _, chapter := range manifest.Root.Children {
		if chapter.Kind != model.KindChapter {
			continue
		}
		files[chapter.ID] = uniqueSlug(Slug(chapter.Title), used) + ".md"
	}
	return files
}

// partOrder returns the part values in chapter order.
func partOrder(manifest *model.Manifest) []string {
	order := make([]string, 0)
	seen := make(map[string]struct{})
	for _, chapter := range manifest.Root.Children {
		if chapter.Kind != model.KindChapter || strings.TrimSpace(chapter.Part) == "" {
			continue
		}
		if _, exists := seen[chapter.Part]; exists {
			continue
		}
		seen[chapter.Part] = struct{}{}
		order = append(order, chapter.Part)
	}
	return order
}

// partFileMap assigns one file per part. With strict set, two part values that slug to the same name
// are a render error; otherwise the second value is uniquified (used by LayoutFor for an invalid
// book, which the renderer rejects separately).
func partFileMap(manifest *model.Manifest, strict bool) (map[string]string, error) {
	files := make(map[string]string)
	used := map[string]struct{}{"readme": {}}
	for _, part := range partOrder(manifest) {
		slug := Slug(part)
		if _, exists := used[slug]; exists {
			if strict {
				return nil, fail("render.part-name-collision",
					"part names %q collide after slugging", part)
			}
			slug = uniqueSlug(slug, used)
		} else {
			used[slug] = struct{}{}
		}
		files[part] = slug + ".md"
	}
	return files, nil
}

// relHref renders a target path relative to an output directory. Both are project-relative slash
// paths; the target may carry a fragment.
func relHref(fromDir, target string) string {
	base, fragment := target, ""
	if index := strings.Index(target, "#"); index >= 0 {
		base, fragment = target[:index], target[index+1:]
	}
	from := splitPath(fromDir)
	to := splitPath(base)
	common := 0
	for common < len(from) && common < len(to) && from[common] == to[common] {
		common++
	}
	parts := make([]string, 0, len(from)-common+len(to)-common)
	for index := common; index < len(from); index++ {
		parts = append(parts, "..")
	}
	parts = append(parts, to[common:]...)
	href := path.Join(parts...)
	if href == "" {
		href = "."
	}
	if fragment != "" {
		href += "#" + fragment
	}
	return href
}

func splitPath(value string) []string {
	clean := path.Clean(value)
	if clean == "." || clean == "/" {
		return nil
	}
	return strings.Split(strings.TrimPrefix(clean, "/"), "/")
}
