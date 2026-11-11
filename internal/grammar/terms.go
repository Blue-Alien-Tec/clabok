package grammar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// ScopeGlobal names the store-wide term layer.
const ScopeGlobal = "global"

// TermSubstringMarker prefixes a term that suppresses substrings, for example ~ark for Arkansas.
const TermSubstringMarker = "~"

// TermForm is one parsed term line.
type TermForm struct {
	Term      string
	Substring bool
}

// TermLayer is one term file and its scope. A language layer applies only to its own language.
type TermLayer struct {
	Scope string
	Path  string
	Terms []string
}

// TermSet is the effective union for one check: the global layer and the layer of the check
// language. There is no hierarchy and no negative layer.
type TermSet struct {
	Layers []TermLayer
}

// Suppress reports whether a term of the set matches the offending text or one of its replacements,
// with one attribution per matching term. A term present in several layers keeps the first layer and
// lists the others in AlsoIn, so a suppression always names its source.
func (set TermSet) Suppress(offending string, replacements []string) []Suppression {
	if len(set.Layers) == 0 {
		return nil
	}
	entries := make([]Suppression, 0)
	seen := map[string]int{}
	for _, layer := range set.Layers {
		for _, raw := range layer.Terms {
			form := ParseTermForm(raw)
			if form.Term == "" {
				continue
			}
			field := ""
			if termMatches(offending, form) {
				field = "offending"
			} else {
				for _, replacement := range replacements {
					if termMatches(replacement, form) {
						field = "replacement"
						break
					}
				}
			}
			if field == "" {
				continue
			}
			key := strings.ToLower(form.Term)
			if index, ok := seen[key]; ok {
				entries[index].AlsoIn = append(entries[index].AlsoIn, layer.Scope)
				continue
			}
			seen[key] = len(entries)
			entries = append(entries, Suppression{
				Term: form.Term, Layer: layer.Scope, Match: formName(form), Field: field,
			})
		}
	}
	return entries
}

// termMatches searches for a term with word boundaries unless the substring form is requested.
func termMatches(text string, form TermForm) bool {
	if form.Term == "" {
		return false
	}
	lowerText := strings.ToLower(text)
	lowerTerm := strings.ToLower(form.Term)
	for start := 0; start <= len(lowerText)-len(lowerTerm); {
		at := strings.Index(lowerText[start:], lowerTerm)
		if at < 0 {
			return false
		}
		at += start
		if form.Substring || bounded(lowerText, at, at+len(lowerTerm)) {
			return true
		}
		start = at + 1
	}
	return false
}

// bounded reports whether the runes around a match are not letters or digits. Underscore, hyphen
// and punctuation are delimiters.
func bounded(text string, start, end int) bool {
	if start > 0 {
		runeValue, _ := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue) {
			return false
		}
	}
	if end < len(text) {
		runeValue, _ := utf8.DecodeRuneInString(text[end:])
		if unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue) {
			return false
		}
	}
	return true
}

// LoadTermSet loads the global layer and, when a language is given, its layer. A missing file is an
// empty layer.
func LoadTermSet(globalPath, termsDir, language string) TermSet {
	set := TermSet{Layers: []TermLayer{
		{Scope: ScopeGlobal, Path: globalPath, Terms: ReadTermLayer(globalPath)},
	}}
	language = strings.TrimSpace(language)
	if language == "" || language == "auto" {
		return set
	}
	path := languageTermsPath(termsDir, language)
	set.Layers = append(set.Layers, TermLayer{
		Scope: strings.ToLower(language), Path: path, Terms: ReadTermLayer(path),
	})
	return set
}

// ReadTermLayer reads one layer: one term per line, trimmed, empty lines skipped.
func ReadTermLayer(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	terms := make([]string, 0)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		terms = append(terms, line)
	}
	return terms
}

// languageTermsPath resolves the file of a language layer.
func languageTermsPath(termsDir, language string) string {
	return filepath.Join(termsDir, strings.ToLower(strings.TrimSpace(language))+".txt")
}

// ParseTermForm splits the optional substring marker from a term.
func ParseTermForm(raw string) TermForm {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, TermSubstringMarker) {
		return TermForm{Term: strings.TrimSpace(strings.TrimPrefix(raw, TermSubstringMarker)),
			Substring: true}
	}
	return TermForm{Term: raw}
}

// TermLayers lists every existing layer: the global file and every language file in termsDir.
func TermLayers(globalPath, termsDir string) []TermLayer {
	layers := []TermLayer{{Scope: ScopeGlobal, Path: globalPath, Terms: ReadTermLayer(globalPath)}}
	entries, err := os.ReadDir(termsDir)
	if err != nil {
		return layers
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txt") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(termsDir, name)
		layers = append(layers, TermLayer{
			Scope: strings.TrimSuffix(name, ".txt"), Path: path, Terms: ReadTermLayer(path),
		})
	}
	return layers
}

// TermPathFor resolves the write target of a scope.
func TermPathFor(globalPath, termsDir, scope string) string {
	if scope == "" || scope == ScopeGlobal {
		return globalPath
	}
	return languageTermsPath(termsDir, scope)
}

// NormalizeScope validates a pending request scope: global or a language id.
func NormalizeScope(raw string) (string, error) {
	scope := strings.TrimSpace(raw)
	if strings.EqualFold(scope, ScopeGlobal) {
		return ScopeGlobal, nil
	}
	if !model.ValidateLanguageTag(scope) {
		return "", &ServerError{Code: "terms.invalid-scope",
			Message: "the scope must be global or a language code such as de-DE"}
	}
	return scope, nil
}

// PendingTerm is one unconfirmed term-list change. Requests never touch a term file; only a
// confirmed commit does.
type PendingTerm struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Term    string `json:"term"`
	Reason  string `json:"reason"`
	Scope   string `json:"scope,omitempty"`
	Created string `json:"created"`
}

// ScopeOf is the effective scope of a pending request. An entry written before scopes existed is
// global.
func (entry PendingTerm) ScopeOf() string {
	if strings.TrimSpace(entry.Scope) == "" {
		return ScopeGlobal
	}
	return entry.Scope
}

// ReadPending reads the pending requests. A missing store is an empty list.
func ReadPending(path string) ([]PendingTerm, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []PendingTerm
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// AddPending records a term request and returns its pending ID. The term list stays untouched.
func AddPending(path, action, term, reason, scope string, now time.Time) (PendingTerm, error) {
	term = strings.TrimSpace(term)
	reason = strings.TrimSpace(reason)
	if action != "add" && action != "remove" {
		return PendingTerm{}, &ServerError{Code: "terms.invalid-action",
			Message: "the action must be add or remove"}
	}
	if term == "" {
		return PendingTerm{}, &ServerError{Code: "terms.invalid-term",
			Message: "a term is required"}
	}
	if reason == "" {
		return PendingTerm{}, &ServerError{Code: "terms.reason-required",
			Message: "a reason is required for a term request"}
	}
	normalized, scopeErr := NormalizeScope(scope)
	if scopeErr != nil {
		return PendingTerm{}, scopeErr
	}
	entries, err := ReadPending(path)
	if err != nil {
		return PendingTerm{}, err
	}
	entry := PendingTerm{
		ID:      pendingID(action, term, reason, normalized, now),
		Action:  action,
		Term:    term,
		Reason:  reason,
		Scope:   normalized,
		Created: now.UTC().Format(time.RFC3339),
	}
	entries = append(entries, entry)
	if err := writePending(path, entries); err != nil {
		return PendingTerm{}, err
	}
	return entry, nil
}

// CommitPending applies one confirmed request to the file of its scope and removes it. The term
// file is written first, so a crash leaves the request pending (a retry is idempotent) instead of
// losing it.
func CommitPending(pendingPath, globalPath, termsDir, id string) (PendingTerm, int, error) {
	entries, err := ReadPending(pendingPath)
	if err != nil {
		return PendingTerm{}, 0, err
	}
	index := -1
	for position, entry := range entries {
		if entry.ID == id {
			index = position
			break
		}
	}
	if index < 0 {
		return PendingTerm{}, 0, &ServerError{Code: "terms.unknown-request",
			Message: "no pending request with id " + id}
	}
	entry := entries[index]
	target := TermPathFor(globalPath, termsDir, entry.ScopeOf())
	terms := ReadTerms(target)
	switch entry.Action {
	case "add":
		terms = addTerm(terms, entry.Term)
	case "remove":
		terms = removeTerm(terms, entry.Term)
	default:
		return PendingTerm{}, 0, &ServerError{Code: "terms.invalid-action",
			Message: "unknown pending action " + entry.Action}
	}
	if err := WriteTerms(target, terms); err != nil {
		return PendingTerm{}, 0, err
	}
	entries = append(entries[:index], entries[index+1:]...)
	if err := writePending(pendingPath, entries); err != nil {
		return PendingTerm{}, 0, err
	}
	return entry, len(ReadTerms(target)), nil
}

// DiscardPending removes one request without touching a term file.
func DiscardPending(pendingPath, id string) (PendingTerm, error) {
	entries, err := ReadPending(pendingPath)
	if err != nil {
		return PendingTerm{}, err
	}
	index := -1
	for position, entry := range entries {
		if entry.ID == id {
			index = position
			break
		}
	}
	if index < 0 {
		return PendingTerm{}, &ServerError{Code: "terms.unknown-request",
			Message: "no pending request with id " + id}
	}
	entry := entries[index]
	entries = append(entries[:index], entries[index+1:]...)
	if err := writePending(pendingPath, entries); err != nil {
		return PendingTerm{}, err
	}
	return entry, nil
}

// WriteTerms writes a term list: one term per line, case-insensitive dedup, first form kept. The
// parent directory is created for a language layer.
func WriteTerms(path string, terms []string) error {
	seen := map[string]bool{}
	lines := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key := strings.ToLower(term)
		if seen[key] {
			continue
		}
		seen[key] = true
		lines = append(lines, term)
	}
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return store.WriteAtomic(path, []byte(content))
}

func writePending(path string, entries []PendingTerm) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteAtomic(path, append(data, '\n'))
}

func addTerm(terms []string, term string) []string {
	for _, existing := range terms {
		if strings.EqualFold(existing, term) {
			return terms
		}
	}
	return append(terms, term)
}

func removeTerm(terms []string, term string) []string {
	result := make([]string, 0, len(terms))
	for _, existing := range terms {
		if strings.EqualFold(existing, term) {
			continue
		}
		result = append(result, existing)
	}
	return result
}

// LayerInfos describes layers for a read-only summary.
func LayerInfos(layers []TermLayer) []TermLayerInfo {
	infos := make([]TermLayerInfo, 0, len(layers))
	for _, layer := range layers {
		infos = append(infos, layerInfo(layer))
	}
	return infos
}

func pendingID(action, term, reason, scope string, now time.Time) string {
	sum := sha256.Sum256([]byte(action + "\x00" + term + "\x00" + reason + "\x00" + scope +
		"\x00" + now.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:4])
}
