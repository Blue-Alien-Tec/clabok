package structure

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

type validationDetails struct {
	Issues []model.Issue `json:"issues"`
}

func (details validationDetails) Text() string {
	var builder strings.Builder
	for _, issue := range details.Issues {
		builder.WriteString(issue.String() + "\n")
	}
	return builder.String()
}

type payload struct {
	DryRun  bool     `json:"dryRun"`
	Book    string   `json:"book"`
	Target  string   `json:"target,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Parent  string   `json:"parent,omitempty"`
	Index   *int     `json:"index,omitempty"`
	Order   []string `json:"order,omitempty"`
	Changed bool     `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload payload) DryRunResult() bool { return payload.DryRun }

func (payload payload) Text() string {
	prefix := ""
	if payload.DryRun {
		prefix = "dry run: "
	}
	if payload.Kind == "settings" {
		return prefix + payload.Book + ": settings changed"
	}
	return fmt.Sprintf("%s%s %s in %s (%s)",
		prefix, payload.Kind, payload.Target, payload.Parent, strings.Join(payload.Order, ","))
}

func payloadOf(receipt Receipt, dryRun bool) payload {
	result := payload{
		DryRun:  dryRun,
		Book:    string(receipt.Book),
		Kind:    receipt.Kind,
		Changed: receipt.Changed,
	}
	if !receipt.Target.IsZero() {
		result.Target = receipt.Target.String()
	}
	if !receipt.Parent.IsZero() {
		result.Parent = receipt.Parent.String()
	}
	if receipt.Index >= 0 {
		index := receipt.Index
		result.Index = &index
	}
	if len(receipt.Order) > 0 {
		result.Order = make([]string, 0, len(receipt.Order))
		for _, id := range receipt.Order {
			result.Order = append(result.Order, id.String())
		}
	}
	return result
}

// Register adds the structure and book settings commands.
func Register(registry *cli.Registry) {
	registry.Add(addCommand())
	registry.Add(renameCommand())
	registry.Add(moveCommand())
	registry.Add(removeCommand())
	registry.Add(blockTypeCommand())
	registry.Add(settingsCommand())
}

func addCommand() *cli.Command {
	var book, title, blockType, parent, before, after string
	var index int
	return &cli.Command{
		Name:     "add",
		Summary:  "add a chapter, subchapter or block",
		Usage:    "pecia add --book <id> --title <title> [--parent <node>] [--index N | --before <id> | --after <id>]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("add")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&title, "title", "", "title of the new unit")
			set.StringVar(&blockType, "type", "", "optional block type")
			set.StringVar(&parent, "parent", "", "destination node")
			set.StringVar(&before, "before", "", "place before this sibling")
			set.StringVar(&after, "after", "", "place after this sibling")
			set.IntVar(&index, "index", -1, "position in the destination")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "add takes flags only")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			placement, placementErr := placementFrom(parent, index, before, after, true)
			if placementErr != nil {
				return nil, placementErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			receipt, addErr := service.AddTyped(bookID, title, blockType, placement, env.Globals.DryRun)
			if addErr != nil {
				return nil, MapError(addErr)
			}
			return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
		},
	}
}

func renameCommand() *cli.Command {
	var book, title string
	return &cli.Command{
		Name:     "rename",
		Summary:  "rename a structural node",
		Usage:    "pecia rename --book <id> <target> --title <title>",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("rename")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&title, "title", "", "new title")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments", "rename takes exactly one unit id").
					WithHint("pecia rename --book <id> <target> --title <title>")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			target, targetErr := requireID(args[0], "target")
			if targetErr != nil {
				return nil, targetErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			receipt, renameErr := service.Rename(bookID, target, title, env.Globals.DryRun)
			if renameErr != nil {
				return nil, MapError(renameErr)
			}
			return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
		},
	}
}

func moveCommand() *cli.Command {
	var book, childOf, before, after string
	var index int
	return &cli.Command{
		Name:     "move",
		Summary:  "move a node or a block",
		Usage:    "pecia move --book <id> <target> --child-of <node> [--index N] | --before <id> | --after <id>",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("move")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&childOf, "child-of", "", "destination node")
			set.StringVar(&before, "before", "", "place before this sibling")
			set.StringVar(&after, "after", "", "place after this sibling")
			set.IntVar(&index, "index", -1, "position in the destination")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments", "move takes exactly one unit id").
					WithHint("pecia move --book <id> <target> --child-of <node>")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			target, targetErr := requireID(args[0], "target")
			if targetErr != nil {
				return nil, targetErr
			}
			placement, placementErr := placementFrom(childOf, index, before, after, false)
			if placementErr != nil {
				return nil, placementErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			receipt, moveErr := service.Move(bookID, target, placement, env.Globals.DryRun)
			if moveErr != nil {
				return nil, MapError(moveErr)
			}
			return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
		},
	}
}

func removeCommand() *cli.Command {
	var book string
	var recursive bool
	return &cli.Command{
		Name:     "remove",
		Summary:  "remove a node or a block",
		Usage:    "pecia remove --book <id> <target> [--recursive]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("remove")
			set.StringVar(&book, "book", "", "book id")
			set.BoolVar(&recursive, "recursive", false, "remove a non-empty node with its subtree")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments", "remove takes exactly one unit id").
					WithHint("pecia remove --book <id> <target> [--recursive]")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			target, targetErr := requireID(args[0], "target")
			if targetErr != nil {
				return nil, targetErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			receipt, removeErr := service.Remove(bookID, target, recursive, env.Globals.DryRun)
			if removeErr != nil {
				return nil, MapError(removeErr)
			}
			return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
		},
	}
}

func blockTypeCommand() *cli.Command {
	var book string
	var clear bool
	return &cli.Command{
		Name:     "block type",
		Summary:  "set or clear a block type",
		Usage:    "pecia block type --book <id> <block> <type>",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("block type")
			set.StringVar(&book, "book", "", "book id")
			set.BoolVar(&clear, "clear", false, "remove the type")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if clear && len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments", "block type --clear takes the block id only")
			}
			if !clear && len(args) != 2 {
				return nil, cli.Usage("usage.invalid-arguments", "block type takes a block id and a type").
					WithHint("pecia block type --book <id> <block> <type>")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			block, blockErr := requireID(args[0], "block")
			if blockErr != nil {
				return nil, blockErr
			}
			value := ""
			if !clear {
				value = args[1]
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			receipt, typeErr := service.SetBlockType(bookID, block, value, env.Globals.DryRun)
			if typeErr != nil {
				return nil, MapError(typeErr)
			}
			return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
		},
	}
}

func settingsCommand() *cli.Command {
	return &cli.Command{
		Name:     "book set",
		Summary:  "change a book setting",
		Usage:    "pecia book set <book-id> <key> <value>",
		Mutating: true,
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 3 {
				return nil, cli.Usage("usage.invalid-arguments", "book set takes a book id, a key and a value").
					WithHint("pecia book set <book-id> numbering|toc|index-depth|output <value>")
			}
			bookID, bookErr := requireBook(args[0])
			if bookErr != nil {
				return nil, bookErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			receipt, setErr := service.SetSetting(bookID, args[1], args[2], env.Globals.DryRun)
			if setErr != nil {
				return nil, MapError(setErr)
			}
			return &cli.Result{Data: payloadOf(receipt, env.Globals.DryRun)}, nil
		},
	}
}

func newService(env *cli.Env) (*Service, *cli.Error) {
	life, _, err := lifecycle.FromEnv(env)
	if err != nil {
		return nil, err
	}
	return New(life, model.NewRandomAllocator()), nil
}

func requireBook(raw string) (model.BookID, *cli.Error) {
	if raw == "" {
		return "", cli.Usage("usage.missing-argument", "--book is required").
			WithHint("run `pecia help <command>` for the usage")
	}
	id := model.BookID(raw)
	if !id.Valid() {
		return "", cli.Usage("usage.invalid-value", "invalid book id %q", raw)
	}
	return id, nil
}

func requireID(raw, name string) (model.ID, *cli.Error) {
	id, err := model.ParseID(raw)
	if err != nil || id.IsZero() {
		return model.ZeroID, cli.Usage("usage.invalid-value", "invalid %s id %q", name, raw)
	}
	return id, nil
}

func placementFrom(parent string, index int, before, after string, allowEmpty bool) (Placement, *cli.Error) {
	forms := 0
	placement := Placement{Index: -1}
	if before != "" && after != "" {
		return Placement{}, cli.Usage("usage.invalid-arguments", "use --before or --after, not both")
	}
	if before != "" {
		id, err := requireID(before, "before")
		if err != nil {
			return Placement{}, err
		}
		placement.Before = id
		forms++
	}
	if after != "" {
		id, err := requireID(after, "after")
		if err != nil {
			return Placement{}, err
		}
		placement.After = id
		forms++
	}
	if parent != "" || index >= 0 {
		if parent == "root" {
			placement.Parent = 0
		} else if parent != "" {
			id, err := requireID(parent, "parent")
			if err != nil {
				return Placement{}, err
			}
			placement.Parent = id
		}
		placement.Index = index
		forms++
	}
	if forms > 1 {
		return Placement{}, cli.Usage("usage.invalid-arguments",
			"use one placement form: --before, --after or --child-of/--parent with --index")
	}
	if forms == 0 {
		if !allowEmpty {
			return Placement{}, cli.Usage("usage.missing-argument", "a destination is required").
				WithHint("pass --child-of <node>, --before <id> or --after <id>")
		}
		return Placement{Index: -1}, nil
	}
	return placement, nil
}

// MapError converts a service error for the CLI boundary. Validation failures are rejects.
func MapError(err error) *cli.Error {
	var validation *model.ValidationError
	if errors.As(err, &validation) {
		return cli.Reject("book.invalid", "the book does not validate; nothing was written").
			WithHint("this indicates a command bug; use rollback to return to the previous state").
			WithDetails(validationDetails{Issues: validation.Issues})
	}
	return cli.AsError(err)
}
