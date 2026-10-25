package vscodekotlin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const settingsPath = ".vscode/settings.json"

func TestConfigure(t *testing.T) {
	root := t.TempDir()
	real21 := fakeJDK(t, root, "java-21-openjdk-amd64", "JAVA_VERSION=\"21.0.12.1\"\n", true)
	fakeJDK(t, root, "java-25-openjdk-amd64", "JAVA_VERSION=\"25.0.1\"\n", true)
	if err := os.Symlink("java-21-openjdk-amd64", filepath.Join(root, "default-java")); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(real21)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("creates settings with only the owned key", func(t *testing.T) {
		dir := t.TempDir()
		result, err := configure(dir, root)
		if err != nil {
			t.Fatalf("configure() error = %v", err)
		}
		if !result.Changed {
			t.Error("Result.Changed = false, want true")
		}
		got, err := os.ReadFile(filepath.Join(dir, settingsPath))
		if err != nil {
			t.Fatal(err)
		}
		want := "{\n    \"kotlin.java.home\": \"" + resolved + "\"\n}\n"
		if string(got) != want {
			t.Errorf("settings = %q, want %q", got, want)
		}
	})

	t.Run("preserves other keys and replaces in place", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, settingsPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		existing := "{\n    \"a\": 1,\n    \"kotlin.java.home\": \"/old\",\n    \"z\": [true]\n}\n"
		if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := configure(dir, root); err != nil {
			t.Fatalf("configure() error = %v", err)
		}
		got, _ := os.ReadFile(path)
		want := "{\n    \"a\": 1,\n    \"kotlin.java.home\": \"" + resolved + "\",\n    \"z\": [\n        true\n    ]\n}\n"
		if string(got) != want {
			t.Errorf("settings = %q, want %q", got, want)
		}
	})

	t.Run("second run is a byte-identical no-op", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, settingsPath)
		if _, err := configure(dir, root); err != nil {
			t.Fatal(err)
		}
		first, _ := os.ReadFile(path)
		past := time.Now().Add(-time.Hour).Truncate(time.Second)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}

		result, err := configure(dir, root)
		if err != nil {
			t.Fatal(err)
		}
		if result.Changed {
			t.Error("Result.Changed = true, want false")
		}
		second, _ := os.ReadFile(path)
		if string(first) != string(second) {
			t.Errorf("bytes changed: %q -> %q", first, second)
		}
		if info, _ := os.Stat(path); !info.ModTime().Equal(past) {
			t.Errorf("mtime = %v, want %v (no write)", info.ModTime(), past)
		}
	})

	t.Run("JAVA_HOME is ignored", func(t *testing.T) {
		decoy := t.TempDir()
		t.Setenv("JAVA_HOME", decoy)
		dir := t.TempDir()
		if _, err := configure(dir, root); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(filepath.Join(dir, settingsPath))
		if strings.Contains(string(got), decoy) || !strings.Contains(string(got), resolved) {
			t.Errorf("settings = %q, want %q and not the JAVA_HOME decoy", got, resolved)
		}
	})
}

func TestConfigureErrors(t *testing.T) {
	t.Run("missing root names the root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "nope")
		_, err := configure(t.TempDir(), root)
		if err == nil || !strings.Contains(err.Error(), root) {
			t.Errorf("error = %v, want it to name %q", err, root)
		}
	})

	t.Run("empty root", func(t *testing.T) {
		root := t.TempDir()
		_, err := configure(t.TempDir(), root)
		if err == nil {
			t.Fatal("error = nil, want error")
		}
		for _, want := range []string{root, "[11, 24]", "found: none"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want substring %q", err, want)
			}
		}
		if strings.Contains(err.Error(), "skipped:") {
			t.Errorf("error = %q, want no skipped section", err)
		}
	})

	t.Run("only over-ceiling JDKs, nothing written", func(t *testing.T) {
		root := t.TempDir()
		fakeJDK(t, root, "jdk-25-ea", "JAVA_VERSION=\"25-ea\"\n", true)
		fakeJDK(t, root, "jdk-26", "JAVA_VERSION=\"26\"\n", true)
		dir := t.TempDir()

		_, err := configure(dir, root)
		if err == nil {
			t.Fatal("error = nil, want error")
		}
		for _, want := range []string{root, "found: 25-ea, 26"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want substring %q", err, want)
			}
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".vscode")); !os.IsNotExist(statErr) {
			t.Errorf(".vscode stat err = %v, want not exist", statErr)
		}
	})

	t.Run("only JDK 8 and a broken JDK, nothing written", func(t *testing.T) {
		root := t.TempDir()
		fakeJDK(t, root, "jdk-8", "JAVA_VERSION=\"1.8.0_392\"\n", true)
		fakeJDK(t, root, "broken", "JAVA_VERSION=\"17.0.1\"\n", false)
		dir := t.TempDir()

		_, err := configure(dir, root)
		if err == nil {
			t.Fatal("error = nil, want error")
		}
		for _, want := range []string{"[11, 24]", "found: 1.8.0_392", "skipped: broken (missing bin/java)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want substring %q", err, want)
			}
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".vscode")); !os.IsNotExist(statErr) {
			t.Errorf(".vscode stat err = %v, want not exist", statErr)
		}
	})
}
