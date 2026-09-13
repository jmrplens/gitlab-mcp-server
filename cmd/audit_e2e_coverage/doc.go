// Command audit_e2e_coverage is the one command that says what the end-to-end
// suite covers.
//
// # What it reads
//
// The suite records what it did while it runs, through
// internal/testutil/e2ecalls: one run line per test package, one session line
// per server configuration, one call line per MCP request a test made, one
// dispatch line per span the server exported, and one skip line per skipped
// test. This command reads those shards (-calls), joins each dispatch to its
// call on the trace id, optionally joins the go test -json stream gotestsum
// wrote (-results), and compares what happened with what the runtime served.
//
// # What a runtime is
//
// A runtime is an edition and a tier: community/free, enterprise/ultimate,
// enterprise/free for an EE image with no license. Every shard under one
// directory is read as one runtime, named by its run lines, because no other
// line carries the runtime and a directory is what a Docker target writes. A
// parent directory whose children each hold shards is read as one runtime per
// child, so dist/e2e-calls with a ce and an ee run under it is compared as two.
// The catalog a runtime serves is built the way the server builds it, at that
// tier, with the diagnostics group and the standalone actions included.
//
// # What a cell is
//
// Every runtime x surface x mode x action gets exactly one state, judged from
// the calls that named the action and the tests they belong to: asserted when
// a passing test asked for it, expected it to work, the server said it ran
// exactly that action and it did; then the shallower credits (sweep-only,
// error-path-only, refused-only, preview-only, cleanup-only), then the states
// that say why there is no credit (unasserted, unservable, skipped, failed,
// absent). Levels summarize the default mode: L1 is asserted on any surface,
// L2 on the default dynamic surface, L3 on all three. Resources, prompts,
// completions, subscriptions, the elicitation flows and the protective modes
// are classified on the same terms.
//
// # The gates
//
// -check fails when an expected runtime left no run line, when no test call
// was recorded, when a package refused to run, when a package ran under a
// -run filter (a partial run is not a coverage claim about the rest), and
// when the asserted count falls below the floor exemptions.go records. -baseline compares two shard
// directories and fails on any runtime x surface x mode x action x credit the
// old suite reached in a passing test and the new one does not. The credits
// order themselves: a cleanup credit is met by the same cell as cleanup, sweep
// or asserted, and a sweep credit by sweep or asserted, since each of those
// says the action ran and answered at least as firmly as the one it stands
// in for. A baseline whose shards carry no verdicts, which is how the old
// suite's recorder wrote them, is joined with the gotestsum stream at
// <directory>.results.json beside it, and refused when there is none, or when
// the stream judges none or not all of the tests the calls name, rather than
// compared against nothing. -port-map
// reads the "// Replaces:" lines of the new suite against every Test function
// of the old one, with declared drops for the tests nothing replaces. An old
// test whose file has already been deleted stays on the map through the
// retired list in portmap.go, held to the same rule, since a deleted file
// declares nothing and the lines that replaced it would otherwise read as
// naming a test that never existed.
//
// -static needs no GitLab and runs on push. It loads test/e2e/gitlab and
// test/e2e/internal with their tests under the e2e tag and, from the type
// checker's own record, collects every constant of the harness's ActionID
// type, helper parameters included: each must name a catalog action, none in
// common or ce may be above Free, and an ee test naming an Ultimate action
// declares Needs(Tier(Ultimate)). It lists the ID sites that are not
// constants, flags a harness result thrown away with a blank assignment, and
// lists the harness exports nothing uses. With the ratchet on, every catalog
// action needs an ID in a package that can run it or an entry in exemptions.go,
// and the unused exports fail too.
package main
