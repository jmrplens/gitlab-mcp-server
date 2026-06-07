// badges_test.go contains unit tests for the badge MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package badges

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpNonNilResult identifies the err exp non nil result constant used by this package.
const errExpNonNilResult = "expected non-nil result"

// errNoReachAPI identifies the err no reach API constant used by this package.
const errNoReachAPI = "should not reach API"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// badgeJSON identifies the badge JSON constant used by this package.
const badgeJSON = `{"id":1,"name":"coverage","link_url":"https://example.com","image_url":"https://img.shields.io/badge/coverage-90%25-green","rendered_link_url":"https://example.com","rendered_image_url":"https://img.shields.io/badge/coverage-90%25-green","kind":"project"}`

// pathBadges identifies the path badges constant used by this package.
const pathBadges = "/badges"

// pathBadge1 identifies the path badge 1 constant used by this package.
const pathBadge1 = "/badges/1"

// fmtExpBadgeID1 identifies the fmt exp badge ID 1 constant used by this package.
const fmtExpBadgeID1 = "expected badge ID 1, got %d"

// testBadgeIDField identifies the test badge ID field constant used by this package.
const testBadgeIDField = "badge_id"

// fmtExpErrBadgeID identifies the fmt exp err badge ID constant used by this package.
const fmtExpErrBadgeID = "expected error containing 'badge_id', got %v"

// testBadgeName identifies the test badge name constant used by this package.
const testBadgeName = "coverage"

// testLinkURL identifies the test link URL constant used by this package.
const testLinkURL = "https://example.com"

func badgeMarkdownText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("expected non-nil markdown result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return text.Text
}

// Project Badges.

// TestListProject_Success verifies ListProject when success.
func TestListProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadges) && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[`+badgeJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(t.Context(), client, ListProjectInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Badges) != 1 {
		t.Fatalf("expected 1 badge, got %d", len(out.Badges))
	}
	if out.Badges[0].Name != testBadgeName {
		t.Errorf("expected name 'coverage', got %q", out.Badges[0].Name)
	}
}

// TestListProject_Error verifies ListProject when error.
func TestListProject_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListProject(t.Context(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetProject_Success verifies GetProject when success.
func TestGetProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadge1) && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1", BadgeID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf(fmtExpBadgeID1, out.Badge.ID)
	}
}

// TestAddProject_Success verifies AddProject when success.
func TestAddProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadges) && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddProject(t.Context(), client, AddProjectInput{
		ProjectID: "1",
		LinkURL:   testLinkURL,
		ImageURL:  "https://img.shields.io/badge/test-pass-green",
		Name:      testBadgeName,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.Name != testBadgeName {
		t.Errorf("expected name 'coverage', got %q", out.Badge.Name)
	}
}

// TestEditProject_Success verifies EditProject when success.
func TestEditProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadge1) && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := EditProject(t.Context(), client, EditProjectInput{
		ProjectID: "1",
		BadgeID:   1,
		Name:      "coverage-updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf(fmtExpBadgeID1, out.Badge.ID)
	}
}

// TestDeleteProject_Success verifies DeleteProject when success.
func TestDeleteProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadge1) && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteProject(t.Context(), client, DeleteProjectInput{ProjectID: "1", BadgeID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPreviewProject_Success verifies PreviewProject when success.
func TestPreviewProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/badges/render") && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PreviewProject(t.Context(), client, PreviewProjectInput{
		ProjectID: "1",
		LinkURL:   "https://example.com/%{project_path}",
		ImageURL:  "https://img.shields.io/badge/%{default_branch}-green",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.RenderedLinkURL == "" {
		t.Error("expected rendered link URL")
	}
}

// Group Badges.

// TestListGroup_Success verifies ListGroup when success.
func TestListGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadges) && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[`+badgeJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListGroup(t.Context(), client, ListGroupInput{GroupID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Badges) != 1 {
		t.Fatalf("expected 1 badge, got %d", len(out.Badges))
	}
}

// TestGetGroup_Success verifies GetGroup when success.
func TestGetGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadge1) && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "1", BadgeID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf(fmtExpBadgeID1, out.Badge.ID)
	}
}

// TestAddGroup_Success verifies AddGroup when success.
func TestAddGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadges) && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := AddGroup(t.Context(), client, AddGroupInput{
		GroupID:  "1",
		LinkURL:  testLinkURL,
		ImageURL: "https://img.shields.io/badge/test-pass-green",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf(fmtExpBadgeID1, out.Badge.ID)
	}
}

// TestEditGroup_Success verifies EditGroup when success.
func TestEditGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadge1) && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := EditGroup(t.Context(), client, EditGroupInput{
		GroupID: "1",
		BadgeID: 1,
		Name:    "updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf(fmtExpBadgeID1, out.Badge.ID)
	}
}

// TestDeleteGroup_Success verifies DeleteGroup when success.
func TestDeleteGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, pathBadge1) && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteGroup(t.Context(), client, DeleteGroupInput{GroupID: "1", BadgeID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPreviewGroup_Success verifies PreviewGroup when success.
func TestPreviewGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/badges/render") && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := PreviewGroup(t.Context(), client, PreviewGroupInput{
		GroupID:  "1",
		LinkURL:  "https://example.com/%{project_path}",
		ImageURL: "https://img.shields.io/badge/%{default_branch}-green",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.RenderedLinkURL == "" {
		t.Error("expected rendered link URL")
	}
}

// Validation Tests.

// TestGetProject_BadgeIDRequired verifies GetProject when badge ID required.
func TestGetProject_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestEditProject_BadgeIDRequired verifies EditProject when badge ID required.
func TestEditProject_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := EditProject(t.Context(), client, EditProjectInput{ProjectID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestDeleteProject_BadgeIDRequired verifies DeleteProject when badge ID required.
func TestDeleteProject_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	err := DeleteProject(t.Context(), client, DeleteProjectInput{ProjectID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestGetGroup_BadgeIDRequired verifies GetGroup when badge ID required.
func TestGetGroup_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestEditGroup_BadgeIDRequired verifies EditGroup when badge ID required.
func TestEditGroup_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := EditGroup(t.Context(), client, EditGroupInput{GroupID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestDeleteGroup_BadgeIDRequired verifies DeleteGroup when badge ID required.
func TestDeleteGroup_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	err := DeleteGroup(t.Context(), client, DeleteGroupInput{GroupID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// Formatters.

// TestFormatBadgeListMarkdown_Empty verifies FormatBadgeListMarkdown when empty.
func TestFormatBadgeListMarkdown_Empty(t *testing.T) {
	result := FormatBadgeListMarkdown(nil, "Badges", toolutil.PaginationOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatBadgeListMarkdown_WithData verifies FormatBadgeListMarkdown when with data.
func TestFormatBadgeListMarkdown_WithData(t *testing.T) {
	result := FormatBadgeListMarkdown([]BadgeItem{
		{ID: 1, Name: testBadgeName, LinkURL: testLinkURL, ImageURL: "https://img.shields.io", Kind: "project"},
	}, "Project Badges", toolutil.PaginationOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatBadgeMarkdown verifies FormatBadgeMarkdown.
func TestFormatBadgeMarkdown(t *testing.T) {
	result := FormatBadgeMarkdown(BadgeItem{
		ID: 1, Name: testBadgeName, LinkURL: testLinkURL, ImageURL: "https://img.shields.io",
		RenderedLinkURL: "https://rendered.com", RenderedImageURL: "https://rendered-img.com", Kind: "project",
	})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// ---------------------------------------------------------------------------
// Project Badges — API errors (400), validation
// ---------------------------------------------------------------------------.

// TestGetProject_APIError400 verifies GetProject when API error 400.
func TestGetProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestAddProject_APIError400 verifies AddProject when API error 400.
func TestAddProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := AddProject(t.Context(), client, AddProjectInput{ProjectID: "1", LinkURL: "u", ImageURL: "i"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestAddProject_APIErrorBranches verifies AddProject returns actionable
// errors for permission failures and preserves fallback API error details.
func TestAddProject_APIErrorBranches(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{
			name:       "forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"message":"403 Forbidden"}`,
			want:       "Maintainer+",
		},
		{
			name:       "unprocessable fallback",
			statusCode: http.StatusUnprocessableEntity,
			body:       `{"message":"link_url has already been taken"}`,
			want:       "link_url has already been taken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.statusCode, tt.body)
			}))
			_, err := AddProject(t.Context(), client, AddProjectInput{ProjectID: "1", LinkURL: "u", ImageURL: "i"})
			if err == nil {
				t.Fatal(errExpectedNil)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error missing %q: %v", tt.want, err)
			}
		})
	}
}

// TestEditProject_APIError400 verifies EditProject when API error 400.
func TestEditProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := EditProject(t.Context(), client, EditProjectInput{ProjectID: "1", BadgeID: 1, LinkURL: "u"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestEditProject_APIError403 verifies EditProject includes the project badge
// permission hint when GitLab rejects the update.
func TestEditProject_APIError403(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := EditProject(t.Context(), client, EditProjectInput{ProjectID: "1", BadgeID: 1, LinkURL: "u"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "Maintainer+ role") {
		t.Fatalf("error missing project badge permission hint: %v", err)
	}
}

// TestDeleteProject_APIError400 verifies DeleteProject when API error 400.
func TestDeleteProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteProject(t.Context(), client, DeleteProjectInput{ProjectID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestPreviewProject_APIError400 verifies PreviewProject when API error 400.
func TestPreviewProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := PreviewProject(t.Context(), client, PreviewProjectInput{ProjectID: "1", LinkURL: "u", ImageURL: "i"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Project Badges — optional fields
// ---------------------------------------------------------------------------.

// TestListProject_WithNameFilter verifies ListProject when with name filter.
func TestListProject_WithNameFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "coverage" {
			testutil.RespondJSON(w, http.StatusOK, `[`+badgeJSON+`]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListProject(t.Context(), client, ListProjectInput{ProjectID: "1", Name: "coverage"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Badges) != 1 {
		t.Fatalf("expected 1, got %d", len(out.Badges))
	}
}

// TestAddProject_WithoutName verifies AddProject when without name.
func TestAddProject_WithoutName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := AddProject(t.Context(), client, AddProjectInput{
		ProjectID: "1", LinkURL: "https://example.com", ImageURL: "https://img.shields.io/badge/t-green",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf("expected badge ID 1, got %d", out.Badge.ID)
	}
}

// TestEditProject_AllOptionalFields verifies EditProject when all optional fields.
func TestEditProject_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := EditProject(t.Context(), client, EditProjectInput{
		ProjectID: "1", BadgeID: 1,
		LinkURL: "https://updated.com", ImageURL: "https://updated-img.com", Name: "updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf("expected badge ID 1, got %d", out.Badge.ID)
	}
}

// ---------------------------------------------------------------------------
// Group Badges — API errors (400), optional fields
// ---------------------------------------------------------------------------.

// TestListGroup_APIError400 verifies ListGroup when API error 400.
func TestListGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListGroup(t.Context(), client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetGroup_APIError400 verifies GetGroup when API error 400.
func TestGetGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestAddGroup_APIError400 verifies AddGroup when API error 400.
func TestAddGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := AddGroup(t.Context(), client, AddGroupInput{GroupID: "1", LinkURL: "u", ImageURL: "i"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestAddGroup_APIErrorBranches verifies AddGroup returns actionable errors
// for permission failures and preserves fallback API error details.
func TestAddGroup_APIErrorBranches(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       string
	}{
		{
			name:       "forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"message":"403 Forbidden"}`,
			want:       "Owner role",
		},
		{
			name:       "unprocessable fallback",
			statusCode: http.StatusUnprocessableEntity,
			body:       `{"message":"group badge name is invalid"}`,
			want:       "group badge name is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.statusCode, tt.body)
			}))
			_, err := AddGroup(t.Context(), client, AddGroupInput{GroupID: "1", LinkURL: "u", ImageURL: "i"})
			if err == nil {
				t.Fatal(errExpectedNil)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error missing %q: %v", tt.want, err)
			}
		})
	}
}

// TestEditGroup_APIError400 verifies EditGroup when API error 400.
func TestEditGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := EditGroup(t.Context(), client, EditGroupInput{GroupID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestEditGroup_APIError403 verifies EditGroup includes the group badge
// permission hint when GitLab rejects the update.
func TestEditGroup_APIError403(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := EditGroup(t.Context(), client, EditGroupInput{GroupID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "Owner role") {
		t.Fatalf("error missing group badge permission hint: %v", err)
	}
}

// TestDeleteGroup_APIError400 verifies DeleteGroup when API error 400.
func TestDeleteGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteGroup(t.Context(), client, DeleteGroupInput{GroupID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestPreviewGroup_APIError400 verifies PreviewGroup when API error 400.
func TestPreviewGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := PreviewGroup(t.Context(), client, PreviewGroupInput{GroupID: "1", LinkURL: "u", ImageURL: "i"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestListGroup_WithNameFilter verifies ListGroup when with name filter.
func TestListGroup_WithNameFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "build" {
			testutil.RespondJSON(w, http.StatusOK, `[`+badgeJSON+`]`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListGroup(t.Context(), client, ListGroupInput{GroupID: "1", Name: "build"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Badges) != 1 {
		t.Fatalf("expected 1, got %d", len(out.Badges))
	}
}

// TestAddGroup_WithName verifies AddGroup when with name.
func TestAddGroup_WithName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := AddGroup(t.Context(), client, AddGroupInput{
		GroupID: "1", LinkURL: "https://example.com", ImageURL: "https://img.shields.io/badge/t-green", Name: "coverage",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf("expected badge ID 1, got %d", out.Badge.ID)
	}
}

// TestEditGroup_AllOptionalFields verifies EditGroup when all optional fields.
func TestEditGroup_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := EditGroup(t.Context(), client, EditGroupInput{
		GroupID: "1", BadgeID: 1,
		LinkURL: "https://up.com", ImageURL: "https://up-img.com", Name: "updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf("expected badge ID 1, got %d", out.Badge.ID)
	}
}

// TestPreviewGroup_WithName verifies PreviewGroup when with name.
func TestPreviewGroup_WithName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/badges/render") {
			testutil.RespondJSON(w, http.StatusOK, badgeJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := PreviewGroup(t.Context(), client, PreviewGroupInput{
		GroupID: "1", LinkURL: "https://example.com", ImageURL: "https://img.shields.io", Name: "preview",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Badge.ID != 1 {
		t.Errorf("expected badge ID 1, got %d", out.Badge.ID)
	}
}

// ---------------------------------------------------------------------------
// Formatters — edge cases
// ---------------------------------------------------------------------------.

// TestFormatBadgeMarkdown_MinimalFields verifies FormatBadgeMarkdown when minimal fields.
func TestFormatBadgeMarkdown_MinimalFields(t *testing.T) {
	result := FormatBadgeMarkdown(BadgeItem{ID: 1, Name: "test", LinkURL: "u", ImageURL: "i"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := fmt.Sprint(result.Content[0])
	if strings.Contains(text, "Rendered") {
		t.Error("should not contain Rendered for empty rendered URLs")
	}
	if strings.Contains(text, "Kind") {
		t.Error("should not contain Kind for empty kind")
	}
}

// TestFormatBadgeListMarkdown_Pagination verifies FormatBadgeListMarkdown when pagination.
func TestFormatBadgeListMarkdown_Pagination(t *testing.T) {
	result := FormatBadgeListMarkdown(
		[]BadgeItem{{ID: 1, Name: "b", LinkURL: "l", ImageURL: "i", Kind: "project"}},
		"Test Badges",
		toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestMarkdownRegistry_BadgeOutputTypes verifies every badge output type is
// registered with the Markdown registry and routes to the expected formatter.
func TestMarkdownRegistry_BadgeOutputTypes(t *testing.T) {
	badge := BadgeItem{ID: 1, Name: testBadgeName, LinkURL: testLinkURL, ImageURL: "https://img.shields.io", Kind: "project"}
	tests := []struct {
		name       string
		output     any
		want       string
		wantAbsent string
	}{
		{
			name:   "list project output",
			output: ListProjectOutput{Badges: []BadgeItem{badge}},
			want:   "## Project Badges (1)",
		},
		{
			name:   "get project output",
			output: GetProjectOutput{Badge: badge},
			want:   "## Badge: coverage (ID: 1)",
		},
		{
			name:   "add project output",
			output: AddProjectOutput{Badge: badge},
			want:   "## Badge: coverage (ID: 1)",
		},
		{
			name:   "edit project output",
			output: EditProjectOutput{Badge: badge},
			want:   "## Badge: coverage (ID: 1)",
		},
		{
			name:   "preview project output",
			output: PreviewProjectOutput{Badge: badge},
			want:   "## Badge: coverage (ID: 1)",
		},
		{
			name:   "list group output",
			output: ListGroupOutput{Badges: []BadgeItem{badge}},
			want:   "## Group Badges (1)",
		},
		{
			name:   "get group output",
			output: GetGroupOutput{Badge: badge},
			want:   "## Badge: coverage (ID: 1)",
		},
		{
			name:   "add group output",
			output: AddGroupOutput{Badge: badge},
			want:   "## Badge: coverage (ID: 1)",
		},
		{
			name:   "edit group output",
			output: EditGroupOutput{Badge: badge},
			want:   "## Badge: coverage (ID: 1)",
		},
		{
			name:       "preview group output",
			output:     PreviewGroupOutput{Badge: BadgeItem{ID: 2, Name: "preview", LinkURL: "u", ImageURL: "i"}},
			want:       "## Badge: preview (ID: 2)",
			wantAbsent: "**Kind**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := badgeMarkdownText(t, toolutil.MarkdownForResult(tt.output))
			if !strings.Contains(text, tt.want) {
				t.Fatalf("markdown missing %q:\n%s", tt.want, text)
			}
			if tt.wantAbsent != "" && strings.Contains(text, tt.wantAbsent) {
				t.Fatalf("markdown contains %q:\n%s", tt.wantAbsent, text)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Action specs — all tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for badge actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := allBadgeActionSpecs(client)
	byTool := badgeSpecsByTool(t, specs)

	if len(specs) != 12 {
		t.Fatalf("len(ActionSpecs) = %d, want 12", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "badges" {
			t.Fatalf("OwnerPackage for %s = %q, want badges", spec.Name, spec.OwnerPackage)
		}
	}
	for _, toolName := range []string{"gitlab_delete_project_badge", "gitlab_delete_group_badge"} {
		if !byTool[toolName].Route.Destructive {
			t.Fatalf("%s should be destructive", toolName)
		}
	}
	assertBadgeEditNameGuidance(t, byTool)
	assertBadgeCreateScopeGuidance(t, byTool)
}

func assertBadgeEditNameGuidance(t *testing.T, byTool map[string]toolutil.ActionSpec) {
	t.Helper()
	for _, toolName := range []string{"gitlab_edit_project_badge", "gitlab_edit_group_badge"} {
		guidance := byTool[toolName].ParameterGuidance["name"]
		if guidance.SemanticRole != "badge_display_name" {
			t.Fatalf("%s name SemanticRole = %q, want badge_display_name", toolName, guidance.SemanticRole)
		}
		if !containsText(guidance.CommonConfusions, "new_name") {
			t.Fatalf("%s name CommonConfusions = %v, want new_name warning", toolName, guidance.CommonConfusions)
		}
	}
}

func assertBadgeCreateScopeGuidance(t *testing.T, byTool map[string]toolutil.ActionSpec) {
	t.Helper()
	projectAdd := byTool["gitlab_add_project_badge"]
	if !strings.Contains(projectAdd.Usage, "project badge") || !strings.Contains(projectAdd.Usage, "project_id") || !strings.Contains(projectAdd.Usage, "group_id") || !strings.Contains(projectAdd.Usage, "do not use gitlab_group") {
		t.Fatalf("project badge add Usage = %q, want project_id guidance", projectAdd.Usage)
	}
	if guidance := projectAdd.ParameterGuidance["project_id"]; guidance.SemanticRole != "scope_project" || !containsText(guidance.CommonConfusions, "group_id") {
		t.Fatalf("project badge project_id guidance = %+v, want group_id warning", guidance)
	}
	groupAdd := byTool["gitlab_add_group_badge"]
	if !strings.Contains(groupAdd.Usage, "group badge") || !strings.Contains(groupAdd.Usage, "group_id") || !strings.Contains(groupAdd.Usage, "project_id") || !strings.Contains(groupAdd.Usage, "project badge CRUD belongs to gitlab_project") {
		t.Fatalf("group badge add Usage = %q, want group_id guidance", groupAdd.Usage)
	}
	if guidance := groupAdd.ParameterGuidance["group_id"]; guidance.SemanticRole != "scope_group" || !containsText(guidance.CommonConfusions, "project_id") {
		t.Fatalf("group badge group_id guidance = %+v, want project_id warning", guidance)
	}
}

// TestActionSpecs_CallAllRoutes validates all badge routes through canonical specs.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newBadgeRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_project", "gitlab_list_project_badges", map[string]any{"project_id": "1"}},
		{"get_project", "gitlab_get_project_badge", map[string]any{"project_id": "1", "badge_id": float64(1)}},
		{"add_project", "gitlab_add_project_badge", map[string]any{"project_id": "1", "link_url": "https://example.com", "image_url": "https://img.shields.io/badge/t-green"}},
		{"edit_project", "gitlab_edit_project_badge", map[string]any{"project_id": "1", "badge_id": float64(1), "name": "updated"}},
		{"delete_project", "gitlab_delete_project_badge", map[string]any{"project_id": "1", "badge_id": float64(1)}},
		{"preview_project", "gitlab_preview_project_badge", map[string]any{"project_id": "1", "link_url": "https://example.com", "image_url": "https://img.shields.io"}},
		{"list_group", "gitlab_list_group_badges", map[string]any{"group_id": "1"}},
		{"get_group", "gitlab_get_group_badge", map[string]any{"group_id": "1", "badge_id": float64(1)}},
		{"add_group", "gitlab_add_group_badge", map[string]any{"group_id": "1", "link_url": "https://example.com", "image_url": "https://img.shields.io/badge/t-green"}},
		{"edit_group", "gitlab_edit_group_badge", map[string]any{"group_id": "1", "badge_id": float64(1), "name": "updated"}},
		{"delete_group", "gitlab_delete_group_badge", map[string]any{"group_id": "1", "badge_id": float64(1)}},
		{"preview_group", "gitlab_preview_group_badge", map[string]any{"group_id": "1", "link_url": "https://example.com", "image_url": "https://img.shields.io"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_GetNotFound validates 404 NotFound paths on canonical get routes.
func TestActionSpecs_GetNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := badgeSpecsByTool(t, allBadgeActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_project_badge", map[string]any{"project_id": "1", "badge_id": float64(999)}},
		{"gitlab_get_group_badge", map[string]any{"group_id": "1", "badge_id": float64(999)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if _, ok := result.(badgeNotFoundOutput); !ok {
				t.Fatalf("result type = %T, want badgeNotFoundOutput", result)
			}
			toolResult := toolutil.MarkdownForResult(result)
			if toolResult == nil || !toolResult.IsError {
				t.Fatalf("expected MarkdownForResult to return an error CallToolResult for %s", tt.name)
			}
		})
	}
}

// TestActionSpecs_DeleteErrors covers the error paths in delete routes.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := badgeSpecsByTool(t, allBadgeActionSpecs(client))

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_delete_project_badge", map[string]any{"project_id": "1", "badge_id": float64(1)}},
		{"gitlab_delete_group_badge", map[string]any{"group_id": "1", "badge_id": float64(1)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			_, err := byTool[tt.name].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatalf("Route.Handler(%s) expected error, got nil", tt.name)
			}
		})
	}
}

// newBadgeRouteSpecs constructs badge route specs test fixtures.
func newBadgeRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()

	// Project badges
	handler.HandleFunc("GET /api/v4/projects/1/badges", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+badgeJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/badges/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, badgeJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/1/badges", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, badgeJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/badges/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, badgeJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/badges/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/projects/1/badges/render", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, badgeJSON)
	})

	// Group badges
	handler.HandleFunc("GET /api/v4/groups/1/badges", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+badgeJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/groups/1/badges/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, badgeJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/1/badges", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, badgeJSON)
	})
	handler.HandleFunc("PUT /api/v4/groups/1/badges/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, badgeJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/1/badges/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/groups/1/badges/render", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, badgeJSON)
	})

	return badgeSpecsByTool(t, allBadgeActionSpecs(testutil.NewTestClient(t, handler)))
}

// allBadgeActionSpecs supports all badge action specs assertions in badges tests.
func allBadgeActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return append(ProjectActionSpecs(client), GroupActionSpecs(client)...)
}

// badgeSpecsByTool supports badge specs by tool assertions in badges tests.
func badgeSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// badgeActionDescription — table-driven coverage
// ---------------------------------------------------------------------------

// TestBadgeActionDescription_AllBranches verifies badgeActionDescription across
// all known verbs and the default (fallback) branch, ensuring every switch
// case returns the expected human-readable description.
func TestBadgeActionDescription_AllBranches(t *testing.T) {
	tests := []struct {
		name     string
		verb     string
		scope    string
		contains string
	}{
		{"add verb", "add", "project", "Add a project badge."},
		{"get verb", "get", "project", "Get a project badge."},
		{"edit verb", "edit", "project", "Edit a project badge."},
		{"delete verb", "delete", "group", "Delete a group badge."},
		{"list verb", "list", "group", "List group badges."},
		{"preview verb", "preview", "project", "Preview a project badge."},
		{"unknown verb falls back", "rotate", "project", "Manage project badges."},
		{"empty verb falls back", "", "group", "Manage group badges."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := badgeActionDescription(tt.verb, tt.scope)
			if got != tt.contains {
				t.Errorf("badgeActionDescription(%q, %q) = %q, want %q", tt.verb, tt.scope, got, tt.contains)
			}
		})
	}
}
