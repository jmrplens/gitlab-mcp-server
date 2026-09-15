package modelrecord

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordingReporter stands in for *testing.T so the writer's failure paths can
// be exercised without failing the test that asked for them.
type recordingReporter struct {
	messages []string
}

// Errorf records one reported failure.
func (r *recordingReporter) Errorf(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// joined returns every message as one string, for a substring assertion.
func (r *recordingReporter) joined() string {
	return strings.Join(r.messages, "\n")
}

// shardLines returns the non-empty lines of the one shard written into dir,
// failing when the number of shards is not one.
func shardLines(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	var shards []string
	for _, entry := range entries {
		if !entry.IsDir() && isShard(entry.Name()) {
			shards = append(shards, filepath.Join(dir, entry.Name()))
		}
	}
	if len(shards) != 1 {
		t.Fatalf("shards in %s = %d, want 1", dir, len(shards))
	}
	content, err := os.ReadFile(shards[0])
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	var lines []string
	for line := range strings.SplitSeq(string(content), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestOpen_RecordsOnlyWhenTheDirectoryIsNamed verifies that recording is off
// unless DirEnv names a directory, and that Open and OpenDir reach the same
// writer for one directory.
//
// Off by default is what lets the provider contract probe, which calls a model
// and never runs a case, leave nothing behind. A nil writer is how the runner
// learns that without a branch of its own.
func TestOpen_RecordsOnlyWhenTheDirectoryIsNamed(t *testing.T) {
	// The temporary directory is made first, so its removal is registered
	// before Release and therefore runs after it. A shard left open is a
	// directory Windows refuses to remove.
	dir := t.TempDir()
	t.Cleanup(Release)

	t.Setenv(DirEnv, "")
	if writer := Open(); writer != nil {
		t.Errorf("Open() = %v with %s unset, want nil", writer, DirEnv)
	}

	t.Setenv(DirEnv, dir)
	writer := Open()
	if writer == nil {
		t.Fatalf("Open() = nil with %s = %q, want a writer", DirEnv, dir)
	}
	if again := OpenDir(dir); again != writer {
		t.Errorf("OpenDir(%q) = %p, want the writer Open returned (%p)", dir, again, writer)
	}
}

// TestOpenDir_ReturnsOneWriterPerDirectory verifies that two directories get
// two writers, one directory gets one, and a blank name gets none.
//
// One writer per directory is what makes the seen set and the shard shared by
// every attempt of a process: a writer per call site would write one shard per
// call site and duplicate every line across them.
func TestOpenDir_ReturnsOneWriterPerDirectory(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	t.Cleanup(Release)

	if writer := OpenDir("   "); writer != nil {
		t.Errorf("OpenDir(blank) = %v, want nil", writer)
	}
	one := OpenDir(first)
	if one == nil {
		t.Fatal("OpenDir(first) = nil, want a writer")
	}
	if OpenDir(first) != one {
		t.Error("OpenDir(first) returned a second writer for one directory")
	}
	if OpenDir(second) == one {
		t.Error("OpenDir(second) returned the writer of the first directory")
	}
}

// TestWriterWrite_NilWriterWritesNothing verifies that writing to the nil
// writer is a no-op rather than a panic.
//
// That is the whole point of returning nil when recording is off: the runner
// records unconditionally and pays a nil check, instead of guarding every call
// site and eventually missing one in the middle of a paid run.
func TestWriterWrite_NilWriterWritesNothing(t *testing.T) {
	var writer *Writer
	reporter := &recordingReporter{}

	writer.Write(reporter, &Verify{Attempt: "a1", Name: "issue exists"})

	if len(reporter.messages) != 0 {
		t.Errorf("reported %v, want nothing from a nil writer", reporter.messages)
	}
}

// TestWriterWrite_WritesEachLineOnceAndDropsRepeats verifies that lines reach
// the shard in order, one per line, and that an identical line offered twice is
// written once.
//
// A repeat is ordinary here: a session line is offered by every attempt that
// used the session, and a flush that runs at the end of an attempt and again at
// the end of the run offers the same lines twice. Writing them all would make
// the shard several times larger than the run it describes.
func TestWriterWrite_WritesEachLineOnceAndDropsRepeats(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	writer := OpenDir(dir)
	reporter := &recordingReporter{}
	session := &Session{Label: "dynamic/default", Surface: "dynamic", ServedTools: 2}

	writer.Write(reporter, session, &Attempt{ID: "a1", Case: "MT-205", EndedBy: EndedCompleted})
	writer.Write(reporter, session)

	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
	lines := shardLines(t, dir)
	if len(lines) != 2 {
		t.Fatalf("shard lines = %d, want 2: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"type":"session"`) {
		t.Errorf("first line = %s, want the session line", lines[0])
	}
	if !strings.Contains(lines[1], `"case":"MT-205"`) {
		t.Errorf("second line = %s, want the attempt line", lines[1])
	}
}

// TestWriterWrite_KeepsTwoCallsThatDifferOnlyInPosition verifies that the same
// call made twice in one attempt is two lines.
//
// This is the reason every line type carries a position of its own. A model
// that sends one call, is refused for a missing parameter and sends the very
// same call again is paying an overhead the record has to show, and without the
// index the second line would be dropped as a duplicate of the first and the
// overhead column would read zero.
func TestWriterWrite_KeepsTwoCallsThatDifferOnlyInPosition(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	writer := OpenDir(dir)
	reporter := &recordingReporter{}
	first := &Call{Attempt: "a1", Turn: 2, Index: 1, Tool: "gitlab_execute_action", Outcome: OutcomeOK}
	second := *first
	second.Index = 2

	writer.Write(reporter, first, &second)

	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
	if lines := shardLines(t, dir); len(lines) != 2 {
		t.Errorf("shard lines = %d, want 2: two calls of one attempt are two lines", len(lines))
	}
}

// TestWriterWrite_RefusesARelativeDirectory verifies that a relative directory
// is reported once and nothing is written.
//
// A test binary runs in its own package directory, so a relative directory
// would leave a shard wherever the process happened to be and the merge would
// find none of them. Resolving it would produce that scattering silently, which
// is why it is refused instead.
func TestWriterWrite_RefusesARelativeDirectory(t *testing.T) {
	t.Cleanup(Release)

	writer := OpenDir(filepath.Join("dist", "modeleval"))
	reporter := &recordingReporter{}

	writer.Write(reporter, &Verify{Attempt: "a1", Name: "issue exists"})
	writer.Write(reporter, &Verify{Attempt: "a2", Name: "label exists"})

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), DirEnv) || !strings.Contains(reporter.joined(), "absolute") {
		t.Errorf("report = %q, want it to name %s and say the path must be absolute", reporter.joined(), DirEnv)
	}
}

// TestWriterWrite_ReportsAnUnopenableShardOnce verifies that a directory the
// writer cannot open a shard in fails loudly and exactly once.
//
// The constructor is stubbed rather than the filesystem made hostile because
// this suite is often run as root, where a directory stripped of write
// permission is still writable.
func TestWriterWrite_ReportsAnUnopenableShardOnce(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	original := createShard
	createShard = func(string) (*os.File, error) { return nil, errors.New("no room") }
	t.Cleanup(func() { createShard = original })

	writer := OpenDir(dir)
	reporter := &recordingReporter{}

	writer.Write(reporter, &Verify{Attempt: "a1", Name: "issue exists"})
	writer.Write(reporter, &Verify{Attempt: "a2", Name: "label exists"})

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), "no room") {
		t.Errorf("report = %q, want it to carry the cause", reporter.joined())
	}
}

// TestWriterWrite_ReportsAShardThatStopsAccepting verifies that a shard which
// no longer accepts writes is reported rather than silently dropping lines.
//
// A dropped line is indistinguishable from a call the model never made, and
// this record exists to be believed about exactly that.
func TestWriterWrite_ReportsAShardThatStopsAccepting(t *testing.T) {
	stubDir, dir := t.TempDir(), t.TempDir()
	t.Cleanup(Release)

	closed, err := os.CreateTemp(stubDir, ShardPattern)
	if err != nil {
		t.Fatalf("CreateTemp error = %v", err)
	}
	if closeErr := closed.Close(); closeErr != nil {
		t.Fatalf("Close error = %v", closeErr)
	}
	original := createShard
	createShard = func(string) (*os.File, error) { return closed, nil }
	t.Cleanup(func() { createShard = original })

	writer := OpenDir(dir)
	reporter := &recordingReporter{}

	writer.Write(reporter, &Verify{Attempt: "a1", Name: "issue exists"})

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), "could not write") {
		t.Errorf("report = %q, want it to say the shard could not be written", reporter.joined())
	}
}

// TestWriterWrite_ReportsALineItCannotEncode verifies that a line JSON cannot
// hold is reported and skipped, and that the lines after it are still written.
//
// A latency of NaN is the reachable case: a provider that answers in no
// measurable time, or a clock that goes backwards, is how a float arrives here
// unrepresentable. It is reported because a line dropped in silence is
// indistinguishable from a call the model never made; it costs that line alone
// because the alternative loses strictly more. The lines that would follow it
// are the rest of the attempts, their verify lines, and the run line written
// from an exit hook, which is the only thing that joins a shard to its commit,
// its instance and its tier: silence there makes everything already recorded
// unpublishable too.
func TestWriterWrite_ReportsALineItCannotEncode(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	writer := OpenDir(dir)
	reporter := &recordingReporter{}

	writer.Write(reporter, &Turn{Attempt: "a1", Index: 1, LatencyMS: math.NaN()})
	writer.Write(reporter, &Verify{Attempt: "a1", Name: "issue exists"})

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), "could not be encoded") {
		t.Errorf("report = %q, want it to say the line could not be encoded", reporter.joined())
	}
	lines := shardLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("shard lines = %d, want 1: the line after the unencodable one is still recorded: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"type":"verify"`) {
		t.Errorf("shard line = %s, want the verify line offered after the one that could not be encoded", lines[0])
	}
}

// TestWriterWrite_ReportsALineCarryingJSONThatDoesNotParse verifies that a raw
// field holding text no reader can parse is reported as that field, skipped,
// and does not silence the writer.
//
// This is the reachable one, not NaN. Block.Arguments, Call.Arguments and
// Call.Result are filled from a model's own output, and a model that stops
// mid-object emits exactly the fragment below. The report names the field and
// says where the text belongs instead, because the encoder's own message for
// this is about a type name and tells the author of a run nothing.
func TestWriterWrite_ReportsALineCarryingJSONThatDoesNotParse(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	writer := OpenDir(dir)
	reporter := &recordingReporter{}

	writer.Write(reporter, &Turn{
		Attempt: "a1", Index: 3, Try: 1, Status: TurnOK,
		Blocks: []Block{{
			Kind:      BlockToolCall,
			Tool:      "gitlab_execute_action",
			Arguments: json.RawMessage(`{"action": "issue.list", "params": {`),
		}},
	})
	writer.Write(reporter, &Verify{Attempt: "a1", Name: "issue exists"})

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), "block 1") || !strings.Contains(reporter.joined(), "raw field") {
		t.Errorf("report = %q, want it to name the block and where the text belongs", reporter.joined())
	}
	lines := shardLines(t, dir)
	if len(lines) != 1 || !strings.Contains(lines[0], `"type":"verify"`) {
		t.Errorf("shard lines = %v, want only the verify line offered after the malformed turn", lines)
	}
}

// TestWriterWrite_RefusesALineTheReaderCouldNotRead verifies that the writer
// accepts the longest line the reader's scanner takes and refuses one byte
// more, reporting it.
//
// The two halves are bounded by one constant and nothing else held them to each
// other: the writer used to write whatever it was given, and three of the values
// a line carries are not capped at all, so one rendered answer of a megabyte
// made a shard the reader could not scan. That loss arrives at read time, months
// later, and it is not one line but every attempt the process paid for, which is
// why the refusal happens here, in front of the run that could still be fixed.
func TestWriterWrite_RefusesALineTheReaderCouldNotRead(t *testing.T) {
	// Text is padded to bring the encoded line to exactly the length the
	// scanner accepts: one byte less than the cap, since the newline the writer
	// adds counts against it too.
	call := &Call{Attempt: "a1", Turn: 1, Index: 1, Tool: "gitlab_execute_action", Outcome: OutcomeOK, Text: "x"}
	probe, err := json.Marshal(call.record())
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	longest := *call
	longest.Text = strings.Repeat("x", maxShardLine-1-(len(probe)-1))
	encoded, err := json.Marshal(longest.record())
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	if len(encoded) != maxShardLine-1 {
		t.Fatalf("the padded line is %d bytes, want %d: every x escapes to one byte, so the padding is exact", len(encoded), maxShardLine-1)
	}
	tooLong := longest
	tooLong.Text += "x"

	t.Run("the longest line the reader takes", func(t *testing.T) {
		dir := t.TempDir()
		t.Cleanup(Release)
		reporter := &recordingReporter{}

		OpenDir(dir).Write(reporter, &longest)

		if len(reporter.messages) != 0 {
			t.Fatalf("reported %v, want nothing", reporter.messages)
		}
		records, readErr := Read(dir)
		if readErr != nil {
			t.Fatalf("Read error = %v: the writer wrote a line its own reader refuses", readErr)
		}
		if len(records) != 1 || records[0].Call == nil {
			t.Fatalf("records = %d, want the one call line", len(records))
		}
	})

	t.Run("one byte more", func(t *testing.T) {
		dir := t.TempDir()
		t.Cleanup(Release)
		reporter := &recordingReporter{}

		OpenDir(dir).Write(reporter, &tooLong, &Verify{Attempt: "a1", Name: "issue exists"})

		if len(reporter.messages) != 1 {
			t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
		}
		if !strings.Contains(reporter.joined(), "line cap") {
			t.Errorf("report = %q, want it to name the cap it is past", reporter.joined())
		}
		// The size is asserted because it is what a person acts on: the report
		// is the only account of a line that was not recorded, and one carrying
		// a number that is not the line's length sends them looking at the
		// wrong value.
		if want := fmt.Sprintf("%d bytes", maxShardLine+1); !strings.Contains(reporter.joined(), want) {
			t.Errorf("report = %q, want it to say the line is %s", reporter.joined(), want)
		}
		lines := shardLines(t, dir)
		if len(lines) != 1 || !strings.Contains(lines[0], `"type":"verify"`) {
			t.Errorf("shard lines = %v, want only the verify line offered after the one that was too long", lines)
		}
	})
}

// TestRelease_ClosesTheShardAndForgetsTheWriter verifies that Release lets a
// later Open of the same directory start a new shard.
//
// Windows is why it exists: a directory holding an open file cannot be removed,
// so a test recording into t.TempDir() would fail in cleanup after every one of
// its own assertions had passed.
func TestRelease_ClosesTheShardAndForgetsTheWriter(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	reporter := &recordingReporter{}
	first := OpenDir(dir)
	first.Write(reporter, &Verify{Attempt: "a1", Name: "issue exists"})

	Release()

	second := OpenDir(dir)
	if second == first {
		t.Fatal("OpenDir returned the released writer, want a new one")
	}
	second.Write(reporter, &Verify{Attempt: "a2", Name: "label exists"})

	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("shards = %d, want 2: a released writer writes a shard of its own", len(entries))
	}
}

// TestRelease_LetsTheSameWriterStartAnotherShard verifies that release lets go
// of the file rather than only of the registry entry: a writer that kept
// writing after Release opens a shard of its own instead of appending to the
// one it was told to let go of.
//
// The sibling above releases and then opens the directory again, which gets a
// new writer from the registry and would pass whether or not the old file was
// ever closed. Writing through the same handle is what asks the question,
// because only a closed file makes the writer open another.
func TestRelease_LetsTheSameWriterStartAnotherShard(t *testing.T) {
	dir := t.TempDir()
	Release()
	t.Cleanup(Release)

	reporter := &recordingReporter{}
	writer := OpenDir(dir)
	// The second shard below belongs to a writer the registry has already let
	// go of, so the package-level Release cannot close it; this test closes it
	// itself, or Windows refuses to remove the directory holding it.
	t.Cleanup(writer.release)
	writer.Write(reporter, &Verify{Attempt: "a1", Name: "issue exists"})

	Release()
	writer.Write(reporter, &Verify{Attempt: "a2", Name: "label exists"})

	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("shards = %d, want 2: a released writer holds no file and opens another", len(entries))
	}
}

// TestCreateShard_ReportsADirectoryItCannotMake verifies that the shard
// constructor fails when the directory cannot be created.
//
// It is called directly because the writer stubs it everywhere else, so this is
// the only place the real one's failure branch is reached.
func TestCreateShard_ReportsADirectoryItCannotMake(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	file, err := createShard(filepath.Join(blocked, "shards"))

	if err == nil {
		_ = file.Close()
		t.Error("createShard succeeded under a regular file, want an error")
	}
}
