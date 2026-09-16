// Package modelscore turns one attempt's record into the verdict a published
// row is made of.
//
// It is a pure function of two things and nothing else: the lines one attempt
// wrote, and the answer key of the case that attempt was put to. It never sees
// a stimulus, never sees a provider, and never sees a GitLab. That is what lets
// a scoring rule be corrected and every past run re-scored without spending a
// token, which the evaluator this replaces made impossible by writing its
// verdicts into Markdown and parsing them back.
//
// # What it reads
//
// The record is observation only ([github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord]):
// what the model named, what it sent, what the server's own span said it
// dispatched, what came back. The key is the corpus's
// ([github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus].Key):
// the steps in the order they must dispatch, each with the arguments whose
// values are compared and where each right value comes from.
//
// Three readings here are the ones the old evaluator got wrong, and each is
// worth naming because a plausible simpler version of this package would
// reintroduce it:
//
//   - A step is reached by the action the server **dispatched**, not by the one
//     the model requested. The two differ wherever an alias is rewritten, and
//     crediting the request would credit an action that never ran. The
//     requested action is the fallback, for the calls where the span names no
//     action at all, which is every call a protective mode withheld.
//   - A discovery call is counted as overhead and never matched against a step.
//     The key contains no find step, so no column can measure a find call under
//     the name of a catalog call.
//   - An argument is compared by value against a truth the case declared, not
//     by name against a list. A parameter name present with the wrong value is
//     a miss here and was a pass there.
//
// # What it costs to import
//
// This package is untagged, so a generator can import it, and it is not stdlib
// only: it needs the action catalog to know which action a call names, which
// arguments a surface puts where, and whether a step mutates or destroys.
// Importing it therefore links the whole catalog, which is why
// cmd/gen_model_results is a command of its own rather than a flag on an
// existing one, and why cmd/server's dependency test names this package as one
// that must never reach the binary a user downloads.
//
// # What a verdict is not
//
// No verdict here is a claim about a call whose span never arrived. Such a
// verdict would be about what the model asked for rather than about what ran,
// so it is marked [OutcomeUnobserved] and left out of every published
// numerator and denominator, counted apart instead.
package modelscore
