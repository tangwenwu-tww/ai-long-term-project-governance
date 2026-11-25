package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const schemaVersion = "0.1-draft"

var (
	idPattern      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{1,127}$`)
	attemptPattern = regexp.MustCompile(`^a[0-9]{3,}$`)
	shaPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type Result struct {
	Output      any
	Diagnostics []Diagnostic
	ExitCode    int
}

type Request struct {
	SchemaVersion      string                `json:"schema_version"`
	TaskID             string                `json:"task_id"`
	TaskRevision       int                   `json:"task_revision"`
	AttemptID          string                `json:"attempt_id"`
	TaskSHA256         string                `json:"task_sha256"`
	AllowedFiles       []string              `json:"allowed_files"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
	Commands           []CommandRequest      `json:"commands"`
}

type AcceptanceCriterion struct {
	ID         string   `json:"id"`
	Text       string   `json:"text"`
	CommandIDs []string `json:"command_ids"`
}

type CommandRequest struct {
	ID               string   `json:"id"`
	Executable       string   `json:"executable"`
	Args             []string `json:"args"`
	CWD              string   `json:"cwd"`
	ExpectedExitCode int      `json:"expected_exit_code"`
}

type instanceManifest struct {
	Resources struct {
		EvidenceStore string `json:"evidence_store"`
	} `json:"resources"`
}

type Snapshot struct {
	SchemaVersion string          `json:"schema_version"`
	Files         []SnapshotEntry `json:"files"`
}

type SnapshotEntry struct {
	Path       string `json:"path"`
	Type       string `json:"type"`
	SHA256     string `json:"sha256"`
	ByteLength int64  `json:"byte_length"`
}

type beginReceipt struct {
	SchemaVersion  string   `json:"schema_version"`
	TaskID         string   `json:"task_id"`
	AttemptID      string   `json:"attempt_id"`
	RequestSHA256  string   `json:"request_sha256"`
	BaselineSHA256 string   `json:"baseline_sha256"`
	ExcludedPaths  []string `json:"excluded_paths"`
}

type collectReceipt struct {
	ManifestPath   string `json:"manifest_path"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type streamSummary struct {
	SHA256     string `json:"sha256"`
	ByteLength int    `json:"byte_length"`
	ValidUTF8  bool   `json:"valid_utf8"`
	LineCount  *int   `json:"line_count,omitempty"`
}

type commandResult struct {
	ID                   string        `json:"id"`
	Executable           string        `json:"executable"`
	Args                 []string      `json:"args"`
	CWD                  string        `json:"cwd"`
	ExpectedExitCode     int           `json:"expected_exit_code"`
	ExitCode             int           `json:"exit_code"`
	StartedAt            string        `json:"started_at"`
	EndedAt              string        `json:"ended_at"`
	DurationMilliseconds int64         `json:"duration_milliseconds"`
	ExecutableSHA256     string        `json:"executable_sha256"`
	Stdout               streamSummary `json:"stdout"`
	Stderr               streamSummary `json:"stderr"`
}

type changedFile struct {
	Path             string `json:"path"`
	Change           string `json:"change"`
	BeforeType       string `json:"before_type,omitempty"`
	BeforeSHA256     string `json:"before_sha256,omitempty"`
	BeforeByteLength *int64 `json:"before_byte_length,omitempty"`
	AfterType        string `json:"after_type,omitempty"`
	AfterSHA256      string `json:"after_sha256,omitempty"`
	AfterByteLength  *int64 `json:"after_byte_length,omitempty"`
}

type acceptanceResult struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	CommandIDs   []string `json:"command_ids"`
	Verified     bool     `json:"verified"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type manifest struct {
	SchemaVersion       string             `json:"schema_version"`
	TaskID              string             `json:"task_id"`
	TaskRevision        int                `json:"task_revision"`
	AttemptID           string             `json:"attempt_id"`
	TaskSHA256          string             `json:"task_sha256"`
	RequestSHA256       string             `json:"request_sha256"`
	BeginSHA256         string             `json:"begin_sha256"`
	BaselineSHA256      string             `json:"baseline_sha256"`
	PostStateSHA256     string             `json:"post_state_sha256"`
	Platform            platform           `json:"platform"`
	Commands            []commandResult    `json:"commands"`
	ChangedFiles        []changedFile      `json:"changed_files"`
	OutsideAllowedFiles []string           `json:"outside_allowed_files"`
	WithinAllowedFiles  bool               `json:"within_allowed_files"`
	AcceptanceChecklist []acceptanceResult `json:"acceptance_checklist"`
	Status              string             `json:"status"`
}

type platform struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

type preparedCommand struct {
	request  CommandRequest
	cwd      string
	resolved string
	hash     string
}

type contractError struct {
	code, pointer, message string
}

func (e *contractError) Error() string { return e.message }

func Begin(projectRoot, requestPath string) Result {
	ctx, err := loadContext(projectRoot, requestPath)
	if err != nil {
		return diagnosticResult(err)
	}
	if _, err := os.Lstat(ctx.attemptPath); err == nil {
		return diag("E_EVIDENCE_SEALED", "/attempt_id", "evidence attempt already exists")
	} else if !os.IsNotExist(err) {
		return diag("E_EVIDENCE_IO", "/attempt_id", "cannot inspect evidence attempt")
	}

	baseline, err := snapshot(ctx.root, ctx.attemptRelative)
	if err != nil {
		return diagnosticResult(err)
	}
	baselineBytes, err := canonicalLine(baseline)
	if err != nil {
		return diag("E_EVIDENCE_IO", "/baseline", "cannot encode baseline")
	}
	if err := createAttempt(ctx, baselineBytes); err != nil {
		return diagnosticResult(err)
	}
	receipt := beginReceipt{
		SchemaVersion: schemaVersion, TaskID: ctx.request.TaskID, AttemptID: ctx.request.AttemptID,
		RequestSHA256: digest(ctx.requestRaw), BaselineSHA256: digest(baselineBytes),
		ExcludedPaths: exclusions(ctx.attemptRelative),
	}
	return Result{Output: receipt, ExitCode: 0}
}

func Collect(projectRoot, requestPath, approvedBeginSHA256 string, clock func() time.Time) Result {
	if !shaPattern.MatchString(approvedBeginSHA256) {
		return diag("E_EVIDENCE_BEGIN_MISMATCH", "/begin_sha256", "provide the complete begin stdout SHA-256")
	}
	ctx, err := loadContext(projectRoot, requestPath)
	if err != nil {
		return diagnosticResult(err)
	}
	manifestPath := filepath.Join(ctx.attemptPath, "manifest.json")
	if _, err := os.Lstat(manifestPath); err == nil {
		return diag("E_EVIDENCE_SEALED", "/attempt_id", "evidence attempt is already sealed")
	} else if !os.IsNotExist(err) {
		return diag("E_EVIDENCE_IO", "/attempt_id", "cannot inspect sealed evidence attempt")
	}

	storedRequest, err := readRegular(filepath.Join(ctx.attemptPath, "request.json"))
	if err != nil || !bytes.Equal(storedRequest, ctx.requestRaw) {
		return diag("E_EVIDENCE_BEGIN_MISMATCH", "/request", "request no longer matches begin")
	}
	baselineBytes, err := readRegular(filepath.Join(ctx.attemptPath, "baseline.json"))
	if err != nil {
		return diag("E_EVIDENCE_BEGIN_MISMATCH", "/baseline", "baseline is missing or unsafe")
	}
	var baseline Snapshot
	if err := decodeStrict(baselineBytes, &baseline); err != nil || baseline.SchemaVersion != schemaVersion {
		return diag("E_EVIDENCE_BEGIN_MISMATCH", "/baseline", "baseline is not canonical evidence data")
	}
	canonicalBaseline, err := canonicalLine(baseline)
	if err != nil || !bytes.Equal(canonicalBaseline, baselineBytes) {
		return diag("E_EVIDENCE_BEGIN_MISMATCH", "/baseline", "baseline bytes changed after begin")
	}
	receipt := beginReceipt{
		SchemaVersion: schemaVersion, TaskID: ctx.request.TaskID, AttemptID: ctx.request.AttemptID,
		RequestSHA256: digest(ctx.requestRaw), BaselineSHA256: digest(baselineBytes),
		ExcludedPaths: exclusions(ctx.attemptRelative),
	}
	receiptBytes, _ := canonicalLine(receipt)
	if digest(receiptBytes) != approvedBeginSHA256 {
		return diag("E_EVIDENCE_BEGIN_MISMATCH", "/begin_sha256", "begin stdout SHA-256 does not match this request and baseline")
	}

	prepared, err := prepareCommands(ctx.root, ctx.request.Commands)
	if err != nil {
		return diagnosticResult(err)
	}
	if clock == nil {
		clock = time.Now
	}
	results := make([]commandResult, 0, len(prepared))
	commandsMatch := true
	for _, command := range prepared {
		result, err := executeCommand(command, clock)
		if err != nil {
			return diagnosticResult(err)
		}
		if result.ExitCode != result.ExpectedExitCode {
			commandsMatch = false
		}
		results = append(results, result)
	}

	post, err := snapshot(ctx.root, ctx.attemptRelative)
	if err != nil {
		return diagnosticResult(err)
	}
	postBytes, err := canonicalLine(post)
	if err != nil {
		return diag("E_EVIDENCE_IO", "/post_state", "cannot encode post-state")
	}
	changes := diffSnapshots(baseline, post)
	outside := outsideAllowed(changes, ctx.request.AllowedFiles)
	checklist, allCriteria := acceptanceChecklist(ctx.request, results)
	status := "PASS"
	if !commandsMatch || len(outside) != 0 || !allCriteria {
		status = "FAIL"
	}
	output := manifest{
		SchemaVersion: schemaVersion, TaskID: ctx.request.TaskID, TaskRevision: ctx.request.TaskRevision,
		AttemptID: ctx.request.AttemptID, TaskSHA256: ctx.request.TaskSHA256,
		RequestSHA256: digest(ctx.requestRaw), BeginSHA256: approvedBeginSHA256,
		BaselineSHA256: digest(baselineBytes), PostStateSHA256: digest(postBytes),
		Platform: platform{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}, Commands: results,
		ChangedFiles: changes, OutsideAllowedFiles: outside, WithinAllowedFiles: len(outside) == 0,
		AcceptanceChecklist: checklist, Status: status,
	}
	manifestBytes, err := canonicalLine(output)
	if err != nil {
		return diag("E_EVIDENCE_IO", "/manifest", "cannot encode evidence manifest")
	}
	if err := publishNoReplace(manifestPath, manifestBytes); err != nil {
		if errors.Is(err, os.ErrExist) {
			return diag("E_EVIDENCE_SEALED", "/attempt_id", "evidence attempt is already sealed")
		}
		return diag("E_EVIDENCE_IO", "/manifest", "cannot publish evidence manifest")
	}
	rel, _ := filepath.Rel(ctx.root, manifestPath)
	result := Result{Output: collectReceipt{ManifestPath: filepath.ToSlash(rel), ManifestSHA256: digest(manifestBytes)}, ExitCode: 0}
	if status == "FAIL" {
		result.ExitCode = 1
	}
	return result
}

type context struct {
	root, requestPath, attemptPath, attemptRelative string
	requestRaw                                      []byte
	request                                         Request
}

func loadContext(projectRoot, requestPath string) (*context, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, &contractError{"E_EVIDENCE_PATH", "/project", "cannot resolve project root"}
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, &contractError{"E_EVIDENCE_PATH", "/project", "project root must be a safe directory"}
	}
	if link, err := linkLike(root, info); err != nil || link {
		return nil, &contractError{"E_EVIDENCE_PATH", "/project", "project root cannot be link-like"}
	}
	requestAbsolute, err := filepath.Abs(requestPath)
	if err != nil || !withinRoot(root, requestAbsolute) {
		return nil, &contractError{"E_EVIDENCE_PATH", "/request", "request must be inside project root"}
	}
	raw, err := readRegularWithin(root, requestAbsolute)
	if err != nil {
		return nil, &contractError{"E_EVIDENCE_PATH", "/request", "request must be a safe regular file inside project root"}
	}
	var request Request
	if err := decodeStrict(raw, &request); err != nil {
		return nil, &contractError{"E_EVIDENCE_REQUEST", "/", "request must be strict JSON matching evidence 0.1-draft"}
	}
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	instanceBytes, err := readRegularWithin(root, filepath.Join(root, ".ai-governance", "instance.json"))
	if err != nil {
		return nil, &contractError{"E_EVIDENCE_PATH", "/instance", "governance instance manifest is missing or unsafe"}
	}
	var instance instanceManifest
	if err := json.Unmarshal(instanceBytes, &instance); err != nil {
		return nil, &contractError{"E_EVIDENCE_REQUEST", "/instance", "governance instance manifest is invalid"}
	}
	store, ok := safeRelative(instance.Resources.EvidenceStore, false)
	if !ok {
		return nil, &contractError{"E_EVIDENCE_PATH", "/instance/resources/evidence_store", "evidence_store must be a safe relative path"}
	}
	attemptRel := path.Join(store, request.TaskID, request.AttemptID)
	attemptPath := filepath.Join(root, filepath.FromSlash(attemptRel))
	return &context{root: root, requestPath: requestAbsolute, requestRaw: raw, request: request, attemptPath: attemptPath, attemptRelative: attemptRel}, nil
}

func validateRequest(request Request) error {
	if request.SchemaVersion != schemaVersion {
		return &contractError{"E_EVIDENCE_REQUEST", "/schema_version", "schema_version must be 0.1-draft"}
	}
	if !idPattern.MatchString(request.TaskID) {
		return &contractError{"E_EVIDENCE_REQUEST", "/task_id", "task_id is invalid"}
	}
	if request.TaskRevision < 1 {
		return &contractError{"E_EVIDENCE_REQUEST", "/task_revision", "task_revision must be positive"}
	}
	if !attemptPattern.MatchString(request.AttemptID) {
		return &contractError{"E_EVIDENCE_REQUEST", "/attempt_id", "attempt_id must match aNNN"}
	}
	if !shaPattern.MatchString(request.TaskSHA256) {
		return &contractError{"E_EVIDENCE_REQUEST", "/task_sha256", "task_sha256 must be lowercase SHA-256"}
	}
	if request.AllowedFiles == nil {
		return &contractError{"E_EVIDENCE_REQUEST", "/allowed_files", "allowed_files is required"}
	}
	for i, pattern := range request.AllowedFiles {
		if !safeAllowedPattern(pattern) {
			return &contractError{"E_EVIDENCE_REQUEST", fmt.Sprintf("/allowed_files/%d", i), "allowed path pattern is unsafe"}
		}
	}
	if len(request.Commands) == 0 {
		return &contractError{"E_EVIDENCE_REQUEST", "/commands", "at least one command is required"}
	}
	commandIDs := map[string]bool{}
	for i, command := range request.Commands {
		base := fmt.Sprintf("/commands/%d", i)
		if !idPattern.MatchString(command.ID) || commandIDs[command.ID] {
			return &contractError{"E_EVIDENCE_REQUEST", base + "/id", "command id is invalid or duplicated"}
		}
		commandIDs[command.ID] = true
		if strings.TrimSpace(command.Executable) == "" || strings.ContainsRune(command.Executable, 0) {
			return &contractError{"E_EVIDENCE_REQUEST", base + "/executable", "executable is required"}
		}
		if command.Args == nil {
			return &contractError{"E_EVIDENCE_REQUEST", base + "/args", "args must be an array"}
		}
		for _, arg := range command.Args {
			if strings.ContainsRune(arg, 0) {
				return &contractError{"E_EVIDENCE_REQUEST", base + "/args", "arguments cannot contain NUL"}
			}
		}
		if _, ok := safeRelative(command.CWD, true); !ok {
			return &contractError{"E_EVIDENCE_REQUEST", base + "/cwd", "cwd must be a safe relative directory"}
		}
	}
	if len(request.AcceptanceCriteria) == 0 {
		return &contractError{"E_EVIDENCE_REQUEST", "/acceptance_criteria", "at least one acceptance criterion is required"}
	}
	criterionIDs := map[string]bool{}
	for i, criterion := range request.AcceptanceCriteria {
		base := fmt.Sprintf("/acceptance_criteria/%d", i)
		if !idPattern.MatchString(criterion.ID) || criterionIDs[criterion.ID] {
			return &contractError{"E_EVIDENCE_REQUEST", base + "/id", "criterion id is invalid or duplicated"}
		}
		criterionIDs[criterion.ID] = true
		if strings.TrimSpace(criterion.Text) == "" || len(criterion.CommandIDs) == 0 {
			return &contractError{"E_EVIDENCE_REQUEST", base + "/command_ids", "criterion text and command mapping are required"}
		}
		seen := map[string]bool{}
		for _, id := range criterion.CommandIDs {
			if !commandIDs[id] || seen[id] {
				return &contractError{"E_EVIDENCE_REQUEST", base + "/command_ids", "criterion references an unknown or duplicate command"}
			}
			seen[id] = true
		}
	}
	return nil
}

func createAttempt(ctx *context, baseline []byte) error {
	parentRel := path.Dir(ctx.attemptRelative)
	if err := ensureSafeDirectories(ctx.root, parentRel); err != nil {
		return err
	}
	if err := os.Mkdir(ctx.attemptPath, 0o700); err != nil {
		if os.IsExist(err) {
			return &contractError{"E_EVIDENCE_SEALED", "/attempt_id", "evidence attempt already exists"}
		}
		return &contractError{"E_EVIDENCE_IO", "/attempt_id", "cannot create evidence attempt"}
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(filepath.Join(ctx.attemptPath, "request.json"))
			_ = os.Remove(filepath.Join(ctx.attemptPath, "baseline.json"))
			_ = os.Remove(ctx.attemptPath)
		}
	}()
	if err := publishNoReplace(filepath.Join(ctx.attemptPath, "request.json"), ctx.requestRaw); err != nil {
		return &contractError{"E_EVIDENCE_IO", "/request", "cannot seal request"}
	}
	if err := publishNoReplace(filepath.Join(ctx.attemptPath, "baseline.json"), baseline); err != nil {
		return &contractError{"E_EVIDENCE_IO", "/baseline", "cannot seal baseline"}
	}
	cleanup = false
	return nil
}

func snapshot(root, attemptRelative string) (Snapshot, error) {
	result := Snapshot{SchemaVersion: schemaVersion, Files: []SnapshotEntry{}}
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return &contractError{"E_EVIDENCE_IO", "/snapshot", "cannot traverse project tree"}
		}
		rel, err := filepath.Rel(root, current)
		if err != nil || strings.HasPrefix(rel, "..") {
			return &contractError{"E_EVIDENCE_PATH", "/snapshot", "project traversal escaped root"}
		}
		normalized := filepath.ToSlash(rel)
		if normalized == "." {
			return nil
		}
		if normalized == ".git" || strings.HasPrefix(normalized, ".git/") || normalized == attemptRelative || strings.HasPrefix(normalized, attemptRelative+"/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(current)
		if err != nil {
			return &contractError{"E_EVIDENCE_IO", "/snapshot", "cannot inspect project entry"}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(current)
			if err != nil {
				return &contractError{"E_EVIDENCE_PATH", "/snapshot", "cannot read symlink text"}
			}
			value := []byte(target)
			result.Files = append(result.Files, SnapshotEntry{Path: normalized, Type: "symlink", SHA256: digest(value), ByteLength: int64(len(value))})
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if link, err := linkLike(current, info); err != nil || link {
				return &contractError{"E_EVIDENCE_PATH", "/snapshot", "junction or reparse directory is not allowed"}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return &contractError{"E_EVIDENCE_PATH", "/snapshot", "special project entry is not allowed"}
		}
		value, err := os.ReadFile(current)
		if err != nil {
			return &contractError{"E_EVIDENCE_IO", "/snapshot", "cannot read project file"}
		}
		result.Files = append(result.Files, SnapshotEntry{Path: normalized, Type: "file", SHA256: digest(value), ByteLength: int64(len(value))})
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, nil
}

func prepareCommands(root string, commands []CommandRequest) ([]preparedCommand, error) {
	prepared := make([]preparedCommand, 0, len(commands))
	for i, command := range commands {
		cwdRel, _ := safeRelative(command.CWD, true)
		cwd := root
		if cwdRel != "." {
			cwd = filepath.Join(root, filepath.FromSlash(cwdRel))
		}
		info, err := os.Lstat(cwd)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, &contractError{"E_EVIDENCE_PATH", fmt.Sprintf("/commands/%d/cwd", i), "command cwd must be a safe directory"}
		}
		if link, err := linkLike(cwd, info); err != nil || link {
			return nil, &contractError{"E_EVIDENCE_PATH", fmt.Sprintf("/commands/%d/cwd", i), "command cwd cannot traverse a link-like directory"}
		}
		resolved := command.Executable
		if !filepath.IsAbs(resolved) && strings.ContainsAny(resolved, `/\`) {
			resolved = filepath.Join(cwd, resolved)
		} else if !filepath.IsAbs(resolved) {
			resolved, err = exec.LookPath(resolved)
			if err != nil {
				return nil, &contractError{"E_EVIDENCE_EXEC", fmt.Sprintf("/commands/%d/executable", i), "cannot resolve executable"}
			}
		}
		value, err := readRegular(resolved)
		if err != nil {
			return nil, &contractError{"E_EVIDENCE_EXEC", fmt.Sprintf("/commands/%d/executable", i), "executable must resolve to a regular file"}
		}
		prepared = append(prepared, preparedCommand{request: command, cwd: cwd, resolved: resolved, hash: digest(value)})
	}
	return prepared, nil
}

func executeCommand(command preparedCommand, clock func() time.Time) (commandResult, error) {
	started := clock().UTC()
	cmd := exec.Command(command.resolved, command.request.Args...)
	cmd.Dir = command.cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	ended := clock().UTC()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return commandResult{}, &contractError{"E_EVIDENCE_EXEC", "/commands", "prepared executable could not be started"}
		}
		exitCode = exitErr.ExitCode()
	}
	duration := ended.Sub(started).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	return commandResult{
		ID: command.request.ID, Executable: command.request.Executable, Args: command.request.Args,
		CWD: command.request.CWD, ExpectedExitCode: command.request.ExpectedExitCode, ExitCode: exitCode,
		StartedAt: started.Format(time.RFC3339Nano), EndedAt: ended.Format(time.RFC3339Nano),
		DurationMilliseconds: duration, ExecutableSHA256: command.hash,
		Stdout: summarize(stdout.Bytes()), Stderr: summarize(stderr.Bytes()),
	}, nil
}

func summarize(value []byte) streamSummary {
	result := streamSummary{SHA256: digest(value), ByteLength: len(value), ValidUTF8: utf8.Valid(value)}
	if result.ValidUTF8 {
		lines := bytes.Count(value, []byte{'\n'})
		if len(value) != 0 && value[len(value)-1] != '\n' {
			lines++
		}
		result.LineCount = &lines
	}
	return result
}

func diffSnapshots(before, after Snapshot) []changedFile {
	left, right := map[string]SnapshotEntry{}, map[string]SnapshotEntry{}
	for _, entry := range before.Files {
		left[entry.Path] = entry
	}
	for _, entry := range after.Files {
		right[entry.Path] = entry
	}
	paths := make([]string, 0, len(left)+len(right))
	seen := map[string]bool{}
	for name := range left {
		paths = append(paths, name)
		seen[name] = true
	}
	for name := range right {
		if !seen[name] {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)
	changes := []changedFile{}
	for _, name := range paths {
		old, oldOK := left[name]
		current, currentOK := right[name]
		if oldOK && currentOK && old.Type == current.Type && old.SHA256 == current.SHA256 && old.ByteLength == current.ByteLength {
			continue
		}
		item := changedFile{Path: name}
		switch {
		case !oldOK:
			item.Change = "added"
		case !currentOK:
			item.Change = "deleted"
		case old.Type != current.Type:
			item.Change = "type_changed"
		default:
			item.Change = "modified"
		}
		if oldOK {
			length := old.ByteLength
			item.BeforeType, item.BeforeSHA256, item.BeforeByteLength = old.Type, old.SHA256, &length
		}
		if currentOK {
			length := current.ByteLength
			item.AfterType, item.AfterSHA256, item.AfterByteLength = current.Type, current.SHA256, &length
		}
		changes = append(changes, item)
	}
	return changes
}

func outsideAllowed(changes []changedFile, patterns []string) []string {
	outside := []string{}
	for _, change := range changes {
		allowed := false
		for _, pattern := range patterns {
			if strings.HasSuffix(pattern, "/") {
				allowed = strings.HasPrefix(change.Path, pattern)
			} else if matched, _ := path.Match(pattern, change.Path); matched {
				allowed = true
			}
			if allowed {
				break
			}
		}
		if !allowed {
			outside = append(outside, change.Path)
		}
	}
	return outside
}

func acceptanceChecklist(request Request, results []commandResult) ([]acceptanceResult, bool) {
	byID := map[string]commandResult{}
	for _, result := range results {
		byID[result.ID] = result
	}
	all := true
	checklist := make([]acceptanceResult, 0, len(request.AcceptanceCriteria))
	for _, criterion := range request.AcceptanceCriteria {
		verified := true
		refs := make([]string, 0, len(criterion.CommandIDs))
		for _, id := range criterion.CommandIDs {
			result := byID[id]
			refs = append(refs, "command:"+id+":exit_code")
			if result.ExitCode != result.ExpectedExitCode {
				verified = false
			}
		}
		if !verified {
			all = false
		}
		checklist = append(checklist, acceptanceResult{ID: criterion.ID, Text: criterion.Text, CommandIDs: criterion.CommandIDs, Verified: verified, EvidenceRefs: refs})
	}
	return checklist, all
}

func safeRelative(value string, dotAllowed bool) (string, bool) {
	if value == "." && dotAllowed {
		return ".", true
	}
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) || strings.ContainsRune(value, 0) {
		return "", false
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value || strings.HasPrefix(value, "/") {
		return "", false
	}
	return clean, true
}

func safeAllowedPattern(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) || strings.Contains(value, "**") || strings.ContainsRune(value, 0) {
		return false
	}
	trimmed := strings.TrimSuffix(value, "/")
	if trimmed == "" {
		return false
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, "[]") {
			return false
		}
	}
	return true
}

func ensureSafeDirectories(root, relative string) error {
	current := root
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return &contractError{"E_EVIDENCE_IO", "/evidence_store", "cannot create evidence directory"}
			}
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return &contractError{"E_EVIDENCE_PATH", "/evidence_store", "evidence directory is unsafe"}
		}
		if link, err := linkLike(current, info); err != nil || link {
			return &contractError{"E_EVIDENCE_PATH", "/evidence_store", "evidence directory is link-like"}
		}
	}
	return nil
}

func linkLike(value string, info os.FileInfo) (bool, error) {
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return false, err
	}
	stat, err := os.Stat(value)
	if err != nil {
		return false, err
	}
	modeChanged := info.Mode().Type() != stat.Mode().Type()
	return info.Mode()&os.ModeSymlink != 0 || modeChanged || !samePath(value, resolved), nil
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func withinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func readRegularWithin(root, value string) ([]byte, error) {
	if !withinRoot(root, value) {
		return nil, os.ErrPermission
	}
	rel, err := filepath.Rel(root, value)
	if err != nil {
		return nil, err
	}
	current := root
	for _, component := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		if component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, os.ErrPermission
		}
		if link, err := linkLike(current, info); err != nil || link {
			return nil, os.ErrPermission
		}
	}
	return readRegular(value)
}

func readRegular(value string) ([]byte, error) {
	info, err := os.Lstat(value)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(value)
}

func publishNoReplace(final string, value []byte) error {
	temp := final + ".tmp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(temp)
		}
	}()
	if _, err := file.Write(value); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Link(temp, final); err != nil {
		return err
	}
	if err := os.Remove(temp); err != nil {
		return err
	}
	ok = true
	return nil
}

func decodeStrict(value []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}

func canonicalLine(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	raw, err = json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func WriteCanonical(writer io.Writer, value any) error {
	raw, err := canonicalLine(value)
	if err != nil {
		return err
	}
	_, err = writer.Write(raw)
	return err
}

func exclusions(attemptRelative string) []string {
	return []string{".git/", attemptRelative + "/"}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func diagnosticResult(err error) Result {
	var contract *contractError
	if errors.As(err, &contract) {
		return diag(contract.code, contract.pointer, contract.message)
	}
	return diag("E_EVIDENCE_IO", "/", "unexpected evidence I/O failure")
}

func diag(code, pointer, message string) Result {
	return Result{Diagnostics: []Diagnostic{{Code: code, Path: pointer, Message: message}}, ExitCode: 2}
}
