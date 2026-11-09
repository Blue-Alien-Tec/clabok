package search

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	Book        string `json:"book"`
	Path        string `json:"path"`
	Count       int    `json:"count"`
	Occurrences int    `json:"occurrences"`
}

// BlockItem aggregates the matches of one block and field.
type BlockItem struct {
	Book        string `json:"book"`
	Block       string `json:"block"`
	Node        string `json:"node,omitempty"`
	NodeTitle   string `json:"nodeTitle,omitempty"`
	NodePath    string `json:"nodePath,omitempty"`
	Type        string `json:"type,omitempty"`
	Status      string `json:"status,omitempty"`
	Locked      bool   `json:"locked"`
	Field       string `json:"field"`
	FirstLine   int    `json:"firstLine"`
	Count       int    `json:"count"`
	Occurrences int    `json:"occurrences"`
}

// NodeItem aggregates the direct matches of one node.
type NodeItem struct {
	Book     string `json:"book"`
	Node     string `json:"node"`
	NodeKind string `json:"nodeKind"`
	Title    string `json:"title"`
	Part     string `json:"part,omitempty"`
	Status   string `json:"status,omitempty"`
	NodePath string `json:"nodePath"`
	Count    int    `json:"count"`
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

// prepared is the paged view of one result.
type prepared struct {
	header  header
	matches []Match
	books   []BookItem
	blocks  []BlockItem
	nodes   []NodeItem
	// counts holds the distinct entity counts for the count result.
	bookCount  int
	blockCount int
	nodeCount  int
}

// prepare computes the units, the counters and the paging of one result.
func (result Result) prepare(limit, offset int) prepared {
	view := prepared{header: result.header()}
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
	switch mode {
	case "books":
		all := bookItems(result.Matches)
		view.bookCount = len(all)
		view.header.Count = len(all)
		view.books, view.header.Truncated = pageItems(all, limit, offset)
		view.header.Returned = len(view.books)
	case "blocks":
		all := blockItems(result.Matches)
		view.blockCount = len(all)
		view.header.Count = len(all)
		view.blocks, view.header.Truncated = pageItems(all, limit, offset)
		view.header.Returned = len(view.blocks)
	case "nodes":
		all := nodeItems(result.Matches)
		view.nodeCount = len(all)
		view.header.Count = len(all)
		view.nodes, view.header.Truncated = pageItems(all, limit, offset)
		view.header.Returned = len(view.nodes)
	case "count":
		view.header.Count = len(result.Matches)
		view.bookCount = len(bookItems(result.Matches))
		view.blockCount = len(blockItems(result.Matches))
		view.nodeCount = len(nodeItems(result.Matches))
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
	return header{
		SchemaVersion: SchemaVersion,
		Book:          result.Book,
		Query:         result.Query,
		Queries:       []QueryInfo{{Index: 0, Text: result.Query, Mode: string(result.Mode), Fuzzy: 0}},
		Join:          "or",
		GroupBy:       "",
		Index:         IndexInfo{Mode: "auto"},
		Mode:          string(result.Mode),
		Scope: ScopeInfo{
			Mode: scopeMode, Books: books, Fields: result.Fields.names(),
			Node: node, Block: block, Kinds: kinds, Types: types,
			Statuses: statuses, Part: result.Filter.Part, Locked: result.Filter.Locked,
		},
	}
}

// Payload returns the JSON payload for the chosen result mode.
func (result Result) Payload(limit, offset int) any {
	view := result.prepare(limit, offset)
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

func bookItems(matches []Match) []BookItem {
	items := make([]BookItem, 0)
	index := map[string]int{}
	for _, match := range matches {
		position, exists := index[match.Book]
		if !exists {
			position = len(items)
			index[match.Book] = position
			items = append(items, BookItem{Book: match.Book, Path: match.BookPath})
		}
		items[position].Count++
		items[position].Occurrences += match.Occurrences
	}
	return items
}

func blockItems(matches []Match) []BlockItem {
	items := make([]BlockItem, 0)
	index := map[string]int{}
	for _, match := range matches {
		if match.Block == "" {
			continue
		}
		key := match.Book + "\x00" + match.Block + "\x00" + match.Field
		position, exists := index[key]
		if !exists {
			position = len(items)
			index[key] = position
			items = append(items, BlockItem{
				Book: match.Book, Block: match.Block, Node: match.Node,
				NodeTitle: match.NodeTitle, NodePath: match.NodePath,
				Type: match.BlockType, Status: match.BlockStatus, Locked: match.BlockLocked,
				Field: match.Field, FirstLine: match.Line,
			})
		}
		items[position].Count++
		items[position].Occurrences += match.Occurrences
	}
	return items
}

func nodeItems(matches []Match) []NodeItem {
	items := make([]NodeItem, 0)
	index := map[string]int{}
	for _, match := range matches {
		if match.Node == "" {
			continue
		}
		key := match.Book + "\x00" + match.Node
		position, exists := index[key]
		if !exists {
			position = len(items)
			index[key] = position
			items = append(items, NodeItem{
				Book: match.Book, Node: match.Node, NodeKind: match.NodeKind,
				Title: match.NodeTitle, Part: match.NodePart, Status: match.NodeStatus,
				NodePath: match.NodePath,
			})
		}
		items[position].Count++
	}
	return items
}

// Text renders the human form of one result.
func (result Result) Text(limit, offset int) string {
	view := result.prepare(limit, offset)
	var builder strings.Builder
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
