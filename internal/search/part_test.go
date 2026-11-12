package search

import "testing"

func TestPartFilterAcceptsSlot(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"1", "Part I"},
		{"2", "Part II"},
		{" 10 ", "Part X"},
		{"Part I", "Part I"},
		{"Teil II — Die Galaxie", "Teil II — Die Galaxie"},
		{"", ""},
	}
	for _, testCase := range cases {
		got, err := partText(testCase.raw)
		if err != nil {
			t.Fatalf("%q: %v", testCase.raw, err)
		}
		if got != testCase.want {
			t.Fatalf("%q: got %q, want %q", testCase.raw, got, testCase.want)
		}
	}
	for _, raw := range []string{"0", "11", "99"} {
		if _, err := partText(raw); err == nil {
			t.Fatalf("%q must be rejected", raw)
		} else if err.Code != "usage.invalid-value" {
			t.Fatalf("%q: code %s", raw, err.Code)
		}
	}
}
