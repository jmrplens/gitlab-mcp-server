// Package e2ecalls declares the record the end-to-end suite writes down while
// it runs, and the coverage audit reads back afterwards: what a test asked the
// server to do, what the server dispatched, and on which runtime, surface and
// mode.
//
// # Why the record exists
//
// Coverage used to be credited by mentions in the source: the audit that ran
// before this one globbed the suite's test files and credited any catalog
// action whose ID appeared on a code line. That counts an action nothing runs,
// and it counted eighteen of them that neither Docker target can reach. A
// call counts here only when
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
// no use for and would carry anyway. So this package is untagged and holds the
// record and nothing else.
//
// The shard mechanism under it is
// [github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/shardio], which is
// the only thing this package imports beyond the standard library and is bound
// by the same two rules: untagged, and never importing testing. What lives here
// is the content, which lines exist, what each field means and which of them a
// reader joins on; what lives there is how a line reaches a file and comes
// back. The two were one package until a second record needed the same
// mechanism and copied it.
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
