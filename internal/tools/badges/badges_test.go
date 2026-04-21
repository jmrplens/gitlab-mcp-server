// badges_test.go contains unit tests for the badge MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package badges

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpNonNilResult = "expected non-nil result"

const errNoReachAPI = "should not reach API"

const fmtUnexpErr = "unexpected error: %v"

const badgeJSON = `{"id":1,"name":"coverage","link_url":"https://example.com","image_url":"https://img.shields.io/badge/coverage-90%25-green","rendered_link_url":"https://example.com","rendered_image_url":"https://img.shields.io/badge/coverage-90%25-green","kind":"project"}`

const pathBadges = "/badges"

const pathBadge1 = "/badges/1"

const fmtExpBadgeID1 = "expected badge ID 1, got %d"

const testBadgeIDField = "badge_id"

const fmtExpErrBadgeID = "expected error containing 'badge_id', got %v"

const testBadgeName = "coverage"

const testLinkURL = "https://example.com"

// Project Badges.

// TestListProject_Success verifies the behavior of list project success.
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

// TestListProject_Error verifies the behavior of list project error.
func TestListProject_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListProject(t.Context(), client, ListProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetProject_Success verifies the behavior of get project success.
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

// TestAddProject_Success verifies the behavior of add project success.
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

// TestEditProject_Success verifies the behavior of edit project success.
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

// TestDeleteProject_Success verifies the behavior of delete project success.
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

// TestPreviewProject_Success verifies the behavior of preview project success.
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

// TestListGroup_Success verifies the behavior of list group success.
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

// TestGetGroup_Success verifies the behavior of get group success.
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

// TestAddGroup_Success verifies the behavior of add group success.
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

// TestEditGroup_Success verifies the behavior of edit group success.
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

// TestDeleteGroup_Success verifies the behavior of delete group success.
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

// TestPreviewGroup_Success verifies the behavior of preview group success.
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

// TestGetProject_BadgeIDRequired verifies the behavior of get project badge i d required.
func TestGetProject_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestEditProject_BadgeIDRequired verifies the behavior of edit project badge i d required.
func TestEditProject_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := EditProject(t.Context(), client, EditProjectInput{ProjectID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestDeleteProject_BadgeIDRequired verifies the behavior of delete project badge i d required.
func TestDeleteProject_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	err := DeleteProject(t.Context(), client, DeleteProjectInput{ProjectID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestGetGroup_BadgeIDRequired verifies the behavior of get group badge i d required.
func TestGetGroup_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestEditGroup_BadgeIDRequired verifies the behavior of edit group badge i d required.
func TestEditGroup_BadgeIDRequired(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := EditGroup(t.Context(), client, EditGroupInput{GroupID: "1", BadgeID: 0})
	if err == nil || !strings.Contains(err.Error(), testBadgeIDField) {
		t.Fatalf(fmtExpErrBadgeID, err)
	}
}

// TestDeleteGroup_BadgeIDRequired verifies the behavior of delete group badge i d required.
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

// TestFormatBadgeListMarkdown_Empty verifies the behavior of format badge list markdown empty.
func TestFormatBadgeListMarkdown_Empty(t *testing.T) {
	result := FormatBadgeListMarkdown(nil, "Badges", toolutil.PaginationOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatBadgeListMarkdown_WithData verifies the behavior of format badge list markdown with data.
func TestFormatBadgeListMarkdown_WithData(t *testing.T) {
	result := FormatBadgeListMarkdown([]BadgeItem{
		{ID: 1, Name: testBadgeName, LinkURL: testLinkURL, ImageURL: "https://img.shields.io", Kind: "project"},
	}, "Project Badges", toolutil.PaginationOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatBadgeMarkdown verifies the behavior of format badge markdown.
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

const errExpectedNil = "expected error, got nil"

// ---------------------------------------------------------------------------
// Project Badges — API errors (400), validation
// ---------------------------------------------------------------------------.

// TestGetProject_APIError400 verifies the behavior of get project a p i error400.
func TestGetProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetProject(t.Context(), client, GetProjectInput{ProjectID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestAddProject_APIError400 verifies the behavior of add project a p i error400.
func TestAddProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := AddProject(t.Context(), client, AddProjectInput{ProjectID: "1", LinkURL: "u", ImageURL: "i"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestEditProject_APIError400 verifies the behavior of edit project a p i error400.
func TestEditProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := EditProject(t.Context(), client, EditProjectInput{ProjectID: "1", BadgeID: 1, LinkURL: "u"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDeleteProject_APIError400 verifies the behavior of delete project a p i error400.
func TestDeleteProject_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteProject(t.Context(), client, DeleteProjectInput{ProjectID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestPreviewProject_APIError400 verifies the behavior of preview project a p i error400.
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

// TestListProject_WithNameFilter verifies the behavior of list project with name filter.
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

// TestAddProject_WithoutName verifies the behavior of add project without name.
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

// TestEditProject_AllOptionalFields verifies the behavior of edit project all optional fields.
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

// TestListGroup_APIError400 verifies the behavior of list group a p i error400.
func TestListGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListGroup(t.Context(), client, ListGroupInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGetGroup_APIError400 verifies the behavior of get group a p i error400.
func TestGetGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetGroup(t.Context(), client, GetGroupInput{GroupID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestAddGroup_APIError400 verifies the behavior of add group a p i error400.
func TestAddGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := AddGroup(t.Context(), client, AddGroupInput{GroupID: "1", LinkURL: "u", ImageURL: "i"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestEditGroup_APIError400 verifies the behavior of edit group a p i error400.
func TestEditGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := EditGroup(t.Context(), client, EditGroupInput{GroupID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDeleteGroup_APIError400 verifies the behavior of delete group a p i error400.
func TestDeleteGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteGroup(t.Context(), client, DeleteGroupInput{GroupID: "1", BadgeID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestPreviewGroup_APIError400 verifies the behavior of preview group a p i error400.
func TestPreviewGroup_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := PreviewGroup(t.Context(), client, PreviewGroupInput{GroupID: "1", LinkURL: "u", ImageURL: "i"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestListGroup_WithNameFilter verifies the behavior of list group with name filter.
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

// TestAddGroup_WithName verifies the behavior of add group with name.
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

// TestEditGroup_AllOptionalFields verifies the behavior of edit group all optional fields.
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

// TestPreviewGroup_WithName verifies the behavior of preview group with name.
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

// TestFormatBadgeMarkdown_MinimalFields verifies the behavior of format badge markdown minimal fields.
func TestFormatBadgeMarkdown_MinimalFields(t *testing.T) {
	result := FormatBadgeMarkdown(BadgeItem{ID: 1, Name: "test", LinkURL: "u", ImageURL: "i"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "Rendered") {
		t.Error("should not contain Rendered for empty rendered URLs")
	}
	if strings.Contains(text, "Kind") {
		t.Error("should not contain Kind for empty kind")
	}
}

// TestFormatBadgeListMarkdown_Pagination verifies the behavior of format badge list markdown pagination.
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
// MCP round-trip — all tools
// ---------------------------------------------------------------------------.

// TestMCPRoundTrip_AllTools validates m c p round trip all tools across multiple scenarios using table-driven subtests.
func TestMCPRoundTrip_AllTools(t *testing.T) {
	session := newBadgesMCPSession(t)
	ctx := context.Background()

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

// TestMCPRoundTrip_NotFound validates 404 NotFound paths in register.go
// for get_project_badge and get_group_badge handlers.
func TestMCPRoundTrip_NotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	session := newBadgesMCPSessionWithHandler(t, handler, nil)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_project_badge", map[string]any{"project_id": "1", "badge_id": float64(999)}},
		{"gitlab_get_group_badge", map[string]any{"group_id": "1", "badge_id": float64(999)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if !result.IsError {
				t.Fatalf("expected IsError=true for 404 on %s", tt.name)
			}
		})
	}
}

// TestMCPRoundTrip_ConfirmDeclined covers the ConfirmAction early-return
// branches in delete_project_badge and delete_group_badge when user declines.
func TestMCPRoundTrip_ConfirmDeclined(t *testing.T) {
	handler := http.NewServeMux()
	session := newBadgesMCPSessionWithHandler(t, handler, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_delete_project_badge", map[string]any{"project_id": "1", "badge_id": float64(1)}},
		{"gitlab_delete_group_badge", map[string]any{"group_id": "1", "badge_id": float64(1)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatal(errExpNonNilResult)
			}
		})
	}
}

// TestMCPRoundTrip_DeleteErrors covers the error paths in delete handlers
// after ConfirmAction succeeds.
func TestMCPRoundTrip_DeleteErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	session := newBadgesMCPSessionWithHandler(t, handler, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_delete_project_badge", map[string]any{"project_id": "1", "badge_id": float64(1)}},
		{"gitlab_delete_group_badge", map[string]any{"group_id": "1", "badge_id": float64(1)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("expected error result for %s with 500 backend", tt.name)
			}
		})
	}
}

// newBadgesMCPSessionWithHandler creates an MCP session with a custom HTTP handler and client options.
func newBadgesMCPSessionWithHandler(t *testing.T, handler http.Handler, clientOpts *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, clientOpts)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// newBadgesMCPSession is an internal helper for the badges package.
func newBadgesMCPSession(t *testing.T) *mcp.ClientSession {
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
