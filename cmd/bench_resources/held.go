// held.go measures what a request the server holds open costs it, and what
// the process does once it is asked to hold more than it can: the measurement
// the held-request ceiling (register row HLD-011, issue 951) is sized from.
//
// Every other mode here drives requests the server answers as fast as it can.
// A held request is the other shape: a tools/call waiting on a GitLab that has
// not answered yet. It holds the inbound connection, the goroutines serving it
// and, for as long as GitLab keeps it waiting, an outbound connection to the
// instance, and nothing ends it but the call. The stand-in instance holds
// every project read a step sends it until the step has been sampled, so each
// step is a known number of calls held at once, measured while they are held.
//
// Two figures come out of it, and both are needed to size a ceiling. What one
// held call costs (descriptors, goroutines, live heap and resident set, each
// over the idle process, per call held) says what a ceiling of N commits the
// process to. What the process does beyond it says why there has to be one:
// started under a descriptor limit, a process holding more than the limit
// allows stops accepting connections, /health among them, which is the state
// the ceiling exists to keep a deployment out of.

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// defaultHeldRecord is where a -held run writes its document. It sits under
// bench/, which is not committed, beside the fairness document: it measures a
// bound rather than the published matrix, and is read to size one.
const defaultHeldRecord = "bench/held.json"

// heldSchema versions the -held document.
const heldSchema = 1

// heldCounted is what a -held count counts, as its refusals name it.
const heldCounted = "held-call"

// heldAction is the catalog action every held call runs: a project read,
// which the stand-in instance holds until the step releases it.
const heldAction = "project.get"

// healthUnanswered is what a sample records for a /health probe that got no
// answer at all, as opposed to one that answered with a status.
const healthUnanswered = "unanswered"

// Settling a step. A step is sampled once every call it offered is either
// held at the stand-in or has come back. A call can also be neither: the
// server has not accepted its connection, which is what a process out of
// descriptors does. heldQuiet is how long the counts may stay unchanged before
// the step is sampled as it stands, and heldDeadline bounds the whole wait.
// Variables so a test can settle in milliseconds.
var heldPoll, heldQuiet, heldDeadline, healthTimeout = heldWaits()

// heldWaits are the pauses a real run waits for, in the order the variables
// above take them. healthTimeout bounds the /health probe a sample takes: five
// seconds is far beyond what the handler takes to answer, so a probe that runs
// out of it is a process that did not accept the connection. They are returned
// from a function rather than written into the declaration so a test can hold
// them to their figures before any test replaces them.
func heldWaits() (poll, quiet, deadline, health time.Duration) {
	return 50 * time.Millisecond, 3 * time.Second, 2 * time.Minute, 5 * time.Second
}

// heldNow and heldSleep are the clock a step's wait reads and the pause between
// its reads, so a test can drive the wait on a clock of its own and say exactly
// when it ended.
var (
	heldNow   = time.Now
	heldSleep = time.Sleep
)

// How a step's wait ended.
const (
	settledAll   = "settled"
	settledQuiet = "quiet"
	settledLimit = "deadline"
)

// nofileScript starts the server under a descriptor limit. POSIX ulimit sets
// the soft and the hard limit together when given neither -S nor -H, and the
// hard one is what matters: Go raises its soft limit to the hard limit at
// startup, so lowering only the soft one here would be undone by the server
// before it served anything. The limit comes in as $0 and the server's own
// command line as the rest, so neither is ever interpreted by the shell.
const nofileScript = `ulimit -n "$0" && exec "$@"`

// HeldDoc is the document a -held run writes.
type HeldDoc struct {
	Schema      int        `json:"schema"`
	GeneratedAt string     `json:"generated_at"`
	Host        HostInfo   `json:"host"`
	Server      ServerInfo `json:"server"`
	// Credentials is how many credentials the held calls were spread across,
	// round robin, and NoFile the descriptor limit the server was started
	// under, zero when it inherited this process's.
	Credentials int    `json:"credentials"`
	NoFile      int    `json:"nofile,omitempty"`
	Action      string `json:"action"`
	// Idle is the process with every credential admitted and nothing held,
	// which is what each step's per-call figures are measured over.
	Idle  HeldSample `json:"idle"`
	Steps []HeldStep `json:"steps"`
}

// HeldSample is the process as it stood at one moment.
type HeldSample struct {
	// Descriptors is the process's open file descriptors, -1 when the platform
	// could not say.
	Descriptors int    `json:"descriptors"`
	Goroutines  int    `json:"goroutines"`
	HeapBytes   uint64 `json:"heap_bytes"`
	RSSBytes    uint64 `json:"rss_bytes"`
	// Health is the status /health answered with, or "unanswered".
	Health   string  `json:"health"`
	HealthMs float64 `json:"health_ms"`
	// Notes name every figure that could not be taken, and why.
	Notes []string `json:"notes,omitempty"`
}

// HeldStep is one count of calls offered at once.
type HeldStep struct {
	Offered int `json:"offered"`
	// Held is the calls that reached the stand-in and were held there when
	// the step was sampled.
	Held int `json:"held"`
	// Settled is how the wait before the sample ended: every call held or
	// back, the counts quiet, or the deadline.
	Settled string `json:"settled"`
	// The outcome of every call once released. Refused is a 503, the status
	// the gate answers a ceiling with; the first one's text is kept so a
	// reader can see which refusal it was.
	Served       int        `json:"served"`
	Refused      int        `json:"refused"`
	Failed       int        `json:"failed"`
	FirstRefusal string     `json:"first_refusal,omitempty"`
	FirstFailure string     `json:"first_failure,omitempty"`
	Sample       HeldSample `json:"sample"`
	// PerHeld is what one held call cost over the idle process, absent when
	// nothing was held.
	PerHeld *HeldCost `json:"per_held,omitempty"`
}

// HeldCost is the growth over the idle process divided by the calls held.
type HeldCost struct {
	Descriptors float64 `json:"descriptors"`
	Goroutines  float64 `json:"goroutines"`
	HeapKiB     float64 `json:"heap_kib"`
	RSSKiB      float64 `json:"rss_kib"`
}

// heldInput is what every step of one run is measured against.
type heldInput struct {
	tgt      *httpTarget
	profiler *pprofClient
	conns    []*clientConn
}

// heldTally counts the calls of one step as they come back.
type heldTally struct {
	returned, served, refused, failed atomic.Int64

	mu                         sync.Mutex
	firstRefusal, firstFailure string
}

// record files one call's outcome. A 503 is the refusal the gate answers a
// process ceiling with; anything else that is not a success is a failure.
func (t *heldTally) record(err error) {
	defer t.returned.Add(1)
	if err == nil {
		t.served.Add(1)
		return
	}
	var status *httpStatusError
	if errors.As(err, &status) && status.Status == http.StatusServiceUnavailable {
		t.refused.Add(1)
		t.keep(&t.firstRefusal, status.Snippet)
		return
	}
	t.failed.Add(1)
	t.keep(&t.firstFailure, err.Error())
}

// keep stores the first message of its kind and nothing after it.
func (t *heldTally) keep(slot *string, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if *slot == "" {
		*slot = message
	}
}

// validateHeld refuses a -held run the flags beside it make meaningless, and
// the counts and limits it cannot measure with. Like the fairness check it
// runs whatever else was asked for, since a -held run is a measurement.
func (o options) validateHeld() error {
	if o.held == "" {
		return nil
	}
	if o.fairness != "" {
		return errors.New("-held and -fairness are two measurements with documents of their own: give one of them")
	}
	if o.render || o.check {
		return fmt.Errorf("-held measures one server and draws nothing, so it cannot be combined with %s, "+
			"which draws the committed artifacts and measures nothing", renderFlagName(o))
	}
	if _, err := parseCounts("-held", heldCounted, o.held); err != nil {
		return err
	}
	if o.heldCredentials <= 0 {
		return fmt.Errorf("-held-credentials must be positive, got %d", o.heldCredentials)
	}
	if o.heldNoFile < 0 {
		return fmt.Errorf("-held-nofile must not be negative, got %d", o.heldNoFile)
	}
	if o.heldNoFile > 0 && runtimeGOOS == "windows" {
		return errors.New("-held-nofile starts the server through /bin/sh and ulimit, which Windows has neither of")
	}
	return nil
}

// heldCallParams builds the tools/call a held call sends, fresh each time: the
// request encoder writes the per-request _meta into the map it is handed.
func heldCallParams() map[string]any {
	return map[string]any{
		"name": executeTool,
		"arguments": map[string]any{
			"action": heldAction,
			"params": map[string]any{"project_id": "1"},
		},
	}
}

// runHeld measures the held-request ladder and writes its own document.
//
// Like the fairness mode it returns before the record is read or a chart is
// drawn, so it cannot rewrite anything published.
func runHeld(opts options, root string) error {
	steps, err := parseCounts("-held", heldCounted, opts.held)
	if err != nil {
		return err
	}
	out := resolve(root, opts.heldJSON)
	for _, published := range []string{opts.record, defaultRecord} {
		if out == resolve(root, published) {
			return fmt.Errorf("-held-json names %s, which is the published record: "+
				"give the held run a path of its own", rel(root, out))
		}
	}

	r, cleanup, err := newHarness(opts, root)
	if err != nil {
		return err
	}
	defer cleanup()

	doc := &HeldDoc{
		Schema:      heldSchema,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Host:        hostInfo(),
		Credentials: opts.heldCredentials,
		NoFile:      opts.heldNoFile,
		Action:      heldAction,
	}
	fmt.Printf("held: %d credentials, descriptor limit %s, counts %v\n",
		opts.heldCredentials, nofileLabel(opts.heldNoFile), steps)
	if runErr := r.runHeldLadder(context.Background(), opts, steps, doc); runErr != nil {
		return runErr
	}
	if writeErr := writeJSON(out, doc, "the held record"); writeErr != nil {
		return writeErr
	}
	fmt.Printf("wrote %s\n", rel(root, out))
	return nil
}

// nofileLabel renders the descriptor limit for the progress line.
func nofileLabel(nofile int) string {
	if nofile <= 0 {
		return "inherited"
	}
	return strconv.Itoa(nofile)
}

// runHeldLadder starts the server, admits the credentials, samples the idle
// process and then every step in turn.
func (r *runner) runHeldLadder(ctx context.Context, opts options, steps []int, doc *HeldDoc) error {
	tgt := &httpTarget{
		binary: r.binary, stubURL: r.stub.url, otlpURL: r.otlp.url,
		plan:  scenarioPlan{ID: "held", Transport: transportHTTP, Surface: surfaceDynamic},
		pprof: true, maxClients: opts.heldCredentials, nofile: opts.heldNoFile,
	}
	defer tgt.close()
	if _, err := tgt.start(ctx); err != nil {
		return err
	}
	doc.Server = tgt.serverInfo()

	conns, err := r.admit(ctx, tgt, 0, opts.heldCredentials)
	defer closeConns(conns)
	if err != nil {
		return err
	}
	in := heldInput{tgt: tgt, profiler: newPprofClient(loopbackURL(tgt.pprofAddr, "")), conns: conns}
	doc.Idle = r.sampleHeld(ctx, in)
	fmt.Printf("  idle: %s\n", doc.Idle.summary())
	for _, offered := range steps {
		step := r.runHeldStep(ctx, in, offered)
		step.PerHeld = step.costOver(doc.Idle)
		doc.Steps = append(doc.Steps, step)
		fmt.Printf("  %s\n", step.summary())
	}
	return nil
}

// runHeldStep offers one count of calls at once, samples the process while
// they are held, releases them and files how each one ended.
func (r *runner) runHeldStep(ctx context.Context, in heldInput, offered int) HeldStep {
	release := r.stub.hold()
	var tally heldTally
	var wg sync.WaitGroup
	for i := range offered {
		conn := in.conns[i%len(in.conns)]
		wg.Go(func() {
			callCtx, cancel := context.WithTimeout(ctx, callTimeout)
			defer cancel()
			_, err := conn.rpc.call(callCtx, methodToolsCall, heldCallParams())
			tally.record(err)
		})
	}
	step := HeldStep{Offered: offered}
	step.Settled = awaitHeld(ctx, offered, func() int64 { return r.stub.holding() + tally.returned.Load() })
	step.Held = int(r.stub.holding())
	step.Sample = r.sampleHeld(ctx, in)
	release()
	wg.Wait()
	step.Served = int(tally.served.Load())
	step.Refused = int(tally.refused.Load())
	step.Failed = int(tally.failed.Load())
	step.FirstRefusal, step.FirstFailure = tally.firstRefusal, tally.firstFailure
	// The next step starts from connections of its own: an idle keep-alive
	// connection left from this one would be counted against it.
	for _, conn := range in.conns {
		conn.rpc.close()
	}
	return step
}

// awaitHeld waits until every offered call is accounted for by the counter,
// the counter has stopped moving for heldQuiet, or heldDeadline has passed,
// and says which.
func awaitHeld(ctx context.Context, offered int, accounted func() int64) string {
	deadline := heldNow().Add(heldDeadline)
	last, lastChange := accounted(), heldNow()
	for {
		if last >= int64(offered) {
			return settledAll
		}
		if heldNow().Sub(lastChange) >= heldQuiet {
			return settledQuiet
		}
		if heldNow().After(deadline) || ctx.Err() != nil {
			return settledLimit
		}
		heldSleep(heldPoll)
		if now := accounted(); now != last {
			last, lastChange = now, heldNow()
		}
	}
}

// sampleHeld reads the process as it stands: its descriptors from the
// operating system, its goroutines and live heap off the profiling listener,
// its resident set, and whether /health still answers.
func (r *runner) sampleHeld(ctx context.Context, in heldInput) HeldSample {
	var s HeldSample
	pid := 0
	if procs := in.tgt.processes(); len(procs) > 0 {
		pid = procs[0].Pid
	}
	descriptors, err := countDescriptors(pid)
	if err != nil {
		descriptors = -1
		s.Notes = append(s.Notes, "descriptors: "+err.Error())
	}
	s.Descriptors = descriptors
	if goroutines, gErr := in.profiler.goroutineCount(ctx); gErr != nil {
		s.Notes = append(s.Notes, "goroutines: "+gErr.Error())
	} else {
		s.Goroutines = goroutines
	}
	if heap, hErr := in.profiler.heapAlloc(ctx); hErr != nil {
		s.Notes = append(s.Notes, "heap: "+hErr.Error())
	} else {
		s.HeapBytes = heap
	}
	if stat, sErr := readProcStat(ctx, pid); sErr != nil {
		s.Notes = append(s.Notes, "resident set: "+sErr.Error())
	} else {
		s.RSSBytes = stat.rssBytes
	}
	status, elapsed := probeHealth(ctx, in.tgt.addr)
	s.Health, s.HealthMs = status, round(msOf(elapsed))
	return s
}

// countDescriptors counts a process's open file descriptors.
//
// Linux alone lists them, under /proc/<pid>/fd; elsewhere the figure is
// reported as unknown rather than guessed.
func countDescriptors(pid int) (int, error) {
	if runtimeGOOS != "linux" {
		return 0, fmt.Errorf("descriptors are read from /proc, which %s does not have", runtimeGOOS)
	}
	entries, err := os.ReadDir(fmt.Sprintf("%s/%d/fd", procRoot, pid))
	if err != nil {
		return 0, fmt.Errorf("list descriptors: %w", err)
	}
	return len(entries), nil
}

// loopbackURL is the address of path on the server this benchmark started. That
// server is the process under measurement: it listens on the loopback interface
// and serves plain HTTP, and nothing outside this host ever reaches it.
func loopbackURL(addr, path string) string {
	return (&url.URL{Scheme: "http", Host: addr, Path: path}).String()
}

// probeHealth asks /health once, on a connection of its own, and reports the
// status it answered with and how long that took.
//
// A connection of its own because the question is whether the process still
// accepts one: a probe riding a keep-alive connection opened before the
// descriptors ran out would answer yes for a process that no longer does.
func probeHealth(ctx context.Context, addr string) (string, time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loopbackURL(addr, "/health"), http.NoBody)
	if err != nil {
		return healthUnanswered, time.Since(started)
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	if err != nil {
		return healthUnanswered, time.Since(started)
	}
	_ = resp.Body.Close()
	return strconv.Itoa(resp.StatusCode), time.Since(started)
}

// costOver is what one held call cost over the idle process, or nil when the
// step held nothing to divide by.
func (s HeldStep) costOver(idle HeldSample) *HeldCost {
	return costPer(s.Sample, idle, s.Held)
}

// costPer is a sample's growth over the idle process divided by the count of
// what it holds, or nil when it holds nothing to divide by. The held and the
// session modes price what they hold the same way, so each figure means the
// same thing in both documents.
func costPer(sample, idle HeldSample, count int) *HeldCost {
	if count <= 0 {
		return nil
	}
	held := float64(count)
	cost := &HeldCost{
		Goroutines: round(float64(sample.Goroutines-idle.Goroutines) / held),
		HeapKiB:    round((float64(sample.HeapBytes) - float64(idle.HeapBytes)) / 1024 / held),
		RSSKiB:     round((float64(sample.RSSBytes) - float64(idle.RSSBytes)) / 1024 / held),
	}
	if sample.Descriptors >= 0 && idle.Descriptors >= 0 {
		cost.Descriptors = round(float64(sample.Descriptors-idle.Descriptors) / held)
	}
	return cost
}

// summary renders a sample for the progress line.
func (s HeldSample) summary() string {
	line := fmt.Sprintf("%d descriptors, %d goroutines, %.1f MiB heap, %.1f MiB resident, /health %s in %.0f ms",
		s.Descriptors, s.Goroutines, mibOf(s.HeapBytes), mibOf(s.RSSBytes), s.Health, s.HealthMs)
	if len(s.Notes) > 0 {
		line += " (" + strings.Join(s.Notes, "; ") + ")"
	}
	return line
}

// summary renders a step for the progress line.
func (s HeldStep) summary() string {
	line := fmt.Sprintf("%d offered, %d held (%s): %s; then %d served, %d refused, %d failed",
		s.Offered, s.Held, s.Settled, s.Sample.summary(), s.Served, s.Refused, s.Failed)
	if s.PerHeld != nil {
		line += fmt.Sprintf("; per held call %.2f descriptors, %.2f goroutines, %.1f KiB heap, %.1f KiB resident",
			s.PerHeld.Descriptors, s.PerHeld.Goroutines, s.PerHeld.HeapKiB, s.PerHeld.RSSKiB)
	}
	return line
}
