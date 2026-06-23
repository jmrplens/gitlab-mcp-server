// group_relations_test.go contains unit tests for the group relation list
// handlers added with client-go v2.41.0: SharedWithList, InvitedList, and
// TransferLocationsList. Tests use httptest to mock GitLab API responses.
package groups

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// SharedWithList
// ---------------------------------------------------------------------------.

// TestSharedWithList_Success verifies that SharedWithList returns the groups shared with a group.
// The test exercises the GET groups/:id/groups/shared path.
// It asserts the returned output contains the mocked shared group.
func TestSharedWithList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/groups/shared", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":11,"name":"Shared","path":"shared","full_path":"shared","visibility":"private","web_url":"https://gitlab.example.com/groups/shared"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := SharedWithList(context.Background(), client, SharedWithListInput{GroupID: toolutil.StringOrInt("7")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != 11 {
		t.Fatalf("expected shared group 11, got %+v", out.Groups)
	}
}

// TestSharedWithList_MissingGroupID verifies that SharedWithList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestSharedWithList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := SharedWithList(context.Background(), client, SharedWithListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvitedList
// ---------------------------------------------------------------------------.

// TestInvitedList_Success verifies that InvitedList returns the groups invited to a group.
// The test exercises the GET groups/:id/invited_groups path.
// It asserts the returned output contains the mocked invited group.
func TestInvitedList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/invited_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":12,"name":"Invited","path":"invited","full_path":"invited","visibility":"private","web_url":"https://gitlab.example.com/groups/invited"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := InvitedList(context.Background(), client, InvitedListInput{
		GroupID:  toolutil.StringOrInt("7"),
		Relation: []string{"direct"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != 12 {
		t.Fatalf("expected invited group 12, got %+v", out.Groups)
	}
}

// TestInvitedList_MissingGroupID verifies that InvitedList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestInvitedList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := InvitedList(context.Background(), client, InvitedListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TransferLocationsList
// ---------------------------------------------------------------------------.

// TestTransferLocationsList_Success verifies that TransferLocationsList returns candidate parent groups.
// The test exercises the GET groups/:id/transfer_locations path.
// It asserts the returned output contains the mocked location.
func TestTransferLocationsList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/transfer_locations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":99,"name":"Target","full_name":"Target Group","full_path":"target","web_url":"https://gitlab.example.com/groups/target"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{GroupID: toolutil.StringOrInt("7")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Locations) != 1 || out.Locations[0].ID != 99 {
		t.Fatalf("expected location 99, got %+v", out.Locations)
	}
	if out.Locations[0].FullPath != "target" {
		t.Errorf("expected full_path target, got %s", out.Locations[0].FullPath)
	}
}

// TestTransferLocationsList_MissingGroupID verifies that TransferLocationsList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestTransferLocationsList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Markdown
// ---------------------------------------------------------------------------.

// TestFormatTransferLocationsListMarkdown verifies the transfer-locations markdown formatter.
// The test exercises rendering of a populated location list.
// It asserts the rendered Markdown contains a clickable name link.
func TestFormatTransferLocationsListMarkdown(t *testing.T) {
	out := TransferLocationsListOutput{Locations: []TransferLocationOutput{
		{ID: 99, Name: "Target", FullPath: "target", WebURL: "https://gitlab.example.com/groups/target"},
	}}
	md := FormatTransferLocationsListMarkdown(out)
	if !strings.Contains(md, "[Target](https://gitlab.example.com/groups/target)") {
		t.Errorf("expected clickable link in markdown, got: %s", md)
	}
}

// TestFormatTransferLocationsListMarkdown_Empty verifies the empty-state rendering.
// The test exercises rendering of an empty location list.
// It asserts the empty-state message is present.
func TestFormatTransferLocationsListMarkdown_Empty(t *testing.T) {
	md := FormatTransferLocationsListMarkdown(TransferLocationsListOutput{})
	if !strings.Contains(md, "No transfer locations available") {
		t.Errorf("expected empty-state message, got: %s", md)
	}
}

// ---------------------------------------------------------------------------
// Create/Update new options (client-go v2.41.0)
// ---------------------------------------------------------------------------.

// TestCreate_NewOptions verifies that the v2.41.0 group create options are sent to the API.
// The test inspects the request body for the new boolean flags.
// It asserts each new flag appears in the marshaled request.
func TestCreate_NewOptions(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body = string(buf)
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":1,"name":"G","path":"g","full_path":"g","visibility":"private","web_url":"https://x/groups/g"}`)
	})
	client := testutil.NewTestClient(t, mux)

	enabled := true
	_, err := Create(context.Background(), client, CreateInput{
		Name:                         "G",
		MathRenderingLimitsEnabled:   &enabled,
		WebBasedCommitSigningEnabled: &enabled,
		AllowPersonalSnippets:        &enabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, field := range []string{"math_rendering_limits_enabled", "web_based_commit_signing_enabled", "allow_personal_snippets"} {
		if !strings.Contains(body, field) {
			t.Errorf("expected %s in request body, got: %s", field, body)
		}
	}
}

// TestUpdate_NewOptions verifies that the v2.41.0 group update options are sent to the API.
// The test inspects the request body for the new boolean flags.
// It asserts each new flag appears in the marshaled request.
func TestUpdate_NewOptions(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body = string(buf)
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"name":"G","path":"g","full_path":"g","visibility":"private","web_url":"https://x/groups/g"}`)
	})
	client := testutil.NewTestClient(t, mux)

	enabled := false
	_, err := Update(context.Background(), client, UpdateInput{
		GroupID:                      toolutil.StringOrInt("5"),
		MathRenderingLimitsEnabled:   &enabled,
		WebBasedCommitSigningEnabled: &enabled,
		AllowPersonalSnippets:        &enabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, field := range []string{"math_rendering_limits_enabled", "web_based_commit_signing_enabled", "allow_personal_snippets"} {
		if !strings.Contains(body, field) {
			t.Errorf("expected %s in request body, got: %s", field, body)
		}
	}
}
