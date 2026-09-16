// What one record contributes to the mechanism, and the per-record registry
// that holds its writers apart from every other record's.

package shardio

import (
	"os"
	"strings"
	"sync"
)

// MaxLine bounds one recorded line. The writer refuses to write a longer one
// and the reader's scanner refuses to read one.
//
// It is a cap rather than a consequence of whatever a record's fields are
// bounded by, because most of them are bounded by nothing: what a model sent
// and what a server answered are written as they happened, and truncating them
// would corrupt the one thing a record exists for. A megabyte is two orders of
// magnitude above the longest line either record writes today.
//
// Both halves are held to it, and each for its own failure. A line silently
// dropped for being long looks exactly like a call nobody made, which is the
// one thing these records exist to be believed about. A line written past what
// the reader takes is worse: the scanner stops at it, so one long answer loses
// the whole shard, and a shard is everything one process recorded, read back
// long after the run that produced it is gone.
const MaxLine = 1 << 20

// shardDirPerm is the mode a shard directory is created with. It holds nothing
// secret, and nothing reads it except the merge that runs as the same user.
const shardDirPerm = 0o750

// Spec is what one record contributes to the mechanism: what its shards are
// called, how a line becomes a record, and what its own refusals say.
//
// Everything here is content. No field of it selects a policy: what a failure
// costs is decided once, in [Writer.Write], for every record alike.
type Spec[R, L any] struct {
	// DirEnv names the environment variable holding the directory this
	// record's shards are written into. [Shards.Open] reads it, and every
	// refusal that is about the directory names it, so a reader of the failure
	// learns which switch to set.
	DirEnv string
	// Prefix and Ext bracket a shard's file name. They are kept apart rather
	// than given as one pattern because the writer needs the pattern for
	// [os.CreateTemp] and the reader needs the two halves to recognize a name,
	// and deriving either from the other would mean handling a pattern error
	// this package could only discard.
	Prefix string
	Ext    string
	// Noun names the record in the two failures that are about the file, with
	// its article: "an e2e call", "a model record". They read "could not open
	// an e2e call shard in ...".
	Noun string
	// TypeOf names the line type of a record, for the three refusals that are
	// about one line. It is required: a report that says which kind of line was
	// lost is one somebody can act on, and one that does not is only news that
	// something went missing.
	TypeOf func(R) string
	// Envelope wraps one line in the record it is written as. It is a closure
	// the record package builds over its own unexported method, so the closed
	// set of line types stays closed and nothing is exported to make this
	// generic possible.
	Envelope func(L) R
	// Validate reports what is wrong with a record read back from a shard, and
	// is what the reader asks of every line. [ValidateEnvelope] is the half of
	// it every record shares.
	Validate func(R) error
	// Check reports, before the record is encoded, the name of the first field
	// holding raw JSON that does not parse, and "" when every one of them
	// does. It is nil for a record carrying no such field.
	//
	// It exists because a record filled from somebody else's output fails to
	// encode for an ordinary reason, and the encoder's own message for it
	// names a Go type rather than the field a person can act on.
	Check func(R) string
	// CheckHint is appended to the refusal [Spec.Check] raises, to say where
	// the text belongs instead. Empty adds nothing.
	CheckHint string
	// CapHint is appended to the refusal a line past [MaxLine] raises, to name
	// whatever the record offers for capping what a line carries. Empty adds
	// nothing.
	CapHint string
}

// Shards is one record's shard mechanism: its spec, and the writers open on its
// behalf.
//
// The registry is per record rather than global on purpose. Two records that
// ever named one directory would otherwise be handed each other's writer, with
// each other's seen set and each other's shard, and the first symptom would be
// a shard holding lines of two kinds. It is also what keeps one record's
// [Shards.Release] from closing another's file, which the Windows reasoning
// there depends on.
type Shards[R, L any] struct {
	spec Spec[R, L]
	// createShard is a seam so the failure branch is reachable from a test: the
	// suite runs as root often enough that a directory made read-only is not
	// read-only. It is a field rather than a package variable so a test stubs
	// its own registry and never another test's.
	createShard func(dir, pattern string) (*os.File, error)

	mu sync.Mutex
	// writers holds one writer per directory, so the many callers of one
	// process share one shard file and one seen set.
	//
	// Keying on the directory rather than caching the first lookup keeps
	// [testing.T.Setenv] meaningful in a record package's own tests.
	writers map[string]*Writer[R, L]
}

// New returns the mechanism for one record.
func New[R, L any](spec Spec[R, L]) *Shards[R, L] {
	return &Shards[R, L]{
		spec:        spec,
		createShard: createShard,
		writers:     map[string]*Writer[R, L]{},
	}
}

// IsShard reports whether a file name is one of this record's shards.
//
// It matches the two halves of the name rather than calling
// [path/filepath.Match] on the pattern, which would hand back a pattern error
// this package owns the pattern for and could only discard.
func (s *Shards[R, L]) IsShard(name string) bool {
	return strings.HasPrefix(name, s.spec.Prefix) && strings.HasSuffix(name, s.spec.Ext)
}

// pattern is the [os.CreateTemp] pattern a shard file is named with. One
// process writes one shard, so package binaries running side by side never
// write to the same file and no cross-process locking is needed.
func (s *Shards[R, L]) pattern() string {
	return s.spec.Prefix + "*" + s.spec.Ext
}

// createShard opens one shard file in dir, making the directory if it is not
// there.
func createShard(dir, pattern string) (*os.File, error) {
	if err := os.MkdirAll(dir, shardDirPerm); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, pattern)
}
