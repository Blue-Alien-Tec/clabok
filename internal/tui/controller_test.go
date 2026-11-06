package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/body"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/idspace"
	"github.com/Blue-Alien-Tec/clabok/internal/library"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

func commandRegistry() *cli.Registry {
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	library.Register(registry)
	return registry
}

func run(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := cli.Run(commandRegistry(), append([]string{"--project", directory}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func runOK(t *testing.T, directory string, args ...string) {
	t.Helper()
	code, stdout, stderr := run(t, directory, args...)
	if code != cli.ExitOK {
		t.Fatalf("%v: exit %d\n%s%s", args, code, stdout, stderr)
	}
}

func fixtureController(t *testing.T) (*Controller, string) {
	t.Helper()
	directory := t.TempDir()
	runOK(t, directory, "book", "new", "--library", "--id", "library", "--title", "CLABOK")
	runOK(t, directory, "book", "new", "--title", "Guide")

	values := &config.Values{
		ProjectDir: directory, StoreRoot: filepath.Join(directory, "_CLABOK"), DocsRoot: "Docs",
		UndoLimit: 10, BookPointLimit: 3,
	}
	layout, err := store.NewLayout(values.StoreRoot)
	if err != nil {
		t.Fatal(err)
	}
	coordinatorBackend, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := lifecycle.New(layout, coordinatorBackend, lifecycle.Options{UndoLimit: 10, BookPointLimit: 3})
	allocator := idspace.New(life, layout)
	services := Services{
		Life:      life,
		Structure: structure.New(life, allocator),
		Body:      body.NewWithAllocator(life, allocator),
		Studio: &library.Service{
			Life: life, Layout: layout, Values: values, IDs: allocator,
		},
	}
	registry, err := store.LoadRegistry(layout.Registry())
	if err != nil {
		t.Fatal(err)
	}
	controller := New(services, pickerEntries(registry, life), "guide")
	return controller, directory
}

func selectTitle(t *testing.T, controller *Controller, title string) {
	t.Helper()
	for index, row := range controller.Model.Snapshot.Nodes {
		if row.Title == title {
			controller.Model.Cursor = index
			return
		}
	}
	t.Fatalf("no row %q in %+v", title, controller.Model.Snapshot.Nodes)
}

func selectKind(t *testing.T, controller *Controller, kind string) Node {
	t.Helper()
	for index, row := range controller.Model.Snapshot.Nodes {
		if row.Kind == kind {
			controller.Model.Cursor = index
			return row
		}
	}
	t.Fatalf("no %q row in %+v", kind, controller.Model.Snapshot.Nodes)
	return Node{}
}

func TestControllerRoundTrip(t *testing.T) {
	controller, directory := fixtureController(t)
	if controller.Book != "guide" {
		t.Fatalf("book %q status %q", controller.Book, controller.Model.Snapshot.Status)
	}

	controller.Handle("N")
	controller.Model.PromptText = "Chapter"
	controller.Handle("enter")
	selectTitle(t, controller, "Chapter")

	controller.Handle("n")
	controller.Model.PromptText = "Sub"
	controller.Handle("enter")
	selectTitle(t, controller, "Sub")

	controller.Handle("b")
	block := selectKind(t, controller, "block")

	controller.Editor = func(path string) error {
		return os.WriteFile(path, []byte("Hello body"), 0o644)
	}
	controller.Handle("e")
	paths, err := controller.Services.Life.RequireOpen("guide")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(paths.BlocksDir, block.ID+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Hello body" {
		t.Fatalf("body %q", data)
	}

	controller.Handle("c")
	if !strings.Contains(controller.Model.Snapshot.Status, "check:") {
		t.Fatalf("check status %q", controller.Model.Snapshot.Status)
	}

	controller.Handle("b")
	selectTitle(t, controller, "Hello body")
	controller.Handle("J")
	emptyAt, helloAt := -1, -1
	for index, row := range controller.Model.Snapshot.Nodes {
		switch row.Title {
		case "(empty)":
			emptyAt = index
		case "Hello body":
			helloAt = index
		}
	}
	if emptyAt < 0 || helloAt < 0 || emptyAt > helloAt {
		t.Fatalf("move order is wrong: empty %d hello %d", emptyAt, helloAt)
	}

	controller.Handle("s")
	if !strings.Contains(controller.Model.Snapshot.Status, "sync") {
		t.Fatalf("sync status %q", controller.Model.Snapshot.Status)
	}
	readme, err := os.ReadFile(filepath.Join(directory, "Docs", "guide", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "## Chapter") || !strings.Contains(string(readme), "Hello body") {
		t.Fatalf("rendered book is incomplete:\n%s", readme)
	}

	controller.Handle("x")
	if controller.Model.Snapshot.Open {
		t.Fatalf("close state %q", controller.Model.Snapshot.Status)
	}
}

func TestDraftKeptOnWriteFailure(t *testing.T) {
	controller, _ := fixtureController(t)
	controller.Handle("N")
	controller.Model.PromptText = "Chapter"
	controller.Handle("enter")
	selectTitle(t, controller, "Chapter")
	controller.Handle("b")
	selectKind(t, controller, "block")

	var draftPath string
	controller.Editor = func(path string) error {
		if err := os.WriteFile(path, []byte("Draft body"), 0o644); err != nil {
			return err
		}
		draftPath = path
		_, err := controller.Services.Life.Close("guide")
		return err
	}
	controller.Handle("e")
	status := controller.Model.Snapshot.Status
	if !strings.Contains(status, "draft kept at") {
		t.Fatalf("status %q", status)
	}
	if draftPath == "" {
		t.Fatal("no draft path")
	}
	defer os.Remove(draftPath)
	data, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Draft body" {
		t.Fatalf("draft %q", data)
	}
}

func TestActionsCoverIntents(t *testing.T) {
	for intent, action := range actions {
		if action == nil {
			t.Fatalf("intent %q has no action", intent)
		}
	}
	for _, binding := range Bindings() {
		if binding.Intent == "" || binding.Intent == IntentQuit {
			continue
		}
		if _, ok := actions[binding.Intent]; !ok {
			t.Fatalf("binding %q has no core action", binding.Key)
		}
	}
	for _, intent := range []Intent{
		IntentSelect, IntentReload, IntentEdit, IntentNewBlock, IntentMoveUp, IntentMoveDown,
		IntentSync, IntentCheck, IntentOpen, IntentClose, IntentSubmitPrompt,
	} {
		if actions[intent] == nil {
			t.Fatalf("core intent %q has no action", intent)
		}
	}
}

func TestNoTerminal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Fatal("a regular file must not be treated as a terminal")
	}
}

func TestPayloadText(t *testing.T) {
	if text := (payload{Book: "guide"}).Text(); !strings.Contains(text, "guide") {
		t.Fatalf("payload text %q", text)
	}
}
