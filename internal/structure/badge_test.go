package structure

import (
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestBadgeLifecycle(t *testing.T) {
	scene := newScene(t)
	added, err := scene.service.SetBadge("fixture", model.Badge{ID: "go"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !added.Changed {
		t.Fatalf("receipt %+v", added)
	}
	if badges := scene.manifest(t).Badges; len(badges) != 1 || badges[0].ID != "go" {
		t.Fatalf("badges %+v", badges)
	}

	replaced, err := scene.service.SetBadge("fixture",
		model.Badge{ID: "go", Label: "Go", Message: "1.99", Link: "https://go.dev"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !replaced.Changed {
		t.Fatalf("receipt %+v", replaced)
	}
	badges := scene.manifest(t).Badges
	if len(badges) != 1 || badges[0].Message != "1.99" || badges[0].Link != "https://go.dev" {
		t.Fatalf("badges %+v", badges)
	}

	before := scene.manifestBytes(t)
	if _, err := scene.service.SetBadge("fixture", model.Badge{ID: "license"}, true); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(scene.manifestBytes(t)) {
		t.Fatal("a dry run wrote the manifest")
	}

	removed, err := scene.service.RemoveBadge("fixture", "go", false)
	if err != nil {
		t.Fatal(err)
	}
	if !removed.Changed || len(scene.manifest(t).Badges) != 0 {
		t.Fatalf("remove receipt %+v", removed)
	}
	missing, err := scene.service.RemoveBadge("fixture", "license", false)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Changed {
		t.Fatalf("removing an absent badge reported a change: %+v", missing)
	}
}
