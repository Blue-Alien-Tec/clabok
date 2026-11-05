package render

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

type syncPayload struct {
	DryRun  bool     `json:"dryRun"`
	Book    string   `json:"book"`
	Out     string   `json:"out"`
	Mode    string   `json:"mode"`
	Status  string   `json:"status"`
	Files   []string `json:"files"`
	Changed []string `json:"changed,omitempty"`
	Foreign []string `json:"foreign,omitempty"`
	Pruned  []string `json:"pruned,omitempty"`
	Written bool     `json:"written"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload syncPayload) DryRunResult() bool { return payload.DryRun }

func (payload syncPayload) Text() string {
	state := "checked"
	if payload.Written {
		state = "written"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s: %s (%d file(s), %d changed, %d pruned) %s\n",
		payload.Book, payload.Status, len(payload.Files), len(payload.Changed), len(payload.Pruned), state)
	for _, name := range payload.Changed {
		fmt.Fprintf(&builder, "  changed %s\n", name)
	}
	for _, name := range payload.Foreign {
		fmt.Fprintf(&builder, "  foreign %s\n", name)
	}
	return builder.String()
}

type checkPayload struct {
	Book     string    `json:"book"`
	Findings []Finding `json:"findings"`
	Blocking int       `json:"blocking"`
	Clean    bool      `json:"clean"`
}

func (payload checkPayload) Text() string {
	var builder strings.Builder
	for _, finding := range payload.Findings {
		level := "info"
		if finding.Blocking {
			level = "error"
		}
		fmt.Fprintf(&builder, "%s %s: %s\n", level, finding.Code, finding.Message)
	}
	fmt.Fprintf(&builder, "%d finding(s), %d blocking\n", len(payload.Findings), payload.Blocking)
	return builder.String()
}

// Register adds the render, sync and check commands.
func Register(registry *cli.Registry) {
	registry.Add(syncCommand())
	registry.Add(checkCommand("check"))
	registry.Add(checkCommand("verify"))
}

func syncCommand() *cli.Command {
	var book, out string
	var check, write, prune, force bool
	return &cli.Command{
		Name:     "sync",
		Summary:  "render the book and compare or write the output",
		Usage:    "pecia sync --book <id> [--out <dir>] [--check | --write] [--prune] [--force]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("sync")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&out, "out", "", "output directory (defaults to the registry path)")
			set.BoolVar(&check, "check", false, "report drift without writing")
			set.BoolVar(&write, "write", false, "write changed files")
			set.BoolVar(&prune, "prune", false, "remove generated files the mode no longer produces")
			set.BoolVar(&force, "force", false, "overwrite files without the generated marker")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "sync takes flags only")
			}
			if check && write {
				return nil, cli.Usage("usage.invalid-arguments", "use --check or --write, not both")
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			target, targetErr := outputDir(env, bookID, out)
			if targetErr != nil {
				return nil, targetErr
			}
			dryRun := !write || env.Globals.DryRun
			result, syncErr := service.Sync(bookID, target, SyncOptions{
				Check: check || dryRun, Write: !dryRun, Prune: prune, Force: force,
			})
			if syncErr != nil {
				return nil, cli.AsError(syncErr)
			}
			payload := syncPayload{
				DryRun: dryRun, Book: string(bookID), Out: target, Mode: string(result.Mode),
				Status: string(result.Status), Files: result.Files, Changed: result.Changed,
				Foreign: result.Foreign, Pruned: result.Pruned, Written: result.Written,
			}
			return &cli.Result{Data: payload}, nil
		},
	}
}

func checkCommand(name string) *cli.Command {
	var book string
	return &cli.Command{
		Name:    name,
		Summary: "run the read-only acceptance checks",
		Usage:   "pecia " + name + " --book <id>",
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet(name)
			set.StringVar(&book, "book", "", "book id")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "%s takes flags only", name)
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			service, serviceErr := newService(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			findings, checkErr := service.Check(bookID)
			if checkErr != nil {
				return nil, cli.AsError(checkErr)
			}
			blocking := 0
			for _, finding := range findings {
				if finding.Blocking {
					blocking++
				}
			}
			payload := checkPayload{
				Book: string(bookID), Findings: findings, Blocking: blocking, Clean: blocking == 0,
			}
			if blocking > 0 {
				return nil, cli.Fail("render.check-failed", "%d blocking finding(s)", blocking).
					WithHint("fix the reported findings and run the check again").
					WithDetails(payload)
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
		return "", cli.Usage("usage.missing-argument", "--book is required")
	}
	id := model.BookID(raw)
	if !id.Valid() {
		return "", cli.Usage("usage.invalid-value", "invalid book id %q", raw)
	}
	return id, nil
}

// outputDir resolves the target directory: --out, then the registry path.
func outputDir(env *cli.Env, book model.BookID, explicit string) (string, *cli.Error) {
	values, err := config.Load(env)
	if err != nil {
		return "", err
	}
	if explicit != "" {
		if filepath.IsAbs(explicit) {
			return filepath.Clean(explicit), nil
		}
		return filepath.Join(values.ProjectDir, explicit), nil
	}
	layout, layoutErr := store.NewLayout(values.StoreRoot)
	if layoutErr != nil {
		return "", cli.AsError(layoutErr)
	}
	registry, registryErr := store.LoadRegistry(layout.Registry())
	if registryErr != nil {
		return "", cli.AsError(registryErr)
	}
	entry, ok := registry.Find(book)
	if !ok {
		return "", cli.Fail("render.not-registered",
			"book %q is not registered; pass --out <dir>", book).
			WithHint("pecia sync --book <id> --out <dir>")
	}
	return filepath.Join(values.ProjectDir, filepath.FromSlash(entry.Path)), nil
}
