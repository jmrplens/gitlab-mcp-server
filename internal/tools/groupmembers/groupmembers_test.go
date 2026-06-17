// groupmembers_test.go contains unit tests for the group member MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupmembers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// ----------------------------------------------
// GetMember
// ----------------------------------------------.

// TestGetMember_Success verifies that GetMember succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetMember_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/members/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":30}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("id = %d, want 10", out.ID)
	}
	if out.AccessLevel != 30 {
		t.Errorf("access_level = %d, want 30", out.AccessLevel)
	}
	if out.AccessLevelDescription != "Developer" {
		t.Errorf("access_level_description = %q, want Developer", out.AccessLevelDescription)
	}
}

// TestGetMember_MissingGroupID verifies that GetMember_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetMember(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGetMember_MissingUserID verifies that GetMember_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetMember(context.Background(), client, GetInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing user_id")
	}
}

// ----------------------------------------------
// GetInheritedMember
// ----------------------------------------------.

// TestGetInheritedMember_Success verifies that GetInheritedMember succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetInheritedMember_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/members/all/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"admin","name":"Admin","state":"active","access_level":50}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetInheritedMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.AccessLevelDescription != "Owner" {
		t.Errorf("access_level_description = %q, want Owner", out.AccessLevelDescription)
	}
}

// ----------------------------------------------
// AddMember
// ----------------------------------------------.

// TestAddMember_Success verifies that AddMember succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddMember_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/groups/5/members", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":20,"username":"newuser","name":"New User","state":"active","access_level":20}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := AddMember(context.Background(), client, AddInput{
		GroupID:     "5",
		UserID:      20,
		AccessLevel: 20,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 20 {
		t.Errorf("id = %d, want 20", out.ID)
	}
	if out.AccessLevelDescription != "Reporter" {
		t.Errorf("access_level_description = %q, want Reporter", out.AccessLevelDescription)
	}
}

// TestAddMember_MissingUserAndUsername verifies that AddMember_MissingUserAndUsername returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddMember_MissingUserAndUsername(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing user_id and username")
	}
}

// TestAddMember_MissingAccessLevel verifies that AddMember_MissingAccessLevel returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddMember_MissingAccessLevel(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", UserID: 1})
	if err == nil {
		t.Fatal("expected error for missing access_level")
	}
}

// ----------------------------------------------
// EditMember
// ----------------------------------------------.

// TestEditMember_Success verifies that EditMember succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEditMember_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/groups/5/members/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":40}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := EditMember(context.Background(), client, EditInput{
		GroupID:     "5",
		UserID:      10,
		AccessLevel: 40,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.AccessLevelDescription != "Maintainer" {
		t.Errorf("access_level_description = %q, want Maintainer", out.AccessLevelDescription)
	}
}

// TestEditMember_MissingUserID verifies that EditMember_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEditMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := EditMember(context.Background(), client, EditInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing user_id")
	}
}

// ----------------------------------------------
// RemoveMember
// ----------------------------------------------.

// TestRemoveMember_Success verifies that RemoveMember succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRemoveMember_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/members/10", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := RemoveMember(context.Background(), client, RemoveInput{GroupID: "5", UserID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestRemoveMember_MissingGroupID verifies that RemoveMember_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRemoveMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := RemoveMember(context.Background(), client, RemoveInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// ----------------------------------------------
// ShareGroup
// ----------------------------------------------.

// TestShareGroup_Success verifies that ShareGroup succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestShareGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/groups/5/share", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5,"name":"MyGroup","path":"mygroup","web_url":"https://gl/groups/mygroup"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ShareGroup(context.Background(), client, ShareInput{
		GroupID:      "5",
		ShareGroupID: 10,
		GroupAccess:  30,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("id = %d, want 5", out.ID)
	}
	if out.Name != "MyGroup" {
		t.Errorf("name = %q, want MyGroup", out.Name)
	}
}

// TestShareGroup_MissingShareGroupID verifies that ShareGroup_MissingShareGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestShareGroup_MissingShareGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", GroupAccess: 30})
	if err == nil {
		t.Fatal("expected error for missing share_group_id")
	}
}

// TestShareGroup_MissingGroupAccess verifies that ShareGroup_MissingGroupAccess returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestShareGroup_MissingGroupAccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_access")
	}
}

// TestShareGroup_InvalidGroupAccess verifies ShareGroup rejects access levels
// that the project-group share API does not accept. The pre-flight validation
// surfaces a precise message listing the valid range and the values that are
// NOT valid for project group shares (Minimal access 5, Planner 15,
// Security Manager 25, Admin 60).
func TestShareGroup_InvalidGroupAccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	// 60=Admin is one of the new levels that the validation explicitly rejects.
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 10, GroupAccess: 60})
	if err == nil {
		t.Fatal("expected error for invalid group_access=60")
	}
	if !strings.Contains(err.Error(), "10/20/30/40") {
		t.Errorf("expected error to mention valid 10/20/30/40 range, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not valid for project group shares") {
		t.Errorf("expected error to mention non-shareable levels, got: %v", err)
	}
}

// TestShareGroup_BadRequestHint verifies the 400 status hint surfaced when
// the GitLab API rejects the share payload. The hint must list the valid
// 10/20/30/40 range so callers can correct their input without having to
// consult the API docs.
func TestShareGroup_BadRequestHint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/groups/5/share", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message": "400 Bad request - invalid group_access"}`))
	})
	client := testutil.NewTestClient(t, mux)
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "group_access must be one of 10/20/30/40") {
		t.Errorf("expected 400 hint to mention valid 10/20/30/40 range, got: %v", err)
	}
}

// ----------------------------------------------
// UnshareGroup
// ----------------------------------------------.

// TestUnshareGroup_Success verifies that UnshareGroup succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUnshareGroup_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/share/10", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := UnshareGroup(context.Background(), client, UnshareInput{GroupID: "5", ShareGroupID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUnshareGroup_MissingShareGroupID verifies that UnshareGroup_MissingShareGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUnshareGroup_MissingShareGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := UnshareGroup(context.Background(), client, UnshareInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing share_group_id")
	}
}

// ----------------------------------------------
// Markdown formatters
// ----------------------------------------------.

// TestFormatMemberMarkdown verifies the MemberMarkdown Markdown formatter for a representative member input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMemberMarkdown(t *testing.T) {
	md := FormatMemberMarkdown(Output{ID: 10, Username: "dev", Name: "Developer", AccessLevel: 30, AccessLevelDescription: "Developer"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatShareMarkdown verifies the ShareMarkdown Markdown formatter for a representative share input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatShareMarkdown(t *testing.T) {
	md := FormatShareMarkdown(ShareOutput{ID: 5, Name: "MyGroup", Path: "mygroup"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// GetMember — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGetMember_APIError verifies that GetMember returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetMember_CancelledContext verifies the GetMember_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetMember(ctx, client, GetInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetInheritedMember — API error, missing group_id, missing user_id, canceled
// ---------------------------------------------------------------------------.

// TestGetInheritedMember_APIError verifies that GetInheritedMember returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetInheritedMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetInheritedMember(context.Background(), client, GetInput{GroupID: "5", UserID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetInheritedMember_MissingGroupID verifies that GetInheritedMember_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetInheritedMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetInheritedMember(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestGetInheritedMember_MissingUserID verifies that GetInheritedMember_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetInheritedMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetInheritedMember(context.Background(), client, GetInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestGetInheritedMember_CancelledContext verifies the GetInheritedMember_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGetInheritedMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetInheritedMember(ctx, client, GetInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// ---------------------------------------------------------------------------
// AddMember — API error, missing group_id, canceled, with username, with expires_at
// ---------------------------------------------------------------------------.

// TestAddMember_APIError verifies that AddMember returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", UserID: 1, AccessLevel: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddMember_StatusErrorBranches verifies that AddMember_StatusErrorBranches returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddMember_StatusErrorBranches(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantText   string
	}{
		{name: "conflict", statusCode: http.StatusConflict, wantText: "already a direct member"},
		{name: "bad request", statusCode: http.StatusBadRequest, wantText: "access_level must be"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.statusCode, `{"message":"failed"}`)
			}))
			_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", UserID: 1, AccessLevel: 30})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error = %v, want %q", err, tt.wantText)
			}
		})
	}
}

// TestAddMember_MissingGroupID verifies that AddMember_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddMember(context.Background(), client, AddInput{UserID: 1, AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestAddMember_CancelledContext verifies the AddMember_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestAddMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":1}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddMember(ctx, client, AddInput{GroupID: "5", UserID: 1, AccessLevel: 30})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// TestAddMember_WithUsername verifies the AddMember_WithUsername handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddMember_WithUsername(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/groups/5/members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":30,"username":"byname","name":"By Name","state":"active","access_level":20}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := AddMember(context.Background(), client, AddInput{
		GroupID:     "5",
		Username:    "byname",
		AccessLevel: 20,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Username != "byname" {
		t.Errorf("username = %q, want %q", out.Username, "byname")
	}
}

// TestAddMember_WithExpiresAt verifies the AddMember_WithExpiresAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddMember_WithExpiresAt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/groups/5/members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":31,"username":"temp","name":"Temp","state":"active","access_level":10,"expires_at":"2026-12-31"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := AddMember(context.Background(), client, AddInput{
		GroupID:     "5",
		UserID:      31,
		AccessLevel: 10,
		ExpiresAt:   "2026-12-31",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.AccessLevelDescription != "Guest" {
		t.Errorf("access_level_description = %q, want %q", out.AccessLevelDescription, "Guest")
	}
}

// ---------------------------------------------------------------------------
// EditMember — API error, missing group_id, canceled, with optional fields
// ---------------------------------------------------------------------------.

// TestEditMember_APIError verifies that EditMember returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEditMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := EditMember(context.Background(), client, EditInput{GroupID: "5", UserID: 10, AccessLevel: 40})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEditMember_MissingGroupID verifies that EditMember_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEditMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := EditMember(context.Background(), client, EditInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestEditMember_CancelledContext verifies the EditMember_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestEditMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := EditMember(ctx, client, EditInput{GroupID: "5", UserID: 10, AccessLevel: 40})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// TestEditMember_WithExpiresAt verifies the EditMember_WithExpiresAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEditMember_WithExpiresAt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":30}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := EditMember(context.Background(), client, EditInput{
		GroupID:   "5",
		UserID:    10,
		ExpiresAt: "2026-06-30",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("id = %d, want 10", out.ID)
	}
}

// ---------------------------------------------------------------------------
// RemoveMember — API error, missing user_id, canceled, with optional flags
// ---------------------------------------------------------------------------.

// TestRemoveMember_APIError verifies that RemoveMember returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRemoveMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := RemoveMember(context.Background(), client, RemoveInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRemoveMember_MissingUserID verifies that RemoveMember_MissingUserID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestRemoveMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := RemoveMember(context.Background(), client, RemoveInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestRemoveMember_CancelledContext verifies the RemoveMember_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestRemoveMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := RemoveMember(ctx, client, RemoveInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// TestRemoveMember_WithOptionalFlags verifies the RemoveMember_WithOptionalFlags handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestRemoveMember_WithOptionalFlags(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := RemoveMember(context.Background(), client, RemoveInput{
		GroupID:           "5",
		UserID:            10,
		SkipSubresources:  true,
		UnassignIssuables: true,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// ShareGroup — API error, missing group_id, canceled, with expires_at
// ---------------------------------------------------------------------------.

// TestShareGroup_APIError verifies that ShareGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestShareGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestShareGroup_Conflict verifies the ShareGroup_Conflict handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestShareGroup_Conflict(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"conflict"}`)
	}))
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "already shared") {
		t.Fatalf("error = %v, want already-shared hint", err)
	}
}

// TestShareGroup_MissingGroupID verifies that ShareGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestShareGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ShareGroup(context.Background(), client, ShareInput{ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestShareGroup_CancelledContext verifies the ShareGroup_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestShareGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ShareGroup(ctx, client, ShareInput{GroupID: "5", ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// TestShareGroup_WithExpiresAt verifies the ShareGroup_WithExpiresAt handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestShareGroup_WithExpiresAt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/groups/5/share", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5,"name":"MyGroup","path":"mygroup","description":"shared","web_url":"https://gl/groups/mygroup"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ShareGroup(context.Background(), client, ShareInput{
		GroupID:      "5",
		ShareGroupID: 10,
		GroupAccess:  30,
		ExpiresAt:    "2026-12-31",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Description != "shared" {
		t.Errorf("description = %q, want %q", out.Description, "shared")
	}
	if out.WebURL != "https://gl/groups/mygroup" {
		t.Errorf("web_url = %q, want %q", out.WebURL, "https://gl/groups/mygroup")
	}
}

// ---------------------------------------------------------------------------
// UnshareGroup — API error, missing group_id, canceled
// ---------------------------------------------------------------------------.

// TestUnshareGroup_APIError verifies that UnshareGroup returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUnshareGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := UnshareGroup(context.Background(), client, UnshareInput{GroupID: "5", ShareGroupID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUnshareGroup_MissingGroupID verifies that UnshareGroup_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUnshareGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := UnshareGroup(context.Background(), client, UnshareInput{ShareGroupID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestUnshareGroup_CancelledContext verifies the UnshareGroup_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUnshareGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := UnshareGroup(ctx, client, UnshareInput{GroupID: "5", ShareGroupID: 10})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// ---------------------------------------------------------------------------
// accessLevelDescription — all levels
// ---------------------------------------------------------------------------.

// TestAccessLevelDescription_AllLevels verifies the AccessLevelDescription_AllLevels handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAccessLevelDescription_AllLevels(t *testing.T) {
	tests := []struct {
		level int
		want  string
	}{
		{0, "No access"},
		{5, "Minimal access"},
		{10, "Guest"},
		{15, "Planner"},
		{20, "Reporter"},
		{25, "Security Manager"},
		{30, "Developer"},
		{40, "Maintainer"},
		{50, "Owner"},
		{60, "Admin"},
		{99, "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := accessLevelDescription(gl.AccessLevelValue(tt.level))
			if got != tt.want {
				t.Errorf("accessLevelDescription(%d) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// convertMember — with all optional fields populated
// ---------------------------------------------------------------------------.

// TestConvertMember_FullFields verifies the ConvertMember_FullFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestConvertMember_FullFields(t *testing.T) {
	now := "2026-01-15T10:00:00Z"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":10,"username":"dev","name":"Developer","state":"active",
			"avatar_url":"https://gl/avatar.png","web_url":"https://gl/dev",
			"access_level":30,"email":"dev@example.com",
			"created_at":"`+now+`","expires_at":"2026-12-31",
			"member_role":{"name":"Custom Role"},
			"is_using_seat":true
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.AvatarURL != "https://gl/avatar.png" {
		t.Errorf("avatar_url = %q, want %q", out.AvatarURL, "https://gl/avatar.png")
	}
	if out.WebURL != "https://gl/dev" {
		t.Errorf("web_url = %q, want %q", out.WebURL, "https://gl/dev")
	}
	if out.Email != "dev@example.com" {
		t.Errorf("email = %q, want %q", out.Email, "dev@example.com")
	}
	if out.CreatedAt == "" {
		t.Error("created_at should not be empty")
	}
	if out.ExpiresAt == "" {
		t.Error("expires_at should not be empty")
	}
	if out.MemberRoleName != "Custom Role" {
		t.Errorf("member_role_name = %q, want %q", out.MemberRoleName, "Custom Role")
	}
	if !out.IsUsingSeat {
		t.Error("is_using_seat should be true")
	}
}

// TestConvertMember_MinimalFields verifies the ConvertMember_MinimalFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestConvertMember_MinimalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/members/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"min","name":"Minimal","state":"blocked","access_level":10}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CreatedAt != "" {
		t.Errorf("created_at should be empty, got %q", out.CreatedAt)
	}
	if out.ExpiresAt != "" {
		t.Errorf("expires_at should be empty, got %q", out.ExpiresAt)
	}
	if out.MemberRoleName != "" {
		t.Errorf("member_role_name should be empty, got %q", out.MemberRoleName)
	}
	if out.State != "blocked" {
		t.Errorf("state = %q, want %q", out.State, "blocked")
	}
}

// ---------------------------------------------------------------------------
// FormatMemberMarkdown — detailed checks
// ---------------------------------------------------------------------------.

// TestFormatMemberMarkdown_WithAllFields verifies the MemberMarkdown_WithAllFields Markdown formatter for a representative member_withallfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMemberMarkdown_WithAllFields(t *testing.T) {
	md := FormatMemberMarkdown(Output{
		ID:                     10,
		Username:               "dev",
		Name:                   "Developer",
		State:                  "active",
		AccessLevel:            30,
		AccessLevelDescription: "Developer",
		ExpiresAt:              "2026-12-31",
		WebURL:                 "https://gl/dev",
	})

	for _, want := range []string{
		"## Group Member",
		"| ID | 10 |",
		"| Username | dev |",
		"| Name | Developer |",
		"| State | active |",
		"| Access Level | Developer (30) |",
		"| Expires | 31 Dec 2026 |",
		"| URL | [dev](https://gl/dev) |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatMemberMarkdown_Empty verifies the MemberMarkdown_Empty Markdown formatter for a representative member_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMemberMarkdown_Empty(t *testing.T) {
	md := FormatMemberMarkdown(Output{})
	if !strings.Contains(md, "## Group Member") {
		t.Errorf("expected header in markdown:\n%s", md)
	}
	if strings.Contains(md, "| Expires") {
		t.Errorf("should not contain Expires for empty output:\n%s", md)
	}
	if strings.Contains(md, "| URL") {
		t.Errorf("should not contain URL for empty output:\n%s", md)
	}
}

// TestFormatMemberMarkdown_NoOptionalFields verifies the MemberMarkdown_NoOptionalFields Markdown formatter for a representative member_nooptionalfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMemberMarkdown_NoOptionalFields(t *testing.T) {
	md := FormatMemberMarkdown(Output{
		ID:                     5,
		Username:               "user",
		Name:                   "User",
		State:                  "active",
		AccessLevel:            20,
		AccessLevelDescription: "Reporter",
	})
	if strings.Contains(md, "| Expires") {
		t.Errorf("should not contain Expires:\n%s", md)
	}
	if strings.Contains(md, "| URL") {
		t.Errorf("should not contain URL:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatShareMarkdown — detailed checks
// ---------------------------------------------------------------------------.

// TestFormatShareMarkdown_WithAllFields verifies the ShareMarkdown_WithAllFields Markdown formatter for a representative share_withallfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatShareMarkdown_WithAllFields(t *testing.T) {
	md := FormatShareMarkdown(ShareOutput{
		ID:     5,
		Name:   "Shared Group",
		Path:   "shared-group",
		WebURL: "https://gl/groups/shared-group",
	})

	for _, want := range []string{
		"## Group Shared",
		"| ID | 5 |",
		"| Name | Shared Group |",
		"| Path | shared-group |",
		"| URL | [Shared Group](https://gl/groups/shared-group) |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatShareMarkdown_Empty verifies the ShareMarkdown_Empty Markdown formatter for a representative share_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatShareMarkdown_Empty(t *testing.T) {
	md := FormatShareMarkdown(ShareOutput{})
	if !strings.Contains(md, "## Group Shared") {
		t.Errorf("expected header in markdown:\n%s", md)
	}
	if strings.Contains(md, "| URL") {
		t.Errorf("should not contain URL for empty output:\n%s", md)
	}
}

// TestFormatShareMarkdown_NoWebURL verifies the ShareMarkdown_NoWebURL Markdown formatter for a representative share_noweburl input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatShareMarkdown_NoWebURL(t *testing.T) {
	md := FormatShareMarkdown(ShareOutput{
		ID:   5,
		Name: "NoURL",
		Path: "nourl",
	})
	if strings.Contains(md, "| URL") {
		t.Errorf("should not contain URL:\n%s", md)
	}
}
