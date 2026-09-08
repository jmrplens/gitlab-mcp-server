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
// # Where the two rules disagree
//
// The inventory has a pre-filter of its own, and it is not this command's
// operation-type rule, so the two part company in both directions on shapes
// nothing in this repository writes today.
//
// The pre-filter is the narrower one: it wants the operation keyword at the
// very start of the comment-stripped text, where the rule here accepts it at
// the start of any line. A string that only the looser rule reads as a
// document, such as a sentence of prose above a mutation, is not in the
// inventory at all and so is neither indexed nor reported. That is a narrowing
// of what this gate sees, and it is deliberate: every document this repository
// sends opens with its keyword, including the four assembled from a shared
// fragment, and a string that does not is not a document GitLab would accept.
//
// The pre-filter is also the looser one, for a literal written inside a
// function body: it may read one as a document that classifyDocument then
// classifies as none, in which case the body walk places nothing at its
// position and the document is reported as unattributed. That is a false alarm
// rather than a silence, which is the trade this gate makes everywhere else
// too, and it is a reviewable line rather than a clean run over a string
// nobody judged.
//
// Usage:
//
//	go run ./cmd/audit_readonly_graphql/
//	go run ./cmd/audit_readonly_graphql/ -v
package main
