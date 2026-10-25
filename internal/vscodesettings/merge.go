// Package vscodesettings merges owned keys into a project's
// ./.vscode/settings.json: strict-JSON, order- and indent-preserving,
// written atomically and skipped entirely when nothing changes. It is the
// shared engine behind the vscode-* subcommands.
package vscodesettings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	defaultIndent   = "    " // 4 spaces, VS Code's own default
	settingsRelPath = ".vscode/settings.json"
)

// Result reports the outcome of a Merge call.
type Result struct {
	// Path is the absolute path to the settings.json file that was read
	// (and possibly written).
	Path string
	// Changed is true if settings.json was created or its content changed.
	Changed bool
}

// Mutator applies a subcommand's owned-key changes to the decoded
// settings: the top-level key order and each key's raw value. It returns
// the updated pair, or an error that Merge reports as an update failure.
type Mutator func(order []string, values map[string]json.RawMessage) ([]string, map[string]json.RawMessage, error)

// resolveTarget returns the file Merge reads and writes, and whether it
// exists. A symlinked settings.json resolves to its target so the link
// itself survives the atomic rename; a dangling link is an error rather
// than a silent create at an arbitrary link target.
func resolveTarget(path string) (target string, exists bool, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return path, false, nil
		}
		return "", false, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, true, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, fmt.Errorf("%s is a dangling symlink: %w", path, err)
		}
		return "", false, fmt.Errorf("resolve %s: %w", path, err)
	}
	return resolved, true, nil
}

// Merge reads dir/.vscode/settings.json (an absent file is an empty
// object), applies mutate, and writes the result atomically, creating
// the file and directory if needed. Unrelated keys and their original
// order are preserved. If settings.json is a symlink, the link's target
// is read and replaced and the link is left intact. If the rendered
// content is byte-identical to what is already on disk, no write occurs
// and Result.Changed is false.
func Merge(dir string, mutate Mutator) (Result, error) {
	path := filepath.Join(dir, settingsRelPath)

	target, fileExists, err := resolveTarget(path)
	if err != nil {
		return Result{}, err
	}

	var existing []byte
	if fileExists {
		existing, err = os.ReadFile(target)
		if err != nil {
			return Result{}, fmt.Errorf("read %s: %w", path, err)
		}
	}

	order, values, err := decodeSettings(existing)
	if err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", path, err)
	}

	order, values, err = mutate(order, values)
	if err != nil {
		return Result{}, fmt.Errorf("update %s: %w", path, err)
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
		if info, statErr := os.Stat(target); statErr == nil {
			mode = info.Mode().Perm()
		}
	}

	if err := writeAtomic(target, rendered, mode); err != nil {
		return Result{}, err
	}

	return Result{Path: path, Changed: true}, nil
}

// SetField sets a field on an ordered object, appending it to order only
// if it wasn't already present (so an existing field keeps its position).
func SetField(order *[]string, fields map[string]json.RawMessage, key string, value json.RawMessage) {
	if _, exists := fields[key]; !exists {
		*order = append(*order, key)
	}
	fields[key] = value
}
