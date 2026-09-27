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
// entries to go.
const (
	// skipCategoryFixtureOnOtherRuntime is a scenario whose fixture a
	// complete run starts on the other runtime only, deliberately, where the
	// scenario runs. It goes when that runtime starts the fixture too.
	skipCategoryFixtureOnOtherRuntime = "fixture-on-other-runtime"
	// skipCategoryGitLabDefect is GitLab answering wrongly on the release the
	// run tested, where the scenario names the answer and skips. It goes on
	// the first release that answers, when the gate reports it stale.
	skipCategoryGitLabDefect = "gitlab-defect"
)

// declaredSkipCategories is the set a category must belong to, so a typo
// does not invent a kind of skip.
var declaredSkipCategories = map[string]bool{
	skipCategoryFixtureOnOtherRuntime: true,
	skipCategoryGitLabDefect:          true,
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
// Issue 1014 found eleven skips on the CE run and ten on the EE run of
// de1ab3b49. Its fixes took eight of the first and six of the second: the
// images are held to their tags, so the two service account reads that need
// GitLab 19.4 run; the complete runs set E2E_EXTERNAL_NETWORK, so the four
// importer scenarios that call a public URL run; and on CE the tier pin and
// the work item lifecycle assert instead of skipping. What remains is declared
// below: the saved view lifecycle on both runs, on each of its three surfaces,
// and the Bitbucket Server import on EE.
var declaredSkips = []skipDeclaration{
	{
		Runtime:  "ee",
		Package:  "common",
		Test:     "TestAdmin_ExternalImporters/BitbucketServerImport",
		Because:  "BITBUCKET_SERVER_URL is not set",
		Category: skipCategoryFixtureOnOtherRuntime,
		Reason:   "run-docker-e2e.sh starts the Bitbucket Data Center fixture under ce only, which keeps a second one-gigabyte JVM off the licensed run. The import is a Free action, and the ce run drives it against that fixture.",
	},
	{
		Runtime:  "ce",
		Package:  "common",
		Test:     "TestWorkItemSavedViews_Lifecycle_CreateGetUpdateSubscribeDelete",
		Because:  savedViewCreateRefusal,
		Category: skipCategoryGitLabDefect,
		Reason:   "GitLab's workItemSavedViewCreate mutation, an experiment since 18.7, answered 500 Internal server error on every surface on 19.3.0 (the CE run of de1ab3b49); the listing is held either way. Unmeasured on 19.4, where this entry goes stale if the create answers.",
	},
	{
		Runtime:  "ee",
		Package:  "common",
		Test:     "TestWorkItemSavedViews_Lifecycle_CreateGetUpdateSubscribeDelete",
		Because:  savedViewCreateRefusal,
		Category: skipCategoryGitLabDefect,
		Reason:   "GitLab's workItemSavedViewCreate mutation, an experiment since 18.7, answered 500 Internal server error on every surface on 19.3.1-ee (the EE run of de1ab3b49); the listing is held either way. Unmeasured on 19.4, where this entry goes stale if the create answers.",
	},
}
