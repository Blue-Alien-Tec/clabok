package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func newService(t *testing.T) (*Service, store.BookPaths) {
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
		{ID: 10, Kind: model.KindChapter, Title: "Hello One", Blocks: []model.ID{11, 12}},
		{ID: 20, Kind: model.KindChapter, Title: "Two", Blocks: []model.ID{21}},
	}
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	container := store.New()
	_ = container.Write("manifest.json", data)
	_ = container.Write("blocks/11.md", []byte("Hello world\nHello again\n"))
	_ = container.Write("blocks/12.md", []byte("Other\n"))
	_ = container.Write("blocks/21.md", []byte("Hello there\n"))
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
	return New(life), paths
}

func TestFindBodyAndTitle(t *testing.T) {
	service, _ := newService(t)
	result, err := service.Find("fixture", "Hello", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 4 || result.Occurrences != 4 {
		t.Fatalf("result %+v", result)
	}
	if result.Matches[0].Field != "title" || result.Matches[0].ID != "10" {
		t.Fatalf("first match %+v", result.Matches[0])
	}
	bodyMatches := 0
	for _, match := range result.Matches {
		if match.Field == "body" {
			bodyMatches++
		}
	}
	if bodyMatches != 3 {
		t.Fatalf("body matches %d", bodyMatches)
	}
}

func TestFindOptions(t *testing.T) {
	service, _ := newService(t)
	result, err := service.Find("fixture", "hello", Options{Fields: []string{"body"}, IgnoreCase: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 3 {
		t.Fatalf("ignore case: %+v", result)
	}
	result, err = service.Find("fixture", "Hello", Options{Fields: []string{"body"}, Word: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 3 {
		t.Fatalf("word: %+v", result)
	}
	result, err = service.Find("fixture", "world", Options{Fields: []string{"body"}, Context: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || len(result.Matches[0].Context) != 2 {
		t.Fatalf("context: %+v", result.Matches)
	}
}

func TestFindRejects(t *testing.T) {
	service, _ := newService(t)
	if _, err := service.Find("fixture", " ", Options{}); err == nil {
		t.Fatal("expected search.empty-query")
	}
	if _, err := service.Find("fixture", "x", Options{Fields: []string{"note"}}); err == nil {
		t.Fatal("expected search.unknown-field")
	}
}

func TestFindWordBoundary(t *testing.T) {
	if count := countOccurrences("cat concatenate cat", "cat", false, true); count != 2 {
		t.Fatalf("count %d", count)
	}
	if count := countOccurrences("cat concatenate", "cat", false, false); count != 2 {
		t.Fatalf("count %d", count)
	}
}

func TestSearchFilesExist(t *testing.T) {
	service, paths := newService(t)
	if _, err := os.Stat(filepath.Join(paths.Dir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Find("fixture", "Hello", Options{}); err != nil {
		t.Fatal(err)
	}
}
