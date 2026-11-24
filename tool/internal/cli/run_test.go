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
