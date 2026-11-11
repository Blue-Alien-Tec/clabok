package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/idspace"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/rollback"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Error is a migration failure with a stable code.
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

// Target lists the current format versions.
type Target struct {
	Manifest  int `json:"manifest"`
	Registry  int `json:"registry"`
	Ledger    int `json:"ledger"`
	Store     int `json:"store"`
	Container int `json:"container"`
	OpenState int `json:"openState"`
	Config    int `json:"config"`
}

// ManifestChange is the manifest part of one book status.
type ManifestChange struct {
	From      int      `json:"from"`
	To        int      `json:"to"`
	Steps     []string `json:"steps"`
	Supported bool     `json:"supported"`
}

// BookStatus is one book in a report.
type BookStatus struct {
	ID       model.BookID   `json:"id"`
	Open     bool           `json:"open"`
	State    string         `json:"state"`
	Manifest ManifestChange `json:"manifest"`
	Derived  bool           `json:"derived"`
	Backup   string         `json:"backup,omitempty"`
	Rollback string         `json:"rollback,omitempty"`
	Reset    bool           `json:"reset,omitempty"`
	Changed  bool           `json:"changed"`
}

// RegistryStatus is the registry part of a report.
type RegistryStatus struct {
	From    int      `json:"from"`
	To      int      `json:"to"`
	Steps   []string `json:"steps,omitempty"`
	Backup  string   `json:"backup,omitempty"`
	Changed bool     `json:"changed"`
	Missing bool     `json:"missing,omitempty"`
}

// LedgerStatus is the ID ledger part of a report.
type LedgerStatus struct {
	From    int  `json:"from"`
	To      int  `json:"to"`
	Changed bool `json:"changed"`
}

// StoreStatus is the store marker part of a report.
type StoreStatus struct {
	From    int      `json:"from"`
	To      int      `json:"to"`
	Steps   []string `json:"steps,omitempty"`
	Backup  string   `json:"backup,omitempty"`
	Changed bool     `json:"changed"`
}

// ContainerStatus is the container format part of a report.
type ContainerStatus struct {
	From    int      `json:"from"`
	To      int      `json:"to"`
	Steps   []string `json:"steps,omitempty"`
	Changed bool     `json:"changed"`
}

// OpenStateStatus is the transient open-state version part of a report.
type OpenStateStatus struct {
	From    int      `json:"from"`
	To      int      `json:"to"`
	Steps   []string `json:"steps,omitempty"`
	Changed bool     `json:"changed"`
}

// ConfigStatus reports the project configuration version.
type ConfigStatus struct {
	Version   int      `json:"version"`
	Supported bool     `json:"supported"`
	Steps     []string `json:"steps,omitempty"`
	Changed   bool     `json:"changed"`
}

// StepDef is one registered format step for a synthetic or future format version. Apply is a pure
// transform on the format bytes.
type StepDef struct {
	Name  string
	From  int
	To    int
	Apply func(data []byte) ([]byte, error)
}

// Warning is a per-entity condition of a migration run.
type Warning struct {
	Code    string `json:"code"`
	Book    string `json:"book,omitempty"`
	Message string `json:"message"`
}

// Report is the status, plan and apply result.
type Report struct {
	DryRun    bool            `json:"dryRun"`
	Target    Target          `json:"target"`
	Books     []BookStatus    `json:"books"`
	Registry  RegistryStatus  `json:"registry"`
	Ledger    LedgerStatus    `json:"ledger"`
	Store     StoreStatus     `json:"store"`
	Container ContainerStatus `json:"container"`
	OpenState OpenStateStatus `json:"openState"`
	Config    ConfigStatus    `json:"config"`
	Warnings  []Warning       `json:"warnings"`
}

// Service owns the migration steps over one store.
type Service struct {
	Lifecycle      *lifecycle.Lifecycle
	Layout         store.Layout
	Registry       *store.Registry
	BookPointLimit int
	ConfigPath     string
	Now            func() time.Time

	// Target versions and the step registries. The defaults are the current product versions and
	// empty step lists; tests inject synthetic future versions to exercise the frameworks.
	ConfigVersion    int
	ConfigSteps      []StepDef
	ContainerVersion int
	ContainerSteps   []StepDef
	OpenStateVersion int
	OpenStateSteps   []StepDef

	// afterWrite is a test hook that runs after the container write and before verification.
	afterWrite func() error
}

// New returns a migration service.
func New(life *lifecycle.Lifecycle, layout store.Layout, registry *store.Registry, bookPointLimit int, configPath string) *Service {
	return &Service{
		Lifecycle: life, Layout: layout, Registry: registry,
		BookPointLimit: bookPointLimit, ConfigPath: configPath, Now: time.Now,
		ConfigVersion: config.Version, ContainerVersion: store.Format,
		OpenStateVersion: lifecycle.StateVersion,
	}
}

// Status reports versions and pending steps without writing.
func (service *Service) Status(book model.BookID, all bool) (Report, error) {
	report := service.emptyReport(true)
	if service.Registry == nil {
		service.reportStore(&report)
		return report, nil
	}
	entries, err := service.scope(book, all)
	if err != nil {
		return Report{}, err
	}
	for _, entry := range entries {
		status, statusErr := service.bookStatus(entry.ID)
		if statusErr != nil {
			if all {
				report.Warnings = append(report.Warnings, Warning{
					Code: "migrate.read-failed", Book: string(entry.ID), Message: statusErr.Error(),
				})
				continue
			}
			return Report{}, statusErr
		}
		report.Books = append(report.Books, status)
	}
	service.reportStore(&report)
	service.reportFormats(&report)
	return report, nil
}

// Apply plans or performs the migration. write=false is a dry run that writes nothing.
func (service *Service) Apply(book model.BookID, all bool, to int, write, strict bool) (Report, error) {
	report := service.emptyReport(!write)
	entries, err := service.scope(book, all)
	if err != nil {
		return Report{}, err
	}
	if all && service.Registry == nil {
		return Report{}, fail("migrate.registry-missing", "the registry is missing; --all refuses")
	}
	timestamp := service.Now().UTC().Format("20060102T150405.000000000Z")
	for _, entry := range entries {
		status, statusErr := service.bookStatus(entry.ID)
		if statusErr != nil {
			if all {
				report.Warnings = append(report.Warnings, Warning{
					Code: "migrate.read-failed", Book: string(entry.ID), Message: statusErr.Error(),
				})
				if strict {
					return Report{}, fail("migrate.strict-failed", "book %q: %v", entry.ID, statusErr)
				}
				continue
			}
			return Report{}, statusErr
		}
		if target := to; target != 0 && target < status.Manifest.From {
			return Report{}, fail("migrate.downgrade", "book %q: %d is lower than %d",
				entry.ID, target, status.Manifest.From)
		}
		if !status.Manifest.Supported {
			if all {
				report.Warnings = append(report.Warnings, Warning{
					Code: "migrate.unsupported-source", Book: string(entry.ID),
					Message: fmt.Sprintf("manifest version %d is not supported", status.Manifest.From),
				})
				if strict {
					return Report{}, fail("migrate.strict-failed", "book %q is unsupported", entry.ID)
				}
				continue
			}
			return Report{}, fail("migrate.unsupported-source",
				"book %q: manifest version %d is not supported", entry.ID, status.Manifest.From)
		}
		if status.State != string(lifecycle.StatusClosed) {
			message := fmt.Sprintf("book %q is %s; close it first", entry.ID, status.State)
			if all {
				report.Warnings = append(report.Warnings, Warning{
					Code: "migrate.book-open", Book: string(entry.ID), Message: message,
				})
				if strict {
					return Report{}, fail("migrate.strict-failed", "%s", message)
				}
				continue
			}
			return Report{}, fail("migrate.book-open", "%s", message)
		}
		if len(status.Manifest.Steps) == 0 && !service.containerPending(entry.ID) {
			report.Books = append(report.Books, status)
			continue
		}
		if !write {
			status.Backup = service.containerBackupName(timestamp, entry.ID)
			status.Changed = true
			report.Books = append(report.Books, status)
			continue
		}
		migrated, migrateErr := service.migrateBook(entry.ID, status, timestamp)
		if migrateErr != nil {
			if all {
				report.Warnings = append(report.Warnings, Warning{
					Code: "migrate.failed", Book: string(entry.ID), Message: migrateErr.Error(),
				})
				if strict {
					return Report{}, fail("migrate.strict-failed", "book %q: %v", entry.ID, migrateErr)
				}
				continue
			}
			return Report{}, migrateErr
		}
		report.Books = append(report.Books, migrated)
	}
	service.reportStore(&report)
	service.reportFormats(&report)
	if !all {
		report.Registry.Changed = false
		report.Store.Changed = false
		report.Container.Changed = false
		report.OpenState.Changed = false
		report.Config.Changed = false
	}
	if write && all {
		if registryErr := service.migrateRegistry(timestamp, &report); registryErr != nil {
			return Report{}, registryErr
		}
		if configErr := service.migrateConfig(timestamp, &report, strict); configErr != nil {
			return Report{}, configErr
		}
		if openErr := service.migrateOpenState(timestamp, &report, strict); openErr != nil {
			return Report{}, openErr
		}
		if storeErr := service.migrateStore(timestamp, &report); storeErr != nil {
			return Report{}, storeErr
		}
	}
	if write && reportHasChanges(report) {
		if receiptErr := service.writeReceipt(timestamp, report); receiptErr != nil {
			return Report{}, receiptErr
		}
	}
	return report, nil
}

func (service *Service) emptyReport(dryRun bool) Report {
	return Report{
		DryRun: dryRun,
		Target: Target{
			Manifest: model.Version, Registry: store.RegistryVersion,
			Ledger: idspace.LedgerVersion, Store: store.StoreVersion,
			Container: service.ContainerVersion, OpenState: service.OpenStateVersion,
			Config: service.ConfigVersion,
		},
		Books: make([]BookStatus, 0), Warnings: make([]Warning, 0),
	}
}

func (service *Service) scope(book model.BookID, all bool) ([]store.RegistryEntry, error) {
	if book != "" && all {
		return nil, fail("migrate.scope-conflict", "use either --book or --all")
	}
	if book == "" && !all {
		return nil, fail("migrate.scope-required", "one of --book or --all is required")
	}
	if service.Registry == nil {
		return nil, fail("migrate.registry-missing", "the registry is missing")
	}
	if all {
		return service.Registry.Books, nil
	}
	entry, exists := service.Registry.Find(book)
	if !exists {
		return nil, fail("migrate.unknown-book", "book %q is not registered", book)
	}
	return []store.RegistryEntry{entry}, nil
}

// bookStatus reads the raw manifest version and the lifecycle state.
func (service *Service) bookStatus(id model.BookID) (BookStatus, error) {
	paths, err := service.Lifecycle.Paths(id)
	if err != nil {
		return BookStatus{}, err
	}
	state, err := service.Lifecycle.State(id)
	if err != nil {
		return BookStatus{}, err
	}
	raw, err := service.rawManifest(paths, state)
	if err != nil {
		return BookStatus{}, err
	}
	version, err := model.PeekVersion(raw)
	if err != nil {
		return BookStatus{}, fail("migrate.read-failed", "book %q: %v", id, err)
	}
	steps, stepErr := manifestSteps(version, 0)
	supported := stepErr == nil
	if stepErr != nil {
		steps = nil
	}
	status := BookStatus{
		ID: id, Open: state.Status != lifecycle.StatusClosed,
		State: string(state.Status),
		Manifest: ManifestChange{
			From: version, To: model.Version, Steps: steps, Supported: supported,
		},
		Derived: fileExists(paths.StateFile),
	}
	if !supported {
		status.Manifest.Steps = []string{}
	}
	return status, nil
}

func (service *Service) rawManifest(paths store.BookPaths, state lifecycle.State) ([]byte, error) {
	if state.Extracted {
		data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fail("migrate.read-failed", "%v", err)
		}
	}
	container, err := store.Load(paths.Container)
	if err != nil {
		return nil, fail("migrate.read-failed", "%v", err)
	}
	data, exists := container.Read(lifecycle.ManifestMember)
	if !exists {
		return nil, fail("migrate.read-failed", "book %q has no manifest", paths.ID)
	}
	return data, nil
}

func (service *Service) reportStore(report *Report) {
	registryVersion, err := store.PeekRegistryVersion(service.Layout.Registry())
	if err == nil {
		steps, _ := registrySteps(registryVersion)
		report.Registry = RegistryStatus{
			From: registryVersion, To: store.RegistryVersion, Steps: steps,
			Changed: registryVersion != store.RegistryVersion,
		}
	} else {
		report.Registry = RegistryStatus{From: 0, To: store.RegistryVersion, Missing: true}
		report.Warnings = append(report.Warnings, Warning{
			Code: "migrate.registry-missing", Message: err.Error(),
		})
	}
	ledgerVersion, ledgerErr := idspace.PeekLedgerVersion(service.Layout.IDs())
	if ledgerErr != nil {
		ledgerVersion = 0
	}
	report.Ledger = LedgerStatus{
		From: ledgerVersion, To: idspace.LedgerVersion,
		Changed: ledgerVersion != 0 && ledgerVersion != idspace.LedgerVersion,
	}
	storeVersion, storeErr := store.PeekStoreVersion(service.Layout.StoreMarkerPath())
	if storeErr != nil {
		report.Warnings = append(report.Warnings, Warning{
			Code: "migrate.store-invalid", Message: storeErr.Error(),
		})
	}
	steps, _ := storeSteps(storeVersion)
	report.Store = StoreStatus{
		From: storeVersion, To: store.StoreVersion, Steps: steps,
		Changed: storeVersion != store.StoreVersion,
	}
	openVersion, openErr := lifecycle.PeekStateVersion(service.Layout.OpenState())
	if openErr != nil {
		report.Warnings = append(report.Warnings, Warning{
			Code: "migrate.read-failed", Message: openErr.Error(),
		})
	}
	openSteps, openStepErr := stepsFor(service.OpenStateSteps, openVersion, service.OpenStateVersion)
	report.OpenState = OpenStateStatus{
		From: openVersion, To: service.OpenStateVersion, Steps: names(openSteps),
		Changed: openVersion != 0 && openVersion != service.OpenStateVersion,
	}
	if openVersion != 0 && openStepErr != nil {
		report.Warnings = append(report.Warnings, Warning{
			Code: "migrate.unsupported-source", Message: openStepErr.Error(),
		})
	}
	report.Config = service.configStatus()
}

// reportFormats reads the container format of the selected books.
func (service *Service) reportFormats(report *Report) {
	format := -1
	mixed := false
	for _, book := range report.Books {
		paths, err := service.Lifecycle.Paths(book.ID)
		if err != nil {
			continue
		}
		data, readErr := os.ReadFile(paths.Container)
		if readErr != nil {
			report.Warnings = append(report.Warnings, Warning{
				Code: "migrate.read-failed", Book: string(book.ID), Message: readErr.Error(),
			})
			continue
		}
		current, peekErr := store.PeekFormat(data)
		if peekErr != nil {
			report.Warnings = append(report.Warnings, Warning{
				Code: "migrate.read-failed", Book: string(book.ID), Message: peekErr.Error(),
			})
			continue
		}
		if format < 0 {
			format = current
		} else if format != current {
			mixed = true
		}
	}
	status := ContainerStatus{From: format, To: service.ContainerVersion}
	if format >= 0 {
		steps, stepErr := stepsFor(service.ContainerSteps, format, service.ContainerVersion)
		status.Steps = names(steps)
		status.Changed = format != service.ContainerVersion
		if stepErr != nil {
			report.Warnings = append(report.Warnings, Warning{
				Code: "migrate.unsupported-source", Message: stepErr.Error(),
			})
		}
	} else {
		status.From = 0
	}
	if mixed {
		report.Warnings = append(report.Warnings, Warning{
			Code: "migrate.container-mixed", Message: "the selected books use different container formats",
		})
	}
	report.Container = status
}

func (service *Service) configStatus() ConfigStatus {
	version := 0
	if service.ConfigPath != "" {
		data, err := os.ReadFile(service.ConfigPath)
		if err == nil {
			if peek, peekErr := rawVersion(data); peekErr == nil {
				version = peek
			}
		}
	}
	status := ConfigStatus{Version: version, Supported: version == 0 || version >= service.ConfigVersion}
	if version != 0 && version < service.ConfigVersion {
		steps, stepErr := stepsFor(service.ConfigSteps, version, service.ConfigVersion)
		status.Steps = names(steps)
		status.Changed = true
		if stepErr != nil {
			status.Supported = false
		}
	}
	return status
}

func rawVersion(data []byte) (int, error) {
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return 0, err
	}
	return header.Version, nil
}

// stepsFor returns the contiguous step sequence from one version to another.
func stepsFor(defs []StepDef, from, to int) ([]StepDef, error) {
	if from == to {
		return nil, nil
	}
	if from > to {
		return nil, fail("migrate.downgrade", "target %d is lower than %d", to, from)
	}
	available := append([]StepDef{}, defs...)
	sort.Slice(available, func(left, right int) bool { return available[left].From < available[right].From })
	current := from
	chosen := make([]StepDef, 0, to-from)
	for current < to {
		found := false
		for _, def := range available {
			if def.From != current {
				continue
			}
			chosen = append(chosen, def)
			current = def.To
			found = true
			break
		}
		if !found {
			return nil, fail("migrate.unsupported-source", "no step from version %d", current)
		}
	}
	return chosen, nil
}

func names(steps []StepDef) []string {
	entries := make([]string, len(steps))
	for index, step := range steps {
		entries[index] = step.Name
	}
	return entries
}

func applySteps(steps []StepDef, data []byte) ([]byte, error) {
	current := data
	for _, step := range steps {
		if step.Apply == nil {
			return nil, fail("migrate.unsupported-source", "step %q has no implementation", step.Name)
		}
		next, err := step.Apply(current)
		if err != nil {
			return nil, fail("migrate.failed", "step %q: %v", step.Name, err)
		}
		current = next
	}
	return current, nil
}

// migrateConfig applies the project configuration steps under the store lock.
func (service *Service) migrateConfig(timestamp string, report *Report, strict bool) error {
	if service.ConfigPath == "" {
		return nil
	}
	data, err := os.ReadFile(service.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fail("migrate.read-failed", "%v", err)
	}
	version, peekErr := rawVersion(data)
	if peekErr != nil {
		return fail("migrate.read-failed", "%v", peekErr)
	}
	if version >= service.ConfigVersion {
		return nil
	}
	steps, stepErr := stepsFor(service.ConfigSteps, version, service.ConfigVersion)
	if stepErr != nil {
		report.Warnings = append(report.Warnings, Warning{
			Code: "migrate.unsupported-source", Message: stepErr.Error(),
		})
		if strict {
			return fail("migrate.strict-failed", "%v", stepErr)
		}
		return nil
	}
	backup := filepath.Join(service.Layout.MigrationsRoot(), timestamp, "clabok.json")
	if writeErr := store.WriteAtomic(backup, data); writeErr != nil {
		return fail("migrate.write-failed", "%v", writeErr)
	}
	converted, applyErr := applySteps(steps, data)
	if applyErr != nil {
		return applyErr
	}
	targetVersion, targetErr := rawVersion(converted)
	if targetErr != nil || targetVersion != service.ConfigVersion {
		return fail("migrate.verify-failed", "config version %d", targetVersion)
	}
	if writeErr := store.WriteAtomic(service.ConfigPath, converted); writeErr != nil {
		return fail("migrate.write-failed", "%v", writeErr)
	}
	report.Config = ConfigStatus{
		Version: service.ConfigVersion, Supported: true, Steps: names(steps), Changed: true,
	}
	return nil
}

// migrateOpenState rewrites the transient open state only when every registered book is closed.
func (service *Service) migrateOpenState(timestamp string, report *Report, strict bool) error {
	path := service.Layout.OpenState()
	version, err := lifecycle.PeekStateVersion(path)
	if err != nil {
		return fail("migrate.read-failed", "%v", err)
	}
	if version == 0 || version >= service.OpenStateVersion {
		return nil
	}
	if service.Registry != nil {
		for _, entry := range service.Registry.Books {
			state, stateErr := service.Lifecycle.State(entry.ID)
			if stateErr != nil {
				return fail("migrate.read-failed", "%v", stateErr)
			}
			if state.Status != lifecycle.StatusClosed {
				message := fmt.Sprintf("book %q is %s; the open-state migration needs all books closed",
					entry.ID, state.Status)
				report.Warnings = append(report.Warnings, Warning{
					Code: "migrate.book-open", Book: string(entry.ID), Message: message,
				})
				if strict {
					return fail("migrate.strict-failed", "%s", message)
				}
				return nil
			}
		}
	}
	steps, stepErr := stepsFor(service.OpenStateSteps, version, service.OpenStateVersion)
	if stepErr != nil {
		report.Warnings = append(report.Warnings, Warning{
			Code: "migrate.unsupported-source", Message: stepErr.Error(),
		})
		if strict {
			return fail("migrate.strict-failed", "%v", stepErr)
		}
		return nil
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return fail("migrate.read-failed", "%v", readErr)
	}
	backup := filepath.Join(service.Layout.MigrationsRoot(), timestamp, "open.json")
	if writeErr := store.WriteAtomic(backup, data); writeErr != nil {
		return fail("migrate.write-failed", "%v", writeErr)
	}
	converted, applyErr := applySteps(steps, data)
	if applyErr != nil {
		return applyErr
	}
	targetVersion, targetErr := rawVersion(converted)
	if targetErr != nil || targetVersion != service.OpenStateVersion {
		return fail("migrate.verify-failed", "open state version %d", targetVersion)
	}
	if writeErr := store.WriteAtomic(path, converted); writeErr != nil {
		return fail("migrate.write-failed", "%v", writeErr)
	}
	report.OpenState = OpenStateStatus{
		From: version, To: service.OpenStateVersion, Steps: names(steps), Changed: true,
	}
	return nil
}

// CleanupReport lists the kept and removed migration history entries.
type CleanupReport struct {
	DryRun  bool     `json:"dryRun"`
	Keep    int      `json:"keep"`
	Kept    []string `json:"kept"`
	Removed []string `json:"removed"`
	Written bool     `json:"written"`
}

// Text renders the human form.
func (report CleanupReport) Text() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "kept %d, removed %d\n", len(report.Kept), len(report.Removed))
	for _, name := range report.Removed {
		fmt.Fprintf(&builder, "removed: %s\n", name)
	}
	return builder.String()
}

// Cleanup keeps the newest history entries and removes the older ones when write is set.
func (service *Service) Cleanup(keep int, write bool) (CleanupReport, error) {
	if keep < 0 {
		return CleanupReport{}, fail("migrate.invalid-keep", "--keep must not be negative")
	}
	report := CleanupReport{DryRun: !write, Keep: keep, Kept: []string{}, Removed: []string{}}
	entries, err := os.ReadDir(service.Layout.MigrationsRoot())
	if errors.Is(err, os.ErrNotExist) {
		return report, nil
	}
	if err != nil {
		return CleanupReport{}, fail("migrate.read-failed", "%v", err)
	}
	namesList := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			namesList = append(namesList, entry.Name())
		}
	}
	sort.Strings(namesList)
	split := len(namesList) - keep
	if split < 0 {
		split = 0
	}
	report.Removed = append(report.Removed, namesList[:split]...)
	report.Kept = append(report.Kept, namesList[split:]...)
	if !write || len(report.Removed) == 0 {
		return report, nil
	}
	storeRelease, lockErr := service.Lifecycle.LockStore()
	if lockErr != nil {
		return CleanupReport{}, fail("migrate.write-failed", "%v", lockErr)
	}
	defer storeRelease()
	for _, name := range report.Removed {
		if removeErr := os.RemoveAll(filepath.Join(service.Layout.MigrationsRoot(), name)); removeErr != nil {
			return CleanupReport{}, fail("migrate.write-failed", "%v", removeErr)
		}
	}
	report.Written = true
	return report, nil
}

func manifestSteps(from, to int) ([]string, error) {
	if to == 0 {
		to = model.Version
	}
	if from < 2 || from > model.Version {
		return nil, fail("migrate.unsupported-source", "manifest version %d is not supported", from)
	}
	if to < from {
		return nil, fail("migrate.downgrade", "target %d is lower than %d", to, from)
	}
	if to > model.Version {
		return nil, fail("migrate.unsupported-target", "target %d is newer than %d", to, model.Version)
	}
	steps := make([]string, 0, 6)
	for version := from; version < to; version++ {
		switch version {
		case 2:
			steps = append(steps, "manifest-v2-v3")
		case 3:
			steps = append(steps, "manifest-v3-v4")
		case 4:
			steps = append(steps, "manifest-v4-v5")
		case 5:
			steps = append(steps, "manifest-v5-v6")
		case 6:
			steps = append(steps, "manifest-v6-v7")
		}
	}
	return steps, nil
}

func registrySteps(from int) ([]string, error) {
	if from > store.RegistryVersion {
		return nil, fail("migrate.unsupported-source", "registry version %d is not supported", from)
	}
	steps := make([]string, 0, 1)
	if from == 1 {
		steps = append(steps, "registry-v1-v2")
	}
	return steps, nil
}

func storeSteps(from int) ([]string, error) {
	if from > store.StoreVersion {
		return nil, fail("migrate.unsupported-source", "store version %d is not supported", from)
	}
	steps := make([]string, 0, 1)
	if from == 0 {
		steps = append(steps, "store-v0-v1")
	}
	return steps, nil
}

// containerPending reports whether a book container needs a registered format step.
func (service *Service) containerPending(id model.BookID) bool {
	paths, err := service.Lifecycle.Paths(id)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(paths.Container)
	if err != nil {
		return false
	}
	format, err := store.PeekFormat(data)
	if err != nil {
		return false
	}
	return format < service.ContainerVersion
}

// migrateBook runs the closed-book transaction.
func (service *Service) migrateBook(id model.BookID, status BookStatus, timestamp string) (BookStatus, error) {
	paths, err := service.Lifecycle.Paths(id)
	if err != nil {
		return BookStatus{}, err
	}
	storeRelease, lockErr := service.Lifecycle.LockStore()
	if lockErr != nil {
		return BookStatus{}, fail("migrate.write-failed", "%v", lockErr)
	}
	defer storeRelease()
	bookRelease, lockErr := service.Lifecycle.LockBookExclusive(id)
	if lockErr != nil {
		return BookStatus{}, fail("migrate.write-failed", "%v", lockErr)
	}
	defer bookRelease()
	containerBytes, err := os.ReadFile(paths.Container)
	if err != nil {
		return BookStatus{}, fail("migrate.read-failed", "%v", err)
	}
	if format, peekErr := store.PeekFormat(containerBytes); peekErr == nil && format < service.ContainerVersion {
		steps, stepErr := stepsFor(service.ContainerSteps, format, service.ContainerVersion)
		if stepErr != nil {
			return BookStatus{}, stepErr
		}
		converted, applyErr := applySteps(steps, containerBytes)
		if applyErr != nil {
			return BookStatus{}, applyErr
		}
		if _, parseErr := store.Parse(converted); parseErr != nil {
			return BookStatus{}, fail("migrate.verify-failed", "%v", parseErr)
		}
		if writeErr := store.WriteAtomic(paths.Container, converted); writeErr != nil {
			return BookStatus{}, fail("migrate.write-failed", "%v", writeErr)
		}
	}
	container, err := store.Load(paths.Container)
	if err != nil {
		return BookStatus{}, fail("migrate.read-failed", "%v", err)
	}
	members := map[string][]byte{}
	for _, name := range container.Names() {
		if data, exists := container.Read(name); exists {
			members[name] = data
		}
	}
	raw, exists := members[lifecycle.ManifestMember]
	if !exists {
		return BookStatus{}, fail("migrate.read-failed", "book %q has no manifest", id)
	}
	manifest, err := model.Decode(raw)
	if err != nil {
		return BookStatus{}, fail("migrate.read-failed", "%v", err)
	}
	encoded, err := manifest.Encode()
	if err != nil {
		return BookStatus{}, fail("migrate.write-failed", "%v", err)
	}
	backupDir := filepath.Join(service.Layout.MigrationsRoot(), timestamp)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return BookStatus{}, fail("migrate.write-failed", "%v", err)
	}
	backupPath := filepath.Join(backupDir, string(id)+".book")
	if err := store.WriteAtomic(backupPath, containerBytes); err != nil {
		return BookStatus{}, fail("migrate.write-failed", "%v", err)
	}
	reason := fmt.Sprintf("migrate-v%d-v%d", status.Manifest.From, model.Version)
	rollbackStore, err := rollback.New(paths.RollbackDir, 0, service.BookPointLimit)
	if err != nil {
		return BookStatus{}, fail("migrate.write-failed", "%v", err)
	}
	packageValue, err := rollbackStore.Snapshot(rollback.KindBook, members, reason)
	if err != nil {
		return BookStatus{}, fail("migrate.write-failed", "%v", err)
	}
	target := store.New()
	for name, data := range members {
		if name == lifecycle.ManifestMember {
			data = encoded
		}
		if writeErr := target.Write(name, data); writeErr != nil {
			return BookStatus{}, fail("migrate.write-failed", "%v", writeErr)
		}
	}
	if err := target.Save(paths.Container); err != nil {
		_ = store.WriteAtomic(paths.Container, containerBytes)
		return BookStatus{}, fail("migrate.failed", "%v", err)
	}
	if service.afterWrite != nil {
		if hookErr := service.afterWrite(); hookErr != nil {
			_ = store.WriteAtomic(paths.Container, containerBytes)
			return BookStatus{}, fail("migrate.verify-failed", "%v", hookErr)
		}
	}
	if verifyErr := service.verifyBook(paths, members, encoded); verifyErr != nil {
		_ = store.WriteAtomic(paths.Container, containerBytes)
		return BookStatus{}, verifyErr
	}
	status.Backup = relative(service.Layout.Root, backupPath)
	status.Rollback = packageValue.Name
	status.Changed = true
	status.Reset = true
	status.Manifest.Steps = append([]string{}, status.Manifest.Steps...)
	if err := os.Remove(paths.StateFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		status.Derived = true
	}
	return status, nil
}

// verifyBook re-reads the target and compares the semantic content with the source.
func (service *Service) verifyBook(paths store.BookPaths, before map[string][]byte, encoded []byte) error {
	version, err := model.PeekVersion(encoded)
	if err != nil || version != model.Version {
		return fail("migrate.verify-failed", "target manifest version %d", version)
	}
	afterContainer, err := store.Load(paths.Container)
	if err != nil {
		return fail("migrate.verify-failed", "%v", err)
	}
	after, exists := afterContainer.Read(lifecycle.ManifestMember)
	if !exists {
		return fail("migrate.verify-failed", "the target has no manifest")
	}
	beforeManifest, err := model.Decode(before[lifecycle.ManifestMember])
	if err != nil {
		return fail("migrate.verify-failed", "%v", err)
	}
	afterManifest, err := model.Decode(after)
	if err != nil {
		return fail("migrate.verify-failed", "%v", err)
	}
	if !reflect.DeepEqual(beforeManifest.OwnedIDs(), afterManifest.OwnedIDs()) {
		return fail("migrate.verify-failed", "owned IDs differ")
	}
	if !reflect.DeepEqual(beforeManifest.Regions, afterManifest.Regions) {
		return fail("migrate.verify-failed", "regions differ")
	}
	if !reflect.DeepEqual(beforeManifest.BlockMeta, afterManifest.BlockMeta) {
		return fail("migrate.verify-failed", "block metadata differs")
	}
	if countNodes(beforeManifest) != countNodes(afterManifest) {
		return fail("migrate.verify-failed", "node count differs")
	}
	beforeHashes := bodyHashes(before)
	afterHashes := bodyHashes(readMembers(afterContainer))
	if !reflect.DeepEqual(beforeHashes, afterHashes) {
		return fail("migrate.verify-failed", "body hashes differ")
	}
	return nil
}

func (service *Service) migrateRegistry(timestamp string, report *Report) error {
	if report.Registry.Missing || !report.Registry.Changed {
		return nil
	}
	storeRelease, lockErr := service.Lifecycle.LockStore()
	if lockErr != nil {
		return fail("migrate.write-failed", "%v", lockErr)
	}
	defer storeRelease()
	backupPath := filepath.Join(service.Layout.MigrationsRoot(), timestamp, "library.json")
	if data, err := os.ReadFile(service.Layout.Registry()); err == nil {
		if writeErr := store.WriteAtomic(backupPath, data); writeErr != nil {
			return fail("migrate.write-failed", "%v", writeErr)
		}
	}
	registry, err := store.LoadRegistry(service.Layout.Registry())
	if err != nil {
		return fail("migrate.read-failed", "%v", err)
	}
	if err := registry.Save(service.Layout.Registry()); err != nil {
		return fail("migrate.write-failed", "%v", err)
	}
	version, err := store.PeekRegistryVersion(service.Layout.Registry())
	if err != nil || version != store.RegistryVersion {
		return fail("migrate.verify-failed", "registry version %d", version)
	}
	report.Registry.Changed = true
	report.Registry.Backup = relative(service.Layout.Root, backupPath)
	return nil
}

func (service *Service) migrateStore(timestamp string, report *Report) error {
	if !report.Store.Changed {
		return nil
	}
	storeRelease, lockErr := service.Lifecycle.LockStore()
	if lockErr != nil {
		return fail("migrate.write-failed", "%v", lockErr)
	}
	defer storeRelease()
	marker := service.Layout.StoreMarkerPath()
	if data, err := os.ReadFile(marker); err == nil {
		backupPath := filepath.Join(service.Layout.MigrationsRoot(), timestamp, "store.json")
		if writeErr := store.WriteAtomic(backupPath, data); writeErr != nil {
			return fail("migrate.write-failed", "%v", writeErr)
		}
		report.Store.Backup = relative(service.Layout.Root, backupPath)
	}
	if err := store.WriteStoreVersion(marker, store.StoreVersion); err != nil {
		return fail("migrate.write-failed", "%v", err)
	}
	version, err := store.PeekStoreVersion(marker)
	if err != nil || version != store.StoreVersion {
		return fail("migrate.verify-failed", "store version %d", version)
	}
	report.Store.Changed = true
	return nil
}

func (service *Service) writeReceipt(timestamp string, report Report) error {
	directory := filepath.Join(service.Layout.MigrationsRoot(), timestamp)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fail("migrate.write-failed", "%v", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fail("migrate.write-failed", "%v", err)
	}
	if err := store.WriteAtomic(filepath.Join(directory, "receipt.json"), append(data, '\n')); err != nil {
		return fail("migrate.write-failed", "%v", err)
	}
	return nil
}

func (service *Service) containerBackupName(timestamp string, id model.BookID) string {
	return filepath.ToSlash(filepath.Join("migrations", timestamp, string(id)+".book"))
}

func reportHasChanges(report Report) bool {
	for _, book := range report.Books {
		if book.Changed {
			return true
		}
	}
	return report.Registry.Changed || report.Store.Changed
}

func readMembers(container *store.Container) map[string][]byte {
	members := map[string][]byte{}
	for _, name := range container.Names() {
		if data, exists := container.Read(name); exists {
			members[name] = data
		}
	}
	return members
}

func bodyHashes(members map[string][]byte) map[string]string {
	hashes := map[string]string{}
	names := make([]string, 0, len(members))
	for name := range members {
		if len(name) > 7 && name[:7] == "blocks/" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		sum := sha256.Sum256(members[name])
		hashes[name] = hex.EncodeToString(sum[:])
	}
	return hashes
}

func countNodes(manifest *model.Manifest) int {
	total := 0
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		total++
		return true
	})
	return total
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func relative(root, path string) string {
	target, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(target)
}
