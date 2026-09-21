package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	rows, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys(), false)
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

	_, _, err := judge(foldShard(t, records, nil), modelcorpus.Keys(), false)
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

// TestDocument_Marshal_IsTheSameBytesHoweverTheRowsArrived is the other half of
// that sort, and the half two rows cannot ask about.
//
// With two rows the comparator is consulted once and can only ever answer one
// way, so an order that merely reversed whatever it was given would pass the
// round trip above. What the record needs is stronger: the order rows were
// folded in is no part of the file, so two documents holding the same rows are
// the same bytes and a diff of two records is a diff of the measurements rather
// than of the order two runs happened to arrive in.
func TestDocument_Marshal_IsTheSameBytesHoweverTheRowsArrived(t *testing.T) {
	rowNamed := func(model string) row {
		return row{Key: rowKey{Model: model, Surface: "dynamic", Mode: "default", Tier: "free", Repeat: 1}}
	}
	first, second, third := rowNamed("a"), rowNamed("m"), rowNamed("z")

	ascending, err := document{SchemaVersion: recordSchemaVersion, Rows: []row{first, second, third}}.marshal()
	if err != nil {
		t.Fatalf("marshal the ascending document: %v", err)
	}
	descending, err := document{SchemaVersion: recordSchemaVersion, Rows: []row{third, second, first}}.marshal()
	if err != nil {
		t.Fatalf("marshal the descending document: %v", err)
	}
	if !bytes.Equal(ascending, descending) {
		t.Errorf("the same three rows folded in two orders wrote two files:\n%s\n---\n%s", ascending, descending)
	}

	read, err := unmarshalDocument(ascending)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	models := []string{read.Rows[0].Key.Model, read.Rows[1].Key.Model, read.Rows[2].Key.Model}
	if !slices.IsSorted(models) {
		t.Errorf("the record holds the models in the order %v, want them sorted by key", models)
	}
}

// TestDocument_EveryPartOfARow_SurvivesTheFileItIsWrittenTo states the property
// that makes the two error arms around this marshaling unreachable, and that
// nothing else in the package asserts.
//
// [document.marshal] and [runWrite] both branch on an error encoding/json
// cannot produce for this type: a document is strings, ints, bools, pointers to
// them, slices of them and maps keyed by string, with no channel, no function,
// no cycle and no float that could be a NaN. Deleting either arm is what
// errcheck refuses, so the arm stays and what is asserted instead is the claim
// behind it -- that every block of a row reaches the file and comes back. A
// field the file cannot carry fails the marshal here, and one it silently drops
// fails the comparison; what this cannot see is two fields exchanging tags,
// which a round trip restores as symmetrically as it wrote.
func TestDocument_EveryPartOfARow_SurvivesTheFileItIsWrittenTo(t *testing.T) {
	one := publishOne(t, twoCaseShard())
	if len(one.Cases) != 2 {
		t.Fatalf("the fixture row carries %d case(s), want the two the shard measured", len(one.Cases))
	}
	one.Key.TierPin, one.Key.MetaParamSchema, one.Key.SliceSize = "ultimate", "compact", 128
	one.Provenance.TierPin, one.Provenance.MetaParamSchema, one.Provenance.SliceSize = "ultimate", "compact", 128
	one.Counts.Shown = &shown{Min: 96, Max: 312, Overflowed: 2}

	body, err := document{SchemaVersion: recordSchemaVersion, Note: recordNote, Rows: []row{one}}.marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	read, err := unmarshalDocument(body)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(read.Rows) != 1 {
		t.Fatalf("the record came back holding %d row(s), want one", len(read.Rows))
	}
	if !reflect.DeepEqual(read.Rows[0], one) {
		t.Errorf("the row came back as\n got %+v\nwant %+v", read.Rows[0], one)
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

	_, _, err := judge(foldShard(t, records, nil), modelcorpus.Keys(), false)
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
		{
			// Every case above meets its smallest list first, so the floor is
			// settled by the opening value and a span that only ever rose would
			// read the same. Here the smaller list arrives second, which is the
			// only arrangement that asks whether the floor is a minimum or
			// merely the first thing seen.
			name:     "a later attempt shown fewer tools lowers the floor",
			attempts: []modelscore.Attempt{shownAttempt(312, true), shownAttempt(96, false)},
			want:     &shown{Min: 96, Max: 312, Overflowed: 1},
			caption:  "96 to 312, 1 over budget",
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

	rows, refusals, err := judge(candidates, modelcorpus.Keys(), false)
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

	_, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys(), false)
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

// secondCaseRecords returns the attempt, turn and call lines of one more
// completed single-step case, so a shard can carry more than the one case
// publishableShard has.
//
// It exists for the partial-fold tests: a shard covering two cases and a
// re-run covering one of them is the shape that tells a merge from a
// replacement, and the base fixture cannot express it.
func secondCaseRecords() []modelrecord.Record {
	const (
		caseID  = "MT-003"
		action  = "project.list"
		attempt = caseID + "/dynamic/1"
	)
	args := json.RawMessage(`{"action":"` + action + `","params":{"per_page":"10"}}`)
	return []modelrecord.Record{
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeAttempt, Attempt: &modelrecord.Attempt{
			ID:       attempt,
			Case:     caseID,
			Model:    fixtureModel,
			Surface:  "dynamic",
			Session:  fixtureSession,
			Repeat:   1,
			Stimulus: "List the 10 most recently updated projects I can access.",
			EndedBy:  modelrecord.EndedCompleted,
		}},
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeTurn, Turn: &modelrecord.Turn{
			Attempt: attempt,
			Index:   1,
			Try:     1,
			Blocks: []modelrecord.Block{{
				Kind:      modelrecord.BlockToolCall,
				Tool:      "gitlab_execute_action",
				Arguments: args,
				CallID:    "call_2",
			}},
			Usage:  modelrecord.Usage{Input: 900, Output: 40},
			Status: modelrecord.TurnOK,
		}},
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeCall, Call: &modelrecord.Call{
			Attempt:          attempt,
			Turn:             1,
			Index:            1,
			Tool:             "gitlab_execute_action",
			Arguments:        args,
			RequestedAction:  action,
			DispatchedAction: action,
			DispatchedTool:   "gitlab_execute_action",
			Outcome:          modelrecord.OutcomeOK,
			Result:           json.RawMessage(`{"projects":[{"id":42}]}`),
			Text:             "| id |",
			TraceID:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa2",
			Requests:         1,
			DispatchObserved: true,
		}},
	}
}

// twoCaseShard is a run that measured both MT-002 and MT-003.
func twoCaseShard() []modelrecord.Record {
	return append(publishableShard(), secondCaseRecords()...)
}

// oneCaseRerunShard is the same run configuration measuring MT-003 alone,
// which is what `MODELEVAL_CASES=MT-003` leaves behind.
func oneCaseRerunShard() []modelrecord.Record {
	return append([]modelrecord.Record{
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeRun, Run: fixtureRun()},
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeSession, Session: fixtureSessionLine()},
	}, secondCaseRecords()...)
}

// skippedAttemptRecord is one case the runtime could not offer, written the way
// the runner writes it: a model, a surface and a reason, and no session,
// because none was ever opened.
func skippedAttemptRecord(caseID string) modelrecord.Record {
	return modelrecord.Record{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeAttempt, Attempt: &modelrecord.Attempt{
		ID:      caseID + "/dynamic/1",
		Case:    caseID,
		Model:   fixtureModel,
		Surface: "dynamic",
		Repeat:  1,
		EndedBy: modelrecord.EndedSkipped,
		Reason:  "needs a licensed instance",
	}}
}

// TestGroupShard_ASkippedAttempt_IsCountedInTheRowItBelongsTo covers the whole
// reason the skip line exists.
//
// A case the runtime could not offer is written down so that it is not
// silently absent from the record, and the row has a Skipped column to count it
// in. But a skip never opened a session, and a row's identity reads the mode,
// the tier pin, the meta schema and the slice size off the session line, so the
// skip could not be keyed: it formed a candidate of its own whose provenance
// was empty, the provenance rule refused it, and every published row read
// skipped: 0 however many cases the instance had turned away.
func TestGroupShard_ASkippedAttempt_IsCountedInTheRowItBelongsTo(t *testing.T) {
	records := append(publishableShard(), skippedAttemptRecord("MT-017"))

	rows, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys(), false)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if len(refusals) > 0 {
		t.Fatalf("the skip was refused instead of counted: %v", refusals)
	}
	if len(rows) != 1 {
		t.Fatalf("published %d rows, want the skip folded into the one measurement", len(rows))
	}
	if got := rows[0].Counts.Skipped; got != 1 {
		t.Errorf("the row counts %d skipped, want 1", got)
	}
	if got := rows[0].Counts.Attempts; got != 1 {
		t.Errorf("the row counts %d attempts, want the skip left out of every denominator", got)
	}
}

// TestGroupShard_ASkippedAttemptNothingClaims_IsRefusedRatherThanGuessed is the
// other half: a skip naming only its model and its surface, with no measurement
// of that model in the shard to belong to, keeps a candidate of its own and is
// refused by name. Placing it anywhere would credit a row with an attempt it
// never made, and a refusal a maintainer can read beats a figure nobody can
// check.
func TestGroupShard_ASkippedAttemptNothingClaims_IsRefusedRatherThanGuessed(t *testing.T) {
	records := []modelrecord.Record{
		{Schema: modelrecord.SchemaVersion, Type: modelrecord.TypeRun, Run: fixtureRun()},
		skippedAttemptRecord("MT-017"),
	}

	_, refusals, err := judge(foldShard(t, records, nil), modelcorpus.Keys(), false)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if len(refusals) != 1 {
		t.Fatalf("a skip belonging to no measurement produced %d refusal(s), want one", len(refusals))
	}
	if !strings.Contains(refusals[0].Reason, "surface") && !strings.Contains(refusals[0].Reason, "mode") {
		t.Errorf("the refusal reads %q, want it to name the provenance it has not got", refusals[0].Reason)
	}
}

// measuredOn is one attempt that opened a session, built for [groupShard]
// directly rather than through a shard, because what is being asked about is
// the placement and not the join that feeds it.
func measuredOn(model, surface, session, mode string) modelscore.Attempt {
	return modelscore.Attempt{
		Run:     *fixtureRun(),
		Session: modelrecord.Session{Label: session, Surface: surface, Mode: mode},
		Line:    modelrecord.Attempt{Case: fixtureCase, Model: model, Surface: surface, Session: session, Repeat: 1},
	}
}

// skippedOn is one attempt the runtime turned away: a model, a surface and
// nothing else, because no session was ever opened.
func skippedOn(model, surface, caseID string) modelscore.Attempt {
	return modelscore.Attempt{
		Run:  *fixtureRun(),
		Line: modelrecord.Attempt{Case: caseID, Model: model, Surface: surface, Repeat: 1, EndedBy: modelrecord.EndedSkipped},
	}
}

// TestPlaceSessionless_ASkipSeveralMeasurementsCouldBelongTo_KeepsItsOwnRow is
// the branch the comment beside [placeSessionless] promises and no fixture
// reached: the one where the answer is refused rather than guessed.
//
// A skip names its model and its surface and nothing else. Where exactly one
// measurement of that pair is in the shard it joins it, which the test above
// covers. Where two are — one shard measuring one model on one surface in two
// modes — choosing either would credit a row with an attempt it never made, so
// the skips keep a candidate of their own with no session behind it, which the
// provenance rule then refuses by name. Both skips land on that same candidate
// rather than on one each, and a second fold over a record that already holds
// that key says where it already stands instead of publishing it twice.
func TestPlaceSessionless_ASkipSeveralMeasurementsCouldBelongTo_KeepsItsOwnRow(t *testing.T) {
	const other = "openai:other-model"
	attempts := []modelscore.Attempt{
		measuredOn(fixtureModel, "meta", "meta-default", "default"),
		measuredOn(fixtureModel, "meta", "meta-read-only", "read-only"),
		// A row of the same model on another surface, and a row of another
		// model on this one. Neither is a home for a skip naming this model on
		// meta, and each is passed over for its own reason.
		measuredOn(fixtureModel, "dynamic", "dynamic-default", "default"),
		measuredOn(other, "meta", "meta-default", "default"),
		skippedOn(fixtureModel, "meta", "MT-017"),
		skippedOn(fixtureModel, "meta", "MT-013"),
	}

	candidates := groupShard("modeleval-ambiguous.jsonl", attempts, map[string]string{})
	if len(candidates) != 5 {
		t.Fatalf("grouped into %d candidate(s), want the four measurements and one candidate for the skips", len(candidates))
	}
	for _, one := range candidates[:4] {
		if len(one.attempts) != 1 {
			t.Errorf("the measurement %s took %d attempt(s), want only its own", one.key.String(), len(one.attempts))
		}
	}

	orphan := candidates[4]
	if orphan.session.Label != "" {
		t.Errorf("the skips were keyed off session %q, want a candidate with no session behind it", orphan.session.Label)
	}
	if len(orphan.attempts) != 2 {
		t.Fatalf("the skips formed %d candidate(s) holding %d attempt(s), want both on one",
			len(candidates)-4, len(orphan.attempts))
	}
	if orphan.claimedBy != "" {
		t.Errorf("nothing had published this key and it reads as claimed by %q", orphan.claimedBy)
	}

	// The same shard folded against a record that already holds that key: the
	// candidate is the same one and it now names where the row stands, which is
	// what the duplicate-row rule refuses it by.
	again := groupShard("modeleval-ambiguous.jsonl", attempts, map[string]string{orphan.key.String(): "an-earlier-shard.jsonl"})
	if len(again) != 5 {
		t.Fatalf("the second fold grouped into %d candidate(s), want the same five", len(again))
	}
	if again[4].claimedBy != "an-earlier-shard.jsonl" {
		t.Errorf("the re-folded skips read as claimed by %q, want the shard that already published the key", again[4].claimedBy)
	}
}
