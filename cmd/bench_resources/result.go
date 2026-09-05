// result.go defines the measurement record this command writes and every
// generator downstream of it reads.
//
// The record is the published artifact: the charts, the Markdown tables and
// the site page are all rendered from it, and nothing re-measures to draw a
// picture. That is what makes a chart re-renderable on a machine that never
// ran the benchmark, and what makes -check possible at all.

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// resultSchema is the version of the JSON shape below. A reader that finds a
// number it does not know refuses rather than drawing a chart from fields it
// guessed at.
const resultSchema = 1

// Run is one benchmark session: what was measured, on what, and with which
// knobs.
//
// Host and Server are not decoration. A resident-set figure with no machine
// and no build behind it is a number nobody can act on or reproduce, which is
// the failure mode this whole command exists to fix.
type Run struct {
	Schema      int        `json:"schema"`
	GeneratedAt string     `json:"generated_at"`
	Server      ServerInfo `json:"server"`
	Host        HostInfo   `json:"host"`
	Settings    Settings   `json:"settings"`
	Scenarios   []Scenario `json:"scenarios"`
}

// ServerInfo identifies the build that was measured, as the binary itself
// reports it through /health rather than as this command assumes.
type ServerInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// HostInfo is the machine the numbers came from.
type HostInfo struct {
	OS          string  `json:"os"`
	Arch        string  `json:"arch"`
	CPUModel    string  `json:"cpu_model"`
	CPUs        int     `json:"cpus"`
	MemTotalGiB float64 `json:"mem_total_gib"`
	Kernel      string  `json:"kernel"`
	GoVersion   string  `json:"go_version"`
}

// Settings records the knobs the matrix was run with, so a later run can be
// compared against this one on equal terms.
type Settings struct {
	Rounds           int  `json:"rounds"`
	SampleIntervalMs int  `json:"sample_interval_ms"`
	Quick            bool `json:"quick"`
}

// Scenario is one point of the matrix, fully measured.
type Scenario struct {
	ID        string `json:"id"`
	Transport string `json:"transport"`
	Surface   string `json:"surface"`
	Telemetry bool   `json:"telemetry"`
	// Clients is the number of distinct credentials on HTTP and the number of
	// server processes on stdio, because that is what a client is on each
	// transport: HTTP pools one entry per token, stdio spawns one process per
	// client.
	Clients int `json:"clients"`
	// Parallel is how many requests each client keeps in flight at once.
	Parallel int `json:"parallel"`
	Rounds   int `json:"rounds"`
	// ListBytes is the size of one tools/list response body, which is the
	// surface's cost to every client on every reconnect.
	ListBytes  int             `json:"list_bytes"`
	Startup    Startup         `json:"startup"`
	Memory     Memory          `json:"memory"`
	CPU        CPU             `json:"cpu"`
	Goroutines int             `json:"goroutines"`
	Ramp       []RampPoint     `json:"ramp"`
	Latency    []MethodLatency `json:"latency"`
	Notes      []string        `json:"notes,omitempty"`
}

// Startup separates the two waits a client can experience, because they moved
// apart: the process answers in milliseconds while the surface behind it is
// still being built, so reporting one number would hide the one that hurts.
type Startup struct {
	// ProcessReadyMs is spawn to a process that can be addressed: /health
	// answering on HTTP, the exec returning on stdio.
	ProcessReadyMs float64 `json:"process_ready_ms"`
	// FirstListMs is the first tools/list of the first client, which is what
	// pays for registration. Protocol 2026-07-28 has no handshake, so this is
	// the first thing a client waits for.
	FirstListMs float64 `json:"first_list_ms"`
	// WarmListMs is a tools/list once the surface is built, for contrast.
	WarmListMs float64 `json:"warm_list_ms"`
}

// Memory is the resident set at the three moments an operator sizes for.
type Memory struct {
	IdleMiB           float64 `json:"idle_mib"`
	OneClientMiB      float64 `json:"one_client_mib"`
	AllClientsMiB     float64 `json:"all_clients_mib"`
	PeakMiB           float64 `json:"peak_mib"`
	PerExtraClientMiB float64 `json:"per_extra_client_mib"`
}

// CPU is processor time, split into the phase that builds surfaces and the
// phase that serves requests.
type CPU struct {
	StartupSeconds float64 `json:"startup_seconds"`
	LoadSeconds    float64 `json:"load_seconds"`
	// LoadPercent is LoadSeconds over the wall time of the load phase, as a
	// percentage of one core.
	LoadPercent  float64 `json:"load_percent"`
	TotalSeconds float64 `json:"total_seconds"`
}

// RampPoint is the cost of admitting the Nth client: what it waited for and
// what the process weighed once it was in.
type RampPoint struct {
	Client     int     `json:"client"`
	ColdListMs float64 `json:"cold_list_ms"`
	RSSMiB     float64 `json:"rss_mib"`
}

// MethodLatency is one MCP method's distribution under the scenario's load.
//
// Percentiles are per method rather than pooled: a run that averages a 12 KB
// tools/list with a 3 MB one describes neither.
// The descriptions this benchmark writes into Detail for a method that is not
// a named tool. They are named here rather than written at the call site so
// the set is enumerable: the Spanish page translates them, and a description
// added without a translation prints English in a Spanish table.
const (
	detailSmallestListing = "smallest listing"
	detailWholeSurface    = "whole surface"
)

// recordedCallDetails is every description above, for the tests that check a
// page can render all of them.
var recordedCallDetails = []string{detailSmallestListing, detailWholeSurface}

// The MCP methods this benchmark times. Named because three places have to
// agree on them: run.go issues the calls, the record stores the method beside
// each distribution, and figures.go orders the chart's series by them.
const (
	methodResourcesList = "resources/list"
	methodToolsCall     = "tools/call"
	methodToolsList     = "tools/list"
)

// MethodLatency is the latency distribution of one MCP method over a run:
// the percentiles and maximum, in milliseconds, of Count timed calls. Detail
// distinguishes calls to the same method that are not comparable, such as a
// tool call that reaches GitLab from one answered locally.
type MethodLatency struct {
	Method string  `json:"method"`
	Detail string  `json:"detail,omitempty"`
	Count  int     `json:"count"`
	P50    float64 `json:"p50_ms"`
	P90    float64 `json:"p90_ms"`
	P99    float64 `json:"p99_ms"`
	Max    float64 `json:"max_ms"`
}

// latency returns one method's distribution from a scenario.
func (s Scenario) latency(method string) (MethodLatency, bool) {
	for _, m := range s.Latency {
		if m.Method == method {
			return m, true
		}
	}
	return MethodLatency{}, false
}

// percentiles summarizes a set of durations with the nearest-rank method.
//
// Nearest-rank rather than interpolation because these samples are few by
// design: a p99 interpolated between two of forty observations invents a value
// that was never measured, and this record is meant to be re-checkable against
// reality.
func percentiles(method, detail string, samples []time.Duration) MethodLatency {
	out := MethodLatency{Method: method, Detail: detail, Count: len(samples)}
	if len(samples) == 0 {
		return out
	}
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	slices.Sort(sorted)

	at := func(q float64) float64 {
		rank := max(int(math.Ceil(q*float64(len(sorted)))), 1)
		rank = min(rank, len(sorted))
		return msOf(sorted[rank-1])
	}
	out.P50 = at(0.50)
	out.P90 = at(0.90)
	out.P99 = at(0.99)
	out.Max = msOf(sorted[len(sorted)-1])
	return out
}

// msOf converts a duration to milliseconds rounded to three decimals, which is
// finer than the measurement is honest to and coarse enough to keep the JSON
// diffable.
func msOf(d time.Duration) float64 {
	return round(float64(d) / float64(time.Millisecond))
}

// round trims a float to three decimals so a committed record does not churn
// on floating-point noise.
func round(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// mibOf converts bytes to mebibytes, rounded the same way.
func mibOf(bytes uint64) float64 {
	return round(float64(bytes) / (1024 * 1024))
}

// writeRun writes the record as indented JSON with a trailing newline, the
// shape every other committed JSON artifact in this repository has.
func writeRun(path string, run *Run) error {
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode results: %w", err)
	}
	data = append(data, '\n')
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o750); mkErr != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), mkErr)
	}
	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	return nil
}

// readRun loads a committed record, refusing a schema this build cannot draw.
func readRun(path string) (*Run, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the path is this command's own flag
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var run Run
	if unmarshalErr := json.Unmarshal(data, &run); unmarshalErr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, unmarshalErr)
	}
	if run.Schema != resultSchema {
		return nil, fmt.Errorf("%s: schema %d, this build reads %d", path, run.Schema, resultSchema)
	}
	if len(run.Scenarios) == 0 {
		return nil, fmt.Errorf("%s: no scenarios recorded", path)
	}
	return &run, nil
}
