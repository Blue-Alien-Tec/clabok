package search

import (
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/source"
)

// fieldFlags selects which text sources a search reads.
type fieldFlags struct {
	title  bool
	body   bool
	layout bool
	prose  bool
	note   bool
	footer bool
}

var knownFields = map[string]struct{}{
	"title": {}, "body": {}, "layout": {}, "prose": {}, "note": {}, "footer": {}, "all": {},
}

// parseFields turns the --field values into flags. The default set is body and title.
func parseFields(values []string) (fieldFlags, error) {
	flags := fieldFlags{}
	if len(values) == 0 {
		flags.body = true
		flags.title = true
		return flags, nil
	}
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if name == "" {
			continue
		}
		if _, known := knownFields[name]; !known {
			return fieldFlags{}, fail("search.invalid-field",
				"unknown field %q; use title, body, layout, prose, note, footer or all", name)
		}
		switch name {
		case "title":
			flags.title = true
		case "body":
			flags.body = true
		case "layout":
			flags.layout = true
		case "prose":
			flags.prose = true
		case "note":
			flags.note = true
		case "footer":
			flags.footer = true
		case "all":
			flags.title, flags.body, flags.layout = true, true, true
			flags.prose, flags.note, flags.footer = true, true, true
		}
	}
	return flags, nil
}

// fieldNames returns the effective field list in the canonical order.
func (flags fieldFlags) names() []string {
	names := make([]string, 0, 6)
	for _, entry := range []struct {
		name string
		set  bool
	}{
		{"title", flags.title}, {"body", flags.body}, {"layout", flags.layout},
		{"prose", flags.prose}, {"note", flags.note}, {"footer", flags.footer},
	} {
		if entry.set {
			names = append(names, entry.name)
		}
	}
	return names
}

// nodeInfo is one manifest node with the derived title path and inherited part.
type nodeInfo struct {
	Node   *model.Node
	Parent *nodeInfo
	Path   string
	Part   string
}

// bookInfo is the placement and path index of one loaded book.
type bookInfo struct {
	Book    *source.Book
	Path    string
	Nodes   map[model.ID]*nodeInfo
	Ordered []*nodeInfo
}

func newBookInfo(book *source.Book, outputPath string) *bookInfo {
	info := &bookInfo{Book: book, Path: outputPath, Nodes: map[model.ID]*nodeInfo{}}
	var walk func(node *model.Node, parent *nodeInfo)
	walk = func(node *model.Node, parent *nodeInfo) {
		if node == nil {
			return
		}
		entry := &nodeInfo{Node: node, Parent: parent}
		entry.Path = node.Title
		if parent != nil && parent.Path != "" {
			entry.Path = parent.Path + " / " + node.Title
		}
		entry.Part = node.Part
		if entry.Part == "" && parent != nil {
			entry.Part = parent.Part
		}
		info.Nodes[node.ID] = entry
		info.Ordered = append(info.Ordered, entry)
		for _, child := range node.Children {
			walk(child, entry)
		}
	}
	if book != nil && book.Manifest != nil {
		walk(book.Manifest.Root, nil)
	}
	return info
}

// nodeOfBlock returns the node that places a block.
func (info *bookInfo) nodeOfBlock(id model.ID) *nodeInfo {
	node, exists := info.Book.NodeForBlock(id)
	if !exists {
		return nil
	}
	return info.Nodes[node.ID]
}

// regionNode returns the node a region targets, directly or through a block target.
func (info *bookInfo) regionNode(region *model.Region) *nodeInfo {
	if region == nil {
		return nil
	}
	switch region.Target.Kind {
	case model.TargetNode:
		return info.Nodes[region.Target.ID]
	case model.TargetBlock:
		return info.nodeOfBlock(region.Target.ID)
	}
	return nil
}

// subtree returns the node and every descendant.
func (info *bookInfo) subtree(id model.ID) map[model.ID]struct{} {
	root, exists := info.Nodes[id]
	if !exists {
		return nil
	}
	ids := map[model.ID]struct{}{}
	var walk func(entry *nodeInfo)
	walk = func(entry *nodeInfo) {
		ids[entry.Node.ID] = struct{}{}
		for _, child := range entry.Node.Children {
			walk(info.Nodes[child.ID])
		}
	}
	walk(root)
	return ids
}

// filterSet is a compiled --kind/--type/--status/--part/--node/--block/--locked filter.
type filterSet struct {
	nodeIDs  map[model.ID]struct{}
	block    model.ID
	kinds    map[model.Kind]struct{}
	types    map[string]struct{}
	statuses map[string]struct{}
	part     string
	locked   *bool
}

func newFilterSet(info *bookInfo, filter Filter) (*filterSet, error) {
	set := &filterSet{block: filter.Block, part: filter.Part, locked: filter.Locked}
	if filter.Node != model.ZeroID {
		ids := info.subtree(filter.Node)
		if ids == nil {
			return nil, fail("search.unknown-id", "node %s does not exist in book %q",
				filter.Node, info.Book.ID)
		}
		set.nodeIDs = ids
	}
	if filter.Block != model.ZeroID && !info.Book.Manifest.IsPlaced(filter.Block) {
		return nil, fail("search.unknown-id", "block %s is not placed in book %q",
			filter.Block, info.Book.ID)
	}
	if len(filter.Kinds) > 0 {
		set.kinds = map[model.Kind]struct{}{}
		for _, name := range filter.Kinds {
			kind := model.Kind(name)
			if !kind.Valid() {
				return nil, fail("search.invalid-kind", "unknown node kind %q", name)
			}
			set.kinds[kind] = struct{}{}
		}
	}
	if len(filter.Types) > 0 {
		set.types = map[string]struct{}{}
		for _, name := range filter.Types {
			set.types[strings.ToLower(name)] = struct{}{}
		}
	}
	if len(filter.Statuses) > 0 {
		set.statuses = map[string]struct{}{}
		for _, name := range filter.Statuses {
			if name != "unset" {
				if _, known := model.FindStatus(name); !known {
					return nil, fail("search.invalid-status", "unknown status %q", name)
				}
			}
			set.statuses[name] = struct{}{}
		}
	}
	return set, nil
}

func statusKey(status string) string {
	if status == "" {
		return "unset"
	}
	return status
}

// titleAllowed applies the filters to a title match.
func (set *filterSet) titleAllowed(info *nodeInfo) bool {
	if info == nil {
		return false
	}
	if set.nodeIDs != nil {
		if _, ok := set.nodeIDs[info.Node.ID]; !ok {
			return false
		}
	}
	if set.block != model.ZeroID {
		return false
	}
	if len(set.kinds) > 0 {
		if _, ok := set.kinds[info.Node.Kind]; !ok {
			return false
		}
	}
	if set.types != nil || set.locked != nil {
		return false
	}
	if set.statuses != nil {
		if _, ok := set.statuses[statusKey(info.Node.Status)]; !ok {
			return false
		}
	}
	if set.part != "" && info.Part != set.part {
		return false
	}
	return true
}

// blockAllowed applies the filters to a block match. owner is the node a title, body, layout or
// region block belongs to; region is set for a region block.
func (set *filterSet) blockAllowed(info *bookInfo, owner *nodeInfo, block model.ID, region *model.Region) bool {
	meta := info.Book.Manifest.Meta(block)
	if set.block != model.ZeroID && block != set.block && !targetsBlock(region, set.block) {
		return false
	}
	if set.nodeIDs != nil {
		allowed := false
		if owner != nil {
			if _, ok := set.nodeIDs[owner.Node.ID]; ok {
				allowed = true
			}
		}
		if !allowed && region != nil {
			target := info.regionNode(region)
			if target != nil {
				if _, ok := set.nodeIDs[target.Node.ID]; ok {
					allowed = true
				}
			}
			if region.Target.Kind == model.TargetBlock {
				if targetBlockOwner := info.nodeOfBlock(region.Target.ID); targetBlockOwner != nil {
					if _, ok := set.nodeIDs[targetBlockOwner.Node.ID]; ok {
						allowed = true
					}
				}
			}
		}
		if !allowed {
			return false
		}
	}
	if len(set.kinds) > 0 {
		if owner == nil {
			return false
		}
		if _, ok := set.kinds[owner.Node.Kind]; !ok {
			return false
		}
	}
	if set.types != nil {
		if _, ok := set.types[strings.ToLower(meta.Type)]; !ok {
			return false
		}
	}
	if set.statuses != nil {
		if _, ok := set.statuses[statusKey(meta.Status)]; !ok {
			return false
		}
	}
	if set.part != "" {
		if owner == nil || owner.Part != set.part {
			return false
		}
	}
	if set.locked != nil && meta.Locked != *set.locked {
		return false
	}
	return true
}

// targetsBlock reports whether a notes region is attached to the block.
func targetsBlock(region *model.Region, block model.ID) bool {
	return region != nil && region.Kind == model.RegionNotes &&
		region.Target.Kind == model.TargetBlock && region.Target.ID == block
}
