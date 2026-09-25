// The writing half: the spec this record hands the shared shard mechanism, and
// the three calls that reach it.

package e2ecalls

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/shardio"
)

// shards is this record's shard mechanism: one registry, one seen set per
// directory, and the policies [shardio] decides for every record alike.
//
// The spec carries content and nothing else. What a failure costs is not a
// choice made here: a line's own failure costs that line, and a directory or a
// file the writer cannot use stops it, for this record as for the other.
var shards = newShards(Record.validate)

// newShards returns this record's shard mechanism holding every line it reads
// to validate. [shards] holds each line to the current schema and is the one
// every writer comes from; [callShards] admits the older schemas
// [ReadShardsForCalls] reads and is never written through, so the two differ
// in that rule alone and cannot come to disagree about a shard's name.
func newShards(validate func(Record) error) *shardio.Shards[Record, Line] {
	return shardio.New(shardio.Spec[Record, Line]{
		DirEnv: DirEnv,
		Prefix: shardPrefix,
		Ext:    shardExt,
		Noun:   "an e2e call",
		TypeOf: func(r Record) string { return r.Type },
		// Envelope is a closure over the unexported method, so the closed set
		// of line types argued at [Line] stays closed: nothing is exported to
		// make the shared mechanism generic over this record.
		Envelope: func(l Line) Record { return l.record() },
		Validate: validate,
		// Check is nil and CapHint empty on purpose. Every field of this
		// record is a string, a bool, an int, a string slice or one float the
		// harness computes from a [time.Duration], so no line here carries
		// JSON somebody else wrote, and none is within two orders of magnitude
		// of the line cap.
	})
}

// Reporter is the part of [testing.TB] the writer reports a broken shard
// through. See [shardio.Reporter] for why it is an interface and why it is
// taken per call.
type Reporter = shardio.Reporter

// Writer appends lines to this process's shard of the call record.
//
// The zero value is not usable: [Open] and [OpenDir] make one. A nil *Writer is
// usable and does nothing, which is what recording being off looks like to a
// caller, so the harness has no branch of its own to get wrong.
type Writer = shardio.Writer[Record, Line]

// Open returns the writer for the directory [DirEnv] names, or nil when
// recording is off.
func Open() *Writer { return shards.Open() }

// OpenDir returns the writer recording into dir, or nil when dir is empty.
func OpenDir(dir string) *Writer { return shards.OpenDir(dir) }

// Release closes every open shard and forgets it, so a later [Open] of the same
// directory starts a new one. Windows is why it exists: a directory holding an
// open file cannot be removed.
func Release() { shards.Release() }
