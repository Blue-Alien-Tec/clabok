package coordinator

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Blue-Alien-Tec/clabok/internal/model"
)

// DefaultSocket derives a short local socket path from the store root. The path stays well below
// the AF_UNIX limit on Windows.
func DefaultSocket(storeRoot string) string {
	sum := sha256.Sum256([]byte(storeRoot))
	return filepath.Join(os.TempDir(), "clabok-"+hex.EncodeToString(sum[:8])+".sock")
}

// String renders the mode for the wire protocol.
func (mode Mode) String() string {
	if mode == Shared {
		return "shared"
	}
	return "exclusive"
}

func parseMode(text string) (Mode, error) {
	switch text {
	case "exclusive":
		return Exclusive, nil
	case "shared":
		return Shared, nil
	}
	return Exclusive, fail("coordinator.invalid-mode", "unknown lock mode %q", text)
}

type wireResource struct {
	Kind string `json:"kind"`
	Book string `json:"book,omitempty"`
	ID   string `json:"id,omitempty"`
}

type request struct {
	Op        string         `json:"op"`
	Version   string         `json:"version,omitempty"`
	Command   string         `json:"command,omitempty"`
	PID       int            `json:"pid,omitempty"`
	Mode      string         `json:"mode,omitempty"`
	TimeoutMs int64          `json:"timeoutMs,omitempty"`
	Resources []wireResource `json:"resources,omitempty"`
}

type response struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func encodeResources(resources []Resource) []wireResource {
	wire := make([]wireResource, 0, len(resources))
	for _, resource := range resources {
		wire = append(wire, wireResource{
			Kind: string(resource.Kind),
			Book: string(resource.Book),
			ID:   resource.ID.String(),
		})
	}
	return wire
}

func decodeResources(wire []wireResource) ([]Resource, error) {
	resources := make([]Resource, 0, len(wire))
	for _, item := range wire {
		resource := Resource{Kind: Kind(item.Kind), Book: model.BookID(item.Book)}
		if item.ID != "" {
			id, err := model.ParseID(item.ID)
			if err != nil {
				return nil, fail("coordinator.invalid-resource", "invalid id %q", item.ID)
			}
			resource.ID = id
		}
		resources = append(resources, resource)
	}
	return sortResources(resources)
}

// Daemon is the client side of the coordination daemon.
type Daemon struct {
	Socket  string
	Version string
	Command string
	Timeout time.Duration
}

// NewDaemon returns a daemon client. The version must match the server's version.
func NewDaemon(socket, version string) (*Daemon, error) {
	if socket == "" || !filepath.IsAbs(socket) {
		return nil, fail("coordinator.invalid-socket", "the daemon socket must be an absolute path")
	}
	if version == "" {
		return nil, fail("coordinator.invalid-version", "the daemon version must not be empty")
	}
	return &Daemon{Socket: filepath.Clean(socket), Version: version, Timeout: DefaultTimeout}, nil
}

var _ Coordinator = (*Daemon)(nil)

// Acquire locks the resource set through the daemon. The lock lives as long as the connection, so
// release closes it.
func (daemon *Daemon) Acquire(resources []Resource, mode Mode, timeout time.Duration) (Release, error) {
	ordered, err := sortResources(resources)
	if err != nil {
		return nil, err
	}
	if len(ordered) == 0 {
		return func() error { return nil }, nil
	}
	if mode != Exclusive && mode != Shared {
		return nil, fail("coordinator.invalid-mode", "unknown lock mode %d", mode)
	}
	if timeout <= 0 {
		timeout = daemon.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
	}

	connection, err := net.DialTimeout("unix", daemon.Socket, timeout)
	if err != nil {
		return nil, fail("coordinator.daemon-unavailable", "no daemon at %s: %v", daemon.Socket, err)
	}
	payload, err := json.Marshal(request{
		Op:        "acquire",
		Version:   daemon.Version,
		Command:   daemon.Command,
		PID:       os.Getpid(),
		Mode:      mode.String(),
		TimeoutMs: timeout.Milliseconds(),
		Resources: encodeResources(ordered),
	})
	if err != nil {
		_ = connection.Close()
		return nil, fail("coordinator.io", "%v", err)
	}
	_ = connection.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := connection.Write(append(payload, '\n')); err != nil {
		_ = connection.Close()
		return nil, fail("coordinator.io", "%v", err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(timeout + time.Second))
	line, err := bufio.NewReader(connection).ReadBytes('\n')
	if err != nil {
		_ = connection.Close()
		return nil, fail("coordinator.io", "the daemon did not answer: %v", err)
	}
	var answer response
	if err := json.Unmarshal(line, &answer); err != nil {
		_ = connection.Close()
		return nil, fail("coordinator.io", "invalid daemon response: %v", err)
	}
	if !answer.OK {
		_ = connection.Close()
		code := answer.Code
		if code == "" {
			code = "coordinator.io"
		}
		return nil, fail(code, "%s", answer.Message)
	}
	released := false
	return func() error {
		if released {
			return nil
		}
		released = true
		return connection.Close()
	}, nil
}

// Server is the in-memory coordination daemon.
type Server struct {
	Socket  string
	Version string
	table   *lockTable
	closing chan struct{}

	listener net.Listener
	once     sync.Once
}

// NewServer returns a server for one socket.
func NewServer(socket, version string) (*Server, error) {
	if socket == "" || !filepath.IsAbs(socket) {
		return nil, fail("coordinator.invalid-socket", "the daemon socket must be an absolute path")
	}
	if version == "" {
		return nil, fail("coordinator.invalid-version", "the daemon version must not be empty")
	}
	return &Server{
		Socket:  filepath.Clean(socket),
		Version: version,
		table:   newLockTable(),
		closing: make(chan struct{}),
	}, nil
}

// Listen prepares the socket. A running daemon on the same socket is an error; a stale socket file
// is removed.
func (server *Server) Listen() error {
	if connection, err := net.DialTimeout("unix", server.Socket, 200*time.Millisecond); err == nil {
		_ = connection.Close()
		return fail("coordinator.already-running", "a daemon is already listening on %s", server.Socket)
	}
	_ = os.Remove(server.Socket)
	if err := os.MkdirAll(filepath.Dir(server.Socket), 0o755); err != nil {
		return fail("coordinator.io", "%v", err)
	}
	listener, err := net.Listen("unix", server.Socket)
	if err != nil {
		return fail("coordinator.io", "%v", err)
	}
	server.listener = listener
	return nil
}

// Serve accepts connections until Close.
func (server *Server) Serve() error {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			select {
			case <-server.closing:
				return nil
			default:
				return fail("coordinator.io", "%v", err)
			}
		}
		go server.handle(connection)
	}
}

// Close stops the listener and removes the socket file.
func (server *Server) Close() error {
	var err error
	server.once.Do(func() {
		close(server.closing)
		if server.listener != nil {
			err = server.listener.Close()
		}
		_ = os.Remove(server.Socket)
	})
	return err
}

func (server *Server) handle(connection net.Conn) {
	defer connection.Close()
	reader := bufio.NewReader(io.LimitReader(connection, 1<<20))
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}
	var incoming request
	if err := json.Unmarshal(line, &incoming); err != nil || incoming.Op != "acquire" {
		writeResponse(connection, response{Code: "coordinator.invalid-request", Message: "the daemon expects an acquire request"})
		return
	}
	if incoming.Version != server.Version {
		writeResponse(connection, response{
			Code: "coordinator.version-mismatch",
			Message: fmt.Sprintf("client version %q does not match the daemon version %q",
				incoming.Version, server.Version),
		})
		return
	}
	resources, err := decodeResources(incoming.Resources)
	if err != nil {
		writeResponse(connection, response{Code: errorCode(err), Message: err.Error()})
		return
	}
	mode, modeErr := parseMode(incoming.Mode)
	if modeErr != nil {
		writeResponse(connection, response{Code: errorCode(modeErr), Message: modeErr.Error()})
		return
	}
	timeout := time.Duration(incoming.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	holder := fmt.Sprintf("pid %d (%s)", incoming.PID, incoming.Command)
	keys, acquireErr := server.table.acquire(resources, mode, timeout, holder)
	if acquireErr != nil {
		writeResponse(connection, response{Code: errorCode(acquireErr), Message: acquireErr.Error()})
		return
	}
	writeResponse(connection, response{OK: true})

	// The lock lives as long as the connection. A release closes the connection.
	_, _ = io.Copy(io.Discard, reader)
	server.table.release(keys)
}

func writeResponse(connection net.Conn, answer response) {
	_ = json.NewEncoder(connection).Encode(answer)
}

func errorCode(err error) string {
	var coordinatorErr *Error
	if errors.As(err, &coordinatorErr) && coordinatorErr.Code != "" {
		return coordinatorErr.Code
	}
	return "coordinator.io"
}

type lockEntry struct {
	readers int
	writer  bool
	holder  string
}

func (entry *lockEntry) describe() string {
	if entry.writer {
		return entry.holder
	}
	return "a shared reader"
}

type lockTable struct {
	mutex   sync.Mutex
	entries map[string]*lockEntry
}

func newLockTable() *lockTable {
	return &lockTable{entries: map[string]*lockEntry{}}
}

// acquire grants all resources at once, so a partial hold cannot deadlock. Waits by polling.
func (table *lockTable) acquire(resources []Resource, mode Mode, timeout time.Duration, holder string) ([]string, error) {
	keys := make([]string, 0, len(resources))
	for _, resource := range resources {
		keys = append(keys, resource.String())
	}
	deadline := time.Now().Add(timeout)
	for {
		table.mutex.Lock()
		conflictKey := ""
		conflictHolder := ""
		for _, key := range keys {
			entry := table.entries[key]
			if entry == nil {
				continue
			}
			if mode == Exclusive {
				if entry.writer || entry.readers > 0 {
					conflictKey = key
					conflictHolder = entry.describe()
					break
				}
				continue
			}
			if entry.writer {
				conflictKey = key
				conflictHolder = entry.describe()
				break
			}
		}
		if conflictKey == "" {
			for _, key := range keys {
				entry := table.entries[key]
				if entry == nil {
					entry = &lockEntry{}
					table.entries[key] = entry
				}
				if mode == Exclusive {
					entry.writer = true
				} else {
					entry.readers++
				}
				entry.holder = holder
			}
			table.mutex.Unlock()
			return keys, nil
		}
		table.mutex.Unlock()
		if !time.Now().Before(deadline) {
			return nil, fail("coordinator.busy", "%s is held by %s; waited %s",
				conflictKey, conflictHolder, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (table *lockTable) release(keys []string) {
	table.mutex.Lock()
	defer table.mutex.Unlock()
	for _, key := range keys {
		entry := table.entries[key]
		if entry == nil {
			continue
		}
		if entry.writer {
			entry.writer = false
		} else if entry.readers > 0 {
			entry.readers--
		}
		if !entry.writer && entry.readers == 0 {
			delete(table.entries, key)
		}
	}
}
