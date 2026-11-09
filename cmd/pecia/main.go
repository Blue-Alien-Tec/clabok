package main

import (
	"os"

	"github.com/Blue-Alien-Tec/clabok/internal/apply"
	"github.com/Blue-Alien-Tec/clabok/internal/body"
	"github.com/Blue-Alien-Tec/clabok/internal/cli"
	"github.com/Blue-Alien-Tec/clabok/internal/config"
	"github.com/Blue-Alien-Tec/clabok/internal/coordinator"
	"github.com/Blue-Alien-Tec/clabok/internal/editorial"
	"github.com/Blue-Alien-Tec/clabok/internal/grammar"
	"github.com/Blue-Alien-Tec/clabok/internal/index"
	"github.com/Blue-Alien-Tec/clabok/internal/library"
	"github.com/Blue-Alien-Tec/clabok/internal/lifecycle"
	"github.com/Blue-Alien-Tec/clabok/internal/render"
	"github.com/Blue-Alien-Tec/clabok/internal/search"
	"github.com/Blue-Alien-Tec/clabok/internal/structure"
	"github.com/Blue-Alien-Tec/clabok/internal/temp"
	"github.com/Blue-Alien-Tec/clabok/internal/tui"
)

func main() {
	registry := cli.NewRegistry()
	config.Register(registry)
	coordinator.Register(registry)
	lifecycle.Register(registry)
	structure.Register(registry)
	editorial.Register(registry)
	grammar.Register(registry)
	apply.Register(registry)
	body.Register(registry)
	search.Register(registry)
	index.Register(registry)
	tui.Register(registry)
	temp.Register(registry)
	render.Register(registry, library.IndexEntries)
	library.Register(registry)
	os.Exit(cli.Run(registry, os.Args[1:], os.Stdout, os.Stderr))
}
