// file_utils_test.go contains unit tests for shared file utility functions
// including file validation, SHA-256 checksum computation, progress tracking,
// and package name validation.
package toolutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestOpenAndValidateFile_RegularFile verifies that a regular file is accepted.
func TestOpenAndValidateFile_RegularFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, info, err := OpenAndValidateFile(path, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer f.Close()

	if info.Size() != 5 {
		t.Errorf("expected size 5, got %d", info.Size())
	}
}

// TestOpenAndValidateFile_Directory verifies directories are rejected.
func TestOpenAndValidateFile_Directory(t *testing.T) {
	tmp := t.TempDir()

	_, _, err := OpenAndValidateFile(tmp, 1024)
	if err == nil {
		t.Fatal("expected error for directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("expected 'not a regular file' error, got: %v", err)
	}
}

// TestOpenAndValidateFile_NotFound verifies missing files return an error.
func TestOpenAndValidateFile_NotFound(t *testing.T) {
	_, _, err := OpenAndValidateFile("/nonexistent/path/file.txt", 1024)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestOpenAndValidateFile_TooLarge verifies files exceeding maxSize are rejected.
func TestOpenAndValidateFile_TooLarge(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "large.bin")
	if err := os.WriteFile(path, make([]byte, 2048), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := OpenAndValidateFile(path, 1024)
	if err == nil {
		t.Fatal("expected error for too-large file, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected 'exceeds maximum' error, got: %v", err)
	}
}

// TestOpenAndValidateFile_ExactlyMaxSize_Accepted verifies that a file of
// exactly the configured size is opened rather than refused: the limit is a
// maximum, and both checks that enforce it — the one on the path before the
// open and the one on the descriptor after it — have to agree about that, or a
// deployment configured to allow 2 GB refuses the 2 GB upload it was told to
// allow.
func TestOpenAndValidateFile_ExactlyMaxSize_Accepted(t *testing.T) {
	const maxSize = 1024
	path := filepath.Join(t.TempDir(), "exact.bin")
	if err := os.WriteFile(path, make([]byte, maxSize), 0o600); err != nil {
		t.Fatal(err)
	}

	f, info, err := OpenAndValidateFile(path, maxSize)
	if err != nil {
		t.Fatalf("OpenAndValidateFile() error = %v, want a file of exactly the limit accepted", err)
	}
	defer f.Close()
	if info.Size() != maxSize {
		t.Errorf("info.Size() = %d, want %d", info.Size(), maxSize)
	}
}

// TestOpenAndValidateFile_EmptyPath verifies empty path is rejected.
func TestOpenAndValidateFile_EmptyPath(t *testing.T) {
	_, _, err := OpenAndValidateFile("", 1024)
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
	if !strings.Contains(err.Error(), "file path is required") {
		t.Errorf("expected 'file path is required' error, got: %v", err)
	}
}

// TestOpenAndValidateFile_ZeroMaxSize verifies maxSize=0 skips size check.
func TestOpenAndValidateFile_ZeroMaxSize(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "any.bin")
	if err := os.WriteFile(path, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}

	f, _, err := OpenAndValidateFile(path, 0)
	if err != nil {
		t.Fatalf("unexpected error with maxSize=0: %v", err)
	}
	f.Close()
}

// TestCanonicalImportArchivePath_TempArchive verifies that a canonical .tar.gz
// archive under the OS temp directory is accepted.
func TestCanonicalImportArchivePath_TempArchive(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "project-export.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}

	got, err := CanonicalImportArchivePath(path)
	if err != nil {
		t.Fatalf("CanonicalImportArchivePath() error: %v", err)
	}
	if got != want {
		t.Fatalf("canonical path = %q, want %q", got, want)
	}
}

// TestCanonicalImportArchivePath_RejectsWrongExtension verifies local import
// archives are constrained to GitLab export archive filenames.
func TestCanonicalImportArchivePath_RejectsWrongExtension(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "project-export.zip")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := CanonicalImportArchivePath(path)
	if err == nil {
		t.Fatal("expected extension validation error")
	}
	if !strings.Contains(err.Error(), ".tar.gz") {
		t.Fatalf("error = %v, want .tar.gz mention", err)
	}
}

// TestCanonicalImportArchivePath_RejectsSymlinkEscape verifies that a symlink
// inside an allowed directory cannot point to an archive outside that directory.
func TestCanonicalImportArchivePath_RejectsSymlinkEscape(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	testutil.IsolateTempDir(t, allowed)
	t.Chdir(allowed)

	target := filepath.Join(outside, "project-export.tar.gz")
	if err := os.WriteFile(target, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(allowed, "linked-export.tar.gz")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := CanonicalImportArchivePath(link)
	if err == nil {
		t.Fatal("expected symlink escape validation error")
	}
	if !strings.Contains(err.Error(), "outside allowed import directories") {
		t.Fatalf("error = %v, want allowed-directory mention", err)
	}
}

// TestCanonicalImportArchivePath_AllowsConfiguredDirectory verifies that
// GITLAB_MCP_ALLOWED_IMPORT_DIRS extends the allowed local archive roots.
func TestCanonicalImportArchivePath_AllowsConfiguredDirectory(t *testing.T) {
	cwd := t.TempDir()
	configured := t.TempDir()
	testutil.IsolateTempDir(t, t.TempDir())
	t.Setenv(ImportArchiveAllowlistEnv, configured)
	t.Chdir(cwd)

	path := filepath.Join(configured, "project-export.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}

	got, err := CanonicalImportArchivePath(path)
	if err != nil {
		t.Fatalf("CanonicalImportArchivePath() error: %v", err)
	}
	if got != want {
		t.Fatalf("canonical path = %q, want %q", got, want)
	}
}

// TestCanonicalImportArchivePath_RejectsInvalidInputs verifies archive path
// validation branches before allowlist checks.
func TestCanonicalImportArchivePath_RejectsInvalidInputs(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		_, err := CanonicalImportArchivePath("")
		if err == nil || !strings.Contains(err.Error(), "archive path is required") {
			t.Fatalf("error = %v, want required path error", err)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		_, err := CanonicalImportArchivePath(filepath.Join(t.TempDir(), "missing.tar.gz"))
		if err == nil || !strings.Contains(err.Error(), "resolve archive symlinks") {
			t.Fatalf("error = %v, want symlink resolution error", err)
		}
	})

	t.Run("directory archive", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "project-export.tar.gz")
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		_, err := CanonicalImportArchivePath(dir)
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("error = %v, want regular-file validation error", err)
		}
	})
}

// TestCanonicalImportArchivePath_RejectsUnsafePermissions verifies import
// archives cannot be group- or world-writable on Unix-like systems.
func TestCanonicalImportArchivePath_RejectsUnsafePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "project-export.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o666); err != nil { //nolint:gosec // Intentionally creates unsafe permissions for validation coverage.
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil { //nolint:gosec // Intentionally creates unsafe permissions for validation coverage.
		t.Fatal(err)
	}

	_, err := CanonicalImportArchivePath(path)
	if err == nil || !strings.Contains(err.Error(), "group/world-writable") {
		t.Fatalf("error = %v, want unsafe permissions error", err)
	}
}

// TestAllowedImportArchiveDirs_SkipsInvalidConfiguredDirectory verifies invalid
// allowlist entries are ignored while valid roots remain available.
func TestAllowedImportArchiveDirs_SkipsInvalidConfiguredDirectory(t *testing.T) {
	base := t.TempDir()
	invalid := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(invalid, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ImportArchiveAllowlistEnv, invalid)

	allowed := allowedImportArchiveDirs()
	for _, dir := range allowed {
		if dir == invalid {
			t.Fatalf("allowedImportArchiveDirs() included invalid configured file %q", invalid)
		}
	}
}

// TestCanonicalDirPath_RejectsInvalidDirectories covers direct directory
// canonicalization errors used by import-archive allowlisting.
func TestCanonicalDirPath_RejectsInvalidDirectories(t *testing.T) {
	if _, err := canonicalDirPath(""); err == nil {
		t.Fatal("canonicalDirPath(empty) error = nil, want error")
	}
	if _, err := canonicalDirPath(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("canonicalDirPath(missing) error = nil, want error")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalDirPath(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("canonicalDirPath(file) error = %v, want not-a-directory", err)
	}
}

// TestPathWithinBase verifies direct allowlist path containment checks for
// the base directory itself, child paths, and sibling escapes.
func TestPathWithinBase(t *testing.T) {
	base := t.TempDir()
	child := filepath.Join(base, "nested", "archive.tar.gz")
	sibling := filepath.Join(filepath.Dir(base), filepath.Base(base)+"-sibling", "archive.tar.gz")

	if !pathWithinBase(base, base) {
		t.Fatal("pathWithinBase(base, base) = false, want true")
	}
	if !pathWithinBase(child, base) {
		t.Fatal("pathWithinBase(child, base) = false, want true")
	}
	if pathWithinBase(sibling, base) {
		t.Fatal("pathWithinBase(sibling, base) = true, want false")
	}
}

// TestComputeSHA256_KnownHash verifies a known content produces the expected SHA-256.
func TestComputeSHA256_KnownHash(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "known.txt")
	content := []byte("Hello, World!")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

// TestComputeSHA256_EmptyFile verifies SHA-256 of an empty file.
func TestComputeSHA256_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.bin")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ComputeSHA256(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != expected {
		t.Errorf("expected empty hash %s, got %s", expected, got)
	}
}

// TestComputeSHA256Reader_FromBytes verifies checksum from a byte reader.
func TestComputeSHA256Reader_FromBytes(t *testing.T) {
	data := []byte("test data for checksum")
	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])

	got, err := ComputeSHA256Reader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

// TestProgressReader_ReportsProgress verifies the progress reader counts bytes
// and calls the progress tracker.
func TestProgressReader_ReportsProgress(t *testing.T) {
	data := make([]byte, 256*1024) // 256 KB
	for i := range data {
		data[i] = byte(i % 256)
	}

	tracker := progress.Tracker{} // inactive tracker — no-op
	pr := NewProgressReader(context.Background(), bytes.NewReader(data), int64(len(data)), tracker)

	buf := make([]byte, 32*1024)
	var totalRead int64
	for {
		n, err := pr.Read(buf)
		totalRead += int64(n)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
	}

	if totalRead != int64(len(data)) {
		t.Errorf("expected to read %d bytes, got %d", len(data), totalRead)
	}
	if pr.BytesRead() != int64(len(data)) {
		t.Errorf("progressReader.BytesRead() = %d, want %d", pr.BytesRead(), len(data))
	}
}

// TestProgressWriter_ReportsProgress verifies the progress writer counts bytes.
func TestProgressWriter_ReportsProgress(t *testing.T) {
	var buf bytes.Buffer
	data := []byte("some download data for writer test")

	tracker := progress.Tracker{} // inactive tracker
	pw := NewProgressWriter(context.Background(), &buf, int64(len(data)), tracker)

	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected to write %d bytes, got %d", len(data), n)
	}
	if pw.BytesWritten() != int64(len(data)) {
		t.Errorf("progressWriter.BytesWritten() = %d, want %d", pw.BytesWritten(), len(data))
	}
	if buf.String() != string(data) {
		t.Error("written content does not match input")
	}
}

// progressReport records one notification a progress wrapper sent, so a test
// can assert which reads and writes reported and which stayed quiet.
type progressReport struct {
	done  int64
	total int64
}

// TestProgressReader_ReportsAtTheIntervalAndAtEOF verifies when a
// [ProgressReader] notifies and when it says nothing: a read that has not
// advanced a whole interval since the last notification is silent, a read that
// lands exactly on the interval reports, a small read after one does not report
// again, and the final read that returns io.EOF flushes the tail.
//
// Both halves matter to the client. Without the interval a large upload
// notifies on every buffer the copy loop fills; without the EOF flush the last
// notification is the last multiple of the interval, so a transfer that
// finished is shown stopped short of its total.
//
// The sink is replaced rather than driven through a real MCP session because
// what is under test is the schedule, not the notification plumbing: an
// inactive tracker drops every update, so the scheduling decisions were
// invisible through the constructor's own callback.
func TestProgressReader_ReportsAtTheIntervalAndAtEOF(t *testing.T) {
	const total = int64(65556) // small enough that the interval is the 64 KB floor
	const interval = int64(64 * 1024)

	source := bytes.NewReader(bytes.Repeat([]byte("x"), int(total)))
	pr := NewProgressReader(context.Background(), source, total, progress.Tracker{})
	var reports []progressReport
	pr.onProgress = func(read, reportedTotal int64) {
		reports = append(reports, progressReport{done: read, total: reportedTotal})
	}
	if pr.interval != interval {
		t.Fatalf("interval = %d, want the %d floor", pr.interval, interval)
	}

	read := func(n int64) {
		t.Helper()
		got, err := pr.Read(make([]byte, n))
		if int64(got) != n || err != nil {
			t.Fatalf("Read(%d) = %d, %v; want %d, nil", n, got, err, n)
		}
	}

	read(100)
	if len(reports) != 0 {
		t.Fatalf("reports after 100 bytes = %v, want none before the interval of %d", reports, interval)
	}

	read(interval - 100)
	if len(reports) != 1 || reports[0].done != interval || reports[0].total != total {
		t.Fatalf("reports at exactly the interval = %v, want one {%d %d}", reports, interval, total)
	}

	read(10)
	if len(reports) != 1 {
		t.Fatalf("reports after a further 10 bytes = %v, want still one: the interval runs from the last report", reports)
	}

	tail, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v, want nil", err)
	}
	if int64(len(tail)) != total-interval-10 {
		t.Fatalf("tail = %d bytes, want %d", len(tail), total-interval-10)
	}
	if len(reports) != 2 || reports[1].done != total {
		t.Errorf("reports after EOF = %v, want a final flush of %d", reports, total)
	}
}

// TestProgressWriter_ReportsAtTheIntervalAndOnFailure verifies the writer's
// schedule and its failure flush: a write that has not advanced a whole
// interval is silent, one that lands exactly on the interval reports, a small
// write after it does not report again, and a write that fails reports what
// reached the destination before the error.
//
// The failure flush is what tells the client where a download stopped; the
// interval is what keeps a large one from notifying on every chunk.
func TestProgressWriter_ReportsAtTheIntervalAndOnFailure(t *testing.T) {
	const total = int64(65556)
	const interval = int64(64 * 1024)

	var sink bytes.Buffer
	pw := NewProgressWriter(context.Background(), &sink, total, progress.Tracker{})
	var reports []progressReport
	pw.onProgress = func(written, reportedTotal int64) {
		reports = append(reports, progressReport{done: written, total: reportedTotal})
	}
	if pw.interval != interval {
		t.Fatalf("interval = %d, want the %d floor", pw.interval, interval)
	}

	write := func(n int64) {
		t.Helper()
		got, err := pw.Write(make([]byte, n))
		if int64(got) != n || err != nil {
			t.Fatalf("Write(%d) = %d, %v; want %d, nil", n, got, err, n)
		}
	}

	write(100)
	if len(reports) != 0 {
		t.Fatalf("reports after 100 bytes = %v, want none before the interval of %d", reports, interval)
	}

	write(interval - 100)
	if len(reports) != 1 || reports[0].done != interval || reports[0].total != total {
		t.Fatalf("reports at exactly the interval = %v, want one {%d %d}", reports, interval, total)
	}

	write(10)
	if len(reports) != 1 {
		t.Fatalf("reports after a further 10 bytes = %v, want still one: the interval runs from the last report", reports)
	}

	t.Run("a failed write reports what reached the destination", func(t *testing.T) {
		failing := NewProgressWriter(context.Background(), &errWriter{err: errors.New("no space left on device")}, total, progress.Tracker{})
		var failures []progressReport
		failing.onProgress = func(written, reportedTotal int64) {
			failures = append(failures, progressReport{done: written, total: reportedTotal})
		}
		if _, err := failing.Write(make([]byte, 10)); err == nil {
			t.Fatal("Write() error = nil, want the destination's failure")
		}
		if len(failures) != 1 {
			t.Errorf("reports on a failed write = %v, want one flush", failures)
		}
	})
}

// progressWaitBound is how long progressProbeMessages gives the whole exchange
// — the tool call and the notifications it produces — before it calls the
// session stalled. It is enormous for an in-memory transport and deliberately
// so: the bound names a hang, it does not measure a speed.
const progressWaitBound = 5 * time.Second

// progressProbeMessages runs fn as the body of a real tool call on an
// in-memory MCP session that carries a progress token, and returns the
// messages the client received, once want of them have arrived.
//
// It exists because the two schedule tests above replace onProgress with a
// sink of their own, which is right for measuring when a report fires and
// leaves the callback the constructor builds — the one that decides what each
// report says — driven by nothing: an inactive tracker returns from its first
// line. Only a live session makes those decisions observable.
func progressProbeMessages(t *testing.T, want int, fn func(ctx context.Context, tracker progress.Tracker)) []string {
	t.Helper()

	var mu sync.Mutex
	var messages []string
	received := make(chan struct{})

	server := mcp.NewServer(&mcp.Implementation{Name: "progress-probe-server", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "probe", Description: "copies bytes under a progress tracker"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			tracker := progress.FromRequest(req)
			if !tracker.IsActive() {
				t.Errorf("tracker inactive inside the tool call: the probe would measure nothing")
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "inactive"}}}, nil, nil
			}
			fn(ctx, tracker)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "progress-probe-client", Version: "1.0.0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			defer mu.Unlock()
			messages = append(messages, req.Params.Message)
			if len(messages) == want {
				close(received)
			}
		},
	})
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	// The call is bounded as well as the wait below, and it has to be: the
	// call completes before the select is even reached, so a stalled tool or a
	// stalled notification would never meet that timeout and would hang the
	// binary until the package deadline instead. One bound serves both, so the
	// two cannot drift apart.
	callCtx, cancelCall := context.WithTimeout(t.Context(), progressWaitBound)
	defer cancelCall()

	if _, callErr := session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      "probe",
		Arguments: map[string]any{},
		Meta:      mcp.Meta{"progressToken": "file-utils-probe"},
	}); callErr != nil {
		t.Fatalf("CallTool: %v", callErr)
	}

	select {
	case <-received:
	case <-time.After(progressWaitBound):
		mu.Lock()
		got := slices.Clone(messages)
		mu.Unlock()
		t.Fatalf("waited for %d progress notifications, got %d: %q", want, len(got), got)
	}

	mu.Lock()
	defer mu.Unlock()
	return slices.Clone(messages)
}

// TestProgressReader_ActiveTracker_ClaimsTheHandoverOnlyOnAKnownTotalReached
// verifies what an upload frame says, which is the half the schedule tests
// cannot see. Two things have to hold and both were stated only by the code
// that does them: the handover line ("uploading to GitLab", the moment after
// which the transfer itself reports nothing) is written when the read has
// reached the total, and it is never written when there is no total to reach,
// because a source of unknown size gives no moment to name.
func TestProgressReader_ActiveTracker_ClaimsTheHandoverOnlyOnAKnownTotalReached(t *testing.T) {
	const size = int64(64 * 1024) // one whole report interval, so a frame fires

	t.Run("the read reaches a known total", func(t *testing.T) {
		messages := progressProbeMessages(t, 1, func(ctx context.Context, tracker progress.Tracker) {
			pr := NewProgressReader(ctx, bytes.NewReader(bytes.Repeat([]byte("x"), int(size))), size, tracker)
			if _, err := io.Copy(io.Discard, pr); err != nil {
				t.Errorf("copy: %v", err)
			}
		})
		want := "Read 65536 bytes, uploading to GitLab"
		if messages[0] != want {
			t.Errorf("frame at the total = %q, want %q", messages[0], want)
		}
	})

	t.Run("the read stops short of a known total", func(t *testing.T) {
		messages := progressProbeMessages(t, 1, func(ctx context.Context, tracker progress.Tracker) {
			pr := NewProgressReader(ctx, bytes.NewReader(bytes.Repeat([]byte("x"), int(size))), size*2, tracker)
			if _, err := io.Copy(io.Discard, pr); err != nil {
				t.Errorf("copy: %v", err)
			}
		})
		want := "Read 65536 / 131072 bytes, preparing the upload"
		if messages[0] != want {
			t.Errorf("frame below the total = %q, want %q", messages[0], want)
		}
	})

	t.Run("the source has no total", func(t *testing.T) {
		messages := progressProbeMessages(t, 1, func(ctx context.Context, tracker progress.Tracker) {
			pr := NewProgressReader(ctx, bytes.NewReader(bytes.Repeat([]byte("x"), int(size))), 0, tracker)
			if _, err := io.Copy(io.Discard, pr); err != nil {
				t.Errorf("copy: %v", err)
			}
		})
		if strings.Contains(messages[0], "uploading to GitLab") {
			t.Errorf("frame with no total = %q, want no claim that the read reached one", messages[0])
		}
		if !strings.Contains(messages[0], "preparing the upload") {
			t.Errorf("frame with no total = %q, want the preparing wording", messages[0])
		}
	})
}

// TestProgressWriter_ActiveTracker_NamesTheTotalOnlyWhenThereIsOne verifies
// the download frame the constructor's callback writes: a known size is shown
// as "N / M bytes", and a stream whose size GitLab did not send is shown as
// "N bytes" alone. The second is the one that matters in practice — a package
// download surfaces no content length, and every frame of one used to read
// "Downloaded 65536 / 0 bytes".
func TestProgressWriter_ActiveTracker_NamesTheTotalOnlyWhenThereIsOne(t *testing.T) {
	const chunk = 64 * 1024 // one whole report interval, so a frame fires

	t.Run("a known total", func(t *testing.T) {
		messages := progressProbeMessages(t, 1, func(ctx context.Context, tracker progress.Tracker) {
			var sink bytes.Buffer
			pw := NewProgressWriter(ctx, &sink, 100000, tracker)
			if _, err := pw.Write(bytes.Repeat([]byte("x"), chunk)); err != nil {
				t.Errorf("write: %v", err)
			}
		})
		want := "Downloaded 65536 / 100000 bytes"
		if messages[0] != want {
			t.Errorf("frame with a total = %q, want %q", messages[0], want)
		}
	})

	t.Run("no total", func(t *testing.T) {
		messages := progressProbeMessages(t, 1, func(ctx context.Context, tracker progress.Tracker) {
			var sink bytes.Buffer
			pw := NewProgressWriter(ctx, &sink, 0, tracker)
			if _, err := pw.Write(bytes.Repeat([]byte("x"), chunk)); err != nil {
				t.Errorf("write: %v", err)
			}
		})
		want := "Downloaded 65536 bytes"
		if messages[0] != want {
			t.Errorf("frame with no total = %q, want %q", messages[0], want)
		}
	})
}

// errWriter fails every write, standing in for a destination that runs out of
// space part-way through a download.
type errWriter struct{ err error }

// Write reports errWriter's configured failure without consuming p.
func (w *errWriter) Write([]byte) (int, error) { return 0, w.err }

// TestProgressReportInterval verifies the interval calculation logic.
func TestProgressReportInterval(t *testing.T) {
	tests := []struct {
		name    string
		total   int64
		wantMin int64
		wantMax int64
	}{
		{"small file (100KB)", 100 * 1024, 64 * 1024, 64 * 1024},
		{"medium file (10MB)", 10 * 1024 * 1024, 64 * 1024, 1024 * 1024},
		{"large file (100MB)", 100 * 1024 * 1024, 1024 * 1024, 1024 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProgressReportInterval(tt.total)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("ProgressReportInterval(%d) = %d, want between %d and %d",
					tt.total, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestValidatePackageName contains table-driven tests for package name validation.
func TestValidatePackageName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "my-package", false},
		{"valid with version path", "my-org/my-package", false},
		{"valid alphanumeric", "pkg123", false},
		{"valid with dots", "com.example.pkg", false},
		{"valid with plus", "my+package", false},
		{"valid with tilde", "my~package", false},
		{"valid with at", "my@package", false},
		{"empty", "", true},
		{"starts with dot", ".hidden", true},
		{"starts with dash", "-invalid", true},
		{"contains space", "my package", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackageName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestValidatePackageFileName contains table-driven tests for filename validation.
func TestValidatePackageFileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "file.txt", false},
		{"valid complex", "my-pkg_v1.0+build.tar.gz", false},
		{"valid with dots", "archive.tar.gz", false},
		{"empty", "", true},
		{"contains space", "my file.txt", true},
		{"starts with tilde", "~tempfile", true},
		{"starts with at", "@scope", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageFileName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackageFileName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestSetGet_UploadConfig verifies that SetUploadConfig stores custom values
// and GetUploadConfig retrieves them. Restores defaults after the test.
func TestSetGet_UploadConfig(t *testing.T) {
	orig := GetUploadConfig()
	t.Cleanup(func() {
		SetUploadConfig(orig.MaxFileSize)
	})

	SetUploadConfig(4096)
	got := GetUploadConfig()

	if got.MaxFileSize != 4096 {
		t.Errorf("MaxFileSize = %d, want 4096", got.MaxFileSize)
	}
}

// TestComputeSHA256_NonexistentFile verifies that ComputeSHA256 returns an
// error when the file does not exist (covers the os.Open error branch).
func TestComputeSHA256_NonexistentFile(t *testing.T) {
	_, err := ComputeSHA256(filepath.Join(t.TempDir(), "does-not-exist.bin"))
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

// TestComputeSHA256Reader_ErrorReader verifies that ComputeSHA256Reader
// propagates errors from a failing io.Reader (covers the io.Copy error branch).
func TestComputeSHA256Reader_ErrorReader(t *testing.T) {
	failReader := &errReader{err: io.ErrUnexpectedEOF}
	_, err := ComputeSHA256Reader(failReader)
	if err == nil {
		t.Fatal("expected error from failing reader, got nil")
	}
}

// errReader is a test helper that always returns the configured error.
type errReader struct{ err error }

// Read streams data from errReader into p.
func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

// TestProgressWriter_ReportsAtInterval verifies that the progress report
// branch triggers when written bytes exceed the report interval threshold.
func TestProgressWriter_ReportsAtInterval(t *testing.T) {
	var buf bytes.Buffer
	total := int64(100)

	tracker := progress.Tracker{}
	pw := NewProgressWriter(context.Background(), &buf, total, tracker)

	// Force a very small interval so the write triggers the report branch.
	pw.interval = 1

	data := []byte("hello world")
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("wrote %d bytes, want %d", n, len(data))
	}
	if pw.lastReport != int64(len(data)) {
		t.Errorf("lastReport = %d, want %d (should update after interval)", pw.lastReport, len(data))
	}
}

// TestOpenAndValidateFile_UnreadableFile_ReturnsOpenError verifies that
// [OpenAndValidateFile] surfaces the os.Open error when the file exists and
// passes the stat/regular/size checks but cannot be opened for reading
// (mode 0o000). This exercises the open-failure branch that runs after all
// validations succeed.
func TestOpenAndValidateFile_UnreadableFile_ReturnsOpenError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}

	path := filepath.Join(t.TempDir(), "secret.bin")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	f, info, err := OpenAndValidateFile(path, 0)
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("OpenAndValidateFile(unreadable) error = nil, want open error")
	}
	if info != nil {
		t.Errorf("OpenAndValidateFile(unreadable) info = %v, want nil", info)
	}
	if !strings.Contains(err.Error(), "open ") {
		t.Errorf("OpenAndValidateFile(unreadable) error = %q, want it to mention open", err)
	}
}

// TestCanonicalImportArchivePath_DeletedCwd_ReturnsResolveError verifies that
// [CanonicalImportArchivePath] fails cleanly for a relative archive path when
// the current working directory has been removed. On Linux os.Getwd fails and
// the filepath.Abs error branch ("resolve archive path") is exercised; on
// macOS getcwd still succeeds for a deleted cwd (vnode name cache), so the
// failure surfaces at symlink resolution instead — both are resolve errors.
// The same deleted-cwd state verifies that canonicalDirPath also fails for a
// relative allowlist directory.
func TestCanonicalImportArchivePath_DeletedCwd_ReturnsResolveError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("removing the current working directory is not supported on Windows")
	}

	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Skipf("cannot remove current working directory: %v", err)
	}

	_, err := CanonicalImportArchivePath("export.tar.gz")
	if err == nil {
		t.Fatal("CanonicalImportArchivePath(relative, deleted cwd) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "resolve archive") {
		t.Errorf("CanonicalImportArchivePath() error = %q, want a 'resolve archive' error", err)
	}

	if _, dirErr := canonicalDirPath("relative-allowlist-dir"); dirErr == nil {
		t.Error("canonicalDirPath(relative, deleted cwd) error = nil, want error")
	}
}

// TestCanonicalLocalPaths_DeletedCwd_ReturnsResolveError is the same deleted
// working directory put to the upload and download resolvers, which make a
// relative path absolute the same way and must refuse the same way rather
// than resolve it against a directory that is gone.
func TestCanonicalLocalPaths_DeletedCwd_ReturnsResolveError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only Linux fails os.Getwd for a removed working directory")
	}

	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Skipf("cannot remove current working directory: %v", err)
	}

	if _, err := CanonicalLocalFilePath("upload.txt"); err == nil || !strings.HasPrefix(err.Error(), "resolve file path: ") {
		t.Errorf("CanonicalLocalFilePath(relative, deleted cwd) error = %v, want the 'resolve file path' refusal", err)
	}
	if _, err := CanonicalDownloadOutputPath("artifact.bin"); err == nil || !strings.HasPrefix(err.Error(), "resolve output path: ") {
		t.Errorf("CanonicalDownloadOutputPath(relative, deleted cwd) error = %v, want the 'resolve output path' refusal", err)
	}
}

// TestPathWithinBase_RelError_ReturnsFalse verifies that pathWithinBase
// returns false when filepath.Rel cannot relate the two paths (an absolute
// target against a relative base), instead of panicking or misclassifying.
func TestPathWithinBase_RelError_ReturnsFalse(t *testing.T) {
	if pathWithinBase("/absolute/target", "relative-base") {
		t.Error(`pathWithinBase("/absolute/target", "relative-base") = true, want false`)
	}
}

// makeDirs creates each directory or fails the test, so a containment fixture
// reads as one line instead of a loop that asserts.
func makeDirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatalf("Mkdir(%q) error = %v", dir, err)
		}
	}
}

// confineLocalPathRoots points the default local-path allow-list roots (the
// working directory and the OS temp directory) at dir, so that a sibling of
// dir is genuinely outside every allowed root without the test needing a
// writable location outside the sandbox.
func confineLocalPathRoots(t *testing.T, dir string) {
	t.Helper()
	testutil.IsolateTempDir(t, dir)
	t.Chdir(dir)
}

// TestOpenAndValidateFile_OutsideAllowedDirs_Rejected verifies that a
// caller-supplied file_path is confined to the allow-listed roots: a file
// inside the workspace is opened, while an absolute path outside it, a
// parent-traversal escape and a symlink inside the workspace whose target
// lives outside are all refused before any byte is read.
func TestOpenAndValidateFile_OutsideAllowedDirs_Rejected(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "secrets")
	makeDirs(t, allowed, outside)
	secret := filepath.Join(outside, "id_rsa")
	if err := os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	inside := filepath.Join(allowed, "upload.txt")
	if err := os.WriteFile(inside, []byte("payload"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(allowed, "innocent.txt")
	symlinked := os.Symlink(secret, link) == nil

	confineLocalPathRoots(t, allowed)

	tests := []struct {
		name    string
		path    string
		wantErr bool
		skip    bool
	}{
		{name: "file inside the allowed root is opened", path: inside},
		{name: "absolute path outside every allowed root is refused", path: secret, wantErr: true},
		{name: "parent traversal out of the allowed root is refused", path: filepath.Join(allowed, "..", "secrets", "id_rsa"), wantErr: true},
		{name: "symlink inside the root pointing outside is refused", path: link, wantErr: true, skip: !symlinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			f, _, err := OpenAndValidateFile(tt.path, 0)
			if f != nil {
				f.Close()
			}
			if tt.wantErr {
				if err == nil {
					t.Fatalf("OpenAndValidateFile(%q) error = nil, want refusal", tt.path)
				}
				if !strings.Contains(err.Error(), "outside allowed") {
					t.Errorf("OpenAndValidateFile(%q) error = %q, want it to name the allow-list", tt.path, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenAndValidateFile(%q) error = %v, want success", tt.path, err)
			}
		})
	}
}

// TestOpenAndValidateFile_ConfiguredUploadDir_Accepted verifies that an
// operator can extend the read allow-list with GITLAB_MCP_ALLOWED_UPLOAD_DIRS
// so a file outside the workspace remains uploadable when they say so.
func TestOpenAndValidateFile_ConfiguredUploadDir_Accepted(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	configured := filepath.Join(root, "assets")
	makeDirs(t, allowed, configured)
	path := filepath.Join(configured, "logo.png")
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	confineLocalPathRoots(t, allowed)

	if f, _, err := OpenAndValidateFile(path, 0); err == nil {
		f.Close()
		t.Fatal("OpenAndValidateFile() error = nil before the directory is allow-listed, want refusal")
	}

	t.Setenv(UploadDirAllowlistEnv, configured)
	f, _, err := OpenAndValidateFile(path, 0)
	if err != nil {
		t.Fatalf("OpenAndValidateFile() error = %v, want success once %s names the directory", err, UploadDirAllowlistEnv)
	}
	f.Close()
}

// TestOpenAndValidateFile_HTTPTransport_Refused verifies that a server reached
// over HTTP refuses local file reads outright: the caller is remote and has no
// files on this machine, so file_path can only ever name someone else's.
func TestOpenAndValidateFile_HTTPTransport_Refused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upload.txt")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	SetLocalFilesystemAccess(false)
	t.Cleanup(func() { SetLocalFilesystemAccess(true) })

	f, _, err := OpenAndValidateFile(path, 0)
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("OpenAndValidateFile() error = nil in HTTP mode, want refusal")
	}
	if !strings.Contains(err.Error(), "content_base64") {
		t.Errorf("OpenAndValidateFile() error = %q, want it to point at content_base64", err)
	}
}

// TestCanonicalDownloadOutputPath_Containment verifies that a download
// destination is confined to the allow-listed roots, that a symlink anywhere
// on the path cannot redirect the write, and that a rejected destination
// leaves nothing behind on disk.
func TestCanonicalDownloadOutputPath_Containment(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	existing := filepath.Join(outside, ".gitlab-mcp-server.env")
	if err := os.WriteFile(existing, []byte("GITLAB_URL=https://gitlab.example\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	linkDir := filepath.Join(allowed, "escape")
	symlinked := os.Symlink(outside, linkDir) == nil

	confineLocalPathRoots(t, allowed)

	tests := []struct {
		name    string
		path    string
		wantErr bool
		skip    bool
	}{
		{name: "nested destination inside the allowed root is accepted", path: filepath.Join(allowed, "build", "artifact.bin")},
		{name: "absolute destination outside every allowed root is refused", path: existing, wantErr: true},
		{name: "parent traversal out of the allowed root is refused", path: filepath.Join(allowed, "..", "home", "stolen.bin"), wantErr: true},
		{name: "symlinked parent directory pointing outside is refused", path: filepath.Join(linkDir, "planted.env"), wantErr: true, skip: !symlinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			_, beforeErr := os.Lstat(tt.path)
			got, err := CanonicalDownloadOutputPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("CanonicalDownloadOutputPath(%q) = %q, want refusal", tt.path, got)
				}
				_, afterErr := os.Lstat(tt.path)
				if errors.Is(beforeErr, os.ErrNotExist) && !errors.Is(afterErr, os.ErrNotExist) {
					t.Errorf("os.Lstat(%q) error = %v after refusal, want nothing created", tt.path, afterErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CanonicalDownloadOutputPath(%q) error = %v, want success", tt.path, err)
			}
			if _, statErr := os.Lstat(filepath.Dir(got)); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("os.Lstat(parent of %q) error = %v, want the parent still uncreated", got, statErr)
			}
		})
	}
}

// TestCanonicalDownloadOutputPath_RejectsNonRegularDestination verifies that a
// destination that already exists as something other than a regular file — a
// symlink, most importantly — is refused rather than followed.
func TestCanonicalDownloadOutputPath_RejectsNonRegularDestination(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	target := filepath.Join(outside, "authorized_keys")
	if err := os.WriteFile(target, []byte("ssh-ed25519 AAAA\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(allowed, "artifact.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	confineLocalPathRoots(t, allowed)

	if got, err := CanonicalDownloadOutputPath(link); err == nil {
		t.Fatalf("CanonicalDownloadOutputPath(symlink) = %q, want refusal", got)
	}
}

// TestCanonicalDownloadOutputPath_RefusalsBeforeContainment verifies the three
// refusals a download destination meets before the allow-list is consulted: an
// empty path, a path running through a regular file as though it were a
// directory, which the ancestor walk cannot resolve and must not take for a
// path that merely does not exist yet, and an existing directory, which a
// download would have to replace to write.
func TestCanonicalDownloadOutputPath_RefusalsBeforeContainment(t *testing.T) {
	root := t.TempDir()
	confineLocalPathRoots(t, root)
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("notes"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	existingDir := filepath.Join(root, "downloads")
	makeDirs(t, existingDir)
	canonicalExistingDir, evalErr := filepath.EvalSymlinks(existingDir)
	if evalErr != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", existingDir, evalErr)
	}

	tests := []struct {
		name       string
		path       string
		wantPrefix string
	}{
		{name: "an empty path", path: "", wantPrefix: "output path is required"},
		{name: "a path through a regular file", path: filepath.Join(file, "sub", "artifact.bin"), wantPrefix: "resolve output path " + filepath.Join(file, "sub", "artifact.bin") + ": "},
		{name: "an existing directory", path: existingDir, wantPrefix: "output path " + canonicalExistingDir + " already exists and is not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalDownloadOutputPath(tt.path)
			if err == nil || !strings.HasPrefix(err.Error(), tt.wantPrefix) {
				t.Fatalf("CanonicalDownloadOutputPath(%q) = (%q, %v), want an error starting %q", tt.path, got, err, tt.wantPrefix)
			}
			if got != "" {
				t.Errorf("CanonicalDownloadOutputPath(%q) = %q beside the error, want the empty string", tt.path, got)
			}
		})
	}
}

// TestJoinBelowDirectory_RefusesAnAncestorThatIsNotADirectory verifies the
// check the ancestor walk ends in: a tail is rejoined only below a directory,
// and a regular file or a vanished ancestor is refused. On Linux the walk
// never reaches a regular file, since a path through one fails with ENOTDIR
// first, so the check is driven directly here; on Windows it is what makes a
// path through a regular file fail at all, which the download refusal test
// above asserts end to end.
func TestJoinBelowDirectory_RefusesAnAncestorThatIsNotADirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("notes"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	tail := filepath.Join("sub", "artifact.bin")

	tests := []struct {
		name       string
		dir        string
		want       string
		wantErr    string
		wantNotExt bool
	}{
		{name: "a directory", dir: root, want: filepath.Join(root, tail)},
		{name: "a regular file", dir: file, wantErr: file + " is not a directory"},
		{name: "an ancestor that vanished", dir: filepath.Join(root, "gone"), wantNotExt: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := joinBelowDirectory(tt.dir, tail)
			switch {
			case tt.wantErr != "":
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("joinBelowDirectory(%q) = (%q, %v), want the error %q", tt.dir, got, err, tt.wantErr)
				}
			case tt.wantNotExt:
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("joinBelowDirectory(%q) = (%q, %v), want an error wrapping fs.ErrNotExist", tt.dir, got, err)
				}
			default:
				if err != nil || got != tt.want {
					t.Fatalf("joinBelowDirectory(%q) = (%q, %v), want (%q, nil)", tt.dir, got, err, tt.want)
				}
				return
			}
			if got != "" {
				t.Errorf("joinBelowDirectory(%q) = %q beside the error, want the empty string", tt.dir, got)
			}
		})
	}
}

// TestCanonicalLocalDirPath_RefusalsBeforeResolution verifies the two
// refusals a directory path meets before anything on disk is looked at: an
// empty path, and any path at all when the server is reached over HTTP, where
// it would name a directory on the server rather than the caller's.
func TestCanonicalLocalDirPath_RefusalsBeforeResolution(t *testing.T) {
	if _, err := CanonicalLocalDirPath(""); err == nil || err.Error() != "directory path is required" {
		t.Errorf("CanonicalLocalDirPath(\"\") error = %v, want the required-path refusal", err)
	}

	SetLocalFilesystemAccess(false)
	t.Cleanup(func() { SetLocalFilesystemAccess(true) })
	got, err := CanonicalLocalDirPath(t.TempDir())
	if err == nil || !strings.HasPrefix(err.Error(), "directory_path is disabled when the server is reached over HTTP") {
		t.Errorf("CanonicalLocalDirPath() in HTTP mode = (%q, %v), want the HTTP refusal", got, err)
	}
}

// TestLocalFilesystemAccessAllowed_ReportsWhatWasSet verifies that the policy
// query answers with what [SetLocalFilesystemAccess] last stored, both ways,
// since every caller-supplied path is gated on it.
func TestLocalFilesystemAccessAllowed_ReportsWhatWasSet(t *testing.T) {
	original := LocalFilesystemAccessAllowed()
	t.Cleanup(func() { SetLocalFilesystemAccess(original) })

	SetLocalFilesystemAccess(false)
	if LocalFilesystemAccessAllowed() {
		t.Error("LocalFilesystemAccessAllowed() = true after SetLocalFilesystemAccess(false)")
	}
	SetLocalFilesystemAccess(true)
	if !LocalFilesystemAccessAllowed() {
		t.Error("LocalFilesystemAccessAllowed() = false after SetLocalFilesystemAccess(true)")
	}
}

// TestCanonicalImportArchivePath_HTTPTransport_Refused verifies that an import
// archive path answers to the same transport rule as file_path: a remote
// caller never placed an archive on this machine.
func TestCanonicalImportArchivePath_HTTPTransport_Refused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project-export.tar.gz")
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	SetLocalFilesystemAccess(false)
	t.Cleanup(func() { SetLocalFilesystemAccess(true) })

	if got, err := CanonicalImportArchivePath(path); err == nil {
		t.Fatalf("CanonicalImportArchivePath() = %q in HTTP mode, want refusal", got)
	}
}

// TestCanonicalDownloadOutputPath_HTTPTransport_Refused verifies that a server
// reached over HTTP writes nothing to its own disk on a remote caller's word.
func TestCanonicalDownloadOutputPath_HTTPTransport_Refused(t *testing.T) {
	dir := t.TempDir()
	SetLocalFilesystemAccess(false)
	t.Cleanup(func() { SetLocalFilesystemAccess(true) })

	if got, err := CanonicalDownloadOutputPath(filepath.Join(dir, "artifact.bin")); err == nil {
		t.Fatalf("CanonicalDownloadOutputPath() = %q in HTTP mode, want refusal", got)
	}
}

// TestCanonicalLocalDirPath_Containment verifies that a caller-supplied
// directory (package.publish_directory) is confined the same way a single
// file path is, and that a symlinked directory cannot escape the allow-list.
func TestCanonicalLocalDirPath_Containment(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "secrets")
	makeDirs(t, allowed, outside)
	link := filepath.Join(allowed, "linked")
	symlinked := os.Symlink(outside, link) == nil
	confineLocalPathRoots(t, allowed)

	tests := []struct {
		name    string
		path    string
		wantErr bool
		skip    bool
	}{
		{name: "directory inside the allowed root is accepted", path: allowed},
		{name: "directory outside every allowed root is refused", path: outside, wantErr: true},
		{name: "symlinked directory pointing outside is refused", path: link, wantErr: true, skip: !symlinked},
		{name: "missing directory is refused", path: filepath.Join(allowed, "absent"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			got, err := CanonicalLocalDirPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("CanonicalLocalDirPath(%q) = %q, want refusal", tt.path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("CanonicalLocalDirPath(%q) error = %v, want success", tt.path, err)
			}
		})
	}
}

// TestAllowedLocalDirs_SkipsFilesystemRootWorkingDirectory verifies that a
// server started with the filesystem root as its working directory — which is
// what Claude Desktop does — does not thereby allow-list the whole disk.
func TestAllowedLocalDirs_SkipsFilesystemRootWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the filesystem root is spelled per volume on Windows")
	}
	testutil.IsolateTempDir(t, t.TempDir())
	t.Chdir("/")

	for _, dir := range allowedLocalDirs(UploadDirAllowlistEnv) {
		if dir == "/" {
			t.Fatal(`allowedLocalDirs() included "/", want the filesystem root skipped`)
		}
	}
}

// useHomeDir points userHomeDir at dir for the duration of the test.
//
// The function is replaced rather than $HOME set, because os.UserHomeDir reads
// a different variable per platform and this test is about the policy, not
// about which variable a platform consults.
func useHomeDir(t *testing.T, dir string) {
	t.Helper()
	original := userHomeDir
	userHomeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userHomeDir = original })
}

// TestAllowedLocalDirs_SkipsHomeWorkingDirectory verifies that a server whose
// working directory is the user's home directory does not thereby allow-list
// the whole home directory.
//
// It is the same argument as the filesystem-root case one level down, and it is
// not hypothetical: the home directory holds ~/.ssh, ~/.aws, the browser
// profiles, and this server's own ~/.gitlab-mcp-server.env. Implicitly
// allow-listing it would mean a file_path naming that last file uploads the
// GITLAB_TOKEN the containment exists to protect.
func TestAllowedLocalDirs_SkipsHomeWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	testutil.IsolateTempDir(t, t.TempDir())
	useHomeDir(t, home)
	t.Chdir(home)

	canonicalHome, err := canonicalDirPath(home)
	if err != nil {
		t.Fatalf("canonicalDirPath(%q) error = %v", home, err)
	}
	for _, dir := range allowedLocalDirs(UploadDirAllowlistEnv) {
		if dir == canonicalHome {
			t.Fatalf("allowedLocalDirs() included the home directory %q, want it skipped", canonicalHome)
		}
	}
}

// TestAllowedLocalDirs_SaysWhyTheHomeRootWasDropped verifies the operator is
// told, and told what to do about it.
//
// The warning is the half that makes the safe default acceptable. Without it a
// user whose workspace is their home directory sees a working setup start
// failing with "outside the allowed directories" and has nothing pointing at
// the cause or the one-variable remedy.
func TestAllowedLocalDirs_SaysWhyTheHomeRootWasDropped(t *testing.T) {
	home := t.TempDir()
	testutil.IsolateTempDir(t, t.TempDir())
	useHomeDir(t, home)
	t.Chdir(home)

	var buf bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	warnedBefore := homeDirWarned.Swap(false)
	t.Cleanup(func() {
		slog.SetDefault(originalLogger)
		homeDirWarned.Store(warnedBefore)
	})

	allowedLocalDirs(UploadDirAllowlistEnv)
	allowedLocalDirs(DownloadDirAllowlistEnv)

	logged := buf.String()
	for _, want := range []string{
		"working directory is the home directory",
		UploadDirAllowlistEnv,
		DownloadDirAllowlistEnv,
		ImportArchiveAllowlistEnv,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(logged, want) {
				t.Errorf("warning = %q, want it to mention %q", logged, want)
			}
		})
	}
	if lines := strings.Count(strings.TrimSpace(logged), "\n") + 1; lines != 1 {
		t.Errorf("warning lines = %d, want exactly 1 for two allowlist lookups: %s", lines, logged)
	}
}

// TestAllowedLocalDirs_HomeIsReachableWhenTheOperatorNamesIt verifies the home
// directory is skipped as an *implicit* root and not forbidden.
//
// An operator whose workspace really is their home directory has a one-variable
// remedy, which is what makes the safe default acceptable: without this, the
// change would be a policy decision taken on their behalf rather than a default
// they can override.
func TestAllowedLocalDirs_HomeIsReachableWhenTheOperatorNamesIt(t *testing.T) {
	home := t.TempDir()
	testutil.IsolateTempDir(t, t.TempDir())
	useHomeDir(t, home)
	t.Chdir(home)
	t.Setenv(UploadDirAllowlistEnv, home)

	canonicalHome, err := canonicalDirPath(home)
	if err != nil {
		t.Fatalf("canonicalDirPath(%q) error = %v", home, err)
	}
	if !slices.Contains(allowedLocalDirs(UploadDirAllowlistEnv), canonicalHome) {
		t.Errorf("allowedLocalDirs() = %v, want it to contain the explicitly allowed home %q",
			allowedLocalDirs(UploadDirAllowlistEnv), canonicalHome)
	}
}

// TestAllowedLocalDirs_KeepsAnOrdinaryWorkingDirectory verifies the ordinary
// case is untouched: a workspace that is neither the filesystem root nor the
// home directory stays an implicit root.
//
// It is the guard against passing the two tests above by dropping the working
// directory altogether, which would silently break every stdio deployment.
func TestAllowedLocalDirs_KeepsAnOrdinaryWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "workspace")
	if err := os.Mkdir(workspace, 0o750); err != nil {
		t.Fatalf("creating the workspace: %v", err)
	}
	testutil.IsolateTempDir(t, t.TempDir())
	useHomeDir(t, home)
	t.Chdir(workspace)

	canonicalWorkspace, err := canonicalDirPath(workspace)
	if err != nil {
		t.Fatalf("canonicalDirPath(%q) error = %v", workspace, err)
	}
	if !slices.Contains(allowedLocalDirs(UploadDirAllowlistEnv), canonicalWorkspace) {
		t.Errorf("allowedLocalDirs() = %v, want it to contain the working directory %q",
			allowedLocalDirs(UploadDirAllowlistEnv), canonicalWorkspace)
	}
}

// TestSkipHomeAsImplicitRoot_KeepsTheWorkingDirectoryWhenHomeIsUnknown verifies
// the failure direction.
//
// A platform where the home directory cannot be resolved must keep the working
// directory as a root: that is the behavior every deployment already has, and
// narrowing the allow-list on a lookup failure would break file_path for
// reasons no operator could see.
func TestSkipHomeAsImplicitRoot_KeepsTheWorkingDirectoryWhenHomeIsUnknown(t *testing.T) {
	tests := []struct {
		name string
		home func() (string, error)
	}{
		{name: "lookup fails", home: func() (string, error) { return "", errors.New("no home directory") }},
		{name: "lookup returns nothing", home: func() (string, error) { return "", nil }},
		{name: "home does not exist", home: func() (string, error) { return filepath.Join(t.TempDir(), "absent"), nil }},
	}
	t.Run("the working directory does not resolve", func(t *testing.T) {
		home := t.TempDir()
		original := userHomeDir
		userHomeDir = func() (string, error) { return home, nil }
		t.Cleanup(func() { userHomeDir = original })

		// Compared as written, a working directory that is gone cannot be the
		// home directory, so the answer stays the one every deployment had.
		if skipHomeAsImplicitRoot(filepath.Join(home, "removed")) {
			t.Error("skipHomeAsImplicitRoot() = true for a working directory that does not resolve, want false")
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := userHomeDir
			userHomeDir = tt.home
			t.Cleanup(func() { userHomeDir = original })

			workspace := t.TempDir()
			if skipHomeAsImplicitRoot(workspace) {
				t.Error("skipHomeAsImplicitRoot() = true, want the working directory kept")
			}
		})
	}
}

// TestSkipHomeAsImplicitRoot_ResolvesTheWorkingDirectoryThroughSymlinks
// verifies that both sides of the comparison are canonical paths.
//
// A server started through a symlink to the home directory is still running in
// the home directory, and keeping it as an implicit allow-list root there is
// exactly what this check exists to prevent: it would put ~/.ssh, ~/.aws and
// this server's own ~/.gitlab-mcp-server.env within reach of a caller-supplied
// file_path.
func TestSkipHomeAsImplicitRoot_ResolvesTheWorkingDirectoryThroughSymlinks(t *testing.T) {
	home := t.TempDir()
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(home, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	original := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = original })

	if !skipHomeAsImplicitRoot(link) {
		t.Errorf("skipHomeAsImplicitRoot(%q) = false, want true: it resolves to the home directory %q", link, home)
	}
}

// TestHTTPTransportConfigured verifies how the default local-filesystem policy
// reads the process arguments: the server is remote-facing when --http is
// present in any of the forms Go's flag package accepts, and local otherwise.
func TestHTTPTransportConfigured(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "an empty argument list is stdio", args: nil},
		{name: "no arguments is stdio", args: []string{"gitlab-mcp-server"}},
		{name: "a positional argument before the flag is passed over", args: []string{"gitlab-mcp-server", "serve", "--http"}, want: true},
		{name: "single dash http", args: []string{"gitlab-mcp-server", "-http"}, want: true},
		{name: "double dash http", args: []string{"gitlab-mcp-server", "--http"}, want: true},
		{name: "explicit true value", args: []string{"gitlab-mcp-server", "--http=true"}, want: true},
		{name: "explicit false value", args: []string{"gitlab-mcp-server", "--http=false"}},
		{name: "http-addr alone does not enable http", args: []string{"gitlab-mcp-server", "--http-addr=:8080"}},
		{name: "arguments after the terminator are not flags", args: []string{"gitlab-mcp-server", "--", "--http"}},
		{name: "test binary flags are stdio", args: []string{"toolutil.test", "-test.timeout=10m", "-test.v=true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := httpTransportConfigured(tt.args); got != tt.want {
				t.Errorf("httpTransportConfigured(%q) = %t, want %t", tt.args, got, tt.want)
			}
		})
	}
}

// TestCreateDownloadOutputFile_CreatesAndTruncates verifies that the download
// destination helper creates a missing file and truncates an existing one, so
// closing the symlink race at the creation costs a download nothing it did
// before. The refusal itself is platform-specific and is pinned in
// file_utils_unix_test.go.
func TestCreateDownloadOutputFile_CreatesAndTruncates(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.bin")
	if err := os.WriteFile(existing, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "destination does not exist", path: filepath.Join(dir, "fresh.bin")},
		{name: "destination already holds a longer file", path: existing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := CreateDownloadOutputFile(tt.path)
			if err != nil {
				t.Fatalf("CreateDownloadOutputFile(%q) error = %v, want nil", tt.path, err)
			}
			if _, err = f.WriteString("new"); err != nil {
				t.Errorf("write error = %v, want nil", err)
			}
			if err = f.Close(); err != nil {
				t.Errorf("close error = %v, want nil", err)
			}
			got, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read back error = %v, want nil", err)
			}
			if string(got) != "new" {
				t.Errorf("file content = %q, want %q", got, "new")
			}
		})
	}
}
