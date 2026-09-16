// The writing half: one shard per test process, opened when the first line is
// written and never closed, because there is no shutdown hook to close it in.

package shardio

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Reporter is the part of [testing.TB] the writer reports a broken shard
// through.
//
// It is an interface rather than the concrete type for two reasons. Neither
// this package nor the records built on it may import testing: they are
// compiled into an ordinary build and linked by ordinary commands, and a
// library that drags the testing package into a command is a library nobody can
// use from one. And a reporter is taken per call rather than held, because one
// shard is shared by every caller in the process while a failure belongs to
// whichever one was writing when it happened.
type Reporter interface {
	// Errorf reports a failure without ending the test, the way
	// [testing.T.Errorf] does.
	Errorf(format string, args ...any)
}

// Writer appends lines to this process's shard of one record.
//
// The zero value is not usable: [Shards.Open] and [Shards.OpenDir] make one. A
// nil *Writer is usable and does nothing, which is what recording being off
// looks like to a caller, so the caller has no branch of its own to get wrong.
type Writer[R, L any] struct {
	shards *Shards[R, L]
	dir    string

	mu   sync.Mutex
	file *os.File
	// seen holds the digest of every line already written, so a line repeated
	// by a retry, by two sessions of one shape, or by a flush that runs both at
	// the end of a test and at the end of the run costs a map lookup and no
	// write.
	//
	// It absorbs a double write and never collapses a genuine repetition:
	// every line carries something that makes a second occurrence a different
	// line, an index, a trace, a test name. A caller that really did the same
	// thing twice is paying an overhead the record has to show.
	//
	// The digest rather than the text is what is kept, because a line may carry
	// a result or a rendered answer of tens of kilobytes, and keeping the text
	// would hold a long run's whole shard in memory beside the file it was
	// already written to. Thirty-two bytes a line answers the same question:
	// the digest of the exact encoded bytes is the same equality as the bytes.
	seen map[[sha256.Size]byte]bool
	// stopped is set once something the writer cannot recover from has gone
	// wrong and been reported, so a broken directory produces one failure and
	// not one per line.
	stopped bool
}

// Open returns the writer for the directory [Spec.DirEnv] names, or nil when
// recording is off.
func (s *Shards[R, L]) Open() *Writer[R, L] {
	return s.OpenDir(os.Getenv(s.spec.DirEnv))
}

// OpenDir returns the writer recording into dir, or nil when dir is empty.
//
// Nothing is created here: the shard is opened when the first line is written,
// so a process that records nothing leaves no empty file behind for the reader
// to count as a run.
func (s *Shards[R, L]) OpenDir(dir string) *Writer[R, L] {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.writers[dir]; ok {
		return existing
	}
	created := &Writer[R, L]{shards: s, dir: dir, seen: map[[sha256.Size]byte]bool{}}
	s.writers[dir] = created
	return created
}

// Release closes every shard this record has open and forgets it, so a later
// [Shards.Open] of the same directory starts a new one.
//
// Nothing in a real run needs this: one shard belongs to one test process and
// the operating system closes it at exit. Windows needs it in a test, where a
// directory holding an open file cannot be removed and a test recording into
// t.TempDir() would fail in cleanup after every one of its own assertions
// passed.
func (s *Shards[R, L]) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for dir, writer := range s.writers {
		writer.release()
		delete(s.writers, dir)
	}
}

// Write appends each line to the shard, reporting through reporter if a line
// cannot be recorded or the shard cannot be written.
//
// A line already written is dropped. A line the shard cannot hold is reported
// and skipped, and the lines offered after it are still written. A nil writer
// writes nothing, and neither does a writer whose directory or file has already
// failed.
func (w *Writer[R, L]) Write(reporter Reporter, lines ...L) {
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
//
// The three failures a line brings on itself, a raw field [Spec.Check] refuses,
// JSON the encoder cannot hold and a length past [MaxLine], are reported and
// cost that line alone. Everything the writer cannot recover from, a directory
// it may not use and a file that stops accepting, stops it.
//
// The split is what makes the record survivable. A line filled from somebody
// else's output fails for ordinary reasons, and one of them silencing the shard
// would take every later line with it: the rest of the run, and the run line
// written from an exit hook, which is the only thing that joins a shard to its
// commit, its instance and its tier. Losing that makes everything already
// recorded unpublishable too, and the writer says so nowhere, because a stopped
// writer reports once and then returns in silence.
func (w *Writer[R, L]) writeLine(reporter Reporter, line L) {
	if w.stopped {
		return
	}
	spec := w.shards.spec
	record := spec.Envelope(line)
	if spec.Check != nil {
		if field := spec.Check(record); field != "" {
			reporter.Errorf("a %s line was not recorded: %s is not JSON that parses, and a raw field can hold nothing else%s", spec.TypeOf(record), field, spec.CheckHint)
			return
		}
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		reporter.Errorf("a %s line was not recorded, because it could not be encoded: %v", spec.TypeOf(record), err)
		return
	}
	text := string(encoded)
	// The newline the line is written with counts against the cap, since it is
	// what the reader's scanner has to find inside its bound.
	if len(text)+1 > MaxLine {
		reporter.Errorf("a %s line was not recorded: it is %d bytes with its newline, past the %d-byte line cap the reader is bounded by%s", spec.TypeOf(record), len(text)+1, MaxLine, spec.CapHint)
		return
	}
	digest := sha256.Sum256(encoded)
	if w.seen[digest] {
		return
	}
	if w.file == nil {
		if !filepath.IsAbs(w.dir) {
			w.stopf(reporter, "%s must be an absolute path, got %q: a test binary runs in its own package directory, so a relative one writes a shard nothing will find", spec.DirEnv, w.dir)
			return
		}
		file, openErr := w.shards.createShard(w.dir, w.shards.pattern())
		if openErr != nil {
			w.stopf(reporter, "could not open %s shard in %s: %v", spec.Noun, w.dir, openErr)
			return
		}
		w.file = file
	}
	// The write is deliberately unbuffered, and that is what makes the shard
	// safe to never close: the writer is process-global with no shutdown hook
	// to hang a Close on, so a buffer would lose whatever its tail held when
	// the test binary exits. The tail is the part worth keeping, because it is
	// what the run got to last.
	if _, writeErr := w.file.WriteString(text + "\n"); writeErr != nil {
		w.stopf(reporter, "could not write %s shard: %v", spec.Noun, writeErr)
		return
	}
	w.seen[digest] = true
}

// stopf reports one failure and silences the writer for the rest of the run.
//
// It is for the failures no later line can avoid: a directory this writer may
// not use and a file that no longer accepts writes. A line's own failure is
// reported by writeLine and skipped, so one unencodable line costs one line.
// The caller holds w.mu.
func (w *Writer[R, L]) stopf(reporter Reporter, format string, args ...any) {
	w.stopped = true
	reporter.Errorf(format, args...)
}

// release closes this writer's shard, if it opened one.
func (w *Writer[R, L]) release() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
}
