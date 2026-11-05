package main

import (
	"os"

	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
)

func main() {
	registry := cli.NewRegistry()
	config.Register(registry)
	os.Exit(cli.Run(registry, os.Args[1:], os.Stdout, os.Stderr))
}
