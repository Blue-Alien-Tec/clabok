package model

import "testing"

func TestPartLabelSlots(t *testing.T) {
	want := []string{
		"Part I", "Part II", "Part III", "Part IV", "Part V",
		"Part VI", "Part VII", "Part VIII", "Part IX", "Part X",
	}
	for index, expected := range want {
		label, ok := PartLabel(index + 1)
		if !ok || label != expected {
			t.Fatalf("slot %d: got %q (%v), want %q", index+1, label, ok, expected)
		}
	}
	for _, slot := range []int{-1, 0, 11, 99} {
		if _, ok := PartLabel(slot); ok {
			t.Fatalf("slot %d must be out of range", slot)
		}
	}
}

func TestParsePartSlot(t *testing.T) {
	cases := []struct {
		raw  string
		slot int
		ok   bool
	}{
		{"1", 1, true},
		{" 10 ", 10, true},
		{"0", 0, true},
		{"11", 11, true},
		{"", 0, false},
		{"Part I", 0, false},
		{"2a", 0, false},
		{"-1", 0, false},
		{"1.5", 0, false},
	}
	for _, testCase := range cases {
		slot, ok := ParsePartSlot(testCase.raw)
		if ok != testCase.ok || (ok && slot != testCase.slot) {
			t.Fatalf("%q: got %d (%v), want %d (%v)", testCase.raw, slot, ok,
				testCase.slot, testCase.ok)
		}
	}
	long, ok := ParsePartSlot("999999999999999999")
	if !ok || long <= PartLimit {
		t.Fatalf("a long digit run must saturate above the limit, got %d (%v)", long, ok)
	}
}

func TestManifestPartsOrderAndDedup(t *testing.T) {
	manifest := &Manifest{Root: &Node{Children: []*Node{
		{Part: ""},
		{Part: "Part I"},
		{Part: "Teil II"},
		{Part: "Part I"},
	}}}
	parts := manifest.Parts()
	if len(parts) != 2 || parts[0] != "Part I" || parts[1] != "Teil II" {
		t.Fatalf("parts %q", parts)
	}
	if (&Manifest{}).Parts() != nil {
		t.Fatal("a manifest without a root has no parts")
	}
}
