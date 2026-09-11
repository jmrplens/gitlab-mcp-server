//go:build e2e

// names_test.go covers the name generation every fixture stands on. These are
// ported from the suite this replaces (setup_helpers_ce_test.go), with the
// package name the new run identifier carries added to each expectation.

package harness

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// hexOnly matches a lowercase hexadecimal string with nothing else in it.
var hexOnly = regexp.MustCompile(`^[a-f0-9]+$`)

// TestShortStableHash_SameInput_ReturnsStableLowercaseHex checks that the hash
// behind every generated name is deterministic, the configured length, and
// legal in a GitLab path.
//
// It hashes one value twice and asserts the two agree, that the result is
// stableHashLength characters and that every character is lowercase hex. All
// three matter: a hash that changed between calls would make one test's
// resources unfindable by its own cleanup, and an upper-case or symbol
// character would be refused by GitLab when the name is used as a path.
func TestShortStableHash_SameInput_ReturnsStableLowercaseHex(t *testing.T) {
	first := shortStableHash("TestCommon_Branches/Create")
	second := shortStableHash("TestCommon_Branches/Create")

	if first != second {
		t.Fatalf("hash is not stable: first=%q second=%q", first, second)
	}
	if len(first) != stableHashLength {
		t.Fatalf("hash length = %d, want %d", len(first), stableHashLength)
	}
	if !hexOnly.MatchString(first) {
		t.Fatalf("hash %q is not lowercase hex", first)
	}
}

// TestSanitizeTestName_MixedCaseWithSeparators_ReturnsSlug checks that a Go
// test name becomes a slug GitLab accepts as a path segment.
//
// The input carries every shape a test name has: upper case, an underscore, a
// subtest slash, spaces and punctuation. The expectation is one lowercase
// string whose separators have become dashes and whose punctuation is gone,
// which is what makes the name usable as a project path.
func TestSanitizeTestName_MixedCaseWithSeparators_ReturnsSlug(t *testing.T) {
	got := sanitizeTestName("TestCommon_Branches/Create With Spaces!")
	want := "testcommon-branches-createwithspaces"
	if got != want {
		t.Fatalf("sanitizeTestName() = %q, want %q", got, want)
	}
}

// TestSanitizeTestName_LongName_TruncatesToFortyCharacters checks the cap that
// leaves room for the run identifier beside the test name.
//
// It feeds eighty characters in and expects forty out. GitLab's path limit is
// what this protects: the test slug is only one of four parts of a generated
// name, and an uncapped one would push the rest past the limit.
func TestSanitizeTestName_LongName_TruncatesToFortyCharacters(t *testing.T) {
	got := sanitizeTestName(strings.Repeat("a", 80))
	if len(got) != 40 {
		t.Fatalf("sanitized length = %d, want 40", len(got))
	}
}

// TestNewRunID_NonUTCClock_UsesUTCStampHashAndPackage checks the shape of a
// generated run identifier.
//
// The clock is deliberately in another zone: the identifier is compared across
// machines, so it stamps UTC whatever the machine is set to. The package name
// is part of it because the three e2e packages run against one instance one
// after another, and the sweep that deletes leftovers has to be able to tell
// whose they are.
func TestNewRunID_NonUTCClock_UsesUTCStampHashAndPackage(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 34, 56, 789, time.FixedZone("UTC+2", 2*60*60))

	got := newRunID(now, "common")

	if !regexp.MustCompile(`^20260430t103456z-[a-f0-9]{10}-common$`).MatchString(got) {
		t.Fatalf("newRunID() = %q, want a UTC stamp, a 10-character hash and the package", got)
	}
}

// TestConfiguredRunID_Override_SanitizesAndKeepsPackage checks that an
// operator-supplied identifier is used, sanitized, and still names the
// package.
//
// The package is appended to an override too, and that is the point of the
// case: one E2E_RUN_ID is exported for a whole run and reaches all three
// packages, so without the suffix they would name their resources identically
// and delete each other's.
func TestConfiguredRunID_Override_SanitizesAndKeepsPackage(t *testing.T) {
	got := configuredRunID(time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC), "Custom_Run/ID!", "ee")
	want := "custom-run-id-ee"
	if got != want {
		t.Fatalf("configuredRunID() = %q, want %q", got, want)
	}
}

// TestConfiguredRunID_NoOverride_GeneratesOne checks the fallback: an override
// that sanitizes to nothing is not an identifier, and generating one is better
// than running with an empty scope that matches every other run.
func TestConfiguredRunID_NoOverride_GeneratesOne(t *testing.T) {
	got := configuredRunID(time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC), "!!!", "ce")

	if !strings.HasSuffix(got, "-ce") || !strings.HasPrefix(got, "20260430t120000z-") {
		t.Fatalf("configuredRunID() = %q, want a generated identifier ending in the package name", got)
	}
}

// TestUniqueName_Prefix_CombinesRunIDHashAndCounter checks the whole name a
// fixture is created under.
//
// The counter is what separates two resources of one test, and the run
// identifier is what separates two runs. The assertion pins the exact shape
// because the sweep matches on it.
func TestUniqueName_Prefix_CombinesRunIDHashAndCounter(t *testing.T) {
	before := nameCounter.Load()
	t.Cleanup(func() { nameCounter.Store(before) })
	nameCounter.Store(0)

	got := uniqueName("run-abc123", "E2E_Project/Test")

	want := "e2e-project-test-run-abc123-" + shortStableHash("e2e-project-test") + "-1"
	if got != want {
		t.Fatalf("uniqueName() = %q, want %q", got, want)
	}
}

// TestUniqueName_EmptyPrefix_UsesTheDefaultPrefix checks that a prefix which
// sanitizes to nothing still produces a name.
//
// A name beginning with a dash, or with nothing, is refused by GitLab and
// would also break the prefix the orphan sweep matches on, so the empty case
// falls back to "e2e" rather than producing one.
func TestUniqueName_EmptyPrefix_UsesTheDefaultPrefix(t *testing.T) {
	before := nameCounter.Load()
	t.Cleanup(func() { nameCounter.Store(before) })
	nameCounter.Store(0)

	got := uniqueName("run-xyz789", "")

	want := "e2e-run-xyz789-" + shortStableHash("e2e") + "-1"
	if got != want {
		t.Fatalf("uniqueName() = %q, want %q", got, want)
	}
}
