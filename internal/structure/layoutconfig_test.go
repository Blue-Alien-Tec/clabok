package structure

import (
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestRepoLayoutLifecycle(t *testing.T) {
	scene := newScene(t)
	added, err := scene.service.AddRepoLayout("fixture", 10,
		model.LayoutConfig{Root: "internal", Depth: 2}, false)
	if err != nil {
		t.Fatal(err)
	}
	if added.Target.IsZero() {
		t.Fatalf("receipt %+v", added)
	}
	manifest := scene.manifest(t)
	meta := manifest.Meta(added.Target)
	if !meta.Locked || meta.Type != "RepoLayout" {
		t.Fatalf("block metadata %+v", meta)
	}
	if manifest.Layout == nil || manifest.Layout.Root != "internal" || manifest.Layout.Depth != 2 {
		t.Fatalf("layout %+v", manifest.Layout)
	}

	if _, err := scene.service.SetRepoLayout("fixture", model.LayoutConfig{
		Root: ".", Depth: 1, Exclude: []string{"internal/badge"},
		Notes: map[string]string{"internal": "The Go sources."},
	}, false); err != nil {
		t.Fatal(err)
	}
	config, anchor, err := scene.service.RepoLayout("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if anchor != added.Target || config.Root != "." || config.Depth != 1 ||
		len(config.Exclude) != 1 || config.Notes["internal"] != "The Go sources." {
		t.Fatalf("read back %+v (%s)", config, anchor)
	}

	before := scene.manifestBytes(t)
	if _, err := scene.service.SetRepoLayout("fixture",
		model.LayoutConfig{Root: ".", Depth: 2}, true); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(scene.manifestBytes(t)) {
		t.Fatal("a dry run wrote the manifest")
	}
}
