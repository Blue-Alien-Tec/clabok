package index

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/source"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// Version is the index format version. It is independent of the book schema and changes whenever
// the derived layout changes, so an old index is ignored instead of misread.
const Version = 2

const (
	manifestName = "manifest.json"
	sectionSize  = 64 << 10
)

// Error is an index failure with a stable code.
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

// Fingerprint identifies the container state an index was built from.
type Fingerprint struct {
	Size    int64  `json:"size"`
	ModTime int64  `json:"mtime"`
	Head    string `json:"head"`
	Tail    string `json:"tail"`
}

// BookEntry records one indexed book in the global manifest.
type BookEntry struct {
	ID          model.BookID `json:"id"`
	Fingerprint Fingerprint  `json:"fingerprint"`
	FullHash    string       `json:"fullHash,omitempty"`
	Size        int64        `json:"size"`
}

// Manifest is the global index manifest.
type Manifest struct {
	Version int         `json:"version"`
	Books   []BookEntry `json:"books"`
}

// Entry is one indexed line with its text.
type Entry struct {
	Node  string `json:"node,omitempty"`
	Block string `json:"block,omitempty"`
	Field string `json:"field"`
	Line  int    `json:"line"`
	Text  string `json:"text"`
}

// EntryMeta is the postings-side metadata of one line; the text lives in the lines file.
type EntryMeta struct {
	Node  string `json:"node,omitempty"`
	Block string `json:"block,omitempty"`
	Field string `json:"field"`
	Line  int    `json:"line"`
}

// Postings is the section a query reads first: metadata, postings, offsets and missing bodies.
type Postings struct {
	Version  int              `json:"version"`
	Book     model.BookID     `json:"book"`
	Entries  []EntryMeta      `json:"entries"`
	Tokens   map[string][]int `json:"tokens"`
	Trigrams map[string][]int `json:"trigrams"`
	Missing  []string         `json:"missing,omitempty"`
	Owned    []string         `json:"owned,omitempty"`
	Offsets  []int64          `json:"offsets"`
}

// BookIndex is the assembled derived content of one book: postings plus line text.
type BookIndex struct {
	Version  int
	Book     model.BookID
	Entries  []Entry
	Tokens   map[string][]int
	Trigrams map[string][]int
	Missing  []string
	Owned    []string
}

// Status is the observed state of one book index.
type Status struct {
	Book    model.BookID `json:"book"`
	State   string       `json:"state"`
	Version int          `json:"version,omitempty"`
	BuiltAt string       `json:"builtAt,omitempty"`
	Size    int64        `json:"size,omitempty"`
}

// Service manages the index of one store.
type Service struct {
	Lifecycle *lifecycle.Lifecycle
	Registry  *store.Registry
	Root      string
	Now       func() time.Time

	mutex      sync.Mutex
	cache      map[model.BookID]cachedPostings
	cacheOrder []model.BookID
	maxCache   int
}

type cachedPostings struct {
	fingerprint Fingerprint
	postings    *Postings
}

// New returns an index service over one store root and registry.
func New(life *lifecycle.Lifecycle, registry *store.Registry, root string) *Service {
	return &Service{
		Lifecycle: life, Registry: registry, Root: root, Now: time.Now,
		cache: map[model.BookID]cachedPostings{}, maxCache: 8,
	}
}

func (service *Service) directory() string { return filepath.Join(service.Root, "index") }

func (service *Service) manifestPath() string {
	return filepath.Join(service.directory(), manifestName)
}

func (service *Service) postingsPath(id model.BookID) string {
	return filepath.Join(service.directory(), string(id)+".postings.json")
}

func (service *Service) linesPath(id model.BookID) string {
	return filepath.Join(service.directory(), string(id)+".lines")
}

// LoadManifest reads the global manifest. A missing file is an empty manifest.
func (service *Service) LoadManifest() (*Manifest, error) {
	data, err := os.ReadFile(service.manifestPath())
	if errors.Is(err, os.ErrNotExist) {
		return &Manifest{Version: Version}, nil
	}
	if err != nil {
		return nil, fail("index.read-failed", "%v", err)
	}
	var manifest Manifest
	if decodeErr := json.Unmarshal(data, &manifest); decodeErr != nil {
		return nil, fail("index.corrupt", "manifest: %v", decodeErr)
	}
	if manifest.Version != Version {
		return nil, fail("index.corrupt", "unsupported index version %d", manifest.Version)
	}
	return &manifest, nil
}

// Status reports the state of one book index.
func (service *Service) Status(id model.BookID) (Status, error) {
	state, err := service.Lifecycle.State(id)
	if err != nil {
		return Status{}, err
	}
	if state.HasEntry || state.Extracted {
		return Status{Book: id, State: "open"}, nil
	}
	manifest, err := service.LoadManifest()
	if err != nil {
		return Status{Book: id, State: "corrupt"}, nil
	}
	entry, exists := findEntry(manifest, id)
	if !exists {
		return Status{Book: id, State: "missing"}, nil
	}
	postingsInfo, statErr := os.Stat(service.postingsPath(id))
	if errors.Is(statErr, os.ErrNotExist) {
		return Status{Book: id, State: "missing"}, nil
	}
	if statErr != nil {
		return Status{Book: id, State: "corrupt"}, nil
	}
	linesInfo, linesErr := os.Stat(service.linesPath(id))
	if linesErr != nil {
		return Status{Book: id, State: "corrupt", Size: postingsInfo.Size()}, nil
	}
	data, readErr := os.ReadFile(service.postingsPath(id))
	if readErr != nil {
		return Status{Book: id, State: "corrupt"}, nil
	}
	var postings Postings
	if decodeErr := json.Unmarshal(data, &postings); decodeErr != nil || postings.Version != Version {
		return Status{Book: id, State: "corrupt", Size: postingsInfo.Size()}, nil
	}
	paths, pathErr := service.Lifecycle.Paths(id)
	if pathErr != nil {
		return Status{}, pathErr
	}
	current, printErr := fingerprint(paths.Container)
	if printErr != nil {
		return Status{Book: id, State: "corrupt", Size: postingsInfo.Size()}, nil
	}
	status := Status{
		Book: id, Version: postings.Version, Size: postingsInfo.Size() + linesInfo.Size(),
		BuiltAt: postingsInfo.ModTime().UTC().Format(time.RFC3339),
	}
	if current != entry.Fingerprint {
		status.State = "stale"
		return status, nil
	}
	status.State = "fresh"
	return status, nil
}

// StatusVerified additionally compares the full container hash; it is the `index status --verify`
// view and detects a stale index that the cheap fingerprint missed.
func (service *Service) StatusVerified(id model.BookID) (Status, error) {
	status, err := service.Status(id)
	if err != nil || status.State != "fresh" {
		return status, err
	}
	manifest, manifestErr := service.LoadManifest()
	if manifestErr != nil {
		return status, nil
	}
	entry, exists := findEntry(manifest, id)
	if !exists || entry.FullHash == "" {
		status.State = "stale"
		return status, nil
	}
	paths, pathErr := service.Lifecycle.Paths(id)
	if pathErr != nil {
		return Status{}, pathErr
	}
	current, hashErr := fullHash(paths.Container)
	if hashErr != nil {
		return Status{}, fail("index.read-failed", "%v", hashErr)
	}
	if current != entry.FullHash {
		status.State = "stale"
	}
	return status, nil
}

// Build derives and writes the index of one closed book. The postings and the lines file are
// written first and the manifest last, so a crash leaves a stale entry instead of a partial index.
func (service *Service) Build(id model.BookID) (Status, error) {
	if err := service.Lifecycle.RequireClosed(id); err != nil {
		return Status{}, fail("index.open-book", "book %q is open; close it first", id)
	}
	storeRelease, lockErr := service.Lifecycle.LockStore()
	if lockErr != nil {
		return Status{}, fail("index.lock-failed", "%v", lockErr)
	}
	defer storeRelease()
	bookRelease, lockErr := service.Lifecycle.LockBookExclusive(id)
	if lockErr != nil {
		return Status{}, fail("index.lock-failed", "%v", lockErr)
	}
	defer bookRelease()
	paths, err := service.Lifecycle.Paths(id)
	if err != nil {
		return Status{}, err
	}
	book, err := source.New(service.Lifecycle).Load(id)
	if err != nil {
		return Status{}, fail("index.read-failed", "%v", err)
	}
	derived := derive(book)
	if err := os.MkdirAll(service.directory(), 0o755); err != nil {
		return Status{}, fail("index.write-failed", "%v", err)
	}
	var lines bytes.Buffer
	metas := make([]EntryMeta, len(derived.Entries))
	offsets := make([]int64, len(derived.Entries))
	for position, entry := range derived.Entries {
		offsets[position] = int64(lines.Len())
		lines.WriteString(entry.Text)
		lines.WriteByte('\n')
		metas[position] = EntryMeta{
			Node: entry.Node, Block: entry.Block, Field: entry.Field, Line: entry.Line,
		}
	}
	postings := &Postings{
		Version: Version, Book: id, Entries: metas, Tokens: derived.Tokens,
		Trigrams: derived.Trigrams, Missing: derived.Missing, Owned: derived.Owned, Offsets: offsets,
	}
	encoded, err := json.MarshalIndent(postings, "", "  ")
	if err != nil {
		return Status{}, fail("index.write-failed", "%v", err)
	}
	if err := store.WriteAtomic(service.linesPath(id), lines.Bytes()); err != nil {
		return Status{}, fail("index.write-failed", "%v", err)
	}
	if err := store.WriteAtomic(service.postingsPath(id), append(encoded, '\n')); err != nil {
		return Status{}, fail("index.write-failed", "%v", err)
	}
	postingsInfo, err := os.Stat(service.postingsPath(id))
	if err != nil {
		return Status{}, fail("index.write-failed", "%v", err)
	}
	linesInfo, err := os.Stat(service.linesPath(id))
	if err != nil {
		return Status{}, fail("index.write-failed", "%v", err)
	}
	print, err := fingerprint(paths.Container)
	if err != nil {
		return Status{}, fail("index.read-failed", "%v", err)
	}
	full, err := fullHash(paths.Container)
	if err != nil {
		return Status{}, fail("index.read-failed", "%v", err)
	}
	manifest, err := service.LoadManifest()
	if err != nil {
		manifest = &Manifest{Version: Version}
	}
	size := postingsInfo.Size() + linesInfo.Size()
	upsertEntry(manifest, BookEntry{ID: id, Fingerprint: print, FullHash: full, Size: size})
	if err := service.saveManifest(manifest); err != nil {
		return Status{}, err
	}
	service.forget(id)
	return Status{
		Book: id, State: "fresh", Version: Version, Size: size,
		BuiltAt: postingsInfo.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// Clear removes the index of one book.
func (service *Service) Clear(id model.BookID) error {
	storeRelease, lockErr := service.Lifecycle.LockStore()
	if lockErr != nil {
		return fail("index.lock-failed", "%v", lockErr)
	}
	defer storeRelease()
	bookRelease, lockErr := service.Lifecycle.LockBookExclusive(id)
	if lockErr != nil {
		return fail("index.lock-failed", "%v", lockErr)
	}
	defer bookRelease()
	manifest, err := service.LoadManifest()
	if err != nil {
		return err
	}
	for _, path := range []string{service.postingsPath(id), service.linesPath(id)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail("index.write-failed", "%v", err)
		}
	}
	if removeEntry(manifest, id) {
		if err := service.saveManifest(manifest); err != nil {
			return err
		}
	}
	service.forget(id)
	return nil
}

// LoadPostings reads the postings section and caches it by fingerprint.
func (service *Service) LoadPostings(id model.BookID) (*Postings, error) {
	manifest, err := service.LoadManifest()
	if err != nil {
		return nil, err
	}
	entry, exists := findEntry(manifest, id)
	if !exists {
		return nil, fail("index.missing", "no index for book %q", id)
	}
	service.mutex.Lock()
	cached, cachedOK := service.cache[id]
	service.mutex.Unlock()
	if cachedOK && cached.fingerprint == entry.Fingerprint {
		return cached.postings, nil
	}
	data, err := os.ReadFile(service.postingsPath(id))
	if err != nil {
		return nil, fail("index.missing", "no index for book %q", id)
	}
	var postings Postings
	if decodeErr := json.Unmarshal(data, &postings); decodeErr != nil {
		return nil, fail("index.corrupt", "%v", decodeErr)
	}
	if postings.Version != Version {
		return nil, fail("index.corrupt", "unsupported index version %d", postings.Version)
	}
	service.mutex.Lock()
	service.remember(id, cachedPostings{fingerprint: entry.Fingerprint, postings: &postings})
	service.mutex.Unlock()
	return &postings, nil
}

// remember stores a postings cache entry and keeps the cache bounded.
func (service *Service) remember(id model.BookID, entry cachedPostings) {
	if _, exists := service.cache[id]; !exists {
		service.cacheOrder = append(service.cacheOrder, id)
	}
	service.cache[id] = entry
	limit := service.maxCache
	if limit <= 0 {
		limit = 8
	}
	for len(service.cacheOrder) > limit {
		oldest := service.cacheOrder[0]
		service.cacheOrder = service.cacheOrder[1:]
		delete(service.cache, oldest)
	}
}

// forget drops the cache entry of one book.
func (service *Service) forget(id model.BookID) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	delete(service.cache, id)
	for index := range service.cacheOrder {
		if service.cacheOrder[index] == id {
			service.cacheOrder = append(service.cacheOrder[:index], service.cacheOrder[index+1:]...)
			break
		}
	}
}

// ReadTexts reads only the requested entry texts from the lines file.
func (service *Service) ReadTexts(book model.BookID, postings *Postings, positions []int) (map[int]string, error) {
	file, err := os.Open(service.linesPath(book))
	if err != nil {
		return nil, fail("index.read-failed", "%v", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fail("index.read-failed", "%v", err)
	}
	texts := make(map[int]string, len(positions))
	for _, position := range positions {
		if position < 0 || position >= len(postings.Offsets) {
			continue
		}
		start := postings.Offsets[position]
		end := info.Size()
		if position+1 < len(postings.Offsets) {
			end = postings.Offsets[position+1]
		}
		if end < start {
			continue
		}
		buffer := make([]byte, end-start)
		count, readErr := file.ReadAt(buffer, start)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, fail("index.read-failed", "%v", readErr)
		}
		texts[position] = strings.TrimSuffix(string(buffer[:count]), "\n")
	}
	return texts, nil
}

// Load assembles the postings and every line text. It is intended for diagnostics and tests; the
// search path uses LoadPostings and ReadTexts.
func (service *Service) Load(id model.BookID) (*BookIndex, error) {
	postings, err := service.LoadPostings(id)
	if err != nil {
		return nil, err
	}
	positions := make([]int, len(postings.Entries))
	for position := range positions {
		positions[position] = position
	}
	texts, err := service.ReadTexts(id, postings, positions)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(postings.Entries))
	for position, meta := range postings.Entries {
		entries[position] = Entry{
			Node: meta.Node, Block: meta.Block, Field: meta.Field, Line: meta.Line,
			Text: texts[position],
		}
	}
	return &BookIndex{
		Version: postings.Version, Book: postings.Book, Entries: entries,
		Tokens: postings.Tokens, Trigrams: postings.Trigrams, Missing: postings.Missing,
		Owned: postings.Owned,
	}, nil
}

// Fresh reports whether the index of a closed book matches its container.
func (service *Service) Fresh(id model.BookID) bool {
	status, err := service.Status(id)
	return err == nil && status.State == "fresh"
}

func (service *Service) saveManifest(manifest *Manifest) error {
	if err := os.MkdirAll(service.directory(), 0o755); err != nil {
		return fail("index.write-failed", "%v", err)
	}
	manifest.Version = Version
	sort.Slice(manifest.Books, func(left, right int) bool {
		return manifest.Books[left].ID < manifest.Books[right].ID
	})
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fail("index.write-failed", "%v", err)
	}
	if err := store.WriteAtomic(service.manifestPath(), append(data, '\n')); err != nil {
		return fail("index.write-failed", "%v", err)
	}
	return nil
}

func findEntry(manifest *Manifest, id model.BookID) (BookEntry, bool) {
	for _, entry := range manifest.Books {
		if entry.ID == id {
			return entry, true
		}
	}
	return BookEntry{}, false
}

func upsertEntry(manifest *Manifest, entry BookEntry) {
	for index := range manifest.Books {
		if manifest.Books[index].ID == entry.ID {
			manifest.Books[index] = entry
			return
		}
	}
	manifest.Books = append(manifest.Books, entry)
}

func removeEntry(manifest *Manifest, id model.BookID) bool {
	for index := range manifest.Books {
		if manifest.Books[index].ID == id {
			manifest.Books = append(manifest.Books[:index], manifest.Books[index+1:]...)
			return true
		}
	}
	return false
}

// derive builds the index entries and the postings of one loaded book.
func derive(book *source.Book) *BookIndex {
	derived := &BookIndex{
		Version: Version, Book: book.ID,
		Entries:  make([]Entry, 0),
		Tokens:   map[string][]int{},
		Trigrams: map[string][]int{},
		Missing:  make([]string, 0, len(book.MissingBodies())),
	}
	for _, id := range book.MissingBodies() {
		derived.Missing = append(derived.Missing, id.String())
	}
	for _, id := range book.Manifest.OwnedIDs() {
		derived.Owned = append(derived.Owned, id.String())
	}
	if book.Manifest == nil {
		return derived
	}
	appendLine := func(node model.ID, block model.ID, field, text string, line int) {
		entry := Entry{Field: field, Line: line, Text: text}
		if node != model.ZeroID {
			entry.Node = node.String()
		}
		if block != model.ZeroID {
			entry.Block = block.String()
		}
		position := len(derived.Entries)
		derived.Entries = append(derived.Entries, entry)
		indexTokens(derived, text, position)
	}
	book.Manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		if node.Title != "" {
			appendLine(node.ID, model.ZeroID, "title", node.Title, 1)
		}
		for _, id := range node.Blocks {
			if book.Manifest.BlockType(id) == "RepoLayout" {
				continue
			}
			if body, exists := book.Body(id); exists {
				appendLines(appendLine, node.ID, id, "body", body)
			}
			if spec, exists := book.Layout(id); exists {
				appendLines(appendLine, node.ID, id, "layout", string(spec))
			}
		}
		return true
	})
	for _, region := range book.Manifest.Regions {
		field := regionField(region.Kind)
		if field == "" {
			continue
		}
		owner := model.ZeroID
		switch region.Target.Kind {
		case model.TargetNode:
			owner = region.Target.ID
		case model.TargetBlock:
			if node, ok := book.NodeForBlock(region.Target.ID); ok {
				owner = node.ID
			}
		}
		for _, id := range region.Blocks {
			if body, exists := book.Body(id); exists {
				appendLines(appendLine, owner, id, field, body)
			}
		}
	}
	return derived
}

func appendLines(appendLine func(node, block model.ID, field, text string, line int), node, block model.ID, field, text string) {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for index, line := range lines {
		appendLine(node, block, field, line, index+1)
	}
}

func regionField(kind model.RegionKind) string {
	switch kind {
	case model.RegionProse:
		return "prose"
	case model.RegionNotes:
		return "note"
	case model.RegionFooter:
		return "footer"
	}
	return ""
}

func indexTokens(derived *BookIndex, text string, position int) {
	seen := map[string]struct{}{}
	words := tokenize(text)
	for _, word := range words {
		lowered := strings.ToLower(word.Text)
		if _, exists := seen[lowered]; exists {
			continue
		}
		seen[lowered] = struct{}{}
		derived.Tokens[lowered] = appendUnique(derived.Tokens[lowered], position)
	}
	trigrams := map[string]struct{}{}
	for _, word := range words {
		runes := []rune(strings.ToLower(word.Text))
		for start := 0; start+3 <= len(runes); start++ {
			trigrams[string(runes[start:start+3])] = struct{}{}
		}
	}
	for trigram := range trigrams {
		derived.Trigrams[trigram] = appendUnique(derived.Trigrams[trigram], position)
	}
}

func appendUnique(values []int, value int) []int {
	if len(values) > 0 && values[len(values)-1] == value {
		return values
	}
	return append(values, value)
}

// word is one token occurrence.
type word struct {
	Start int
	End   int
	Text  string
}

func tokenize(text string) []word {
	words := make([]word, 0)
	start := -1
	for index, character := range text {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '_' {
			if start < 0 {
				start = index
			}
			continue
		}
		if start >= 0 {
			words = append(words, word{Start: start, End: index, Text: text[start:index]})
			start = -1
		}
	}
	if start >= 0 {
		words = append(words, word{Start: start, End: len(text), Text: text[start:]})
	}
	return words
}

// fingerprint hashes the container size, mtime, first and last section.
func fingerprint(path string) (Fingerprint, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Fingerprint{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Fingerprint{}, err
	}
	defer file.Close()
	head, err := hashRange(file, 0, sectionSize)
	if err != nil {
		return Fingerprint{}, err
	}
	tailOffset := info.Size() - sectionSize
	if tailOffset < 0 {
		tailOffset = 0
	}
	tail, err := hashRange(file, tailOffset, sectionSize)
	if err != nil {
		return Fingerprint{}, err
	}
	return Fingerprint{
		Size: info.Size(), ModTime: info.ModTime().UnixNano(), Head: head, Tail: tail,
	}, nil
}

func hashRange(file *os.File, offset, length int64) (string, error) {
	buffer := make([]byte, length)
	count, err := file.ReadAt(buffer, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	sum := sha256.Sum256(buffer[:count])
	return hex.EncodeToString(sum[:]), nil
}

// fullHash hashes the whole container for `index status --verify`.
func fullHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
