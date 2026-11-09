// Package search finds text in the manifests, bodies and asciiui specifications of the registered
// books. Reads take no lock: the store writes files atomically, so a search always sees complete
// bodies. A closed book is read through its container, an open book through its extracted state.
package search

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/source"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Error is a search failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *Error) ErrorCode() string { return err.Code }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ContextLine is one line of the numbered context around a match.
type ContextLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Match is one matching line or title.
type Match struct {
	Kind        string        `json:"kind"`
	Book        string        `json:"book"`
	BookPath    string        `json:"bookPath"`
	Node        string        `json:"node,omitempty"`
	NodeKind    string        `json:"nodeKind,omitempty"`
	NodeTitle   string        `json:"nodeTitle,omitempty"`
	NodePath    string        `json:"nodePath,omitempty"`
	NodePart    string        `json:"-"`
	NodeStatus  string        `json:"-"`
	Block       string        `json:"block,omitempty"`
	BlockType   string        `json:"blockType,omitempty"`
	BlockStatus string        `json:"blockStatus,omitempty"`
	BlockLocked bool          `json:"blockLocked"`
	Field       string        `json:"field"`
	Line        int           `json:"line"`
	Column      int           `json:"column"`
	Offset      int           `json:"offset"`
	Length      int           `json:"length"`
	Text        string        `json:"text"`
	Spans       []Span        `json:"spans"`
	Occurrences int           `json:"occurrences"`
	Context     []string      `json:"context,omitempty"`
	Before      []ContextLine `json:"contextBefore,omitempty"`
	After       []ContextLine `json:"contextAfter,omitempty"`
}

// Warning is a per-book condition that does not stop a multi-book search.
type Warning struct {
	Code    string `json:"code"`
	Book    string `json:"book,omitempty"`
	Message string `json:"message"`
}

// Filter is the compiled scope filter of one search.
type Filter struct {
	Node        model.ID
	Block       model.ID
	Kinds       []string
	Types       []string
	Statuses    []string
	Part        string
	Locked      *bool
	NodePath    string
	BookPath    string
	Ref         model.ID
	MissingBody bool
}

// Query is one submitted search query.
type Query struct {
	Text  string
	Mode  Mode
	Fuzzy int
}

// Options controls one search.
type Options struct {
	// Book scopes the search to one registered book; All searches every registered book.
	Book model.BookID
	All  bool

	// Queries are the submitted queries. Query is the single-query compatibility field.
	Queries []Query
	Query   string
	Fields  []string
	Mode    Mode
	// Join is and or or; the empty value means or.
	Join       string
	IgnoreCase bool
	Word       bool
	Context    int
	Filter     Filter

	// Result is matches, books, blocks, nodes or count. The empty value means matches.
	Result string
	Limit  int
	Offset int
	Format string

	// FieldsExplicit reports whether the caller set --field; a filter-only listing needs it.
	FieldsExplicit bool
	RequireMatch   bool
	Snapshot       bool
	Strict         bool
}

// Result is the complete unscoped search result before paging.
type Result struct {
	Book     string
	All      bool
	Queries  []Query
	Join     string
	Fields   fieldFlags
	Filter   Filter
	Result   string
	Ref      *RefInfo
	Books    []string
	Matches  []Match
	Warnings []Warning
	// A filter-only listing has no matches; the entities are collected directly.
	FilterOnly bool
	ListBlocks []BlockItem
	ListNodes  []NodeItem
	ListBooks  []BookItem
}

// Service searches registered books.
type Service struct {
	Lifecycle *lifecycle.Lifecycle
	Registry  *store.Registry
	Sources   *source.Service
}

// New returns a search service over one registry.
func New(life *lifecycle.Lifecycle, registry *store.Registry) *Service {
	return &Service{Lifecycle: life, Registry: registry, Sources: source.New(life)}
}

// Find runs one search. It never opens a book and takes no lock.
func (service *Service) Find(options Options) (Result, error) {
	if options.All && options.Book != "" {
		return Result{}, fail("search.scope-conflict", "use either --book or --all")
	}
	if !options.All && options.Book == "" {
		return Result{}, fail("search.scope-required", "one of --book or --all is required")
	}
	var queries []Query
	filterOnly := len(options.Queries) == 0 && strings.TrimSpace(options.Query) == ""
	var compiled []*Matcher
	join := options.Join
	if join == "" {
		join = "or"
	}
	if join != "and" && join != "or" {
		return Result{}, fail("search.invalid-join", "unknown join %q; use and or or", join)
	}
	fields, err := parseFields(options.Fields)
	if err != nil {
		return Result{}, err
	}
	if filterOnly {
		if filterErr := validateFilterOnly(options); filterErr != nil {
			return Result{}, filterErr
		}
	} else {
		queries, err = compileQueries(options)
		if err != nil {
			return Result{}, err
		}
		if options.Filter.MissingBody {
			return Result{}, fail("search.missing-body-query", "--missing-body does not take a query")
		}
		compiled = make([]*Matcher, len(queries))
		for index, query := range queries {
			matcher, matchErr := Compile(query.Text, MatcherOptions{
				Mode: query.Mode, IgnoreCase: options.IgnoreCase, Word: options.Word, Fuzzy: query.Fuzzy,
			})
			if matchErr != nil {
				return Result{}, matchErr
			}
			compiled[index] = matcher
		}
	}
	entries, err := service.scope(options)
	if err != nil {
		return Result{}, err
	}
	if options.Filter.BookPath != "" {
		entries, err = filterBookPaths(entries, options.Filter.BookPath, options.IgnoreCase)
		if err != nil {
			return Result{}, err
		}
	}
	result := Result{
		Book: string(options.Book), All: options.All, Queries: queries, Join: join,
		Fields: fields, Filter: options.Filter, Result: options.Result, Warnings: []Warning{},
		FilterOnly: filterOnly,
	}
	if options.Filter.Ref != model.ZeroID {
		result.Ref = &RefInfo{ID: options.Filter.Ref.String()}
	}
	entries, warnings, err := service.resolveOwners(entries, options)
	result.Warnings = append(result.Warnings, warnings...)
	if err != nil {
		return Result{}, err
	}
	for _, entry := range entries {
		if bookErr := service.searchBook(&result, entry, options, compiled, fields, join, filterOnly); bookErr != nil {
			var searchErr *Error
			if options.All && errors.As(bookErr, &searchErr) && bookWarning(searchErr.Code) {
				result.Warnings = append(result.Warnings, Warning{
					Code: searchErr.Code, Book: string(entry.ID), Message: searchErr.Message,
				})
				continue
			}
			return Result{}, bookErr
		}
	}
	if options.Strict && len(result.Warnings) > 0 {
		return Result{}, fail("search.strict-failed", "%d warning(s) treated as failure", len(result.Warnings))
	}
	return result, nil
}

// searchBook loads, locks and searches one registered book.
func (service *Service) searchBook(result *Result, entry store.RegistryEntry, options Options, matchers []*Matcher, fields fieldFlags, join string, filterOnly bool) error {
	if options.Snapshot {
		release, lockErr := service.Lifecycle.LockBookShared(entry.ID)
		if lockErr != nil {
			return fail("search.lock-failed", "book %q: %v", entry.ID, lockErr)
		}
		defer release()
	}
	book, err := service.Sources.Load(entry.ID)
	if err != nil {
		return loadFailure(err)
	}
	result.Books = append(result.Books, string(entry.ID))
	for _, id := range book.MissingBodies() {
		result.Warnings = append(result.Warnings, Warning{
			Code: "search.missing-body", Book: string(entry.ID),
			Message: fmt.Sprintf("block %s has no body", id),
		})
	}
	if result.Ref != nil && ownsID(book, options.Filter.Ref) {
		result.Ref.Exists = true
	}
	info := newBookInfo(book, entry.Path)
	set, filterErr := newFilterSet(info, options.Filter, options.IgnoreCase)
	if filterErr != nil {
		return filterErr
	}
	if filterOnly {
		listFilterOnly(result, info, set, fields)
		return nil
	}
	scanBook(result, info, set, matchers, fields, options.Context, join, options.Result)
	return nil
}

// bookWarning reports whether a per-book error is a warning in an --all search.
func bookWarning(code string) bool {
	return code == "search.read-failed" || code == "search.unsupported-schema"
}

// validateFilterOnly enforces the filter-only listing contract.
func validateFilterOnly(options Options) error {
	switch options.Result {
	case "blocks", "nodes", "books", "count":
	default:
		return fail("search.empty-query", "a query is required for --result matches")
	}
	if !options.FieldsExplicit && filterEmpty(options.Filter) {
		return fail("search.empty-query", "a query or an explicit filter is required")
	}
	return nil
}

func filterEmpty(filter Filter) bool {
	return filter.Node == model.ZeroID && filter.Block == model.ZeroID &&
		len(filter.Kinds) == 0 && len(filter.Types) == 0 && len(filter.Statuses) == 0 &&
		filter.Part == "" && filter.Locked == nil && filter.NodePath == "" &&
		filter.BookPath == "" && filter.Ref == model.ZeroID && !filter.MissingBody
}

// filterBookPaths keeps the entries whose output path matches the glob.
func filterBookPaths(entries []store.RegistryEntry, pattern string, ignoreCase bool) ([]store.RegistryEntry, error) {
	expression, err := regexp.Compile(globExpression(pattern, ignoreCase))
	if err != nil {
		return nil, fail("search.invalid-glob", "invalid book path glob %q: %v", pattern, err)
	}
	filtered := make([]store.RegistryEntry, 0, len(entries))
	for _, entry := range entries {
		if expression.MatchString(entry.Path) {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

// listFilterOnly collects the entities that pass the filters without a text query.
func listFilterOnly(result *Result, info *bookInfo, set *filterSet, fields fieldFlags) {
	seen := map[model.ID]struct{}{}
	blocks := 0
	for _, node := range info.Ordered {
		if set.titleAllowed(node) {
			seen[node.Node.ID] = struct{}{}
		}
		for _, id := range node.Node.Blocks {
			if !set.blockAllowed(info, node, id, nil) {
				continue
			}
			blocks += listBlock(result, info, node, id, nil, fields)
			seen[node.Node.ID] = struct{}{}
		}
	}
	for _, region := range info.Book.Manifest.Regions {
		field := regionField(region.Kind)
		if !fields.has(field) {
			continue
		}
		owner := info.regionNode(region)
		for _, id := range region.Blocks {
			if !set.blockAllowed(info, owner, id, region) {
				continue
			}
			blocks += listBlock(result, info, owner, id, region, fields)
			if owner != nil {
				seen[owner.Node.ID] = struct{}{}
			}
		}
	}
	for _, node := range info.Ordered {
		if _, exists := seen[node.Node.ID]; !exists {
			continue
		}
		result.ListNodes = append(result.ListNodes, NodeItem{
			Book: string(info.Book.ID), Node: node.Node.ID.String(),
			NodeKind: string(node.Node.Kind), Title: node.Node.Title, Part: node.Part,
			Status: node.Node.Status, NodePath: node.Path,
		})
	}
	if blocks > 0 || len(seen) > 0 {
		result.ListBooks = append(result.ListBooks, BookItem{Book: string(info.Book.ID), Path: info.Path})
	}
}

func listBlock(result *Result, info *bookInfo, owner *nodeInfo, block model.ID, region *model.Region, fields fieldFlags) int {
	listed := 0
	meta := info.Book.Manifest.Meta(block)
	for _, field := range listedFields(info, block, region, fields) {
		item := BlockItem{
			Book: string(info.Book.ID), Block: block.String(), Field: field,
			Type: meta.Type, Status: meta.Status, Locked: meta.Locked,
		}
		if owner != nil {
			item.Node = owner.Node.ID.String()
			item.NodeTitle = owner.Node.Title
			item.NodePath = owner.Path
		}
		result.ListBlocks = append(result.ListBlocks, item)
		listed++
	}
	return listed
}

func listedFields(info *bookInfo, block model.ID, region *model.Region, fields fieldFlags) []string {
	if region != nil {
		field := regionField(region.Kind)
		if fields.has(field) {
			return []string{field}
		}
		return nil
	}
	names := make([]string, 0, 2)
	if fields.body {
		names = append(names, "body")
	}
	if fields.layout {
		if _, exists := info.Book.Layout(block); exists {
			names = append(names, "layout")
		}
	}
	return names
}

// compileQueries normalizes the submitted queries and rejects duplicate or excessive lists.
func compileQueries(options Options) ([]Query, error) {
	queries := options.Queries
	if len(queries) == 0 {
		if strings.TrimSpace(options.Query) == "" {
			return nil, fail("search.empty-query", "a query is required")
		}
		queries = []Query{{Text: options.Query, Mode: options.Mode}}
	}
	if len(queries) > 8 {
		return nil, fail("search.too-many-queries", "at most 8 queries are allowed")
	}
	seen := make(map[string]struct{}, len(queries))
	normalized := make([]Query, len(queries))
	for index, query := range queries {
		key := strings.TrimSpace(query.Text)
		if key == "" {
			return nil, fail("search.empty-query", "query %d is empty", index+1)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, fail("search.duplicate-query", "query %q is repeated", key)
		}
		seen[key] = struct{}{}
		mode := query.Mode
		if mode == "" {
			mode = options.Mode
		}
		if mode == "" {
			mode = ModeLiteral
		}
		normalized[index] = Query{Text: query.Text, Mode: mode, Fuzzy: query.Fuzzy}
	}
	return normalized, nil
}

func (service *Service) scope(options Options) ([]store.RegistryEntry, error) {
	if service.Registry == nil {
		if options.All {
			return nil, nil
		}
		return nil, fail("search.unknown-book", "book %q is not registered", options.Book)
	}
	if options.All {
		return service.Registry.Books, nil
	}
	entry, exists := service.Registry.Find(options.Book)
	if !exists {
		return nil, fail("search.unknown-book", "book %q is not registered", options.Book)
	}
	return []store.RegistryEntry{entry}, nil
}

// resolveOwners narrows an --all search with --node or --block to the books that own the ID. A
// duplicate owner is a warning, not a silent pick.
func (service *Service) resolveOwners(entries []store.RegistryEntry, options Options) ([]store.RegistryEntry, []Warning, error) {
	target := options.Filter.Node
	if target == model.ZeroID {
		target = options.Filter.Block
	}
	if !options.All || target == model.ZeroID {
		return entries, nil, nil
	}
	owners := make([]store.RegistryEntry, 0, 1)
	warnings := make([]Warning, 0, 1)
	for _, entry := range entries {
		book, err := service.Sources.LoadManifest(entry.ID)
		if err != nil {
			warnings = append(warnings, Warning{
				Code: "search.read-failed", Book: string(entry.ID), Message: err.Error(),
			})
			continue
		}
		if !ownsID(book, target) {
			continue
		}
		owners = append(owners, entry)
	}
	switch len(owners) {
	case 0:
		return nil, warnings, fail("search.unknown-id", "%s is not owned by any registered book", target)
	case 1:
		return owners, warnings, nil
	}
	names := make([]string, 0, len(owners))
	for _, entry := range owners {
		names = append(names, string(entry.ID))
	}
	warnings = append(warnings, Warning{
		Code:    "search.duplicate-id",
		Message: fmt.Sprintf("%s is owned by %s", target, strings.Join(names, ", ")),
	})
	return owners, warnings, nil
}

func ownsID(book *source.Book, id model.ID) bool {
	if book == nil || book.Manifest == nil {
		return false
	}
	for _, owned := range book.Manifest.OwnedIDs() {
		if owned == id {
			return true
		}
	}
	return false
}

// loadFailure turns a read error into a search code while keeping a model version gap explicit.
func loadFailure(err error) error {
	coded, ok := err.(interface{ ErrorCode() string })
	if ok && strings.Contains(coded.ErrorCode(), "version") {
		return fail("search.unsupported-schema", "%v", err)
	}
	var searchErr *Error
	if errors.As(err, &searchErr) {
		return err
	}
	return fail("search.read-failed", "%v", err)
}

// scanBook appends the matches of one book in structural order.
func scanBook(result *Result, info *bookInfo, set *filterSet, matchers []*Matcher, fields fieldFlags, context int, join, resultMode string) {
	manifest := info.Book.Manifest
	lineJoin := resultMode == "" || resultMode == "matches" || resultMode == "count"
	joinAnd := join == "and" && len(matchers) > 1
	for _, node := range info.Ordered {
		if fields.title && set.titleAllowed(node) {
			spans := matchLine(matchers, node.Node.Title)
			if len(spans) > 0 && (!lineJoin || !joinAnd || coversQueries(spans, len(matchers))) {
				result.Matches = append(result.Matches,
					newMatch(info, node, model.ZeroID, nil, "title", 1, node.Node.Title, spans))
			}
		}
		for _, id := range node.Node.Blocks {
			if fields.body && set.blockAllowed(info, node, id, nil) && manifest.BlockType(id) != "RepoLayout" {
				if body, exists := info.Book.Body(id); exists {
					appendLines(result, info, node, id, nil, "body", body, matchers, context, lineJoin, joinAnd)
				}
			}
			if fields.layout && set.blockAllowed(info, node, id, nil) {
				if spec, exists := info.Book.Layout(id); exists {
					appendLines(result, info, node, id, nil, "layout", string(spec), matchers, context, lineJoin, joinAnd)
				}
			}
		}
	}
	for _, region := range manifest.Regions {
		field := regionField(region.Kind)
		if !fields.has(field) {
			continue
		}
		owner := info.regionNode(region)
		for _, id := range region.Blocks {
			if !set.blockAllowed(info, owner, id, region) {
				continue
			}
			if body, exists := info.Book.Body(id); exists {
				appendLines(result, info, owner, id, region, field, body, matchers, context, lineJoin, joinAnd)
			}
		}
	}
}

func regionField(kind model.RegionKind) string {
	switch kind {
	case model.RegionProse:
		return "prose"
	case model.RegionNotes:
		return "note"
	case model.RegionFooter:
		return "footer"
	}
	return ""
}

func (flags fieldFlags) has(field string) bool {
	switch field {
	case "title":
		return flags.title
	case "body":
		return flags.body
	case "layout":
		return flags.layout
	case "prose":
		return flags.prose
	case "note":
		return flags.note
	case "footer":
		return flags.footer
	}
	return false
}

func appendLines(result *Result, info *bookInfo, owner *nodeInfo, block model.ID, region *model.Region, field, text string, matchers []*Matcher, context int, lineJoin, joinAnd bool) {
	lines := splitLines(text)
	for index, line := range lines {
		spans := matchLine(matchers, line)
		if len(spans) == 0 {
			continue
		}
		if lineJoin && joinAnd && !coversQueries(spans, len(matchers)) {
			continue
		}
		match := newMatch(info, owner, block, region, field, index+1, line, spans)
		if context > 0 {
			match.Context = contextLines(lines, index, context)
			match.Before = contextBefore(lines, index, context)
			match.After = contextAfter(lines, index, context)
		}
		result.Matches = append(result.Matches, match)
	}
}

// matchLine collects the spans of every query on one line and sorts them by position.
func matchLine(matchers []*Matcher, line string) []Span {
	spans := make([]Span, 0)
	for index, matcher := range matchers {
		for _, span := range matcher.Match(line) {
			span.Query = index
			spans = append(spans, span)
		}
	}
	sort.Slice(spans, func(left, right int) bool {
		if spans[left].Offset != spans[right].Offset {
			return spans[left].Offset < spans[right].Offset
		}
		return spans[left].Query < spans[right].Query
	})
	return spans
}

// coversQueries reports whether the spans contain a hit for every query.
func coversQueries(spans []Span, count int) bool {
	seen := make(map[int]struct{}, count)
	for _, span := range spans {
		seen[span.Query] = struct{}{}
	}
	return len(seen) == count
}

func newMatch(info *bookInfo, owner *nodeInfo, block model.ID, region *model.Region, field string, line int, text string, spans []Span) Match {
	match := Match{
		Kind: "block", Book: string(info.Book.ID), BookPath: info.Path,
		Field: field, Line: line, Text: text, Spans: spans, Occurrences: len(spans),
		Column: spans[0].Column, Offset: spans[0].Offset, Length: spans[0].Length,
	}
	if field == "title" {
		match.Kind = "title"
	}
	if owner != nil {
		match.Node = owner.Node.ID.String()
		match.NodeKind = string(owner.Node.Kind)
		match.NodeTitle = owner.Node.Title
		match.NodePath = owner.Path
		match.NodePart = owner.Part
		match.NodeStatus = owner.Node.Status
	}
	if block != model.ZeroID {
		meta := info.Book.Manifest.Meta(block)
		match.Block = block.String()
		match.BlockType = meta.Type
		match.BlockStatus = meta.Status
		match.BlockLocked = meta.Locked
	}
	return match
}

func splitLines(text string) []string {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func contextLines(lines []string, index, radius int) []string {
	if radius <= 0 {
		return nil
	}
	start := index - radius
	if start < 0 {
		start = 0
	}
	end := index + radius + 1
	if end > len(lines) {
		end = len(lines)
	}
	context := make([]string, 0, end-start)
	context = append(context, lines[start:end]...)
	return context
}

func contextBefore(lines []string, index, radius int) []ContextLine {
	if radius <= 0 {
		return nil
	}
	start := index - radius
	if start < 0 {
		start = 0
	}
	entries := make([]ContextLine, 0, index-start)
	for position := start; position < index; position++ {
		entries = append(entries, ContextLine{Line: position + 1, Text: lines[position]})
	}
	return entries
}

func contextAfter(lines []string, index, radius int) []ContextLine {
	if radius <= 0 {
		return nil
	}
	end := index + radius + 1
	if end > len(lines) {
		end = len(lines)
	}
	entries := make([]ContextLine, 0, end-index-1)
	for position := index + 1; position < end; position++ {
		entries = append(entries, ContextLine{Line: position + 1, Text: lines[position]})
	}
	return entries
}

func wordBoundary(text string, start, end int) bool {
	if start > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsLetter(previous) || unicode.IsDigit(previous) || previous == '_' {
			return false
		}
	}
	if end < len(text) {
		next, _ := utf8.DecodeRuneInString(text[end:])
		if unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_' {
			return false
		}
	}
	return true
}
