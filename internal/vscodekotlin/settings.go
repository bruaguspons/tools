// Package vscodekotlin configures the VS Code Kotlin extension
// (fwcd.kotlin) for the current project by pointing kotlin.java.home in
// ./.vscode/settings.json at the newest installed JDK it can run on.
package vscodekotlin

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bruaguspons/tools/internal/vscodesettings"
)

// Result reports the outcome of a Configure call.
type Result = vscodesettings.Result

const (
	keyJavaHome    = "kotlin.java.home"
	defaultJVMRoot = "/usr/lib/jvm"

	// minSupportedMajor is the oldest JDK major the Kotlin language
	// server can run on: fwcd.kotlin 0.2.36 requires JDK 11 or newer.
	minSupportedMajor = 11

	// maxSupportedMajor is the newest JDK major the Kotlin language
	// server can run on: fwcd.kotlin 0.2.36 bundles kotlin-compiler 2.1.0,
	// whose JavaVersion.parse rejects feature versions >= 25.
	maxSupportedMajor = 24
)

// Configure scans /usr/lib/jvm for JDKs, picks the highest one whose
// major version the Kotlin language server supports (11 to 24), and sets
// kotlin.java.home in dir/.vscode/settings.json to its resolved path.
// $JAVA_HOME is deliberately not consulted. Discovery and selection
// happen before settings.json is read, so a failure leaves it untouched.
func Configure(dir string) (Result, error) {
	return configure(dir, defaultJVMRoot)
}

// configure is Configure with an injectable JDK scan root for tests.
func configure(dir, root string) (Result, error) {
	cands, seen, skips, err := discover(root)
	if err != nil {
		return Result{}, fmt.Errorf("scan %s for JDKs: %w", root, err)
	}

	selected, ok := selectJDK(cands, minSupportedMajor, maxSupportedMajor)
	if !ok {
		found := "none"
		if len(seen) > 0 {
			found = strings.Join(seen, ", ")
		}
		msg := fmt.Sprintf("no JDK with major version in [%d, %d] found under %s (fwcd.kotlin 0.2.36 requires JDK %d to %d); found: %s",
			minSupportedMajor, maxSupportedMajor, root, minSupportedMajor, maxSupportedMajor, found)
		if len(skips) > 0 {
			parts := make([]string, len(skips))
			for i, sk := range skips {
				parts[i] = fmt.Sprintf("%s (%s)", sk.name, sk.reason)
			}
			msg += "; skipped: " + strings.Join(parts, ", ")
		}
		return Result{}, errors.New(msg)
	}

	value, _ := json.Marshal(selected.path)
	return vscodesettings.Merge(dir, func(order []string, values map[string]json.RawMessage) ([]string, map[string]json.RawMessage, error) {
		vscodesettings.SetField(&order, values, keyJavaHome, value)
		return order, values, nil
	})
}
