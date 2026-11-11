package editorial

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/badge"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

type badgePayload struct {
	DryRun   bool   `json:"dryRun"`
	Book     string `json:"book"`
	Action   string `json:"action"`
	ID       string `json:"id,omitempty"`
	Label    string `json:"label,omitempty"`
	Message  string `json:"message,omitempty"`
	Link     string `json:"link,omitempty"`
	Style    string `json:"style,omitempty"`
	Logo     string `json:"logo,omitempty"`
	Required bool   `json:"required,omitempty"`
	Changed  bool   `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (result badgePayload) DryRunResult() bool { return result.DryRun }

func (result badgePayload) Text() string {
	change := "unchanged"
	if result.Changed {
		change = "changed"
	}
	line := fmt.Sprintf("%s badge %s: %s", result.Book, result.Action, change)
	if result.ID != "" {
		line += " " + result.ID
	}
	return line + "\n"
}

type badgeState struct {
	ID       string `json:"id"`
	Detector string `json:"detector,omitempty"`
	Label    string `json:"label,omitempty"`
	Message  string `json:"message,omitempty"`
	Link     string `json:"link,omitempty"`
	Style    string `json:"style,omitempty"`
	Logo     string `json:"logo,omitempty"`
	Color    string `json:"color,omitempty"`
	Alt      string `json:"alt,omitempty"`
	Required bool   `json:"required,omitempty"`
	Order    int    `json:"order,omitempty"`
	Group    string `json:"group,omitempty"`
	URL      string `json:"url,omitempty"`
	Resolved bool   `json:"resolved"`
	Value    string `json:"value,omitempty"`
	Source   string `json:"source,omitempty"`
	Dynamic  bool   `json:"dynamic,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type badgeView struct {
	DryRun bool         `json:"dryRun"`
	Book   string       `json:"book"`
	Badges []badgeState `json:"badges"`
}

// DryRunResult marks the payload as a dry-run receipt; a view writes nothing either way.
func (result badgeView) DryRunResult() bool { return true }

func badgeCommand() *cli.Command {
	var book, id, source, label, message, link, style, logo, logoColor, labelColor, color, alt, group string
	var preset string
	var order, cacheSeconds int
	var required, force bool
	var command *cli.Command
	command = &cli.Command{
		Name:    "badge",
		Summary: "declare, remove or inspect the repository badges of the library README",
		Actions: []cli.Action{
			{Name: "add", Summary: "declare a badge or update one declaration",
				Reads: []string{"book", "id", "source", "label", "message", "link", "style", "logo",
					"logo-color", "label-color", "color", "alt", "group", "order", "cache-seconds",
					"required"}},
			{Name: "remove", Summary: "remove one badge declaration",
				Reads: []string{"book", "id"}},
			{Name: "show", Summary: "list the badges, their detectors and their values",
				Reads: []string{"book", "id"}},
			{Name: "detect", Summary: "report the project kinds that match a preset",
				Reads: []string{}},
			{Name: "explain", Summary: "show the detector, precedence and hash of one badge",
				Reads: []string{"book", "id"}},
			{Name: "check", Summary: "report unresolved badges",
				Reads: []string{"book"}},
			{Name: "init", Summary: "write a preset's detectors and badge declarations",
				Reads: []string{"book", "preset", "force"}},
		},
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("badge")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&id, "id", "", "badge id or detector")
			set.StringVar(&source, "source", "", "detector id, when it differs from the badge id")
			set.StringVar(&label, "label", "", "badge label")
			set.StringVar(&message, "message", "", "explicit message that overrides the detector")
			set.StringVar(&link, "link", "", "link target of the badge")
			set.StringVar(&style, "style", "", "shields style")
			set.StringVar(&logo, "logo", "", "simple-icons slug")
			set.StringVar(&logoColor, "logo-color", "", "logo color")
			set.StringVar(&labelColor, "label-color", "", "label background color")
			set.StringVar(&color, "color", "", "message color")
			set.StringVar(&alt, "alt", "", "Markdown alt text")
			set.StringVar(&group, "group", "", "badge line group")
			set.IntVar(&order, "order", 0, "sort key within the badge line")
			set.IntVar(&cacheSeconds, "cache-seconds", 0, "cache lifetime of an external source")
			set.BoolVar(&required, "required", false, "fail the check when the badge stays unresolved")
			set.StringVar(&preset, "preset", "", "detector preset for badge init")
			set.BoolVar(&force, "force", false, "overwrite a conflicting detector")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 1 {
				return nil, cli.Usage("usage.invalid-arguments",
					"badge takes %s", command.ActionList()).
					WithHint("pecia badge add --book <id> --id go")
			}
			action := args[0]
			if action == "detect" {
				return badgeDetect(env)
			}
			bookID, bookErr := requireBook(book)
			if bookErr != nil {
				return nil, bookErr
			}
			switch action {
			case "init":
				return badgeInit(env, bookID, preset, force)
			case "check":
				return badgeCheck(env, bookID)
			case "add":
				if id == "" {
					return nil, cli.Usage("usage.missing-argument", "--id is required")
				}
				service, serviceErr := newService(env)
				if serviceErr != nil {
					return nil, serviceErr
				}
				declared := model.Badge{
					ID: id, Source: source, Label: label, Message: message, Link: link,
					Style: style, Logo: logo, LogoColor: logoColor, LabelColor: labelColor,
					Color: color, Alt: alt, Required: required, Order: order, Group: group,
					CacheSeconds: cacheSeconds,
				}
				receipt, err := service.SetBadge(bookID, declared, env.Globals.DryRun)
				if err != nil {
					return nil, cli.AsError(err)
				}
				return &cli.Result{Data: badgePayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					ID: id, Label: label, Message: message, Link: link, Style: style, Logo: logo,
					Required: required, Changed: receipt.Changed,
				}}, nil
			case "remove":
				if id == "" {
					return nil, cli.Usage("usage.missing-argument", "--id is required")
				}
				if source != "" || label != "" || message != "" || link != "" || style != "" ||
					logo != "" || logoColor != "" || labelColor != "" || color != "" || alt != "" ||
					group != "" || order != 0 || cacheSeconds != 0 || required {
					return nil, cli.Usage("usage.invalid-arguments", "remove takes --book and --id only")
				}
				service, serviceErr := newService(env)
				if serviceErr != nil {
					return nil, serviceErr
				}
				receipt, err := service.RemoveBadge(bookID, id, env.Globals.DryRun)
				if err != nil {
					return nil, cli.AsError(err)
				}
				return &cli.Result{Data: badgePayload{
					DryRun: env.Globals.DryRun, Book: string(bookID), Action: action,
					ID: id, Changed: receipt.Changed,
				}}, nil
			case "show":
				if id != "" || source != "" || label != "" || message != "" || link != "" ||
					style != "" || logo != "" || logoColor != "" || labelColor != "" || color != "" ||
					alt != "" || group != "" || order != 0 || cacheSeconds != 0 || required {
					return nil, cli.Usage("usage.invalid-arguments", "show takes --book only")
				}
				return badgeShow(env, bookID)
			case "explain":
				if id == "" {
					return nil, cli.Usage("usage.missing-argument", "--id is required")
				}
				return badgeExplain(env, bookID, id)
			default:
				return nil, cli.Usage("usage.invalid-arguments",
					"unknown badge action %q; use %s", action, command.ActionList())
			}
		},
	}
	return command
}

// badgeShow lists the declared badges with their resolution.
func badgeShow(env *cli.Env, bookID model.BookID) (*cli.Result, error) {
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	declared, err := service.Badges(bookID)
	if err != nil {
		return nil, cli.AsError(err)
	}
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	resolver := badge.Resolver{ProjectDir: values.ProjectDir, Detectors: values.BadgeDetectors}
	view := badgeView{
		DryRun: env.Globals.DryRun, Book: string(bookID),
		Badges: make([]badgeState, 0, len(declared)),
	}
	for _, item := range declared {
		view.Badges = append(view.Badges, badgeStateOf(resolver, item))
	}
	return &cli.Result{Data: view}, nil
}

// badgeStateOf resolves one declared badge into its report form.
func badgeStateOf(resolver badge.Resolver, item model.Badge) badgeState {
	state := badgeState{
		ID: item.ID, Detector: item.Source, Label: item.Label, Message: item.Message,
		Link: item.Link, Style: item.Style, Logo: item.Logo, Color: item.Color, Alt: item.Alt,
		Required: item.Required, Order: item.Order, Group: item.Group,
	}
	if state.Detector == "" {
		state.Detector = item.ID
	}
	resolved, ok, reason := resolver.Resolve(item)
	if !ok {
		state.Reason = reason
		return state
	}
	state.Resolved = true
	state.Value = resolved.Message
	state.Source = resolved.Source
	state.Dynamic = resolved.Dynamic
	state.URL = badge.BadgeURL(resolved)
	if state.Label == "" {
		state.Label = resolved.Label
	}
	if state.Link == "" {
		state.Link = resolved.Link
	}
	if state.Style == "" {
		state.Style = resolved.Style
	}
	if state.Logo == "" {
		state.Logo = resolved.Logo
	}
	return state
}

// badgeExplain reports the detector, source, value and hash participation of one badge.
func badgeExplain(env *cli.Env, bookID model.BookID, id string) (*cli.Result, error) {
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	declared, err := service.Badges(bookID)
	if err != nil {
		return nil, cli.AsError(err)
	}
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	resolver := badge.Resolver{ProjectDir: values.ProjectDir, Detectors: values.BadgeDetectors}
	for _, item := range declared {
		if item.ID != id {
			continue
		}
		state := badgeStateOf(resolver, item)
		payload := badgeExplainPayload{
			DryRun: env.Globals.DryRun, Book: string(bookID), Badge: state,
			Detector: badge.Detector{},
		}
		if detector, ok := values.BadgeDetectors[state.Detector]; ok {
			payload.Detector = detector
		}
		switch {
		case !state.Resolved:
			payload.Precedence = "unresolved"
			payload.Hash = "unresolved"
		case state.Source == "explicit":
			payload.Precedence = "explicit message"
			payload.Hash = "value"
		case state.Dynamic:
			payload.Precedence = "detector"
			payload.Hash = "url"
		default:
			payload.Precedence = "detector"
			payload.Hash = "value"
		}
		return &cli.Result{Data: payload}, nil
	}
	return nil, cli.Reject("badge.unknown", "the book declares no badge %q", id)
}

type badgeExplainPayload struct {
	DryRun     bool           `json:"dryRun"`
	Book       string         `json:"book"`
	Badge      badgeState     `json:"badge"`
	Detector   badge.Detector `json:"detector"`
	Precedence string         `json:"precedence"`
	Hash       string         `json:"hash"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload badgeExplainPayload) DryRunResult() bool { return payload.DryRun }

func (payload badgeExplainPayload) Text() string {
	line := fmt.Sprintf("%s: %s, detector %s (%s), precedence %s, hash %s",
		payload.Badge.ID, payload.Badge.Value, payload.Badge.Detector,
		payload.Detector.Kind, payload.Precedence, payload.Hash)
	if payload.Badge.URL != "" {
		line += "\nurl: " + payload.Badge.URL
	}
	if payload.Badge.Reason != "" {
		line += "\nreason: " + payload.Badge.Reason
	}
	return line + "\n"
}

type badgeDetectPayload struct {
	DryRun     bool              `json:"dryRun"`
	ProjectDir string            `json:"projectDir"`
	Detections []badge.Detection `json:"detections"`
	Suggested  string            `json:"suggested,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt; detect writes nothing.
func (payload badgeDetectPayload) DryRunResult() bool { return true }

func (payload badgeDetectPayload) Text() string {
	line := ""
	for _, detection := range payload.Detections {
		line += fmt.Sprintf("%s (%s) -> preset %s\n", detection.Kind, detection.Evidence,
			detection.Preset)
	}
	if line == "" {
		return "no project kind detected\n"
	}
	if payload.Suggested != "" {
		line += "suggested preset: " + payload.Suggested + "\n"
	}
	return line
}

// badgeDetect reports the applicable project kinds; it never picks or writes a preset.
func badgeDetect(env *cli.Env) (*cli.Result, error) {
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	detections := badge.Detect(values.ProjectDir)
	suggested := ""
	if len(detections) == 1 {
		suggested = detections[0].Preset
	}
	return &cli.Result{Data: badgeDetectPayload{
		DryRun: true, ProjectDir: values.ProjectDir, Detections: detections, Suggested: suggested,
	}}, nil
}

type badgeInitPayload struct {
	DryRun    bool     `json:"dryRun"`
	Book      string   `json:"book"`
	Preset    string   `json:"preset"`
	Detectors []string `json:"detectors,omitempty"`
	Badges    []string `json:"badges,omitempty"`
	Changed   bool     `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload badgeInitPayload) DryRunResult() bool { return payload.DryRun }

func (payload badgeInitPayload) Text() string {
	return fmt.Sprintf("%s: preset %s, %d detector(s), %d badge(s)\n",
		payload.Book, payload.Preset, len(payload.Detectors), len(payload.Badges))
}

// badgeInit writes a preset's detectors and badge declarations. The config block is written first;
// a later book failure leaves the (harmless) detectors in place.
func badgeInit(env *cli.Env, bookID model.BookID, name string, force bool) (*cli.Result, error) {
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	preset, ok := badge.Preset(name)
	if !ok {
		if strings.TrimSpace(name) == "" {
			hint := "pick one of: " + strings.Join(badge.PresetNames(), ", ")
			if detections := badge.Detect(values.ProjectDir); len(detections) > 0 {
				hint = "detected " + detections[0].Kind + "; run with --preset " +
					detections[0].Preset
			}
			return nil, cli.Usage("usage.missing-argument", "--preset is required").WithHint(hint)
		}
		return nil, cli.Usage("usage.invalid-arguments",
			"unknown preset %q; use %s", name, strings.Join(badge.PresetNames(), ", "))
	}
	if conflicts := badge.DetectorConflicts(preset, values.BadgeDetectors); len(conflicts) > 0 &&
		!force {
		return nil, cli.Reject("badge.detector-exists",
			"the preset would overwrite the configured detector(s) %s",
			strings.Join(conflicts, ", ")).
			WithHint("repeat with --force to overwrite them")
	}
	ids := make([]string, 0, len(preset.Detectors))
	for id := range preset.Detectors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, putErr := config.PutDetector(env, id, preset.Detectors[id],
			env.Globals.DryRun); putErr != nil {
			return nil, putErr
		}
	}
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	payload := badgeInitPayload{
		DryRun: env.Globals.DryRun, Book: string(bookID), Preset: preset.Name, Detectors: ids,
	}
	for _, declared := range preset.Badges {
		receipt, setErr := service.SetBadge(bookID, declared, env.Globals.DryRun)
		if setErr != nil {
			return nil, cli.AsError(setErr)
		}
		payload.Badges = append(payload.Badges, declared.ID)
		payload.Changed = payload.Changed || receipt.Changed
	}
	return &cli.Result{Data: payload}, nil
}

type badgeCheckPayload struct {
	DryRun     bool               `json:"dryRun"`
	Book       string             `json:"book"`
	Checked    int                `json:"checked"`
	Resolved   []badgeState       `json:"resolved,omitempty"`
	Unresolved []badge.Unresolved `json:"unresolved,omitempty"`
	Changed    bool               `json:"changed"`
}

// DryRunResult marks the payload as a dry-run receipt; check writes only the temp cache.
func (payload badgeCheckPayload) DryRunResult() bool { return true }

func (payload badgeCheckPayload) Text() string {
	line := ""
	for _, state := range payload.Resolved {
		line += fmt.Sprintf("%s: %s (%s)\n", state.ID, state.Value, state.Source)
	}
	for _, item := range payload.Unresolved {
		line += fmt.Sprintf("%s: unresolved: %s\n", item.ID, item.Reason)
	}
	line += fmt.Sprintf("%d badge(s) checked, %d unresolved\n", payload.Checked,
		len(payload.Unresolved))
	return line
}

// badgeCheck evaluates the external sources once and fills the cache. Render and sync never call it.
func badgeCheck(env *cli.Env, bookID model.BookID) (*cli.Result, error) {
	service, serviceErr := newService(env)
	if serviceErr != nil {
		return nil, serviceErr
	}
	declared, err := service.Badges(bookID)
	if err != nil {
		return nil, cli.AsError(err)
	}
	values, configErr := config.Load(env)
	if configErr != nil {
		return nil, configErr
	}
	resolver := badge.Resolver{
		ProjectDir: values.ProjectDir, Detectors: values.BadgeDetectors,
		CacheDir: values.TempRoot, AllowExternal: true,
	}
	resolved, unresolved := resolver.ResolveAll(declared)
	payload := badgeCheckPayload{
		DryRun: env.Globals.DryRun, Book: string(bookID), Checked: len(declared),
		Unresolved: unresolved,
	}
	for _, value := range resolved {
		payload.Resolved = append(payload.Resolved, badgeState{
			ID: value.ID, Detector: value.Detector, Label: value.Label, Value: value.Message,
			Source: value.Source, Dynamic: value.Dynamic, URL: badge.BadgeURL(value),
			Resolved: true,
		})
	}
	return &cli.Result{Data: payload}, nil
}
