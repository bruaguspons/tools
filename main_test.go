package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantExitCode  int
		wantStderrHas string
	}{
		{
			name:          "unknown subcommand",
			args:          []string{"frobnicate"},
			wantExitCode:  2,
			wantStderrHas: "frobnicate",
		},
		{
			name:          "no subcommand shows usage",
			args:          []string{},
			wantExitCode:  2,
			wantStderrHas: "Usage",
		},
		{
			name:         "version flag",
			args:         []string{"-version"},
			wantExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(tt.args, &stdout, &stderr)
			if got != tt.wantExitCode {
				t.Errorf("run(%v) exit code = %d, want %d (stderr=%q)", tt.args, got, tt.wantExitCode, stderr.String())
			}
			if tt.wantStderrHas != "" && !strings.Contains(stderr.String(), tt.wantStderrHas) {
				t.Errorf("run(%v) stderr = %q, want substring %q", tt.args, stderr.String(), tt.wantStderrHas)
			}
		})
	}
}

func TestRunExtraArgsToVscodeJava(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run([]string{"vscode-java", "../x"}, &stdout, &stderr)
	if got != 2 {
		t.Errorf("run(vscode-java with extra arg) exit code = %d, want 2 (stderr=%q)", got, stderr.String())
	}
}
