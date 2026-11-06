package render

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestReadmeNavigation(t *testing.T) {
	libraryScene := newScene(t, model.OutputMonolith)
	libraryScene.service.Navigation = Navigation{
		IsLibrary: true,
		Books:     []BookLink{{Title: "Guide", Href: "Docs/guide/README.md"}},
	}
	libraryScene.service.LibraryIndex = []IndexEntry{
		{Title: "Guide", Href: "Docs/guide/README.md"},
	}
	result, err := libraryScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	library := result.Files[0].Content
	libraryNav := "[Guide](Docs/guide/README.md) · [Document Index](#document-index)"
	if strings.Count(library, libraryNav) != 2 {
		t.Fatalf("the library navigation must list the books and appear twice:\n%s", library)
	}
	if strings.Index(library, libraryNav) > strings.Index(library, "# Book") {
		t.Fatal("the top library navigation must precede the title")
	}

	bookScene := newScene(t, model.OutputMonolith)
	bookScene.service.Navigation = Navigation{
		LibraryHref: "../README.md",
		Books:       []BookLink{{Title: "Other", Href: "../other/README.md"}},
	}
	bookResult, err := bookScene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	book := bookResult.Files[0].Content
	bookNav := "[← Library](../README.md) · [Other](../other/README.md)"
	if strings.Count(book, bookNav) != 2 {
		t.Fatalf("the book navigation must list the sibling and appear twice:\n%s", book)
	}
	if !strings.Contains(book, "---\n\n[← Library](../README.md) · [Other](../other/README.md)") {
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

func TestLibraryNavigationWithoutBooks(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	scene.service.Navigation = Navigation{IsLibrary: true}
	scene.service.LibraryIndex = nil
	result, err := scene.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	content := result.Files[0].Content
	if strings.Contains(content, "Document Index") {
		t.Fatalf("an empty library must not link to a missing Document Index:\n%s", content)
	}
}
