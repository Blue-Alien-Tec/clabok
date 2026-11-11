package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/refs"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
	"github.com/Blue-Alien-Tec/clabok/internal/textinput"
)

// descriptor is the export sidecar. Member hashes are informational: `export --check` uses them and
// `import --verify` enforces them.
type descriptor struct {
	Format          string            `json:"format"`
	Version         int               `json:"version"`
	Book            string            `json:"book"`
	CLABOKVersion   string            `json:"clabokVersion,omitempty"`
	ManifestVersion int               `json:"manifestVersion"`
	ExportedAt      string            `json:"exportedAt"`
	Members         map[string]string `json:"members"`
}

// runExport writes the text form of a closed book.
func runExport(env *cli.Env, bookRaw, out string, dryRun, check bool) (exportPayload, error) {
	context, err := openTransfer(env)
	if err != nil {
		return exportPayload{}, err
	}
	id, err := bookID(bookRaw)
	if err != nil {
		return exportPayload{}, err
	}
	if closedErr := context.Life.RequireClosed(id); closedErr != nil {
		return exportPayload{}, closedErr
	}
	paths, pathErr := context.Life.Paths(id)
	if pathErr != nil {
		return exportPayload{}, pathErr
	}
	container, loadErr := store.Load(paths.Container)
	if loadErr != nil {
		return exportPayload{}, fail("export.missing", "book %q has no container", id)
	}
	manifestData, ok := container.Read(manifestMember)
	if !ok {
		return exportPayload{}, fail("export.missing-manifest", "the container has no manifest")
	}
	manifest, decodeErr := model.Decode(manifestData)
	if decodeErr != nil {
		return exportPayload{}, decodeErr
	}
	members := make(map[string]string, container.Len())
	payload := exportPayload{
		DryRun: dryRun, Action: "export", Book: string(id), Dir: out, Members: container.Len(),
	}
	for _, name := range container.Names() {
		if err := validMemberName(name); err != nil {
			return payload, err
		}
		data, _ := container.Read(name)
		members[name] = hashBytes(data)
	}
	sidecar := descriptor{
		Format: exportFormat, Version: exportVersion, Book: string(id),
		ManifestVersion: manifest.Version, ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Members: members,
	}
	if check {
		existing, readErr := readDescriptor(out)
		if readErr != nil {
			return payload, readErr
		}
		actual, unreadable := hashExportMembers(out, existing.Members)
		missing, extra, changed := compareMembers(existing.Members, actual)
		missing = append(missing, unreadable...)
		bookMissing, bookExtra, bookChanged := compareMembers(existing.Members, members)
		missing = mergeNames(missing, bookMissing)
		extra = mergeNames(extra, bookExtra)
		changed = mergeNames(changed, bookChanged)
		match := len(missing)+len(extra)+len(changed) == 0
		payload.Match = &match
		payload.Missing, payload.Extra, payload.Changed = missing, extra, changed
		if !match {
			return payload, fail("export.drift", "the directory differs from the book")
		}
		return payload, nil
	}
	if dryRun {
		return payload, nil
	}
	if entries, readDir := os.ReadDir(out); readDir == nil && len(entries) > 0 {
		if _, hasSidecar := sidecarMember(out); !hasSidecar {
			return payload, fail("export.target-exists",
				"%s holds files but no export sidecar; pick another directory", out)
		}
	}
	if err := writeExport(out, sidecar, container); err != nil {
		return payload, err
	}
	return payload, nil
}

func writeExport(out string, sidecar descriptor, container *store.Container) error {
	temporary := out + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.MkdirAll(temporary, 0o755); err != nil {
		return fail("export.write-failed", "%v", err)
	}
	for _, name := range container.Names() {
		data, _ := container.Read(name)
		target := filepath.Join(temporary, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fail("export.write-failed", "%v", err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fail("export.write-failed", "%v", err)
		}
	}
	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return fail("export.write-failed", "%v", err)
	}
	if err := os.WriteFile(filepath.Join(temporary, descriptorMember), append(data, '\n'),
		0o644); err != nil {
		return fail("export.write-failed", "%v", err)
	}
	if err := os.RemoveAll(out); err != nil {
		return fail("export.write-failed", "%v", err)
	}
	if err := os.Rename(temporary, out); err != nil {
		return fail("export.write-failed", "%v", err)
	}
	return nil
}

// readDescriptor reads and validates the export sidecar.
func readDescriptor(directory string) (descriptor, error) {
	data, err := os.ReadFile(filepath.Join(directory, descriptorMember))
	if err != nil {
		return descriptor{}, fail("import.bad-format",
			"%s has no %s sidecar", directory, descriptorMember)
	}
	var sidecar descriptor
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return descriptor{}, fail("import.bad-format", "%s is not valid JSON: %v",
			descriptorMember, err)
	}
	if sidecar.Format != exportFormat || sidecar.Version != exportVersion {
		return descriptor{}, fail("import.bad-format",
			"unsupported export format %q version %d", sidecar.Format, sidecar.Version)
	}
	return sidecar, nil
}

// readExport reads the descriptor and every member of a text form. Members are decoded to UTF-8;
// normalized reports whether any member changed in the process.
func readExport(directory string, verify bool) (descriptor, map[string][]byte, bool, error) {
	sidecar, err := readDescriptor(directory)
	if err != nil {
		return descriptor{}, nil, false, err
	}
	members := make(map[string][]byte, len(sidecar.Members))
	normalized := false
	for name, expected := range sidecar.Members {
		if err := validMemberName(name); err != nil {
			return descriptor{}, nil, false, err
		}
		data, readErr := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if readErr != nil {
			return descriptor{}, nil, false, fail("import.member-missing",
				"the export names %s but the file is missing", name)
		}
		if verify && hashBytes(data) != expected {
			return descriptor{}, nil, false, fail("import.member-changed",
				"%s does not match the export hash; pass without --verify for a hand-edited export",
				name)
		}
		decoded, decodeErr := textinput.Decode(data)
		if decodeErr != nil {
			return descriptor{}, nil, false, fmt.Errorf("member %s: %w", name, decodeErr)
		}
		if !bytes.Equal(decoded, data) {
			normalized = true
		}
		members[name] = decoded
	}
	if _, ok := members[manifestMember]; !ok {
		return descriptor{}, nil, false, fail("import.missing-manifest",
			"the export carries no %s", manifestMember)
	}
	return sidecar, members, normalized, nil
}

// runImport creates a book from a text form with fresh IDs.
func runImport(env *cli.Env, from, as, requestedPath string, library, verify bool) (importPayload, error) {
	_, members, normalized, err := readExport(from, verify)
	if err != nil {
		return importPayload{}, err
	}
	manifest, decodeErr := model.Decode(members[manifestMember])
	if decodeErr != nil {
		return importPayload{}, decodeErr
	}
	if validateErr := manifest.Validate(); validateErr != nil {
		return importPayload{}, validateErr
	}
	target, err := bookID(as)
	if err != nil {
		return importPayload{}, err
	}
	context, err := openTransfer(env)
	if err != nil {
		return importPayload{}, err
	}
	registry, registryErr := store.LoadRegistry(context.Layout.Registry())
	if registryErr != nil {
		return importPayload{}, registryErr
	}
	if _, exists := registry.FindFold(target); exists {
		return importPayload{}, fail("import.book-exists",
			"book %q is registered; import never overwrites", target)
	}
	if library {
		if existing, hasLibrary := registry.Library(); hasLibrary {
			return importPayload{}, fail("library.library-exists",
				"the library already has the book %q", existing.ID)
		}
	}
	output, pathErr := context.outputPath(target, requestedPath, library)
	if pathErr != nil {
		return importPayload{}, pathErr
	}
	source := manifest.ID
	ids := sortedIDs(manifestIDSet(manifest))
	payload := importPayload{
		DryRun: env.Globals.DryRun, Action: "import", Book: string(target), Source: string(source),
		IDCount: len(ids), Members: len(members), Renamed: !sameBook(source, target),
		Normalized: normalized, Inbound: "unverified",
	}
	var ledgerSnapshot []byte
	if !env.Globals.DryRun {
		ledgerSnapshot, err = context.Allocator.SnapshotLedger()
		if err != nil {
			return payload, err
		}
	}
	base, err := context.allocate(env.Globals.DryRun, len(ids))
	if err != nil {
		return payload, err
	}
	committed := false
	defer func() {
		if !env.Globals.DryRun && !committed {
			_ = context.Allocator.RestoreLedger(ledgerSnapshot)
		}
	}()
	mapping := make(map[model.ID]model.ID, len(ids))
	for index, old := range ids {
		mapping[old] = base + model.ID(index)
	}
	for index, old := range ids {
		if index >= 5 {
			break
		}
		payload.IDs = append(payload.IDs, idPair{From: old.String(),
			To: mapping[old].String()})
	}
	otherIDs, otherBooks, err := context.otherIDs(registry, target)
	if err != nil {
		return payload, err
	}
	if err := rewriteManifest(manifest, source, target, mapping); err != nil {
		return payload, err
	}
	manifest.ID = target
	if err := verifyReferences(manifest, mapping, otherIDs, otherBooks); err != nil {
		return payload, err
	}
	encoded, encodeErr := manifest.Encode()
	if encodeErr != nil {
		return payload, encodeErr
	}
	if validateErr := manifest.Validate(); validateErr != nil {
		return payload, validateErr
	}
	rewritten := make(map[string][]byte, len(members))
	for name, data := range members {
		if name == manifestMember {
			rewritten[name] = encoded
			continue
		}
		newName, renameErr := renamedMember(name, mapping)
		if renameErr != nil {
			return payload, renameErr
		}
		if strings.HasSuffix(newName, ".md") {
			rewritten[newName], err = rewriteBodyRefs(data, mapping, otherIDs)
			if err != nil {
				return payload, err
			}
		} else {
			rewritten[newName] = data
		}
	}
	container := store.New()
	names := make([]string, 0, len(rewritten))
	for name := range rewritten {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if writeErr := container.Write(name, rewritten[name]); writeErr != nil {
			return payload, writeErr
		}
	}
	if env.Globals.DryRun {
		return payload, nil
	}
	containerData, marshalErr := container.Marshal()
	if marshalErr != nil {
		return payload, marshalErr
	}
	receiptPath, receiptErr := context.commitImport(target, output, library, containerData,
		len(members), source, normalized)
	if receiptErr != nil {
		return payload, receiptErr
	}
	payload.Receipt = receiptPath
	committed = true
	return payload, nil
}

// allocate reserves the ID range. A dry run only peeks, so the ledger is untouched.
func (context *transferContext) allocate(dryRun bool, count int) (model.ID, error) {
	if count == 0 {
		count = 1
	}
	if dryRun {
		return context.Allocator.Peek()
	}
	return context.Allocator.Reserve(count)
}

// commitImport writes the container and registers the book, restoring the ledger and the registry
// when a later step fails.
func (context *transferContext) commitImport(target model.BookID, output string, library bool,
	containerData []byte, members int, source model.BookID, normalized bool) (string, error) {
	release, lockErr := context.Life.LockStore()
	if lockErr != nil {
		return "", lockErr
	}
	defer func() { _ = release() }()
	paths, err := context.Layout.Book(target)
	if err != nil {
		return "", err
	}
	registryData, _ := os.ReadFile(context.Layout.Registry())
	registry, registryErr := store.LoadRegistry(context.Layout.Registry())
	if registryErr != nil {
		return "", registryErr
	}
	if err := store.WriteAtomic(paths.Container, containerData); err != nil {
		return "", err
	}
	kind := store.KindBook
	if library {
		kind = store.KindLibrary
	}
	registry.Upsert(store.RegistryEntry{ID: target, Path: output, Kind: kind})
	if saveErr := registry.Save(context.Layout.Registry()); saveErr != nil {
		_ = os.RemoveAll(paths.Dir)
		if registryData != nil {
			_ = store.WriteAtomic(context.Layout.Registry(), registryData)
		}
		return "", saveErr
	}
	return context.writeReceipt(map[string]any{
		"format": exportFormat, "version": exportVersion, "sourceBook": string(source),
		"targetBook": string(target), "members": members, "normalized": normalized,
	})
}

// otherIDs collects the IDs and book ids of every other registered book.
func (context *transferContext) otherIDs(registry *store.Registry,
	exclude model.BookID) (map[model.ID]bool, map[string]bool, error) {
	ids := map[model.ID]bool{}
	books := map[string]bool{}
	for _, entry := range registry.Books {
		if sameBook(entry.ID, exclude) {
			continue
		}
		books[string(entry.ID)] = true
		manifest, err := readManifest(context.Life, context.Layout, entry.ID)
		if err != nil {
			return nil, nil, err
		}
		for id := range manifestIDSet(manifest) {
			ids[id] = true
		}
	}
	return ids, books, nil
}

func validMemberName(name string) error {
	cleaned := path.Clean(filepath.ToSlash(name))
	if cleaned != filepath.ToSlash(name) || strings.HasPrefix(cleaned, "../") ||
		strings.HasPrefix(cleaned, "/") {
		return fail("import.unexpected-member", "member %q is not a safe relative path", name)
	}
	if cleaned == manifestMember || cleaned == descriptorMember {
		return nil
	}
	if strings.HasPrefix(cleaned, "blocks/") && strings.HasSuffix(cleaned, ".md") {
		return nil
	}
	if strings.HasPrefix(cleaned, "layouts/") {
		return nil
	}
	return fail("import.unexpected-member", "member %q is not part of the text form", name)
}

func renamedMember(name string, mapping map[model.ID]model.ID) (string, error) {
	if name == manifestMember {
		return name, nil
	}
	directory, base := path.Split(name)
	extension := path.Ext(base)
	stem := strings.TrimSuffix(base, extension)
	id, err := model.ParseID(stem)
	if err != nil {
		return "", fail("import.unexpected-member", "member %q has no numeric name", name)
	}
	mapped, ok := mapping[id]
	if !ok {
		return "", fail("import.unresolved-member", "member %q has no mapping", name)
	}
	return directory + mapped.String() + extension, nil
}

// rewriteManifest maps every ID and in-book reference.
func rewriteManifest(manifest *model.Manifest, source, target model.BookID,
	mapping map[model.ID]model.ID) error {
	remap := func(id model.ID) (model.ID, error) {
		if id.IsZero() {
			return model.ZeroID, nil
		}
		mapped, ok := mapping[id]
		if !ok {
			return model.ZeroID, fail("import.unresolved-ref",
				"the ID %s has no mapping", id)
		}
		return mapped, nil
	}
	nodes := []*model.Node{manifest.Root}
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		if node != manifest.Root {
			nodes = append(nodes, node)
		}
		return true
	})
	for _, node := range nodes {
		mapped, err := remap(node.ID)
		if err != nil {
			return err
		}
		node.ID = mapped
		for index := range node.Blocks {
			block, blockErr := remap(node.Blocks[index])
			if blockErr != nil {
				return blockErr
			}
			node.Blocks[index] = block
		}
		if !node.ProseBlock.IsZero() {
			prose, proseErr := remap(node.ProseBlock)
			if proseErr != nil {
				return proseErr
			}
			node.ProseBlock = prose
		}
		if node.Ref == nil || !node.Ref.Valid() {
			continue
		}
		if node.Ref.Book == source {
			node.Ref.Book = target
		}
		if node.Ref.Book == target || node.Ref.Book == "" {
			nodeMapped, nodeErr := remap(node.Ref.Node)
			if nodeErr != nil {
				return nodeErr
			}
			blockMapped, blockErr := remap(node.Ref.Block)
			if blockErr != nil {
				return blockErr
			}
			node.Ref.Node, node.Ref.Block = nodeMapped, blockMapped
		}
	}
	meta := make(map[model.ID]model.BlockMeta, len(manifest.BlockMeta))
	for oldID, value := range manifest.BlockMeta {
		mapped, err := remap(oldID)
		if err != nil {
			return err
		}
		meta[mapped] = value
	}
	manifest.BlockMeta = meta
	for _, region := range manifest.Regions {
		mapped, err := remap(region.ID)
		if err != nil {
			return err
		}
		region.ID = mapped
		if region.Target.Kind != model.TargetBook {
			targetID, targetErr := remap(region.Target.ID)
			if targetErr != nil {
				return targetErr
			}
			region.Target.ID = targetID
		}
		for index := range region.Blocks {
			block, blockErr := remap(region.Blocks[index])
			if blockErr != nil {
				return blockErr
			}
			region.Blocks[index] = block
		}
	}
	return nil
}

// verifyReferences refuses an unresolvable cross-book link. In-book references were remapped by
// rewriteManifest; an inline reference is checked by rewriteBodyRefs.
func verifyReferences(manifest *model.Manifest, mapping map[model.ID]model.ID,
	otherIDs map[model.ID]bool, otherBooks map[string]bool) error {
	var failure error
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		if node.Ref == nil || !node.Ref.Valid() {
			return true
		}
		if node.Ref.Book == "" || node.Ref.Book == manifest.ID {
			return true
		}
		if !otherBooks[string(node.Ref.Book)] {
			failure = fail("import.unresolved-ref",
				"link %s names the unregistered book %q", node.ID, node.Ref.Book)
			return false
		}
		if !node.Ref.Node.IsZero() && !otherIDs[node.Ref.Node] {
			failure = fail("import.unresolved-ref",
				"link %s names the missing node %s in %q", node.ID, node.Ref.Node, node.Ref.Book)
			return false
		}
		if !node.Ref.Block.IsZero() && !otherIDs[node.Ref.Block] {
			failure = fail("import.unresolved-ref",
				"link %s names the missing block %s in %q", node.ID, node.Ref.Block,
				node.Ref.Book)
			return false
		}
		return true
	})
	return failure
}

// rewriteBodyRefs maps every inline ID reference and refuses a reference that resolves nowhere.
func rewriteBodyRefs(body []byte, mapping map[model.ID]model.ID,
	otherIDs map[model.ID]bool) ([]byte, error) {
	text := string(body)
	if !strings.Contains(text, "[[id:") {
		return body, nil
	}
	var builder strings.Builder
	last := 0
	for _, match := range refs.Pattern.FindAllStringSubmatchIndex(text, -1) {
		start, end := match[0], match[1]
		idText := text[match[2]:match[3]]
		id, err := model.ParseID(idText)
		if err != nil {
			continue
		}
		mapped, ok := mapping[id]
		if !ok {
			if !otherIDs[id] {
				return nil, fail("import.unresolved-ref",
					"the body refers to the ID %s, which resolves nowhere in the target store", id)
			}
			mapped = id
		}
		builder.WriteString(text[last:start])
		builder.WriteString("[[id:" + mapped.String() + "]]")
		last = end
	}
	builder.WriteString(text[last:])
	return []byte(builder.String()), nil
}

func sortedIDs(set map[model.ID]bool) []model.ID {
	ids := make([]model.ID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// hashExportMembers hashes the files named by the descriptor. Unreadable files are reported as
// missing.
func hashExportMembers(directory string, names map[string]string) (map[string]string, []string) {
	hashes := make(map[string]string, len(names))
	missing := make([]string, 0)
	for name := range names {
		data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			missing = append(missing, name)
			continue
		}
		hashes[name] = hashBytes(data)
	}
	return hashes, missing
}

// mergeNames unions two name lists and sorts them.
func mergeNames(left, right []string) []string {
	seen := map[string]bool{}
	merged := make([]string, 0, len(left)+len(right))
	for _, name := range append(append([]string(nil), left...), right...) {
		if seen[name] {
			continue
		}
		seen[name] = true
		merged = append(merged, name)
	}
	sort.Strings(merged)
	return merged
}

func compareMembers(expected, actual map[string]string) (missing, extra, changed []string) {
	for name, hash := range expected {
		value, ok := actual[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		if value != hash {
			changed = append(changed, name)
		}
	}
	for name := range actual {
		if _, ok := expected[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(changed)
	return missing, extra, changed
}

func sidecarMember(directory string) (string, bool) {
	path := filepath.Join(directory, descriptorMember)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}
