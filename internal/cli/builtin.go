package cli

import (
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/buildinfo"
)

const tagline = "Command-Line Authoring for Blocks and Orchestration Kit"

type commandInfo struct {
	Name     string `json:"name"`
	Summary  string `json:"summary"`
	Usage    string `json:"usage"`
	Mutating bool   `json:"mutating,omitempty"`
}

func (info commandInfo) Text() string {
	text := info.Usage + "\n"
	if info.Summary != "" {
		text += "  " + info.Summary + "\n"
	}
	if info.Mutating {
		text += "  mutating; supports --dry-run\n"
	}
	return text
}

type helpList struct {
	Product  string        `json:"product"`
	CLI      string        `json:"cli"`
	Tagline  string        `json:"tagline"`
	Usage    string        `json:"usage"`
	Commands []commandInfo `json:"commands"`
}

func (list helpList) Text() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s - %s\n", list.Product, list.Tagline)
	fmt.Fprintf(&builder, "usage: %s\n\ncommands:\n", list.Usage)
	width := 0
	for _, command := range list.Commands {
		if len(command.Name) > width {
			width = len(command.Name)
		}
	}
	for _, command := range list.Commands {
		fmt.Fprintf(&builder, "  %-*s  %s\n", width, command.Name, command.Summary)
	}
	return builder.String()
}

type versionData struct {
	Product string `json:"product"`
	CLI     string `json:"cli"`
	Version string `json:"version"`
}

func (data versionData) Text() string {
	return fmt.Sprintf("%s %s (%s)", data.Product, data.Version, data.CLI)
}

func commandInfoOf(command *Command) commandInfo {
	return commandInfo{
		Name:     command.Name,
		Summary:  command.Summary,
		Usage:    command.Usage,
		Mutating: command.Mutating,
	}
}

// helpListResult describes the whole command surface.
func helpListResult(registry *Registry) *Result {
	commands := registry.List()
	infos := make([]commandInfo, 0, len(commands))
	for _, command := range commands {
		infos = append(infos, commandInfoOf(command))
	}
	return &Result{Data: helpList{
		Product:  buildinfo.Product,
		CLI:      buildinfo.CLI,
		Tagline:  tagline,
		Usage:    buildinfo.CLI + " <command> [flags]",
		Commands: infos,
	}}
}

// commandHelpResult describes one command.
func commandHelpResult(registry *Registry, name string) (*Result, error) {
	command, ok := registry.Get(name)
	if !ok {
		return nil, Usage("usage.unknown-command", "unknown command %q", name)
	}
	return &Result{Data: commandInfoOf(command)}, nil
}

func registerBuiltins(registry *Registry) {
	registry.Add(&Command{
		Name:    "help",
		Summary: "list the commands or describe one",
		Usage:   "pecia help [command]",
		Run: func(env *Env, args []string) (*Result, error) {
			if len(args) > 1 {
				return nil, Usage("usage.unexpected-argument", "help takes at most one command").
					WithHint("run `pecia help` for the command list")
			}
			if len(args) == 1 {
				return commandHelpResult(registry, args[0])
			}
			return helpListResult(registry), nil
		},
	})
	registry.Add(&Command{
		Name:    "version",
		Summary: "print the version",
		Usage:   "pecia version",
		Run: func(env *Env, args []string) (*Result, error) {
			if len(args) > 0 {
				return nil, Usage("usage.unexpected-argument", "version takes no arguments")
			}
			return &Result{Data: versionData{
				Product: buildinfo.Product,
				CLI:     buildinfo.CLI,
				Version: buildinfo.Version,
			}}, nil
		},
	})
}
