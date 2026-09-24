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
