// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	// skipCategoryRunsLast is a scenario that changes an instance-wide
	// setting, and so runs alone and after every other test of the run: the
	// main pass skips it with its switch off, and run-docker-e2e.sh runs it in
	// a final pass on the same stack before the one teardown, with the switch
	// on and nothing after it but the teardown. Such a declaration excuses the
	// main pass's skip only when the final pass's stream (-final-results)
	// records the test passed, so a final pass that failed it, skipped it or
	// never ran fails the run. It is not a setting the run left out: the skip
	// is where the scenario waits for its turn. It goes with the scenario.
	skipCategoryRunsLast = "runs-last"
)

// declaredSkipCategories is the set a category must belong to, so a typo
// does not invent a kind of skip.
var declaredSkipCategories = map[string]bool{
	skipCategoryGitLabDefect: true,
	skipCategoryRunsLast:     true,
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
// It holds one entry, the throttle probe of issue 954, which turns GitLab's
// unauthenticated API throttle on for the whole instance and so runs only when
// E2E_AUTH_THROTTLE_PROBE is set. A complete CE run starts its main pass with
// the switch off, so the probe skips there, and then runs it last, alone, with
// the switch on, on the stack the main pass ran on; the declaration holds that
// final pass to having passed it.
//
// No skip of a scenario that a complete run means to run is declared, which is
// the state the table exists to keep. The two complete
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
var declaredSkips = []skipDeclaration{
	{
		Runtime: "ce", Package: "ce",
		Test:     "TestAuthThrottle_InvalidTokens_SpendTheAddressUnauthenticatedAllowance",
		Because:  "a complete CE run runs it last on the same stack",
		Category: skipCategoryRunsLast,
		// One literal rather than a concatenation: a package-level
		// initializer has no statement the coverage profile counts, so the
		// mutation gate would report each + in it as not covered.
		Reason: "The probe changes an instance-wide setting, GitLab's unauthenticated API throttle, so it cannot run beside the other packages: a probe cut short before its cleanup would leave the throttle on for every package that runs after it. A complete CE run therefore skips it in the main pass and runs it in a final pass of its own, with E2E_AUTH_THROTTLE_PROBE=true, on the same stack and before the one teardown, so nothing runs after it on the instance it changed and no second stack is booted for it. The skip stands only when that final pass passed the probe.",
	},
}
