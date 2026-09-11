// Command recall catches *arr imports that score lower than their grab. It
// receives Radarr's and Sonarr's webhooks, keeps the grab and the import for
// each download, and logs a decision for every import.
package main

import (
	"fmt"
	"os"
)

// version is set by the linker at release time.
var version = "dev"

const usage = `usage: recall <command> [flags]

  serve     receive webhooks and log a decision for every import
  version   print the version
`

const (
	exitClean  = 0
	exitFailed = 1
	exitUsage  = 2
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(exitUsage)
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "serve":
		os.Exit(runServe(args))
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(exitUsage)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
