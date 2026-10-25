package vscodekotlin

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    []int
		wantErr bool
	}{
		{in: "21.0.12.1", want: []int{21, 0, 12, 1}},
		{in: "1.8.0_292", want: []int{8, 0, 292}},
		{in: "1.8.0_392", want: []int{8, 0, 392}},
		{in: "25-ea", want: []int{25}},
		{in: "21.0.1+12", want: []int{21, 0, 1}},
		{in: "17.0.2-beta", want: []int{17, 0, 2}},
		{in: "17", want: []int{17}},
		{in: "ea-25", wantErr: true},
		{in: "-ea", wantErr: true},
		{in: "x21", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseVersion(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseVersion(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVersion(%q) error = %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a, b []int
		want int
	}{
		{"missing components are zero", []int{21}, []int{21, 0}, 0},
		{"longer fourth component wins", []int{21, 0, 12, 1}, []int{21, 0, 1}, 1},
		{"numeric not lexicographic", []int{21, 0, 12}, []int{21, 0, 2}, 1},
		{"lower major", []int{21}, []int{24}, -1},
		{"equal", []int{17, 0, 9}, []int{17, 0, 9}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareVersions(tt.a, tt.b); got != tt.want {
				t.Errorf("compareVersions(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func mk(path, raw string) candidate {
	v, err := parseVersion(raw)
	if err != nil {
		panic(err)
	}
	return candidate{path: path, raw: raw, version: v}
}

func TestSelectJDK(t *testing.T) {
	tests := []struct {
		name     string
		cands    []candidate
		wantPath string
		wantOK   bool
	}{
		{"highest in range", []candidate{mk("/j/8", "1.8.0_392"), mk("/j/17", "17.0.9"), mk("/j/21", "21.0.12.1"), mk("/j/25", "25.0.1")}, "/j/21", true},
		{"floor boundary 11 accepted", []candidate{mk("/j/11", "11.0.2")}, "/j/11", true},
		{"below floor 10 rejected", []candidate{mk("/j/10", "10.0.2")}, "", false},
		{"highest under ceiling", []candidate{mk("/j/17", "17.0.9"), mk("/j/21", "21.0.12.1"), mk("/j/25", "25.0.1")}, "/j/21", true},
		{"ceiling boundary accepted", []candidate{mk("/j/24", "24.0.1")}, "/j/24", true},
		{"over ceiling rejected", []candidate{mk("/j/25", "25.0.1")}, "", false},
		{"suffixed over ceiling rejected", []candidate{mk("/j/25", "25-ea")}, "", false},
		{"mixed over ceiling", []candidate{mk("/j/26", "26"), mk("/j/24", "24.0.1+26")}, "/j/24", true},
		{"legacy 1.8 loses to 11", []candidate{mk("/j/8", "1.8.0_392"), mk("/j/11", "11.0.2")}, "/j/11", true},
		{"legacy 1.8 alone rejected", []candidate{mk("/j/8", "1.8.0_392")}, "", false},
		{"higher full version wins", []candidate{mk("/j/a", "21.0.2"), mk("/j/b", "21.0.12.1")}, "/j/b", true},
		{"equal version smallest path", []candidate{mk("/j/z", "21.0.2"), mk("/j/a", "21.0.2")}, "/j/a", true},
		{"21 vs 21.0 falls to path", []candidate{mk("/j/b", "21"), mk("/j/a", "21.0")}, "/j/a", true},
		{"none", nil, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := selectJDK(tt.cands, minSupportedMajor, maxSupportedMajor)
			if ok != tt.wantOK || (ok && got.path != tt.wantPath) {
				t.Errorf("selectJDK() = %q, %v; want %q, %v", got.path, ok, tt.wantPath, tt.wantOK)
			}
		})
	}
}

// fakeJDK creates root/name with a release file and, if withJava, bin/java.
func fakeJDK(t *testing.T, root, name, release string, withJava bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if withJava {
		if err := os.WriteFile(filepath.Join(dir, "bin", "java"), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if release != "" {
		if err := os.WriteFile(filepath.Join(dir, "release"), []byte(release), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	real21 := fakeJDK(t, root, "java-21-openjdk-amd64", "JAVA_VERSION=\"21.0.12.1\"\n", true)
	fakeJDK(t, root, "no-java", "JAVA_VERSION=\"17.0.1\"\n", false)
	fakeJDK(t, root, "bad-release", "JAVA_VERSION=\"abc\"\n", true)
	fakeJDK(t, root, "empty-release", "JAVA_VERSION=\"\"\n", true)
	fakeJDK(t, root, "missing-release", "", true)
	fakeJDK(t, root, "jdk-25-ea", "JAVA_VERSION=\"25-ea\"\n", true)
	if err := os.Mkdir(filepath.Join(root, "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plain-file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("java-21-openjdk-amd64", filepath.Join(root, "java-1.21.0-openjdk-amd64")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("java-1.21.0-openjdk-amd64", filepath.Join(root, "default-java")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("does-not-exist", filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}

	cands, seen, skips, err := discover(root)
	if err != nil {
		t.Fatalf("discover() error = %v", err)
	}

	resolvedReal, err := filepath.EvalSymlinks(real21)
	if err != nil {
		t.Fatal(err)
	}
	gotPaths := map[string]string{}
	for _, c := range cands {
		gotPaths[c.path] = c.raw
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %v, want 2 (21 deduped via symlinks, 25-ea)", gotPaths)
	}
	if gotPaths[resolvedReal] != "21.0.12.1" {
		t.Errorf("candidates = %v, want resolved %q => 21.0.12.1", gotPaths, resolvedReal)
	}
	wantSeen := map[string]bool{"21.0.12.1": true, "25-ea": true}
	if len(seen) != len(wantSeen) {
		t.Errorf("seen = %v, want %v", seen, wantSeen)
	}
	for _, v := range seen {
		if !wantSeen[v] {
			t.Errorf("seen = %v, unexpected %q", seen, v)
		}
	}

	// Plain file, dangling link, symlink duplicates and the dir with
	// neither bin/java nor release must stay silent.
	wantSkipped := map[string]string{
		"no-java":         "missing bin/java",
		"bad-release":     "JAVA_VERSION \"abc\"",
		"empty-release":   "release:",
		"missing-release": "missing release file",
	}
	if len(skips) != len(wantSkipped) {
		t.Fatalf("skipped = %v, want names %v", skips, wantSkipped)
	}
	for _, sk := range skips {
		want, ok := wantSkipped[sk.name]
		if !ok {
			t.Errorf("skipped = %v, unexpected %q", skips, sk.name)
			continue
		}
		if !strings.Contains(sk.reason, want) {
			t.Errorf("skipped %q reason = %q, want substring %q", sk.name, sk.reason, want)
		}
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nope")
	_, _, _, err := discover(root)
	if err == nil || !strings.Contains(err.Error(), root) {
		t.Errorf("discover() error = %v, want it to name %q", err, root)
	}
}
