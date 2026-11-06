package tui

import (
	"strings"
	"testing"
)

func fixture() Snapshot {
	return Snapshot{
		Book: "guide", Title: "Guide", Output: "monolith", Open: true,
		Nodes: []Node{
			{ID: "1", Title: "One", Depth: 1, Kind: "chapter", Blocks: 1},
			{ID: "2", Title: "Sub", Depth: 2, Kind: "subchapter", Blocks: 0},
			{ID: "3", Title: "Two", Depth: 1, Kind: "chapter", Blocks: 0},
		},
		Body:   "line one\nline two\nline three",
		Status: "ready",
	}
}

func TestModelNavigation(t *testing.T) {
	model := NewModel(fixture())
	model, intent := model.Handle("down")
	if model.Cursor != 1 || intent != IntentSelect {
		t.Fatalf("down: cursor %d intent %q", model.Cursor, intent)
	}
	model, _ = model.Handle("up")
	model, _ = model.Handle("up")
	if model.Cursor != 2 {
		t.Fatalf("up wraps to the last node, got %d", model.Cursor)
	}
	if node, ok := model.Selected(); !ok || node.ID != "3" {
		t.Fatalf("selected %+v", node)
	}
}

func TestModelBodyMode(t *testing.T) {
	model := NewModel(fixture())
	model, _ = model.Handle("enter")
	if model.Mode != ModeBody {
		t.Fatalf("mode %q", model.Mode)
	}
	model, intent := model.Handle("e")
	if intent != IntentEdit {
		t.Fatalf("edit intent %q", intent)
	}
	model, _ = model.Handle("tab")
	if model.Mode != ModeBrowse {
		t.Fatalf("mode %q", model.Mode)
	}
}

func TestModelPicker(t *testing.T) {
	model := NewPicker([]PickerEntry{{ID: "a"}, {ID: "b", Open: true}})
	if _, intent := model.Handle("enter"); intent != IntentOpen {
		t.Fatalf("enter intent %q", intent)
	}
	model, _ = model.Handle("down")
	if entry, _ := model.SelectedBook(); entry.ID != "b" {
		t.Fatalf("selected %+v", entry)
	}
	model, intent := model.Handle("q")
	if !model.Quit || intent != IntentQuit {
		t.Fatalf("quit: %+v %q", model.Quit, intent)
	}
}

func TestModelPrompt(t *testing.T) {
	model := NewModel(fixture())
	model, _ = model.Handle("n")
	if model.Mode != ModePrompt {
		t.Fatalf("mode %q", model.Mode)
	}
	for _, key := range []string{"N", "e", "w", "space", "N", "o", "d", "e"} {
		model, _ = model.Handle(key)
	}
	if model.PromptText != "New Node" {
		t.Fatalf("prompt %q", model.PromptText)
	}
	model, _ = model.Handle("backspace")
	model, intent := model.Handle("enter")
	if intent != IntentSubmitPrompt || model.PromptText != "New Nod" {
		t.Fatalf("submit: %q %q", model.PromptText, intent)
	}
	model, _ = model.Handle("n")
	model, _ = model.Handle("esc")
	if model.Mode != ModeBrowse {
		t.Fatalf("escape left mode %q", model.Mode)
	}
}

func TestView(t *testing.T) {
	model := NewModel(fixture())
	lines := model.View(80, 20)
	if len(lines) != 20 {
		t.Fatalf("lines %d", len(lines))
	}
	for index, line := range lines {
		if len([]rune(line)) != 80 {
			t.Fatalf("line %d width %d: %q", index, len([]rune(line)), line)
		}
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"CLABOK | guide | monolith | open", "> One [chapter]", "line one", "ready"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in\n%s", want, joined)
		}
	}
}

func TestViewPicker(t *testing.T) {
	model := NewPicker([]PickerEntry{{ID: "alpha", Open: true}, {ID: "beta"}})
	joined := strings.Join(model.View(60, 10), "\n")
	for _, want := range []string{"pick a book", "> alpha (open)", "beta (closed)"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in\n%s", want, joined)
		}
	}
}

func TestViewClipsLongValues(t *testing.T) {
	snapshot := fixture()
	snapshot.Nodes[0].Title = strings.Repeat("x", 200)
	model := NewModel(snapshot)
	for _, line := range model.View(40, 10) {
		if len([]rune(line)) != 40 {
			t.Fatalf("line width %d: %q", len([]rune(line)), line)
		}
	}
}

func TestBindingsCoverIntents(t *testing.T) {
	wanted := map[Intent]bool{
		IntentEdit: false, IntentNewBlock: false, IntentMoveUp: false,
		IntentMoveDown: false, IntentSync: false, IntentCheck: false, IntentReload: false,
		IntentOpen: false, IntentClose: false, IntentQuit: false,
	}
	for _, binding := range Bindings() {
		if _, ok := wanted[binding.Intent]; ok {
			wanted[binding.Intent] = true
		}
	}
	for intent, found := range wanted {
		if !found {
			t.Fatalf("no binding for %q", intent)
		}
	}
}
