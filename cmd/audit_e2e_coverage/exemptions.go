package main

// ratchetEnabled is the switch the static gate's ratchet turns on with.
//
// It ships off. While the old suite is being ported, most catalog actions
// have no scenario in test/e2e/gitlab yet and the harness exports nothing
// uses, and a gate that failed on both would fail on every push until the
// port was complete. It is turned on in the step that makes the new job the
// release gate, after which every catalog action needs a scenario or an
// entry in [exemptedActions], and every harness export needs a consumer.
const ratchetEnabled = true

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
//
// The four guided flows were held here, under a category of their own for an
// action a scenario reached by naming its tool, because the harness could not
// spell a standalone action. It can, so their scenarios name typed constants
// like every other, and the category went with them: an action a scenario
// drives is one this gate can see.
var exemptedActions = map[string]actionExemption{}

// assertedFloors is the fewest actions each runtime must have asserted on
// some surface for -check to pass, keyed by the -runtime selector.
//
// It ships at zero for every runtime: the floors are recorded once the new
// suite covers what the old one did, and raised as the port proceeds. A
// runtime with no entry has no floor.
var assertedFloors = map[string]int{}

// fineGrainedFloors is the least each runtime's sessions on a fine-grained
// token must show for -check to pass, keyed by the -runtime selector like
// [assertedFloors], and applied to a live run alone ([checkFineGrained]).
//
// The figures are what issue 952's scenarios reach on GitLab 19.4.1, each
// runtime's the fewest any of its runs reached, so a run passes them only if
// the fine-grained scenarios ran and saw both halves: actions served, and
// actions no fine-grained token reaches refused for the credential before
// anything reached GitLab. The fine-grained sessions are those scenarios'
// alone, so a run of them by name reaches the figures a complete run does,
// which is how the EE figure was measured again once the attestation
// scenario pinned the flag its routes answer behind.
//
// They have to hold on a release the table was not recorded from as well,
// since the complete runs and CI's run pull the latest images and a scenario
// asserts what that release decides there rather than skipping. So the
// refused floor counts only the cells of actions the table denies to every
// fine-grained token, which every phase withholds: on CE the branch rule list
// and the work item create, on EE those and the epic read, the security
// attribute create and the vulnerability count, three surfaces each. The
// write a grant does not reach (a branch create) is withheld only where the
// grant decides the calls and is GitLab's own refusal elsewhere, so its three
// cells are not counted. Every asserted cell is a call a scenario makes in
// every phase, on each of the three surfaces: the eight actions the common
// scenarios run, and on EE the attestation list and the vulnerability list
// too. The two public reads among them, the issue links and the attestations,
// are served whether or not the grant decides the listing; only the
// assertion that the listing leaves them out waits for a phase that decides
// it.
var fineGrainedFloors = map[string]fineGrainedFloor{
	"ce": {Asserted: 24, Refused: 6},
	"ee": {Asserted: 30, Refused: 15},
}

// declaredCategories is the set a category must belong to, so a typo does
// not invent a fourth kind of exemption.
var declaredCategories = map[string]bool{
	categoryGitLabComOnly: true,
	categoryNoFixture:     true,
	categoryNotYetWritten: true,
}
