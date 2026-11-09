// Package search finds text in the manifests, bodies and asciiui specifications of the registered
// books. Reads take no lock: the store writes files atomically, so a search always sees complete
// bodies. A closed book is read through its container, an open book through its extracted state.
package search

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Blue-Alien-Tec/clabok/internal/index"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
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
	File        string        `json:"file,omitempty"`
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
	// GroupBy is type, status, kind or part; CountBy suppresses the match list.
	GroupBy string
	CountBy bool
	// IndexMode is auto, force or off; Verify re-checks indexed lines against the source.
	IndexMode string
	Verify    bool
	Strict    bool
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
	GroupBy  string
	CountBy  bool
	Index    IndexInfo
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
	Index     *index.Service
}

// New returns a search service over one registry.
func New(life *lifecycle.Lifecycle, registry *store.Registry, root string) *Service {
	return &Service{
		Lifecycle: life, Registry: registry, Sources: source.New(life),
		Index: index.New(life, registry, root),
	}
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
		options.Queries = queries
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
	indexMode := options.IndexMode
	if indexMode == "" {
		indexMode = "auto"
	}
	result := Result{
		Book: string(options.Book), All: options.All, Queries: queries, Join: join,
		Fields: fields, Filter: options.Filter, Result: options.Result, Warnings: []Warning{},
		FilterOnly: filterOnly, GroupBy: options.GroupBy, CountBy: options.CountBy,
		Index: IndexInfo{Mode: indexMode},
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
	if !filterOnly {
		handled, indexErr := service.tryIndexed(result, entry, options, matchers, fields, join)
		if indexErr != nil {
			return indexErr
		}
		if handled {
			return nil
		}
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

// tryIndexed uses a fresh index when it can answer the request. It returns handled=false when the
// caller must scan, and reports the documented index codes under --index force.
func (service *Service) tryIndexed(result *Result, entry store.RegistryEntry, options Options, matchers []*Matcher, fields fieldFlags, join string) (bool, error) {
	mode := result.Index.Mode
	if mode == "off" {
		return false, nil
	}
	state, err := service.Lifecycle.State(entry.ID)
	if err != nil {
		return false, err
	}
	status := "open"
	if !state.HasEntry && !state.Extracted {
		observed, statusErr := service.Index.Status(entry.ID)
		if statusErr != nil {
			return false, statusErr
		}
		status = observed.State
	}
	if status == "fresh" {
		result.Index.Fresh = true
	}
	usable := indexUsable(options)
	if status == "fresh" && usable {
		if indexErr := service.searchIndexed(result, entry, options, matchers, fields, join); indexErr != nil {
			return false, indexErr
		}
		result.Index.Used = true
		return true, nil
	}
	if mode == "force" {
		switch status {
		case "fresh":
			return false, fail("index.unsupported", "book %q cannot be served by the index", entry.ID)
		case "open":
			return false, fail("index.open-book", "book %q is open; close it to use the index", entry.ID)
		default:
			return false, fail("index."+status, "book %q has no fresh index", entry.ID)
		}
	}
	return false, nil
}

// indexUsable reports whether the request can be answered from postings alone.
func indexUsable(options Options) bool {
	if options.Filter.Block != model.ZeroID || options.Filter.Ref != model.ZeroID || options.Filter.MissingBody {
		return false
	}
	if len(options.Queries) == 0 {
		return false
	}
	for _, query := range options.Queries {
		if _, ok := queryCandidateFor(query); !ok {
			return false
		}
	}
	return true
}

// queryCandidate is the postings lookup of one query.
type queryCandidate struct {
	tokens      []string
	trigrams    []string
	fuzzyTokens []string
	distance    int
}

func queryCandidateFor(query Query) (queryCandidate, bool) {
	if query.Mode == ModeFuzzy || query.Fuzzy > 0 {
		distance := query.Fuzzy
		if distance == 0 {
			distance = 1
		}
		words := tokenize(query.Text)
		if len(words) == 0 {
			return queryCandidate{}, false
		}
		tokens := make([]string, len(words))
		for index, word := range words {
			tokens[index] = strings.ToLower(word.Text)
		}
		return queryCandidate{fuzzyTokens: tokens, distance: distance}, true
	}
	switch query.Mode {
	case ModeLiteral:
		words := tokenize(query.Text)
		if len(words) == 0 {
			return queryCandidate{}, false
		}
		longest := words[0].Text
		for _, word := range words {
			if len(word.Text) > len(longest) {
				longest = word.Text
			}
		}
		return queryCandidate{tokens: []string{strings.ToLower(longest)}}, true
	case ModeGlob:
		return prefixCandidate(globPrefix(query.Text))
	case ModeRegex:
		return prefixCandidate(regexPrefix(query.Text))
	}
	return queryCandidate{}, false
}

func prefixCandidate(prefix string) (queryCandidate, bool) {
	runes := []rune(strings.ToLower(prefix))
	if len(runes) < 3 {
		return queryCandidate{}, false
	}
	trigrams := make([]string, 0, len(runes)-2)
	for index := 0; index+3 <= len(runes); index++ {
		trigrams = append(trigrams, string(runes[index:index+3]))
	}
	return queryCandidate{trigrams: trigrams}, true
}

func globPrefix(pattern string) string {
	var builder strings.Builder
	for _, character := range pattern {
		switch character {
		case '*', '?', '\\':
			return builder.String()
		}
		builder.WriteRune(character)
	}
	return builder.String()
}

func regexPrefix(pattern string) string {
	var builder strings.Builder
	for _, character := range pattern {
		if strings.ContainsRune(`\.+*?()[]{}|^$`, character) {
			return builder.String()
		}
		builder.WriteRune(character)
	}
	return builder.String()
}

func candidateSets(postings *index.Postings, queries []Query) []map[int]struct{} {
	sets := make([]map[int]struct{}, len(queries))
	for queryIndex, query := range queries {
		candidate, ok := queryCandidateFor(query)
		if !ok {
			sets[queryIndex] = map[int]struct{}{}
			continue
		}
		set := map[int]struct{}{}
		if len(candidate.fuzzyTokens) > 0 {
			for _, word := range candidate.fuzzyTokens {
				wordLength := len([]rune(word))
				for token, positions := range postings.Tokens {
					tokenLength := len([]rune(token))
					if wordLength-tokenLength > candidate.distance || tokenLength-wordLength > candidate.distance {
						continue
					}
					if editDistance(word, token) > candidate.distance {
						continue
					}
					for _, position := range positions {
						set[position] = struct{}{}
					}
				}
			}
		} else if len(candidate.tokens) > 0 {
			for _, token := range candidate.tokens {
				for _, position := range postings.Tokens[token] {
					set[position] = struct{}{}
				}
			}
		} else {
			first := true
			for _, trigram := range candidate.trigrams {
				positions := postings.Trigrams[trigram]
				if first {
					for _, position := range positions {
						set[position] = struct{}{}
					}
					first = false
					continue
				}
				next := map[int]struct{}{}
				for _, position := range positions {
					if _, exists := set[position]; exists {
						next[position] = struct{}{}
					}
				}
				set = next
			}
		}
		sets[queryIndex] = set
	}
	return sets
}

func selectCandidates(sets []map[int]struct{}, join string) map[int]struct{} {
	if len(sets) == 0 {
		return nil
	}
	if join != "and" {
		union := map[int]struct{}{}
		for _, set := range sets {
			for position := range set {
				union[position] = struct{}{}
			}
		}
		return union
	}
	selected := sets[0]
	for _, set := range sets[1:] {
		next := map[int]struct{}{}
		for position := range selected {
			if _, exists := set[position]; exists {
				next[position] = struct{}{}
			}
		}
		selected = next
	}
	return selected
}

// searchIndexed answers a search from the derived index without loading bodies.
func (service *Service) searchIndexed(result *Result, entry store.RegistryEntry, options Options, matchers []*Matcher, fields fieldFlags, join string) error {
	postings, err := service.Index.LoadPostings(entry.ID)
	if err != nil {
		return fail("index.corrupt", "%v", err)
	}
	book, err := service.Sources.LoadManifest(entry.ID)
	if err != nil {
		return loadFailure(err)
	}
	result.Books = append(result.Books, string(entry.ID))
	for _, id := range postings.Missing {
		result.Warnings = append(result.Warnings, Warning{
			Code: "search.missing-body", Book: string(entry.ID),
			Message: fmt.Sprintf("block %s has no body", id),
		})
	}
	if result.Ref != nil && ownsID(book, options.Filter.Ref) {
		result.Ref.Exists = true
	}
	info := newBookInfo(book, entry.Path)
	files := render.LayoutFor(book.Manifest)
	set, filterErr := newFilterSet(info, options.Filter, options.IgnoreCase)
	if filterErr != nil {
		return filterErr
	}
	selected := selectCandidates(candidateSets(postings, options.Queries), join)
	if selected == nil {
		return nil
	}
	positions := make([]int, 0, len(selected))
	for position := range selected {
		positions = append(positions, position)
	}
	texts, textErr := service.Index.ReadTexts(entry.ID, postings, positions)
	if textErr != nil {
		return fail("index.read-failed", "%v", textErr)
	}
	var bodies map[model.ID]string
	if options.Verify {
		blocks := make([]model.ID, 0)
		seen := map[model.ID]struct{}{}
		for position := range selected {
			if position >= len(postings.Entries) {
				continue
			}
			value := postings.Entries[position]
			if value.Field != "body" || value.Block == "" {
				continue
			}
			id, parseErr := model.ParseID(value.Block)
			if parseErr != nil {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			blocks = append(blocks, id)
		}
		if len(blocks) > 0 {
			loaded, loadErr := service.Sources.LoadBodies(entry.ID, blocks)
			if loadErr != nil {
				return loadFailure(loadErr)
			}
			bodies = loaded.Bodies
		}
	}
	lineJoin := options.Result == "" || options.Result == "matches" ||
		options.Result == "count" || options.GroupBy != ""
	joinAnd := join == "and" && len(matchers) > 1
	for position := 0; position < len(postings.Entries); position++ {
		if _, exists := selected[position]; !exists {
			continue
		}
		value := postings.Entries[position]
		if !fields.has(value.Field) {
			continue
		}
		var blockID model.ID
		if value.Block != "" {
			parsed, parseErr := model.ParseID(value.Block)
			if parseErr != nil {
				continue
			}
			blockID = parsed
		}
		var owner *nodeInfo
		if value.Node != "" {
			if parsed, parseErr := model.ParseID(value.Node); parseErr == nil {
				owner = info.Nodes[parsed]
			}
		}
		if value.Field == "title" {
			if !set.titleAllowed(owner) {
				continue
			}
		} else if !set.blockAllowed(info, owner, blockID, nil) {
			continue
		}
		text, exists := texts[position]
		if !exists {
			continue
		}
		if bodies != nil && value.Field == "body" {
			if body, exists := bodies[blockID]; exists {
				lines := splitLines(body)
				if value.Line-1 < len(lines) && lines[value.Line-1] != text {
					result.Warnings = append(result.Warnings, Warning{
						Code: "index.verify-mismatch", Book: string(entry.ID),
						Message: fmt.Sprintf("block %s line %d differs from the index", blockID, value.Line),
					})
					text = lines[value.Line-1]
				}
			}
		}
		spans := matchLine(matchers, text)
		if len(spans) == 0 {
			continue
		}
		if lineJoin && joinAnd && !coversQueries(spans, len(matchers)) {
			continue
		}
		result.Matches = append(result.Matches,
			newMatch(info, owner, blockID, nil, value.Field, value.Line, text, spans, files))
	}
	return nil
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
	useIndex := options.IndexMode != "off"
	for _, entry := range entries {
		owns, ownErr := service.ownsBook(entry, target, useIndex)
		if ownErr != nil {
			warnings = append(warnings, Warning{
				Code: "search.read-failed", Book: string(entry.ID), Message: ownErr.Error(),
			})
			continue
		}
		if !owns {
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

// ownsBook reports whether a book owns an ID, using a fresh index when allowed and the manifest
// otherwise.
func (service *Service) ownsBook(entry store.RegistryEntry, target model.ID, useIndex bool) (bool, error) {
	if useIndex {
		if status, err := service.Index.Status(entry.ID); err == nil && status.State == "fresh" {
			if postings, loadErr := service.Index.LoadPostings(entry.ID); loadErr == nil {
				for _, owned := range postings.Owned {
					if owned == target.String() {
						return true, nil
					}
				}
				return false, nil
			}
		}
	}
	book, err := service.Sources.LoadManifest(entry.ID)
	if err != nil {
		return false, err
	}
	return ownsID(book, target), nil
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
	files := render.LayoutFor(manifest)
	lineJoin := resultMode == "" || resultMode == "matches" || resultMode == "count"
	joinAnd := join == "and" && len(matchers) > 1
	for _, node := range info.Ordered {
		if fields.title && set.titleAllowed(node) {
			spans := matchLine(matchers, node.Node.Title)
			if len(spans) > 0 && (!lineJoin || !joinAnd || coversQueries(spans, len(matchers))) {
				result.Matches = append(result.Matches,
					newMatch(info, node, model.ZeroID, nil, "title", 1, node.Node.Title, spans, files))
			}
		}
		for _, id := range node.Node.Blocks {
			if fields.body && set.blockAllowed(info, node, id, nil) && manifest.BlockType(id) != "RepoLayout" {
				if body, exists := info.Book.Body(id); exists {
					appendLines(result, info, node, id, nil, "body", body, matchers, files, context, lineJoin, joinAnd)
				}
			}
			if fields.layout && set.blockAllowed(info, node, id, nil) {
				if spec, exists := info.Book.Layout(id); exists {
					appendLines(result, info, node, id, nil, "layout", string(spec), matchers, files, context, lineJoin, joinAnd)
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
				appendLines(result, info, owner, id, region, field, body, matchers, files, context, lineJoin, joinAnd)
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

func appendLines(result *Result, info *bookInfo, owner *nodeInfo, block model.ID, region *model.Region, field, text string, matchers []*Matcher, files map[model.ID]render.LinkTarget, context int, lineJoin, joinAnd bool) {
	lines := splitLines(text)
	for index, line := range lines {
		spans := matchLine(matchers, line)
		if len(spans) == 0 {
			continue
		}
		if lineJoin && joinAnd && !coversQueries(spans, len(matchers)) {
			continue
		}
		match := newMatch(info, owner, block, region, field, index+1, line, spans, files)
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

func newMatch(info *bookInfo, owner *nodeInfo, block model.ID, region *model.Region, field string, line int, text string, spans []Span, files map[model.ID]render.LinkTarget) Match {
	rendered := "README.md"
	if block != model.ZeroID {
		if target, exists := files[block]; exists {
			rendered = target.File
		} else if owner != nil {
			if target, exists := files[owner.Node.ID]; exists {
				rendered = target.File
			}
		}
	} else if owner != nil {
		if target, exists := files[owner.Node.ID]; exists {
			rendered = target.File
		}
	}
	match := Match{
		Kind: "block", Book: string(info.Book.ID), BookPath: info.Path,
		File:  path.Join(info.Path, rendered),
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
