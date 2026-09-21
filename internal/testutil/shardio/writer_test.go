package shardio

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// recordingReporter stands in for *testing.T so the writer's failure paths can
// be exercised without failing the test that asked for them.
//
// Errorf is guarded because one writer is shared by every caller in a process
// and a reporter is taken per call, so a concurrent test hands the same
// reporter to several goroutines. The messages themselves are read only after
// those goroutines have been joined.
type recordingReporter struct {
	mu       sync.Mutex
	messages []string
}

// Errorf records one reported failure.
func (r *recordingReporter) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// joined returns every message as one string, for a substring assertion.
func (r *recordingReporter) joined() string {
	return strings.Join(r.messages, "\n")
}

// shardLines returns the non-empty lines of the one shard written into dir,
// failing when the number of shards is not one.
func shardLines(t *testing.T, shards *Shards[fixtureRecord, fixtureLine], dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && shards.IsShard(entry.Name()) {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	if len(files) != 1 {
		t.Fatalf("shards in %s = %d, want 1", dir, len(files))
	}
	content, err := os.ReadFile(files[0])
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

// TestShardsOpen_RecordsOnlyWhenTheDirectoryIsNamed verifies that recording is
// off unless the spec's variable names a directory, and that Open and OpenDir
// reach the same writer for one directory.
//
// Off by default is what lets an ordinary run pay nothing for a record, and a
// nil writer is how a caller learns that without a branch of its own.
func TestShardsOpen_RecordsOnlyWhenTheDirectoryIsNamed(t *testing.T) {
	dir := t.TempDir()
	shards := newFixture(t, plainSpec())

	t.Setenv(fixtureDirEnv, "")
	if writer := shards.Open(); writer != nil {
		t.Errorf("Open() = %v with %s unset, want nil", writer, fixtureDirEnv)
	}

	t.Setenv(fixtureDirEnv, dir)
	writer := shards.Open()
	if writer == nil {
		t.Fatalf("Open() = nil with %s = %q, want a writer", fixtureDirEnv, dir)
	}
	if again := shards.OpenDir(dir); again != writer {
		t.Errorf("OpenDir(%q) = %p, want the writer Open returned (%p)", dir, again, writer)
	}
}

// TestShardsOpenDir_ReturnsOneWriterPerDirectory verifies that two directories
// get two writers, one directory gets one, and a blank name gets none.
//
// One writer per directory is what makes the seen set and the shard shared by
// every caller of a process: a writer per call site would write one shard each
// and duplicate every line across them.
func TestShardsOpenDir_ReturnsOneWriterPerDirectory(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	shards := newFixture(t, plainSpec())

	if writer := shards.OpenDir("   "); writer != nil {
		t.Errorf("OpenDir(blank) = %v, want nil", writer)
	}
	one := shards.OpenDir(first)
	if one == nil {
		t.Fatal("OpenDir(first) = nil, want a writer")
	}
	if shards.OpenDir(first) != one {
		t.Error("OpenDir(first) returned a second writer for one directory")
	}
	if shards.OpenDir(second) == one {
		t.Error("OpenDir(second) returned the writer of the first directory")
	}
}

// TestShardsOpenDir_OneDirectorySpelledTwoWaysIsOneWriter verifies that the
// registry key is the cleaned path, so two spellings of one directory share a
// writer.
//
// Two writers for one directory is not a tidiness problem. Each opens its own
// shard and keeps its own seen set, so the process is split across two files
// and a line already deduplicated against one of them is written again into
// the other, which is exactly the double counting the seen set exists to
// prevent. The spellings below are what a Makefile and an environment variable
// actually produce between them, the padded one included: an exported value
// carries whatever whitespace the line that set it had, and trimming it only to
// decide whether recording is on would leave the padding in the key and in the
// directory, where it fails the absolute-path rule and records nothing.
func TestShardsOpenDir_OneDirectorySpelledTwoWaysIsOneWriter(t *testing.T) {
	dir := t.TempDir()
	shards := newFixture(t, plainSpec())

	canonical := shards.OpenDir(dir)
	if canonical == nil {
		t.Fatal("OpenDir(dir) = nil, want a writer")
	}
	for _, spelling := range []string{dir + "/.", dir + "/", dir + "/sub/..", " " + dir + " "} {
		t.Run(spelling, func(t *testing.T) {
			if got := shards.OpenDir(spelling); got != canonical {
				t.Errorf("OpenDir(%q) returned a second writer for one directory", spelling)
			}
		})
	}

	// The writer the padded spelling hands back records into the directory that
	// was meant, which is what the identity check above cannot say on its own:
	// a registry that trimmed the key and kept the padding in the writer's own
	// directory would pass it and then refuse every line.
	reporter := &recordingReporter{}
	shards.OpenDir(" "+dir+" ").Write(reporter, &note{Text: "recorded through a padded spelling"})

	wantNoMessage(t, reporter)
	if lines := shardLines(t, shards, dir); len(lines) != 1 {
		t.Errorf("shard lines = %d, want the one line written through the padded spelling: %v", len(lines), lines)
	}
}

// TestWriterWrite_NilWriterWritesNothing verifies that writing to the nil
// writer is a no-op rather than a panic.
//
// That is the whole point of returning nil when recording is off: a caller
// records unconditionally and pays a nil check, instead of guarding every call
// site and eventually missing one. It is also why Writer is handed to a record
// package as a type alias: a wrapper struct around it would need a nil check of
// its own, and the day somebody forgot it the panic would be in the harness.
func TestWriterWrite_NilWriterWritesNothing(t *testing.T) {
	var writer *Writer[fixtureRecord, fixtureLine]
	reporter := &recordingReporter{}

	writer.Write(reporter, &note{Text: "nothing to record"})

	wantNoMessage(t, reporter)
}

// TestWriterWrite_WritesEachLineOnceAndDropsRepeats verifies that lines reach
// the shard in order, one per line, and that an identical line offered twice is
// written once.
//
// Repeats are ordinary: a line is offered again by a retry, by a second caller
// of one session, or by a flush that runs both when a test ends and when the
// run does. The seen set holds the digest of the encoded bytes rather than the
// bytes, which is the same equality relation and thirty-two bytes a line, so a
// long run does not hold its whole shard in memory beside the file it was
// already written to.
//
// Two notes that differ only in their text are what says the digest is taken
// over the encoded record and not over something coarser: a set keyed on the
// line type absorbs a repeat exactly as this one does, and silently drops the
// second of every two lines of a kind, which is the one failure this record is
// built not to have and which no run would ever report.
func TestWriterWrite_WritesEachLineOnceAndDropsRepeats(t *testing.T) {
	dir := t.TempDir()
	shards := newFixture(t, plainSpec())
	reporter := &recordingReporter{}
	line := &note{Text: "offered twice"}

	shards.OpenDir(dir).Write(reporter, &measure{Value: 1.5}, line, &note{Text: "a note of its own"})
	shards.OpenDir(dir).Write(reporter, line)

	wantNoMessage(t, reporter)
	lines := shardLines(t, shards, dir)
	if len(lines) != 3 {
		t.Fatalf("shard lines = %d, want 3: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"type":"measure"`) {
		t.Errorf("first line = %s, want the measure line", lines[0])
	}
	if !strings.Contains(lines[1], `"text":"offered twice"`) {
		t.Errorf("second line = %s, want the note line offered twice", lines[1])
	}
	if !strings.Contains(lines[2], `"text":"a note of its own"`) {
		t.Errorf("third line = %s, want the second note, which differs from the first only in its text", lines[2])
	}
}

// TestWriterWrite_ConcurrentCallersShareOneShardAndLoseNoLine verifies that
// callers writing at once through the registry produce one shard holding every
// line each of them offered, exactly once.
//
// This is the shape the package is used in: one writer is process-global and
// taken by whichever caller records next, so the registry's lock and the
// writer's are the only thing keeping two lines from interleaving or a seen set
// from being read while it is written. A lock is invisible to both coverage
// gates, which read operators and operands, so one that stopped being taken
// would leave them green and corrupt a record only on the runs busy enough to
// matter. Driving the writer from several goroutines is what lets -race say so.
func TestWriterWrite_ConcurrentCallersShareOneShardAndLoseNoLine(t *testing.T) {
	const (
		callers        = 8
		linesPerCaller = 16
	)
	dir := t.TempDir()
	shards := newFixture(t, plainSpec())
	reporter := &recordingReporter{}

	var writing sync.WaitGroup
	for caller := range callers {
		writing.Go(func() {
			// Opened per goroutine rather than once on the parent, so the
			// registry is exercised concurrently too: every caller must be
			// handed the one writer of this directory.
			writer := shards.OpenDir(dir)
			for line := range linesPerCaller {
				writer.Write(reporter, &note{Text: fmt.Sprintf("caller %d line %d", caller, line)})
			}
		})
	}
	writing.Wait()

	wantNoMessage(t, reporter)
	records, err := shards.Read(dir)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if len(records) != callers*linesPerCaller {
		t.Fatalf("records = %d, want %d: every line each caller offered, and no line twice", len(records), callers*linesPerCaller)
	}
	texts := map[string]int{}
	for _, record := range records {
		if record.Note == nil {
			t.Fatalf("record = %+v, want a note line", record)
		}
		texts[record.Note.Text]++
	}
	if len(texts) != callers*linesPerCaller {
		t.Errorf("distinct lines = %d, want %d: a shard holding one line twice has lost another", len(texts), callers*linesPerCaller)
	}
}

// TestWriterWrite_RefusesARelativeDirectory verifies that a relative directory
// is reported once, naming the spec's variable, and that nothing is written
// afterwards.
//
// A test binary runs in its own package directory, so a relative directory
// would leave one shard under each package and the merge would find none of
// them. Resolving it would produce that scattering silently, which is why it is
// refused instead. It stops the writer rather than costing one line, because no
// later line can avoid it.
//
// The variable and the path it held are asserted apart, the path in the quoted
// form the refusal writes it in. They are a pair nothing else here can tell
// apart: crossed, the message says the directory must be absolute and quotes
// the variable's name as the value, and it still carries the variable's name
// and the word absolute that a laxer assertion looks for.
func TestWriterWrite_RefusesARelativeDirectory(t *testing.T) {
	relative := filepath.Join("dist", "fixtures")
	shards := newFixture(t, plainSpec())
	writer := shards.OpenDir(relative)
	reporter := &recordingReporter{}

	writer.Write(reporter, &note{Text: "first"})
	writer.Write(reporter, &note{Text: "second"})

	wantMessage(t, reporter, fixtureDirEnv, "absolute", fmt.Sprintf("%q", relative))
}

// TestWriterWrite_ReportsAnUnopenableShardOnce verifies that a directory the
// writer cannot open a shard in fails loudly and exactly once, naming the
// record.
//
// The constructor is stubbed rather than the filesystem made hostile because
// this suite is often run as root, where a directory stripped of write
// permission is still writable. The stub is on this test's own registry, so it
// is never another test's.
//
// The directory is asserted beside the record's name because the two are a pair
// a fixture cannot tell apart on its own: the message is the only account of a
// shard that was never opened, and one naming some other string of the spec
// reads exactly as well while sending a reader to a directory nobody wrote to.
func TestWriterWrite_ReportsAnUnopenableShardOnce(t *testing.T) {
	dir := t.TempDir()
	shards := newFixture(t, plainSpec())
	shards.createShard = func(string, string) (*os.File, error) { return nil, errors.New("no room") }

	writer := shards.OpenDir(dir)
	reporter := &recordingReporter{}

	writer.Write(reporter, &note{Text: "first"})
	writer.Write(reporter, &note{Text: "second"})

	wantMessage(t, reporter, "no room", "a fixture shard", dir)
}

// TestWriterWrite_ReportsAShardThatStopsAccepting verifies that a shard which
// no longer accepts writes is reported rather than silently dropping lines.
//
// A dropped line is indistinguishable from something that never happened, which
// is the one reading of these records that must never be wrong.
func TestWriterWrite_ReportsAShardThatStopsAccepting(t *testing.T) {
	stubDir, dir := t.TempDir(), t.TempDir()
	shards := newFixture(t, plainSpec())

	closed, err := os.CreateTemp(stubDir, shards.pattern())
	if err != nil {
		t.Fatalf("CreateTemp error = %v", err)
	}
	if closeErr := closed.Close(); closeErr != nil {
		t.Fatalf("Close error = %v", closeErr)
	}
	shards.createShard = func(string, string) (*os.File, error) { return closed, nil }

	writer := shards.OpenDir(dir)
	reporter := &recordingReporter{}

	writer.Write(reporter, &note{Text: "first"})

	wantMessage(t, reporter, "could not write a fixture shard")
}

// TestWriterWrite_ReportsALineItCannotEncode verifies that a line JSON cannot
// hold is reported and skipped, naming the line type, and that the line offered
// after it is still written.
//
// A float of NaN is the reachable case: it is the one ordinary Go value with no
// JSON spelling. It is reported because a line dropped in silence is
// indistinguishable from something that never happened; it costs that line
// alone because the alternative loses strictly more. The lines that would
// follow it are the rest of the run, and the run line written from an exit
// hook, which is the only thing that joins a shard to its commit, its instance
// and its tier: silence there makes everything already recorded unpublishable
// too.
func TestWriterWrite_ReportsALineItCannotEncode(t *testing.T) {
	dir := t.TempDir()
	shards := newFixture(t, plainSpec())
	reporter := &recordingReporter{}

	writer := shards.OpenDir(dir)
	writer.Write(reporter, &measure{Value: math.NaN()})
	writer.Write(reporter, &note{Text: "written anyway"})

	wantMessage(t, reporter, "a measure line was not recorded", "could not be encoded")
	lines := shardLines(t, shards, dir)
	if len(lines) != 1 {
		t.Fatalf("shard lines = %d, want 1: the line after the unencodable one is still recorded: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"text":"written anyway"`) {
		t.Errorf("shard line = %s, want the note line offered after the one that could not be encoded", lines[0])
	}
}

// TestWriterWrite_AsksTheSpecBeforeItEncodes verifies that a record the spec's
// own check refuses is reported as the field it names, with the hint the spec
// carries, and costs that line alone; and that the same line offered under a
// spec with no check is refused by the encoder instead, in the terms that makes
// it the check's whole reason for existing.
//
// The check exists for a record filled from somebody else's output, where a
// value that does not parse is ordinary rather than a defect. encoding/json
// refuses a raw message it cannot parse, so the line is lost either way; what
// differs is the account of it. The encoder names a Go type and an offset,
// which tells the author of a run nothing about which value to look at, while
// the check names the field and where the text belongs instead. Nothing here
// looks inside a raw field on its own: a spec that asks for no check pays for
// none, which is the other arm this takes.
func TestWriterWrite_AsksTheSpecBeforeItEncodes(t *testing.T) {
	malformed := &carrier{Raw: json.RawMessage(`{"action": "issue.list", "params": {`)}

	t.Run("a spec that checks", func(t *testing.T) {
		dir := t.TempDir()
		shards := newFixture(t, checkedSpec())
		reporter := &recordingReporter{}

		writer := shards.OpenDir(dir)
		writer.Write(reporter, malformed)
		writer.Write(reporter, &note{Text: "written anyway"})

		wantMessage(t, reporter, "a carrier line was not recorded", "the carried value", "raw field", fixtureRawHint)
		lines := shardLines(t, shards, dir)
		if len(lines) != 1 || !strings.Contains(lines[0], `"text":"written anyway"`) {
			t.Errorf("shard lines = %v, want only the note offered after the malformed carrier", lines)
		}
	})

	t.Run("a spec that does not", func(t *testing.T) {
		dir := t.TempDir()
		shards := newFixture(t, plainSpec())
		reporter := &recordingReporter{}

		shards.OpenDir(dir).Write(reporter, malformed)

		wantMessage(t, reporter, "a carrier line was not recorded", "could not be encoded")
		if strings.Contains(reporter.joined(), "the carried value") {
			t.Errorf("report = %q, want no field name from a spec that asks for no check", reporter.joined())
		}
		if entries, err := os.ReadDir(dir); err != nil {
			t.Fatalf("ReadDir error = %v", err)
		} else if len(entries) != 0 {
			t.Errorf("directory holds %d entr(ies), want none: a skipped line opens no shard", len(entries))
		}
	})
}

// TestWriterWrite_RefusesALineTheReaderCouldNotRead verifies that the writer
// accepts the longest line the reader's scanner takes and refuses one byte
// more, reporting its size and the spec's hint.
//
// The two halves are bounded by one constant and nothing held them to each
// other: a writer that writes whatever it is given produces a shard its own
// reader cannot scan, and that loss arrives at read time, long after the run,
// and is not one line but everything the process recorded. The refusal
// therefore happens here, in front of the run that could still be fixed. The
// size is asserted because it is what a person acts on: the report is the only
// account of a line that was not recorded, and a number that is not the line's
// length sends them looking at the wrong value.
func TestWriterWrite_RefusesALineTheReaderCouldNotRead(t *testing.T) {
	// The newline the writer adds counts against the scanner's bound, so the
	// longest line it can read back is one byte under the cap.
	longest := lineOfLength(t, MaxLine-1)
	tooLong := lineOfLength(t, MaxLine)

	t.Run("the longest line the reader takes", func(t *testing.T) {
		dir := t.TempDir()
		shards := newFixture(t, checkedSpec())
		reporter := &recordingReporter{}

		shards.OpenDir(dir).Write(reporter, longest)

		wantNoMessage(t, reporter)
		records, readErr := shards.Read(dir)
		if readErr != nil {
			t.Fatalf("Read error = %v: the writer wrote a line its own reader refuses", readErr)
		}
		if len(records) != 1 || records[0].Note == nil {
			t.Fatalf("records = %d, want the one note line", len(records))
		}
	})

	t.Run("one byte more, with a hint", func(t *testing.T) {
		dir := t.TempDir()
		shards := newFixture(t, checkedSpec())
		reporter := &recordingReporter{}

		shards.OpenDir(dir).Write(reporter, tooLong, &note{Text: "written anyway"})

		wantMessage(t, reporter,
			"a note line was not recorded",
			"line cap",
			fmt.Sprintf("%d bytes", MaxLine+1),
			fixtureCapHint,
		)
		lines := shardLines(t, shards, dir)
		if len(lines) != 1 || !strings.Contains(lines[0], `"text":"written anyway"`) {
			t.Errorf("shard lines = %v, want only the note offered after the one that was too long", lines)
		}
	})

	t.Run("one byte more, with no hint", func(t *testing.T) {
		dir := t.TempDir()
		shards := newFixture(t, plainSpec())
		reporter := &recordingReporter{}

		shards.OpenDir(dir).Write(reporter, tooLong)

		wantMessage(t, reporter, "line cap")
		if strings.Contains(reporter.joined(), fixtureCapHint) {
			t.Errorf("report = %q, want no hint from a spec that carries none", reporter.joined())
		}
		if entries, err := os.ReadDir(dir); err != nil {
			t.Fatalf("ReadDir error = %v", err)
		} else if len(entries) != 0 {
			t.Errorf("directory holds %d entr(ies), want none: a skipped line opens no shard", len(entries))
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
	shards := newFixture(t, plainSpec())
	reporter := &recordingReporter{}

	first := shards.OpenDir(dir)
	first.Write(reporter, &note{Text: "first"})

	shards.Release()

	second := shards.OpenDir(dir)
	if second == first {
		t.Fatal("OpenDir returned the released writer, want a new one")
	}
	second.Write(reporter, &note{Text: "second"})

	wantNoMessage(t, reporter)
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
	shards := newFixture(t, plainSpec())
	reporter := &recordingReporter{}

	writer := shards.OpenDir(dir)
	// The second shard below belongs to a writer the registry has already let
	// go of, so Release cannot close it; this test closes it itself, or Windows
	// refuses to remove the directory holding it.
	t.Cleanup(writer.release)
	writer.Write(reporter, &note{Text: "first"})

	shards.Release()
	writer.Write(reporter, &note{Text: "second"})

	wantNoMessage(t, reporter)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("shards = %d, want 2: a released writer holds no file and opens another", len(entries))
	}
}

// TestRelease_DoesNotDeduplicateTheNewShardAgainstTheOldOne verifies that a
// line written before a release is written again into the shard opened after
// it.
//
// The deduplication exists so that a line offered twice costs one write, and
// what it is deduplicating against is the shard it already wrote to. A release
// ends that shard, so a set carried across it would refuse a line on the
// evidence of a file the new shard has nothing to do with, and refuse it in
// silence, which is the one failure this whole record is built not to have.
func TestRelease_DoesNotDeduplicateTheNewShardAgainstTheOldOne(t *testing.T) {
	dir := t.TempDir()
	shards := newFixture(t, plainSpec())
	writer := shards.OpenDir(dir)
	// The shard written after the release belongs to a writer the registry has
	// already let go of, so Release cannot close it; this test closes it
	// itself, or Windows refuses to remove the directory holding it.
	t.Cleanup(writer.release)
	reporter := &recordingReporter{}
	line := &note{Text: "written on both sides of the release"}

	writer.Write(reporter, line)
	shards.Release()
	writer.Write(reporter, line)

	wantNoMessage(t, reporter)
	records, err := shards.Read(dir)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if len(records) != 2 {
		t.Errorf("records = %d, want 2: the new shard holds the line whatever the old shard held", len(records))
	}
}

// TestRelease_ReportsAgainAfterTheWriterWasStopped verifies that a writer
// silenced by a failure reports once more after a release.
//
// A stop is a refusal earned by the shard the writer was writing, and a
// release ends that shard. Keeping the flag would make the writer silent for
// the rest of the process about a directory it has not tried since, and
// silence from this writer means nothing went wrong.
func TestRelease_ReportsAgainAfterTheWriterWasStopped(t *testing.T) {
	shards := newFixture(t, plainSpec())
	writer := shards.OpenDir("relative")
	reporter := &recordingReporter{}

	writer.Write(reporter, &note{Text: "first"})
	shards.Release()
	writer.Write(reporter, &note{Text: "second"})

	if len(reporter.messages) != 2 {
		t.Errorf("messages = %d, want 2: the release ended the refusal the first write earned", len(reporter.messages))
	}
}
