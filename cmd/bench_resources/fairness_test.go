package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
			name: "a refused listing carries the code instead", method: methodToolsList,
			err: fmt.Errorf("tools/list: %w", rpcError{
				Code: rateLimitCode, Message: toolutil.RateLimitRefusalPrefix + "tools/list; retry after a short backoff",
			}),
			refusals: listing.Refusals, want: outcomeRefused,
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
