package vscodejava

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

// newFakeJDK creates a directory with a bin/java file and a release file
// (JAVA_VERSION="21.0.1", i.e. runtime name JavaSE-21) inside it, and
// returns the directory path, suitable for use as JAVA_HOME.
func newFakeJDK(t *testing.T) string {
	t.Helper()
	return newFakeJDKWithVersion(t, `JAVA_VERSION="21.0.1"`)
}

// newFakeJDKWithVersion creates a fake JDK like newFakeJDK, but with the
// given release file content instead of the default JAVA_VERSION line.
// An empty releaseLine omits the release file entirely.
func newFakeJDKWithVersion(t *testing.T, releaseLine string) string {
	t.Helper()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", binDir, err)
	}
	javaPath := filepath.Join(binDir, "java")
	if err := os.WriteFile(javaPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", javaPath, err)
	}
	if releaseLine != "" {
		releasePath := filepath.Join(dir, "release")
		content := releaseLine + "\n"
		if err := os.WriteFile(releasePath, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) failed: %v", releasePath, err)
		}
	}
	return dir
}

func writeSettings(t *testing.T, dir, content string) string {
	t.Helper()
	vscodeDir := filepath.Join(dir, ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", vscodeDir, err)
	}
	path := filepath.Join(vscodeDir, "settings.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", path, err)
	}
	return path
}

func TestConfigure(t *testing.T) {
	tests := []struct {
		name            string
		existing        *string // nil = no .vscode/settings.json at all
		want            string
		wantErrContains string
	}{
		{
			name:     "creates settings when absent",
			existing: nil,
			want: `{
    "java.jdt.ls.java.home": "` + "JAVA_HOME_PLACEHOLDER" + `",
    "java.configuration.updateBuildConfiguration": "automatic",
    "java.format.onType.enabled": false,
    "java.configuration.runtimes": [
        {
            "name": "JavaSE-21",
            "path": "` + "JAVA_HOME_PLACEHOLDER" + `",
            "default": true
        }
    ]
}
`,
		},
		{
			name: "preserves unrelated keys and their order",
			existing: strPtr(`{
    "editor.tabSize": 2,
    "files.exclude": {
        "**/.git": true
    }
}
`),
			want: `{
    "editor.tabSize": 2,
    "files.exclude": {
        "**/.git": true
    },
    "java.jdt.ls.java.home": "` + "JAVA_HOME_PLACEHOLDER" + `",
    "java.configuration.updateBuildConfiguration": "automatic",
    "java.format.onType.enabled": false,
    "java.configuration.runtimes": [
        {
            "name": "JavaSE-21",
            "path": "` + "JAVA_HOME_PLACEHOLDER" + `",
            "default": true
        }
    ]
}
`,
		},
		{
			name: "overwrites stale owned key in place",
			existing: strPtr(`{
    "java.jdt.ls.java.home": "/old/jdk",
    "editor.tabSize": 4
}
`),
			want: `{
    "java.jdt.ls.java.home": "` + "JAVA_HOME_PLACEHOLDER" + `",
    "editor.tabSize": 4,
    "java.configuration.updateBuildConfiguration": "automatic",
    "java.format.onType.enabled": false,
    "java.configuration.runtimes": [
        {
            "name": "JavaSE-21",
            "path": "` + "JAVA_HOME_PLACEHOLDER" + `",
            "default": true
        }
    ]
}
`,
		},
		{
			name:     "empty file treated as empty object",
			existing: strPtr(""),
			want: `{
    "java.jdt.ls.java.home": "` + "JAVA_HOME_PLACEHOLDER" + `",
    "java.configuration.updateBuildConfiguration": "automatic",
    "java.format.onType.enabled": false,
    "java.configuration.runtimes": [
        {
            "name": "JavaSE-21",
            "path": "` + "JAVA_HOME_PLACEHOLDER" + `",
            "default": true
        }
    ]
}
`,
		},
		{
			name:     "whitespace-only file treated as empty object",
			existing: strPtr("   \n\t\n"),
			want: `{
    "java.jdt.ls.java.home": "` + "JAVA_HOME_PLACEHOLDER" + `",
    "java.configuration.updateBuildConfiguration": "automatic",
    "java.format.onType.enabled": false,
    "java.configuration.runtimes": [
        {
            "name": "JavaSE-21",
            "path": "` + "JAVA_HOME_PLACEHOLDER" + `",
            "default": true
        }
    ]
}
`,
		},
		{
			name: "line comment fails loudly and makes no change",
			existing: strPtr(`{
    // a comment
    "editor.tabSize": 2
}
`),
			wantErrContains: "comment",
		},
		{
			name: "block comment fails loudly and makes no change",
			existing: strPtr(`{
    /* a comment */
    "editor.tabSize": 2
}
`),
			wantErrContains: "comment",
		},
		{
			name: "trailing comma fails loudly",
			existing: strPtr(`{
    "editor.tabSize": 2,
}
`),
			wantErrContains: "trailing comma",
		},
		{
			name:            "top-level array fails loudly",
			existing:        strPtr(`["not", "an", "object"]`),
			wantErrContains: "object",
		},
		{
			name:            "top-level scalar fails loudly",
			existing:        strPtr(`"just a string"`),
			wantErrContains: "object",
		},
		{
			name: "2-space indent is preserved",
			existing: strPtr(`{
  "editor.tabSize": 2
}
`),
			want: `{
  "editor.tabSize": 2,
  "java.jdt.ls.java.home": "` + "JAVA_HOME_PLACEHOLDER" + `",
  "java.configuration.updateBuildConfiguration": "automatic",
  "java.format.onType.enabled": false,
  "java.configuration.runtimes": [
    {
      "name": "JavaSE-21",
      "path": "` + "JAVA_HOME_PLACEHOLDER" + `",
      "default": true
    }
  ]
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			javaHome := newFakeJDK(t)
			t.Setenv("JAVA_HOME", javaHome)

			var originalContent []byte
			if tt.existing != nil {
				path := writeSettings(t, dir, *tt.existing)
				originalContent = []byte(*tt.existing)
				_ = path
			}

			result, err := Configure(dir)

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("Configure() error = nil, want error containing %q", tt.wantErrContains)
				}
				if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErrContains)) {
					t.Errorf("Configure() error = %q, want substring %q", err.Error(), tt.wantErrContains)
				}
				// File must be left unchanged on disk.
				if tt.existing != nil {
					got, readErr := os.ReadFile(filepath.Join(dir, ".vscode", "settings.json"))
					if readErr != nil {
						t.Fatalf("ReadFile after failed Configure: %v", readErr)
					}
					if !bytes.Equal(got, originalContent) {
						t.Errorf("settings.json was modified despite parse failure: got %q, want unchanged %q", got, originalContent)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Configure() unexpected error: %v", err)
			}

			wantPath := filepath.Join(dir, ".vscode", "settings.json")
			if result.Path != wantPath {
				t.Errorf("Result.Path = %q, want %q", result.Path, wantPath)
			}
			if !result.Changed {
				t.Errorf("Result.Changed = false, want true on first successful run")
			}

			want := replacePlaceholder(tt.want, javaHome)
			got, readErr := os.ReadFile(wantPath)
			if readErr != nil {
				t.Fatalf("ReadFile(%s) failed: %v", wantPath, readErr)
			}
			if string(got) != want {
				t.Errorf("settings.json content =\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestConfigureIdempotent(t *testing.T) {
	dir := t.TempDir()
	javaHome := newFakeJDK(t)
	t.Setenv("JAVA_HOME", javaHome)

	first, err := Configure(dir)
	if err != nil {
		t.Fatalf("first Configure() failed: %v", err)
	}
	if !first.Changed {
		t.Fatalf("first Configure().Changed = false, want true")
	}

	firstContent, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", first.Path, err)
	}

	second, err := Configure(dir)
	if err != nil {
		t.Fatalf("second Configure() failed: %v", err)
	}
	if second.Changed {
		t.Errorf("second Configure().Changed = true, want false (idempotent no-op)")
	}

	secondContent, err := os.ReadFile(second.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", second.Path, err)
	}
	if !bytes.Equal(firstContent, secondContent) {
		t.Errorf("second run produced different content:\nfirst:\n%s\nsecond:\n%s", firstContent, secondContent)
	}
}

func TestConfigureJavaHomeUnset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAVA_HOME", "")

	existing := "{\n    \"editor.tabSize\": 2\n}\n"
	writeSettings(t, dir, existing)

	_, err := Configure(dir)
	if err == nil {
		t.Fatal("Configure() error = nil, want error naming JAVA_HOME")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("JAVA_HOME")) {
		t.Errorf("Configure() error = %q, want it to name JAVA_HOME", err.Error())
	}

	got, readErr := os.ReadFile(filepath.Join(dir, ".vscode", "settings.json"))
	if readErr != nil {
		t.Fatalf("ReadFile after failed Configure: %v", readErr)
	}
	if string(got) != existing {
		t.Errorf("settings.json was modified despite JAVA_HOME being unset: got %q, want unchanged %q", got, existing)
	}
}

func TestConfigureJavaHomeMissingBinJava(t *testing.T) {
	dir := t.TempDir()
	// A directory that exists but has no bin/java inside it.
	badHome := t.TempDir()
	t.Setenv("JAVA_HOME", badHome)

	_, err := Configure(dir)
	if err == nil {
		t.Fatal("Configure() error = nil, want error about missing bin/java")
	}
}

func TestConfigureLegacyJavaVersion(t *testing.T) {
	dir := t.TempDir()
	javaHome := newFakeJDKWithVersion(t, `JAVA_VERSION="1.8.0_292"`)
	t.Setenv("JAVA_HOME", javaHome)

	result, err := Configure(dir)
	if err != nil {
		t.Fatalf("Configure() failed: %v", err)
	}

	got, readErr := os.ReadFile(result.Path)
	if readErr != nil {
		t.Fatalf("ReadFile(%s) failed: %v", result.Path, readErr)
	}
	if !bytes.Contains(got, []byte(`"name": "JavaSE-1.8"`)) {
		t.Errorf("settings.json = %s, want it to contain JavaSE-1.8 runtime name", got)
	}
}

func TestConfigureNoReleaseFile(t *testing.T) {
	dir := t.TempDir()
	javaHome := newFakeJDKWithVersion(t, "")
	t.Setenv("JAVA_HOME", javaHome)

	existing := "{\n    \"editor.tabSize\": 2\n}\n"
	writeSettings(t, dir, existing)

	_, err := Configure(dir)
	if err == nil {
		t.Fatal("Configure() error = nil, want error mentioning release")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("release")) {
		t.Errorf("Configure() error = %q, want it to mention release", err.Error())
	}

	got, readErr := os.ReadFile(filepath.Join(dir, ".vscode", "settings.json"))
	if readErr != nil {
		t.Fatalf("ReadFile after failed Configure: %v", readErr)
	}
	if string(got) != existing {
		t.Errorf("settings.json was modified despite missing release file: got %q, want unchanged %q", got, existing)
	}
}

func TestConfigureUnparseableJavaVersion(t *testing.T) {
	tests := []struct {
		name        string
		releaseLine string
	}{
		{name: "no JAVA_VERSION line", releaseLine: `OS_NAME="Linux"`},
		{name: "empty JAVA_VERSION", releaseLine: `JAVA_VERSION=""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			javaHome := newFakeJDKWithVersion(t, tt.releaseLine)
			t.Setenv("JAVA_HOME", javaHome)

			existing := "{\n    \"editor.tabSize\": 2\n}\n"
			writeSettings(t, dir, existing)

			_, err := Configure(dir)
			if err == nil {
				t.Fatal("Configure() error = nil, want error about JAVA_VERSION")
			}

			got, readErr := os.ReadFile(filepath.Join(dir, ".vscode", "settings.json"))
			if readErr != nil {
				t.Fatalf("ReadFile after failed Configure: %v", readErr)
			}
			if string(got) != existing {
				t.Errorf("settings.json was modified despite unparseable JAVA_VERSION: got %q, want unchanged %q", got, existing)
			}
		})
	}
}

func TestConfigureRuntimesMerge(t *testing.T) {
	tests := []struct {
		name            string
		existing        string
		wantContains    []string
		wantNotContains []string
		wantErrContains string
	}{
		{
			name: "updates matching entry in place, preserving extra fields and order",
			existing: `{
    "java.configuration.runtimes": [
        {
            "name": "JavaSE-21",
            "sources": "/some/src.zip",
            "default": false
        }
    ]
}
`,
			wantContains: []string{
				`"name": "JavaSE-21",
            "sources": "/some/src.zip",
            "default": true,
            "path": "` + "JAVA_HOME_PLACEHOLDER" + `"`,
			},
		},
		{
			name: "clears default on other entries and appends ours",
			existing: `{
    "java.configuration.runtimes": [
        {
            "name": "JavaSE-8",
            "path": "/old/jdk8",
            "default": true
        }
    ]
}
`,
			wantContains: []string{
				`"name": "JavaSE-8",
            "path": "/old/jdk8",
            "default": false`,
				`"name": "JavaSE-21",
            "path": "` + "JAVA_HOME_PLACEHOLDER" + `",
            "default": true`,
			},
		},
		{
			name: "non-array runtimes value fails loudly",
			existing: `{
    "java.configuration.runtimes": "not an array"
}
`,
			wantErrContains: "array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			javaHome := newFakeJDK(t)
			t.Setenv("JAVA_HOME", javaHome)

			writeSettings(t, dir, tt.existing)

			result, err := Configure(dir)

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("Configure() error = nil, want error containing %q", tt.wantErrContains)
				}
				if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErrContains)) {
					t.Errorf("Configure() error = %q, want substring %q", err.Error(), tt.wantErrContains)
				}
				got, readErr := os.ReadFile(filepath.Join(dir, ".vscode", "settings.json"))
				if readErr != nil {
					t.Fatalf("ReadFile after failed Configure: %v", readErr)
				}
				if string(got) != tt.existing {
					t.Errorf("settings.json was modified despite merge failure: got %q, want unchanged %q", got, tt.existing)
				}
				return
			}

			if err != nil {
				t.Fatalf("Configure() unexpected error: %v", err)
			}

			got, readErr := os.ReadFile(result.Path)
			if readErr != nil {
				t.Fatalf("ReadFile(%s) failed: %v", result.Path, readErr)
			}
			for _, want := range tt.wantContains {
				wantResolved := replacePlaceholder(want, javaHome)
				if !bytes.Contains(got, []byte(wantResolved)) {
					t.Errorf("settings.json =\n%s\nwant it to contain:\n%s", got, wantResolved)
				}
			}
		})
	}
}

func TestConfigureNestedDirOnlyWritesThatDir(t *testing.T) {
	parent := t.TempDir()
	nested := filepath.Join(parent, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", nested, err)
	}
	javaHome := newFakeJDK(t)
	t.Setenv("JAVA_HOME", javaHome)

	if _, err := Configure(nested); err != nil {
		t.Fatalf("Configure(%s) failed: %v", nested, err)
	}

	if _, err := os.Stat(filepath.Join(parent, ".vscode")); !os.IsNotExist(err) {
		t.Errorf("Configure wrote .vscode in the parent directory, want only in %s", nested)
	}
	if _, err := os.Stat(filepath.Join(nested, ".vscode", "settings.json")); err != nil {
		t.Errorf("expected settings.json in %s: %v", nested, err)
	}
}

func TestConfigureRealisticFixture(t *testing.T) {
	dir := t.TempDir()

	fixture, err := os.ReadFile(filepath.Join("testdata", "settings_real.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	writeSettings(t, dir, string(fixture))

	fakeJDK := newFakeJDK(t)
	t.Setenv("JAVA_HOME", fakeJDK)

	result, err := Configure(dir)
	if err != nil {
		t.Fatalf("Configure() failed: %v", err)
	}
	if !result.Changed {
		t.Fatalf("Configure().Changed = false, want true")
	}

	got, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", result.Path, err)
	}

	goldenPath := filepath.Join("testdata", "settings_real.golden")
	if *update {
		golden := bytes.ReplaceAll(got, []byte(fakeJDK), []byte("JAVA_HOME_PLACEHOLDER"))
		if err := os.WriteFile(goldenPath, golden, 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}

	wantTemplate, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	want := bytes.ReplaceAll(wantTemplate, []byte("JAVA_HOME_PLACEHOLDER"), []byte(fakeJDK))

	if !bytes.Equal(got, want) {
		t.Errorf("settings.json mismatch with golden:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func strPtr(s string) *string { return &s }

func replacePlaceholder(want, javaHome string) string {
	return string(bytes.ReplaceAll([]byte(want), []byte("JAVA_HOME_PLACEHOLDER"), []byte(javaHome)))
}
