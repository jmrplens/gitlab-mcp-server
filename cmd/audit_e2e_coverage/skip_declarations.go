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

// savedViewCreateRefusal is what TestWorkItemSavedViews_Lifecycle prints when
// GitLab's saved view create answers 500, which both runtimes do.
const savedViewCreateRefusal = "the saved view create answered the experiment's 500"

// declaredSkips holds every skip a complete run may end with.
//
// A skip nothing here declares fails the run, and so does a declaration that
// matched no skip of the run it declares for: a declaration left behind after
// its skip went is a claim a later reader would trust.
//
// The two complete runs of de1ab3b49 ended with eleven skipped tests in seven
// test functions on CE and ten in five on EE, of which issue 1014 listed five
// and three; the gate found the rest. Its fixes remove all but the saved view
// lifecycle's: the images are held to their tags, so the two service account
// reads that need GitLab 19.4 run; a complete run turns E2E_EXTERNAL_NETWORK
// on, so the four importer tests that call a public URL run; both runtimes
// start the Bitbucket fixture, so the Bitbucket Server import runs on EE too;
// and on CE the tier pin and the work item lifecycle assert instead of
// skipping. What remains is the saved view lifecycle on both runtimes, on each
// of its three surfaces, declared below.
//
// Those two entries are GitLab's in the sense the category means: a 500 is an
// exception GitLab did not handle, and the create it answered is one the
// pinned schema accepts. What is not established is which exception, and so
// whether client-go's document or this server's input sets it off and whether
// another input GitLab accepts would avoid it; run-docker-e2e.sh now keeps
// GitLab's exceptions log among the reports, which is where the next complete
// run answers that. The entries go stale, and fail the gate, the first run the
// create answers.
var declaredSkips = []skipDeclaration{
	{
		Runtime:  "ce",
		Package:  "common",
		Test:     "TestWorkItemSavedViews_Lifecycle_CreateGetUpdateSubscribeDelete",
		Because:  savedViewCreateRefusal,
		Category: skipCategoryGitLabDefect,
		Reason:   "GitLab's workItemSavedViewCreate mutation, an experiment since 18.7, answered a create the pinned schema accepts with 500 Internal server error on every surface on 19.3.0 (the CE run of de1ab3b49); the listing is held either way. The exception behind it was not read: the next run's e2e-ce-gitlab-exceptions.json names it, and says whether the input sets it off. Unmeasured on 19.4, where this entry goes stale if the create answers.",
	},
	{
		Runtime:  "ee",
		Package:  "common",
		Test:     "TestWorkItemSavedViews_Lifecycle_CreateGetUpdateSubscribeDelete",
		Because:  savedViewCreateRefusal,
		Category: skipCategoryGitLabDefect,
		Reason:   "GitLab's workItemSavedViewCreate mutation, an experiment since 18.7, answered a create the pinned schema accepts with 500 Internal server error on every surface on 19.3.1-ee (the EE run of de1ab3b49); the listing is held either way. The exception behind it was not read: the next run's e2e-ee-gitlab-exceptions.json names it, and says whether the input sets it off. Unmeasured on 19.4, where this entry goes stale if the create answers.",
	},
}
