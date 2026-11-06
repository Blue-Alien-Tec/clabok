package editorial

import (
	"flag"
	"fmt"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
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

type statusState struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Color       string `json:"color"`
	Description string `json:"description"`
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
	States      []statusState      `json:"states"`
	Assignments []statusAssignment `json:"assignments"`
}

// DryRunResult marks the payload as a dry-run receipt; a view writes nothing either way.
func (result statusView) DryRunResult() bool { return true }

func statusCommand() *cli.Command {
	var book, target, id, state string
	return &cli.Command{
		Name:     "status",
		Summary:  "switch the status system, assign states and show the vocabulary",
		Usage:    "pecia status <enable|disable|set|clear|show> --book <id> [--target node|block --id <id>] [--state <id>]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("status")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&target, "target", "", "node or block")
			set.StringVar(&id, "id", "", "target id")
			set.StringVar(&state, "state", "", "state id")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"status takes enable, disable, set, clear or show").
					WithHint("pecia status set --book <id> --target node --id <id> --state final")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
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
				if action == "set" {
					if state == "" {
						return nil, cli.Usage("usage.missing-argument", "--state is required")
					}
					if _, known := model.FindStatus(state); !known {
						return nil, cli.Usage("usage.invalid-value", "unknown status %q", state).
							WithHint("run `pecia status show --book " + string(bookID) + "` for the states")
					}
				} else if state != "" {
					return nil, cli.Usage("usage.invalid-arguments", "clear takes no --state")
				}
				service, serviceErr := newService(env)
				if serviceErr != nil {
					return nil, serviceErr
				}
				receipt, err := service.SetStatus(bookID, targetID, state, env.Globals.DryRun)
				if err != nil {
					return nil, cli.AsError(err)
				}
				return &cli.Result{Data: statusPayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					Target: target, ID: targetID.String(), State: state, Changed: receipt.Changed,
				}}, nil
			case "show":
				if target != "" || id != "" || state != "" {
					return nil, cli.Usage("usage.invalid-arguments", "show takes --book only")
				}
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
					States:      make([]statusState, 0, len(model.Statuses)),
					Assignments: make([]statusAssignment, 0, len(summary.Assignments)),
				}
				for _, status := range model.Statuses {
					view.States = append(view.States, statusState{
						ID: status.ID, Label: status.Label, Color: status.Color,
						Description: status.Description,
					})
				}
				for _, assignment := range summary.Assignments {
					view.Assignments = append(view.Assignments, statusAssignment{
						ID: assignment.ID.String(), Target: assignment.Target, State: assignment.Status,
					})
				}
				return &cli.Result{Data: view}, nil
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown status action %q; use enable, disable, set, clear or show", action)
			}
		},
	}
}
