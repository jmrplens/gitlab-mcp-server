package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fairnessOptions are the flags a fairness run is built from, small enough for
// a test and valid enough to reach validation's later checks.
func fairnessOptions() options {
	return options{
		fairness:          "tools-call-rps",
		fairnessJSON:      defaultFairnessRecord,
		fairnessSurface:   surfaceDynamic,
		fairnessQuiet:     2,
		fairnessNoisy:     1,
		fairnessQuietRate: 2,
		fairnessNoisyRate: 40,
		fairnessPhase:     2 * time.Second,
		fairnessLeadIn:    4 * time.Second,
		fairnessDeadline:  time.Second,
		fairnessRepeats:   2,
	}
}

// TestRateLimitRefusalPrefix_MatchesTheServersOwnWording verifies the harness
// classifies refusals by the constant the server writes them with rather than
// by a copy of the sentence.
//
// The whole classifier rests on this: a refused tools/call arrives as a
// successful result whose only mark is its text, so a wording change that this
// package did not follow would silently turn every refusal into a failure, and
// the arm with the bound in force would read as a broken server rather than a
// fair one. Reading the constant makes that a compile error instead.
func TestRateLimitRefusalPrefix_MatchesTheServersOwnWording(t *testing.T) {
	if rateLimitRefusal != toolutil.RateLimitRefusalPrefix {
		t.Errorf("the harness matches %q against a server that writes %q", rateLimitRefusal, toolutil.RateLimitRefusalPrefix)
	}
	for _, bound := range fairnessBounds {
		for _, refusal := range bound.Refusals {
			t.Run(bound.ID+" "+refusal.Method, func(t *testing.T) {
				if refusal.TextPrefix == "" {
					t.Error("a refusal shape with no wording would match any message the status and code allow")
				}
				if refusal.Status == 0 {
					t.Error("a refusal shape with no status cannot tell the credential bucket from the address lockout")
				}
			})
		}
	}
}

// TestClassifyOutcome_KeepsRefusalsApartFromSuccessAndFailure verifies the four
// terminal outcomes are decided from the wire and that the whitelist admits
// only the bound under test.
//
// Each case is a shape the real server actually produces. The two that matter
// most are the refused tools/call, which is a successful HTTP 200 response with
// no code anywhere, and the per-address lockout, which carries the same
// JSON-RPC code as the credential bucket and is separated from it only by the
// status: counting either of those as the bound's refusal would turn a broken
// or throttled run into an apparent fairness result.
func TestClassifyOutcome_KeepsRefusalsApartFromSuccessAndFailure(t *testing.T) {
	bucket, err := boundByID("tools-call-rps")
	if err != nil {
		t.Fatalf("boundByID: %v", err)
	}
	listing, err := boundByID("tools-list-rps")
	if err != nil {
		t.Fatalf("boundByID: %v", err)
	}
	listen, err := boundByID("listen-streams")
	if err != nil {
		t.Fatalf("boundByID: %v", err)
	}
	processListing, err := boundByID("tools-list-process")
	if err != nil {
		t.Fatalf("boundByID: %v", err)
	}

	cases := []struct {
		name     string
		method   string
		err      error
		refusals []refusalSpec
		want     string
	}{
		{
			name: "a completed call is served", method: methodToolsCall,
			refusals: bucket.Refusals, want: outcomeServed,
		},
		{
			name: "a refused tools/call is a successful result flagged in error", method: methodToolsCall,
			err: fmt.Errorf("tools/call: %w", &toolResultError{
				Text: toolutil.RateLimitRefusalPrefix + "gitlab_find_action; retry after a short backoff",
			}),
			refusals: bucket.Refusals, want: outcomeRefused,
		},
		{
			name: "a tool that broke on its own is a failure, not a refusal", method: methodToolsCall,
			err:      fmt.Errorf("tools/call: %w", &toolResultError{Text: "the project does not exist"}),
			refusals: bucket.Refusals, want: outcomeFailed,
		},
		{
			// The code as the server writes it (internal/toolutil's
			// rateLimitedErrorCode), not this package's name for it, so the
			// name is held to the wire rather than to itself.
			name: "a refused listing carries the code instead", method: methodToolsList,
			err: fmt.Errorf("tools/list: %w", rpcError{
				Code: -42900, Message: toolutil.RateLimitRefusalPrefix + "tools/list; retry after a short backoff",
			}),
			refusals: listing.Refusals, want: outcomeRefused,
		},
		{
			// The bucket the whole process shares refuses in the credential's
			// own words and code, so a caller is not told others are listing;
			// its arm keeps every credential's bucket out of reach, which is
			// what makes a listing refused there the process's.
			name: "the process's listing refusal is its bound's", method: methodToolsList,
			err: fmt.Errorf("tools/list: %w", rpcError{
				Code: -42900, Message: toolutil.RateLimitRefusalPrefix + "tools/list; retry after a short backoff",
			}),
			refusals: processListing.Refusals, want: outcomeRefused,
		},
		{
			// It meters listings and nothing else, so a refused tool call in
			// its arm is a failure of the run rather than its bound refusing.
			name: "a refused tool call is not the process's listing bound's", method: methodToolsCall,
			err: fmt.Errorf("tools/call: %w", &toolResultError{
				Text: toolutil.RateLimitRefusalPrefix + "gitlab_find_action; retry after a short backoff",
			}),
			refusals: processListing.Refusals, want: outcomeFailed,
		},
		{
			// cmd/server's codeServerBusy, which the listen ceiling refuses
			// with: a different number from the buckets', written literally
			// for the same reason as the case above.
			name: "a refused stream carries the server-busy code", method: methodSubscriptionsListen,
			err: fmt.Errorf("subscriptions/listen: %w", rpcError{
				Code: -32000, Message: "too many open subscriptions/listen streams for this credential",
			}),
			refusals: listen.Refusals, want: outcomeRefused,
		},
		{
			name: "the address lockout carries the same code at another status", method: methodToolsList,
			err: fmt.Errorf("tools/list: %w", &httpStatusError{
				Method: methodToolsList, Status: httpTooManyRequests, Snippet: "too many failed authentications",
			}),
			refusals: listing.Refusals, want: outcomeFailed,
		},
		{
			name: "a refusal of another method is not this bound's", method: methodToolsCall,
			err: fmt.Errorf("tools/call: %w", rpcError{
				Code: rateLimitCode, Message: toolutil.RateLimitRefusalPrefix + "tools/list; retry after a short backoff",
			}),
			refusals: listing.Refusals, want: outcomeFailed,
		},
		{
			name: "a refusal whose code is not the bound's is not the bound's", method: methodToolsList,
			err: fmt.Errorf("tools/list: %w", rpcError{
				Code: serverBusyCode, Message: toolutil.RateLimitRefusalPrefix + "tools/list; retry after a short backoff",
			}),
			refusals: listing.Refusals, want: outcomeFailed,
		},
		{
			// The bound's shape names a code, so a refusal carried by a tool
			// result rather than by a JSON-RPC error is not its shape however
			// familiar the words are. Admitting it would count one bound's
			// refusals against another and report a bucket that meters
			// listings as metering calls.
			name: "the bound's words carried by a result rather than a code are not its refusal", method: methodToolsList,
			err: fmt.Errorf("tools/list: %w", &toolResultError{
				Text: toolutil.RateLimitRefusalPrefix + "tools/list; retry after a short backoff",
			}),
			refusals: listing.Refusals, want: outcomeFailed,
		},
		{
			// The code alone is not enough either: two bounds share a code in
			// this server already, and the words are what separate them.
			name: "the bound's code with another refusal's words is not its refusal", method: methodToolsList,
			err: fmt.Errorf("tools/list: %w", rpcError{
				Code: rateLimitCode, Message: "the credential pool is full",
			}),
			refusals: listing.Refusals, want: outcomeFailed,
		},
		{
			// A shape that names no method is the one the whitelist uses for a
			// bound that refuses everything, and it has to match whichever
			// method carried the refusal rather than none of them.
			name: "a shape naming no method matches the method that carried it", method: methodResourcesList,
			err:      fmt.Errorf("resources/list: %w", &toolResultError{Text: "quota reached for this credential"}),
			refusals: []refusalSpec{{Status: httpOK, TextPrefix: "quota reached"}},
			want:     outcomeRefused,
		},
		{
			name: "a client that gave up is timed out however it looks", method: methodToolsCall,
			err:      fmt.Errorf("tools/call: %w", context.DeadlineExceeded),
			refusals: bucket.Refusals, want: outcomeTimedOut,
		},
		{
			name: "anything else is a failure", method: methodToolsCall,
			err: errors.New("connection reset"), refusals: bucket.Refusals, want: outcomeFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyOutcome(tc.method, tc.err, tc.refusals); got != tc.want {
				t.Errorf("classifyOutcome = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBoundByID_FindsDeclaredBoundsAndNamesTheRest verifies the lookup and the
// message an unknown name gets, which is where the list of bounds is published.
func TestBoundByID_FindsDeclaredBoundsAndNamesTheRest(t *testing.T) {
	for _, id := range boundIDs() {
		t.Run(id, func(t *testing.T) {
			bound, err := boundByID(id)
			if err != nil {
				t.Fatalf("boundByID(%q): %v", id, err)
			}
			if bound.Label == "" || len(bound.QuietVerbs) == 0 || len(bound.NoisyVerbs) == 0 {
				t.Errorf("bound %q is not fully declared: %+v", id, bound)
			}
			for _, verb := range append(append([]string{}, bound.QuietVerbs...), bound.NoisyVerbs...) {
				if _, ok := verbs[verb]; !ok {
					t.Errorf("bound %q asks for unknown verb %q", id, verb)
				}
			}
		})
	}
	_, err := boundByID("no-such-bound")
	if err == nil || !strings.Contains(err.Error(), "tools-call-rps") {
		t.Errorf("an unknown bound gave %v, want an error naming the ones that exist", err)
	}
}

// TestBoundSpec_Meters_AnswersForTheMethodTheBoundRefuses verifies the
// positive control is specific to the bound rather than to any refusal.
func TestBoundSpec_Meters_AnswersForTheMethodTheBoundRefuses(t *testing.T) {
	bucket, err := boundByID("tools-call-rps")
	if err != nil {
		t.Fatalf("boundByID: %v", err)
	}
	cases := map[string]bool{methodToolsCall: true, methodToolsList: false}
	for method, want := range cases {
		t.Run(method, func(t *testing.T) {
			if got := bucket.meters(method); got != want {
				t.Errorf("meters(%q) = %v, want %v", method, got, want)
			}
		})
	}
	t.Run("a shape naming no method covers every method", func(t *testing.T) {
		unrestricted := boundSpec{Refusals: []refusalSpec{{Status: httpOK, TextPrefix: "x"}}}
		if !unrestricted.meters(methodToolsList) {
			t.Error("a refusal shape restricted to no method should cover them all")
		}
	})
}

// TestFairnessPlanFor_BuildsTheComparisonTheFlagsAskFor verifies the plan is
// assembled from the flags and that its populations take distinct credential
// ranges, which is what makes them two tenants rather than one.
func TestFairnessPlanFor_BuildsTheComparisonTheFlagsAskFor(t *testing.T) {
	plan, err := fairnessPlanFor(fairnessOptions())
	if err != nil {
		t.Fatalf("fairnessPlanFor: %v", err)
	}
	// The surface first and the bound after it, the order the run's progress
	// line and its scenario are named in.
	if plan.ID != "fairness-dynamic-tools-call-rps" {
		t.Errorf("plan ID = %q, want fairness-dynamic-tools-call-rps", plan.ID)
	}
	if plan.Quiet.Credentials != 2 || plan.Noisy.Credentials != 1 {
		t.Errorf("populations = %d quiet and %d noisy, want 2 and 1", plan.Quiet.Credentials, plan.Noisy.Credentials)
	}
	quietFirst, quietLast := plan.credentials(plan.Quiet)
	noisyFirst, noisyLast := plan.credentials(plan.Noisy)
	if quietFirst != 0 || quietLast != 2 || noisyFirst != 2 || noisyLast != 3 {
		t.Errorf("credential ranges %d-%d and %d-%d overlap or leave a gap", quietFirst, quietLast, noisyFirst, noisyLast)
	}
	if plan.totalCredentials() != 3 {
		t.Errorf("totalCredentials = %d, want 3", plan.totalCredentials())
	}
	if !strings.Contains(plan.describe(), "2 quiet credentials") {
		t.Errorf("describe = %q, want it to name the populations", plan.describe())
	}
}

// TestFairnessPlan_Validate_RefusesAPlanThatWouldMeasureSomethingElse verifies
// every way a plan is refused before a process is started.
//
// The lead-in check is the one worth reading twice: a phase that begins with a
// full bucket measures the burst rather than the bound, and would report that a
// limit does almost nothing.
func TestFairnessPlan_Validate_RefusesAPlanThatWouldMeasureSomethingElse(t *testing.T) {
	cases := []struct {
		name string
		edit func(*options)
		want string
	}{
		{name: "an unknown bound", edit: func(o *options) { o.fairness = "nope" }, want: "no bound named"},
		{
			name: "a bound this driver cannot provoke",
			edit: func(o *options) { o.fairness = "listen-streams" }, want: "cannot be measured yet",
		},
		{name: "an unknown surface", edit: func(o *options) { o.fairnessSurface = "sideways" }, want: "unknown surface"},
		{name: "a population with no credentials", edit: func(o *options) { o.fairnessQuiet = 0 }, want: "at least one credential"},
		{name: "a population with no rate", edit: func(o *options) { o.fairnessNoisyRate = 0 }, want: "positive offered rate"},
		{name: "no phase", edit: func(o *options) { o.fairnessPhase = 0 }, want: "must be positive"},
		{name: "a negative lead-in", edit: func(o *options) { o.fairnessLeadIn = -time.Second }, want: "must not be negative"},
		{name: "no deadline", edit: func(o *options) { o.fairnessDeadline = 0 }, want: "must be positive"},
		{name: "a lead-in the burst outlives", edit: func(o *options) { o.fairnessLeadIn = time.Second }, want: "shorter than"},
		{name: "no repetitions", edit: func(o *options) { o.fairnessRepeats = 0 }, want: "must be positive"},
		{
			name: "a quiet population too small for a percentile",
			edit: func(o *options) { o.fairnessPhase = 5 * time.Second; o.fairnessQuietRate = 0.1 },
			want: "too few for a percentile",
		},
		{
			name: "a quiet population the bound would refuse",
			edit: func(o *options) { o.fairnessQuietRate = 60 },
			want: "is not the quiet tenant",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := fairnessOptions()
			tc.edit(&opts)
			_, err := fairnessPlanFor(opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("fairnessPlanFor = %v, want an error about %q", err, tc.want)
			}
		})
	}
}

// TestFairnessPlan_Validate_AcceptsAPlanOnEachBoundary verifies each limit the
// plan is held to admits the value that sits exactly on it: a lead-in exactly
// as long as the burst takes to drain, a quiet population offering exactly the
// share of the bound it may take, and no lead-in at all where there is no
// burst to drain.
//
// These are the values an operator reaches by working the arithmetic out, so
// refusing them would refuse the one plan that followed the rule precisely.
func TestFairnessPlan_Validate_AcceptsAPlanOnEachBoundary(t *testing.T) {
	cases := []struct {
		name string
		edit func(*options)
	}{
		// Forty requests of burst against the twenty a second offered above
		// the bucket's ten drain in exactly two seconds.
		{name: "a lead-in exactly as long as the drain", edit: func(o *options) { o.fairnessLeadIn = 2 * time.Second }},
		// Half of the quiet verbs are metered, so ten a second offers five,
		// which is half the bucket's ten: the most a quiet tenant may take.
		{name: "a quiet population exactly at its share", edit: func(o *options) { o.fairnessQuietRate = 10 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := fairnessOptions()
			opts.fairnessNoisyRate = 30
			tc.edit(&opts)
			if _, err := fairnessPlanFor(opts); err != nil {
				t.Errorf("fairnessPlanFor = %v, want a plan on the boundary accepted", err)
			}
		})
	}

	t.Run("no lead-in where there is no burst", func(t *testing.T) {
		plan := fairnessPlan{
			Surface: surfaceDynamic,
			Bound: boundSpec{
				Refusals:   []refusalSpec{{Status: httpOK, TextPrefix: "quota reached", Method: methodToolsCall}},
				QuietVerbs: []string{verbCall}, NoisyVerbs: []string{verbCall},
			},
			Quiet:    populationSpec{Name: populationQuiet, Credentials: 1, Rate: 4, Verbs: []string{verbCall}},
			Noisy:    populationSpec{Name: populationNoisy, Credentials: 1, Rate: 40, Verbs: []string{verbCall}},
			Phase:    2 * time.Second,
			LeadIn:   0,
			Deadline: time.Second,
			Repeats:  1,
		}
		if err := plan.validate(); err != nil {
			t.Errorf("validate = %v, want a bound with no bucket to need no lead-in", err)
		}
	})
}

// TestParseFlags_FairnessDefaults_MakeAPlanThatValidates verifies a fairness
// run started with nothing but the bound's name is one the plan accepts.
//
// The defaults are compared against themselves everywhere else; what makes
// them right is that together they describe a run that measures something: a
// lead-in that outlasts the shipped bucket's burst at the default noisy rate,
// and a deadline a request can be answered inside.
func TestParseFlags_FairnessDefaults_MakeAPlanThatValidates(t *testing.T) {
	opts := withArgs(t, "-fairness=tools-call-rps")
	if _, err := fairnessPlanFor(opts); err != nil {
		t.Errorf("fairnessPlanFor over the flag defaults = %v, want a plan the defaults can run", err)
	}
}

// TestPopulationSpec_Schedule_SpacesAndBoundsTheRequests verifies the two
// numbers a credential's schedule is built from: the interval between its
// requests is a second over its rate, and the requests it may hold in flight
// are twice what that rate accumulates inside one deadline.
func TestPopulationSpec_Schedule_SpacesAndBoundsTheRequests(t *testing.T) {
	pop := populationSpec{Rate: 5}
	if got := pop.period(); got != 200*time.Millisecond {
		t.Errorf("period = %s at five a second, want 200ms", got)
	}
	plan := fairnessPlan{Deadline: 2 * time.Second}
	if got := plan.inFlight(pop); got != 20 {
		t.Errorf("inFlight = %d at five a second over a two-second deadline, want twice the ten it can accumulate", got)
	}
}

// TestBoundSpec_Drain_DerivesTheLeadInFromTheBucketRatherThanAConstant
// verifies how long the burst takes to empty is computed from the bound's own
// numbers and the rate offered at it, and that a population the bound would
// never refuse is named as such.
//
// A constant would be right at one offered rate and wrong at every other, and
// the two failures point opposite ways: too short a lead-in opens the measured
// window on a full bucket and reports that the bound does almost nothing,
// while a noisy population under the limit is never refused at all and would
// spend both arms discovering it.
func TestBoundSpec_Drain_DerivesTheLeadInFromTheBucketRatherThanAConstant(t *testing.T) {
	metered := boundSpec{Bucket: &bucketSpec{Rate: 10, Burst: 40}}
	// A bucket the server derives from the configured one, refilled a tenth as
	// fast and holding the same burst, which is the shape a metered listing
	// has. Read from the configured rate instead, the two cases below answer
	// the opposite of the truth: a population at six a second is refused by
	// this bound and would be called too quiet to measure, and the burst it
	// does drain in eight seconds would be believed never to drain at all.
	derived := boundSpec{Bucket: &bucketSpec{Rate: 10, Burst: 40, Metered: 1}}
	cases := []struct {
		name      string
		bound     boundSpec
		offered   float64
		want      time.Duration
		wantBites bool
	}{
		{name: "twice the bound drains in four seconds", bound: metered, offered: 20, want: 4 * time.Second, wantBites: true},
		{name: "twenty times it drains in two", bound: metered, offered: 210, want: 200 * time.Millisecond, wantBites: true},
		{name: "at the bound it never drains", bound: metered, offered: 10, wantBites: false},
		{name: "under the bound it never drains", bound: metered, offered: 1, wantBites: false},
		{name: "a derived bucket drains at the rate it meters", bound: derived, offered: 6, want: 8 * time.Second, wantBites: true},
		{name: "under the derived rate it never drains", bound: derived, offered: 1, wantBites: false},
		{name: "a bound that holds no bucket needs no lead-in", bound: boundSpec{}, offered: 100, wantBites: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, bites := tc.bound.drain(tc.offered)
			if bites != tc.wantBites {
				t.Errorf("bites = %v, want %v", bites, tc.wantBites)
			}
			if got != tc.want {
				t.Errorf("drain = %s, want %s", got, tc.want)
			}
		})
	}
	t.Run("a plan the bound would never refuse", func(t *testing.T) {
		opts := fairnessOptions()
		opts.fairnessNoisyRate = 5
		_, err := fairnessPlanFor(opts)
		if err == nil || !strings.Contains(err.Error(), "would never refuse it") {
			t.Errorf("fairnessPlanFor = %v, want the noisy population refused as too quiet", err)
		}
	})
}

// TestBoundSpec_MeteredOffered_CountsOnlyTheVerbsTheBoundRefuses verifies a
// population is weighed against a bound by what reaches the bucket rather than
// by its rate.
//
// The two are the same number only for a population whose every verb the bound
// meters, and every judgement about a population is derived from it: whether
// the noisy one will be refused at all, how long the burst takes to drain,
// whether the quiet one is quiet, and how much the bound should have refused.
func TestBoundSpec_MeteredOffered_CountsOnlyTheVerbsTheBoundRefuses(t *testing.T) {
	listings, err := boundByID("tools-list-rps")
	if err != nil {
		t.Fatalf("boundByID: %v", err)
	}
	cases := []struct {
		name  string
		verbs []string
		rate  float64
		want  float64
	}{
		{name: "a population of the metered verb alone", verbs: []string{verbList}, rate: 20, want: 20},
		{name: "one alternating it with an unmetered verb", verbs: []string{verbCall, verbList}, rate: 20, want: 10},
		{name: "one the bound never looks at", verbs: []string{verbCall}, rate: 20, want: 0},
		{name: "one with no verbs at all", rate: 20, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := listings.meteredOffered(populationSpec{Rate: tc.rate, Verbs: tc.verbs})
			if got != tc.want {
				t.Errorf("meteredOffered = %g, want %g", got, tc.want)
			}
		})
	}
}

// TestBoundSpec_QuietRate_ComesFromTheBoundRatherThanAConstant verifies the
// rate a quiet population takes by default leaves it under every declared
// bound, which one constant cannot do.
//
// The shipped bucket meters ten requests a second and the listing bucket it
// derives meters one, so the two-a-second default that leaves the first a
// tenfold margin sat exactly on the second: the quiet tenant of that
// comparison was one jitter away from being refused by the bound it was
// supposed to be protected by, and a run that refused it would have reported
// on a tenant it was turning away.
func TestBoundSpec_QuietRate_ComesFromTheBoundRatherThanAConstant(t *testing.T) {
	for _, bound := range fairnessBounds {
		t.Run(bound.ID, func(t *testing.T) {
			rate := bound.quietRate()
			if rate <= 0 || rate > defaultQuietRate {
				t.Errorf("quietRate = %g, want a positive rate no busier than the shipped %g", rate, defaultQuietRate)
			}
			if bound.Bucket == nil {
				return
			}
			offered := bound.meteredOffered(populationSpec{Rate: rate, Verbs: bound.QuietVerbs})
			if ceiling := quietMaxShare * bound.Bucket.meteredRate(); offered > ceiling {
				t.Errorf("the default quiet population offers %g against a ceiling of %g", offered, ceiling)
			}
		})
	}
	t.Run("a bound that meters no rate has no quiet ceiling", func(t *testing.T) {
		// The listen ceiling's shape: it refuses a held resource rather than a
		// rate, so there is no rate a quiet population could sit on top of.
		plan := fairnessPlan{Bound: boundSpec{}, Quiet: populationSpec{Rate: 1000, Verbs: []string{verbCall}}}
		if err := plan.quietIsQuiet(); err != nil {
			t.Errorf("quietIsQuiet = %v, want no ceiling where the bound meters no rate", err)
		}
	})
	// Under the cap the rate is a quarter of what the bound meters, spread over
	// the share of the quiet verbs it meters: a bound metering four a second,
	// every quiet verb of it, leaves a quiet tenant one.
	t.Run("a quarter of what the bound meters", func(t *testing.T) {
		bound := boundSpec{
			Bucket:     &bucketSpec{Rate: 4, Burst: 40},
			Refusals:   []refusalSpec{{Status: httpOK, TextPrefix: "x", Method: methodToolsCall}},
			QuietVerbs: []string{verbCall},
		}
		if got := bound.quietRate(); got != 1 {
			t.Errorf("quietRate = %g, want a quarter of the 4 a second the bound meters", got)
		}
	})
	t.Run("a bound whose quiet verbs it never meters", func(t *testing.T) {
		unmetered := boundSpec{
			Bucket:     &bucketSpec{Rate: 10, Burst: 40},
			Refusals:   []refusalSpec{{Status: httpOK, TextPrefix: "x", Method: methodSubscriptionsListen}},
			QuietVerbs: []string{verbCall},
		}
		if got := unmetered.quietRate(); got != defaultQuietRate {
			t.Errorf("quietRate = %g, want the shipped default when the bound meters none of the quiet verbs", got)
		}
	})
	t.Run("and the plan takes it when the flag names none", func(t *testing.T) {
		opts := fairnessOptions()
		opts.fairness = "tools-list-rps"
		opts.fairnessQuietRate = 0
		opts.fairnessNoisyRate = 20
		// The shipped phase, because a quiet rate derived from a bound metered
		// once a second is slow enough that a shorter one cannot fill a
		// percentile, which is the trade the derivation makes.
		opts.fairnessPhase = defaultFairnessPhase
		plan, err := fairnessPlanFor(opts)
		if err != nil {
			t.Fatalf("fairnessPlanFor: %v", err)
		}
		bound, _ := boundByID("tools-list-rps")
		if plan.Quiet.Rate != bound.quietRate() {
			t.Errorf("quiet rate = %g, want the %g the bound names", plan.Quiet.Rate, bound.quietRate())
		}
	})
}

// TestFairnessPlan_RefusalsExpected_IsWhatThePopulationOffersAboveTheBound
// verifies the arithmetic the positive control weighs an arm against, which is
// what makes that control a quantity rather than a presence.
//
// A bound that fired once in three thousand requests and a bound absent from
// the build are the same arm to a control that only asks whether anything was
// refused.
func TestFairnessPlan_RefusalsExpected_IsWhatThePopulationOffersAboveTheBound(t *testing.T) {
	bucket, err := boundByID("tools-call-rps")
	if err != nil {
		t.Fatalf("boundByID: %v", err)
	}
	plan := fairnessPlan{Bound: bucket, Phase: 10 * time.Second}
	cases := []struct {
		name string
		pop  populationSpec
		want float64
	}{
		{
			name: "twice the bound, four credentials, ten seconds",
			pop:  populationSpec{Credentials: 4, Rate: 20, Verbs: []string{verbCall}},
			want: 400,
		},
		{
			name: "half its ticks metered halves what it offers",
			pop:  populationSpec{Credentials: 4, Rate: 20, Verbs: []string{verbCall, verbList}},
			want: 0,
		},
		{
			name: "a population under the bound offers nothing above it",
			pop:  populationSpec{Credentials: 4, Rate: 5, Verbs: []string{verbCall}},
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := plan.refusalsExpected(tc.pop); got != tc.want {
				t.Errorf("refusalsExpected = %g, want %g", got, tc.want)
			}
			if floor := refusalFloor(fairnessPlan{Bound: plan.Bound, Phase: plan.Phase, Noisy: tc.pop}); floor < 1 {
				t.Errorf("refusalFloor = %g, want a bound to be held to having fired at all", floor)
			}
		})
	}
	t.Run("a bound with no bucket expects none", func(t *testing.T) {
		none := fairnessPlan{Bound: boundSpec{}, Phase: time.Second}
		if got := none.refusalsExpected(populationSpec{Credentials: 4, Rate: 20}); got != 0 {
			t.Errorf("refusalsExpected = %g, want nothing derivable without a bucket", got)
		}
	})
}

// TestBoundSpec_OnArgs_AreWrittenFromTheBucketTheValidationReads verifies the
// switches the server is given and the numbers the plan is validated against
// are one declaration.
//
// A bound whose flags said ten and whose validation believed twenty would pass
// a lead-in check the run then failed, and the failure would look like the
// server's.
func TestBoundSpec_OnArgs_AreWrittenFromTheBucketTheValidationReads(t *testing.T) {
	for _, bound := range fairnessBounds {
		t.Run(bound.ID, func(t *testing.T) {
			args := strings.Join(bound.onArgs(), " ")
			if bound.Bucket == nil {
				if args != strings.Join(bound.ArgsOn, " ") {
					t.Errorf("onArgs = %q, want the declared switches", args)
				}
				return
			}
			want := fmt.Sprintf("--rate-limit-rps=%g --rate-limit-burst=%d", bound.Bucket.Rate, bound.Bucket.Burst)
			if args != want {
				t.Errorf("onArgs = %q, want %q", args, want)
			}
		})
	}
}

// TestPopulationSpec_Validate_RefusesAVerbThatDoesNotExist verifies a
// population is held to the verb table, which no flag can reach and a future
// bound literal can.
func TestPopulationSpec_Validate_RefusesAVerbThatDoesNotExist(t *testing.T) {
	cases := []struct {
		name string
		spec populationSpec
		want string
	}{
		{
			name: "no verbs at all",
			spec: populationSpec{Name: populationQuiet, Credentials: 1, Rate: 1},
			want: "no verbs to issue",
		},
		{
			name: "a verb the table does not hold",
			spec: populationSpec{Name: populationQuiet, Credentials: 1, Rate: 1, Verbs: []string{"subscribe"}},
			want: `unknown verb "subscribe"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("validate = %v, want an error about %q", err, tc.want)
			}
		})
	}
}

// TestFairnessPlan_Schedule_IsOpenLoopAndBoundedByTheDeadline verifies the
// derived schedule: the period follows the rate, the tick counts follow the
// windows, and the in-flight ceiling sits above what one deadline can
// accumulate.
//
// The ceiling is the load-bearing one. A ceiling that bound under load would
// censor exactly the slow requests, and would free its slots faster in the arm
// where refusals return in two milliseconds, which is the closed loop this
// driver exists to avoid wearing an open loop's clothes.
func TestFairnessPlan_Schedule_IsOpenLoopAndBoundedByTheDeadline(t *testing.T) {
	plan, err := fairnessPlanFor(fairnessOptions())
	if err != nil {
		t.Fatalf("fairnessPlanFor: %v", err)
	}
	if got, want := plan.Quiet.period(), 500*time.Millisecond; got != want {
		t.Errorf("quiet period = %s, want %s", got, want)
	}
	if got, want := plan.ticks(plan.Quiet), 4; got != want {
		t.Errorf("quiet ticks = %d, want %d", got, want)
	}
	if got, want := plan.leadInTicks(plan.Noisy), 160; got != want {
		t.Errorf("noisy lead-in ticks = %d, want %d", got, want)
	}
	outstanding := plan.Noisy.Rate * plan.Deadline.Seconds()
	if got := plan.inFlight(plan.Noisy); float64(got) <= outstanding {
		t.Errorf("in-flight ceiling %d does not sit above the %.0f a deadline can accumulate", got, outstanding)
	}
	t.Run("a rate below one request per deadline still gets a slot", func(t *testing.T) {
		slow := fairnessPlan{Deadline: time.Millisecond}
		if got := slow.inFlight(populationSpec{Rate: 0.001}); got < 1 {
			t.Errorf("in-flight ceiling = %d, want at least one", got)
		}
	})
}

// TestArmOrder_AlternatesBetweenRepetitions verifies the arms are
// counterbalanced, which is what turns a monotone drift of the host into
// spread the verdict can see rather than a bias it cannot.
func TestArmOrder_AlternatesBetweenRepetitions(t *testing.T) {
	cases := map[int][]string{0: {armOff, armOn}, 1: {armOn, armOff}, 2: {armOff, armOn}}
	for repeat, want := range cases {
		t.Run(fmt.Sprintf("repeat %d", repeat), func(t *testing.T) {
			got := armOrder(repeat)
			if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
				t.Errorf("armOrder(%d) = %v, want %v", repeat, got, want)
			}
		})
	}
}

// TestFairnessPlan_ArmSwitches_ComeFromTheBound verifies each arm is put in its
// state by the bound's own declaration, so a second bound is a literal rather
// than a branch.
func TestFairnessPlan_ArmSwitches_ComeFromTheBound(t *testing.T) {
	plan := fairnessPlan{Bound: boundSpec{
		ArgsOff: []string{"--off"}, ArgsOn: []string{"--on"},
		EnvOff: []string{"A=0"}, EnvOn: []string{"A=1"},
	}}
	cases := map[string]struct{ arg, env string }{
		armOff: {arg: "--off", env: "A=0"},
		armOn:  {arg: "--on", env: "A=1"},
	}
	for arm, want := range cases {
		t.Run(arm, func(t *testing.T) {
			if got := plan.armArgs(arm); len(got) != 1 || got[0] != want.arg {
				t.Errorf("armArgs(%q) = %v, want [%s]", arm, got, want.arg)
			}
			if got := plan.armEnv(arm); len(got) != 1 || got[0] != want.env {
				t.Errorf("armEnv(%q) = %v, want [%s]", arm, got, want.env)
			}
		})
	}
}

// TestVerbs_BuildFreshParametersPerRequest verifies a verb hands out a new
// parameter map every time.
//
// The encoder writes the per-request _meta into the map it is given, so a
// shared map would be written by every in-flight goroutine at once: a data race
// on the hot path of a scenario whose whole point is running many requests in
// parallel.
func TestVerbs_BuildFreshParametersPerRequest(t *testing.T) {
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	t.Run(verbCall, func(t *testing.T) {
		first, second := verbs[verbCall].params(call), verbs[verbCall].params(call)
		first["_meta"] = "written by one request"
		if _, leaked := second["_meta"]; leaked {
			t.Error("two requests were handed the same parameter map")
		}
		if verbs[verbCall].detail(call) != call.Detail {
			t.Errorf("detail = %q, want the tool the surface calls", verbs[verbCall].detail(call))
		}
	})
	t.Run(verbList, func(t *testing.T) {
		if got := verbs[verbList].params(call); got != nil {
			t.Errorf("params = %v, want none for a listing", got)
		}
		if got := verbs[verbList].detail(call); got != detailWholeSurface {
			t.Errorf("detail = %q, want %q", got, detailWholeSurface)
		}
	})
}

// oauthOptions are the flags of an OAuth verification run that typed nothing
// but the bound, so every setting the bound has a default for takes it.
func oauthOptions() options {
	opts := fairnessOptions()
	opts.fairness = "oauth-verification"
	return opts
}

// oauthPlan is the plan oauthOptions build, which the bound's defaults make a
// valid one.
func oauthPlan(t *testing.T) fairnessPlan {
	t.Helper()
	plan, err := fairnessPlanFor(oauthOptions())
	if err != nil {
		t.Fatalf("fairnessPlanFor: %v", err)
	}
	return plan
}

// TestRowKey_QualifiesOnlyACredentialThatIsNotTheLanesOwn verifies a row is
// named by its method alone unless it presented another credential, which is
// what keeps every record the bounds before this one wrote reading as it did.
func TestRowKey_QualifiesOnlyACredentialThatIsNotTheLanesOwn(t *testing.T) {
	cases := map[string]string{
		verbCall:         methodToolsCall,
		verbList:         methodToolsList,
		verbCallNew:      "tools/call (new credential)",
		verbListInvented: "tools/list (invented credential)",
	}
	for id, want := range cases {
		t.Run(id, func(t *testing.T) {
			verb := verbs[id]
			if got := verb.key(); got != want {
				t.Errorf("key = %q, want %q", got, want)
			}
			row := FairnessMethod{Method: verb.Method, Credential: verb.Credential}
			if got := row.label(); got != want {
				t.Errorf("label = %q, want %q", got, want)
			}
			if got := (FairnessComparison{Method: verb.Method, Credential: verb.Credential}).label(); got != want {
				t.Errorf("comparison label = %q, want %q", got, want)
			}
		})
	}
}

// TestRegisterRefusal_ReadsTheShapeTheRegisterDeclares verifies the
// verification bound's two refusal shapes come from the register rows that
// declare them, and that a row or status the register does not hold gives a
// shape that matches nothing.
func TestRegisterRefusal_ReadsTheShapeTheRegisterDeclares(t *testing.T) {
	busy := registerRefusal("ADM-014", httpServiceUnavailable)
	if busy.Status != httpServiceUnavailable || busy.Code != tenancy.CodeUnavailable ||
		busy.TextPrefix != "GitLab could not verify this token right now." {
		t.Errorf("ADM-014 = %+v, want the register's 503, -50300 and words", busy)
	}
	rejected := registerRefusal("ADM-002", httpUnauthorized)
	if rejected.Status != httpUnauthorized || rejected.Code != tenancy.CodeUnauthorized ||
		rejected.TextPrefix != "GitLab rejected this token." {
		t.Errorf("ADM-002 at 401 = %+v, want the register's rejection", rejected)
	}
	// ADM-002 refuses twice at 403, once in words built at run time and once in
	// words that fold; only the second is a shape a classifier can match.
	if scope := registerRefusal("ADM-002", 403); scope.TextPrefix != "GitLab rejected this token for lacking the scope" {
		t.Errorf("ADM-002 at 403 = %+v, want the refusal whose words fold", scope)
	}
	for name, got := range map[string]refusalSpec{
		"a row the register does not hold": registerRefusal("ADM-999", httpUnauthorized),
		"a status the row does not refuse": registerRefusal("ADM-014", httpUnauthorized),
	} {
		t.Run(name, func(t *testing.T) {
			if got != (refusalSpec{}) {
				t.Errorf("registerRefusal = %+v, want the zero shape", got)
			}
		})
	}
}

// TestFairnessPlanFor_TakesTheBoundsDefaultsWhereNoFlagWasTyped verifies a
// bound's own defaults stand wherever the caller typed nothing, and that a
// flag the caller typed wins over them.
//
// The verification bound cannot be driven by the command's defaults at all: a
// two-second deadline counts every request that waited out the five-second
// slot wait as one the client gave up on, and eighty invented tokens a second
// never fill slots that finish a hundred and sixty. So `make bench-fairness
// BOUND=oauth-verification` would refuse to run without them.
func TestFairnessPlanFor_TakesTheBoundsDefaultsWhereNoFlagWasTyped(t *testing.T) {
	plan := oauthPlan(t)
	if plan.Noisy.Credentials != 8 || plan.Noisy.Rate != 50 || plan.Phase != 30*time.Second ||
		plan.LeadIn != 10*time.Second || plan.Deadline != 15*time.Second || plan.UpstreamDelay != 100*time.Millisecond {
		t.Errorf("plan = %+v, want the bound's own defaults", plan)
	}
	// The bound has no default of its own for the quiet population's size, so
	// the flag's value stands.
	if plan.Quiet.Credentials != 2 {
		t.Errorf("quiet credentials = %d, want the flag's 2", plan.Quiet.Credentials)
	}

	opts := oauthOptions()
	opts.fairnessSet = map[string]bool{
		flagFairnessNoisy: true, flagFairnessNoisyRate: true, flagFairnessPhase: true,
		flagFairnessLeadIn: true, flagFairnessDeadline: true, flagFairnessQuiet: true,
	}
	opts.fairnessNoisy, opts.fairnessNoisyRate = 20, 30
	// Six hundred invented tokens a second against the three hundred and
	// twenty the slots finish at fifty milliseconds settle in under eleven
	// seconds, which the typed lead-in outlasts.
	opts.fairnessPhase, opts.fairnessLeadIn, opts.fairnessDeadline = 5*time.Second, 12*time.Second, 9*time.Second
	opts.fairnessUpstreamDelay = 50 * time.Millisecond
	typed, err := fairnessPlanFor(opts)
	if err != nil {
		t.Fatalf("fairnessPlanFor: %v", err)
	}
	if typed.Noisy.Credentials != 20 || typed.Noisy.Rate != 30 || typed.Phase != 5*time.Second ||
		typed.LeadIn != 12*time.Second || typed.Deadline != 9*time.Second || typed.UpstreamDelay != 50*time.Millisecond {
		t.Errorf("plan = %+v, want every typed flag to win", typed)
	}
}

// TestParseFlags_OAuthVerificationDefaults_MakeAPlanThatValidates verifies the
// Makefile's `make bench-fairness BOUND=oauth-verification` describes a run
// that measures something, and that a flag typed on the command line is the
// one that counts.
func TestParseFlags_OAuthVerificationDefaults_MakeAPlanThatValidates(t *testing.T) {
	if _, err := fairnessPlanFor(withArgs(t, "-fairness=oauth-verification")); err != nil {
		t.Errorf("fairnessPlanFor over the bound's defaults = %v, want a plan they can run", err)
	}
	opts := withArgs(t, "-fairness=oauth-verification", "-fairness-noisy=2", "-json=x.json")
	if !opts.fairnessSet[flagFairnessNoisy] || opts.fairnessSet["json"] {
		t.Errorf("fairnessSet = %v, want the fairness flag typed and nothing else", opts.fairnessSet)
	}
	// Two credentials at fifty a second is a hundred, under the hundred and
	// sixty the slots finish: the typed value won and the plan says why it
	// cannot be run.
	if _, err := fairnessPlanFor(opts); err == nil || !strings.Contains(err.Error(), "never all be held") {
		t.Errorf("fairnessPlanFor = %v, want the typed flood refused as too small", err)
	}
}

// TestFairnessPlan_SlotsFill_RefusesAPlanThatCouldNotFillTheSlots verifies
// every way a slot bound's plan is refused before a process is started, and
// that each limit admits the value sitting exactly on it.
func TestFairnessPlan_SlotsFill_RefusesAPlanThatCouldNotFillTheSlots(t *testing.T) {
	cases := []struct {
		name string
		edit func(*fairnessPlan)
		want string
	}{
		{name: "no round trip at the instance", edit: func(p *fairnessPlan) { p.UpstreamDelay = 0 }, want: "-fairness-upstream-delay"},
		// Sixteen slots at a hundred milliseconds finish a hundred and sixty,
		// which eight credentials at twenty a second offer exactly.
		{name: "a flood the slots finish as it arrives", edit: func(p *fairnessPlan) { p.Noisy.Rate = 20 }, want: "never all be held"},
		// Four hundred invented tokens a second against the hundred and sixty
		// the slots finish settle in five seconds times 400/240, so eight
		// seconds, which outlasts the wait, is still too short.
		{name: "a lead-in the queue has not settled in", edit: func(p *fairnessPlan) { p.LeadIn = 8 * time.Second }, want: "the 8.333s a flood of 400 requests a second takes to settle"},
		// Four hundred and eighty against a hundred and sixty settle in five
		// seconds times 480/320, seven and a half, so a millisecond under it
		// is refused.
		{name: "a lead-in a millisecond under the queue's settling", edit: func(p *fairnessPlan) {
			p.Noisy.Rate, p.LeadIn = 60, 7499*time.Millisecond
		}, want: "the 7.5s a flood of 480 requests"},
		{name: "a deadline shorter than a waiter's", edit: func(p *fairnessPlan) { p.Deadline = 6 * time.Second }, want: "gave up"},
		// The floor is the wait, four round trips of a hundred milliseconds
		// and the second of margin, so a millisecond under it is refused.
		{name: "a deadline a millisecond under its floor", edit: func(p *fairnessPlan) { p.Deadline = 6399 * time.Millisecond }, want: "the 6.4s a new credential"},
		{
			name: "a quiet population whose onboarding fills the slots itself",
			edit: func(p *fairnessPlan) { p.Quiet.Credentials, p.Quiet.Rate = 100, 3 },
			want: "busy on their own, above the 8 this comparison allows it",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := oauthPlan(t)
			tc.edit(&plan)
			if err := plan.validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("validate = %v, want an error about %q", err, tc.want)
			}
		})
	}

	boundaries := []struct {
		name string
		edit func(*fairnessPlan)
	}{
		{name: "a lead-in exactly as long as the queue takes to settle", edit: func(p *fairnessPlan) {
			p.Noisy.Rate, p.LeadIn = 60, 7500*time.Millisecond
		}},
		// The wait, four round trips of a hundred milliseconds and the second
		// of margin.
		{name: "a deadline exactly on its floor", edit: func(p *fairnessPlan) { p.Deadline = 6400 * time.Millisecond }},
		// Forty credentials at four a second, one request in four new, present
		// forty new credentials a second; at two round trips of a tenth of a
		// second each they keep exactly the eight slots a quiet tenant may. The
		// slots are a copy, since the plan's points at the bound table's own.
		{name: "a quiet onboarding exactly at its share", edit: func(p *fairnessPlan) {
			slots := *p.Bound.Slots
			slots.NewRoundTrips = 2
			p.Bound.Slots = &slots
			p.Quiet.Credentials, p.Quiet.Rate = 40, 4
		}},
	}
	for _, tc := range boundaries {
		t.Run(tc.name, func(t *testing.T) {
			plan := oauthPlan(t)
			tc.edit(&plan)
			if err := plan.validate(); err != nil {
				t.Errorf("validate = %v, want a plan on the boundary accepted", err)
			}
		})
	}
}

// TestFairnessPlan_Validate_RefusesAFloodNoLoopbackRangeCanCarry verifies a
// flood needing more transport sources than 127.2.0.0/16 holds is refused
// rather than sent through sources that wrap around onto each other.
func TestFairnessPlan_Validate_RefusesAFloodNoLoopbackRangeCanCarry(t *testing.T) {
	plan := oauthPlan(t)
	plan.Noisy.Rate = 1e6
	if err := plan.validate(); err == nil || !strings.Contains(err.Error(), "127.2.0.0/16") {
		t.Errorf("validate = %v, want the flood refused for its sources", err)
	}

	// One credential at 273058 invented tokens a second is 16383480 a minute,
	// which at 250 a source needs 65533.92 sources: exactly the 65534 the range
	// holds, and one more token a second needs one more.
	cases := []struct {
		name    string
		rate    float64
		sources int
		refused bool
	}{
		{name: "a flood filling the range exactly", rate: 273058, sources: maxFloodSources},
		{name: "a flood one source past it", rate: 273059, sources: maxFloodSources + 1, refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edge := oauthPlan(t)
			edge.Noisy.Credentials, edge.Noisy.Rate = 1, tc.rate
			if got := edge.floodSources(); got != tc.sources {
				t.Fatalf("floodSources = %d, want %d", got, tc.sources)
			}
			err := edge.validate()
			if refused := err != nil && strings.Contains(err.Error(), "127.2.0.0/16"); refused != tc.refused {
				t.Errorf("validate = %v, want refused for its sources: %t", err, tc.refused)
			}
		})
	}
}

// TestFairnessPlan_SlotArithmetic_FollowsTheFlood verifies the figures a slot
// bound's run is sized from: the share of a cycle that presents a credential,
// the flood's transport sources, the pool, and the refusals the positive
// control demands.
func TestFairnessPlan_SlotArithmetic_FollowsTheFlood(t *testing.T) {
	plan := oauthPlan(t)
	if got := credentialShare(plan.Quiet.Verbs, credentialNew); got != 0.25 {
		t.Errorf("new credential share = %g, want one in four", got)
	}
	if got := credentialShare(nil, credentialNew); got != 0 {
		t.Errorf("share of no verbs = %g, want zero", got)
	}
	// Four hundred a second over a minute is twenty-four thousand distinct
	// addresses, at two hundred and fifty a source.
	if got := plan.floodSources(); got != 96 {
		t.Errorf("floodSources = %d, want 96", got)
	}
	if got := (fairnessPlan{Noisy: populationSpec{Credentials: 1, Rate: 1, Verbs: []string{verbList}}}).floodSources(); got != 0 {
		t.Errorf("floodSources of a flood of own credentials = %d, want none", got)
	}
	// A flood that invents a token on every other request fails half as many
	// distinct credentials, and needs half the sources.
	half := plan
	half.Noisy.Verbs = []string{verbListInvented, verbList}
	if got := half.floodSources(); got != 48 {
		t.Errorf("floodSources of a flood inventing every other token = %d, want 48", got)
	}
	// Two quiet credentials at two a second over forty seconds present eighty
	// requests each, a quarter of them new: forty new credentials beside the
	// ten the populations hold.
	if got := plan.poolEntries(); got != 50 {
		t.Errorf("poolEntries = %d, want 50", got)
	}
	huge := plan
	huge.Quiet.Credentials, huge.Phase = 10000, time.Hour
	if got := huge.poolEntries(); got != tenancy.PoolSizeMax {
		t.Errorf("poolEntries = %d, want it held to the largest pool the server accepts", got)
	}
	// Four hundred offered against a hundred and sixty finished, for thirty
	// seconds.
	if got := plan.refusalsExpected(plan.Noisy); got != 7200 {
		t.Errorf("refusalsExpected = %g, want 7200", got)
	}
	if got := plan.refusalsExpected(plan.Quiet); got != 0 {
		t.Errorf("refusalsExpected of the quiet population = %g, want none", got)
	}
	if got := (slotSpec{Count: 16, InventedRoundTrips: 2}).capacity(100 * time.Millisecond); got != 80 {
		t.Errorf("capacity = %g, want sixteen slots over two round trips of a tenth of a second", got)
	}
	if got := plan.describe(); !strings.Contains(got, "100ms at the instance") || !strings.Contains(got, "96 transport sources") {
		t.Errorf("describe = %q, want the round trip and the sources named", got)
	}
	// A bound with no instance round trip and no invented tokens says neither.
	bucket, err := fairnessPlanFor(withArgs(t, "-fairness=tools-call-rps"))
	if err != nil {
		t.Fatalf("fairnessPlanFor: %v", err)
	}
	if got := bucket.describe(); strings.Contains(got, "at the instance") || strings.Contains(got, "transport sources") {
		t.Errorf("describe = %q, want no round trip and no sources named for a bucket bound", got)
	}
}

// TestClassifyExpecting_FilesTheRunsOwnRefusalsApart verifies a refusal the
// run expects and the bound does not own is neither the bound's nor a failure,
// and that the order of the checks leaves every other outcome where it was.
func TestClassifyExpecting_FilesTheRunsOwnRefusalsApart(t *testing.T) {
	bound := oauthPlan(t).Bound
	gate := func(status, code int, message string) error {
		return fmt.Errorf("tools/list: %w", &httpStatusError{
			Method: methodToolsList, Status: status, RPC: &rpcError{Code: code, Message: message},
		})
	}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "an invented token's 401", want: outcomeRefusedOther,
			err: gate(httpUnauthorized, tenancy.CodeUnauthorized, "GitLab rejected this token. Check that it is valid."),
		},
		{
			name: "a slot that never freed", want: outcomeRefused,
			err: gate(httpServiceUnavailable, tenancy.CodeUnavailable, "GitLab could not verify this token right now. Retry shortly."),
		},
		{
			name: "an instance that did not answer is neither", want: outcomeFailed,
			err: gate(httpServiceUnavailable, tenancy.CodeUnavailable, "GitLab could not verify this token right now; the instance is unreachable."),
		},
		{name: "a client that gave up", err: context.DeadlineExceeded, want: outcomeTimedOut},
		{name: "a served request", want: outcomeServed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyExpecting(methodToolsList, tc.err, bound.Refusals, bound.Otherwise); got != tc.want {
				t.Errorf("classifyExpecting = %q, want %q", got, tc.want)
			}
		})
	}
}
