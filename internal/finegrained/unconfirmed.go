package finegrained

import (
	"errors"
	"fmt"
)

// What a write answers a fine-grained session with when GitLab ran it and
// answered without the object it returns (issue 1103).
//
// GitLab checks a fine-grained token at the mutation and then again at every
// object the answer selects, the payload's own object among them, and only
// the first check stops the write: Mutations::BaseMutation#authorized? raises
// before the write runs, while Types::BaseObject.authorized? runs on the
// object the payload carries after it ran, and a denial there is the null
// graphql-ruby's default unauthorized_object answers, which GitlabSchema does
// not override (app/graphql/mutations/base_mutation.rb,
// app/graphql/types/base_object.rb and app/graphql/gitlab_schema.rb, the same
// at v19.4.1-ee and on GitLab's master of 2026-10-09). So a write whose
// payload object's type declares no fine-grained permission, or one the grant
// does not hold, commits and answers without the object: null with no error
// where the position is nullable, and the error a null below a non-null
// position writes where it is not. A handler reading either as a write that
// did not happen tells a model to repeat a write that is already done.
//
// The words are written here, beside the other notes the authority words,
// because whether they apply is the authority's to say: only a fine-grained
// token's session carries one, so a classic token's write is never described
// as probably committed. Not every fine-grained token's session does: one
// whose kind the server could not learn carries none, is served as a classic
// token, and gets the handler's own answer. They name no GitLab version,
// unlike the notes that name what the recorded table holds, because the order
// of the two checks is GitLab's design rather than a state of its
// declarations: a type declared since the table was recorded and not granted
// answers the same way, and so does a declared type the grant does not hold,
// which is how the vulnerability state changes and the epic note edits reach
// this answer with the table as it is.

// ErrUnconfirmedWrite is what [Authority.UnconfirmedWrite]'s error is, so a
// caller can tell a write GitLab probably committed from one it refused.
var ErrUnconfirmedWrite = errors.New("GitLab ran the write and answered without the object it returns")

// UnconfirmedWrite is the error a GraphQL write returns when GitLab answered
// without the object the write returns.
//
// A session with no authority, which is every credential that is not a
// fine-grained token, gets classic, the handler's own error, unchanged. A
// fine-grained session gets the one sentence that says the write was probably
// committed and why, naming operation as the handler's errors do and object in
// a reader's words. It does not wrap classic: for a client-go write that is
// the not-found sentinel every 404 matches, and the write was no 404.
func (a *Authority) UnconfirmedWrite(operation, object string, classic error) error {
	if a == nil {
		return classic
	}
	return &unconfirmedWriteError{text: fmt.Sprintf("%s: GitLab answered without the %s this write returns. "+
		"GitLab checks that object against a fine-grained personal access token only after the write has run, "+
		"and answers null for one whose GraphQL type declares no fine-grained permission or needs one this token "+
		"was not granted, so the write was probably committed. Before repeating it, check in GitLab or with a "+
		"classic token whether it took effect: repeating a create makes a second one.",
		operation, object)}
}

// unconfirmedWriteError is the error [Authority.UnconfirmedWrite] returns to
// a fine-grained session.
type unconfirmedWriteError struct {
	text string
}

// Error returns the sentence.
func (e *unconfirmedWriteError) Error() string { return e.text }

// Is reports whether target is [ErrUnconfirmedWrite].
func (e *unconfirmedWriteError) Is(target error) bool { return target == ErrUnconfirmedWrite }
