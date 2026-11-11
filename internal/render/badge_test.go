package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

func TestBadgeLineOrderAndGroups(t *testing.T) {
	service := &Service{}
	manifest := &model.Manifest{Badges: []model.Badge{
		{ID: "c", Message: "3", Order: 3, Group: "apps"},
		{ID: "a", Message: "1", Order: 2, Group: "stack"},
		{ID: "b", Message: "2", Order: 1},
	}}
	line := service.badgeMarkdown(manifest)
	parts := strings.Split(line, "\n")
	if len(parts) != 3 {
		t.Fatalf("badge lines %q", line)
	}
	if !strings.Contains(parts[0], "![b: 2]") || !strings.Contains(parts[1], "![a: 1]") ||
		!strings.Contains(parts[2], "![c: 3]") {
		t.Fatalf("order %q", parts)
	}
}

func TestBadgeURLUsesPresentationFields(t *testing.T) {
	value := model.Badge{ID: "x", Message: "1", Style: "plastic", Logo: "go",
		LabelColor: "blue", Alt: "alt"}
	manifest := &model.Manifest{Badges: []model.Badge{value}}
	line := (&Service{}).badgeMarkdown(manifest)
	if !strings.Contains(line, "style=plastic") || !strings.Contains(line, "logo=go") ||
		!strings.Contains(line, "labelColor=blue") || !strings.Contains(line, "![alt]") {
		t.Fatalf("badge line %q", line)
	}
}

func TestRequiredBadgeBlocksTheCheck(t *testing.T) {
	scene := newScene(t, model.OutputMonolith)
	manifestPath := filepath.Join(scene.paths.Dir, lifecycle.ManifestMember)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Badges = []model.Badge{{ID: "license", Required: true}}
	encoded, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(manifestPath, encoded); err != nil {
		t.Fatal(err)
	}
	findings, err := scene.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	blocking := 0
	for _, finding := range findings {
		if finding.Code == "badge-unresolved" && finding.Blocking {
			blocking++
		}
	}
	if blocking != 1 {
		t.Fatalf("findings %+v", findings)
	}
}
