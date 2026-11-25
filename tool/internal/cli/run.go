package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"example.invalid/aigov/internal/plan"
)

const version = "0.1.0-dev"

const helpText = `aigov - AI governance CLI skeleton

Usage:
  aigov help
  aigov version
  aigov plan --manifest <path>

Commands:
  help     Show this help
  version  Show the version
  plan     Validate and print an experimental read-only plan
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

	if args[0] == "plan" {
		return runPlan(args[1:], stdout, stderr)
	}

	fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
	fmt.Fprintln(stderr, "Run 'aigov help' for usage.")
	return 2
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	if os.Getenv("AIGOV_ENABLE_EXPERIMENTAL_PLAN") != "1" {
		fmt.Fprintln(stderr, "experimental plan is disabled; set AIGOV_ENABLE_EXPERIMENTAL_PLAN=1 to enable")
		return 2
	}
	if len(args) != 2 || args[0] != "--manifest" || args[1] == "" {
		fmt.Fprintln(stderr, "usage: aigov plan --manifest <path>")
		return 2
	}
	workingDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, "cannot determine the target project directory")
		return 2
	}
	result := plan.Evaluate(args[1], workingDir, time.Now())
	if len(result.Diagnostics) != 0 {
		encoder := json.NewEncoder(stderr)
		for _, item := range result.Diagnostics {
			if err := encoder.Encode(item); err != nil {
				return 2
			}
		}
		return 2
	}
	for _, line := range result.Lines {
		fmt.Fprintln(stdout, line)
	}
	return 0
}
