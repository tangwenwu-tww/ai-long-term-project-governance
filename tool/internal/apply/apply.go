// Package apply publishes CREATE operations from an explicitly approved plan.
package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"example.invalid/aigov/internal/plan"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var testFailpoint func(string) bool

func injected(stage string) bool { return testFailpoint != nil && testFailpoint(stage) }

// Output is the stable successful apply envelope.
type Output struct {
	ContractVersion string   `json:"contract_version"`
	InstanceID      string   `json:"instance_id"`
	PlanSHA256      string   `json:"plan_sha256"`
	Status          string   `json:"status"`
	Created         []string `json:"created"`
}

// Result is either a successful output or stable diagnostics.
type Result struct {
	Output      *Output
	Diagnostics []plan.Diagnostic
}

type transaction struct {
	root        string
	lockPath    string
	lockContent string
	lockOwned   bool
	lockCreated bool
	temps       []string
	finals      []published
	createdDirs []string
}

type published struct {
	path string
	hash string
}

// Execute recomputes, verifies, and publishes an approved CREATE-only plan.
func Execute(manifestPath, workingDir string, now time.Time, version, approved string) Result {
	if !shaPattern.MatchString(approved) {
		return failure("E_PLAN_APPROVAL_REQUIRED")
	}
	first := plan.Evaluate(manifestPath, workingDir, now, version)
	if len(first.Diagnostics) != 0 {
		return Result{Diagnostics: first.Diagnostics}
	}
	if first.Mode != "apply" {
		return failure("E_APPLY_MODE")
	}
	actualPlanHash := digest(first.StdoutBytes())
	if actualPlanHash != approved {
		return failure("E_PLAN_CHANGED")
	}
	if first.Conflict {
		return failure("E_PLAN_CONFLICT")
	}

	paths, errCode := internalPaths(first, approved)
	if errCode != "" {
		return failure(errCode)
	}
	if len(first.Prepared) == 0 {
		return success(first, approved, "NO_CHANGES", nil)
	}

	txn := &transaction{root: first.ProjectRoot, lockPath: paths.lock, lockContent: approved + "\n"}
	if code := ensureDirectory(first.ProjectRoot, filepath.Dir(paths.internal), txn); code != "" {
		return failWithRollback(code, txn)
	}
	internalCreated, code := prepareInternal(paths.internal, paths.lock, txn)
	if code != "" {
		return failWithRollback(code, txn)
	}
	if code := createLock(paths.lock, approved, txn); code != "" {
		return failWithRollback(code, txn)
	}

	second := plan.Evaluate(manifestPath, workingDir, now, version)
	if len(second.Diagnostics) != 0 || digest(second.StdoutBytes()) != approved {
		return failWithRollback("E_TARGET_CHANGED", txn)
	}

	prepared := append([]plan.PreparedFile(nil), second.Prepared...)
	sort.Slice(prepared, func(i, j int) bool { return prepared[i].Operation.Path < prepared[j].Operation.Path })
	for _, file := range prepared {
		finalPath := filepath.Join(second.ProjectRoot, filepath.FromSlash(file.Operation.Path))
		if code := ensureDirectory(second.ProjectRoot, filepath.Dir(finalPath), txn); code != "" {
			return failWithRollback(code, txn)
		}
		tempPath := paths.temps[file.Operation.LogicalID]
		if code := createTemp(tempPath, file.Content, txn); code != "" {
			return failWithRollback(code, txn)
		}
	}

	for _, file := range prepared {
		finalPath := filepath.Join(second.ProjectRoot, filepath.FromSlash(file.Operation.Path))
		tempPath := paths.temps[file.Operation.LogicalID]
		if _, err := os.Lstat(finalPath); err == nil {
			return failWithRollback("E_TARGET_CHANGED", txn)
		} else if !os.IsNotExist(err) {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		if err := os.Link(tempPath, finalPath); err != nil {
			if _, statErr := os.Lstat(finalPath); statErr == nil {
				return failWithRollback("E_TARGET_CHANGED", txn)
			}
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		expected := value(file.Operation.ExpectedSHA256)
		txn.finals = append(txn.finals, published{path: finalPath, hash: expected})
		if injected("publish") {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		if injected("final-hash") || digestFile(finalPath) != expected {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		if code := verifyFile(finalPath, expected, 0o644); code != "" {
			return failWithRollback(code, txn)
		}
		if injected("final-attribute") {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		tempInfo, tempErr := os.Stat(tempPath)
		finalInfo, finalErr := os.Stat(finalPath)
		if tempErr != nil || finalErr != nil || !os.SameFile(tempInfo, finalInfo) {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		if injected("final-verify") || (runtime.GOOS == "windows" && injected("final-windows-identity")) {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		if injected("temp-cleanup") {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		if err := os.Remove(tempPath); err != nil {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		removeString(&txn.temps, tempPath)
	}

	if injected("lock-cleanup") {
		return failWithRollback("E_APPLY_WRITE", txn)
	}
	if err := os.Remove(paths.lock); err != nil {
		return failWithRollback("E_APPLY_WRITE", txn)
	}
	txn.lockOwned = false
	if internalCreated {
		if injected("internal-cleanup") {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		if err := os.Remove(paths.internal); err != nil {
			return failWithRollback("E_APPLY_WRITE", txn)
		}
		removeString(&txn.createdDirs, paths.internal)
	}
	created := make([]string, len(prepared))
	for i, file := range prepared {
		created[i] = file.Operation.Path
	}
	return success(second, approved, "APPLIED", created)
}

type applyPaths struct {
	internal string
	lock     string
	temps    map[string]string
}

func internalPaths(result plan.Result, approved string) (applyPaths, string) {
	instancePath := ""
	allFinals := make([]string, 0, len(result.Lines))
	var operations struct {
		Operations []plan.Operation `json:"operations"`
	}
	if len(result.Lines) != 1 || json.Unmarshal([]byte(result.Lines[0]), &operations) != nil {
		return applyPaths{}, "E_APPLY_WRITE"
	}
	for _, operation := range operations.Operations {
		allFinals = append(allFinals, operation.Path)
		if operation.LogicalID == "instance_manifest" {
			instancePath = operation.Path
		}
	}
	if instancePath == "" {
		return applyPaths{}, "E_APPLY_WRITE"
	}
	internalRel := path.Join(path.Dir(instancePath), ".aigov-internal")
	lockRel := path.Join(internalRel, "apply.lock")
	tempsRel := make(map[string]string, len(result.Prepared))
	logicalHashes := make(map[string]bool)
	for _, file := range result.Prepared {
		logicalHash := digest([]byte(file.Operation.LogicalID))
		if logicalHashes[logicalHash] {
			return applyPaths{}, "E_INTERNAL_PATH_COLLISION"
		}
		logicalHashes[logicalHash] = true
		tempsRel[file.Operation.LogicalID] = path.Join(path.Dir(file.Operation.Path), ".aigov-tmp-"+approved+"-"+logicalHash)
	}
	reserved := []string{internalRel, lockRel}
	for _, temp := range tempsRel {
		reserved = append(reserved, temp)
	}
	for _, final := range allFinals {
		for _, item := range reserved {
			if collides(final, item) {
				return applyPaths{}, "E_INTERNAL_PATH_COLLISION"
			}
		}
	}
	temps := make([]string, 0, len(tempsRel))
	for _, item := range tempsRel {
		temps = append(temps, item)
	}
	for i, left := range temps {
		for _, right := range temps[i+1:] {
			if collides(left, right) {
				return applyPaths{}, "E_INTERNAL_PATH_COLLISION"
			}
		}
	}
	absTemps := make(map[string]string, len(tempsRel))
	for id, relative := range tempsRel {
		absTemps[id] = filepath.Join(result.ProjectRoot, filepath.FromSlash(relative))
	}
	return applyPaths{
		internal: filepath.Join(result.ProjectRoot, filepath.FromSlash(internalRel)),
		lock:     filepath.Join(result.ProjectRoot, filepath.FromSlash(lockRel)),
		temps:    absTemps,
	}, ""
}

func collides(left, right string) bool {
	left, right = asciiLower(path.Clean(left)), asciiLower(path.Clean(right))
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func prepareInternal(internal, lock string, txn *transaction) (bool, string) {
	info, err := os.Lstat(internal)
	if os.IsNotExist(err) {
		if err := os.Mkdir(internal, 0o755); err != nil {
			return false, "E_APPLY_WRITE"
		}
		txn.createdDirs = append(txn.createdDirs, internal)
		if injected("internal-mkdir") {
			return true, "E_APPLY_WRITE"
		}
		if injected("internal-chmod") || (runtime.GOOS != "windows" && os.Chmod(internal, 0o755) != nil) {
			return true, "E_APPLY_WRITE"
		}
		if runtime.GOOS != "windows" {
			createdInfo, statErr := os.Stat(internal)
			if statErr != nil || createdInfo.Mode().Perm() != 0o755 {
				return true, "E_APPLY_WRITE"
			}
		}
		if injected("internal-mode") {
			return true, "E_APPLY_WRITE"
		}
		return true, ""
	}
	if err != nil || !realDirectory(internal, info) {
		return false, "E_TARGET_CHANGED"
	}
	if _, err := os.Lstat(lock); err == nil {
		return false, "E_APPLY_LOCKED"
	} else if !os.IsNotExist(err) {
		return false, "E_APPLY_WRITE"
	}
	entries, err := os.ReadDir(internal)
	if err != nil {
		return false, "E_APPLY_WRITE"
	}
	if len(entries) != 0 {
		return false, "E_APPLY_TEMP_CONFLICT"
	}
	return false, ""
}

func createLock(lock, approved string, txn *transaction) string {
	file, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "E_APPLY_LOCKED"
		}
		return "E_APPLY_WRITE"
	}
	txn.lockOwned = true
	txn.lockCreated = true
	if injected("lock-create") {
		_ = file.Close()
		return "E_APPLY_WRITE"
	}
	if injected("lock-chmod") || (runtime.GOOS != "windows" && file.Chmod(0o600) != nil) {
		_ = file.Close()
		return "E_APPLY_WRITE"
	}
	content := approved + "\n"
	written, writeErr := file.WriteString(content)
	if injected("lock-write") {
		writeErr = errors.New("injected")
	}
	syncErr := file.Sync()
	if injected("lock-sync") {
		syncErr = errors.New("injected")
	}
	closeErr := file.Close()
	if injected("lock-close") {
		closeErr = errors.New("injected")
	}
	if writeErr != nil || written != len(content) || syncErr != nil || closeErr != nil {
		return "E_APPLY_WRITE"
	}
	if injected("lock-verify") || verifyFile(lock, digest([]byte(content)), 0o600) != "" {
		return "E_APPLY_WRITE"
	}
	return ""
}

func createTemp(temp string, content []byte, txn *transaction) string {
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "E_APPLY_TEMP_CONFLICT"
		}
		return "E_APPLY_WRITE"
	}
	txn.temps = append(txn.temps, temp)
	if injected("temp-create") {
		_ = file.Close()
		return "E_APPLY_WRITE"
	}
	if injected("temp-chmod") || (runtime.GOOS != "windows" && file.Chmod(0o644) != nil) {
		_ = file.Close()
		return "E_APPLY_WRITE"
	}
	written, writeErr := file.Write(content)
	if injected("temp-write") {
		writeErr = errors.New("injected")
	}
	syncErr := file.Sync()
	if injected("temp-sync") {
		syncErr = errors.New("injected")
	}
	closeErr := file.Close()
	if injected("temp-close") {
		closeErr = errors.New("injected")
	}
	if writeErr != nil || written != len(content) || syncErr != nil || closeErr != nil {
		return "E_APPLY_WRITE"
	}
	if injected("temp-hash") || digestFile(temp) != digest(content) {
		return "E_APPLY_WRITE"
	}
	if injected("temp-verify") || (runtime.GOOS == "windows" && injected("temp-windows-attribute")) || verifyFile(temp, digest(content), 0o644) != "" {
		return "E_APPLY_WRITE"
	}
	return ""
}

func verifyFile(file, expected string, mode os.FileMode) string {
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || digestFile(file) != expected {
		return "E_APPLY_WRITE"
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != mode {
		return "E_APPLY_WRITE"
	}
	return ""
}

func ensureDirectory(root, directory string, txn *transaction) string {
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "E_TARGET_CHANGED"
	}
	current := root
	if relative == "." {
		return ""
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return "E_APPLY_WRITE"
			}
			txn.createdDirs = append(txn.createdDirs, current)
			if injected("mkdir-chmod") || (runtime.GOOS != "windows" && os.Chmod(current, 0o755) != nil) {
				return "E_APPLY_WRITE"
			}
			if runtime.GOOS != "windows" {
				createdInfo, statErr := os.Stat(current)
				if statErr != nil || createdInfo.Mode().Perm() != 0o755 {
					return "E_APPLY_WRITE"
				}
			}
			if injected("mkdir-mode") {
				return "E_APPLY_WRITE"
			}
			if injected("mkdir") {
				return "E_APPLY_WRITE"
			}
			continue
		}
		if err != nil || !realDirectory(current, info) {
			return "E_TARGET_CHANGED"
		}
	}
	return ""
}

func realDirectory(path string, info os.FileInfo) bool {
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	cleanPath, cleanResolved := filepath.Clean(path), filepath.Clean(resolved)
	if filepath.Separator == '\\' {
		return strings.EqualFold(cleanPath, cleanResolved)
	}
	return cleanPath == cleanResolved
}

func failWithRollback(code string, txn *transaction) Result {
	diagnostics := []plan.Diagnostic{diagnostic(code)}
	if !rollback(txn) {
		retainRecoveryLock(txn)
		diagnostics = append(diagnostics, diagnostic("E_APPLY_ROLLBACK"))
	}
	return Result{Diagnostics: diagnostics}
}

func rollback(txn *transaction) bool {
	for i := len(txn.finals) - 1; i >= 0; i-- {
		item := txn.finals[i]
		info, err := os.Lstat(item.path)
		if injected("rollback-final") || injected("rollback-final-attribute") || err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if injected("rollback-final-hash") || digestFile(item.path) != item.hash || injected("rollback-final-remove") || os.Remove(item.path) != nil {
			return false
		}
	}
	for i := len(txn.temps) - 1; i >= 0; i-- {
		if injected("rollback-temp-remove") {
			return false
		}
		if err := os.Remove(txn.temps[i]); err != nil && !os.IsNotExist(err) {
			return false
		}
	}
	if txn.lockOwned {
		if injected("rollback-lock-remove") {
			return false
		}
		if err := os.Remove(txn.lockPath); err != nil && !os.IsNotExist(err) {
			return false
		}
		txn.lockOwned = false
	}
	sort.Slice(txn.createdDirs, func(i, j int) bool { return len(txn.createdDirs[i]) > len(txn.createdDirs[j]) })
	internalRemoved := false
	for _, directory := range txn.createdDirs {
		if injected("rollback-dir-remove") || (internalRemoved && injected("rollback-dir-remove-after-internal")) {
			return false
		}
		if err := os.Remove(directory); err != nil && !os.IsNotExist(err) {
			return false
		}
		if filepath.Clean(directory) == filepath.Clean(filepath.Dir(txn.lockPath)) {
			internalRemoved = true
		}
	}
	return true
}

func retainRecoveryLock(txn *transaction) {
	if !txn.lockCreated || txn.lockOwned {
		return
	}
	if !ensureRecoveryDirectory(txn.root, filepath.Dir(txn.lockPath)) {
		return
	}
	file, err := os.OpenFile(txn.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	if runtime.GOOS != "windows" {
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return
		}
	}
	written, writeErr := file.WriteString(txn.lockContent)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || written != len(txn.lockContent) || syncErr != nil || closeErr != nil {
		return
	}
	if verifyFile(txn.lockPath, digest([]byte(txn.lockContent)), 0o600) != "" {
		return
	}
	txn.lockOwned = true
}

func ensureRecoveryDirectory(root, directory string) bool {
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		created := false
		if os.IsNotExist(statErr) {
			if os.Mkdir(current, 0o755) != nil {
				return false
			}
			created = true
			if runtime.GOOS != "windows" && os.Chmod(current, 0o755) != nil {
				return false
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil || !realDirectory(current, info) {
			return false
		}
		if created && runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
			return false
		}
	}
	return true
}

func success(result plan.Result, approved, status string, created []string) Result {
	if created == nil {
		created = []string{}
	}
	return Result{Output: &Output{
		ContractVersion: "0.2-draft", InstanceID: result.InstanceID, PlanSHA256: approved,
		Status: status, Created: created,
	}}
}

func failure(code string) Result { return Result{Diagnostics: []plan.Diagnostic{diagnostic(code)}} }

func diagnostic(code string) plan.Diagnostic {
	paths := map[string]string{
		"E_APPLY_MODE": "/integration/mode", "E_PLAN_APPROVAL_REQUIRED": "/plan_sha256",
		"E_PLAN_CHANGED": "/plan_sha256", "E_PLAN_CONFLICT": "/operations",
	}
	messages := map[string]string{
		"E_APPLY_MODE":              "set integration.mode to apply",
		"E_PLAN_APPROVAL_REQUIRED":  "provide the complete approved plan stdout SHA-256",
		"E_PLAN_CHANGED":            "run plan again and approve its new stdout SHA-256",
		"E_PLAN_CONFLICT":           "resolve every conflict and approve a new plan",
		"E_INTERNAL_PATH_COLLISION": "move every target outside the reserved apply namespace",
		"E_APPLY_LOCKED":            "remove the apply lock only after manual recovery confirms target state",
		"E_TARGET_CHANGED":          "rerun plan after the target state changed",
		"E_APPLY_TEMP_CONFLICT":     "inspect and remove only the stale temp after manual recovery",
		"E_APPLY_WRITE":             "fix the target filesystem and rerun plan before applying",
		"E_APPLY_ROLLBACK":          "stop and recover the target manually before another apply",
	}
	pointer := paths[code]
	if pointer == "" {
		pointer = "/apply"
	}
	return plan.Diagnostic{Code: code, Path: pointer, Message: messages[code]}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func digestFile(file string) string {
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return digest(raw)
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}

func asciiLower(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}

func removeString(values *[]string, target string) {
	for i, value := range *values {
		if value == target {
			*values = append((*values)[:i], (*values)[i+1:]...)
			return
		}
	}
}
