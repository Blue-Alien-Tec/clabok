package lifecycle

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/buildinfo"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/rollback"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

type validationPayload struct {
	Issues []model.Issue `json:"issues"`
}

func (payload validationPayload) Text() string {
	var builder strings.Builder
	for _, issue := range payload.Issues {
		builder.WriteString(issue.String() + "\n")
	}
	return builder.String()
}

type openPayload struct {
	DryRun     bool   `json:"dryRun"`
	ID         string `json:"id"`
	Status     string `json:"status,omitempty"`
	OutputPath string `json:"outputPath,omitempty"`
	Session    string `json:"session,omitempty"`
	PID        int    `json:"pid,omitempty"`
	OpenedAt   string `json:"openedAt,omitempty"`
	Recovered  bool   `json:"recovered,omitempty"`
	Action     string `json:"action,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload openPayload) DryRunResult() bool { return payload.DryRun }

func (payload openPayload) Text() string {
	if payload.DryRun {
		return fmt.Sprintf("dry run: %s (%s)", payload.Action, payload.ID)
	}
	note := ""
	if payload.Recovered {
		note = " (recovered)"
	}
	return fmt.Sprintf("%s: open%s", payload.ID, note)
}

type closePayload struct {
	DryRun       bool   `json:"dryRun"`
	ID           string `json:"id"`
	Repacked     bool   `json:"repacked,omitempty"`
	EntryRemoved bool   `json:"entryRemoved,omitempty"`
	Recovered    bool   `json:"recovered,omitempty"`
	BookPoint    bool   `json:"bookPoint,omitempty"`
	Action       string `json:"action,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload closePayload) DryRunResult() bool { return payload.DryRun }

func (payload closePayload) Text() string {
	if payload.DryRun {
		return fmt.Sprintf("dry run: %s (%s)", payload.Action, payload.ID)
	}
	if !payload.Repacked && !payload.EntryRemoved {
		return payload.ID + ": closed (no change)"
	}
	note := ""
	if payload.Recovered {
		note = " (recovered)"
	}
	return payload.ID + ": closed" + note
}

type statusPayload struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Open       bool   `json:"open"`
	Extracted  bool   `json:"extracted"`
	OutputPath string `json:"outputPath,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Session    string `json:"session,omitempty"`
	OpenedAt   string `json:"openedAt,omitempty"`
}

func (payload statusPayload) Text() string {
	if payload.OutputPath != "" {
		return fmt.Sprintf("%s: %s (%s)", payload.ID, payload.Status, payload.OutputPath)
	}
	return fmt.Sprintf("%s: %s", payload.ID, payload.Status)
}

// Register adds the book lifecycle commands.
func Register(registry *cli.Registry) {
	registry.Add(&cli.Command{
		Name:     "book open",
		Summary:  "extract a book for authoring",
		Usage:    "pecia book open <book-id>",
		Mutating: true,
		Run:      runOpen,
	})
	registry.Add(&cli.Command{
		Name:     "book close",
		Summary:  "repack an open book",
		Usage:    "pecia book close <book-id>",
		Mutating: true,
		Run:      runClose,
	})
	registry.Add(&cli.Command{
		Name:    "book status",
		Summary: "show the open state of a book",
		Usage:   "pecia book status <book-id>",
		Run:     runStatus,
	})
	registry.Add(&cli.Command{
		Name:    "rollback list",
		Summary: "list the rollback packages of a book",
		Usage:   "pecia rollback list <book-id>",
		Run:     runRollbackList,
	})
	registry.Add(&cli.Command{
		Name:     "rollback restore",
		Summary:  "restore a rollback package",
		Usage:    "pecia rollback restore <book-id> <package>",
		Mutating: true,
		Run:      runRollbackRestore,
	})
}

func runOpen(env *cli.Env, args []string) (*cli.Result, error) {
	id, argErr := bookID(args, "book open")
	if argErr != nil {
		return nil, argErr
	}
	life, layout, err := newLifecycle(env)
	if err != nil {
		return nil, err
	}
	outputPath, pathErr := registryPath(layout, id)
	if pathErr != nil {
		return nil, pathErr
	}
	if env.Globals.DryRun {
		state, stateErr := life.State(id)
		if stateErr != nil {
			return nil, cli.AsError(stateErr)
		}
		return &cli.Result{Data: openPayload{
			DryRun: true,
			ID:     string(id),
			Status: string(state.Status),
			Action: openAction(state.Status),
		}}, nil
	}
	result, openErr := life.Open(id, outputPath)
	if openErr != nil {
		return nil, cli.AsError(openErr)
	}
	return &cli.Result{Data: openPayload{
		ID:         string(result.Entry.ID),
		Status:     string(StatusOpen),
		OutputPath: result.Entry.OutputPath,
		Session:    result.Entry.Session,
		PID:        result.Entry.PID,
		OpenedAt:   result.Entry.OpenedAt,
		Recovered:  result.Recovered,
	}}, nil
}

func runClose(env *cli.Env, args []string) (*cli.Result, error) {
	id, argErr := bookID(args, "book close")
	if argErr != nil {
		return nil, argErr
	}
	life, _, err := newLifecycle(env)
	if err != nil {
		return nil, err
	}
	if env.Globals.DryRun {
		state, stateErr := life.State(id)
		if stateErr != nil {
			return nil, cli.AsError(stateErr)
		}
		return &cli.Result{Data: closePayload{
			DryRun: true,
			ID:     string(id),
			Action: closeAction(state.Status),
		}}, nil
	}
	result, closeErr := life.Close(id)
	if closeErr != nil {
		var validation *model.ValidationError
		if errors.As(closeErr, &validation) {
			return nil, cli.Reject("book.invalid", "the book does not validate; nothing was written").
				WithHint("fix the reported issues and close again").
				WithDetails(validationPayload{Issues: validation.Issues})
		}
		return nil, cli.AsError(closeErr)
	}
	return &cli.Result{Data: closePayload{
		ID:           string(id),
		Repacked:     result.Repacked,
		EntryRemoved: result.EntryRemoved,
		Recovered:    result.Recovered,
		BookPoint:    result.BookPoint,
	}}, nil
}

func runStatus(env *cli.Env, args []string) (*cli.Result, error) {
	id, argErr := bookID(args, "book status")
	if argErr != nil {
		return nil, argErr
	}
	life, _, err := newLifecycle(env)
	if err != nil {
		return nil, err
	}
	state, stateErr := life.State(id)
	if stateErr != nil {
		return nil, cli.AsError(stateErr)
	}
	return &cli.Result{Data: statusPayload{
		ID:         string(state.ID),
		Status:     string(state.Status),
		Open:       state.Status == StatusOpen,
		Extracted:  state.Extracted,
		OutputPath: state.Entry.OutputPath,
		PID:        state.Entry.PID,
		Session:    state.Entry.Session,
		OpenedAt:   state.Entry.OpenedAt,
	}}, nil
}

func newLifecycle(env *cli.Env) (*Lifecycle, store.Layout, *cli.Error) {
	values, err := config.Load(env)
	if err != nil {
		return nil, store.Layout{}, err
	}
	layout, layoutErr := store.NewLayout(values.StoreRoot)
	if layoutErr != nil {
		return nil, store.Layout{}, cli.AsError(layoutErr)
	}
	var locks coordinator.Coordinator
	if values.ConcurrencyMode == "daemon" {
		socket := values.DaemonSocket
		if socket == "" {
			socket = coordinator.DefaultSocket(values.StoreRoot)
		}
		daemon, daemonErr := coordinator.NewDaemon(socket, buildinfo.Version)
		if daemonErr != nil {
			return nil, store.Layout{}, cli.AsError(daemonErr)
		}
		daemon.Command = "book"
		locks = daemon
	} else {
		fileLocks, fileErr := coordinator.NewFile(layout.Coordinator())
		if fileErr != nil {
			return nil, store.Layout{}, cli.AsError(fileErr)
		}
		fileLocks.Command = "book"
		locks = fileLocks
	}
	return New(layout, locks, Options{
		UndoLimit:      values.UndoLimit,
		BookPointLimit: values.BookPointLimit,
	}), layout, nil
}

func bookID(args []string, command string) (model.BookID, *cli.Error) {
	if len(args) != 1 {
		return "", cli.Usage("usage.invalid-arguments", "%s takes exactly one book id", command).
			WithHint("pecia " + command + " <book-id>")
	}
	id := model.BookID(args[0])
	if !id.Valid() {
		return "", cli.Usage("usage.invalid-value", "invalid book id %q", args[0]).
			WithHint("book ids contain no path separators or spaces")
	}
	return id, nil
}

func registryPath(layout store.Layout, id model.BookID) (string, *cli.Error) {
	registry, err := store.LoadRegistry(layout.Registry())
	if err != nil {
		return "", cli.AsError(err)
	}
	if entry, ok := registry.Find(id); ok {
		return entry.Path, nil
	}
	return "", nil
}

func openAction(status Status) string {
	switch status {
	case StatusOpen:
		return "no change"
	case StatusUnsaved:
		return "adopt the extracted state and mark the book open"
	case StatusOrphaned:
		return "re-extract the container and mark the book open"
	default:
		return "extract the container and mark the book open"
	}
}

func closeAction(status Status) string {
	switch status {
	case StatusOpen:
		return "repack the extracted state and remove the open entry"
	case StatusUnsaved:
		return "repack without an open entry"
	case StatusOrphaned:
		return "remove the stale open entry"
	default:
		return "no change"
	}
}

type packageInfo struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Reason  string `json:"reason"`
	Created string `json:"created"`
	Size    int64  `json:"size"`
}

type rollbackListPayload struct {
	Book     string        `json:"book"`
	Packages []packageInfo `json:"packages"`
}

func (payload rollbackListPayload) Text() string {
	if len(payload.Packages) == 0 {
		return payload.Book + ": no rollback packages"
	}
	var builder strings.Builder
	for _, pkg := range payload.Packages {
		fmt.Fprintf(&builder, "%s  %s  %s  %d bytes\n", pkg.Name, pkg.Kind, pkg.Reason, pkg.Size)
	}
	return builder.String()
}

type rollbackRestorePayload struct {
	DryRun  bool   `json:"dryRun"`
	Book    string `json:"book"`
	Package string `json:"package"`
	Kind    string `json:"kind,omitempty"`
	Members int    `json:"members,omitempty"`
	Action  string `json:"action,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (payload rollbackRestorePayload) DryRunResult() bool { return payload.DryRun }

func (payload rollbackRestorePayload) Text() string {
	if payload.DryRun {
		return fmt.Sprintf("dry run: %s (%s in %s)", payload.Action, payload.Package, payload.Book)
	}
	return fmt.Sprintf("%s: restored %s (%d members)", payload.Book, payload.Package, payload.Members)
}

func runRollbackList(env *cli.Env, args []string) (*cli.Result, error) {
	id, argErr := bookID(args, "rollback list")
	if argErr != nil {
		return nil, argErr
	}
	life, _, err := newLifecycle(env)
	if err != nil {
		return nil, err
	}
	rollbacks, rollbackErr := life.RollbackStore(id)
	if rollbackErr != nil {
		return nil, cli.AsError(rollbackErr)
	}
	packages, listErr := rollbacks.List()
	if listErr != nil {
		return nil, cli.AsError(listErr)
	}
	infos := make([]packageInfo, 0, len(packages))
	for index := len(packages) - 1; index >= 0; index-- {
		pkg := packages[index]
		infos = append(infos, packageInfo{
			Name:    pkg.Name,
			Kind:    string(pkg.Kind),
			Reason:  pkg.Reason,
			Created: pkg.Created.UTC().Format(time.RFC3339),
			Size:    pkg.Size,
		})
	}
	return &cli.Result{Data: rollbackListPayload{Book: string(id), Packages: infos}}, nil
}

func runRollbackRestore(env *cli.Env, args []string) (*cli.Result, error) {
	if len(args) != 2 {
		return nil, cli.Usage("usage.invalid-arguments", "rollback restore takes a book id and a package name").
			WithHint("pecia rollback restore <book-id> <package>")
	}
	id, argErr := bookID(args[:1], "rollback restore")
	if argErr != nil {
		return nil, argErr
	}
	name := args[1]
	life, _, err := newLifecycle(env)
	if err != nil {
		return nil, err
	}
	if env.Globals.DryRun {
		rollbacks, rollbackErr := life.RollbackStore(id)
		if rollbackErr != nil {
			return nil, cli.AsError(rollbackErr)
		}
		pkg, _, loadErr := rollbacks.Load(name)
		if loadErr != nil {
			return nil, cli.AsError(loadErr)
		}
		action := "restore the snapshot into the extracted state"
		if pkg.Kind == rollback.KindBook {
			action = "restore the container"
		}
		return &cli.Result{Data: rollbackRestorePayload{
			DryRun:  true,
			Book:    string(id),
			Package: name,
			Kind:    string(pkg.Kind),
			Action:  action,
		}}, nil
	}
	result, restoreErr := life.Restore(id, name)
	if restoreErr != nil {
		return nil, cli.AsError(restoreErr)
	}
	return &cli.Result{Data: rollbackRestorePayload{
		Book:    string(id),
		Package: name,
		Kind:    string(result.Package.Kind),
		Members: result.Members,
	}}, nil
}
