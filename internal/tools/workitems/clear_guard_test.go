// clear_guard_test.go covers the guard that stands between a work item update
// and the two lists GitLab replaces wholesale. An explicit empty array is a
// legitimate way to remove every assignee or CRM contact, and it is also what
// a model sends by accident; the response says nothing about what was there
// before, so a mistake is not recoverable from it.
//
// The tests drive the confirmation exchange over a real MCP session rather
// than a stub: the answer arrives as an input response on the retried call,
// and that plumbing is most of what the guard depends on.
package workitems

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/elicitation"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// elicitingSession returns a server session whose connected client advertises
// elicitation support, so a flow built from it takes the confirmation path
// instead of the "client cannot be asked" fallback.
func elicitingSession(t *testing.T) *mcp.ServerSession {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{Name: "clearguard-test", Version: "0.0.1"}, nil)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			// The session negotiates a protocol that forbids server-initiated
			// elicitation, so the guard must queue an input request instead of
			// sending one. Reaching this handler means it did not.
			t.Error("the guard sent a synchronous elicitation request")
			return nil, errors.New("unexpected synchronous elicitation")
		},
	})
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		_ = ss.Close()
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		_ = ss.Close()
	})
	return ss
}

// guardContext builds the context a work item update handler runs under,
// carrying an MCP request with the given confirmation answer already
// recorded. A nil answer models the first round, before the user has replied.
func guardContext(t *testing.T, answer *mcp.ElicitResult) context.Context {
	t.Helper()
	params := &mcp.CallToolParamsRaw{}
	if answer != nil {
		params.InputResponses = mcp.InputResponseMap{clearListConfirmID: answer}
	}
	req := &mcp.CallToolRequest{Session: elicitingSession(t), Params: params}
	return toolutil.ContextWithRequest(context.Background(), req)
}

// TestConfirmListClearing_AnswerDecidesTheOutcome verifies that each answer to
// the confirmation maps to the outcome the caller must see.
//
// The four cases are genuinely different code paths, not variations of one:
// an unanswered prompt has to suspend the call and come back, a declined one
// has to fail with advice, and an accepted-but-not-confirmed answer is a
// distinct third thing — the client rendered the form, the user submitted it,
// and the box was left unticked. Collapsing that into "declined" would be
// wrong in the direction that deletes data.
func TestConfirmListClearing_AnswerDecidesTheOutcome(t *testing.T) {
	tests := []struct {
		name   string
		answer *mcp.ElicitResult
		check  func(*testing.T, error)
	}{
		{
			name:   "unanswered suspends the call",
			answer: nil,
			check: func(t *testing.T, err error) {
				t.Helper()
				if _, ok := errors.AsType[*elicitation.InputRequiredError](err); !ok {
					t.Errorf("error = %v (%T), want *elicitation.InputRequiredError", err, err)
				}
			},
		},
		{
			name:   "confirmed proceeds",
			answer: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}},
			check: func(t *testing.T, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("error = %v, want nil so the update proceeds", err)
				}
			},
		},
		{
			name:   "submitted without confirming is refused",
			answer: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": false}},
			check: func(t *testing.T, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("error = nil, want the update refused")
				}
				if !strings.Contains(err.Error(), "declined") {
					t.Errorf("error = %q, want it to say the user declined", err)
				}
				// The advice is the actionable half: without it the caller
				// only learns that something was refused, not how to proceed.
				if !strings.Contains(err.Error(), "omit assignee_ids/crm_contact_ids") {
					t.Errorf("error = %q, want it to name the way to leave the lists untouched", err)
				}
			},
		},
		{
			name:   "declined is a failed confirmation",
			answer: &mcp.ElicitResult{Action: "decline"},
			check: func(t *testing.T, err error) {
				t.Helper()
				if err == nil {
					t.Fatal("error = nil, want the update refused")
				}
				if !strings.Contains(err.Error(), "confirmation failed") {
					t.Errorf("error = %q, want a confirmation failure", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clearing CRM contacts alone: that list is not on the read model,
			// so the guard never calls GitLab and any request would be a bug.
			client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				t.Errorf("unexpected GitLab request: %s %s", r.Method, r.URL.Path)
			}))
			ctx := guardContext(t, tt.answer)

			err := confirmListClearing(ctx, client, UpdateInput{
				FullPath:      "acme/widgets",
				IID:           7,
				CRMContactIDs: []int64{},
			})
			tt.check(t, err)
		})
	}
}

// TestConfirmListClearing_UnreadableWorkItem_StillAsks verifies that a failure
// to read the current assignees makes the guard ask rather than assume.
//
// Reading the work item is how the guard skips the prompt when the list is
// already empty. When that read fails it cannot tell an empty-to-empty no-op
// from a deletion, and the two are not equally safe to guess at, so it asks
// and says why in the message.
func TestConfirmListClearing_UnreadableWorkItem_StillAsks(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"503 Service Unavailable"}`, http.StatusServiceUnavailable)
	}))

	err := confirmListClearing(guardContext(t, nil), client, UpdateInput{
		FullPath:    "acme/widgets",
		IID:         7,
		AssigneeIDs: []int64{},
	})

	inputErr, ok := errors.AsType[*elicitation.InputRequiredError](err)
	if !ok {
		t.Fatalf("error = %v (%T), want the guard to ask; an unreadable list must not be read as 'nothing to lose'", err, err)
	}
	prompt, ok := inputErr.Result().InputRequests[clearListConfirmID].(*mcp.ElicitParams)
	if !ok {
		t.Fatalf("queued request = %T, want *mcp.ElicitParams", inputErr.Result().InputRequests[clearListConfirmID])
	}
	if !strings.Contains(prompt.Message, "could not be read") {
		t.Errorf("prompt = %q, want it to say the current list could not be read", prompt.Message)
	}
}

// TestPendingClearLosses_NothingCleared_ReportsNothing verifies the guard's
// inner check returns no losses for an update that clears neither list.
//
// Its caller already screens for that, so this re-check is defense in depth
// against the two ever disagreeing — which they would, silently, the moment a
// third clearable list is added to one and not the other.
func TestPendingClearLosses_NothingCleared_ReportsNothing(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected GitLab request: %s %s", r.Method, r.URL.Path)
	}))

	// A populated list replaces rather than clears, and an omitted one is
	// left untouched: neither is a deletion.
	losses := pendingClearLosses(context.Background(), client, UpdateInput{
		FullPath:    "acme/widgets",
		IID:         7,
		AssigneeIDs: []int64{4},
	})
	if len(losses) != 0 {
		t.Errorf("pendingClearLosses() = %v, want none", losses)
	}
}
