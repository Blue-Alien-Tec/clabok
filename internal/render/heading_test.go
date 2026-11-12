package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

func writeBody(t *testing.T, s scene, block model.ID, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.paths.BlocksDir, block.String()+".md"),
		[]byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func renderAll(t *testing.T, s scene) string {
	t.Helper()
	result, err := s.service.Render("fixture")
	if err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	for _, file := range result.Files {
		builder.WriteString(file.Content)
	}
	return builder.String()
}

func TestRenderKeepsHeadingLevel(t *testing.T) {
	for _, mode := range []model.OutputMode{model.OutputChapter, model.OutputMonolith, model.OutputPart} {
		s := newScene(t, mode)
		writeBody(t, s, 13, "### Der Unabhängige Bund\n\n#### Entstehung\n\nText\n")
		content := renderAll(t, s)
		for _, want := range []string{"\n### Der Unabhängige Bund\n", "\n#### Entstehung\n"} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s: missing %q in\n%s", mode, want, content)
			}
		}
	}
}

func TestStructuralHeadingIsAnchored(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	writeBody(t, s, 13, "### Prosa\n")
	content := renderAll(t, s)
	if !strings.Contains(content, "<a id=\"sub\"></a>\n\n### Sub") {
		t.Fatalf("a subchapter must carry its anchor line above the heading:\n%s", content)
	}
	if !strings.Contains(content, "\n### Prosa\n") {
		t.Fatalf("the prose heading is missing:\n%s", content)
	}
	if strings.Contains(content, "</a>\n\n### Prosa") {
		t.Fatalf("a prose heading must not carry an anchor line:\n%s", content)
	}
}

func TestProseHeadingLeavesIndexAlone(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	writeBody(t, s, 13, "### Prosa\n")
	content := renderAll(t, s)
	start := strings.Index(content, "## Index")
	chapter := strings.Index(content, "## One")
	if start < 0 || chapter < 0 || chapter < start {
		t.Fatalf("index or chapter heading missing:\n%s", content)
	}
	if strings.Contains(content[start:chapter], "Prosa") {
		t.Fatalf("the index must not list a prose heading:\n%s", content[start:chapter])
	}
}

func TestCheckSilentForHeadingThree(t *testing.T) {
	s := newScene(t, model.OutputMonolith)
	writeBody(t, s, 11, "### Der Bund\n\n#### Entstehung\n")
	findings, err := s.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if finding.Code == "body.forbidden-heading" {
			t.Fatalf("### must not be a structural claim: %+v", finding)
		}
	}
	writeBody(t, s, 11, "## nope\n")
	findings, err = s.service.Check("fixture")
	if err != nil {
		t.Fatal(err)
	}
	blocked := false
	for _, finding := range findings {
		if finding.Code == "body.forbidden-heading" {
			blocked = true
		}
	}
	if !blocked {
		t.Fatalf("## must stay rejected: %+v", findings)
	}
}
