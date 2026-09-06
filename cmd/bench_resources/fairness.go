// fairness.go declares what a fairness comparison is: two populations of
// credentials driven differently, one bound, and two arms.
//
// Every other scenario here drives every credential the same way and starts
// the server with the limiter off, so none of them can see fairness: a harness
// where every caller behaves alike has no quiet neighbor to protect, and with
// no bound in force there is nothing to protect it. This scenario answers the
// question those cannot, which is not "does the bound refuse the noisy tenant"
// but "is the quiet tenant better off". Those are different questions and the
// second one may honestly answer no.
//
// Three things about the shape are load-bearing, and each of them is a way the
// measurement would otherwise lie.
//
// The driver is open loop. A refused request comes back in about two
// milliseconds where a served one takes tens, so a closed-loop driver would
// send several times as many requests in the arm with the bound on, and the
// two arms would be different experiments whose processor-time difference is
// the sum of two opposite effects. Both populations therefore follow a
// schedule computed before the phase starts, identical in both arms, and a
// tick the driver could not fire is counted rather than deferred.
//
// Served and refused are never one number. A refusal is cheap, so anything
// that pools them improves as the bound refuses more, which is exactly
// backwards. There are four terminal outcomes per population and per method,
// they are held to an arithmetic identity in code, and no field anywhere in
// the document carries a latency over all requests, so a merged percentile
// cannot be read out of the record by accident.
//
// The bound is a value rather than a code path. A boundSpec carries the
// arguments and environment of both arms and the wire shape of its own
// refusal, so the listen ceilings and whatever the policy work produces are a
// literal in the table below rather than a copy of this file.
package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// The two populations. Named because the verdict is not symmetric between
// them: the noisy one exists to create contention and its own experience is
// not the result, while the quiet one's experience is the whole answer.
const (
	populationQuiet = "quiet"
	populationNoisy = "noisy"
)

// The verbs a population may be given. A verb is a request kind rather than a
// method, because the bounds this has to reach next refuse a held resource
// rather than a rate and will need a kind that opens a stream and keeps it.
const (
	verbCall = "call"
	verbList = "list"
)

// verbSpec is one request a population issues.
type verbSpec struct {
	ID     string
	Method string
	// params builds fresh parameters per request. Fresh because the request
	// encoder writes the per-request _meta into the map it is handed, so a
	// shared one would be written by every goroutine at once.
	params func(call toolCall) map[string]any
	// detail names what was called, since the tool differs per surface and a
	// percentile with no call behind it is not comparable to anything.
	detail func(call toolCall) string
}

// verbs are every request kind a population can be given.
var verbs = map[string]verbSpec{
	verbCall: {
		ID:     verbCall,
		Method: methodToolsCall,
		params: func(call toolCall) map[string]any {
			return map[string]any{"name": call.Name, "arguments": call.Args}
		},
		detail: func(call toolCall) string { return call.Detail },
	},
	verbList: {
		ID:     verbList,
		Method: methodToolsList,
		params: func(toolCall) map[string]any { return nil },
		detail: func(toolCall) string { return detailWholeSurface },
	},
}

// refusalSpec is one wire shape a bound's refusal arrives in.
//
// Three fields rather than one because the shapes are not uniform and two of
// them collide. A refused tools/call is HTTP 200 carrying a successful result
// flagged isError, with no code anywhere and the message as its only mark; a
// refused resources/read is HTTP 200 carrying JSON-RPC -42900; and the
// per-address authentication lockout carries that same -42900 at HTTP 429. So
// a refusal is matched on the status, the code and the wording together, and
// anything that does not match all three stays a failure. This is a whitelist
// of the bound under test and never a blacklist of success: a refusal counted
// against the wrong bound turns a null result into an apparent one.
type refusalSpec struct {
	// Status is the HTTP status the refusal arrives with.
	Status int
	// Code is the JSON-RPC error code, or zero when the refusal is carried by
	// a tool result rather than by an error.
	Code int
	// TextPrefix is what the message must begin with.
	TextPrefix string
	// Method, when set, restricts the shape to one method.
	Method string
}

// bucketSpec is a bound that meters a rate: what it allows and how much of it
// may arrive at once.
//
// Held as numbers rather than only as flags because two things have to reason
// about them. A noisy population offering no more than the bound allows would
// never be refused, and the run would spend both arms discovering that; and
// the lead-in has to outlast the burst, or the measured window opens on a full
// bucket and reports that the bound does almost nothing. Both follow from the
// rate and the burst, so the on-arm's flags are written from them and the two
// cannot disagree.
type bucketSpec struct {
	Rate  float64
	Burst int
}

// boundSpec is one limit, and how to put it in force and take it out.
type boundSpec struct {
	ID    string
	Label string
	// ArgsOff and ArgsOn replace the server's rate-limiter arguments; EnvOff
	// and EnvOn are appended to its environment, for a bound whose switch is a
	// variable rather than a flag. A bound with a Bucket writes its own ArgsOn.
	ArgsOff, ArgsOn []string
	EnvOff, EnvOn   []string
	// Bucket, when set, is the rate this bound meters.
	Bucket *bucketSpec
	// Refusals is every wire shape this bound's refusal arrives in.
	Refusals []refusalSpec
	// NoisyVerbs and QuietVerbs are what each population issues against it.
	NoisyVerbs, QuietVerbs []string
	// Undrivable, when set, is why this driver cannot provoke the bound yet.
	// A bound is declared before it can be driven so the shape above is fitted
	// to more than one instance of it, and naming the gap beats a plan that
	// runs and measures nothing.
	Undrivable string
}

// onArgs are the switches that put the bound in force.
//
// A metered rate writes its own, from the same numbers the plan is validated
// against: a bound whose flags said ten and whose validation believed twenty
// would pass a lead-in check the run then failed.
func (b boundSpec) onArgs() []string {
	if b.Bucket == nil {
		return b.ArgsOn
	}
	return []string{
		fmt.Sprintf("--rate-limit-rps=%g", b.Bucket.Rate),
		fmt.Sprintf("--rate-limit-burst=%d", b.Bucket.Burst),
	}
}

// drain is how long the burst takes to empty at an offered rate, and whether
// that rate exceeds the bound at all.
//
// A population offering no more than the bound allows is never refused, so
// there is no drain and no measurement: the caller refuses the plan rather
// than spending two arms finding out.
func (b boundSpec) drain(offered float64) (time.Duration, bool) {
	if b.Bucket == nil {
		return 0, true
	}
	if offered <= b.Bucket.Rate {
		return 0, false
	}
	return time.Duration(float64(b.Bucket.Burst) / (offered - b.Bucket.Rate) * float64(time.Second)), true
}

// The refusal every rate-limit bucket writes, read from the server rather than
// restated, so a change of wording there is a compile error here.
const rateLimitRefusal = toolutil.RateLimitRefusalPrefix

// serverBusyCode is what the listen ceilings refuse with, which is a different
// number from the buckets': a classifier that keyed on one code alone would
// count one bound's refusals against the other.
const serverBusyCode = -32000

// httpOK and httpTooManyRequests are the two statuses a refusal can arrive
// with, spelled here so a refusalSpec reads as data.
const (
	httpOK              = 200
	httpTooManyRequests = 429
)

// fairnessBounds are the bounds this scenario can be pointed at.
//
// Three are declared rather than one, because a shape fitted to a single
// instance is not a shape. The token bucket is a flag and refuses a rate; the
// listen ceiling is an environment variable and refuses a held resource; and
// the metered listing shares the first one's switch while refusing a different
// method, which is why the refusal shape is per bound rather than one global
// classifier.
var fairnessBounds = []boundSpec{
	{
		ID:    "tools-call-rps",
		Label: "the per-credential request bucket (--rate-limit-rps)",
		// The shipped HTTP default, which is the configuration whose fairness
		// claim is worth testing: a deployment that changed it is measuring a
		// setting it chose, and can say so with the flags below.
		ArgsOff: []string{"--rate-limit-rps=0"},
		Bucket:  &bucketSpec{Rate: 10, Burst: 40},
		Refusals: []refusalSpec{
			{Status: httpOK, TextPrefix: rateLimitRefusal, Method: methodToolsCall},
		},
		NoisyVerbs: []string{verbCall},
		QuietVerbs: []string{verbCall, verbList},
	},
	{
		ID:    "tools-list-rps",
		Label: "the per-credential listing bucket",
		// The same switch as above: the listing bucket is derived from the
		// credential's own, so putting the bucket in force is what puts the
		// listing bound in force. The refusal names tools/list instead, and a
		// build that does not meter listings is caught by the probe rather
		// than reported as a bound that helped nobody.
		ArgsOff: []string{"--rate-limit-rps=0"},
		Bucket:  &bucketSpec{Rate: 10, Burst: 40},
		Refusals: []refusalSpec{
			{Status: httpOK, Code: rateLimitCode, TextPrefix: rateLimitRefusal, Method: methodToolsList},
		},
		NoisyVerbs: []string{verbList},
		QuietVerbs: []string{verbCall, verbList},
	},
	{
		ID:      "listen-streams",
		Label:   "the per-credential subscriptions/listen ceiling",
		ArgsOff: []string{"--rate-limit-rps=0"},
		ArgsOn:  []string{"--rate-limit-rps=0"},
		EnvOn:   []string{"GITLAB_MCP_MAX_LISTEN_STREAMS=4"},
		Refusals: []refusalSpec{
			{
				Status: httpOK, Code: serverBusyCode, TextPrefix: "too many open subscriptions/listen streams",
				Method: methodSubscriptionsListen,
			},
		},
		NoisyVerbs: []string{verbList},
		QuietVerbs: []string{verbCall, verbList},
		Undrivable: "this driver has no verb that opens a stream and holds it, which is what a held-resource ceiling refuses",
	},
}

// rateLimitCode is the JSON-RPC code a bucket refuses with on a method whose
// result carries no error flag.
const rateLimitCode = -42900

// methodSubscriptionsListen is the method the listen ceiling refuses, named so
// the bound above reads as data rather than as a string.
const methodSubscriptionsListen = "subscriptions/listen"

// boundByID finds a declared bound, refusing an unknown name with the list.
func boundByID(id string) (boundSpec, error) {
	for _, bound := range fairnessBounds {
		if bound.ID == id {
			return bound, nil
		}
	}
	return boundSpec{}, fmt.Errorf("no bound named %q; this command knows %s", id, strings.Join(boundIDs(), ", "))
}

// boundIDs are the declared bounds, for a flag's help and an error message.
func boundIDs() []string {
	ids := make([]string, 0, len(fairnessBounds))
	for _, bound := range fairnessBounds {
		ids = append(ids, bound.ID)
	}
	return ids
}

// populationSpec is one tenant population: how many credentials it holds, how
// fast each of them offers requests, and what it asks for.
//
// Credentials rather than a single one with a higher rate, and it is not a
// detail: the bucket is per credential, so minting a second token doubles the
// allowance a per-credential bound grants. Making the count a parameter is
// what makes that visible instead of argued about.
type populationSpec struct {
	Name string
	// Credentials is how many distinct tokens this population holds.
	Credentials int
	// Rate is requests per second offered by each credential.
	Rate float64
	// Verbs are cycled, one per tick.
	Verbs []string
}

// fairnessPlan is one comparison.
type fairnessPlan struct {
	ID      string
	Surface string
	Bound   boundSpec
	Quiet   populationSpec
	Noisy   populationSpec
	// Phase is the measured window; LeadIn is the unmeasured one before it,
	// which drains the bound's burst so the refusal ratio is a property of the
	// bound rather than of the phase length, and warms a heap that has never
	// been collected. Both arms pay the same lead-in.
	Phase, LeadIn time.Duration
	// Deadline is how long a request may take before a client would have given
	// up, measured from its intended dispatch. What exceeds it is counted as
	// timed out and never sampled: a starved tenant published as a served call
	// at tens of seconds hides starvation as slowness.
	Deadline time.Duration
	// Repeats is how many times the pair of arms is run. The arms alternate
	// order between repetitions, so a monotone drift of the host does not land
	// on one arm, and the spread between repetitions is the only measure of
	// host noise the verdict has.
	Repeats int
}

// The default shape, sized for a host a developer has and a run that happens
// often.
//
// The quiet rate is far below the shipped bound of ten per second, so a quiet
// refusal is a finding rather than an expected event. The noisy rate is twice
// it, which is enough for the bound to bite without asking the driver to hold
// more sockets than a default file-descriptor limit allows: a credential holds
// at most Rate x Deadline requests outstanding, and the in-flight ceiling is
// twice that so it binds only when the driver itself has fallen behind.
const (
	defaultQuietCredentials = 8
	defaultNoisyCredentials = 4
	defaultQuietRate        = 2.0
	defaultNoisyRate        = 20.0
	defaultFairnessPhase    = 20 * time.Second
	defaultFairnessLeadIn   = 5 * time.Second
	defaultFairnessDeadline = 2 * time.Second
	defaultFairnessRepeats  = 2
)

// fairnessPlanFor builds the plan a set of flags asks for.
func fairnessPlanFor(opts options) (fairnessPlan, error) {
	bound, err := boundByID(opts.fairness)
	if err != nil {
		return fairnessPlan{}, err
	}
	plan := fairnessPlan{
		ID:      "fairness-" + opts.fairnessSurface + "-" + bound.ID,
		Surface: opts.fairnessSurface,
		Bound:   bound,
		Quiet: populationSpec{
			Name: populationQuiet, Credentials: opts.fairnessQuiet,
			Rate: opts.fairnessQuietRate, Verbs: bound.QuietVerbs,
		},
		Noisy: populationSpec{
			Name: populationNoisy, Credentials: opts.fairnessNoisy,
			Rate: opts.fairnessNoisyRate, Verbs: bound.NoisyVerbs,
		},
		Phase:    opts.fairnessPhase,
		LeadIn:   opts.fairnessLeadIn,
		Deadline: opts.fairnessDeadline,
		Repeats:  opts.fairnessRepeats,
	}
	return plan, plan.validate()
}

// validate refuses a plan that would measure something other than what it
// claims.
func (p fairnessPlan) validate() error {
	if p.Bound.Undrivable != "" {
		return fmt.Errorf("bound %s cannot be measured yet: %s", p.Bound.ID, p.Bound.Undrivable)
	}
	if _, err := callFor(p.Surface); err != nil {
		return err
	}
	for _, pop := range []populationSpec{p.Quiet, p.Noisy} {
		if err := pop.validate(); err != nil {
			return err
		}
	}
	if p.Phase <= 0 || p.LeadIn < 0 || p.Deadline <= 0 {
		return fmt.Errorf("the phase and the deadline must be positive and the lead-in must not be negative, got %s, %s and %s",
			p.Phase, p.LeadIn, p.Deadline)
	}
	drain, bites := p.Bound.drain(p.Noisy.Rate)
	if !bites {
		return fmt.Errorf("the noisy population offers %g requests a second per credential against a bound of %g: "+
			"the bound would never refuse it, and the run would spend both arms discovering that",
			p.Noisy.Rate, p.Bound.Bucket.Rate)
	}
	// A phase that opens on a full bucket measures the burst, and a lead-in
	// shorter than the drain leaves that burst inside the measured window.
	if p.LeadIn < drain {
		return fmt.Errorf("the lead-in of %s is shorter than the %s this bound's burst takes to drain at %g requests "+
			"a second: the measured phase would begin with a full bucket and report that the bound does almost nothing",
			p.LeadIn, drain.Round(time.Millisecond), p.Noisy.Rate)
	}
	if p.Repeats <= 0 {
		return fmt.Errorf("-fairness-repeats must be positive, got %d", p.Repeats)
	}
	// The quiet population must be able to fill a percentile. Nearest-rank p99
	// over a handful of samples is the maximum wearing a label nobody measured.
	if ticks := p.ticks(p.Quiet); ticks < minQuietTicks {
		return fmt.Errorf("the quiet population would offer %d requests per credential over a %s phase, "+
			"which is too few for a percentile: raise -fairness-phase or -fairness-quiet-rate", ticks, p.Phase)
	}
	return nil
}

// minQuietTicks is the fewest requests a quiet credential may offer in a
// phase. Below this the population's pooled distribution is a handful of
// observations however many credentials hold it.
const minQuietTicks = 4

// validate refuses a population that would not be a tenant.
func (s populationSpec) validate() error {
	if s.Credentials <= 0 {
		return fmt.Errorf("the %s population needs at least one credential, got %d", s.Name, s.Credentials)
	}
	if s.Rate <= 0 {
		return fmt.Errorf("the %s population needs a positive offered rate, got %g", s.Name, s.Rate)
	}
	if len(s.Verbs) == 0 {
		return fmt.Errorf("the %s population has no verbs to issue", s.Name)
	}
	for _, id := range s.Verbs {
		if _, ok := verbs[id]; !ok {
			return fmt.Errorf("the %s population asks for unknown verb %q", s.Name, id)
		}
	}
	return nil
}

// period is the interval between one credential's requests.
func (s populationSpec) period() time.Duration {
	return time.Duration(float64(time.Second) / s.Rate)
}

// ticks is how many requests one credential of a population offers in a
// window.
func (p fairnessPlan) ticks(s populationSpec) int {
	return int(math.Floor(p.Phase.Seconds() * s.Rate))
}

// leadInTicks is the same for the unmeasured window.
func (p fairnessPlan) leadInTicks(s populationSpec) int {
	return int(math.Floor(p.LeadIn.Seconds() * s.Rate))
}

// inFlight is how many of one credential's requests may be outstanding at
// once.
//
// Twice what the schedule can accumulate, since a request cannot outlive its
// deadline and so at most Rate x Deadline of them are ever in flight. The
// ceiling therefore never binds on a server that is merely slow, and a tick it
// does refuse means the driver itself fell behind, which is a fact about the
// measurement and is recorded as one. A ceiling that bound under load would be
// a closed loop wearing an open loop's clothes: it would censor exactly the
// slow requests, and it would free slots faster in the arm where refusals come
// back in two milliseconds.
func (p fairnessPlan) inFlight(s populationSpec) int {
	return max(1, 2*int(math.Ceil(s.Rate*p.Deadline.Seconds())))
}

// credentials is the index range one population holds.
//
// Ranges over the index the client already carries: each credential is
// bench-token-<index>, and the pool keys on token, so one index is one pool
// entry, one bucket and one connection pool. Two populations are two tenants
// with no new machinery, and they cannot accidentally share a bucket.
func (p fairnessPlan) credentials(s populationSpec) (first, last int) {
	if s.Name == populationQuiet {
		return 0, p.Quiet.Credentials
	}
	return p.Quiet.Credentials, p.Quiet.Credentials + p.Noisy.Credentials
}

// totalCredentials is every credential both populations hold.
func (p fairnessPlan) totalCredentials() int {
	return p.Quiet.Credentials + p.Noisy.Credentials
}

// describe renders the plan for the progress line.
func (p fairnessPlan) describe() string {
	return fmt.Sprintf("%s surface, %s: %d quiet credentials at %g/s against %d noisy at %g/s, %s phase after a %s lead-in, %d repetitions",
		p.Surface, p.Bound.Label, p.Quiet.Credentials, p.Quiet.Rate, p.Noisy.Credentials, p.Noisy.Rate,
		p.Phase, p.LeadIn, p.Repeats)
}

// armArgs and armEnv are the bound's switches for one arm.
func (p fairnessPlan) armArgs(arm string) []string {
	if arm == armOn {
		return p.Bound.onArgs()
	}
	return p.Bound.ArgsOff
}

func (p fairnessPlan) armEnv(arm string) []string {
	if arm == armOn {
		return p.Bound.EnvOn
	}
	return p.Bound.EnvOff
}

// The two arms of a comparison.
const (
	armOff = "off"
	armOn  = "on"
)

// armOrder is the order the two arms run in for one repetition.
//
// Alternating by parity rather than fixed, because the arms are two processes
// at two moments: with a fixed order every difference between them carries
// whatever the host did between the first and the second, and counterbalancing
// turns that bias into spread the verdict can see.
func armOrder(repeat int) []string {
	if repeat%2 == 1 {
		return []string{armOn, armOff}
	}
	return []string{armOff, armOn}
}

// errBoundDidNotFire is the positive control failing: an arm that was supposed
// to have the bound in force refused nothing.
//
// It is an error rather than a null result because the two are opposite
// conclusions from identical numbers. A bound absent from the build, a
// mistyped flag, and two populations that accidentally share a bucket all
// produce "no refusals", and reporting that as "the bound helped nobody" would
// publish a verdict on a bound that was never there.
var errBoundDidNotFire = errors.New("the bound refused nothing")

// classifyOutcome decides what one request did.
//
// The order is the honesty. A nil error is the only way to be served. A
// deadline is a timeout whatever else it looks like, since a client would have
// given up. Only then is the whitelist consulted, and anything that does not
// match one of the declared shapes stays a failure: over-matching would count
// a broken server as a fair one.
func classifyOutcome(method string, err error, refusals []refusalSpec) string {
	switch {
	case err == nil:
		return outcomeServed
	case isDeadline(err):
		return outcomeTimedOut
	}
	if slices.ContainsFunc(refusals, func(r refusalSpec) bool { return r.matches(method, err) }) {
		return outcomeRefused
	}
	return outcomeFailed
}

// The four terminal outcomes of a request. Four rather than two because a
// refusal is neither a success nor a breakage: counted as a success it makes
// the bound look efficient for refusing work, and counted as a failure it
// makes it look broken.
const (
	outcomeServed   = "served"
	outcomeRefused  = "refused"
	outcomeFailed   = "failed"
	outcomeTimedOut = "timed_out"
)

// matches reports whether an error is this refusal shape.
func (r refusalSpec) matches(method string, err error) bool {
	if r.Method != "" && r.Method != method {
		return false
	}
	if responseStatus(err) != r.Status {
		return false
	}
	if r.Code != 0 {
		var jsonErr rpcError
		return errors.As(err, &jsonErr) && jsonErr.Code == r.Code &&
			strings.HasPrefix(jsonErr.Message, r.TextPrefix)
	}
	var toolErr *toolResultError
	return errors.As(err, &toolErr) && strings.HasPrefix(toolErr.Text, r.TextPrefix)
}

// responseStatus is the HTTP status a failed call came back with; a response
// the transport accepted carries none of its own and is reported as 200.
func responseStatus(err error) int {
	if status, ok := errors.AsType[*httpStatusError](err); ok {
		return status.Status
	}
	return httpOK
}

// isDeadline reports a request the client gave up on.
func isDeadline(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// meters reports whether this bound claims to refuse a method at all, which is
// what makes the positive control specific to the bound rather than to any
// refusal that happened to arrive.
func (b boundSpec) meters(method string) bool {
	return slices.ContainsFunc(b.Refusals, func(r refusalSpec) bool {
		return r.Method == "" || r.Method == method
	})
}
