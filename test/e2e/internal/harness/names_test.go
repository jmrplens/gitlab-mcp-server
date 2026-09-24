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

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
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

// TestSanitizeNamePart_NoMaxLength_TruncatesNothing checks the documented
// meaning of a zero cap: no cap at all, rather than a cap of nothing, which
// would turn every name built with it into the empty string.
func TestSanitizeNamePart_NoMaxLength_TruncatesNothing(t *testing.T) {
	if got := sanitizeNamePart("Some_Long/Name", 0); got != "some-long-name" {
		t.Errorf("sanitizeNamePart(0) = %q, want the whole name sanitized", got)
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

// TestNewRunID_Stamp_IsTheOneTheCoverageRecordReadsBack ties the identifier
// this package mints to the parse that dates the committed coverage record.
//
// The two sides are in different build worlds -- this one is behind the e2e
// tag and cmd/audit_e2e_coverage is not -- and the layout used to be spelled
// once on each side with nothing holding them together. The committed shard
// fixtures could not: they are hand-written files carrying invented
// identifiers, so a layout change here would have left every test green and
// been found by the first hour-long Docker run, whose entries would have had
// no date at all. Both sides now read e2ecalls.RunIDStampLayout, and this is
// the test that says so.
func TestNewRunID_Stamp_IsTheOneTheCoverageRecordReadsBack(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 34, 56, 789, time.FixedZone("UTC+2", 2*60*60))

	runID := newRunID(now, "common")
	at, read := e2ecalls.RunIDDate(runID)

	if !read {
		t.Fatalf("e2ecalls.RunIDDate(%q) read no stamp off an identifier this package minted", runID)
	}
	if want := now.UTC().Truncate(time.Second); !at.Equal(want) {
		t.Errorf("e2ecalls.RunIDDate(%q) = %s, want %s", runID, at, want)
	}
}

// TestMintedRunStart_Names_FindTheRunWhereverItSits checks the shape the
// orphan sweep finds a run by when it does not know the run: every name a
// builder hands out, the World's names and a path through a World group all
// carry the identifier somewhere after a separator, and each gives back the
// second the run started.
//
// The identifier is minted rather than written out, so a change to how
// newRunID spells one that the shape does not follow fails here rather than on
// an instance whose leftovers the sweep can no longer see.
func TestMintedRunStart_Names_FindTheRunWhereverItSits(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 15, 0, 999, time.FixedZone("UTC+2", 2*60*60))
	want := now.UTC().Truncate(time.Second)
	runID := newRunID(now, "common")

	cases := map[string]string{
		"a builder's name":         uniqueName(runID, "proj-TestX"),
		"a World name":             "world-snippet-" + runID,
		"a path through the World": "e2e-world-group-" + runID + "/e2e-world-project-" + runID,
		"the identifier alone":     runID,
		"a package with a dash":    newRunID(now, "gitlab-common"),
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			got, found := MintedRunStart(s)
			if !found || !got.Equal(want) {
				t.Errorf("MintedRunStart(%q) = %s, %t; want %s and true", s, got, found, want)
			}
		})
	}
}

// TestMintedRunStart_OtherShapes_FindNoRun checks what the orphan sweep must
// leave alone for want of a run it can date: a run whose identifier
// E2E_RUN_ID replaced, a person's object, and the near misses of the shape,
// each of which is one character away from an identifier this package mints.
// A near miss the shape accepted would put an object nobody can date in front
// of a sweep that deletes by date.
func TestMintedRunStart_OtherShapes_FindNoRun(t *testing.T) {
	const stamp, hash = "20260912t101500z", "0123456789"
	cases := map[string]string{
		"an overridden run":         "proj-" + configuredRunID(time.Now(), "nightly", "common") + "-abc-1",
		"a person's project":        "user/real-work",
		"nothing at all":            "",
		"a stamp run into a word":   "proj" + stamp + "-" + hash + "-common",
		"a stamp run into a number": "proj-1" + stamp + "-" + hash + "-common",
		"a short hash":              "proj-" + stamp + "-" + hash[:9] + "-common",
		"a hash that is not hex":    "proj-" + stamp + "-" + "012345678g" + "-common",
		"an upper-case hash":        "proj-" + stamp + "-" + "ABCDEF0123" + "-common",
		"no package after the hash": "proj-" + stamp + "-" + hash,
		"a dash for a package":      "proj-" + stamp + "-" + hash + "--common",
		"a stamp missing its zone":  "proj-20260912t101500-" + hash + "-common",
		"a month that is not one":   "proj-20261312t101500z-" + hash + "-common",
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if got, found := MintedRunStart(s); found || !got.IsZero() {
				t.Errorf("MintedRunStart(%q) = %s, %t; want no run", s, got, found)
			}
		})
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

// TestWithPackage_NoLegalPackageName_LeavesTheRunIDAlone checks the one
// fallback of the package suffix: a package name with nothing legal in it
// adds no dash and no empty part, so a name built on the identifier stays one
// GitLab accepts.
func TestWithPackage_NoLegalPackageName_LeavesTheRunIDAlone(t *testing.T) {
	if got := withPackage("20260430t120000z-0123456789", "!!!"); got != "20260430t120000z-0123456789" {
		t.Errorf("withPackage() = %q, want the run ID unchanged", got)
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
