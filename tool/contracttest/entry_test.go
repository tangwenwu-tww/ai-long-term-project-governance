package contracttest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const managedEntryBody = "<!-- aigov:managed-entry:v1 begin -->\n" +
	"AI Agent: after reading this file, read the governance instance manifest at\n" +
	"`.ai-governance/instance.json`, then read the `phase_snapshot` resource\n" +
	"named there. These local files expose the instance ID, current phase, and core\n" +
	"hypothesis. Confirmed project facts in this file take precedence on conflict.\n" +
	"Do not edit this managed block manually.\n" +
	"<!-- aigov:managed-entry:v1 end -->\n"

func TestEntryContractPreserveOrGenerate(t *testing.T) {
	t.Run("missing entry remains generated without phase snapshot", func(t *testing.T) {
		target := t.TempDir()
		plan := requireEntryPlan(t, runManifest(t, target, entryManifest(t, target, "preserve-or-generate", nil), true))
		agent := requireEntryOperation(t, plan, "agent_entry")
		if agent.Action != "CREATE" || agent.Ownership != "project" || agent.Reason != "target_missing" {
			t.Fatalf("agent operation = %#v", agent)
		}
		if operationByLogicalID(plan, "phase_snapshot") != nil {
			t.Fatal("preserve-or-generate unexpectedly manages phase_snapshot")
		}
	})

	t.Run("existing entry remains project fact", func(t *testing.T) {
		target := t.TempDir()
		content := []byte("# Project rules\n\nKeep local facts.\n")
		writeEntryFixture(t, target, content)
		plan := requireEntryPlan(t, runManifest(t, target, entryManifest(t, target, "preserve-or-generate", nil), true))
		agent := requireEntryOperation(t, plan, "agent_entry")
		if agent.Action != "NOOP" || agent.Ownership != "project" || agent.Reason != "project_fact_preserved" {
			t.Fatalf("agent operation = %#v", agent)
		}
		if operationByLogicalID(plan, "phase_snapshot") != nil {
			t.Fatal("preserve-or-generate unexpectedly manages phase_snapshot")
		}
	})
}

func TestEntryAppendManagedReferencePlan(t *testing.T) {
	target := t.TempDir()
	prefix := []byte("# Project rules\n\nKeep local facts.\n")
	writeEntryFixture(t, target, prefix)
	approved := sha256HexForEntry(prefix)
	manifest := entryManifest(t, target, "append-managed-reference", &approved)
	setPointer(t, manifest, "/governance/core_hypothesis", "Validate the local governance contract.")
	setPointer(t, manifest, "/governance/phase_basis/assessed_at", "2026-11-24T00:00:00Z")
	setPointer(t, manifest, "/governance/phase_basis/assessed_by", "human:owner")

	plan := requireEntryPlan(t, runManifest(t, target, manifest, true))
	if plan.ContractVersion != "0.4-draft" {
		t.Fatalf("contract version = %q", plan.ContractVersion)
	}
	agent := requireEntryOperation(t, plan, "agent_entry")
	postimage := append(append([]byte(nil), prefix...), '\n')
	postimage = append(postimage, []byte(managedEntryBody)...)
	postHash := sha256HexForEntry(postimage)
	if agent.Action != "UPDATE" || agent.Ownership != "shared" || agent.Reason != "managed_reference_append" ||
		agent.ActualSHA256 == nil || *agent.ActualSHA256 != approved || agent.ExpectedSHA256 == nil || *agent.ExpectedSHA256 != postHash {
		t.Fatalf("agent operation = %#v; want UPDATE/shared pre=%s post=%s", agent, approved, postHash)
	}
	phase := requireEntryOperation(t, plan, "phase_snapshot")
	if phase.Action != "CREATE" || phase.Ownership != "tool" || phase.Reason != "target_missing" {
		t.Fatalf("phase snapshot operation = %#v", phase)
	}
	phaseBytes := []byte("{\"schema_version\":\"0.1-draft\",\"instance_id\":\"inst-example-01\",\"phase\":0,\"core_hypothesis\":\"Validate the local governance contract.\",\"phase_basis\":{\"assessed_at\":\"2026-11-24T00:00:00Z\",\"assessed_by\":\"human:owner\",\"signals\":[\"product_idea_exists\"]}}\n")
	if phase.ExpectedSHA256 == nil || *phase.ExpectedSHA256 != sha256HexForEntry(phaseBytes) {
		t.Fatalf("phase snapshot hash = %v, want %s", phase.ExpectedSHA256, sha256HexForEntry(phaseBytes))
	}
}

func TestEntryAppendManagedReferenceCustomPaths(t *testing.T) {
	target := t.TempDir()
	prefix := []byte("project facts\n")
	writeEntryFixture(t, target, prefix)
	approved := sha256HexForEntry(prefix)
	manifest := entryManifest(t, target, "append-managed-reference", &approved)
	setPointer(t, manifest, "/layout/profile", "custom-v0")
	setPointer(t, manifest, "/layout/resources", map[string]any{
		"instance_manifest": "custom/governance-instance.json",
		"phase_snapshot":    "custom/current-phase.json",
	})
	plan := requireEntryPlan(t, runManifest(t, target, manifest, true))
	agent := requireEntryOperation(t, plan, "agent_entry")
	phase := requireEntryOperation(t, plan, "phase_snapshot")
	if phase.Path != "custom/current-phase.json" {
		t.Fatalf("phase snapshot path = %q", phase.Path)
	}
	if agent.ExpectedSHA256 == nil {
		t.Fatal("agent postimage hash is missing")
	}
	defaultPost := append(append([]byte(nil), prefix...), '\n')
	defaultPost = append(defaultPost, []byte(managedEntryBody)...)
	if *agent.ExpectedSHA256 == sha256HexForEntry(defaultPost) {
		t.Fatal("custom instance path did not affect the managed reference bytes")
	}
}

func TestEntryAppendManagedReferenceLineEndings(t *testing.T) {
	fixtures := []struct {
		name, newline string
		prefix        []byte
	}{
		{"LF with terminal newline", "\n", []byte("project facts\n")},
		{"LF without terminal newline", "\n", []byte("project facts")},
		{"CRLF with terminal newline", "\r\n", []byte("project facts\r\n")},
		{"CRLF without terminal newline", "\r\n", []byte("first\r\nproject facts")},
		{"UTF-8 BOM", "\n", append([]byte{0xef, 0xbb, 0xbf}, []byte("project facts\n")...)},
		{"empty", "\n", []byte{}},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			target := t.TempDir()
			writeEntryFixture(t, target, fixture.prefix)
			approved := sha256HexForEntry(fixture.prefix)
			plan := requireEntryPlan(t, runManifest(t, target, entryManifest(t, target, "append-managed-reference", &approved), true))
			agent := requireEntryOperation(t, plan, "agent_entry")
			want := expectedManagedPostimage(fixture.prefix, fixture.newline, ".ai-governance/instance.json")
			if agent.ExpectedSHA256 == nil || *agent.ExpectedSHA256 != sha256HexForEntry(want) {
				t.Fatalf("postimage hash = %v, want %s", agent.ExpectedSHA256, sha256HexForEntry(want))
			}
			if !bytes.HasPrefix(want, fixture.prefix) {
				t.Fatal("reference renderer changed the project-owned prefix")
			}
		})
	}
}

func TestEntryAppendManagedReferenceExactBlockIsNoop(t *testing.T) {
	target := t.TempDir()
	prefix := []byte("project facts\n")
	postimage := expectedManagedPostimage(prefix, "\n", ".ai-governance/instance.json")
	writeEntryFixture(t, target, postimage)
	approved := sha256HexForEntry(prefix)
	plan := requireEntryPlan(t, runManifest(t, target, entryManifest(t, target, "append-managed-reference", &approved), true))
	agent := requireEntryOperation(t, plan, "agent_entry")
	if agent.Action != "NOOP" || agent.Ownership != "shared" || agent.Reason != "managed_reference_matches" {
		t.Fatalf("idempotent managed entry operation = %#v", agent)
	}
}

func TestEntryIntegrationModeAndApprovalMatrix(t *testing.T) {
	tests := []struct {
		name, mode string
		approved   *string
		withEntry  bool
	}{
		{"preserve rejects approval", "preserve-or-generate", stringPointer(strings.Repeat("a", 64)), true},
		{"append requires approval", "append-managed-reference", nil, true},
		{"append rejects wrong approval", "append-managed-reference", stringPointer(strings.Repeat("b", 64)), true},
		{"append requires existing entry", "append-managed-reference", stringPointer(strings.Repeat("c", 64)), false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			target := t.TempDir()
			if test.withEntry {
				writeEntryFixture(t, target, []byte("project facts\n"))
			}
			got := runManifest(t, target, entryManifest(t, target, test.mode, test.approved), true)
			if got.code == 0 {
				t.Fatalf("invalid entry integration input was accepted: %s", got.stdout)
			}
		})
	}
}

func TestEntryAppendManagedReferenceInputMatrix(t *testing.T) {
	fixtures := map[string][]byte{
		"mixed newline":    []byte("first\r\nsecond\n"),
		"isolated CR":      []byte("first\rsecond"),
		"invalid UTF-8":    {0xff, 0xfe, 0xfd},
		"NUL":              []byte("project\x00facts\n"),
		"partial marker":   []byte("project\n\n<!-- aigov:managed-entry:v1 begin -->\n"),
		"duplicate marker": []byte(managedEntryBody + "\n" + managedEntryBody),
	}
	for name, content := range fixtures {
		name, content := name, content
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			writeEntryFixture(t, target, content)
			approved := sha256HexForEntry(content)
			got := runManifest(t, target, entryManifest(t, target, "append-managed-reference", &approved), true)
			if got.code == 0 {
				t.Fatalf("unsafe entry was accepted: stdout=%q", got.stdout)
			}
		})
	}
}

func TestEntryAppendManagedReferenceApplyAndIdempotence(t *testing.T) {
	target := t.TempDir()
	prefix := []byte("# Project rules\n\nKeep local facts.\n")
	writeEntryFixture(t, target, prefix)
	approved := sha256HexForEntry(prefix)
	manifest := entryManifest(t, target, "append-managed-reference", &approved)
	setPointer(t, manifest, "/integration/mode", "apply")
	manifestPath := writeManifestFile(t, manifest)

	firstPlan := runCommand(t, target, manifestPath, true)
	requireEntryPlan(t, firstPlan)
	planSum := sha256.Sum256([]byte(firstPlan.stdout))
	firstApply := runRawCLI(t, target, []string{"apply", "--manifest", manifestPath, "--plan-sha256", hex.EncodeToString(planSum[:])}, true)
	if firstApply.code != 0 || firstApply.stderr != "" {
		t.Fatalf("first apply = %#v", firstApply)
	}
	entryBytes, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(entryBytes), string(prefix)) || !strings.Contains(string(entryBytes), "<!-- aigov:managed-entry:v1 begin -->") {
		t.Fatalf("updated agent entry does not preserve prefix and managed block: %q", entryBytes)
	}
	if _, err := os.Stat(filepath.Join(target, ".ai-governance", "phase.json")); err != nil {
		t.Fatalf("phase snapshot missing after apply: %v", err)
	}

	secondPlan := runCommand(t, target, manifestPath, true)
	plan := requireEntryPlan(t, secondPlan)
	agent := requireEntryOperation(t, plan, "agent_entry")
	if agent.Action != "NOOP" || agent.Reason != "managed_reference_matches" {
		t.Fatalf("idempotent agent operation = %#v", agent)
	}
	secondSum := sha256.Sum256([]byte(secondPlan.stdout))
	secondApply := runRawCLI(t, target, []string{"apply", "--manifest", manifestPath, "--plan-sha256", hex.EncodeToString(secondSum[:])}, true)
	if secondApply.code != 0 || secondApply.stderr != "" || !strings.Contains(secondApply.stdout, `"status":"NO_CHANGES"`) {
		t.Fatalf("second apply = %#v", secondApply)
	}
}

func TestEntryAppendManagedReferenceRejectsTargetChangeAfterPlan(t *testing.T) {
	target := t.TempDir()
	prefix := []byte("project facts\n")
	writeEntryFixture(t, target, prefix)
	approved := sha256HexForEntry(prefix)
	manifest := entryManifest(t, target, "append-managed-reference", &approved)
	setPointer(t, manifest, "/integration/mode", "apply")
	manifestPath := writeManifestFile(t, manifest)

	planResult := runCommand(t, target, manifestPath, true)
	requireEntryPlan(t, planResult)
	changed := []byte("project facts changed after approval\n")
	writeEntryFixture(t, target, changed)
	planSum := sha256.Sum256([]byte(planResult.stdout))
	applyResult := runRawCLI(t, target, []string{"apply", "--manifest", manifestPath, "--plan-sha256", hex.EncodeToString(planSum[:])}, true)
	if applyResult.code != 2 || applyResult.stdout != "" || !strings.Contains(applyResult.stderr, `"code":"E_TARGET_CHANGED"`) {
		t.Fatalf("changed target apply = %#v", applyResult)
	}
	got, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, changed) {
		t.Fatalf("failed apply changed project entry: got %q want %q", got, changed)
	}
	if _, err := os.Stat(filepath.Join(target, ".ai-governance")); !os.IsNotExist(err) {
		t.Fatalf("failed apply left governance writes: %v", err)
	}
}

func entryManifest(t *testing.T, target, mode string, approved *string) map[string]any {
	t.Helper()
	manifest := baseManifest(t, target)
	setPointer(t, manifest, "/contract_version", "0.4-draft")
	setPointer(t, manifest, "/layout/agent_entry_integration", map[string]any{
		"mode": mode, "approved_sha256": approved,
	})
	return manifest
}

func requireEntryPlan(t *testing.T, got commandResult) expectedPlan {
	t.Helper()
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("entry plan = code %d stdout %q stderr %q; want code 0 and empty stderr", got.code, got.stdout, got.stderr)
	}
	var plan expectedPlan
	if err := json.Unmarshal([]byte(got.stdout), &plan); err != nil {
		t.Fatalf("entry plan is not JSON: %v; stdout %q", err, got.stdout)
	}
	return plan
}

func requireEntryOperation(t *testing.T, plan expectedPlan, logicalID string) expectedOperation {
	t.Helper()
	operation := operationByLogicalID(plan, logicalID)
	if operation == nil {
		t.Fatalf("operation %q missing from %#v", logicalID, plan.Operations)
	}
	return *operation
}

func operationByLogicalID(plan expectedPlan, logicalID string) *expectedOperation {
	for i := range plan.Operations {
		if plan.Operations[i].LogicalID == logicalID {
			return &plan.Operations[i]
		}
	}
	return nil
}

func writeEntryFixture(t *testing.T, target string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(target, "AGENTS.md"), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func sha256HexForEntry(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func expectedManagedPostimage(prefix []byte, newline, instancePath string) []byte {
	body := strings.ReplaceAll(managedEntryBody, ".ai-governance/instance.json", instancePath)
	if newline == "\r\n" {
		body = strings.ReplaceAll(body, "\n", "\r\n")
	}
	result := append([]byte(nil), prefix...)
	if len(result) != 0 {
		double := []byte(newline + newline)
		single := []byte(newline)
		switch {
		case bytes.HasSuffix(result, double):
		case bytes.HasSuffix(result, single):
			result = append(result, single...)
		default:
			result = append(result, double...)
		}
	}
	return append(result, []byte(body)...)
}
