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
// without declaring the tier, which is the attribution through helpers at
// work.
func TestPlanted_HelperReachedWithoutNeeds_Reported(t *testing.T) {
	readVulnerability(harness.New(t).Session())
}

// TestPlanted_TwoLevelHelperWithoutNeeds_Reported reaches the Ultimate
// helper through another helper and declares nothing, which the attribution
// must follow two calls down.
func TestPlanted_TwoLevelHelperWithoutNeeds_Reported(t *testing.T) {
	readThroughHelper(harness.New(t).Session())
}

// readThroughHelper is the helper one step above readVulnerability.
func readThroughHelper(s *harness.Session) {
	readVulnerability(s)
}

// TestPlanted_UltimateViaTwoHelpers_Clean declares the tier two helpers away
// and names the action two helpers away, which is clean on the same terms.
func TestPlanted_UltimateViaTwoHelpers_Clean(t *testing.T) {
	readThroughHelper(ultimateSessionAgain(t))
}

// ultimateSessionAgain is one step above ultimateSession.
func ultimateSessionAgain(t *testing.T) *harness.Session {
	return ultimateSession(t)
}

// TestPlanted_HelperCycle_Terminates reaches two helpers that call each
// other, which the attribution must walk once and not forever; it declares
// the tier, so the Ultimate id they carry is clean.
func TestPlanted_HelperCycle_Terminates(t *testing.T) {
	cycleA(ultimateSession(t), 2)
}

// cycleA and cycleB call each other until the count runs out.
func cycleA(s *harness.Session, n int) {
	if n > 0 {
		cycleB(s, n-1)
	}
}

// cycleB is the half of the cycle carrying the id.
func cycleB(s *harness.Session, n int) {
	harness.DoVoid(s, "vulnerability.get", nil)
	cycleA(s, n)
}

// TestPlanted_TierOutsideNeeds_Reported builds the Ultimate requirement and
// hands it to nothing, which declares nothing: only a Tier inside Needs
// reaches New.
func TestPlanted_TierOutsideNeeds_Reported(t *testing.T) {
	_ = harness.Tier(edition.Ultimate)
	harness.DoVoid(harness.New(t).Session(), "vulnerability.list", nil)
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

// get is the pointer-receiver method carrying an Ultimate id, keyed the same
// as a value-receiver method of the type.
func (r *reader) get() {
	harness.DoVoid(r.s, "vulnerability.get", nil)
}

// TestPlanted_MethodSite_Listed calls the method, and declares a tier that
// is not a constant, which the gate reads as no declaration.
func TestPlanted_MethodSite_Listed(t *testing.T) {
	tier := edition.Premium
	env := harness.New(t, harness.Needs(harness.Tier(tier)))
	reader{s: env.Session()}.list()
}

// TestPlanted_PointerMethodWithoutNeeds_Reported reaches the Ultimate id
// through the pointer-receiver method and declares nothing, which the gate
// must attribute to this test through the method's key.
func TestPlanted_PointerMethodWithoutNeeds_Reported(t *testing.T) {
	r := &reader{s: harness.New(t).Session()}
	r.get()
}
