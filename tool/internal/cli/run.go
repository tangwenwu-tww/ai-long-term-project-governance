package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	applycmd "example.invalid/aigov/internal/apply"
	"example.invalid/aigov/internal/plan"
)

const version = "0.1.0-dev"

const helpText = `aigov - AI governance CLI skeleton

Usage:
  aigov help
  aigov version
  aigov plan --manifest <path>
  aigov apply --manifest <path> --plan-sha256 <sha256>

Commands:
  help     Show this help
  version  Show the version
  plan     Validate and print an experimental read-only plan
  apply    Apply an explicitly approved experimental plan
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
	if args[0] == "apply" {
		return runApply(args[1:], stdout, stderr)
	}

	fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
	fmt.Fprintln(stderr, "Run 'aigov help' for usage.")
	return 2
}

func runApply(args []string, stdout, stderr io.Writer) int {
	if os.Getenv("AIGOV_ENABLE_EXPERIMENTAL_APPLY") != "1" {
		fmt.Fprintln(stderr, "experimental apply is disabled; set AIGOV_ENABLE_EXPERIMENTAL_APPLY=1 to enable")
		return 2
	}
	manifestPath, approved := "", ""
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--manifest":
			manifestPath = args[i+1]
		case "--plan-sha256":
			approved = args[i+1]
		}
	}
	if manifestPath == "" {
		encoder := json.NewEncoder(stderr)
		_ = encoder.Encode(plan.Diagnostic{Code: "E_PLAN_APPROVAL_REQUIRED", Path: "/plan_sha256", Message: "provide the complete approved plan stdout SHA-256"})
		return 2
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return 2
	}
	result := applycmd.Execute(manifestPath, workingDir, time.Now(), version, approved)
	if len(result.Diagnostics) != 0 {
		encoder := json.NewEncoder(stderr)
		for _, item := range result.Diagnostics {
			if encoder.Encode(item) != nil {
				return 2
			}
		}
		return 2
	}
	if result.Output == nil {
		return 2
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(result.Output) != nil {
		return 2
	}
	return 0
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
	result := plan.Evaluate(args[1], workingDir, time.Now(), version)
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
	if result.Conflict {
		return 2
	}
	return 0
}
