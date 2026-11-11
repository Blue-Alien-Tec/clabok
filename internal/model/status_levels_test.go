package model

import (
	"strings"
	"testing"
)

func TestStatusLevelsFallbackAndCustom(t *testing.T) {
	manifest := validManifest()
	if !manifest.StatusFallback() {
		t.Fatal("a book without status config is a fallback")
	}
	levels := manifest.StatusLevels()
	if len(levels) != 4 || levels[0].ID != "final" || levels[3].ID != "draft" {
		t.Fatalf("fallback levels %+v", levels)
	}
	if levels[0].DescriptionBlock != 0 {
		t.Fatal("built-in levels carry no block")
	}

	manifest.Status = &StatusConfig{
		Enabled: true,
		Levels:  []StatusLevel{{ID: "approved", Label: "Approved", Color: "green"}},
	}
	if manifest.StatusFallback() {
		t.Fatal("a custom list is not a fallback")
	}
	if _, ok := manifest.StatusLevel("final"); ok {
		t.Fatal("the built-in levels must not leak into a custom list")
	}
	level, ok := manifest.StatusLevel("approved")
	if !ok || level.Label != "Approved" || level.Color != "green" {
		t.Fatalf("level %+v %v", level, ok)
	}
}

func TestStatusLevelValidation(t *testing.T) {
	manifest := validManifest()
	manifest.Status = &StatusConfig{Enabled: true, Levels: []StatusLevel{
		{ID: "approved", Label: "Approved", Color: "green"},
		{ID: "frozen", Label: "Frozen", Color: "000080"},
	}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("a valid custom list was rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*StatusConfig)
	}{
		{"nine", func(config *StatusConfig) {
			config.Levels = nil
			for index := 0; index < StatusLevelCap+1; index++ {
				config.Levels = append(config.Levels, StatusLevel{
					ID: "level" + string(rune('a'+index)), Label: "L",
				})
			}
		}},
		{"duplicate", func(config *StatusConfig) {
			config.Levels = append(config.Levels, StatusLevel{ID: "approved", Label: "Again"})
		}},
		{"empty-label", func(config *StatusConfig) {
			config.Levels[0].Label = " "
		}},
		{"long-label", func(config *StatusConfig) {
			config.Levels[0].Label = strings.Repeat("x", 41)
		}},
		{"bad-color", func(config *StatusConfig) {
			config.Levels[0].Color = "not a color"
		}},
		{"bad-id", func(config *StatusConfig) {
			config.Levels[0].ID = "two words"
		}},
	}
	for _, testCase := range cases {
		manifest := validManifest()
		manifest.Status = &StatusConfig{Enabled: true, Levels: []StatusLevel{
			{ID: "approved", Label: "Approved"},
		}}
		testCase.mutate(manifest.Status)
		if err := manifest.Validate(); err == nil {
			t.Fatalf("%s: expected a validation error", testCase.name)
		}
	}
}

func TestStatusDescriptionBlocks(t *testing.T) {
	manifest := validManifest()
	manifest.Status = &StatusConfig{Enabled: true, Levels: []StatusLevel{
		{ID: "approved", Label: "Approved", DescriptionBlock: 31},
	}}
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 30, Kind: RegionStatus, Target: Target{Kind: TargetBook}, Blocks: []ID{31},
	})
	if err := manifest.Validate(); err != nil {
		t.Fatalf("a valid status region was rejected: %v", err)
	}

	manifest = validManifest()
	manifest.Status = &StatusConfig{Enabled: true, Levels: []StatusLevel{
		{ID: "approved", Label: "Approved", DescriptionBlock: 31},
	}}
	if err := manifest.Validate(); err == nil {
		t.Fatal("a description block outside a status region must be refused")
	}

	manifest = validManifest()
	manifest.Status = &StatusConfig{Enabled: true, Levels: []StatusLevel{
		{ID: "approved", Label: "Approved", DescriptionBlock: 31},
		{ID: "frozen", Label: "Frozen", DescriptionBlock: 31},
	}}
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 30, Kind: RegionStatus, Target: Target{Kind: TargetBook}, Blocks: []ID{31},
	})
	if err := manifest.Validate(); err == nil {
		t.Fatal("two levels must not share a description block")
	}

	manifest = validManifest()
	manifest.Status = &StatusConfig{Enabled: true, Levels: []StatusLevel{
		{ID: "approved", Label: "Approved"},
	}}
	manifest.Regions = append(manifest.Regions, &Region{
		ID: 30, Kind: RegionStatus, Target: Target{Kind: TargetBook}, Blocks: []ID{31},
	})
	if err := manifest.Validate(); err == nil {
		t.Fatal("an unreferenced status block must be refused")
	}
}

func TestCustomLevelValidatesAssignments(t *testing.T) {
	manifest := validManifest()
	manifest.Status = &StatusConfig{Enabled: true, Levels: []StatusLevel{
		{ID: "approved", Label: "Approved"},
	}}
	manifest.Root.Children[0].Status = "final"
	if err := manifest.Validate(); err == nil {
		t.Fatal("a built-in assignment must be refused when a custom list exists")
	}
	manifest.Root.Children[0].Status = "approved"
	if err := manifest.Validate(); err != nil {
		t.Fatalf("a custom assignment was rejected: %v", err)
	}
}
