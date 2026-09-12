// Package vscodejava configures the VS Code Java extension's run/debug
// settings for the current project by merging a small, fixed set of owned
// keys into ./.vscode/settings.json.
package vscodejava

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Result reports the outcome of a Configure call.
type Result struct {
	// Path is the absolute path to the settings.json file that was read
	// (and possibly written).
	Path string
	// Changed is true if settings.json was created or its content changed.
	Changed bool
}

// Owned keys: the tool is authoritative for exactly these two settings.
// Every other key in settings.json is preserved untouched.
const (
	keyJavaHome             = "java.jdt.ls.java.home"
	keyUpdateBuildConfig    = "java.configuration.updateBuildConfiguration"
	valueUpdateBuildConfig  = "automatic"
	defaultIndent           = "    " // 4 spaces, VS Code's own default
	settingsRelPath         = ".vscode/settings.json"
	settingsDirPermissions  = 0o755
	settingsFilePermissions = 0o644
)

// Configure resolves the JDK from $JAVA_HOME, merges the owned Java keys
// into dir/.vscode/settings.json (creating it if absent), and writes the
// result atomically. Unrelated keys and their original order are
// preserved. If the merged content is byte-identical to what is already
// on disk, no write occurs and Result.Changed is false.
func Configure(dir string) (Result, error) {
	javaHome, err := validateJavaHome()
	if err != nil {
		return Result{}, err
	}

	path := filepath.Join(dir, settingsRelPath)

	existing, err := os.ReadFile(path)
	fileExists := true
	if err != nil {
		if !os.IsNotExist(err) {
			return Result{}, fmt.Errorf("read %s: %w", path, err)
		}
		fileExists = false
		existing = nil
	}

	order, values, err := decodeSettings(existing)
	if err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", path, err)
	}

	order, values = setOwnedKeys(order, values, javaHome)

	indentUnit := detectIndent(existing)
	rendered, err := render(order, values, indentUnit)
	if err != nil {
		return Result{}, fmt.Errorf("render %s: %w", path, err)
	}

	if fileExists && bytes.Equal(rendered, existing) {
		return Result{Path: path, Changed: false}, nil
	}

	mode := os.FileMode(settingsFilePermissions)
	if fileExists {
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode()
		}
	}

	if err := writeAtomic(path, rendered, mode); err != nil {
		return Result{}, err
	}

	return Result{Path: path, Changed: true}, nil
}

// validateJavaHome resolves $JAVA_HOME and checks it looks like a JDK
// (contains bin/java). There is no filesystem discovery fallback: an
// unset/empty JAVA_HOME, or one missing bin/java, is a hard error naming
// JAVA_HOME so the caller can fix their environment.
func validateJavaHome() (string, error) {
	home := os.Getenv("JAVA_HOME")
	if home == "" {
		return "", errors.New("JAVA_HOME is not set; the vscode-java subcommand requires JAVA_HOME to point at a JDK installation (no auto-discovery fallback)")
	}

	javaBin := filepath.Join(home, "bin", "java")
	if _, err := os.Stat(javaBin); err != nil {
		return "", fmt.Errorf("JAVA_HOME=%q does not look like a JDK (missing %s): %w", home, javaBin, err)
	}

	return home, nil
}

// writeAtomic writes data to path by staging a temp file in the same
// directory and renaming it into place, so readers never observe a
// partially-written file.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, settingsDirPermissions); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "settings-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpPath, path, err)
	}

	return nil
}

// setOwnedKeys sets (or overwrites in place) the tool's two owned keys.
// A key that already exists keeps its position in order; a new key is
// appended at the end.
func setOwnedKeys(order []string, values map[string]json.RawMessage, javaHome string) ([]string, map[string]json.RawMessage) {
	setKey := func(key string, value any) {
		raw, _ := json.Marshal(value)
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = raw
	}

	setKey(keyJavaHome, javaHome)
	setKey(keyUpdateBuildConfig, valueUpdateBuildConfig)

	return order, values
}
