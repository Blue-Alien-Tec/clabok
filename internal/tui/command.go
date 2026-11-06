package tui

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/body"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
	"github.com/Blue-Alien-Tec/clabok/internal/idspace"
	"github.com/Blue-Alien-Tec/clabok/internal/library"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

// Register adds the interactive terminal surface.
func Register(registry *cli.Registry) {
	registry.Add(tuiCommand())
}

type payload struct {
	Book string `json:"book,omitempty"`
}

// Text reports the exit of the interactive loop.
func (payload payload) Text() string {
	if payload.Book == "" {
		return "tui closed\n"
	}
	return "tui closed " + payload.Book + "\n"
}

func tuiCommand() *cli.Command {
	var book string
	return &cli.Command{
		Name:     "tui",
		Summary:  "interactive terminal surface over the core services",
		Usage:    "pecia tui [--book <id>]",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("tui")
			set.StringVar(&book, "book", "", "book id")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.invalid-arguments", "tui takes no arguments")
			}
			if env.Globals.DryRun {
				return nil, cli.Reject("tui.no-dry-run", "the TUI is interactive and has no dry run")
			}
			if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
				return nil, cli.Reject("tui.no-terminal",
					"the TUI needs an interactive terminal").
					WithHint("use the CLI commands for automation and CI")
			}
			services, serviceErr := servicesFor(env)
			if serviceErr != nil {
				return nil, serviceErr
			}
			registry, registryErr := loadRegistry(env)
			if registryErr != nil {
				return nil, registryErr
			}
			controller := New(*services, pickerEntries(registry, services.Life), model.BookID(book))
			runLoop(controller)
			return &cli.Result{Data: payload{Book: string(controller.Book)}}, nil
		},
	}
}

func servicesFor(env *cli.Env) (*Services, *cli.Error) {
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	life, layout, lifeErr := lifecycle.FromEnv(env)
	if lifeErr != nil {
		return nil, lifeErr
	}
	allocator := idspace.New(life, layout)
	bodyService := body.NewWithAllocator(life, allocator)
	bodyService.Grammar = grammar.NewGate(values.GrammarEndpoint,
		grammar.ParsePolicy(values.GrammarPolicy), grammar.ReadTerms(layout.Terms()))
	return &Services{
		Life:      life,
		Structure: structure.New(life, allocator),
		Body:      bodyService,
		Studio: &library.Service{
			Life: life, Layout: layout, Values: values, IDs: allocator,
		},
	}, nil
}

func loadRegistry(env *cli.Env) (*store.Registry, *cli.Error) {
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	layout, layoutErr := store.NewLayout(values.StoreRoot)
	if layoutErr != nil {
		return nil, cli.AsError(layoutErr)
	}
	registry, registryErr := store.LoadRegistry(layout.Registry())
	if registryErr != nil {
		return nil, cli.AsError(registryErr)
	}
	return registry, nil
}

func pickerEntries(registry *store.Registry, life *lifecycle.Lifecycle) []PickerEntry {
	entries := make([]PickerEntry, 0, len(registry.Books))
	for _, entry := range registry.Books {
		if entry.Kind == store.KindLibrary {
			continue
		}
		open := false
		if state, err := life.State(entry.ID); err == nil {
			open = state.Extracted
		}
		entries = append(entries, PickerEntry{ID: string(entry.ID), Path: entry.Path, Open: open})
	}
	return entries
}

func runLoop(controller *Controller) {
	reader := bufio.NewReader(os.Stdin)
	for {
		drawScreen(controller.Model)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return
		}
		command := strings.TrimSpace(line)
		if controller.Model.Mode == ModePrompt {
			controller.Model.PromptText = command
			controller.Handle("enter")
		} else if command != "" && controller.Handle(command) {
			return
		}
		if err == io.EOF {
			return
		}
	}
}

func drawScreen(model Model) {
	fmt.Print("\x1b[2J\x1b[H")
	for _, line := range model.View(screenWidth(), screenHeight()) {
		fmt.Println(line)
	}
}

func screenWidth() int {
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 24 {
			return value
		}
	}
	return 100
}

func screenHeight() int {
	if raw := strings.TrimSpace(os.Getenv("LINES")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 6 {
			return value
		}
	}
	return 30
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
