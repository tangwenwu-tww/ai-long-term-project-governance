package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const contractVersion = "0.1-draft"

var (
	subjectPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}:[A-Za-z0-9][A-Za-z0-9._@+/-]{0,127}$`)
	shaPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	drivePattern   = regexp.MustCompile(`^[A-Za-z]:`)
)

var signalValues = []string{
	"product_idea_exists",
	"core_hypothesis_proven",
	"first_external_user",
	"code_lines>=20000",
	"module_count>5",
	"first_production_incident",
	"team_size>1",
	"parallel_agent_sessions>3",
	"max_module_reuse>=3",
	"compliance_requirement",
	"long_term_sla_module",
	"maintenance_freeze",
}

var defaultResources = map[string]string{
	"agent_entry":         "AGENTS.md",
	"design_spec":         "DESIGN.md",
	"project_decisions":   "docs/decisions.md",
	"instance_manifest":   ".ai-governance/instance.json",
	"phase_snapshot":      ".ai-governance/phase.json",
	"maturity_fact_store": "governance/maturity/facts",
	"maturity_snapshot":   ".ai-governance/maturity.json",
	"policy_snapshot":     ".ai-governance/policy.json",
	"trust_anchor":        ".ai-governance/trust.json",
	"runtime_root":        ".ai-governance/runtime",
	"schema_root":         ".ai-governance/schemas",
	"task_store":          "governance/tasks",
	"evidence_store":      "governance/evidence",
	"review_store":        "governance/reviews",
	"task_state_cache":    "governance/state",
	"task_audit_store":    "governance/audit/tasks",
	"phase_audit_store":   "governance/audit/phase",
}

// Diagnostic is one stable JSONL validation result.
type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Result is the complete read-only plan result.
type Result struct {
	Lines       []string
	Diagnostics []Diagnostic
}

type validator struct {
	root        map[string]any
	diagnostics []Diagnostic
	invalid     map[string]bool
	workingDir  string
	now         time.Time
}

// Evaluate reads and validates one manifest without writing to the target project.
func Evaluate(manifestPath, workingDir string, now time.Time) Result {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return Result{Diagnostics: []Diagnostic{diagnostic("E_JSON", "", "")}}
	}
	value, err := decodeJSON(raw)
	if err != nil {
		return Result{Diagnostics: []Diagnostic{diagnostic("E_JSON", "", "")}}
	}
	root, ok := value.(map[string]any)
	if !ok {
		return Result{Diagnostics: []Diagnostic{diagnostic("E_TYPE", "", "object")}}
	}

	v := &validator{root: root, invalid: make(map[string]bool), workingDir: workingDir, now: now}
	v.validateStructure()
	v.validateFieldsAndEnvironment()
	sort.Slice(v.diagnostics, func(i, j int) bool {
		if v.diagnostics[i].Path != v.diagnostics[j].Path {
			return v.diagnostics[i].Path < v.diagnostics[j].Path
		}
		return v.diagnostics[i].Code < v.diagnostics[j].Code
	})
	if len(v.diagnostics) != 0 {
		return Result{Diagnostics: v.diagnostics}
	}

	project := root["project"].(map[string]any)
	governance := root["governance"].(map[string]any)
	var lines []string
	if !governance["selection_finalized"].(bool) {
		lines = append(lines, "SKIP project_decisions: lifecycle_not_reached")
	}
	if !project["has_frontend"].(bool) {
		lines = append(lines, "SKIP design_spec: no_frontend")
	}
	return Result{Lines: lines}
}

func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON token")
		}
		return nil, err
	}
	return value, nil
}

func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate object key")
			}
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return nil, fmt.Errorf("invalid object close")
		}
		return object, nil
	case '[':
		var array []any
		for decoder.More() {
			value, err := decodeValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return nil, fmt.Errorf("invalid array close")
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected delimiter")
	}
}

func (v *validator) validateStructure() {
	v.unknown(v.root, "", keys("contract_version", "project", "governance", "integration", "layout"))
	v.required(v.root, "", "contract_version", "project", "governance", "integration", "layout")
	v.expect(v.root, "contract_version", "/contract_version", "string", false)

	project, ok := v.object(v.root, "project", "/project")
	if ok {
		v.unknown(project, "/project", keys("name", "root", "technology", "has_frontend", "git_required"))
		v.required(project, "/project", "name", "root", "technology", "has_frontend", "git_required")
		v.expect(project, "name", "/project/name", "string", false)
		v.expect(project, "root", "/project/root", "string", false)
		v.array(project, "technology", "/project/technology", "string")
		v.expect(project, "has_frontend", "/project/has_frontend", "boolean", false)
		v.expect(project, "git_required", "/project/git_required", "boolean", false)
	}

	governance, ok := v.object(v.root, "governance", "/governance")
	if ok {
		v.validateGovernanceStructure(governance)
	}

	integration, ok := v.object(v.root, "integration", "/integration")
	if ok {
		v.unknown(integration, "/integration", keys("ci", "mode"))
		v.required(integration, "/integration", "ci", "mode")
		v.expect(integration, "ci", "/integration/ci", "string", false)
		v.expect(integration, "mode", "/integration/mode", "string", false)
	}

	layout, ok := v.object(v.root, "layout", "/layout")
	if ok {
		v.unknown(layout, "/layout", keys("profile", "resources"))
		v.required(layout, "/layout", "profile", "resources")
		v.expect(layout, "profile", "/layout/profile", "string", false)
		resources, resourcesOK := v.object(layout, "resources", "/layout/resources")
		if resourcesOK {
			for resource, value := range resources {
				pointer := "/layout/resources/" + escapePointer(resource)
				if _, known := defaultResources[resource]; !known {
					v.add("E_UNKNOWN_RESOURCE", pointer, "")
					v.invalid[pointer] = true
					continue
				}
				if _, ok := value.(string); !ok {
					v.add("E_TYPE", pointer, "string")
					v.invalid[pointer] = true
				}
			}
		}
	}
}

func (v *validator) validateGovernanceStructure(governance map[string]any) {
	allowed := keys("phase", "phase_basis", "core_hypothesis", "selection_finalized", "selection_record", "independent_review_available", "independent_review_source")
	v.unknown(governance, "/governance", allowed)
	v.required(governance, "/governance", "phase", "phase_basis", "core_hypothesis", "selection_finalized", "selection_record", "independent_review_available", "independent_review_source")
	v.expect(governance, "phase", "/governance/phase", "integer", false)
	v.expect(governance, "core_hypothesis", "/governance/core_hypothesis", "string", false)
	v.expect(governance, "selection_finalized", "/governance/selection_finalized", "boolean", false)
	v.expect(governance, "independent_review_available", "/governance/independent_review_available", "boolean", false)

	phaseBasis, ok := v.object(governance, "phase_basis", "/governance/phase_basis")
	if ok {
		v.unknown(phaseBasis, "/governance/phase_basis", keys("assessed_at", "assessed_by", "signals"))
		v.required(phaseBasis, "/governance/phase_basis", "assessed_at", "assessed_by", "signals")
		v.expect(phaseBasis, "assessed_at", "/governance/phase_basis/assessed_at", "string", false)
		v.expect(phaseBasis, "assessed_by", "/governance/phase_basis/assessed_by", "string", false)
		v.array(phaseBasis, "signals", "/governance/phase_basis/signals", "string")
	}

	selectionControl, controlOK := governance["selection_finalized"].(bool)
	selectionRaw, selectionPresent := governance["selection_record"]
	if selectionPresent && selectionRaw != nil {
		selection, objectOK := selectionRaw.(map[string]any)
		if !objectOK {
			v.add("E_TYPE", "/governance/selection_record", "object or null")
			v.invalid["/governance/selection_record"] = true
		} else if controlOK && selectionControl {
			v.validateSelectionStructure(selection)
		}
	}

	reviewControl, reviewControlOK := governance["independent_review_available"].(bool)
	reviewRaw, reviewPresent := governance["independent_review_source"]
	if reviewPresent && reviewRaw != nil {
		review, objectOK := reviewRaw.(map[string]any)
		if !objectOK {
			v.add("E_TYPE", "/governance/independent_review_source", "object or null")
			v.invalid["/governance/independent_review_source"] = true
		} else if reviewControlOK && reviewControl {
			v.validateReviewStructure(review)
		}
	}
}

func (v *validator) validateSelectionStructure(selection map[string]any) {
	base := "/governance/selection_record"
	v.unknown(selection, base, keys("path", "sha256", "approved_by", "approved_at"))
	v.required(selection, base, "path", "sha256", "approved_by", "approved_at")
	v.expect(selection, "path", base+"/path", "string", false)
	v.expect(selection, "sha256", base+"/sha256", "string", false)
	v.expect(selection, "approved_by", base+"/approved_by", "string", false)
	v.expect(selection, "approved_at", base+"/approved_at", "string", false)
}

func (v *validator) validateReviewStructure(review map[string]any) {
	base := "/governance/independent_review_source"
	v.unknown(review, base, keys("provider", "assurance_level", "evidence_ref"))
	v.required(review, base, "provider", "assurance_level", "evidence_ref")
	v.expect(review, "provider", base+"/provider", "string", false)
	v.expect(review, "assurance_level", base+"/assurance_level", "string", false)
	v.expect(review, "evidence_ref", base+"/evidence_ref", "string", false)
}

func (v *validator) validateFieldsAndEnvironment() {
	if value, ok := v.stringAt("/contract_version"); ok && value != contractVersion {
		v.add("E_CONTRACT_VERSION", "/contract_version", "")
	}

	project, projectOK := mapAt(v.root, "project")
	governance, governanceOK := mapAt(v.root, "governance")
	integration, integrationOK := mapAt(v.root, "integration")
	layout, layoutOK := mapAt(v.root, "layout")

	if projectOK {
		v.validateProjectFields(project)
	}
	if governanceOK {
		v.validateGovernanceFields(governance)
	}
	if integrationOK {
		v.enum(integration, "ci", "/integration/ci", []string{"none", "github-actions", "gitlab-ci", "azure-pipelines", "other"})
		v.enum(integration, "mode", "/integration/mode", []string{"plan", "apply"})
	}
	if layoutOK {
		v.enum(layout, "profile", "/layout/profile", []string{"default-v0", "custom-v0"})
		v.validateResources(layout)
	}
	if projectOK && governanceOK {
		v.validateEnvironment(project, governance)
	}
}

func (v *validator) validateProjectFields(project map[string]any) {
	if name, ok := v.stringAt("/project/name"); ok {
		trimmed := strings.TrimSpace(name)
		count := utf8.RuneCountInString(trimmed)
		if count < 1 || count > 128 {
			v.add("E_VALUE", "/project/name", "")
			v.invalid["/project/name"] = true
		}
	}
	if root, ok := v.stringAt("/project/root"); ok && strings.TrimSpace(root) == "" {
		v.add("E_VALUE", "/project/root", "")
		v.invalid["/project/root"] = true
	}
	technology, ok := project["technology"].([]any)
	if ok && v.valid("/project/technology") {
		seen := make(map[string]bool)
		for index, raw := range technology {
			pointer := fmt.Sprintf("/project/technology/%d", index)
			value, isString := raw.(string)
			if !isString || !v.valid(pointer) {
				continue
			}
			trimmed := strings.TrimSpace(value)
			if trimmed == "" || seen[trimmed] {
				v.add("E_VALUE", pointer, "")
				v.invalid[pointer] = true
				continue
			}
			seen[trimmed] = true
		}
	}
}

func (v *validator) validateGovernanceFields(governance map[string]any) {
	if phase, ok := v.integerAt("/governance/phase"); ok && (phase < 0 || phase > 4) {
		v.add("E_ENUM", "/governance/phase", "0, 1, 2, 3, 4")
		v.invalid["/governance/phase"] = true
	}
	if hypothesis, ok := v.stringAt("/governance/core_hypothesis"); ok {
		if strings.TrimSpace(hypothesis) == "" || strings.Contains(hypothesis, "[待定]") {
			v.add("E_VALUE", "/governance/core_hypothesis", "")
			v.invalid["/governance/core_hypothesis"] = true
		}
	}
	if basis, ok := mapAt(governance, "phase_basis"); ok && v.valid("/governance/phase_basis") {
		v.validatePhaseBasisFields(basis)
	}

	selection, selectionOK := governance["selection_finalized"].(bool)
	record, recordPresent := governance["selection_record"]
	if selectionOK && v.valid("/governance/selection_finalized") && recordPresent && v.valid("/governance/selection_record") {
		if (!selection && record != nil) || (selection && record == nil) {
			v.add("E_INCONSISTENT", "/governance/selection_record", "")
			v.invalid["/governance/selection_record"] = true
		} else if selection {
			if object, ok := record.(map[string]any); ok {
				v.validateSelectionFields(object)
			}
		}
	}

	review, reviewOK := governance["independent_review_available"].(bool)
	source, sourcePresent := governance["independent_review_source"]
	if reviewOK && v.valid("/governance/independent_review_available") && sourcePresent && v.valid("/governance/independent_review_source") {
		if (!review && source != nil) || (review && source == nil) {
			v.add("E_INCONSISTENT", "/governance/independent_review_source", "")
			v.invalid["/governance/independent_review_source"] = true
		} else if review {
			if object, ok := source.(map[string]any); ok {
				v.validateReviewFields(object)
			}
		}
	}
	phase, phaseOK := v.integerAt("/governance/phase")
	if phaseOK && phase >= 2 && reviewOK && !review && v.valid("/governance/independent_review_available") {
		v.add("E_REVIEW_UNAVAILABLE", "/governance/independent_review_available", "")
		v.invalid["/governance/independent_review_available"] = true
	}
}

func (v *validator) validatePhaseBasisFields(basis map[string]any) {
	if assessedAt, ok := v.stringAt("/governance/phase_basis/assessed_at"); ok {
		parsed, err := time.Parse(time.RFC3339, assessedAt)
		if err != nil || parsed.After(v.now.UTC().Add(300*time.Second)) {
			v.add("E_VALUE", "/governance/phase_basis/assessed_at", "")
			v.invalid["/governance/phase_basis/assessed_at"] = true
		}
	}
	if assessedBy, ok := v.stringAt("/governance/phase_basis/assessed_by"); ok && !subjectPattern.MatchString(assessedBy) {
		v.add("E_VALUE", "/governance/phase_basis/assessed_by", "")
		v.invalid["/governance/phase_basis/assessed_by"] = true
	}
	signals, ok := basis["signals"].([]any)
	if !ok || !v.valid("/governance/phase_basis/signals") {
		return
	}
	if len(signals) == 0 {
		v.add("E_VALUE", "/governance/phase_basis/signals", "")
		v.invalid["/governance/phase_basis/signals"] = true
		return
	}
	for index, raw := range signals {
		pointer := fmt.Sprintf("/governance/phase_basis/signals/%d", index)
		value, isString := raw.(string)
		if !isString || !v.valid(pointer) {
			continue
		}
		if !contains(signalValues, value) {
			v.add("E_ENUM", pointer, strings.Join(signalValues, ", "))
			v.invalid[pointer] = true
		}
	}
}

func (v *validator) validateSelectionFields(selection map[string]any) {
	base := "/governance/selection_record"
	if value, ok := v.stringAt(base + "/path"); ok {
		if strings.TrimSpace(value) == "" {
			v.add("E_VALUE", base+"/path", "")
			v.invalid[base+"/path"] = true
		} else if _, safe := safeRelativePath(value); !safe {
			v.add("E_UNSAFE_PATH", base+"/path", "")
			v.invalid[base+"/path"] = true
		}
	}
	if value, ok := v.stringAt(base + "/sha256"); ok && !shaPattern.MatchString(value) {
		v.add("E_VALUE", base+"/sha256", "")
		v.invalid[base+"/sha256"] = true
	}
	if value, ok := v.stringAt(base + "/approved_by"); ok && !subjectPattern.MatchString(value) {
		v.add("E_VALUE", base+"/approved_by", "")
		v.invalid[base+"/approved_by"] = true
	}
	if value, ok := v.stringAt(base + "/approved_at"); ok {
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			v.add("E_VALUE", base+"/approved_at", "")
			v.invalid[base+"/approved_at"] = true
		}
	}
}

func (v *validator) validateReviewFields(review map[string]any) {
	base := "/governance/independent_review_source"
	v.enum(review, "provider", base+"/provider", []string{"orchestrator", "ci", "manual", "other"})
	v.enum(review, "assurance_level", base+"/assurance_level", []string{"declarative", "verified"})
	if value, ok := v.stringAt(base + "/evidence_ref"); ok && strings.TrimSpace(value) == "" {
		v.add("E_VALUE", base+"/evidence_ref", "")
		v.invalid[base+"/evidence_ref"] = true
	}
}

func (v *validator) validateResources(layout map[string]any) {
	resources, ok := mapAt(layout, "resources")
	if !ok || !v.valid("/layout/resources") {
		return
	}
	effective := make(map[string]string, len(defaultResources))
	explicit := make(map[string]bool)
	for resource, value := range defaultResources {
		effective[resource] = value
	}
	for resource, raw := range resources {
		pointer := "/layout/resources/" + escapePointer(resource)
		if _, known := defaultResources[resource]; !known || !v.valid(pointer) {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			continue
		}
		if strings.TrimSpace(value) == "" {
			v.add("E_VALUE", pointer, "")
			v.invalid[pointer] = true
			continue
		}
		normalized, safe := safeRelativePath(value)
		if !safe {
			v.add("E_UNSAFE_PATH", pointer, "")
			v.invalid[pointer] = true
			continue
		}
		effective[resource] = normalized
		explicit[resource] = true
	}

	groups := make(map[string][]string)
	for resource, resourcePath := range effective {
		key := resourcePath
		if runtime.GOOS == "windows" {
			key = asciiLower(key)
		}
		groups[key] = append(groups[key], resource)
	}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		sort.Strings(group)
		hasDefault := false
		for _, resource := range group {
			if !explicit[resource] {
				hasDefault = true
				break
			}
		}
		for index, resource := range group {
			if !explicit[resource] || (!hasDefault && index == 0) {
				continue
			}
			pointer := "/layout/resources/" + escapePointer(resource)
			v.add("E_RESOURCE_COLLISION", pointer, "")
			v.invalid[pointer] = true
		}
	}
}

func (v *validator) validateEnvironment(project, governance map[string]any) {
	if !v.valid("/project/root") {
		return
	}
	rootValue, ok := project["root"].(string)
	if !ok {
		return
	}
	rootPath := rootValue
	if !filepath.IsAbs(rootPath) {
		rootPath = filepath.Join(v.workingDir, rootPath)
	}
	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		v.add("E_ROOT_NOT_FOUND", "/project/root", "")
		v.invalid["/project/root"] = true
		return
	}
	info, err := os.Stat(rootPath)
	if err != nil || !info.IsDir() {
		v.add("E_ROOT_NOT_FOUND", "/project/root", "")
		v.invalid["/project/root"] = true
		return
	}
	if required, ok := project["git_required"].(bool); ok && required && v.valid("/project/git_required") {
		if _, err := os.Stat(filepath.Join(rootPath, ".git")); err != nil {
			v.add("E_GIT_REQUIRED", "/project/root", "")
		}
	}

	selection, selectionOK := governance["selection_finalized"].(bool)
	if !selectionOK || !selection || !v.valid("/governance/selection_record") {
		return
	}
	record, ok := governance["selection_record"].(map[string]any)
	if !ok {
		return
	}
	pathPointer := "/governance/selection_record/path"
	hashPointer := "/governance/selection_record/sha256"
	if !v.valid(pathPointer) || !v.valid(hashPointer) {
		return
	}
	relative, pathOK := record["path"].(string)
	expectedHash, hashOK := record["sha256"].(string)
	if !pathOK || !hashOK {
		return
	}
	normalized, safe := safeRelativePath(relative)
	if !safe {
		return
	}
	realRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		v.add("E_ROOT_NOT_FOUND", "/project/root", "")
		return
	}
	targetPath := filepath.Join(rootPath, filepath.FromSlash(normalized))
	targetInfo, err := os.Stat(targetPath)
	if err != nil || !targetInfo.Mode().IsRegular() {
		v.add("E_FILE_NOT_FOUND", pathPointer, "")
		return
	}
	realTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil || !within(realRoot, realTarget) {
		v.add("E_UNSAFE_PATH", pathPointer, "")
		return
	}
	content, err := os.ReadFile(realTarget)
	if err != nil {
		v.add("E_FILE_NOT_FOUND", pathPointer, "")
		return
	}
	sum := sha256.Sum256(content)
	if hex.EncodeToString(sum[:]) != expectedHash {
		v.add("E_HASH_MISMATCH", hashPointer, "")
	}
}

func (v *validator) unknown(object map[string]any, base string, allowed map[string]bool) {
	for name := range object {
		if !allowed[name] {
			pointer := base + "/" + escapePointer(name)
			v.add("E_UNKNOWN_FIELD", pointer, "")
			v.invalid[pointer] = true
		}
	}
}

func (v *validator) required(object map[string]any, base string, names ...string) {
	for _, name := range names {
		if _, exists := object[name]; !exists {
			pointer := base + "/" + escapePointer(name)
			v.add("E_REQUIRED", pointer, "")
			v.invalid[pointer] = true
		}
	}
}

func (v *validator) object(parent map[string]any, name, pointer string) (map[string]any, bool) {
	raw, exists := parent[name]
	if !exists {
		return nil, false
	}
	object, ok := raw.(map[string]any)
	if !ok {
		v.add("E_TYPE", pointer, "object")
		v.invalid[pointer] = true
		return nil, false
	}
	return object, true
}

func (v *validator) expect(parent map[string]any, name, pointer, kind string, nullable bool) {
	raw, exists := parent[name]
	if !exists || (nullable && raw == nil) {
		return
	}
	valid := false
	switch kind {
	case "string":
		_, valid = raw.(string)
	case "boolean":
		_, valid = raw.(bool)
	case "integer":
		number, ok := raw.(json.Number)
		if ok {
			_, err := strconv.ParseInt(number.String(), 10, 64)
			valid = err == nil
		}
	}
	if !valid {
		v.add("E_TYPE", pointer, kind)
		v.invalid[pointer] = true
	}
}

func (v *validator) array(parent map[string]any, name, pointer, itemKind string) {
	raw, exists := parent[name]
	if !exists {
		return
	}
	values, ok := raw.([]any)
	if !ok {
		v.add("E_TYPE", pointer, "array")
		v.invalid[pointer] = true
		return
	}
	for index, value := range values {
		itemPointer := fmt.Sprintf("%s/%d", pointer, index)
		if itemKind == "string" {
			if _, ok := value.(string); !ok {
				v.add("E_TYPE", itemPointer, "string")
				v.invalid[itemPointer] = true
			}
		}
	}
}

func (v *validator) enum(parent map[string]any, name, pointer string, allowed []string) {
	if !v.valid(pointer) {
		return
	}
	value, ok := parent[name].(string)
	if !ok {
		return
	}
	if !contains(allowed, value) {
		v.add("E_ENUM", pointer, strings.Join(allowed, ", "))
		v.invalid[pointer] = true
	}
}

func (v *validator) stringAt(pointer string) (string, bool) {
	if !v.valid(pointer) {
		return "", false
	}
	value, ok := at(v.root, pointer).(string)
	return value, ok
}

func (v *validator) integerAt(pointer string) (int64, bool) {
	if !v.valid(pointer) {
		return 0, false
	}
	number, ok := at(v.root, pointer).(json.Number)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseInt(number.String(), 10, 64)
	return value, err == nil
}

func (v *validator) valid(pointer string) bool {
	for current := pointer; ; {
		if v.invalid[current] {
			return false
		}
		if current == "" {
			break
		}
		index := strings.LastIndex(current, "/")
		if index < 0 {
			break
		}
		current = current[:index]
	}
	return true
}

func (v *validator) add(code, pointer, detail string) {
	v.diagnostics = append(v.diagnostics, diagnostic(code, pointer, detail))
}

func diagnostic(code, pointer, detail string) Diagnostic {
	message := ""
	switch code {
	case "E_JSON":
		message = "provide one JSON object without duplicate keys or trailing tokens"
	case "E_REQUIRED":
		message = "add required field " + pointer
	case "E_UNKNOWN_FIELD":
		message = "remove unknown field " + pointer
	case "E_CONTRACT_VERSION":
		message = "set contract_version to 0.1-draft"
	case "E_TYPE":
		message = "set " + pointer + " to JSON type " + detail
	case "E_ENUM":
		message = "set " + pointer + " to one of: " + detail
	case "E_VALUE":
		message = "replace " + pointer + " with a value satisfying its documented constraints"
	case "E_INCONSISTENT":
		message = "make " + pointer + " consistent with its controlling field"
	case "E_ROOT_NOT_FOUND":
		message = "set project.root to an existing directory"
	case "E_FILE_NOT_FOUND":
		message = "set selection_record.path to an existing project file"
	case "E_HASH_MISMATCH":
		message = "set selection_record.sha256 to the selected file SHA-256"
	case "E_UNKNOWN_RESOURCE":
		message = "use a known logical resource ID at " + pointer
	case "E_UNSAFE_PATH":
		message = "use a project-relative path without absolute prefixes or .. segments"
	case "E_RESOURCE_COLLISION":
		message = "map " + pointer + " to a distinct project-relative path"
	case "E_GIT_REQUIRED":
		message = "initialize Git at project.root or set git_required to false"
	case "E_REVIEW_UNAVAILABLE":
		message = "provide an independent review source for phase 2 or later"
	}
	return Diagnostic{Code: code, Path: pointer, Message: message}
}

func safeRelativePath(value string) (string, bool) {
	replaced := strings.ReplaceAll(value, `\`, "/")
	if strings.HasPrefix(replaced, "/") || drivePattern.MatchString(replaced) {
		return "", false
	}
	for _, segment := range strings.Split(replaced, "/") {
		if segment == ".." {
			return "", false
		}
	}
	return path.Clean(replaced), true
}

func within(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func asciiLower(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}

func keys(names ...string) map[string]bool {
	result := make(map[string]bool, len(names))
	for _, name := range names {
		result[name] = true
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func mapAt(parent map[string]any, name string) (map[string]any, bool) {
	value, ok := parent[name].(map[string]any)
	return value, ok
}

func at(root map[string]any, pointer string) any {
	if pointer == "" {
		return root
	}
	var current any = root
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch value := current.(type) {
		case map[string]any:
			current = value[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) {
				return nil
			}
			current = value[index]
		default:
			return nil
		}
	}
	return current
}
