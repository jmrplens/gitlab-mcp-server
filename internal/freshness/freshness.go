package freshness

import (
	"os"
	"testing"
)

// EnvVar is the harness setting that defers a committed-artifact comparison.
// It is developer-only, like GITLAB_MCP_TEST_INVENTORY_DIR: it configures the
// harness rather than the server, so it is read here with [os.Getenv] rather
// than through internal/config, which resolves the settings the server itself
// defines.
const EnvVar = "GITLAB_MCP_TEST_SNAPSHOT_PARITY"

// deferredValue is the one value that defers. Anything else compares,
// "checked" and an unset variable included, because the failure modes are not
// symmetric: a run that compares when it did not have to costs a red check on
// a layer nobody merges, and a run that defers when it should have compared
// lets a stale artifact reach main.
const deferredValue = "deferred"

// SkipReason says what was deferred and on whose authority, so a reader of the
// log sees a decision rather than a gap.
//
// The setting is spelled out rather than concatenated from the two constants
// above: a constant expression is not executable, so nothing can exercise the
// concatenation and no analysis can tell a correct one from a wrong one. What
// keeps the message honest is TestSkipReason_NamesTheSettingAndTheValue, which
// asserts both names appear here.
const SkipReason = "GITLAB_MCP_TEST_SNAPSHOT_PARITY=deferred: the committed artifacts are refreshed and compared where they land, at the top of a stack, on a pull request to main and on every push to main"

// Deferred reports whether the harness asked for committed-artifact
// comparisons to be left to the run that refreshes them.
func Deferred() bool {
	return os.Getenv(EnvVar) == deferredValue
}

// SkipIfDeferred skips the calling test when [Deferred] holds, naming
// [SkipReason]. Call it before the comparison reads anything, so a deferred
// run neither fails on a stale artifact nor pays for measuring one.
func SkipIfDeferred(tb testing.TB) {
	tb.Helper()
	if Deferred() {
		tb.Skip(SkipReason)
	}
}
