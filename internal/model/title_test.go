package model

import "testing"

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"1. Ziel", "Ziel"},
		{"1.2 Ziel", "Ziel"},
		{"1.2.3 Ziel", "Ziel"},
		{"1.2. Ziel", "Ziel"},
		{"IV. Kapitel", "Kapitel"},
		{"2026 - Year of X", "2026 - Year of X"},
		{"3D Printing", "3D Printing"},
		{"1.Ziel", "1.Ziel"},
		{"Ziel", "Ziel"},
		{"  Ziel  ", "Ziel"},
		{"1.2", "1.2"},
		{"", ""},
	}
	for _, testCase := range cases {
		if got := NormalizeTitle(testCase.input); got != testCase.want {
			t.Fatalf("NormalizeTitle(%q) = %q, want %q", testCase.input, got, testCase.want)
		}
	}
}
