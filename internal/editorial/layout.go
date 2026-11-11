package editorial

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/repolayout"
)

type repoLayoutPayload struct {
	DryRun   bool              `json:"dryRun"`
	Book     string            `json:"book"`
	Action   string            `json:"action"`
	Anchor   string            `json:"anchor,omitempty"`
	Path     string            `json:"path,omitempty"`
	Depth    int               `json:"depth,omitempty"`
	Changed  bool              `json:"changed"`
	Grammar  *grammar.Report   `json:"grammar,omitempty"`
	Warnings []grammar.Warning `json:"warnings,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result repoLayoutPayload) DryRunResult() bool { return result.DryRun }

func (result repoLayoutPayload) Text() string {
	change := "unchanged"
	if result.Changed {
		change = "changed"
	}
	line := fmt.Sprintf("%s layout %s: %s", result.Book, result.Action, change)
	if result.Grammar != nil {
		line += grammarReportText(*result.Grammar)
	}
	return line + "\n"
}

type repoLayoutView struct {
	DryRun      bool              `json:"dryRun"`
	Book        string            `json:"book"`
	Anchor      string            `json:"anchor,omitempty"`
	Root        string            `json:"root"`
	Depth       int               `json:"depth"`
	Exclude     []string          `json:"exclude,omitempty"`
	Notes       map[string]string `json:"notes,omitempty"`
	Tree        string            `json:"tree,omitempty"`
	Unavailable string            `json:"unavailable,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt; a view writes nothing either way.
func (result repoLayoutView) DryRunResult() bool { return true }

func layoutCommand() *cli.Command {
	var book, parent, path, noteText, file string
	var depth int
	var clear bool
	actions := []cli.Action{
		{Name: "add", Summary: "add the repository-layout block",
			Reads: []string{"book", "parent"}},
		{Name: "set", Summary: "set the layout root and depth",
			Reads: []string{"book", "path", "depth"}},
		{Name: "exclude", Summary: "exclude a path from the generated tree",
			Reads: []string{"book", "path"}},
		{Name: "include", Summary: "drop an exclusion",
			Reads: []string{"book", "path", "clear"}},
		{Name: "note", Summary: "set or clear the one-line note of a path",
			Reads: []string{"book", "path", "note", "file", "clear"}},
		{Name: "show", Summary: "show the layout configuration and its notes",
			Reads: []string{"book"}},
	}
	return &cli.Command{
		Name:     "layout",
		Summary:  "author the generated repository-layout block from git ls-files",
		Usage:    "pecia layout <add|set|exclude|include|note|show> --book <id> [--parent <node>] [--path <dir>] [--depth <n>] [--note <t> | --file <path|->] [--clear]",
		Actions:  actions,
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("layout")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&parent, "parent", "", "target node for add")
			set.StringVar(&path, "path", "", "repository-relative path")
			set.IntVar(&depth, "depth", 0, "levels below the root (default 2)")
			set.StringVar(&noteText, "note", "", "note text")
			set.StringVar(&file, "file", "", "note file, or - for stdin")
			set.BoolVar(&clear, "clear", false, "remove the path from the config (note) or the excludes (include)")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"layout takes %s", cli.ActionList(actions)).
					WithHint("pecia layout add --book <id> --parent <node> --path . --depth 2")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			action := args[0]
			switch action {
			case "add":
				if path == "" {
					return nil, cli.Usage("usage.missing-argument", "--path is required")
				}
				parentID, idErr := requireID(parent, "parent")
				if idErr != nil {
					return nil, idErr
				}
				_, anchor, readErr := service.RepoLayout(bookID)
				if readErr != nil {
					return nil, cli.AsError(readErr)
				}
				if !anchor.IsZero() {
					return nil, cli.Reject("layout.already-configured",
						"the book already has a RepoLayout block").
						WithHint("use `pecia layout set` or remove the block first")
				}
				level := depth
				if level <= 0 {
					level = 2
				}
				layoutConfig := model.LayoutConfig{Root: path, Depth: level}
				receipt, addErr := service.AddRepoLayout(bookID, parentID, layoutConfig, env.Globals.DryRun)
				if addErr != nil {
					return nil, cli.AsError(addErr)
				}
				return &cli.Result{Data: repoLayoutPayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					Anchor: receipt.Target.String(), Path: path, Depth: level, Changed: receipt.Changed,
				}}, nil
			case "set":
				layoutConfig, _, readErr := service.RepoLayout(bookID)
				if readErr != nil {
					return nil, cli.AsError(readErr)
				}
				if path != "" {
					layoutConfig.Root = path
				}
				if depth > 0 {
					layoutConfig.Depth = depth
				}
				receipt, setErr := service.SetRepoLayout(bookID, layoutConfig, env.Globals.DryRun)
				if setErr != nil {
					return nil, cli.AsError(setErr)
				}
				return &cli.Result{Data: repoLayoutPayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					Path: layoutConfig.Root, Depth: layoutConfig.Depth, Changed: receipt.Changed,
				}}, nil
			case "exclude", "include":
				if path == "" {
					return nil, cli.Usage("usage.missing-argument", "--path is required")
				}
				layoutConfig, _, readErr := service.RepoLayout(bookID)
				if readErr != nil {
					return nil, cli.AsError(readErr)
				}
				target := normalizeLayoutPath(path)
				if action == "exclude" {
					layoutConfig.Exclude = append(layoutConfig.Exclude, target)
				} else {
					layoutConfig.Exclude = removeLayoutPath(layoutConfig.Exclude, target)
				}
				receipt, setErr := service.SetRepoLayout(bookID, layoutConfig, env.Globals.DryRun)
				if setErr != nil {
					return nil, cli.AsError(setErr)
				}
				return &cli.Result{Data: repoLayoutPayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					Path: target, Changed: receipt.Changed,
				}}, nil
			case "note":
				if path == "" {
					return nil, cli.Usage("usage.missing-argument", "--path is required")
				}
				layoutConfig, _, readErr := service.RepoLayout(bookID)
				if readErr != nil {
					return nil, cli.AsError(readErr)
				}
				target := normalizeLayoutPath(path)
				var report grammar.Report
				if clear {
					if layoutConfig.Notes != nil {
						delete(layoutConfig.Notes, target)
					}
				} else {
					prose, textErr := readText(noteText, file)
					if textErr != nil {
						return nil, textErr
					}
					report, textErr = evaluateProse(env, bookID, prose)
					if textErr != nil {
						return nil, textErr
					}
					if report.Blocked() {
						return nil, proseRejection(report)
					}
					if layoutConfig.Notes == nil {
						layoutConfig.Notes = map[string]string{}
					}
					layoutConfig.Notes[target] = prose
				}
				receipt, setErr := service.SetRepoLayout(bookID, layoutConfig, env.Globals.DryRun)
				if setErr != nil {
					return nil, cli.AsError(setErr)
				}
				result := repoLayoutPayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					Path: target, Changed: receipt.Changed,
				}
				result.Grammar = grammarPtr(report)
				result.Warnings = report.Warnings()
				return &cli.Result{Data: result}, nil
			case "show":
				layoutConfig, anchor, readErr := service.RepoLayout(bookID)
				if readErr != nil {
					return nil, cli.AsError(readErr)
				}
				values, configErr := config.Load(env)
				if configErr != nil {
					return nil, configErr
				}
				view := repoLayoutView{
					DryRun: env.Globals.DryRun, Book: string(bookID), Root: layoutConfig.Root,
					Depth: layoutConfig.Depth, Exclude: layoutConfig.Exclude, Notes: layoutConfig.Notes,
				}
				if !anchor.IsZero() {
					view.Anchor = anchor.String()
				}
				tree, renderErr := repolayout.Render(values.ProjectDir, &layoutConfig)
				switch {
				case errors.Is(renderErr, repolayout.ErrNoGit):
					view.Unavailable = "no git work tree"
				case renderErr != nil:
					view.Unavailable = renderErr.Error()
				default:
					view.Tree = tree
				}
				return &cli.Result{Data: view}, nil
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown layout action %q; use %s", action, cli.ActionList(actions))
			}
		},
	}
}

func normalizeLayoutPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "/")
	return strings.Trim(value, "/")
}

func removeLayoutPath(paths []string, target string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != target {
			result = append(result, path)
		}
	}
	return result
}
