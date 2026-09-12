package main

// ratchetEnabled is the switch the static gate's ratchet turns on with.
//
// It ships off. While the old suite is being ported, most catalog actions
// have no scenario in test/e2e/gitlab yet and the harness exports nothing
// uses, and a gate that failed on both would fail on every push until the
// port was complete. It is turned on in the step that makes the new job the
// release gate, after which every catalog action needs a scenario or an
// entry in [exemptedActions], and every harness export needs a consumer.
const ratchetEnabled = false

// actionExemption records why a catalog action has no scenario in any
// package that could run it.
type actionExemption struct {
	// Category says what kind of exemption this is, so a reader can tell the
	// kinds apart without reading every reason.
	Category string
	// Reason says why, in the words a reviewer needs to judge whether it
	// still holds.
	Reason string
}

// Exemption categories. The set is small on purpose: an action nothing can
// run on a Docker instance is the only honest exemption, and "not yet
// written" is a category so that a backlog is a declaration rather than a
// silence.
const (
	// categoryGitLabComOnly is an action only GitLab.com serves, which
	// test/e2e/orbit covers against the real instance.
	categoryGitLabComOnly = "gitlab-com-only"
	// categoryNoFixture is an action whose precondition no Docker fixture can
	// provide: a Geo secondary, an identity provider, an importer credential.
	categoryNoFixture = "no-fixture"
	// categoryNotYetWritten is a scenario that is owed, named so the backlog
	// is visible in the gate rather than hidden in a silence.
	categoryNotYetWritten = "not-yet-written"
)

// exemptedActions holds every catalog action without a scenario, each with a
// category and a reason.
//
// It ships empty. The ratchet reads it only once [ratchetEnabled] is on, but
// a stale entry is a finding on every run: an entry naming an action the
// catalog no longer has, or one a package can now run, leaves a claim behind
// that a later reader would trust.
var exemptedActions = map[string]actionExemption{}

// assertedFloors is the fewest actions each runtime must have asserted on
// some surface for -check to pass, keyed by the -runtime selector.
//
// It ships at zero for every runtime: the floors are recorded once the new
// suite covers what the old one did, and raised as the port proceeds. A
// runtime with no entry has no floor.
var assertedFloors = map[string]int{}

// declaredCategories is the set a category must belong to, so a typo does
// not invent a fourth kind of exemption.
var declaredCategories = map[string]bool{
	categoryGitLabComOnly: true,
	categoryNoFixture:     true,
	categoryNotYetWritten: true,
}
