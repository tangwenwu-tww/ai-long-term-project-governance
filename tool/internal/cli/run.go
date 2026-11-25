package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	applycmd "example.invalid/aigov/internal/apply"
	"example.invalid/aigov/internal/evidence"
	"example.invalid/aigov/internal/halt"
	"example.invalid/aigov/internal/plan"
)

const version = "0.1.0-dev"

const helpText = `aigov - AI governance CLI skeleton

Usage:
  aigov help
  aigov version
  aigov plan --manifest <path>
  aigov apply --manifest <path> --plan-sha256 <sha256>
  aigov evidence begin --project <root> --request <request.json>
  aigov evidence collect --project <root> --request <request.json> --begin-sha256 <sha256>
  aigov record-attempt --project <root> --task-id <id> --attempt-id <aNNN> --exit-code <integer> [--signature-sha256 <sha256>]
  aigov halt-status --project <root> --task-id <id>

Commands:
  help     Show this help
  version  Show the version
  plan     Validate and print an experimental read-only plan
  apply    Apply an explicitly approved experimental plan
  evidence Capture an experimental machine-verifiable evidence attempt
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
	if args[0] == "evidence" {
		return runEvidence(args[1:], stdout, stderr)
	}
	if args[0] == "record-attempt" {
		return runRecordAttempt(args[1:], stdout, stderr)
	}
	if args[0] == "halt-status" {
		return runHaltStatus(args[1:], stdout, stderr)
	}

	fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
	fmt.Fprintln(stderr, "Run 'aigov help' for usage.")
	return 2
}

func runRecordAttempt(args []string, stdout, stderr io.Writer) int {
	values, diagnostic := haltArguments(args, true)
	if diagnostic != nil {
		_ = halt.WriteCanonical(stderr, diagnostic)
		return 2
	}
	exitCode, err := strconv.Atoi(values["--exit-code"])
	if err != nil {
		_ = halt.WriteCanonical(stderr, halt.Diagnostic{Code: "E_HALT_REQUEST", Message: "exit code must be an integer", Path: "/exit_code"})
		return 2
	}
	result := halt.RecordAttempt(values["--project"], values["--task-id"], values["--attempt-id"], exitCode, values["--signature-sha256"], time.Now())
	return writeHaltResult(result, stdout, stderr)
}

func runHaltStatus(args []string, stdout, stderr io.Writer) int {
	values, diagnostic := haltArguments(args, false)
	if diagnostic != nil {
		_ = halt.WriteCanonical(stderr, diagnostic)
		return 2
	}
	result := halt.Status(values["--project"], values["--task-id"])
	return writeHaltResult(result, stdout, stderr)
}

func haltArguments(args []string, record bool) (map[string]string, *halt.Diagnostic) {
	if len(args)%2 != 0 {
		return nil, &halt.Diagnostic{Code: "E_HALT_REQUEST", Message: "arguments must be flag-value pairs", Path: "/arguments"}
	}
	allowed := map[string]bool{"--project": true, "--task-id": true}
	required := []string{"--project", "--task-id"}
	if record {
		allowed["--attempt-id"], allowed["--exit-code"], allowed["--signature-sha256"] = true, true, true
		required = append(required, "--attempt-id", "--exit-code")
	}
	values := make(map[string]string)
	for index := 0; index < len(args); index += 2 {
		if !allowed[args[index]] || values[args[index]] != "" || args[index+1] == "" {
			return nil, &halt.Diagnostic{Code: "E_HALT_REQUEST", Message: "arguments contain an unknown, duplicate, or empty value", Path: "/arguments"}
		}
		values[args[index]] = args[index+1]
	}
	for _, name := range required {
		if values[name] == "" {
			return nil, &halt.Diagnostic{Code: "E_HALT_REQUEST", Message: "required argument is missing", Path: "/arguments"}
		}
	}
	return values, nil
}

func writeHaltResult(result halt.Result, stdout, stderr io.Writer) int {
	if result.Diagnostic != nil {
		if halt.WriteCanonical(stderr, result.Diagnostic) != nil {
			return 2
		}
		return result.ExitCode
	}
	if result.State == nil || halt.WriteCanonical(stdout, result.State) != nil {
		return 2
	}
	return result.ExitCode
}

func runEvidence(args []string, stdout, stderr io.Writer) int {
	if os.Getenv("AIGOV_ENABLE_EXPERIMENTAL_EVIDENCE") != "1" {
		fmt.Fprintln(stderr, "experimental evidence is disabled; set AIGOV_ENABLE_EXPERIMENTAL_EVIDENCE=1 to enable")
		return 2
	}
	if len(args) == 0 || (args[0] != "begin" && args[0] != "collect") {
		return writeEvidenceDiagnostic(stderr, evidence.Diagnostic{Code: "E_EVIDENCE_REQUEST", Path: "/command", Message: "use evidence begin or evidence collect"})
	}
	mode := args[0]
	projectRoot, requestPath, beginSHA := "", "", ""
	if (len(args)-1)%2 != 0 {
		return writeEvidenceDiagnostic(stderr, evidence.Diagnostic{Code: "E_EVIDENCE_REQUEST", Path: "/arguments", Message: "evidence arguments must be flag-value pairs"})
	}
	for i := 1; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--project":
			projectRoot = args[i+1]
		case "--request":
			requestPath = args[i+1]
		case "--begin-sha256":
			beginSHA = args[i+1]
		default:
			return writeEvidenceDiagnostic(stderr, evidence.Diagnostic{Code: "E_EVIDENCE_REQUEST", Path: "/arguments", Message: "unknown evidence argument"})
		}
	}
	if projectRoot == "" || requestPath == "" || (mode == "collect" && beginSHA == "") || (mode == "begin" && beginSHA != "") {
		return writeEvidenceDiagnostic(stderr, evidence.Diagnostic{Code: "E_EVIDENCE_REQUEST", Path: "/arguments", Message: "required evidence arguments are missing or incompatible"})
	}
	var result evidence.Result
	if mode == "begin" {
		result = evidence.Begin(projectRoot, requestPath)
	} else {
		result = evidence.Collect(projectRoot, requestPath, beginSHA, time.Now)
	}
	if len(result.Diagnostics) != 0 {
		for _, item := range result.Diagnostics {
			if writeEvidenceDiagnostic(stderr, item) != 2 {
				return 2
			}
		}
		return 2
	}
	if err := evidence.WriteCanonical(stdout, result.Output); err != nil {
		return 2
	}
	return result.ExitCode
}

func writeEvidenceDiagnostic(stderr io.Writer, item evidence.Diagnostic) int {
	if err := evidence.WriteCanonical(stderr, item); err != nil {
		return -1
	}
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
