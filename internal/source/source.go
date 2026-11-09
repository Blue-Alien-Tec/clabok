package source

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/model"
	"github.com/Blue-Alien-Tec/clabok/internal/store"
)

const (
	blocksPrefix  = "blocks/"
	layoutsPrefix = "layouts/asciiui/"
	bodySuffix    = ".md"
	layoutSuffix  = ".asciiui"
)

// Error is a source failure with a stable code.
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

// Service loads books through the lifecycle, without opening or locking them.
type Service struct {
	Lifecycle *lifecycle.Lifecycle
	bodyLoads atomic.Int64
}

// New returns a source service.
func New(life *lifecycle.Lifecycle) *Service {
	return &Service{Lifecycle: life}
}

// BodyReads reports how many block bodies this service has read. It is a diagnostic for the indexed
// search path, which must not parse every body.
func (service *Service) BodyReads() int64 { return service.bodyLoads.Load() }

// selection controls how much of a book a load reads.
type selection struct {
	allBodies bool
	bodies    []model.ID
	layouts   bool
}

var (
	selectAll      = selection{allBodies: true, layouts: true}
	selectManifest = selection{}
)

// Load reads the manifest, every body and every asciiui specification.
func (service *Service) Load(id model.BookID) (*Book, error) {
	return service.load(id, selectAll)
}

// LoadManifest reads only the manifest. The body and layout maps are empty; the manifest bytes and
// the placement indexes are complete.
func (service *Service) LoadManifest(id model.BookID) (*Book, error) {
	return service.load(id, selectManifest)
}

// LoadBodies reads the manifest and only the requested bodies. The layout map is empty.
func (service *Service) LoadBodies(id model.BookID, blocks []model.ID) (*Book, error) {
	return service.load(id, selection{bodies: blocks})
}

// load prefers the extracted state while the book is extracted and falls back to the container when
// the extract is incomplete.
func (service *Service) load(id model.BookID, choice selection) (*Book, error) {
	paths, err := service.Lifecycle.Paths(id)
	if err != nil {
		return nil, err
	}
	state, err := service.Lifecycle.State(id)
	if err != nil {
		return nil, err
	}
	if state.Extracted {
		book, extractErr := readExtracted(paths, choice)
		if extractErr == nil {
			service.bodyLoads.Add(int64(len(book.Bodies)))
			return book, nil
		}
		if !errors.Is(extractErr, os.ErrNotExist) {
			return nil, extractErr
		}
	}
	book, err := readContainer(paths, choice)
	if err == nil && book != nil {
		service.bodyLoads.Add(int64(len(book.Bodies)))
	}
	return book, err
}

func readExtracted(paths store.BookPaths, choice selection) (*Book, error) {
	data, err := os.ReadFile(filepath.Join(paths.Dir, lifecycle.ManifestMember))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fail("source.read-failed", "%v", err)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		return nil, err
	}
	bodies, err := readExtractedBodies(paths, choice)
	if err != nil {
		return nil, err
	}
	layouts := map[model.ID][]byte{}
	if choice.layouts {
		layouts, err = readExtractedLayouts(paths)
		if err != nil {
			return nil, err
		}
	}
	return newBook(paths, data, manifest, bodies, layouts, true), nil
}

func readExtractedBodies(paths store.BookPaths, choice selection) (map[model.ID]string, error) {
	bodies := map[model.ID]string{}
	if !choice.allBodies {
		for _, id := range choice.bodies {
			data, err := os.ReadFile(filepath.Join(paths.BlocksDir, id.String()+bodySuffix))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fail("source.read-failed", "%v", err)
			}
			bodies[id] = string(data)
		}
		return bodies, nil
	}
	entries, err := os.ReadDir(paths.BlocksDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return bodies, nil
		}
		return nil, fail("source.read-failed", "%v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), bodySuffix) {
			continue
		}
		id, parseErr := model.ParseID(strings.TrimSuffix(entry.Name(), bodySuffix))
		if parseErr != nil {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(paths.BlocksDir, entry.Name()))
		if readErr != nil {
			return nil, fail("source.read-failed", "%v", readErr)
		}
		bodies[id] = string(data)
	}
	return bodies, nil
}

func readExtractedLayouts(paths store.BookPaths) (map[model.ID][]byte, error) {
	layouts := map[model.ID][]byte{}
	directory := filepath.Join(paths.LayoutsDir, "asciiui")
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return layouts, nil
		}
		return nil, fail("source.read-failed", "%v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), layoutSuffix) {
			continue
		}
		id, parseErr := model.ParseID(strings.TrimSuffix(entry.Name(), layoutSuffix))
		if parseErr != nil {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			return nil, fail("source.read-failed", "%v", readErr)
		}
		layouts[id] = data
	}
	return layouts, nil
}

func readContainer(paths store.BookPaths, choice selection) (*Book, error) {
	container, err := store.Load(paths.Container)
	if err != nil {
		return nil, err
	}
	data, ok := container.Read(lifecycle.ManifestMember)
	if !ok {
		return nil, fail("source.manifest-missing",
			"book %q has no %s member", paths.ID, lifecycle.ManifestMember)
	}
	manifest, err := model.Decode(data)
	if err != nil {
		return nil, err
	}
	bodies := readContainerBodies(container, choice)
	layouts := map[model.ID][]byte{}
	if choice.layouts {
		for _, name := range container.Names() {
			id, member := layoutMemberID(name)
			if !member {
				continue
			}
			if spec, exists := container.Read(name); exists {
				layouts[id] = spec
			}
		}
	}
	return newBook(paths, data, manifest, bodies, layouts, false), nil
}

func readContainerBodies(container *store.Container, choice selection) map[model.ID]string {
	bodies := map[model.ID]string{}
	if !choice.allBodies {
		for _, id := range choice.bodies {
			if body, exists := container.Text(blocksPrefix + id.String() + bodySuffix); exists {
				bodies[id] = body
			}
		}
		return bodies
	}
	for _, name := range container.Names() {
		id, member := blockMemberID(name)
		if !member {
			continue
		}
		if body, exists := container.Text(name); exists {
			bodies[id] = body
		}
	}
	return bodies
}

func blockMemberID(name string) (model.ID, bool) {
	if !strings.HasPrefix(name, blocksPrefix) || !strings.HasSuffix(name, bodySuffix) {
		return model.ZeroID, false
	}
	id, err := model.ParseID(strings.TrimSuffix(strings.TrimPrefix(name, blocksPrefix), bodySuffix))
	if err != nil {
		return model.ZeroID, false
	}
	return id, true
}

func layoutMemberID(name string) (model.ID, bool) {
	if !strings.HasPrefix(name, layoutsPrefix) || !strings.HasSuffix(name, layoutSuffix) {
		return model.ZeroID, false
	}
	id, err := model.ParseID(strings.TrimSuffix(strings.TrimPrefix(name, layoutsPrefix), layoutSuffix))
	if err != nil {
		return model.ZeroID, false
	}
	return id, true
}
