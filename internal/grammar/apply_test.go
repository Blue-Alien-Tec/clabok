package grammar

import (
	"strings"
	"testing"
)

func TestApplySelected(t *testing.T) {
	text := "teh cat"
	findings := []Finding{
		{Index: 1, Offset: 0, Length: 3, Offending: "teh", Replacements: []string{"the"}},
		{Index: 2, Offset: 4, Length: 3, Offending: "cat", Replacements: []string{"dog"}},
	}
	next, applied, err := applySelected(text, findings, -1)
	if err != nil || applied != 2 || next != "the dog" {
		t.Fatalf("apply: %q %d %v", next, applied, err)
	}
	if _, _, err := applySelected(text, findings, 1); err == nil ||
		!strings.Contains(err.Error(), "expected 1") {
		t.Fatalf("expectation: %v", err)
	}
	stale := []Finding{{Index: 1, Offset: 0, Length: 3, Offending: "xxx", Replacements: []string{"the"}}}
	if _, _, err := applySelected(text, stale, -1); err == nil {
		t.Fatal("a stale finding must be rejected")
	}
}

func TestSelectFindings(t *testing.T) {
	findings := []Finding{
		{Index: 1, Replacements: []string{"the"}},
		{Index: 2, Replacements: []string{"a", "b"}},
		{Index: 3},
	}
	safe, err := selectFindings(findings, true, "")
	if err != nil || len(safe) != 1 || safe[0].Index != 1 {
		t.Fatalf("all-safe: %+v %v", safe, err)
	}
	explicit, err := selectFindings(findings, false, "2")
	if err != nil || len(explicit) != 1 || explicit[0].Index != 2 {
		t.Fatalf("explicit: %+v %v", explicit, err)
	}
	if _, err := selectFindings(findings, false, "3"); err == nil {
		t.Fatal("a finding without a replacement must be rejected")
	}
	if _, err := selectFindings(findings, false, "9"); err == nil {
		t.Fatal("an unknown index must be rejected")
	}
}
