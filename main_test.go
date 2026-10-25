package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/bruaguspons/tools/internal/vscodesettings"
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

func TestRunExtraArgsStderr(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "vscode-java",
			args:       []string{"vscode-java", "x"},
			wantStderr: "tools: vscode-java takes no arguments (got [x])\n",
		},
		{
			name:       "vscode-kotlin",
			args:       []string{"vscode-kotlin", "x"},
			wantStderr: "tools: vscode-kotlin takes no arguments (got [x])\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run in an empty dir to prove nothing is written.
			dir := t.TempDir()
			chdir(t, dir)

			var stdout, stderr bytes.Buffer
			got := run(tt.args, &stdout, &stderr)
			if got != 2 {
				t.Errorf("exit code = %d, want 2", got)
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("working dir has %v, want it untouched", entries)
			}
		})
	}
}

func TestUsageListsSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"--help"}, &stdout, &stderr); got != 0 {
		t.Fatalf("exit code = %d, want 0", got)
	}
	for _, name := range []string{"vscode-java", "vscode-kotlin"} {
		if !strings.Contains(stdout.String(), name) {
			t.Errorf("usage = %q, want it to list %q", stdout.String(), name)
		}
	}
}

func TestNoArgSettingsCommand(t *testing.T) {
	tests := []struct {
		name       string
		result     vscodesettings.Result
		err        error
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "changed",
			result:     vscodesettings.Result{Path: "/p/settings.json", Changed: true},
			wantStdout: "tools: updated /p/settings.json\n",
		},
		{
			name:       "no-op",
			result:     vscodesettings.Result{Path: "/p/settings.json"},
			wantStdout: "tools: /p/settings.json already up to date\n",
		},
		{
			name:       "error",
			err:        errors.New("boom"),
			wantCode:   1,
			wantStderr: "tools: boom\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := noArgSettingsCommand("vscode-x", func(string) (vscodesettings.Result, error) {
				return tt.result, tt.err
			})
			var stdout, stderr bytes.Buffer
			if got := run(nil, &stdout, &stderr); got != tt.wantCode {
				t.Errorf("exit code = %d, want %d", got, tt.wantCode)
			}
			if stdout.String() != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantStdout)
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// chdir switches the working directory for the test and restores it after.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}
