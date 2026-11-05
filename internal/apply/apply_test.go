package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/rollback"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	if applyErr, ok := err.(*Error); !ok || applyErr.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func TestParseValidPlan(t *testing.T) {
	document := `{"version":1,"steps":[
		{"op":"add","title":"One","parent":"10","index":0},
		{"op":"add","title":"Two"},
		{"op":"rename","target":"10","title":"Eins"},
		{"op":"move","target":"11","childOf":"root"},
		{"op":"remove","target":"12","recursive":true},
		{"op":"type","target":"11","type":"UILayout"},
		{"op":"setting","key":"numbering","value":"on"}
	]}`
	steps, err := Parse([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 7 {
		t.Fatalf("steps %d", len(steps))
	}
	if steps[0].Placement.Parent != 10 || steps[0].Placement.Index != 0 {
		t.Fatalf("first step %+v", steps[0])
	}
	if steps[3].Placement.Parent != model.ZeroID {
		t.Fatalf("root placement %+v", steps[3])
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name     string
		document string
		code     string
	}{
		{"version missing", `{}`, "apply.version"},
		{"version future", `{"version":2,"steps":[{"op":"add"}]}`, "apply.version"},
		{"empty", `{"version":1,"steps":[]}`, "apply.empty-plan"},
		{"unknown op", `{"version":1,"steps":[{"op":"bogus"}]}`, "apply.invalid-step"},
		{"bad id", `{"version":1,"steps":[{"op":"rename","target":"x","title":"T"}]}`, "apply.invalid-step"},
		{"before and after", `{"version":1,"steps":[{"op":"add","title":"T","before":"1","after":"2"}]}`, "apply.invalid-step"},
		{"parent and childOf", `{"version":1,"steps":[{"op":"add","title":"T","parent":"1","childOf":"2"}]}`, "apply.invalid-step"},
		{"unknown field", `{"version":1,"steps":[{"op":"add","bogus":1}]}`, "apply.invalid-plan"},
		{"move without destination", `{"version":1,"steps":[{"op":"move","target":"1"}]}`, "apply.invalid-step"},
	}
	for _, testCase := range cases {
		_, err := Parse([]byte(testCase.document))
		if err == nil {
			t.Fatalf("%s: expected an error", testCase.name)
		}
		expectCode(t, err, testCase.code)
	}
}

func newScene(t *testing.T) (*structure.Service, *lifecycle.Lifecycle, store.BookPaths) {
	t.Helper()
	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := lifecycle.New(layout, locks, lifecycle.Options{UndoLimit: 10, BookPointLimit: 3})
	manifest, err := model.NewManifest("fixture", "Fixture", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children = []*model.Node{
		{ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{11}},
		{ID: 20, Kind: model.KindChapter, Title: "Two", Blocks: []model.ID{21}},
	}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	_ = container.Write("manifest.json", data)
	_ = container.Write("blocks/11.md", []byte("Body 11\n"))
	_ = container.Write("blocks/21.md", []byte("Body 21\n"))
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	if _, err := life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	return structure.New(life, model.NewSequenceAllocator(100)), life, paths
}

func manifestOf(t *testing.T, paths store.BookPaths) *model.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func undoCount(t *testing.T, life *lifecycle.Lifecycle, book model.BookID) int {
	t.Helper()
	rollbacks, err := life.RollbackStore(book)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := rollbacks.List()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, pkg := range packages {
		if pkg.Kind == rollback.KindUndo {
			count++
		}
	}
	return count
}

func TestExecuteBatch(t *testing.T) {
	service, life, paths := newScene(t)
	steps, err := Parse([]byte(`{"version":1,"steps":[
		{"op":"add","title":"Sub","parent":"10"},
		{"op":"setting","key":"numbering","value":"on"},
		{"op":"move","target":"11","childOf":"20"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := service.Execute("fixture", steps, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 3 {
		t.Fatalf("receipts %d", len(receipts))
	}
	manifest := manifestOf(t, paths)
	first, _ := manifest.FindNode(10)
	if len(first.Children) != 1 || first.Children[0].Title != "Sub" {
		t.Fatalf("chapter 10 %+v", first)
	}
	second, _ := manifest.FindNode(20)
	if len(second.Blocks) != 2 || second.Blocks[1] != 11 {
		t.Fatalf("chapter 20 blocks %v", second.Blocks)
	}
	if !manifest.Settings.Numbering.Enabled {
		t.Fatal("numbering not enabled")
	}
	if undo := undoCount(t, life, "fixture"); undo != 1 {
		t.Fatalf("undo packages %d, want 1", undo)
	}
}

func TestExecuteAtomicOnFailure(t *testing.T) {
	service, life, paths := newScene(t)
	before, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	steps, err := Parse([]byte(`{"version":1,"steps":[
		{"op":"add","title":"New"},
		{"op":"move","target":"10","childOf":"10"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Execute("fixture", steps, false); err == nil {
		t.Fatal("expected the plan to fail")
	}
	after, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a failed plan wrote the manifest")
	}
	if undo := undoCount(t, life, "fixture"); undo != 0 {
		t.Fatalf("undo packages %d after a failed plan", undo)
	}
}

func TestExecuteDryRun(t *testing.T) {
	service, life, paths := newScene(t)
	before, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	steps, err := Parse([]byte(`{"version":1,"steps":[{"op":"add","title":"Dry"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := service.Execute("fixture", steps, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || !receipts[0].Changed {
		t.Fatalf("receipts %+v", receipts)
	}
	after, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a dry run wrote the manifest")
	}
	if undo := undoCount(t, life, "fixture"); undo != 0 {
		t.Fatalf("undo packages %d after a dry run", undo)
	}
}

func runCommand(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(directory)
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	structure.Register(registry)
	Register(registry)
	var stdout, stderr strings.Builder
	code := cli.Run(registry, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func prepareProject(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	layout, err := store.NewLayout(filepath.Join(directory, "_CLABOK"))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := layout.Book("fixture")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.NewManifest("fixture", "Fixture", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children = []*model.Node{
		{ID: 10, Kind: model.KindChapter, Title: "One", Blocks: []model.ID{11}},
	}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	_ = container.Write("manifest.json", data)
	_ = container.Write("blocks/11.md", []byte("Body 11\n"))
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCommand(t, directory, "book", "open", "fixture"); code != cli.ExitOK {
		t.Fatalf("open failed: %s", stderr)
	}
	return directory
}

func TestApplyCommand(t *testing.T) {
	directory := prepareProject(t)
	planPath := filepath.Join(directory, "plan.json")
	document := `{"version":1,"steps":[
		{"op":"add","title":"Two"},
		{"op":"setting","key":"numbering","value":"on"}
	]}`
	if err := os.WriteFile(planPath, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCommand(t, directory, "apply", "--book", "fixture", "--file", planPath)
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var envelope struct {
		Data struct {
			Steps int `json:"steps"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Steps != 2 {
		t.Fatalf("steps %d", envelope.Data.Steps)
	}
}

func TestApplyCommandDryRun(t *testing.T) {
	directory := prepareProject(t)
	planPath := filepath.Join(directory, "plan.json")
	if err := os.WriteFile(planPath, []byte(`{"version":1,"steps":[{"op":"add","title":"Dry"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCommand(t, directory, "apply", "--book", "fixture", "--file", planPath, "--dry-run")
	if code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"dryRun": true`) {
		t.Fatalf("unexpected receipt %s", stdout)
	}
}
