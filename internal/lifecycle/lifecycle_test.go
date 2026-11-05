package lifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/rollback"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func saveBook(t *testing.T, layout store.Layout, id model.BookID, chapterID, blockID model.ID, body string) store.BookPaths {
	t.Helper()
	paths, err := layout.Book(id)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.NewManifest(id, "Fixture", model.NewSequenceAllocator(1))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Root.Children = []*model.Node{{
		ID:     chapterID,
		Kind:   model.KindChapter,
		Title:  "One",
		Blocks: []model.ID{blockID},
	}}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	if err := container.Write("manifest.json", data); err != nil {
		t.Fatal(err)
	}
	if err := container.Write("blocks/"+blockID.String()+".md", []byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := container.Save(paths.Container); err != nil {
		t.Fatal(err)
	}
	return paths
}

type scene struct {
	life   *Lifecycle
	layout store.Layout
	paths  store.BookPaths
}

func newScene(t *testing.T) scene {
	return newSceneWithOptions(t, Options{UndoLimit: 10, BookPointLimit: 3})
}

func newSceneWithOptions(t *testing.T, options Options) scene {
	t.Helper()
	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	locks.Command = "test"
	paths := saveBook(t, layout, "fixture", 10, 11, "Body one\n")
	return scene{life: New(layout, locks, options), layout: layout, paths: paths}
}

func TestOpenExtracts(t *testing.T) {
	s := newScene(t)
	result, err := s.life.Open("fixture", "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if result.Entry.OutputPath != "README.md" || result.Entry.Session == "" || result.Entry.PID == 0 {
		t.Fatalf("entry %+v", result.Entry)
	}
	if _, err := os.Stat(filepath.Join(s.paths.Dir, "manifest.json")); err != nil {
		t.Fatalf("manifest not extracted: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(s.paths.BlocksDir, "11.md"))
	if err != nil {
		t.Fatalf("body not extracted: %v", err)
	}
	if string(body) != "Body one\n" {
		t.Fatalf("body %q", body)
	}
	state, err := s.life.State("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusOpen || !state.HasEntry || !state.Extracted {
		t.Fatalf("state %+v", state)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	s := newScene(t)
	first, err := s.life.Open("fixture", "README.md")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.life.Open("fixture", "OTHER.md")
	if err != nil {
		t.Fatal(err)
	}
	if second.Entry.Session != first.Entry.Session || second.Entry.OutputPath != "README.md" {
		t.Fatalf("second open changed the entry: %+v", second.Entry)
	}
	file, err := s.life.readOpenFile()
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Books) != 1 {
		t.Fatalf("duplicate open entries: %+v", file.Books)
	}
}

func TestCloseRepacksUnchanged(t *testing.T) {
	s := newScene(t)
	before, err := os.ReadFile(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Open("fixture", "README.md"); err != nil {
		t.Fatal(err)
	}
	result, err := s.life.Close("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Repacked || result.Recovered {
		t.Fatalf("result %+v", result)
	}
	after, err := os.ReadFile(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("an unchanged open/close round trip changed the container bytes")
	}
	if _, err := os.Stat(filepath.Join(s.paths.Dir, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("manifest still extracted: %v", err)
	}
	if _, err := os.Stat(s.paths.BlocksDir); !os.IsNotExist(err) {
		t.Fatalf("blocks still extracted: %v", err)
	}
	if _, err := os.Stat(s.layout.OpenState()); !os.IsNotExist(err) {
		t.Fatalf("open state still present: %v", err)
	}
}

func TestCloseAfterEdit(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	container, err := store.Load(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := container.Text("blocks/11.md")
	if !ok || body != "Edited\n" {
		t.Fatalf("body %q exists %t", body, ok)
	}
}

func TestCloseWithoutOpen(t *testing.T) {
	s := newScene(t)
	before, err := os.ReadFile(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.life.Close("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if result.Repacked || result.EntryRemoved || result.Recovered {
		t.Fatalf("result %+v", result)
	}
	after, err := os.ReadFile(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("close on a closed book changed the container")
	}
}

func TestCloseCreatesEmptyBody(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.paths.BlocksDir, "11.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatalf("a placed block without a body is an empty body: %v", err)
	}
	container, err := store.Load(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := container.Text("blocks/11.md")
	if !ok || body != "" {
		t.Fatalf("body %q exists %t", body, ok)
	}
}

func TestTwoBooks(t *testing.T) {
	s := newScene(t)
	saveBook(t, s.layout, "second", 20, 21, "Body two\n")
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Open("second", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	state, err := s.life.State("second")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusOpen {
		t.Fatalf("closing one book changed the other: %+v", state)
	}
	if _, err := os.Stat(filepath.Join(s.paths.Dir, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("closed book still extracted: %v", err)
	}
	if _, err := s.life.Close("second"); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRecoversUnsaved(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.layout.OpenState()); err != nil {
		t.Fatal(err)
	}
	state, err := s.life.State("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusUnsaved {
		t.Fatalf("state %+v", state)
	}
	opened, err := s.life.Open("fixture", "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !opened.Recovered {
		t.Fatal("open did not report the recovery")
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestCloseFromUnsaved(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.layout.OpenState()); err != nil {
		t.Fatal(err)
	}
	result, err := s.life.Close("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Repacked || !result.Recovered || result.EntryRemoved {
		t.Fatalf("result %+v", result)
	}
}

func TestOpenRecoversOrphaned(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(s.paths.BlocksDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.paths.Dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	state, err := s.life.State("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusOrphaned {
		t.Fatalf("state %+v", state)
	}
	opened, err := s.life.Open("fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	if !opened.Recovered {
		t.Fatal("open did not report the recovery")
	}
	if _, err := os.Stat(filepath.Join(s.paths.Dir, "manifest.json")); err != nil {
		t.Fatalf("manifest not re-extracted: %v", err)
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestCloseFromOrphaned(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(s.paths.BlocksDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.paths.Dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	result, err := s.life.Close("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if result.Repacked || !result.EntryRemoved || !result.Recovered {
		t.Fatalf("result %+v", result)
	}
	state, err := s.life.State("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusClosed {
		t.Fatalf("state %+v", state)
	}
}

func TestRequireOpenAndClosed(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.RequireOpen("fixture"); err == nil {
		t.Fatal("expected lifecycle.not-open")
	} else if lifecycleErr, ok := err.(*Error); !ok || lifecycleErr.Code != "lifecycle.not-open" {
		t.Fatalf("unexpected error %v", err)
	}
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.RequireOpen("fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.life.RequireClosed("fixture"); err == nil {
		t.Fatal("expected lifecycle.open")
	} else if lifecycleErr, ok := err.(*Error); !ok || lifecycleErr.Code != "lifecycle.open" {
		t.Fatalf("unexpected error %v", err)
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.life.RequireClosed("fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestCloseRejectsInvalid(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "99.md"), []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	_, closeErr := s.life.Close("fixture")
	var validation *model.ValidationError
	if !errors.As(closeErr, &validation) {
		t.Fatalf("expected a validation error, got %v", closeErr)
	}
	after, err := os.ReadFile(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a rejected close wrote the container")
	}
	if _, err := os.Stat(filepath.Join(s.paths.Dir, "manifest.json")); err != nil {
		t.Fatalf("a rejected close removed the extracted state: %v", err)
	}
}

func TestOpenMissingContainer(t *testing.T) {
	layout, err := store.NewLayout(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	locks, err := coordinator.NewFile(layout.Coordinator())
	if err != nil {
		t.Fatal(err)
	}
	life := New(layout, locks, Options{UndoLimit: 10, BookPointLimit: 3})
	if _, err := life.Open("absent", ""); err == nil {
		t.Fatal("expected lifecycle.no-container")
	} else if lifecycleErr, ok := err.(*Error); !ok || lifecycleErr.Code != "lifecycle.no-container" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestStateVersion(t *testing.T) {
	s := newScene(t)
	if err := os.WriteFile(s.layout.OpenState(), []byte(`{"version":2,"books":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.State("fixture"); err == nil {
		t.Fatal("expected lifecycle.state-version")
	} else if lifecycleErr, ok := err.(*Error); !ok || lifecycleErr.Code != "lifecycle.state-version" {
		t.Fatalf("unexpected error %v", err)
	}
}

func packagesOf(t *testing.T, life *Lifecycle, id model.BookID, kind rollback.Kind) []rollback.Package {
	t.Helper()
	rollbacks, err := life.RollbackStore(id)
	if err != nil {
		t.Fatal(err)
	}
	all, err := rollbacks.List()
	if err != nil {
		t.Fatal(err)
	}
	filtered := make([]rollback.Package, 0, len(all))
	for _, pkg := range all {
		if pkg.Kind == kind {
			filtered = append(filtered, pkg)
		}
	}
	return filtered
}

func TestSnapshotCreatesUndoPackage(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	pkg, err := s.life.Snapshot("fixture", "body.set")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Kind != rollback.KindUndo || pkg.Name == "" || pkg.Reason != "body.set" {
		t.Fatalf("package %+v", pkg)
	}
	if packages := packagesOf(t, s.life, "fixture", rollback.KindUndo); len(packages) != 1 {
		t.Fatalf("packages %+v", packages)
	}
	rollbacks, err := s.life.RollbackStore("fixture")
	if err != nil {
		t.Fatal(err)
	}
	_, members, err := rollbacks.Load(pkg.Name)
	if err != nil {
		t.Fatal(err)
	}
	if string(members["blocks/11.md"]) != "Body one\n" {
		t.Fatalf("members %v", members)
	}
}

func TestUndoLimit(t *testing.T) {
	s := newSceneWithOptions(t, Options{UndoLimit: 2, BookPointLimit: 3})
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		if _, err := s.life.Snapshot("fixture", fmt.Sprintf("edit-%d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if packages := packagesOf(t, s.life, "fixture", rollback.KindUndo); len(packages) != 2 {
		t.Fatalf("packages %+v", packages)
	}
}

func TestCloseRemovesUndoPackages(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Snapshot("fixture", "edit"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	if packages := packagesOf(t, s.life, "fixture", rollback.KindUndo); len(packages) != 0 {
		t.Fatalf("undo packages survived close: %+v", packages)
	}
}

func TestCloseCreatesBookPoint(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := s.life.Close("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !result.BookPoint {
		t.Fatal("close did not report the book point")
	}
	points := packagesOf(t, s.life, "fixture", rollback.KindBook)
	if len(points) != 1 {
		t.Fatalf("points %+v", points)
	}
	rollbacks, err := s.life.RollbackStore("fixture")
	if err != nil {
		t.Fatal(err)
	}
	_, members, err := rollbacks.Load(points[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if string(members["blocks/11.md"]) != "Body one\n" {
		t.Fatalf("book point body %q", members["blocks/11.md"])
	}
}

func TestCloseUnchangedCreatesNoPoint(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	result, err := s.life.Close("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if result.BookPoint {
		t.Fatal("unchanged close created a book point")
	}
	if points := packagesOf(t, s.life, "fixture", rollback.KindBook); len(points) != 0 {
		t.Fatalf("points %+v", points)
	}
}

func TestBookPointCap(t *testing.T) {
	s := newSceneWithOptions(t, Options{UndoLimit: 10, BookPointLimit: 2})
	for index := 0; index < 3; index++ {
		if _, err := s.life.Open("fixture", ""); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf("Edit %d\n", index)
		if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := s.life.Close("fixture"); err != nil {
			t.Fatal(err)
		}
	}
	if points := packagesOf(t, s.life, "fixture", rollback.KindBook); len(points) != 2 {
		t.Fatalf("points %+v", points)
	}
}

func TestRestoreUndo(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	pkg, err := s.life.Snapshot("fixture", "before-edit")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := s.life.Restore("fixture", pkg.Name)
	if err != nil {
		t.Fatal(err)
	}
	if result.Members != 2 {
		t.Fatalf("members %d", result.Members)
	}
	body, err := os.ReadFile(filepath.Join(s.paths.BlocksDir, "11.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Body one\n" {
		t.Fatalf("restored body %q", body)
	}
	packages := packagesOf(t, s.life, "fixture", rollback.KindUndo)
	if len(packages) != 2 {
		t.Fatalf("packages %+v", packages)
	}
	rollbacks, err := s.life.RollbackStore("fixture")
	if err != nil {
		t.Fatal(err)
	}
	_, members, err := rollbacks.Load(packages[len(packages)-1].Name)
	if err != nil {
		t.Fatal(err)
	}
	if string(members["blocks/11.md"]) != "Edited\n" {
		t.Fatalf("pre-restore snapshot %q", members["blocks/11.md"])
	}
}

func TestRestoreBook(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	points := packagesOf(t, s.life, "fixture", rollback.KindBook)
	if len(points) != 1 {
		t.Fatalf("points %+v", points)
	}
	if _, err := s.life.Restore("fixture", points[0].Name); err != nil {
		t.Fatal(err)
	}
	container, err := store.Load(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := container.Text("blocks/11.md")
	if body != "Body one\n" {
		t.Fatalf("restored container body %q", body)
	}
	points = packagesOf(t, s.life, "fixture", rollback.KindBook)
	if len(points) != 2 {
		t.Fatalf("points after restore %+v", points)
	}
}

func TestRestoreUndoWhileClosed(t *testing.T) {
	s := newScene(t)
	container, err := store.Load(s.paths.Container)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := container.Read("manifest.json")
	rollbacks, err := s.life.RollbackStore("fixture")
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := rollbacks.Snapshot(rollback.KindUndo, map[string][]byte{"manifest.json": manifest}, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, restoreErr := s.life.Restore("fixture", pkg.Name)
	if restoreErr == nil {
		t.Fatal("expected lifecycle.not-open")
	} else if lifecycleErr, ok := restoreErr.(*Error); !ok || lifecycleErr.Code != "lifecycle.not-open" {
		t.Fatalf("unexpected error %v", restoreErr)
	}
}

func TestRestoreBookWhileOpen(t *testing.T) {
	s := newScene(t)
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, "11.md"), []byte("Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.life.Close("fixture"); err != nil {
		t.Fatal(err)
	}
	points := packagesOf(t, s.life, "fixture", rollback.KindBook)
	if len(points) != 1 {
		t.Fatal("missing book point")
	}
	if _, err := s.life.Open("fixture", ""); err != nil {
		t.Fatal(err)
	}
	_, restoreErr := s.life.Restore("fixture", points[0].Name)
	if restoreErr == nil {
		t.Fatal("expected lifecycle.open")
	} else if lifecycleErr, ok := restoreErr.(*Error); !ok || lifecycleErr.Code != "lifecycle.open" {
		t.Fatalf("unexpected error %v", restoreErr)
	}
}
