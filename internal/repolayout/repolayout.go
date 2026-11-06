// Package repolayout renders the tracked repository tree for a generated layout block.
//
// The tree comes from `git ls-files`, never from the local working copy, so untracked and ignored
// paths never render. Root, Exclude and Notes are repository-relative; the rendered labels are
// relative to Root. A missing git work tree is ErrNoGit; the renderer omits the generated blocks, and
// a chapter that contains only the generated block disappears from the index, navigation and output.
package repolayout

import (
	"errors"
	"os/exec"
	"sort"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// ErrNoGit reports that the project directory is not a git work tree.
var ErrNoGit = errors.New("no git work tree")

const (
	defaultDepth     = 2
	maxLabelColumn   = 60
	annotationIndent = 2
)

type node struct {
	rel      string
	repoPath string
	name     string
	dir      bool
}

// Available reports whether the project directory is a git work tree.
func Available(projectDir string) bool {
	command := exec.Command("git", "-C", projectDir, "rev-parse", "--is-inside-work-tree")
	output, err := command.Output()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}

// Render returns the tree for the configured layout.
func Render(projectDir string, config *model.LayoutConfig) (string, error) {
	paths, err := trackedPaths(projectDir)
	if err != nil {
		return "", err
	}
	return Build(paths, config), nil
}

// Build lays out the tracked paths. It is the deterministic core behind Render.
func Build(paths []string, config *model.LayoutConfig) string {
	root := ""
	depth := defaultDepth
	var exclude []string
	notes := map[string]string{}
	if config != nil {
		root = normalize(config.Root)
		if root == "." {
			root = ""
		}
		if config.Depth > 0 {
			depth = config.Depth
		}
		for _, item := range config.Exclude {
			if normalized := normalize(item); normalized != "" && normalized != "." {
				exclude = append(exclude, normalized)
			}
		}
		notes = config.Notes
	}
	prefix := ""
	if root != "" {
		prefix = root + "/"
	}

	nodes := map[string]node{}
	for _, raw := range paths {
		path := normalize(raw)
		if path == "" {
			continue
		}
		rel := path
		if prefix != "" {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			rel = strings.TrimPrefix(path, prefix)
		}
		if rel == "" || excluded(path, exclude) {
			continue
		}
		parts := strings.Split(rel, "/")
		for index := 1; index < len(parts); index++ {
			if depth > 0 && index > depth {
				break
			}
			ancestorRel := strings.Join(parts[:index], "/")
			ancestorRepo := ancestorRel
			if root != "" {
				ancestorRepo = root + "/" + ancestorRel
			}
			addNode(nodes, ancestorRel, ancestorRepo, true)
		}
	}

	children := map[string][]node{}
	for _, entry := range nodes {
		parent := ""
		if slash := strings.LastIndex(entry.rel, "/"); slash >= 0 {
			parent = entry.rel[:slash]
		}
		children[parent] = append(children[parent], entry)
	}
	for parent := range children {
		list := children[parent]
		sort.Slice(list, func(left, right int) bool {
			if list[left].dir != list[right].dir {
				return list[left].dir
			}
			leftName := strings.ToLower(list[left].name)
			rightName := strings.ToLower(list[right].name)
			if leftName != rightName {
				return leftName < rightName
			}
			return list[left].name < list[right].name
		})
		children[parent] = list
	}

	type line struct {
		label string
		note  string
	}
	lines := make([]line, 0, len(nodes))
	var walk func(parent string, level int)
	walk = func(parent string, level int) {
		for _, entry := range children[parent] {
			label := strings.Repeat("  ", level) + entry.name
			if entry.dir {
				label += "/"
			}
			lines = append(lines, line{label: label, note: notes[entry.repoPath]})
			if entry.dir {
				walk(entry.rel, level+1)
			}
		}
	}
	walk("", 0)
	if len(lines) == 0 {
		return ""
	}

	column := 0
	for _, item := range lines {
		if len(item.label) > column {
			column = len(item.label)
		}
	}
	column += annotationIndent
	if column > maxLabelColumn {
		column = maxLabelColumn
	}
	var builder strings.Builder
	for _, item := range lines {
		switch {
		case item.note == "":
			builder.WriteString(item.label)
		case len(item.label)+1 <= column:
			builder.WriteString(padRight(item.label, column))
			builder.WriteString(item.note)
		default:
			builder.WriteString(item.label)
			builder.WriteByte('\n')
			builder.WriteString(strings.Repeat(" ", column))
			builder.WriteString(item.note)
		}
		builder.WriteByte('\n')
	}
	return strings.TrimRight(builder.String(), "\n")
}

func addNode(nodes map[string]node, rel, repoPath string, dir bool) {
	if rel == "" {
		return
	}
	if _, exists := nodes[rel]; exists {
		return
	}
	name := rel
	if slash := strings.LastIndex(rel, "/"); slash >= 0 {
		name = rel[slash+1:]
	}
	nodes[rel] = node{rel: rel, repoPath: repoPath, name: name, dir: dir}
}

func excluded(path string, exclude []string) bool {
	for _, item := range exclude {
		if path == item || strings.HasPrefix(path, item+"/") {
			return true
		}
	}
	return false
}

func normalize(path string) string {
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	path = strings.Trim(path, "/")
	return path
}

func padRight(text string, width int) string {
	if len(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(text))
}

func trackedPaths(projectDir string) ([]string, error) {
	command := exec.Command("git", "-C", projectDir, "ls-files", "-z")
	output, err := command.Output()
	if err != nil {
		return nil, ErrNoGit
	}
	raw := strings.Split(string(output), "\x00")
	paths := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.Trim(item, "\r\n")
		if item == "" {
			continue
		}
		paths = append(paths, item)
	}
	sort.Strings(paths)
	return paths, nil
}
