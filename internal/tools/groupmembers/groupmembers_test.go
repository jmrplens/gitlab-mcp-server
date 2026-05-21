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

// TestGetMember_Success verifies GetMember when success.
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

// TestGetMember_MissingGroupID verifies GetMember when missing group ID.
func TestGetMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetMember(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGetMember_MissingUserID verifies GetMember when missing user ID.
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

// TestGetInheritedMember_Success verifies GetInheritedMember when success.
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

// TestAddMember_Success verifies AddMember when success.
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

// TestAddMember_MissingUserAndUsername verifies AddMember when missing user and username.
func TestAddMember_MissingUserAndUsername(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing user_id and username")
	}
}

// TestAddMember_MissingAccessLevel verifies AddMember when missing access level.
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

// TestEditMember_Success verifies EditMember when success.
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

// TestEditMember_MissingUserID verifies EditMember when missing user ID.
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

// TestRemoveMember_Success verifies RemoveMember when success.
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

// TestRemoveMember_MissingGroupID verifies RemoveMember when missing group ID.
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

// TestShareGroup_Success verifies ShareGroup when success.
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

// TestShareGroup_MissingShareGroupID verifies ShareGroup when missing share group ID.
func TestShareGroup_MissingShareGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", GroupAccess: 30})
	if err == nil {
		t.Fatal("expected error for missing share_group_id")
	}
}

// TestShareGroup_MissingGroupAccess verifies ShareGroup when missing group access.
func TestShareGroup_MissingGroupAccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_access")
	}
}

// ----------------------------------------------
// UnshareGroup
// ----------------------------------------------.

// TestUnshareGroup_Success verifies UnshareGroup when success.
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

// TestUnshareGroup_MissingShareGroupID verifies UnshareGroup when missing share group ID.
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

// TestFormatMemberMarkdown verifies FormatMemberMarkdown.
func TestFormatMemberMarkdown(t *testing.T) {
	md := FormatMemberMarkdown(Output{ID: 10, Username: "dev", Name: "Developer", AccessLevel: 30, AccessLevelDescription: "Developer"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatShareMarkdown verifies FormatShareMarkdown.
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

// TestGetMember_APIError verifies GetMember when API error.
func TestGetMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetMember_CancelledContext verifies GetMember when cancelled context.
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

// TestGetInheritedMember_APIError verifies GetInheritedMember when API error.
func TestGetInheritedMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetInheritedMember(context.Background(), client, GetInput{GroupID: "5", UserID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetInheritedMember_MissingGroupID verifies GetInheritedMember when missing group ID.
func TestGetInheritedMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetInheritedMember(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestGetInheritedMember_MissingUserID verifies GetInheritedMember when missing user ID.
func TestGetInheritedMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetInheritedMember(context.Background(), client, GetInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestGetInheritedMember_CancelledContext verifies GetInheritedMember when cancelled context.
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

// TestAddMember_APIError verifies AddMember when API error.
func TestAddMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", UserID: 1, AccessLevel: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddMember_StatusErrorBranches verifies add-member status-specific hints.
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

// TestAddMember_MissingGroupID verifies AddMember when missing group ID.
func TestAddMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddMember(context.Background(), client, AddInput{UserID: 1, AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestAddMember_CancelledContext verifies AddMember when cancelled context.
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

// TestAddMember_WithUsername verifies AddMember when with username.
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

// TestAddMember_WithExpiresAt verifies AddMember when with expires at.
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

// TestEditMember_APIError verifies EditMember when API error.
func TestEditMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := EditMember(context.Background(), client, EditInput{GroupID: "5", UserID: 10, AccessLevel: 40})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEditMember_MissingGroupID verifies EditMember when missing group ID.
func TestEditMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := EditMember(context.Background(), client, EditInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestEditMember_CancelledContext verifies EditMember when cancelled context.
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

// TestEditMember_WithExpiresAt verifies EditMember when with expires at.
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

// TestRemoveMember_APIError verifies RemoveMember when API error.
func TestRemoveMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := RemoveMember(context.Background(), client, RemoveInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRemoveMember_MissingUserID verifies RemoveMember when missing user ID.
func TestRemoveMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := RemoveMember(context.Background(), client, RemoveInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestRemoveMember_CancelledContext verifies RemoveMember when cancelled context.
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

// TestRemoveMember_WithOptionalFlags verifies RemoveMember flags for with optional.
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

// TestShareGroup_APIError verifies ShareGroup when API error.
func TestShareGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestShareGroup_Conflict verifies already-shared group hints.
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

// TestShareGroup_MissingGroupID verifies ShareGroup when missing group ID.
func TestShareGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ShareGroup(context.Background(), client, ShareInput{ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestShareGroup_CancelledContext verifies ShareGroup when cancelled context.
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

// TestShareGroup_WithExpiresAt verifies ShareGroup when with expires at.
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

// TestUnshareGroup_APIError verifies UnshareGroup when API error.
func TestUnshareGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := UnshareGroup(context.Background(), client, UnshareInput{GroupID: "5", ShareGroupID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUnshareGroup_MissingGroupID verifies UnshareGroup when missing group ID.
func TestUnshareGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := UnshareGroup(context.Background(), client, UnshareInput{ShareGroupID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestUnshareGroup_CancelledContext verifies UnshareGroup when cancelled context.
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

// TestAccessLevelDescription_AllLevels covers AccessLevelDescription with table-driven subtests for all levels.
func TestAccessLevelDescription_AllLevels(t *testing.T) {
	tests := []struct {
		level int
		want  string
	}{
		{0, "No access"},
		{5, "Minimal access"},
		{10, "Guest"},
		{20, "Reporter"},
		{30, "Developer"},
		{40, "Maintainer"},
		{50, "Owner"},
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

// TestConvertMember_FullFields verifies ConvertMember when full fields.
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

// TestConvertMember_MinimalFields verifies ConvertMember when minimal fields.
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

// TestFormatMemberMarkdown_WithAllFields verifies FormatMemberMarkdown when with all fields.
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

// TestFormatMemberMarkdown_Empty verifies FormatMemberMarkdown when empty.
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

// TestFormatMemberMarkdown_NoOptionalFields verifies FormatMemberMarkdown when no optional fields.
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

// TestFormatShareMarkdown_WithAllFields verifies FormatShareMarkdown when with all fields.
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

// TestFormatShareMarkdown_Empty verifies FormatShareMarkdown when empty.
func TestFormatShareMarkdown_Empty(t *testing.T) {
	md := FormatShareMarkdown(ShareOutput{})
	if !strings.Contains(md, "## Group Shared") {
		t.Errorf("expected header in markdown:\n%s", md)
	}
	if strings.Contains(md, "| URL") {
		t.Errorf("should not contain URL for empty output:\n%s", md)
	}
}

// TestFormatShareMarkdown_NoWebURL verifies FormatShareMarkdown when no web URL.
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
