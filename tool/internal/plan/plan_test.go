package plan

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsAmbiguousInput(t *testing.T) {
	for name, input := range map[string]string{
		"duplicate key":  `{"a":1,"a":2}`,
		"trailing value": `{} {}`,
		"malformed":      `{`,
	} {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			if _, err := decodeJSON([]byte(input)); err == nil {
				t.Fatalf("decodeJSON(%q) succeeded", input)
			}
		})
	}
}

func TestSafeRelativePath(t *testing.T) {
	tests := []struct {
		input      string
		want       string
		wantSafety bool
	}{
		{"a/b", "a/b", true},
		{"a/./b", "a/b", true},
		{`a\b`, "a/b", true},
		{"../escape", "", false},
		{`a\..\escape`, "", false},
		{"/escape", "", false},
		{"C:escape", "", false},
		{`\\server\share`, "", false},
	}
	for _, test := range tests {
		got, safe := safeRelativePath(test.input)
		if got != test.want || safe != test.wantSafety {
			t.Errorf("safeRelativePath(%q) = %q, %v; want %q, %v", test.input, got, safe, test.want, test.wantSafety)
		}
	}
}

func TestDiagnosticMessages(t *testing.T) {
	got := diagnostic("E_REQUIRED", "/governance/phase_basis", "")
	if got.Code != "E_REQUIRED" || got.Path != "/governance/phase_basis" || got.Message != "add required field /governance/phase_basis" {
		t.Fatalf("diagnostic = %#v", got)
	}
}

func TestInstanceIDPattern(t *testing.T) {
	for _, value := range []string{"inst-abc", "inst-example-01", "inst-01HZX9ABC_DEF.02"} {
		if !instancePattern.MatchString(value) {
			t.Errorf("valid instance ID %q was rejected", value)
		}
	}
	for _, value := range []string{"inst-a", "instance-example", "inst-example/01", " inst-example-01"} {
		if instancePattern.MatchString(value) {
			t.Errorf("invalid instance ID %q was accepted", value)
		}
	}
}

func TestValidateInstanceBytesNegativeMatrix(t *testing.T) {
	resources := make(map[string]string, len(defaultResources))
	for key, value := range defaultResources {
		resources[key] = value
	}
	base := instanceManifest{
		SchemaVersion: "0.1-draft", InstanceID: "inst-example-01", InstallContractVersion: "0.3-draft",
		MethodologyVersion: "0.1.0-dev", AssuranceLevel: "declarative", Resources: resources,
		ManagedFiles: map[string]string{
			"schema_root/install-manifest.schema.json": strings.Repeat("a", 64),
			"schema_root/instance.schema.json":         strings.Repeat("b", 64),
		},
	}
	valid, err := marshalJSONLine(base)
	if err != nil || validateInstanceBytes(valid, resources, "0.1.0-dev") != nil {
		t.Fatalf("valid instance rejected: marshal=%v validate=%v", err, validateInstanceBytes(valid, resources, "0.1.0-dev"))
	}
	var object map[string]any
	if err := json.Unmarshal(valid, &object); err != nil {
		t.Fatal(err)
	}
	type mutation struct {
		name   string
		mutate func(map[string]any)
	}
	var mutations []mutation
	for _, key := range []string{"schema_version", "instance_id", "install_contract_version", "methodology_version", "assurance_level", "resources", "managed_files"} {
		key := key
		mutations = append(mutations, mutation{"missing root " + key, func(v map[string]any) { delete(v, key) }})
	}
	mutations = append(mutations,
		mutation{"unknown root", func(v map[string]any) { v["unknown"] = true }},
		mutation{"wrong schema version", func(v map[string]any) { v["schema_version"] = "9" }},
		mutation{"wrong install version", func(v map[string]any) { v["install_contract_version"] = "9" }},
		mutation{"wrong methodology version", func(v map[string]any) { v["methodology_version"] = "9" }},
		mutation{"wrong assurance", func(v map[string]any) { v["assurance_level"] = "verified" }},
		mutation{"invalid id", func(v map[string]any) { v["instance_id"] = "bad" }},
		mutation{"unknown resource", func(v map[string]any) { v["resources"].(map[string]any)["unknown"] = "unknown" }},
		mutation{"unsafe resource", func(v map[string]any) { v["resources"].(map[string]any)["agent_entry"] = "../escape" }},
		mutation{"unnormalized resource", func(v map[string]any) { v["resources"].(map[string]any)["agent_entry"] = "a/./b" }},
		mutation{"resource collision", func(v map[string]any) { v["resources"].(map[string]any)["design_spec"] = "AGENTS.md" }},
		mutation{"resource mismatch", func(v map[string]any) { v["resources"].(map[string]any)["agent_entry"] = "other/AGENTS.md" }},
		mutation{"unknown managed", func(v map[string]any) { v["managed_files"].(map[string]any)["extra"] = strings.Repeat("c", 64) }},
		mutation{"bad hash", func(v map[string]any) {
			v["managed_files"].(map[string]any)["schema_root/instance.schema.json"] = "ABC"
		}},
	)
	for key := range resources {
		key := key
		mutations = append(mutations, mutation{"missing resource " + key, func(v map[string]any) { delete(v["resources"].(map[string]any), key) }})
	}
	for _, key := range []string{"schema_root/install-manifest.schema.json", "schema_root/instance.schema.json"} {
		key := key
		mutations = append(mutations, mutation{"missing managed " + key, func(v map[string]any) { delete(v["managed_files"].(map[string]any), key) }})
	}
	for _, tc := range mutations {
		name, mutate := tc.name, tc.mutate
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(object)
			var candidate map[string]any
			_ = json.Unmarshal(raw, &candidate)
			mutate(candidate)
			encoded, _ := json.Marshal(candidate)
			if err := validateInstanceBytes(append(encoded, '\n'), resources, "0.1.0-dev"); err == nil {
				t.Fatal("invalid instance was accepted")
			}
		})
	}
}
