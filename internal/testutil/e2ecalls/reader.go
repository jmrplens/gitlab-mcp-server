// The reading half: every shard of a directory tree, merged in one pass, with
// a line nobody can read reported rather than dropped.

package e2ecalls

import (
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/shardio"
)

// Shard is one shard file read back: where it was, and every line it held.
//
// It exists because a shard is the one unit that says which process wrote a
// line. No call, skip or session line names its package, only the run line
// does, and one process writes one shard, so a reader that wants to place a
// call in its package reads the shards apart and joins each to the run line
// beside it. Merged, that attribution is gone.
type Shard = shardio.Shard[Record]

// ReadShards reads every shard under dir, subdirectories included, one entry
// per file in the order the directory tree walks.
//
// Subdirectories are read because one run per Docker target writes into a
// directory of its own, and the coverage audit compares the runtimes against
// each other. A directory holding no shard is an error rather than an empty
// result, since reporting nothing recorded as zero coverage would be a claim
// about the server made from a claim about the harness.
func ReadShards(dir string) ([]Shard, error) { return shards.ReadShards(dir) }

// callShards is the mechanism [ReadShardsForCalls] reads through: the same
// shards under the same names, held to [Record.validateForCalls] rather than
// to [Record.validate]. Nothing writes through it.
var callShards = newShards(Record.validateForCalls)

// ReadShardsForCalls is [ReadShards] for a reader of the run, call, dispatch
// and skip lines alone, such as a comparison of what two runs credited.
//
// It accepts a line written under any schema from [OldestCallsSchemaVersion]
// to [SchemaVersion], since those four lines read the same under each to a
// reader that skips a dispatch line naming no action (version 1 wrote some
// that version 2 no longer writes, and every reader skips them), and
// drops the session lines of an older schema, which are the lines a later
// version changed the meaning of: a reader handed one would fold it under a
// reading it was not written for, which is what [ReadShards] refuses a whole
// shard for. A session line of the current schema is kept. Every other rule
// [ReadShards] holds a line to still applies.
func ReadShardsForCalls(dir string) ([]Shard, error) {
	read, err := callShards.ReadShards(dir)
	if err != nil {
		return nil, err
	}
	for i := range read {
		read[i].Records = slices.DeleteFunc(read[i].Records, olderSession)
	}
	return read, nil
}

// olderSession reports whether a record is a session line written under an
// older schema than the current one.
func olderSession(r Record) bool {
	return r.Type == TypeSession && r.Schema != SchemaVersion
}

// Read merges every shard under dir, subdirectories included, in the order the
// directory tree walks. It is [ReadShards] with the file boundaries dropped,
// for a reader that wants the lines and not their provenance.
func Read(dir string) ([]Record, error) { return shards.Read(dir) }

// ReadForCalls is [ReadShardsForCalls] with the file boundaries dropped, as
// [Read] is [ReadShards]: for a reader of the run, call, dispatch and skip
// lines that wants the lines and not their provenance, such as R-PATH's
// per-action observation, which folds dispatch lines alone.
func ReadForCalls(dir string) ([]Record, error) {
	read, err := ReadShardsForCalls(dir)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, shard := range read {
		records = append(records, shard.Records...)
	}
	return records, nil
}

// IsShard reports whether a file name is one of this package's shards.
//
// It is exported because the coverage audit asks the same question of a
// directory before it walks it, and spelling the two halves of the name a
// second time there is how the answer would come to disagree with the one the
// reader gives.
func IsShard(name string) bool { return shards.IsShard(name) }
