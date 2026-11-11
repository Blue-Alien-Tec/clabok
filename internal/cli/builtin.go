package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/buildinfo"
)

const tagline = "Command-Line Authoring for Blocks and Orchestration Kit"

type flagInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type actionInfo struct {
	Name    string     `json:"name"`
	Summary string     `json:"summary"`
	Usage   string     `json:"usage"`
	Flags   []flagInfo `json:"flags,omitempty"`
}

type commandInfo struct {
	Name     string       `json:"name"`
	Summary  string       `json:"summary"`
	Usage    string       `json:"usage"`
	Mutating bool         `json:"mutating,omitempty"`
	Actions  []actionInfo `json:"actions,omitempty"`
	Flags    []flagInfo   `json:"flags,omitempty"`
}

func (info commandInfo) Text() string {
	var builder strings.Builder
	builder.WriteString(info.Usage + "\n")
	if info.Summary != "" {
		builder.WriteString("  " + info.Summary + "\n")
	}
	if info.Mutating {
		builder.WriteString("  mutating; supports --dry-run\n")
	}
	if len(info.Actions) > 0 {
		for _, action := range info.Actions {
			fmt.Fprintf(&builder, "\n  %s\n    %s\n", action.Usage, action.Summary)
			writeFlags(&builder, action.Flags, flagWidth(action.Flags))
		}
		return builder.String()
	}
	writeFlags(&builder, info.Flags, flagWidth(info.Flags))
	return builder.String()
}

// writeFlags renders a flag table. A zero width means "no flags".
func writeFlags(builder *strings.Builder, infos []flagInfo, width int) {
	if len(infos) == 0 || width == 0 {
		return
	}
	for _, info := range infos {
		fmt.Fprintf(builder, "      --%-*s  %s\n", width, info.Name, info.Description)
	}
}

// flagWidth returns the longest flag name so a table aligns, or zero for an empty list.
func flagWidth(infos []flagInfo) int {
	width := 0
	for _, info := range infos {
		if len(info.Name) > width {
			width = len(info.Name)
		}
	}
	return width
}

// selectFlags returns the flags an action reads, in the declared order. A nil list shows every group
// flag, so a forgotten list never hides a flag; an empty list states that the action has no flags.
func selectFlags(all []flagInfo, reads []string) []flagInfo {
	if reads == nil {
		return all
	}
	byName := make(map[string]flagInfo, len(all))
	for _, info := range all {
		byName[info.Name] = info
	}
	chosen := make([]flagInfo, 0, len(reads))
	for _, name := range reads {
		if info, ok := byName[name]; ok {
			chosen = append(chosen, info)
		}
	}
	return chosen
}

// flagsOf lists the command-specific flags a command declares, without parsing them. The global
// flags are omitted; they are accepted everywhere and documented once.
func flagsOf(env *Env, command *Command) []flagInfo {
	if command.Flags == nil {
		return nil
	}
	infos := make([]flagInfo, 0)
	command.Flags(env).VisitAll(func(entry *flag.Flag) {
		if globalFlagNames[entry.Name] {
			return
		}
		infos = append(infos, flagInfo{Name: entry.Name, Description: entry.Usage})
	})
	return infos
}

// actionTopic describes one action of a group command.
func actionTopic(group *Command, action Action, all []flagInfo) commandInfo {
	return commandInfo{
		Name:     group.Name + " " + action.Name,
		Summary:  action.Summary,
		Usage:    "pecia " + group.Name + " " + action.Name + " [flags]",
		Mutating: group.Mutating,
		Flags:    selectFlags(all, action.Reads),
	}
}

// actionTopicName returns the longest action topic of a group that starts with the positionals.
func actionTopicName(command *Command, positionals []string) string {
	for count := len(positionals); count > 0; count-- {
		candidate := strings.Join(positionals[:count], " ")
		for _, action := range command.Actions {
			if action.Name == candidate {
				return command.Name + " " + candidate
			}
		}
	}
	return command.Name
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

func commandInfoOf(env *Env, command *Command) commandInfo {
	info := commandInfo{
		Name:     command.Name,
		Summary:  command.Summary,
		Usage:    command.Usage,
		Mutating: command.Mutating,
	}
	if len(command.Actions) == 0 {
		info.Flags = flagsOf(env, command)
		return info
	}
	all := flagsOf(env, command)
	for _, action := range command.Actions {
		info.Actions = append(info.Actions, actionInfo{
			Name:    action.Name,
			Summary: action.Summary,
			Usage:   "pecia " + command.Name + " " + action.Name + " [flags]",
			Flags:   selectFlags(all, action.Reads),
		})
	}
	return info
}

// helpListResult describes the whole command surface.
func helpListResult(registry *Registry) *Result {
	commands := registry.List()
	infos := make([]commandInfo, 0, len(commands))
	for _, command := range commands {
		infos = append(infos, commandInfo{
			Name: command.Name, Summary: command.Summary, Usage: command.Usage,
			Mutating: command.Mutating,
		})
	}
	return &Result{Data: helpList{
		Product:  buildinfo.Product,
		CLI:      buildinfo.CLI,
		Tagline:  tagline,
		Usage:    buildinfo.CLI + " <command> [flags]",
		Commands: infos,
	}}
}

// commandHelpResult describes one command or one action of a group command. The topic is whitespace
// separated: "badge" names the group, "badge init", "status level add" and "library defaults show"
// name an action. The longest registered group name wins, so a multi-word group resolves.
func commandHelpResult(env *Env, registry *Registry, topic string) (*Result, error) {
	if command, ok := registry.Get(topic); ok {
		return &Result{Data: commandInfoOf(env, command)}, nil
	}
	group := ""
	for index := strings.LastIndex(topic, " "); index > 0; index = strings.LastIndex(topic[:index], " ") {
		candidate, ok := registry.Get(topic[:index])
		if !ok {
			continue
		}
		group = topic[:index]
		actionName := topic[index+1:]
		for _, action := range candidate.Actions {
			if action.Name == actionName {
				return &Result{Data: actionTopic(candidate, action, flagsOf(env, candidate))}, nil
			}
		}
		break
	}
	if group != "" {
		return nil, Usage("usage.unknown-command", "unknown action %q of %q",
			topic[len(group)+1:], group).
			WithHint("run `pecia help " + group + "` for the action list")
	}
	return nil, Usage("usage.unknown-command", "unknown command %q", topic)
}

func registerBuiltins(registry *Registry) {
	registry.Add(&Command{
		Name:    "help",
		Summary: "list the commands or describe one",
		Usage:   "pecia help [command]",
		Run: func(env *Env, args []string) (*Result, error) {
			if len(args) > 0 {
				return commandHelpResult(env, registry, strings.Join(args, " "))
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
