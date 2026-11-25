package cli

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "help",
			args:       []string{"help"},
			wantCode:   0,
			wantStdout: helpText,
		},
		{
			name:       "no arguments shows help",
			wantCode:   0,
			wantStdout: helpText,
		},
		{
			name:       "version",
			args:       []string{"version"},
			wantCode:   0,
			wantStdout: "aigov 0.1.0-dev\n",
		},
		{
			name:       "unknown command",
			args:       []string{"unsupported"},
			wantCode:   2,
			wantStderr: "unknown command: unsupported\nRun 'aigov help' for usage.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			gotCode := Run(test.args, &stdout, &stderr)

			if gotCode != test.wantCode {
				t.Fatalf("Run() exit code = %d, want %d", gotCode, test.wantCode)
			}
			if got := stdout.String(); got != test.wantStdout {
				t.Errorf("Run() stdout = %q, want %q", got, test.wantStdout)
			}
			if got := stderr.String(); got != test.wantStderr {
				t.Errorf("Run() stderr = %q, want %q", got, test.wantStderr)
			}
		})
	}
}

func TestExperimentalPlanGuard(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("AIGOV_ENABLE_EXPERIMENTAL_PLAN", "")
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		gotCode := Run([]string{"plan", "--manifest", "manifest.json"}, &stdout, &stderr)
		if gotCode != 2 || stdout.String() != "" || stderr.String() != "experimental plan is disabled; set AIGOV_ENABLE_EXPERIMENTAL_PLAN=1 to enable\n" {
			t.Fatalf("disabled plan = code %d stdout %q stderr %q", gotCode, stdout.String(), stderr.String())
		}
	})

	t.Run("enabled requires manifest", func(t *testing.T) {
		t.Setenv("AIGOV_ENABLE_EXPERIMENTAL_PLAN", "1")
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		gotCode := Run([]string{"plan"}, &stdout, &stderr)
		if gotCode != 2 || stdout.String() != "" || stderr.String() != "usage: aigov plan --manifest <path>\n" {
			t.Fatalf("plan usage = code %d stdout %q stderr %q", gotCode, stdout.String(), stderr.String())
		}
	})
}

func TestExperimentalApplyGuardAndApproval(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("AIGOV_ENABLE_EXPERIMENTAL_APPLY", "")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"apply", "--manifest", "manifest.json", "--plan-sha256", "bad"}, &stdout, &stderr)
		if code != 2 || stdout.String() != "" || stderr.String() != "experimental apply is disabled; set AIGOV_ENABLE_EXPERIMENTAL_APPLY=1 to enable\n" {
			t.Fatalf("disabled apply = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
		}
	})
	t.Run("missing approval", func(t *testing.T) {
		t.Setenv("AIGOV_ENABLE_EXPERIMENTAL_APPLY", "1")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"apply", "--manifest", "manifest.json"}, &stdout, &stderr)
		want := "{\"code\":\"E_PLAN_APPROVAL_REQUIRED\",\"path\":\"/plan_sha256\",\"message\":\"provide the complete approved plan stdout SHA-256\"}\n"
		if code != 2 || stdout.String() != "" || stderr.String() != want {
			t.Fatalf("missing approval = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
		}
	})
}

func TestExperimentalEvidenceGuardAndUsage(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("AIGOV_ENABLE_EXPERIMENTAL_EVIDENCE", "")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"evidence", "begin", "--project", ".", "--request", "request.json"}, &stdout, &stderr)
		want := "experimental evidence is disabled; set AIGOV_ENABLE_EXPERIMENTAL_EVIDENCE=1 to enable\n"
		if code != 2 || stdout.String() != "" || stderr.String() != want {
			t.Fatalf("disabled evidence = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
		}
	})

	t.Run("enabled requires complete arguments", func(t *testing.T) {
		t.Setenv("AIGOV_ENABLE_EXPERIMENTAL_EVIDENCE", "1")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"evidence", "begin"}, &stdout, &stderr)
		want := "{\"code\":\"E_EVIDENCE_REQUEST\",\"message\":\"required evidence arguments are missing or incompatible\",\"path\":\"/arguments\"}\n"
		if code != 2 || stdout.String() != "" || stderr.String() != want {
			t.Fatalf("evidence usage = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
		}
	})
}
