//go:build e2e

// needs_test.go covers the three declarations a test makes about how it runs:
// what the environment must provide, what it will change, and whether it can
// share the instance while it does.

package harness

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// testInstance builds an instance for the need table, with no client: every
// need judged here reads the probed facts or the settings, and one that asked
// GitLab would need a GitLab, which these tests deliberately do not have.
func testInstance(facts runtimeFacts, values map[string]string) *instance {
	return &instance{settings: settings{values: values}, facts: facts}
}

// TestNeeds_EachRequirement_IsJudgedAgainstTheRun checks every Need against a
// run that provides it and one that does not.
//
// The reason matters as much as the verdict: a skipped test is only useful if
// its message says what to provision, so each unsatisfied case asserts that
// the reason names the thing that is missing.
func TestNeeds_EachRequirement_IsJudgedAgainstTheRun(t *testing.T) {
	cases := []struct {
		name       string
		need       Need
		provided   *instance
		absent     *instance
		reasonWord string
	}{
		{
			name:       "admin",
			need:       NeedAdmin,
			provided:   testInstance(runtimeFacts{Admin: true}, nil),
			absent:     testInstance(runtimeFacts{}, nil),
			reasonWord: "administrator",
		},
		{
			name:       "runner",
			need:       NeedRunner,
			provided:   testInstance(runtimeFacts{}, map[string]string{envMode: "docker"}),
			absent:     testInstance(runtimeFacts{}, nil),
			reasonWord: "runner",
		},
		{
			name:       "fixture service",
			need:       NeedFixtureService,
			provided:   testInstance(runtimeFacts{}, map[string]string{envFixtureURL: "http://e2e-fixture:8080"}),
			absent:     testInstance(runtimeFacts{}, nil),
			reasonWord: envFixtureURL,
		},
		{
			name:       "bitbucket fixture",
			need:       NeedBitbucket,
			provided:   testInstance(runtimeFacts{}, map[string]string{envBitbucketServerURL: "http://bitbucket:7990"}),
			absent:     testInstance(runtimeFacts{}, nil),
			reasonWord: envBitbucketServerURL,
		},
		{
			name:       "github token",
			need:       NeedGitHubToken,
			provided:   testInstance(runtimeFacts{}, map[string]string{envGitHubToken: "gho-token"}),
			absent:     testInstance(runtimeFacts{}, nil),
			reasonWord: envGitHubToken,
		},
		{
			name:       "external network",
			need:       NeedExternalNetwork,
			provided:   testInstance(runtimeFacts{}, map[string]string{envExternalNetwork: "TRUE"}),
			absent:     testInstance(runtimeFacts{}, map[string]string{envExternalNetwork: "no"}),
			reasonWord: envExternalNetwork,
		},
		{
			name:       "tier ultimate",
			need:       Tier(edition.Ultimate),
			provided:   testInstance(runtimeFacts{Tier: edition.Ultimate}, nil),
			absent:     testInstance(runtimeFacts{Tier: edition.Premium}, nil),
			reasonWord: "premium",
		},
		{
			name:       "gitlab release",
			need:       GitLabAtLeast(19, 4),
			provided:   testInstance(runtimeFacts{Version: "19.4.0-ee"}, nil),
			absent:     testInstance(runtimeFacts{Version: "19.3.1-ee"}, nil),
			reasonWord: "19.4",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if ok, reason := testCase.need.available(testCase.provided); !ok {
				t.Fatalf("%s reported unavailable on a run that provides it: %s", testCase.need, reason)
			}
			ok, reason := testCase.need.available(testCase.absent)
			if ok {
				t.Fatalf("%s reported available on a run that does not provide it", testCase.need)
			}
			if !containsFold(reason, testCase.reasonWord) {
				t.Fatalf("%s reason = %q, want it to name %q", testCase.need, reason, testCase.reasonWord)
			}
		})
	}
}

// TestGitLabAtLeast_ComparesTheReleaseAndRefusesAnUnreadableVersion checks the
// release comparison at each of its edges: a later major whatever its minor, the
// same major with the minor at or past the one wanted, the same major below it,
// and an earlier major whose minor is larger, which a comparison of the minor
// alone would pass. A version that does not read as a release is judged too old
// and says so, because running a scenario against an instance that may lack
// the route fails it for a reason that is not this server's.
func TestGitLabAtLeast_ComparesTheReleaseAndRefusesAnUnreadableVersion(t *testing.T) {
	need := GitLabAtLeast(19, 4)
	if got, want := need.String(), "GitLab 19.4 or later"; got != want {
		t.Errorf("need name = %q, want %q", got, want)
	}
	cases := []struct {
		version    string
		want       bool
		reasonWord string
	}{
		{version: "20.0.0", want: true},
		{version: "19.4.0-ee", want: true},
		{version: "19.5.0-pre", want: true},
		// A version with no patch still names its release.
		{version: "19.4", want: true},
		{version: "19.3.1-ee", want: false, reasonWord: "19.3.1-ee"},
		{version: "18.11.0", want: false, reasonWord: "18.11.0"},
		{version: "", want: false, reasonWord: "does not read"},
		{version: "19", want: false, reasonWord: "does not read"},
		{version: "x.4.0", want: false, reasonWord: "does not read"},
		{version: "19.y.0", want: false, reasonWord: "does not read"},
	}
	for _, testCase := range cases {
		t.Run(testCase.version, func(t *testing.T) {
			ok, reason := need.available(testInstance(runtimeFacts{Version: testCase.version}, nil))
			if ok != testCase.want {
				t.Fatalf("available(%q) = %v (%s), want %v", testCase.version, ok, reason, testCase.want)
			}
			if testCase.want {
				if reason != "" {
					t.Errorf("available(%q) reason = %q, want none", testCase.version, reason)
				}
				return
			}
			if !strings.Contains(reason, testCase.reasonWord) {
				t.Errorf("available(%q) reason = %q, want it to name %q", testCase.version, reason, testCase.reasonWord)
			}
		})
	}
}

// TestNeedRunner_NoClientOutsideDocker_IsUnavailable checks the lookup path a
// self-hosted run takes.
//
// Docker mode answers yes without asking GitLab, because the compose stack
// registers a runner the token may not be allowed to list. Everywhere else the
// answer comes from the API, and a run that cannot ask is not a run that has a
// runner.
func TestNeedRunner_NoClientOutsideDocker_IsUnavailable(t *testing.T) {
	inst := testInstance(runtimeFacts{}, nil)

	if ok, _ := NeedRunner.available(inst); ok {
		t.Fatal("NeedRunner reported available with no client and no Docker stack")
	}
	// The answer is remembered, so a package full of runner tests asks once.
	if ok, _ := NeedRunner.available(inst); ok {
		t.Fatal("NeedRunner changed its answer on the second call")
	}
}

// TestNeedRunner_ListingOutsideDocker_AsksForAnInstanceRunner checks the half
// of the runner lookup a self-hosted run with a client takes: it lists the
// instance runners and answers yes only when the listing holds one. A listing
// GitLab refuses is a run that cannot tell, which is judged as no runner.
func TestNeedRunner_ListingOutsideDocker_AsksForAnInstanceRunner(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "one instance runner", status: http.StatusOK, body: `[{"id":1,"runner_type":"instance_type"}]`, want: true},
		{name: "no runner", status: http.StatusOK, body: `[]`},
		{name: "listing refused", status: http.StatusForbidden, body: `{"message":"403 Forbidden"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst := testInstance(runtimeFacts{}, nil)
			inst.client = testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v4/runners" || r.URL.Query().Get("type") != "instance_type" {
					t.Errorf("request = %s?%s, want the instance runner listing", r.URL.Path, r.URL.RawQuery)
				}
				testutil.RespondJSON(w, tc.status, tc.body)
			}))
			if ok, _ := NeedRunner.available(inst); ok != tc.want {
				t.Errorf("NeedRunner available = %v, want %v", ok, tc.want)
			}
		})
	}
}

// TestOptions_LocksAndSerial_RecordTheirDeclarations checks the two run
// declarations reach the options the Env is built from: every lock named, in
// order and repeats included (acquireLocks takes a repeated one once), and the
// serial flag.
func TestOptions_LocksAndSerial_RecordTheirDeclarations(t *testing.T) {
	var opts envOptions
	for _, option := range []Option{Locks(LockInstanceGlobal, LockCurrentUserState), Locks(LockInstanceGlobal), Serial()} {
		option(&opts)
	}
	if want := []Lock{LockInstanceGlobal, LockCurrentUserState, LockInstanceGlobal}; !slices.Equal(opts.locks, want) {
		t.Errorf("locks = %v, want %v", opts.locks, want)
	}
	if !opts.serial {
		t.Error("serial = false after Serial()")
	}
}

// TestAcquireLocks_RepeatedName_TakesItOnce checks that a test naming one lock
// twice does not deadlock against itself.
//
// The duplicate is not hypothetical: a helper that declares a lock and a test
// that declares the same one both reach this list.
func TestAcquireLocks_RepeatedName_TakesItOnce(t *testing.T) {
	done := make(chan struct{})
	go func() {
		release := acquireLocks([]Lock{LockInstanceGlobal, LockInstanceGlobal})
		release()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("acquireLocks deadlocked on a repeated lock name")
	}
}

// TestAcquireLocks_TwoHolders_RunOneAfterTheOther checks that the lock does
// what it is for.
//
// The second holder must not begin until the first has released, which is the
// whole reason a test declares a lock: it is about to change something every
// other test can see.
func TestAcquireLocks_TwoHolders_RunOneAfterTheOther(t *testing.T) {
	release := acquireLocks([]Lock{LockCurrentUserState})

	entered := make(chan struct{})
	go func() {
		second := acquireLocks([]Lock{LockCurrentUserState})
		close(entered)
		second()
	}()

	select {
	case <-entered:
		t.Fatal("the second holder entered while the first still held the lock")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the second holder never entered after the first released")
	}
}

// TestSerialGate_SerialTest_WaitsForTheOthers checks that Serial means alone.
//
// A license that comes and goes underneath the other tests is the case this
// exists for: no lock can express it, because every test is affected and none
// of them names the license.
func TestSerialGate_SerialTest_WaitsForTheOthers(t *testing.T) {
	g := newSerialGate()
	shared := g.enter("TestOther", false)

	alone := make(chan struct{})
	go func() {
		release := g.enter("TestSerial", true)
		close(alone)
		release()
	}()

	select {
	case <-alone:
		t.Fatal("the serial test entered while another test held the gate")
	case <-time.After(50 * time.Millisecond):
	}

	shared()
	select {
	case <-alone:
	case <-time.After(5 * time.Second):
		t.Fatal("the serial test never entered after the last other test left")
	}
}

// TestSerialGate_SecondEnvOfOneTest_ReEntersWhileSerialWaits checks the
// re-entry that keeps one parent fixture with three subtests from deadlocking.
//
// A plain RWMutex blocks a new reader once a writer is waiting, so the parent
// test's second Env would wait for a serial test that is itself waiting for
// the parent to finish. Counting holders per top-level test is what avoids it,
// and this is the case that proves it.
func TestSerialGate_SecondEnvOfOneTest_ReEntersWhileSerialWaits(t *testing.T) {
	g := newSerialGate()
	first := g.enter("TestParent", false)

	waiting := make(chan struct{})
	go func() {
		release := g.enter("TestSerial", true)
		close(waiting)
		release()
	}()
	// Give the serial waiter time to claim the gate before the parent asks
	// for its second slot.
	time.Sleep(50 * time.Millisecond)

	entered := make(chan struct{})
	go func() {
		second := g.enter("TestParent", false)
		close(entered)
		second()
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("a second Env of a test already holding the gate could not re-enter")
	}

	first()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("the serial test never entered after the parent released")
	}
}

// TestSerialGate_ManyHolders_ReleaseEveryOne checks the counting itself: the
// gate opens for a serial test only when the last of several holders of one
// test has left.
func TestSerialGate_ManyHolders_ReleaseEveryOne(t *testing.T) {
	g := newSerialGate()

	var wg sync.WaitGroup
	releases := make([]func(), 3)
	for i := range releases {
		releases[i] = g.enter("TestParent", false)
	}

	opened := make(chan struct{})
	wg.Go(func() {
		release := g.enter("TestSerial", true)
		close(opened)
		release()
	})

	for i, release := range releases {
		// The wait gives the serial test time to walk through a gate that
		// opened too early; a non-blocking look would pass before it had run.
		select {
		case <-opened:
			t.Fatalf("the gate opened with %d holders still inside", len(releases)-i)
		case <-time.After(50 * time.Millisecond):
		}
		release()
	}

	wg.Wait()
	select {
	case <-opened:
	default:
		t.Fatal("the gate never opened after every holder left")
	}
}

// TestSerialGate_SerialTestInside_KeepsEveryOtherTestOut checks the other
// direction of Serial: once a serial test holds the gate, a test that did not
// hold a slot before waits until the serial test leaves, rather than starting
// beside the change it declared nothing could share the instance with.
func TestSerialGate_SerialTestInside_KeepsEveryOtherTestOut(t *testing.T) {
	g := newSerialGate()
	release := g.enter("TestSerial", true)

	entered := make(chan struct{})
	go func() {
		other := g.enter("TestOther", false)
		close(entered)
		other()
	}()

	select {
	case <-entered:
		t.Fatal("another test entered while the serial test held the gate")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the other test never entered after the serial test left")
	}
}

// TestTopLevelTest_SubtestName_ReturnsTheParent checks the grain the gate
// counts at.
func TestTopLevelTest_SubtestName_ReturnsTheParent(t *testing.T) {
	cases := map[string]string{
		"TestCommon_Issues":              "TestCommon_Issues",
		"TestCommon_Issues/dynamic":      "TestCommon_Issues",
		"TestCommon_Issues/meta/refused": "TestCommon_Issues",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := topLevelTest(name); got != want {
				t.Fatalf("topLevelTest(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

// containsFold reports whether haystack contains needle, ignoring case, so a
// reason can be asserted on without pinning its exact wording.
func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
