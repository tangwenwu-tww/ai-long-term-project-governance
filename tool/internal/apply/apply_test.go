package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"example.invalid/aigov/internal/plan"
)

var fixedNow = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func TestFailureInjectionRestoresBlankTarget(t *testing.T) {
	stages := []string{
		"mkdir", "mkdir-chmod", "mkdir-mode", "internal-mkdir", "internal-chmod", "internal-mode",
		"lock-create", "lock-chmod", "lock-write", "lock-sync", "lock-close", "lock-verify",
		"temp-create", "temp-chmod", "temp-write", "temp-sync", "temp-close", "temp-hash", "temp-verify",
		"publish", "final-hash", "final-attribute", "final-verify", "temp-cleanup", "lock-cleanup", "internal-cleanup",
	}
	if runtime.GOOS == "windows" {
		stages = append(stages, "temp-windows-attribute", "final-windows-identity")
	}
	for _, stage := range stages {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			target := t.TempDir()
			manifest := writeManifest(t, target, nil)
			approved := planHash(t, manifest, target)
			testFailpoint = func(current string) bool { return current == stage }
			defer func() { testFailpoint = nil }()
			result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
			if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "E_APPLY_WRITE" {
				t.Fatalf("result = %#v", result)
			}
			entries, err := os.ReadDir(target)
			if err != nil || len(entries) != 0 {
				t.Fatalf("rollback left target entries %v, %v", entries, err)
			}
		})
	}
}

func TestUpdateFailureInjectionRestoresOriginalAgentEntry(t *testing.T) {
	stages := []string{"update-backup", "update-publish", "update-final-hash", "lock-cleanup", "internal-cleanup", "backup-cleanup"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			target := t.TempDir()
			agent, original, originalInfo, manifest := writeAppendManifest(t, target)
			approved := planHash(t, manifest, target)
			testFailpoint = func(current string) bool { return current == stage }
			defer func() { testFailpoint = nil }()

			result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "E_APPLY_WRITE" {
				t.Fatalf("result = %#v", result)
			}
			assertRestoredAgentEntry(t, target, agent, original, originalInfo)
		})
	}
}

func TestUpdateRollbackFailureKeepsBackupAndRecoveryLock(t *testing.T) {
	target := t.TempDir()
	_, original, _, manifest := writeAppendManifest(t, target)
	approved := planHash(t, manifest, target)
	testFailpoint = func(stage string) bool {
		return stage == "update-publish" || stage == "rollback-update-rename"
	}
	defer func() { testFailpoint = nil }()

	result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
	if len(result.Diagnostics) != 2 || result.Diagnostics[0].Code != "E_APPLY_WRITE" || result.Diagnostics[1].Code != "E_APPLY_ROLLBACK" {
		t.Fatalf("result = %#v", result)
	}
	lock := filepath.Join(target, ".ai-governance", ".aigov-internal", "apply.lock")
	if raw, err := os.ReadFile(lock); err != nil || string(raw) != approved+"\n" {
		t.Fatalf("recovery lock = %q, %v", raw, err)
	}
	backups, err := filepath.Glob(filepath.Join(target, ".aigov-bak-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, %v", backups, err)
	}
	if raw, err := os.ReadFile(backups[0]); err != nil || string(raw) != string(original) {
		t.Fatalf("backup = %q, %v", raw, err)
	}
}

func TestExternalUpdateChangeStopsRollbackAndKeepsLock(t *testing.T) {
	target := t.TempDir()
	agent, _, _, manifest := writeAppendManifest(t, target)
	approved := planHash(t, manifest, target)
	external := []byte("external concurrent change\n")
	changed := false
	testFailpoint = func(stage string) bool {
		if stage == "update-publish" && !changed {
			changed = true
			if err := os.WriteFile(agent, external, 0o600); err != nil {
				t.Fatal(err)
			}
			return true
		}
		return false
	}
	defer func() { testFailpoint = nil }()

	result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
	if len(result.Diagnostics) != 2 || result.Diagnostics[0].Code != "E_APPLY_WRITE" || result.Diagnostics[1].Code != "E_APPLY_ROLLBACK" {
		t.Fatalf("result = %#v", result)
	}
	if raw, err := os.ReadFile(agent); err != nil || string(raw) != string(external) {
		t.Fatalf("external update was changed: %q, %v", raw, err)
	}
	lock := filepath.Join(target, ".ai-governance", ".aigov-internal", "apply.lock")
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("recovery lock missing: %v", err)
	}
}

func TestRollbackFailureMatrixKeepsRecoveryLock(t *testing.T) {
	cases := []struct {
		name     string
		primary  string
		rollback string
	}{
		{"final attribute", "publish", "rollback-final-attribute"},
		{"final hash", "publish", "rollback-final-hash"},
		{"final remove", "publish", "rollback-final-remove"},
		{"temp remove", "temp-write", "rollback-temp-remove"},
		{"lock remove", "lock-write", "rollback-lock-remove"},
		{"directory cleanup after lock removal", "internal-cleanup", "rollback-dir-remove"},
		{"directory cleanup after internal removal", "internal-cleanup", "rollback-dir-remove-after-internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := t.TempDir()
			manifest := writeManifest(t, target, nil)
			approved := planHash(t, manifest, target)
			testFailpoint = func(stage string) bool { return stage == tc.primary || stage == tc.rollback }
			defer func() { testFailpoint = nil }()
			result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
			if len(result.Diagnostics) != 2 || result.Diagnostics[0].Code != "E_APPLY_WRITE" || result.Diagnostics[1].Code != "E_APPLY_ROLLBACK" {
				t.Fatalf("result = %#v", result)
			}
			lock := filepath.Join(target, ".ai-governance", ".aigov-internal", "apply.lock")
			if raw, err := os.ReadFile(lock); err != nil || string(raw) != approved+"\n" {
				t.Fatalf("recovery lock = %q, %v", raw, err)
			}
		})
	}
}

func TestExternalFinalChangeStopsRollbackAndKeepsLock(t *testing.T) {
	target := t.TempDir()
	manifest := writeManifest(t, target, nil)
	approved := planHash(t, manifest, target)
	changed := false
	testFailpoint = func(stage string) bool {
		if stage == "publish" && !changed {
			changed = true
			if err := os.WriteFile(filepath.Join(target, ".ai-governance", "instance.json"), []byte("external\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return true
		}
		return false
	}
	defer func() { testFailpoint = nil }()
	result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
	if len(result.Diagnostics) != 2 || result.Diagnostics[1].Code != "E_APPLY_ROLLBACK" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(target, ".ai-governance", ".aigov-internal", "apply.lock")); err != nil {
		t.Fatalf("recovery lock missing: %v", err)
	}
}

func TestWindowsPublishedFinalSharesTempFileIdentity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows hard-link identity carries the inherited security descriptor")
	}
	target := t.TempDir()
	manifest := writeManifest(t, target, nil)
	approved := planHash(t, manifest, target)
	observed := false
	testFailpoint = func(stage string) bool {
		if stage != "final-verify" || observed {
			return false
		}
		finalInfo, err := os.Stat(filepath.Join(target, ".ai-governance", "instance.json"))
		if err != nil {
			return false
		}
		entries, err := os.ReadDir(filepath.Join(target, ".ai-governance"))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".aigov-tmp-") {
				tempInfo, statErr := os.Stat(filepath.Join(target, ".ai-governance", entry.Name()))
				if statErr == nil && os.SameFile(tempInfo, finalInfo) {
					observed = true
				}
			}
		}
		return false
	}
	defer func() { testFailpoint = nil }()
	result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
	if result.Output == nil || !observed {
		t.Fatalf("hard-link identity not observed: %#v", result)
	}
}

func TestRollbackFailureKeepsRecoverySignal(t *testing.T) {
	target := t.TempDir()
	manifest := writeManifest(t, target, nil)
	approved := planHash(t, manifest, target)
	testFailpoint = func(stage string) bool { return stage == "publish" || stage == "rollback-final" }
	defer func() { testFailpoint = nil }()
	result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
	if len(result.Diagnostics) != 2 || result.Diagnostics[0].Code != "E_APPLY_WRITE" || result.Diagnostics[1].Code != "E_APPLY_ROLLBACK" {
		t.Fatalf("result = %#v", result)
	}
	lock := filepath.Join(target, ".ai-governance", ".aigov-internal", "apply.lock")
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("recovery lock missing: %v", err)
	}
}

func TestInternalNamespaceCollisionIsZeroWrite(t *testing.T) {
	target := t.TempDir()
	manifest := writeManifest(t, target, map[string]any{"agent_entry": ".ai-governance/.aigov-internal/agent.md"})
	approved := planHash(t, manifest, target)
	result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "E_INTERNAL_PATH_COLLISION" {
		t.Fatalf("result = %#v", result)
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 0 {
		t.Fatalf("collision wrote target: %v", entries)
	}
}

func TestInternalPathCollisionMatrix(t *testing.T) {
	approved := strings.Repeat("a", 64)
	cases := []string{
		".ai-governance/.aigov-internal",
		".ai-governance/.aigov-internal/apply.lock",
		".ai-governance/.aigov-internal/child",
	}
	for _, collision := range cases {
		t.Run(collision, func(t *testing.T) {
			operations := []plan.Operation{
				{Action: "CREATE", LogicalID: "instance_manifest", Path: ".ai-governance/instance.json"},
				{Action: "CREATE", LogicalID: "agent_entry", Path: collision},
			}
			raw, err := json.Marshal(map[string]any{"operations": operations})
			if err != nil {
				t.Fatal(err)
			}
			result := plan.Result{ProjectRoot: t.TempDir(), Lines: []string{string(raw)}, Prepared: []plan.PreparedFile{{Operation: operations[1]}}}
			if _, code := internalPaths(result, approved); code != "E_INTERNAL_PATH_COLLISION" {
				t.Fatalf("collision %q code = %q", collision, code)
			}
		})
	}

	t.Run("final collides with temp or its child", func(t *testing.T) {
		logical := "source"
		temp := ".ai-governance/.aigov-tmp-" + approved + "-" + digest([]byte(logical))
		for _, collision := range []string{temp, temp + "/child"} {
			operations := []plan.Operation{
				{Action: "CREATE", LogicalID: "instance_manifest", Path: ".ai-governance/instance.json"},
				{Action: "CREATE", LogicalID: logical, Path: ".ai-governance/source.json"},
				{Action: "CREATE", LogicalID: "collision", Path: collision},
			}
			raw, _ := json.Marshal(map[string]any{"operations": operations})
			result := plan.Result{ProjectRoot: t.TempDir(), Lines: []string{string(raw)}, Prepared: []plan.PreparedFile{{Operation: operations[1]}}}
			if _, code := internalPaths(result, approved); code != "E_INTERNAL_PATH_COLLISION" {
				t.Fatalf("collision %q code = %q", collision, code)
			}
		}
	})

	t.Run("final collides with update backup or its child", func(t *testing.T) {
		logical := "agent_entry"
		backup := ".aigov-bak-" + approved + "-" + digest([]byte(logical))
		for _, collision := range []string{backup, backup + "/child"} {
			operations := []plan.Operation{
				{Action: "CREATE", LogicalID: "instance_manifest", Path: ".ai-governance/instance.json"},
				{Action: "UPDATE", LogicalID: logical, Path: "AGENTS.md"},
				{Action: "CREATE", LogicalID: "collision", Path: collision},
			}
			raw, _ := json.Marshal(map[string]any{"operations": operations})
			result := plan.Result{ProjectRoot: t.TempDir(), Lines: []string{string(raw)}, Prepared: []plan.PreparedFile{{Operation: operations[1]}}}
			if _, code := internalPaths(result, approved); code != "E_INTERNAL_PATH_COLLISION" {
				t.Fatalf("collision %q code = %q", collision, code)
			}
		}
	})

	t.Run("duplicate logical ids collide temps", func(t *testing.T) {
		operations := []plan.Operation{{Action: "CREATE", LogicalID: "instance_manifest", Path: ".ai-governance/instance.json"}}
		raw, _ := json.Marshal(map[string]any{"operations": operations})
		prepared := []plan.PreparedFile{
			{Operation: plan.Operation{LogicalID: "duplicate", Path: "one/file"}},
			{Operation: plan.Operation{LogicalID: "duplicate", Path: "two/file"}},
		}
		result := plan.Result{ProjectRoot: t.TempDir(), Lines: []string{string(raw)}, Prepared: prepared}
		if _, code := internalPaths(result, approved); code != "E_INTERNAL_PATH_COLLISION" {
			t.Fatalf("duplicate logical id code = %q", code)
		}
	})

	t.Run("internal to lock is legal", func(t *testing.T) {
		operation := plan.Operation{Action: "CREATE", LogicalID: "instance_manifest", Path: ".ai-governance/instance.json"}
		raw, _ := json.Marshal(map[string]any{"operations": []plan.Operation{operation}})
		result := plan.Result{ProjectRoot: t.TempDir(), Lines: []string{string(raw)}, Prepared: []plan.PreparedFile{{Operation: operation}}}
		if _, code := internalPaths(result, approved); code != "" {
			t.Fatalf("legal hierarchy code = %q", code)
		}
	})
}

func TestConcurrentApplyUsesExclusiveLock(t *testing.T) {
	target := t.TempDir()
	manifest := writeManifest(t, target, nil)
	approved := planHash(t, manifest, target)
	locked := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	testFailpoint = func(stage string) bool {
		if stage == "lock-create" {
			once.Do(func() {
				close(locked)
				<-release
			})
		}
		return false
	}
	defer func() { testFailpoint = nil }()
	firstDone := make(chan Result, 1)
	go func() { firstDone <- Execute(manifest, target, fixedNow, "0.1.0-dev", approved) }()
	<-locked
	second := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
	if len(second.Diagnostics) != 1 || second.Diagnostics[0].Code != "E_APPLY_LOCKED" {
		t.Fatalf("second apply = %#v", second)
	}
	close(release)
	first := <-firstDone
	if first.Output == nil || first.Output.Status != "APPLIED" {
		t.Fatalf("first apply = %#v", first)
	}
}

func TestPreexistingInternalArtifacts(t *testing.T) {
	t.Run("empty internal is preserved", func(t *testing.T) {
		target := t.TempDir()
		internal := filepath.Join(target, ".ai-governance", ".aigov-internal")
		if err := os.MkdirAll(internal, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := writeManifest(t, target, nil)
		approved := planHash(t, manifest, target)
		result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
		if result.Output == nil || result.Output.Status != "APPLIED" {
			t.Fatalf("result = %#v", result)
		}
		if info, err := os.Stat(internal); err != nil || !info.IsDir() {
			t.Fatalf("preexisting internal removed: %v, %v", info, err)
		}
	})
	t.Run("lock exists", func(t *testing.T) {
		target := t.TempDir()
		internal := filepath.Join(target, ".ai-governance", ".aigov-internal")
		if err := os.MkdirAll(internal, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(internal, "apply.lock"), []byte("stale\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		manifest := writeManifest(t, target, nil)
		approved := planHash(t, manifest, target)
		result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
		if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "E_APPLY_LOCKED" {
			t.Fatalf("result = %#v", result)
		}
	})
	t.Run("nonempty internal without lock", func(t *testing.T) {
		target := t.TempDir()
		internal := filepath.Join(target, ".ai-governance", ".aigov-internal")
		if err := os.MkdirAll(internal, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(internal, "orphan"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		manifest := writeManifest(t, target, nil)
		approved := planHash(t, manifest, target)
		result := Execute(manifest, target, fixedNow, "0.1.0-dev", approved)
		if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "E_APPLY_TEMP_CONFLICT" {
			t.Fatalf("result = %#v", result)
		}
	})
}

func planHash(t *testing.T, manifest, workingDir string) string {
	t.Helper()
	result := plan.Evaluate(manifest, workingDir, fixedNow, "0.1.0-dev")
	if len(result.Diagnostics) != 0 {
		t.Fatalf("plan diagnostics: %#v", result.Diagnostics)
	}
	sum := sha256.Sum256(result.StdoutBytes())
	return hex.EncodeToString(sum[:])
}

func writeManifest(t *testing.T, target string, resources map[string]any) string {
	t.Helper()
	if resources == nil {
		resources = map[string]any{}
	}
	manifest := map[string]any{
		"contract_version": "0.3-draft", "instance_id": "inst-example-01",
		"project": map[string]any{"name": "example-service", "root": target, "technology": []any{"Go"}, "has_frontend": false, "git_required": false},
		"governance": map[string]any{
			"phase": 0, "phase_basis": map[string]any{"assessed_at": "2026-11-25T12:00:00Z", "assessed_by": "human:project-owner", "signals": []any{"product_idea_exists"}},
			"core_hypothesis": "Target users choose X because Y", "selection_finalized": false, "selection_record": nil,
			"independent_review_available": false, "independent_review_source": nil,
		},
		"integration": map[string]any{"ci": "none", "mode": "apply"},
		"layout": map[string]any{
			"profile": "custom-v0", "resources": resources,
			"agent_entry_integration": map[string]any{"mode": "preserve-or-generate", "approved_sha256": nil},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeAppendManifest(t *testing.T, target string) (string, []byte, os.FileInfo, string) {
	t.Helper()
	agent := filepath.Join(target, "AGENTS.md")
	original := []byte("# Existing project instructions\r\n\r\nKeep this exact.\r\n")
	if err := os.WriteFile(agent, original, 0o600); err != nil {
		t.Fatal(err)
	}
	originalTime := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(agent, originalTime, originalTime); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(agent)
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"contract_version": "0.3-draft", "instance_id": "inst-example-01",
		"project": map[string]any{"name": "example-service", "root": target, "technology": []any{"Go"}, "has_frontend": false, "git_required": false},
		"governance": map[string]any{
			"phase": 0, "phase_basis": map[string]any{"assessed_at": "2026-11-25T12:00:00Z", "assessed_by": "human:project-owner", "signals": []any{"product_idea_exists"}},
			"core_hypothesis": "Target users choose X because Y", "selection_finalized": false, "selection_record": nil,
			"independent_review_available": false, "independent_review_source": nil,
		},
		"integration": map[string]any{"ci": "none", "mode": "apply"},
		"layout": map[string]any{
			"profile": "custom-v0", "resources": map[string]any{},
			"agent_entry_integration": map[string]any{"mode": "append-managed-reference", "approved_sha256": digest(original)},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return agent, original, originalInfo, manifestPath
}

func assertRestoredAgentEntry(t *testing.T, target, agent string, original []byte, originalInfo os.FileInfo) {
	t.Helper()
	raw, err := os.ReadFile(agent)
	if err != nil || string(raw) != string(original) {
		t.Fatalf("restored bytes = %q, %v", raw, err)
	}
	info, err := os.Stat(agent)
	if err != nil || info.Mode().Perm() != originalInfo.Mode().Perm() || !info.ModTime().Equal(originalInfo.ModTime()) {
		t.Fatalf("restored metadata = mode %v mtime %v; want mode %v mtime %v; err %v", info.Mode(), info.ModTime(), originalInfo.Mode(), originalInfo.ModTime(), err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 1 || entries[0].Name() != "AGENTS.md" {
		t.Fatalf("rollback left target entries %v, %v", entries, err)
	}
}
