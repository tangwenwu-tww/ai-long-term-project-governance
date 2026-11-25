package contracttest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHaltInstalledRuntimeThirdFailureAndFourthRejection(t *testing.T) {
	target, installed := installHaltRuntime(t)
	taskID := "T-HALT-RED-001"
	signature := strings.Repeat("a", 64)
	for attempt := 1; attempt <= 3; attempt++ {
		got := runInstalledHalt(t, target, installed, "record-attempt", "--project", target, "--task-id", taskID, "--attempt-id", fmt.Sprintf("a%03d", attempt), "--exit-code", "1", "--signature-sha256", signature)
		wantCode := 0
		if attempt == 3 {
			wantCode = 3
		}
		if got.code != wantCode || got.stderr != "" {
			t.Fatalf("failure %d = code %d stdout %q stderr %q; want code %d", attempt, got.code, got.stdout, got.stderr, wantCode)
		}
		state := parseCanonicalHaltLine(t, got.stdout)
		if state["consecutive_count"] != float64(attempt) {
			t.Fatalf("failure %d state = %#v", attempt, state)
		}
	}

	status := runInstalledHalt(t, target, installed, "halt-status", "--project", target, "--task-id", taskID)
	if status.code != 3 || status.stderr != "" {
		t.Fatalf("halt status = %#v", status)
	}
	state := parseCanonicalHaltLine(t, status.stdout)
	if state["status"] != "HALTED_FOR_HUMAN" || state["last_attempt_id"] != "a003" || state["consecutive_count"] != float64(3) {
		t.Fatalf("halted state = %#v", state)
	}

	before := snapshotTree(t, target)
	fourth := runInstalledHalt(t, target, installed, "record-attempt", "--project", target, "--task-id", taskID, "--attempt-id", "a004", "--exit-code", "1", "--signature-sha256", signature)
	requireHaltDiagnostic(t, fourth, 3, "E_TASK_HALTED")
	after := snapshotTree(t, target)
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("fourth attempt changed target\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestHaltSuccessAndDifferentSignatureResetConsecutiveCount(t *testing.T) {
	target, installed := installHaltRuntime(t)
	taskID := "T-HALT-RED-002"
	sigA, sigB := strings.Repeat("a", 64), strings.Repeat("b", 64)

	requireHaltRecord(t, target, installed, taskID, "a001", 1, sigA, 0, 1)
	requireHaltRecord(t, target, installed, taskID, "a002", 0, "", 0, 0)
	requireHaltRecord(t, target, installed, taskID, "a003", 1, sigA, 0, 1)
	requireHaltRecord(t, target, installed, taskID, "a004", 1, sigB, 0, 1)

	status := runInstalledHalt(t, target, installed, "halt-status", "--project", target, "--task-id", taskID)
	if status.code != 0 || status.stderr != "" {
		t.Fatalf("active status = %#v", status)
	}
	state := parseCanonicalHaltLine(t, status.stdout)
	if state["status"] != "EXECUTING" || state["consecutive_signature_sha256"] != sigB || state["total_failures"] != float64(3) {
		t.Fatalf("reset state = %#v", state)
	}
}

func TestHaltAuditTamperSequenceAndCacheRebuild(t *testing.T) {
	t.Run("attempt sequence is strict", func(t *testing.T) {
		target, installed := installHaltRuntime(t)
		got := runInstalledHalt(t, target, installed, "record-attempt", "--project", target, "--task-id", "T-HALT-RED-003", "--attempt-id", "a002", "--exit-code", "1", "--signature-sha256", strings.Repeat("c", 64))
		requireHaltDiagnostic(t, got, 2, "E_ATTEMPT_SEQUENCE")
	})

	t.Run("tampered ledger fails closed", func(t *testing.T) {
		target, installed := installHaltRuntime(t)
		taskID := "T-HALT-RED-004"
		requireHaltRecord(t, target, installed, taskID, "a001", 1, strings.Repeat("d", 64), 0, 1)
		record := filepath.Join(target, "governance", "audit", "tasks", taskID, "attempts", "a001.json")
		if err := os.WriteFile(record, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := runInstalledHalt(t, target, installed, "halt-status", "--project", target, "--task-id", taskID)
		requireHaltDiagnostic(t, got, 2, "E_HALT_AUDIT")
	})

	t.Run("cache is not authoritative", func(t *testing.T) {
		target, installed := installHaltRuntime(t)
		taskID := "T-HALT-RED-005"
		requireHaltRecord(t, target, installed, taskID, "a001", 1, strings.Repeat("e", 64), 0, 1)
		want := runInstalledHalt(t, target, installed, "halt-status", "--project", target, "--task-id", taskID)
		cache := filepath.Join(target, "governance", "state", taskID+".json")
		if err := os.WriteFile(cache, []byte("{\"status\":\"FORGED\"}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := runInstalledHalt(t, target, installed, "halt-status", "--project", target, "--task-id", taskID)
		if got.code != want.code || got.stdout != want.stdout || got.stderr != want.stderr {
			t.Fatalf("forged cache changed status\nwant: %#v\ngot:  %#v", want, got)
		}
		if err := os.Remove(cache); err != nil {
			t.Fatal(err)
		}
		got = runInstalledHalt(t, target, installed, "halt-status", "--project", target, "--task-id", taskID)
		if got.code != want.code || got.stdout != want.stdout || got.stderr != want.stderr {
			t.Fatalf("missing cache changed status\nwant: %#v\ngot:  %#v", want, got)
		}
	})
}

func TestHaltInstalledRuntimeIdempotenceAndConflict(t *testing.T) {
	target, installed := installHaltRuntime(t)
	cliBytes, err := os.ReadFile(cliPath)
	if err != nil {
		t.Fatal(err)
	}
	installedBytes, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installedBytes, cliBytes) {
		t.Fatalf("installed runtime SHA-256 = %s, want executing CLI %s", sha256HexForHalt(installedBytes), sha256HexForHalt(cliBytes))
	}

	if err := os.WriteFile(installed, append(installedBytes, 0), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := haltManifest(t, target)
	got := runManifest(t, target, manifest, true)
	if got.code != 2 || got.stderr != "" || !strings.Contains(got.stdout, `"logical_id":"runtime_entrypoint"`) || !strings.Contains(got.stdout, `"action":"CONFLICT"`) {
		t.Fatalf("tampered runtime plan = %#v", got)
	}
}

func installHaltRuntime(t *testing.T) (string, string) {
	t.Helper()
	target := t.TempDir()
	manifest := haltManifest(t, target)
	manifestPath := writeManifestFile(t, manifest)

	planned := runCommand(t, target, manifestPath, true)
	requirePlanSuccess(t, planned)
	if !strings.Contains(planned.stdout, `"logical_id":"runtime_entrypoint"`) {
		t.Fatalf("plan has no runtime entrypoint: %s", planned.stdout)
	}
	planHash := sha256.Sum256([]byte(planned.stdout))
	applied := runRawCLI(t, target, []string{"apply", "--manifest", manifestPath, "--plan-sha256", hex.EncodeToString(planHash[:])}, true)
	if applied.code != 0 || applied.stderr != "" {
		t.Fatalf("runtime apply = %#v", applied)
	}

	installed := filepath.Join(target, ".ai-governance", "runtime", "aigov")
	if runtime.GOOS == "windows" {
		installed += ".exe"
	}
	if info, err := os.Stat(installed); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("installed runtime = %v, %v", info, err)
	}

	secondPlan := runCommand(t, target, manifestPath, true)
	requirePlanSuccess(t, secondPlan)
	secondHash := sha256.Sum256([]byte(secondPlan.stdout))
	second := runRawCLI(t, target, []string{"apply", "--manifest", manifestPath, "--plan-sha256", hex.EncodeToString(secondHash[:])}, true)
	if second.code != 0 || second.stderr != "" || !strings.Contains(second.stdout, `"status":"NO_CHANGES"`) {
		t.Fatalf("runtime idempotent apply = %#v", second)
	}
	return target, installed
}

func haltManifest(t *testing.T, target string) map[string]any {
	t.Helper()
	manifest := baseManifest(t, target)
	setPointer(t, manifest, "/contract_version", "0.4-draft")
	setPointer(t, manifest, "/integration/mode", "apply")
	return manifest
}

func runInstalledHalt(t *testing.T, target, installed string, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command(installed, args...)
	cmd.Dir = target
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run installed runtime: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return commandResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func requireHaltRecord(t *testing.T, target, installed, taskID, attemptID string, exitCode int, signature string, wantCode, wantConsecutive int) {
	t.Helper()
	args := []string{"record-attempt", "--project", target, "--task-id", taskID, "--attempt-id", attemptID, "--exit-code", fmt.Sprint(exitCode)}
	if signature != "" {
		args = append(args, "--signature-sha256", signature)
	}
	got := runInstalledHalt(t, target, installed, args...)
	if got.code != wantCode || got.stderr != "" {
		t.Fatalf("record %s = %#v, want code %d", attemptID, got, wantCode)
	}
	state := parseCanonicalHaltLine(t, got.stdout)
	if state["consecutive_count"] != float64(wantConsecutive) {
		t.Fatalf("record %s state = %#v", attemptID, state)
	}
}

func requireHaltDiagnostic(t *testing.T, got commandResult, wantExit int, wantCode string) {
	t.Helper()
	if got.code != wantExit || got.stdout != "" {
		t.Fatalf("halt diagnostic = %#v, want exit %d and empty stdout", got, wantExit)
	}
	diag := parseCanonicalHaltLine(t, got.stderr)
	if diag["code"] != wantCode || diag["path"] == nil || diag["message"] == nil {
		t.Fatalf("halt diagnostic = %#v, want %s", diag, wantCode)
	}
}

func parseCanonicalHaltLine(t *testing.T, value string) map[string]any {
	t.Helper()
	if !strings.HasSuffix(value, "\n") || strings.Count(value, "\n") != 1 {
		t.Fatalf("output is not one LF-terminated line: %q", value)
	}
	line := strings.TrimSuffix(value, "\n")
	var result map[string]any
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("output is not JSON: %v: %q", err, value)
	}
	canonical, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != line {
		t.Fatalf("output is not canonical\ngot:  %s\nwant: %s", line, canonical)
	}
	return result
}

func sha256HexForHalt(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
