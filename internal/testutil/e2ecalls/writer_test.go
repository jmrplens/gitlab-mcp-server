package e2ecalls

import (
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
	var shardFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && IsShard(entry.Name()) {
			shardFiles = append(shardFiles, filepath.Join(dir, entry.Name()))
		}
	}
	if len(shardFiles) != 1 {
		t.Fatalf("shards in %s = %d, want 1", dir, len(shardFiles))
	}
	content, err := os.ReadFile(shardFiles[0])
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
// The mechanism is internal/testutil/shardio's and is tested there; what this
// asks is the half this package contributes, that the variable the harness is
// documented to set is the one Open reads. Off by default is what lets an
// ordinary suite run pay nothing for this record, and a nil writer is how the
// harness learns that without a branch of its own.
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

// TestWriterWrite_ReportsALineItCannotEncode verifies that a line JSON cannot
// hold is reported and skipped, and that the line offered after it is still
// written.
//
// A duration of NaN is the reachable case here, and only barely: every other
// field is a string, a bool, an int or a string slice, and the harness computes
// this one from a time.Duration, which has no NaN to give. It is reported
// because a writer that drops what it cannot encode would produce a shard that
// is missing calls and says so nowhere. It costs that line alone because
// stopping costs strictly more: every later test's call, dispatch and skip
// lines vanish in silence, and so do the session lines and the run line, which
// are written after the last test from an exit hook through a reporter that
// only writes to the log. The run line is what cmd/audit_e2e_coverage joins a
// shard's calls to their package on, so losing it makes the calls that were
// recorded unusable too.
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
	if !strings.Contains(reporter.joined(), "could not be encoded") {
		t.Errorf("report = %q, want it to say the line could not be encoded", reporter.joined())
	}
	lines := shardLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("shard lines = %d, want 1: the line after the unencodable one is still recorded: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"type":"skip"`) {
		t.Errorf("shard line = %s, want the skip line offered after the one that could not be encoded", lines[0])
	}
}
