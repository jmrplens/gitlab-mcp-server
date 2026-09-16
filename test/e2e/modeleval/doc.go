// Package modeleval runs a language model against the real
// gitlab-mcp-server binary and writes down what it did.
//
// It lives beside the end-to-end suite rather than under cmd/ because it needs
// exactly what that suite already has and nothing a command could assemble: a
// child process started from the released binary with the environment a
// deployment gives it, a real GitLab behind it, a fixture library that builds
// ground truth through client-go, and the server's own telemetry coming back
// so that what ran is read from the server rather than guessed from what was
// asked. The evaluator this replaces assembled an mcp.Server of its own from a
// three-field configuration, and so measured an assembly nobody deploys.
//
// What is here is the run: the settings, the server shape, and the adapter
// that turns one call a model chose into a line of the observation record. The
// data it runs on, the record it writes and the verdicts read back out of that
// record are untagged packages elsewhere, so a scorer can be corrected and
// every past run re-scored without spending a token:
// internal/testutil/modelcorpus, internal/testutil/modelrecord and
// internal/testutil/modelscore.
//
// The tests carry the e2e build tag; this file is what a plain build sees of
// the package. Nothing here is run by CI: a run costs money at a provider, and
// the targets that start one refuse without explicit consent.
package modeleval
