// group_members_test.go contains unit tests for the group member MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupmembers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// descFor returns the canonical access-level label for an output's access
// level, mirroring how markdown derives the human-readable role.
func descFor(out Output) string {
	return toolutil.AccessLevelDescription(gl.AccessLevelValue(out.AccessLevel))
}

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
	if descFor(out) != "Developer" {
		t.Errorf("access level description = %q, want Developer", descFor(out))
	}
}

// TestGetMember_ReadsWhatTheSDKDoesNotModel verifies a member carries, beside
// what client-go decoded, the fields lib/api/entities/member.rb sends and
// gl.GroupMember does not: locked and membership_state on every member, and
// two_factor_enabled, group_scim_identity and override when the caller may
// see them.
func TestGetMember_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/members/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","access_level":30,"locked":true,`+
			`"membership_state":"awaiting","two_factor_enabled":false,"override":true,`+
			`"group_scim_identity":{"extern_uid":"s1","group_id":5,"active":true}}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Locked || out.MembershipState != "awaiting" || out.TwoFactorEnabled == nil || *out.TwoFactorEnabled ||
		out.Override == nil || !*out.Override || out.GroupSCIMIdentity == nil || out.GroupSCIMIdentity.ExternUID != "s1" {
		t.Errorf("GetMember() = %+v, want the captured fields", out)
	}
}

// TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds to every handler that presents a
// member: GitLab's answer decodes for the SDK and not for the fields read
// beside it, and the handler reports it rather than swallowing it.
func TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","access_level":30,"locked":"not-a-bool"}`)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "get", Call: func() error {
			_, err := GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
			return err
		}},
		{Name: "get inherited", Call: func() error {
			_, err := GetInheritedMember(context.Background(), client, GetInput{GroupID: "5", UserID: 10})
			return err
		}},
		{Name: "add", Call: func() error {
			_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", UserID: 10, AccessLevel: 30})
			return err
		}},
		{Name: "edit", Call: func() error {
			_, err := EditMember(context.Background(), client, EditInput{GroupID: "5", UserID: 10, AccessLevel: 30})
			return err
		}},
	})
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
	if descFor(out) != "Owner" {
		t.Errorf("access level description = %q, want Owner", descFor(out))
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
	if descFor(out) != "Reporter" {
		t.Errorf("access level description = %q, want Reporter", descFor(out))
	}
}

// TestAddMember_MemberRoleID verifies that a non-zero member_role_id is sent
// in the AddGroupMember request body (Premium/Ultimate custom-role path).
func TestAddMember_MemberRoleID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/groups/5/members", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"member_role_id":7`) {
			t.Errorf("request body missing member_role_id: %s", body)
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":20,"username":"u","name":"U","state":"active","access_level":30,"member_role":{"id":7,"name":"Custom"}}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := AddMember(context.Background(), client, AddInput{
		GroupID: "5", UserID: 20, AccessLevel: 30, MemberRoleID: 7, Username: "u", ExpiresAt: "2026-12-31",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.MemberRole == nil || out.MemberRole.ID != 7 {
		t.Errorf("member_role not mapped: %+v", out.MemberRole)
	}
}

// TestEditMember_MemberRoleID verifies that a non-zero member_role_id is sent
// in the EditGroupMember request body.
func TestEditMember_MemberRoleID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/groups/5/members/10", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"member_role_id":9`) {
			t.Errorf("request body missing member_role_id: %s", body)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"username":"dev","name":"Dev","state":"active","access_level":40}`)
	})
	client := testutil.NewTestClient(t, mux)

	_, err := EditMember(context.Background(), client, EditInput{
		GroupID: "5", UserID: 10, AccessLevel: 40, MemberRoleID: 9, ExpiresAt: "2026-12-31",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
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
	if descFor(out) != "Maintainer" {
		t.Errorf("access level description = %q, want Maintainer", descFor(out))
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
	md := FormatMemberMarkdown(Output{
		ID: 10, Username: "dev", Name: "Developer", AccessLevel: 30,
		MemberRole: &MemberRoleOutput{ID: 7, Name: "Custom Role"},
	})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
	if !strings.Contains(md, "Developer") {
		t.Errorf("markdown should contain derived access level label, got:\n%s", md)
	}
	if !strings.Contains(md, "Custom Role") {
		t.Errorf("markdown should contain member role name, got:\n%s", md)
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
	if descFor(out) != "Guest" {
		t.Errorf("access level description = %q, want %q", descFor(out), "Guest")
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
		{99, "Level 99"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := toolutil.AccessLevelDescription(gl.AccessLevelValue(tt.level))
			if got != tt.want {
				t.Errorf("AccessLevelDescription(%d) = %q, want %q", tt.level, got, tt.want)
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
			"access_level":30,"email":"dev@example.com","public_email":"dev@public.example.com",
			"created_at":"`+now+`","expires_at":"2026-12-31",
			"created_by":{"id":99,"username":"owner","name":"Owner","state":"active","avatar_url":"https://gl/owner.png","web_url":"https://gl/owner"},
			"group_saml_identity":{"extern_uid":"uid-123","provider":"okta","saml_provider_id":4},
			"member_role":{"id":7,"name":"Custom Role","description":"role desc","group_id":5,"base_access_level":30,"read_code":true,"manage_deploy_tokens":true},
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
	if out.PublicEmail != "dev@public.example.com" {
		t.Errorf("public_email = %q, want %q", out.PublicEmail, "dev@public.example.com")
	}
	if !out.IsUsingSeat {
		t.Error("is_using_seat should be true")
	}
	assertFullSubObjects(t, out)
}

// assertFullSubObjects verifies the created_by, group_saml_identity, and
// member_role sub-objects mapped by convertMember in TestConvertMember_FullFields.
func assertFullSubObjects(t *testing.T, out Output) {
	t.Helper()
	if out.CreatedBy == nil {
		t.Fatal("created_by should not be nil")
	}
	if out.CreatedBy.ID != 99 || out.CreatedBy.Username != "owner" || out.CreatedBy.WebURL != "https://gl/owner" {
		t.Errorf("created_by mismatch: %+v", out.CreatedBy)
	}
	if out.GroupSAMLIdentity == nil {
		t.Fatal("group_saml_identity should not be nil")
	}
	if out.GroupSAMLIdentity.ExternUID != "uid-123" || out.GroupSAMLIdentity.Provider != "okta" || out.GroupSAMLIdentity.SAMLProviderID != 4 {
		t.Errorf("group_saml_identity mismatch: %+v", out.GroupSAMLIdentity)
	}
	if out.MemberRole == nil {
		t.Fatal("member_role should not be nil")
	}
	if out.MemberRole.ID != 7 || out.MemberRole.Name != "Custom Role" || out.MemberRole.GroupID != 5 ||
		out.MemberRole.BaseAccessLevel != 30 || !out.MemberRole.ReadCode || !out.MemberRole.ManageDeployTokens {
		t.Errorf("member_role mismatch: %+v", out.MemberRole)
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
	if out.MemberRole != nil {
		t.Errorf("member_role should be nil, got %+v", out.MemberRole)
	}
	if out.CreatedBy != nil {
		t.Errorf("created_by should be nil, got %+v", out.CreatedBy)
	}
	if out.GroupSAMLIdentity != nil {
		t.Errorf("group_saml_identity should be nil, got %+v", out.GroupSAMLIdentity)
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
		ID:          10,
		Username:    "dev",
		Name:        "Developer",
		State:       "active",
		AccessLevel: 30,
		MemberRole:  &MemberRoleOutput{ID: 7, Name: "Custom Role"},
		ExpiresAt:   "2026-12-31",
		WebURL:      "https://gl/dev",
	})

	for _, want := range []string{
		"## Group Member",
		"| ID | 10 |",
		"| Username | dev |",
		"| Name | Developer |",
		"| State | active |",
		"| Access Level | Developer (30) |",
		"| Member Role | Custom Role |",
		"| Expires | 31 Dec 2026 |",
		"| URL | [dev](https://gl/dev) |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
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
		ID:          5,
		Username:    "user",
		Name:        "User",
		State:       "active",
		AccessLevel: 20,
	})
	if !strings.Contains(md, "| Access Level | Reporter (20) |") {
		t.Errorf("expected derived Reporter access level:\n%s", md)
	}
	if strings.Contains(md, "| Member Role") {
		t.Errorf("should not contain Member Role when nil:\n%s", md)
	}
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
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

// TestListBillableMembers_Success verifies that ListBillableMembers succeeds,
// mirrors the SDK fields 1:1, applies search/sort, and surfaces pagination.
// The test exercises the GET /groups/:id/billable_members path.
func TestListBillableMembers_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("search"); got != "dev" {
			t.Errorf("search = %q, want dev", got)
		}
		if got := r.URL.Query().Get("sort"); got != "name_asc" {
			t.Errorf("sort = %q, want name_asc", got)
		}
		if got := r.URL.Query().Get("order_by"); got != "last_activity_on" {
			t.Errorf("order_by = %q, want last_activity_on", got)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":10,"username":"dev","name":"Developer","state":"active","avatar_url":"https://gl/a.png","web_url":"https://gl/dev","email":"dev@x.io","last_activity_on":"2026-06-01","membership_type":"group_member","removable":true,"created_at":"2026-01-01T00:00:00Z","is_last_owner":false,"last_login_at":"2026-06-20T08:00:00Z"}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListBillableMembers(t.Context(), client, ListBillableMembersInput{GroupID: "5", Search: "dev", Sort: "name_asc", OrderBy: "last_activity_on"})
	if err != nil {
		t.Fatalf("ListBillableMembers error: %v", err)
	}
	if len(out.Members) != 1 {
		t.Fatalf("len(Members) = %d, want 1", len(out.Members))
	}
	m := out.Members[0]
	if m.ID != 10 || m.Username != "dev" || m.Name != "Developer" || m.State != "active" {
		t.Errorf("unexpected member identity: %+v", m)
	}
	if m.Email != "dev@x.io" || m.MembershipType != "group_member" || !m.Removable || m.IsLastOwner {
		t.Errorf("unexpected member fields: %+v", m)
	}
	if m.LastActivityOn != "2026-06-01" {
		t.Errorf("LastActivityOn = %q", m.LastActivityOn)
	}
	if m.CreatedAt == "" || m.LastLoginAt == "" {
		t.Errorf("expected created_at and last_login_at set: %+v", m)
	}
	if out.Pagination.TotalItems != 1 {
		t.Errorf("pagination total = %d, want 1", out.Pagination.TotalItems)
	}
}

// TestListBillableMembers_MissingGroupID verifies validation when group_id is
// empty.
func TestListBillableMembers_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListBillableMembers(t.Context(), client, ListBillableMembersInput{}); err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestListBillableMembers_APIError verifies the 404 error path is wrapped with
// a useful hint.
func TestListBillableMembers_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	if _, err := ListBillableMembers(t.Context(), client, ListBillableMembersInput{GroupID: "5"}); err == nil {
		t.Fatal("expected error for 404")
	}
}

// ----------------------------------------------
// ListBillableMemberMemberships
// ----------------------------------------------.

// TestListBillableMemberMemberships_Success verifies the memberships list
// mirrors the SDK 1:1 including the access_level sub-object and pagination.
func TestListBillableMemberMemberships_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members/10/memberships", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("order_by"); got != "id" {
			t.Errorf("order_by = %q, want id", got)
		}
		if got := r.URL.Query().Get("sort"); got != "desc" {
			t.Errorf("sort = %q, want desc", got)
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":99,"source_id":7,"source_full_name":"Org / Team","source_members_url":"https://gl/groups/team/-/group_members","created_at":"2026-01-01T00:00:00Z","expires_at":"2026-12-31","access_level":{"integer_value":30,"string_value":"Developer"}}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListBillableMemberMemberships(t.Context(), client, ListBillableMemberMembershipsInput{GroupID: "5", UserID: 10, OrderBy: "id", Sort: "desc"})
	if err != nil {
		t.Fatalf("ListBillableMemberMemberships error: %v", err)
	}
	if len(out.Memberships) != 1 {
		t.Fatalf("len(Memberships) = %d, want 1", len(out.Memberships))
	}
	ms := out.Memberships[0]
	if ms.ID != 99 || ms.SourceID != 7 || ms.SourceFullName != "Org / Team" {
		t.Errorf("unexpected membership: %+v", ms)
	}
	if ms.AccessLevel == nil || ms.AccessLevel.IntegerValue != 30 || ms.AccessLevel.StringValue != "Developer" {
		t.Errorf("unexpected access_level: %+v", ms.AccessLevel)
	}
	if ms.CreatedAt == "" || ms.ExpiresAt == "" {
		t.Errorf("expected created_at and expires_at set: %+v", ms)
	}
	if out.Pagination.TotalItems != 1 {
		t.Errorf("pagination total = %d, want 1", out.Pagination.TotalItems)
	}
}

// TestListBillableMemberMemberships_MissingGroupID verifies validation when
// group_id is empty.
func TestListBillableMemberMemberships_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListBillableMemberMemberships(t.Context(), client, ListBillableMemberMembershipsInput{UserID: 10}); err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestListBillableMemberMemberships_MissingUserID verifies validation when
// user_id is zero.
func TestListBillableMemberMemberships_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := ListBillableMemberMemberships(t.Context(), client, ListBillableMemberMembershipsInput{GroupID: "5"}); err == nil {
		t.Fatal("expected error for missing user_id")
	}
}

// TestListBillableMemberMemberships_APIError verifies the 404 error path.
func TestListBillableMemberMemberships_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members/10/memberships", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	if _, err := ListBillableMemberMemberships(t.Context(), client, ListBillableMemberMembershipsInput{GroupID: "5", UserID: 10}); err == nil {
		t.Fatal("expected error for 404")
	}
}

// ----------------------------------------------
// RemoveBillableMember
// ----------------------------------------------.

// TestRemoveBillableMember_Success verifies the DELETE path succeeds.
func TestRemoveBillableMember_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	if err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10}); err != nil {
		t.Fatalf("RemoveBillableMember error: %v", err)
	}
}

// TestRemoveBillableMember_Output verifies the DeleteOutput wrapper message.
func TestRemoveBillableMember_Output(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	out, err := removeBillableMemberOutput(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10})
	if err != nil {
		t.Fatalf("removeBillableMemberOutput error: %v", err)
	}
	if out.Status != "success" || out.Message != "Successfully removed billable group member." {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestRemoveBillableMember_OutputError verifies the DeleteOutput wrapper
// propagates the underlying error (empty group_id) instead of a success.
func TestRemoveBillableMember_OutputError(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if _, err := removeBillableMemberOutput(t.Context(), client, RemoveBillableMemberInput{UserID: 10}); err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestRemoveBillableMember_MissingGroupID verifies validation when group_id is
// empty.
func TestRemoveBillableMember_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{UserID: 10}); err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// TestRemoveBillableMember_MissingUserID verifies validation when user_id is
// zero.
func TestRemoveBillableMember_MissingUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5"}); err == nil {
		t.Fatal("expected error for missing user_id")
	}
}

// TestRemoveBillableMember_Forbidden verifies the 403 hint path (last owner).
func TestRemoveBillableMember_Forbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "Owner role") {
		t.Errorf("expected owner-role hint, got: %v", err)
	}
}

// TestRemoveBillableMember_BadRequest verifies the 400 hint path (not
// removable).
func TestRemoveBillableMember_BadRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "removable") {
		t.Errorf("expected removable hint, got: %v", err)
	}
}

// TestRemoveBillableMember_NotFound verifies the 404 error path.
func TestRemoveBillableMember_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/groups/5/billable_members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	if err := RemoveBillableMember(t.Context(), client, RemoveBillableMemberInput{GroupID: "5", UserID: 10}); err == nil {
		t.Fatal("expected error for 404")
	}
}

// ----------------------------------------------
// The user keys GitLab merges into a member and a billable member
// ----------------------------------------------.

// billableMembersClient answers the billable members list with one body.
func billableMembersClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/groups/5/billable_members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, body,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	})
	return testutil.NewTestClient(t, mux)
}

// TestListBillableMembers_SentUserKeys verifies the two keys
// ee/lib/api/entities/billable_member.rb inherits from UserBasic and sends on
// every member reach the output. client-go's BillableGroupMember models
// neither, so only the read beside its decode can carry them.
//
// The body also spells avatar_path and custom_attributes, which this route
// cannot send because it declares neither only_path nor with_custom_attributes,
// and the published member is held to carrying no trace of either.
func TestListBillableMembers_SentUserKeys(t *testing.T) {
	const body = `[{"id":10,"username":"dev","public_email":"dev@public.io","name":"Developer","state":"active",` +
		`"locked":true,"avatar_url":"https://gl/a.png","avatar_path":"/uploads/a.png",` +
		`"custom_attributes":[{"key":"cost_center","value":"rnd"}],"web_url":"https://gl/dev"}]`
	out, err := ListBillableMembers(t.Context(), billableMembersClient(t, body), ListBillableMembersInput{GroupID: "5"})
	if err != nil {
		t.Fatalf("ListBillableMembers: %v", err)
	}
	if len(out.Members) != 1 {
		t.Fatalf("len(Members) = %d, want 1", len(out.Members))
	}
	m := out.Members[0]
	if m.PublicEmail != "dev@public.io" {
		t.Errorf("public_email = %q, want dev@public.io", m.PublicEmail)
	}
	if !m.Locked {
		t.Error("locked = false, want true")
	}
	assertNoOptionGatedUserKey(t, m)
}

// assertNoOptionGatedUserKey holds a marshaled value to carrying neither of
// the two UserBasic keys that wait on a presenter option.
//
// No route of this package declares only_path or with_custom_attributes, so
// GitLab cannot send either and the surface must not claim it can. The audit
// answers the matching findings with entity-option-no-endpoint-passes
// declarations; this is what stops the code drifting back from them.
func assertNoOptionGatedUserKey(t *testing.T, published any) {
	t.Helper()
	encoded, err := json.Marshal(published)
	if err != nil {
		t.Fatalf("marshal the published value: %v", err)
	}
	for _, key := range []string{"avatar_path", "custom_attributes"} {
		t.Run("no "+key, func(t *testing.T) {
			if strings.Contains(string(encoded), key) {
				t.Errorf("published %s, which no route of this package can send:\n%s", key, encoded)
			}
		})
	}
}

// TestListBillableMembers_UnconditionalUserKeysSurviveAThinBody verifies the
// two keys GitLab always sends are still read off a body carrying nothing
// else, so the assertions above cannot pass on an output that is empty.
func TestListBillableMembers_UnconditionalUserKeysSurviveAThinBody(t *testing.T) {
	const body = `[{"id":10,"username":"dev","public_email":"dev@public.io","name":"Developer",` +
		`"state":"active","locked":true,"web_url":"https://gl/dev"}]`
	out, err := ListBillableMembers(t.Context(), billableMembersClient(t, body), ListBillableMembersInput{GroupID: "5"})
	if err != nil {
		t.Fatalf("ListBillableMembers: %v", err)
	}
	m := out.Members[0]
	if m.PublicEmail == "" || !m.Locked {
		t.Errorf("the unconditional keys were dropped: %+v", m)
	}
	assertNoOptionGatedUserKey(t, m)
}

// TestListBillableMembers_PairsEachExtraWithItsOwnMember verifies the reader
// pairs the extra read at one position with the member decoded at the same
// one, which a reader reusing the first extra would fail only on a page of
// more than one.
func TestListBillableMembers_PairsEachExtraWithItsOwnMember(t *testing.T) {
	const body = `[{"id":10,"username":"dev","locked":true,"public_email":"dev@public.io"},` +
		`{"id":11,"username":"ops","locked":false,"public_email":"ops@public.io"}]`
	out, err := ListBillableMembers(t.Context(), billableMembersClient(t, body), ListBillableMembersInput{GroupID: "5"})
	if err != nil {
		t.Fatalf("ListBillableMembers: %v", err)
	}
	if len(out.Members) != 2 {
		t.Fatalf("len(Members) = %d, want 2", len(out.Members))
	}
	if !out.Members[0].Locked || out.Members[0].PublicEmail != "dev@public.io" {
		t.Errorf("first member = %+v, want the first extra", out.Members[0])
	}
	if out.Members[1].Locked || out.Members[1].PublicEmail != "ops@public.io" {
		t.Errorf("second member = %+v, want the second extra", out.Members[1])
	}
}

// TestListBillableMembers_UnreadableCapturedFields verifies the handler
// reports the captured response's decode failure rather than a half-filled
// member. The SDK's BillableGroupMember has no locked, so only the read
// beside it can notice that GitLab sent a string there.
func TestListBillableMembers_UnreadableCapturedFields(t *testing.T) {
	const poisoned = `[{"id":10,"username":"dev","locked":"not-a-bool"}]`
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{{
		Name: "billable_members_list",
		Call: func() error {
			_, err := ListBillableMembers(t.Context(), billableMembersClient(t, poisoned), ListBillableMembersInput{GroupID: "5"})
			return err
		},
	}})
}

// TestGroupMember_OptionGatedUserKeysAreNotPublished verifies no handler that
// answers with a group member publishes avatar_path or custom_attributes, even
// when the body spells both.
//
// lib/api/entities/user_basic.rb sends each only when the presenter is given
// only_path or with_custom_attributes, and none of the twelve group-member
// routes declares either, so the keys can never be on one of their responses.
// The group member lists do declare show_seat_info, which is why is_using_seat
// is published on this type and the other two are not: the answer is per route
// set, not per entity.
func TestGroupMember_OptionGatedUserKeysAreNotPublished(t *testing.T) {
	const sent = `{"id":7,"username":"dev","name":"Developer","state":"active","access_level":30,` +
		`"locked":true,"is_using_seat":true,` +
		`"avatar_path":"/uploads/dev.png","custom_attributes":[{"key":"team","value":"core"}]}`
	for _, memberCall := range groupMemberCalls {
		t.Run(memberCall.name, func(t *testing.T) {
			out, err := memberCall.call(groupMemberClient(t, sent))
			if err != nil {
				t.Fatalf("%s: %v", memberCall.name, err)
			}
			if !out.Locked {
				t.Error("locked = false, want the captured value, so the assertion below is not vacuous")
			}
			assertNoOptionGatedUserKey(t, out)
		})
	}
}

// groupMemberCalls are the four handlers that answer with one group member.
var groupMemberCalls = []struct {
	name string
	call func(client *gitlabclient.Client) (Output, error)
}{
	{name: "get", call: func(client *gitlabclient.Client) (Output, error) {
		return GetMember(context.Background(), client, GetInput{GroupID: "5", UserID: 7})
	}},
	{name: "get_inherited", call: func(client *gitlabclient.Client) (Output, error) {
		return GetInheritedMember(context.Background(), client, GetInput{GroupID: "5", UserID: 7})
	}},
	{name: "add", call: func(client *gitlabclient.Client) (Output, error) {
		return AddMember(context.Background(), client, AddInput{GroupID: "5", UserID: 7, AccessLevel: 30})
	}},
	{name: "edit", call: func(client *gitlabclient.Client) (Output, error) {
		return EditMember(context.Background(), client, EditInput{GroupID: "5", UserID: 7, AccessLevel: 30})
	}},
}

// groupMemberClient answers every group-member route with one body.
func groupMemberClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
}

// recordGroupMemberBody answers one group-member write and hands back the body
// the handler sent, which is the only place its optional parameters appear.
func recordGroupMemberBody(t *testing.T, send func(*gitlabclient.Client) error) string {
	t.Helper()
	var sent string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("read request body: %v", readErr)
		}
		sent = string(body)
		testutil.RespondJSON(w, http.StatusOK, `{"id":7,"username":"dev","access_level":30}`)
	}))
	if err := send(client); err != nil {
		t.Fatalf("write: %v", err)
	}
	return sent
}

// TestGroupMemberWrites_OptionalParametersReachTheRequestOnlyWhenGiven
// verifies the three write handlers put each optional parameter in the request
// body when the input carries one, and leave it out when it does not.
//
// GitLab answers with the member either way, so the output cannot tell a
// parameter that was sent from one that was dropped: a username silently lost
// would add nobody, and an expiry silently lost would make a temporary
// membership permanent.
//
// The fragments carry values rather than bare keys because two of the SDK's
// options structs write expires_at as null when it is unset, so the key is on
// the wire either way and only the value says whether the handler filled it.
func TestGroupMemberWrites_OptionalParametersReachTheRequestOnlyWhenGiven(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		full    func(*gitlabclient.Client) error
		bare    func(*gitlabclient.Client) error
		present []string
		absent  []string
	}{
		{
			name: "add",
			full: func(client *gitlabclient.Client) error {
				_, err := AddMember(context.Background(), client, AddInput{
					GroupID: "5", UserID: 7, Username: "dev", AccessLevel: 30,
					ExpiresAt: "2027-01-31", MemberRoleID: 3,
				})
				return err
			},
			bare: func(client *gitlabclient.Client) error {
				_, err := AddMember(context.Background(), client, AddInput{GroupID: "5", Username: "dev", AccessLevel: 30})
				return err
			},
			present: []string{`"user_id":7`, `"username":"dev"`, `"expires_at":"2027-01-31"`, `"member_role_id":3`},
			absent:  []string{`"user_id"`, `"expires_at":"`, `"member_role_id"`},
		},
		{
			name: "edit",
			full: func(client *gitlabclient.Client) error {
				_, err := EditMember(context.Background(), client, EditInput{
					GroupID: "5", UserID: 7, AccessLevel: 30, ExpiresAt: "2027-01-31", MemberRoleID: 3,
				})
				return err
			},
			bare: func(client *gitlabclient.Client) error {
				_, err := EditMember(context.Background(), client, EditInput{GroupID: "5", UserID: 7})
				return err
			},
			present: []string{`"access_level":30`, `"expires_at":"2027-01-31"`, `"member_role_id":3`},
			absent:  []string{`"access_level"`, `"expires_at":"`, `"member_role_id"`},
		},
		{
			name: "share",
			full: func(client *gitlabclient.Client) error {
				_, err := ShareGroup(context.Background(), client, ShareInput{
					GroupID: "5", ShareGroupID: 9, GroupAccess: 30, ExpiresAt: "2027-01-31",
				})
				return err
			},
			bare: func(client *gitlabclient.Client) error {
				_, err := ShareGroup(context.Background(), client, ShareInput{GroupID: "5", ShareGroupID: 9, GroupAccess: 30})
				return err
			},
			present: []string{`"expires_at":"2027-01-31"`},
			absent:  []string{`"expires_at":"`},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			sent := recordGroupMemberBody(t, testCase.full)
			for _, want := range testCase.present {
				t.Run("sends"+want, func(t *testing.T) {
					if !strings.Contains(sent, want) {
						t.Errorf("body %q is missing %s", sent, want)
					}
				})
			}
			bare := recordGroupMemberBody(t, testCase.bare)
			for _, notWanted := range testCase.absent {
				t.Run("omits"+notWanted, func(t *testing.T) {
					if strings.Contains(bare, notWanted) {
						t.Errorf("body %q carries %s the input did not name", bare, notWanted)
					}
				})
			}
		})
	}
}

// TestShareGroup_AcceptsEveryValidGroupAccess verifies each of the four levels
// GitLab accepts for a group share is let through, not only the one the rest
// of the tests happen to use. The switch names them as one case, so a level
// dropped from that list would refuse a share the endpoint supports.
func TestShareGroup_AcceptsEveryValidGroupAccess(t *testing.T) {
	for _, level := range []int{10, 20, 30, 40} {
		t.Run(toolutil.AccessLevelDescription(gl.AccessLevelValue(level)), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v4/groups/5/share", func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusCreated, `{"id":9,"name":"Shared","path":"shared","web_url":"https://gl/shared"}`)
			})
			out, err := ShareGroup(t.Context(), testutil.NewTestClient(t, mux), ShareInput{
				GroupID: "5", ShareGroupID: 9, GroupAccess: level,
			})
			if err != nil {
				t.Fatalf("ShareGroup at level %d: %v", level, err)
			}
			if out.ID != 9 {
				t.Errorf("ShareGroup at level %d = %+v, want the shared group", level, out)
			}
		})
	}
}

// TestApplyGroupMemberMeta_KeepsWhatAnEntryDoesNotCarry verifies the fallback
// every guard in the applier exists for: an entry that fills nothing leaves
// the generic options exactly as they were.
//
// Every entry of the real table fills every field, so this contract is
// unreachable through [decorateGroupMemberMeta] and is asserted against the
// applier directly, which is why that function takes the entry.
func TestApplyGroupMemberMeta_KeepsWhatAnEntryDoesNotCarry(t *testing.T) {
	options := groupMemberOptions("gitlab_group_member_get")
	generic := options

	applyGroupMemberMeta(&options, "gitlab_group_member_get", groupMemberActionMetaEntry{})

	if options.Usage != generic.Usage {
		t.Errorf("Usage = %q, want the generic %q", options.Usage, generic.Usage)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != "gitlab_group_member_get" {
		t.Errorf("Aliases = %v, want only the tool name", options.Aliases)
	}
	if len(options.RelatedActions) != len(generic.RelatedActions) {
		t.Errorf("RelatedActions = %v, want the generic %v", options.RelatedActions, generic.RelatedActions)
	}
	if options.ParameterGuidance != nil {
		t.Errorf("ParameterGuidance = %v, want none", options.ParameterGuidance)
	}
	if options.IndividualTool.Description != generic.IndividualTool.Description {
		t.Errorf("Description = %q, want the generic %q", options.IndividualTool.Description, generic.IndividualTool.Description)
	}
}

// TestBillableMarkdown_OptionalCellsFallBackToPlainText verifies the other side
// of the four guards in the billable formatters: a member with no web URL is
// named in plain text rather than linked, and a membership with no source URL,
// no access level and no expiry renders those cells empty instead of panicking
// or printing a nil.
func TestBillableMarkdown_OptionalCellsFallBackToPlainText(t *testing.T) {
	members := FormatBillableMembersMarkdown(BillableMembersOutput{
		Members: []BillableMemberOutput{{ID: 10, Username: "dev", Name: "Developer", State: "active"}},
	})
	if strings.Contains(members, "[dev](") {
		t.Errorf("member with no web_url was linked:\n%s", members)
	}
	if !strings.Contains(members, "| dev |") {
		t.Errorf("member with no web_url lost its username:\n%s", members)
	}

	memberships := FormatBillableMembershipsMarkdown(BillableMembershipsOutput{
		Memberships: []BillableMembershipOutput{{ID: 1, SourceID: 2, SourceFullName: "group/project"}},
	})
	if strings.Contains(memberships, "[group/project](") {
		t.Errorf("membership with no source URL was linked:\n%s", memberships)
	}
	if !strings.Contains(memberships, "| group/project |  |  |") {
		t.Errorf("membership with no access level or expiry did not render empty cells:\n%s", memberships)
	}
}

// ----------------------------------------------
// Markdown formatters
// ----------------------------------------------.
