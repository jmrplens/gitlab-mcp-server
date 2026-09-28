package main

// skipDeclaration excuses one skip a complete Docker run may end with.
//
// A complete run is make test-e2e-ce or make test-e2e-ee: the runs whose record
// says what the suite covers. Each is held to the skips declared for its
// runtime and to no other, by -check-skips, which run-docker-e2e.sh applies
// after the tests when the target sets E2E_GATE_SKIPS.
type skipDeclaration struct {
	// Runtime is the run the skip happens in, ce or ee. A skip both runs have
	// is declared once per runtime, since each run is judged on its own and a
	// declaration one run never needs is stale there.
	Runtime string
	// Package is the package the test is in, as the last element of the
	// stream's import path spells it: common, ce or ee.
	Package string
	// Test is the skipped test's full name, or the name of a test every
	// subtest of which the declaration covers, such as a scenario that skips
	// on each surface for one reason.
	Test string
	// Because is a fragment of the reason the test printed, which the skip's
	// reason must contain. It is what keeps a declaration from excusing the
	// same test when it starts skipping for another reason.
	Because string
	// Category says what kind of skip this is.
	Category string
	// Reason says why the skip is allowed to stand, in the words a reviewer
	// needs to judge whether it still does.
	Reason string
}

// Skip categories. The set is small on purpose: a skip a complete run cannot
// avoid is rare, and every category names what would have to change for its
// entries to go. There is deliberately none for a fixture or a setting a run
// chose to leave out: a complete run starts every fixture on both runtimes and
// refuses to start without the external network, so such a skip is a run that
// is not complete rather than one to excuse.
const (
	// skipCategoryGitLabDefect is GitLab answering wrongly on the release the
	// run tested, where the scenario names the answer and skips. It goes on
	// the first release that answers, when the gate reports it stale.
	skipCategoryGitLabDefect = "gitlab-defect"
)

// declaredSkipCategories is the set a category must belong to, so a typo
// does not invent a kind of skip.
var declaredSkipCategories = map[string]bool{
	skipCategoryGitLabDefect: true,
}

// skipRuntimes are the runs -check-skips judges, one per Docker target.
var skipRuntimes = map[string]bool{
	"ce": true,
	"ee": true,
}

// declaredSkips holds every skip a complete run may end with.
//
// A skip nothing here declares fails the run, and so does a declaration that
// matched no skip of the run it declares for: a declaration left behind after
// its skip went is a claim a later reader would trust.
//
// The table is empty, which is the state it exists to keep. The two complete
// runs of de1ab3b49 ended with eleven skipped tests in seven test functions on
// CE and ten in five on EE, of which issue 1014 listed five and three; the
// gate found the rest. Its fixes removed all but the saved view lifecycle's:
// the images are held to their tags, so the two service account reads that
// need GitLab 19.4 run; a complete run turns E2E_EXTERNAL_NETWORK on, so the
// four importer tests that call a public URL run; both runtimes start the
// Bitbucket fixture, so the Bitbucket Server import runs on EE too; and on CE
// the tier pin and the work item lifecycle assert instead of skipping.
//
// The saved view lifecycle was declared on both runtimes until the complete
// runs of the wave 1 stack read GitLab's exceptions log: the 500 comes from a
// lock on the caller's user row that every token-authenticated create and
// subscribe meets on 19.4, after the create has saved the view
// (docs/development/upstream-bugs.md, entry 72). Nothing a client sends avoids
// it, so the scenario now asserts what GitLab does, the view the create left
// in the listing and a subscribe that recorded nothing, and runs on every
// release instead of skipping on the one that has the defect.
var declaredSkips = []skipDeclaration{}
