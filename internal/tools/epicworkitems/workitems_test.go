package epicworkitems

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
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
