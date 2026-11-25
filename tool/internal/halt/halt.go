// Package halt implements the phase-1 immutable attempt ledger and halt gate.
package halt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const recordSchemaVersion = "0.1-draft"

var (
	shaPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	taskPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	attemptPattern = regexp.MustCompile(`^a[0-9]{3}$`)
)

// Diagnostic is a stable command failure.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

// State is the state derived exclusively from verified attempt records.
type State struct {
	ConsecutiveCount           int     `json:"consecutive_count"`
	ConsecutiveSignatureSHA256 *string `json:"consecutive_signature_sha256"`
	LastAttemptID              *string `json:"last_attempt_id"`
	Status                     string  `json:"status"`
	TotalFailures              int     `json:"total_failures"`
}

// Result contains either a state or one diagnostic.
type Result struct {
	State      *State
	Diagnostic *Diagnostic
	ExitCode   int
}

type instance struct {
	InstanceID    string
	AuditRelative string
	CacheRelative string
}

type record struct {
	AttemptID       string  `json:"attempt_id"`
	ExitCode        int     `json:"exit_code"`
	InstanceID      string  `json:"instance_id"`
	Outcome         string  `json:"outcome"`
	RecordSHA256    string  `json:"record_sha256"`
	SchemaVersion   string  `json:"schema_version"`
	SignatureSHA256 *string `json:"signature_sha256"`
	TaskID          string  `json:"task_id"`
	Timestamp       string  `json:"timestamp"`
}

type unsignedRecord struct {
	AttemptID       string  `json:"attempt_id"`
	ExitCode        int     `json:"exit_code"`
	InstanceID      string  `json:"instance_id"`
	Outcome         string  `json:"outcome"`
	SchemaVersion   string  `json:"schema_version"`
	SignatureSHA256 *string `json:"signature_sha256"`
	TaskID          string  `json:"task_id"`
	Timestamp       string  `json:"timestamp"`
}

// WriteCanonical writes one canonical JSON line.
func WriteCanonical(writer io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = writer.Write(append(raw, '\n'))
	return err
}

// RecordAttempt validates, appends, and derives one task attempt.
func RecordAttempt(projectRoot, taskID, attemptID string, exitCode int, signature string, now time.Time) Result {
	if !taskPattern.MatchString(taskID) {
		return fail("E_HALT_REQUEST", "/task_id", "use a safe task ID")
	}
	if !attemptPattern.MatchString(attemptID) || attemptID == "a000" {
		return fail("E_HALT_REQUEST", "/attempt_id", "use a sequential attempt ID matching aNNN")
	}
	if (exitCode == 0 && signature != "") || (exitCode != 0 && !shaPattern.MatchString(signature)) {
		return fail("E_HALT_REQUEST", "/signature_sha256", "success requires no signature; failure requires 64 lowercase hex characters")
	}
	root, inst, diagnostic := loadInstance(projectRoot)
	if diagnostic != nil {
		return Result{Diagnostic: diagnostic, ExitCode: 2}
	}
	taskRoot := filepath.Join(root, filepath.FromSlash(inst.AuditRelative), taskID)
	attemptsRoot := filepath.Join(taskRoot, "attempts")
	if diagnostic := validateSafeChain(root, attemptsRoot); diagnostic != nil {
		return Result{Diagnostic: diagnostic, ExitCode: 2}
	}
	preState, _, preDiagnostic := replay(inst, taskID, attemptsRoot)
	if preDiagnostic != nil {
		return Result{Diagnostic: preDiagnostic, ExitCode: 2}
	}
	if preState.Status == "HALTED_FOR_HUMAN" {
		return failWithCode("E_TASK_HALTED", "/task_id", "task requires human intervention", 3)
	}
	if diagnostic := ensureSafeDirectories(root, attemptsRoot); diagnostic != nil {
		return Result{Diagnostic: diagnostic, ExitCode: 2}
	}
	taskInfo, _ := os.Stat(taskRoot)
	lockPath := filepath.Join(taskRoot, ".record-attempt.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fail("E_CONCURRENT_UPDATE", "/task_id", "another update holds this task lock")
		}
		return fail("E_HALT_WRITE", "/task_id", "cannot create the task lock")
	}
	_ = lock.Close()
	defer func() {
		_ = os.Remove(lockPath)
		if taskInfo != nil {
			_ = os.Chtimes(taskRoot, taskInfo.ModTime(), taskInfo.ModTime())
		}
	}()

	state, records, diagnostic := replay(inst, taskID, attemptsRoot)
	if diagnostic != nil {
		return Result{Diagnostic: diagnostic, ExitCode: 2}
	}
	if state.Status == "HALTED_FOR_HUMAN" {
		return failWithCode("E_TASK_HALTED", "/task_id", "task requires human intervention", 3)
	}
	wantAttempt := fmt.Sprintf("a%03d", len(records)+1)
	if attemptID != wantAttempt {
		return fail("E_ATTEMPT_SEQUENCE", "/attempt_id", "provide the next sequential attempt ID "+wantAttempt)
	}

	created := record{
		AttemptID: attemptID, ExitCode: exitCode, InstanceID: inst.InstanceID,
		Outcome: "FAILURE", SchemaVersion: recordSchemaVersion, TaskID: taskID,
		Timestamp: now.UTC().Truncate(time.Second).Format(time.RFC3339),
	}
	if exitCode == 0 {
		created.Outcome = "SUCCESS"
	} else {
		created.SignatureSHA256 = &signature
	}
	unsigned := unsignedRecord{
		AttemptID: created.AttemptID, ExitCode: created.ExitCode, InstanceID: created.InstanceID,
		Outcome: created.Outcome, SchemaVersion: created.SchemaVersion,
		SignatureSHA256: created.SignatureSHA256, TaskID: created.TaskID, Timestamp: created.Timestamp,
	}
	unsignedBytes, _ := json.Marshal(unsigned)
	created.RecordSHA256 = digest(unsignedBytes)
	raw, _ := json.Marshal(created)
	if diagnostic := publishNoReplace(attemptsRoot, attemptID+".json", append(raw, '\n')); diagnostic != nil {
		return Result{Diagnostic: diagnostic, ExitCode: 2}
	}
	state = applyRecord(state, created)
	cacheRoot := filepath.Join(root, filepath.FromSlash(inst.CacheRelative))
	if diagnostic := ensureSafeDirectories(root, cacheRoot); diagnostic != nil {
		return Result{State: &state, ExitCode: stateExit(state)}
	}
	cacheBytes, _ := json.Marshal(state)
	_ = replaceCache(cacheRoot, taskID+".json", append(cacheBytes, '\n'))
	return Result{State: &state, ExitCode: stateExit(state)}
}

// Status replays the immutable ledger without writing cache or locks.
func Status(projectRoot, taskID string) Result {
	if !taskPattern.MatchString(taskID) {
		return fail("E_HALT_REQUEST", "/task_id", "use a safe task ID")
	}
	root, inst, diagnostic := loadInstance(projectRoot)
	if diagnostic != nil {
		return Result{Diagnostic: diagnostic, ExitCode: 2}
	}
	attemptsRoot := filepath.Join(root, filepath.FromSlash(inst.AuditRelative), taskID, "attempts")
	if diagnostic := validateSafeChain(root, attemptsRoot); diagnostic != nil {
		return Result{Diagnostic: diagnostic, ExitCode: 2}
	}
	state, _, diagnostic := replay(inst, taskID, attemptsRoot)
	if diagnostic != nil {
		return Result{Diagnostic: diagnostic, ExitCode: 2}
	}
	return Result{State: &state, ExitCode: stateExit(state)}
}

func replay(inst instance, taskID, attemptsRoot string) (State, []record, *Diagnostic) {
	state := State{Status: "EXECUTING"}
	info, err := os.Lstat(attemptsRoot)
	if os.IsNotExist(err) {
		return state, nil, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return state, nil, haltAudit("attempts store is not a safe directory")
	}
	entries, err := os.ReadDir(attemptsRoot)
	if err != nil {
		return state, nil, haltAudit("cannot read attempt ledger")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	records := make([]record, 0, len(entries))
	for index, entry := range entries {
		wantName := fmt.Sprintf("a%03d.json", index+1)
		if entry.Name() != wantName || entry.Type()&os.ModeSymlink != 0 {
			return state, nil, haltAudit("attempt ledger sequence or object type is invalid")
		}
		filePath := filepath.Join(attemptsRoot, entry.Name())
		fileInfo, err := os.Lstat(filePath)
		if err != nil || !fileInfo.Mode().IsRegular() {
			return state, nil, haltAudit("attempt record is not a regular file")
		}
		raw, err := os.ReadFile(filePath)
		if err != nil || !bytes.HasSuffix(raw, []byte{'\n'}) || bytes.Count(raw, []byte{'\n'}) != 1 {
			return state, nil, haltAudit("attempt record is not one canonical JSON line")
		}
		var item record
		if decodeStrict(raw[:len(raw)-1], &item) != nil {
			return state, nil, haltAudit("attempt record JSON is invalid")
		}
		canonical, _ := json.Marshal(item)
		if !bytes.Equal(canonical, raw[:len(raw)-1]) || item.AttemptID+".json" != entry.Name() || item.TaskID != taskID || item.InstanceID != inst.InstanceID || item.SchemaVersion != recordSchemaVersion {
			return state, nil, haltAudit("attempt record binding or canonical bytes are invalid")
		}
		if item.Outcome == "SUCCESS" {
			if item.ExitCode != 0 || item.SignatureSHA256 != nil {
				return state, nil, haltAudit("success record fields are inconsistent")
			}
		} else if item.Outcome == "FAILURE" {
			if item.ExitCode == 0 || item.SignatureSHA256 == nil || !shaPattern.MatchString(*item.SignatureSHA256) {
				return state, nil, haltAudit("failure record fields are inconsistent")
			}
		} else {
			return state, nil, haltAudit("attempt outcome is invalid")
		}
		parsedTime, err := time.Parse(time.RFC3339, item.Timestamp)
		if err != nil || parsedTime.Location() != time.UTC || parsedTime.Nanosecond() != 0 || parsedTime.Format(time.RFC3339) != item.Timestamp {
			return state, nil, haltAudit("attempt timestamp is invalid")
		}
		unsigned := unsignedRecord{AttemptID: item.AttemptID, ExitCode: item.ExitCode, InstanceID: item.InstanceID, Outcome: item.Outcome, SchemaVersion: item.SchemaVersion, SignatureSHA256: item.SignatureSHA256, TaskID: item.TaskID, Timestamp: item.Timestamp}
		unsignedBytes, _ := json.Marshal(unsigned)
		if item.RecordSHA256 != digest(unsignedBytes) {
			return state, nil, haltAudit("attempt record hash is invalid")
		}
		state = applyRecord(state, item)
		records = append(records, item)
	}
	return state, records, nil
}

func applyRecord(state State, item record) State {
	last := item.AttemptID
	state.LastAttemptID = &last
	if item.Outcome == "SUCCESS" {
		state.ConsecutiveCount = 0
		state.ConsecutiveSignatureSHA256 = nil
	} else {
		state.TotalFailures++
		if state.ConsecutiveSignatureSHA256 != nil && *state.ConsecutiveSignatureSHA256 == *item.SignatureSHA256 {
			state.ConsecutiveCount++
		} else {
			signature := *item.SignatureSHA256
			state.ConsecutiveSignatureSHA256 = &signature
			state.ConsecutiveCount = 1
		}
	}
	if state.ConsecutiveCount >= 3 {
		state.Status = "HALTED_FOR_HUMAN"
	}
	return state
}

func loadInstance(projectRoot string) (string, instance, *Diagnostic) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", instance{}, requestDiagnostic("/project", "project root is invalid")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", instance{}, requestDiagnostic("/project", "project root must be a real directory")
	}
	path := filepath.Join(root, ".ai-governance", "instance.json")
	if !safeRegularWithin(root, path) {
		return "", instance{}, haltAudit("instance manifest is not a safe regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", instance{}, haltAudit("cannot read instance manifest")
	}
	var value struct {
		AssuranceLevel         string            `json:"assurance_level"`
		InstallContractVersion string            `json:"install_contract_version"`
		InstanceID             string            `json:"instance_id"`
		ManagedFiles           map[string]string `json:"managed_files"`
		MethodologyVersion     string            `json:"methodology_version"`
		Resources              map[string]string `json:"resources"`
		SchemaVersion          string            `json:"schema_version"`
	}
	if decodeStrict(raw, &value) != nil || value.InstallContractVersion != "0.4-draft" || value.InstanceID == "" {
		return "", instance{}, haltAudit("instance manifest is invalid")
	}
	audit, auditOK := safeRelative(value.Resources["task_audit_store"])
	cache, cacheOK := safeRelative(value.Resources["task_state_cache"])
	runtimeRelative, runtimeOK := safeRelative(value.Resources["runtime_entrypoint"])
	runtimeHash := value.ManagedFiles["runtime_entrypoint"]
	if !auditOK || !cacheOK || !runtimeOK || !shaPattern.MatchString(runtimeHash) {
		return "", instance{}, haltAudit("instance halt resources are unsafe")
	}
	executable, err := os.Executable()
	if err != nil {
		return "", instance{}, haltAudit("cannot resolve installed runtime")
	}
	executable, _ = filepath.Abs(executable)
	wantExecutable := filepath.Join(root, filepath.FromSlash(runtimeRelative))
	if !samePath(executable, wantExecutable) || !safeRegularWithin(root, wantExecutable) {
		return "", instance{}, haltAudit("command is not the installed instance runtime")
	}
	runtimeBytes, err := os.ReadFile(executable)
	if err != nil || digest(runtimeBytes) != runtimeHash {
		return "", instance{}, haltAudit("installed runtime hash does not match instance manifest")
	}
	return root, instance{InstanceID: value.InstanceID, AuditRelative: audit, CacheRelative: cache}, nil
}

func decodeStrict(raw []byte, target any) error {
	if err := rejectDuplicateKeys(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func rejectDuplicateKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return fmt.Errorf("duplicate or invalid object key")
			}
			seen[key] = true
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("invalid object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("invalid array")
		}
	default:
		return fmt.Errorf("invalid JSON delimiter")
	}
	return nil
}

func ensureSafeDirectories(root, target string) *Diagnostic {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return requestDiagnostic("/project", "target escapes project root")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return requestDiagnostic("/project", "cannot create halt storage")
			}
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return haltAudit("halt storage contains an unsafe object")
		}
	}
	return nil
}

func validateSafeChain(root, target string) *Diagnostic {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return requestDiagnostic("/project", "target escapes project root")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return haltAudit("halt storage contains an unsafe object")
		}
	}
	return nil
}

func publishNoReplace(directory, name string, content []byte) *Diagnostic {
	temp, err := os.CreateTemp(directory, ".aigov-attempt-*")
	if err != nil {
		return requestDiagnostic("/attempt_id", "cannot create attempt temp file")
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return requestDiagnostic("/attempt_id", "cannot secure attempt temp file")
	}
	if _, err := temp.Write(content); err != nil || temp.Sync() != nil || temp.Close() != nil {
		return requestDiagnostic("/attempt_id", "cannot persist attempt record")
	}
	finalPath := filepath.Join(directory, name)
	if err := os.Link(tempPath, finalPath); err != nil {
		if _, statErr := os.Lstat(finalPath); statErr == nil {
			return requestDiagnostic("/attempt_id", "attempt record already exists")
		}
		return requestDiagnostic("/attempt_id", "cannot publish attempt record")
	}
	return nil
}

func replaceCache(directory, name string, content []byte) error {
	finalPath := filepath.Join(directory, name)
	if info, err := os.Lstat(finalPath); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("unsafe cache")
	}
	temp, err := os.CreateTemp(directory, ".aigov-cache-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil || temp.Sync() != nil || temp.Close() != nil {
		return fmt.Errorf("cache write failed")
	}
	return os.Rename(tempPath, finalPath)
}

func safeRegularWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if current != target && !info.IsDir() {
			return false
		}
	}
	info, err := os.Lstat(target)
	return err == nil && info.Mode().IsRegular()
}

func safeRelative(value string) (string, bool) {
	if value == "" || filepath.IsAbs(value) || regexp.MustCompile(`^[A-Za-z]:`).MatchString(value) {
		return "", false
	}
	replaced := strings.ReplaceAll(value, `\`, "/")
	for _, part := range strings.Split(replaced, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	return replaced, true
}

func samePath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if filepath.Separator == '\\' {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func stateExit(state State) int {
	if state.Status == "HALTED_FOR_HUMAN" {
		return 3
	}
	return 0
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func haltAudit(message string) *Diagnostic {
	return &Diagnostic{Code: "E_HALT_AUDIT", Message: message, Path: "/attempts"}
}

func requestDiagnostic(path, message string) *Diagnostic {
	return &Diagnostic{Code: "E_HALT_REQUEST", Message: message, Path: path}
}

func fail(code, path, message string) Result {
	return failWithCode(code, path, message, 2)
}

func failWithCode(code, path, message string, exitCode int) Result {
	return Result{Diagnostic: &Diagnostic{Code: code, Message: message, Path: path}, ExitCode: exitCode}
}

// ParseExitCode parses the CLI integer without accepting fractional syntax.
func ParseExitCode(value string) (int, error) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int(parsed), err
}
