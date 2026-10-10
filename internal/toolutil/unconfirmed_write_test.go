package toolutil

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// The messages GitLab's GraphQL library writes when a null reaches a
// non-null position, for a field and for a list element, as graphql-ruby
// 2.6.10 words them (InvalidNullError). An element's type is written two
// wrappers below the field's, so it carries its "!" only under a list that is
// itself non-null: SecurityAttributeCreatePayload.securityAttributes is
// [SecurityAttribute!], which GitLab 19.4.1 was measured to name
// 'SecurityAttribute'.
const (
	nulledField   = "Cannot return null for non-nullable field WorkItem.workItemType"
	nulledElement = "Cannot return null for non-nullable element of type 'SecurityAttribute' for SecurityAttributeCreatePayload.securityAttributes"
	refusal       = "The resource that you are attempting to access does not exist or you don't have permission to perform this action"
)

// answeredWith is a response with the status a GraphQL write was answered
// with.
func answeredWith(status int) *gl.Response {
	return &gl.Response{Response: &http.Response{StatusCode: status}}
}

// TestGraphQLNullPropagated_MessageSets_OnlyNullPropagationQualifies verifies the predicate that tells a write whose returned object GitLab
// nulled below a non-null position from one GitLab refused. GitLab's GraphQL
// library writes one message per such null, for a field and for a list
// element, and a refusal writes something else, so only a set made of those
// messages alone qualifies: one other message among them is a refusal the
// caller has to read, and an empty set is no answer about the object at all.
func TestGraphQLNullPropagated_MessageSets_OnlyNullPropagationQualifies(t *testing.T) {
	cases := []struct {
		name     string
		messages []string
		want     bool
	}{
		{name: "no message", want: false},
		{name: "a field", messages: []string{nulledField}, want: true},
		{name: "a list element", messages: []string{nulledElement}, want: true},
		{name: "both, padded", messages: []string{"  " + nulledField + "\n", nulledElement}, want: true},
		{name: "a refusal among them", messages: []string{nulledField, refusal}, want: false},
		{name: "an empty message among them", messages: []string{nulledField, ""}, want: false},
		{name: "the words mid-sentence", messages: []string{"GitLab said: " + nulledField}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := GraphQLNullPropagated(tc.messages); got != tc.want {
				t.Errorf("GraphQLNullPropagated(%q) = %v, want %v", tc.messages, got, tc.want)
			}
		})
	}
}

// TestWriteAnsweredWithoutObject_EachFormOfTheAnswer_OnlyANullOnHTTP200Qualifies
// verifies the answers that say GitLab ran a write and answered without the object it
// returns, in each form client-go and this server's own decoders report one:
// the not-found and empty-response sentinels a nulled object becomes, the
// top-level errors of a null below a non-null position as client-go's typed
// error, as this server's own top-level error and as the text client-go's
// services join them into. Each holds only on an HTTP 200, since a 404 of the
// GraphQL endpoint reaches a caller as the same not-found sentinel and ran
// nothing, and a request nothing answered ran nothing anyone saw.
func TestWriteAnsweredWithoutObject_EachFormOfTheAnswer_OnlyANullOnHTTP200Qualifies(t *testing.T) {
	typed := &gl.GraphQLResponseError{Err: errors.New("Mutation.workItemCreate failed")}
	typed.Errors.Errors = append(typed.Errors.Errors, struct {
		Message string `json:"message"`
	}{Message: nulledField})
	refused := &gl.GraphQLResponseError{Err: errors.New("Mutation.workItemCreate failed")}
	refused.Errors.Errors = append(refused.Errors.Errors, struct {
		Message string `json:"message"`
	}{Message: refusal})
	cases := []struct {
		name string
		resp *gl.Response
		err  error
		want bool
	}{
		{name: "no error", resp: answeredWith(http.StatusOK), want: false},
		{name: "no response", err: gl.ErrNotFound, want: false},
		{name: "a response with no status line", resp: &gl.Response{}, err: gl.ErrNotFound, want: false},
		{name: "the endpoint not found", resp: answeredWith(http.StatusNotFound), err: gl.ErrNotFound, want: false},
		{name: "the object nulled, as not found", resp: answeredWith(http.StatusOK), err: gl.ErrNotFound, want: true},
		{name: "the object nulled, as not found, wrapped", resp: answeredWith(http.StatusOK), err: fmt.Errorf("op: %w", gl.ErrNotFound), want: true},
		{name: "the object nulled, as an empty response", resp: answeredWith(http.StatusOK), err: gl.ErrEmptyResponse, want: true},
		{name: "nulled below a non-null field, typed", resp: answeredWith(http.StatusOK), err: typed, want: true},
		{name: "a refusal, typed", resp: answeredWith(http.StatusOK), err: refused, want: false},
		{name: "nulled below a non-null field, ours", resp: answeredWith(http.StatusOK), err: GraphQLTopLevelError("op", []GraphQLError{{Message: nulledElement}, {Message: nulledElement}}), want: true},
		{name: "a refusal, ours", resp: answeredWith(http.StatusOK), err: GraphQLTopLevelError("op", []GraphQLError{{Message: refusal}}), want: false},
		{name: "nulled below a non-null element, joined", resp: answeredWith(http.StatusOK), err: errors.New(nulledElement + "; " + nulledElement), want: true},
		{name: "a refusal beside a null, joined", resp: answeredWith(http.StatusOK), err: errors.New(nulledElement + "; " + refusal), want: false},
		{name: "a payload error", resp: answeredWith(http.StatusOK), err: errors.New("achievementsCreate mutation errors: [Name has already been taken]"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WriteAnsweredWithoutObject(tc.resp, tc.err); got != tc.want {
				t.Errorf("WriteAnsweredWithoutObject(%v, %v) = %v, want %v", tc.resp, tc.err, got, tc.want)
			}
		})
	}
}

// TestUnconfirmedWrite_ClassicOrFineGrainedSession_OnlyFineGrainedIsToldProbablyCommitted
// verifies the one answer a GraphQL write's handler returns for its error: a
// classic session keeps the handler's own error whatever GitLab answered,
// because nothing GitLab does to a classic token's answer is a reason to think
// a write it reported nothing about was made; a fine-grained session is told
// the write was probably committed when GitLab ran it and answered without the
// object, and keeps the handler's own error for anything else, a refusal
// above all.
func TestUnconfirmedWrite_ClassicOrFineGrainedSession_OnlyFineGrainedIsToldProbablyCommitted(t *testing.T) {
	classic := gitlabclient.NewUnboundClient("https://gitlab.example.com")
	fine := gitlabclient.NewUnboundClient("https://gitlab.example.com")
	fine.SetAuthority(finegrained.Unevaluated(fineGrainedTable(), finegrained.FallbackNone, ""))
	wrapped := errors.New("create_work_item: not found")
	cases := []struct {
		name        string
		client      *gitlabclient.Client
		resp        *gl.Response
		err         error
		unconfirmed bool
	}{
		{name: "classic, the object nulled", client: classic, resp: answeredWith(http.StatusOK), err: gl.ErrEmptyResponse},
		{name: "classic, nulled below a non-null field", client: classic, resp: answeredWith(http.StatusOK), err: errors.New(nulledField)},
		{name: "fine-grained, a refusal", client: fine, resp: answeredWith(http.StatusOK), err: errors.New(refusal)},
		{name: "fine-grained, the endpoint not found", client: fine, resp: answeredWith(http.StatusNotFound), err: gl.ErrNotFound},
		{name: "fine-grained, the object nulled", client: fine, resp: answeredWith(http.StatusOK), err: gl.ErrEmptyResponse, unconfirmed: true},
		{name: "fine-grained, nulled below a non-null field", client: fine, resp: answeredWith(http.StatusOK), err: errors.New(nulledField), unconfirmed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := UnconfirmedWrite(tc.client, tc.resp, "create_work_item", "work item", tc.err, wrapped)
			if tc.unconfirmed {
				if !errors.Is(got, finegrained.ErrUnconfirmedWrite) {
					t.Fatalf("UnconfirmedWrite = %v, want an unconfirmed write", got)
				}
				return
			}
			if !errors.Is(got, wrapped) || got.Error() != wrapped.Error() || errors.Is(got, finegrained.ErrUnconfirmedWrite) {
				t.Errorf("UnconfirmedWrite = %v, want the handler's own error %v unchanged", got, wrapped)
			}
		})
	}
}
