package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

// runDump copies the container of a closed book byte for byte.
func runDump(env *cli.Env, bookRaw, out string, check bool) (dumpPayload, error) {
	context, err := openTransfer(env)
	if err != nil {
		return dumpPayload{}, err
	}
	id, err := bookID(bookRaw)
	if err != nil {
		return dumpPayload{}, err
	}
	if closedErr := context.Life.RequireClosed(id); closedErr != nil {
		return dumpPayload{}, closedErr
	}
	paths, err := context.Life.Paths(id)
	if err != nil {
		return dumpPayload{}, err
	}
	data, readErr := os.ReadFile(paths.Container)
	if readErr != nil {
		return dumpPayload{}, fail("dump.missing", "book %q has no container", id)
	}
	sum := sha256.Sum256(data)
	payload := dumpPayload{
		DryRun: env.Globals.DryRun, Action: "dump", Book: string(id), File: out,
		Bytes: len(data), SHA256: hex.EncodeToString(sum[:]),
	}
	if check {
		existing, readExisting := os.ReadFile(out)
		if readExisting != nil {
			return payload, fail("dump.missing", "no artifact at %s", out)
		}
		match := bytes.Equal(existing, data)
		payload.Match = &match
		if !match {
			return payload, fail("dump.differs", "the artifact %s differs from the container", out)
		}
		return payload, nil
	}
	if err := store.WriteAtomic(out, data); err != nil {
		return payload, err
	}
	return payload, nil
}

// runRestore writes a container artifact into the store and registers the book.
func runRestore(env *cli.Env, file, as, requestedPath string, library, force bool) (restorePayload, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return restorePayload{}, fail("restore.missing", "no artifact at %s", file)
	}
	container, parseErr := store.Parse(data)
	if parseErr != nil {
		return restorePayload{}, parseErr
	}
	manifestData, ok := container.Read(manifestMember)
	if !ok {
		return restorePayload{}, fail("restore.missing-manifest",
			"the artifact carries no %s", manifestMember)
	}
	manifest, decodeErr := model.Decode(manifestData)
	if decodeErr != nil {
		return restorePayload{}, decodeErr
	}
	if validateErr := manifest.Validate(); validateErr != nil {
		return restorePayload{}, validateErr
	}
	target, err := bookID(as)
	if err != nil {
		return restorePayload{}, err
	}
	context, err := openTransfer(env)
	if err != nil {
		return restorePayload{}, err
	}
	if closedErr := context.Life.RequireClosed(target); closedErr != nil {
		return restorePayload{}, closedErr
	}
	registry, registryErr := store.LoadRegistry(context.Layout.Registry())
	if registryErr != nil {
		return restorePayload{}, registryErr
	}
	_, exists := registry.FindFold(target)
	if exists && !force {
		return restorePayload{}, fail("restore.book-exists",
			"book %q is registered; pass --force to replace it", target)
	}
	if library {
		if existing, hasLibrary := registry.Library(); hasLibrary &&
			!sameBook(existing.ID, target) {
			return restorePayload{}, fail("library.library-exists",
				"the library already has the book %q", existing.ID)
		}
	}
	artifactIDs := manifestIDSet(manifest)
	if err := context.checkCollisions(registry, target, artifactIDs); err != nil {
		return restorePayload{}, err
	}
	output, pathErr := context.outputPath(target, requestedPath, library)
	if pathErr != nil {
		return restorePayload{}, pathErr
	}
	kind := store.KindBook
	if library {
		kind = store.KindLibrary
	}
	payload := restorePayload{
		DryRun: env.Globals.DryRun, Action: "restore", Book: string(target), File: file,
		Version: manifest.Version, Replaced: exists,
	}
	if env.Globals.DryRun {
		return payload, nil
	}
	release, lockErr := context.Life.LockStore()
	if lockErr != nil {
		return payload, lockErr
	}
	defer func() { _ = release() }()
	paths, pathErr := context.Layout.Book(target)
	if pathErr != nil {
		return payload, pathErr
	}
	previous, previousErr := os.ReadFile(paths.Container)
	hadPrevious := previousErr == nil
	if exists && force {
		if _, snapErr := context.Life.SnapshotContainer(target, "restore"); snapErr != nil {
			return payload, snapErr
		}
	}
	if err := context.Allocator.EnsureAbove(highestID(manifest)); err != nil {
		return payload, err
	}
	if err := store.WriteAtomic(paths.Container, data); err != nil {
		return payload, err
	}
	registry.Upsert(store.RegistryEntry{ID: target, Path: output, Kind: kind})
	if saveErr := registry.Save(context.Layout.Registry()); saveErr != nil {
		if hadPrevious {
			_ = store.WriteAtomic(paths.Container, previous)
		} else {
			_ = os.RemoveAll(paths.Dir)
		}
		return payload, saveErr
	}
	receiptPath, receiptErr := context.writeReceipt(map[string]any{
		"format": "clabok-container", "version": 1, "targetBook": string(target),
		"manifestVersion": manifest.Version, "replaced": exists, "members": container.Len(),
	})
	if receiptErr != nil {
		return payload, receiptErr
	}
	payload.Receipt = receiptPath
	return payload, nil
}

// checkCollisions refuses an artifact whose IDs are owned by another registered book. IDs below the
// ledger are otherwise free to restore.
func (context *transferContext) checkCollisions(registry *store.Registry, target model.BookID,
	artifactIDs map[model.ID]bool) error {
	for _, entry := range registry.Books {
		if sameBook(entry.ID, target) {
			continue
		}
		manifest, err := readManifest(context.Life, context.Layout, entry.ID)
		if err != nil {
			return err
		}
		for id := range manifestIDSet(manifest) {
			if artifactIDs[id] {
				return fail("restore.id-collision",
					"ID %s belongs to the registered book %q", id, entry.ID)
			}
		}
	}
	return nil
}

// manifestIDSet collects every ID of a book.
func manifestIDSet(manifest *model.Manifest) map[model.ID]bool {
	set := map[model.ID]bool{}
	add := func(id model.ID) {
		if !id.IsZero() {
			set[id] = true
		}
	}
	add(manifest.Root.ID)
	manifest.Walk(func(node *model.Node, depth int, parent *model.Node) bool {
		add(node.ID)
		for _, id := range node.Blocks {
			add(id)
		}
		add(node.ProseBlock)
		return true
	})
	for _, region := range manifest.Regions {
		add(region.ID)
		add(region.Target.ID)
		for _, id := range region.Blocks {
			add(id)
		}
	}
	for id := range manifest.BlockMeta {
		add(id)
	}
	return set
}

// highestID returns the largest ID a manifest owns.
func highestID(manifest *model.Manifest) model.ID {
	highest := model.ZeroID
	for id := range manifestIDSet(manifest) {
		if id > highest {
			highest = id
		}
	}
	return highest
}

func sameBook(left, right model.BookID) bool {
	return store.BookIDKey(left) == store.BookIDKey(right)
}
