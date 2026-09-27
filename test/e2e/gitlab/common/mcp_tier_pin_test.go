//go:build e2e

// mcp_tier_pin_test.go drives GITLAB_MCP_TIER, the one setting that changes
// which actions exist without changing anything about the instance.
//
// mcp_tiers_test.go covers the tier this runtime detects. What it cannot cover
// is the pin, and the pin is what a licensed deployment behind a
// non-administrator token depends on: GET /license answers administrators
// only, so such a token reads no license and is served the Free catalog
// however the instance is licensed. An operator's only remedy is to say the
// tier, and nothing asserted that saying it works.
//
// The catalogs are nested rather than disjoint (Free < Premium < Ultimate), so
// the assertion is growth: pinning higher may never take an action away, and
// each licensed pin must add at least one, or the pin bought nothing.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestTierPin_ServesTheCatalogOfThePinnedTier starts one session per pin and
// holds each to the tier it asked for and to the catalog that tier implies.
//
// Every pin runs on this runtime whatever it is licensed for, which is the
// point: the catalog is assembled from the tier the server resolved, not from
// anything GitLab is asked. A licensed action is registered and callable; what
// GitLab answers when it is called is a different question, and this does not
// call one.
func TestTierPin_ServesTheCatalogOfThePinnedTier(t *testing.T) {
	e := harness.New(t)

	want := map[harness.TierPin]edition.Tier{
		harness.TierFree:     edition.Free,
		harness.TierPremium:  edition.Premium,
		harness.TierUltimate: edition.Ultimate,
	}

	counts := make(map[harness.TierPin]int, len(harness.AllTierPins()))
	for _, pin := range harness.AllTierPins() {
		t.Run(pin.String(), func(t *testing.T) {
			s := e.Session(harness.ServerConfig{Surface: harness.SurfaceDynamic, Tier: pin})

			if got := s.Tier(); got != want[pin] {
				t.Fatalf("a session pinned to %q serves tier %s, want %s", pin, got, want[pin])
			}
			counts[pin] = len(s.Actions())
			if counts[pin] == 0 {
				t.Fatalf("a session pinned to %q reaches no action at all", pin)
			}
		})
	}

	// Nested, not disjoint: a higher tier is a superset, so the counts may
	// never fall and each licensed pin has to add something. A pin that
	// resolved to the runtime's own tier instead of the one asked for would
	// show up here as three equal numbers.
	if counts[harness.TierPremium] <= counts[harness.TierFree] {
		t.Errorf("premium reaches %d actions and free reaches %d, want premium to add some",
			counts[harness.TierPremium], counts[harness.TierFree])
	}
	if counts[harness.TierUltimate] <= counts[harness.TierPremium] {
		t.Errorf("ultimate reaches %d actions and premium reaches %d, want ultimate to add some",
			counts[harness.TierUltimate], counts[harness.TierPremium])
	}
}

// TestTierPin_DetectionAgreesWithTheRuntimeProbe holds the tier a detecting
// session resolved against what the harness itself found on the instance, on
// a licensed instance and on a Community Edition one alike.
//
// The two answers come from different places and have to agree: the harness
// probes once at bootstrap with the run's own token, and the child reads GET
// /license for itself at startup. A disagreement would mean the catalog the
// suite expects is not the catalog the binary built, and every tier assertion
// in this package would be measuring the wrong server.
//
// It is asserted wherever the probe's tier is a reading rather than a guess,
// which is two cases. A license the probe read is one. A Community Edition
// image is the other: it cannot hold a license, so Free is the only tier it
// has whatever the token may read, and both the probe and the binary have to
// say Free. This is the case the unlicensed Docker run is, and it used to be
// skipped along with the next one, so the CE run held the server's tier to
// nothing at all.
//
// What is left is an Enterprise image whose license the token did not read.
// That endpoint answers administrators only, so a non-administrator token
// reads no license and falls back to Free, and so does an administrator's on
// an image nobody licensed; the harness cannot tell those apart, and comparing
// the fallback would be comparing two guesses rather than two readings. No
// Docker run is that case: the licensed one is licensed and the unlicensed one
// is Community Edition.
func TestTierPin_DetectionAgreesWithTheRuntimeProbe(t *testing.T) {
	e := harness.New(t)
	runtime := e.Runtime()
	want := runtime.Tier
	switch {
	case runtime.TierConfirmed:
		// The license the probe read is the answer the binary must give.
	case !runtime.Enterprise:
		if runtime.Tier != edition.Free {
			t.Errorf("the probe read tier %s on a Community Edition image, which can hold no license", runtime.Tier)
		}
		want = edition.Free
	default:
		e.Skipf("the run's token read no license on an Enterprise image, so the probe's tier (%s) is the fallback "+
			"rather than a reading", runtime.Tier)
	}

	if got := e.On(harness.SurfaceDynamic).Tier(); got != want {
		t.Errorf("a detecting session serves tier %s and the instance is %s: the suite and the binary "+
			"disagree about which catalog this runtime has", got, want)
	}
}

// TestTierPin_Absent_LetsTheChildDetect checks that the zero value is
// detection rather than a tier of its own.
//
// TierDetect is the empty pin, so this is the ordinary session every other
// test in the suite starts, asserted once: the server reads the license itself
// and the harness reports what it found. Getting this wrong in the other
// direction would be silent, since edition.Free is also the zero value of the
// tier type, and a pin that resolved to Free would look like a detection that
// found nothing.
func TestTierPin_Absent_LetsTheChildDetect(t *testing.T) {
	e := harness.New(t)

	detecting := e.Session(harness.ServerConfig{Surface: harness.SurfaceDynamic, Tier: harness.TierDetect})
	ordinary := e.On(harness.SurfaceDynamic)

	if detecting.Tier() != ordinary.Tier() {
		t.Errorf("an explicitly undetermined pin resolved to %s and the default session to %s",
			detecting.Tier(), ordinary.Tier())
	}
	if detecting.Label() != ordinary.Label() {
		t.Errorf("the two sessions have labels %q and %q, want one session: TierDetect is the absence of a pin",
			detecting.Label(), ordinary.Label())
	}
}
