package render

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestReadmeNavigation(t *testing.T) {
	libraryScene := newScene(t, model.OutputMonolith)
	libraryScene.service.Navigation = Navigation{IsLibrary: true}
	libraryScene.service.LibraryIndex = []IndexEntry{
		{Title: "Guide", Href: "Docs/guide/README.md"},
	}
	result, err := libraryScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	library := result.Files[0].Content
	if strings.Count(library, "[Document Index](#document-index)") != 2 {
		t.Fatalf("the library navigation must appear at the top and bottom:\n%s", library)
	}
	if strings.Index(library, "[Document Index](#document-index)") > strings.Index(library, "# Book") {
		t.Fatal("the top library navigation must precede the title")
	}

	bookScene := newScene(t, model.OutputMonolith)
	bookScene.service.Navigation = Navigation{LibraryHref: "../README.md"}
	bookResult, err := bookScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	book := bookResult.Files[0].Content
	if strings.Count(book, "[← Library](../README.md)") != 2 {
		t.Fatalf("the book navigation must appear at the top and bottom:\n%s", book)
	}
	if !strings.Contains(book, "---\n\n[← Library](../README.md)") {
		t.Fatalf("the bottom navigation needs its rule:\n%s", book)
	}
}

func TestSplitFileNavigation(t *testing.T) {
	scene := newScene(t, model.OutputChapter)
	scene.service.Navigation = Navigation{LibraryHref: "../README.md"}
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	one := scene.file(t, result, "one.md")
	if !strings.Contains(one, "---\n\n[Index](README.md) · [Next →](two.md)") {
		t.Fatalf("the first chapter navigation is wrong:\n%s", one)
	}
	if strings.Contains(one, "[← Previous]") {
		t.Fatalf("the first chapter must omit Previous:\n%s", one)
	}
	if strings.Count(one, "[Index](README.md) · [Next →](two.md)") != 2 {
		t.Fatalf("navigation must appear at the top and bottom:\n%s", one)
	}
	two := scene.file(t, result, "two.md")
	if !strings.Contains(two, "[← Previous](one.md) · [Index](README.md)") {
		t.Fatalf("the last chapter navigation is wrong:\n%s", two)
	}
	if strings.Contains(two, "[Next →]") {
		t.Fatalf("the last chapter must omit Next:\n%s", two)
	}
	readme := scene.file(t, result, "README.md")
	if !strings.Contains(readme, "[← Library](../README.md)") {
		t.Fatalf("the chapter README misses the library link:\n%s", readme)
	}
	if !strings.Contains(readme, "[← Library](../README.md) · [First document →](one.md)") {
		t.Fatalf("the chapter README misses the first-document link:\n%s", readme)
	}
}
