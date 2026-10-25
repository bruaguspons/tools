package vscodesettings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setKey returns a Mutator that sets key to the JSON-encoded value.
func setKey(key string, value any) Mutator {
	return func(order []string, values map[string]json.RawMessage) ([]string, map[string]json.RawMessage, error) {
		raw, _ := json.Marshal(value)
		SetField(&order, values, key, raw)
		return order, values, nil
	}
}

func writeSettings(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, settingsRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name        string
		existing    string // empty means the file does not exist
		mutate      Mutator
		want        string
		wantChanged bool
	}{
		{
			name:        "absent file is created with 4-space indent",
			mutate:      setKey("c", 3),
			want:        "{\n    \"c\": 3\n}\n",
			wantChanged: true,
		},
		{
			name:        "order and 4-space indent preserved, new key appended",
			existing:    "{\n    \"b\": 1,\n    \"a\": {\n        \"x\": 2\n    }\n}\n",
			mutate:      setKey("c", 3),
			want:        "{\n    \"b\": 1,\n    \"a\": {\n        \"x\": 2\n    },\n    \"c\": 3\n}\n",
			wantChanged: true,
		},
		{
			name:        "2-space indent preserved",
			existing:    "{\n  \"b\": 1\n}\n",
			mutate:      setKey("c", 3),
			want:        "{\n  \"b\": 1,\n  \"c\": 3\n}\n",
			wantChanged: true,
		},
		{
			name:        "tab indent preserved",
			existing:    "{\n\t\"b\": 1,\n\t\"a\": {\n\t\t\"x\": 2\n\t}\n}\n",
			mutate:      setKey("c", 3),
			want:        "{\n\t\"b\": 1,\n\t\"a\": {\n\t\t\"x\": 2\n\t},\n\t\"c\": 3\n}\n",
			wantChanged: true,
		},
		{
			name:        "existing key updated in place",
			existing:    "{\n    \"a\": 1,\n    \"b\": 2,\n    \"c\": 3\n}\n",
			mutate:      setKey("b", 9),
			want:        "{\n    \"a\": 1,\n    \"b\": 9,\n    \"c\": 3\n}\n",
			wantChanged: true,
		},
		{
			name:        "identical result is a no-op",
			existing:    "{\n    \"a\": 1\n}\n",
			mutate:      setKey("a", 1),
			want:        "{\n    \"a\": 1\n}\n",
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, settingsRelPath)
			if tt.existing != "" {
				writeSettings(t, dir, tt.existing)
			}

			result, err := Merge(dir, tt.mutate)
			if err != nil {
				t.Fatalf("Merge() error = %v", err)
			}
			if result.Path != path {
				t.Errorf("Result.Path = %q, want %q", result.Path, path)
			}
			if result.Changed != tt.wantChanged {
				t.Errorf("Result.Changed = %v, want %v", result.Changed, tt.wantChanged)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
			if tt.existing != "" && tt.existing != "{}\n" {
				var v map[string]any
				if err := json.Unmarshal(got, &v); err != nil {
					t.Errorf("output is not valid JSON: %v", err)
				}
			}
		})
	}
}

func TestMergeNoOpDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := writeSettings(t, dir, "{\n    \"a\": 1\n}\n")
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}

	result, err := Merge(dir, setKey("a", 1))
	if err != nil || result.Changed {
		t.Fatalf("Merge() = %+v, %v; want unchanged", result, err)
	}
	info, _ := os.Stat(path)
	if !info.ModTime().Equal(past) {
		t.Errorf("mtime = %v, want %v (no write)", info.ModTime(), past)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("dir entries = %v, want only settings.json", entries)
	}
}

func TestMergePreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := writeSettings(t, dir, "{\n    \"a\": 1\n}\n")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Merge(dir, setKey("b", 2)); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("existing file mode = %v, want 0600", info.Mode().Perm())
	}

	fresh := t.TempDir()
	if _, err := Merge(fresh, setKey("b", 2)); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(filepath.Join(fresh, settingsRelPath))
	if info.Mode().Perm() != 0o644 {
		t.Errorf("new file mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestMergeErrors(t *testing.T) {
	const jsoncHint = "JSONC is not supported"
	tests := []struct {
		name       string
		existing   string
		mutate     Mutator
		wantErr    string
		wantLabel  string // error prefix before the path; defaults to "parse "
		wantNoHint bool
	}{
		{
			name:     "line comment gives JSONC hint",
			existing: "{\n    // note\n    \"a\": 1\n}\n",
			mutate:   setKey("b", 2),
			wantErr:  jsoncHint,
		},
		{
			name:     "block comment gives JSONC hint",
			existing: "{\n    /* note */\n    \"a\": 1\n}\n",
			mutate:   setKey("b", 2),
			wantErr:  jsoncHint,
		},
		{
			name:     "trailing comma gives hint",
			existing: "{\n    \"a\": 1,\n}\n",
			mutate:   setKey("b", 2),
			wantErr:  "trailing comma",
		},
		{
			name:       "truncated JSON has no JSONC hint",
			existing:   "{\n    \"a\": 1",
			mutate:     setKey("b", 2),
			wantErr:    "parse ",
			wantNoHint: true,
		},
		{
			name:     "non-object top level",
			existing: "[]\n",
			mutate:   setKey("b", 2),
			wantErr:  "top-level JSON value must be an object",
		},
		{
			name:     "mutate error is wrapped with update path",
			existing: "{}\n",
			mutate: func([]string, map[string]json.RawMessage) ([]string, map[string]json.RawMessage, error) {
				return nil, nil, errors.New("boom")
			},
			wantErr:   ": boom",
			wantLabel: "update ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeSettings(t, dir, tt.existing)

			_, err := Merge(dir, tt.mutate)
			if err == nil {
				t.Fatal("Merge() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err, tt.wantErr)
			}
			label := tt.wantLabel
			if label == "" {
				label = "parse "
			}
			if !strings.Contains(err.Error(), label+path) {
				t.Errorf("error = %q, want it to name %q", err, label+path)
			}
			// The temp dir name embeds the test name, so strip the path first.
			msg := strings.ReplaceAll(err.Error(), path, "<path>")
			if tt.wantNoHint && (strings.Contains(msg, "JSONC") || strings.Contains(msg, "trailing comma")) {
				t.Errorf("error = %q, want no JSONC hint", err)
			}
			got, _ := os.ReadFile(path)
			if string(got) != tt.existing {
				t.Errorf("file bytes changed: %q, want %q", got, tt.existing)
			}
		})
	}
}

func TestMergeMutateErrorExactText(t *testing.T) {
	dir := t.TempDir()
	path := writeSettings(t, dir, "{}\n")
	_, err := Merge(dir, func([]string, map[string]json.RawMessage) ([]string, map[string]json.RawMessage, error) {
		return nil, nil, errors.New("boom")
	})
	if want := "update " + path + ": boom"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}

func TestMergeSymlink(t *testing.T) {
	assertNoTemp := func(t *testing.T, dirs ...string) {
		t.Helper()
		for _, d := range dirs {
			entries, _ := os.ReadDir(d)
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".tmp") {
					t.Errorf("leftover temp file %s in %s", e.Name(), d)
				}
			}
		}
	}

	tests := []struct {
		name     string
		relative bool
		mode     os.FileMode
	}{
		{"absolute link keeps 0600", false, 0o600},
		{"relative link keeps 0644", true, 0o644},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			shared := t.TempDir()
			real := filepath.Join(shared, "real.json")
			if err := os.WriteFile(real, []byte("{\n    \"a\": 1\n}\n"), tt.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(real, tt.mode); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, settingsRelPath)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			dest := real
			if tt.relative {
				rel, err := filepath.Rel(filepath.Dir(link), real)
				if err != nil {
					t.Fatal(err)
				}
				dest = rel
			}
			if err := os.Symlink(dest, link); err != nil {
				t.Fatal(err)
			}

			result, err := Merge(dir, setKey("b", 2))
			if err != nil {
				t.Fatalf("Merge() error = %v", err)
			}
			if result.Path != link || !result.Changed {
				t.Errorf("Result = %+v, want logical path %q and Changed", result, link)
			}
			info, err := os.Lstat(link)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("settings.json is no longer a symlink: %v, %v", info, err)
			}
			got, _ := os.ReadFile(real)
			if want := "{\n    \"a\": 1,\n    \"b\": 2\n}\n"; string(got) != want {
				t.Errorf("target = %q, want %q", got, want)
			}
			tinfo, _ := os.Stat(real)
			if tinfo.Mode().Perm() != tt.mode {
				t.Errorf("target mode = %v, want %v", tinfo.Mode().Perm(), tt.mode)
			}
			assertNoTemp(t, filepath.Dir(link), shared)
		})
	}

	t.Run("dangling link errors and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(t.TempDir(), "missing.json")
		link := filepath.Join(dir, settingsRelPath)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(missing, link); err != nil {
			t.Fatal(err)
		}

		_, err := Merge(dir, setKey("b", 2))
		if err == nil || !strings.Contains(err.Error(), "dangling symlink") {
			t.Fatalf("Merge() error = %v, want dangling symlink error", err)
		}
		if _, statErr := os.Lstat(missing); !os.IsNotExist(statErr) {
			t.Errorf("target stat err = %v, want not exist", statErr)
		}
		if dest, _ := os.Readlink(link); dest != missing {
			t.Errorf("link = %q, want unchanged %q", dest, missing)
		}
		assertNoTemp(t, filepath.Dir(link), filepath.Dir(missing))
	})
}
