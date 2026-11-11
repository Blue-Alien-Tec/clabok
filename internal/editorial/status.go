package editorial

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

type statusPayload struct {
	DryRun  bool   `json:"dryRun"`
	Book    string `json:"book"`
	Action  string `json:"action"`
	Target  string `json:"target,omitempty"`
	ID      string `json:"id,omitempty"`
	State   string `json:"state,omitempty"`
	Enabled bool   `json:"enabled"`
	Changed bool   `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result statusPayload) DryRunResult() bool { return result.DryRun }

func (result statusPayload) Text() string {
	change := "unchanged"
	if result.Changed {
		change = "changed"
	}
	line := fmt.Sprintf("%s status %s: %s", result.Book, result.Action, change)
	if result.ID != "" {
		line += " " + result.ID
		if result.State != "" {
			line += "=" + result.State
		}
	}
	return line + "\n"
}

type statusLevelPayload struct {
	DryRun   bool            `json:"dryRun"`
	Book     string          `json:"book"`
	Action   string          `json:"action"`
	Level    string          `json:"level"`
	Label    string          `json:"label,omitempty"`
	Color    string          `json:"color,omitempty"`
	Block    string          `json:"block,omitempty"`
	Position string          `json:"position,omitempty"`
	Changed  bool            `json:"changed"`
	Grammar  *grammar.Report `json:"grammar,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload statusLevelPayload) DryRunResult() bool { return payload.DryRun }

func (payload statusLevelPayload) Text() string {
	change := "unchanged"
	if payload.Changed {
		change = "changed"
	}
	line := fmt.Sprintf("%s status level %s %s: %s", payload.Book, payload.Action,
		payload.Level, change)
	if payload.Block != "" {
		line += " block " + payload.Block
	}
	if payload.Grammar != nil && payload.Grammar.Degraded != "" {
		line += "\ngrammar: " + payload.Grammar.Degraded
	}
	return line + "\n"
}

type statusState struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Block       string `json:"block,omitempty"`
}

type statusAssignment struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	State  string `json:"state"`
}

type statusView struct {
	DryRun      bool               `json:"dryRun"`
	Book        string             `json:"book"`
	Enabled     bool               `json:"enabled"`
	Fallback    bool               `json:"fallback"`
	States      []statusState      `json:"states"`
	Assignments []statusAssignment `json:"assignments"`
}

// DryRunResult marks the payload as a dry-run receipt; a view writes nothing either way.
func (result statusView) DryRunResult() bool { return true }

// statusLevelActions are the nested `status level …` actions.
var statusLevelActions = []cli.Action{
	{Name: "level add", Summary: "add a custom status level",
		Reads: []string{"book", "level-id", "label", "color", "description", "description-file",
			"description-block", "before", "after", "first", "last"}},
	{Name: "level set", Summary: "change a status level",
		Reads: []string{"book", "level-id", "label", "color", "description", "description-file",
			"description-block", "clear-description"}},
	{Name: "level move", Summary: "reorder a status level",
		Reads: []string{"book", "level-id", "before", "after", "first", "last"}},
	{Name: "level remove", Summary: "remove a status level",
		Reads: []string{"book", "level-id"}},
}

// statusActions are the top-level `status …` actions plus the nested level actions.
var statusActions = append([]cli.Action{
	{Name: "enable", Summary: "render the status column and the legend",
		Reads: []string{"book"}},
	{Name: "disable", Summary: "stop rendering the status output and keep the assignments",
		Reads: []string{"book"}},
	{Name: "set", Summary: "assign a status level to a node or block",
		Reads: []string{"book", "target", "id", "state"}},
	{Name: "clear", Summary: "remove a status assignment",
		Reads: []string{"book", "target", "id"}},
	{Name: "show", Summary: "show the levels, the legend and the assignments",
		Reads: []string{"book"}},
	{Name: "list", Summary: "list the levels, the legend and the assignments",
		Reads: []string{"book"}},
}, statusLevelActions...)

func statusCommand() *cli.Command {
	var book, target, id, state, levelID, label, color, description, descriptionFile string
	var descriptionBlock, before, after string
	var clearDescription, first, last bool
	return &cli.Command{
		Name:     "status",
		Summary:  "switch the status system, configure levels, assign levels and show them",
		Usage:    "pecia status <enable|disable|set|clear|show|list> --book <id> | status level <add|set|move|remove> --book <id>",
		Actions:  statusActions,
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("status")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&target, "target", "", "node or block")
			set.StringVar(&id, "id", "", "target id")
			set.StringVar(&state, "state", "", "level id of an assignment")
			set.StringVar(&levelID, "level-id", "", "configured level id")
			set.StringVar(&label, "label", "", "level label")
			set.StringVar(&color, "color", "", "level badge color")
			set.StringVar(&description, "description", "", "legend text")
			set.StringVar(&descriptionFile, "description-file", "", "legend text file, or -")
			set.StringVar(&descriptionBlock, "description-block", "", "existing status block")
			set.BoolVar(&clearDescription, "clear-description", false, "remove the legend text")
			set.StringVar(&before, "before", "", "insert before this level")
			set.StringVar(&after, "after", "", "insert after this level")
			set.BoolVar(&first, "first", false, "insert first")
			set.BoolVar(&last, "last", false, "insert last")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) == 0 {
				return nil, statusUsage()
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			if args[0] == "level" {
				if len(args) != 2 {
					return nil, statusUsage()
				}
				return runStatusLevel(env, bookID, args[1], statusLevelFlags{
					LevelID: levelID, Label: label, Color: color, Description: description,
					DescriptionFile: descriptionFile, DescriptionBlock: descriptionBlock,
					ClearDescription: clearDescription, Before: before, After: after,
					First: first, Last: last,
				})
			}
			if len(args) != 1 {
				return nil, statusUsage()
			}
			action := args[0]
			switch action {
			case "enable", "disable":
				if target != "" || id != "" || state != "" {
					return nil, cli.Usage("usage.invalid-arguments", "%s takes --book only", action)
				}
				service, serviceErr := newService(env)
				if serviceErr != nil {
					return nil, serviceErr
				}
				receipt, err := service.SetStatusEnabled(bookID, action == "enable", env.Globals.DryRun)
				if err != nil {
					return nil, cli.AsError(err)
				}
				enabled := action == "enable"
				return &cli.Result{Data: statusPayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					Enabled: enabled, Changed: receipt.Changed,
				}}, nil
			case "set", "clear":
				if target != "node" && target != "block" {
					return nil, cli.Usage("usage.invalid-arguments",
						"--target must be node or block")
				}
				targetID, idErr := requireID(id, "target")
				if idErr != nil {
					return nil, idErr
				}
				service, serviceErr := newService(env)
				if serviceErr != nil {
					return nil, serviceErr
				}
				if action == "set" {
					if state == "" {
						return nil, cli.Usage("usage.missing-argument", "--state is required")
					}
					summary, summaryErr := service.StatusSummary(bookID)
					if summaryErr != nil {
						return nil, cli.AsError(summaryErr)
					}
					known := false
					for _, level := range summary.Levels {
						if level.ID == state {
							known = true
						}
					}
					if !known {
						return nil, cli.Usage("usage.invalid-value", "unknown status %q", state).
							WithHint("run `pecia status list --book " + string(bookID) +
								"` for the levels")
					}
				} else if state != "" {
					return nil, cli.Usage("usage.invalid-arguments", "clear takes no --state")
				}
				receipt, err := service.SetStatus(bookID, targetID, state, env.Globals.DryRun)
				if err != nil {
					return nil, cli.AsError(err)
				}
				return &cli.Result{Data: statusPayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					Target: target, ID: targetID.String(), State: state, Changed: receipt.Changed,
				}}, nil
			case "show", "list":
				if target != "" || id != "" || state != "" {
					return nil, cli.Usage("usage.invalid-arguments", "%s takes --book only", action)
				}
				return statusList(env, bookID)
			default:
				return nil, statusUsage()
			}
		},
	}
}

func statusUsage() *cli.Error {
	return cli.Usage("usage.invalid-arguments",
		"status takes %s", cli.ActionList(statusActions)).
		WithHint("pecia status level add --book <id> --level-id <id> --label <label>")
}

// statusLevelFlags carries the level-operation flags.
type statusLevelFlags struct {
	LevelID, Label, Color, Description, DescriptionFile, DescriptionBlock string
	ClearDescription, First, Last                                         bool
	Before, After                                                         string
}

// runStatusLevel dispatches one level operation.
func runStatusLevel(env *cli.Env, bookID model.BookID, action string,
	flags statusLevelFlags) (*cli.Result, error) {
	if flags.LevelID == "" {
		return nil, cli.Usage("usage.missing-argument", "--level-id is required")
	}
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	payload := statusLevelPayload{
		DryRun: env.Globals.DryRun, Book: string(bookID), Action: action, Level: flags.LevelID,
	}
	switch action {
	case "add":
		if flags.Description != "" || flags.DescriptionFile != "" || flags.DescriptionBlock != "" ||
			flags.ClearDescription {
			return nil, cli.Usage("usage.invalid-arguments",
				"add takes the level and its position; set the description with `level set`")
		}
		if strings.TrimSpace(flags.Label) == "" {
			return nil, cli.Usage("usage.missing-argument", "--label is required")
		}
		position, anchor, positionErr := levelPosition(flags)
		if positionErr != nil {
			return nil, positionErr
		}
		receipt, err := service.AddStatusLevel(bookID, model.StatusLevel{
			ID: flags.LevelID, Label: flags.Label, Color: flags.Color,
		}, position, anchor, env.Globals.DryRun)
		if err != nil {
			return nil, cli.AsError(err)
		}
		payload.Label, payload.Color = flags.Label, flags.Color
		payload.Position = position
		payload.Changed = receipt.Changed
		return &cli.Result{Data: payload}, nil
	case "set":
		update := structure.StatusLevelUpdate{ClearDescription: flags.ClearDescription}
		if flags.Label != "" {
			update.Label = &flags.Label
		}
		if flags.Color != "" {
			update.Color = &flags.Color
		}
		sources := 0
		if flags.Description != "" || flags.DescriptionFile != "" {
			sources++
		}
		if flags.DescriptionBlock != "" {
			sources++
		}
		if flags.ClearDescription {
			sources++
		}
		if sources > 1 {
			return nil, cli.Usage("usage.invalid-arguments",
				"use one of --description, --description-file, --description-block or "+
					"--clear-description")
		}
		if flags.Description != "" || flags.DescriptionFile != "" {
			text, textErr := readText(flags.Description, flags.DescriptionFile)
			if textErr != nil {
				return nil, textErr
			}
			report, gateErr := evaluateProse(env, bookID, text)
			if gateErr != nil {
				return nil, gateErr
			}
			if report.Blocked() {
				return nil, proseRejection(report)
			}
			update.Description = &text
			payload.Grammar = grammarPtr(report)
		}
		if flags.DescriptionBlock != "" {
			blockID, idErr := requireID(flags.DescriptionBlock, "description block")
			if idErr != nil {
				return nil, idErr
			}
			update.DescriptionBlock = blockID
		}
		receipt, err := service.UpdateStatusLevel(bookID, flags.LevelID, update,
			env.Globals.DryRun)
		if err != nil {
			return nil, cli.AsError(err)
		}
		payload.Label, payload.Color = flags.Label, flags.Color
		payload.Changed = receipt.Changed
		if !receipt.Block.IsZero() {
			payload.Block = receipt.Block.String()
		}
		return &cli.Result{Data: payload}, nil
	case "move":
		position, anchor, positionErr := levelPosition(flags)
		if positionErr != nil {
			return nil, positionErr
		}
		if flags.Before == "" && flags.After == "" && !flags.First && !flags.Last {
			return nil, cli.Usage("usage.missing-argument",
				"a move needs --before, --after, --first or --last")
		}
		receipt, err := service.MoveStatusLevel(bookID, flags.LevelID, position, anchor,
			env.Globals.DryRun)
		if err != nil {
			return nil, cli.AsError(err)
		}
		payload.Position = position
		payload.Changed = receipt.Changed
		return &cli.Result{Data: payload}, nil
	case "remove":
		receipt, err := service.RemoveStatusLevel(bookID, flags.LevelID, env.Globals.DryRun)
		if err != nil {
			return nil, cli.AsError(err)
		}
		payload.Changed = receipt.Changed
		return &cli.Result{Data: payload}, nil
	}
	return nil, cli.Usage("usage.invalid-arguments",
		"status level takes add, set, move or remove")
}

// levelPosition resolves the position flags into a service position and anchor.
func levelPosition(flags statusLevelFlags) (string, string, *cli.Error) {
	selected := 0
	position, anchor := "", ""
	if flags.Before != "" {
		selected++
		position, anchor = "before", flags.Before
	}
	if flags.After != "" {
		selected++
		position, anchor = "after", flags.After
	}
	if flags.First {
		selected++
		position = "first"
	}
	if flags.Last {
		selected++
		position = "last"
	}
	if selected > 1 {
		return "", "", cli.Usage("usage.invalid-arguments",
			"use one of --before, --after, --first or --last")
	}
	return position, anchor, nil
}

// statusList prints the effective levels with their resolved legend text.
func statusList(env *cli.Env, bookID model.BookID) (*cli.Result, error) {
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	summary, err := service.StatusSummary(bookID)
	if err != nil {
		return nil, cli.AsError(err)
	}
	view := statusView{
		DryRun: env.Globals.DryRun, Book: string(bookID), Enabled: summary.Enabled,
		Fallback: summary.Fallback, States: make([]statusState, 0, len(summary.Levels)),
		Assignments: make([]statusAssignment, 0, len(summary.Assignments)),
	}
	for _, level := range summary.Levels {
		state := statusState{
			ID: level.ID, Label: level.Label, Color: level.Color, Description: level.Description,
		}
		if !level.DescriptionBlock.IsZero() {
			state.Block = level.DescriptionBlock.String()
		}
		view.States = append(view.States, state)
	}
	for _, assignment := range summary.Assignments {
		view.Assignments = append(view.Assignments, statusAssignment{
			ID: assignment.ID.String(), Target: assignment.Target, State: assignment.Status,
		})
	}
	return &cli.Result{Data: view}, nil
}
