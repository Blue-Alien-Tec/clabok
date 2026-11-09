package search

import (
	"errors"
	"flag"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// maxQueries bounds one search request.
const maxQueries = 8

// stringList is a repeatable flag value.
type stringList []string

// String returns the comma-joined values for the flag help.
func (list *stringList) String() string { return strings.Join(*list, ",") }

// Set appends one value.
func (list *stringList) Set(value string) error {
	*list = append(*list, value)
	return nil
}

// Register adds the find command.
func Register(registry *cli.Registry) {
	var input inputOptions
	registry.Add(&cli.Command{
		Name:    "find",
		Summary: "search titles, bodies, layouts and regions",
		Usage:   "pecia find (--book <id> | --all) --query <text> [--query <text> ...] [--join and|or] [--mode literal|glob|regex] [--result matches|books|blocks|nodes|count]",
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("find")
			set.StringVar(&input.Book, "book", "", "book id")
			set.BoolVar(&input.All, "all", false, "search every registered book")
			set.Var(&input.Queries, "query", "text to find (repeatable)")
			set.Var(&input.QueryModes, "query-mode", "per-query mode (repeatable)")
			set.StringVar(&input.Join, "join", "or", "combine queries with and or or")
			set.StringVar(&input.Mode, "mode", "literal", "default match mode: literal, glob or regex")
			set.IntVar(&input.Fuzzy, "fuzzy", 0, "fuzzy edit distance 1 to 3 (literal)")
			set.StringVar(&input.Field, "field", "", "comma-separated fields (default body,title)")
			set.StringVar(&input.Kind, "kind", "", "comma-separated node kinds")
			set.StringVar(&input.Type, "type", "", "comma-separated block types")
			set.StringVar(&input.Status, "status", "", "comma-separated editorial states")
			set.StringVar(&input.Part, "part", "", "part name")
			set.StringVar(&input.Node, "node", "", "node id and subtree")
			set.StringVar(&input.Block, "block", "", "exactly one placed block")
			set.StringVar(&input.NodePath, "node-path", "", "glob over the node title path")
			set.StringVar(&input.BookPath, "book-path", "", "glob over the registry output path (--all)")
			set.StringVar(&input.Ref, "ref", "", "blocks that reference this id")
			set.BoolVar(&input.MissingBody, "missing-body", false, "blocks without a body")
			set.BoolVar(&input.Locked, "locked", false, "locked blocks only")
			set.BoolVar(&input.Unlocked, "unlocked", false, "unlocked blocks only")
			set.StringVar(&input.Result, "result", "matches", "result mode: matches, books, blocks, nodes or count")
			set.StringVar(&input.GroupBy, "group-by", "", "group by type, status, kind or part")
			set.StringVar(&input.CountBy, "count-by", "", "count per type, status, kind or part")
			set.IntVar(&input.Limit, "limit", 0, "maximum result items")
			set.IntVar(&input.Offset, "offset", 0, "result items to skip")
			set.IntVar(&input.Context, "context", 0, "context lines around a match")
			set.BoolVar(&input.IgnoreCase, "ignore-case", false, "match case-insensitively")
			set.BoolVar(&input.Word, "word", false, "match whole words only (literal)")
			set.BoolVar(&input.CountOnly, "count", false, "print counts only")
			set.BoolVar(&input.Strict, "strict", false, "fail instead of warning")
			set.BoolVar(&input.RequireMatch, "require-match", false, "exit 1 when the result is empty")
			set.BoolVar(&input.Snapshot, "snapshot", false, "take the book lock for a consistent read")
			set.StringVar(&input.IndexMode, "index", "auto", "index mode: auto, force or off")
			set.BoolVar(&input.Verify, "verify", false, "re-check indexed lines against the source")
			set.StringVar(&input.Format, "format", "json", "output format: json or ndjson")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "find takes flags only")
			}
			options, parseErr := parseOptions(env, input)
			if parseErr != nil {
				return nil, parseErr
			}
			life, layout, envErr := lifecycle.FromEnv(env)
			if envErr != nil {
				return nil, envErr
			}
			registry, registryErr := store.LoadRegistry(layout.Registry())
			if registryErr != nil {
				return nil, cli.Fail("search.registry-invalid", "%v", registryErr)
			}
			found, findErr := New(life, registry, layout.Root).Find(options)
			if findErr != nil {
				return nil, asCLI(findErr)
			}
			var result *cli.Result
			if options.Format == "ndjson" {
				data, encodeErr := found.NDJSON(options.Limit, options.Offset)
				if encodeErr != nil {
					return nil, cli.Fail("search.write-failed", "%v", encodeErr)
				}
				result = &cli.Result{Raw: data}
			} else {
				result = &cli.Result{
					Data: found.Payload(options.Limit, options.Offset),
					Text: found.Text(options.Limit, options.Offset),
				}
			}
			if options.RequireMatch && found.Count() == 0 {
				result.Exit = cli.ExitFailure
				result.Notice = cli.Fail("search.no-match", "no match for the given query")
			}
			return result, nil
		},
	})
}

// inputOptions is the raw flag state before validation.
type inputOptions struct {
	Book        string
	All         bool
	Queries     stringList
	Join        string
	Mode        string
	Fuzzy       int
	Field       string
	Kind        string
	Type        string
	Status      string
	Part        string
	Node        string
	Block       string
	NodePath    string
	BookPath    string
	Ref         string
	MissingBody bool
	Result      string
	GroupBy     string
	CountBy     string
	Format      string
	QueryModes  stringList
	Context     int
	Limit       int
	Offset      int
	// IgnoreCase, Word, Locked and Unlocked refine the match.
	IgnoreCase   bool
	Word         bool
	Locked       bool
	Unlocked     bool
	CountOnly    bool
	RequireMatch bool
	Snapshot     bool
	IndexMode    string
	Verify       bool
	Strict       bool
}

func parseOptions(env *cli.Env, input inputOptions) (Options, *cli.Error) {
	if input.Book == "" && !input.All {
		return Options{}, cli.Reject("search.scope-required", "one of --book or --all is required")
	}
	if input.Book != "" && input.All {
		return Options{}, cli.Reject("search.scope-conflict", "use either --book or --all")
	}
	if input.Book != "" && !model.BookID(input.Book).Valid() {
		return Options{}, cli.Usage("usage.invalid-value", "invalid book id %q", input.Book)
	}
	if input.Node != "" && input.Block != "" {
		return Options{}, cli.Reject("search.scope-conflict", "use either --node or --block")
	}
	if input.Locked && input.Unlocked {
		return Options{}, cli.Reject("search.scope-conflict", "use either --locked or --unlocked")
	}
	if input.BookPath != "" && !input.All {
		return Options{}, cli.Reject("search.scope-conflict", "--book-path needs --all")
	}
	if input.MissingBody && len(input.Queries) > 0 {
		return Options{}, cli.Usage("search.missing-body-query", "--missing-body does not take a query")
	}
	queries, queryErr := parseQueries(input)
	if queryErr != nil {
		return Options{}, queryErr
	}
	fields, fieldErr := parseFields(splitList(input.Field))
	if fieldErr != nil {
		return Options{}, toUsage(fieldErr)
	}
	filter, filterErr := parseFilter(input)
	if filterErr != nil {
		return Options{}, filterErr
	}
	if filter.Block != model.ZeroID {
		if input.Field != "" && fields.title {
			return Options{}, cli.Usage("search.result-conflict", "--field title does not apply to --block")
		}
		fields.title = false
	}
	resultMode, resultErr := parseResult(input.Result, input.CountOnly)
	if resultErr != nil {
		return Options{}, resultErr
	}
	if input.Limit < 0 {
		return Options{}, cli.Usage("search.invalid-limit", "--limit must not be negative")
	}
	if input.Offset < 0 {
		return Options{}, cli.Usage("search.invalid-offset", "--offset must not be negative")
	}
	if resultMode != "matches" && input.Context > 0 {
		return Options{}, cli.Usage("search.result-conflict", "--context applies only to --result matches")
	}
	if resultMode == "count" && (input.Limit > 0 || input.Offset > 0) {
		return Options{}, cli.Usage("search.result-conflict", "--result count does not page")
	}
	switch input.IndexMode {
	case "auto", "force", "off":
	default:
		return Options{}, cli.Usage("usage.invalid-value",
			"unknown index mode %q; use auto, force or off", input.IndexMode)
	}
	if input.IndexMode == "off" && input.Verify {
		return Options{}, cli.Usage("usage.invalid-value", "--verify needs the index")
	}
	if input.MissingBody && resultMode != "blocks" && resultMode != "count" {
		return Options{}, cli.Usage("search.missing-body-result",
			"--missing-body needs --result blocks or --result count")
	}
	groupBy := strings.TrimSpace(input.GroupBy)
	countBy := strings.TrimSpace(input.CountBy)
	if groupBy != "" && countBy != "" {
		return Options{}, cli.Usage("search.group-conflict", "use either --group-by or --count-by")
	}
	group := groupBy
	countOnlyGroup := false
	if group == "" && countBy != "" {
		group = countBy
		countOnlyGroup = true
	}
	if group != "" {
		if resultMode != "matches" {
			return Options{}, cli.Usage("search.group-conflict", "grouping needs the default --result matches")
		}
		switch group {
		case "type", "status", "kind", "part":
		default:
			return Options{}, cli.Usage("search.invalid-group",
				"unknown group %q; use type, status, kind or part", group)
		}
	}
	format := input.Format
	switch format {
	case "json":
	case "ndjson":
		if env.Globals.Text {
			return Options{}, cli.Usage("usage.invalid-value", "--text cannot be combined with --format ndjson")
		}
	default:
		return Options{}, cli.Usage("usage.invalid-value", "unknown format %q; use json or ndjson", format)
	}
	return Options{
		Book: model.BookID(input.Book), All: input.All, Queries: queries, Join: input.Join,
		Fields: fields.names(), FieldsExplicit: input.Field != "", IgnoreCase: input.IgnoreCase,
		Word: input.Word, Context: input.Context, Filter: filter, Result: resultMode,
		Limit: input.Limit, Offset: input.Offset, RequireMatch: input.RequireMatch,
		Snapshot: input.Snapshot, GroupBy: group, CountBy: countOnlyGroup,
		IndexMode: input.IndexMode, Verify: input.Verify,
		Strict: input.Strict, Format: format,
	}, nil
}

func parseQueries(input inputOptions) ([]Query, *cli.Error) {
	if len(input.Queries) == 0 {
		return nil, nil
	}
	if len(input.Queries) > maxQueries {
		return nil, cli.Usage("search.too-many-queries", "at most %d queries are allowed", maxQueries)
	}
	seen := make(map[string]struct{}, len(input.Queries))
	globalMode, known := ParseMode(input.Mode)
	if !known {
		return nil, cli.Usage("search.invalid-mode", "unknown match mode %q", input.Mode)
	}
	if input.Fuzzy < 0 || input.Fuzzy > 3 {
		return nil, cli.Usage("search.invalid-fuzzy", "--fuzzy accepts 1 to 3")
	}
	if len(input.QueryModes) != 0 && len(input.QueryModes) != len(input.Queries) {
		return nil, cli.Usage("search.query-mode-count",
			"--query-mode needs one value per --query")
	}
	queries := make([]Query, len(input.Queries))
	fuzzyActive := input.Fuzzy > 0
	for index, text := range input.Queries {
		key := strings.TrimSpace(text)
		if key == "" {
			return nil, cli.Usage("search.empty-query", "query %d is empty", index+1)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, cli.Usage("search.duplicate-query", "query %q is repeated", key)
		}
		seen[key] = struct{}{}
		mode := globalMode
		if len(input.QueryModes) != 0 {
			perMode, modeKnown := ParseQueryMode(input.QueryModes[index])
			if !modeKnown {
				return nil, cli.Usage("search.invalid-mode",
					"unknown query mode %q", input.QueryModes[index])
			}
			mode = perMode
		}
		distance := 0
		switch mode {
		case ModeFuzzy:
			distance = input.Fuzzy
			if distance == 0 {
				distance = 1
			}
			fuzzyActive = true
		case ModeLiteral:
			distance = input.Fuzzy
			fuzzyActive = fuzzyActive || distance > 0
		default:
			if input.Fuzzy > 0 {
				return nil, cli.Usage("search.fuzzy-mode", "--fuzzy needs literal matching")
			}
		}
		queries[index] = Query{Text: text, Mode: mode, Fuzzy: distance}
	}
	if input.Word && fuzzyActive {
		return nil, cli.Usage("search.fuzzy-word", "--word cannot be combined with --fuzzy")
	}
	return queries, nil
}

func parseFilter(input inputOptions) (Filter, *cli.Error) {
	filter := Filter{
		Kinds:       splitList(input.Kind),
		Types:       splitList(input.Type),
		Statuses:    splitList(input.Status),
		Part:        strings.TrimSpace(input.Part),
		NodePath:    strings.TrimSpace(input.NodePath),
		BookPath:    strings.TrimSpace(input.BookPath),
		MissingBody: input.MissingBody,
	}
	node, nodeErr := parseID(input.Node)
	if nodeErr != nil {
		return Filter{}, nodeErr
	}
	block, blockErr := parseID(input.Block)
	if blockErr != nil {
		return Filter{}, blockErr
	}
	ref, refErr := parseID(input.Ref)
	if refErr != nil {
		return Filter{}, refErr
	}
	filter.Node, filter.Block, filter.Ref = node, block, ref
	if input.Locked {
		locked := true
		filter.Locked = &locked
	}
	if input.Unlocked {
		locked := false
		filter.Locked = &locked
	}
	for _, name := range filter.Kinds {
		if !model.Kind(name).Valid() {
			return Filter{}, cli.Usage("search.invalid-kind",
				"unknown node kind %q; use title, chapter, subchapter or link", name)
		}
	}
	for _, name := range filter.Statuses {
		if name == "unset" {
			continue
		}
		if _, known := model.FindStatus(name); !known {
			return Filter{}, cli.Usage("search.invalid-status", "unknown status %q", name)
		}
	}
	return filter, nil
}

func parseResult(value string, countOnly bool) (string, *cli.Error) {
	result := strings.TrimSpace(value)
	if result == "" {
		result = "matches"
	}
	if countOnly {
		if result != "matches" && result != "count" {
			return "", cli.Usage("search.result-conflict", "--count conflicts with --result %s", result)
		}
		result = "count"
	}
	switch result {
	case "matches", "books", "blocks", "nodes", "count":
		return result, nil
	}
	return "", cli.Usage("usage.invalid-value",
		"unknown result %q; use matches, books, blocks, nodes or count", result)
}

func parseID(value string) (model.ID, *cli.Error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return model.ZeroID, nil
	}
	id, err := model.ParseID(trimmed)
	if err != nil || id.IsZero() {
		return model.ZeroID, cli.Usage("usage.invalid-value", "invalid id %q", trimmed)
	}
	return id, nil
}

func splitList(value string) []string {
	values := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

// toUsage maps a search error to the usage exit code.
func toUsage(err error) *cli.Error {
	var searchErr *Error
	if errors.As(err, &searchErr) {
		return cli.Usage(searchErr.Code, "%s", searchErr.Message)
	}
	return cli.AsError(err)
}

// asCLI maps an engine error to the documented exit code.
func asCLI(err error) *cli.Error {
	var searchErr *Error
	if !errors.As(err, &searchErr) {
		return cli.AsError(err)
	}
	switch searchErr.Code {
	case "search.scope-required", "search.scope-conflict", "search.unknown-book", "search.unknown-id",
		"index.missing", "index.stale", "index.corrupt", "index.open-book", "index.unsupported":
		return cli.Reject(searchErr.Code, "%s", searchErr.Message)
	case "search.strict-failed", "search.read-failed", "search.unsupported-schema",
		"index.read-failed", "index.write-failed":
		return cli.Fail(searchErr.Code, "%s", searchErr.Message)
	default:
		return cli.Usage(searchErr.Code, "%s", searchErr.Message)
	}
}
