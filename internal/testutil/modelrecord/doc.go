// Package modelrecord declares what one model evaluation run writes down: the
// stimulus a model was given, every request that went to a provider, every
// tool call the model made, what the server dispatched for it, and what GitLab
// answered.
//
// # Why the record holds no verdict
//
// Nothing here says whether a model did well. A shard is observation only, and
// the scoring is a pure function of a shard and the corpus key computed when a
// report is generated. That is the one property the evaluator this replaces did
// not have: it rendered a verdict into Markdown and parsed the verdict back out
// of its own text, so a scoring rule could not be corrected without spending
// the tokens again. Here a rule can be corrected and every past run re-scored
// offline, and the cost of a run is paid once.
//
// The one consequence worth naming is on [Call.Result]: a later step's argument
// may be bound to a field of an earlier step's answer, and a scorer that runs
// after the fact can only check that binding if the answer was written down. So
// a call line carries the structured result, capped at [MaxResultBytes] with
// [Call.ResultTruncated] set when the cap bit, which a binding reads as a
// failure with a reason rather than as a match it cannot make. [Call.Text], the
// rendered answer the model actually read and acted on, is kept and capped on
// the same terms by [CapText].
//
// # Why it is a package of its own
//
// Two programs share it. The runner is built with the e2e tag, because it
// starts the real server binary against a real GitLab; cmd/gen_model_results is
// an ordinary command, because scoring and rendering need neither. A shared Go
// type is what keeps the two halves from drifting, and it can only be shared
// from a package both may import, so this package is untagged and imports only
// the standard library.
//
// The sibling [github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls]
// is the same arrangement for the end-to-end coverage record, and the shard
// mechanics here follow it deliberately: the same absolute-directory rule, the
// same one-shard-per-process naming, the same envelope with exactly one
// payload. What differs is the content, and it differs on purpose. That record
// drops argument values, because a value is a fixture and a coverage figure is
// about names; this one keeps them, because whether a model sent the right
// value is the question it exists to answer.
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
// [Run], [Session], [Attempt], [Turn], [Call] or [Verify].
//
// The six join to each other by name rather than by position: a [Turn] and a
// [Call] name their [Attempt.ID], a [Call] names the turn it was made in, a
// [Verify] names the attempt it checked, and an [Attempt] names the
// [Session.Label] that served it. Only the run line is joined by the shard it
// sits in, which is why [ReadShards] keeps the file boundaries and [Read] is
// the merge for a reader that does not need them: one process writes one shard
// and one run line, so the shard is what says which run an attempt belongs to.
package modelrecord
