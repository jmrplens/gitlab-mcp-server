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
//
// One bound has no switch at all: the OAuth verification ceiling is a constant
// no operator can move (register row ADM-014), so the arm without it cannot be
// the same binary started differently. Its bound names a variant instead, a
// build of this checkout with the one declaration that sizes the ceiling
// replaced, so the two arms still differ in exactly one thing and the server
// carries no switch that takes a security bound out.
package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
//
// The last two are the same methods as the first two, carrying a credential
// other than the lane's own. A bound on verification cannot be reached by a
// credential that was verified before the phase began, which is every
// credential the first two present: what reaches it is a credential presented
// for the first time, and a token no GitLab ever issued.
const (
	verbCall         = "call"
	verbList         = "list"
	verbCallNew      = "call-new"
	verbListInvented = "list-invented"
)

// The credentials a verb may present in place of its lane's own.
//
// The lane's own is the one admission verified before the phase, or the new
// one it most recently presented and was served with, which is how a new
// credential is presented once and then reused, as a client that has just
// been issued a token does. Either way it is one the server already holds, so
// the rows that present it are the population's cached credentials.
const (
	credentialOwn      = ""
	credentialNew      = "new"
	credentialInvented = "invented"
)

// verbSpec is one request a population issues.
type verbSpec struct {
	ID     string
	Method string
	// Credential is which credential the request presents: the lane's own
	// when empty, one presented for the first time, or one no GitLab issued.
	Credential string
	// params builds fresh parameters per request. Fresh because the request
	// encoder writes the per-request _meta into the map it is handed, so a
	// shared one would be written by every goroutine at once.
	params func(call toolCall) map[string]any
	// detail names what was called, since the tool differs per surface and a
	// percentile with no call behind it is not comparable to anything.
	detail func(call toolCall) string
}

// key is the row this verb's requests are recorded under.
func (v verbSpec) key() string { return rowKey(v.Method, v.Credential) }

// rowKey names one row of a population's record: the method, qualified by the
// credential it presented when that was not the lane's own.
//
// One method is two rows when a population presents two kinds of credential
// with it, and the two must never be one distribution: a first presentation
// that waited five seconds for a verification slot and a cached credential
// answered at once are different experiences of the same call, and a
// percentile over both would report neither.
func rowKey(method, credential string) string {
	if credential == credentialOwn {
		return method
	}
	return method + " (" + credential + " credential)"
}

// callParams and listParams build the two methods' parameters, shared by the
// verbs that send the same method with another credential.
func callParams(call toolCall) map[string]any {
	return map[string]any{"name": call.Name, "arguments": call.Args}
}

func listParams(toolCall) map[string]any { return nil }

func callDetail(call toolCall) string { return call.Detail }

func listDetail(toolCall) string { return detailWholeSurface }

// verbs are every request kind a population can be given.
var verbs = map[string]verbSpec{
	verbCall:         {ID: verbCall, Method: methodToolsCall, params: callParams, detail: callDetail},
	verbList:         {ID: verbList, Method: methodToolsList, params: listParams, detail: listDetail},
	verbCallNew:      {ID: verbCallNew, Method: methodToolsCall, Credential: credentialNew, params: callParams, detail: callDetail},
	verbListInvented: {ID: verbListInvented, Method: methodToolsList, Credential: credentialInvented, params: listParams, detail: listDetail},
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
	// Rate and Burst are what the server is told, and are the switches the
	// on-arm passes verbatim.
	Rate  float64
	Burst int
	// Metered, when set, is the rate this bound actually refuses above, for a
	// bucket the server derives from the configured one instead of metering
	// the configured one itself.
	//
	// The two are one number for most bounds and were held as one until the
	// first real run against a server that meters listings, where they are
	// not: that bucket refills a tenth as fast as the rate its flag names and
	// keeps the same burst. Told only the configured rate, the plan demanded a
	// noisy population above ten listings a second before it would believe the
	// bound could bite, when one a second is the truth, and it then computed a
	// lead-in from a drain that was ten times too slow. Which number belongs
	// where is not a detail: the switches say what to pass, and this says what
	// a population has to exceed to be refused at all.
	Metered float64
}

// meteredRate is the rate a population must exceed before this bound refuses
// it, which is the configured rate unless the server derives a slower bucket
// from it.
func (b bucketSpec) meteredRate() float64 {
	if b.Metered > 0 {
		return b.Metered
	}
	return b.Rate
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

	// OAuth starts both arms in OAuth mode, with every credential presented
	// as a bearer token.
	OAuth bool
	// Variant, when set, is what the arm without the bound runs in place of
	// the binary under test: this checkout with one declaration replaced. It
	// is for a bound no switch reaches, and ArgsOff then equals ArgsOn.
	Variant *buildVariant
	// Slots, when set, is a ceiling on concurrent work that makes a request
	// wait for a slot and refuses it when none frees, as the verification
	// ceiling does. It is what the plan reasons about for such a bound, the
	// way Bucket is for a rate.
	Slots *slotSpec
	// Otherwise is every wire shape a refusal arrives in that is expected of
	// the run and is not this bound's: the 401 an invented token earns is the
	// whole of what a flood of them is answered with when the bound is out.
	// Such a request is refused otherwise, which is neither this bound's
	// refusal nor a failure of the run.
	Otherwise []refusalSpec
	// Protects names what the bound protects when that is not the quiet
	// population. Such a bound is not measured for whether it leaves the quiet
	// tenant better off, which it never claimed, but for what it costs that
	// tenant, and the verdict's sentences say so.
	Protects string
	// Shared names what the two populations contend for when it is not the
	// machine. The saturation gate is about the machine: a bound whose
	// populations contend for its own slots reaches the quiet tenant whether
	// or not the host is busy.
	Shared string
	// Defaults are the settings a run of this bound takes where the flag was
	// not given, for a bound the command's own defaults would not drive.
	Defaults planDefaults
}

// planDefaults are a bound's own defaults for the settings a flag can name. A
// zero field keeps the command's default.
type planDefaults struct {
	Quiet, Noisy            int
	NoisyRate               float64
	Phase, LeadIn, Deadline time.Duration
	UpstreamDelay           time.Duration
}

// slotSpec is a ceiling on concurrent work: how many requests hold a slot at
// once, how long a request waits for one, and how many round trips to the
// instance a request holds it for.
//
// Held as numbers for the same reason bucketSpec is. A flood that the slots
// can serve as fast as it arrives never fills them, and a lead-in shorter than
// the wait measures a queue that has not formed yet; both follow from these
// figures and the round trip the run puts in front of every verification.
type slotSpec struct {
	// Count and Wait are the ceiling itself.
	Count int
	Wait  time.Duration
	// InventedRoundTrips is how many requests to the instance a refused
	// credential holds a slot for, and NewRoundTrips the same for one the
	// instance accepts.
	InventedRoundTrips, NewRoundTrips int
}

// capacity is how many invented credentials the slots finish a second when
// each request to the instance takes delay, which is what a flood has to
// exceed before any of it waits.
func (s slotSpec) capacity(delay time.Duration) float64 {
	return float64(s.Count) / (float64(s.InventedRoundTrips) * delay.Seconds())
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

// meteredShare is the fraction of a population's ticks this bound refuses at
// all, since a population cycles its verbs one per tick and a bound meters
// some methods and not others.
//
// Without it every number derived from an offered rate is wrong by whatever
// fraction of that rate the bound never looks at: a population alternating a
// call and a listing against a bound that meters listings offers half what its
// rate says, and a drain, a headroom and an expected refusal count computed
// from the whole rate would each be out by the same factor.
func (b boundSpec) meteredShare(verbIDs []string) float64 {
	if len(verbIDs) == 0 {
		return 0
	}
	metered := 0
	for _, id := range verbIDs {
		if b.meters(verbs[id].Method) {
			metered++
		}
	}
	return float64(metered) / float64(len(verbIDs))
}

// meteredOffered is the rate one credential of a population offers the bound,
// counting only the verbs the bound meters.
func (b boundSpec) meteredOffered(s populationSpec) float64 {
	return s.Rate * b.meteredShare(s.Verbs)
}

// quietRate is the rate a quiet credential takes when the flag names none.
//
// Derived from the bound rather than fixed, because a rate that is a tenth of
// one bound is ten times another: the shipped bucket meters ten requests a
// second and the listing bucket it derives meters one, so the two-a-second
// default that leaves the first a tenfold margin sits exactly on the second.
// A population sitting on the bound is not the quiet tenant this scenario
// reports on, and the fix cannot be a second constant per bound or the next
// bound is a copy of this file rather than a literal in it.
//
// Capped at the shipped default, since the question is what a bound does for a
// tenant working at a human rate, and a bound metered high enough to allow
// more than that does not make the tenant busier.
func (b boundSpec) quietRate() float64 {
	if b.Bucket == nil {
		return defaultQuietRate
	}
	share := b.meteredShare(b.QuietVerbs)
	if share <= 0 {
		return defaultQuietRate
	}
	return min(quietDefaultShare*b.Bucket.meteredRate()/share, defaultQuietRate)
}

// quietDefaultShare and quietMaxShare are how much of a bound a quiet tenant
// takes by default and the most it may take at all.
//
// Two numbers rather than one so the default sits a factor of two inside the
// rule: a hand-edited rate a little above the default is still a quiet tenant,
// and a plan is only refused when the population is close enough to the bound
// that the bound would refuse it.
const (
	quietDefaultShare = 0.25
	quietMaxShare     = 0.5
)

// drain is how long the burst takes to empty at an offered rate, and whether
// that rate exceeds the bound at all.
//
// Both answers are about the bucket the bound actually meters rather than the
// one its flags configure, since a population is refused by the first and not
// by the second.
//
// A population offering no more than the bound allows is never refused, so
// there is no drain and no measurement: the caller refuses the plan rather
// than spending two arms finding out.
func (b boundSpec) drain(offered float64) (time.Duration, bool) {
	if b.Bucket == nil {
		return 0, true
	}
	metered := b.Bucket.meteredRate()
	if offered <= metered {
		return 0, false
	}
	return time.Duration(float64(b.Bucket.Burst) / (offered - metered) * float64(time.Second)), true
}

// The refusal every rate-limit bucket writes, read from the server rather than
// restated, so a change of wording there is a compile error here.
const rateLimitRefusal = toolutil.RateLimitRefusalPrefix

// serverBusyCode is what the listen ceilings refuse with, which is a different
// number from the buckets': a classifier that keyed on one code alone would
// count one bound's refusals against the other.
const serverBusyCode = -32000

// The statuses a refusal can arrive with, spelled here so a refusalSpec reads
// as data.
const (
	httpOK                 = 200
	httpUnauthorized       = 401
	httpTooManyRequests    = 429
	httpServiceUnavailable = 503
)

// registerRefusal is the refusal a register row declares at one status, in the
// shape this classifier matches.
//
// Read from the register rather than restated, because the register is where
// a refusal's words are held to the code that writes them (make check-tenancy,
// G8): a sentence copied here would drift from the server without anything
// noticing, and every refusal it stopped matching would count as a failure of
// the run. A row or status the register does not hold gives the zero shape,
// which matches nothing and which the bound table's own test refuses.
func registerRefusal(id string, status int) refusalSpec {
	row, ok := tenancy.Lookup(id)
	if !ok {
		return refusalSpec{}
	}
	for _, refusal := range row.Refusals {
		if refusal.Status == status && refusal.Prefix != "" {
			return refusalSpec{Status: refusal.Status, Code: refusal.Code, TextPrefix: refusal.Prefix}
		}
	}
	return refusalSpec{}
}

// oauthArgs start a server in OAuth mode behind the loopback addresses the
// flood arrives from, with the rate limit off in both arms.
//
// The public URL is required by OAuth mode and is never dialed. The trusted
// proxies are the whole loopback range because the flood's transport sources
// are loopback addresses of their own: behind one proxy the transport-source
// budget (register row AUB-002) blocks a flood after five hundred distinct
// forwarded addresses a minute, so a flood large enough to fill the slots
// arrives the way a large one does, through many sources, and no budget but
// the one under test turns any of it away. The rate limit is off because it is
// per credential and every credential here is new.
var oauthArgs = []string{
	limiterOffArg,
	"--auth-mode=oauth",
	"--public-url=https://bench.invalid",
	"--trusted-proxies=127.0.0.0/8",
	"--trusted-proxy-header=" + headerForwardedFor,
}

// headerForwardedFor is the header the flood names its client address in.
const headerForwardedFor = "X-Forwarded-For"

// fairnessBounds are the bounds this scenario can be pointed at.
//
// Several are declared rather than one, because a shape fitted to a single
// instance is not a shape. The token bucket is a flag and refuses a rate; the
// listen ceiling is an environment variable and refuses a held resource; the
// metered listing shares the first one's switch while refusing a different
// method, which is why the refusal shape is per bound rather than one global
// classifier; and the process's listing bucket shares that switch too while
// counting the whole process rather than one credential. It refuses in the
// credential's own words, so what makes its refusals its own is the arm that
// puts it in force, which leaves every credential's bucket out of reach.
var fairnessBounds = []boundSpec{
	{
		ID:    "tools-call-rps",
		Label: "the per-credential request bucket (--rate-limit-rps)",
		// The shipped HTTP default, which is the configuration whose fairness
		// claim is worth testing: a deployment that changed it is measuring a
		// setting it chose, and can say so with the flags below.
		ArgsOff: []string{limiterOffArg},
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
		//
		// Derived, and so metered a tenth as fast as the flag says while
		// holding the same burst: a listing arrives once at connect and again
		// when the catalog changes, and costs far more to answer than a tool
		// call, so the server refills its bucket that much more slowly. The
		// ten is a literal here because this command measures a binary it did
		// not build and cannot read a constant out of; what catches it going
		// stale is the positive control, which stops a run whose on-arm
		// refused nothing rather than reporting a bound that helped nobody.
		ArgsOff: []string{limiterOffArg},
		Bucket:  &bucketSpec{Rate: 10, Burst: 40, Metered: 1},
		Refusals: []refusalSpec{
			{Status: httpOK, Code: rateLimitCode, TextPrefix: rateLimitRefusal, Method: methodToolsList},
		},
		NoisyVerbs: []string{verbList},
		QuietVerbs: []string{verbCall, verbList},
	},
	{
		ID:    "tools-list-process",
		Label: "the listing bucket the whole process shares",
		// The bucket keyed on the process beside every credential's listing
		// bucket (register row RTC-007), put in force on its own: the on-arm
		// gives each credential a listing bucket of a hundred a second and ten
		// thousand in hand, which no noisy credential here reaches, so every
		// listing refused is the process's. That is the arm's doing and not
		// the classifier's: the process refuses in the credential's words, so
		// that a caller is not told other callers are listing (INV-019), and
		// only the server's log line says which bucket refused. The off-arm
		// turns the rate limit off, which turns this bucket off with it
		// (issue 951). No Bucket,
		// because it counts tools rather than requests: a listing on the
		// individual surface is some nine hundred of them against a refill of
		// three thousand a second, so the surface decides the rate a noisy
		// population must exceed, and only individual gives a population of
		// this size something to exceed (-fairness-surface individual). On
		// dynamic the positive control stops the run instead.
		//
		// The quiet population only calls tools, because what this bucket
		// claims to protect is the processor its co-tenants' calls wait for,
		// and it refuses newcomers whoever they are: a quiet tenant listing
		// too would be refused by a bound that promises no one a share.
		ArgsOff: []string{limiterOffArg},
		ArgsOn:  []string{"--rate-limit-rps=1000", "--rate-limit-burst=10000"},
		Refusals: []refusalSpec{
			{
				Status: httpOK, Code: rateLimitCode, Method: methodToolsList,
				TextPrefix: rateLimitRefusal + methodToolsList,
			},
		},
		NoisyVerbs: []string{verbList},
		QuietVerbs: []string{verbCall},
	},
	{
		ID:      "listen-streams",
		Label:   "the per-credential subscriptions/listen ceiling",
		ArgsOff: []string{limiterOffArg},
		ArgsOn:  []string{limiterOffArg},
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
	{
		ID:    "oauth-verification",
		Label: "the OAuth verification ceiling (register row ADM-014)",
		// The ceiling is sixteen verifications at once with a five-second
		// wait, both constants no operator can move, so both arms start with
		// the same switches and the arm without it runs a build whose slots
		// no flood here can fill. Everything else about the two binaries is
		// the one checkout.
		OAuth:   true,
		ArgsOff: oauthArgs,
		ArgsOn:  oauthArgs,
		Variant: &buildVariant{
			File:        "internal/oauth/verifier.go",
			Declaration: "const verificationSlots = ",
			Replacement: "const verificationSlots = 1 << 20 // cmd/bench_resources: the arm without the ceiling",
		},
		// An invented token costs its slot one request, which GitLab answers
		// 401; a token GitLab accepts costs three, the identity and the two
		// scope introspection requests, which the stand-in answers the way an
		// instance that will not describe a token does.
		Slots: &slotSpec{
			Count: tenancy.OAuthVerifications, Wait: tenancy.OAuthVerificationWait,
			InventedRoundTrips: 1, NewRoundTrips: 3,
		},
		Refusals:  []refusalSpec{registerRefusal("ADM-014", httpServiceUnavailable)},
		Otherwise: []refusalSpec{registerRefusal("ADM-002", httpUnauthorized)},
		Protects:  "the GitLab instance the verifications reach",
		Shared:    "the verification slots",
		// The flood is nothing but invented tokens. The quiet population
		// presents one new credential in every four requests and reuses it
		// for the rest, so its two rows are the onboarding the ceiling can
		// cost and the cached credentials it must not.
		NoisyVerbs: []string{verbListInvented},
		QuietVerbs: []string{verbCallNew, verbCall, verbCall, verbCall},
		// Sized for a host of a few cores: four hundred invented tokens a
		// second against slots that finish a hundred and sixty at a hundred
		// milliseconds a round trip, which is a round trip GitLab.com answers
		// the identity request in from a nearby region. The lead-in outlasts
		// the wait, so the queue has formed before the phase begins, and the
		// deadline outlasts the wait, so a client that waits it out is not
		// counted as one that gave up.
		Defaults: planDefaults{
			Noisy: 8, NoisyRate: 50,
			Phase: 30 * time.Second, LeadIn: 10 * time.Second, Deadline: 15 * time.Second,
			UpstreamDelay: 100 * time.Millisecond,
		},
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
	// UpstreamDelay is how long the stand-in GitLab takes to answer each
	// token verification request. Zero answers at once, which is what every
	// scenario but a verification bound's measures with: on loopback a slot
	// frees in microseconds, so no flood this driver can offer fills one.
	UpstreamDelay time.Duration
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
	// A quiet rate the flag did not name is taken from the bound, since what
	// counts as quiet is a fact about the bound and not about this command.
	quietRate := opts.fairnessQuietRate
	if quietRate <= 0 {
		quietRate = bound.quietRate()
	}
	// The same holds for the upstream round trip, and for every setting the
	// bound has a default of its own for: a flag the caller typed wins, and a
	// flag left at the command's default takes the bound's.
	delay := opts.fairnessUpstreamDelay
	if delay <= 0 {
		delay = bound.Defaults.UpstreamDelay
	}
	given := opts.fairnessSet
	defaults := bound.Defaults
	plan := fairnessPlan{
		ID:      "fairness-" + opts.fairnessSurface + "-" + bound.ID,
		Surface: opts.fairnessSurface,
		Bound:   bound,
		Quiet: populationSpec{
			Name: populationQuiet, Credentials: setting(given[flagFairnessQuiet], opts.fairnessQuiet, defaults.Quiet),
			Rate: quietRate, Verbs: bound.QuietVerbs,
		},
		Noisy: populationSpec{
			Name: populationNoisy, Credentials: setting(given[flagFairnessNoisy], opts.fairnessNoisy, defaults.Noisy),
			Rate:  setting(given[flagFairnessNoisyRate], opts.fairnessNoisyRate, defaults.NoisyRate),
			Verbs: bound.NoisyVerbs,
		},
		Phase:         setting(given[flagFairnessPhase], opts.fairnessPhase, defaults.Phase),
		LeadIn:        setting(given[flagFairnessLeadIn], opts.fairnessLeadIn, defaults.LeadIn),
		Deadline:      setting(given[flagFairnessDeadline], opts.fairnessDeadline, defaults.Deadline),
		Repeats:       opts.fairnessRepeats,
		UpstreamDelay: delay,
	}
	return plan, plan.validate()
}

// setting is the value a plan takes for one setting: the flag's when the
// caller typed it or the bound has no default of its own, the bound's
// otherwise.
func setting[T comparable](typed bool, flagged, bound T) T {
	var none T
	if typed || bound == none {
		return flagged
	}
	return bound
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
	if sources := p.floodSources(); sources > maxFloodSources {
		return fmt.Errorf("the noisy population's invented tokens would need %d transport sources to stay under the "+
			"transport-source budget, more than the %d that 127.2.0.0/16 holds: lower -fairness-noisy-rate or -fairness-noisy",
			sources, maxFloodSources)
	}
	// Both populations are measured against the bound by what they offer it
	// rather than by their rate, which are the same number only for a
	// population whose every verb the bound meters.
	noisyOffered := p.Bound.meteredOffered(p.Noisy)
	drain, bites := p.Bound.drain(noisyOffered)
	if !bites {
		return fmt.Errorf("the noisy population offers %g metered requests a second per credential against a bound of %g: "+
			"the bound would never refuse it, and the run would spend both arms discovering that",
			noisyOffered, p.Bound.Bucket.meteredRate())
	}
	if err := p.quietIsQuiet(); err != nil {
		return err
	}
	if err := p.slotsFill(); err != nil {
		return err
	}
	// A phase that opens on a full bucket measures the burst, and a lead-in
	// shorter than the drain leaves that burst inside the measured window.
	if p.LeadIn < drain {
		return fmt.Errorf("the lead-in of %s is shorter than the %s this bound's burst takes to drain at %g metered requests "+
			"a second: the measured phase would begin with a full bucket and report that the bound does almost nothing",
			p.LeadIn, drain.Round(time.Millisecond), noisyOffered)
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

// quietIsQuiet refuses a plan whose quiet population the bound would refuse.
//
// The mirror of the check above it, and it was missing while that one was
// there: the plan refused a noisy population the bound would never bite, and
// accepted a quiet population sitting on top of the bound. Such a run reports
// on a tenant that was itself being turned away, which is not the tenant this
// scenario claims to be reporting on, and the numbers it produces are the
// numbers of a bound protecting nobody.
//
// Counted in what the bound actually meters, so a population alternating a
// metered verb with an unmetered one is judged by the half that reaches the
// bucket.
func (p fairnessPlan) quietIsQuiet() error {
	if p.Bound.Bucket == nil {
		return nil
	}
	offered := p.Bound.meteredOffered(p.Quiet)
	ceiling := quietMaxShare * p.Bound.Bucket.meteredRate()
	if offered <= ceiling {
		return nil
	}
	return fmt.Errorf("the quiet population offers %g metered requests a second per credential against a bound of %g, "+
		"above the %g this comparison allows it: a population the bound refuses is not the quiet tenant the bound "+
		"exists to protect, and the run would report on a tenant it was turning away. Lower -fairness-quiet-rate, "+
		"or leave it unset to take %g from the bound itself",
		offered, p.Bound.Bucket.meteredRate(), ceiling, p.Bound.quietRate())
}

// slotsFill refuses a plan that could not fill a slot bound, or that would
// measure its queue before the queue had formed.
//
// Each check is the slot counterpart of one the bucket gets above. A flood the
// slots finish as fast as it arrives is a noisy population the bound never
// refuses; a lead-in shorter than the wait is a phase that begins before the
// first waiter has been turned away; and two more are the slots' own. With no
// round trip at the instance a slot frees in microseconds, so no rate this
// driver can offer holds one. And a deadline shorter than a new credential's
// wait and verification counts a client that waited the ceiling out as one
// that gave up, which files the cost this bound imposes under the wrong
// outcome.
func (p fairnessPlan) slotsFill() error {
	slots := p.Bound.Slots
	if slots == nil {
		return nil
	}
	if p.UpstreamDelay <= 0 {
		return fmt.Errorf("%s holds a slot for as long as the instance takes to answer, and the stand-in's round trip "+
			"is %s: a slot then frees in microseconds and no flood this driver can offer holds one. "+
			"Name one with -fairness-upstream-delay", p.Bound.Label, p.UpstreamDelay)
	}
	offered := p.Bound.meteredOffered(p.Noisy) * float64(p.Noisy.Credentials)
	capacity := slots.capacity(p.UpstreamDelay)
	if offered <= capacity {
		return fmt.Errorf("the noisy population offers %g requests a second against %d slots that finish %g a second "+
			"at a %s round trip: the slots would never all be held, and the run would spend both arms discovering that",
			offered, slots.Count, capacity, p.UpstreamDelay)
	}
	if p.LeadIn < slots.Wait {
		return fmt.Errorf("the lead-in of %s is shorter than the %s a request waits for a slot: the measured phase would "+
			"begin before the first waiter was refused, and report a queue that had not formed", p.LeadIn, slots.Wait)
	}
	if floor := p.slotDeadlineFloor(); p.Deadline < floor {
		return fmt.Errorf("the deadline of %s is shorter than the %s a new credential may take to wait for a slot and "+
			"make its %d round trips: a client that waited the ceiling out would be counted as one that gave up",
			p.Deadline, floor, slots.NewRoundTrips)
	}
	// The quiet population's own new credentials take slots too, and a
	// population whose onboarding alone kept half of them busy would be the
	// contention rather than the tenant it falls on.
	newcomers := float64(p.Quiet.Credentials) * p.Quiet.Rate * credentialShare(p.Quiet.Verbs, credentialNew)
	if held := newcomers * float64(slots.NewRoundTrips) * p.UpstreamDelay.Seconds(); held > quietMaxShare*float64(slots.Count) {
		return fmt.Errorf("the quiet population presents %g new credentials a second, which keep %.1f of the %d slots "+
			"busy on their own, above the %g this comparison allows it: lower -fairness-quiet-rate or -fairness-quiet",
			newcomers, held, slots.Count, quietMaxShare*float64(slots.Count))
	}
	return nil
}

// slotDeadlineFloor is the least a request may take before its client gives up
// on a slot bound: the whole wait, a new credential's round trips and the
// call's own, and a second for the server between them.
func (p fairnessPlan) slotDeadlineFloor() time.Duration {
	trips := int64(p.Bound.Slots.NewRoundTrips + 1)
	return p.Bound.Slots.Wait + time.Duration(trips*int64(p.UpstreamDelay)) + slotDeadlineMargin
}

// slotDeadlineMargin is the server's own time inside a slot bound's deadline:
// admission, the pool and the call, which on loopback take milliseconds, with
// room for a host busy enough to be worth measuring.
const slotDeadlineMargin = time.Second

// credentialShare is the fraction of a verb cycle that presents one kind of
// credential.
func credentialShare(verbIDs []string, credential string) float64 {
	if len(verbIDs) == 0 {
		return 0
	}
	matching := 0
	for _, id := range verbIDs {
		if verbs[id].Credential == credential {
			matching++
		}
	}
	return float64(matching) / float64(len(verbIDs))
}

// floodSources is how many transport sources the noisy population's invented
// tokens arrive from, and zero when it presents none.
//
// Enough that each stays at half of what the transport-source budget
// (register row AUB-002) counts in a window: that budget blocks a source once
// five hundred distinct forwarded addresses have failed through it inside a
// minute, and then refuses everything from that source, the quiet tenant's
// new credentials included, before any slot is asked for. A flood that tripped
// it would measure that budget rather than the ceiling.
func (p fairnessPlan) floodSources() int {
	share := credentialShare(p.Noisy.Verbs, credentialInvented)
	if share == 0 {
		return 0
	}
	perWindow := float64(p.Noisy.Credentials) * p.Noisy.Rate * share * tenancy.AuthFailureWindow.Seconds()
	return max(1, int(math.Ceil(perWindow/(sourceKeyShare*tenancy.TransportSourceDistinctKeys))))
}

// sourceKeyShare is how much of the transport-source budget one flood source
// may spend in a window.
const sourceKeyShare = 0.5

// poolEntries is how many credentials one arm presents, the new ones
// included, which is what the server's pool is sized to: a credential the pool
// evicted mid-phase would be rebuilt, and the rebuild measured as its latency.
func (p fairnessPlan) poolEntries() int {
	fresh := credentialShare(p.Quiet.Verbs, credentialNew) *
		float64(p.Quiet.Credentials*(p.leadInTicks(p.Quiet)+p.ticks(p.Quiet)))
	return min(p.totalCredentials()+int(math.Ceil(fresh)), tenancy.PoolSizeMax)
}

// refusalsExpected is how many requests this bound must refuse over a phase if
// it is in force at all: everything a population offers above what the bound
// meters, for as long as the phase lasts.
//
// It exists so the positive control can be a quantity rather than a boolean. A
// bound that fired once in three thousand requests and a bound absent from the
// build are the same arm to a control that only asks whether anything was
// refused, and telling them apart is the whole job of that control.
//
// A slot bound's capacity is the process's rather than one credential's, so
// what a population offers above it is counted over all its credentials at
// once.
func (p fairnessPlan) refusalsExpected(s populationSpec) float64 {
	if slots := p.Bound.Slots; slots != nil {
		above := p.Bound.meteredOffered(s)*float64(s.Credentials) - slots.capacity(p.UpstreamDelay)
		return max(0, above) * p.Phase.Seconds()
	}
	if p.Bound.Bucket == nil {
		return 0
	}
	above := p.Bound.meteredOffered(s) - p.Bound.Bucket.meteredRate()
	return max(0, above) * float64(s.Credentials) * p.Phase.Seconds()
}

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
	line := fmt.Sprintf("%s surface, %s: %d quiet credentials at %g/s against %d noisy at %g/s, %s phase after a %s lead-in, %d repetitions",
		p.Surface, p.Bound.Label, p.Quiet.Credentials, p.Quiet.Rate, p.Noisy.Credentials, p.Noisy.Rate,
		p.Phase, p.LeadIn, p.Repeats)
	if p.UpstreamDelay > 0 {
		line += fmt.Sprintf(", %s at the instance for each verification request", p.UpstreamDelay)
	}
	if sources := p.floodSources(); sources > 0 {
		line += fmt.Sprintf(", invented tokens from %d transport sources", sources)
	}
	return line
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

// classifyExpecting is classifyOutcome with the refusals a run expects that are
// not the bound's: a failure one of those shapes matches is refused otherwise.
//
// Asked only of what classifyOutcome called a failure, so a shape listed in
// both places is the bound's, and a request the client gave up on stays timed
// out whatever it would have been answered with.
func classifyExpecting(method string, err error, refusals, otherwise []refusalSpec) string {
	kind := classifyOutcome(method, err, refusals)
	if kind == outcomeFailed && slices.ContainsFunc(otherwise, func(r refusalSpec) bool { return r.matches(method, err) }) {
		return outcomeRefusedOther
	}
	return kind
}

// The terminal outcomes of a request. More than two because a refusal is
// neither a success nor a breakage: counted as a success it makes the bound
// look efficient for refusing work, and counted as a failure it makes it look
// broken. And a refusal the run expects that is not the bound's, the 401 an
// invented token earns, is neither the bound's refusal nor a failure.
const (
	outcomeServed       = "served"
	outcomeRefused      = "refused"
	outcomeRefusedOther = "refused_other"
	outcomeFailed       = "failed"
	outcomeTimedOut     = "timed_out"
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
