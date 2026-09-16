package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelscore"
)

// The fixture constants every builder below shares. They are spelled once so a
// test asserting on a key is asserting on the same strings the builder wrote.
const (
	fixtureCommit   = "1111111111111111111111111111111111111111"
	fixtureContract = "contract-1"
	fixtureTools    = "tools-1"
	fixtureModel    = "anthropic:test-model"
	fixtureSession  = "dynamic-default"
	fixtureCase     = "MT-002"
	fixtureProject  = "eval-group/eval-project"
)

// publishableShard is a shard every refusal rule lets through: one dynamic
// session, one attempt of a case the corpus really has, a dispatch the server's
// span described, and a run put the corpus this tree holds.
//
// Every refusal test below starts from it and breaks exactly one thing, so a
// test that fails is telling you about its own rule rather than about a fixture
// that was never publishable in the first place.
func publishableShard() []modelrecord.Record {
	return []modelrecord.Record{
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeRun, Run: fixtureRun()},
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeAttempt, Attempt: fixtureAttempt()},
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeTurn, Turn: fixtureTurn()},
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeCall, Call: fixtureCall()},
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeSession, Session: fixtureSessionLine()},
	}
}

// fixtureRun is the run line of a publishable shard.
func fixtureRun() *modelrecord.Run {
	return &modelrecord.Run{
		Package:        "test/e2e/modeleval",
		RunID:          "run-fixture",
		Commit:         fixtureCommit,
		StartedAt:      time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC),
		Edition:        "enterprise",
		GitLabVersion:  "19.3.0",
		Tier:           "ultimate",
		TierConfirmed:  true,
		CorpusDigest:   modelcorpus.Digest(),
		ContractDigest: fixtureContract,
		Repeat:         1,
		Providers: []modelrecord.Provider{{
			Spec:    fixtureModel,
			Name:    "anthropic",
			Model:   "test-model",
			Options: map[string]string{"max_tokens": "4096", "temperature": "0"},
		}},
	}
}

// fixtureSessionLine is the session line of a publishable shard.
func fixtureSessionLine() *modelrecord.Session {
	return &modelrecord.Session{
		Label:             fixtureSession,
		Surface:           "dynamic",
		Mode:              "default",
		Capabilities:      "full",
		TokenScopes:       []string{"api"},
		ServedTools:       2,
		ToolSchemaDigests: map[string]string{fixtureModel: fixtureTools},
	}
}

// fixtureAttempt is the attempt line of a publishable shard.
func fixtureAttempt() *modelrecord.Attempt {
	return &modelrecord.Attempt{
		ID:      fixtureCase + "/dynamic/1",
		Case:    fixtureCase,
		Model:   fixtureModel,
		Surface: "dynamic",
		Session: fixtureSession,
		Repeat:  1,
		Facts: map[string]string{
			"project_path":   fixtureProject,
			"project_id":     "42",
			"default_branch": "main",
		},
		Stimulus: "Find project `" + fixtureProject + "` and give me its ID and default branch.",
		EndedBy:  modelrecord.EndedCompleted,
	}
}

// fixtureTurn is the one provider turn of a publishable shard.
func fixtureTurn() *modelrecord.Turn {
	return &modelrecord.Turn{
		Attempt: fixtureCase + "/dynamic/1",
		Index:   1,
		Try:     1,
		Blocks: []modelrecord.Block{{
			Kind:      modelrecord.BlockToolCall,
			Tool:      "gitlab_execute_action",
			Arguments: json.RawMessage(`{"action":"project.get","params":{"project_id":"` + fixtureProject + `"}}`),
			CallID:    "call_1",
		}},
		Usage:  modelrecord.Usage{Input: 1180, Output: 52, CacheCreated: 900, CacheRead: 1100},
		Status: modelrecord.TurnOK,
	}
}

// fixtureCall is the one tools/call of a publishable shard, dispatched as the
// case's key declares and seen by the server's own span.
func fixtureCall() *modelrecord.Call {
	return &modelrecord.Call{
		Attempt:          fixtureCase + "/dynamic/1",
		Turn:             1,
		Index:            1,
		Tool:             "gitlab_execute_action",
		Arguments:        json.RawMessage(`{"action":"project.get","params":{"project_id":"` + fixtureProject + `"}}`),
		RequestedAction:  "project.get",
		DispatchedAction: "project.get",
		DispatchedTool:   "gitlab_execute_action",
		Outcome:          modelrecord.OutcomeOK,
		Result:           json.RawMessage(`{"project":{"id":42,"path_with_namespace":"` + fixtureProject + `"}}`),
		Text:             "## " + fixtureProject,
		TraceID:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1",
		Requests:         1,
		DispatchObserved: true,
	}
}

// writeShard writes one shard into a directory of its own and returns it.
func writeShard(t *testing.T, records []modelrecord.Record) string {
	t.Helper()
	dir := t.TempDir()
	writeShardInto(t, dir, "modeleval-fixture.jsonl", records)
	return dir
}

// writeShardInto writes one shard under a name of the caller's choosing, so a
// test can put two of them in one directory.
func writeShardInto(t *testing.T, dir, name string, records []modelrecord.Record) {
	t.Helper()
	var body []byte
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal a %s line: %v", record.Type, err)
		}
		body = append(body, line...)
		body = append(body, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
		t.Fatalf("write the shard %s: %v", name, err)
	}
}

// foldShard reads one hand-written shard the way the command does.
func foldShard(t *testing.T, records []modelrecord.Record, claimed map[string]string) []candidate {
	t.Helper()
	if claimed == nil {
		claimed = map[string]string{}
	}
	shards, err := modelrecord.ReadShards(writeShard(t, records))
	if err != nil {
		t.Fatalf("read the shard back: %v", err)
	}
	candidates, err := fold(shards, claimed)
	if err != nil {
		t.Fatalf("fold the shard: %v", err)
	}
	return candidates
}

// publishOne folds one shard and returns the single row it publishes, failing
// when anything refused it.
func publishOne(t *testing.T, records []modelrecord.Record) row {
	t.Helper()
	rows, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys())
	if err != nil {
		t.Fatalf("judge the shard: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("the fixture was refused: %v", refusals)
	}
	if len(rows) != 1 {
		t.Fatalf("published %d rows, want exactly one", len(rows))
	}
	return rows[0]
}

// TestFold_OneShard_KeysTheRowFromTheRunSessionAndModel checks the join every
// published figure hangs off: a row's identity is read from three lines, and
// reading any of them from the wrong place would put two measurements in one
// row without anything saying so.
func TestFold_OneShard_KeysTheRowFromTheRunSessionAndModel(t *testing.T) {
	candidates := foldShard(t, publishableShard(), nil)
	if len(candidates) != 1 {
		t.Fatalf("folded %d candidates, want one", len(candidates))
	}
	got := candidates[0].key
	want := rowKey{
		Model:            fixtureModel,
		Surface:          "dynamic",
		Mode:             "default",
		Tier:             "ultimate",
		CorpusDigest:     modelcorpus.Digest(),
		ContractDigest:   fixtureContract,
		ToolSchemaDigest: fixtureTools,
		Repeat:           1,
	}
	if got != want {
		t.Errorf("row key\n got %+v\nwant %+v", got, want)
	}
	if !candidates[0].providerKnown {
		t.Error("the run line describes the model, so the candidate should know its provider")
	}
	if candidates[0].sessionCalls != 1 || candidates[0].sessionObserved != 1 {
		t.Errorf("session observation = %d of %d call(s), want 1 of 1",
			candidates[0].sessionObserved, candidates[0].sessionCalls)
	}
}

// TestFold_TwoModelsOneSession_AreTwoRows holds the grain of a row. The session
// is shared and the tool-schema digest is per provider, so two models of one
// run are two rows and never one, whatever they were served.
func TestFold_TwoModelsOneSession_AreTwoRows(t *testing.T) {
	records := publishableShard()
	second := *fixtureAttempt()
	second.ID = fixtureCase + "/dynamic/2"
	second.Model = "openai:other-model"
	records = append(records,
		modelrecord.Record{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeAttempt, Attempt: &second})
	records[0].Run.Providers = append(records[0].Run.Providers, modelrecord.Provider{
		Spec: "openai:other-model", Name: "openai", Model: "other-model",
		Options: map[string]string{"max_tokens": "4096"},
	})
	records[4].Session.ToolSchemaDigests["openai:other-model"] = "tools-2"

	candidates := foldShard(t, records, nil)
	if len(candidates) != 2 {
		t.Fatalf("folded %d candidates, want one per model", len(candidates))
	}
	if candidates[0].key.ToolSchemaDigest == candidates[1].key.ToolSchemaDigest {
		t.Error("the two rows carry one tool-schema digest, so a provider-specific rewrite would be invisible")
	}
}

// TestScore_PublishableShard_CarriesTheCountsAndTheFourTokenNumbers is the fold
// end to end: the columns come from the scorer, the tokens from the turns, and
// nothing is folded into one cost figure.
func TestScore_PublishableShard_CarriesTheCountsAndTheFourTokenNumbers(t *testing.T) {
	one := publishOne(t, publishableShard())

	if one.Counts.Attempts != 1 || one.Counts.Turns != 1 {
		t.Errorf("counts = %d attempt(s) over %d turn(s), want 1 and 1", one.Counts.Attempts, one.Counts.Turns)
	}
	if one.Columns.Reached.Denominator != 1 {
		t.Errorf("reached = %s, want one declared step", one.Columns.Reached)
	}
	want := tokens{Input: 1180, Output: 52, CacheCreated: 900, CacheRead: 1100}
	if one.Tokens != want {
		t.Errorf("tokens\n got %+v\nwant %+v", one.Tokens, want)
	}
	if one.Provenance.Commit != fixtureCommit || one.Provenance.Date != "2026-09-16" {
		t.Errorf("provenance names %q on %q, want the run's own commit and day",
			one.Provenance.Commit, one.Provenance.Date)
	}
}

// TestScore_UnknownCase_IsAnErrorAndNotAnEmptyVerdict holds the one failure
// that must never be quiet. A zero verdict reads as a model that did nothing,
// which is a measurement; an attempt naming a case nobody wrote is not one.
func TestScore_UnknownCase_IsAnErrorAndNotAnEmptyVerdict(t *testing.T) {
	records := publishableShard()
	records[1].Attempt.Case = "MT-does-not-exist"

	_, _, err := judge(foldShard(t, records, nil), modelcorpus.Keys())
	if err == nil {
		t.Fatal("a case the corpus does not have was scored rather than reported")
	}
}

// TestDocument_RoundTrip_SortsRowsAndKeepsTheSchemaVersion checks the bytes a
// re-fold produces: the same rows in the same order, so a diff of two records
// is a diff of the measurements.
func TestDocument_RoundTrip_SortsRowsAndKeepsTheSchemaVersion(t *testing.T) {
	first := row{Key: rowKey{Model: "z", Surface: "meta", Mode: "default", Tier: "free", Repeat: 1}}
	second := row{Key: rowKey{Model: "a", Surface: "dynamic", Mode: "default", Tier: "free", Repeat: 1}}
	doc := document{SchemaVersion: recordSchemaVersion, Note: recordNote, Rows: []row{first, second}}

	body, err := doc.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	read, err := unmarshalDocument(body)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(read.Rows) != 2 || read.Rows[0].Key.Model != "a" {
		t.Errorf("rows came back as %+v, want them sorted by key", read.Rows)
	}
	if doc.Rows[0].Key.Model != "z" {
		t.Error("marshaling sorted the caller's own slice")
	}
}

// TestUnmarshalDocument_AnotherSchemaVersion_IsRefused is the rule every record
// in this tree is held to: a document written by another version of this
// command may spell a field differently, and reading what we recognize would
// compare a page against a record we had only half understood.
func TestUnmarshalDocument_AnotherSchemaVersion_IsRefused(t *testing.T) {
	_, err := unmarshalDocument([]byte(`{"schema_version":99,"rows":[]}`))
	if err == nil {
		t.Fatal("a record of another schema version was read rather than refused")
	}
	if _, brokenErr := unmarshalDocument([]byte(`{not json at all`)); brokenErr == nil {
		t.Fatal("a record that is not JSON was read rather than refused")
	}
}

// TestScore_ASurfaceTheCatalogCannotBeBuiltFor_IsAnError separates the two
// kinds of failure this package has: a model that did badly is a verdict, and a
// record this tree cannot read is an error. A surface that is not one of the
// three is the second.
func TestScore_ASurfaceTheCatalogCannotBeBuiltFor_IsAnError(t *testing.T) {
	records := publishableShard()
	records[1].Attempt.Surface = "a surface nobody serves"

	_, _, err := judge(foldShard(t, records, nil), modelcorpus.Keys())
	if err == nil {
		t.Fatal("a surface the catalog cannot be built for was scored rather than reported")
	}
}

// TestKeyOf_ReadsTheSurfaceTheScorerReads holds the two halves of a row to one
// answer. The key names the surface a table publishes under and the scorer
// reads the one it builds a catalog from; taking them from different lines
// would publish one catalog's figures under another's name.
func TestKeyOf_ReadsTheSurfaceTheScorerReads(t *testing.T) {
	records := publishableShard()
	records[1].Attempt.Surface = ""

	candidates := foldShard(t, records, nil)
	if got := candidates[0].key.Surface; got != "dynamic" {
		t.Errorf("surface = %q, want the session's own when the attempt line left it out", got)
	}
}

// TestKeyOf_WhenTheTwoLinesDisagree_StillReadsTheSurfaceTheScorerScored is the
// half of that claim the fallback cannot test.
//
// Where both lines say the same thing, reading either one passes, so the test
// above holds nothing in place: a key that read the session line would agree
// with the scorer on every fixture and disagree on exactly the run this key
// exists for, one whose attempts did not all run on the session's own surface.
// Here they disagree on purpose, and the key is held to the verdict the scorer
// returns rather than to a literal, because mirroring the scorer is the claim.
func TestKeyOf_WhenTheTwoLinesDisagree_StillReadsTheSurfaceTheScorerScored(t *testing.T) {
	records := publishableShard()
	records[1].Attempt.Surface = "meta"
	records[4].Session.Surface = "dynamic"

	candidates := foldShard(t, records, nil)
	if len(candidates) != 1 {
		t.Fatalf("folded %d candidates, want one", len(candidates))
	}
	attempt := candidates[0].attempts[0]
	verdict, err := modelscore.Score(attempt, modelcorpus.Keys()[fixtureCase])
	if err != nil {
		t.Fatalf("score the attempt: %v", err)
	}
	if verdict.Surface != "meta" {
		t.Fatalf("the scorer scored surface %q, want the attempt line's own", verdict.Surface)
	}
	if got := keyOf(attempt).Surface; got != verdict.Surface {
		t.Errorf("the key names surface %q and the figures under it were scored on %q",
			got, verdict.Surface)
	}
}

// TestColumnsOf_AndCountsOf_PutEveryFigureUnderTheNameItWasComputedFor is the
// mapping test, and the one the rebuild exists for.
//
// Every fixture that scores a real shard has one attempt and one step, so every
// column reads 1 over 1 and any two of them could be swapped without a test
// noticing: a number published under another column's name is precisely the
// class of defect the withdrawn tables shipped. The totals here therefore carry
// a different pair of numbers in every column and a different count in every
// tally, and the whole of both structs is asserted rather than one field of
// each.
func TestColumnsOf_AndCountsOf_PutEveryFigureUnderTheNameItWasComputedFor(t *testing.T) {
	totals := modelscore.Totals{
		Attempts:       9,
		Skipped:        2,
		Unobserved:     3,
		ProviderErrors: 4,
		HarnessErrors:  5,
		GitLabRefused:  6,
		Outcomes: map[modelscore.Outcome]int{
			modelscore.OutcomeCompleted: 7,
			modelscore.OutcomeFailed:    8,
		},
		Declines:          map[modelscore.Decline]int{modelscore.DeclineByText: 10},
		Confirmations:     map[modelscore.Confirmation]int{modelscore.ConfirmationUnaided: 11},
		Reached:           modelscore.Ratio{Numerator: 21, Denominator: 31},
		AcceptedFirstTime: modelscore.Ratio{Numerator: 22, Denominator: 32},
		ArgumentFidelity:  modelscore.Ratio{Numerator: 23, Denominator: 33},
		Confirmation:      modelscore.Ratio{Numerator: 24, Denominator: 34},
		Unaided:           modelscore.Ratio{Numerator: 25, Denominator: 35},
		Completion:        modelscore.Ratio{Numerator: 26, Denominator: 36},
		Overhead:          modelscore.Overhead{Discovery: 27, InvalidParams: 28, Steps: 37},
	}
	wantColumns := columns{
		Reached:           ratio{Numerator: 21, Denominator: 31},
		AcceptedFirstTime: ratio{Numerator: 22, Denominator: 32},
		ArgumentFidelity:  ratio{Numerator: 23, Denominator: 33},
		Confirmation:      ratio{Numerator: 24, Denominator: 34},
		Unaided:           ratio{Numerator: 25, Denominator: 35},
		Completion:        ratio{Numerator: 26, Denominator: 36},
		Overhead:          overhead{Discovery: 27, InvalidParams: 28, Steps: 37},
	}
	if got := columnsOf(totals); got != wantColumns {
		t.Errorf("columns\n got %+v\nwant %+v", got, wantColumns)
	}

	attempts := []modelscore.Attempt{
		{Turns: make([]modelrecord.Turn, 2)},
		{Turns: make([]modelrecord.Turn, 3)},
	}
	wantCounts := counts{
		Attempts:       9,
		Skipped:        2,
		Unobserved:     3,
		ProviderErrors: 4,
		HarnessErrors:  5,
		GitLabRefused:  6,
		Turns:          5,
		Outcomes:       map[string]int{"completed": 7, "failed": 8},
		Declines:       map[string]int{"by-text": 10},
		Confirmations:  map[string]int{"unaided": 11},
	}
	if got := countsOf(totals, attempts); !reflect.DeepEqual(got, wantCounts) {
		t.Errorf("counts\n got %+v\nwant %+v", got, wantCounts)
	}
}

// TestShownOf_ReadsTheSpanTheAttemptsRecorded covers the figure that keeps an
// individual row from being read under its budget.
//
// The budget is one number and the slice is chosen per case, so a row that
// published only the budget would seat an attempt shown 312 tools under the
// figure 128. The span is what says otherwise, and the nil case is what keeps a
// run made before the attempt carried the count from reading as attempts shown
// no tools at all.
func TestShownOf_ReadsTheSpanTheAttemptsRecorded(t *testing.T) {
	shownAttempt := func(count int, overflowed bool) modelscore.Attempt {
		return modelscore.Attempt{Line: modelrecord.Attempt{ShownTools: count, Overflowed: overflowed}}
	}

	for _, testCase := range []struct {
		name     string
		attempts []modelscore.Attempt
		want     *shown
		caption  string
	}{
		{
			name:     "nothing recorded reads as no answer, not as nothing shown",
			attempts: []modelscore.Attempt{{}, {}},
		},
		{
			name:     "one value on a surface that shows the whole list",
			attempts: []modelscore.Attempt{shownAttempt(2, false), shownAttempt(2, false)},
			want:     &shown{Min: 2, Max: 2},
			caption:  "2",
		},
		{
			name: "a span with the over-budget attempts counted",
			attempts: []modelscore.Attempt{
				shownAttempt(96, false), shownAttempt(312, true), shownAttempt(128, false), shownAttempt(168, true),
			},
			want:    &shown{Min: 96, Max: 312, Overflowed: 2},
			caption: "96 to 312, 2 over budget",
		},
		{
			name:     "an attempt that never reached a session does not floor the span",
			attempts: []modelscore.Attempt{shownAttempt(0, false), shownAttempt(64, false)},
			want:     &shown{Min: 64, Max: 64},
			caption:  "64",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := shownOf(testCase.attempts)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("span\n got %+v\nwant %+v", got, testCase.want)
			}
			if got == nil {
				return
			}
			if rendered := got.String(); rendered != testCase.caption {
				t.Errorf("caption is %q, want %q", rendered, testCase.caption)
			}
		})
	}
}

// TestProvenanceOf_IsAssertedWhole holds the other mapping nothing else does.
//
// Two fields of the same type read off the same line swap without a symptom:
// the edition and the GitLab version are both strings of the run, and a row
// that published one under the other's name would still be a full provenance
// and would still pass every rule. The whole struct is compared against a
// literal here, so any such swap is a failure and a field added to the record
// arrives with an assertion rather than a silence.
func TestProvenanceOf_IsAssertedWhole(t *testing.T) {
	one := publishOne(t, publishableShard())
	want := provenance{
		Commit:            fixtureCommit,
		Date:              "2026-09-16",
		GitLabVersion:     "19.3.0",
		Edition:           "enterprise",
		Tier:              "ultimate",
		TierConfirmed:     true,
		Surface:           "dynamic",
		Mode:              "default",
		CapabilitySurface: "full",
		TokenScopes:       []string{"api"},
		ServedTools:       2,
		Provider:          "anthropic",
		Model:             "test-model",
		RequestOptions:    map[string]string{"max_tokens": "4096", "temperature": "0"},
		Repeat:            1,
		CorpusDigest:      modelcorpus.Digest(),
		ContractDigest:    fixtureContract,
		ToolSchemaDigest:  fixtureTools,
	}
	if !reflect.DeepEqual(one.Provenance, want) {
		t.Errorf("provenance\n got %+v\nwant %+v", one.Provenance, want)
	}
}

// TestFold_TwoShardsOfOneConfiguration_RefuseTheSecondByShardName covers the
// collision the record cannot see.
//
// [ReadShards] walks subdirectories, so one -shards directory is routinely
// several runs: dist/modeleval with ce and ee under it, or one directory a
// second run was pointed at by accident. Two shards of one configuration in
// that tree publish one row each, and without a claim made inside the fold both
// would be written and only the next -check would say so, by which time the
// record holds two rows of one configuration and nothing says which run either
// came from.
func TestFold_TwoShardsOfOneConfiguration_RefuseTheSecondByShardName(t *testing.T) {
	dir := t.TempDir()
	first := writeRunDir(t, dir, "run-1")
	writeRunDir(t, dir, "run-2")

	shards, err := modelrecord.ReadShards(dir)
	if err != nil {
		t.Fatalf("read the shards: %v", err)
	}
	candidates, err := fold(shards, map[string]string{})
	if err != nil {
		t.Fatalf("fold the shards: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("folded %d candidates, want one per shard", len(candidates))
	}

	rows, refusals, err := judge(candidates, modelcorpus.Keys())
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("published %d row(s), want the first shard's only", len(rows))
	}
	if len(refusals) != 1 || refusals[0].Rule != "duplicate-row" {
		t.Fatalf("got %v, want the second shard refused as a duplicate", refusals)
	}
	if !strings.Contains(refusals[0].Reason, first) {
		t.Errorf("the refusal %q does not name the shard that claimed the row first", refusals[0].Reason)
	}
}

// writeRunDir puts one run's shard in a subdirectory of its own and returns the
// shard's path, which is what a collision has to name.
func writeRunDir(t *testing.T, dir, name string) string {
	t.Helper()
	under := filepath.Join(dir, name)
	if err := os.MkdirAll(under, 0o750); err != nil {
		t.Fatalf("make the run directory %s: %v", name, err)
	}
	writeShardInto(t, under, "modeleval-fixture.jsonl", publishableShard())
	return filepath.Join(under, "modeleval-fixture.jsonl")
}

// TestRunDate_ARunThatNeverStarted_LeavesTheDateEmpty keeps a hole a hole. The
// zero time would render as the year one, which reads as a day somebody might
// have measured on; empty is what the provenance rule then refuses.
func TestRunDate_ARunThatNeverStarted_LeavesTheDateEmpty(t *testing.T) {
	records := publishableShard()
	records[0].Run.StartedAt = time.Time{}

	_, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys())
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if len(refusals) != 1 || !strings.Contains(refusals[0].Reason, `"date"`) {
		t.Fatalf("got %v, want the empty date refused", refusals)
	}
}

// TestComparisonSummary_CountsTheComparisonsARecordSupports is the sentence a
// maintainer reads after folding a run in, and the only place the second
// comparison key is counted rather than rendered.
func TestComparisonSummary_CountsTheComparisonsARecordSupports(t *testing.T) {
	dynamic := oneRow()
	meta := oneRow()
	meta.Key.Surface = "meta"
	meta.Key.ToolSchemaDigest = "tools-meta"
	alone := oneRow()
	alone.Key.Model = "openai:only-here"

	got := comparisonSummary([]row{dynamic, meta, alone})
	if got != "3 row(s), 2 cross-vendor table(s), 1 cross-surface comparison(s)" {
		t.Errorf("summary = %q", got)
	}
	if empty := comparisonSummary(nil); empty != "0 row(s), 0 cross-vendor table(s), 0 cross-surface comparison(s)" {
		t.Errorf("an empty record summarized as %q", empty)
	}
}

// TestRowLines_RoundTrip_RebuildTheThreeLinesARuleReads is what lets the
// refusal table judge a committed record with the same rules that judged the
// shards it came from.
func TestRowLines_RoundTrip_RebuildTheThreeLinesARuleReads(t *testing.T) {
	one := publishOne(t, publishableShard())

	if got := runOf(one); got.CorpusDigest != modelcorpus.Digest() || got.Filter != "" {
		t.Errorf("run line = %+v, want the row's own corpus digest and no filter", got)
	}
	if got := sessionOf(one); got.Surface != "dynamic" || got.ServedTools != 2 {
		t.Errorf("session line = %+v, want the row's own surface and tool count", got)
	}
	if got := providerOf(one); got.Name != "anthropic" || got.Spec != fixtureModel {
		t.Errorf("provider line = %+v, want the row's own provider and spec", got)
	}
}

// TestParseDate_Unreadable_IsTheZeroTime keeps a hole a hole: a date nothing
// can read must not come back as a day somebody might have measured on.
func TestParseDate_Unreadable_IsTheZeroTime(t *testing.T) {
	if got := parseDate("not a date"); !got.IsZero() {
		t.Errorf("parseDate(%q) = %v, want the zero time", "not a date", got)
	}
}
