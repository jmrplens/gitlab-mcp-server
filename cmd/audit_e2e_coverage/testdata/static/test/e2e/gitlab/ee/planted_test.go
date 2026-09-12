//go:build e2e

package ee

import (
	"testing"

	"example.com/e2efake/internal/edition"
	"example.com/e2efake/test/e2e/internal/harness"
)

// TestPlanted_UltimateWithoutNeeds_Reported names an Ultimate action and
// declares no tier.
func TestPlanted_UltimateWithoutNeeds_Reported(t *testing.T) {
	s := harness.New(t).Session()
	harness.DoVoid(s, "vulnerability.list", nil)
}

// TestPlanted_UltimateWithNeeds_Clean declares the tier itself.
func TestPlanted_UltimateWithNeeds_Clean(t *testing.T) {
	s := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate))).Session()
	harness.DoVoid(s, "vulnerability.get", nil)
}

// TestPlanted_UltimateViaHelper_Clean declares the tier through a helper it
// calls, and names the action through another.
func TestPlanted_UltimateViaHelper_Clean(t *testing.T) {
	s := ultimateSession(t)
	readVulnerability(s)
}

// ultimateSession is the helper that carries the declaration.
func ultimateSession(t *testing.T) *harness.Session {
	return harness.New(t, harness.Needs(harness.Tier(edition.Ultimate))).Session()
}

// readVulnerability is the helper that carries the Ultimate id.
func readVulnerability(s *harness.Session) {
	harness.DoVoid(s, "vulnerability.get", nil)
}

// TestPlanted_PremiumInEE_Clean names a Premium action, which needs no
// declaration in this package.
func TestPlanted_PremiumInEE_Clean(t *testing.T) {
	s := harness.New(t).Session()
	harness.DoVoid(s, "merge_train.list", nil)
}

// TestPlanted_HelperReachedWithoutNeeds_Reported reaches the Ultimate helper
// without declaring the tier, which is the one-level attribution at work.
func TestPlanted_HelperReachedWithoutNeeds_Reported(t *testing.T) {
	readVulnerability(harness.New(t).Session())
}

// reader names an action from a method, which the gate attributes to the
// method by its receiver and name.
type reader struct {
	s *harness.Session
}

// list is the method carrying a Free id.
func (r reader) list() {
	harness.DoVoid(r.s, "issue.list", nil)
}

// TestPlanted_MethodSite_Listed calls the method, and declares a tier that
// is not a constant, which the gate reads as no declaration.
func TestPlanted_MethodSite_Listed(t *testing.T) {
	tier := edition.Premium
	env := harness.New(t, harness.Needs(harness.Tier(tier)))
	reader{s: env.Session()}.list()
}
