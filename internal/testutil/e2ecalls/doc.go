// Package e2ecalls declares the record the end-to-end suite writes down while
// it runs, and the coverage audit reads back afterwards: what a test asked the
// server to do, what the server dispatched, and on which runtime, surface and
// mode.
//
// # Why the record exists
//
// Coverage used to be credited by mentions in the source: cmd/audit_e2e_gaps
// globs the suite's test files and credits any catalog action whose ID appears
// on a code line. That counts an action nothing runs, and it counted eighteen
// of them that neither Docker target can reach. A call counts here only when
// the server dispatched it, in a test that passed, on a named runtime, surface
// and mode, which is what these lines carry.
//
// # Why it is a package of its own
//
// Two programs read and write it: the e2e harness, which is built with the e2e
// tag, and cmd/audit_e2e_coverage, which is an ordinary command. A shared Go
// type is what keeps the two halves from drifting, and it can only be shared
// from a package both may import. internal/testutil cannot be that package: it
// embeds the pinned GraphQL schema and net/http/httptest, which a command has
// no use for and would carry anyway. So this package is untagged, imports only
// the standard library, and holds nothing else.
//
// Being untagged means it is compiled by an ordinary build, enters make test,
// the coverage job and Sonar, and is held to the same bar as production code.
// It must still never reach the server binary: cmd/server's
// TestDependencies_TestSupport_NeverReachesTheServerBinary names it, because
// that test matches exact import paths and internal/testutil alone does not
// cover a subpackage of it.
//
// # What a shard is
//
// One test process writes one shard, named [ShardPattern], into the directory
// [DirEnv] names. Nothing is written when that variable is unset. Each line is
// one JSON [Record]: a schema version, a line type, and exactly one payload of
// [Run], [Session], [Call], [Dispatch] or [Skip]. [Read] merges a directory of
// them, subdirectories included, so one run per Docker target can be compared
// against another without moving files around.
package e2ecalls
