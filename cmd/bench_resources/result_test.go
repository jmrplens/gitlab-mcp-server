// result_test.go covers the measurement record: the percentile summary the
// published tables are built from, the unit conversions, and the round trip
// through the JSON file every renderer reads.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestPercentiles_SortedSamples_UsesNearestRank verifies that each reported
// percentile is a value that was actually observed, at the nearest rank, and
// that the count and maximum describe the same set. Interpolation would invent
// a number nobody measured, which is the opposite of what this record is for.
func TestPercentiles_SortedSamples_UsesNearestRank(t *testing.T) {
	samples := make([]time.Duration, 0, 100)
	// Deliberately out of order: the summary must sort for itself.
	for i := 100; i >= 1; i-- {
		samples = append(samples, time.Duration(i)*time.Millisecond)
	}

	got := percentiles("tools/list", "whole surface", samples)

	if got.Method != "tools/list" || got.Detail != "whole surface" {
		t.Errorf("method/detail = %q/%q, want tools/list/whole surface", got.Method, got.Detail)
	}
	if got.Count != 100 {
		t.Errorf("Count = %d, want 100", got.Count)
	}
	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		{"p50", got.P50, 50},
		{"p90", got.P90, 90},
		{"p99", got.P99, 99},
		{"max", got.Max, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %v ms, want %v ms", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestPercentiles_EdgeCases_StayInRange verifies the summary handles the two
// sample sets a scenario can legitimately produce without inventing values: no
// samples at all, when every call of a method failed, and a single sample,
// when the matrix was cut down to one round of one client.
func TestPercentiles_EdgeCases_StayInRange(t *testing.T) {
	tests := []struct {
		name    string
		samples []time.Duration
		want    MethodLatency
	}{
		{
			name:    "no samples",
			samples: nil,
			want:    MethodLatency{Method: "ping", Count: 0},
		},
		{
			name:    "one sample",
			samples: []time.Duration{7 * time.Millisecond},
			want:    MethodLatency{Method: "ping", Count: 1, P50: 7, P90: 7, P99: 7, Max: 7},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := percentiles("ping", "", tc.samples)
			if got != tc.want {
				t.Errorf("percentiles = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestMsOf_And_MibOf_RoundToThreeDecimals verifies the two unit conversions
// round to a stable precision. The record is committed, so a figure that
// carried full floating-point noise would churn the file on every run.
func TestMsOf_And_MibOf_RoundToThreeDecimals(t *testing.T) {
	if got := msOf(1500 * time.Microsecond); got != 1.5 {
		t.Errorf("msOf(1.5ms) = %v, want 1.5", got)
	}
	if got := msOf(1234567 * time.Nanosecond); got != 1.235 {
		t.Errorf("msOf(1.234567ms) = %v, want 1.235", got)
	}
	if got := mibOf(1024 * 1024); got != 1 {
		t.Errorf("mibOf(1 MiB) = %v, want 1", got)
	}
	if got := mibOf(1024 * 1024 * 3 / 2); got != 1.5 {
		t.Errorf("mibOf(1.5 MiB) = %v, want 1.5", got)
	}
}

// TestWriteRun_ThenReadRun_RoundTrips verifies a record survives the file it
// is published as, which is the only path the renderers ever take to it.
func TestWriteRun_ThenReadRun_RoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "record.json")
	run := sampleRun()

	if err := writeRun(path, run); err != nil {
		t.Fatalf("writeRun: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading what was written: %v", err)
	}
	if data[len(data)-1] != '\n' {
		t.Error("the record does not end in a newline, which every other committed JSON here does")
	}

	got, err := readRun(path)
	if err != nil {
		t.Fatalf("readRun: %v", err)
	}
	if len(got.Scenarios) != len(run.Scenarios) {
		t.Fatalf("read %d scenarios, wrote %d", len(got.Scenarios), len(run.Scenarios))
	}
	if got.Scenarios[0].Memory.AllClientsMiB != run.Scenarios[0].Memory.AllClientsMiB {
		t.Errorf("resident set did not survive: %v", got.Scenarios[0].Memory)
	}
}

// TestReadRun_AcceptsEverySchemaThisBuildDraws verifies the reader takes
// both the schema-1 record the published figures were measured under, which
// carries no series, and a schema-2 record that carries only a series, so a
// filtered series run has a record of its own to render.
func TestReadRun_AcceptsEverySchemaThisBuildDraws(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "schema 1 without series", content: `{"schema":1,"scenarios":[{"id":"x"}]}`},
		{name: "schema 2 with a series only", content: `{"schema":2,"scenarios":[],"series":[{"id":"http-dynamic-series","steps":[{"clients":1}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "record.json")
			//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("writing the fixture: %v", err)
			}
			if _, err := readRun(path); err != nil {
				t.Errorf("readRun refused a record this build draws: %v", err)
			}
		})
	}
}

// TestWriteRun_ThenReadRun_KeepsTheSeries verifies the series survives the
// file, stop included, since the renderers read every field of it back.
func TestWriteRun_ThenReadRun_KeepsTheSeries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.json")
	run := sampleSeriesRun()
	if err := writeRun(path, run); err != nil {
		t.Fatalf("writeRun: %v", err)
	}
	got, err := readRun(path)
	if err != nil {
		t.Fatalf("readRun: %v", err)
	}
	if got.Schema != resultSchema || len(got.Series) != len(run.Series) {
		t.Fatalf("read schema %d with %d series, wrote schema %d with %d", got.Schema, len(got.Series), run.Schema, len(run.Series))
	}
	if !reflect.DeepEqual(got.Series[2], run.Series[2]) {
		t.Errorf("the stopped series did not survive the round trip:\n got %+v\nwant %+v", got.Series[2], run.Series[2])
	}
	if got.Series[0].Stop != nil || got.Series[0].Skipped != nil {
		t.Errorf("a complete series read back with a stop: %+v", got.Series[0])
	}
}

// TestWriteRun_EncodingFailure_IsReported drives the one failure the
// encoder has through its seam, since a record of plain numbers and strings
// never produces it.
func TestWriteRun_EncodingFailure_IsReported(t *testing.T) {
	previous := marshalRecord
	marshalRecord = func(any, string, string) ([]byte, error) { return nil, errors.New("cannot encode") }
	t.Cleanup(func() { marshalRecord = previous })
	err := writeRun(filepath.Join(t.TempDir(), "record.json"), sampleRun())
	if err == nil || !strings.Contains(err.Error(), "encode results") {
		t.Errorf("writeRun = %v, want the encoding failure", err)
	}
}

// TestReadRun_RefusesUnusableRecords verifies the reader fails loudly on a
// record this build cannot draw, rather than rendering a chart from fields it
// guessed at: a schema from another version, and a run with nothing in it.
func TestReadRun_RefusesUnusableRecords(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "future schema", content: `{"schema":99,"scenarios":[{"id":"x"}]}`},
		{name: "no scenarios", content: `{"schema":1,"scenarios":[]}`},
		{name: "nothing at all under schema 2", content: `{"schema":2,"scenarios":[],"series":[]}`},
		{name: "not json", content: `{`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "record.json")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("writing the fixture: %v", err)
			}
			if _, err := readRun(path); err == nil {
				t.Error("readRun accepted a record it cannot draw")
			}
		})
	}
}

// TestScenarioLatency_FindAndMiss verifies the per-method lookup every
// renderer uses, including a method the scenario never measured.
func TestScenarioLatency_FindAndMiss(t *testing.T) {
	run := sampleRun()
	scenario := run.Scenarios[0]
	if latency, ok := scenario.latency("tools/list"); !ok || latency.P50 == 0 {
		t.Errorf("latency(tools/list) = %+v, ok=%v", latency, ok)
	}
	if _, ok := scenario.latency("resources/read"); ok {
		t.Error("latency found a method that was never measured")
	}
}

// sampleRun is a small but complete record, shaped like a real one: both
// transports, all three surfaces and a credential ramp, so the renderers under
// test have something to draw.
func sampleRun() *Run {
	scenario := func(id, transport, surface string, clients int) Scenario {
		s := Scenario{
			ID: id, Transport: transport, Surface: surface,
			Clients: clients, Parallel: 2, Rounds: 1, ListBytes: 12000,
			Startup: Startup{ProcessReadyMs: 100, FirstListMs: 2500, WarmListMs: 3},
			Memory: Memory{
				IdleMiB: 40, OneClientMiB: 240, AllClientsMiB: 240 + 50*float64(clients-1),
				PeakMiB: 300, PerExtraClientMiB: 50,
			},
			CPU:        CPU{StartupSeconds: 3, LoadSeconds: 1, LoadPercent: 120, TotalSeconds: 4},
			Goroutines: 30,
			Latency: []MethodLatency{
				{Method: "resources/list", Detail: "smallest listing", Count: 4, P50: 2, P90: 3, P99: 4, Max: 4},
				{Method: "tools/call", Detail: "gitlab_find_action", Count: 4, P50: 30, P90: 40, P99: 50, Max: 50},
				{Method: "tools/list", Detail: "whole surface", Count: 4, P50: 5, P90: 6, P99: 7, Max: 7},
			},
		}
		for i := 1; i <= clients; i++ {
			s.Ramp = append(s.Ramp, RampPoint{
				Client: i, ColdListMs: 2500, RSSMiB: 240 + 50*float64(i-1),
			})
		}
		return s
	}
	return &Run{
		Schema:      resultSchema,
		GeneratedAt: "2026-09-04T00:00:00Z",
		Server:      ServerInfo{Version: "2.7.6", Commit: "0123456789abcdef"},
		Host: HostInfo{
			OS: "linux", Arch: "amd64", CPUModel: "Test CPU", CPUs: 8,
			MemTotalGiB: 61, Kernel: "6.1.0", GoVersion: "go1.27.1",
		},
		Settings: Settings{Rounds: 1, SampleIntervalMs: 100},
		Scenarios: []Scenario{
			scenario("stdio-dynamic", transportStdio, surfaceDynamic, 2),
			scenario("stdio-meta", transportStdio, surfaceMeta, 2),
			scenario("stdio-individual", transportStdio, surfaceIndividual, 2),
			scenario("http-dynamic", transportHTTP, surfaceDynamic, 4),
			scenario("http-meta", transportHTTP, surfaceMeta, 4),
			scenario("http-individual", transportHTTP, surfaceIndividual, 4),
		},
	}
}

// sampleSeriesRun is a record with three series in it, shaped like a real
// one: the meta series ran every step, the dynamic one stopped on the
// budget, and the individual one on the latency ceiling. Listed out of
// surface order on purpose, so the ordering is asserted rather than assumed.
func sampleSeriesRun() *Run {
	step := func(clients int, peak, cpu, p50, p99 float64) SeriesStep {
		return SeriesStep{
			Clients: clients, RSSMeanMiB: peak - 20, RSSPeakMiB: peak, CPUMsPerCall: cpu, Calls: 400 * clients,
			CallP50Ms: p50, CallP99Ms: p99, ListP50Ms: p50 / 2, ListP99Ms: p99 / 2, Goroutines: 20 + clients,
			Pool:     PoolCounters{Entries: clients, Capacity: 20},
			Profiles: StepProfiles{CPU: "x/1.cpu.pb.gz", Heap: "x/1.heap.pb.gz"},
		}
	}
	run := sampleRun()
	run.Series = []SeriesScenario{
		{
			ID: "http-meta-series", Transport: transportHTTP, Surface: surfaceMeta, Parallel: 4, StepSeconds: 10,
			Clients: []int{1, 5, 20}, BudgetMiB: 4000, StoppedAt: 20,
			Steps: []SeriesStep{step(1, 200, 1, 10, 20), step(5, 360, 1.1, 11, 25), step(20, 900, 1.4, 15, 40)},
		},
		{
			ID: "http-individual-series", Transport: transportHTTP, Surface: surfaceIndividual, Parallel: 2, StepSeconds: 10,
			Clients: []int{1, 5, 20}, BudgetMiB: 4000, StoppedAt: 5, Skipped: []int{20},
			Stop:       &SeriesStop{Kind: stopLatency, P99Ms: 31000},
			StopReason: "stopped at 5 credentials: tools/call p99 reached 31000 ms, above the 30000 ms ceiling",
			Steps:      []SeriesStep{step(1, 330, 2, 100, 400), step(5, 500, 4, 2000, 31000)},
		},
		{
			ID: "http-dynamic-series", Transport: transportHTTP, Surface: surfaceDynamic, Parallel: 4, StepSeconds: 10,
			Clients: []int{1, 5, 20}, BudgetMiB: 4000, StoppedAt: 5, Skipped: []int{20},
			Stop:       &SeriesStop{Kind: stopBudget, NextClients: 20, EstimateMiB: 4300},
			StopReason: "stopped at 5 credentials: the next step (20) was estimated at 4300 MiB against a budget of 4000 MiB",
			Steps:      []SeriesStep{step(1, 220, 3, 20, 40), step(5, 500, 3.2, 25, 60)},
		},
		// A series that measured nothing is not drawn, whatever its surface.
		{ID: "http-meta-series-empty", Transport: transportHTTP, Surface: surfaceMeta},
	}
	return run
}

// TestWriteRun_ReportsWhereItCouldNotWrite covers the two filesystem
// refusals: a parent that cannot be created because a file is in the way, and
// a path that is a directory.
func TestWriteRun_ReportsWhereItCouldNotWrite(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "file")
	//#nosec G703 -- both halves of the path are this test's own: a t.TempDir and a literal
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write the blocking file: %v", err)
	}
	cases := []struct{ name, path, wantErr string }{
		{name: "parent is a file", path: filepath.Join(blocker, "record.json"), wantErr: "create"},
		{name: "path is a directory", path: root, wantErr: "write"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := writeRun(tc.path, sampleRun())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("writeRun(%s) = %v, want %q", tc.path, err, tc.wantErr)
			}
		})
	}
}
