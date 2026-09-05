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
// Usage:
//
//	go run ./cmd/audit_readonly_graphql/
//	go run ./cmd/audit_readonly_graphql/ -v
package main
