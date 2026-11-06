package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/body"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/library"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

// Editor edits a file and returns when the user is done.
type Editor func(path string) error

// Services is the core set the TUI drives. The controller adds no domain rule.
type Services struct {
	Life      *lifecycle.Lifecycle
	Structure *structure.Service
	Body      *body.Service
	Studio    *library.Service
}

// Controller maps intents to core calls and rebuilds the view snapshot.
type Controller struct {
	Services Services
	Book     model.BookID
	Model    Model
	Editor   Editor
}

type action func(*Controller) error

var actions = map[Intent]action{
	IntentSelect:       func(controller *Controller) error { return controller.reload() },
	IntentReload:       func(controller *Controller) error { return controller.reload() },
	IntentEdit:         (*Controller).editBody,
	IntentNewBlock:     (*Controller).newBlock,
	IntentMoveUp:       func(controller *Controller) error { return controller.moveBlock(-1) },
	IntentMoveDown:     func(controller *Controller) error { return controller.moveBlock(1) },
	IntentSync:         (*Controller).sync,
	IntentCheck:        (*Controller).check,
	IntentOpen:         (*Controller).openSelected,
	IntentClose:        (*Controller).closeBook,
	IntentSubmitPrompt: (*Controller).submitPrompt,
}

// New builds a controller. A non-empty book opens immediately; otherwise the picker is shown.
func New(services Services, entries []PickerEntry, book model.BookID) *Controller {
	controller := &Controller{Services: services, Book: book, Editor: SystemEditor}
	controller.Model = NewPicker(entries)
	if book != "" {
		controller.Book = ""
		if entry, ok := findEntry(entries, book); ok {
			controller.open(book, entry.Path)
		} else {
			controller.setStatus(fmt.Errorf("tui.unknown-book: book %s is not registered", book))
		}
	}
	return controller
}

// Handle applies one key, executes the intent and reports whether the loop must stop.
func (controller *Controller) Handle(key string) bool {
	model, intent := controller.Model.Handle(key)
	controller.Model = model
	switch intent {
	case "":
		return controller.Model.Quit
	case IntentQuit:
		controller.Model.Quit = true
		return true
	}
	execute, ok := actions[intent]
	if !ok {
		return controller.Model.Quit
	}
	if err := execute(controller); err != nil {
		controller.setStatus(err)
	}
	return controller.Model.Quit
}

func (controller *Controller) open(id model.BookID, path string) {
	state, err := controller.Services.Life.State(id)
	if err != nil {
		controller.setStatus(err)
		return
	}
	if !state.Extracted {
		if path == "" {
			controller.setStatus(fmt.Errorf("tui.unknown-path: book %s is not registered", id))
			return
		}
		if _, err := controller.Services.Life.Open(id, path); err != nil {
			controller.setStatus(err)
			return
		}
	}
	controller.Book = id
	controller.Model = NewModel(Snapshot{Book: string(id)})
	if err := controller.reload(); err != nil {
		controller.setStatus(err)
	}
}

func (controller *Controller) openSelected() error {
	entry, ok := controller.Model.SelectedBook()
	if !ok {
		return errors.New("tui.no-book: no book is selectable")
	}
	controller.open(model.BookID(entry.ID), entry.Path)
	return nil
}

func (controller *Controller) closeBook() error {
	if controller.Book == "" {
		return errors.New("tui.no-book: no book is open")
	}
	if _, err := controller.Services.Life.Close(controller.Book); err != nil {
		return err
	}
	controller.Model = controller.Model.SetSnapshot(Snapshot{
		Book: string(controller.Book), Open: false, Status: "closed",
	})
	return nil
}

func (controller *Controller) reload() error {
	if controller.Book == "" {
		return nil
	}
	paths, manifest, err := controller.source()
	if err != nil {
		return err
	}
	selected := ""
	if node, ok := controller.Model.Selected(); ok {
		selected = node.ID
	}
	rows, bodies := layoutRows(paths, manifest)
	bodyText := ""
	if row, ok := findRow(rows, selected); ok {
		bodyText = bodyFor(manifest, bodies, row)
	}
	model := NewModel(Snapshot{
		Book: string(controller.Book), Title: manifest.Root.Title,
		Output: string(manifest.Settings.Output), Open: true,
		Nodes: rows, Body: bodyText, Status: controller.Model.Snapshot.Status,
	})
	for index, row := range rows {
		if row.ID == selected {
			model.Cursor = index
			break
		}
	}
	controller.Model = model
	return nil
}

func (controller *Controller) source() (store.BookPaths, *model.Manifest, error) {
	paths, err := controller.Services.Life.RequireOpen(controller.Book)
	if err != nil {
		return store.BookPaths{}, nil, err
	}
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		return store.BookPaths{}, nil, err
	}
	manifest, err := model.Decode(data)
	if err != nil {
		return store.BookPaths{}, nil, err
	}
	return paths, manifest, nil
}

func (controller *Controller) editBody() error {
	block, err := controller.selectedBlock()
	if err != nil {
		return err
	}
	paths, _, err := controller.source()
	if err != nil {
		return err
	}
	current, readErr := os.ReadFile(filepath.Join(paths.BlocksDir, block.String()+".md"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	temp, err := os.CreateTemp("", "clabok-body-*.md")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	keepDraft := false
	defer func() {
		if !keepDraft {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(current); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	editor := controller.Editor
	if editor == nil {
		editor = SystemEditor
	}
	if err := editor(tempPath); err != nil {
		return err
	}
	next, err := os.ReadFile(tempPath)
	if err != nil {
		return err
	}
	if _, err := controller.Services.Body.Set(controller.Book, block, 0, string(next), false); err != nil {
		keepDraft = true
		return &draftError{path: tempPath, err: err}
	}
	return controller.reload()
}

// draftError carries the untouched editor buffer when a write is refused.
type draftError struct {
	path string
	err  error
}

func (err *draftError) Error() string { return err.err.Error() }

func (err *draftError) Unwrap() error { return err.err }

func (controller *Controller) newBlock() error {
	parent, err := controller.selectedParent()
	if err != nil {
		return err
	}
	if _, err := controller.Services.Structure.AddBlock(controller.Book, parent, false); err != nil {
		return err
	}
	return controller.reload()
}

func (controller *Controller) moveBlock(delta int) error {
	block, err := controller.selectedBlock()
	if err != nil {
		return err
	}
	_, manifest, err := controller.source()
	if err != nil {
		return err
	}
	parent, index, ok := blockParent(manifest, block)
	if !ok {
		return fmt.Errorf("tui.unknown-block: block %s is not placed", block)
	}
	next := index + delta
	if next < 0 || next >= len(parent.Blocks) {
		return nil
	}
	if _, err := controller.Services.Structure.Move(controller.Book, block,
		structure.Placement{Parent: parent.ID, Index: next}, false); err != nil {
		return err
	}
	return controller.reload()
}

func (controller *Controller) submitPrompt() error {
	text := strings.TrimSpace(controller.Model.PromptText)
	prompt := controller.Model.Prompt
	controller.Model.Mode = ModeBrowse
	controller.Model.PromptText = ""
	if text == "" {
		return nil
	}
	if prompt == PromptChapter {
		if _, err := controller.Services.Structure.Add(controller.Book, text,
			structure.Append(0), false); err != nil {
			return err
		}
		return controller.reload()
	}
	parent, err := controller.selectedChapter()
	if err != nil {
		return err
	}
	if _, err := controller.Services.Structure.Add(controller.Book, text,
		structure.Append(parent), false); err != nil {
		return err
	}
	return controller.reload()
}

func (controller *Controller) sync() error {
	result, err := controller.Services.Studio.SyncBook(controller.Book, render.SyncOptions{Write: true})
	if err != nil {
		return err
	}
	controller.setStatus(fmt.Errorf("sync: %s (%d file(s))", result.Status, len(result.Files)))
	return nil
}

func (controller *Controller) check() error {
	findings, err := controller.Services.Studio.CheckBook(controller.Book)
	if err != nil {
		return err
	}
	status := fmt.Sprintf("check: %d finding(s)", len(findings))
	for index, finding := range findings {
		if index == 3 {
			break
		}
		status += " | " + finding.Code
	}
	controller.setStatus(errors.New(status))
	return nil
}

func (controller *Controller) setStatus(err error) {
	controller.Model = controller.Model.SetStatus(statusText(err))
}

// selectedBlock returns the selected block or the first block of the selected node.
func (controller *Controller) selectedBlock() (model.ID, error) {
	row, ok := controller.Model.Selected()
	if !ok {
		return model.ZeroID, errors.New("tui.no-selection: select a node or a block")
	}
	id, err := model.ParseID(row.ID)
	if err != nil {
		return model.ZeroID, err
	}
	if row.Kind == "block" {
		return id, nil
	}
	_, manifest, err := controller.source()
	if err != nil {
		return model.ZeroID, err
	}
	node, exists := manifest.FindNode(id)
	if !exists || len(node.Blocks) == 0 {
		return model.ZeroID, fmt.Errorf("tui.no-body: %s has no body", row.Title)
	}
	return node.Blocks[0], nil
}

// selectedParent returns the node that receives a new block.
func (controller *Controller) selectedParent() (model.ID, error) {
	row, ok := controller.Model.Selected()
	if !ok {
		return model.ZeroID, errors.New("tui.no-selection: select a node or a block")
	}
	id, err := model.ParseID(row.ID)
	if err != nil {
		return model.ZeroID, err
	}
	if row.Kind != "block" {
		return id, nil
	}
	_, manifest, err := controller.source()
	if err != nil {
		return model.ZeroID, err
	}
	parent, _, ok := blockParent(manifest, id)
	if !ok {
		return model.ZeroID, fmt.Errorf("tui.unknown-block: block %s is not placed", id)
	}
	return parent.ID, nil
}

// selectedChapter returns the chapter under which a new subchapter belongs.
func (controller *Controller) selectedChapter() (model.ID, error) {
	row, ok := controller.Model.Selected()
	if !ok {
		return model.ZeroID, errors.New("tui.no-selection: select a node or a block")
	}
	_, manifest, err := controller.source()
	if err != nil {
		return model.ZeroID, err
	}
	id, err := model.ParseID(row.ID)
	if err != nil {
		return model.ZeroID, err
	}
	nodeID := id
	if row.Kind == "block" {
		parent, _, found := blockParent(manifest, id)
		if !found {
			return model.ZeroID, fmt.Errorf("tui.unknown-block: block %s is not placed", id)
		}
		nodeID = parent.ID
	}
	node, exists := manifest.FindNode(nodeID)
	if !exists {
		return model.ZeroID, fmt.Errorf("tui.unknown-node: node %s does not exist", nodeID)
	}
	if node.Kind == model.KindChapter {
		return node.ID, nil
	}
	for _, chapter := range manifest.Root.Children {
		for _, child := range chapter.Children {
			if child.ID == node.ID {
				return chapter.ID, nil
			}
		}
	}
	return model.ZeroID, errors.New("tui.no-chapter: select a chapter")
}

// SystemEditor opens the user's editor. EDITOR wins; Windows falls back to notepad.
func SystemEditor(path string) error {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "notepad"
	}
	command := exec.Command(editor, path)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}

func statusText(err error) string {
	var draft *draftError
	if errors.As(err, &draft) {
		return statusText(draft.err) + " (draft kept at " + draft.path + ")"
	}
	var bodyErr *body.Error
	if errors.As(err, &bodyErr) {
		return bodyErr.Code + ": " + bodyErr.Message
	}
	var structureErr *structure.Error
	if errors.As(err, &structureErr) {
		return structureErr.Code + ": " + structureErr.Message
	}
	var cliErr *cli.Error
	if errors.As(err, &cliErr) {
		return cliErr.Code + ": " + cliErr.Message
	}
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) {
		return coded.ErrorCode() + ": " + err.Error()
	}
	return err.Error()
}

func findEntry(entries []PickerEntry, book model.BookID) (PickerEntry, bool) {
	for _, entry := range entries {
		if entry.ID == string(book) {
			return entry, true
		}
	}
	return PickerEntry{}, false
}

func layoutRows(paths store.BookPaths, manifest *model.Manifest) ([]Node, map[model.ID]string) {
	rows := make([]Node, 0)
	bodies := map[model.ID]string{}
	var walk func(children []*model.Node, depth int)
	walk = func(children []*model.Node, depth int) {
		for _, node := range children {
			if node.Kind == model.KindLink {
				rows = append(rows, Node{
					ID: node.ID.String(), Title: node.Title, Depth: depth, Kind: "link",
				})
				continue
			}
			rows = append(rows, Node{
				ID: node.ID.String(), Title: node.Title, Depth: depth,
				Kind: string(node.Kind), Blocks: len(node.Blocks),
			})
			for _, block := range node.Blocks {
				text := readBlockBody(paths, block)
				bodies[block] = text
				preview := firstLine(text)
				if preview == "" {
					preview = "(empty)"
				}
				rows = append(rows, Node{
					ID: block.String(), Title: preview, Depth: depth + 1, Kind: "block",
				})
			}
			walk(node.Children, depth+1)
		}
	}
	walk(manifest.Root.Children, 1)
	return rows, bodies
}

func bodyFor(manifest *model.Manifest, bodies map[model.ID]string, row Node) string {
	id, err := model.ParseID(row.ID)
	if err != nil {
		return ""
	}
	if row.Kind == "block" {
		return bodies[id]
	}
	node, ok := manifest.FindNode(id)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(node.Blocks))
	for _, block := range node.Blocks {
		if text := bodies[block]; text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func readBlockBody(paths store.BookPaths, block model.ID) string {
	data, err := os.ReadFile(filepath.Join(paths.BlocksDir, block.String()+".md"))
	if err != nil {
		return ""
	}
	return string(data)
}

func firstLine(text string) string {
	line := splitLines(text)[0]
	runes := []rune(line)
	if len(runes) > 60 {
		return string(runes[:60])
	}
	return line
}

func findRow(rows []Node, id string) (Node, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return Node{}, false
}

func blockParent(manifest *model.Manifest, block model.ID) (*model.Node, int, bool) {
	var parent *model.Node
	var index int
	var found bool
	manifest.Walk(func(node *model.Node, depth int, ancestor *model.Node) bool {
		if found {
			return false
		}
		for position, candidate := range node.Blocks {
			if candidate == block {
				parent, index, found = node, position, true
				return false
			}
		}
		return true
	})
	if !found {
		return nil, 0, false
	}
	return parent, index, true
}
