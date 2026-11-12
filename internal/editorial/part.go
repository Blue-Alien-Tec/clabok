package editorial

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

type partGroupPayload struct {
	Slot     int    `json:"slot"`
	Text     string `json:"text"`
	Chapters int    `json:"chapters"`
}

type partPayload struct {
	DryRun   bool               `json:"dryRun"`
	Book     string             `json:"book"`
	Action   string             `json:"action"`
	Target   string             `json:"target,omitempty"`
	Part     string             `json:"part,omitempty"`
	From     string             `json:"from,omitempty"`
	Affected []string           `json:"affected,omitempty"`
	Changed  bool               `json:"changed"`
	Limit    int                `json:"limit,omitempty"`
	Parts    []partGroupPayload `json:"parts,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt; a list writes nothing either way.
func (payload partPayload) DryRunResult() bool { return payload.DryRun }

func (payload partPayload) Text() string {
	var builder strings.Builder
	change := "unchanged"
	if payload.Changed {
		change = "changed"
	}
	switch payload.Action {
	case "list":
		fmt.Fprintf(&builder, "%s parts (%d of %d used)\n", payload.Book,
			len(payload.Parts), payload.Limit)
		for _, group := range payload.Parts {
			fmt.Fprintf(&builder, "%d. %s (%d chapter(s))\n", group.Slot, group.Text, group.Chapters)
		}
	case "label":
		fmt.Fprintf(&builder, "%s part label %q -> %q: %s (%d node(s))\n",
			payload.Book, payload.From, payload.Part, change, len(payload.Affected))
	default:
		fmt.Fprintf(&builder, "%s part %s: %s", payload.Book, payload.Action, change)
		if payload.Target != "" {
			builder.WriteString(" " + payload.Target)
		}
		if payload.Part != "" {
			builder.WriteString(" = " + payload.Part)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

var partActions = []cli.Action{
	{Name: "set", Summary: "assign a part group to a chapter or a link entry",
		Reads: []string{"book", "target", "id", "part"}},
	{Name: "label", Summary: "rename a whole part group",
		Reads: []string{"book", "from", "part"}},
	{Name: "list", Summary: "list the part groups of a book",
		Reads: []string{"book"}},
}

// partFlags carries the group flags of one invocation.
type partFlags struct {
	Book   string
	Target string
	ID     string
	Part   string
	From   string
}

// partCommand groups the part authoring actions. The mutations live in structure, like status.
func partCommand() *cli.Command {
	var flags partFlags
	return &cli.Command{
		Name:     "part",
		Summary:  "assign, rename and list the part groups of a book",
		Usage:    "pecia part <set|label|list> --book <id> [--target node --id <id> --part <slot|text> | --from <text> --part <slot|text>]",
		Actions:  partActions,
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("part")
			set.StringVar(&flags.Book, "book", "", "book id")
			set.StringVar(&flags.Target, "target", "", "target kind: node")
			set.StringVar(&flags.ID, "id", "", "target id")
			set.StringVar(&flags.Part, "part", "", "part group: a slot 1 to 10 or the group text")
			set.StringVar(&flags.From, "from", "", "current group text to rename")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, partUsage()
			}
			bookID, bookErr := requireBook(flags.Book)
			if bookErr != nil {
				return nil, bookErr
			}
			switch args[0] {
			case "set":
				return runPartSet(env, bookID, flags)
			case "label":
				return runPartLabel(env, bookID, flags)
			case "list":
				return runPartList(env, bookID)
			default:
				return nil, partUsage()
			}
		},
	}
}

func runPartSet(env *cli.Env, book model.BookID, flags partFlags) (*cli.Result, error) {
	if strings.TrimSpace(flags.Target) != "node" {
		return nil, cli.Usage("usage.invalid-arguments", "part set needs --target node")
	}
	unit, idErr := requireID(flags.ID, "node")
	if idErr != nil {
		return nil, idErr
	}
	if strings.TrimSpace(flags.Part) == "" {
		return nil, cli.Usage("usage.missing-argument",
			"--part is required; pass a slot 1 to 10 or the group text")
	}
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	receipt, setErr := service.SetPart(book, unit, flags.Part, env.Globals.DryRun)
	if setErr != nil {
		return nil, cli.AsError(setErr)
	}
	return &cli.Result{Data: partPayload{
		DryRun: env.Globals.DryRun, Book: string(book), Action: "set",
		Target: receipt.Target.String(), Part: receipt.Part, Changed: receipt.Changed,
	}}, nil
}

func runPartLabel(env *cli.Env, book model.BookID, flags partFlags) (*cli.Result, error) {
	if strings.TrimSpace(flags.From) == "" || strings.TrimSpace(flags.Part) == "" {
		return nil, cli.Usage("usage.missing-argument",
			"part label needs --from <text> and --part <slot|text>")
	}
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	receipt, labelErr := service.SetPartLabel(book, flags.From, flags.Part, env.Globals.DryRun)
	if labelErr != nil {
		return nil, cli.AsError(labelErr)
	}
	affected := make([]string, 0, len(receipt.Order))
	for _, affectedID := range receipt.Order {
		affected = append(affected, affectedID.String())
	}
	return &cli.Result{Data: partPayload{
		DryRun: env.Globals.DryRun, Book: string(book), Action: "label",
		From: flags.From, Part: receipt.Part, Affected: affected, Changed: receipt.Changed,
	}}, nil
}

func runPartList(env *cli.Env, book model.BookID) (*cli.Result, error) {
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	summary, summaryErr := service.PartSummary(book)
	if summaryErr != nil {
		return nil, cli.AsError(summaryErr)
	}
	groups := make([]partGroupPayload, 0, len(summary.Parts))
	for _, view := range summary.Parts {
		groups = append(groups, partGroupPayload{
			Slot: view.Slot, Text: view.Text, Chapters: view.Chapters,
		})
	}
	return &cli.Result{Data: partPayload{
		DryRun: true, Book: string(book), Action: "list", Limit: summary.Limit, Parts: groups,
	}}, nil
}

func partUsage() *cli.Error {
	return cli.Usage("usage.invalid-arguments", "part takes one action: set, label or list").
		WithHint("pecia part <set|label|list> --book <id>")
}
