package toolutil

import (
	"errors"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// What a GraphQL write answers when GitLab ran it and answered without the
// object it returns (issue 1103).
//
// GitLab authorizes a mutation against its own declaration before it runs,
// and every object its payload selects against that object's type only after
// the write ran (Mutations::BaseMutation#authorized? and
// Types::BaseObject.authorized? in GitLab's source). A fine-grained personal
// access token the payload's type does not admit, because the type declares no
// fine-grained permission or needs one the grant does not hold, gets that
// object as null: with no error where its position is nullable, and with the
// error GitLab's GraphQL library writes for a null below a non-null position
// where it is not. A handler that reads either as a write that did not happen
// tells a model to repeat a write GitLab already made, so the handlers of
// such writes answer through [UnconfirmedWrite].

// graphQLNullPropagationPrefix opens every message GitLab's GraphQL library
// writes when a null reaches a non-null position, for a field ("Cannot return
// null for non-nullable field WorkItem.workItemType") and for a list element
// ("Cannot return null for non-nullable element of type ..."): the
// InvalidNullError of graphql-ruby 2.6.10, the version GitLab pins.
const graphQLNullPropagationPrefix = "Cannot return null for non-nullable "

// GraphQLNullPropagated reports whether the top-level errors of a GraphQL
// answer are all the ones a null below a non-null position writes, which is
// how an object GitLab denied answers where its position cannot be null. A
// set holding any other message, a refusal among them, is not one, and
// neither is an empty set: an answer that came back null with no error is
// the caller's to see.
func GraphQLNullPropagated(messages []string) bool {
	if len(messages) == 0 {
		return false
	}
	for _, message := range messages {
		if !strings.HasPrefix(strings.TrimSpace(message), graphQLNullPropagationPrefix) {
			return false
		}
	}
	return true
}

// writeErrorMessages returns the top-level messages an error of a GraphQL
// write carries: the entries [graphQLErrorMessages] reads whole, and when it
// reads none, the parts of the innermost error's text, which is how
// client-go's achievement services join the messages they read ("; "). A
// message holding that separator itself is split too, and the parts it
// leaves do not open as a null's, so the split can only make a set read as
// something other than a null, never the reverse.
func writeErrorMessages(err error) []string {
	if messages := graphQLErrorMessages(err); len(messages) > 0 {
		return messages
	}
	return strings.Split(recoveredErrorText(innermostError(err)), "; ")
}

// WriteAnsweredWithoutObject reports whether err, what a client-go GraphQL
// write returned with resp, is GitLab having run the write and answered
// without the object it returns: the object answered null, which client-go
// reports as its not-found or its empty-response sentinel, or nulled below a
// non-null position, which it reports as the top-level errors that null
// writes.
//
// Only an answer GitLab gave with HTTP 200 qualifies. client-go reports a 404
// of the GraphQL endpoint itself with the same not-found sentinel, and that
// request ran nothing; an error with no response is no answer of GitLab's to
// read at all. Every write this is asked about is one request, so the answer
// is the write's own and never a lookup's made before it.
func WriteAnsweredWithoutObject(resp *gl.Response, err error) bool {
	if err == nil || resp == nil || resp.Response == nil || resp.StatusCode != http.StatusOK {
		return false
	}
	if errors.Is(err, gl.ErrNotFound) || errors.Is(err, gl.ErrEmptyResponse) {
		return true
	}
	return GraphQLNullPropagated(writeErrorMessages(err))
}

// UnconfirmedWrite returns the error a GraphQL write's handler answers with,
// given what its client-go call returned: wrapped, the handler's own error,
// except for a write GitLab ran and answered without the object it returns
// ([WriteAnsweredWithoutObject]) on a fine-grained session, which is told the
// write was probably committed. The decision of which session that is belongs
// to the client's authority, which only a fine-grained credential's client
// carries, so a classic token's write is never described as probably
// committed.
func UnconfirmedWrite(client *gitlabclient.Client, resp *gl.Response, operation, object string, err, wrapped error) error {
	if !WriteAnsweredWithoutObject(resp, err) {
		return wrapped
	}
	return client.Authority().UnconfirmedWrite(operation, object, wrapped)
}
