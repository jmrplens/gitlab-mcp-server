package shardio

import (
	"encoding/json"
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
// the writer wrote, from the directory and from its subdirectories, and ignores
// everything that is not a shard.
//
// Subdirectories are read because one run per target records into a directory
// of its own and what a reader wants is the comparison between them. Files that
// are not shards are ignored because the same directory carries whatever else
// the run left there.
func TestRead_MergesEveryShardUnderTheDirectory(t *testing.T) {
	root := t.TempDir()
	shards := newFixture(t, plainSpec())

	reporter := &recordingReporter{}
	shards.OpenDir(root).Write(reporter, &note{Text: "written"}, &measure{Value: 2})
	wantNoMessage(t, reporter)

	writeShard(t, filepath.Join(root, "nested"), "fixture-nested.jsonl",
		`{"schema":1,"type":"note","note":{"text":"nested"}}`,
		"",
	)
	writeShard(t, root, "notes.txt", "not a shard")
	writeShard(t, root, "fixture-report.txt", "not a shard either")

	records, err := shards.Read(root)
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
	for _, lineType := range []string{typeNote, typeMeasure} {
		t.Run(lineType, func(t *testing.T) {
			want := map[string]int{typeNote: 2, typeMeasure: 1}[lineType]
			if byType[lineType] != want {
				t.Errorf("%s lines = %d, want %d", lineType, byType[lineType], want)
			}
		})
	}
}

// TestReadShards_KeepsTheFileBoundaries verifies that ReadShards hands back one
// entry per shard file, each with its path and its own lines in file order, so
// a reader can join every line to the run line of the process that wrote it.
// Read is the same walk with the boundaries dropped, and the two must agree on
// what was read.
func TestReadShards_KeepsTheFileBoundaries(t *testing.T) {
	root := t.TempDir()
	shards := New(plainSpec())

	first := writeShard(t, root, "fixture-first.jsonl",
		`{"schema":1,"type":"note","note":{"text":"one"}}`,
		`{"schema":1,"type":"measure","measure":{"value":1}}`,
	)
	second := writeShard(t, filepath.Join(root, "nested"), "fixture-second.jsonl",
		`{"schema":1,"type":"note","note":{"text":"two"}}`,
	)

	read, err := shards.ReadShards(root)
	if err != nil {
		t.Fatalf("ReadShards error = %v", err)
	}
	if len(read) != 2 {
		t.Fatalf("shards = %d, want 2: %+v", len(read), read)
	}
	if read[0].Path != first || read[1].Path != second {
		t.Errorf("paths = %q, %q; want %q then %q in walk order", read[0].Path, read[1].Path, first, second)
	}
	if len(read[0].Records) != 2 || read[0].Records[0].Type != typeNote || read[0].Records[1].Type != typeMeasure {
		t.Errorf("first shard = %+v, want its note then its measure line", read[0].Records)
	}
	if len(read[1].Records) != 1 || read[1].Records[0].Note == nil || read[1].Records[0].Note.Text != "two" {
		t.Errorf("second shard = %+v, want the one note line it holds", read[1].Records)
	}

	merged, err := shards.Read(root)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if len(merged) != 3 {
		t.Errorf("Read = %d records, want the 3 the shards hold together", len(merged))
	}
}

// TestRead_RefusesADirectoryHoldingNoShard verifies that a directory with no
// shard in it is an error naming the pattern and the spec's variable, rather
// than an empty result.
//
// An empty result would be read as a run that did nothing, which is a claim
// about the thing under test made from a fact about an environment variable:
// nothing was recorded because nothing asked for recording.
func TestRead_RefusesADirectoryHoldingNoShard(t *testing.T) {
	shards := New(plainSpec())

	records, err := shards.Read(t.TempDir())

	if err == nil {
		t.Fatalf("Read = %d record(s), want an error", len(records))
	}
	for _, fragment := range []string{shards.pattern(), fixtureDirEnv} {
		t.Run(fragment, func(t *testing.T) {
			if !strings.Contains(err.Error(), fragment) {
				t.Errorf("Read error = %v, want it to name %s", err, fragment)
			}
		})
	}
}

// TestRead_ReportsADirectoryItCannotWalk verifies that a directory that is not
// there is reported with its path.
func TestRead_ReportsADirectoryItCannotWalk(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never-created")
	shards := New(plainSpec())

	_, err := shards.Read(missing)

	if err == nil {
		t.Fatal("Read error = nil, want an error for a missing directory")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("Read error = %v, want it to name %s", err, missing)
	}
}

// TestRead_ReportsALineItCannotRead verifies that a shard line which is not
// JSON, or which is JSON the spec's own check does not accept, is reported with
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
			lines:   []string{`{"schema":1,"type":"note","note":{"text":"a"}}`, "{"},
			wantErr: "parse",
		},
		{
			name:    "type without its payload",
			lines:   []string{`{"schema":1,"type":"note"}`},
			wantErr: "carries no payload",
		},
		{
			name:    "another schema",
			lines:   []string{`{"schema":99,"type":"note","note":{"text":"a"}}`},
			wantErr: "schema",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			shards := New(plainSpec())
			path := writeShard(t, dir, "fixture-broken.jsonl", testCase.lines...)

			_, err := shards.Read(dir)

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

// TestRead_NamesTheLineTheBadRecordIsOn verifies that both refusals count lines
// from one and name the line the bad record is actually on, with a good record
// and a blank line ahead of it.
//
// The number is the whole value of the message: a shard is one line per event
// and can hold thousands, so an error that names the file and not the line says
// only that the run is unreadable. A blank line is skipped rather than counted,
// because the writer ends every line with a newline and the last one therefore
// reads as empty, which is exactly what makes a counter of records rather than
// of lines read correctly on a shard with nothing skipped and send a reader to
// the wrong line on every other one. Both branches are driven because they hold
// the same number and were not written together.
func TestRead_NamesTheLineTheBadRecordIsOn(t *testing.T) {
	cases := []struct {
		name string
		bad  string
	}{
		{name: "a line that is not json", bad: "{"},
		{name: "a line the spec refuses", bad: `{"schema":1,"type":"note"}`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			shards := New(plainSpec())
			writeShard(t, dir, "fixture-numbered.jsonl",
				`{"schema":1,"type":"note","note":{"text":"a"}}`,
				"",
				testCase.bad,
			)

			_, err := shards.Read(dir)

			if err == nil {
				t.Fatal("Read error = nil, want one naming the third line")
			}
			if !strings.Contains(err.Error(), "line 3") {
				t.Errorf("Read error = %v, want it to name line 3", err)
			}
		})
	}
}

// TestReadShards_ReadsPastADirectoryNamedLikeAShard verifies that the walk
// judges a directory by being one, so a directory whose name matches the
// record's pattern is descended into rather than read as a file, and the shard
// inside it is read.
//
// The name rule and the directory rule look like one filter and are two. A
// record whose shards are written one subdirectory per target names those
// directories after the same run the files are named after, so this is a shape
// the tree produces rather than one invented for the test: read as a file, the
// directory takes the whole walk down with an error about a shard nobody wrote.
func TestReadShards_ReadsPastADirectoryNamedLikeAShard(t *testing.T) {
	root := t.TempDir()
	shards := New(plainSpec())

	beside := writeShard(t, root, "fixture-beside.jsonl",
		`{"schema":1,"type":"note","note":{"text":"beside"}}`,
	)
	// The directory itself carries a shard name, and holds a shard of its own.
	inside := writeShard(t, filepath.Join(root, "fixture-a-directory.jsonl"), "fixture-inside.jsonl",
		`{"schema":1,"type":"note","note":{"text":"inside"}}`,
	)

	read, err := shards.ReadShards(root)
	if err != nil {
		t.Fatalf("ReadShards error = %v, want the two shard files under the root", err)
	}
	if len(read) != 2 {
		t.Fatalf("shards = %d, want the 2 files: %+v", len(read), read)
	}
	// The walk visits a directory's contents before the entries that sort after
	// it, and "fixture-a-directory.jsonl" sorts ahead of "fixture-beside.jsonl".
	if read[0].Path != inside || read[1].Path != beside {
		t.Errorf("paths = %q, %q; want %q then %q, and no directory among them", read[0].Path, read[1].Path, inside, beside)
	}
}

// TestReadShard_ReportsAFileItCannotOpen verifies that a shard that cannot be
// opened is reported.
//
// It is called directly because a walk only offers files it has just listed, so
// this branch is unreachable through Read on any tree a test can build.
func TestReadShard_ReportsAFileItCannotOpen(t *testing.T) {
	shards := New(plainSpec())

	_, err := shards.readShard(filepath.Join(t.TempDir(), "fixture-missing.jsonl"))

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
// A line silently dropped for being long looks exactly like something that
// never happened, and a record that can lose events quietly is not one anybody
// should act on. The writer refuses to produce one, so a shard holding it was
// written by hand or by another version of this package.
func TestRead_RefusesALineBeyondTheCap(t *testing.T) {
	dir := t.TempDir()
	shards := New(plainSpec())
	writeShard(t, dir, "fixture-long.jsonl", strings.Repeat("x", MaxLine+1))

	_, err := shards.Read(dir)

	if err == nil {
		t.Fatal("Read error = nil, want an error for a line beyond the cap")
	}
	if !strings.Contains(err.Error(), "fixture-long.jsonl") {
		t.Errorf("Read error = %v, want it to name the shard", err)
	}
}

// TestRead_TakesTheLongestLineTheWriterWrites verifies that the longest line
// the writer accepts is one the reader reads back.
//
// The two halves read one constant from opposite sides, and nothing above
// holds them to each other at the value where they meet: the refusal test
// drives a longer line and every other test a much shorter one. The writer
// refuses a line whose encoded text plus its newline is past MaxLine, so the
// longest it writes encodes to MaxLine-1 bytes and occupies exactly MaxLine in
// the file, which is what the scanner's maximum buffer must hold, newline
// included. An off-by-one either way would leave this package unable to read a
// shard it wrote itself, and it would show up on a long answer in a real run
// rather than here.
func TestRead_TakesTheLongestLineTheWriterWrites(t *testing.T) {
	dir := t.TempDir()
	shards := newFixture(t, plainSpec())

	// Measured rather than assumed: the envelope's own bytes are whatever the
	// fixture record encodes to, and the text field carries no escapes, so one
	// more byte of text is one more byte of line.
	empty, err := json.Marshal((&note{}).record())
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	longest := &note{Text: strings.Repeat("a", MaxLine-1-len(empty))}
	encoded, err := json.Marshal(longest.record())
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	if len(encoded) != MaxLine-1 {
		t.Fatalf("the fixture line encodes to %d bytes, want %d", len(encoded), MaxLine-1)
	}

	reporter := &recordingReporter{}
	shards.OpenDir(dir).Write(reporter, longest)
	wantNoMessage(t, reporter)

	records, err := shards.Read(dir)
	if err != nil {
		t.Fatalf("Read error = %v, want the line the writer accepted", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Note == nil || records[0].Note.Text != longest.Text {
		t.Error("Read returned a record that is not the line that was written")
	}
}
