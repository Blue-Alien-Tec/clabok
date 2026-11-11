package render

import (
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func TestStatusLegendCustomLevels(t *testing.T) {
	manifest := &model.Manifest{Status: &model.StatusConfig{
		Enabled: true,
		Levels: []model.StatusLevel{
			{ID: "approved", Label: "Approved", Color: "green", DescriptionBlock: 31},
			{ID: "frozen", Label: "Frozen", Color: "000080"},
		},
	}}
	bodies := map[model.ID]string{31: "Content is approved and signed off.\n"}
	legend := statusLegendMarkdown(manifest, bodies)
	lines := strings.Split(legend, "\n")
	if len(lines) != 6 {
		t.Fatalf("legend %q", legend)
	}
	if !strings.Contains(lines[4], "![status: Approved]") ||
		!strings.Contains(lines[4], "status-Approved-green") ||
		!strings.Contains(lines[4], "Content is approved and signed off.") {
		t.Fatalf("first row %q", lines[4])
	}
	if !strings.Contains(lines[5], "![status: Frozen]") || !strings.HasSuffix(lines[5], "|  |") {
		t.Fatalf("a level without a description must show an empty cell: %q", lines[5])
	}
	if badge := statusBadge(manifest, "approved"); !strings.Contains(badge, "Approved-green") {
		t.Fatalf("badge %q", badge)
	}
	if badge := statusBadge(manifest, "final"); badge != "" {
		t.Fatalf("a built-in level must not leak into a custom list: %q", badge)
	}
}

func TestStatusLegendFallback(t *testing.T) {
	manifest := &model.Manifest{Status: &model.StatusConfig{Enabled: true}}
	legend := statusLegendMarkdown(manifest, nil)
	for _, want := range []string{
		"| Status | Meaning |",
		"![status: Final]",
		"Content is settled; only errata and clarifications remain.",
		"![status: Draft]",
	} {
		if !strings.Contains(legend, want) {
			t.Fatalf("fallback legend misses %q:\n%s", want, legend)
		}
	}
	if count := strings.Count(legend, "![status:"); count != 4 {
		t.Fatalf("fallback rows %d", count)
	}
}
