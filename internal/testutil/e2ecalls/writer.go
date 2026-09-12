// The writing half: one shard per test process, opened when the first line is
// written and never closed, because there is no shutdown hook to close it in.

package e2ecalls

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// shardDirPerm is the mode the shard directory is created with. It holds
// nothing secret, and nothing reads it except the merge that runs as the same
// user.
const shardDirPerm = 0o750

// Reporter is the part of [testing.TB] the writer reports a broken shard
// through.
//
// It is an interface rather than the concrete type for two reasons. This
// package must not import testing: it is compiled into an ordinary build and
// linked by cmd/audit_e2e_coverage, and a library that drags the testing
// package into a command is a library nobody can use from one. And a reporter
// is taken per call rather than held, because one shard is shared by every
// test in the process while a failure belongs to whichever test was writing
// when it happened.
type Reporter interface {
	// Errorf reports a failure without ending the test, the way
	// [testing.T.Errorf] does.
	Errorf(format string, args ...any)
}

// writers holds one writer per directory, so the many sessions and tests of
// one process share one shard file and one seen set.
//
// Keying on the directory rather than caching the first lookup keeps
// [testing.T.Setenv] meaningful in this package's own tests.
var (
	writersMu sync.Mutex
	writers   = map[string]*Writer{}
)

// Writer appends lines to this process's shard of the call record.
//
// The zero value is not usable: [Open] and [OpenDir] make one. A nil *Writer
// is usable and does nothing, which is what recording being off looks like to
// a caller, so the harness has no branch of its own to get wrong.
type Writer struct {
	dir string

	mu   sync.Mutex
	file *os.File
	// seen holds the lines already written, so a line repeated by a retry or
	// by two sessions of one shape costs a map lookup and no write.
	seen map[string]bool
	// stopped is set once something has gone wrong and been reported, so a
	// broken directory produces one failure and not one per line.
	stopped bool
}

// Open returns the writer for the directory [DirEnv] names, or nil when
// recording is off.
func Open() *Writer {
	return OpenDir(os.Getenv(DirEnv))
}

// OpenDir returns the writer recording into dir, or nil when dir is empty.
//
// Nothing is created here: the shard is opened when the first line is written,
// so a process that records nothing leaves no empty file behind for the reader
// to count as a run.
func OpenDir(dir string) *Writer {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	writersMu.Lock()
	defer writersMu.Unlock()
	if existing, ok := writers[dir]; ok {
		return existing
	}
	created := &Writer{dir: dir, seen: map[string]bool{}}
	writers[dir] = created
	return created
}

// Release closes every open shard and forgets it, so a later [Open] of the
// same directory starts a new one.
//
// Nothing in a real run needs this: one shard belongs to one test process and
// the operating system closes it at exit. Windows needs it in a test, where a
// directory holding an open file cannot be removed and a test recording into
// t.TempDir() would fail in cleanup after every one of its own assertions
// passed.
func Release() {
	writersMu.Lock()
	defer writersMu.Unlock()
	for dir, writer := range writers {
		writer.release()
		delete(writers, dir)
	}
}

// Write appends each line to the shard, reporting through reporter if the
// shard cannot be written.
//
// A line already written is dropped. A nil writer writes nothing, and neither
// does a writer that has already reported a failure.
func (w *Writer) Write(reporter Reporter, lines ...Line) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, line := range lines {
		w.writeLine(reporter, line)
	}
}

// writeLine appends one line. The caller holds w.mu.
func (w *Writer) writeLine(reporter Reporter, line Line) {
	if w.stopped {
		return
	}
	encoded, err := json.Marshal(line.record())
	if err != nil {
		w.stopf(reporter, "could not encode an e2e call line: %v", err)
		return
	}
	text := string(encoded)
	if w.seen[text] {
		return
	}
	if w.file == nil {
		if !filepath.IsAbs(w.dir) {
			w.stopf(reporter, "%s must be an absolute path, got %q: a test binary runs in its own package directory, so a relative one writes a shard nothing will find", DirEnv, w.dir)
			return
		}
		file, openErr := createShard(w.dir)
		if openErr != nil {
			w.stopf(reporter, "could not open an e2e call shard in %s: %v", w.dir, openErr)
			return
		}
		w.file = file
	}
	// The write is deliberately unbuffered, and that is what makes the shard
	// safe to never close: the writer is process-global with no shutdown hook
	// to hang a Close on, so a buffer would lose whatever its tail held when
	// the test binary exits. One syscall per new line is cheap because a line
	// is written once however many times it is offered.
	if _, writeErr := w.file.WriteString(text + "\n"); writeErr != nil {
		w.stopf(reporter, "could not write an e2e call shard: %v", writeErr)
		return
	}
	w.seen[text] = true
}

// stopf reports one failure and silences the writer for the rest of the run.
// The caller holds w.mu.
func (w *Writer) stopf(reporter Reporter, format string, args ...any) {
	w.stopped = true
	reporter.Errorf(format, args...)
}

// release closes this writer's shard, if it opened one.
func (w *Writer) release() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
}

// createShard opens this process's shard file.
//
// It is a variable so the failure branch is reachable from a test: the suite
// runs as root often enough that a directory made read-only is not read-only.
var createShard = func(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, shardDirPerm); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, ShardPattern)
}
