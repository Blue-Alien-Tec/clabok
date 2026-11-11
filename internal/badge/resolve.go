package badge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// Resolver resolves badges against one project. External sources run only when AllowExternal is set,
// which `badge check` does; render and sync never set it.
type Resolver struct {
	ProjectDir    string
	Detectors     map[string]Detector
	CacheDir      string
	AllowExternal bool
	Now           func() time.Time
}

// Resolve returns the concrete value of one declared badge.
func (resolver Resolver) Resolve(declared model.Badge) (Value, bool, string) {
	source := declared.Source
	if strings.TrimSpace(source) == "" {
		source = declared.ID
	}
	builtin, hasDescriptor := descriptors[source]
	if !hasDescriptor {
		builtin = descriptors[declared.ID]
	}
	if declared.Message != "" {
		return resolver.present(declared, source, resolver.Detectors[source], builtin,
			Value{}), true, ""
	}
	if hasDescriptor {
		value, found, reason := detectors[source](resolver.ProjectDir)
		if !found {
			return Value{}, false, reason
		}
		return resolver.present(declared, source, Detector{}, builtin, value), true, ""
	}
	detector, exists := resolver.Detectors[source]
	if !exists {
		return Value{}, false, fmt.Sprintf(
			"badge %q has no detector %q and no message", declared.ID, source)
	}
	value, found, reason := resolver.resolveDetector(source, detector, declared.CacheSeconds)
	if !found {
		return Value{}, false, reason
	}
	return resolver.present(declared, source, detector, descriptor{}, value), true, ""
}

// present merges explicit badge fields with the detector defaults.
func (resolver Resolver) present(declared model.Badge, source string, detector Detector,
	builtin descriptor, value Value) Value {
	value.ID = declared.ID
	value.Detector = source
	value.Label = firstNonEmpty(declared.Label, detector.Label, builtin.label, source)
	value.Color = firstNonEmpty(declared.Color, detector.Color, builtin.color, "lightgrey")
	if declared.Link != "" {
		value.Link = declared.Link
	}
	value.Style = firstNonEmpty(declared.Style, detector.Style, "flat-square")
	value.LogoColor = declared.LogoColor
	value.LabelColor = declared.LabelColor
	value.Logo = firstNonEmpty(declared.Logo, detector.Logo)
	value.Alt = declared.Alt
	value.Required = declared.Required
	value.Order = declared.Order
	value.Group = declared.Group
	if declared.Message != "" {
		value.Message = declared.Message
		value.Source = "explicit"
	}
	return value
}

// ResolveAll resolves the declared badges in declaration order.
func (resolver Resolver) ResolveAll(badges []model.Badge) ([]Value, []Unresolved) {
	values := make([]Value, 0, len(badges))
	unresolved := make([]Unresolved, 0)
	for _, declared := range badges {
		value, ok, reason := resolver.Resolve(declared)
		if ok {
			values = append(values, value)
			continue
		}
		unresolved = append(unresolved, Unresolved{ID: declared.ID, Reason: reason})
	}
	return values, unresolved
}

func (resolver Resolver) resolveDetector(id string, detector Detector, cacheSeconds int) (Value, bool, string) {
	switch detector.Kind {
	case KindFile:
		return resolver.resolveFile(id, detector)
	case KindExists:
		matches, err := resolver.matches(detector.Path)
		if err != nil {
			return Value{}, false, err.Error()
		}
		if len(matches) == 0 {
			return Value{Message: "no", Source: detector.Path}, true, ""
		}
		return Value{Message: "yes", Source: relativeName(resolver.ProjectDir, matches[0])}, true, ""
	case KindCount:
		matches, err := resolver.matches(detector.Path)
		if err != nil {
			return Value{}, false, err.Error()
		}
		return Value{Message: strconv.Itoa(len(matches)), Source: detector.Path}, true, ""
	case KindPackage:
		return resolver.resolvePackage(id, detector)
	case KindGit:
		return resolver.resolveGit(detector)
	case KindBuild:
		if detector.Query != "version" {
			return Value{}, false, "unknown build query"
		}
		return resolver.builtinVersion()
	case KindStatic:
		return Value{Message: detector.Value, Source: "config"}, true, ""
	case KindHTTP:
		return Value{URL: detector.URL, Source: "config", Dynamic: true}, true, ""
	case KindCommand:
		return resolver.resolveCommand(id, detector, cacheSeconds)
	}
	return Value{}, false, "unknown detector kind"
}

func (resolver Resolver) builtinVersion() (Value, bool, string) {
	return versionValue(resolver.ProjectDir)
}

func (resolver Resolver) resolveFile(id string, detector Detector) (Value, bool, string) {
	matches, err := resolver.matches(detector.Path)
	if err != nil {
		return Value{}, false, err.Error()
	}
	if len(matches) == 0 {
		return Value{}, false, fmt.Sprintf("%s is missing", detector.Path)
	}
	path := matches[0]
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return Value{}, false, fmt.Sprintf("%s cannot be read", detector.Path)
	}
	format := detector.Format
	if format == "" {
		format = FormatText
	}
	var message string
	switch format {
	case FormatText:
		message = cutMessage(strings.TrimSpace(string(data)))
	case FormatRegex:
		pattern, compileErr := compilePattern(detector)
		if compileErr != nil {
			return Value{}, false, compileErr.Error()
		}
		match := pattern.FindStringSubmatch(string(data))
		if match == nil {
			return Value{}, false, fmt.Sprintf("%s does not match %s", detector.Path, detector.Pattern)
		}
		message = match[0]
		if len(match) > 1 {
			message = match[1]
		}
	case FormatJSON, FormatYAML, FormatTOML, FormatXML:
		parsed, parseErr := parseStructured(format, data)
		if parseErr != nil {
			return Value{}, false, fmt.Sprintf("%s: %v", detector.Path, parseErr)
		}
		found, ok := queryValue(parsed, detector.Query)
		if !ok {
			return Value{}, false, fmt.Sprintf("%s has no %s", detector.Path, detector.Query)
		}
		message = formatValue(found)
	default:
		return Value{}, false, "unknown file format"
	}
	if mapped, ok := detector.Map[message]; ok {
		message = mapped
	}
	message = cutMessage(detector.Prefix + message + detector.Suffix)
	if message == "" {
		return Value{}, false, fmt.Sprintf("%s resolved to an empty value", detector.Path)
	}
	return Value{Message: message, Source: relativeName(resolver.ProjectDir, path)}, true, ""
}

func (resolver Resolver) resolvePackage(id string, detector Detector) (Value, bool, string) {
	var declared Detector
	switch detector.Ecosystem {
	case "node":
		declared = Detector{Kind: KindFile, Path: "package.json", Format: FormatJSON,
			Query: detector.Field}
	case "dotnet":
		declared = Detector{Kind: KindFile, Path: "*.csproj", Format: FormatXML,
			Query: "Project/PropertyGroup/" + detector.Field}
	case "rust":
		query := detector.Field
		if !strings.Contains(query, ".") {
			query = "package." + query
		}
		declared = Detector{Kind: KindFile, Path: "Cargo.toml", Format: FormatTOML, Query: query}
	case "go":
		return resolver.resolveGoField(detector.Field)
	}
	declared.Map = detector.Map
	declared.Prefix = detector.Prefix
	declared.Suffix = detector.Suffix
	return resolver.resolveFile(id, declared)
}

func (resolver Resolver) resolveGoField(field string) (Value, bool, string) {
	text, ok := readText(resolver.ProjectDir, "go.mod")
	if !ok {
		return Value{}, false, "go.mod is missing"
	}
	prefix := field + " "
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return Value{Message: strings.TrimSpace(strings.TrimPrefix(line, prefix)),
				Source: "go.mod"}, true, ""
		}
	}
	return Value{}, false, "go.mod has no " + field + " directive"
}

func (resolver Resolver) resolveGit(detector Detector) (Value, bool, string) {
	if detector.Query == "branch" {
		text, ok := readText(resolver.ProjectDir, filepath.Join(".git", "HEAD"))
		if !ok {
			return Value{}, false, ".git/HEAD is missing"
		}
		head := strings.TrimSpace(text)
		if strings.HasPrefix(head, "ref: refs/heads/") {
			return Value{Message: strings.TrimPrefix(head, "ref: refs/heads/"),
				Source: ".git/HEAD"}, true, ""
		}
		return Value{}, false, ".git/HEAD holds no branch"
	}
	value, ok, reason := remoteURL(resolver.ProjectDir)
	return value, ok, reason
}

func (resolver Resolver) resolveCommand(id string, detector Detector, cacheSeconds int) (Value, bool, string) {
	if !resolver.AllowExternal {
		return Value{}, false,
			"the command source runs only in `badge check`"
	}
	cache := resolver.readCommandCache(id, detector.Command)
	ttl := time.Duration(cacheSeconds) * time.Second
	if cache != nil && ttl > 0 && resolver.now().Sub(cache.Fetched) < ttl {
		return Value{Message: cache.Value, Source: "cache", Dynamic: true}, true, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, detector.Command[0], detector.Command[1:]...)
	command.Dir = resolver.ProjectDir
	output, err := command.Output()
	if err != nil {
		return Value{}, false, fmt.Sprintf("the command %q failed", detector.Command[0])
	}
	message := cutMessage(collapse(string(output)))
	if message == "" {
		return Value{}, false, "the command produced no output"
	}
	resolver.writeCommandCache(id, detector.Command, message)
	return Value{Message: message, Source: "command", Dynamic: true}, true, ""
}

type commandCache struct {
	Argv    string    `json:"argv"`
	Value   string    `json:"value"`
	Fetched time.Time `json:"fetched"`
}

func (resolver Resolver) commandCachePath(id string) string {
	return filepath.Join(resolver.CacheDir, "badges", safeName(id)+".json")
}

func (resolver Resolver) readCommandCache(id string, argv []string) *commandCache {
	data, err := os.ReadFile(resolver.commandCachePath(id))
	if err != nil {
		return nil
	}
	var cache commandCache
	if json.Unmarshal(data, &cache) != nil || cache.Argv != argvHash(argv) {
		return nil
	}
	return &cache
}

func (resolver Resolver) writeCommandCache(id string, argv []string, value string) {
	path := resolver.commandCachePath(id)
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	data, err := json.Marshal(commandCache{Argv: argvHash(argv), Value: value,
		Fetched: resolver.now().UTC()})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func resolverNow(resolver Resolver) time.Time {
	if resolver.Now != nil {
		return resolver.Now()
	}
	return time.Now()
}

func (resolver Resolver) now() time.Time { return resolverNow(resolver) }

func argvHash(argv []string) string {
	sum := sha256.Sum256([]byte(strings.Join(argv, "\x00")))
	return hex.EncodeToString(sum[:8])
}

func safeName(id string) string {
	return strings.Map(func(runeValue rune) rune {
		switch {
		case runeValue >= 'a' && runeValue <= 'z',
			runeValue >= 'A' && runeValue <= 'Z',
			runeValue >= '0' && runeValue <= '9',
			runeValue == '.', runeValue == '_', runeValue == '-':
			return runeValue
		}
		return '_'
	}, id)
}

// matches resolves a path or glob inside the project. Matches are ordered lexicographically.
func (resolver Resolver) matches(pattern string) ([]string, error) {
	if !safeRelative(pattern) {
		return nil, fmt.Errorf("path %q leaves the project", pattern)
	}
	slashed := filepath.ToSlash(pattern)
	if !strings.ContainsAny(slashed, "*?[") {
		path := filepath.Join(resolver.ProjectDir, filepath.FromSlash(slashed))
		if _, err := os.Stat(path); err != nil {
			return nil, nil
		}
		return []string{path}, nil
	}
	expression, err := globRegexp(slashed)
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0)
	walkErr := filepath.WalkDir(resolver.ProjectDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, relErr := filepath.Rel(resolver.ProjectDir, path)
		if relErr != nil {
			return nil
		}
		if expression.MatchString(filepath.ToSlash(relative)) {
			matches = append(matches, path)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(matches)
	return matches, nil
}

// globRegexp converts a slash glob to a regular expression. `**` crosses directories.
func globRegexp(pattern string) (*regexp.Regexp, error) {
	var builder strings.Builder
	builder.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		character := pattern[index]
		switch character {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				index++
				if index+1 < len(pattern) && pattern[index+1] == '/' {
					index++
					builder.WriteString("(?:.*/)?")
					continue
				}
				builder.WriteString(".*")
				continue
			}
			builder.WriteString("[^/]*")
		case '?':
			builder.WriteString("[^/]")
		default:
			builder.WriteString(regexp.QuoteMeta(string(character)))
		}
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

func parseStructured(format string, data []byte) (any, error) {
	switch format {
	case FormatJSON:
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	case FormatYAML:
		var value any
		if err := yaml.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		return value, nil
	case FormatTOML:
		var value any
		if err := toml.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		return value, nil
	case FormatXML:
		return parseXML(data)
	}
	return nil, fmt.Errorf("unknown format %q", format)
}

type xmlNode struct {
	name     string
	text     string
	children []*xmlNode
}

func parseXML(data []byte) (*xmlNode, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var root *xmlNode
	stack := make([]*xmlNode, 0)
	for {
		token, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			node := &xmlNode{name: element.Name.Local}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, node)
			} else if root == nil {
				root = node
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) > 0 {
				text := strings.TrimSpace(string(element))
				if text != "" {
					node := stack[len(stack)-1]
					if node.text != "" {
						node.text += " "
					}
					node.text += text
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("the document is empty")
	}
	return root, nil
}

// queryValue walks a dotted path through maps, slices and XML nodes. A segment may carry an [n]
// index suffix.
func queryValue(value any, query string) (any, bool) {
	if strings.TrimSpace(query) == "" {
		return value, true
	}
	current := value
	separator := "."
	if strings.Contains(query, "/") {
		separator = "/"
	}
	for _, segment := range strings.Split(query, separator) {
		name := segment
		index := -1
		if open := strings.Index(segment, "["); open >= 0 && strings.HasSuffix(segment, "]") {
			name = segment[:open]
			parsed, err := strconv.Atoi(segment[open+1 : len(segment)-1])
			if err != nil {
				return nil, false
			}
			index = parsed
		}
		if name != "" {
			var ok bool
			current, ok = queryKey(current, name)
			if !ok {
				return nil, false
			}
		}
		if index >= 0 {
			var ok bool
			current, ok = queryIndex(current, index)
			if !ok {
				return nil, false
			}
		}
	}
	return current, true
}

func queryKey(value any, key string) (any, bool) {
	switch node := value.(type) {
	case map[string]any:
		result, ok := node[key]
		return result, ok
	case map[any]any:
		result, ok := node[key]
		return result, ok
	case *xmlNode:
		if node.name == key {
			return node, true
		}
		for _, child := range node.children {
			if child.name == key {
				if len(child.children) == 0 {
					return child.text, true
				}
				return child, true
			}
		}
		return nil, false
	}
	return nil, false
}

func queryIndex(value any, index int) (any, bool) {
	switch node := value.(type) {
	case []any:
		if index < 0 || index >= len(node) {
			return nil, false
		}
		return node[index], true
	case *xmlNode:
		if index < 0 || index >= len(node.children) {
			return nil, false
		}
		return node.children[index], true
	}
	return nil, false
}

func formatValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	case nil:
		return ""
	}
	return fmt.Sprint(value)
}

func cutMessage(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 64 {
		return text
	}
	return strings.TrimSpace(text[:64])
}

func collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func relativeName(projectDir, path string) string {
	relative, err := filepath.Rel(projectDir, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
