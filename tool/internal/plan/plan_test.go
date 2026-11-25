package plan

import "testing"

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
