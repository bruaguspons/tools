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
	"strconv"
	"strings"
)

// Result reports the outcome of a Configure call.
type Result struct {
	// Path is the absolute path to the settings.json file that was read
	// (and possibly written).
	Path string
	// Changed is true if settings.json was created or its content changed.
	Changed bool
}

// Owned keys: the tool is authoritative for two scalar settings
// (keyJavaHome, keyUpdateBuildConfig), which are overwritten outright,
// plus keyRuntimes, which is merged instead — only the array element
// matching the detected JDK's name is updated (or a new one appended),
// and any other element's "default" is cleared so at most one runtime
// stays default, per the extension's schema. Every other key in
// settings.json is preserved untouched.
const (
	keyJavaHome             = "java.jdt.ls.java.home"
	keyUpdateBuildConfig    = "java.configuration.updateBuildConfiguration"
	valueUpdateBuildConfig  = "automatic"
	keyRuntimes             = "java.configuration.runtimes"
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

	runtime, err := runtimeName(javaHome)
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

	order, values, err = setRuntimesKey(order, values, runtime, javaHome)
	if err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", path, err)
	}

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

// runtimeName derives the JavaSE-* runtime name VS Code Java expects
// from $JAVA_HOME/release's JAVA_VERSION field. There is no fallback:
// a missing/unreadable release file, a missing JAVA_VERSION line, or an
// unparseable version string is a hard error naming the offending path,
// mirroring validateJavaHome's philosophy so a misconfigured JDK is
// caught before any settings.json read/write.
func runtimeName(javaHome string) (string, error) {
	releasePath := filepath.Join(javaHome, "release")

	data, err := os.ReadFile(releasePath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", releasePath, err)
	}

	version, err := parseJavaVersion(data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", releasePath, err)
	}

	name, err := javaSEName(version)
	if err != nil {
		return "", fmt.Errorf("%s: JAVA_VERSION=%q: %w", releasePath, version, err)
	}

	return name, nil
}

// parseJavaVersion extracts the JAVA_VERSION value from a JDK's release
// file (lines of the form KEY="VALUE").
func parseJavaVersion(data []byte) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "JAVA_VERSION=") {
			continue
		}
		value := strings.Trim(strings.TrimPrefix(line, "JAVA_VERSION="), `"`)
		if value == "" {
			return "", errors.New("JAVA_VERSION is empty")
		}
		return value, nil
	}
	return "", errors.New("no JAVA_VERSION line found")
}

// javaSEName maps a JAVA_VERSION string to the JavaSE-* name the
// extension's package.json enumerates: legacy versions ("1.8.0_292")
// become "JavaSE-" + the second segment ("JavaSE-1.8"); modern versions
// ("21.0.1", "9.0.1") become "JavaSE-" + the first segment.
func javaSEName(version string) (string, error) {
	segments := strings.Split(version, ".")
	first := segments[0]
	if first == "" {
		return "", errors.New("version has no leading numeric segment")
	}
	if _, err := strconv.Atoi(first); err != nil {
		return "", fmt.Errorf("leading version segment %q is not numeric", first)
	}

	if first == "1" {
		if len(segments) < 2 || segments[1] == "" {
			return "", errors.New("legacy 1.x version is missing its minor segment")
		}
		if _, err := strconv.Atoi(segments[1]); err != nil {
			return "", fmt.Errorf("legacy minor version segment %q is not numeric", segments[1])
		}
		return "JavaSE-1." + segments[1], nil
	}

	return "JavaSE-" + first, nil
}

// setRuntimesKey merges our JDK into keyRuntimes: the element (if any)
// whose "name" matches ours is updated in place, otherwise a new element
// is appended. A key that already exists keeps its top-level position;
// a new key is appended at the end, matching setOwnedKeys.
func setRuntimesKey(order []string, values map[string]json.RawMessage, name, javaHome string) ([]string, map[string]json.RawMessage, error) {
	raw, exists := values[keyRuntimes]

	var elements []json.RawMessage
	if exists {
		var err error
		elements, err = decodeOrderedArray(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", keyRuntimes, err)
		}
	}

	merged, err := mergeRuntimes(elements, name, javaHome)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", keyRuntimes, err)
	}

	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal %s: %w", keyRuntimes, err)
	}

	if !exists {
		order = append(order, keyRuntimes)
	}
	values[keyRuntimes] = encoded

	return order, values, nil
}

// mergeRuntimes applies the java.configuration.runtimes merge rules: the
// element matching name gets path/default set (its other fields and key
// order are left untouched), every other element with "default": true is
// cleared to false since the extension allows only one default runtime,
// and if no element matches, a fresh one is appended.
func mergeRuntimes(elements []json.RawMessage, name, javaHome string) ([]json.RawMessage, error) {
	pathRaw, _ := json.Marshal(javaHome)
	trueRaw, _ := json.Marshal(true)
	falseRaw, _ := json.Marshal(false)

	found := false
	result := make([]json.RawMessage, len(elements))

	for i, elem := range elements {
		order, fields, err := decodeOrderedObject(elem, bytes.TrimSpace(elem))
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}

		nameRaw, ok := fields["name"]
		if !ok {
			return nil, fmt.Errorf("element %d: missing \"name\" field", i)
		}
		var elemName string
		if err := json.Unmarshal(nameRaw, &elemName); err != nil {
			return nil, fmt.Errorf("element %d: \"name\" field: %w", i, err)
		}

		switch {
		case elemName == name:
			found = true
			setField(&order, fields, "path", pathRaw)
			setField(&order, fields, "default", trueRaw)
		case fields["default"] != nil:
			fields["default"] = falseRaw
		}

		encoded, err := encodeOrderedObject(order, fields)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		result[i] = encoded
	}

	if !found {
		nameRaw, _ := json.Marshal(name)
		newOrder := []string{"name", "path", "default"}
		newFields := map[string]json.RawMessage{
			"name":    nameRaw,
			"path":    pathRaw,
			"default": trueRaw,
		}
		encoded, err := encodeOrderedObject(newOrder, newFields)
		if err != nil {
			return nil, err
		}
		result = append(result, encoded)
	}

	return result, nil
}

// setField sets a field on an ordered object, appending it to order only
// if it wasn't already present (so an existing field keeps its position).
func setField(order *[]string, fields map[string]json.RawMessage, key string, value json.RawMessage) {
	if _, exists := fields[key]; !exists {
		*order = append(*order, key)
	}
	fields[key] = value
}
