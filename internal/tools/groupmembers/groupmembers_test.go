// groupmembers_test.go contains unit tests for the group member MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupmembers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ----------------------------------------------
// GetMember
// ----------------------------------------------.

// TestGetMember_Success verifies that GetMember handles the success scenario correctly.
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

// TestGetMember_MissingGroupID verifies that GetMember handles the missing group i d scenario correctly.
func TestGetMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetMember(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestGetMember_MissingUserID verifies that GetMember handles the missing user i d scenario correctly.
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

// TestGetInheritedMember_Success verifies that GetInheritedMember handles the success scenario correctly.
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

// TestAddMember_Success verifies that AddMember handles the success scenario correctly.
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

// TestAddMember_MissingUserAndUsername verifies that AddMember handles the missing user and username scenario correctly.
func TestAddMember_MissingUserAndUsername(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing user_id and username")
	}
}

// TestAddMember_MissingAccessLevel verifies that AddMember handles the missing access level scenario correctly.
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

// TestEditMember_Success verifies that EditMember handles the success scenario correctly.
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

// TestEditMember_MissingUserID verifies that EditMember handles the missing user i d scenario correctly.
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

// TestRemoveMember_Success verifies that RemoveMember handles the success scenario correctly.
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

// TestRemoveMember_MissingGroupID verifies that RemoveMember handles the missing group i d scenario correctly.
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

// TestShareGroup_Success verifies that ShareGroup handles the success scenario correctly.
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

// TestShareGroup_MissingShareGroupID verifies that ShareGroup handles the missing share group i d scenario correctly.
func TestShareGroup_MissingShareGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", GroupAccess: 30})
	if err == nil {
		t.Fatal("expected error for missing share_group_id")
	}
}

// TestShareGroup_MissingGroupAccess verifies that ShareGroup handles the missing group access scenario correctly.
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

// TestUnshareGroup_Success verifies that UnshareGroup handles the success scenario correctly.
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

// TestUnshareGroup_MissingShareGroupID verifies that UnshareGroup handles the missing share group i d scenario correctly.
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

// TestFormatMemberMarkdown verifies the behavior of format member markdown.
func TestFormatMemberMarkdown(t *testing.T) {
	md := FormatMemberMarkdown(Output{ID: 10, Username: "dev", Name: "Developer", AccessLevel: 30, AccessLevelDescription: "Developer"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatShareMarkdown verifies the behavior of format share markdown.
func TestFormatShareMarkdown(t *testing.T) {
	md := FormatShareMarkdown(ShareOutput{ID: 5, Name: "MyGroup", Path: "mygroup"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// GetMember — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGetMember_APIError verifies the behavior of get member a p i error.
func TestGetMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetMember_CancelledContext verifies the behavior of get member cancelled context.
func TestGetMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetMember(ctx, client, GetInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetInheritedMember — API error, missing group_id, missing user_id, canceled
// ---------------------------------------------------------------------------.

// TestGetInheritedMember_APIError verifies the behavior of get inherited member a p i error.
func TestGetInheritedMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := GetInheritedMember(context.Background(), client, GetInput{GroupID: "5", UserID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetInheritedMember_MissingGroupID verifies the behavior of get inherited member missing group i d.
func TestGetInheritedMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetInheritedMember(context.Background(), client, GetInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestGetInheritedMember_MissingUserID verifies the behavior of get inherited member missing user i d.
func TestGetInheritedMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetInheritedMember(context.Background(), client, GetInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestGetInheritedMember_CancelledContext verifies the behavior of get inherited member cancelled context.
func TestGetInheritedMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetInheritedMember(ctx, client, GetInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// ---------------------------------------------------------------------------
// AddMember — API error, missing group_id, canceled, with username, with expires_at
// ---------------------------------------------------------------------------.

// TestAddMember_APIError verifies the behavior of add member a p i error.
func TestAddMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", UserID: 1, AccessLevel: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAddMember_MissingGroupID verifies the behavior of add member missing group i d.
func TestAddMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := AddMember(context.Background(), client, AddInput{UserID: 1, AccessLevel: 30})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestAddMember_CancelledContext verifies the behavior of add member cancelled context.
func TestAddMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":1}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AddMember(ctx, client, AddInput{GroupID: "5", UserID: 1, AccessLevel: 30})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// TestAddMember_WithUsername verifies the behavior of add member with username.
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

// TestAddMember_WithExpiresAt verifies the behavior of add member with expires at.
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

// TestEditMember_APIError verifies the behavior of edit member a p i error.
func TestEditMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := EditMember(context.Background(), client, EditInput{GroupID: "5", UserID: 10, AccessLevel: 40})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEditMember_MissingGroupID verifies the behavior of edit member missing group i d.
func TestEditMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := EditMember(context.Background(), client, EditInput{UserID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestEditMember_CancelledContext verifies the behavior of edit member cancelled context.
func TestEditMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := EditMember(ctx, client, EditInput{GroupID: "5", UserID: 10, AccessLevel: 40})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// TestEditMember_WithExpiresAt verifies the behavior of edit member with expires at.
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

// TestRemoveMember_APIError verifies the behavior of remove member a p i error.
func TestRemoveMember_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := RemoveMember(context.Background(), client, RemoveInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRemoveMember_MissingUserID verifies the behavior of remove member missing user i d.
func TestRemoveMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := RemoveMember(context.Background(), client, RemoveInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error for missing user_id, got nil")
	}
}

// TestRemoveMember_CancelledContext verifies the behavior of remove member cancelled context.
func TestRemoveMember_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RemoveMember(ctx, client, RemoveInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// TestRemoveMember_WithOptionalFlags verifies the behavior of remove member with optional flags.
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

// TestShareGroup_APIError verifies the behavior of share group a p i error.
func TestShareGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestShareGroup_MissingGroupID verifies the behavior of share group missing group i d.
func TestShareGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ShareGroup(context.Background(), client, ShareInput{ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestShareGroup_CancelledContext verifies the behavior of share group cancelled context.
func TestShareGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":5}`)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ShareGroup(ctx, client, ShareInput{GroupID: "5", ShareGroupID: 10, GroupAccess: 30})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// TestShareGroup_WithExpiresAt verifies the behavior of share group with expires at.
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

// TestUnshareGroup_APIError verifies the behavior of unshare group a p i error.
func TestUnshareGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := UnshareGroup(context.Background(), client, UnshareInput{GroupID: "5", ShareGroupID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUnshareGroup_MissingGroupID verifies the behavior of unshare group missing group i d.
func TestUnshareGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := UnshareGroup(context.Background(), client, UnshareInput{ShareGroupID: 10})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestUnshareGroup_CancelledContext verifies the behavior of unshare group cancelled context.
func TestUnshareGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := UnshareGroup(ctx, client, UnshareInput{GroupID: "5", ShareGroupID: 10})
	if err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}

// ---------------------------------------------------------------------------
// accessLevelDescription — all levels
// ---------------------------------------------------------------------------.

// TestAccessLevelDescription_AllLevels validates access level description all levels across multiple scenarios using table-driven subtests.
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

// TestConvertMember_FullFields verifies the behavior of convert member full fields.
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

// TestConvertMember_MinimalFields verifies the behavior of convert member minimal fields.
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

// TestFormatMemberMarkdown_WithAllFields verifies the behavior of format member markdown with all fields.
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

// TestFormatMemberMarkdown_Empty verifies the behavior of format member markdown empty.
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

// TestFormatMemberMarkdown_NoOptionalFields verifies the behavior of format member markdown no optional fields.
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

// TestFormatShareMarkdown_WithAllFields verifies the behavior of format share markdown with all fields.
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

// TestFormatShareMarkdown_Empty verifies the behavior of format share markdown empty.
func TestFormatShareMarkdown_Empty(t *testing.T) {
	md := FormatShareMarkdown(ShareOutput{})
	if !strings.Contains(md, "## Group Shared") {
		t.Errorf("expected header in markdown:\n%s", md)
	}
	if strings.Contains(md, "| URL") {
		t.Errorf("should not contain URL for empty output:\n%s", md)
	}
}

// TestFormatShareMarkdown_NoWebURL verifies the behavior of format share markdown no web u r l.
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

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 7 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newGroupMembersMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get", "gitlab_group_member_get", map[string]any{"group_id": "5", "user_id": 10}},
		{"get_inherited", "gitlab_group_member_get_inherited", map[string]any{"group_id": "5", "user_id": 10}},
		{"add", "gitlab_group_member_add", map[string]any{"group_id": "5", "user_id": 20, "access_level": 30}},
		{"edit", "gitlab_group_member_edit", map[string]any{"group_id": "5", "user_id": 10, "access_level": 40}},
		{"remove", "gitlab_group_member_remove", map[string]any{"group_id": "5", "user_id": 10}},
		{"share", "gitlab_group_share", map[string]any{"group_id": "5", "share_group_id": 10, "group_access": 30}},
		{"unshare", "gitlab_group_unshare", map[string]any{"group_id": "5", "share_group_id": 10}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory — individual tools
// ---------------------------------------------------------------------------.

// newGroupMembersMCPSession is an internal helper for the groupmembers package.
func newGroupMembersMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	memberJSON := `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":30}`
	groupJSON := `{"id":5,"name":"MyGroup","path":"mygroup","web_url":"https://gl/groups/mygroup"}`

	handler := http.NewServeMux()

	// Get group member
	handler.HandleFunc("GET /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})

	// Get inherited group member
	handler.HandleFunc("GET /api/v4/groups/5/members/all/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, memberJSON)
	})

	// Add group member
	handler.HandleFunc("POST /api/v4/groups/5/members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":20,"username":"newuser","name":"New User","state":"active","access_level":30}`)
	})

	// Edit group member
	handler.HandleFunc("PUT /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","name":"Developer","state":"active","access_level":40}`)
	})

	// Remove group member
	handler.HandleFunc("DELETE /api/v4/groups/5/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Share group
	handler.HandleFunc("POST /api/v4/groups/5/share", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, groupJSON)
	})

	// Unshare group
	handler.HandleFunc("DELETE /api/v4/groups/5/share/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
