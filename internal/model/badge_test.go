package model

import "testing"

func TestBadgeValidation(t *testing.T) {
	manifest := validManifest()
	manifest.Badges = []Badge{{
		ID: "node", Source: "node", Style: "flat-square", Logo: "nodedotjs",
		Color: "339933", LogoColor: "fff", LabelColor: "lightgrey", Alt: "Node",
		CacheSeconds: 300, Required: true, Order: 2, Group: "stack",
	}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("a valid badge was rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Badge)
	}{
		{"style", func(badge *Badge) { badge.Style = "neon" }},
		{"logo", func(badge *Badge) { badge.Logo = "https://example.com/x.svg" }},
		{"color", func(badge *Badge) { badge.Color = "not-a-color" }},
		{"alt", func(badge *Badge) { badge.Alt = "two\nlines" }},
		{"cache", func(badge *Badge) { badge.CacheSeconds = 90000 }},
		{"group", func(badge *Badge) { badge.Group = "two words" }},
		{"source", func(badge *Badge) { badge.Source = "two words" }},
	}
	for _, testCase := range cases {
		manifest := validManifest()
		manifest.Badges = []Badge{{ID: "node"}}
		testCase.mutate(&manifest.Badges[0])
		if err := manifest.Validate(); err == nil {
			t.Fatalf("%s: expected a validation error", testCase.name)
		}
	}
}

func TestBadgeURLHelpers(t *testing.T) {
	if !ValidBadgeStyle("") || !ValidBadgeStyle("for-the-badge") || ValidBadgeStyle("neon") {
		t.Fatal("style validation")
	}
	if !ValidBadgeColor("00ADD8") || !ValidBadgeColor("abc") || !ValidBadgeColor("lightgrey") ||
		ValidBadgeColor("blueish") {
		t.Fatal("color validation")
	}
	if !ValidBadgeLogo("unity") || !ValidBadgeLogo("") || ValidBadgeLogo("../logo") ||
		ValidBadgeLogo("two words") {
		t.Fatal("logo validation")
	}
}
