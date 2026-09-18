// work_items_test.go contains unit tests for the shared epic-backed work item
// GraphQL helpers (ResolveEpicGID, ResolveWorkItemGID).
package epicworkitems

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestResolveEpicGID verifies ResolveEpicGID returns a work item GID for a
// valid GraphQL namespace response and preserves the epic-specific not-found message.
func TestResolveEpicGID(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr string
	}{
		{
			name: "resolves id",
			body: `{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}`,
			want: "gid://gitlab/WorkItem/1",
		},
		{
			name:    "missing epic",
			body:    `{"namespace":null}`,
			wantErr: "epic not found in group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, tt.body)
				},
			}))

			got, err := ResolveEpicGID(t.Context(), client, "group", 1)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ResolveEpicGID() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveEpicGID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveEpicGID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveWorkItemGID_GraphQLError verifies GraphQL transport errors are
// returned unchanged so callers can wrap them with tool-specific guidance.
func TestResolveWorkItemGID_GraphQLError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "forbidden", http.StatusForbidden)
		},
	}))

	_, err := ResolveWorkItemGID(t.Context(), client, "group", 1)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("ResolveWorkItemGID() error = %v, want forbidden", err)
	}
}

// TestResolveWorkItemGID_NotFound verifies ResolveWorkItemGID returns the
// generic work-item not-found message when GraphQL returns no matching item.
func TestResolveWorkItemGID_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"workItem":null}}`)
		},
	}))

	_, err := ResolveWorkItemGID(t.Context(), client, "group", 1)
	if err == nil || !strings.Contains(err.Error(), "work item not found") {
		t.Fatalf("ResolveWorkItemGID() error = %v, want work item not found", err)
	}
}

// gidResolver is the shape ResolveEpicGID and ResolveWorkItemGID share, so one
// table can hold both to the request they build and to the message they return.
type gidResolver func(context.Context, *gitlabclient.Client, string, int64) (string, error)

// gidResolvers names both exported resolvers with the prefix each puts on its
// not-found message, which is the only place the two differ.
var gidResolvers = []struct {
	name        string
	resolve     gidResolver
	notFoundMsg string
}{
	{name: "epic", resolve: ResolveEpicGID, notFoundMsg: "epic not found in group"},
	{name: "work_item", resolve: ResolveWorkItemGID, notFoundMsg: "work item not found in"},
}

// capturedRequest holds the GraphQL variables one request carried. The httptest
// handler runs on the server's own goroutine, so the values are taken under a
// mutex and asserted back on the test goroutine, where a failure is attributed
// to the test that caused it.
type capturedRequest struct {
	mu   sync.Mutex
	vars map[string]any
}

func (c *capturedRequest) record(vars map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vars = vars
}

func (c *capturedRequest) snapshot() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.vars
}

// TestResolveGID_Request_CarriesTheNamespaceAndIIDTheCallerAsked reads back the
// variables GitLab received and holds them to the pair the caller passed in.
//
// Translating a (namespace, iid) pair into the GID every later mutation
// addresses is the whole job of this package, and it is the one part of that
// job nothing else can notice going wrong: the resolver returns whatever id the
// response carries, so one asking about another namespace or another IID hands
// back a GID that is perfectly well-formed and belongs to the wrong work item,
// which the caller then edits or deletes. Asserting only the returned id leaves
// that unguarded — with the request values unasserted, sending `fullPath + "…"`
// and `iid + 1` kept the whole package green.
//
// The expected IID is written out as a literal rather than derived with
// strconv, so a resolver that changed base would move only one side of the
// comparison.
func TestResolveGID_Request_CarriesTheNamespaceAndIIDTheCallerAsked(t *testing.T) {
	const (
		askedPath        = "acme/platform"
		askedIID         = int64(42)
		iidOnTheWire     = "42"
		wantGID          = "gid://gitlab/WorkItem/77"
		wantGIDResponse  = `{"namespace":{"workItem":{"id":"` + wantGID + `"}}}`
		unreadableReason = "unreadable GraphQL variables"
	)

	for _, tt := range gidResolvers {
		t.Run(tt.name, func(t *testing.T) {
			var captured capturedRequest
			client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, r *http.Request) {
					vars, err := testutil.ParseGraphQLVariables(r)
					if err != nil {
						t.Errorf("ParseGraphQLVariables() error = %v", err)
						http.Error(w, unreadableReason, http.StatusInternalServerError)
						return
					}
					captured.record(vars)
					testutil.RespondGraphQL(w, http.StatusOK, wantGIDResponse)
				},
			}))

			got, err := tt.resolve(t.Context(), client, askedPath, askedIID)
			if err != nil {
				t.Fatalf("resolve() error = %v", err)
			}
			if got != wantGID {
				t.Errorf("resolve() = %q, want %q", got, wantGID)
			}

			vars := captured.snapshot()
			if vars["fullPath"] != askedPath {
				t.Errorf("fullPath variable = %v, want %q", vars["fullPath"], askedPath)
			}
			// GitLab types a work item's iid as String, so the resolver has to
			// send the decimal spelling of the number the caller gave it.
			if vars["iid"] != iidOnTheWire {
				t.Errorf("iid variable = %v, want %q", vars["iid"], iidOnTheWire)
			}
		})
	}
}

// TestResolveGID_NotFound_MessageNamesTheNamespaceAndIID asserts the not-found
// message carries the pair that was asked about, not merely its fixed prefix.
//
// That message is the whole of what a model is told when the epic or work item
// is absent, and it is what a caller re-reads to see which identifier it got
// wrong. Asserting the prefix alone holds while the interpolated values come
// from anywhere at all, so the two arguments are checked by the values a caller
// would recognize.
func TestResolveGID_NotFound_MessageNamesTheNamespaceAndIID(t *testing.T) {
	const (
		askedPath = "acme/platform"
		askedIID  = int64(9001)
		iidInText = "9001"
	)

	for _, tt := range gidResolvers {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"workItem":null}}`)
				},
			}))

			_, err := tt.resolve(t.Context(), client, askedPath, askedIID)
			if err == nil {
				t.Fatalf("resolve() error = nil, want %q", tt.notFoundMsg)
			}
			if !strings.Contains(err.Error(), tt.notFoundMsg) {
				t.Errorf("resolve() error = %v, want containing %q", err, tt.notFoundMsg)
			}
			if !strings.Contains(err.Error(), askedPath) {
				t.Errorf("resolve() error = %v, want naming namespace %q", err, askedPath)
			}
			if !strings.Contains(err.Error(), iidInText) {
				t.Errorf("resolve() error = %v, want naming IID %s", err, iidInText)
			}
		})
	}
}
