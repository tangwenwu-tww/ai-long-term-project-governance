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
	"sort"
	"strings"
	"testing"
)

const evidenceFeatureEnv = "AIGOV_ENABLE_EXPERIMENTAL_EVIDENCE"

func TestEvidenceLifecycleAndContentDiff(t *testing.T) {
	target, requestPath := newEvidenceFixture(t, "T-EVIDENCE-RED-001", []string{"work/"}, []map[string]any{
		evidenceCommand("verify-core", []string{"help"}, 0),
	}, []map[string]any{
		evidenceCriterion("AC-01", "core verification passes", "verify-core"),
	})
	writeEvidenceFile(t, target, "work/modified.txt", []byte("before\n"))
	writeEvidenceFile(t, target, "work/deleted.txt", []byte("delete me\n"))

	begin := requireEvidenceBegin(t, target, requestPath)
	beginHash := sha256HexEvidence([]byte(begin.stdout))
	writeEvidenceFile(t, target, "work/modified.txt", []byte("after\n"))
	writeEvidenceFile(t, target, "work/added.txt", []byte("new\n"))
	if err := os.Remove(filepath.Join(target, "work", "deleted.txt")); err != nil {
		t.Fatal(err)
	}

	collected := runEvidenceCLI(t, target, true, "evidence", "collect", "--project", target, "--request", requestPath, "--begin-sha256", beginHash)
	if collected.code != 0 || collected.stderr != "" {
		t.Fatalf("collect = code %d stdout %q stderr %q; want code 0 and empty stderr", collected.code, collected.stdout, collected.stderr)
	}
	manifest := readEvidenceManifestFromCollect(t, target, collected.stdout)
	if got := manifest["status"]; got != "PASS" {
		t.Fatalf("manifest status = %v, want PASS", got)
	}
	if got := manifest["within_allowed_files"]; got != true {
		t.Fatalf("within_allowed_files = %v, want true", got)
	}

	changes := evidenceObjectsByKey(t, manifest["changed_files"], "path")
	requireEvidenceChange(t, changes, "work/added.txt", "added", "", sha256HexEvidence([]byte("new\n")))
	requireEvidenceChange(t, changes, "work/modified.txt", "modified", sha256HexEvidence([]byte("before\n")), sha256HexEvidence([]byte("after\n")))
	requireEvidenceChange(t, changes, "work/deleted.txt", "deleted", sha256HexEvidence([]byte("delete me\n")), "")

	commands := evidenceObjectsByKey(t, manifest["commands"], "id")
	core := commands["verify-core"]
	if core == nil || core["exit_code"] != float64(0) || core["expected_exit_code"] != float64(0) {
		t.Fatalf("verify-core command = %#v", core)
	}
	for _, stream := range []string{"stdout", "stderr"} {
		summary, ok := core[stream].(map[string]any)
		if !ok || summary["sha256"] == nil || summary["byte_length"] == nil || summary["valid_utf8"] != true || summary["line_count"] == nil {
			t.Fatalf("%s summary = %#v", stream, core[stream])
		}
	}
	criteria := evidenceObjectsByKey(t, manifest["acceptance_checklist"], "id")
	if got := criteria["AC-01"]["verified"]; got != true {
		t.Fatalf("AC-01 verified = %v, want true", got)
	}
	requireNoEvidenceBlobs(t, target, "T-EVIDENCE-RED-001", "a001")
}

func TestEvidenceLiteralArgvAndFailedSeal(t *testing.T) {
	argument := "literal value | > < & ; 中文"
	target, requestPath := newEvidenceFixture(t, "T-EVIDENCE-RED-002", []string{"work/"}, []map[string]any{
		evidenceCommand("literal-argv", []string{argument}, 2),
		evidenceCommand("unexpected-failure", []string{"unsupported"}, 0),
	}, []map[string]any{
		evidenceCriterion("AC-01", "literal argv is preserved", "literal-argv"),
		evidenceCriterion("AC-02", "all required commands pass", "unexpected-failure"),
	})
	begin := requireEvidenceBegin(t, target, requestPath)
	beginHash := sha256HexEvidence([]byte(begin.stdout))

	collected := runEvidenceCLI(t, target, true, "evidence", "collect", "--project", target, "--request", requestPath, "--begin-sha256", beginHash)
	if collected.code != 1 || collected.stderr != "" {
		t.Fatalf("failed collect = code %d stdout %q stderr %q; want code 1 with sealed manifest", collected.code, collected.stdout, collected.stderr)
	}
	manifest := readEvidenceManifestFromCollect(t, target, collected.stdout)
	if got := manifest["status"]; got != "FAIL" {
		t.Fatalf("manifest status = %v, want FAIL", got)
	}
	commands := evidenceObjectsByKey(t, manifest["commands"], "id")
	literal := commands["literal-argv"]
	args, ok := literal["args"].([]any)
	if !ok || len(args) != 1 || args[0] != argument || literal["exit_code"] != float64(2) {
		t.Fatalf("literal argv command = %#v", literal)
	}
	criteria := evidenceObjectsByKey(t, manifest["acceptance_checklist"], "id")
	if criteria["AC-01"]["verified"] != true || criteria["AC-02"]["verified"] != false {
		t.Fatalf("acceptance checklist = %#v", criteria)
	}
}

func TestEvidenceOutsideAllowedFilesForcesFailure(t *testing.T) {
	target, requestPath := newEvidenceFixture(t, "T-EVIDENCE-RED-003", []string{"work/"}, []map[string]any{
		evidenceCommand("verify", []string{"help"}, 0),
	}, []map[string]any{
		evidenceCriterion("AC-01", "verification passes", "verify"),
	})
	begin := requireEvidenceBegin(t, target, requestPath)
	writeEvidenceFile(t, target, "outside.txt", []byte("not allowed\n"))

	collected := runEvidenceCLI(t, target, true, "evidence", "collect", "--project", target, "--request", requestPath, "--begin-sha256", sha256HexEvidence([]byte(begin.stdout)))
	if collected.code != 1 || collected.stderr != "" {
		t.Fatalf("outside collect = code %d stdout %q stderr %q; want sealed FAIL", collected.code, collected.stdout, collected.stderr)
	}
	manifest := readEvidenceManifestFromCollect(t, target, collected.stdout)
	if manifest["status"] != "FAIL" || manifest["within_allowed_files"] != false {
		t.Fatalf("outside manifest = %#v", manifest)
	}
	outside, ok := manifest["outside_allowed_files"].([]any)
	if !ok || len(outside) != 1 || outside[0] != "outside.txt" {
		t.Fatalf("outside_allowed_files = %#v, want [outside.txt]", manifest["outside_allowed_files"])
	}
}

func TestEvidenceTamperAndSealedAttemptFailClosed(t *testing.T) {
	t.Run("request changed after begin", func(t *testing.T) {
		target, requestPath := newEvidenceFixture(t, "T-EVIDENCE-RED-004", []string{"work/"}, []map[string]any{
			evidenceCommand("verify", []string{"help"}, 0),
		}, []map[string]any{
			evidenceCriterion("AC-01", "verification passes", "verify"),
		})
		begin := requireEvidenceBegin(t, target, requestPath)
		raw, err := os.ReadFile(requestPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(requestPath, bytes.Replace(raw, []byte("verification passes"), []byte("changed verification"), 1), 0o600); err != nil {
			t.Fatal(err)
		}
		got := runEvidenceCLI(t, target, true, "evidence", "collect", "--project", target, "--request", requestPath, "--begin-sha256", sha256HexEvidence([]byte(begin.stdout)))
		requireEvidenceDiagnostic(t, got, "E_EVIDENCE_BEGIN_MISMATCH")
		requireEvidenceManifestMissing(t, target, "T-EVIDENCE-RED-004", "a001")
	})

	t.Run("sealed attempt cannot be overwritten", func(t *testing.T) {
		target, requestPath := newEvidenceFixture(t, "T-EVIDENCE-RED-005", []string{"work/"}, []map[string]any{
			evidenceCommand("verify", []string{"help"}, 0),
		}, []map[string]any{
			evidenceCriterion("AC-01", "verification passes", "verify"),
		})
		begin := requireEvidenceBegin(t, target, requestPath)
		beginHash := sha256HexEvidence([]byte(begin.stdout))
		first := runEvidenceCLI(t, target, true, "evidence", "collect", "--project", target, "--request", requestPath, "--begin-sha256", beginHash)
		if first.code != 0 {
			t.Fatalf("first collect = code %d stderr %q", first.code, first.stderr)
		}
		manifestPath := evidenceManifestPath(target, "T-EVIDENCE-RED-005", "a001")
		before, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		second := runEvidenceCLI(t, target, true, "evidence", "collect", "--project", target, "--request", requestPath, "--begin-sha256", beginHash)
		requireEvidenceDiagnostic(t, second, "E_EVIDENCE_SEALED")
		after, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("sealed manifest was overwritten")
		}
	})
}

func TestEvidenceInvalidRequestAndFeatureFlag(t *testing.T) {
	t.Run("empty acceptance mapping", func(t *testing.T) {
		target, requestPath := newEvidenceFixture(t, "T-EVIDENCE-RED-006", []string{"work/"}, []map[string]any{
			evidenceCommand("verify", []string{"help"}, 0),
		}, []map[string]any{
			{"id": "AC-01", "text": "must be mapped", "command_ids": []string{}},
		})
		got := runEvidenceCLI(t, target, true, "evidence", "begin", "--project", target, "--request", requestPath)
		requireEvidenceDiagnostic(t, got, "E_EVIDENCE_REQUEST")
	})

	t.Run("feature disabled", func(t *testing.T) {
		target, requestPath := newEvidenceFixture(t, "T-EVIDENCE-RED-007", []string{"work/"}, []map[string]any{
			evidenceCommand("verify", []string{"help"}, 0),
		}, []map[string]any{
			evidenceCriterion("AC-01", "verification passes", "verify"),
		})
		got := runEvidenceCLI(t, target, false, "evidence", "begin", "--project", target, "--request", requestPath)
		want := "experimental evidence is disabled; set AIGOV_ENABLE_EXPERIMENTAL_EVIDENCE=1 to enable\n"
		if got.code != 2 || got.stdout != "" || got.stderr != want {
			t.Fatalf("disabled evidence = code %d stdout %q stderr %q; want code 2 stdout empty stderr %q", got.code, got.stdout, got.stderr, want)
		}
	})
}

func TestEvidenceSymlinkIsNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("portable contract covers symlink semantics on hosts that permit unprivileged symlinks")
	}
	target, requestPath := newEvidenceFixture(t, "T-EVIDENCE-RED-008", []string{"work/"}, []map[string]any{
		evidenceCommand("verify", []string{"help"}, 0),
	}, []map[string]any{
		evidenceCriterion("AC-01", "verification passes", "verify"),
	})
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(target, "work", "outside-link")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("host cannot create symlink fixture: %v", err)
	}
	begin := requireEvidenceBegin(t, target, requestPath)
	if err := os.WriteFile(outside, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	collected := runEvidenceCLI(t, target, true, "evidence", "collect", "--project", target, "--request", requestPath, "--begin-sha256", sha256HexEvidence([]byte(begin.stdout)))
	if collected.code != 0 {
		t.Fatalf("symlink collect = code %d stderr %q", collected.code, collected.stderr)
	}
	manifest := readEvidenceManifestFromCollect(t, target, collected.stdout)
	changes, ok := manifest["changed_files"].([]any)
	if !ok || len(changes) != 0 {
		t.Fatalf("outside symlink target produced changes: %#v", manifest["changed_files"])
	}
}

func newEvidenceFixture(t *testing.T, taskID string, allowed []string, commands, criteria []map[string]any) (string, string) {
	t.Helper()
	target := t.TempDir()
	writeEvidenceFile(t, target, ".ai-governance/instance.json", []byte(canonicalInstance))
	request := map[string]any{
		"schema_version":      "0.1-draft",
		"task_id":             taskID,
		"task_revision":       1,
		"attempt_id":          "a001",
		"task_sha256":         strings.Repeat("1", 64),
		"allowed_files":       allowed,
		"acceptance_criteria": criteria,
		"commands":            commands,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	requestPath := filepath.Join(target, ".ai-governance", "requests", taskID+".json")
	if err := os.MkdirAll(filepath.Dir(requestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return target, requestPath
}

func evidenceCommand(id string, args []string, expectedExit int) map[string]any {
	return map[string]any{
		"id": id, "executable": cliPath, "args": args, "cwd": ".", "expected_exit_code": expectedExit,
	}
}

func evidenceCriterion(id, text, commandID string) map[string]any {
	return map[string]any{"id": id, "text": text, "command_ids": []string{commandID}}
}

func requireEvidenceBegin(t *testing.T, target, requestPath string) commandResult {
	t.Helper()
	got := runEvidenceCLI(t, target, true, "evidence", "begin", "--project", target, "--request", requestPath)
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("evidence begin = code %d stdout %q stderr %q; want code 0 and empty stderr", got.code, got.stdout, got.stderr)
	}
	receipt := parseCanonicalJSONObjectLine(t, []byte(got.stdout))
	for _, field := range []string{"schema_version", "task_id", "attempt_id", "request_sha256", "baseline_sha256", "excluded_paths"} {
		if receipt[field] == nil {
			t.Fatalf("begin receipt missing %s: %#v", field, receipt)
		}
	}
	return got
}

func runEvidenceCLI(t *testing.T, target string, enabled bool, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command(cliPath, args...)
	cmd.Dir = target
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), evidenceFeatureEnv+"=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	if enabled {
		cmd.Env = append(cmd.Env, evidenceFeatureEnv+"=1")
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run evidence CLI: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return commandResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func readEvidenceManifestFromCollect(t *testing.T, target, stdout string) map[string]any {
	t.Helper()
	receipt := parseCanonicalJSONObjectLine(t, []byte(stdout))
	path, ok := receipt["manifest_path"].(string)
	if !ok || path == "" || filepath.IsAbs(path) {
		t.Fatalf("manifest_path = %#v", receipt["manifest_path"])
	}
	raw, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	wantHash, ok := receipt["manifest_sha256"].(string)
	if !ok || sha256HexEvidence(raw) != wantHash {
		t.Fatalf("manifest hash = %s receipt = %#v", sha256HexEvidence(raw), receipt["manifest_sha256"])
	}
	return parseCanonicalJSONObjectLine(t, raw)
}

func parseCanonicalJSONObjectLine(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	if len(raw) == 0 || raw[len(raw)-1] != '\n' || bytes.Count(raw, []byte("\n")) != 1 {
		t.Fatalf("canonical JSON must be exactly one LF-terminated line, got %q", raw)
	}
	line := raw[:len(raw)-1]
	var value map[string]any
	if err := json.Unmarshal(line, &value); err != nil {
		t.Fatalf("invalid JSON %q: %v", line, err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(line, canonical) {
		t.Fatalf("JSON is not canonical\ngot:  %s\nwant: %s", line, canonical)
	}
	return value
}

func evidenceObjectsByKey(t *testing.T, raw any, key string) map[string]map[string]any {
	t.Helper()
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("value = %#v, want array", raw)
	}
	result := make(map[string]map[string]any, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("array item = %#v, want object", item)
		}
		name, ok := object[key].(string)
		if !ok || name == "" {
			t.Fatalf("object key %s = %#v", key, object[key])
		}
		if result[name] != nil {
			t.Fatalf("duplicate %s %q", key, name)
		}
		result[name] = object
	}
	return result
}

func requireEvidenceChange(t *testing.T, changes map[string]map[string]any, path, kind, beforeHash, afterHash string) {
	t.Helper()
	change := changes[path]
	if change == nil || change["change"] != kind {
		t.Fatalf("change %s = %#v, want %s", path, change, kind)
	}
	if beforeHash != "" && change["before_sha256"] != beforeHash {
		t.Fatalf("change %s before hash = %#v, want %s", path, change["before_sha256"], beforeHash)
	}
	if afterHash != "" && change["after_sha256"] != afterHash {
		t.Fatalf("change %s after hash = %#v, want %s", path, change["after_sha256"], afterHash)
	}
}

func requireEvidenceDiagnostic(t *testing.T, got commandResult, code string) {
	t.Helper()
	if got.code != 2 || got.stdout != "" {
		t.Fatalf("diagnostic = code %d stdout %q stderr %q; want exit 2 and empty stdout", got.code, got.stdout, got.stderr)
	}
	diag := parseCanonicalJSONObjectLine(t, []byte(got.stderr))
	if diag["code"] != code || diag["path"] == nil || diag["message"] == nil {
		t.Fatalf("diagnostic = %#v, want code %s and code/path/message", diag, code)
	}
}

func requireEvidenceManifestMissing(t *testing.T, target, taskID, attemptID string) {
	t.Helper()
	if _, err := os.Stat(evidenceManifestPath(target, taskID, attemptID)); !os.IsNotExist(err) {
		t.Fatalf("manifest exists or stat failed unexpectedly: %v", err)
	}
}

func requireNoEvidenceBlobs(t *testing.T, target, taskID, attemptID string) {
	t.Helper()
	root := filepath.Dir(evidenceManifestPath(target, taskID, attemptID))
	var names []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.Contains(name, "blob") || strings.Contains(name, "stdout") || strings.Contains(name, "stderr") {
			t.Fatalf("R1 wrote a raw output blob: %s (files: %v)", name, names)
		}
	}
}

func writeEvidenceFile(t *testing.T, target, relative string, content []byte) {
	t.Helper()
	path := filepath.Join(target, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func evidenceManifestPath(target, taskID, attemptID string) string {
	return filepath.Join(target, "governance", "evidence", taskID, attemptID, "manifest.json")
}

func sha256HexEvidence(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func TestEvidenceContractNamesAreUnique(t *testing.T) {
	// This compile-time-visible inventory makes accidental ID reuse in fixtures easy to diagnose.
	ids := []string{"T-EVIDENCE-RED-001", "T-EVIDENCE-RED-002", "T-EVIDENCE-RED-003", "T-EVIDENCE-RED-004", "T-EVIDENCE-RED-005", "T-EVIDENCE-RED-006", "T-EVIDENCE-RED-007", "T-EVIDENCE-RED-008"}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatal(fmt.Sprintf("duplicate evidence fixture ID %s", id))
		}
		seen[id] = true
	}
}
