package e2ecalls

import (
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
// Off by default is what lets an ordinary suite run pay nothing for this
// record, and a nil writer is how the harness learns that without a branch of
// its own.
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
// every session and test of a process: a writer per caller would write one
// shard per call site and duplicate every line across them.
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
// That is the whole point of returning nil when recording is off: the harness
// records unconditionally and pays a nil check, instead of guarding every call
// site and eventually missing one.
func TestWriterWrite_NilWriterWritesNothing(t *testing.T) {
	var writer *Writer
	reporter := &recordingReporter{}

	writer.Write(reporter, &Skip{Test: "TestCommon_Issues", Reason: "no runner"})

	if len(reporter.messages) != 0 {
		t.Errorf("reported %v, want nothing from a nil writer", reporter.messages)
	}
}

// TestWriterWrite_WritesEachLineOnceAndDropsRepeats verifies that lines reach
// the shard in order, one per line, and that an identical line offered twice
// is written once.
//
// Repeats are ordinary: a session line is offered by every test that uses the
// session, and a retried call offers the same line again. Writing them all
// would inflate nothing in the audit, which counts distinct cells, and would
// make the shard several times larger than the run it describes.
func TestWriterWrite_WritesEachLineOnceAndDropsRepeats(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	writer := OpenDir(dir)
	reporter := &recordingReporter{}
	call := &Call{Test: "TestCommon_Issues", Action: "issue.list", Outcome: OutcomeOK, TestStatus: StatusPassed}

	writer.Write(reporter, &Run{Package: "common", Status: RunStarted}, call)
	writer.Write(reporter, call)

	if len(reporter.messages) != 0 {
		t.Fatalf("reported %v, want nothing", reporter.messages)
	}
	lines := shardLines(t, dir)
	if len(lines) != 2 {
		t.Fatalf("shard lines = %d, want 2: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"type":"run"`) {
		t.Errorf("first line = %s, want the run line", lines[0])
	}
	if !strings.Contains(lines[1], `"action":"issue.list"`) {
		t.Errorf("second line = %s, want the call line", lines[1])
	}
}

// TestWriterWrite_RefusesARelativeDirectory verifies that a relative directory
// is reported once and nothing is written.
//
// A test binary runs in its own package directory, so a relative directory
// would leave one shard under each package and the merge would find none of
// them. Resolving it would produce that scattering silently, which is why it
// is refused instead.
func TestWriterWrite_RefusesARelativeDirectory(t *testing.T) {
	t.Cleanup(Release)

	writer := OpenDir(filepath.Join("dist", "e2e-calls"))
	reporter := &recordingReporter{}

	writer.Write(reporter, &Skip{Test: "TestCommon_Issues", Reason: "no runner"})
	writer.Write(reporter, &Skip{Test: "TestCommon_Labels", Reason: "no runner"})

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

	writer.Write(reporter, &Skip{Test: "TestCommon_Issues", Reason: "no runner"})
	writer.Write(reporter, &Skip{Test: "TestCommon_Labels", Reason: "no runner"})

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
// A dropped line is indistinguishable from a call nobody made, which is the
// one reading of this record that must never be wrong.
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

	writer.Write(reporter, &Skip{Test: "TestCommon_Issues", Reason: "no runner"})

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), "could not write") {
		t.Errorf("report = %q, want it to say the shard could not be written", reporter.joined())
	}
}

// TestWriterWrite_ReportsALineItCannotEncode verifies that a line JSON cannot
// hold is reported once and stops the writer.
//
// A duration of NaN is the reachable case: every other field is a string, a
// bool or a string slice. It is reported rather than skipped because a writer
// that drops what it cannot encode would produce a shard that is missing calls
// and says so nowhere.
func TestWriterWrite_ReportsALineItCannotEncode(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	writer := OpenDir(dir)
	reporter := &recordingReporter{}

	writer.Write(reporter, &Call{Test: "TestCommon_Issues", DurationMS: math.NaN()})
	writer.Write(reporter, &Skip{Test: "TestCommon_Labels", Reason: "no runner"})

	if len(reporter.messages) != 1 {
		t.Fatalf("reported %d time(s), want 1: %v", len(reporter.messages), reporter.messages)
	}
	if !strings.Contains(reporter.joined(), "could not encode") {
		t.Errorf("report = %q, want it to say the line could not be encoded", reporter.joined())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("directory holds %d entr(ies), want none: a stopped writer opens no shard", len(entries))
	}
}

// TestRelease_ClosesTheShardAndForgetsTheWriter verifies that Release lets a
// later Open of the same directory start a new shard.
//
// Windows is why it exists: a directory holding an open file cannot be
// removed, so a test recording into t.TempDir() would fail in cleanup after
// every one of its own assertions had passed.
func TestRelease_ClosesTheShardAndForgetsTheWriter(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(Release)

	reporter := &recordingReporter{}
	first := OpenDir(dir)
	first.Write(reporter, &Skip{Test: "TestCommon_Issues", Reason: "no runner"})

	Release()

	second := OpenDir(dir)
	if second == first {
		t.Fatal("OpenDir returned the released writer, want a new one")
	}
	second.Write(reporter, &Skip{Test: "TestCommon_Labels", Reason: "no runner"})

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
	writer.Write(reporter, &Skip{Test: "TestCommon_Issues", Reason: "no runner"})

	Release()
	writer.Write(reporter, &Skip{Test: "TestCommon_Labels", Reason: "no runner"})

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
// It is called directly because the writer stubs it everywhere else, so this
// is the only place the real one's failure branch is reached.
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
