package cli

import (
	"fmt"
	"io"
)

const version = "0.1.0-dev"

const helpText = `aigov - AI governance CLI skeleton

Usage:
  aigov help
  aigov version

Commands:
  help     Show this help
  version  Show the version
`

// Run handles the minimal CLI surface and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprint(stdout, helpText)
		return 0
	}

	if args[0] == "version" {
		fmt.Fprintf(stdout, "aigov %s\n", version)
		return 0
	}

	fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
	fmt.Fprintln(stderr, "Run 'aigov help' for usage.")
	return 2
}
