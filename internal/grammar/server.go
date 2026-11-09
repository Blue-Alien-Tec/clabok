package grammar

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	stateFileName = "state.json"
	lockFileName  = "serve.lock"
	logFileName   = "server.log"
	startTimeout  = 40 * time.Second
	lockTimeout   = 60 * time.Second
)

// ServerError is a stable server lifecycle failure.
type ServerError struct {
	Code    string
	Message string
}

// Error returns the human message.
func (err *ServerError) Error() string { return err.Message }

// ErrorCode returns the stable code for the CLI boundary.
func (err *ServerError) ErrorCode() string { return err.Code }

// ServerInfo reports the shared LanguageTool server.
type ServerInfo struct {
	DryRun  bool   `json:"dryRun"`
	URL     string `json:"url"`
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Jar     string `json:"jar,omitempty"`
	Started string `json:"started,omitempty"`
	Shared  bool   `json:"shared,omitempty"`
	Log     string `json:"log,omitempty"`
}

// DryRunResult marks the payload as a dry-run receipt.
func (info ServerInfo) DryRunResult() bool { return info.DryRun }

// Text renders the server state.
func (info ServerInfo) Text() string {
	state := "not running"
	if info.Running {
		state = "running"
	}
	if info.Shared {
		state += ", shared"
	}
	line := fmt.Sprintf("grammar server %s: %s", info.URL, state)
	if info.PID > 0 {
		line += fmt.Sprintf(", pid %d", info.PID)
	}
	if info.Jar != "" {
		line += "\njar: " + info.Jar
	}
	if info.Log != "" {
		line += "\nlog: " + info.Log
	}
	return line + "\n"
}

type serverState struct {
	PID     int    `json:"pid"`
	URL     string `json:"url"`
	Jar     string `json:"jar"`
	Started string `json:"started"`
}

// Serve starts or attaches to the one shared LanguageTool server.
func Serve(endpoint, jar string) (ServerInfo, error) {
	base, port, err := parseEndpoint(endpoint)
	if err != nil {
		return ServerInfo{}, err
	}
	if reachable(base) {
		return Info(endpoint)
	}
	jarPath := resolveJar(jar)
	if jarPath == "" {
		return ServerInfo{}, &ServerError{
			Code:    "grammar.no-jar",
			Message: "no LanguageTool jar found; pass --jar or set CLABOK_LANGUAGETOOL_JAR",
		}
	}
	if _, lookupErr := exec.LookPath("java"); lookupErr != nil {
		return ServerInfo{}, &ServerError{
			Code:    "grammar.no-java",
			Message: "java is not on PATH; install a Java runtime or start a server manually",
		}
	}
	release, lockErr := acquireLock()
	if lockErr != nil {
		return ServerInfo{}, lockErr
	}
	defer release()
	if reachable(base) {
		return Info(endpoint)
	}
	if startErr := startServer(base, port, jarPath); startErr != nil {
		return ServerInfo{}, startErr
	}
	return Info(endpoint)
}

// Stop stops the recorded server process and removes the state.
func Stop(endpoint string) (ServerInfo, error) {
	base, _, err := parseEndpoint(endpoint)
	if err != nil {
		return ServerInfo{}, err
	}
	state, exists := readState()
	if !exists {
		return ServerInfo{URL: base, Running: reachable(base), Shared: reachable(base)}, nil
	}
	if state.PID > 0 && shouldStopProcess(base, state.Started) {
		if process, findErr := os.FindProcess(state.PID); findErr == nil {
			_ = process.Kill()
		}
	}
	_ = os.Remove(statePath())
	return ServerInfo{URL: base, Running: false}, nil
}

// shouldStopProcess guards against pid reuse. A stale state file must not kill an unrelated
// process that inherited the recorded pid: signal only when the endpoint still answers, or when
// the state is fresh enough to describe a server that is still starting.
func shouldStopProcess(base, started string) bool {
	if reachable(base) {
		return true
	}
	created, err := time.Parse(time.RFC3339, started)
	if err != nil {
		return false
	}
	return time.Since(created) < time.Minute
}

// Info reports the state and probes the endpoint.
func Info(endpoint string) (ServerInfo, error) {
	base, _, err := parseEndpoint(endpoint)
	if err != nil {
		return ServerInfo{}, err
	}
	info := infoFromState(base, false)
	info.Running = reachable(base)
	info.Shared = info.Running && info.PID == 0
	return info, nil
}

func startServer(base string, port int, jarPath string) error {
	directory := stateDir()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return &ServerError{Code: "grammar.start-failed", Message: err.Error()}
	}
	logPath := filepath.Join(directory, logFileName)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return &ServerError{Code: "grammar.start-failed", Message: err.Error()}
	}
	defer logFile.Close()
	command := exec.Command("java", "-cp", jarPath, "org.languagetool.server.HTTPServer",
		"--port", strconv.Itoa(port), "--allow-origin", "*")
	command.Stdout = logFile
	command.Stderr = logFile
	detach(command)
	if err := command.Start(); err != nil {
		return &ServerError{Code: "grammar.start-failed", Message: err.Error()}
	}
	state := serverState{
		PID: command.Process.Pid, URL: base, Jar: jarPath,
		Started: time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeState(state); err != nil {
		_ = command.Process.Kill()
		return &ServerError{Code: "grammar.start-failed", Message: err.Error()}
	}
	deadline := time.Now().Add(startTimeout)
	for time.Now().Before(deadline) {
		if reachable(base) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	_ = command.Process.Kill()
	_ = os.Remove(statePath())
	return &ServerError{
		Code: "grammar.start-failed",
		Message: fmt.Sprintf("LanguageTool did not answer within %s; see %s: %s",
			startTimeout, logPath, tailOf(logPath, 5)),
	}
}

func parseEndpoint(endpoint string) (string, int, error) {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return "", 0, &ServerError{
			Code: "grammar.invalid-endpoint", Message: fmt.Sprintf("invalid endpoint %q", endpoint),
		}
	}
	switch parsed.Hostname() {
	case "127.0.0.1", "localhost", "::1":
	default:
		return "", 0, &ServerError{
			Code:    "grammar.remote-endpoint",
			Message: "only a local endpoint can be started; start the remote server yourself",
		}
	}
	port := 8081
	if raw := parsed.Port(); raw != "" {
		parsedPort, portErr := strconv.Atoi(raw)
		if portErr != nil {
			return "", 0, &ServerError{
				Code: "grammar.invalid-endpoint", Message: fmt.Sprintf("invalid port %q", raw),
			}
		}
		port = parsedPort
	}
	return parsed.Scheme + "://" + parsed.Host, port, nil
}

func reachable(base string) bool {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	response, err := client.Get(base + "/v2/languages")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func acquireLock() (func(), error) {
	directory := stateDir()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, &ServerError{Code: "grammar.start-failed", Message: err.Error()}
	}
	path := filepath.Join(directory, lockFileName)
	deadline := time.Now().Add(lockTimeout)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(file, "%d\n", os.Getpid())
			file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, &ServerError{Code: "grammar.start-failed", Message: err.Error()}
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > 2*lockTimeout {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, &ServerError{
				Code:    "grammar.busy",
				Message: "another process is starting the grammar server",
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// resolveJar finds the LanguageTool server jar: flag, environment, then the popular
// language_tool_python caches.
func resolveJar(override string) string {
	for _, candidate := range []string{
		override,
		os.Getenv("CLABOK_LANGUAGETOOL_JAR"),
		os.Getenv("LANGUAGETOOL_JAR"),
	} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	home, _ := os.UserHomeDir()
	roots := []string{
		filepath.Join(home, ".cache", "language_tool_python"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "language_tool_python"),
		filepath.Join(os.TempDir(), "language_tool_python"),
	}
	best := ""
	for _, root := range roots {
		if root == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, "LanguageTool-*", "languagetool-server.jar"))
		for _, match := range matches {
			if match > best {
				best = match
			}
		}
	}
	return best
}

func stateDir() string {
	if directory := strings.TrimSpace(os.Getenv("CLABOK_GRAMMAR_STATE_DIR")); directory != "" {
		return directory
	}
	return filepath.Join(os.TempDir(), "clabok-grammar")
}

func statePath() string {
	return filepath.Join(stateDir(), stateFileName)
}

func writeState(state serverState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), data, 0o644)
}

func readState() (serverState, bool) {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return serverState{}, false
	}
	var state serverState
	if err := json.Unmarshal(data, &state); err != nil {
		return serverState{}, false
	}
	return state, true
}

func infoFromState(base string, shared bool) ServerInfo {
	info := ServerInfo{URL: base, Shared: shared}
	if state, exists := readState(); exists {
		info.PID = state.PID
		info.Jar = state.Jar
		info.Started = state.Started
		info.Log = filepath.Join(stateDir(), logFileName)
	}
	return info
}

func tailOf(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, " | ")
}
