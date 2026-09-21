// Package freshness reads the one harness setting that decides whether a test
// comparing a committed, generated artifact runs that comparison now or leaves
// it to the run where the artifact is refreshed.
//
// # Why the question exists
//
// Every freshness gate (the README stats, llms*.txt, the tool snapshots, the
// token footprint, the testing reference, the site data, the request
// inventory, the manifests) holds a committed file against what the source
// tree would generate now. In a stack of pull requests the artifacts are
// refreshed once, at the top, so every layer below carries them stale on
// purpose and would fail on drift the top overwrites. CI computes that answer
// once, as FRESHNESS in .github/workflows/ci.yml, and hands it to the unit
// suite through [EnvVar].
//
// # What reads it
//
// Test files only. [SkipIfDeferred] is called by the tests of these packages,
// each of which reads a committed artifact and compares it:
//
//   - internal/tools
//   - cmd/audit_metrics
//   - cmd/audit_tokens
//   - cmd/gen_lhm_manifest
//   - cmd/gen_llms
//   - cmd/gen_model_corpus
//
// The list is a claim about the tree rather than a note, so
// TestDoc_NamesEveryPackageThatDefers holds it both ways: it named four while
// six called, cmd/gen_lhm_manifest and cmd/gen_model_corpus having arrived
// after it was written, and a reader sizing what deferring reaches by it was
// reading a third of it short. Nothing in the server reads any of this, which
// is why TestDependencies_TestSupport_NeverReachesTheServerBinary in
// cmd/server names this package alongside internal/testutil.
//
// # What deferring does not mean
//
// It never means the artifact goes unchecked. The comparison runs wherever the
// refresh lands: at the top of a stack, on a pull request to main, and on
// every push to main, where CI answers "checked" and the generated-artifacts
// job runs the matching make check-* gate as well.
package freshness
