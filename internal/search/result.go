package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// SchemaVersion is the result schema version of find.
const SchemaVersion = 2

// QueryInfo describes one submitted query.
type QueryInfo struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	Mode  string `json:"mode"`
	Fuzzy int    `json:"fuzzy"`
}

// IndexInfo reports the index decision of the search.
type IndexInfo struct {
	Mode  string `json:"mode"`
	Used  bool   `json:"used"`
	Fresh bool   `json:"fresh"`
}

// RefInfo reports an explicit --ref target.
type RefInfo struct {
	ID     string `json:"id"`
	Exists bool   `json:"exists"`
}

// ScopeInfo echoes the effective search scope.
type ScopeInfo struct {
	Mode     string   `json:"mode"`
	Books    []string `json:"books"`
	Fields   []string `json:"fields"`
	Node     string   `json:"node"`
	Block    string   `json:"block"`
	Kinds    []string `json:"kinds"`
	Types    []string `json:"types"`
	Statuses []string `json:"statuses"`
	Part     string   `json:"part"`
	Locked   *bool    `json:"locked"`
}

// QueryStats are the per-query counters of one group item.
type QueryStats struct {
	Query       int `json:"query"`
	Count       int `json:"count"`
	Occurrences int `json:"occurrences"`
	FirstLine   int `json:"firstLine"`
}

type header struct {
	SchemaVersion int         `json:"schemaVersion"`
	Book          string      `json:"book"`
	Query         string      `json:"query"`
	Queries       []QueryInfo `json:"queries"`
	Join          string      `json:"join"`
	Fuzzy         int         `json:"fuzzy"`
	GroupBy       string      `json:"groupBy"`
	Ref           *RefInfo    `json:"ref"`
	Index         IndexInfo   `json:"index"`
	Mode          string      `json:"mode"`
	Scope         ScopeInfo   `json:"scope"`
	Count         int         `json:"count"`
	Occurrences   int         `json:"occurrences"`
	Returned      int         `json:"returned"`
	Truncated     bool        `json:"truncated"`
	Warnings      []Warning   `json:"warnings"`
}

// BookItem aggregates the matches of one book.
type BookItem struct {
	Book        string       `json:"book"`
	Path        string       `json:"path"`
	Count       int          `json:"count"`
	Occurrences int          `json:"occurrences"`
	PerQuery    []QueryStats `json:"perQuery,omitempty"`
}

// BlockItem aggregates the matches of one block and field.
type BlockItem struct {
	Book        string       `json:"book"`
	Block       string       `json:"block"`
	Node        string       `json:"node,omitempty"`
	NodeTitle   string       `json:"nodeTitle,omitempty"`
	NodePath    string       `json:"nodePath,omitempty"`
	Type        string       `json:"type,omitempty"`
	Status      string       `json:"status,omitempty"`
	Locked      bool         `json:"locked"`
	Field       string       `json:"field"`
	FirstLine   int          `json:"firstLine"`
	Count       int          `json:"count"`
	Occurrences int          `json:"occurrences"`
	PerQuery    []QueryStats `json:"perQuery,omitempty"`
}

// NodeItem aggregates the direct matches of one node.
type NodeItem struct {
	Book        string       `json:"book"`
	Node        string       `json:"node"`
	NodeKind    string       `json:"nodeKind"`
	Title       string       `json:"title"`
	Part        string       `json:"part,omitempty"`
	Status      string       `json:"status,omitempty"`
	NodePath    string       `json:"nodePath"`
	Count       int          `json:"count"`
	Occurrences int          `json:"occurrences"`
	PerQuery    []QueryStats `json:"perQuery,omitempty"`
}

// GroupItem aggregates the matches of one group key.
type GroupItem struct {
	Key         string       `json:"key"`
	Count       int          `json:"count"`
	Occurrences int          `json:"occurrences"`
	FirstLine   int          `json:"firstLine"`
	PerQuery    []QueryStats `json:"perQuery,omitempty"`
}

type matchesPayload struct {
	header
	Matches []Match `json:"matches"`
}

type booksPayload struct {
	header
	Books []BookItem `json:"books"`
}

type blocksPayload struct {
	header
	Blocks []BlockItem `json:"blocks"`
}

type nodesPayload struct {
	header
	Nodes []NodeItem `json:"nodes"`
}

type countPayload struct {
	header
	Books  int `json:"books"`
	Blocks int `json:"blocks"`
	Nodes  int `json:"nodes"`
}

type groupedPayload struct {
	header
	Groups  []GroupItem `json:"groups"`
	Matches []Match     `json:"matches,omitempty"`
}

type groupedCountPayload struct {
	header
	Groups []GroupItem `json:"groups"`
}

// prepared is the paged view of one result.
type prepared struct {
	header     header
	matches    []Match
	books      []BookItem
	blocks     []BlockItem
	nodes      []NodeItem
	groups     []GroupItem
	grouped    bool
	countBy    bool
	bookCount  int
	blockCount int
	nodeCount  int
}

// prepare computes the units, the counters and the paging of one result.
func (result Result) prepare(limit, offset int) prepared {
	view := prepared{header: result.header()}
	queryCount := len(result.Queries)
	joinAnd := result.Join == "and" && queryCount > 1
	occurrences := 0
	for _, match := range result.Matches {
		occurrences += match.Occurrences
	}
	view.header.Occurrences = occurrences
	view.header.Warnings = result.Warnings
	if view.header.Warnings == nil {
		view.header.Warnings = []Warning{}
	}
	mode := result.Result
	if mode == "" {
		mode = "matches"
	}
	if result.FilterOnly {
		view.header.Occurrences = 0
		switch mode {
		case "blocks":
			view.blockCount = len(result.ListBlocks)
			view.header.Count = len(result.ListBlocks)
			view.blocks, view.header.Truncated = pageItems(result.ListBlocks, limit, offset)
			view.header.Returned = len(view.blocks)
		case "nodes":
			view.nodeCount = len(result.ListNodes)
			view.header.Count = len(result.ListNodes)
			view.nodes, view.header.Truncated = pageItems(result.ListNodes, limit, offset)
			view.header.Returned = len(view.nodes)
		case "books":
			view.bookCount = len(result.ListBooks)
			view.header.Count = len(result.ListBooks)
			view.books, view.header.Truncated = pageItems(result.ListBooks, limit, offset)
			view.header.Returned = len(view.books)
		case "count":
			view.header.Count = len(result.ListBlocks)
			view.blockCount = len(result.ListBlocks)
			view.nodeCount = len(result.ListNodes)
			view.bookCount = len(result.ListBooks)
		}
		return view
	}
	if result.GroupBy != "" {
		all := groupItems(result.Matches, result.GroupBy, queryCount)
		view.grouped = true
		view.countBy = result.CountBy
		view.header.Count = len(all)
		view.groups, view.header.Truncated = pageItems(all, limit, offset)
		view.header.Returned = len(view.groups)
		if !result.CountBy {
			view.matches = result.Matches
		}
		return view
	}
	switch mode {
	case "books":
		all := bookItems(result.Matches, joinAnd, queryCount)
		view.bookCount = len(all)
		view.header.Count = len(all)
		view.header.Occurrences = booksOccurrences(all)
		view.books, view.header.Truncated = pageItems(all, limit, offset)
		view.header.Returned = len(view.books)
	case "blocks":
		all := blockItems(result.Matches, joinAnd, queryCount)
		view.blockCount = len(all)
		view.header.Count = len(all)
		view.header.Occurrences = blocksOccurrences(all)
		view.blocks, view.header.Truncated = pageItems(all, limit, offset)
		view.header.Returned = len(view.blocks)
	case "nodes":
		all := nodeItems(result.Matches, joinAnd, queryCount)
		view.nodeCount = len(all)
		view.header.Count = len(all)
		view.header.Occurrences = nodesOccurrences(all)
		view.nodes, view.header.Truncated = pageItems(all, limit, offset)
		view.header.Returned = len(view.nodes)
	case "count":
		view.header.Count = len(result.Matches)
		view.bookCount = len(bookItems(result.Matches, joinAnd, queryCount))
		view.blockCount = len(blockItems(result.Matches, joinAnd, queryCount))
		view.nodeCount = len(nodeItems(result.Matches, joinAnd, queryCount))
	default:
		view.header.Count = len(result.Matches)
		view.matches, view.header.Truncated = pageItems(result.Matches, limit, offset)
		view.header.Returned = len(view.matches)
	}
	return view
}

func (result Result) header() header {
	books := result.Books
	if books == nil {
		books = []string{}
	}
	kinds := result.Filter.Kinds
	if kinds == nil {
		kinds = []string{}
	}
	types := result.Filter.Types
	if types == nil {
		types = []string{}
	}
	statuses := result.Filter.Statuses
	if statuses == nil {
		statuses = []string{}
	}
	node := ""
	if result.Filter.Node != model.ZeroID {
		node = result.Filter.Node.String()
	}
	block := ""
	if result.Filter.Block != model.ZeroID {
		block = result.Filter.Block.String()
	}
	scopeMode := "book"
	if result.All {
		scopeMode = "all"
	}
	queries := make([]QueryInfo, len(result.Queries))
	fuzzy := 0
	mode := ""
	query := ""
	for index, entry := range result.Queries {
		queries[index] = QueryInfo{Index: index, Text: entry.Text, Mode: string(entry.Mode), Fuzzy: entry.Fuzzy}
		if entry.Fuzzy > fuzzy {
			fuzzy = entry.Fuzzy
		}
		if index == 0 {
			mode = string(entry.Mode)
			query = entry.Text
		}
	}
	join := result.Join
	if join == "" {
		join = "or"
	}
	index := result.Index
	if index.Mode == "" {
		index.Mode = "auto"
	}
	return header{
		SchemaVersion: SchemaVersion,
		Book:          result.Book,
		Query:         query,
		Queries:       queries,
		Join:          join,
		Fuzzy:         fuzzy,
		GroupBy:       result.GroupBy,
		Ref:           result.Ref,
		Index:         index,
		Mode:          mode,
		Scope: ScopeInfo{
			Mode: scopeMode, Books: books, Fields: result.Fields.names(),
			Node: node, Block: block, Kinds: kinds, Types: types,
			Statuses: statuses, Part: result.Filter.Part, Locked: result.Filter.Locked,
		},
	}
}

// Count returns the total number of result items before paging.
func (result Result) Count() int { return result.prepare(0, 0).header.Count }

// Payload returns the JSON payload for the chosen result mode.
func (result Result) Payload(limit, offset int) any {
	view := result.prepare(limit, offset)
	if result.GroupBy != "" {
		if result.CountBy {
			return groupedCountPayload{header: view.header, Groups: view.groups}
		}
		return groupedPayload{header: view.header, Groups: view.groups, Matches: view.matches}
	}
	switch result.Result {
	case "books":
		return booksPayload{header: view.header, Books: view.books}
	case "blocks":
		return blocksPayload{header: view.header, Blocks: view.blocks}
	case "nodes":
		return nodesPayload{header: view.header, Nodes: view.nodes}
	case "count":
		return countPayload{header: view.header, Books: view.bookCount, Blocks: view.blockCount, Nodes: view.nodeCount}
	default:
		return matchesPayload{header: view.header, Matches: view.matches}
	}
}

// NDJSON renders the streaming form: a meta line, the items, the warnings and a summary.
func (result Result) NDJSON(limit, offset int) ([]byte, error) {
	view := result.prepare(limit, offset)
	var buffer bytes.Buffer
	meta := struct {
		Type string `json:"type"`
		header
	}{Type: "meta", header: view.header}
	meta.Count, meta.Returned, meta.Truncated = 0, 0, false
	meta.Occurrences = 0
	if err := writeNDJSON(&buffer, meta); err != nil {
		return nil, err
	}
	if view.grouped {
		if !view.countBy {
			for _, item := range view.matches {
				if err := writeNDJSON(&buffer, ndjsonItem[Match]{Type: "match", Item: item}); err != nil {
					return nil, err
				}
			}
		}
		for _, item := range view.groups {
			if err := writeNDJSON(&buffer, ndjsonItem[GroupItem]{Type: "group", Item: item}); err != nil {
				return nil, err
			}
		}
	} else {
		switch result.Result {
		case "books":
			for _, item := range view.books {
				if err := writeNDJSON(&buffer, ndjsonItem[BookItem]{Type: "book", Item: item}); err != nil {
					return nil, err
				}
			}
		case "blocks":
			for _, item := range view.blocks {
				if err := writeNDJSON(&buffer, ndjsonItem[BlockItem]{Type: "block", Item: item}); err != nil {
					return nil, err
				}
			}
		case "nodes":
			for _, item := range view.nodes {
				if err := writeNDJSON(&buffer, ndjsonItem[NodeItem]{Type: "node", Item: item}); err != nil {
					return nil, err
				}
			}
		default:
			if result.Result != "count" {
				for _, item := range view.matches {
					if err := writeNDJSON(&buffer, ndjsonItem[Match]{Type: "match", Item: item}); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	for _, warning := range view.header.Warnings {
		if err := writeNDJSON(&buffer, ndjsonItem[Warning]{Type: "warning", Item: warning}); err != nil {
			return nil, err
		}
	}
	summary := struct {
		Type        string    `json:"type"`
		Count       int       `json:"count"`
		Occurrences int       `json:"occurrences"`
		Returned    int       `json:"returned"`
		Truncated   bool      `json:"truncated"`
		Warnings    []Warning `json:"warnings"`
		Books       int       `json:"books,omitempty"`
		Blocks      int       `json:"blocks,omitempty"`
		Nodes       int       `json:"nodes,omitempty"`
	}{
		Type: "summary", Count: view.header.Count, Occurrences: view.header.Occurrences,
		Returned: view.header.Returned, Truncated: view.header.Truncated,
		Warnings: view.header.Warnings, Books: view.bookCount, Blocks: view.blockCount, Nodes: view.nodeCount,
	}
	if result.Result != "count" {
		summary.Books, summary.Blocks, summary.Nodes = 0, 0, 0
	}
	if err := writeNDJSON(&buffer, summary); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

type ndjsonItem[T any] struct {
	Type string `json:"type"`
	Item T
}

// MarshalJSON flattens the type marker and the item into one object.
func (item ndjsonItem[T]) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(item.Item)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || data[0] != '{' {
		return json.Marshal(struct {
			Type string `json:"type"`
			Data T      `json:"data"`
		}{Type: item.Type, Data: item.Item})
	}
	var buffer bytes.Buffer
	buffer.WriteString(`{"type":`)
	encoded, err := json.Marshal(item.Type)
	if err != nil {
		return nil, err
	}
	buffer.Write(encoded)
	buffer.WriteString(",")
	buffer.Write(data[1:])
	return buffer.Bytes(), nil
}

func writeNDJSON(buffer *bytes.Buffer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	buffer.Write(data)
	buffer.WriteByte('\n')
	return nil
}

func pageItems[T any](items []T, limit, offset int) ([]T, bool) {
	start := offset
	if start > len(items) {
		start = len(items)
	}
	end := len(items)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	paged := items[start:end]
	if paged == nil {
		paged = []T{}
	}
	return paged, end < len(items)
}

func statsList(stats map[int]*QueryStats) []QueryStats {
	if len(stats) == 0 {
		return nil
	}
	entries := make([]QueryStats, 0, len(stats))
	for _, entry := range stats {
		entries = append(entries, *entry)
	}
	for left := 0; left < len(entries); left++ {
		for right := left + 1; right < len(entries); right++ {
			if entries[right].Query < entries[left].Query {
				entries[left], entries[right] = entries[right], entries[left]
			}
		}
	}
	return entries
}

func collectStats(stats map[int]*QueryStats, match Match) {
	distinct := make(map[int]struct{}, len(match.Spans))
	for _, span := range match.Spans {
		distinct[span.Query] = struct{}{}
		entry := stats[span.Query]
		if entry == nil {
			entry = &QueryStats{Query: span.Query, FirstLine: match.Line}
			stats[span.Query] = entry
		}
		entry.Occurrences++
	}
	for query := range distinct {
		stats[query].Count++
	}
}

func collectQueries(queries map[int]struct{}, match Match) {
	for _, span := range match.Spans {
		queries[span.Query] = struct{}{}
	}
}

func bookItems(matches []Match, joinAnd bool, queryCount int) []BookItem {
	type aggregate struct {
		item    BookItem
		queries map[int]struct{}
		stats   map[int]*QueryStats
	}
	order := make([]string, 0)
	index := map[string]*aggregate{}
	for _, match := range matches {
		entry := index[match.Book]
		if entry == nil {
			entry = &aggregate{
				item:    BookItem{Book: match.Book, Path: match.BookPath},
				queries: map[int]struct{}{},
				stats:   map[int]*QueryStats{},
			}
			index[match.Book] = entry
			order = append(order, match.Book)
		}
		entry.item.Count++
		entry.item.Occurrences += match.Occurrences
		collectStats(entry.stats, match)
		collectQueries(entry.queries, match)
	}
	items := make([]BookItem, 0, len(order))
	for _, key := range order {
		entry := index[key]
		if joinAnd && len(entry.queries) < queryCount {
			continue
		}
		if queryCount > 1 {
			entry.item.PerQuery = statsList(entry.stats)
		}
		items = append(items, entry.item)
	}
	return items
}

func blockItems(matches []Match, joinAnd bool, queryCount int) []BlockItem {
	type aggregate struct {
		item    BlockItem
		queries map[int]struct{}
		stats   map[int]*QueryStats
	}
	order := make([]string, 0)
	index := map[string]*aggregate{}
	for _, match := range matches {
		if match.Block == "" {
			continue
		}
		key := match.Book + "\x00" + match.Block + "\x00" + match.Field
		entry := index[key]
		if entry == nil {
			entry = &aggregate{
				item: BlockItem{
					Book: match.Book, Block: match.Block, Node: match.Node,
					NodeTitle: match.NodeTitle, NodePath: match.NodePath,
					Type: match.BlockType, Status: match.BlockStatus, Locked: match.BlockLocked,
					Field: match.Field, FirstLine: match.Line,
				},
				queries: map[int]struct{}{},
				stats:   map[int]*QueryStats{},
			}
			index[key] = entry
			order = append(order, key)
		}
		entry.item.Count++
		entry.item.Occurrences += match.Occurrences
		collectStats(entry.stats, match)
		collectQueries(entry.queries, match)
	}
	items := make([]BlockItem, 0, len(order))
	for _, key := range order {
		entry := index[key]
		if joinAnd && len(entry.queries) < queryCount {
			continue
		}
		if queryCount > 1 {
			entry.item.PerQuery = statsList(entry.stats)
		}
		items = append(items, entry.item)
	}
	return items
}

func nodeItems(matches []Match, joinAnd bool, queryCount int) []NodeItem {
	type aggregate struct {
		item    NodeItem
		queries map[int]struct{}
		stats   map[int]*QueryStats
	}
	order := make([]string, 0)
	index := map[string]*aggregate{}
	for _, match := range matches {
		if match.Node == "" {
			continue
		}
		key := match.Book + "\x00" + match.Node
		entry := index[key]
		if entry == nil {
			entry = &aggregate{
				item: NodeItem{
					Book: match.Book, Node: match.Node, NodeKind: match.NodeKind,
					Title: match.NodeTitle, Part: match.NodePart, Status: match.NodeStatus,
					NodePath: match.NodePath,
				},
				queries: map[int]struct{}{},
				stats:   map[int]*QueryStats{},
			}
			index[key] = entry
			order = append(order, key)
		}
		entry.item.Count++
		entry.item.Occurrences += match.Occurrences
		collectStats(entry.stats, match)
		collectQueries(entry.queries, match)
	}
	items := make([]NodeItem, 0, len(order))
	for _, key := range order {
		entry := index[key]
		if joinAnd && len(entry.queries) < queryCount {
			continue
		}
		if queryCount > 1 {
			entry.item.PerQuery = statsList(entry.stats)
		}
		items = append(items, entry.item)
	}
	return items
}

func booksOccurrences(items []BookItem) int {
	total := 0
	for _, item := range items {
		total += item.Occurrences
	}
	return total
}

func blocksOccurrences(items []BlockItem) int {
	total := 0
	for _, item := range items {
		total += item.Occurrences
	}
	return total
}

func nodesOccurrences(items []NodeItem) int {
	total := 0
	for _, item := range items {
		total += item.Occurrences
	}
	return total
}

// groupKey derives the group key of one match.
func groupKey(match Match, groupBy string) string {
	key := ""
	switch groupBy {
	case "type":
		key = match.BlockType
	case "status":
		if match.Kind == "title" {
			key = match.NodeStatus
		} else {
			key = match.BlockStatus
		}
	case "kind":
		key = match.NodeKind
	case "part":
		key = match.NodePart
	}
	if key == "" {
		return "unset"
	}
	return key
}

// groupItems aggregates the matches by key. Keys are ordered case-insensitively, and equal keys
// keep the structural order of their first hit.
func groupItems(matches []Match, groupBy string, queryCount int) []GroupItem {
	order := make([]string, 0)
	index := map[string]*GroupItem{}
	stats := map[string]map[int]*QueryStats{}
	for _, match := range matches {
		key := groupKey(match, groupBy)
		item := index[key]
		if item == nil {
			item = &GroupItem{Key: key, FirstLine: match.Line}
			index[key] = item
			order = append(order, key)
			stats[key] = map[int]*QueryStats{}
		}
		item.Count++
		item.Occurrences += match.Occurrences
		collectStats(stats[key], match)
	}
	items := make([]GroupItem, 0, len(order))
	for _, key := range order {
		item := *index[key]
		if queryCount > 1 {
			item.PerQuery = statsList(stats[key])
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(left, right int) bool {
		return strings.ToLower(items[left].Key) < strings.ToLower(items[right].Key)
	})
	return items
}

// Text renders the human form of one result.
func (result Result) Text(limit, offset int) string {
	view := result.prepare(limit, offset)
	var builder strings.Builder
	if result.GroupBy != "" {
		for _, group := range view.groups {
			fmt.Fprintf(&builder, "%s: %d match(es), %d occurrence(s)\n",
				group.Key, group.Count, group.Occurrences)
		}
		if !result.CountBy {
			for _, match := range view.matches {
				target := match.Node
				if match.Block != "" {
					target = match.Block
				}
				fmt.Fprintf(&builder, "%s:%s:%d:%d: %s\n",
					match.Book, target, match.Line, match.Column, match.Text)
			}
		}
		fmt.Fprintf(&builder, "%d group(s)\n", view.header.Count)
		for _, warning := range view.header.Warnings {
			fmt.Fprintf(&builder, "warning: %s: %s\n", warning.Code, warning.Message)
		}
		return builder.String()
	}
	switch result.Result {
	case "books":
		for _, item := range view.books {
			fmt.Fprintf(&builder, "%s (%s): %d match(es), %d occurrence(s)\n",
				item.Book, item.Path, item.Count, item.Occurrences)
		}
	case "blocks":
		for _, item := range view.blocks {
			fmt.Fprintf(&builder, "%s:%s %s:%d: %d match(es), %d occurrence(s)\n",
				item.Book, item.Block, item.Field, item.FirstLine, item.Count, item.Occurrences)
		}
	case "nodes":
		for _, item := range view.nodes {
			fmt.Fprintf(&builder, "%s:%s %s: %d match(es)\n",
				item.Book, item.Node, item.NodePath, item.Count)
		}
	case "count":
		fmt.Fprintf(&builder, "%d match(es), %d occurrence(s) in %d book(s), %d block(s), %d node(s)\n",
			view.header.Count, view.header.Occurrences, view.bookCount, view.blockCount, view.nodeCount)
	default:
		for _, match := range view.matches {
			target := match.Node
			if match.Block != "" {
				target = match.Block
			}
			fmt.Fprintf(&builder, "%s:%s:%d:%d: %s\n",
				match.Book, target, match.Line, match.Column, match.Text)
			for _, line := range match.Context {
				fmt.Fprintf(&builder, "  %s\n", line)
			}
		}
		fmt.Fprintf(&builder, "%d match(es), %d occurrence(s)\n",
			view.header.Count, view.header.Occurrences)
	}
	for _, warning := range view.header.Warnings {
		fmt.Fprintf(&builder, "warning: %s: %s\n", warning.Code, warning.Message)
	}
	return builder.String()
}
