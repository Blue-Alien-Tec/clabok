package badge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/buildinfo"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module github.com/example/project\n\ngo 1.27.3\n")
	write("LICENSE", "MIT License\n\nPermission is hereby granted...\n")
	write(".git/config", `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = git@github.com:example/project.git
`)
	return dir
}

func TestResolveAll(t *testing.T) {
	dir := fixture(t)
	values, unresolved := ResolveAll(dir, []model.Badge{
		{ID: "go"},
		{ID: "module"},
		{ID: "license"},
		{ID: "remote"},
		{ID: "version"},
	})
	if len(unresolved) != 0 {
		t.Fatalf("unexpected unresolved: %+v", unresolved)
	}
	byID := map[string]Value{}
	for _, value := range values {
		byID[value.ID] = value
	}
	if byID["go"].Message != "1.27.3" || byID["go"].Source != "go.mod" {
		t.Fatalf("go badge %+v", byID["go"])
	}
	if byID["module"].Message != "github.com/example/project" {
		t.Fatalf("module badge %+v", byID["module"])
	}
	if byID["license"].Message != "MIT" || byID["license"].Link != "LICENSE" {
		t.Fatalf("license badge %+v", byID["license"])
	}
	if byID["remote"].Message != "example/project" ||
		byID["remote"].Link != "https://github.com/example/project" {
		t.Fatalf("remote badge %+v", byID["remote"])
	}
	if byID["version"].Message != buildinfo.Version {
		t.Fatalf("version badge %+v", byID["version"])
	}
}

func TestExplicitMessageWins(t *testing.T) {
	dir := fixture(t)
	value, ok, reason := Resolve(dir, model.Badge{ID: "license", Message: "Custom-1.0"})
	if !ok {
		t.Fatalf("explicit message unresolved: %s", reason)
	}
	if value.Message != "Custom-1.0" || value.Source != "explicit" {
		t.Fatalf("value %+v", value)
	}
}

func TestUnresolved(t *testing.T) {
	dir := t.TempDir()
	if _, ok, _ := Resolve(dir, model.Badge{ID: "license"}); ok {
		t.Fatal("a missing license must be unresolved")
	}
	if _, ok, reason := Resolve(dir, model.Badge{ID: "unknown"}); ok || !strings.Contains(reason, "unknown") {
		t.Fatalf("unknown badge: ok=%v reason=%q", ok, reason)
	}
	if value, ok, _ := Resolve(dir, model.Badge{ID: "unknown", Message: "X"}); !ok || value.Message != "X" {
		t.Fatalf("an explicit message must resolve an unknown detector: %+v", value)
	}
}

func TestBadgeURL(t *testing.T) {
	url := BadgeURL(Value{Label: "go", Message: "1.27.3", Color: "00ADD8"})
	if url != "https://img.shields.io/badge/go-1.27.3-00ADD8?style=flat-square" {
		t.Fatalf("url %q", url)
	}
	url = BadgeURL(Value{Label: "work in progress", Message: "a-b", Color: "orange"})
	if !strings.Contains(url, "work_in_progress-a--b-orange") {
		t.Fatalf("escaped url %q", url)
	}
}

func TestNormalizeRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:owner/repo.git":       "owner/repo",
		"https://github.com/owner/repo.git":   "owner/repo",
		"ssh://git@gitlab.com/group/sub/repo": "group/sub/repo",
	}
	for raw, want := range cases {
		message, link, ok := normalizeRemote(raw)
		if !ok || message != want || !strings.HasPrefix(link, "https://") {
			t.Fatalf("normalizeRemote(%q) = %q, %q, %v", raw, message, link, ok)
		}
	}
}
