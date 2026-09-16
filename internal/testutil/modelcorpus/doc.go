// Package modelcorpus is the model evaluation corpus: for each case, the
// stimulus a model is given and the key that stimulus is scored against, held
// apart so that no code which produces a stimulus can read a key.
//
// # The boundary, and what it does and does not close
//
// A [Case] carries both halves and publishes them through two accessors.
// [Stimuli] returns what a run may see: the identifier, the prompt template,
// the recipe that builds the world, the environment the case needs and the
// surfaces it may run on. [Keys] returns what only a scorer may see: the
// ordered steps and, per argument, where its right value comes from.
//
// The key is an unexported field, so a builder, a repair message, a simulated
// result or a mock fixture cannot be derived from it: the runner imports this
// package and calls [Stimuli], and a test in this package fails when any
// package other than the scorer, the two generators that publish about the key
// and the fake provider so much as names [Keys].
//
// That closes the machine half of the leak. It does nothing about the author,
// who writes both halves in one file, and calling it the structural fix for
// that would be a claim the mechanism does not support. The authorial half is
// held by two other things: the prompt audit in this package's tests, which
// renders every stimulus on every surface and refuses one that spells its own
// answer, and review, under the rule that changing a prompt to make a case
// pass is the defect rather than the fix.
//
// # What is data here and what is read from the catalog
//
// A step names one canonical catalog action, never a per-surface tool name.
// How that action is spelled on each surface, which arguments its schema
// admits, whether it destroys and which tier it needs are all read from the
// catalog at run time, by the harness that calls it and by the tests that gate
// this corpus. Written by hand, and audited, are only the prompt, the key and
// the three surface contracts in contract.go.
//
// # Why it is untagged and imports nothing
//
// Two programs share it: the runner, which is built with the e2e tag because
// it starts the real server binary against a real GitLab, and the scoring
// command, which is an ordinary binary. A shared type can only be shared from
// a package both may import, so this one is untagged and stdlib only. Its
// sibling [github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord]
// is the same arrangement for what a run writes down.
//
// Being untagged means an ordinary build compiles it, so it is held to the
// same bar as production code, and cmd/server's
// TestDependencies_TestSupport_NeverReachesTheServerBinary names it so that it
// can never reach the binary a user downloads.
//
// # Identifiers
//
// A case identifier is allocated once and retired rather than reused, which is
// why the numbers are not contiguous: MT-199 and above were renumbered out of
// a collision, and every identifier a retirement takes out of the corpus stays
// spent. access_test.go is what holds that.
//
// # What a retirement is
//
// [Retired] answers what became of an identifier the corpus no longer holds,
// so a person reading a report from before the move is not left with a number
// and nothing else. The categories are the survivor rule read backwards: a
// case survives when every step of its key is a tools/call this server
// registers, its result comes from the server or from GitLab, and a recipe can
// build its world. retired.go carries the two classes that could be decided by
// reading the old corpus; the third needs a builder to have been tried, so its
// category is declared there and used by the step that tries.
package modelcorpus
