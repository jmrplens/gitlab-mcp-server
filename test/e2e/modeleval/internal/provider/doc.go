//go:build e2e

// Package provider sends one conversation to one model and returns what it
// said, in the record's own vocabulary.
//
// It is four adapters and a fake behind one interface, and everything about it
// is deliberately dumb. An adapter marshals a request, sends it, decodes the
// answer and returns it. It does not retry, it does not repair, it does not
// decide what a malformed answer means and it does not know what a case is: the
// runner owns the turn loop, the retries and the endings, so that every
// decision about what an attempt was worth is made in one place and can be
// re-made later from the record without spending a token.
//
// Three properties of this package are answers to findings about the evaluator
// it replaces, and each is load-bearing rather than tidy:
//
// The tool list is normalized once, here, before any adapter sees it, and every
// adapter sends it verbatim and hashes what it sent ([Provider.ToolDigest]).
// Two of the old four adapters injected thirty-six parameter names into the
// dynamic execute schema for their provider only, so two columns of a published
// table measured a different surface from the other two and nothing on the row
// said so. The digest is what makes that visible now: one tool list, four
// digests, and a rewrite shows as a digest that differs from its siblings.
//
// Tool choice is never forced. The old adapters all demanded a tool call
// (Anthropic "any", OpenAI "required", Gemini "VALIDATED"), which makes the one
// behavior a read-only deployment actually wants, answering in text that the
// operation is unavailable, impossible to observe. A model that declines here
// can decline.
//
// A tool call whose arguments are not JSON is returned as it arrived, in
// [github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord.Block.Raw],
// and nothing tries to rescue it. The old adapter carried five layers of repair
// for exactly that text, then retried, then re-ran the whole task and replaced
// the result, so a model that could not emit a tool call scored as one that
// could.
package provider
