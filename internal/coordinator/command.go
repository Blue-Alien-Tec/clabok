package coordinator

import (
	"errors"
	"fmt"
	"net"

	"github.com/Blue-Alien-Tec/clabok/internal/buildinfo"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
)

type servePayload struct {
	Socket  string `json:"socket"`
	Version string `json:"version"`
}

func (payload servePayload) Text() string {
	return fmt.Sprintf("daemon stopped: %s (version %s)", payload.Socket, payload.Version)
}

// Register adds the coordination daemon command.
func Register(registry *cli.Registry) {
	registry.Add(&cli.Command{
		Name:    "serve",
		Summary: "run the coordination daemon in the foreground",
		Usage:   "pecia serve",
		Run:     runServe,
	})
}

func runServe(env *cli.Env, args []string) (*cli.Result, error) {
	if len(args) > 0 {
		return nil, cli.Usage("usage.unexpected-argument", "serve takes no arguments")
	}
	values, err := config.Load(env)
	if err != nil {
		return nil, err
	}
	socket := values.DaemonSocket
	if socket == "" {
		socket = DefaultSocket(values.StoreRoot)
	}
	server, serverErr := NewServer(socket, buildinfo.Version)
	if serverErr != nil {
		return nil, cli.AsError(serverErr)
	}
	if listenErr := server.Listen(); listenErr != nil {
		return nil, cli.AsError(listenErr)
	}
	defer func() { _ = server.Close() }()
	fmt.Fprintf(env.Err, "pecia serve: listening on %s (version %s)\n", socket, buildinfo.Version)
	if serveErr := server.Serve(); serveErr != nil && !errors.Is(serveErr, net.ErrClosed) {
		return nil, cli.AsError(serveErr)
	}
	return &cli.Result{Data: servePayload{Socket: socket, Version: buildinfo.Version}}, nil
}
