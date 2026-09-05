// run.go executes one scenario end to end.
//
// The order of the phases is the measurement. A server is started, sampled
// while nothing has asked it for anything, then given one client at a time so
// the cost of admitting the Nth is visible on its own, and only then loaded in
// parallel. Collapsing those phases would produce one resident-set number that
// answers none of the three questions an operator asks: what does it cost
// idle, what does one more client cost, and what does it peak at.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sync"
	"time"
)

// callTimeout bounds one MCP request. A cold tools/list on the individual
// surface builds the whole catalog behind it, so this is generous by design;
// it exists to fail a wedged run rather than to police latency.
const callTimeout = 5 * time.Minute

// settleCeiling bounds the wait for a resident set to stop growing. A
// variable so a test can reach the ceiling without spending three seconds
// on a process that never settles.
var settleCeiling = 3 * time.Second

// runner holds what every scenario needs: the binary under measurement and the
// two stand-in services.
type runner struct {
	binary         string
	stub           *stubGitLab
	otlp           *otlpSink
	sampleInterval time.Duration
	// progress reports per client and per round, only when asked; report
	// prints the lines a run always shows, one per series step, and may be
	// nil in a runner that is not driving a terminal.
	progress func(format string, args ...any)
	report   func(format string, args ...any)

	// profilesDir is where the series writes its profiles, empty to write
	// none; budgetMiB is the resident set the series will not plan a step
	// beyond, zero for no budget.
	profilesDir string
	budgetMiB   float64

	// serverInfo is what the measured build says about itself. Only the HTTP
	// scenarios can ask, since /health is an HTTP endpoint, and the answer is
	// the same binary for every scenario in a run.
	serverInfo ServerInfo
}

// runScenario measures one plan.
func (r *runner) runScenario(ctx context.Context, plan scenarioPlan) (Scenario, error) {
	call, err := callFor(plan.Surface)
	if err != nil {
		return Scenario{}, err
	}

	tgt := r.newTarget(plan)
	defer tgt.close()

	sampler := newSampler(ctx, r.sampleInterval, func() []int { return pids(tgt.processes()) })
	sampler.start()
	defer sampler.stop()

	result := Scenario{
		ID:        plan.ID,
		Transport: plan.Transport,
		Surface:   plan.Surface,
		Telemetry: plan.Telemetry,
		Clients:   plan.Clients,
		Parallel:  plan.Parallel,
		Rounds:    plan.Rounds,
	}

	ready, startErr := tgt.start(ctx)
	if startErr != nil {
		return Scenario{}, startErr
	}
	result.Startup.ProcessReadyMs = msOf(ready)

	// Idle is an HTTP-only figure, and deliberately so. On HTTP the process is
	// up and holds no surface at all until a credential asks for one, which is
	// the empty container an operator sizes first. A stdio process has no such
	// state: it starts building its catalog on a background goroutine the
	// moment it is executed, so any resident set read before its first request
	// is a snapshot of a build in progress. Measuring it anyway produced
	// figures between 77 and 136 MiB for the same binary, which is a
	// measurement of when the sample landed rather than of anything a reader
	// could size for.
	if plan.Transport == transportHTTP {
		result.Memory.IdleMiB = mibOf(settledRSS(sampler))
	}

	conns, rampErr := r.ramp(ctx, tgt, sampler, plan, &result)
	if rampErr != nil {
		return Scenario{}, rampErr
	}

	startupCPU, startupCPUOK := sampleCPU(sampler)

	sampler.resetPeak()
	loadStarted := time.Now()
	notes := r.load(ctx, conns, plan, call, &result)
	loadWall := time.Since(loadStarted)
	result.Notes = append(result.Notes, notes...)

	result.Memory.PeakMiB = mibOf(sampler.peakRSS())
	totalCPU, totalCPUOK := sampleCPU(sampler)
	cpu, cpuNotes := cpuFigures(
		cpuSample{seconds: startupCPU, ok: startupCPUOK},
		cpuSample{seconds: totalCPU, ok: totalCPUOK},
		loadWall,
	)
	result.CPU = cpu
	result.Notes = append(result.Notes, cpuNotes...)

	// The goroutine count is taken last because reading it kills the process:
	// the shipped binary exposes no debug endpoint, so a traceback signal is
	// the only way to ask.
	sampler.stop()
	if count, dumpErr := tgt.goroutines(); dumpErr == nil {
		result.Goroutines = count
	} else {
		result.Notes = append(result.Notes, "goroutine count unavailable: "+dumpErr.Error())
	}

	if info := tgt.serverInfo(); info.Version != "" {
		r.serverInfo = info
	}
	return result, nil
}

// newTarget builds the transport-specific server under measurement.
func (r *runner) newTarget(plan scenarioPlan) target {
	if plan.Transport == transportHTTP {
		return &httpTarget{binary: r.binary, plan: plan, stubURL: r.stub.url, otlpURL: r.otlp.url}
	}
	return &stdioTarget{binary: r.binary, plan: plan, stubURL: r.stub.url, otlpURL: r.otlp.url}
}

// ramp admits one client at a time and records what each one cost.
//
// Serial on purpose: the question is what the Nth credential adds, and clients
// admitted in parallel would each pay a share of the others' registration and
// none of the numbers would separate.
func (r *runner) ramp(ctx context.Context, tgt target, s *sampler, plan scenarioPlan, result *Scenario) ([]*clientConn, error) {
	conns := make([]*clientConn, 0, plan.Clients)
	for i := range plan.Clients {
		conn, spawn, err := tgt.addClient(ctx, i)
		if err != nil {
			return nil, err
		}
		conns = append(conns, conn)

		if i == 0 && plan.Transport == transportStdio {
			result.Startup.ProcessReadyMs = msOf(spawn)
		}

		listCtx, cancel := context.WithTimeout(ctx, callTimeout)
		coldStarted := time.Now()
		list, listErr := conn.rpc.call(listCtx, methodToolsList, nil)
		cold := time.Since(coldStarted)
		cancel()
		if listErr != nil {
			return nil, fmt.Errorf("cold tools/list for %s: %w", conn.label, listErr)
		}

		rss := sampleRSS(s)
		if i == 0 {
			result.Startup.FirstListMs = msOf(cold)
			result.ListBytes = len(list)
			result.Memory.OneClientMiB = mibOf(rss)
		}
		result.Ramp = append(result.Ramp, RampPoint{
			Client:     i + 1,
			ColdListMs: msOf(cold),
			RSSMiB:     mibOf(rss),
		})
		r.progress("    client %d/%d admitted in %.0f ms, resident set %.0f MiB",
			i+1, plan.Clients, msOf(cold), mibOf(rss))
	}

	result.Memory.AllClientsMiB = result.Ramp[len(result.Ramp)-1].RSSMiB
	if plan.Clients > 1 {
		result.Memory.PerExtraClientMiB = round(
			(result.Memory.AllClientsMiB - result.Memory.OneClientMiB) / float64(plan.Clients-1),
		)
	}
	return conns, nil
}

// load runs the measured rounds and fills in the per-method percentiles.
//
// Percentiles are kept per method rather than pooled because the methods are
// not comparable: a ping is a transport round-trip, a tools/list is the whole
// surface serialized, and a tools/call is a handler. One distribution over all
// three would describe none of them.
func (r *runner) load(ctx context.Context, conns []*clientConn, plan scenarioPlan, call toolCall, result *Scenario) []string {
	type methodSpec struct {
		name   string
		detail string
		invoke func(context.Context, *clientConn) error
	}
	methods := []methodSpec{
		{
			// The lightest listing the server has, and the closest thing to a
			// transport round-trip that protocol 2026-07-28 still offers:
			// that revision removed both ping and initialize, so there is no
			// method whose cost is purely the wire.
			name:   methodResourcesList,
			detail: detailSmallestListing,
			invoke: func(ctx context.Context, c *clientConn) error {
				_, err := c.rpc.call(ctx, methodResourcesList, nil)
				return err
			},
		},
		{
			name:   methodToolsCall,
			detail: call.Detail,
			invoke: func(ctx context.Context, c *clientConn) error {
				_, err := c.rpc.call(ctx, methodToolsCall, map[string]any{
					"name":      call.Name,
					"arguments": call.Args,
				})
				return err
			},
		},
		{
			name:   methodToolsList,
			detail: detailWholeSurface,
			invoke: func(ctx context.Context, c *clientConn) error {
				_, err := c.rpc.call(ctx, methodToolsList, nil)
				return err
			},
		},
	}

	var notes []string
	for _, method := range methods {
		var samples []time.Duration
		var failures int
		var firstFailure error
		for round := range plan.Rounds {
			got, failed, failure := fanOut(ctx, conns, plan.Parallel, method.invoke)
			samples = append(samples, got...)
			failures += failed
			if firstFailure == nil {
				firstFailure = failure
			}
			r.progress("    %s round %d/%d: %d samples", method.name, round+1, plan.Rounds, len(got))
		}
		if failures > 0 {
			notes = append(notes, fmt.Sprintf("%d %s calls failed: %v", failures, method.name, firstFailure))
		}
		latency := percentiles(method.name, method.detail, samples)
		result.Latency = append(result.Latency, latency)
		if method.name == methodToolsList {
			result.Startup.WarmListMs = latency.P50
		}
	}
	return notes
}

// fanOut issues clients x parallel requests at once and times each one,
// returning the durations it observed and how many calls failed.
func fanOut(ctx context.Context, conns []*clientConn, parallel int, invoke func(context.Context, *clientConn) error) ([]time.Duration, int, error) {
	var mu sync.Mutex
	var samples []time.Duration
	var firstFailure error
	failures := 0

	var wg sync.WaitGroup
	for _, conn := range conns {
		for range parallel {
			wg.Add(1)
			go func(c *clientConn) {
				defer wg.Done()
				callCtx, cancel := context.WithTimeout(ctx, callTimeout)
				defer cancel()
				started := time.Now()
				err := invoke(callCtx, c)
				elapsed := time.Since(started)

				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failures++
					if firstFailure == nil {
						firstFailure = err
					}
					return
				}
				samples = append(samples, elapsed)
			}(conn)
		}
	}
	wg.Wait()
	return samples, failures, firstFailure
}

// sampleRSS reads the resident set now, reporting zero when the platform will
// not say.
func sampleRSS(s *sampler) uint64 {
	stat, err := s.current()
	if err != nil {
		return 0
	}
	return stat.rssBytes
}

// settledRSS waits for the resident set to stop growing before reading it.
//
// Two consecutive samples within one percent of each other is the condition,
// which a freshly executed process reaches in a few hundred milliseconds and a
// pathological one never does, hence the ceiling.
func settledRSS(s *sampler) uint64 {
	const (
		step      = 50 * time.Millisecond
		tolerance = 0.01
	)
	previous := uint64(0)
	deadline := time.Now().Add(settleCeiling)
	for time.Now().Before(deadline) {
		time.Sleep(step)
		current := sampleRSS(s)
		if previous > 0 && current > 0 &&
			math.Abs(float64(current)-float64(previous)) <= tolerance*float64(previous) {
			return current
		}
		previous = current
	}
	return previous
}

// cpuSample is one reading of consumed processor time, and whether the
// platform gave one.
type cpuSample struct {
	seconds float64
	ok      bool
}

// cpuFigures decides what a scenario can honestly publish about processor
// time, and returns the notes explaining anything it left out.
//
// The load figures are a difference between two samples, so they are only
// meaningful when both were answered and the second is not smaller than the
// first. Publishing the subtraction regardless put negative seconds and
// negative percentages into the committed record and drew them on the charts.
func cpuFigures(startup, total cpuSample, loadWall time.Duration) (cpu CPU, notes []string) {
	if startup.ok {
		cpu.StartupSeconds = round(startup.seconds)
	}
	if total.ok {
		cpu.TotalSeconds = round(total.seconds)
	}
	switch {
	case !startup.ok || !total.ok:
		return cpu, []string{"CPU time unavailable: the platform did not answer a sample"}
	case total.seconds < startup.seconds:
		// Consumed time is monotonic per process, but the sampler sums a set
		// of them, so a client that exits between the two samples takes its
		// time out of the total. Saying so beats publishing the difference.
		return cpu, []string{"CPU load time unavailable: consumed time fell between samples"}
	}
	cpu.LoadSeconds = round(total.seconds - startup.seconds)
	if loadWall > 0 {
		cpu.LoadPercent = round((total.seconds - startup.seconds) / loadWall.Seconds() * 100)
	}
	return cpu, nil
}

// sampleCPU reads consumed processor time now, saying whether the platform
// answered.
//
// The second return is the whole point. Reporting an unanswered sample as zero
// makes it indistinguishable from an idle process, and the caller subtracts one
// sample from another: an unanswered second sample against an answered first
// one yields a negative figure, which was then published as the CPU cost.
func sampleCPU(s *sampler) (seconds float64, ok bool) {
	stat, err := s.current()
	if err != nil {
		return 0, false
	}
	return stat.cpuSeconds, true
}

// pids maps processes to their identifiers for the sampler.
func pids(procs []*os.Process) []int {
	out := make([]int, 0, len(procs))
	for _, proc := range procs {
		if proc != nil {
			out = append(out, proc.Pid)
		}
	}
	return out
}

// decodeHealth reads the build a running server reports about itself. The
// caller owns the body and closes it, because it also retries.
func decodeHealth(resp *http.Response) (ServerInfo, error) {
	if resp.StatusCode != http.StatusOK {
		return ServerInfo{}, fmt.Errorf("health returned %d", resp.StatusCode)
	}
	var body struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ServerInfo{}, fmt.Errorf("decode health: %w", err)
	}
	return ServerInfo{Version: body.Version, Commit: body.Commit}, nil
}
