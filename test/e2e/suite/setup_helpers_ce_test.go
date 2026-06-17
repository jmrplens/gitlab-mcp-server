//go:build e2e && !enterprise

// setup_helpers_ce_test.go verifies E2E name generation helpers without requiring
// a live GitLab instance.
package suite

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestShortStableHash_ReturnsStableLowercaseHex verifies that shortStableHash
// is deterministic, uses the configured hash length, and emits lowercase hex.
//
// The test hashes the same input twice and asserts the outputs are equal,
// the length matches [stableHashLength], and the result consists of lowercase
// hex characters. These properties underpin the per-run resource-name hashing
// used to disambiguate parallel E2E runs.
func TestShortStableHash_ReturnsStableLowercaseHex(t *testing.T) {
	first := shortStableHash("TestIndividual_Branches/Create")
	second := shortStableHash("TestIndividual_Branches/Create")

	if first != second {
		t.Fatalf("hash is not stable: first=%q second=%q", first, second)
	}
	if len(first) != stableHashLength {
		t.Fatalf("hash length = %d, want %d", len(first), stableHashLength)
	}
	if !regexp.MustCompile(`^[a-f0-9]+$`).MatchString(first) {
		t.Fatalf("hash %q is not lowercase hex", first)
	}
}

// TestSanitizeTestName_ConvertsGoTestNameToSlug verifies that Go test names
// are converted into GitLab-safe slug segments.
//
// The test passes a mixed-case test name containing spaces, slashes, and
// underscores and asserts the output is lowercased, separators collapsed to
// dashes, unsupported characters stripped, and boundary dashes trimmed. The
// resulting slug is safe to embed in GitLab project, group, and branch names.
func TestSanitizeTestName_ConvertsGoTestNameToSlug(t *testing.T) {
	got := sanitizeTestName("TestIndividual_Branches/Create With Spaces!")
	want := "testindividual-branches-createwithspaces"
	if got != want {
		t.Fatalf("sanitizeTestName() = %q, want %q", got, want)
	}
}

// TestSanitizeTestName_TruncatesToFortyCharacters verifies that sanitized
// test name segments are capped at 40 characters for compact resource names.
//
// The test feeds an 80-character run of 'a' into sanitizeTestName and
// asserts the result is exactly 40 characters, with any trailing dash
// boundary trimmed by the sanitization pass. The 40-character cap keeps
// resource names compact when concatenated with the run ID and counter.
func TestSanitizeTestName_TruncatesToFortyCharacters(t *testing.T) {
	got := sanitizeTestName(strings.Repeat("a", 80))
	if len(got) != 40 {
		t.Fatalf("sanitized name length = %d, want 40", len(got))
	}
}

// TestNewE2ERunID_UsesUTCStampAndHashSuffix verifies that newE2ERunID encodes
// the UTC timestamp and a stable lowercase hash suffix.
//
// The test seeds a fixed time in a non-UTC zone and asserts the resulting
// run ID is the UTC stamp (Z suffix) plus a 10-character hex hash, even
// when the input time is in another zone. The hash keeps parallel run
// identifiers unique when the clock collides across processes.
func TestNewE2ERunID_UsesUTCStampAndHashSuffix(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 34, 56, 789, time.FixedZone("UTC+2", 2*60*60))
	got := newE2ERunID(now)

	if !regexp.MustCompile(`^20260430t103456z-[a-f0-9]{10}$`).MatchString(got) {
		t.Fatalf("newE2ERunID() = %q, want lowercase UTC timestamp plus 10-char hex suffix", got)
	}
}

// TestConfiguredE2ERunID_UsesEnvironmentOverride verifies that E2E_RUN_ID
// is sanitized and used instead of generating a timestamped run ID.
//
// The test sets E2E_RUN_ID to a mixed-case string with slashes and
// underscores, then asserts configuredE2ERunID returns the sanitized
// version ("custom-run-id"). When the override is missing or sanitizes
// to empty, configuredE2ERunID falls back to [newE2ERunID].
func TestConfiguredE2ERunID_UsesEnvironmentOverride(t *testing.T) {
	t.Setenv("E2E_RUN_ID", "Custom_Run/ID!")
	got := configuredE2ERunID(time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC))
	want := "custom-run-id"
	if got != want {
		t.Fatalf("configuredE2ERunID() = %q, want %q", got, want)
	}
}

// TestUniqueName_IncludesRunIDHashAndCounter verifies that uniqueName
// combines the sanitized prefix, run ID, prefix hash, and monotonically
// increasing count.
//
// The test pins e2eRunID and the atomic counter to known values, restores
// them via [t.Cleanup], and asserts uniqueName produces the expected
// concatenation: sanitized prefix, run ID, prefix hash, and a "-1"
// counter. This format keeps every E2E resource globally unique within a
// run and across runs.
func TestUniqueName_IncludesRunIDHashAndCounter(t *testing.T) {
	originalRunID := e2eRunID
	originalCounter := uniqueCounter.Load()
	e2eRunID = "run-abc123"
	uniqueCounter.Store(0)
	t.Cleanup(func() {
		e2eRunID = originalRunID
		uniqueCounter.Store(originalCounter)
	})

	got := uniqueName("E2E_Project/Test")
	want := "e2e-project-test-run-abc123-" + shortStableHash("e2e-project-test") + "-1"
	if got != want {
		t.Fatalf("uniqueName() = %q, want %q", got, want)
	}
}

// TestUniqueName_UsesDefaultPrefixForEmptyInput verifies that uniqueName
// falls back to the "e2e" prefix when the supplied prefix sanitizes to
// an empty string.
//
// The test pins e2eRunID and the atomic counter to known values, then
// calls uniqueName with an empty string. The sanitizer strips the empty
// input to nothing, and [sanitizeNamePrefix] returns the "e2e" default.
// The resulting name starts with "e2e-run-xyz789-..." and never with an
// empty prefix, so downstream assertions and the resource ledger can
// rely on a non-empty prefix.
func TestUniqueName_UsesDefaultPrefixForEmptyInput(t *testing.T) {
	originalRunID := e2eRunID
	originalCounter := uniqueCounter.Load()
	e2eRunID = "run-xyz789"
	uniqueCounter.Store(0)
	t.Cleanup(func() {
		e2eRunID = originalRunID
		uniqueCounter.Store(originalCounter)
	})

	got := uniqueName("")
	wantPrefix := "e2e-run-xyz789-" + shortStableHash("e2e") + "-1"
	if got != wantPrefix {
		t.Fatalf("uniqueName() = %q, want %q", got, wantPrefix)
	}
}
