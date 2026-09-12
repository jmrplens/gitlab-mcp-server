//go:build e2e

// names.go gives every GitLab object a test creates a name that says which
// run, which package and which test made it.
//
// Names are not cosmetic here. The sweep that deletes what a failed run left
// behind matches on the run ID, so a fixed name would make one run delete
// another run's fixtures, and a name without the package would make the three
// packages collide on an instance they share.

package harness

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

// stableHashLength is how many hexadecimal characters of a SHA-256 sum a
// generated name carries. Ten is short enough to leave room inside GitLab's
// path limits and long enough that two names minted in one afternoon do not
// collide.
const stableHashLength = 10

// runIDStampLayout is the UTC timestamp a run ID opens with, lowercased so the
// whole identifier is a legal GitLab path segment.
const runIDStampLayout = "20060102t150405z"

// unsafeChars matches everything GitLab refuses in a path segment, once the
// name has been lowercased and its separators turned into dashes.
var unsafeChars = regexp.MustCompile(`[^a-z0-9-]`)

// runIDCounter disambiguates two run IDs minted in the same nanosecond by one
// process, which a package that computes its identifier at startup can do.
var runIDCounter atomic.Int64

// nameCounter makes every name one process hands out distinct from the others
// even when their prefixes and their run ID are identical.
var nameCounter atomic.Int64

// shortStableHash returns a deterministic lowercase hexadecimal prefix of the
// SHA-256 sum of value.
func shortStableHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:stableHashLength]
}

// sanitizeNamePart lowercases name, turns its separators into dashes, drops
// everything GitLab refuses, trims dash boundaries and truncates the result to
// maxLength characters. A maxLength of zero or less truncates nothing.
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

// sanitizeTestName converts a Go test name into the slug segment a resource
// name carries, short enough to leave room for the run ID beside it.
func sanitizeTestName(name string) string {
	return sanitizeNamePart(name, 40)
}

// sanitizeNamePrefix normalizes a caller's prefix and falls back to "e2e" when
// nothing legal survives the sanitization.
func sanitizeNamePrefix(prefix string) string {
	name := sanitizeNamePart(prefix, 80)
	if name == "" {
		return "e2e"
	}
	return name
}

// newRunID builds the identifier one package's run is known by: a UTC stamp, a
// hash that separates two runs started in the same second, and the package
// name.
//
// The package is part of it because the three packages run against one
// instance, one after another, and the sweep has to be able to tell whose
// leftovers it is looking at.
func newRunID(now time.Time, pkg string) string {
	source := fmt.Sprintf("%d-%d-%d", now.UnixNano(), os.Getpid(), runIDCounter.Add(1))
	return withPackage(now.UTC().Format(runIDStampLayout)+"-"+shortStableHash(source), pkg)
}

// configuredRunID returns the sanitized override when one was given, and a
// fresh identifier otherwise. The package name is appended either way: an
// override set once for a whole run reaches every package, and without it the
// three would name their resources identically.
func configuredRunID(now time.Time, override, pkg string) string {
	if runID := sanitizeNamePart(override, 48); runID != "" {
		return withPackage(runID, pkg)
	}
	return newRunID(now, pkg)
}

// withPackage appends the package name to a run identifier, leaving it alone
// when the caller could not name a package.
func withPackage(runID, pkg string) string {
	if name := sanitizeNamePart(pkg, 24); name != "" {
		return runID + "-" + name
	}
	return runID
}

// uniqueName returns a GitLab-safe resource name scoped to runID: the caller's
// prefix, the run, a hash of the prefix and a counter no two calls share.
func uniqueName(runID, prefix string) string {
	sanitized := sanitizeNamePrefix(prefix)
	return fmt.Sprintf("%s-%s-%s-%d", sanitized, runID, shortStableHash(sanitized), nameCounter.Add(1))
}
