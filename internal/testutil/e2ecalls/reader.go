// The reading half: every shard of a directory tree, merged in one pass, with
// a line nobody can read reported rather than dropped.

package e2ecalls

import (
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

// Read merges every shard under dir, subdirectories included, in the order the
// directory tree walks. It is [ReadShards] with the file boundaries dropped,
// for a reader that wants the lines and not their provenance.
func Read(dir string) ([]Record, error) { return shards.Read(dir) }

// IsShard reports whether a file name is one of this package's shards.
//
// It is exported because the coverage audit asks the same question of a
// directory before it walks it, and spelling the two halves of the name a
// second time there is how the answer would come to disagree with the one the
// reader gives.
func IsShard(name string) bool { return shards.IsShard(name) }
