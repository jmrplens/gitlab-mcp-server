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
		`{"schema":1,"type":"skip","skip":{"test":"TestEE_Epics","reason":"no license"}}`,
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

// TestRead_RefusesADirectoryHoldingNoShard verifies that an empty directory is
// an error naming the switch, rather than an empty result.
//
// An empty result would be read as a suite that covered nothing, which is a
// claim about the server made from a fact about the harness: nothing was
// recorded because nothing asked for recording.
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
		name    string
		lines   []string
		wantErr string
	}{
		{
			name:    "not json",
			lines:   []string{`{"schema":1,"type":"skip","skip":{"test":"a","reason":"b"}}`, "{"},
			wantErr: "parse",
		},
		{
			name:    "type without its payload",
			lines:   []string{`{"schema":1,"type":"call"}`},
			wantErr: "carries no payload",
		},
		{
			name:    "another schema",
			lines:   []string{`{"schema":99,"type":"skip","skip":{"test":"a","reason":"b"}}`},
			wantErr: "schema",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeShard(t, dir, "calls-broken.jsonl", testCase.lines...)

			_, err := Read(dir)

			if err == nil {
				t.Fatalf("Read error = nil, want one mentioning %q", testCase.wantErr)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("Read error = %v, want it to mention %q", err, testCase.wantErr)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("Read error = %v, want it to name %s", err, path)
			}
		})
	}
}

// TestRead_NamesTheLineTheBadRecordIsOn verifies that the error counts lines
// from one and names the line the bad record is actually on, with a good
// record and a blank line ahead of it.
//
// The number is the whole value of the message: a shard is one line per call
// and can hold thousands, so an error that names the file and not the line
// says only that the run is unreadable. A blank line is skipped rather than
// counted, because the writer ends every line with a newline and the last one
// therefore reads as empty.
func TestRead_NamesTheLineTheBadRecordIsOn(t *testing.T) {
	dir := t.TempDir()
	writeShard(t, dir, "calls-numbered.jsonl",
		`{"schema":1,"type":"skip","skip":{"test":"a","reason":"b"}}`,
		"",
		`{"schema":1,"type":"skip"}`,
	)

	_, err := Read(dir)

	if err == nil {
		t.Fatal("Read error = nil, want one naming the third line")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("Read error = %v, want it to name line 3", err)
	}
}

// TestShardPattern_MatchesWhatTheWriterNames verifies that the pattern a
// reader is told to look for is the one a writer's own file name satisfies,
// and that isShard agrees with filepath.Match on it.
//
// The two are spelled apart on purpose, the pattern from its prefix and
// extension and the predicate from the same two constants, so nothing but a
// test holds them to each other.
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
	if !isShard(name) {
		t.Errorf("isShard(%q) = false, want true: it must agree with ShardPattern", name)
	}
}

// TestReadShard_ReportsAFileItCannotOpen verifies that a shard that cannot be
// opened is reported.
//
// It is called directly because a walk only offers files it has just listed,
// so this branch is unreachable through Read on any tree a test can build.
func TestReadShard_ReportsAFileItCannotOpen(t *testing.T) {
	_, err := readShard(filepath.Join(t.TempDir(), "calls-missing.jsonl"))

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
// A line silently dropped for being long looks exactly like a call nobody
// made, and a coverage record that can lose calls quietly is not one anybody
// should act on.
func TestRead_RefusesALineBeyondTheCap(t *testing.T) {
	dir := t.TempDir()
	writeShard(t, dir, "calls-long.jsonl", strings.Repeat("x", maxShardLine+1))

	_, err := Read(dir)

	if err == nil {
		t.Fatal("Read error = nil, want an error for a line beyond the cap")
	}
	if !strings.Contains(err.Error(), "calls-long.jsonl") {
		t.Errorf("Read error = %v, want it to name the shard", err)
	}
}
