// Package shardio is the shard mechanism the records written by a test process
// and read back by a command are built on: one shard file per process, one JSON
// line per record, a directory tree read in one pass, and a line nobody can
// read reported rather than dropped.
//
// # Why it is a package of its own
//
// A record of what a run did has two halves that live in different programs.
// The half that writes is built with the e2e tag, because it drives a real
// binary; the half that reads is an ordinary command, because merging and
// reporting need neither GitLab nor a test binary. Each such record therefore
// needs a package both may import, and every one of them needs the same
// mechanism: the absolute-directory rule, one shard per process, the seen set
// that absorbs a line offered twice, the walk that merges a tree, the line cap
// both halves are held to.
//
// That mechanism was written twice before this package existed, once for the
// end-to-end coverage record and once for the model evaluation record, and the
// two copies had already drifted on the one question that matters: what a line
// that cannot be encoded costs. This package answers it once. The record
// packages keep their own line types, their own field names and their own
// reasons, and hand this one a [Spec] saying what they are called and how a
// line becomes a record.
//
// # What it is not
//
// It never imports testing. A record package is linked by an ordinary command,
// and a library that drags the testing package into a command is a library
// nobody can use from one, so a failure is reported through the [Reporter]
// interface instead. Being untagged, it is compiled by an ordinary build, enters
// make test, the coverage job and Sonar, and is held to the same bar as
// production code. It must still never reach the server binary: cmd/server's
// TestDependencies_TestSupport_NeverReachesTheServerBinary names it, because
// that test matches exact import paths and internal/testutil alone does not
// cover a subpackage of it.
//
// # What a shard is
//
// One process writes one shard, named after the record's prefix and extension,
// into the directory the record's own environment variable names. Nothing is
// written when that variable is unset, and a relative value is refused rather
// than resolved: a test binary runs in its own package directory, so a relative
// path writes one shard under each of them and the merge finds none.
//
// Each line is one JSON record of the type the [Spec] is instantiated with.
// [Shards.ReadShards] hands back one entry per file, so a reader can join every
// line to the run line of the process that wrote it, and [Shards.Read] is the
// same walk with the boundaries dropped.
//
// # What a failure costs
//
// A line's own failure costs that line and nothing else: a record the [Spec]
// refuses before encoding, a record encoding/json cannot hold, and a line past
// [MaxLine] are each reported and skipped, and the lines offered afterwards are
// still written. What the writer cannot recover from stops it after one report:
// a relative directory, a shard it cannot open, and a file that stops accepting
// writes.
//
// The split is the whole point. These records are joined on a run line written
// last, from an exit hook, and it is the only thing that says which commit,
// instance and tier a shard belongs to. A writer silenced by one line halfway
// through loses every later line, the run line with them, and says so nowhere,
// so one unencodable value costs the whole record of a run that has already
// been paid for.
package shardio
