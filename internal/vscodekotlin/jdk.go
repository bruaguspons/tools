package vscodekotlin

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bruaguspons/tools/internal/jdk"
)

// candidate is one installed JDK found under the scan root.
type candidate struct {
	path    string // fully symlink-resolved directory
	raw     string // JAVA_VERSION as written in the release file
	version []int  // numeric components of raw, legacy "1." prefix dropped
}

// parseVersion turns a JAVA_VERSION string into numeric components. It
// splits on '.' and '_' and strips each field at its first non-digit
// ("25-ea" -> [25], "21.0.1+12" -> [21 0 1]); a field that ends up empty
// stops parsing and keeps the prefix. The first field must be numeric.
// Legacy "1.x" versions drop the leading 1 ("1.8.0_292" -> [8 0 292]).
func parseVersion(s string) ([]int, error) {
	var version []int
	for i, field := range strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '_' }) {
		end := 0
		for end < len(field) && field[end] >= '0' && field[end] <= '9' {
			end++
		}
		if end == 0 {
			if i == 0 {
				return nil, fmt.Errorf("leading version field %q is not numeric", field)
			}
			break
		}
		n, err := strconv.Atoi(field[:end])
		if err != nil {
			return nil, fmt.Errorf("version field %q: %w", field, err)
		}
		version = append(version, n)
		if end < len(field) {
			break // suffix such as "-ea" or "+12" ends the version
		}
	}
	if len(version) == 0 {
		return nil, errors.New("version has no leading numeric field")
	}
	if version[0] == 1 && len(version) >= 2 {
		version = version[1:]
	}
	return version, nil
}

// compareVersions compares two versions component-wise, treating missing
// components as 0, and returns -1, 0 or 1.
func compareVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

// skipped is a JDK-looking directory under the scan root that discover
// rejected, with the reason, so the no-JDK error can explain itself.
type skipped struct {
	name   string // entry name under the scan root
	reason string
}

// discover scans the direct children of root for JDKs. A child counts
// only if, after symlink resolution, it is a directory holding bin/java
// and a release file with a parseable JAVA_VERSION. Entries that look
// like a JDK (have bin/java or a release file) but fail those checks are
// returned as skipped with a reason; anything else (plain files, dangling
// symlinks, symlink duplicates, unrelated directories) is ignored
// silently. Candidates are deduplicated by resolved path. It also returns
// the raw JAVA_VERSION of every candidate, in discovery order.
func discover(root string) ([]candidate, []string, []skipped, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, nil, err
	}

	var cands []candidate
	var seen []string
	var skips []skipped
	resolvedSeen := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		resolved, err := filepath.EvalSymlinks(filepath.Join(root, name))
		if err != nil || resolvedSeen[resolved] {
			continue
		}
		resolvedSeen[resolved] = true
		if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
			continue
		}
		hasJava := fileExists(filepath.Join(resolved, "bin", "java"))
		release, relErr := os.ReadFile(filepath.Join(resolved, "release"))
		if !hasJava && errors.Is(relErr, fs.ErrNotExist) {
			continue // not JDK-like
		}
		skip := func(reason string) {
			skips = append(skips, skipped{name: name, reason: reason})
		}
		if !hasJava {
			skip("missing bin/java")
			continue
		}
		if relErr != nil {
			if errors.Is(relErr, fs.ErrNotExist) {
				skip("missing release file")
			} else {
				skip(fmt.Sprintf("read release: %v", relErr))
			}
			continue
		}
		raw, err := jdk.ParseJavaVersion(release)
		if err != nil {
			skip(fmt.Sprintf("release: %v", err))
			continue
		}
		version, err := parseVersion(raw)
		if err != nil {
			skip(fmt.Sprintf("JAVA_VERSION %q: %v", raw, err))
			continue
		}
		cands = append(cands, candidate{path: resolved, raw: raw, version: version})
		seen = append(seen, raw)
	}
	return cands, seen, skips, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// selectJDK picks the candidate with the highest version whose major is
// within [minMajor, maxMajor]. Equal versions fall back to the
// lexicographically smallest resolved path so the choice is
// deterministic.
func selectJDK(cands []candidate, minMajor, maxMajor int) (candidate, bool) {
	var eligible []candidate
	for _, c := range cands {
		if c.version[0] >= minMajor && c.version[0] <= maxMajor {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return candidate{}, false
	}
	sort.Slice(eligible, func(i, j int) bool {
		if cmp := compareVersions(eligible[i].version, eligible[j].version); cmp != 0 {
			return cmp > 0
		}
		return eligible[i].path < eligible[j].path
	})
	return eligible[0], true
}
