package grammar

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
)

type termsPayload struct {
	DryRun  bool          `json:"dryRun"`
	Action  string        `json:"action"`
	Term    string        `json:"term,omitempty"`
	Reason  string        `json:"reason,omitempty"`
	ID      string        `json:"id,omitempty"`
	Pending []PendingTerm `json:"pending,omitempty"`
	Terms   []string      `json:"terms,omitempty"`
	Count   int           `json:"count,omitempty"`
	Changed bool          `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload termsPayload) DryRunResult() bool { return payload.DryRun }

func (payload termsPayload) Text() string {
	var builder strings.Builder
	switch payload.Action {
	case "pending":
		for _, entry := range payload.Pending {
			fmt.Fprintf(&builder, "%s %s %s (%s)\n", entry.ID, entry.Action, entry.Term, entry.Created)
		}
		fmt.Fprintf(&builder, "%d pending request(s)\n", len(payload.Pending))
	case "list":
		for _, term := range payload.Terms {
			builder.WriteString(term + "\n")
		}
		fmt.Fprintf(&builder, "%d term(s)\n", len(payload.Terms))
	case "commit":
		fmt.Fprintf(&builder, "committed %s %s (%d term(s))\n", payload.Action, payload.Term, payload.Count)
	case "discard":
		fmt.Fprintf(&builder, "discarded %s\n", payload.ID)
	default:
		prefix := ""
		if payload.DryRun {
			prefix = "dry run: "
		}
		fmt.Fprintf(&builder, "%srequest %s %s pending\n", prefix, payload.Action, payload.Term)
	}
	return builder.String()
}

func termsCommand() *cli.Command {
	var term, reason, id string
	var confirm bool
	return &cli.Command{
		Name:     "terms",
		Summary:  "request, confirm and inspect the store-level term list",
		Usage:    "pecia terms <set|delete|pending|commit|discard|list> [--term <t>] [--reason <r>] [--id <id>] [--confirm]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("terms")
			set.StringVar(&term, "term", "", "term to add or remove")
			set.StringVar(&reason, "reason", "", "why the term needs the change")
			set.StringVar(&id, "id", "", "pending request id")
			set.BoolVar(&confirm, "confirm", false, "confirm a commit")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"terms takes set, delete, pending, commit, discard or list").
					WithHint("pecia terms set --term <t> --reason <r>")
			}
			_, layout, lifeErr := lifecycle.FromEnv(env)
			if lifeErr != nil {
				return nil, lifeErr
			}
			pendingPath := layout.TermsPending()
			termsPath := layout.Terms()
			switch args[0] {
			case "set", "delete":
				action := "add"
				if args[0] == "delete" {
					action = "remove"
				}
				if strings.TrimSpace(term) == "" {
					return nil, cli.Usage("usage.missing-argument", "--term is required")
				}
				if strings.TrimSpace(reason) == "" {
					return nil, cli.Usage("usage.missing-argument", "--reason is required")
				}
				if env.Globals.DryRun {
					now := time.Now()
					preview := PendingTerm{
						ID: pendingID(action, term, reason, now), Action: action,
						Term: term, Reason: reason, Created: now.UTC().Format(time.RFC3339),
					}
					return &cli.Result{Data: termsPayload{
						DryRun: true, Action: args[0], Term: term, Reason: reason,
						ID: preview.ID, Changed: true,
					}}, nil
				}
				entry, addErr := AddPending(pendingPath, action, term, reason, time.Now())
				if addErr != nil {
					return nil, cli.AsError(addErr)
				}
				return &cli.Result{Data: termsPayload{
					Action: args[0], Term: entry.Term, Reason: entry.Reason,
					ID: entry.ID, Changed: true,
				}}, nil
			case "pending":
				entries, readErr := ReadPending(pendingPath)
				if readErr != nil {
					return nil, cli.AsError(readErr)
				}
				return &cli.Result{Data: termsPayload{
					DryRun: env.Globals.DryRun, Action: "pending",
					Pending: entries, Count: len(entries),
				}}, nil
			case "commit":
				if id == "" {
					return nil, cli.Usage("usage.missing-argument", "--id is required")
				}
				if !confirm {
					return nil, cli.Reject("terms.confirmation-required",
						"a term change needs an explicit confirmation").
						WithHint("repeat the command with --confirm to write the term list")
				}
				if env.Globals.DryRun {
					entries, readErr := ReadPending(pendingPath)
					if readErr != nil {
						return nil, cli.AsError(readErr)
					}
					for _, entry := range entries {
						if entry.ID == id {
							return &cli.Result{Data: termsPayload{
								DryRun: true, Action: "commit", Term: entry.Term,
								ID: entry.ID, Changed: true,
							}}, nil
						}
					}
					return nil, cli.Reject("terms.unknown-request",
						"no pending request with id %s", id)
				}
				entry, count, commitErr := CommitPending(pendingPath, termsPath, id)
				if commitErr != nil {
					return nil, cli.AsError(commitErr)
				}
				return &cli.Result{Data: termsPayload{
					Action: "commit", Term: entry.Term, ID: entry.ID, Count: count, Changed: true,
				}}, nil
			case "discard":
				if id == "" {
					return nil, cli.Usage("usage.missing-argument", "--id is required")
				}
				if env.Globals.DryRun {
					entries, readErr := ReadPending(pendingPath)
					if readErr != nil {
						return nil, cli.AsError(readErr)
					}
					for _, entry := range entries {
						if entry.ID == id {
							return &cli.Result{Data: termsPayload{
								DryRun: true, Action: "discard", ID: entry.ID, Changed: true,
							}}, nil
						}
					}
					return nil, cli.Reject("terms.unknown-request",
						"no pending request with id %s", id)
				}
				entry, discardErr := DiscardPending(pendingPath, id)
				if discardErr != nil {
					return nil, cli.AsError(discardErr)
				}
				return &cli.Result{Data: termsPayload{
					Action: "discard", ID: entry.ID, Changed: true,
				}}, nil
			case "list":
				terms := ReadTerms(termsPath)
				return &cli.Result{Data: termsPayload{
					DryRun: env.Globals.DryRun, Action: "list", Terms: terms, Count: len(terms),
				}}, nil
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown terms action %q; use set, delete, pending, commit, discard or list", args[0])
			}
		},
	}
}
