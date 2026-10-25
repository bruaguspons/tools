// Package jdk reads metadata from installed JDK directories.
package jdk

import (
	"errors"
	"strings"
)

// ParseJavaVersion extracts the JAVA_VERSION value from a JDK's release
// file (lines of the form KEY="VALUE").
func ParseJavaVersion(data []byte) (string, error) {
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
