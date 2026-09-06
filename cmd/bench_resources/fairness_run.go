// fairness_run.go drives a fairness comparison: two populations, two arms, and
// a tally that keeps served and refused apart.
//
// The driver here is deliberately not steadyLoad. That one is a closed loop
// with no think time, which is the right shape for a capacity question and the
// wrong one for this: a refusal returns in about two milliseconds against tens
// for a served call, so a closed-loop noisy population would offer far more
// requests in the arm with the bound on, and the comparison would be between
// two different experiments. The schedule below is computed from the plan
// before the phase starts and is identical in both arms, and every request's
// latency is measured from its intended instant rather than from when it
// actually left, so a request queued behind the noisy population is charged
// the queueing instead of the driver quietly not sending it.

package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// runFairness measures one fairness comparison and writes its own document.
//
// It returns before the record is read or any chart is drawn, so this mode
// cannot rewrite the published record or a figure: the committed artifacts are
// gated by a byte comparison and their rendering is being reworked separately,
// and a scenario that is not yet drawn should not be drawn twice.
func runFairness(opts options, root string) error {
	plan, err := fairnessPlanFor(opts)
	if err != nil {
		return err
	}
	out := resolve(root, opts.fairnessJSON)
	// Held against the published path as well as against whatever -json names,
	// because the published one is a constant and the flag is not: compared
	// only against the flag, redirecting -json elsewhere disarmed the guard and
	// left the record, whose charts and tables are gated by a byte comparison,
	// writable by this mode.
	for _, published := range []string{opts.record, defaultRecord} {
		if out == resolve(root, published) {
			return fmt.Errorf("-fairness-json names %s, which is the published record: "+
				"give the fairness run a path of its own", rel(root, out))
		}
	}

	r, cleanup, err := newHarness(opts, root)
	if err != nil {
		return err
	}
	defer cleanup()

	fmt.Printf("%s: %s\n", plan.ID, plan.describe())
	// The header is taken before the run rather than during it, because none of
	// it is a measurement: it is the machine, the plan and the switches, and
	// reading the host's own description is not something an arm's context has
	// any business canceling.
	doc := fairnessHeader(plan)
	if runErr := r.runFairnessScenario(context.Background(), plan, doc); runErr != nil {
		return runErr
	}
	if writeErr := writeFairness(out, doc); writeErr != nil {
		return writeErr
	}
	fmt.Printf("wrote %s\n", rel(root, out))
	fmt.Println(doc.summary())
	return nil
}

// fairnessHeader is everything the document says about a run before any of it
// has happened: when, on what, and with which switches.
func fairnessHeader(plan fairnessPlan) *FairnessDoc {
	return &FairnessDoc{
		Schema:      fairnessSchema,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Host:        hostInfo(),
		Settings:    settingsFor(plan),
		Bound:       boundRecord(plan),
	}
}

// runFairnessScenario runs every repetition into the document the caller
// prepared, and judges it.
func (r *runner) runFairnessScenario(ctx context.Context, plan fairnessPlan, doc *FairnessDoc) error {
	for repeat := range plan.Repeats {
		order := armOrder(repeat)
		record := FairnessRepeat{Index: repeat, Order: order}
		for position, arm := range order {
			r.sayf("  repeat %d, %s arm (%d of %d)", repeat+1, arm, position+1, len(order))
			measured, err := r.runFairnessArm(ctx, plan, arm)
			if err != nil {
				return fmt.Errorf("repeat %d, %s arm: %w", repeat+1, arm, err)
			}
			r.sayf("      %s", measured.summary())
			record.Arms = append(record.Arms, measured)
		}
		doc.Repeats = append(doc.Repeats, record)
	}
	doc.Server = r.serverInfo
	doc.Comparisons, doc.Verdict = judgeFairness(doc)
	return nil
}

// runFairnessArm measures one arm: a fresh process with the bound in the state
// this arm names, every credential admitted and warmed the same way, an
// unmeasured lead-in, and then the phase.
func (r *runner) runFairnessArm(ctx context.Context, plan fairnessPlan, arm string) (FairnessArm, error) {
	call, err := callFor(plan.Surface)
	if err != nil {
		return FairnessArm{}, err
	}
	tgt := &httpTarget{
		binary:  r.binary,
		plan:    scenarioPlan{ID: plan.ID, Transport: transportHTTP, Surface: plan.Surface},
		stubURL: r.stub.url, otlpURL: r.otlp.url,
		// Sized to both populations so nothing admitted is evicted under the
		// pool's own default while the phase is running.
		maxClients: plan.totalCredentials(),
		boundArgs:  plan.armArgs(arm),
		boundEnv:   plan.armEnv(arm),
	}
	defer tgt.close()

	server := newSampler(ctx, r.sampleInterval, func() []int { return pids(tgt.processes()) })
	server.start()
	defer server.stop()
	// The driver runs on the same host as the server it measures, and it costs
	// very different amounts in the two arms: parsing a two-kilobyte refusal is
	// nothing beside a hundred-and-seventy-kilobyte result. Sampling it is what
	// lets the verdict refuse an improvement the driver could have caused.
	driver := newSampler(ctx, r.sampleInterval, func() []int { return []int{os.Getpid()} })
	driver.start()
	defer driver.stop()

	if _, startErr := tgt.start(ctx); startErr != nil {
		return FairnessArm{}, startErr
	}
	if info := tgt.serverInfo(); info.Version != "" {
		r.serverInfo = info
	}
	conns, admitErr := r.admit(ctx, tgt, 0, plan.totalCredentials())
	defer closeConns(conns)
	if admitErr != nil {
		return FairnessArm{}, admitErr
	}

	// The lead-in is discarded on purpose and both arms pay it: it drains the
	// bound's burst, so the measured window reports the bound rather than the
	// bucket it started full, and it warms a pool and a heap that a fresh
	// process has neither of.
	drive(ctx, plan, conns, call, plan.leadInTicks, newFairTally(call, plan.Bound.Refusals))

	tally := newFairTally(call, plan.Bound.Refusals)
	server.resetPeak()
	serverStart, serverStartOK := sampleCPU(server)
	driverStart, driverStartOK := sampleCPU(driver)
	phaseStarted := time.Now()
	drive(ctx, plan, conns, call, plan.ticks, tally)
	phaseWall := time.Since(phaseStarted)
	serverEnd, serverEndOK := sampleCPU(server)
	driverEnd, driverEndOK := sampleCPU(driver)

	measured := FairnessArm{
		Arm:         arm,
		Args:        plan.armArgs(arm),
		Env:         plan.armEnv(arm),
		Populations: tally.populations(plan),
	}
	measured.Process, measured.Notes = fairnessProcess(processInput{
		serverStart: cpuSample{seconds: serverStart, ok: serverStartOK},
		serverEnd:   cpuSample{seconds: serverEnd, ok: serverEndOK},
		driverStart: cpuSample{seconds: driverStart, ok: driverStartOK},
		driverEnd:   cpuSample{seconds: driverEnd, ok: driverEndOK},
		wall:        phaseWall,
		peakRSS:     server.peakRSS(),
		meanRSS:     server.meanRSS(),
		served:      measured.served(),
	})
	notes, err := checkArm(plan, measured)
	measured.Notes = append(measured.Notes, notes...)
	measured.Comparable = len(notes) == 0
	return measured, err
}

// refusalFloor is the fewest refusals an arm with the bound in force may
// record, and it is a quantity rather than a presence.
//
// Half of what the plan's own arithmetic says the noisy population offered
// above the bound, and never less than one so a bound with no bucket behind it
// is still held to having fired. Half rather than all of it because a refusal
// competes with everything else the phase is doing: a request the driver could
// not send and one that timed out are neither served nor refused, and holding
// the control to the full arithmetic would fail an arm for the host being busy.
func refusalFloor(plan fairnessPlan) float64 {
	return max(1, math.Ceil(boundFireShare*plan.refusalsExpected(plan.Noisy)))
}

// boundFireShare is how much of the expected refusal count the positive
// control demands.
const boundFireShare = 0.5

// checkArm applies the two controls an arm has to pass, and returns what it
// failed as notes.
//
// The positive control is an error rather than a note, because a bound that
// never fired and a bound that helped nobody produce identical numbers and
// opposite conclusions. Everything else is a note that costs the arm its
// comparability: the run still writes what it measured, and the verdict
// refuses to compare it.
func checkArm(plan fairnessPlan, arm FairnessArm) ([]string, error) {
	var notes []string
	for _, pop := range arm.Populations {
		for _, method := range pop.Methods {
			if got, want := method.Dispatched, method.Intended-method.Dropped; got != want {
				notes = append(notes, fmt.Sprintf(
					"%s %s: %d dispatched against %d offered less %d dropped; the schedule did not run to its end",
					pop.Name, method.Method, got, method.Intended, method.Dropped,
				))
			}
			if method.Failed > 0 {
				notes = append(notes, fmt.Sprintf("%s %s: %d requests failed for a reason that is not this bound: %s",
					pop.Name, method.Method, method.Failed, method.FirstFailure))
			}
		}
	}
	switch arm.Arm {
	case armOn:
		if got, want := arm.refusals(), refusalFloor(plan); float64(got) < want {
			return notes, fmt.Errorf("%w: %s was supposed to be in force and refused %d requests, against the %.0f "+
				"this control needs, which is %.0f%% of the %.0f the noisy population offered above it. "+
				"A build without the bound, a mistyped switch and two populations sharing one bucket all look like this",
				errBoundDidNotFire, plan.Bound.Label, got, want, boundFireShare*100, plan.refusalsExpected(plan.Noisy))
		}
	default:
		if arm.refusedAnything() {
			notes = append(notes, "the arm with the bound off recorded refusals, so something other than the bound under test refused them")
		}
	}
	return notes, nil
}

// drive runs one window: every credential of both populations follows its own
// schedule, and the whole thing returns when the last request has landed.
func drive(ctx context.Context, plan fairnessPlan, conns []*clientConn, call toolCall,
	ticksFor func(populationSpec) int, tally *fairTally,
) {
	start := time.Now()
	var wg sync.WaitGroup
	for _, pop := range []populationSpec{plan.Quiet, plan.Noisy} {
		first, last := plan.credentials(pop)
		last = min(last, len(conns))
		for index := first; index < last; index++ {
			wg.Add(1)
			go func(conn *clientConn, position int) {
				defer wg.Done()
				paceCredential(ctx, paceInput{
					conn:  conn,
					pop:   pop.Name,
					start: start,
					// Credentials are spread across one period rather than
					// firing together, since a population that fired in step
					// would offer a burst per period and not a rate.
					offset:   time.Duration(float64(pop.period()) * float64(position) / float64(max(1, last-first))),
					period:   pop.period(),
					ticks:    ticksFor(pop),
					verbs:    pop.Verbs,
					call:     call,
					deadline: plan.Deadline,
					inFlight: plan.inFlight(pop),
					tally:    tally,
				})
			}(conns[index], index-first)
		}
	}
	wg.Wait()
}

// paceInput is one credential's schedule.
type paceInput struct {
	conn           *clientConn
	pop            string
	start          time.Time
	offset, period time.Duration
	ticks          int
	verbs          []string
	call           toolCall
	deadline       time.Duration
	inFlight       int
	tally          *fairTally
}

// paceCredential issues one credential's requests at their intended instants.
func paceCredential(ctx context.Context, in paceInput) {
	slots := make(chan struct{}, in.inFlight)
	var wg sync.WaitGroup
	defer wg.Wait()
	for tick := range in.ticks {
		intended := in.start.Add(in.offset + time.Duration(tick)*in.period)
		verb := verbs[in.verbs[tick%len(in.verbs)]]
		if !waitUntil(ctx, intended) {
			return
		}
		in.tally.offered(in.pop, verb)
		select {
		case slots <- struct{}{}:
		default:
			// The ceiling is twice what the schedule can accumulate inside one
			// deadline, so reaching it means the driver fell behind rather than
			// the server being slow. Recorded rather than deferred: a tick that
			// slid to the next free moment would turn this back into a closed
			// loop, and one silently skipped would let an arm offer less than
			// it claimed.
			in.tally.dropped(in.pop, verb)
			continue
		}
		wg.Go(func() {
			defer func() { <-slots }()
			issue(ctx, in, verb, intended)
		})
	}
}

// issue performs one request and files what it did.
func issue(ctx context.Context, in paceInput, verb verbSpec, intended time.Time) {
	sent := time.Now()
	giveUp := intended.Add(in.deadline)
	if !sent.Before(giveUp) {
		// The deadline is anchored at the intended instant, so a driver this
		// far behind would enter the call with a context that has already
		// expired: the transport returns a deadline without a byte reaching
		// the server. Filed as a tick the driver never sent, because the
		// outcome it would otherwise land in is the one that means the tenant
		// gave up on the server, and a harness failure counted there reads as
		// starvation and inflates the very ceiling genuine starvation hides
		// under.
		in.tally.dropped(in.pop, verb)
		return
	}
	// Anchored at the intended instant rather than at the send, so a request
	// held up by the driver is given up on when a client would have given up
	// rather than being granted a fresh deadline the moment it leaves.
	callCtx, cancel := context.WithDeadline(ctx, giveUp)
	_, err := in.conn.rpc.call(callCtx, verb.Method, verb.params(in.call))
	cancel()
	in.tally.record(in.pop, verb, observation{
		latency:  time.Since(intended),
		lateness: sent.Sub(intended),
		err:      err,
	})
}

// waitUntil sleeps to an instant, reporting false when the run was cancelled
// first. An instant already past is not slept through, so a driver that fell
// behind catches up rather than stretching the schedule.
func waitUntil(ctx context.Context, when time.Time) bool {
	delay := time.Until(when)
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// observation is what one request did, measured from the instant it was meant
// to be sent.
type observation struct {
	latency  time.Duration
	lateness time.Duration
	err      error
}

// fairTally collects a window's outcomes, per population and per method.
type fairTally struct {
	mu       sync.Mutex
	call     toolCall
	refusals []refusalSpec
	methods  map[string]map[string]*methodTally
}

// methodTally is one population's record for one method.
//
// Served and refused durations are separate slices under separate names, and
// there is no field holding a duration over all requests. That is the point:
// a merged percentile improves whenever the bound refuses more, so the record
// makes one unconstructible rather than merely discouraged.
type methodTally struct {
	detail       string
	intended     int
	dropped      int
	counts       map[string]int
	served       []time.Duration
	refused      []time.Duration
	lateness     []time.Duration
	firstFailure error
}

// newFairTally builds a tally for one window.
func newFairTally(call toolCall, refusals []refusalSpec) *fairTally {
	return &fairTally{call: call, refusals: refusals, methods: map[string]map[string]*methodTally{}}
}

// entry returns the record for one population and method, creating it on first
// use so a method with no requests never appears at all.
func (t *fairTally) entry(pop string, verb verbSpec) *methodTally {
	byMethod, ok := t.methods[pop]
	if !ok {
		byMethod = map[string]*methodTally{}
		t.methods[pop] = byMethod
	}
	entry, ok := byMethod[verb.Method]
	if !ok {
		entry = &methodTally{detail: verb.detail(t.call), counts: map[string]int{}}
		byMethod[verb.Method] = entry
	}
	return entry
}

// offered records a tick the schedule reached.
func (t *fairTally) offered(pop string, verb verbSpec) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entry(pop, verb).intended++
}

// dropped records a tick the driver did not send: one it could hold no slot
// for, or one it reached so late that the request's own deadline had already
// passed. Both are the harness failing to run the schedule, which is a
// different statement from anything the server did.
func (t *fairTally) dropped(pop string, verb verbSpec) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entry(pop, verb).dropped++
}

// record files one completed request under its outcome.
func (t *fairTally) record(pop string, verb verbSpec, obs observation) {
	kind := classifyOutcome(verb.Method, obs.err, t.refusals)
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entry(pop, verb)
	entry.counts[kind]++
	entry.lateness = append(entry.lateness, obs.lateness)
	switch kind {
	case outcomeServed:
		entry.served = append(entry.served, obs.latency)
	case outcomeRefused:
		entry.refused = append(entry.refused, obs.latency)
	case outcomeFailed:
		if entry.firstFailure == nil {
			entry.firstFailure = obs.err
		}
	}
}

// populations renders the tally as the record's populations, in plan order so
// the quiet one always reads first.
func (t *fairTally) populations(plan fairnessPlan) []FairnessPopulation {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]FairnessPopulation, 0, 2)
	for _, spec := range []populationSpec{plan.Quiet, plan.Noisy} {
		population := FairnessPopulation{
			Name: spec.Name, Credentials: spec.Credentials, RatePerCredential: spec.Rate,
		}
		for _, id := range spec.Verbs {
			verb := verbs[id]
			entry, ok := t.methods[spec.Name][verb.Method]
			if !ok {
				continue
			}
			population.Methods = append(population.Methods, entry.render(verb))
		}
		out = append(out, population)
	}
	return out
}

// render turns one method's record into what the document publishes.
func (m *methodTally) render(verb verbSpec) FairnessMethod {
	out := FairnessMethod{
		Method: verb.Method, Detail: m.detail,
		Intended: m.intended, Dropped: m.dropped,
		Served: m.counts[outcomeServed], Refused: m.counts[outcomeRefused],
		Failed: m.counts[outcomeFailed], TimedOut: m.counts[outcomeTimedOut],
		ServedLatency:  percentiles(verb.Method, m.detail, m.served),
		RefusedLatency: percentiles(verb.Method, m.detail, m.refused),
		Lateness:       percentiles(verb.Method, m.detail, m.lateness),
	}
	out.Dispatched = out.Served + out.Refused + out.Failed + out.TimedOut
	if m.firstFailure != nil {
		out.FirstFailure = m.firstFailure.Error()
	}
	return out
}

// processInput is what an arm's process figures are computed from.
type processInput struct {
	serverStart, serverEnd cpuSample
	driverStart, driverEnd cpuSample
	wall                   time.Duration
	peakRSS, meanRSS       uint64
	served                 int
}

// fairnessProcess computes what the two processes cost over a phase.
//
// Processor time is published twice and never once: as seconds over the phase
// beside how busy that made the host, and as milliseconds per **served**
// request. Never per request. A refusal costs microseconds against tens of
// milliseconds served, so a per-request figure falls by an order of magnitude
// the moment a bound refuses anything, and the report would then present not
// doing the work as doing it cheaply.
//
// There is no per-population processor figure and there will not be one: the
// sampler reads /proc per process and nothing anywhere attributes processor
// time to a credential, so a per-tenant number would be invented. The figures
// below cover both populations together and the record says so.
func fairnessProcess(in processInput) (process FairnessProcess, notes []string) {
	out := FairnessProcess{
		PhaseSeconds: round(in.wall.Seconds()),
		RSSPeakMiB:   mibOf(in.peakRSS),
		RSSMeanMiB:   mibOf(in.meanRSS),
	}
	server, serverNotes := consumed("server", in.serverStart, in.serverEnd)
	notes = append(notes, serverNotes...)
	driver, driverNotes := consumed("driver", in.driverStart, in.driverEnd)
	notes = append(notes, driverNotes...)

	if server.ok && in.wall > 0 {
		out.CPUSeconds = round(server.seconds)
		out.CoresBusy = round(server.seconds / in.wall.Seconds())
		if in.served > 0 {
			out.CPUMsPerServed = round(server.seconds * 1000 / float64(in.served))
		} else {
			notes = append(notes, "processor time per served request unavailable: nothing was served")
		}
	}
	if driver.ok && in.wall > 0 {
		out.DriverCPUSeconds = round(driver.seconds)
		out.DriverCoresBusy = round(driver.seconds / in.wall.Seconds())
	}
	return out, notes
}

// consumed is the processor time between two samples, refusing to publish a
// difference either sample cannot support.
func consumed(who string, start, end cpuSample) (delta cpuSample, notes []string) {
	switch {
	case !start.ok || !end.ok:
		return cpuSample{}, []string{who + " processor time unavailable: the platform did not answer a sample"}
	case end.seconds < start.seconds:
		return cpuSample{}, []string{who + " processor time unavailable: consumed time fell between samples"}
	}
	return cpuSample{seconds: end.seconds - start.seconds, ok: true}, nil
}

// buildServerBinary compiles the binary under measurement. A variable so a
// test can drive the harness's own cleanup without paying a minute of linking
// for a path it is about to delete.
var buildServerBinary = buildServer

// newHarness builds the runner a measuring mode drives, and the cleanup that
// releases everything it started.
func newHarness(opts options, root string) (*runner, func(), error) {
	binary := opts.binary
	built := ""
	if binary == "" {
		out, err := buildServerBinary(root)
		if err != nil {
			return nil, nil, err
		}
		binary, built = out, out
	}
	stub := startStubGitLab()
	sink := startOTLPSink()
	profilesDir := ""
	if opts.profiles != "" {
		profilesDir = resolve(root, opts.profiles)
	}
	r := &runner{
		binary:         binary,
		stub:           stub,
		otlp:           sink,
		sampleInterval: opts.sampleInterval,
		progress:       progressFunc(opts.verbose),
		report:         progressFunc(true),
		profilesDir:    profilesDir,
		budgetMiB:      memoryBudgetMiB(opts.memoryBudget),
	}
	return r, func() {
		stub.close()
		sink.close()
		if built != "" {
			_ = os.RemoveAll(filepath.Dir(built))
		}
	}, nil
}

// medianOf is the middle of a set, averaging the two middle values of an even
// one. Median rather than mean because a run with one disturbed repetition
// should not have that repetition set the answer.
func medianOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	slices.Sort(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// spreadOf is how far apart a set's extremes are, which is the only measure of
// host noise a handful of repetitions offers.
//
// Fewer than two values fix no spread. That is reported as zero rather than as
// an infinity, because the figure is written to a JSON document and neither an
// infinity nor a NaN survives being encoded; the caller has its own explicit
// check for too few repetitions, and reaches its answer there rather than
// through this number.
func spreadOf(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	return slices.Max(values) - slices.Min(values)
}
