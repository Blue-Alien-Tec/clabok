package structure

import (
	"sort"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// StatusLevelUpdate carries the optional fields of one level edit.
type StatusLevelUpdate struct {
	Label            *string
	Color            *string
	Description      *string
	DescriptionBlock model.ID
	ClearDescription bool
}

// AddStatusLevel appends or inserts one configured level. The first level on a fallback book starts
// a custom list; the built-in levels are never copied.
func (service *Service) AddStatusLevel(book model.BookID, level model.StatusLevel, position, anchor string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.status-level-add",
		[]Step{{Op: "status-level-add", Level: &level, Position: position, Anchor: anchor}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// UpdateStatusLevel edits a configured level. A description is written to its status block through
// the mutation's body set, so it shares the prose validation with every other body.
func (service *Service) UpdateStatusLevel(book model.BookID, id string, update StatusLevelUpdate, dryRun bool) (Receipt, error) {
	step := Step{Op: "status-level-set", Key: id, Description: update.Description,
		ClearDescription: update.ClearDescription,
		Level:            &model.StatusLevel{DescriptionBlock: update.DescriptionBlock}}
	if update.Label != nil || update.Color != nil {
		step.Level.ID = id
		if update.Label != nil {
			step.Level.Label = *update.Label
			step.Value = "label"
		}
		if update.Color != nil {
			step.Level.Color = *update.Color
			step.Value = "color"
		}
	}
	receipts, err := service.run(book, "struct.status-level-set", []Step{step}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// MoveStatusLevel reorders a configured level.
func (service *Service) MoveStatusLevel(book model.BookID, id, position, anchor string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.status-level-move",
		[]Step{{Op: "status-level-move", Key: id, Position: position, Anchor: anchor}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// RemoveStatusLevel removes an unassigned level and its description block and region.
func (service *Service) RemoveStatusLevel(book model.BookID, id string, dryRun bool) (Receipt, error) {
	receipts, err := service.run(book, "struct.status-level-remove",
		[]Step{{Op: "status-level-remove", Key: id}}, dryRun)
	if err != nil {
		return Receipt{}, err
	}
	return receipts[0], nil
}

// applyStatusLevelAdd inserts one level into the configured list.
func applyStatusLevelAdd(manifest *model.Manifest, _ *bookIDs, step Step) (mutation, error) {
	if step.Level == nil {
		return mutation{}, fail("structure.invalid-status-level", "a level is required")
	}
	level := *step.Level
	level.ID = strings.TrimSpace(level.ID)
	level.Label = strings.TrimSpace(level.Label)
	level.Color = strings.TrimSpace(level.Color)
	if level.ID == "" {
		return mutation{}, fail("structure.invalid-status-level", "a level id is required")
	}
	if message := model.ValidateTypeToken(level.ID); message != "" {
		return mutation{}, fail("structure.invalid-status-level", "level id: %s", message)
	}
	if level.Label == "" {
		return mutation{}, fail("structure.invalid-status-level", "a level needs a label")
	}
	if len([]rune(level.Label)) > 40 {
		return mutation{}, fail("structure.invalid-status-level",
			"a level label is at most 40 characters")
	}
	if !model.ValidBadgeColor(level.Color) {
		return mutation{}, fail("structure.invalid-status-level",
			"expected a named color or a hex value, got %q", level.Color)
	}
	if manifest.Status == nil {
		manifest.Status = &model.StatusConfig{}
	}
	if len(manifest.Status.Levels) >= model.StatusLevelCap {
		return mutation{}, fail("structure.status-level-cap",
			"at most %d status levels are allowed", model.StatusLevelCap)
	}
	for _, existing := range manifest.Status.Levels {
		if existing.ID == level.ID {
			return mutation{}, fail("structure.status-level-exists",
				"the level %q already exists", level.ID)
		}
	}
	index, err := levelPosition(manifest.Status.Levels, step.Position, step.Anchor, true)
	if err != nil {
		return mutation{}, err
	}
	manifest.Status.Levels = insertLevel(manifest.Status.Levels, index, level)
	return mutation{receipt: Receipt{
		Book: manifest.ID, Kind: "status-level", Index: index, Changed: true,
	}}, nil
}

// applyStatusLevelSet edits label, color or description of a configured level.
func applyStatusLevelSet(manifest *model.Manifest, ids *bookIDs, step Step) (mutation, error) {
	if manifest.Status == nil || len(manifest.Status.Levels) == 0 {
		return mutation{}, fail("structure.status-level-fallback",
			"the built-in status levels are read-only; add a level to start a custom list")
	}
	index := levelIndex(manifest.Status.Levels, step.Key)
	if index < 0 {
		return mutation{}, fail("structure.unknown-status-level",
			"the level %q is not configured", step.Key)
	}
	level := &manifest.Status.Levels[index]
	changed := false
	if step.Level != nil && strings.TrimSpace(step.Level.Label) != "" {
		label := strings.TrimSpace(step.Level.Label)
		if len([]rune(label)) > 40 {
			return mutation{}, fail("structure.invalid-status-level",
				"a level label is at most 40 characters")
		}
		changed = changed || level.Label != label
		level.Label = label
	}
	if step.Level != nil && step.Level.Color != "" {
		color := strings.TrimSpace(step.Level.Color)
		if !model.ValidBadgeColor(color) {
			return mutation{}, fail("structure.invalid-status-level",
				"expected a named color or a hex value, got %q", color)
		}
		changed = changed || level.Color != color
		level.Color = color
	}
	result := mutation{receipt: Receipt{
		Book: manifest.ID, Kind: "status-level", Index: index, Changed: changed,
	}}
	switch {
	case step.ClearDescription:
		created, removed := clearStatusDescription(manifest, level)
		if len(removed) > 0 {
			changed = true
		}
		result.created = created
		result.removed = removed
	case step.Description != nil:
		text := strings.TrimSpace(*step.Description)
		if text == "" {
			created, removed := clearStatusDescription(manifest, level)
			if len(removed) > 0 {
				changed = true
			}
			result.created = created
			result.removed = removed
			break
		}
		if message := model.ValidateProse(text); message != "" {
			return mutation{}, fail("structure.invalid-prose", "%s", message)
		}
		blockID := level.DescriptionBlock
		created := make([]model.ID, 0, 1)
		if blockID.IsZero() {
			regionID, err := ids.next()
			if err != nil {
				return mutation{}, err
			}
			blockID, err = ids.next()
			if err != nil {
				return mutation{}, err
			}
			manifest.Regions = append(manifest.Regions, &model.Region{
				ID: regionID, Kind: model.RegionStatus, Target: model.Target{Kind: model.TargetBook},
				Blocks: []model.ID{blockID},
			})
			level.DescriptionBlock = blockID
			created = append(created, blockID)
		}
		changed = true
		result.created = append(result.created, created...)
		result.bodies = map[model.ID]string{blockID: text}
	case step.Level != nil && !step.Level.DescriptionBlock.IsZero():
		blockID := step.Level.DescriptionBlock
		if !statusBlockExists(manifest, blockID) {
			return mutation{}, fail("structure.status-block-missing",
				"block %s is not placed in a status region", blockID)
		}
		for _, other := range manifest.Status.Levels {
			if other.ID != level.ID && other.DescriptionBlock == blockID {
				return mutation{}, fail("structure.status-block-shared",
					"block %s already describes the level %q", blockID, other.ID)
			}
		}
		if level.DescriptionBlock != blockID {
			_, removed := clearStatusDescription(manifest, level)
			result.removed = append(result.removed, removed...)
			level.DescriptionBlock = blockID
			changed = true
		}
	}
	result.receipt.Changed = changed
	return result, nil
}

// applyStatusLevelMove reorders a level relative to an anchor.
func applyStatusLevelMove(manifest *model.Manifest, step Step) (mutation, error) {
	if manifest.Status == nil || len(manifest.Status.Levels) == 0 {
		return mutation{}, fail("structure.status-level-fallback",
			"the built-in status levels are fixed")
	}
	index := levelIndex(manifest.Status.Levels, step.Key)
	if index < 0 {
		return mutation{}, fail("structure.unknown-status-level",
			"the level %q is not configured", step.Key)
	}
	level := manifest.Status.Levels[index]
	levels := make([]model.StatusLevel, 0, len(manifest.Status.Levels))
	levels = append(levels, manifest.Status.Levels[:index]...)
	levels = append(levels, manifest.Status.Levels[index+1:]...)
	target, err := levelPosition(levels, step.Position, step.Anchor, false)
	if err != nil {
		return mutation{}, err
	}
	levels = insertLevel(levels, target, level)
	manifest.Status.Levels = levels
	return mutation{receipt: Receipt{
		Book: manifest.ID, Kind: "status-level", Index: target, Changed: true,
	}}, nil
}

// applyStatusLevelRemove deletes an unassigned level and its description region.
func applyStatusLevelRemove(manifest *model.Manifest, step Step) (mutation, error) {
	if manifest.Status == nil || len(manifest.Status.Levels) == 0 {
		return mutation{}, fail("structure.status-level-fallback",
			"the built-in status levels are fixed")
	}
	index := levelIndex(manifest.Status.Levels, step.Key)
	if index < 0 {
		return mutation{}, fail("structure.unknown-status-level",
			"the level %q is not configured", step.Key)
	}
	level := manifest.Status.Levels[index]
	inUse := statusAssignments(manifest, level.ID)
	if len(inUse) > 0 {
		return mutation{}, fail("structure.status-in-use",
			"level %q is assigned to %s; reassign those targets first", level.ID,
			strings.Join(inUse, ", "))
	}
	_, removed := clearStatusDescription(manifest, &level)
	levels := make([]model.StatusLevel, 0, len(manifest.Status.Levels)-1)
	levels = append(levels, manifest.Status.Levels[:index]...)
	levels = append(levels, manifest.Status.Levels[index+1:]...)
	if len(levels) == 0 {
		manifest.Status.Levels = nil
	} else {
		manifest.Status.Levels = levels
	}
	return mutation{
		receipt: Receipt{Book: manifest.ID, Kind: "status-level", Index: index, Changed: true},
		removed: removed,
	}, nil
}

// clearStatusDescription removes the status region and block of a level.
func clearStatusDescription(manifest *model.Manifest, level *model.StatusLevel) (created, removed []model.ID) {
	if level.DescriptionBlock.IsZero() {
		return nil, nil
	}
	blockID := level.DescriptionBlock
	if region := statusRegionFor(manifest, blockID); region != nil {
		removeRegion(manifest, region)
	}
	level.DescriptionBlock = model.ZeroID
	return nil, []model.ID{blockID}
}

func statusRegionFor(manifest *model.Manifest, blockID model.ID) *model.Region {
	for _, region := range manifest.Regions {
		if region == nil || region.Kind != model.RegionStatus || len(region.Blocks) != 1 {
			continue
		}
		if region.Blocks[0] == blockID {
			return region
		}
	}
	return nil
}

func statusBlockExists(manifest *model.Manifest, blockID model.ID) bool {
	return statusRegionFor(manifest, blockID) != nil
}

// statusAssignments lists the nodes and blocks carrying one level.
func statusAssignments(manifest *model.Manifest, id string) []string {
	targets := make([]string, 0)
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		if node.Status == id {
			targets = append(targets, "node "+node.ID.String())
		}
		return true
	})
	for blockID, meta := range manifest.BlockMeta {
		if meta.Status == id {
			targets = append(targets, "block "+blockID.String())
		}
	}
	sort.Strings(targets)
	return targets
}

func levelIndex(levels []model.StatusLevel, id string) int {
	for index, level := range levels {
		if level.ID == id {
			return index
		}
	}
	return -1
}

// levelPosition resolves a position against an anchor. allowEnd permits an empty anchor as the end
// (an add); a move requires an explicit position.
func levelPosition(levels []model.StatusLevel, position, anchor string, allowEnd bool) (int, error) {
	switch position {
	case "", "last":
		if allowEnd {
			return len(levels), nil
		}
	case "first":
		return 0, nil
	case "before", "after":
		index := levelIndex(levels, anchor)
		if index < 0 {
			return 0, fail("structure.unknown-status-level",
				"the anchor level %q is not configured", anchor)
		}
		if position == "after" {
			return index + 1, nil
		}
		return index, nil
	}
	if allowEnd {
		return len(levels), nil
	}
	return 0, fail("structure.invalid-position",
		"a move needs --before, --after, --first or --last")
}

func insertLevel(levels []model.StatusLevel, index int, level model.StatusLevel) []model.StatusLevel {
	if index > len(levels) {
		index = len(levels)
	}
	result := make([]model.StatusLevel, 0, len(levels)+1)
	result = append(result, levels[:index]...)
	result = append(result, level)
	result = append(result, levels[index:]...)
	return result
}
