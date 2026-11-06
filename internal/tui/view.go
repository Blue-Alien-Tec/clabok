package tui

import (
	"fmt"
	"strings"
)

// Binding is one documented key.
type Binding struct {
	Key         string
	Intent      Intent
	Description string
}

// Bindings lists the browse-mode keys for the hint line and the parity test.
func Bindings() []Binding {
	return []Binding{
		{Key: "up/down", Description: "move"},
		{Key: "enter", Description: "open body"},
		{Key: "e", Intent: IntentEdit, Description: "edit body"},
		{Key: "n", Description: "new node (prompt)"},
		{Key: "N", Description: "new chapter (prompt)"},
		{Key: "b", Intent: IntentNewBlock, Description: "new block"},
		{Key: "J", Intent: IntentMoveDown, Description: "move block down"},
		{Key: "K", Intent: IntentMoveUp, Description: "move block up"},
		{Key: "s", Intent: IntentSync, Description: "sync"},
		{Key: "c", Intent: IntentCheck, Description: "check"},
		{Key: "r", Intent: IntentReload, Description: "reload"},
		{Key: "o", Intent: IntentOpen, Description: "open book"},
		{Key: "x", Intent: IntentClose, Description: "close book"},
		{Key: "q", Intent: IntentQuit, Description: "quit"},
	}
}

// View renders the screen as plain ASCII lines. Each line is clipped and padded to width; the
// result has exactly height lines (at least 6).
func (model Model) View(width, height int) []string {
	if width < 24 {
		width = 24
	}
	if height < 6 {
		height = 6
	}
	if model.Mode == ModePicker {
		return model.pickerView(width, height)
	}
	header := model.header()
	if model.Mode == ModePrompt {
		header = fmt.Sprintf("%s  > %s: %s", header, model.PromptBase, model.PromptText)
	}
	contentRows := height - 5
	if contentRows < 1 {
		contentRows = 1
	}
	treeWidth := (width - 7) * 2 / 5
	if treeWidth < 12 {
		treeWidth = 12
	}
	bodyWidth := width - 7 - treeWidth
	if bodyWidth < 10 {
		bodyWidth = 10
	}
	tree := model.treeLines(contentRows)
	body := model.bodyPane(contentRows)
	lines := make([]string, 0, height)
	lines = append(lines, pad(header, width))
	lines = append(lines, "+"+strings.Repeat("-", treeWidth+2)+"+"+strings.Repeat("-", bodyWidth+2)+"+")
	for row := 0; row < contentRows; row++ {
		left, right := "", ""
		if row < len(tree) {
			left = tree[row]
		}
		if row < len(body) {
			right = body[row]
		}
		lines = append(lines, "| "+pad(left, treeWidth)+" | "+pad(right, bodyWidth)+" |")
	}
	lines = append(lines, "+"+strings.Repeat("-", treeWidth+2)+"+"+strings.Repeat("-", bodyWidth+2)+"+")
	status := model.Snapshot.Status
	if status == "" {
		status = "ready"
	}
	lines = append(lines, pad(status, width))
	lines = append(lines, pad(hint(), width))
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

func (model Model) header() string {
	state := "closed"
	if model.Snapshot.Open {
		state = "open"
	}
	book := model.Snapshot.Book
	if book == "" {
		book = "-"
	}
	output := model.Snapshot.Output
	if output == "" {
		output = "monolith"
	}
	return fmt.Sprintf("CLABOK | %s | %s | %s", book, output, state)
}

func (model Model) treeLines(rows int) []string {
	lines := make([]string, 0, len(model.Snapshot.Nodes))
	for index, node := range model.Snapshot.Nodes {
		marker := " "
		if index == model.Cursor {
			marker = ">"
		}
		indent := strings.Repeat("  ", max(node.Depth-1, 0))
		line := marker + " " + indent + node.Title
		if node.Kind != "" {
			line += " [" + node.Kind + "]"
		}
		lines = append(lines, line)
	}
	return window(lines, model.Cursor, rows)
}

func (model Model) bodyPane(rows int) []string {
	if model.Snapshot.Body == "" {
		return []string{"(no body)"}
	}
	lines := splitLines(model.Snapshot.Body)
	start := clamp(model.BodyAt, 0, max(len(lines)-1, 0))
	end := start + rows
	if end > len(lines) {
		end = len(lines)
	}
	return append([]string(nil), lines[start:end]...)
}

func (model Model) pickerView(width, height int) []string {
	lines := make([]string, 0, height)
	lines = append(lines, pad("CLABOK | pick a book", width))
	lines = append(lines, strings.Repeat("-", width))
	rows := height - 3
	if rows < 1 {
		rows = 1
	}
	entries := make([]string, 0, len(model.Picker))
	for index, entry := range model.Picker {
		marker := " "
		if index == model.Cursor {
			marker = ">"
		}
		state := "closed"
		if entry.Open {
			state = "open"
		}
		entries = append(entries, fmt.Sprintf("%s %s (%s)", marker, entry.ID, state))
	}
	entries = window(entries, model.Cursor, rows)
	for _, entry := range entries {
		lines = append(lines, pad(entry, width))
	}
	for len(lines) < height-1 {
		lines = append(lines, strings.Repeat(" ", width))
	}
	lines = append(lines, pad("up/down move  enter open  q quit", width))
	return lines
}

func hint() string {
	return "up/down move  enter body  e edit  n/N node  b block  J/K move  s sync  c check  r reload  o/x book  q quit"
}

// window returns at most rows lines around cursor, keeping the cursor visible.
func window(lines []string, cursor, rows int) []string {
	if len(lines) <= rows {
		return lines
	}
	start := cursor - rows/2
	if start < 0 {
		start = 0
	}
	if start+rows > len(lines) {
		start = len(lines) - rows
	}
	return lines[start : start+rows]
}

func splitLines(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(normalized, "\n")
}

// pad clips to width and fills with spaces.
func pad(text string, width int) string {
	runes := []rune(text)
	if len(runes) > width {
		if width <= 0 {
			return ""
		}
		return string(runes[:width])
	}
	if len(runes) == width {
		return text
	}
	return text + strings.Repeat(" ", width-len(runes))
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
