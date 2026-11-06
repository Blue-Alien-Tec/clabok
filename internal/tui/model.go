// Package tui is the interactive view over the CLABOK core. The model is pure: a snapshot and key
// events in, state and an intent out. The controller in the same package maps intents to core calls.
package tui

// Mode selects the active pane.
type Mode string

const (
	// ModePicker lists the registered books.
	ModePicker Mode = "picker"
	// ModeBrowse focuses the structure tree.
	ModeBrowse Mode = "browse"
	// ModeBody focuses the body pane.
	ModeBody Mode = "body"
	// ModePrompt shows the one-line text prompt.
	ModePrompt Mode = "prompt"
)

// Node is one structure row.
type Node struct {
	ID     string
	Title  string
	Depth  int
	Kind   string
	Blocks int
}

// PickerEntry is one book in the picker.
type PickerEntry struct {
	ID   string
	Path string
	Open bool
}

// Snapshot is the data the model renders and acts on.
type Snapshot struct {
	Book   string
	Title  string
	Output string
	Open   bool
	Nodes  []Node
	Body   string
	Status string
}

// PromptKind is the kind of value the prompt collects.
type PromptKind string

const (
	// PromptNode collects a chapter or subchapter title.
	PromptNode PromptKind = "node"
	// PromptChapter collects a chapter title.
	PromptChapter PromptKind = "chapter"
)

// Intent is a controller action requested by a key. It is empty when the key only changed the view.
type Intent string

const (
	// IntentSelect asks the controller to load the selected node's body.
	IntentSelect Intent = "select"
	// IntentEdit asks the controller to edit the selected body.
	IntentEdit Intent = "edit"
	// IntentNewNode asks the controller to create a node below the selection.
	IntentNewNode Intent = "new-node"
	// IntentNewChapter asks the controller to create a chapter at the root.
	IntentNewChapter Intent = "new-chapter"
	// IntentNewBlock asks the controller to create a body block below the selection.
	IntentNewBlock Intent = "new-block"
	// IntentMoveUp asks the controller to move the selected block up.
	IntentMoveUp Intent = "move-up"
	// IntentMoveDown asks the controller to move the selected block down.
	IntentMoveDown Intent = "move-down"
	// IntentSync asks the controller to sync the book.
	IntentSync Intent = "sync"
	// IntentCheck asks the controller to check the book.
	IntentCheck Intent = "check"
	// IntentReload asks the controller to reload the snapshot.
	IntentReload Intent = "reload"
	// IntentOpen asks the controller to open the selected book.
	IntentOpen Intent = "open"
	// IntentClose asks the controller to close the book.
	IntentClose Intent = "close"
	// IntentSubmitPrompt asks the controller to apply the prompt buffer.
	IntentSubmitPrompt Intent = "submit-prompt"
	// IntentQuit asks the controller to exit.
	IntentQuit Intent = "quit"
)

// Model is the pure UI state.
type Model struct {
	Snapshot Snapshot
	Picker   []PickerEntry
	Mode     Mode
	Cursor   int
	BodyAt   int

	Prompt     PromptKind
	PromptText string
	PromptBase string

	Quit bool
}

// NewPicker returns a picker model for the registered books.
func NewPicker(entries []PickerEntry) Model {
	return Model{Mode: ModePicker, Picker: entries}
}

// NewModel returns a browse model for the snapshot.
func NewModel(snapshot Snapshot) Model {
	return Model{Snapshot: snapshot, Mode: ModeBrowse}
}

// SetSnapshot replaces the data and keeps a valid cursor.
func (model Model) SetSnapshot(snapshot Snapshot) Model {
	model.Snapshot = snapshot
	model.Mode = ModeBrowse
	model.Cursor = clamp(model.Cursor, 0, len(snapshot.Nodes)-1)
	model.BodyAt = 0
	return model
}

// SetStatus sets the status line without changing the mode.
func (model Model) SetStatus(status string) Model {
	model.Snapshot.Status = status
	return model
}

// Selected returns the selected node.
func (model Model) Selected() (Node, bool) {
	if model.Mode == ModePicker || model.Cursor < 0 || model.Cursor >= len(model.Snapshot.Nodes) {
		return Node{}, false
	}
	return model.Snapshot.Nodes[model.Cursor], true
}

// SelectedBook returns the highlighted picker entry.
func (model Model) SelectedBook() (PickerEntry, bool) {
	if len(model.Picker) == 0 {
		return PickerEntry{}, false
	}
	return model.Picker[clamp(model.Cursor, 0, len(model.Picker)-1)], true
}

// Handle applies one key. Keys are "up", "down", "enter", "tab", "esc", "backspace", "space" or a
// single printable character.
func (model Model) Handle(key string) (Model, Intent) {
	switch model.Mode {
	case ModePicker:
		return model.handlePicker(key)
	case ModePrompt:
		return model.handlePrompt(key)
	case ModeBody:
		return model.handleBody(key)
	default:
		return model.handleBrowse(key)
	}
}

func (model Model) handlePicker(key string) (Model, Intent) {
	switch key {
	case "up":
		model.Cursor = clamp(model.Cursor-1, 0, len(model.Picker)-1)
	case "down":
		model.Cursor = clamp(model.Cursor+1, 0, len(model.Picker)-1)
	case "enter":
		if _, ok := model.SelectedBook(); ok {
			return model, IntentOpen
		}
	case "q":
		model.Quit = true
		return model, IntentQuit
	}
	return model, ""
}

func (model Model) handleBrowse(key string) (Model, Intent) {
	switch key {
	case "up", "k":
		model.Cursor = wrap(model.Cursor-1, len(model.Snapshot.Nodes))
		return model, IntentSelect
	case "down", "j":
		model.Cursor = wrap(model.Cursor+1, len(model.Snapshot.Nodes))
		return model, IntentSelect
	case "enter", "tab":
		model.Mode = ModeBody
		model.BodyAt = 0
	case "e":
		return model, IntentEdit
	case "n":
		model.Mode = ModePrompt
		model.Prompt = PromptNode
		model.PromptText = ""
		model.PromptBase = "New node title"
	case "N":
		model.Mode = ModePrompt
		model.Prompt = PromptChapter
		model.PromptText = ""
		model.PromptBase = "New chapter title"
	case "b":
		return model, IntentNewBlock
	case "J":
		return model, IntentMoveDown
	case "K":
		return model, IntentMoveUp
	case "s":
		return model, IntentSync
	case "c":
		return model, IntentCheck
	case "r":
		return model, IntentReload
	case "o":
		return model, IntentOpen
	case "x":
		return model, IntentClose
	case "q":
		model.Quit = true
		return model, IntentQuit
	}
	return model, ""
}

func (model Model) handleBody(key string) (Model, Intent) {
	switch key {
	case "up", "k":
		model.BodyAt = clamp(model.BodyAt-1, 0, bodyLines(model.Snapshot.Body))
	case "down", "j":
		model.BodyAt = clamp(model.BodyAt+1, 0, bodyLines(model.Snapshot.Body))
	case "tab", "esc":
		model.Mode = ModeBrowse
	case "e":
		return model, IntentEdit
	case "q":
		model.Quit = true
		return model, IntentQuit
	}
	return model, ""
}

func (model Model) handlePrompt(key string) (Model, Intent) {
	switch key {
	case "esc":
		model.Mode = ModeBrowse
		model.PromptText = ""
	case "enter":
		if model.PromptText == "" {
			model.Mode = ModeBrowse
			return model, ""
		}
		return model, IntentSubmitPrompt
	case "backspace":
		runes := []rune(model.PromptText)
		if len(runes) > 0 {
			model.PromptText = string(runes[:len(runes)-1])
		}
	case "space":
		model.PromptText += " "
	default:
		if len([]rune(key)) == 1 {
			model.PromptText += key
		}
	}
	return model, ""
}

func clamp(value, low, high int) int {
	if high < low {
		return low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func wrap(value, length int) int {
	if length == 0 {
		return 0
	}
	if value < 0 {
		return length - 1
	}
	if value >= length {
		return 0
	}
	return value
}

func bodyLines(text string) int {
	if text == "" {
		return 0
	}
	return len(splitLines(text)) - 1
}
