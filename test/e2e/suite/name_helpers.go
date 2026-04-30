// name_helpers.go builds deterministic, GitLab-safe identifiers for E2E
// resources so parallel runs can isolate their projects, groups, and branches.
package suite

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// stableHashLength is the number of hexadecimal characters kept from SHA-256
// hashes used in generated E2E names.
const stableHashLength = 10

// E2E name generation state shared by all tests in the process.
var (
	e2eRunID      = newE2ERunID(time.Now())
	uniqueCounter atomic.Int64
	unsafeChars   = regexp.MustCompile(`[^a-z0-9-]`)
	runIDCounter  atomic.Int64
)

// configuredE2ERunID returns the sanitized E2E_RUN_ID override or creates a
// fresh timestamped run identifier when no valid override is configured.
func configuredE2ERunID(now time.Time) string {
	if value := os.Getenv("E2E_RUN_ID"); value != "" {
		if runID := sanitizeNamePart(value, 48); runID != "" {
			return runID
		}
	}
	return newE2ERunID(now)
}

// newE2ERunID builds a lowercase, GitLab-safe run identifier from the UTC time,
// process ID, and an atomic counter to avoid collisions in rapid startups.
func newE2ERunID(now time.Time) string {
	source := fmt.Sprintf("%d-%d-%d", now.UnixNano(), os.Getpid(), runIDCounter.Add(1))
	return fmt.Sprintf("%s-%s", now.UTC().Format("20060102t150405z"), shortStableHash(source))
}

// shortStableHash returns a deterministic short hexadecimal SHA-256 prefix for
// value, suitable for compact E2E resource names.
func shortStableHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:stableHashLength]
}

// sanitizeTestName converts a Go test name into the shorter slug segment used
// in GitLab resource names.
func sanitizeTestName(name string) string {
	return sanitizeNamePart(name, 40)
}

// sanitizeNamePrefix normalizes a caller-provided prefix and falls back to
// "e2e" when the sanitized value is empty.
func sanitizeNamePrefix(prefix string) string {
	name := sanitizeNamePart(prefix, 80)
	if name == "" {
		return "e2e"
	}
	return name
}

// sanitizeNamePart lowercases name, removes unsupported characters, trims dash
// boundaries, and optionally truncates the result to maxLength characters.
func sanitizeNamePart(name string, maxLength int) string {
	sanitized := strings.ToLower(name)
	sanitized = strings.ReplaceAll(sanitized, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = unsafeChars.ReplaceAllString(sanitized, "")
	sanitized = strings.Trim(sanitized, "-")
	if maxLength > 0 && len(sanitized) > maxLength {
		sanitized = strings.Trim(sanitized[:maxLength], "-")
	}
	return sanitized
}

// uniqueName returns a process-unique, run-scoped GitLab-safe resource name for
// the supplied prefix.
func uniqueName(prefix string) string {
	sanitizedPrefix := sanitizeNamePrefix(prefix)
	return fmt.Sprintf("%s-%s-%s-%d", sanitizedPrefix, e2eRunID, shortStableHash(sanitizedPrefix), uniqueCounter.Add(1))
}
