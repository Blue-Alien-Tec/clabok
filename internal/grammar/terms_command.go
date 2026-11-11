package grammar

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// TermLayerInfo describes one layer file for list output.
type TermLayerInfo struct {
	Scope  string `json:"scope"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Count  int    `json:"count"`
}

// TermEntry is one term in the effective set.
type TermEntry struct {
	Term   string `json:"term"`
	Form   string `json:"form"`
	Layer  string `json:"layer"`
	Source string `json:"source"`
}

type termsPayload struct {
	DryRun   bool            `json:"dryRun"`
	Action   string          `json:"action"`
	Term     string          `json:"term,omitempty"`
	Reason   string          `json:"reason,omitempty"`
	Scope    string          `json:"scope,omitempty"`
	ID       string          `json:"id,omitempty"`
	Pending  []PendingTerm   `json:"pending,omitempty"`
	Terms    []string        `json:"terms,omitempty"`
	Layers   []TermLayerInfo `json:"layers,omitempty"`
	Entries  []TermEntry     `json:"entries,omitempty"`
	Warnings []Warning       `json:"warnings,omitempty"`
	Count    int             `json:"count,omitempty"`
	Changed  bool            `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload termsPayload) DryRunResult() bool { return payload.DryRun }

func (payload termsPayload) Text() string {
	var builder strings.Builder
	switch payload.Action {
	case "pending":
		for _, entry := range payload.Pending {
			fmt.Fprintf(&builder, "%s %s [%s] %s (%s)\n",
				entry.ID, entry.Action, entry.ScopeOf(), entry.Term, entry.Created)
		}
		fmt.Fprintf(&builder, "%d pending request(s)\n", len(payload.Pending))
	case "list":
		for _, layer := range payload.Layers {
			state := fmt.Sprintf("%d term(s)", layer.Count)
			if !layer.Exists {
				state = "missing"
			}
			fmt.Fprintf(&builder, "%s (%s, %s)\n", layer.Scope, layer.Path, state)
		}
		for _, entry := range payload.Entries {
			form := ""
			if entry.Form == "substring" {
				form = "  [substring]"
			}
			fmt.Fprintf(&builder, "%s  %s%s\n", entry.Layer, entry.Term, form)
		}
		fmt.Fprintf(&builder, "%d term(s)\n", payload.Count)
	case "commit":
		fmt.Fprintf(&builder, "committed %s %s [%s] (%d term(s))\n",
			payload.Action, payload.Term, payload.Scope, payload.Count)
	case "discard":
		fmt.Fprintf(&builder, "discarded %s\n", payload.ID)
	default:
		prefix := ""
		if payload.DryRun {
			prefix = "dry run: "
		}
		fmt.Fprintf(&builder, "%srequest %s %s [%s] pending\n",
			prefix, payload.Action, payload.Term, payload.Scope)
	}
	for _, warning := range payload.Warnings {
		fmt.Fprintf(&builder, "warning: %s\n", warning.Message)
	}
	return builder.String()
}

func termsCommand() *cli.Command {
	var term, reason, id, scope, book, language string
	var confirm, allTerms bool
	return &cli.Command{
		Name:     "terms",
		Summary:  "request, confirm and inspect the store-level term layers",
		Usage:    "pecia terms <set|delete|pending|commit|discard|list|check> [--term <t>] [--reason <r>] [--scope <global|code>|--book <b>] [--id <id>] [--confirm]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("terms")
			set.StringVar(&term, "term", "", "term to add or remove")
			set.StringVar(&reason, "reason", "", "why the term needs the change")
			set.StringVar(&id, "id", "", "pending request id")
			set.StringVar(&scope, "scope", "", "global or a language code such as de-DE")
			set.StringVar(&book, "book", "", "book whose language scopes the term")
			set.StringVar(&language, "language", "", "show the effective layer of one language")
			set.BoolVar(&confirm, "confirm", false, "confirm a commit")
			set.BoolVar(&allTerms, "all", false, "check every supported language")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"terms takes set, delete, pending, commit, discard, list or check").
					WithHint("pecia terms set --term <t> --reason <r> --scope global")
			}
			life, layout, lifeErr := lifecycle.FromEnv(env)
			if lifeErr != nil {
				return nil, lifeErr
			}
			values, configErr := config.Load(env)
			if configErr != nil {
				return nil, configErr
			}
			pendingPath := layout.TermsPending()
			termsPath := layout.Terms()
			termsDir := layout.TermsDir()
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
				resolved, warnings, scopeErr := resolveTermScope(values, life, scope, book)
				if scopeErr != nil {
					return nil, scopeErr
				}
				if env.Globals.DryRun {
					now := time.Now()
					preview := PendingTerm{
						ID: pendingID(action, term, reason, resolved, now), Action: action,
						Term: term, Reason: reason, Scope: resolved,
						Created: now.UTC().Format(time.RFC3339),
					}
					return &cli.Result{Data: termsPayload{
						DryRun: true, Action: args[0], Term: term, Reason: reason,
						Scope: preview.Scope, ID: preview.ID, Warnings: warnings, Changed: true,
					}}, nil
				}
				entry, addErr := AddPending(pendingPath, action, term, reason, resolved, time.Now())
				if addErr != nil {
					return nil, cli.AsError(addErr)
				}
				return &cli.Result{Data: termsPayload{
					Action: args[0], Term: entry.Term, Reason: entry.Reason, Scope: entry.Scope,
					ID: entry.ID, Warnings: warnings, Changed: true,
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
						WithHint("repeat the command with --confirm to write the term file")
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
								Scope: entry.ScopeOf(), ID: entry.ID, Changed: true,
							}}, nil
						}
					}
					return nil, cli.Reject("terms.unknown-request",
						"no pending request with id %s", id)
				}
				entry, count, commitErr := CommitPending(pendingPath, termsPath, termsDir, id)
				if commitErr != nil {
					return nil, cli.AsError(commitErr)
				}
				return &cli.Result{Data: termsPayload{
					Action: "commit", Term: entry.Term, Scope: entry.ScopeOf(),
					ID: entry.ID, Count: count, Changed: true,
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
				return listTerms(env, termsPath, termsDir, language)
			case "check":
				return checkTerms(env, values, life, layout, term, scope, book, allTerms)
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown terms action %q; use set, delete, pending, commit, discard, list or check",
					args[0])
			}
		},
	}
}

// resolveTermScope turns --scope or --book into a canonical scope and an optional warning.
func resolveTermScope(values *config.Values, life *lifecycle.Lifecycle,
	rawScope, rawBook string) (string, []Warning, *cli.Error) {
	if rawScope != "" && rawBook != "" {
		return "", nil, cli.Usage("usage.invalid-arguments",
			"pass either --scope or --book, not both")
	}
	if rawScope == "" && rawBook == "" {
		return "", nil, cli.Usage("usage.missing-argument",
			"--scope global|<code> or --book <id> is required").
			WithHint("the consent step must know which layer the term belongs to")
	}
	scope := rawScope
	if rawBook != "" {
		bookID, bookErr := requireBook(rawBook)
		if bookErr != nil {
			return "", nil, bookErr
		}
		settings, settingsErr := BookSettings(life, bookID)
		if settingsErr != nil {
			return "", nil, cli.AsError(settingsErr)
		}
		scope = Resolve(settings, values.GrammarLanguage)
	}
	normalized, scopeErr := NormalizeScope(scope)
	if scopeErr != nil {
		return "", nil, cli.Usage("usage.invalid-value",
			"the scope must be global or a language code such as de-DE")
	}
	if normalized == ScopeGlobal {
		return normalized, nil, nil
	}
	canonical, warning, checkErr := LanguageCheck(values.GrammarEndpoint,
		values.GrammarTimeout)(normalized)
	if checkErr != nil {
		return "", nil, cli.AsError(checkErr)
	}
	warnings := make([]Warning, 0)
	if warning != "" {
		warnings = append(warnings, Warning{Code: LanguageUnverified, Message: warning})
	}
	if canonical != "" {
		normalized = canonical
	}
	return normalized, warnings, nil
}

func listTerms(env *cli.Env, termsPath, termsDir, language string) (*cli.Result, error) {
	language = strings.TrimSpace(language)
	layers := make([]TermLayerInfo, 0)
	entries := make([]TermEntry, 0)
	if language == "" {
		for _, layer := range TermLayers(termsPath, termsDir) {
			layers = append(layers, layerInfo(layer))
			for _, raw := range layer.Terms {
				form := ParseTermForm(raw)
				if form.Term == "" {
					continue
				}
				entries = append(entries, TermEntry{
					Term: form.Term, Form: formName(form), Layer: layer.Scope, Source: layer.Path,
				})
			}
		}
	} else {
		seen := map[string]bool{}
		for _, layer := range LoadTermSet(termsPath, termsDir, language).Layers {
			layers = append(layers, layerInfo(layer))
			for _, raw := range layer.Terms {
				form := ParseTermForm(raw)
				if form.Term == "" {
					continue
				}
				key := strings.ToLower(form.Term)
				if seen[key] {
					continue
				}
				seen[key] = true
				entries = append(entries, TermEntry{
					Term: form.Term, Form: formName(form), Layer: layer.Scope, Source: layer.Path,
				})
			}
		}
	}
	return &cli.Result{Data: termsPayload{
		DryRun: env.Globals.DryRun, Action: "list", Layers: layers, Entries: entries,
		Count: len(entries),
	}}, nil
}

func layerInfo(layer TermLayer) TermLayerInfo {
	_, err := os.Stat(layer.Path)
	return TermLayerInfo{
		Scope: layer.Scope, Path: layer.Path, Exists: err == nil, Count: len(layer.Terms),
	}
}

func formName(form TermForm) string {
	if form.Substring {
		return "substring"
	}
	return "word"
}

// TermCheckOutcome is the server judgment of one term in one language.
type TermCheckOutcome struct {
	Language string   `json:"language"`
	Accepted bool     `json:"accepted"`
	Findings int      `json:"findings"`
	Rules    []string `json:"rules,omitempty"`
}

type termsCheckPayload struct {
	DryRun   bool               `json:"dryRun"`
	Term     string             `json:"term"`
	Scope    string             `json:"scope"`
	TooBroad bool               `json:"tooBroad"`
	Checked  int                `json:"checked"`
	Results  []TermCheckOutcome `json:"results,omitempty"`
	Warnings []Warning          `json:"warnings,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt; the command writes nothing.
func (payload termsCheckPayload) DryRunResult() bool { return payload.DryRun }

func (payload termsCheckPayload) Text() string {
	var builder strings.Builder
	for _, result := range payload.Results {
		state := "suppressible"
		if result.Accepted {
			state = "accepted as a word"
		}
		fmt.Fprintf(&builder, "%s: %s", result.Language, state)
		if len(result.Rules) > 0 {
			fmt.Fprintf(&builder, " (%s)", strings.Join(result.Rules, ", "))
		}
		builder.WriteByte('\n')
	}
	if payload.TooBroad {
		fmt.Fprintf(&builder, "%q is too broad for %s\n", payload.Term, payload.Scope)
	} else {
		fmt.Fprintf(&builder, "%q is suppressible for %s\n", payload.Term, payload.Scope)
	}
	for _, warning := range payload.Warnings {
		fmt.Fprintf(&builder, "warning: %s\n", warning.Message)
	}
	return builder.String()
}

// checkTerms asks the server whether a term is an ordinary word in the relevant languages. The
// server is the authority; nothing is cached and no term file is touched.
func checkTerms(env *cli.Env, values *config.Values, life *lifecycle.Lifecycle,
	layout store.Layout, term, scope, book string, all bool) (*cli.Result, error) {
	if strings.TrimSpace(term) == "" {
		return nil, cli.Usage("usage.missing-argument", "--term is required")
	}
	normalized, warnings, scopeErr := resolveTermScope(values, life, scope, book)
	if scopeErr != nil {
		return nil, scopeErr
	}
	client := NewClientTimeout(values.GrammarEndpoint, values.GrammarTimeout)
	languages := make([]string, 0)
	if normalized == ScopeGlobal {
		if all {
			list, _, _, _ := languageSnapshot(client, false)
			for _, language := range list {
				languages = append(languages, language.Code)
			}
		} else {
			used, usedErr := StoreLanguages(life, layout, values.GrammarLanguage)
			if usedErr != nil {
				return nil, cli.AsError(usedErr)
			}
			languages = used
		}
	} else {
		languages = []string{normalized}
	}
	if len(languages) == 0 {
		return nil, cli.Fail("grammar.unreachable",
			"the grammar server cannot be reached and no language is known").
			WithHint("run `pecia grammar serve` once, or pass --all")
	}
	payload := termsCheckPayload{
		DryRun: env.Globals.DryRun, Term: strings.TrimSpace(term), Scope: normalized,
		Warnings: warnings,
	}
	ctx := context.Background()
	for _, language := range languages {
		canonical, ok, supportedErr := client.Supported(ctx, language)
		if supportedErr != nil {
			return nil, cli.AsError(supportedErr)
		}
		if !ok {
			continue
		}
		result, checkErr := client.Check(ctx, strings.TrimSpace(term), canonical.Code)
		if checkErr != nil {
			return nil, cli.AsError(checkErr)
		}
		outcome := TermCheckOutcome{
			Language: canonical.Code, Accepted: len(result.Findings) == 0,
			Findings: len(result.Findings),
		}
		seen := map[string]bool{}
		for _, finding := range result.Findings {
			if finding.Rule != "" && !seen[finding.Rule] {
				seen[finding.Rule] = true
				outcome.Rules = append(outcome.Rules, finding.Rule)
			}
		}
		if outcome.Accepted {
			payload.TooBroad = true
		}
		payload.Results = append(payload.Results, outcome)
	}
	payload.Checked = len(payload.Results)
	return &cli.Result{Data: payload}, nil
}
