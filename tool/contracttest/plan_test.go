package contracttest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

var (
	cliPath  string
	toolRoot string
)

type commandResult struct {
	code   int
	stdout string
	stderr string
}

type diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func TestMain(m *testing.M) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "cannot locate contract test source")
		os.Exit(1)
	}
	toolRoot = filepath.Dir(filepath.Dir(source))
	buildDir, err := os.MkdirTemp("", "aigov-contract-build-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	name := "aigov-contract"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cliPath = filepath.Join(buildDir, name)
	goName := "go"
	if runtime.GOOS == "windows" {
		goName += ".exe"
	}
	goExe := filepath.Join(runtime.GOROOT(), "bin", goName)
	cmd := exec.Command(goExe, "build", "-o", cliPath, "./cmd/aigov")
	cmd.Dir = toolRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build contract subject: %v\n%s", err, output)
		_ = os.RemoveAll(buildDir)
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(buildDir); err != nil {
		fmt.Fprintf(os.Stderr, "remove contract build directory: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func TestInstallManifestSchema(t *testing.T) {
	schemaPath := filepath.Join(toolRoot, "contracts", "v0", "install-manifest.schema.json")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	sum := sha256.Sum256(raw)
	if got, want := hex.EncodeToString(sum[:]), "490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1"; got != want {
		t.Fatalf("schema SHA-256 = %s, want canonical %s", got, want)
	}
	if got := schema["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("schema dialect = %v", got)
	}
	if got := schema["additionalProperties"]; got != false {
		t.Fatalf("root additionalProperties = %v, want false", got)
	}
	wantRequired := []string{"contract_version", "governance", "instance_id", "integration", "layout", "project"}
	gotRequired := stringSlice(t, schema["required"])
	sort.Strings(gotRequired)
	if fmt.Sprint(gotRequired) != fmt.Sprint(wantRequired) {
		t.Fatalf("root required = %v, want %v", gotRequired, wantRequired)
	}
}

func TestManifestAcceptedDomain(t *testing.T) {
	target := t.TempDir()
	for name, instanceID := range map[string]string{
		"minimum instance id": "inst-abc",
		"maximum instance id": "inst-" + strings.Repeat("a", 123),
	} {
		name, instanceID := name, instanceID
		t.Run(name, func(t *testing.T) {
			manifest := baseManifest(t, target)
			setPointer(t, manifest, "/instance_id", instanceID)
			requirePlanSuccess(t, runManifest(t, target, manifest, true))
		})
	}

	for phase := 0; phase <= 4; phase++ {
		phase := phase
		t.Run(fmt.Sprintf("phase %d", phase), func(t *testing.T) {
			manifest := baseManifest(t, target)
			setPointer(t, manifest, "/governance/phase", float64(phase))
			if phase >= 2 {
				setPointer(t, manifest, "/governance/independent_review_available", true)
				setPointer(t, manifest, "/governance/independent_review_source", validReviewSource())
			}
			requirePlanSuccess(t, runManifest(t, target, manifest, true))
		})
	}

	accepted := []struct {
		name, pointer string
		values        []string
	}{
		{"signal", "/governance/phase_basis/signals/0", []string{"product_idea_exists", "core_hypothesis_proven", "first_external_user", "code_lines>=20000", "module_count>5", "first_production_incident", "team_size>1", "parallel_agent_sessions>3", "max_module_reuse>=3", "compliance_requirement", "long_term_sla_module", "maintenance_freeze"}},
		{"ci", "/integration/ci", []string{"none", "github-actions", "gitlab-ci", "azure-pipelines", "other"}},
		{"mode", "/integration/mode", []string{"plan", "apply"}},
		{"profile", "/layout/profile", []string{"default-v0", "custom-v0"}},
	}
	for _, field := range accepted {
		field := field
		for _, value := range field.values {
			value := value
			t.Run("accepted "+field.name+" "+value, func(t *testing.T) {
				manifest := baseManifest(t, target)
				setPointer(t, manifest, field.pointer, value)
				requirePlanSuccess(t, runManifest(t, target, manifest, true))
			})
		}
	}

	for _, provider := range []string{"orchestrator", "ci", "manual", "other"} {
		provider := provider
		t.Run("accepted provider "+provider, func(t *testing.T) {
			manifest := baseManifest(t, target)
			setPointer(t, manifest, "/governance/independent_review_available", true)
			setPointer(t, manifest, "/governance/independent_review_source", validReviewSource())
			setPointer(t, manifest, "/governance/independent_review_source/provider", provider)
			requirePlanSuccess(t, runManifest(t, target, manifest, true))
		})
	}
	for _, assurance := range []string{"declarative", "verified"} {
		assurance := assurance
		t.Run("accepted assurance "+assurance, func(t *testing.T) {
			manifest := baseManifest(t, target)
			setPointer(t, manifest, "/governance/independent_review_available", true)
			setPointer(t, manifest, "/governance/independent_review_source", validReviewSource())
			setPointer(t, manifest, "/governance/independent_review_source/assurance_level", assurance)
			requirePlanSuccess(t, runManifest(t, target, manifest, true))
		})
	}

	knownResources := []string{"agent_entry", "design_spec", "project_decisions", "instance_manifest", "phase_snapshot", "maturity_fact_store", "maturity_snapshot", "policy_snapshot", "trust_anchor", "runtime_root", "schema_root", "task_store", "evidence_store", "review_store", "task_state_cache", "task_audit_store", "phase_audit_store"}
	for _, resource := range knownResources {
		resource := resource
		t.Run("accepted resource "+resource, func(t *testing.T) {
			manifest := baseManifest(t, target)
			setPointer(t, manifest, "/layout/profile", "custom-v0")
			setPointer(t, manifest, "/layout/resources", map[string]any{resource: "custom/" + resource})
			requirePlanSuccess(t, runManifest(t, target, manifest, true))
		})
	}
}

func TestPlanCanonicalFixtures(t *testing.T) {
	t.Run("valid phase zero", func(t *testing.T) {
		target := t.TempDir()
		got := runFixture(t, target, "valid-phase0.json", true)
		wantStdout := expectedPlanJSON(t, "blank")
		if got.code != 0 || got.stdout != wantStdout || got.stderr != "" {
			t.Fatalf("valid plan = code %d stdout %q stderr %q; want code 0 stdout %q stderr empty", got.code, got.stdout, got.stderr, wantStdout)
		}
	})

	t.Run("missing phase basis", func(t *testing.T) {
		got := runFixture(t, t.TempDir(), "missing-phase-basis.json", true)
		want := "{\"code\":\"E_REQUIRED\",\"path\":\"/governance/phase_basis\",\"message\":\"add required field /governance/phase_basis\"}\n"
		if got.code != 2 || got.stdout != "" || got.stderr != want {
			t.Fatalf("missing phase_basis = code %d stdout %q stderr %q; want code 2 stdout empty stderr %q", got.code, got.stdout, got.stderr, want)
		}
	})

	t.Run("unknown and unsafe are sorted", func(t *testing.T) {
		got := runFixture(t, t.TempDir(), "unknown-and-unsafe.json", true)
		want := "{\"code\":\"E_UNSAFE_PATH\",\"path\":\"/layout/resources/task_store\",\"message\":\"use a project-relative path without absolute prefixes or .. segments\"}\n" +
			"{\"code\":\"E_UNKNOWN_FIELD\",\"path\":\"/project/unknown\",\"message\":\"remove unknown field /project/unknown\"}\n"
		if got.code != 2 || got.stdout != "" || got.stderr != want {
			t.Fatalf("invalid manifest = code %d stdout %q stderr %q; want code 2 stdout empty stderr %q", got.code, got.stdout, got.stderr, want)
		}
	})
}

func TestPlanFeatureFlag(t *testing.T) {
	got := runFixture(t, t.TempDir(), "valid-phase0.json", false)
	want := "experimental plan is disabled; set AIGOV_ENABLE_EXPERIMENTAL_PLAN=1 to enable\n"
	if got.code != 2 || got.stdout != "" || got.stderr != want {
		t.Fatalf("disabled plan = code %d stdout %q stderr %q; want code 2 stdout empty stderr %q", got.code, got.stdout, got.stderr, want)
	}
}

func TestManifestContractVersionUpgrade(t *testing.T) {
	target := t.TempDir()
	manifest := baseManifest(t, target)
	setPointer(t, manifest, "/contract_version", "0.1-draft")
	got := runManifest(t, target, manifest, true)
	want := "{\"code\":\"E_CONTRACT_VERSION\",\"path\":\"/contract_version\",\"message\":\"set contract_version to 0.2-draft\"}\n"
	if got.code != 2 || got.stdout != "" || got.stderr != want {
		t.Fatalf("old contract version = code %d stdout %q stderr %q; want code 2 stdout empty stderr %q", got.code, got.stdout, got.stderr, want)
	}
}

func TestManifestRequiredAndTypeMatrix(t *testing.T) {
	target := t.TempDir()
	required := []string{
		"/contract_version",
		"/instance_id",
		"/project", "/project/name", "/project/root", "/project/technology", "/project/has_frontend", "/project/git_required",
		"/governance", "/governance/phase", "/governance/phase_basis", "/governance/phase_basis/assessed_at", "/governance/phase_basis/assessed_by", "/governance/phase_basis/signals", "/governance/core_hypothesis", "/governance/selection_finalized", "/governance/selection_record", "/governance/independent_review_available", "/governance/independent_review_source",
		"/integration", "/integration/ci", "/integration/mode",
		"/layout", "/layout/profile", "/layout/resources",
	}
	for _, pointer := range required {
		pointer := pointer
		t.Run("required "+strings.ReplaceAll(pointer, "/", "_"), func(t *testing.T) {
			manifest := baseManifest(t, target)
			deletePointer(t, manifest, pointer)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_REQUIRED", pointer)
		})
	}

	typeCases := []struct {
		name    string
		pointer string
		value   any
	}{
		{"root", "", []any{}},
		{"contract version", "/contract_version", true},
		{"instance id", "/instance_id", true},
		{"project", "/project", "bad"}, {"project name", "/project/name", 1.0}, {"project root", "/project/root", false},
		{"technology", "/project/technology", "Go"}, {"technology item", "/project/technology/0", true},
		{"has frontend", "/project/has_frontend", "false"}, {"git required", "/project/git_required", 0.0},
		{"governance", "/governance", []any{}}, {"phase decimal", "/governance/phase", 0.5},
		{"phase basis", "/governance/phase_basis", "bad"}, {"assessed at", "/governance/phase_basis/assessed_at", true},
		{"assessed by", "/governance/phase_basis/assessed_by", 1.0}, {"signals", "/governance/phase_basis/signals", "bad"},
		{"signal item", "/governance/phase_basis/signals/0", false}, {"core hypothesis", "/governance/core_hypothesis", nil},
		{"selection finalized", "/governance/selection_finalized", "false"}, {"selection record", "/governance/selection_record", 1.0},
		{"review available", "/governance/independent_review_available", "false"}, {"review source", "/governance/independent_review_source", 1.0},
		{"integration", "/integration", false}, {"ci", "/integration/ci", 1.0}, {"mode", "/integration/mode", false},
		{"layout", "/layout", "bad"}, {"profile", "/layout/profile", true}, {"resources", "/layout/resources", []any{}},
	}
	for _, tc := range typeCases {
		tc := tc
		t.Run("type "+tc.name, func(t *testing.T) {
			manifest := baseManifest(t, target)
			if tc.pointer == "" {
				requireOnlyDiagnostic(t, runRawManifest(t, target, []byte("[]"), true), "E_TYPE", "")
				return
			}
			setPointer(t, manifest, tc.pointer, tc.value)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_TYPE", tc.pointer)
		})
	}
}

func TestManifestConditionalRequiredMatrix(t *testing.T) {
	target := t.TempDir()
	selection := manifestWithSelection(t, target)
	for _, leaf := range []string{"/governance/selection_record/path", "/governance/selection_record/sha256", "/governance/selection_record/approved_by", "/governance/selection_record/approved_at"} {
		leaf := leaf
		t.Run("selection "+filepath.Base(leaf), func(t *testing.T) {
			manifest := cloneManifest(t, selection)
			deletePointer(t, manifest, leaf)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_REQUIRED", leaf)
		})
	}

	review := baseManifest(t, target)
	setPointer(t, review, "/governance/independent_review_available", true)
	setPointer(t, review, "/governance/independent_review_source", validReviewSource())
	for _, leaf := range []string{"/governance/independent_review_source/provider", "/governance/independent_review_source/assurance_level", "/governance/independent_review_source/evidence_ref"} {
		leaf := leaf
		t.Run("review "+filepath.Base(leaf), func(t *testing.T) {
			manifest := cloneManifest(t, review)
			deletePointer(t, manifest, leaf)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_REQUIRED", leaf)
		})
	}

	typeCases := []struct {
		name, pointer string
		base          map[string]any
	}{
		{"selection path", "/governance/selection_record/path", selection},
		{"selection sha256", "/governance/selection_record/sha256", selection},
		{"selection approved by", "/governance/selection_record/approved_by", selection},
		{"selection approved at", "/governance/selection_record/approved_at", selection},
		{"review provider", "/governance/independent_review_source/provider", review},
		{"review assurance", "/governance/independent_review_source/assurance_level", review},
		{"review evidence", "/governance/independent_review_source/evidence_ref", review},
	}
	for _, tc := range typeCases {
		tc := tc
		t.Run("type "+tc.name, func(t *testing.T) {
			manifest := cloneManifest(t, tc.base)
			setPointer(t, manifest, tc.pointer, true)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_TYPE", tc.pointer)
		})
	}
}

func TestManifestEnumUnknownAndValueMatrix(t *testing.T) {
	target := t.TempDir()
	enums := []struct {
		name, pointer string
		value         any
	}{
		{"contract version", "/contract_version", "0.1"},
		{"phase", "/governance/phase", 5.0},
		{"signal", "/governance/phase_basis/signals/0", "invented"},
		{"ci", "/integration/ci", "jenkins"}, {"mode", "/integration/mode", "doctor"}, {"profile", "/layout/profile", "future"},
	}
	reviewEnums := baseManifest(t, target)
	setPointer(t, reviewEnums, "/governance/independent_review_available", true)
	setPointer(t, reviewEnums, "/governance/independent_review_source", validReviewSource())
	for _, tc := range []struct{ pointer, value string }{
		{"/governance/independent_review_source/provider", "robot"},
		{"/governance/independent_review_source/assurance_level", "strong"},
	} {
		tc := tc
		t.Run("enum "+filepath.Base(tc.pointer), func(t *testing.T) {
			manifest := cloneManifest(t, reviewEnums)
			setPointer(t, manifest, tc.pointer, tc.value)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_ENUM", tc.pointer)
		})
	}
	for _, tc := range enums {
		tc := tc
		t.Run("enum "+tc.name, func(t *testing.T) {
			manifest := baseManifest(t, target)
			setPointer(t, manifest, tc.pointer, tc.value)
			code := "E_ENUM"
			if tc.pointer == "/contract_version" {
				code = "E_CONTRACT_VERSION"
			}
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), code, tc.pointer)
		})
	}

	unknownObjects := []string{"", "/project", "/governance", "/governance/phase_basis", "/integration", "/layout"}
	for _, parent := range unknownObjects {
		parent := parent
		t.Run("unknown "+strings.ReplaceAll(parent, "/", "_"), func(t *testing.T) {
			manifest := baseManifest(t, target)
			pointer := parent + "/unknown"
			setPointer(t, manifest, pointer, true)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_UNKNOWN_FIELD", pointer)
		})
	}

	t.Run("unknown selection record", func(t *testing.T) {
		selection := manifestWithSelection(t, target)
		setPointer(t, selection, "/governance/selection_record/unknown", true)
		requireOnlyDiagnostic(t, runManifest(t, target, selection, true), "E_UNKNOWN_FIELD", "/governance/selection_record/unknown")
	})
	t.Run("unknown review source", func(t *testing.T) {
		review := baseManifest(t, target)
		setPointer(t, review, "/governance/independent_review_available", true)
		setPointer(t, review, "/governance/independent_review_source", validReviewSource())
		setPointer(t, review, "/governance/independent_review_source/unknown", true)
		requireOnlyDiagnostic(t, runManifest(t, target, review, true), "E_UNKNOWN_FIELD", "/governance/independent_review_source/unknown")
	})

	values := []struct {
		name, pointer string
		value         any
	}{
		{"blank name", "/project/name", "   "}, {"long name", "/project/name", strings.Repeat("界", 129)},
		{"short instance id", "/instance_id", "inst-a"},
		{"bad instance prefix", "/instance_id", "instance-example-01"},
		{"unsafe instance character", "/instance_id", "inst-example/01"},
		{"long instance id", "/instance_id", "inst-" + strings.Repeat("a", 124)},
		{"blank technology", "/project/technology/0", " "},
		{"duplicate technology", "/project/technology", []any{"Go", " Go "}},
		{"empty signals", "/governance/phase_basis/signals", []any{}},
		{"future assessed at", "/governance/phase_basis/assessed_at", "9999-12-31T23:59:59Z"},
		{"invalid assessed at", "/governance/phase_basis/assessed_at", "yesterday"},
		{"invalid subject", "/governance/phase_basis/assessed_by", "project-owner"},
		{"blank hypothesis", "/governance/core_hypothesis", "\t"},
		{"placeholder hypothesis", "/governance/core_hypothesis", "[待定]"},
	}
	for _, tc := range values {
		tc := tc
		t.Run("value "+tc.name, func(t *testing.T) {
			manifest := baseManifest(t, target)
			setPointer(t, manifest, tc.pointer, tc.value)
			wantPointer := tc.pointer
			if tc.name == "duplicate technology" {
				wantPointer = "/project/technology/1"
			}
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_VALUE", wantPointer)
		})
	}

	selectionValues := []struct {
		name, pointer, value string
	}{
		{"selection sha format", "/governance/selection_record/sha256", "ABC"},
		{"selection subject", "/governance/selection_record/approved_by", "owner"},
		{"selection approved time", "/governance/selection_record/approved_at", "tomorrow"},
	}
	for _, tc := range selectionValues {
		tc := tc
		t.Run("value "+tc.name, func(t *testing.T) {
			manifest := manifestWithSelection(t, target)
			setPointer(t, manifest, tc.pointer, tc.value)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), "E_VALUE", tc.pointer)
		})
	}
	t.Run("value blank review evidence", func(t *testing.T) {
		reviewValue := cloneManifest(t, reviewEnums)
		setPointer(t, reviewValue, "/governance/independent_review_source/evidence_ref", " ")
		requireOnlyDiagnostic(t, runManifest(t, target, reviewValue, true), "E_VALUE", "/governance/independent_review_source/evidence_ref")
	})

	t.Run("duplicate signals accepted", func(t *testing.T) {
		duplicateSignals := baseManifest(t, target)
		setPointer(t, duplicateSignals, "/governance/phase_basis/signals", []any{"product_idea_exists", "product_idea_exists"})
		requirePlanSuccess(t, runManifest(t, target, duplicateSignals, true))
	})
}

func TestManifestCompoundEnvironmentAndResourceMatrix(t *testing.T) {
	target := t.TempDir()

	compound := []struct {
		name, code, pointer string
		mutate              func(map[string]any)
	}{
		{"selection false object", "E_INCONSISTENT", "/governance/selection_record", func(m map[string]any) { setPointer(t, m, "/governance/selection_record", map[string]any{}) }},
		{"selection true null", "E_INCONSISTENT", "/governance/selection_record", func(m map[string]any) { setPointer(t, m, "/governance/selection_finalized", true) }},
		{"review false object", "E_INCONSISTENT", "/governance/independent_review_source", func(m map[string]any) { setPointer(t, m, "/governance/independent_review_source", validReviewSource()) }},
		{"review true null", "E_INCONSISTENT", "/governance/independent_review_source", func(m map[string]any) { setPointer(t, m, "/governance/independent_review_available", true) }},
		{"phase two no review", "E_REVIEW_UNAVAILABLE", "/governance/independent_review_available", func(m map[string]any) { setPointer(t, m, "/governance/phase", 2.0) }},
	}
	for _, tc := range compound {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			manifest := baseManifest(t, target)
			tc.mutate(manifest)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), tc.code, tc.pointer)
		})
	}

	t.Run("root not found", func(t *testing.T) {
		notFound := baseManifest(t, filepath.Join(target, "missing"))
		requireOnlyDiagnostic(t, runManifest(t, target, notFound, true), "E_ROOT_NOT_FOUND", "/project/root")
	})
	t.Run("root not found suppresses git", func(t *testing.T) {
		notFound := baseManifest(t, filepath.Join(target, "missing-git"))
		setPointer(t, notFound, "/project/git_required", true)
		requireOnlyDiagnostic(t, runManifest(t, target, notFound, true), "E_ROOT_NOT_FOUND", "/project/root")
	})
	t.Run("root not found suppresses selection environment", func(t *testing.T) {
		notFound := baseManifest(t, filepath.Join(target, "missing-selection"))
		setPointer(t, notFound, "/governance/selection_finalized", true)
		setPointer(t, notFound, "/governance/selection_record", map[string]any{
			"path": "docs/selection-report.md", "sha256": strings.Repeat("0", 64),
			"approved_by": "human:project-owner", "approved_at": "2026-11-24T10:30:00+08:00",
		})
		requireOnlyDiagnostic(t, runManifest(t, target, notFound, true), "E_ROOT_NOT_FOUND", "/project/root")
	})
	t.Run("git required", func(t *testing.T) {
		notGit := baseManifest(t, target)
		setPointer(t, notGit, "/project/git_required", true)
		requireOnlyDiagnostic(t, runManifest(t, target, notGit, true), "E_GIT_REQUIRED", "/project/root")
	})

	resourceCases := []struct {
		name, code, pointer string
		resources           map[string]any
	}{
		{"unknown", "E_UNKNOWN_RESOURCE", "/layout/resources/invented", map[string]any{"invented": "../escape"}},
		{"unknown non-string suppressed", "E_UNKNOWN_RESOURCE", "/layout/resources/invented", map[string]any{"invented": true}},
		{"known non-string", "E_TYPE", "/layout/resources/task_store", map[string]any{"task_store": true}},
		{"blank", "E_VALUE", "/layout/resources/task_store", map[string]any{"task_store": "  "}},
		{"root absolute", "E_UNSAFE_PATH", "/layout/resources/task_store", map[string]any{"task_store": "/escape"}},
		{"absolute", "E_UNSAFE_PATH", "/layout/resources/task_store", map[string]any{"task_store": "C:/escape"}},
		{"drive relative", "E_UNSAFE_PATH", "/layout/resources/task_store", map[string]any{"task_store": "C:escape"}},
		{"unc", "E_UNSAFE_PATH", "/layout/resources/task_store", map[string]any{"task_store": "\\\\server\\share"}},
		{"parent", "E_UNSAFE_PATH", "/layout/resources/task_store", map[string]any{"task_store": "a\\..\\escape"}},
		{"default collision", "E_RESOURCE_COLLISION", "/layout/resources/agent_entry", map[string]any{"agent_entry": "DESIGN.md"}},
		{"explicit collision", "E_RESOURCE_COLLISION", "/layout/resources/design_spec", map[string]any{"agent_entry": "custom/shared", "design_spec": "custom/shared"}},
		{"dot collision", "E_RESOURCE_COLLISION", "/layout/resources/design_spec", map[string]any{"agent_entry": "custom/a/b", "design_spec": "custom/a/./b"}},
		{"separator collision", "E_RESOURCE_COLLISION", "/layout/resources/design_spec", map[string]any{"agent_entry": "custom/a\\b", "design_spec": "custom/a/b"}},
	}
	for _, tc := range resourceCases {
		tc := tc
		t.Run("resource "+tc.name, func(t *testing.T) {
			manifest := baseManifest(t, target)
			setPointer(t, manifest, "/layout/profile", "custom-v0")
			setPointer(t, manifest, "/layout/resources", tc.resources)
			requireOnlyDiagnostic(t, runManifest(t, target, manifest, true), tc.code, tc.pointer)
		})
	}
	t.Run("resource case collision", func(t *testing.T) {
		manifest := baseManifest(t, target)
		setPointer(t, manifest, "/layout/profile", "custom-v0")
		setPointer(t, manifest, "/layout/resources", map[string]any{"agent_entry": "custom/A/b", "design_spec": "custom/a/b"})
		got := runManifest(t, target, manifest, true)
		if runtime.GOOS == "windows" {
			requireOnlyDiagnostic(t, got, "E_RESOURCE_COLLISION", "/layout/resources/design_spec")
		} else {
			requirePlanSuccess(t, got)
		}
	})
}

func TestSelectionRecordFileHashAndContainment(t *testing.T) {
	target := t.TempDir()
	manifest := manifestWithSelection(t, target)

	t.Run("valid selection", func(t *testing.T) {
		requirePlanSuccess(t, runManifest(t, target, cloneManifest(t, manifest), true))
	})
	t.Run("missing file", func(t *testing.T) {
		missing := cloneManifest(t, manifest)
		setPointer(t, missing, "/governance/selection_record/path", "docs/missing.md")
		requireOnlyDiagnostic(t, runManifest(t, target, missing, true), "E_FILE_NOT_FOUND", "/governance/selection_record/path")
	})
	t.Run("hash mismatch", func(t *testing.T) {
		mismatch := cloneManifest(t, manifest)
		setPointer(t, mismatch, "/governance/selection_record/sha256", strings.Repeat("0", 64))
		requireOnlyDiagnostic(t, runManifest(t, target, mismatch, true), "E_HASH_MISMATCH", "/governance/selection_record/sha256")
	})
	t.Run("unsafe lexical path", func(t *testing.T) {
		unsafe := cloneManifest(t, manifest)
		setPointer(t, unsafe, "/governance/selection_record/path", "a\\..\\outside.md")
		requireOnlyDiagnostic(t, runManifest(t, target, unsafe, true), "E_UNSAFE_PATH", "/governance/selection_record/path")
	})
	for name, path := range map[string]string{
		"root absolute":  "/outside.md",
		"drive absolute": "C:/outside.md",
		"drive relative": "C:outside.md",
		"unc":            "\\\\server\\share\\outside.md",
	} {
		name, path := name, path
		t.Run("unsafe "+name, func(t *testing.T) {
			unsafe := cloneManifest(t, manifest)
			setPointer(t, unsafe, "/governance/selection_record/path", path)
			requireOnlyDiagnostic(t, runManifest(t, target, unsafe, true), "E_UNSAFE_PATH", "/governance/selection_record/path")
		})
	}
	t.Run("blank path", func(t *testing.T) {
		blank := cloneManifest(t, manifest)
		setPointer(t, blank, "/governance/selection_record/path", "  ")
		requireOnlyDiagnostic(t, runManifest(t, target, blank, true), "E_VALUE", "/governance/selection_record/path")
	})
	t.Run("outside symlink", func(t *testing.T) {
		outside := t.TempDir()
		outsideFile := filepath.Join(outside, "selection.md")
		if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(target, "outside-link")
		if runtime.GOOS == "windows" {
			if output, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
				t.Skipf("host cannot create required junction fixture: %v: %s", err, output)
			}
		} else if err := os.Symlink(outside, link); err != nil {
			t.Skipf("host cannot create required symlink fixture: %v", err)
		}
		symlink := cloneManifest(t, manifest)
		setPointer(t, symlink, "/governance/selection_record/path", "outside-link/selection.md")
		requireOnlyDiagnostic(t, runManifest(t, target, symlink, true), "E_UNSAFE_PATH", "/governance/selection_record/path")
	})
}

func TestJSONFailClosedAndDeterministic(t *testing.T) {
	target := t.TempDir()
	for name, raw := range map[string]string{
		"malformed":      `{`,
		"trailing token": `{} {}`,
		"duplicate key":  `{"contract_version":"0.2-draft","contract_version":"0.2-draft"}`,
	} {
		name, raw := name, raw
		t.Run(name, func(t *testing.T) {
			requireOnlyDiagnostic(t, runRawManifest(t, target, []byte(raw), true), "E_JSON", "")
		})
	}

	t.Run("repeat bytes", func(t *testing.T) {
		manifest := baseManifest(t, target)
		setPointer(t, manifest, "/governance/phase_basis/assessed_at", "9999-12-31T23:59:59Z")
		first := runManifest(t, target, manifest, true)
		second := runManifest(t, target, manifest, true)
		if first != second {
			t.Fatalf("repeat run differs\nfirst:  %#v\nsecond: %#v", first, second)
		}
	})
}

const canonicalInstance = `{"schema_version":"0.1-draft","instance_id":"inst-example-01","install_contract_version":"0.2-draft","methodology_version":"0.1.0-dev","assurance_level":"declarative","resources":{"agent_entry":"AGENTS.md","design_spec":"DESIGN.md","evidence_store":"governance/evidence","instance_manifest":".ai-governance/instance.json","maturity_fact_store":"governance/maturity/facts","maturity_snapshot":".ai-governance/maturity.json","phase_audit_store":"governance/audit/phase","phase_snapshot":".ai-governance/phase.json","policy_snapshot":".ai-governance/policy.json","project_decisions":"docs/decisions.md","review_store":"governance/reviews","runtime_root":".ai-governance/runtime","schema_root":".ai-governance/schemas","task_audit_store":"governance/audit/tasks","task_state_cache":"governance/state","task_store":"governance/tasks","trust_anchor":".ai-governance/trust.json"},"managed_files":{"schema_root/install-manifest.schema.json":"490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1","schema_root/instance.schema.json":"6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af"}}
`

const canonicalAgent = `---
type: reference
purpose: 目标项目的 AI Agent 本地启动入口与项目事实摘要
audience: 参与目标项目的 AI Agent 与人类协作者
status: active
---

# Target project AI entry

Read this file first, then the instance manifest at:

    ".ai-governance/instance.json"

## Project facts

    {"core_hypothesis":"Target users choose X because Y","instance_id":"inst-example-01","name":"example-service","phase":0,"technology":["Go"]}

## Working boundary

Follow the recorded phase and repository-local task constraints. Do not treat generated tool files as authority over confirmed project facts.
`

type expectedOperation struct {
	Action         string  `json:"action"`
	LogicalID      string  `json:"logical_id"`
	Path           string  `json:"path"`
	Ownership      string  `json:"ownership"`
	ExpectedSHA256 *string `json:"expected_sha256"`
	ActualSHA256   *string `json:"actual_sha256"`
	Reason         string  `json:"reason"`
}

type expectedPlan struct {
	ContractVersion string              `json:"contract_version"`
	InstanceID      string              `json:"instance_id"`
	Operations      []expectedOperation `json:"operations"`
}

func stringPointer(value string) *string { return &value }

func expectedPlanJSON(t *testing.T, state string) string {
	t.Helper()
	const instanceHash = "d2334d08bc2f9fec68d66b5f3af59e213d0f518e940057b194de2f25432bee3b"
	const installHash = "490a29358da5d1ad678effe58635fab5af88cf9693bf0f246d7b30a88abb4ed1"
	const instanceSchemaHash = "6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af"
	const agentHash = "08958b0e158adf4141fa7219e67146e439f476f240699306bbbfe3d18435ad0a"
	const conflictHash = "ca3d163bab055381827226140568f3bef7eaac187cebd76878e0b63e9e442356"

	managed := func(logicalID, path, expected string) expectedOperation {
		action, reason := "CREATE", "target_missing"
		var actual *string
		if state == "noop" || state == "conflict" {
			action, reason, actual = "NOOP", "managed_content_matches", stringPointer(expected)
		}
		if state == "conflict" && logicalID == "instance_manifest" {
			action, reason, actual = "CONFLICT", "managed_content_differs", stringPointer(conflictHash)
		}
		return expectedOperation{action, logicalID, path, "tool", stringPointer(expected), actual, reason}
	}
	agentAction, agentReason := "CREATE", "target_missing"
	var agentExpected, agentActual *string = stringPointer(agentHash), nil
	if state == "noop" || state == "conflict" {
		agentAction, agentReason = "NOOP", "project_fact_preserved"
		agentExpected, agentActual = nil, stringPointer(agentHash)
	}
	plan := expectedPlan{ContractVersion: "0.2-draft", InstanceID: "inst-example-01", Operations: []expectedOperation{
		managed("instance_manifest", ".ai-governance/instance.json", instanceHash),
		managed("schema_root/install-manifest.schema.json", ".ai-governance/schemas/install-manifest.schema.json", installHash),
		managed("schema_root/instance.schema.json", ".ai-governance/schemas/instance.schema.json", instanceSchemaHash),
		{agentAction, "agent_entry", "AGENTS.md", "project", agentExpected, agentActual, agentReason},
		{"SKIP", "design_spec", "DESIGN.md", "project", nil, nil, "no_frontend"},
		{"SKIP", "project_decisions", "docs/decisions.md", "project", nil, nil, "lifecycle_not_reached"},
	}}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw) + "\n"
}

func TestExecutablePlanGoldenStates(t *testing.T) {
	target := t.TempDir()
	if got := runManifest(t, target, baseManifest(t, target), true); got.code != 0 || got.stderr != "" || got.stdout != expectedPlanJSON(t, "blank") {
		t.Fatalf("blank plan = %#v\nwant stdout %q", got, expectedPlanJSON(t, "blank"))
	}

	writeCanonicalTargets(t, target, false)
	if got := runManifest(t, target, baseManifest(t, target), true); got.code != 0 || got.stderr != "" || got.stdout != expectedPlanJSON(t, "noop") {
		t.Fatalf("noop plan = %#v\nwant stdout %q", got, expectedPlanJSON(t, "noop"))
	}

	if err := os.WriteFile(filepath.Join(target, ".ai-governance", "instance.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := runManifest(t, target, baseManifest(t, target), true); got.code != 2 || got.stderr != "" || got.stdout != expectedPlanJSON(t, "conflict") {
		t.Fatalf("conflict plan = %#v\nwant stdout %q", got, expectedPlanJSON(t, "conflict"))
	}
}

func TestInstanceSchemaAndCanonicalBytes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(toolRoot, "contracts", "v0", "instance.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got, want := hex.EncodeToString(sum[:]), "6dfe34eda589b1c8a948669fef164c60ff7272b5ed65eeb05f47af669bf559af"; got != want {
		t.Fatalf("instance schema SHA-256 = %s, want %s", got, want)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("instance schema permits unknown root fields")
	}
	for name, content := range map[string]string{"instance": canonicalInstance, "agent": canonicalAgent} {
		sum := sha256.Sum256([]byte(content))
		want := map[string]string{"instance": "d2334d08bc2f9fec68d66b5f3af59e213d0f518e940057b194de2f25432bee3b", "agent": "08958b0e158adf4141fa7219e67146e439f476f240699306bbbfe3d18435ad0a"}[name]
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("%s SHA-256 = %s, want %s", name, got, want)
		}
	}
}

func TestTargetTypeFailsClosed(t *testing.T) {
	target := t.TempDir()
	if err := os.Mkdir(filepath.Join(target, "AGENTS.md"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := runManifest(t, target, baseManifest(t, target), true)
	requireOnlyDiagnostic(t, got, "E_TARGET_TYPE", "/targets/agent_entry")
	if !strings.Contains(got.stderr, "replace target or parent with the required regular file or directory type") {
		t.Fatalf("target type diagnostic = %q", got.stderr)
	}
}

func writeCanonicalTargets(t *testing.T, target string, conflict bool) {
	t.Helper()
	schemaDir := filepath.Join(target, ".ai-governance", "schemas")
	if err := os.MkdirAll(schemaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	instance := canonicalInstance
	if conflict {
		instance = "{}\n"
	}
	files := map[string][]byte{
		filepath.Join(target, "AGENTS.md"):                       []byte(canonicalAgent),
		filepath.Join(target, ".ai-governance", "instance.json"): []byte(instance),
	}
	for _, name := range []string{"install-manifest.schema.json", "instance.schema.json"} {
		raw, err := os.ReadFile(filepath.Join(toolRoot, "contracts", "v0", name))
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Join(schemaDir, name)] = raw
	}
	for path, content := range files {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func runFixture(t *testing.T, target, name string, enabled bool) commandResult {
	t.Helper()
	path := filepath.Join(toolRoot, "contracttest", "testdata", "plan", name)
	return runCommand(t, target, path, enabled)
}

func runManifest(t *testing.T, target string, manifest map[string]any, enabled bool) commandResult {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return runRawManifest(t, target, raw, enabled)
}

func runRawManifest(t *testing.T, target string, raw []byte, enabled bool) commandResult {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return runCommand(t, target, path, enabled)
}

func runCommand(t *testing.T, target, manifestPath string, enabled bool) commandResult {
	t.Helper()
	stabilizeDirectoryTimes(t, target)
	before := snapshotTree(t, target)
	cmd := exec.Command(cliPath, "plan", "--manifest", manifestPath)
	cmd.Dir = target
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "AIGOV_ENABLE_EXPERIMENTAL_PLAN=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	if enabled {
		cmd.Env = append(cmd.Env, "AIGOV_ENABLE_EXPERIMENTAL_PLAN=1")
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run plan: %v", err)
		}
		code = exitErr.ExitCode()
	}
	after := snapshotTree(t, target)
	if fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("plan changed target tree\nbefore: %v\nafter:  %v", before, after)
	}
	return commandResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func requireOnlyDiagnostic(t *testing.T, got commandResult, code, pointer string) {
	t.Helper()
	if got.code != 2 || got.stdout != "" {
		t.Fatalf("result = code %d stdout %q stderr %q; want code 2 and empty stdout", got.code, got.stdout, got.stderr)
	}
	if !strings.HasSuffix(got.stderr, "\n") || strings.HasSuffix(got.stderr, "\n\n") {
		t.Fatalf("stderr must end in exactly one newline, got %q", got.stderr)
	}
	lines := strings.Split(strings.TrimSuffix(got.stderr, "\n"), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("stderr must contain exactly one JSON diagnostic, got %q", got.stderr)
	}
	line := lines[0]
	codeKey := strings.Index(line, `{"code":`)
	pathKey := strings.Index(line, `,"path":`)
	messageKey := strings.Index(line, `,"message":`)
	if codeKey != 0 || pathKey <= codeKey || messageKey <= pathKey || !strings.HasSuffix(line, "}") {
		t.Fatalf("diagnostic keys must be ordered code,path,message, got %q", line)
	}
	var diag diagnostic
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&diag); err != nil {
		t.Fatalf("stderr is not JSONL: %v; stderr %q", err, got.stderr)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("diagnostic line contains trailing JSON content: %q", line)
	}
	canonical, err := json.Marshal(diag)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != line {
		t.Fatalf("diagnostic is not the canonical three-key object: got %q want %q", line, canonical)
	}
	if diag.Code != code || diag.Path != pointer {
		t.Fatalf("diagnostic = %#v, want code %s path %q", diag, code, pointer)
	}
	if strings.TrimSpace(diag.Message) == "" {
		t.Fatal("diagnostic message is empty")
	}
	wantDirection := map[string]string{
		"E_JSON": "provide ", "E_REQUIRED": "add ", "E_UNKNOWN_FIELD": "remove ", "E_CONTRACT_VERSION": "set ",
		"E_TYPE": "set ", "E_ENUM": "set ", "E_VALUE": "replace ", "E_INCONSISTENT": "make ",
		"E_ROOT_NOT_FOUND": "set ", "E_FILE_NOT_FOUND": "set ", "E_HASH_MISMATCH": "set ",
		"E_UNKNOWN_RESOURCE": "use ", "E_UNSAFE_PATH": "use ", "E_RESOURCE_COLLISION": "map ",
		"E_GIT_REQUIRED": "initialize ", "E_REVIEW_UNAVAILABLE": "provide ",
	}[code]
	if wantDirection == "" || !strings.HasPrefix(diag.Message, wantDirection) {
		t.Fatalf("diagnostic message %q lacks repair direction %q", diag.Message, wantDirection)
	}
}

func requirePlanSuccess(t *testing.T, got commandResult) {
	t.Helper()
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("valid plan = code %d stdout %q stderr %q; want code 0 and empty stderr", got.code, got.stdout, got.stderr)
	}
}

func baseManifest(t *testing.T, target string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(toolRoot, "contracttest", "testdata", "plan", "valid-phase0.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	setPointer(t, manifest, "/project/root", target)
	return manifest
}

func manifestWithSelection(t *testing.T, target string) map[string]any {
	t.Helper()
	docs := filepath.Join(target, "docs")
	if err := os.MkdirAll(docs, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("approved selection\n")
	if err := os.WriteFile(filepath.Join(docs, "selection-report.md"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	manifest := baseManifest(t, target)
	setPointer(t, manifest, "/governance/selection_finalized", true)
	setPointer(t, manifest, "/governance/selection_record", map[string]any{
		"path": "docs/selection-report.md", "sha256": hex.EncodeToString(sum[:]),
		"approved_by": "human:project-owner", "approved_at": "2026-11-24T10:30:00+08:00",
	})
	return manifest
}

func validReviewSource() map[string]any {
	return map[string]any{"provider": "orchestrator", "assurance_level": "verified", "evidence_ref": "run:reviewer-1"}
}

func cloneManifest(t *testing.T, input map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	return output
}

func setPointer(t *testing.T, root map[string]any, pointer string, value any) {
	t.Helper()
	parts := pointerParts(pointer)
	if len(parts) == 0 {
		t.Fatal("setPointer cannot replace root")
	}
	var current any = root
	for _, part := range parts[:len(parts)-1] {
		switch node := current.(type) {
		case map[string]any:
			current = node[part]
		case []any:
			var index int
			if _, err := fmt.Sscanf(part, "%d", &index); err != nil {
				t.Fatalf("bad array index %q", part)
			}
			current = node[index]
		default:
			t.Fatalf("cannot traverse %q in %s", part, pointer)
		}
	}
	last := parts[len(parts)-1]
	switch node := current.(type) {
	case map[string]any:
		node[last] = value
	case []any:
		var index int
		if _, err := fmt.Sscanf(last, "%d", &index); err != nil {
			t.Fatalf("bad array index %q", last)
		}
		node[index] = value
	default:
		t.Fatalf("cannot set %s", pointer)
	}
}

func deletePointer(t *testing.T, root map[string]any, pointer string) {
	t.Helper()
	parts := pointerParts(pointer)
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			t.Fatalf("cannot traverse %s", pointer)
		}
		current = next
	}
	delete(current, parts[len(parts)-1])
}

func pointerParts(pointer string) []string {
	if pointer == "" {
		return nil
	}
	raw := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i := range raw {
		raw[i] = strings.ReplaceAll(strings.ReplaceAll(raw[i], "~1", "/"), "~0", "~")
	}
	return raw
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("value is %T, want array", value)
	}
	result := make([]string, len(values))
	for i, value := range values {
		result[i], ok = value.(string)
		if !ok {
			t.Fatalf("array item is %T, want string", value)
		}
	}
	return result
}

func snapshotTree(t *testing.T, root string) []string {
	t.Helper()
	var snapshot []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s|%v|%s|%d", filepath.ToSlash(rel), entry.IsDir(), info.Mode(), info.ModTime().UnixNano())
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			line += "|link=" + target
		} else if !entry.IsDir() {
			content, err := os.ReadFile(path)
			if err != nil {
				target, linkErr := os.Readlink(path)
				if linkErr != nil {
					return err
				}
				line += "|link=" + target
				snapshot = append(snapshot, line)
				return nil
			}
			sum := sha256.Sum256(content)
			line += "|" + hex.EncodeToString(sum[:])
		}
		snapshot = append(snapshot, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(snapshot)
	return snapshot
}

func stabilizeDirectoryTimes(t *testing.T, root string) {
	t.Helper()
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	fixed := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, directory := range directories {
		if err := os.Chtimes(directory, fixed, fixed); err != nil {
			t.Fatalf("stabilize directory time %s: %v", directory, err)
		}
	}
}
