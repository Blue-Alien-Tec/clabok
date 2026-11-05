package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	product = "CLABOK"
	cli     = "pecia"
	version = "0.1.0-dev"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	switch args[0] {
	case "version":
		if hasFlag(args[1:], "--json") {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{
				"product": product,
				"cli":     cli,
				"version": version,
			})
			return
		}
		fmt.Printf("%s %s (%s)\n", product, version, cli)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		usage()
		os.Exit(2)
	}
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func usage() {
	fmt.Printf("%s — Command-Line Authoring for Blocks and Orchestration Kit\n", product)
	fmt.Printf("usage: %s <command> [flags]\n\n", cli)
	fmt.Println("commands:")
	fmt.Println("  version   print the version (--json for machine output)")
	fmt.Println("  help      print this help")
}
