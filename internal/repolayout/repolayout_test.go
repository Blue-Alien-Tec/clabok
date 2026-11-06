package repolayout

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestBuildTree(t *testing.T) {
	paths := []string{
		"README.md",
		"internal/main.go",
		"internal/badge/badge.go",
		"internal/badge/badge_test.go",
		"internal/render/render.go",
	}
	config := &model.LayoutConfig{
		Root:    "internal",
		Depth:   2,
		Exclude: []string{"internal/render"},
		Notes:   map[string]string{"internal/badge": "Resolves repository facts."},
	}
	want := "badge/  Resolves repository facts."
	if got := Build(paths, config); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBuildDepthAndExclude(t *testing.T) {
	paths := []string{"README.md", "internal/main.go", "internal/badge/badge.go", "internal/badge/deep/one.go"}
	depthOne := Build(paths, &model.LayoutConfig{Root: "internal", Depth: 1})
	if depthOne != "badge/" {
		t.Fatalf("depth one: %q", depthOne)
	}
	depthTwo := Build(paths, &model.LayoutConfig{Root: "internal", Depth: 2})
	if depthTwo != "badge/\n  deep/" {
		t.Fatalf("depth two: %q", depthTwo)
	}
	excluded := Build(paths, &model.LayoutConfig{
		Root: "internal", Depth: 2, Exclude: []string{"internal/badge"},
	})
	if excluded != "" {
		t.Fatalf("exclude: %q", excluded)
	}
	root := Build(paths, &model.LayoutConfig{Depth: 1})
	if root != "internal/" {
		t.Fatalf("root: %q", root)
	}
	if empty := Build(nil, nil); empty != "" {
		t.Fatalf("empty: %q", empty)
	}
}

func TestAvailable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	if Available(t.TempDir()) {
		t.Fatal("a plain directory must not be a work tree")
	}
	directory := t.TempDir()
	command := exec.Command("git", "-C", directory, "init", "-q")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	if !Available(directory) {
		t.Fatal("an initialized repository must be a work tree")
	}
}

func TestRenderWithoutGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	_, err := Render(t.TempDir(), &model.LayoutConfig{Depth: 2})
	if !errors.Is(err, ErrNoGit) {
		t.Fatalf("expected ErrNoGit, got %v", err)
	}
}

func TestRenderTracksGitFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("init", "-q")
	write := func(name string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/tracked.go")
	write("ignored/tracked.go")
	runGit("add", "src/tracked.go")

	tree, err := Render(dir, &model.LayoutConfig{Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if tree != "src/" {
		t.Fatalf("tree %q", tree)
	}
	if strings.Contains(tree, "ignored") {
		t.Fatalf("an untracked directory rendered: %q", tree)
	}
}
