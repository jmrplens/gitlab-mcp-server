// fairness_result.go is the document a fairness run writes and the verdict it
// computes.
//
// The document is its own file with its own schema, not a section of the
// measurement record. Two reasons, and the first is the constraint: the
// committed record's charts and tables are gated by a byte comparison, and
// adding a section to it would make a full re-measure on the reference host a
// precondition for that gate going green. The second is that the two are
// different kinds of statement. A scenario says what the server costs; a
// fairness run says whether one tenant took what another was waiting for, and
// it is only an answer as a pair of arms measured in one session on one host.
// Both arms of a repetition are therefore fields of one object, so a pair
// assembled from two runs is not constructible.
//
// The shape is a top-level "fairness" array so that publishing it later, once
// the chart rework lands, is a schema bump and a renderer rather than a second
// measurement.

package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// fairnessSchema is the version of the shape below, independent of the
// measurement record's, so a change here never bumps the schema of a record
// this mode does not write.
const fairnessSchema = 1

// FairnessDoc is one fairness session.
type FairnessDoc struct {
	Schema      int                  `json:"schema"`
	GeneratedAt string               `json:"generated_at"`
	Server      ServerInfo           `json:"server"`
	Host        HostInfo             `json:"host"`
	Settings    FairnessSettings     `json:"settings"`
	Bound       FairnessBound        `json:"bound"`
	Repeats     []FairnessRepeat     `json:"repeats"`
	Comparisons []FairnessComparison `json:"comparisons"`
	Verdict     FairnessVerdict      `json:"verdict"`
}

// FairnessSettings are the knobs the run was made with.
type FairnessSettings struct {
	Surface       string  `json:"surface"`
	PhaseSeconds  float64 `json:"phase_seconds"`
	LeadInSeconds float64 `json:"lead_in_seconds"`
	DeadlineMs    float64 `json:"deadline_ms"`
	Repeats       int     `json:"repeats"`
}

// FairnessBound is the limit under test and how each arm put it there.
type FairnessBound struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	ArgsOff []string `json:"args_off"`
	ArgsOn  []string `json:"args_on"`
	EnvOff  []string `json:"env_off,omitempty"`
	EnvOn   []string `json:"env_on,omitempty"`
}

// FairnessRepeat is one pair of arms, in the order they ran.
type FairnessRepeat struct {
	Index int           `json:"index"`
	Order []string      `json:"order"`
	Arms  []FairnessArm `json:"arms"`
}

// FairnessArm is one measured window with the bound in one state.
type FairnessArm struct {
	Arm         string               `json:"arm"`
	Args        []string             `json:"args"`
	Env         []string             `json:"env,omitempty"`
	Populations []FairnessPopulation `json:"populations"`
	Process     FairnessProcess      `json:"process"`
	// Comparable is false when this arm failed a control, which is what stops
	// the verdict comparing it rather than the arm being discarded: what it
	// measured is still written, with the reason beside it.
	Comparable bool     `json:"comparable"`
	Notes      []string `json:"notes,omitempty"`
}

// FairnessPopulation is one tenant population's experience.
type FairnessPopulation struct {
	Name              string           `json:"name"`
	Credentials       int              `json:"credentials"`
	RatePerCredential float64          `json:"rate_per_credential"`
	Methods           []FairnessMethod `json:"methods"`
}

// FairnessMethod is what one population asked one method for, and what came
// back.
//
// The counts come before the distributions on purpose, in the struct and in
// every line printed from it: a percentile is only readable beside the number
// of requests that survived to be in it, and a latency that improved because
// the slow work was refused should not be legible without the refusal count on
// the same row.
type FairnessMethod struct {
	Method string `json:"method"`
	Detail string `json:"detail,omitempty"`
	// Intended is every tick the schedule reached; Dropped is those the driver
	// never sent, whether because it could hold no slot or because it arrived
	// past the request's own deadline; Dispatched is the rest, and the four
	// outcomes below sum to it.
	Intended   int `json:"intended"`
	Dropped    int `json:"dropped"`
	Dispatched int `json:"dispatched"`
	Served     int `json:"served"`
	Refused    int `json:"refused"`
	Failed     int `json:"failed"`
	TimedOut   int `json:"timed_out"`
	// ServedLatency and RefusedLatency are separate distributions under
	// separate names and are never merged. How fast a refusal comes back is
	// worth knowing; counted as a served request it would make every bound look
	// like an improvement.
	ServedLatency  MethodLatency `json:"served_latency"`
	RefusedLatency MethodLatency `json:"refused_latency"`
	// Lateness is the driver's own delay between a request's intended instant
	// and its actual send. It is a measurement of the harness rather than of
	// the server, and the verdict refuses a claim it could account for.
	Lateness     MethodLatency `json:"dispatch_lateness"`
	FirstFailure string        `json:"first_failure,omitempty"`
}

// FairnessProcess is what the two processes cost over one arm's phase.
type FairnessProcess struct {
	PhaseSeconds float64 `json:"phase_seconds"`
	CPUSeconds   float64 `json:"cpu_seconds"`
	// CoresBusy is processor seconds over wall seconds: how many cores the
	// server kept busy. It is the saturation reading, and without it "no
	// improvement" is a statement about the host that reads as one about the
	// bound.
	CoresBusy float64 `json:"cores_busy"`
	// CPUMsPerServed divides by served requests alone. There is deliberately no
	// per-request figure anywhere in this document.
	CPUMsPerServed   float64 `json:"cpu_ms_per_served_request"`
	RSSPeakMiB       float64 `json:"rss_peak_mib"`
	RSSMeanMiB       float64 `json:"rss_mean_mib"`
	DriverCPUSeconds float64 `json:"driver_cpu_seconds"`
	DriverCoresBusy  float64 `json:"driver_cores_busy"`
}

// FairnessComparison is one population and method compared across the arms.
type FairnessComparison struct {
	Population string `json:"population"`
	Method     string `json:"method"`
	Detail     string `json:"detail,omitempty"`
	// Metric names which percentile was compared: p99 when both arms carried
	// enough served samples for it to be a distinct observation, p50 otherwise.
	Metric string `json:"metric"`
	// DeltasMs is one per repetition, the bound-off figure less the bound-on
	// one, so a positive number means the arm with the bound was faster.
	DeltasMs      []float64 `json:"deltas_ms"`
	MedianDeltaMs float64   `json:"median_delta_ms"`
	SpreadMs      float64   `json:"spread_ms"`
	// LatenessShiftMs is how much the driver's own dispatch lateness moved
	// between the arms, on the same scale as the delta above.
	LatenessShiftMs float64 `json:"lateness_shift_ms"`
	// ServedShare is the smallest share of the quiet population's dispatched
	// requests that completed in any arm the comparison read, and it is
	// repeated in the reason below rather than only stored here. A percentile
	// is quoted, and a percentile quoted without the survivorship behind it is
	// the one reading error this whole scenario exists to prevent.
	ServedShare float64 `json:"served_share"`
	Direction   string  `json:"direction"`
	Reason      string  `json:"reason"`
}

// FairnessVerdict is the run's answer.
type FairnessVerdict struct {
	Direction string   `json:"direction"`
	Reason    string   `json:"reason"`
	Notes     []string `json:"notes,omitempty"`
}

// The four answers a comparison can give. "No improvement" is a first-class
// outcome and so is "worse": a scenario that could only report success would
// be an advocate rather than a measurement.
const (
	directionBetter            = "better"
	directionWorse             = "worse"
	directionIndistinguishable = "indistinguishable"
	directionNotComparable     = "not comparable"
)

// p99MinSamples is the served count below which a nearest-rank p99 is the
// maximum wearing a percentile's name, and the comparison falls back to p50.
const p99MinSamples = 100

// saturationFloor is the share of the host's cores the arm with the bound off
// must keep busy for the comparison to mean anything.
//
// The bucket is per credential, so a noisy tenant never draws on a quiet one's
// budget: the only channel by which the bound can help the quiet tenant is
// reduced contention for the machine. On a host the noisy population failed to
// contend for there is nothing to protect anybody from, and any difference
// between the arms is noise wearing a verdict.
const saturationFloor = 0.5

// offeredTolerance is how far the two arms' dispatched counts may differ
// before they are not the same experiment. Both arms follow one schedule so
// they should be equal, and a gap means one arm's driver could not keep up,
// which is exactly the asymmetry a refusal that returns in two milliseconds
// creates.
const offeredTolerance = 0.10

// judgeFairness compares the arms and returns the per-method comparisons and
// the run's verdict.
//
// The order of the gates is the honesty, and the first two were the other way
// round until a reader pointed out what that cost.
//
// The quiet regression comes first because it is the only answer here that
// needs no comparison at all: that the bound refused the tenant it exists to
// protect, or that the tenant gave up on more requests with it in force, is a
// fact about one pair of arms and holds however few repetitions were run and
// whatever else about the pair was irregular. Behind the repetition-count gate
// it was unreachable at -fairness-repeats=1, so the single run in which the
// bound turned the quiet tenant away was reported as no difference; behind the
// comparability gate it was discarded in favor of the more procedural answer
// whenever anything else had also gone wrong.
//
// Everything after it is a comparison and needs both arms to be the same
// experiment, so those gates run in the order of how specific their sentence
// is.
func judgeFairness(doc *FairnessDoc) ([]FairnessComparison, FairnessVerdict) {
	if quiet := quietRegression(doc); quiet != "" {
		return nil, FairnessVerdict{Direction: directionWorse, Reason: quiet}
	}
	if blocked := comparabilityFailure(doc); blocked != "" {
		return nil, FairnessVerdict{Direction: directionNotComparable, Reason: blocked}
	}
	if len(doc.Repeats) < 2 {
		return nil, FairnessVerdict{
			Direction: directionIndistinguishable,
			Reason: "one repetition carries no measure of host noise, so nothing measured here can be told from the run-to-run " +
				"variation of the machine; raise -fairness-repeats",
		}
	}
	if idle := saturationFailure(doc); idle != "" {
		return nil, FairnessVerdict{Direction: directionIndistinguishable, Reason: idle}
	}

	comparisons := compareQuiet(doc)
	verdict := summarizeDirections(comparisons)
	// Asked only of a claim that the bound helped, and last. The harness
	// competing for its own host is an alternative explanation for an
	// improvement and not for a regression: a quiet tenant that came out worse
	// while the driver was busier is worse for one more reason, not for one
	// fewer.
	if verdict.Direction == directionBetter {
		if confound := driverConfound(doc); confound != "" {
			return comparisons, FairnessVerdict{Direction: directionNotComparable, Reason: confound}
		}
	}
	return comparisons, verdict
}

// driverConfound reports an improvement the harness could have caused by
// getting out of its own way.
//
// The driver shares the host with the server it measures and costs very
// different amounts in the two arms: parsing a two-kilobyte refusal is nothing
// beside a hundred-and-seventy-kilobyte result, so with the bound in force it
// hands the host back exactly where the quiet tenant is supposed to look
// better. When it handed back at least as much as the server itself did, the
// harness is at least as good an explanation for the improvement as the bound,
// and no arithmetic here can separate them.
//
// This is the second reading of that effect and not a replacement for the
// first. The dispatch lateness the per-method gate uses is captured before the
// call and so cannot see the response decode, which is the larger half and the
// half that differs most between the arms; this one is the processor time both
// halves are inside, measured directly, and it was being recorded and never
// consulted.
func driverConfound(doc *FairnessDoc) string {
	var byDriver, byServer []float64
	for _, repeat := range doc.Repeats {
		off, offOK := repeat.arm(armOff)
		on, onOK := repeat.arm(armOn)
		if !offOK || !onOK {
			continue
		}
		byDriver = append(byDriver, off.Process.DriverCoresBusy-on.Process.DriverCoresBusy)
		byServer = append(byServer, off.Process.CoresBusy-on.Process.CoresBusy)
	}
	if len(byDriver) == 0 {
		return ""
	}
	freedByDriver, freedByServer := medianOf(byDriver), medianOf(byServer)
	if freedByDriver <= 0 || freedByDriver < freedByServer {
		return ""
	}
	return fmt.Sprintf(
		"the driver handed back %.2f of the host's cores with the bound in force against the %.2f the server itself "+
			"handed back, so the harness competing for the machine is at least as good an explanation for the quiet "+
			"population's improvement as the bound is: measure again with the driver on another host, or with a noisy "+
			"population whose refusals cost the driver less to read",
		freedByDriver, freedByServer,
	)
}

// comparabilityFailure names the first reason the arms cannot be compared at
// all, and the empty string when there is none.
func comparabilityFailure(doc *FairnessDoc) string {
	for _, repeat := range doc.Repeats {
		off, offOK := repeat.arm(armOff)
		on, onOK := repeat.arm(armOn)
		if !offOK || !onOK {
			return fmt.Sprintf("repetition %d did not run both arms", repeat.Index+1)
		}
		for _, arm := range []FairnessArm{off, on} {
			if !arm.Comparable {
				return fmt.Sprintf("repetition %d, the %s arm: %s", repeat.Index+1, arm.Arm, strings.Join(arm.Notes, "; "))
			}
		}
		// The drop check comes first because it names the cause: a schedule the
		// driver could not send is also a pair of arms that offered different
		// work, and the more specific sentence is the useful one.
		if reason := quietWasDropped(repeat.Index, off, on); reason != "" {
			return reason
		}
		if reason := quietSurvived(repeat.Index, off, on); reason != "" {
			return reason
		}
		// Both ways round, because one direction only sees what the arm it
		// starts from drove: an arm missing a population the other has would
		// otherwise pass whichever of the two was read first.
		if reason := offeredEqually(repeat.Index, off, on); reason != "" {
			return reason
		}
		if reason := offeredEqually(repeat.Index, on, off); reason != "" {
			return reason
		}
	}
	return ""
}

// offeredEqually refuses a repetition whose arms did not put the same work in
// front of the server, reading from one arm and looking the counterpart up in
// the other.
func offeredEqually(index int, from, to FairnessArm) string {
	for _, pop := range from.Populations {
		other, ok := to.population(pop.Name)
		if !ok {
			return fmt.Sprintf("repetition %d: the arms did not drive the same populations", index+1)
		}
		for _, method := range pop.Methods {
			counterpart, found := other.method(method.Method)
			if !found {
				return fmt.Sprintf("repetition %d: the arms did not drive %s for the %s population", index+1, method.Method, pop.Name)
			}
			if diverged(method.Dispatched, counterpart.Dispatched) {
				return fmt.Sprintf(
					"repetition %d: the %s population dispatched %d %s requests with the bound %s and %d with it %s, "+
						"more than the %.0f%% the arms may differ by; the two arms did not offer the server the same work",
					index+1, pop.Name, method.Dispatched, method.Method, from.Arm, counterpart.Dispatched, to.Arm,
					offeredTolerance*100,
				)
			}
		}
	}
	return ""
}

// diverged reports two counts further apart than the tolerance allows, taking
// the larger as the base so the test is symmetric.
func diverged(a, b int) bool {
	larger := max(a, b)
	if larger == 0 {
		return false
	}
	return math.Abs(float64(a-b))/float64(larger) > offeredTolerance
}

// quietWasDropped refuses a repetition in which the driver failed to offer the
// quiet population's own schedule, since its percentiles would then be over
// whatever the driver managed to send.
func quietWasDropped(index int, arms ...FairnessArm) string {
	for _, arm := range arms {
		quiet, ok := arm.population(populationQuiet)
		if !ok {
			continue
		}
		for _, method := range quiet.Methods {
			if method.Dropped > 0 {
				return fmt.Sprintf(
					"repetition %d, the %s arm: the driver could not send %d of the quiet population's %s requests, "+
						"so its distribution is over what the driver managed rather than what it intended",
					index+1, arm.Arm, method.Dropped, method.Method,
				)
			}
		}
	}
	return ""
}

// quietSurvived refuses a repetition in which most of the quiet population's
// requests never completed, whichever arm they were in.
//
// The percentiles this scenario publishes are over served requests alone,
// which is what keeps a refusal out of them. The cost of that is a distribution
// whose meaning depends on how much of the population reached it: three
// requests out of a hundred and sixty, in both arms, still produce two
// percentiles and a difference between them, and nothing above this would
// object, since the arms dispatched the same work, the bound refused nobody
// quiet, and neither arm's timeouts rose against the other's. A verdict
// computed from the survivors of a catastrophe is not a verdict about the
// population, so it is refused rather than qualified.
//
// The floor is deliberately well below a healthy run: a quiet tenant losing a
// tenth of its requests to a contended host is the interesting case this
// scenario exists for, and refusing to compare it would throw away the finding
// along with the noise.
func quietSurvived(index int, arms ...FairnessArm) string {
	for _, arm := range arms {
		quiet, ok := arm.population(populationQuiet)
		if !ok {
			continue
		}
		for _, method := range quiet.Methods {
			if method.Dispatched == 0 {
				continue
			}
			share := float64(method.Served) / float64(method.Dispatched)
			if share < quietSurvivorshipFloor {
				return fmt.Sprintf(
					"repetition %d, the %s arm: %d of the quiet population's %d dispatched %s requests completed (%.0f%%), "+
						"below the %.0f%% a comparison of served percentiles needs; what survived a phase that lost "+
						"most of the population is not that population's experience",
					index+1, arm.Arm, method.Served, method.Dispatched, method.Method,
					share*100, quietSurvivorshipFloor*100,
				)
			}
		}
	}
	return ""
}

// quietSurvivorshipFloor is the share of the quiet population's dispatched
// requests that must have completed for its served percentiles to stand for
// the population.
const quietSurvivorshipFloor = 0.75

// quietRegression names the first way the quiet population was worse off with
// the bound on, and the empty string when it was not.
//
// This runs before any latency comparison, and that order is the point: a
// bound that refuses the tenant it exists to protect improves its surviving
// percentiles by killing the slow work, and a report that read the percentile
// first would call that an improvement.
//
// There is deliberately no separate check that the quiet population completed
// as much work as before. Under the identity every method record is held to,
// served plus refused plus failed plus timed out is what was dispatched, and
// the arms are already required to have dispatched the same: so a fall in
// served is exactly a rise in one of the other three, and each of those is
// answered here or by the arm's own controls. A fourth check would be a fourth
// name for one of these three, and it would be the one that fired, hiding the
// specific answer behind a general one.
//
// The survivorship floor above is not that fourth check: it asks how much of
// the population completed at all, in each arm on its own, which the identity
// says nothing about because it holds just as well when both arms lost almost
// everything.
func quietRegression(doc *FairnessDoc) string {
	if refused := quietWasRefused(doc); refused != "" {
		return refused
	}
	return quietGaveUpMore(doc)
}

// quietWasRefused names the first repetition in which the bound turned the
// quiet population away.
//
// One refusal is enough and needs no counterpart in the other arm: it is the
// bound acting on the tenant it exists to protect, which is a categorical
// finding rather than a quantity to be weighed against the run's noise.
func quietWasRefused(doc *FairnessDoc) string {
	for _, repeat := range doc.Repeats {
		on, _ := repeat.arm(armOn)
		quiet, ok := on.population(populationQuiet)
		if !ok {
			continue
		}
		for _, method := range quiet.Methods {
			if method.Refused > 0 {
				return fmt.Sprintf(
					"repetition %d: the bound refused %d of the quiet population's %d %s requests. "+
						"The tenant this bound exists to protect was the one it turned away",
					repeat.Index+1, method.Refused, method.Intended, method.Method,
				)
			}
		}
	}
	return ""
}

// quietGaveUpMore reports a quiet population that abandoned more requests with
// the bound in force, when every repetition says so.
//
// Every repetition, rather than any one of them, and the difference between
// those two is the difference between a measurement and a coin. A timeout is a
// tail event on a contended host: the first real run of this scenario against
// the shipped bucket lost one of a hundred and twenty quiet calls in the arm
// with the bound on and fifteen of them in the arm with it off, and a check
// that fires on any single repetition read the one and reported that the bound
// left the quiet tenant worse off. On a host contended enough for the question
// to be worth asking, that check answers "worse" whichever way the machine
// jitters, and a scenario whose answer is settled before it runs is not
// measuring anything.
//
// This is the same agreement rule the latency comparison applies to its own
// deltas, for the same reason and stated in the same words: one disturbed
// window may not carry the answer.
func quietGaveUpMore(doc *FairnessDoc) string {
	for _, method := range quietMethods(doc) {
		pairs := methodPairs(doc, method)
		if len(pairs) == 0 {
			continue
		}
		rose := true
		for _, pair := range pairs {
			rose = rose && pair.on.TimedOut > pair.off.TimedOut
		}
		if !rose {
			continue
		}
		on, off := 0, 0
		for _, pair := range pairs {
			on += pair.on.TimedOut
			off += pair.off.TimedOut
		}
		return fmt.Sprintf(
			"the quiet population gave up on %d %s requests with the bound on against %d with it off, in every one of "+
				"the %d repetitions, so whatever its latency did, it got less work done",
			on, method, off, len(pairs),
		)
	}
	return ""
}

// saturationFailure reports a host the noisy population never contended for.
func saturationFailure(doc *FairnessDoc) string {
	floor := saturationFloor * float64(doc.Host.CPUs)
	if floor <= 0 {
		return ""
	}
	worst := math.Inf(1)
	for _, repeat := range doc.Repeats {
		if off, ok := repeat.arm(armOff); ok {
			worst = math.Min(worst, off.Process.CoresBusy)
		}
	}
	if math.IsInf(worst, 1) || worst >= floor {
		return ""
	}
	return fmt.Sprintf(
		"the arm with the bound off kept only %.2f of the host's %d cores busy, below the %.2f this comparison needs. "+
			"A limit that is per credential can only reach the quiet tenant through contention for the machine, "+
			"so on a host that was not contended there is nothing here to measure: raise -fairness-noisy or -fairness-noisy-rate",
		worst, doc.Host.CPUs, floor,
	)
}

// compareQuiet compares the quiet population, method by method.
func compareQuiet(doc *FairnessDoc) []FairnessComparison {
	var out []FairnessComparison
	for _, method := range quietMethods(doc) {
		out = append(out, compareMethod(doc, method))
	}
	return out
}

// quietMethods are the methods the quiet population issued, in record order.
func quietMethods(doc *FairnessDoc) []string {
	var methods []string
	for _, repeat := range doc.Repeats {
		for _, arm := range repeat.Arms {
			quiet, ok := arm.population(populationQuiet)
			if !ok {
				continue
			}
			for _, method := range quiet.Methods {
				if !slices.Contains(methods, method.Method) {
					methods = append(methods, method.Method)
				}
			}
		}
	}
	return methods
}

// compareMethod compares one method across every repetition.
func compareMethod(doc *FairnessDoc, method string) FairnessComparison {
	out := FairnessComparison{Population: populationQuiet, Method: method, Metric: metricP99}
	pairs := methodPairs(doc, method)
	// The metric is settled over every repetition before a single delta is
	// taken, so one short window cannot leave the rest compared on a different
	// percentile from itself.
	for _, pair := range pairs {
		out.Detail = pair.on.Detail
		if pair.off.Served < p99MinSamples || pair.on.Served < p99MinSamples {
			out.Metric = metricP50
		}
	}
	latenessShifts := make([]float64, 0, len(pairs))
	out.ServedShare = 1
	for _, pair := range pairs {
		out.DeltasMs = append(out.DeltasMs, round(servedAt(pair.off, out.Metric)-servedAt(pair.on, out.Metric)))
		latenessShifts = append(latenessShifts, math.Abs(pair.off.Lateness.P99-pair.on.Lateness.P99))
		out.ServedShare = min(out.ServedShare, servedShare(pair.off), servedShare(pair.on))
	}
	out.ServedShare = round(out.ServedShare)
	out.MedianDeltaMs = round(medianOf(out.DeltasMs))
	out.SpreadMs = round(spreadOf(out.DeltasMs))
	out.LatenessShiftMs = round(medianOf(latenessShifts))
	out.Direction, out.Reason = decide(out)
	return out
}

// The two percentiles a comparison can be made on.
const (
	metricP50 = "p50"
	metricP99 = "p99"
)

// methodPair is one repetition's two records for one method.
type methodPair struct{ off, on FairnessMethod }

// methodPairs are the repetitions that carry both arms of one method.
func methodPairs(doc *FairnessDoc, method string) []methodPair {
	pairs := make([]methodPair, 0, len(doc.Repeats))
	for _, repeat := range doc.Repeats {
		off, offOK := armMethod(repeat, armOff, method)
		on, onOK := armMethod(repeat, armOn, method)
		if offOK && onOK {
			pairs = append(pairs, methodPair{off: off, on: on})
		}
	}
	return pairs
}

// servedShare is how much of what one method dispatched came back served,
// which is the survivorship the compared percentile is over. A method that
// dispatched nothing has no survivorship rather than none of it, and reports
// the whole of what it dispatched, so it cannot pull a comparison down on its
// own.
func servedShare(method FairnessMethod) float64 {
	if method.Dispatched == 0 {
		return 1
	}
	return float64(method.Served) / float64(method.Dispatched)
}

// servedAt reads the compared percentile out of a method's served
// distribution. Served alone: a refused request has its own distribution and
// never enters this one.
func servedAt(method FairnessMethod, metric string) float64 {
	if metric == metricP50 {
		return method.ServedLatency.P50
	}
	return method.ServedLatency.P99
}

// decide turns one method's deltas into a direction.
//
// Four gates, in this order. Fewer than two repetitions carry no measure of
// host noise at all. Then the repetitions have to agree in sign, so a single
// disturbed window cannot carry the answer. Then the difference has to exceed
// the spread between repetitions, which is the only measure of host noise a
// handful of them offers, and is why the field is called a direction and not a
// significance. That gate is weaker than it sounds at two repetitions, and
// worth stating rather than leaving to be derived: with two same-signed
// deltas, the median beating the spread reduces to the larger delta being
// under three times the smaller, which is an agreement test and no more.
// Repetitions past the second are what make it stronger.
//
// The last gate is the driver's own, and it is the one this scenario would be
// dishonest without. The driver shares the host with the server it measures,
// and with the bound in force it parses a two-kilobyte refusal where it parsed
// a hundred-and-seventy-kilobyte result, so it frees a core in exactly the arm
// that is supposed to look better: an improvement the harness caused by
// getting out of its own way would otherwise be published as the server being
// fairer. Its dispatch lateness is the observable of that on the same scale as
// the claim, so a claim smaller than the lateness moved between the arms is
// refused rather than made. It comes last because it is only worth asking of a
// difference that would otherwise stand, and because a claim of zero has
// nothing to attribute to anybody.
func decide(c FairnessComparison) (direction, reason string) {
	if len(c.DeltasMs) < 2 {
		return directionIndistinguishable, "fewer than two repetitions carry no measure of host noise"
	}
	claim := math.Abs(c.MedianDeltaMs)
	if !sameSign(c.DeltasMs) {
		return directionIndistinguishable, fmt.Sprintf("the repetitions disagreed in sign: %v ms", c.DeltasMs)
	}
	if claim <= c.SpreadMs {
		return directionIndistinguishable, fmt.Sprintf(
			"the %.3f ms difference is inside the %.3f ms the repetitions of one arm differ from each other by",
			claim, c.SpreadMs,
		)
	}
	if c.LatenessShiftMs >= claim {
		return directionNotComparable, fmt.Sprintf(
			"the driver's own dispatch lateness moved by %.3f ms between the arms against the %.3f ms difference claimed, "+
				"so the change is as easily the harness competing for the host as the server being fairer",
			c.LatenessShiftMs, claim,
		)
	}
	if c.MedianDeltaMs > 0 {
		return directionBetter, fmt.Sprintf("the quiet population's served %s fell by %.3f ms, beyond the %.3f ms spread between repetitions, %s",
			c.Metric, c.MedianDeltaMs, c.SpreadMs, survivorship(c.ServedShare))
	}
	return directionWorse, fmt.Sprintf("the quiet population's served %s rose by %.3f ms, beyond the %.3f ms spread between repetitions, %s",
		c.Metric, -c.MedianDeltaMs, c.SpreadMs, survivorship(c.ServedShare))
}

// survivorship is the clause every quotable direction ends on, so the sentence
// a reader copies out carries how much of the population the percentile was
// over.
func survivorship(share float64) string {
	return fmt.Sprintf("over the %.0f%% of the quiet population's requests that completed", share*100)
}

// sameSign reports whether every repetition moved the same way. A zero is not
// a direction and breaks agreement, which errs toward under-claiming.
func sameSign(deltas []float64) bool {
	for _, delta := range deltas {
		if delta == 0 || (delta > 0) != (deltas[0] > 0) {
			return false
		}
	}
	return true
}

// summarizeDirections folds the per-method comparisons into one answer, worst
// first: a bound that helped one method and hurt another has not helped.
func summarizeDirections(comparisons []FairnessComparison) FairnessVerdict {
	if len(comparisons) == 0 {
		return FairnessVerdict{Direction: directionNotComparable, Reason: "the quiet population issued nothing to compare"}
	}
	for _, direction := range []string{directionNotComparable, directionWorse, directionIndistinguishable} {
		for _, c := range comparisons {
			if c.Direction == direction {
				return FairnessVerdict{Direction: direction, Reason: c.Method + ": " + c.Reason}
			}
		}
	}
	reasons := make([]string, 0, len(comparisons))
	for _, c := range comparisons {
		reasons = append(reasons, c.Method+": "+c.Reason)
	}
	return FairnessVerdict{Direction: directionBetter, Reason: strings.Join(reasons, "; ")}
}

// arm returns one arm of a repetition by name.
func (r FairnessRepeat) arm(name string) (FairnessArm, bool) {
	for _, arm := range r.Arms {
		if arm.Arm == name {
			return arm, true
		}
	}
	return FairnessArm{}, false
}

// population returns one population of an arm by name.
func (a FairnessArm) population(name string) (FairnessPopulation, bool) {
	for _, pop := range a.Populations {
		if pop.Name == name {
			return pop, true
		}
	}
	return FairnessPopulation{}, false
}

// method returns one method of a population.
func (p FairnessPopulation) method(name string) (FairnessMethod, bool) {
	for _, method := range p.Methods {
		if method.Method == name {
			return method, true
		}
	}
	return FairnessMethod{}, false
}

// armMethod reaches one arm's quiet record for one method.
func armMethod(repeat FairnessRepeat, arm, method string) (FairnessMethod, bool) {
	found, ok := repeat.arm(arm)
	if !ok {
		return FairnessMethod{}, false
	}
	quiet, ok := found.population(populationQuiet)
	if !ok {
		return FairnessMethod{}, false
	}
	return quiet.method(method)
}

// served is every request this arm completed, both populations together, which
// is the denominator of its processor-time-per-served figure.
func (a FairnessArm) served() int {
	total := 0
	for _, pop := range a.Populations {
		for _, method := range pop.Methods {
			total += method.Served
		}
	}
	return total
}

// refusals is every request this arm had refused, which is what the positive
// control weighs against the number the plan says the bound should have
// refused.
func (a FairnessArm) refusals() int {
	total := 0
	for _, pop := range a.Populations {
		for _, method := range pop.Methods {
			total += method.Refused
		}
	}
	return total
}

// refusedAnything reports whether this arm recorded a refusal at all, which is
// what the arm with the bound off is held to: there, any refusal at all is
// something other than the bound under test, and one is as damning as a
// thousand.
func (a FairnessArm) refusedAnything() bool { return a.refusals() > 0 }

// summary is the line an arm prints as it completes.
//
// The counts come before the percentiles in the sentence for the same reason
// they come before them in the struct: a latency that is legible without its
// survivorship is a latency that will be quoted without it.
func (a FairnessArm) summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "bound %s: %.2f cores busy, %.3f ms per served request, driver %.2f cores",
		a.Arm, a.Process.CoresBusy, a.Process.CPUMsPerServed, a.Process.DriverCoresBusy)
	for _, pop := range a.Populations {
		for _, method := range pop.Methods {
			fmt.Fprintf(&b, "; %s %s served %d refused %d failed %d timed out %d, served p50 %s ms p99 %s ms",
				pop.Name, method.Method, method.Served, method.Refused, method.Failed, method.TimedOut,
				msLabel(method.ServedLatency.P50), msLabel(method.ServedLatency.P99))
		}
	}
	return b.String()
}

// summary is the sentence the run ends on.
func (d *FairnessDoc) summary() string {
	line := fmt.Sprintf("%s: the quiet population is %s with %s in force", d.Bound.ID, d.Verdict.Direction, d.Bound.Label)
	if d.Verdict.Reason != "" {
		line += " (" + d.Verdict.Reason + ")"
	}
	return line
}

// settingsFor records the knobs a plan was run with.
func settingsFor(plan fairnessPlan) FairnessSettings {
	return FairnessSettings{
		Surface:       plan.Surface,
		PhaseSeconds:  round(plan.Phase.Seconds()),
		LeadInSeconds: round(plan.LeadIn.Seconds()),
		DeadlineMs:    round(float64(plan.Deadline.Milliseconds())),
		Repeats:       plan.Repeats,
	}
}

// boundRecord records the bound and the switches each arm used, so a reader
// can see what was actually in force rather than trusting the name.
func boundRecord(plan fairnessPlan) FairnessBound {
	return FairnessBound{
		ID: plan.Bound.ID, Label: plan.Bound.Label,
		ArgsOff: plan.armArgs(armOff), ArgsOn: plan.armArgs(armOn),
		EnvOff: plan.armEnv(armOff), EnvOn: plan.armEnv(armOn),
	}
}

// writeFairness writes the document, in the shape every other committed JSON
// artifact here has.
func writeFairness(path string, doc *FairnessDoc) error {
	return writeJSON(path, doc, "the fairness record")
}
