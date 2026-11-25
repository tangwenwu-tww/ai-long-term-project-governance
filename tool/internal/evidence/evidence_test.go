package evidence

import (
	"strings"
	"testing"
)

func TestSafeAllowedPattern(t *testing.T) {
	accepted := []string{"file.txt", "src/", "tests/*.json", "docs/file-?.md"}
	for _, value := range accepted {
		if !safeAllowedPattern(value) {
			t.Errorf("safeAllowedPattern(%q) = false, want true", value)
		}
	}
	rejected := []string{"", "/absolute", `C:\absolute`, `dir\file`, "../escape", "dir/../escape", "**/*.go", "dir/[ab].go", "dir//file"}
	for _, value := range rejected {
		if safeAllowedPattern(value) {
			t.Errorf("safeAllowedPattern(%q) = true, want false", value)
		}
	}
}

func TestDiffSnapshotsIncludesTypeAndDeletionHashes(t *testing.T) {
	before := Snapshot{SchemaVersion: schemaVersion, Files: []SnapshotEntry{
		{Path: "deleted.txt", Type: "file", SHA256: strings.Repeat("1", 64), ByteLength: 7},
		{Path: "typed", Type: "file", SHA256: strings.Repeat("2", 64), ByteLength: 4},
	}}
	after := Snapshot{SchemaVersion: schemaVersion, Files: []SnapshotEntry{
		{Path: "typed", Type: "symlink", SHA256: strings.Repeat("3", 64), ByteLength: 8},
	}}
	changes := diffSnapshots(before, after)
	if len(changes) != 2 {
		t.Fatalf("changes = %#v, want two", changes)
	}
	if changes[0].Path != "deleted.txt" || changes[0].Change != "deleted" || changes[0].BeforeSHA256 != strings.Repeat("1", 64) || changes[0].AfterSHA256 != "" {
		t.Fatalf("deleted change = %#v", changes[0])
	}
	if changes[1].Path != "typed" || changes[1].Change != "type_changed" || changes[1].BeforeType != "file" || changes[1].AfterType != "symlink" {
		t.Fatalf("type change = %#v", changes[1])
	}
}

func TestValidateRequestRejectsUnmappedAndUnsafeInputs(t *testing.T) {
	base := Request{
		SchemaVersion: schemaVersion, TaskID: "T-TEST-001", TaskRevision: 1, AttemptID: "a001",
		TaskSHA256: strings.Repeat("1", 64), AllowedFiles: []string{"src/"},
		Commands:           []CommandRequest{{ID: "verify", Executable: "go", Args: []string{"test"}, CWD: ".", ExpectedExitCode: 0}},
		AcceptanceCriteria: []AcceptanceCriterion{{ID: "AC-01", Text: "verify", CommandIDs: []string{"verify"}}},
	}
	if err := validateRequest(base); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	unsafe := base
	unsafe.AllowedFiles = []string{"../escape"}
	if err := validateRequest(unsafe); err == nil {
		t.Fatal("unsafe allowed path accepted")
	}
	unmapped := base
	unmapped.AcceptanceCriteria = []AcceptanceCriterion{{ID: "AC-01", Text: "verify", CommandIDs: nil}}
	if err := validateRequest(unmapped); err == nil {
		t.Fatal("unmapped acceptance criterion accepted")
	}
}
