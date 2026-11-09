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
	"time"

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
	Manifest int `json:"manifest"`
	Registry int `json:"registry"`
	Ledger   int `json:"ledger"`
	Store    int `json:"store"`
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

// ConfigStatus reports the project configuration version.
type ConfigStatus struct {
	Version   int  `json:"version"`
	Supported bool `json:"supported"`
}

// Warning is a per-entity condition of a migration run.
type Warning struct {
	Code    string `json:"code"`
	Book    string `json:"book,omitempty"`
	Message string `json:"message"`
}

// Report is the status, plan and apply result.
type Report struct {
	DryRun   bool           `json:"dryRun"`
	Target   Target         `json:"target"`
	Books    []BookStatus   `json:"books"`
	Registry RegistryStatus `json:"registry"`
	Ledger   LedgerStatus   `json:"ledger"`
	Store    StoreStatus    `json:"store"`
	Config   ConfigStatus   `json:"config"`
	Warnings []Warning      `json:"warnings"`
}

// Service owns the migration steps over one store.
type Service struct {
	Lifecycle      *lifecycle.Lifecycle
	Layout         store.Layout
	Registry       *store.Registry
	BookPointLimit int
	ConfigPath     string
	Now            func() time.Time

	// afterWrite is a test hook that runs after the container write and before verification.
	afterWrite func() error
}

// New returns a migration service.
func New(life *lifecycle.Lifecycle, layout store.Layout, registry *store.Registry, bookPointLimit int, configPath string) *Service {
	return &Service{
		Lifecycle: life, Layout: layout, Registry: registry,
		BookPointLimit: bookPointLimit, ConfigPath: configPath, Now: time.Now,
	}
}

// Status reports versions and pending steps without writing.
func (service *Service) Status(book model.BookID, all bool) (Report, error) {
	report := service.emptyReport(true)
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
		if len(status.Manifest.Steps) == 0 {
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
	if !all {
		report.Registry.Changed = false
		report.Store.Changed = false
	}
	if write && all {
		if registryErr := service.migrateRegistry(timestamp, &report); registryErr != nil {
			return Report{}, registryErr
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
	report.Config = service.configStatus()
}

func (service *Service) configStatus() ConfigStatus {
	if service.ConfigPath == "" {
		return ConfigStatus{Version: 0, Supported: true}
	}
	data, err := os.ReadFile(service.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return ConfigStatus{Version: 0, Supported: true}
	}
	if err != nil {
		return ConfigStatus{}
	}
	var header struct {
		Version int `json:"version"`
	}
	if decodeErr := json.Unmarshal(data, &header); decodeErr != nil {
		return ConfigStatus{}
	}
	return ConfigStatus{Version: header.Version, Supported: header.Version <= 1}
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
	steps := make([]string, 0, 1)
	if from == 2 {
		steps = append(steps, "manifest-v2-v3")
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
	containerBytes, err := os.ReadFile(paths.Container)
	if err != nil {
		return BookStatus{}, fail("migrate.read-failed", "%v", err)
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
