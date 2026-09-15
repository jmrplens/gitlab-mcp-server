package modelrecord

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// writeShard writes one shard file holding the given raw lines and returns its
// path.
func writeShard(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	return path
}

// populatedLines returns every line type with every field of the record filled,
// and the run line's timestamp apart, since a time compares by its instant
// rather than by its representation.
//
// Every field is filled on purpose, and
// TestPopulatedLines_LeaveNoFieldUnheld is what makes that true rather than
// intended. The round trip below asks nothing of a field left at its zero,
// since zero compares equal to zero however the field was written, so a value
// the encoder drops on the way out passes unseen. The spelling of the names is
// a question of its own, answered by
// TestRecordFieldNames_AreTheGoNamesInSnakeCase, because one struct writes and
// reads a shard and a misspelled tag therefore agrees with itself here.
//
// Some fields cannot be filled together on one line: a call carries either a
// dispatched action or a refusal reason, and a request that was rate limited
// carried no blocks and was billed for nothing. So two turns and two calls are
// written, each of them a line a real run would produce, and the fields are
// covered across them rather than by contorting one line into a shape the runner
// never writes.
func populatedLines() (time.Time, []Line) {
	startedAt := time.Date(2026, 9, 15, 11, 22, 33, 0, time.UTC)
	return startedAt, []Line{
		&Run{
			Package:        "modeleval",
			RunID:          "20260915t112233z-9f1c2d-ce",
			Commit:         "642817bac",
			StartedAt:      startedAt,
			Edition:        "community",
			GitLabVersion:  "19.4.1",
			Tier:           "free",
			TierConfirmed:  true,
			Filter:         "TestModelEval/MT-205",
			CorpusDigest:   "sha256:corpus",
			ContractDigest: "sha256:contract",
			Providers: []Provider{{
				Spec:    "anthropic:claude-x;temperature=0",
				Name:    "anthropic",
				Model:   "claude-x",
				Options: map[string]string{"temperature": "0", "max_tokens": "4096"},
				Price: &Price{
					InputPerMillionUSD:      3,
					OutputPerMillionUSD:     15,
					CacheWritePerMillionUSD: 3.75,
					CacheReadPerMillionUSD:  0.3,
					RetrievedOn:             "2026-09-15",
				},
			}},
			BudgetUSD: 12.5,
			Repeat:    3,
		},
		&Session{
			Label:             "dynamic/read-only/premium",
			Surface:           "dynamic",
			Mode:              "read-only",
			Capabilities:      "full",
			MetaParamSchema:   "opaque",
			TierPin:           "premium",
			SliceSize:         128,
			TokenScopes:       []string{"api", "read_api"},
			ServedTools:       2,
			ToolSchemaDigests: map[string]string{"anthropic:claude-x": "sha256:tools"},
		},
		&Attempt{
			ID:       "MT-205/anthropic:claude-x/dynamic/1",
			Case:     "MT-205",
			Model:    "anthropic:claude-x",
			Surface:  "dynamic",
			Session:  "dynamic/read-only/premium",
			Repeat:   1,
			Facts:    map[string]string{"project_path": "g/p", "issue_iid": "7"},
			Stimulus: "List the open issues of g/p and close the oldest one.",
			EndedBy:  EndedMalformed,
			Reason:   `the third turn's tool call did not parse: {"action": "issue.list", "params": {`,
		},
		// The request that was refused for rate and the try that answered are
		// two lines at one index, which is what the record means by "each
		// recorded" and is why Try is a field of its own.
		&Turn{
			Attempt:       "MT-205/anthropic:claude-x/dynamic/1",
			Index:         2,
			Try:           1,
			RequestDigest: "sha256:request",
			Status:        TurnRateLimited,
			Detail:        "429 rate_limit_error: retry in 2s",
			LatencyMS:     118.75,
		},
		&Turn{
			Attempt:       "MT-205/anthropic:claude-x/dynamic/1",
			Index:         2,
			Try:           2,
			RequestDigest: "sha256:request",
			Blocks: []Block{
				{Kind: BlockThinking, Text: "the catalog has a list action"},
				{Kind: BlockText, Text: "Looking it up."},
				{
					Kind:      BlockToolCall,
					Tool:      "gitlab_execute_action",
					Arguments: json.RawMessage(`{"action":"issue.list","params":{"project_id":"g/p"}}`),
					CallID:    "toolu_01",
				},
				// The malformed call the attempt ended on: its text is kept as
				// it was emitted, because a raw field cannot hold something
				// that does not parse.
				{
					Kind:   BlockToolCall,
					Tool:   "gitlab_execute_action",
					Raw:    `{"action": "issue.list", "params": {`,
					CallID: "toolu_02",
				},
			},
			Usage:     Usage{Input: 1200, Output: 90, CacheCreated: 800, CacheRead: 400},
			Status:    TurnOK,
			LatencyMS: 1843.5,
		},
		&Call{
			Attempt:          "MT-205/anthropic:claude-x/dynamic/1",
			Turn:             2,
			Index:            1,
			Tool:             "gitlab_execute_action",
			Arguments:        json.RawMessage(`{"action":"issue.list","params":{"project_id":"g/p"}}`),
			RequestedAction:  "issue.list",
			DispatchedAction: "issue.list",
			DispatchedTool:   "gitlab_execute_action",
			Outcome:          OutcomeOK,
			Result:           json.RawMessage(`"{\"issues\":[{\"iid\":7}"`),
			ResultTruncated:  true,
			Text:             "| iid | title |",
			TextTruncated:    true,
			DurationMS:       312.25,
			TraceID:          "0af7651916cd43dd8448eb211c80319c",
			Requests:         1,
			DispatchObserved: true,
		},
		// The mutating step the read-only session declined. It carries a
		// refusal reason and no dispatched action, which is the shape of every
		// withheld call and the reason RequestedAction is recorded beside the
		// dispatched one.
		&Call{
			Attempt:          "MT-205/anthropic:claude-x/dynamic/1",
			Turn:             2,
			Index:            2,
			Tool:             "gitlab_execute_action",
			Arguments:        json.RawMessage(`{"action":"issue.close","params":{"project_id":"g/p","issue_iid":7}}`),
			RequestedAction:  "issue.close",
			RefusalReason:    "read_only",
			Outcome:          RefusedOutcome("read_only"),
			Text:             "This server is running in read-only mode.",
			DurationMS:       4.5,
			TraceID:          "0af7651916cd43dd8448eb211c80319d",
			DispatchObserved: true,
		},
		&Verify{
			Attempt: "MT-205/anthropic:claude-x/dynamic/1",
			Name:    "the issue is closed",
			Passed:  true,
			Detail:  "state = closed",
		},
	}
}

// TestReadShards_RoundTripsEveryLineType verifies that each of the six lines
// survives the writer and the reader with every field it carried.
//
// It is the contract this whole package is: the runner writes a shard during a
// paid run, and nothing reads it until a scoring command opens it afterwards,
// possibly months later against a corrected rule. A field that does not survive
// the round trip is a field the score is computed without, silently.
func TestReadShards_RoundTripsEveryLineType(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	startedAt, lines := populatedLines()
	reporter := &recordingReporter{}
	OpenDir(dir).Write(reporter, lines...)
	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}

	records, err := Read(dir)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if len(records) != len(lines) {
		t.Fatalf("records = %d, want %d", len(records), len(lines))
	}

	for index, line := range lines {
		record := records[index]
		// The position is part of the name because two turns and two calls are
		// written, and a subtest that fails has to say which of them did.
		t.Run(fmt.Sprintf("%s line %d", record.Type, index+1), func(t *testing.T) {
			want := line.record()
			if record.Type != want.Type {
				t.Fatalf("line %d is a %s, want a %s: the lines are read in the order they were written", index, record.Type, want.Type)
			}
			// The run line's instant is compared as an instant; the rest of it
			// goes through the same deep comparison as the other five once the
			// two timestamps are known to agree.
			if record.Run != nil {
				if !record.Run.StartedAt.Equal(startedAt) {
					t.Errorf("Run.StartedAt = %s, want %s", record.Run.StartedAt, startedAt)
				}
				record.Run.StartedAt, want.Run.StartedAt = time.Time{}, time.Time{}
			}
			if !reflect.DeepEqual(record, want) {
				t.Errorf("round trip of the %s line\n got %+v\nwant %+v", record.Type, record, want)
			}
		})
	}
}

// TestPopulatedLines_LeaveNoFieldUnheld verifies that the fixture the round
// trip is run over fills every value the six line types can carry, so that
// every JSON name in the record is held by something.
//
// It is what turns "every field is filled on purpose" from a comment into a
// rule. The round trip asks nothing of a field left at its zero: zero is
// compared with zero and matches however the field was written, so a tag of
// "-", two fields colliding on one name, or a value the encoder drops would all
// pass unseen there. Four fields were in that state when this was written, two
// of them the ones a score is read from, [Verify.Passed] and
// [Call.RefusalReason].
//
// What it deliberately does not answer is the spelling of a name, because no
// round trip can: one struct writes and reads a shard, so a misspelled tag
// agrees with itself. That is
// TestRecordFieldNames_AreTheGoNamesInSnakeCase, and the two are only useful
// together.
//
// The walk is over the types rather than over the fixture, so a field added to
// the record later fails here until a line carries it, and it descends into
// [Block], [Provider], [Price] and [Usage] for the same reason.
func TestPopulatedLines_LeaveNoFieldUnheld(t *testing.T) {
	_, lines := populatedLines()

	filled := map[string]bool{}
	written := map[string]bool{}
	for _, line := range lines {
		record := line.record()
		written[record.Type] = true
		filledValues(reflect.ValueOf(line).Elem(), record.Type, filled)
	}

	for lineType, payload := range payloadTypes(t) {
		t.Run(lineType, func(t *testing.T) {
			if !written[lineType] {
				t.Fatalf("no %s line is written, so nothing holds its field names to anything", lineType)
			}
			for _, leaf := range leafValues(payload, lineType) {
				if !filled[leaf.path] {
					t.Errorf("%s is never filled, so the round trip compares its zero with its zero and asks nothing of how it is written", leaf.path)
				}
			}
		})
	}
}

// TestRead_MergesEveryShardUnderTheDirectory verifies that Read returns what
// the writer wrote, from the directory and from its subdirectories, and ignores
// everything that is not a shard.
//
// Subdirectories are read because one Docker target records into a directory of
// its own and a cross-surface table is built by comparing them. Files that are
// not shards are ignored because the same directory carries the coverage record
// of the very same run, in a subdirectory of its own.
func TestRead_MergesEveryShardUnderTheDirectory(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(Release)

	reporter := &recordingReporter{}
	OpenDir(root).Write(reporter,
		&Run{Package: "modeleval", Edition: "community", Tier: "free"},
		&Attempt{ID: "a1", Case: "MT-205", EndedBy: EndedCompleted},
	)
	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
	nested := filepath.Join(root, "ee")
	writeShard(t, nested, "modeleval-ee.jsonl",
		`{"schema":1,"type":"verify","verify":{"attempt":"a1","name":"the epic exists","passed":true}}`,
		"",
	)
	writeShard(t, root, "notes.txt", "not a shard")
	writeShard(t, root, "modeleval-report.txt", "not a shard either")

	records, err := Read(root)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3: %+v", len(records), records)
	}
	byType := map[string]int{}
	for _, record := range records {
		byType[record.Type]++
	}
	for _, lineType := range []string{TypeRun, TypeAttempt, TypeVerify} {
		t.Run(lineType, func(t *testing.T) {
			if byType[lineType] != 1 {
				t.Errorf("%s lines = %d, want 1", lineType, byType[lineType])
			}
		})
	}
	for _, record := range records {
		if record.Attempt != nil && record.Attempt.Case != "MT-205" {
			t.Errorf("attempt case = %q, want the one the writer wrote", record.Attempt.Case)
		}
	}
}

// TestReadShards_KeepsTheFileBoundaries verifies that ReadShards hands back one
// entry per shard file, each with its path and its own lines in file order, so
// a reader can join every attempt to the run line of the process that wrote it.
// Read is the same walk with the boundaries dropped, and the two must agree on
// what was read.
//
// The boundary is load-bearing here and not a convenience. No attempt, turn,
// call or verify line names its run, so the shard is the only thing that says
// which commit, which instance and which tier a row was measured under, and a
// row published without those is exactly the provenance failure the tables this
// replaces had.
func TestReadShards_KeepsTheFileBoundaries(t *testing.T) {
	root := t.TempDir()
	ce := writeShard(t, root, "modeleval-ce.jsonl",
		`{"schema":1,"type":"attempt","attempt":{"id":"a1","case":"MT-205","model":"fake:perfect","surface":"dynamic","session":"d","repeat":1,"ended_by":"completed"}}`,
		`{"schema":1,"type":"run","run":{"package":"modeleval","run_id":"r","started_at":"2026-09-15T00:00:00Z","edition":"community","gitlab_version":"19.4.1","tier":"free","tier_confirmed":true,"corpus_digest":"c","contract_digest":"k","repeat":1}}`,
	)
	ee := writeShard(t, filepath.Join(root, "ee"), "modeleval-ee.jsonl",
		`{"schema":1,"type":"run","run":{"package":"modeleval","run_id":"r","started_at":"2026-09-15T00:00:00Z","edition":"enterprise","gitlab_version":"19.4.1","tier":"ultimate","tier_confirmed":true,"corpus_digest":"c","contract_digest":"k","repeat":1}}`,
	)

	shards, err := ReadShards(root)
	if err != nil {
		t.Fatalf("ReadShards error = %v", err)
	}
	if len(shards) != 2 {
		t.Fatalf("shards = %d, want 2: %+v", len(shards), shards)
	}
	// A walk is lexical, and the ee subdirectory sorts before the ce shard's
	// own name, so the nested shard comes first.
	if shards[0].Path != ee || shards[1].Path != ce {
		t.Errorf("paths = %q, %q; want %q then %q in walk order", shards[0].Path, shards[1].Path, ee, ce)
	}
	if len(shards[0].Records) != 1 || shards[0].Records[0].Run == nil || shards[0].Records[0].Run.Tier != "ultimate" {
		t.Errorf("ee shard = %+v, want the one run line naming the licensed tier", shards[0].Records)
	}
	if len(shards[1].Records) != 2 || shards[1].Records[0].Type != TypeAttempt || shards[1].Records[1].Type != TypeRun {
		t.Errorf("ce shard = %+v, want its attempt then its run line", shards[1].Records)
	}

	merged, err := Read(root)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if len(merged) != 3 {
		t.Errorf("Read = %d records, want the 3 the shards hold together", len(merged))
	}
}

// TestRead_RefusesADirectoryHoldingNoShard verifies that an empty directory is
// an error naming the switch, rather than an empty result.
//
// An empty result would be rendered as a table of zeroes, which is a claim
// about four models made from a fact about one environment variable: nothing
// was recorded because nothing asked for recording.
func TestRead_RefusesADirectoryHoldingNoShard(t *testing.T) {
	records, err := Read(t.TempDir())

	if err == nil {
		t.Fatalf("Read = %d record(s), want an error", len(records))
	}
	if !strings.Contains(err.Error(), DirEnv) {
		t.Errorf("Read error = %v, want it to name %s", err, DirEnv)
	}
}

// TestRead_ReportsADirectoryItCannotWalk verifies that a directory that is not
// there is reported with its path.
func TestRead_ReportsADirectoryItCannotWalk(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never-created")

	_, err := Read(missing)

	if err == nil {
		t.Fatal("Read error = nil, want an error for a missing directory")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("Read error = %v, want it to name %s", err, missing)
	}
}

// TestRead_ReportsALineItCannotRead verifies that a shard line which is not
// JSON, or which is JSON the record does not hold together, is reported with
// the file and the line number.
//
// A shard is machine-written, so either means the artifact is stale or
// truncated. The line number is what makes that diagnosable at all, since a
// shard has no other landmarks.
func TestRead_ReportsALineItCannotRead(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{name: "not JSON at all", line: "{", want: "parse"},
		{
			name: "a schema this package did not write",
			line: `{"schema":2,"type":"attempt","attempt":{"id":"a1"}}`,
			want: "another version",
		},
		{
			name: "a line type this package does not know",
			line: `{"schema":1,"type":"verdict","attempt":{"id":"a1"}}`,
			want: "unknown line type",
		},
		{
			name: "a type whose payload is missing",
			line: `{"schema":1,"type":"turn","attempt":{"id":"a1"}}`,
			want: "carries no payload",
		},
		{
			name: "two payloads under one type",
			line: `{"schema":1,"type":"call","call":{"attempt":"a1"},"turn":{"attempt":"a1"}}`,
			want: "carries 2 payloads",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			writeShard(t, dir, "modeleval-broken.jsonl", testCase.line)

			_, err := Read(dir)

			if err == nil {
				t.Fatal("Read error = nil, want an error")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Read error = %v, want it to say %q", err, testCase.want)
			}
			if !strings.Contains(err.Error(), "modeleval-broken.jsonl") {
				t.Errorf("Read error = %v, want it to name the shard", err)
			}
		})
	}
}

// TestRead_NamesTheLineTheBadRecordIsOn verifies that the error counts lines as
// a person reading the file does: blank lines are skipped by the reader but
// still counted, so the number in the message is the one an editor shows.
func TestRead_NamesTheLineTheBadRecordIsOn(t *testing.T) {
	dir := t.TempDir()
	writeShard(t, dir, "modeleval-broken.jsonl",
		`{"schema":1,"type":"verify","verify":{"attempt":"a1","name":"n","passed":true}}`,
		"",
		`{"schema":1,"type":"verify"}`,
	)

	_, err := Read(dir)

	if err == nil {
		t.Fatal("Read error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("Read error = %v, want it to name line 3", err)
	}
}

// TestShardPattern_MatchesWhatTheWriterNames verifies that the pattern a reader
// is told to look for is the one a writer's own file name satisfies, and that
// isShard agrees with filepath.Match on it.
//
// The two are spelled apart on purpose, the pattern from its prefix and
// extension and the predicate from the same two constants, so nothing but a
// test holds them to each other.
func TestShardPattern_MatchesWhatTheWriterNames(t *testing.T) {
	if ShardPattern != "modeleval-*.jsonl" {
		t.Errorf("ShardPattern = %q, want modeleval-*.jsonl", ShardPattern)
	}

	dir := t.TempDir()
	Release()
	t.Cleanup(Release)
	OpenDir(dir).Write(&recordingReporter{}, &Verify{Attempt: "a1", Name: "n"})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("the writer left %d file(s) in its directory, want exactly one shard", len(entries))
	}
	name := entries[0].Name()
	matched, matchErr := filepath.Match(ShardPattern, name)
	if matchErr != nil {
		t.Fatalf("Match error = %v", matchErr)
	}
	if !matched {
		t.Errorf("the shard the writer named, %q, does not match ShardPattern %q", name, ShardPattern)
	}
	if !isShard(name) {
		t.Errorf("isShard(%q) = false, want true: it must agree with ShardPattern", name)
	}
}

// TestShardPattern_DoesNotMatchTheCoverageRecord verifies that the two records a
// model run writes are told apart by name as well as by directory.
//
// One run writes both, the coverage shards into a subdirectory of the model
// shards' own tree, and a reader that walked one tree and accepted the other's
// files would score a table over lines written by a different harness for a
// different question.
func TestShardPattern_DoesNotMatchTheCoverageRecord(t *testing.T) {
	for _, name := range []string{"calls-abc.jsonl", "modeleval.jsonl", "modeleval-abc.json"} {
		t.Run(name, func(t *testing.T) {
			if isShard(name) {
				t.Errorf("isShard(%q) = true, want false", name)
			}
		})
	}
}

// TestReadShard_ReportsAFileItCannotOpen verifies that a shard that cannot be
// opened is reported.
//
// It is called directly because a walk only offers files it has just listed, so
// this branch is unreachable through Read on any tree a test can build.
func TestReadShard_ReportsAFileItCannotOpen(t *testing.T) {
	_, err := readShard(filepath.Join(t.TempDir(), "modeleval-missing.jsonl"))

	if err == nil {
		t.Fatal("readShard error = nil, want an error for a file that is not there")
	}
	if !strings.Contains(err.Error(), "open shard") {
		t.Errorf("readShard error = %v, want it to say the shard could not be opened", err)
	}
}

// TestRead_RefusesALineBeyondTheCap verifies that a line longer than the cap is
// reported rather than skipped.
//
// A line silently dropped for being long looks exactly like a call the model
// never made, and the call lines are the long ones: each carries a structured
// result bounded at MaxResultBytes, which JSON escaping can multiply several
// times on the way out.
func TestRead_RefusesALineBeyondTheCap(t *testing.T) {
	dir := t.TempDir()
	writeShard(t, dir, "modeleval-long.jsonl", strings.Repeat("x", maxShardLine+1))

	_, err := Read(dir)

	if err == nil {
		t.Fatal("Read error = nil, want an error for a line beyond the cap")
	}
	if !strings.Contains(err.Error(), "modeleval-long.jsonl") {
		t.Errorf("Read error = %v, want it to name the shard", err)
	}
}

// TestRead_AcceptsACallLineCarryingACappedResult verifies that the line cap
// leaves room for both capped values of one call line once JSON has escaped
// them.
//
// The three constants are set apart and only this holds them to each other: a
// result bounded at MaxResultBytes and a rendered answer bounded at
// MaxTextBytes, both of control characters, become a six-character escape per
// byte on the way out, and a call line carries the two of them together. A
// reader that refused what its own writer produced would lose the run at the
// moment it was read rather than when it was written, and that loss is a whole
// shard: every attempt the process paid for, not the one long answer.
func TestRead_AcceptsACallLineCarryingACappedResult(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	// Every byte of these escapes to six characters, which is the worst case
	// the line cap has to survive.
	result, resultTruncated := CapResult(json.RawMessage(strings.Repeat("\x01", MaxResultBytes+1)))
	if !resultTruncated {
		t.Fatal("CapResult did not truncate a result past the cap")
	}
	text, textTruncated := CapText(strings.Repeat("\x01", MaxTextBytes+1))
	if !textTruncated {
		t.Fatal("CapText did not truncate a rendered answer past the cap")
	}
	reporter := &recordingReporter{}
	OpenDir(dir).Write(reporter, &Call{
		Attempt: "a1", Turn: 1, Index: 1, Tool: "gitlab_execute_action",
		Outcome: OutcomeOK, Result: result, ResultTruncated: true,
		Text: text, TextTruncated: true, DispatchObserved: true,
	})
	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}

	records, err := Read(dir)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if len(records) != 1 || records[0].Call == nil {
		t.Fatalf("records = %+v, want the one call line", records)
	}
	if !records[0].Call.ResultTruncated || !records[0].Call.TextTruncated {
		t.Errorf("the call line came back truncated result = %t, text = %t; want both flags the writer set",
			records[0].Call.ResultTruncated, records[0].Call.TextTruncated)
	}
}
