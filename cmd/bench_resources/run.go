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

// runner holds what every scenario needs: the binary under measurement and the
// two stand-in services.
type runner struct {
	binary         string
	stub           *stubGitLab
	otlp           *otlpSink
	sampleInterval time.Duration
	progress       func(format string, args ...any)

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

	// On HTTP the process is up and holds no surface at all, which is the
	// idle figure an operator sizes the empty container for. On stdio nothing
	// exists yet, so idle is taken after the first handshake instead, below.
	if plan.Transport == transportHTTP {
		result.Memory.IdleMiB = mibOf(settledRSS(sampler))
	}

	conns, rampErr := r.ramp(ctx, tgt, sampler, plan, &result)
	if rampErr != nil {
		return Scenario{}, rampErr
	}

	startupCPU := sampleCPU(sampler)
	result.CPU.StartupSeconds = round(startupCPU)

	sampler.resetPeak()
	loadStarted := time.Now()
	notes := r.load(ctx, conns, plan, call, &result)
	loadWall := time.Since(loadStarted)
	result.Notes = append(result.Notes, notes...)

	result.Memory.PeakMiB = mibOf(sampler.peakRSS())
	totalCPU := sampleCPU(sampler)
	result.CPU.TotalSeconds = round(totalCPU)
	result.CPU.LoadSeconds = round(totalCPU - startupCPU)
	if loadWall > 0 {
		result.CPU.LoadPercent = round((totalCPU - startupCPU) / loadWall.Seconds() * 100)
	}

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
			// The process exists but has been asked for nothing, so the
			// readiness gate has not built a surface for it yet. That is the
			// same "up and empty" moment /health reports on HTTP.
			//
			// Sampled after it settles rather than immediately: exec returns
			// before the runtime has finished starting, and reading there
			// published a one-megabyte idle figure for a process that was
			// about to be forty times that.
			result.Memory.IdleMiB = mibOf(settledRSS(s))
		}

		listCtx, cancel := context.WithTimeout(ctx, callTimeout)
		coldStarted := time.Now()
		list, listErr := conn.rpc.call(listCtx, "tools/list", nil)
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
			name:   "resources/list",
			detail: "smallest listing",
			invoke: func(ctx context.Context, c *clientConn) error {
				_, err := c.rpc.call(ctx, "resources/list", nil)
				return err
			},
		},
		{
			name:   "tools/call",
			detail: call.Detail,
			invoke: func(ctx context.Context, c *clientConn) error {
				_, err := c.rpc.call(ctx, "tools/call", map[string]any{
					"name":      call.Name,
					"arguments": call.Args,
				})
				return err
			},
		},
		{
			name:   "tools/list",
			detail: "whole surface",
			invoke: func(ctx context.Context, c *clientConn) error {
				_, err := c.rpc.call(ctx, "tools/list", nil)
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
		if method.name == "tools/list" {
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
		ceiling   = 3 * time.Second
	)
	previous := uint64(0)
	deadline := time.Now().Add(ceiling)
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

// sampleCPU reads consumed processor time now, reporting zero when the
// platform will not say.
func sampleCPU(s *sampler) float64 {
	stat, err := s.current()
	if err != nil {
		return 0
	}
	return stat.cpuSeconds
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
