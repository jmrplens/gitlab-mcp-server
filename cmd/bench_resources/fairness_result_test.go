package main

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// methodFixture is one population's record for one method, spelled so a test
// can state only the number it is about.
type methodFixture struct {
	method      string
	intended    int
	dropped     int
	served      int
	refused     int
	failed      int
	timedOut    int
	p50, p99    float64
	latenessP99 float64
}

// quietAt is a quiet record with enough served samples for a p99 and nothing
// unusual about it.
func quietAt(method string, p99 float64) methodFixture {
	return methodFixture{method: method, intended: 200, served: 200, p50: p99 / 2, p99: p99, latenessP99: 0.1}
}

// noisyAt is a noisy record: dispatched is what matters about it, since it is
// the work the two arms have to put in front of the server equally.
func noisyAt(served, refused int) methodFixture {
	return methodFixture{method: methodToolsCall, intended: served + refused, served: served, refused: refused, p99: 30}
}

// build renders a fixture as the record the verdict reads.
func (f methodFixture) build() FairnessMethod {
	out := FairnessMethod{
		Method: f.method, Intended: f.intended, Dropped: f.dropped,
		Served: f.served, Refused: f.refused, Failed: f.failed, TimedOut: f.timedOut,
		ServedLatency:  MethodLatency{Method: f.method, Count: f.served, P50: f.p50, P99: f.p99},
		RefusedLatency: MethodLatency{Method: f.method, Count: f.refused},
		Lateness:       MethodLatency{Method: f.method, Count: f.served, P99: f.latenessP99},
	}
	out.Dispatched = out.Served + out.Refused + out.Failed + out.TimedOut
	return out
}

// armFixture is one arm of a repetition.
type armFixture struct {
	arm       string
	coresBusy float64
	quiet     []methodFixture
	noisy     []methodFixture
	notes     []string
	// dropQuiet removes the quiet population altogether, for the paths that
	// answer when the arms did not drive the same tenants.
	dropQuiet bool
}

// build renders an arm fixture.
func (f armFixture) build() FairnessArm {
	arm := FairnessArm{
		Arm: f.arm, Comparable: len(f.notes) == 0, Notes: f.notes,
		Process: FairnessProcess{CoresBusy: f.coresBusy},
	}
	if !f.dropQuiet {
		quiet := FairnessPopulation{Name: populationQuiet, Credentials: 8, RatePerCredential: 2}
		for _, method := range f.quiet {
			quiet.Methods = append(quiet.Methods, method.build())
		}
		arm.Populations = append(arm.Populations, quiet)
	}
	noisy := FairnessPopulation{Name: populationNoisy, Credentials: 4, RatePerCredential: 20}
	for _, method := range f.noisy {
		noisy.Methods = append(noisy.Methods, method.build())
	}
	arm.Populations = append(arm.Populations, noisy)
	return arm
}

// fairnessDocOf builds a document from one arm fixture per arm per repetition.
func fairnessDocOf(cpus int, repeats ...[]armFixture) *FairnessDoc {
	doc := &FairnessDoc{Schema: fairnessSchema, Host: HostInfo{CPUs: cpus}}
	for index, arms := range repeats {
		repeat := FairnessRepeat{Index: index, Order: armOrder(index)}
		for _, arm := range arms {
			repeat.Arms = append(repeat.Arms, arm.build())
		}
		doc.Repeats = append(doc.Repeats, repeat)
	}
	return doc
}

// saturated is an arm that kept the host busy enough for the comparison to
// mean anything, with the quiet tools/call at the given tail.
func saturated(arm string, quietP99 float64) armFixture {
	return atLoad(arm, 6, quietP99)
}

// atLoad is the same arm with the host busyness named, for the cases about a
// machine nothing contended for.
func atLoad(arm string, coresBusy, quietP99 float64) armFixture {
	return armFixture{
		arm: arm, coresBusy: coresBusy,
		quiet: []methodFixture{quietAt(methodToolsCall, quietP99)},
		noisy: []methodFixture{noisyAt(1000, 0)},
	}
}

// TestJudgeFairness_ReportsBetterOnlyWhenTheQuietTenantReallyIs verifies the
// verdict over every answer it can give, and in particular that it can say no.
//
// The cases are the ways a fairness report lies if nobody guards them. A bound
// that refuses the quiet tenant improves its surviving percentiles by killing
// the slow work; a host nothing contended for cannot show a per-credential
// bound helping anybody; arms that offered different work are two experiments;
// and a driver whose own dispatch lateness moved by as much as the improvement
// claimed is as likely the cause as the server.
func TestJudgeFairness_ReportsBetterOnlyWhenTheQuietTenantReallyIs(t *testing.T) {
	cases := []struct {
		name          string
		doc           *FairnessDoc
		wantDirection string
		wantReason    string
	}{
		{
			name:          "the quiet tail fell in every repetition, beyond their spread",
			doc:           fairnessDocOf(8, []armFixture{saturated(armOff, 20), saturated(armOn, 10)}, []armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionBetter,
			wantReason:    "fell by 10.000 ms",
		},
		{
			name:          "the quiet tail rose in every repetition",
			doc:           fairnessDocOf(8, []armFixture{saturated(armOff, 10), saturated(armOn, 20)}, []armFixture{saturated(armOn, 20), saturated(armOff, 10)}),
			wantDirection: directionWorse,
			wantReason:    "rose by 10.000 ms",
		},
		{
			name:          "the repetitions disagreed about which arm was faster",
			doc:           fairnessDocOf(8, []armFixture{saturated(armOff, 20), saturated(armOn, 10)}, []armFixture{saturated(armOn, 20), saturated(armOff, 10)}),
			wantDirection: directionIndistinguishable,
			wantReason:    "disagreed in sign",
		},
		{
			name:          "the difference is inside the spread between repetitions",
			doc:           fairnessDocOf(8, []armFixture{saturated(armOff, 21), saturated(armOn, 20)}, []armFixture{saturated(armOn, 10), saturated(armOff, 30)}),
			wantDirection: directionIndistinguishable,
			wantReason:    "is inside the",
		},
		{
			name: "the driver's own lateness moved by as much as the improvement",
			doc: func() *FairnessDoc {
				doc := fairnessDocOf(8,
					[]armFixture{saturated(armOff, 20), saturated(armOn, 10)},
					[]armFixture{saturated(armOn, 10), saturated(armOff, 20)})
				for _, repeat := range doc.Repeats {
					for i := range repeat.Arms {
						if repeat.Arms[i].Arm == armOff {
							repeat.Arms[i].Populations[0].Methods[0].Lateness.P99 = 12
						}
					}
				}
				return doc
			}(),
			wantDirection: directionNotComparable,
			wantReason:    "dispatch lateness moved by",
		},
		{
			name: "one repetition carries no measure of host noise",
			doc:  fairnessDocOf(8, []armFixture{saturated(armOff, 20), saturated(armOn, 10)}),

			wantDirection: directionIndistinguishable,
			wantReason:    "one repetition carries no measure",
		},
		{
			name: "the host was never contended for",
			doc: fairnessDocOf(8,
				[]armFixture{atLoad(armOff, 0.2, 20), atLoad(armOn, 0.2, 10)},
				[]armFixture{atLoad(armOn, 0.2, 10), atLoad(armOff, 0.2, 20)}),
			wantDirection: directionIndistinguishable,
			wantReason:    "kept only 0.20 of the host's 8 cores busy",
		},
		{
			name: "an arm that failed a control",
			doc: fairnessDocOf(8,
				[]armFixture{{arm: armOff, notes: []string{"something else refused"}}, saturated(armOn, 10)},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionNotComparable,
			wantReason:    "something else refused",
		},
		{
			name:          "a repetition that ran one arm",
			doc:           fairnessDocOf(8, []armFixture{saturated(armOff, 20)}, []armFixture{saturated(armOff, 20), saturated(armOn, 10)}),
			wantDirection: directionNotComparable,
			wantReason:    "did not run both arms",
		},
		{
			name: "the arms did not put the same work in front of the server",
			doc: fairnessDocOf(8,
				[]armFixture{
					{arm: armOff, coresBusy: 6, quiet: []methodFixture{quietAt(methodToolsCall, 20)}, noisy: []methodFixture{noisyAt(1000, 0)}},
					{arm: armOn, coresBusy: 6, quiet: []methodFixture{quietAt(methodToolsCall, 10)}, noisy: []methodFixture{noisyAt(400, 0)}},
				},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionNotComparable,
			wantReason:    "did not offer the server the same work",
		},
		{
			name: "the driver could not send the quiet population's own schedule",
			doc: fairnessDocOf(8,
				[]armFixture{
					{arm: armOff, coresBusy: 6, quiet: []methodFixture{{method: methodToolsCall, intended: 200, dropped: 40, served: 160, p99: 20}}, noisy: []methodFixture{noisyAt(1000, 0)}},
					saturated(armOn, 10),
				},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionNotComparable,
			wantReason:    "could not send 40 of the quiet population's",
		},
		{
			// Dropped from the arm the comparison reads second, so it is only
			// caught because the check runs both ways round.
			name: "the arms did not drive the same populations",
			doc: fairnessDocOf(8,
				[]armFixture{saturated(armOff, 20), {arm: armOn, coresBusy: 6, dropQuiet: true, noisy: []methodFixture{noisyAt(1000, 0)}}},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionNotComparable,
			wantReason:    "did not drive the same populations",
		},
		{
			name: "and the same the other way round",
			doc: fairnessDocOf(8,
				[]armFixture{{arm: armOff, coresBusy: 6, dropQuiet: true, noisy: []methodFixture{noisyAt(1000, 0)}}, saturated(armOn, 10)},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionNotComparable,
			wantReason:    "did not drive the same populations",
		},
		{
			name: "the arms did not drive the same methods",
			doc: fairnessDocOf(8,
				[]armFixture{
					{arm: armOff, coresBusy: 6, quiet: []methodFixture{quietAt(methodToolsList, 20)}, noisy: []methodFixture{noisyAt(1000, 0)}},
					saturated(armOn, 10),
				},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionNotComparable,
			wantReason:    "did not drive tools/list",
		},
		{
			name: "the bound refused the tenant it exists to protect",
			doc: fairnessDocOf(8,
				[]armFixture{
					saturated(armOff, 20),
					{arm: armOn, coresBusy: 6, quiet: []methodFixture{{method: methodToolsCall, intended: 200, served: 190, refused: 10, p99: 1}}, noisy: []methodFixture{noisyAt(1000, 0)}},
				},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionWorse,
			wantReason:    "the one it turned away",
		},
		{
			name: "the quiet tenant gave up on more requests with the bound on",
			doc: fairnessDocOf(8,
				[]armFixture{
					saturated(armOff, 20),
					{arm: armOn, coresBusy: 6, quiet: []methodFixture{{method: methodToolsCall, intended: 200, served: 190, timedOut: 10, p99: 1}}, noisy: []methodFixture{noisyAt(1000, 0)}},
				},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionWorse,
			wantReason:    "gave up on 10",
		},
		{
			name: "an arm whose schedule did not run to its end",
			doc: fairnessDocOf(8,
				[]armFixture{
					{
						arm: armOff, coresBusy: 6, notes: []string{"quiet tools/call: 150 dispatched against 200 offered less 0 dropped; the schedule did not run to its end"},
						quiet: []methodFixture{quietAt(methodToolsCall, 20)}, noisy: []methodFixture{noisyAt(1000, 0)},
					},
					saturated(armOn, 10),
				},
				[]armFixture{saturated(armOn, 10), saturated(armOff, 20)}),
			wantDirection: directionNotComparable,
			wantReason:    "did not run to its end",
		},
		{
			name: "the quiet population issued nothing at all",
			doc: fairnessDocOf(8,
				[]armFixture{{arm: armOff, coresBusy: 6, noisy: []methodFixture{noisyAt(1000, 0)}}, {arm: armOn, coresBusy: 6, noisy: []methodFixture{noisyAt(1000, 0)}}},
				[]armFixture{{arm: armOn, coresBusy: 6, noisy: []methodFixture{noisyAt(1000, 0)}}, {arm: armOff, coresBusy: 6, noisy: []methodFixture{noisyAt(1000, 0)}}}),
			wantDirection: directionNotComparable,
			wantReason:    "issued nothing to compare",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comparisons, verdict := judgeFairness(tc.doc)
			if verdict.Direction != tc.wantDirection {
				t.Errorf("direction = %q (%s), want %q", verdict.Direction, verdict.Reason, tc.wantDirection)
			}
			if !strings.Contains(verdict.Reason, tc.wantReason) {
				t.Errorf("reason = %q, want it to mention %q", verdict.Reason, tc.wantReason)
			}
			tc.doc.Comparisons = comparisons
			if _, err := json.Marshal(tc.doc); err != nil {
				t.Errorf("the verdict produced a document that cannot be written: %v", err)
			}
		})
	}
}

// TestQuietRegression_SkipsWhatItCannotCompare verifies the regression check
// passes over an arm or a method it has no counterpart for, leaving the
// comparability gates above it to answer for those.
func TestQuietRegression_SkipsWhatItCannotCompare(t *testing.T) {
	cases := []struct {
		name string
		doc  *FairnessDoc
	}{
		{
			name: "an arm with no quiet population at all",
			doc: fairnessDocOf(8, []armFixture{
				{arm: armOff, coresBusy: 6, dropQuiet: true},
				{arm: armOn, coresBusy: 6, dropQuiet: true},
			}),
		},
		{
			name: "a method only one arm issued",
			doc: fairnessDocOf(8, []armFixture{
				{arm: armOff, coresBusy: 6, quiet: []methodFixture{quietAt(methodToolsCall, 20)}},
				{arm: armOn, coresBusy: 6, quiet: []methodFixture{quietAt(methodToolsList, 10)}},
			}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quietRegression(tc.doc); got != "" {
				t.Errorf("quietRegression = %q, want nothing it cannot compare to be called a regression", got)
			}
		})
	}
}

// TestQuietMethods_AndArmMethod_ReadAroundWhatIsMissing verifies the two
// lookups the comparison is built on answer for a document with an arm, a
// population or a method it does not carry.
func TestQuietMethods_AndArmMethod_ReadAroundWhatIsMissing(t *testing.T) {
	doc := fairnessDocOf(8, []armFixture{
		{arm: armOff, coresBusy: 6, dropQuiet: true},
		{arm: armOn, coresBusy: 6, quiet: []methodFixture{quietAt(methodToolsCall, 10)}},
	})
	if got := quietMethods(doc); len(got) != 1 || got[0] != methodToolsCall {
		t.Errorf("quietMethods = %v, want the one method an arm carried", got)
	}
	cases := []struct {
		name        string
		arm, method string
		wantFound   bool
	}{
		{name: "an arm the repetition does not carry", arm: "sideways", method: methodToolsCall},
		{name: "an arm with no quiet population", arm: armOff, method: methodToolsCall},
		{name: "a method the population did not issue", arm: armOn, method: methodToolsList},
		{name: "one that is there", arm: armOn, method: methodToolsCall, wantFound: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, found := armMethod(doc.Repeats[0], tc.arm, tc.method)
			if found != tc.wantFound {
				t.Errorf("armMethod(%q, %q) found = %v, want %v", tc.arm, tc.method, found, tc.wantFound)
			}
		})
	}
}

// TestJudgeFairness_WorstAnswerWins verifies a bound that helped one method and
// hurt another has not helped.
func TestJudgeFairness_WorstAnswerWins(t *testing.T) {
	better := func(arm string, p99 float64) armFixture {
		f := saturated(arm, p99)
		f.quiet = append(f.quiet, quietAt(methodToolsList, p99))
		return f
	}
	doc := fairnessDocOf(8,
		[]armFixture{better(armOff, 20), better(armOn, 10)},
		[]armFixture{better(armOn, 10), better(armOff, 20)})
	// One method improves, the other degrades by the same amount.
	for _, repeat := range doc.Repeats {
		for i := range repeat.Arms {
			if repeat.Arms[i].Arm == armOn {
				repeat.Arms[i].Populations[0].Methods[1].ServedLatency.P99 = 40
			}
		}
	}
	comparisons, verdict := judgeFairness(doc)
	if len(comparisons) != 2 {
		t.Fatalf("compared %d methods, want both", len(comparisons))
	}
	if verdict.Direction != directionWorse {
		t.Errorf("direction = %q (%s), want %q", verdict.Direction, verdict.Reason, directionWorse)
	}
}

// TestCompareMethod_FallsBackToP50WhenAP99WouldBeTheMaximum verifies the
// compared percentile is chosen from the sample counts, and chosen once for
// every repetition.
//
// Nearest-rank p99 over a handful of observations is the maximum wearing a
// percentile's name, which is a number nobody measured.
func TestCompareMethod_FallsBackToP50WhenAP99WouldBeTheMaximum(t *testing.T) {
	thin := func(arm string, p50 float64) armFixture {
		return armFixture{
			arm: arm, coresBusy: 6,
			quiet: []methodFixture{{method: methodToolsCall, intended: 20, served: 20, p50: p50, p99: 999, latenessP99: 0.1}},
			noisy: []methodFixture{noisyAt(1000, 0)},
		}
	}
	doc := fairnessDocOf(8,
		[]armFixture{thin(armOff, 20), thin(armOn, 10)},
		[]armFixture{thin(armOn, 10), thin(armOff, 20)})
	comparisons, verdict := judgeFairness(doc)
	if len(comparisons) != 1 || comparisons[0].Metric != metricP50 {
		t.Fatalf("comparisons = %+v, want one compared on p50", comparisons)
	}
	if verdict.Direction != directionBetter {
		t.Errorf("direction = %q (%s), want %q", verdict.Direction, verdict.Reason, directionBetter)
	}
}

// TestDecide_RefusesToJudgeASingleRepetition verifies the direct guard, which
// the assembled verdict reaches only through a document that lost an arm.
func TestDecide_RefusesToJudgeASingleRepetition(t *testing.T) {
	direction, reason := decide(FairnessComparison{DeltasMs: []float64{5}, MedianDeltaMs: 5})
	if direction != directionIndistinguishable || !strings.Contains(reason, "fewer than two") {
		t.Errorf("decide = %q (%s), want an indistinguishable answer about too few repetitions", direction, reason)
	}
}

// TestSaturationFailure_SaysNothingWhenTheHostReportsNoCores verifies a host
// that did not say how many processors it has does not have a saturation gate
// invented for it.
func TestSaturationFailure_SaysNothingWhenTheHostReportsNoCores(t *testing.T) {
	doc := fairnessDocOf(0, []armFixture{saturated(armOff, 20), saturated(armOn, 10)})
	if got := saturationFailure(doc); got != "" {
		t.Errorf("saturationFailure = %q, want nothing on a host that reported no cores", got)
	}
	t.Run("and nothing when no repetition carries the arm", func(t *testing.T) {
		empty := &FairnessDoc{Host: HostInfo{CPUs: 8}, Repeats: []FairnessRepeat{{}}}
		if got := saturationFailure(empty); got != "" {
			t.Errorf("saturationFailure = %q, want nothing when there is no arm to read", got)
		}
	})
}

// TestDiverged_IsSymmetricAndAnswersForTwoEmptyCounts verifies the tolerance
// test does not depend on which arm is named first, and that two zeros are not
// a divergence.
func TestDiverged_IsSymmetricAndAnswersForTwoEmptyCounts(t *testing.T) {
	cases := []struct {
		name string
		a, b int
		want bool
	}{
		{name: "nothing either way", a: 0, b: 0, want: false},
		{name: "within the tolerance", a: 100, b: 95, want: false},
		{name: "beyond it", a: 100, b: 80, want: true},
		{name: "beyond it, the other way round", a: 80, b: 100, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := diverged(tc.a, tc.b); got != tc.want {
				t.Errorf("diverged(%d, %d) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestSameSign_TreatsNoMovementAsNoAgreement verifies a zero breaks agreement,
// which errs toward under-claiming.
func TestSameSign_TreatsNoMovementAsNoAgreement(t *testing.T) {
	cases := []struct {
		name   string
		deltas []float64
		want   bool
	}{
		{name: "both improved", deltas: []float64{5, 7}, want: true},
		{name: "both degraded", deltas: []float64{-5, -7}, want: true},
		{name: "one of each", deltas: []float64{5, -7}, want: false},
		{name: "one did not move", deltas: []float64{5, 0}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameSign(tc.deltas); got != tc.want {
				t.Errorf("sameSign(%v) = %v, want %v", tc.deltas, got, tc.want)
			}
		})
	}
}

// TestMedianOf_AndSpreadOf_AnswerForTheSetsAVerdictHands verifies the two
// summaries, including the empty and single-valued sets that must not put an
// infinity into a document.
func TestMedianOf_AndSpreadOf_AnswerForTheSetsAVerdictHands(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		median float64
		spread float64
	}{
		{name: "nothing", values: nil, median: 0, spread: 0},
		{name: "one value", values: []float64{4}, median: 4, spread: 0},
		{name: "an even count averages the middle", values: []float64{4, 10}, median: 7, spread: 6},
		{name: "an odd count takes the middle", values: []float64{10, 4, 7}, median: 7, spread: 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := medianOf(tc.values); got != tc.median {
				t.Errorf("medianOf(%v) = %v, want %v", tc.values, got, tc.median)
			}
			got := spreadOf(tc.values)
			if got != tc.spread {
				t.Errorf("spreadOf(%v) = %v, want %v", tc.values, got, tc.spread)
			}
			if math.IsInf(got, 0) || math.IsNaN(got) {
				t.Errorf("spreadOf(%v) = %v, which no JSON document can carry", tc.values, got)
			}
		})
	}
}

// TestFairnessArm_Summary_PrintsTheCountsBeforeThePercentiles verifies the line
// a reader sees puts survivorship before latency.
//
// A percentile quoted without the number of requests that survived into it is
// the reading error this whole scenario exists to prevent, and a terminal line
// is where it would happen first.
func TestFairnessArm_Summary_PrintsTheCountsBeforeThePercentiles(t *testing.T) {
	arm := armFixture{
		arm: armOn, coresBusy: 3,
		quiet: []methodFixture{quietAt(methodToolsCall, 12)},
		noisy: []methodFixture{noisyAt(100, 900)},
	}.build()
	// Read within the noisy population's own clause: the counts of one method
	// must come before that method's percentiles, not before some other's.
	line := arm.summary()
	_, noisy, found := strings.Cut(line, "noisy tools/call ")
	if !found {
		t.Fatalf("summary = %q, want a clause for the noisy population", line)
	}
	served, refused := strings.Index(noisy, "served 100"), strings.Index(noisy, "refused 900")
	p50 := strings.Index(noisy, "served p50")
	if served < 0 || refused < 0 || p50 < 0 {
		t.Fatalf("noisy clause = %q, want the counts and the percentiles", noisy)
	}
	if refused > p50 || served > p50 {
		t.Errorf("noisy clause = %q, want the counts before the served percentiles", noisy)
	}
	if !arm.refusedAnything() {
		t.Error("an arm with 900 refusals reports refusing nothing")
	}
	if got := arm.served(); got != 300 {
		t.Errorf("served = %d, want both populations together", got)
	}
}

// TestFairnessDoc_Summary_NamesTheBoundAndTheAnswer verifies the sentence a run
// ends on, with and without a reason.
func TestFairnessDoc_Summary_NamesTheBoundAndTheAnswer(t *testing.T) {
	doc := &FairnessDoc{
		Bound:   FairnessBound{ID: "tools-call-rps", Label: "the bucket"},
		Verdict: FairnessVerdict{Direction: directionBetter, Reason: "it fell"},
	}
	if got := doc.summary(); !strings.Contains(got, "better") || !strings.Contains(got, "(it fell)") {
		t.Errorf("summary = %q, want the direction and the reason", got)
	}
	doc.Verdict.Reason = ""
	if got := doc.summary(); strings.Contains(got, "(") {
		t.Errorf("summary = %q, want no empty parenthesis when there is no reason", got)
	}
}

// TestWriteFairness_RoundTripsAndRefusesAnUnreadableSchema verifies the
// document survives a write and a read, and that a schema this build does not
// know is refused rather than drawn from fields it guessed at.
func TestWriteFairness_RoundTripsAndRefusesAnUnreadableSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "fairness.json")
	doc := fairnessDocOf(8, []armFixture{saturated(armOff, 20), saturated(armOn, 10)})
	doc.Bound = FairnessBound{ID: "tools-call-rps"}
	if err := writeFairness(path, doc); err != nil {
		t.Fatalf("writeFairness: %v", err)
	}
	back, err := readFairness(path)
	if err != nil {
		t.Fatalf("readFairness: %v", err)
	}
	if back.Bound.ID != "tools-call-rps" || len(back.Repeats) != 1 {
		t.Errorf("read back %+v, want the document that was written", back)
	}

	cases := []struct {
		name    string
		file    string
		content string
		write   bool
		want    string
	}{
		{name: "a file that is not there", file: "absent.json", want: "read "},
		{name: "a file that is not JSON", file: "bad.json", content: "{", write: true, want: "parse "},
		{
			name: "a schema this build does not read", file: "future.json",
			content: `{"schema":99}`, write: true, want: "fairness schema 99",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(dir, tc.file)
			if tc.write {
				if writeErr := os.WriteFile(target, []byte(tc.content), 0o600); writeErr != nil {
					t.Fatalf("write: %v", writeErr)
				}
			}
			_, readErr := readFairness(target)
			if readErr == nil || !strings.Contains(readErr.Error(), tc.want) {
				t.Errorf("readFairness = %v, want an error about %q", readErr, tc.want)
			}
		})
	}
}

// TestWriteFairness_ReportsWhatItCouldNotWrite verifies the three failures of
// writing, which a filesystem the test owns does not produce on its own.
func TestWriteFairness_ReportsWhatItCouldNotWrite(t *testing.T) {
	dir := t.TempDir()
	t.Run("an encoder that refuses", func(t *testing.T) {
		original := marshalRecord
		t.Cleanup(func() { marshalRecord = original })
		marshalRecord = func(any, string, string) ([]byte, error) { return nil, errors.New("no") }
		if err := writeFairness(filepath.Join(dir, "a.json"), &FairnessDoc{}); err == nil ||
			!strings.Contains(err.Error(), "encode the fairness record") {
			t.Errorf("writeFairness = %v, want the encoder's failure", err)
		}
	})
	t.Run("a directory that cannot be made", func(t *testing.T) {
		file := filepath.Join(dir, "file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := writeFairness(filepath.Join(file, "under", "a.json"), &FairnessDoc{}); err == nil ||
			!strings.Contains(err.Error(), "create ") {
			t.Errorf("writeFairness = %v, want the directory failure", err)
		}
	})
	t.Run("a path that cannot be written", func(t *testing.T) {
		if err := writeFairness(dir, &FairnessDoc{}); err == nil || !strings.Contains(err.Error(), "write ") {
			t.Errorf("writeFairness = %v, want the write failure", err)
		}
	})
}

// TestSettingsFor_AndBoundRecord_RecordWhatWasActuallyInForce verifies the
// document carries the switches rather than only the bound's name, so a reader
// can see what the two arms really differed by.
func TestSettingsFor_AndBoundRecord_RecordWhatWasActuallyInForce(t *testing.T) {
	plan, err := fairnessPlanFor(fairnessOptions())
	if err != nil {
		t.Fatalf("fairnessPlanFor: %v", err)
	}
	settings := settingsFor(plan)
	if settings.Surface != surfaceDynamic || settings.PhaseSeconds != 2 || settings.Repeats != 2 {
		t.Errorf("settings = %+v, want the plan's own knobs", settings)
	}
	record := boundRecord(plan)
	if record.ID != "tools-call-rps" || len(record.ArgsOn) == 0 || len(record.ArgsOff) == 0 {
		t.Errorf("bound record = %+v, want the switches of both arms", record)
	}
}
