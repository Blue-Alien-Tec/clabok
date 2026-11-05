package body

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/jsonstrict"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

type getPayload struct {
	Block      string `json:"block"`
	Body       string `json:"text"`
	From       int    `json:"from"`
	To         int    `json:"to"`
	TotalLines int    `json:"totalLines"`
	Truncated  bool   `json:"truncated,omitempty"`
}

// Text returns the body for --text output.
func (payload getPayload) Text() string {
	return payload.Body
}

type writePayload struct {
	DryRun  bool   `json:"dryRun"`
	Block   string `json:"block"`
	Created bool   `json:"created,omitempty"`
	Changed bool   `json:"changed"`
	Lines   int    `json:"lines"`
	Bytes   int    `json:"bytes"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload writePayload) DryRunResult() bool { return payload.DryRun }

func (payload writePayload) Text() string {
	prefix := ""
	if payload.DryRun {
		prefix = "dry run: "
	}
	created := ""
	if payload.Created {
		created = " (new block)"
	}
	return fmt.Sprintf("%sblock %s%s: %d line(s), %d byte(s)", prefix, payload.Block, created, payload.Lines, payload.Bytes)
}

type replacePayload struct {
	DryRun       bool   `json:"dryRun"`
	Block        string `json:"block"`
	Replacements int    `json:"replacements"`
	Lines        int    `json:"lines"`
	Bytes        int    `json:"bytes"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload replacePayload) DryRunResult() bool { return payload.DryRun }

func (payload replacePayload) Text() string {
	prefix := ""
	if payload.DryRun {
		prefix = "dry run: "
	}
	return fmt.Sprintf("%sblock %s: %d replacement(s)", prefix, payload.Block, payload.Replacements)
}

type rowPayload struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Depth  int    `json:"depth"`
	Lines  int    `json:"lines"`
	Bytes  int    `json:"bytes"`
	Type   string `json:"type,omitempty"`
	Locked bool   `json:"locked,omitempty"`
}

type listPayload struct {
	Book   string       `json:"book"`
	Blocks []rowPayload `json:"blocks"`
}

func (payload listPayload) Text() string {
	var builder strings.Builder
	for _, row := range payload.Blocks {
		fmt.Fprintf(&builder, "%*s%s  %s  %d line(s), %d byte(s)\n",
			row.Depth, "", row.ID, row.Title, row.Lines, row.Bytes)
	}
	if len(payload.Blocks) == 0 {
		return payload.Book + ": no blocks"
	}
	return builder.String()
}

// Register adds the body command group.
func Register(registry *cli.Registry) {
	registry.Add(getCommand())
	registry.Add(setCommand("body set", "replace a body", modeSet))
	registry.Add(setCommand("body append", "append to a body", modeAppend))
	registry.Add(setCommand("body prepend", "prepend to a body", modePrepend))
	registry.Add(replaceCommand())
	registry.Add(listCommand())
}

func getCommand() *cli.Command {
	var book, lines string
	return &cli.Command{
		Name:    "body get",
		Summary: "read a body, optionally a line range",
		Usage:   "pecia body get --book <id> <block> [--lines A-B]",
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("body get")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&lines, "lines", "", "line range A-B")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments", "body get takes exactly one block id")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			block, blockErr := requireBlock(args[0], false)
			if blockErr != nil {
				return nil, blockErr
			}
			from, to, rangeErr := ParseLines(lines)
			if rangeErr != nil {
				return nil, cli.AsError(rangeErr)
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			slice, getErr := service.Get(bookID, block, from, to)
			if getErr != nil {
				return nil, cli.AsError(getErr)
			}
			return &cli.Result{Data: getPayload{
				Block: slice.Block.String(), Body: slice.Text,
				From: slice.From, To: slice.To, TotalLines: slice.TotalLines, Truncated: slice.Truncated,
			}}, nil
		},
	}
}

func setCommand(name, summary string, op mode) *cli.Command {
	var book, file, bodyText, parent string
	return &cli.Command{
		Name:     name,
		Summary:  summary,
		Usage:    "pecia " + name + " --book <id> <block> [--parent <node>] (--file <path|->|--body <value>)",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet(name)
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&file, "file", "", "body file, or - for stdin")
			set.StringVar(&bodyText, "body", "", "body text")
			set.StringVar(&parent, "parent", "", "parent node for block 0")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments", "%s takes exactly one block id", name)
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			block, blockErr := requireBlock(args[0], true)
			if blockErr != nil {
				return nil, blockErr
			}
			parentID := model.ZeroID
			if parent != "" {
				id, parentErr := requireBlock(parent, false)
				if parentErr != nil {
					return nil, parentErr
				}
				parentID = id
			}
			value, textErr := readBodyText(file, bodyText)
			if textErr != nil {
				return nil, textErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			var result WriteResult
			var writeErr error
			switch op {
			case modeAppend:
				result, writeErr = service.Append(bookID, block, parentID, value, env.Globals.DryRun)
			case modePrepend:
				result, writeErr = service.Prepend(bookID, block, parentID, value, env.Globals.DryRun)
			default:
				result, writeErr = service.Set(bookID, block, parentID, value, env.Globals.DryRun)
			}
			if writeErr != nil {
				return nil, MapError(writeErr)
			}
			return &cli.Result{Data: writePayload{
				DryRun: env.Globals.DryRun, Block: result.Block.String(), Created: result.Created,
				Changed: result.Changed, Lines: result.Lines, Bytes: result.Bytes,
			}}, nil
		},
	}
}

func replaceCommand() *cli.Command {
	var book, find, replace, pairsFile string
	var expect int
	var regex, ignoreCase, word bool
	return &cli.Command{
		Name:     "body replace",
		Summary:  "apply a replacement batch to a body",
		Usage:    "pecia body replace --book <id> <block> (--find F --replace R | --pairs <file>) [--expect N]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("body replace")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&find, "find", "", "text to find")
			set.StringVar(&replace, "replace", "", "replacement")
			set.StringVar(&pairsFile, "pairs", "", "JSON file with a pairs list")
			set.IntVar(&expect, "expect", -1, "expected number of replacements")
			set.BoolVar(&regex, "regex", false, "treat the find patterns as regular expressions")
			set.BoolVar(&ignoreCase, "ignore-case", false, "match case-insensitively")
			set.BoolVar(&word, "word", false, "match whole words only")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments", "body replace takes exactly one block id")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			block, blockErr := requireBlock(args[0], false)
			if blockErr != nil {
				return nil, blockErr
			}
			pairs, pairErr := readPairs(find, replace, pairsFile, regex)
			if pairErr != nil {
				return nil, pairErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			result, replaceErr := service.Replace(bookID, block, pairs, ReplaceOptions{
				Expect: expect, IgnoreCase: ignoreCase, Word: word,
			}, env.Globals.DryRun)
			if replaceErr != nil {
				return nil, MapError(replaceErr)
			}
			return &cli.Result{Data: replacePayload{
				DryRun: env.Globals.DryRun, Block: result.Block.String(),
				Replacements: result.Replacements, Lines: result.Lines, Bytes: result.Bytes,
			}}, nil
		},
	}
}

func listCommand() *cli.Command {
	var book string
	return &cli.Command{
		Name:    "body list",
		Summary: "list the blocks of a book",
		Usage:   "pecia body list --book <id>",
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("body list")
			set.StringVar(&book, "book", "", "book id")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "body list takes no arguments")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			rows, listErr := service.List(bookID)
			if listErr != nil {
				return nil, cli.AsError(listErr)
			}
			payload := listPayload{Book: string(bookID), Blocks: make([]rowPayload, 0, len(rows))}
			for _, row := range rows {
				payload.Blocks = append(payload.Blocks, rowPayload{
					ID: row.ID.String(), Title: row.Title, Depth: row.Depth,
					Lines: row.Lines, Bytes: row.Bytes, Type: row.Type, Locked: row.Locked,
				})
			}
			return &cli.Result{Data: payload}, nil
		},
	}
}

func newService(env *cli.Env) (*Service, *cli.Error) {
	life, _, err := lifecycle.FromEnv(env)
	if err != nil {
		return nil, err
	}
	return New(life), nil
}

func requireBook(raw string) (model.BookID, *cli.Error) {
	if raw == "" {
		return "", cli.Usage("usage.missing-argument", "--book is required").
			WithHint("run `pecia help body set` for the usage")
	}
	id := model.BookID(raw)
	if !id.Valid() {
		return "", cli.Usage("usage.invalid-value", "invalid book id %q", raw)
	}
	return id, nil
}

func requireBlock(raw string, allowZero bool) (model.ID, *cli.Error) {
	id, err := model.ParseID(raw)
	if err != nil {
		return model.ZeroID, cli.Usage("usage.invalid-value", "invalid block id %q", raw)
	}
	if id.IsZero() && !allowZero {
		return model.ZeroID, cli.Usage("usage.invalid-value", "block 0 is only valid for a write")
	}
	return id, nil
}

func readBodyText(file, text string) (string, *cli.Error) {
	if file != "" && text != "" {
		return "", cli.Usage("usage.invalid-arguments", "use --file or --body, not both")
	}
	if file == "" && text == "" {
		return "", cli.Usage("usage.missing-argument", "a body is required").
			WithHint("pass --file <path|-> or --body <value>")
	}
	if text != "" {
		return text, nil
	}
	if file == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", cli.Fail("body.read-failed", "%v", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", cli.Fail("body.read-failed", "%v", err)
	}
	return string(data), nil
}

type pairFile struct {
	Pairs []struct {
		Find    string `json:"find"`
		Replace string `json:"replace"`
		Regex   bool   `json:"regex,omitempty"`
	} `json:"pairs"`
}

func readPairs(find, replace, file string, regex bool) ([]Pair, *cli.Error) {
	if file != "" {
		if find != "" || replace != "" {
			return nil, cli.Usage("usage.invalid-arguments", "use --pairs or --find/--replace, not both")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, cli.Fail("body.read-failed", "%v", err)
		}
		var parsed pairFile
		if err := jsonstrict.Decode(data, &parsed, jsonstrict.Options{DisallowUnknownFields: true}); err != nil {
			return nil, cli.Fail("body.invalid-pairs", "%v", err)
		}
		if len(parsed.Pairs) == 0 {
			return nil, cli.Usage("usage.invalid-arguments", "the pairs file has no pairs")
		}
		pairs := make([]Pair, 0, len(parsed.Pairs))
		for _, item := range parsed.Pairs {
			pairs = append(pairs, Pair{Find: item.Find, Replace: item.Replace, Regex: item.Regex || regex})
		}
		return pairs, nil
	}
	if find == "" || replace == "" {
		return nil, cli.Usage("usage.missing-argument", "pass --find and --replace, or --pairs").
			WithHint("pecia body replace --book <id> <block> --find <text> --replace <text>")
	}
	return []Pair{{Find: find, Replace: replace, Regex: regex}}, nil
}

// MapError converts a body error for the CLI boundary. Prose and expectation gates are rejects.
func MapError(err error) *cli.Error {
	var bodyErr *Error
	if errors.As(err, &bodyErr) {
		switch bodyErr.Code {
		case "body.forbidden-heading":
			return cli.Reject(bodyErr.Code, "%s", bodyErr.Message).
				WithHint("use #### or deeper for text; #, ## and ### are structure")
		case "body.expectation-failed":
			return cli.Reject(bodyErr.Code, "%s", bodyErr.Message).
				WithHint("count the matches with `pecia body get` and pass the exact number")
		}
	}
	return cli.AsError(err)
}
