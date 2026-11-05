// Package apply executes declarative batch plans against an open book.
//
// A plan is a versioned JSON document. It runs under one book lock with one validation, one undo
// package and one manifest write, so a failing step leaves the book untouched.
package apply

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/jsonstrict"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
)

// PlanVersion is the supported plan schema version.
const PlanVersion = 1

// Error is a plan failure with a stable code.
type Error struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *Error) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *Error) ErrorCode() string { return err.Code }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

type rawStep struct {
	Op        string `json:"op"`
	Title     string `json:"title,omitempty"`
	Type      string `json:"type,omitempty"`
	Target    string `json:"target,omitempty"`
	Parent    string `json:"parent,omitempty"`
	ChildOf   string `json:"childOf,omitempty"`
	Index     *int   `json:"index,omitempty"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value,omitempty"`
}

type planFile struct {
	Version int       `json:"version"`
	Steps   []rawStep `json:"steps"`
}

// Parse validates a plan document and converts it into service steps.
func Parse(data []byte) ([]structure.Step, error) {
	var file planFile
	if err := jsonstrict.Decode(data, &file, jsonstrict.Options{DisallowUnknownFields: true}); err != nil {
		return nil, fail("apply.invalid-plan", "%v", err)
	}
	if file.Version != PlanVersion {
		return nil, fail("apply.version",
			"unsupported plan version %d (expected %d)", file.Version, PlanVersion)
	}
	if len(file.Steps) == 0 {
		return nil, fail("apply.empty-plan", "the plan has no steps")
	}
	steps := make([]structure.Step, 0, len(file.Steps))
	for index, raw := range file.Steps {
		step, err := parseStep(index, raw)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func parseStep(index int, raw rawStep) (structure.Step, error) {
	where := fmt.Sprintf("steps[%d]", index)
	switch raw.Op {
	case "add":
		placement, err := placementOf(raw, true)
		if err != nil {
			return structure.Step{}, located(where, err)
		}
		return structure.Step{Op: "add", Title: raw.Title, Type: raw.Type, Placement: placement}, nil
	case "rename":
		target, err := idOf(raw.Target, where+".target")
		if err != nil {
			return structure.Step{}, err
		}
		if strings.TrimSpace(raw.Title) == "" {
			return structure.Step{}, fail("apply.invalid-step", "%s.title must not be empty", where)
		}
		return structure.Step{Op: "rename", Target: target, Title: raw.Title}, nil
	case "move":
		target, err := idOf(raw.Target, where+".target")
		if err != nil {
			return structure.Step{}, err
		}
		placement, err := placementOf(raw, false)
		if err != nil {
			return structure.Step{}, located(where, err)
		}
		return structure.Step{Op: "move", Target: target, Placement: placement}, nil
	case "remove":
		target, err := idOf(raw.Target, where+".target")
		if err != nil {
			return structure.Step{}, err
		}
		return structure.Step{Op: "remove", Target: target, Recursive: raw.Recursive}, nil
	case "type":
		target, err := idOf(raw.Target, where+".target")
		if err != nil {
			return structure.Step{}, err
		}
		return structure.Step{Op: "type", Target: target, Value: raw.Type}, nil
	case "setting":
		if strings.TrimSpace(raw.Key) == "" || strings.TrimSpace(raw.Value) == "" {
			return structure.Step{}, fail("apply.invalid-step", "%s needs key and value", where)
		}
		return structure.Step{Op: "setting", Key: raw.Key, Value: raw.Value}, nil
	default:
		return structure.Step{}, fail("apply.invalid-step", "%s.op: unknown operation %q", where, raw.Op)
	}
}

func placementOf(raw rawStep, allowEmpty bool) (structure.Placement, error) {
	forms := 0
	placement := structure.Placement{Index: -1}
	if raw.Before != "" && raw.After != "" {
		return structure.Placement{}, fail("apply.invalid-step", "use before or after, not both")
	}
	if raw.Before != "" {
		id, err := model.ParseID(raw.Before)
		if err != nil || id.IsZero() {
			return structure.Placement{}, fail("apply.invalid-step", "before: invalid id %q", raw.Before)
		}
		placement.Before = id
		forms++
	}
	if raw.After != "" {
		id, err := model.ParseID(raw.After)
		if err != nil || id.IsZero() {
			return structure.Placement{}, fail("apply.invalid-step", "after: invalid id %q", raw.After)
		}
		placement.After = id
		forms++
	}
	if raw.Parent != "" || raw.ChildOf != "" || raw.Index != nil {
		if raw.Parent != "" && raw.ChildOf != "" {
			return structure.Placement{}, fail("apply.invalid-step", "use parent or childOf, not both")
		}
		parent := raw.Parent
		if raw.ChildOf != "" {
			parent = raw.ChildOf
		}
		if parent == "root" {
			placement.Parent = model.ZeroID
		} else if parent != "" {
			id, err := model.ParseID(parent)
			if err != nil || id.IsZero() {
				return structure.Placement{}, fail("apply.invalid-step", "parent: invalid id %q", parent)
			}
			placement.Parent = id
		}
		if raw.Index != nil {
			if *raw.Index < -1 {
				return structure.Placement{}, fail("apply.invalid-step", "index must be -1 or greater")
			}
			placement.Index = *raw.Index
		}
		forms++
	}
	if forms > 1 {
		return structure.Placement{}, fail("apply.invalid-step",
			"use one placement form: before, after or parent/childOf with index")
	}
	if forms == 0 {
		if !allowEmpty {
			return structure.Placement{}, fail("apply.invalid-step", "a destination is required")
		}
		return structure.Placement{Index: -1}, nil
	}
	return placement, nil
}

func idOf(raw, where string) (model.ID, error) {
	id, err := model.ParseID(raw)
	if err != nil || id.IsZero() {
		return model.ZeroID, fail("apply.invalid-step", "%s: invalid id %q", where, raw)
	}
	return id, nil
}

func located(where string, err error) error {
	var applyErr *Error
	if errors.As(err, &applyErr) && applyErr != nil {
		return &Error{Code: applyErr.Code, Message: where + ": " + applyErr.Message}
	}
	return &Error{Code: "apply.invalid-step", Message: where + ": " + err.Error()}
}

// Register adds the apply command.
func Register(registry *cli.Registry) {
	registry.Add(applyCommand())
}

type receiptPayload struct {
	Target  string   `json:"target,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Parent  string   `json:"parent,omitempty"`
	Index   *int     `json:"index,omitempty"`
	Order   []string `json:"order,omitempty"`
	Changed bool     `json:"changed"`
}

type payload struct {
	DryRun   bool             `json:"dryRun"`
	Book     string           `json:"book"`
	Steps    int              `json:"steps"`
	Receipts []receiptPayload `json:"receipts"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload payload) DryRunResult() bool { return payload.DryRun }

func (payload payload) Text() string {
	if payload.DryRun {
		return fmt.Sprintf("dry run: %d step(s) for %s", payload.Steps, payload.Book)
	}
	return fmt.Sprintf("%s: applied %d step(s)", payload.Book, payload.Steps)
}

func applyCommand() *cli.Command {
	var book, file string
	return &cli.Command{
		Name:     "apply",
		Summary:  "apply a declarative plan",
		Usage:    "pecia apply --book <id> --file <path|->",
		Mutating: true,
		Flags: func(env *cli.Env) *flag.FlagSet {
			set := env.NewFlagSet("apply")
			set.StringVar(&book, "book", "", "book id")
			set.StringVar(&file, "file", "", "plan file, or - for stdin")
			return set
		},
		Run: func(env *cli.Env, args []string) (*cli.Result, error) {
			if len(args) != 0 {
				return nil, cli.Usage("usage.unexpected-argument", "apply takes flags only")
			}
			if book == "" {
				return nil, cli.Usage("usage.missing-argument", "--book is required").
					WithHint("pecia apply --book <id> --file <path>")
			}
			if file == "" {
				return nil, cli.Usage("usage.missing-argument", "--file is required").
					WithHint("pecia apply --book <id> --file <path|->")
			}
			bookID := model.BookID(book)
			if !bookID.Valid() {
				return nil, cli.Usage("usage.invalid-value", "invalid book id %q", book)
			}
			data, readErr := readPlan(file)
			if readErr != nil {
				return nil, readErr
			}
			steps, parseErr := Parse(data)
			if parseErr != nil {
				return nil, cli.AsError(parseErr)
			}
			life, _, envErr := lifecycle.FromEnv(env)
			if envErr != nil {
				return nil, envErr
			}
			service := structure.New(life, model.NewRandomAllocator())
			receipts, applyErr := service.Execute(bookID, steps, env.Globals.DryRun)
			if applyErr != nil {
				return nil, structure.MapError(applyErr)
			}
			return &cli.Result{Data: payloadOf(bookID, steps, receipts, env.Globals.DryRun)}, nil
		},
	}
}

func payloadOf(book model.BookID, steps []structure.Step, receipts []structure.Receipt, dryRun bool) payload {
	result := payload{DryRun: dryRun, Book: string(book), Steps: len(steps)}
	result.Receipts = make([]receiptPayload, 0, len(receipts))
	for _, receipt := range receipts {
		item := receiptPayload{Kind: receipt.Kind, Changed: receipt.Changed}
		if !receipt.Target.IsZero() {
			item.Target = receipt.Target.String()
		}
		if !receipt.Parent.IsZero() {
			item.Parent = receipt.Parent.String()
		}
		if receipt.Index >= 0 {
			index := receipt.Index
			item.Index = &index
		}
		if len(receipt.Order) > 0 {
			item.Order = make([]string, 0, len(receipt.Order))
			for _, id := range receipt.Order {
				item.Order = append(item.Order, id.String())
			}
		}
		result.Receipts = append(result.Receipts, item)
	}
	return result
}

func readPlan(path string) ([]byte, *cli.Error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, cli.Fail("apply.read-failed", "%v", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, cli.Fail("apply.read-failed", "%v", err)
	}
	return data, nil
}
