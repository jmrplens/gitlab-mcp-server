package suite

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

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

func TestSanitizeTestName_ConvertsGoTestNameToSlug(t *testing.T) {
	got := sanitizeTestName("TestIndividual_Branches/Create With Spaces!")
	want := "testindividual-branches-createwithspaces"
	if got != want {
		t.Fatalf("sanitizeTestName() = %q, want %q", got, want)
	}
}

func TestSanitizeTestName_TruncatesToFortyCharacters(t *testing.T) {
	got := sanitizeTestName(strings.Repeat("a", 80))
	if len(got) != 40 {
		t.Fatalf("sanitized name length = %d, want 40", len(got))
	}
}

func TestNewE2ERunID_UsesUTCStampAndHashSuffix(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 34, 56, 789, time.FixedZone("UTC+2", 2*60*60))
	got := newE2ERunID(now)

	if !regexp.MustCompile(`^20260430t103456z-[a-f0-9]{10}$`).MatchString(got) {
		t.Fatalf("newE2ERunID() = %q, want lowercase UTC timestamp plus 10-char hex suffix", got)
	}
}

func TestConfiguredE2ERunID_UsesEnvironmentOverride(t *testing.T) {
	t.Setenv("E2E_RUN_ID", "Custom_Run/ID!")
	got := configuredE2ERunID(time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC))
	want := "custom-run-id"
	if got != want {
		t.Fatalf("configuredE2ERunID() = %q, want %q", got, want)
	}
}

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
