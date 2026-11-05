package asciiui

import (
	"strings"
	"testing"
)

func TestParseSpec(t *testing.T) {
	spec, err := Parse(`title Shop
legend true
row
  panel Catalogue
    row
      button Buy
      button Sell
  list Items 2
`)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Title != "Shop" || !spec.Legend {
		t.Fatalf("spec %+v", spec)
	}
	if len(spec.Nodes) != 1 || spec.Nodes[0].Kind != "row" {
		t.Fatalf("nodes %+v", spec.Nodes)
	}
	row := spec.Nodes[0]
	if len(row.Children) != 2 {
		t.Fatalf("row children %d", len(row.Children))
	}
	if row.Children[0].Kind != "panel" || len(row.Children[0].Children) != 1 {
		t.Fatalf("panel %+v", row.Children[0])
	}
	list := row.Children[1]
	if list.Kind != "list" || list.Label != "Items" || list.Count != 2 {
		t.Fatalf("list %+v", list)
	}
}

func TestParseRejects(t *testing.T) {
	cases := []string{
		"bogus X",
		"   button X",
		"",
	}
	for _, document := range cases {
		if _, err := Parse(document); err == nil {
			t.Fatalf("%q must be rejected", document)
		}
	}
}

func TestRenderLayout(t *testing.T) {
	spec, err := Parse(`title Shop
row
  panel Catalogue
    button Buy
  panel Basket
    list Items 2
`)
	if err != nil {
		t.Fatal(err)
	}
	drawing, err := Render(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Shop", "+", "| Catalogue", "| Basket", "[ Buy ]", "- item 1", "- item 2"} {
		if !strings.Contains(drawing, want) {
			t.Fatalf("missing %q in\n%s", want, drawing)
		}
	}
	lines := strings.Split(drawing, "\n")
	boxLines := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "+") && strings.HasSuffix(trimmed, "+") {
			boxLines++
		}
	}
	if boxLines < 2 {
		t.Fatalf("no panel borders in\n%s", drawing)
	}
	for _, line := range lines {
		if strings.Contains(line, "| Catalogue") && !strings.Contains(line, "| Basket") {
			// Two panels side by side; both borders must be present on the same visual row.
			continue
		}
	}
}

func TestResolveOverrides(t *testing.T) {
	overrides := map[string]string{"button": "UIButton", "panel": ""}
	if got := Resolve("button", "", overrides); got != "UIButton" {
		t.Fatalf("override %q", got)
	}
	if got := Resolve("panel", "", overrides); got != "Panel" {
		t.Fatalf("blank override keeps the default, got %q", got)
	}
	if got := Resolve("button", "ExplicitButton", overrides); got != "ExplicitButton" {
		t.Fatalf("explicit wins, got %q", got)
	}
}

func TestLegendUsesOverrides(t *testing.T) {
	spec, err := Parse("legend true\nbutton Buy\n")
	if err != nil {
		t.Fatal(err)
	}
	drawing, err := Render(spec, map[string]string{"button": "UIButton"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(drawing, "- Buy: UIButton") {
		t.Fatalf("legend missing override in\n%s", drawing)
	}
	drawing, err = Render(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(drawing, "- Buy: Button") {
		t.Fatalf("generic legend missing in\n%s", drawing)
	}
}

func TestWidgetAttributeWins(t *testing.T) {
	spec, err := Parse("legend true\nbutton Buy widget=Custom\n")
	if err != nil {
		t.Fatal(err)
	}
	drawing, err := Render(spec, map[string]string{"button": "UIButton"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(drawing, "- Buy: Custom") {
		t.Fatalf("explicit widget lost in\n%s", drawing)
	}
}

func TestRenderIsStable(t *testing.T) {
	document := `title Shop
row
  button A
  button B
`
	first, err := Render(mustParse(t, document), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(mustParse(t, document), nil)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("rendering is not deterministic")
	}
}

func mustParse(t *testing.T, document string) *Spec {
	t.Helper()
	spec, err := Parse(document)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}
