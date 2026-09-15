// Command audit_readonly_graphql fails when an action the canonical catalog
// classifies ReadOnly can reach a GraphQL mutation.
//
// --read-only removes actions through FilterReadOnlyActions, and the surface
// served to a read_api OAuth token is narrowed the same way. Both key on the
// action's catalog classification, not on what its handler does. An action
// classified ReadOnly whose handler issues a GraphQL mutation therefore
// survives both filters and executes a write precisely where a write is
// supposed to be impossible, with nothing anywhere reporting it: GitLab
// performs the write, because the credential's scope is whatever the caller's
// token actually carries.
//
// The HTTP method cannot be the test. client-go sends every GraphQL request as
// a POST, so around twenty read-only actions legitimately POST and the verb
// separates nothing. The operation type in the document is the whole of the
// distinction, and it is in the source: a mutation is "mutation ..." in the
// document handed to GraphQL.Do, against "query ..." or a bare selection set
// for a read.
//
// So the audit resolves every read-only catalog action to the function its
// route runs, walks what that function can call, and classifies every GraphQL
// document those bodies name. An action that sends no GraphQL is not a
// finding, and neither is a mutation reached from an action already classified
// as mutating.
//
// # Where the documents come from
//
// The inventory is cmd/internal/graphqldocs, the same reading the schema gate
// judges, and only the operation-type rule is this command's own. It used to
// have a document walk of its own that read string constants and package-level
// variables, which is every document this repository writes today and not every
// document it may write tomorrow: a document moved into a .graphql file and
// pulled in with an embed directive folds to nothing for the type checker, so
// that walk saw none of it while the schema gate read it straight off disk. Two
// detectors of the same thing disagreeing is how a gate ends up answering a
// narrower question than the one it prints.
//
// What one shared inventory does not fix is attribution. This audit places a
// document through the object that declares it, or through the body that writes
// it inline; a .graphql file belongs to no function and no object, so it can be
// read and still not be tied to the handler that sends it. Rather than pass
// what it cannot classify, the run reports every such document as a finding and
// exits non-zero, which is the same answer it gives to a read-only action whose
// handler it cannot resolve.
//
// # There is one rule, and it is not here
//
// The inventory used to have a pre-filter of its own and this command used to
// have an operation-type rule of its own, and the two parted company in both
// directions. The pre-filter was the narrower one: it wanted the operation
// keyword at the very start of the comment-stripped text, where the rule here
// accepted it at the start of any line, and it refused a brace-wrapped single
// word with no space in it, the shape an OpenTelemetry unit annotation is
// written in. A mutation written in either shape left the inventory and was
// judged by nothing, while this command went on printing that no read-only
// action reaches a mutation, having read fewer documents than its own rule
// described. Nothing in the repository was written that way, so it was latent;
// a gate that narrows silently is the class this repository keeps writing gates
// against, so it is not a state to leave alone.
//
// Both questions are answered by the inventory now.
// [graphqldocs.LooksLikeDocument] says whether a string is a document, and
// [graphqldocs.DefinesMutation] says whether it carries a mutation, so this
// command sees exactly the documents the schema gate judges and every one of
// them is classified. What stays here is what the answer means, which is that
// an action classified ReadOnly must not be able to reach a mutation, and the
// two shapes above are the ones the convergence had to keep: both have a test
// in this package that drives the real loader over a fixture written that way.
//
// One narrowness is left, on purpose and in one place: a bare selection set of
// a single field with no spaces, `{id}`, is a unit annotation as far as any
// text rule can tell, and is refused. That costs nothing this gate is about.
// The brace form declares no operation, so it is a read by construction, and a
// mutation cannot be written without its keyword.
//
// Usage:
//
//	go run ./cmd/audit_readonly_graphql/
//	go run ./cmd/audit_readonly_graphql/ -v
package main
