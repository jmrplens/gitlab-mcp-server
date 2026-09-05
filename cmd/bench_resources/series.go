// series.go steps one HTTP process through more and more credentials, and
// profiles it at each step.
//
// The point scenarios say what a handful of credentials cost. A shared
// deployment meets hundreds, and the published per-credential figure,
// multiplied out, puts such a deployment far beyond anything those scenarios
// measured. The series measures it instead of extrapolating: one process,
// credentials admitted in batches, a steady phase of calls at each count, and
// a CPU and a heap profile taken while the phase runs, so the analysis can say
// what a pooled entry is made of and where a call's time goes.
//
// Two guards keep it from taking the host down. Before each step the resident
// set it would reach is estimated from the steps so far, and the rest of the
// list is skipped when the estimate exceeds the memory budget; and a step
// whose tool calls take longer than a client would wait is the last one run.
// Both are recorded, so the page can say where the series stopped and why
// rather than presenting a shorter series as the whole.

package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"
)

const (
	// warmParallel is how many new credentials are admitted at once. Serial
	// admission is what the ramp scenario measures; here the question is the
	// steady state at N rather than the cost of the Nth, and warming a
	// thousand credentials one at a time at two seconds each would spend more
	// of the hour after which the pool rebuilds a credential on the warm-up
	// than on the measurement.
	warmParallel = 8
	// profileFraction is the share of a step's steady phase a CPU profile
	// covers, started with the phase. The remainder keeps the profile inside
	// the phase when the last calls of a slow step run past the deadline.
	profileFraction = 0.8
)

// latencyCeiling ends a series once a step's tools/call tail crosses it.
// Past thirty seconds a client has given up, so the next step would be
// measuring timeouts rather than the server. A variable so a test can lower
// it to something a stand-in crosses.
var latencyCeiling = 30 * time.Second

// runSeries measures one concurrency series.
func (r *runner) runSeries(ctx context.Context, plan scenarioPlan) (SeriesScenario, error) {
	call, err := callFor(plan.Surface)
	if err != nil {
		return SeriesScenario{}, err
	}
	pprofPort, err := freePort(ctx)
	if err != nil {
		return SeriesScenario{}, err
	}
	tgt := &httpTarget{
		binary: r.binary, plan: plan, stubURL: r.stub.url, otlpURL: r.otlp.url,
		pprofAddr: "127.0.0.1:" + strconv.Itoa(pprofPort),
		// Sized to the largest step, so that nothing a step admitted is
		// evicted before the next one measures it.
		maxClients: slices.Max(plan.Steps),
	}
	defer tgt.close()

	s := newSampler(ctx, r.sampleInterval, func() []int { return pids(tgt.processes()) })
	s.start()
	defer s.stop()

	result := SeriesScenario{
		ID: plan.ID, Transport: plan.Transport, Surface: plan.Surface, Parallel: plan.Parallel,
		StepSeconds: round(plan.StepDuration.Seconds()), Clients: plan.Steps, BudgetMiB: r.budgetMiB,
	}
	if r.budgetMiB <= 0 {
		result.Notes = append(result.Notes,
			"no memory budget: the host did not report its available memory, so no step was held back on its account")
	}
	if _, startErr := tgt.start(ctx); startErr != nil {
		return SeriesScenario{}, startErr
	}

	var conns []*clientConn
	defer func() { closeConns(conns) }()
	r.walkSteps(ctx, plan, tgt, newPprofClient("http://"+tgt.pprofAddr), s, call, &result, &conns)

	if len(result.Steps) == 0 {
		return SeriesScenario{}, fmt.Errorf("no step completed: %s", result.StopReason)
	}
	if info := tgt.serverInfo(); info.Version != "" {
		r.serverInfo = info
	}
	return result, nil
}

// walkSteps runs the planned counts in order until one of the guards stops
// the series, admitting credentials into conns as it goes so the caller can
// close them whatever happened.
func (r *runner) walkSteps(ctx context.Context, plan scenarioPlan, tgt target, profiler *pprofClient,
	s *sampler, call toolCall, result *SeriesScenario, conns *[]*clientConn,
) {
	for i, count := range plan.Steps {
		if stop := r.budgetStop(result.Steps, count); stop != nil {
			result.stopBefore(i, stop)
			return
		}
		admitted, admitErr := r.admit(ctx, tgt, len(*conns), count)
		*conns = append(*conns, admitted...)
		if admitErr != nil {
			result.stopBefore(i, &SeriesStop{Kind: stopFailure, NextClients: count, Error: admitErr.Error()})
			return
		}
		step := r.runStep(ctx, stepInput{
			plan: plan, call: call, conns: *conns, sampler: s, profiler: profiler,
			capacity: slices.Max(plan.Steps),
		})
		result.Steps = append(result.Steps, step)
		result.StoppedAt = step.Clients
		r.sayf("      %s", step.summary())
		if step.CallP99Ms > float64(latencyCeiling.Milliseconds()) {
			result.stopAfter(i, &SeriesStop{Kind: stopLatency, P99Ms: step.CallP99Ms})
			return
		}
	}
}

// sayf prints a line whatever the verbosity: a series can run for an hour,
// and one line per step is what makes that hour watchable.
func (r *runner) sayf(format string, args ...any) {
	if r.report != nil {
		r.report(format, args...)
	}
}

// budgetStop decides whether the next step may start: nothing stops it when
// there is no budget or nothing yet to estimate from, and otherwise the
// estimate has to fit.
func (r *runner) budgetStop(steps []SeriesStep, next int) *SeriesStop {
	if r.budgetMiB <= 0 {
		return nil
	}
	estimate, ok := estimateRSS(steps, next)
	if !ok || estimate <= r.budgetMiB {
		return nil
	}
	return &SeriesStop{Kind: stopBudget, NextClients: next, EstimateMiB: round(estimate)}
}

// estimateRSS predicts the peak resident set at the next count from the
// steps so far.
//
// A straight line through their peaks, fitted by least squares, since every
// credential carries its own catalog and the growth is linear by
// construction: MiB per credential times the count, plus the process's own
// share. One step alone fixes no slope, so it is extrapolated in proportion,
// which counts the process's fixed share once per credential and so errs
// toward stopping early rather than late, the right side to err on for a
// guard whose failure mode is a host with no memory left.
func estimateRSS(steps []SeriesStep, next int) (estimate float64, ok bool) {
	switch len(steps) {
	case 0:
		return 0, false
	case 1:
		return steps[0].RSSPeakMiB / float64(steps[0].Clients) * float64(next), true
	}
	var sumX, sumY, sumXY, sumXX float64
	for _, step := range steps {
		x, y := float64(step.Clients), step.RSSPeakMiB
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	n := float64(len(steps))
	// The counts ascend, so at least two differ and the denominator is not
	// zero.
	slope := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	intercept := (sumY - slope*sumX) / n
	return slope*float64(next) + intercept, true
}

// admit connects credentials have through want-1, each warmed with one cold
// tools/list so the pool holds a built entry for it before the step is
// measured, at most warmParallel at a time.
//
// Every connection made is returned, whether or not its warm-up succeeded,
// so the caller can close them all; the first failure is returned beside
// them, and the step is not run on a pool that is only partly built.
func (r *runner) admit(ctx context.Context, tgt target, have, want int) ([]*clientConn, error) {
	type outcome struct {
		conn *clientConn
		err  error
	}
	outcomes := make([]outcome, want-have)
	slots := make(chan struct{}, warmParallel)
	var wg sync.WaitGroup
	for index := have; index < want; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			conn, _, err := tgt.addClient(ctx, index)
			if err == nil {
				listCtx, cancel := context.WithTimeout(ctx, callTimeout)
				_, err = conn.rpc.call(listCtx, methodToolsList, nil)
				cancel()
				if err != nil {
					err = fmt.Errorf("cold tools/list for %s: %w", conn.label, err)
				}
			}
			outcomes[index-have] = outcome{conn: conn, err: err}
		}(index)
	}
	wg.Wait()

	conns := make([]*clientConn, 0, len(outcomes))
	var firstErr error
	for _, o := range outcomes {
		if o.conn != nil {
			conns = append(conns, o.conn)
		}
		if o.err != nil && firstErr == nil {
			firstErr = o.err
		}
	}
	r.progress("    %d credentials admitted", want)
	return conns, firstErr
}

// stepInput is what one step measures with.
type stepInput struct {
	plan     scenarioPlan
	call     toolCall
	conns    []*clientConn
	sampler  *sampler
	profiler *pprofClient
	capacity int
}

// runStep measures one credential count: a steady phase in which every
// credential alternates tools/call and tools/list at the plan's parallelism,
// with a CPU profile running over most of it, and a heap profile and the
// goroutine count taken as it ends.
func (r *runner) runStep(ctx context.Context, in stepInput) SeriesStep {
	step := SeriesStep{
		Clients: len(in.conns),
		Pool:    PoolCounters{Entries: len(in.conns), Capacity: in.capacity},
	}

	in.sampler.resetPeak()
	startCPU, startOK := sampleCPU(in.sampler)

	// Profiles are only collected when there is somewhere to write them: a
	// CPU profile holds the listener for most of the phase, which is work the
	// server would not otherwise do, so a run that keeps no profiles is not
	// made to pay for them.
	cpuProfile := make(chan captured, 1)
	if r.profilesDir != "" {
		seconds := max(1, int(in.plan.StepDuration.Seconds()*profileFraction))
		go func() { cpuProfile <- in.profiler.cpuProfile(ctx, seconds) }()
	}

	load := steadyLoad(ctx, in.conns, in.plan.Parallel, in.plan.StepDuration, in.call)

	endCPU, endOK := sampleCPU(in.sampler)
	step.RSSPeakMiB = mibOf(in.sampler.peakRSS())
	step.RSSMeanMiB = mibOf(in.sampler.meanRSS())

	calls := percentiles(methodToolsCall, in.call.Detail, load.samples[methodToolsCall])
	lists := percentiles(methodToolsList, detailWholeSurface, load.samples[methodToolsList])
	step.Calls = calls.Count + lists.Count
	step.CallP50Ms, step.CallP99Ms = calls.P50, calls.P99
	step.ListP50Ms, step.ListP99Ms = lists.P50, lists.P99
	step.Notes = append(step.Notes, load.notes...)

	perCall, cpuNotes := cpuPerCall(cpuSample{seconds: startCPU, ok: startOK}, cpuSample{seconds: endCPU, ok: endOK}, step.Calls)
	step.CPUMsPerCall = perCall
	step.Notes = append(step.Notes, cpuNotes...)

	if count, err := in.profiler.goroutineCount(ctx); err == nil {
		step.Goroutines = count
	} else {
		step.Notes = append(step.Notes, "goroutine count unavailable: "+err.Error())
	}
	if r.profilesDir != "" {
		var profileNotes []string
		step.Profiles, profileNotes = writeProfiles(r.profilesDir, in.plan.ID, step.Clients,
			<-cpuProfile, in.profiler.heapProfile(ctx))
		step.Notes = append(step.Notes, profileNotes...)
	}
	return step
}

// loadOutcome is what a steady phase observed: the durations of every call
// that completed, per method, and a note per method whose calls failed.
type loadOutcome struct {
	samples map[string][]time.Duration
	notes   []string
}

// steadyLoad keeps clients x parallel workers calling for the duration, each
// alternating tools/call and tools/list, and times every call.
//
// A fixed duration rather than a fixed count, for two reasons. A CPU profile
// is taken over wall-clock seconds, so the phase has to be one; and a count
// that took seconds at one credential would take unbounded time once
// latency degrades, which is exactly the region the series exists to reach.
// The per-call figures divide by the calls that were actually made.
func steadyLoad(ctx context.Context, conns []*clientConn, parallel int, duration time.Duration, call toolCall) loadOutcome {
	deadline := time.Now().Add(duration)
	tally := &loadTally{samples: map[string][]time.Duration{}, failures: map[string]int{}, firstFailure: map[string]error{}}

	var wg sync.WaitGroup
	for _, conn := range conns {
		for range parallel {
			wg.Add(1)
			go func(c *clientConn) {
				defer wg.Done()
				steadyWorker(ctx, c, deadline, call, tally)
			}(conn)
		}
	}
	wg.Wait()
	return tally.outcome()
}

// loadTally collects what the workers of a steady phase observed.
type loadTally struct {
	mu           sync.Mutex
	samples      map[string][]time.Duration
	failures     map[string]int
	firstFailure map[string]error
}

// record files one call's outcome under its method.
func (t *loadTally) record(method string, elapsed time.Duration, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err != nil {
		t.failures[method]++
		if t.firstFailure[method] == nil {
			t.firstFailure[method] = err
		}
		return
	}
	t.samples[method] = append(t.samples[method], elapsed)
}

// outcome is the phase's durations and a note per method whose calls failed.
func (t *loadTally) outcome() loadOutcome {
	out := loadOutcome{samples: t.samples}
	for _, method := range []string{methodToolsCall, methodToolsList} {
		if t.failures[method] > 0 {
			out.notes = append(out.notes, fmt.Sprintf("%d %s calls failed: %v", t.failures[method], method, t.firstFailure[method]))
		}
	}
	return out
}

// steadyWorker keeps one connection calling until the deadline, a tools/call
// then a tools/list, timing each.
func steadyWorker(ctx context.Context, c *clientConn, deadline time.Time, call toolCall, tally *loadTally) {
	for turn := 0; time.Now().Before(deadline) && ctx.Err() == nil; turn++ {
		method := methodToolsList
		var params map[string]any
		if turn%2 == 0 {
			method = methodToolsCall
			params = map[string]any{"name": call.Name, "arguments": call.Args}
		}
		callCtx, cancel := context.WithTimeout(ctx, callTimeout)
		started := time.Now()
		_, err := c.rpc.call(callCtx, method, params)
		elapsed := time.Since(started)
		cancel()
		tally.record(method, elapsed, err)
	}
}

// cpuPerCall divides the processor time consumed over a phase by the calls
// completed in it, in milliseconds, with the same honesty rule cpuFigures
// applies: an unanswered or backwards sample publishes nothing, with a note.
func cpuPerCall(start, end cpuSample, calls int) (msPerCall float64, notes []string) {
	switch {
	case !start.ok || !end.ok:
		return 0, []string{"CPU per call unavailable: the platform did not answer a sample"}
	case end.seconds < start.seconds:
		return 0, []string{"CPU per call unavailable: consumed time fell between samples"}
	case calls == 0:
		return 0, []string{"CPU per call unavailable: no call completed"}
	}
	return round((end.seconds - start.seconds) * 1000 / float64(calls)), nil
}

// closeConns releases every client of a series.
func closeConns(conns []*clientConn) {
	for _, conn := range conns {
		conn.rpc.close()
	}
}

// stopBefore records that the step at index i was not started, and nothing
// after it either.
func (s *SeriesScenario) stopBefore(i int, stop *SeriesStop) {
	s.Skipped = slices.Clone(s.Clients[i:])
	s.Stop = stop
	s.StopReason = stop.sentence(s.StoppedAt, s.BudgetMiB)
}

// stopAfter records that the step at index i was the last one run.
func (s *SeriesScenario) stopAfter(i int, stop *SeriesStop) {
	s.Skipped = slices.Clone(s.Clients[i+1:])
	s.Stop = stop
	s.StopReason = stop.sentence(s.StoppedAt, s.BudgetMiB)
}

// sentence is the English form of a stop, for the record and the terminal;
// the pages render the same fact in their own language.
func (stop *SeriesStop) sentence(stoppedAt int, budgetMiB float64) string {
	switch stop.Kind {
	case stopBudget:
		return fmt.Sprintf("stopped at %d credentials: the next step (%d) was estimated at %.0f MiB against a budget of %.0f MiB",
			stoppedAt, stop.NextClients, stop.EstimateMiB, budgetMiB)
	case stopLatency:
		return fmt.Sprintf("stopped at %d credentials: tools/call p99 reached %.0f ms, above the %d ms ceiling",
			stoppedAt, stop.P99Ms, latencyCeiling.Milliseconds())
	default:
		return fmt.Sprintf("stopped at %d credentials: admitting credentials for the next step (%d) failed: %s",
			stoppedAt, stop.NextClients, stop.Error)
	}
}

// summary is the one line a step prints as it completes.
func (step SeriesStep) summary() string {
	return fmt.Sprintf("%d credentials: rss %.0f MiB mean, %.0f MiB peak; cpu %.3f ms/call over %d calls; tools/call p50 %s ms p99 %s ms; tools/list p50 %s ms p99 %s ms; %d goroutines",
		step.Clients, step.RSSMeanMiB, step.RSSPeakMiB, step.CPUMsPerCall, step.Calls,
		msLabel(step.CallP50Ms), msLabel(step.CallP99Ms), msLabel(step.ListP50Ms), msLabel(step.ListP99Ms), step.Goroutines)
}

// summary is the line a series prints when it ends.
func (s *SeriesScenario) summary(elapsed time.Duration) string {
	if s.StopReason != "" {
		return fmt.Sprintf("%d steps, %s, %s", len(s.Steps), s.StopReason, elapsed.Round(time.Second))
	}
	return fmt.Sprintf("%d steps to %d credentials, %s", len(s.Steps), s.StoppedAt, elapsed.Round(time.Second))
}
