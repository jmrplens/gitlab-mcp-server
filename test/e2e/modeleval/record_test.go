//go:build e2e

// record_test.go drives the shard writer's two lifetimes without a run.
//
// The split is what is under test: an attempt's own lines are written when
// that attempt ends, so a run that dies in the middle leaves behind everything
// it paid for, and the run and session lines are written afterwards, because
// neither is complete until the last attempt has been made. A reader that
// found a shard of attempts and no run line could publish none of them: the
// commit, the instance and the tier are all on that line.

package modeleval

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// TestRecorder_WritesAnAttemptWhenItEndsAndTheRunAfterwards checks both
// lifetimes and the order a reader gets them in.
func TestRecorder_WritesAnAttemptWhenItEndsAndTheRunAfterwards(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(modelrecord.Release)

	rec := &recorder{
		writer:   modelrecord.OpenDir(dir),
		sessions: map[string]*modelrecord.Session{},
	}
	rec.run = modelrecord.Run{Package: "modeleval", RunID: "run-1", Tier: "free", Repeat: 1}
	rec.sessions["dynamic-default-full"] = &modelrecord.Session{
		Label:             "dynamic-default-full",
		Surface:           "dynamic",
		ToolSchemaDigests: map[string]string{},
	}
	rec.order = append(rec.order, "dynamic-default-full")
	rec.noteToolDigest("dynamic-default-full", "fake:perfect", "0123456789abcdef")

	rec.writeAttempt(t,
		&modelrecord.Attempt{
			ID: "a1", Case: "MT-001", Model: "fake:perfect", Surface: "dynamic",
			Session: "dynamic-default-full", Repeat: 1, EndedBy: modelrecord.EndedCompleted,
		},
		[]*modelrecord.Turn{{Attempt: "a1", Index: 1, Try: 1, Status: modelrecord.TurnOK}},
		[]*modelrecord.Call{{Attempt: "a1", Turn: 1, Index: 1, Tool: "gitlab_find_action", Outcome: modelrecord.OutcomeOK}},
		[]*modelrecord.Verify{{Attempt: "a1", Name: "issue", Passed: true}},
	)

	// Before the hook: the attempt and its lines are on disk and the run is
	// not, which is the state a run killed halfway leaves behind.
	written := readOneShard(t, dir)
	if counted := countTypes(written); counted[modelrecord.TypeAttempt] != 1 || counted[modelrecord.TypeRun] != 0 {
		t.Errorf("before the exit hook the shard holds %v, want the attempt and no run line", counted)
	}

	if err := rec.flush(); err != nil {
		t.Fatalf("flush error = %v, want nil", err)
	}

	written = readOneShard(t, dir)
	counted := countTypes(written)
	for _, want := range []struct {
		kind  string
		count int
	}{
		{modelrecord.TypeRun, 1},
		{modelrecord.TypeSession, 1},
		{modelrecord.TypeAttempt, 1},
		{modelrecord.TypeTurn, 1},
		{modelrecord.TypeCall, 1},
		{modelrecord.TypeVerify, 1},
	} {
		t.Run(want.kind, func(t *testing.T) {
			if counted[want.kind] != want.count {
				t.Errorf("the shard holds %d %s line(s), want %d", counted[want.kind], want.kind, want.count)
			}
		})
	}
	for _, record := range written {
		if record.Session != nil && record.Session.ToolSchemaDigests["fake:perfect"] != "0123456789abcdef" {
			t.Errorf("the session line carries digests %v, want the one the provider was given",
				record.Session.ToolSchemaDigests)
		}
	}
}

// TestRecorder_RecordingOff_WritesNothingAndFailsNothing checks the state a
// maintainer driving one case by hand is in: no directory, no shard, no
// complaint.
func TestRecorder_RecordingOff_WritesNothingAndFailsNothing(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{writer: modelrecord.OpenDir(""), sessions: map[string]*modelrecord.Session{}}

	rec.writeAttempt(t, &modelrecord.Attempt{ID: "a1", EndedBy: modelrecord.EndedCompleted}, nil, nil, nil)
	if err := rec.flush(); err != nil {
		t.Errorf("flush with recording off error = %v, want nil", err)
	}

	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil || len(entries) != 0 {
		t.Errorf("recording off left %v behind", entries)
	}
}

// TestRecorder_ALineTheShardCannotHold_FailsTheRunAtTheHook checks that the
// exit hook does not swallow what it could not write.
//
// There is no test to report to at that moment, so the hook's error is the
// only place it can be said, and a run whose run line was lost silently would
// be a run of attempts nobody can publish.
func TestRecorder_ALineTheShardCannotHold_FailsTheRunAtTheHook(t *testing.T) {
	rec := &recorder{
		writer:   modelrecord.OpenDir("a-relative-directory"),
		sessions: map[string]*modelrecord.Session{},
	}
	err := rec.flush()
	if err == nil {
		t.Fatalf("flush into a relative directory returned no error")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("flush error = %v, want it to say why the shard could not be opened", err)
	}
}

// TestExitReporter_CollectsEveryProblem checks the reporter the hook writes
// through.
func TestExitReporter_CollectsEveryProblem(t *testing.T) {
	reporter := &exitReporter{}
	reporter.Errorf("first: %d", 1)
	reporter.Errorf("second: %s", "two")
	if len(reporter.problems) != 2 || reporter.problems[0] != "first: 1" || reporter.problems[1] != "second: two" {
		t.Errorf("the reporter collected %v", reporter.problems)
	}
}

// TestProviderLines_SpellTheOptionsAsTheyWentOnTheWire checks the difference a
// published row rests on: a temperature that was omitted is not a temperature
// of zero.
func TestProviderLines_SpellTheOptionsAsTheyWentOnTheWire(t *testing.T) {
	spec, err := provider.ParseSpec("anthropic:claude-haiku-4-5-20251001;temperature=default")
	if err != nil {
		t.Fatalf("ParseSpec error = %v", err)
	}
	price := modelrecord.Price{InputPerMillionUSD: 1, OutputPerMillionUSD: 5, RetrievedOn: "2026-09-16"}
	lines := providerLines([]provider.Spec{spec}, map[string]*modelrecord.Price{spec.String(): &price})

	if len(lines) != 1 {
		t.Fatalf("providerLines returned %d line(s), want 1", len(lines))
	}
	if lines[0].Name != provider.Anthropic || lines[0].Model != "claude-haiku-4-5-20251001" {
		t.Errorf("the line names %s:%s", lines[0].Name, lines[0].Model)
	}
	if lines[0].Options["temperature"] != "provider default" {
		t.Errorf("temperature reads %q, want the words a reader can tell from zero",
			lines[0].Options["temperature"])
	}
	if lines[0].Price == nil || lines[0].Price.InputPerMillionUSD != 1 {
		t.Errorf("the line carries price %+v", lines[0].Price)
	}

	unpriced := providerLines([]provider.Spec{spec}, nil)
	if unpriced[0].Price != nil {
		t.Errorf("a model with no price carries %+v", unpriced[0].Price)
	}
}

// TestEditionName_IsTheVersionEndpointsFlagAndNotTheTier checks the one word
// that is easy to write as the other.
func TestEditionName_IsTheVersionEndpointsFlagAndNotTheTier(t *testing.T) {
	if got := editionName(true); got != "enterprise" {
		t.Errorf("editionName(true) = %q", got)
	}
	if got := editionName(false); got != "community" {
		t.Errorf("editionName(false) = %q", got)
	}
}

// TestTestFilter_ReadsTheFlagTheBinaryWasGiven checks that the run line's
// filter comes from the flag rather than from anything that has to remember to
// set it: a partial run is not a claim about the rest, and the publisher
// refuses a record carrying one.
func TestTestFilter_ReadsTheFlagTheBinaryWasGiven(t *testing.T) {
	want := ""
	if flagged := flag.Lookup("test.run"); flagged != nil {
		want = flagged.Value.String()
	}
	if got := testFilter(); got != want {
		t.Errorf("testFilter() = %q, want the -run expression %q", got, want)
	}
}

// readOneShard reads the single shard a directory holds.
func readOneShard(t *testing.T, dir string) []modelrecord.Record {
	t.Helper()
	shards, err := modelrecord.ReadShards(dir)
	if err != nil {
		t.Fatalf("reading the shards under %s: %v", dir, err)
	}
	if len(shards) != 1 {
		t.Fatalf("the directory holds %d shard(s), want one per process", len(shards))
	}
	return shards[0].Records
}

// countTypes counts the lines of each type a shard holds.
func countTypes(records []modelrecord.Record) map[string]int {
	counted := map[string]int{}
	for _, record := range records {
		counted[record.Type]++
	}
	return counted
}
