// Package asciiui renders compact UI wireframes as aligned ASCII.
//
// A spec is a small indented tree. Generic widget names have built-in labels; `asciiui.types`
// overrides them, and `widget=` overrides one node explicitly. There is no parity check against
// project C# types.
package asciiui

import (
	"fmt"
	"strings"
)

// Error is an ASCII UI failure with a stable code.
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

// Defaults maps a generic node kind to its generic widget label.
var Defaults = map[string]string{
	"panel":     "Panel",
	"button":    "Button",
	"label":     "Label",
	"input":     "Input",
	"checkbox":  "Checkbox",
	"radio":     "Radio",
	"list":      "List",
	"image":     "Image",
	"value":     "Value",
	"separator": "Separator",
}

// Resolve returns the widget label for a node. A project type is the one place that names a concrete
// widget: an explicit generic label (the built-in default, for example `Button`) yields to the
// configured override, so `asciiui.types.button = UIButton` turns every generic button into UIButton.
// An explicit non-generic label stays a one-node override.
func Resolve(kind, explicit string, overrides map[string]string) string {
	if explicit != "" && !isGenericLabel(explicit) {
		return explicit
	}
	if overrides != nil {
		if value := overrides[kind]; value != "" {
			return value
		}
	}
	if explicit != "" {
		return explicit
	}
	if value := Defaults[kind]; value != "" {
		return value
	}
	return kind
}

// isGenericLabel reports whether the label is one of the built-in generic widget names.
func isGenericLabel(label string) bool {
	for _, generic := range Defaults {
		if strings.EqualFold(generic, label) {
			return true
		}
	}
	return false
}

// Node is one spec entry.
type Node struct {
	Kind     string
	Label    string
	Widget   string
	Count    int
	Children []*Node
}

// Spec is a parsed wireframe.
type Spec struct {
	Title  string
	Legend bool
	Nodes  []*Node
}

// Parse reads a spec document. Two spaces per level define nesting; `#` starts a comment.
func Parse(text string) (*Spec, error) {
	spec := &Spec{}
	type frame struct {
		indent int
		node   *Node
	}
	stack := make([]frame, 0)
	for index, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent%2 != 0 {
			return nil, fail("asciiui.invalid-spec", "line %d: indentation must be a multiple of two", index+1)
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "title":
			spec.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "title"))
			continue
		case "legend":
			spec.Legend = len(fields) > 1 && strings.EqualFold(fields[1], "true")
			continue
		}
		node, err := parseNode(fields, index+1)
		if err != nil {
			return nil, err
		}
		level := indent / 2
		for len(stack) > 0 && stack[len(stack)-1].indent >= level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			spec.Nodes = append(spec.Nodes, node)
		} else {
			parent := stack[len(stack)-1].node
			parent.Children = append(parent.Children, node)
		}
		stack = append(stack, frame{indent: level, node: node})
	}
	if len(spec.Nodes) == 0 {
		return nil, fail("asciiui.invalid-spec", "the spec has no nodes")
	}
	return spec, nil
}

func parseNode(fields []string, line int) (*Node, error) {
	kind := fields[0]
	switch kind {
	case "panel", "row", "col", "button", "label", "input", "checkbox", "radio", "list", "image", "value", "separator":
	default:
		return nil, fail("asciiui.unknown-kind", "line %d: unknown widget %q", line, kind)
	}
	node := &Node{Kind: kind, Count: 3}
	label := make([]string, 0, len(fields))
	for _, field := range fields[1:] {
		if key, value, ok := strings.Cut(field, "="); ok {
			switch key {
			case "widget":
				node.Widget = value
			default:
				label = append(label, field)
			}
			continue
		}
		label = append(label, field)
	}
	if len(label) > 0 && kind == "list" {
		if count, ok := parseInt(label[len(label)-1]); ok {
			node.Count = count
			label = label[:len(label)-1]
		}
	}
	node.Label = strings.Join(label, " ")
	if node.Count < 1 {
		node.Count = 1
	}
	return node, nil
}

func parseInt(text string) (int, bool) {
	value := 0
	if text == "" {
		return 0, false
	}
	for _, character := range text {
		if character < '0' || character > '9' {
			return 0, false
		}
		value = value*10 + int(character-'0')
	}
	return value, true
}

type block struct {
	lines []string
	frame bool
}

// stretch grows a framed block to height by inserting empty content rows before the bottom border.
// A row uses it so panels of unequal content still close on the same line.
func (b block) stretch(height int) block {
	if !b.frame || len(b.lines) < 2 || len(b.lines) >= height {
		return b
	}
	width := len(b.lines[0])
	fill := "|" + strings.Repeat(" ", width-2) + "|"
	lines := make([]string, 0, height)
	lines = append(lines, b.lines[:len(b.lines)-1]...)
	for len(lines) < height-1 {
		lines = append(lines, fill)
	}
	lines = append(lines, b.lines[len(b.lines)-1])
	return block{lines: lines, frame: true}
}

func (b block) width() int {
	width := 0
	for _, line := range b.lines {
		if len(line) > width {
			width = len(line)
		}
	}
	return width
}

// Render turns a spec into aligned ASCII.
func Render(spec *Spec, overrides map[string]string) (string, error) {
	blocks := make([]block, 0, len(spec.Nodes))
	for _, node := range spec.Nodes {
		rendered, err := renderNode(node, overrides)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, rendered)
	}
	page := stack(blocks)
	lines := make([]string, 0, len(page.lines)+4)
	if spec.Title != "" {
		lines = append(lines, spec.Title, "")
	}
	lines = append(lines, page.lines...)
	if spec.Legend {
		legend := legendLines(spec.Nodes, overrides)
		if len(legend) > 0 {
			lines = append(lines, "", "Legend:")
			lines = append(lines, legend...)
		}
	}
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n"), nil
}

func renderNode(node *Node, overrides map[string]string) (block, error) {
	switch node.Kind {
	case "row":
		children := make([]block, 0, len(node.Children))
		for _, child := range node.Children {
			rendered, err := renderNode(child, overrides)
			if err != nil {
				return block{}, err
			}
			children = append(children, rendered)
		}
		return row(children), nil
	case "col":
		children := make([]block, 0, len(node.Children))
		for _, child := range node.Children {
			rendered, err := renderNode(child, overrides)
			if err != nil {
				return block{}, err
			}
			children = append(children, rendered)
		}
		return stack(children), nil
	case "panel":
		inner := make([]block, 0, len(node.Children))
		for _, child := range node.Children {
			rendered, err := renderNode(child, overrides)
			if err != nil {
				return block{}, err
			}
			inner = append(inner, rendered)
		}
		return panel(node.Label, stack(inner)), nil
	}
	return leaf(node), nil
}

func padRight(text string, width int) string {
	if len(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(text))
}

// isRule reports whether a raw content line is a horizontal rule.
func isRule(line string) bool {
	return strings.Contains(line, "-") && strings.Trim(line, "- ") == ""
}

// isRuleLine reports whether a wrapped panel line holds only a horizontal rule between its borders.
func isRuleLine(line string) bool {
	if len(line) < 3 || line[0] != '|' || line[len(line)-1] != '|' {
		return false
	}
	return isRule(line[1 : len(line)-1])
}

// widen grows a framed block to width by extending its border and filling content rows, so stacked
// panels share one right edge.
func (b block) widen(width int) block {
	if !b.frame || len(b.lines) == 0 {
		return b
	}
	current := len(b.lines[0])
	if current >= width {
		return b
	}
	extra := width - current
	lines := make([]string, len(b.lines))
	for index, line := range b.lines {
		if strings.HasPrefix(line, "+") {
			lines[index] = line[:len(line)-1] + strings.Repeat("-", extra) + "+"
			continue
		}
		if isRuleLine(line) {
			lines[index] = "| " + strings.Repeat("-", width-4) + " |"
			continue
		}
		lines[index] = line[:len(line)-1] + strings.Repeat(" ", extra) + line[len(line)-1:]
	}
	return block{lines: lines, frame: true}
}

func row(children []block) block {
	if len(children) == 0 {
		return block{}
	}
	widths := make([]int, len(children))
	height := 0
	for index, child := range children {
		widths[index] = child.width()
		if len(child.lines) > height {
			height = len(child.lines)
		}
	}
	lines := make([]string, height)
	for line := 0; line < height; line++ {
		parts := make([]string, len(children))
		for index, child := range children {
			grown := child.stretch(height)
			text := ""
			if line < len(grown.lines) {
				text = grown.lines[line]
			}
			parts[index] = padRight(text, widths[index])
		}
		lines[line] = strings.Join(parts, "  ")
	}
	return block{lines: lines}
}

func stack(children []block) block {
	width := 0
	for _, child := range children {
		if child.width() > width {
			width = child.width()
		}
	}
	lines := make([]string, 0)
	for index, child := range children {
		if child.frame {
			child = child.widen(width)
		}
		if index > 0 && (child.frame || children[index-1].frame) {
			lines = append(lines, "")
		}
		for _, line := range child.lines {
			lines = append(lines, padRight(line, width))
		}
	}
	return block{lines: lines}
}

func panel(title string, content block) block {
	inner := content.width()
	if len(title) > inner {
		inner = len(title)
	}
	if inner < 4 {
		inner = 4
	}
	lines := make([]string, 0, len(content.lines)+3)
	border := "+" + strings.Repeat("-", inner+2) + "+"
	lines = append(lines, border)
	if title != "" {
		lines = append(lines, "| "+padRight(title, inner)+" |")
	}
	for _, line := range content.lines {
		if isRule(line) {
			line = strings.Repeat("-", inner)
		}
		lines = append(lines, "| "+padRight(line, inner)+" |")
	}
	lines = append(lines, border)
	return block{lines: lines, frame: true}
}

func leaf(node *Node) block {
	label := node.Label
	switch node.Kind {
	case "button":
		return block{lines: []string{"[ " + label + " ]"}}
	case "input":
		width := len(label) + 8
		if width < 12 {
			width = 12
		}
		return block{lines: []string{"[" + padRight(" "+label, width-2) + "]"}}
	case "checkbox":
		return block{lines: []string{"[ ] " + label}}
	case "radio":
		return block{lines: []string{"( ) " + label}}
	case "list":
		lines := make([]string, 0, node.Count+1)
		if label != "" {
			lines = append(lines, label)
		}
		for index := 1; index <= node.Count; index++ {
			lines = append(lines, fmt.Sprintf("- item %d", index))
		}
		return block{lines: lines}
	case "image":
		return block{lines: []string{"[image: " + label + "]"}}
	case "separator":
		return block{lines: []string{strings.Repeat("-", 20)}}
	default:
		return block{lines: []string{label}}
	}
}

func legendLines(nodes []*Node, overrides map[string]string) []string {
	lines := make([]string, 0)
	var walk func(node *Node)
	walk = func(node *Node) {
		if node.Widget != "" || Defaults[node.Kind] != "" {
			label := node.Label
			if label == "" {
				label = node.Kind
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", label, Resolve(node.Kind, node.Widget, overrides)))
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	return lines
}
