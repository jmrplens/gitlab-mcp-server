// The writing half: one shard per test process, opened when the first line is
// written and never closed, because there is no shutdown hook to close it in.

package modelrecord

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// shardDirPerm is the mode the shard directory is created with. It holds
// nothing secret, and nothing reads it except the scoring run that follows as
// the same user.
const shardDirPerm = 0o750

// Reporter is the part of [testing.TB] the writer reports a broken shard
// through.
//
// It is an interface rather than the concrete type for two reasons. This
// package must not import testing: it is compiled into an ordinary build and
// linked by cmd/gen_model_results, and a library that drags the testing package
// into a command is a library nobody can use from one. And a reporter is taken
// per call rather than held, because one shard is shared by every attempt in
// the process while a failure belongs to whichever one was writing when it
// happened.
type Reporter interface {
	// Errorf reports a failure without ending the test, the way
	// [testing.T.Errorf] does.
	Errorf(format string, args ...any)
}

// writers holds one writer per directory, so the many attempts and sessions of
// one process share one shard file and one seen set.
//
// Keying on the directory rather than caching the first lookup keeps
// [testing.T.Setenv] meaningful in this package's own tests.
var (
	writersMu sync.Mutex
	writers   = map[string]*Writer{}
)

// Writer appends lines to this process's shard of the model record.
//
// The zero value is not usable: [Open] and [OpenDir] make one. A nil *Writer is
// usable and does nothing, which is what recording being off looks like to a
// caller, so the runner has no branch of its own to get wrong.
type Writer struct {
	dir string

	mu   sync.Mutex
	file *os.File
	// seen holds the digest of every line already written, so a line offered
	// twice by a flush that runs at the end of an attempt and again at the end
	// of the run costs a map lookup and no write.
	//
	// It absorbs a double write and never collapses a repeat: every line type
	// carries something that makes a genuine repetition a different line, the
	// turn its index and try, the call its index among the attempt's calls. A
	// model that sends the same call twice is paying an overhead this record
	// has to show.
	//
	// The digest rather than the line is what differs from the coverage
	// record's own seen set, and the reason is the content: a call line here
	// carries a result and a rendered answer, each bounded at 64 KiB and each
	// able to escape to several times that, so keeping the text would hold a
	// long run's whole shard in memory beside the file it was already written
	// to. Thirty-two bytes a line answers the same question.
	seen map[[sha256.Size]byte]bool
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
// to count as a run that produced no attempts.
func OpenDir(dir string) *Writer {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	// Cleaned before it becomes the key, because the key is what makes one
	// directory one writer. Two spellings of one path ("shards" and "./shards")
	// would otherwise open two writers over the same files, each with a seen set
	// the other cannot consult, and Read concatenates shards without
	// deduplicating: the duplicate observations would survive into scoring.
	dir = filepath.Clean(dir)
	writersMu.Lock()
	defer writersMu.Unlock()
	if existing, ok := writers[dir]; ok {
		return existing
	}
	created := &Writer{dir: dir, seen: map[[sha256.Size]byte]bool{}}
	writers[dir] = created
	return created
}

// Release closes every open shard and forgets it, so a later [Open] of the same
// directory starts a new one.
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

// Write appends each line to the shard, reporting through reporter if a line
// cannot be recorded or the shard cannot be written.
//
// A line already written is dropped. A line the shard cannot hold is reported
// and skipped, and the lines offered after it are still written. A nil writer
// writes nothing, and neither does a writer whose directory or file has already
// failed.
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
//
// The two failures a line brings on itself, JSON it cannot hold and a length
// past [maxShardLine], are reported and cost that line alone. Everything the
// writer cannot recover from, a directory it may not use and a file that stops
// accepting, stops it. The split is the whole point: a malformed tool call is
// ordinary here, since [Block.Arguments], [Call.Arguments] and [Call.Result] are
// filled from a model's own output, and one of them silencing the shard would
// take every later attempt, its verify lines and the run line with it. The run
// line is written from an exit hook and is the only thing that joins a shard to
// its commit, its instance and its tier, so losing it makes everything before it
// unpublishable too.
func (w *Writer) writeLine(reporter Reporter, line Line) {
	if w.stopped {
		return
	}
	record := line.record()
	if field := record.invalidRawField(); field != "" {
		reporter.Errorf("a %s line was not recorded: %s is not JSON that parses, and a raw field can hold nothing else; the text of a tool call that does not parse belongs in a block's raw field", record.Type, field)
		return
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		reporter.Errorf("a %s line was not recorded, because it could not be encoded: %v", record.Type, err)
		return
	}
	text := string(encoded)
	// The newline the line is written with counts against the cap, since it is
	// what the reader's scanner has to find inside its bound.
	if len(text)+1 > maxShardLine {
		reporter.Errorf("a %s line was not recorded: it is %d bytes with its newline, past the %d-byte line cap the reader is bounded by; cap what it carries with CapResult or CapText", record.Type, len(text)+1, maxShardLine)
		return
	}
	digest := sha256.Sum256(encoded)
	if w.seen[digest] {
		return
	}
	if w.file == nil {
		if !filepath.IsAbs(w.dir) {
			w.stopf(reporter, "%s must be an absolute path, got %q: a test binary runs in its own package directory, so a relative one writes a shard nothing will find", DirEnv, w.dir)
			return
		}
		file, openErr := createShard(w.dir)
		if openErr != nil {
			w.stopf(reporter, "could not open a model record shard in %s: %v", w.dir, openErr)
			return
		}
		w.file = file
	}
	// The write is deliberately unbuffered, and that is what makes the shard
	// safe to never close: the writer is process-global with no shutdown hook
	// to hang a Close on, so a buffer would lose whatever its tail held when
	// the test binary exits. Losing the tail is worse here than it is for a
	// coverage record, because the tail of a model run is the attempts that
	// were paid for last.
	if _, writeErr := w.file.WriteString(text + "\n"); writeErr != nil {
		w.stopf(reporter, "could not write a model record shard: %v", writeErr)
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
