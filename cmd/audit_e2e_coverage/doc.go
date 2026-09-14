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
// # The committed record
//
// Everything above describes one run and is written into the gitignored
// dist/, so nothing on main can say what the suite covers. -record commits an
// allowlist of the report to docs/development/e2e-coverage.json, one entry per
// Docker target (ce, ee): the runtime, the run rows with their commit, GitLab
// version and fixture profile, the session rows, the summary, and the three
// level lists, which are what make the record an answer to which actions
// rather than only to how many. The per-action cells stay out, being a
// thousand rows per runtime that no reviewer would read, and the shard
// directory stays out for being a path on the machine that ran the suite. The
// entry's date is read off the run IDs' own timestamps, not the writing
// clock, so rebuilding the file from old shards does not reset the window.
// The write insists on -results and -static, because without them the
// classification frozen would be a more generous one than the gates judge by.
//
// -render-record redraws docs/development/testing/e2e-coverage.md from the
// committed record, which is why that half is a member of make update-all and
// the measurement is not. -check-record is the offline gate: the schema, the
// runtime set, the levels against the lists beside them, the floors -check
// applies, the staleness window, and the page byte-equal to a fresh
// rendering. A catalog that has moved under the record is reported and does
// not fail, since -static already fails on the same rename from the
// scenario's side; so is an entry measured on a revision that is not an
// ancestor of HEAD, and only when git can resolve that revision at all, since
// a shallow CI checkout knows none of them.
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
