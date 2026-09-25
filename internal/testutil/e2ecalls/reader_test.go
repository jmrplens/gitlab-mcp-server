package e2ecalls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestRead_MergesEveryShardUnderTheDirectory verifies that Read returns what
// the writer wrote, from the directory and from its subdirectories, and
// ignores everything that is not a shard.
//
// Subdirectories are read because one Docker target records into a directory
// of its own and the audit compares the runtimes against each other. Files
// that are not shards are ignored because the same directory carries the
// gotestsum JSON and the junit report of the run.
func TestRead_MergesEveryShardUnderTheDirectory(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(Release)

	reporter := &recordingReporter{}
	OpenDir(root).Write(reporter,
		&Run{Package: "common", Requirement: "any", Status: RunStarted},
		&Call{Test: "TestCommon_Issues", Action: "issue.list", Outcome: OutcomeOK, TestStatus: StatusPassed},
	)
	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
	nested := filepath.Join(root, "ee")
	writeShard(t, nested, "calls-ee.jsonl",
		`{"schema":2,"type":"skip","skip":{"test":"TestEE_Epics","reason":"no license"}}`,
		"",
	)
	writeShard(t, root, "notes.txt", "not a shard")
	writeShard(t, root, "calls-report.txt", "not a shard either")

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
	for _, lineType := range []string{TypeRun, TypeCall, TypeSkip} {
		t.Run(lineType, func(t *testing.T) {
			if byType[lineType] != 1 {
				t.Errorf("%s lines = %d, want 1", lineType, byType[lineType])
			}
		})
	}
	for _, record := range records {
		if record.Call != nil && record.Call.Action != "issue.list" {
			t.Errorf("call action = %q, want the one the writer wrote", record.Call.Action)
		}
	}
}

// TestReadShards_KeepsTheFileBoundaries verifies that ReadShards hands back
// one entry per shard file, each with its path and its own lines in file
// order, so a reader can join every line to the run line of the process that
// wrote it. Read is the same walk with the boundaries dropped, and the two
// must agree on what was read.
func TestReadShards_KeepsTheFileBoundaries(t *testing.T) {
	root := t.TempDir()
	common := writeShard(t, root, "calls-common.jsonl",
		`{"schema":2,"type":"call","call":{"test":"TestIssue_List","purpose":"test","expectation":"ok","session":"d","surface":"dynamic","mode":"default","capabilities":"full","requirement":"any","method":"tools/call","action":"issue.list","outcome":"ok","test_status":"passed"}}`,
		`{"schema":2,"type":"run","run":{"package":"common","requirement":"any","edition":"community","tier":"free","run_id":"r","status":"started"}}`,
	)
	ee := writeShard(t, filepath.Join(root, "ee"), "calls-ee.jsonl",
		`{"schema":2,"type":"run","run":{"package":"ee","requirement":"enterprise","edition":"enterprise","tier":"ultimate","run_id":"r","status":"started"}}`,
	)

	shardFiles, err := ReadShards(root)
	if err != nil {
		t.Fatalf("ReadShards error = %v", err)
	}
	if len(shardFiles) != 2 {
		t.Fatalf("shards = %d, want 2: %+v", len(shardFiles), shardFiles)
	}
	if shardFiles[0].Path != common || shardFiles[1].Path != ee {
		t.Errorf("paths = %q, %q; want %q then %q in walk order", shardFiles[0].Path, shardFiles[1].Path, common, ee)
	}
	if len(shardFiles[0].Records) != 2 || shardFiles[0].Records[0].Type != TypeCall || shardFiles[0].Records[1].Type != TypeRun {
		t.Errorf("common shard = %+v, want its call then its run line", shardFiles[0].Records)
	}
	if len(shardFiles[1].Records) != 1 || shardFiles[1].Records[0].Run == nil || shardFiles[1].Records[0].Run.Package != "ee" {
		t.Errorf("ee shard = %+v, want the one run line naming ee", shardFiles[1].Records)
	}

	merged, err := Read(root)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if len(merged) != 3 {
		t.Errorf("Read = %d records, want the 3 the shards hold together", len(merged))
	}
}

// TestReadShardsForCalls_SchemaOneShard_KeepsItsCallsAndDropsItsSessions
// verifies that a shard written before version 2 is read for its run, call,
// dispatch and skip lines, with its session lines dropped, while a current
// shard beside it keeps its session line, and that the ordinary reader still
// refuses the same directory.
//
// This is the old suite's baseline: written once under version 1 by a suite
// that no longer exists, and compared on what its calls credited, which
// version 2 left alone. Its session lines are the ones version 2 changed the
// meaning of, so they are the one kind a reader must not be handed.
func TestReadShardsForCalls_SchemaOneShard_KeepsItsCallsAndDropsItsSessions(t *testing.T) {
	root := t.TempDir()
	old := writeShard(t, filepath.Join(root, "old"), "calls-old.jsonl",
		`{"schema":1,"type":"run","run":{"package":"suite","requirement":"any","edition":"community","tier":"free","run_id":"r","status":"started"}}`,
		`{"schema":1,"type":"session","session":{"label":"dynamic","surface":"dynamic","mode":"default","capabilities":"full","transport":"in-memory","dispatch_observed":false}}`,
		`{"schema":1,"type":"call","call":{"test":"TestOld_Issues","purpose":"test","expectation":"ok","session":"dynamic","surface":"dynamic","mode":"default","capabilities":"full","requirement":"any","method":"tools/call","action":"issue.list","outcome":"ok","trace_id":"t1"}}`,
		`{"schema":1,"type":"dispatch","dispatch":{"trace_id":"t1","action":"issue.list"}}`,
		`{"schema":1,"type":"skip","skip":{"test":"TestOld_Epics","reason":"no license"}}`,
	)
	current := writeShard(t, filepath.Join(root, "new"), "calls-new.jsonl",
		`{"schema":2,"type":"session","session":{"label":"dynamic","surface":"dynamic","mode":"default","capabilities":"full","transport":"stdio","dispatch_observed":true}}`,
		`{"schema":2,"type":"call","call":{"test":"TestNew_Issues","purpose":"test","expectation":"ok","session":"dynamic","surface":"dynamic","mode":"default","capabilities":"full","requirement":"any","method":"tools/call","action":"issue.list","outcome":"ok"}}`,
	)

	shardFiles, err := ReadShardsForCalls(root)
	if err != nil {
		t.Fatalf("ReadShardsForCalls error = %v, want both schemas read", err)
	}
	types := map[string][]string{}
	for _, shard := range shardFiles {
		for _, record := range shard.Records {
			types[shard.Path] = append(types[shard.Path], record.Type)
		}
	}
	want := map[string][]string{
		current: {TypeSession, TypeCall},
		old:     {TypeRun, TypeCall, TypeDispatch, TypeSkip},
	}
	for path, wantTypes := range want {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if got := strings.Join(types[path], ","); got != strings.Join(wantTypes, ",") {
				t.Errorf("line types = %s, want %s", got, strings.Join(wantTypes, ","))
			}
		})
	}

	if _, strictErr := ReadShards(root); strictErr == nil || !strings.Contains(strictErr.Error(), "schema 1 is not 2") {
		t.Errorf("ReadShards error = %v, want the schema 1 shard refused", strictErr)
	}
}

// TestReadForCalls_MergesWhatReadShardsForCallsKeeps verifies the merged form
// of the older reader: the lines of every shard in walk order, a version 1
// shard's session line dropped as [ReadShardsForCalls] drops it, and a
// directory it refuses refused here too.
func TestReadForCalls_MergesWhatReadShardsForCallsKeeps(t *testing.T) {
	root := t.TempDir()
	writeShard(t, filepath.Join(root, "old"), "calls-old.jsonl",
		`{"schema":1,"type":"session","session":{"label":"dynamic","surface":"dynamic","mode":"default","capabilities":"full","transport":"in-memory","dispatch_observed":false}}`,
		`{"schema":1,"type":"dispatch","dispatch":{"trace_id":"t1","action":"issue.list","requests":2}}`,
	)
	writeShard(t, filepath.Join(root, "new"), "calls-new.jsonl",
		`{"schema":2,"type":"dispatch","dispatch":{"trace_id":"t2","action":"issue.get","requests":1}}`,
	)

	records, err := ReadForCalls(root)
	if err != nil {
		t.Fatalf("ReadForCalls error = %v, want both schemas read", err)
	}
	var actions []string
	for _, record := range records {
		if record.Type != TypeDispatch {
			t.Errorf("record type = %s, want only the dispatch lines", record.Type)
			continue
		}
		actions = append(actions, record.Dispatch.Action)
	}
	if got := strings.Join(actions, ","); got != "issue.get,issue.list" {
		t.Errorf("dispatched actions = %s, want both shards' in walk order", got)
	}

	empty := t.TempDir()
	if _, emptyErr := ReadForCalls(empty); emptyErr == nil {
		t.Error("ReadForCalls of a directory holding no shard succeeded, want the refusal ReadShardsForCalls gives")
	}
}

// TestReadShardsForCalls_LineItCannotRead_IsRefused verifies that the older
// reader widens the schema it accepts and nothing else: a schema outside the
// range is refused as a stale artifact, and an old line that does not hold
// together is refused as a current one would be.
func TestReadShardsForCalls_LineItCannotRead_IsRefused(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "a schema older than any it reads",
			line: `{"schema":0,"type":"skip","skip":{"test":"TestOld","reason":"r"}}`,
			want: "schema 0 is not between 1 and 2",
		},
		{
			name: "a schema newer than this package",
			line: `{"schema":3,"type":"skip","skip":{"test":"TestNew","reason":"r"}}`,
			want: "schema 3 is not between 1 and 2",
		},
		{
			name: "an old line carrying two payloads",
			line: `{"schema":1,"type":"skip","skip":{"test":"TestOld","reason":"r"},"call":{"test":"TestOld"}}`,
			want: "carries 2 payloads",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			writeShard(t, dir, "calls-x.jsonl", testCase.line)

			shardFiles, err := ReadShardsForCalls(dir)
			if err == nil {
				t.Fatalf("ReadShardsForCalls = %+v, want a refusal", shardFiles)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("ReadShardsForCalls error = %v, want it to say %q", err, testCase.want)
			}
		})
	}
}

// TestRead_RefusesADirectoryHoldingNoShard verifies that an empty directory is
// an error naming the switch, rather than an empty result.
//
// An empty result would be read as a suite that covered nothing, which is a
// claim about the server made from a fact about the harness: nothing was
// recorded because nothing asked for recording. The variable the refusal names
// is this package's, which is the half of the message it contributes.
func TestRead_RefusesADirectoryHoldingNoShard(t *testing.T) {
	records, err := Read(t.TempDir())

	if err == nil {
		t.Fatalf("Read = %d record(s), want an error", len(records))
	}
	if !strings.Contains(err.Error(), DirEnv) {
		t.Errorf("Read error = %v, want it to name %s", err, DirEnv)
	}
}

// TestShardPattern_MatchesWhatTheWriterNames verifies that the pattern a
// reader is told to look for is the one a writer's own file name satisfies,
// and that IsShard agrees with filepath.Match on it.
//
// The three are spelled apart on purpose: the pattern from its prefix and
// extension, the file name from the pattern the spec hands the mechanism, and
// the predicate from the same two constants. Nothing but a test holds them to
// each other.
func TestShardPattern_MatchesWhatTheWriterNames(t *testing.T) {
	if ShardPattern != "calls-*.jsonl" {
		t.Errorf("ShardPattern = %q, want calls-*.jsonl", ShardPattern)
	}

	dir := t.TempDir()
	Release()
	t.Cleanup(Release)
	OpenDir(dir).Write(&recordingReporter{}, &Skip{Test: "a", Reason: "b"})

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
	if !IsShard(name) {
		t.Errorf("IsShard(%q) = false, want true: it must agree with ShardPattern", name)
	}
}
