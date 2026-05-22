// issuelinks_test.go contains unit tests for the issue link MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package issuelinks

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// errExpMissingProjectID identifies the err exp missing project ID constant used by this package.
const errExpMissingProjectID = "expected error for missing project_id"

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testPathIssueLinks identifies the test path issue links constant used by this package.
const testPathIssueLinks = "/api/v4/projects/10/issues/5/links"

// errExpMissingIssueIID identifies the err exp missing issue IID constant used by this package.
const errExpMissingIssueIID = "expected error for missing issue_iid"

// testProjectID identifies the test project ID constant used by this package.
const testProjectID = "10"

// fmtLinkTypeWant identifies the fmt link type want constant used by this package.
const fmtLinkTypeWant = "LinkType = %q, want %q"

// testLinkRelatesTo identifies the test link relates to constant used by this package.
const testLinkRelatesTo = "relates_to"

// ---------------------------------------------------------------------------
// Issue Link List
// ---------------------------------------------------------------------------.

// TestIssueLinkList_Success verifies IssueLinkList when success.
func TestIssueLinkList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathIssueLinks && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"id":100,"iid":8,"title":"Related issue","state":"opened","project_id":10,"issue_link_id":1,"link_type":"relates_to","web_url":"https://gitlab.example.com/group/project/-/issues/8"}
			]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		IssueIID:  5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Relations) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(out.Relations))
	}
	r := out.Relations[0]
	if r.ID != 100 {
		t.Errorf("ID = %d, want 100", r.ID)
	}
	if r.IID != 8 {
		t.Errorf("IID = %d, want 8", r.IID)
	}
	if r.Title != "Related issue" {
		t.Errorf("Title = %q, want %q", r.Title, "Related issue")
	}
	if r.LinkType != testLinkRelatesTo {
		t.Errorf(fmtLinkTypeWant, r.LinkType, testLinkRelatesTo)
	}
	if r.IssueLinkID != 1 {
		t.Errorf("IssueLinkID = %d, want 1", r.IssueLinkID)
	}
}

// TestIssueLinkList_Empty verifies IssueLinkList when empty.
func TestIssueLinkList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathIssueLinks && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: testProjectID,
		IssueIID:  5,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Relations) != 0 {
		t.Fatalf("expected 0 relations, got %d", len(out.Relations))
	}
}

// TestIssueLinkList_MissingProjectID verifies IssueLinkList when missing project ID.
func TestIssueLinkList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(context.Background(), client, ListInput{IssueIID: 5})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestIssueLinkList_MissingIssueIID verifies IssueLinkList when missing issue IID.
func TestIssueLinkList_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal(errExpMissingIssueIID)
	}
}

// TestIssueLinkList_CancelledContext verifies IssueLinkList when cancelled context.
func TestIssueLinkList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID, IssueIID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Issue Link Get
// ---------------------------------------------------------------------------.

// TestIssueLinkGet_Success verifies IssueLinkGet when success.
func TestIssueLinkGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/issues/5/links/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,
				"source_issue":{"id":50,"iid":5,"project_id":10,"title":"Source","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
				"target_issue":{"id":80,"iid":8,"project_id":10,"title":"Target","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
				"link_type":"blocks"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:   testProjectID,
		IssueIID:    5,
		IssueLinkID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.SourceIssueIID != 5 {
		t.Errorf("SourceIssueIID = %d, want 5", out.SourceIssueIID)
	}
	if out.TargetIssueIID != 8 {
		t.Errorf("TargetIssueIID = %d, want 8", out.TargetIssueIID)
	}
	if out.LinkType != "blocks" {
		t.Errorf(fmtLinkTypeWant, out.LinkType, "blocks")
	}
}

// TestIssueLinkGet_MissingProjectID verifies IssueLinkGet when missing project ID.
func TestIssueLinkGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Get(context.Background(), client, GetInput{IssueIID: 5, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestIssueLinkGet_MissingIssueIID verifies IssueLinkGet when missing issue IID.
func TestIssueLinkGet_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpMissingIssueIID)
	}
}

// TestIssueLinkGet_MissingLinkID verifies IssueLinkGet when missing link ID.
func TestIssueLinkGet_MissingLinkID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 5})
	if err == nil {
		t.Fatal("expected error for missing issue_link_id")
	}
}

// TestIssueLinkGet_CancelledContext verifies IssueLinkGet when cancelled context.
func TestIssueLinkGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: testProjectID, IssueIID: 5, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Issue Link Create
// ---------------------------------------------------------------------------.

// TestIssueLinkCreate_Success verifies IssueLinkCreate when success.
func TestIssueLinkCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathIssueLinks && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":2,
				"source_issue":{"id":50,"iid":5,"project_id":10,"title":"Source","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
				"target_issue":{"id":120,"iid":12,"project_id":20,"title":"Target","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
				"link_type":"is_blocked_by"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:       testProjectID,
		IssueIID:        5,
		TargetProjectID: "20",
		TargetIssueIID:  "12",
		LinkType:        "is_blocked_by",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
	if out.TargetIssueIID != 12 {
		t.Errorf("TargetIssueIID = %d, want 12", out.TargetIssueIID)
	}
	if out.TargetProjectID != 20 {
		t.Errorf("TargetProjectID = %d, want 20", out.TargetProjectID)
	}
	if out.LinkType != "is_blocked_by" {
		t.Errorf(fmtLinkTypeWant, out.LinkType, "is_blocked_by")
	}
}

// TestIssueLinkCreate_WithoutLinkType verifies IssueLinkCreate when without link type.
func TestIssueLinkCreate_WithoutLinkType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathIssueLinks && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":3,
				"source_issue":{"id":50,"iid":5,"project_id":10,"title":"Source","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
				"target_issue":{"id":70,"iid":7,"project_id":10,"title":"Target","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
				"link_type":"relates_to"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:       testProjectID,
		IssueIID:        5,
		TargetProjectID: testProjectID,
		TargetIssueIID:  "7",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.LinkType != testLinkRelatesTo {
		t.Errorf(fmtLinkTypeWant, out.LinkType, testLinkRelatesTo)
	}
}

// TestIssueLinkCreate_MissingProjectID verifies IssueLinkCreate when missing project ID.
func TestIssueLinkCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Create(context.Background(), client, CreateInput{IssueIID: 5, TargetProjectID: "20", TargetIssueIID: "12"})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestIssueLinkCreate_MissingIssueIID verifies IssueLinkCreate when missing issue IID.
func TestIssueLinkCreate_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, TargetProjectID: "20", TargetIssueIID: "12"})
	if err == nil {
		t.Fatal(errExpMissingIssueIID)
	}
}

// TestIssueLinkCreate_MissingTargetProject verifies IssueLinkCreate when missing target project.
func TestIssueLinkCreate_MissingTargetProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, IssueIID: 5, TargetIssueIID: "12"})
	if err == nil {
		t.Fatal("expected error for missing target_project_id")
	}
}

// TestIssueLinkCreate_MissingTargetIssue verifies IssueLinkCreate when missing target issue.
func TestIssueLinkCreate_MissingTargetIssue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, IssueIID: 5, TargetProjectID: "20"})
	if err == nil {
		t.Fatal("expected error for missing target_issue_iid")
	}
}

// TestIssueLinkCreate_CancelledContext verifies IssueLinkCreate when cancelled context.
func TestIssueLinkCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: testProjectID, IssueIID: 5, TargetProjectID: "20", TargetIssueIID: "12"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Issue Link Delete
// ---------------------------------------------------------------------------.

// TestIssueLinkDelete_Success verifies IssueLinkDelete when success.
func TestIssueLinkDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/issues/5/links/1" && r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,
				"source_issue":{"id":50,"iid":5,"project_id":10,"title":"Source","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
				"target_issue":{"id":80,"iid":8,"project_id":10,"title":"Target","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
				"link_type":"relates_to"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:   testProjectID,
		IssueIID:    5,
		IssueLinkID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestIssueLinkDelete_MissingProjectID verifies IssueLinkDelete when missing project ID.
func TestIssueLinkDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(context.Background(), client, DeleteInput{IssueIID: 5, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestIssueLinkDelete_MissingIssueIID verifies IssueLinkDelete when missing issue IID.
func TestIssueLinkDelete_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpMissingIssueIID)
	}
}

// TestIssueLinkDelete_MissingLinkID verifies IssueLinkDelete when missing link ID.
func TestIssueLinkDelete_MissingLinkID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, IssueIID: 5})
	if err == nil {
		t.Fatal("expected error for missing issue_link_id")
	}
}

// TestIssueLinkDelete_CancelledContext verifies IssueLinkDelete when cancelled context.
func TestIssueLinkDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: testProjectID, IssueIID: 5, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// assertContains verifies that err is non-nil and its message contains substr.
// ---------------------------------------------------------------------------.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestIssueIIDNegative_Validation ensures negative issue_iid is rejected by all handlers.
func TestIssueIIDNegative_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when issue_iid is negative")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, IssueIID: -1}); return e }},
		{"Get", func() error {
			_, e := Get(ctx, client, GetInput{ProjectID: pid, IssueIID: -5, IssueLinkID: 1})
			return e
		}},
		{"Create", func() error {
			_, e := Create(ctx, client, CreateInput{ProjectID: pid, IssueIID: -3, TargetProjectID: "other", TargetIssueIID: "10"})
			return e
		}},
		{"Delete", func() error {
			return Delete(ctx, client, DeleteInput{ProjectID: pid, IssueIID: -2, IssueLinkID: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "issue_iid")
		})
	}
}

// TestIssueLinkIDNegative_Validation ensures negative issue_link_id is rejected.
func TestIssueLinkIDNegative_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when issue_link_id is negative")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Get", func() error {
			_, e := Get(ctx, client, GetInput{ProjectID: pid, IssueIID: 10, IssueLinkID: -1})
			return e
		}},
		{"Delete", func() error {
			return Delete(ctx, client, DeleteInput{ProjectID: pid, IssueIID: 10, IssueLinkID: -5})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "issue_link_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_Populated covers FormatOutputMarkdown with table-driven subtests for populated.
func TestFormatOutputMarkdown_Populated(t *testing.T) {
	out := Output{
		ID:              42,
		SourceIssueIID:  5,
		SourceProjectID: 10,
		TargetIssueIID:  8,
		TargetProjectID: 20,
		LinkType:        "blocks",
	}
	md := FormatOutputMarkdown(out)

	checks := []struct {
		label, want string
	}{
		{"header", "## Issue Link"},
		{"id", "**ID**: 42"},
		{"link type", "**Link Type**: blocks"},
		{"source", "**Source Issue IID**: 5 (project 10)"},
		{"target", "**Target Issue IID**: 8 (project 20)"},
	}
	for _, c := range checks {
		if !strings.Contains(md, c.want) {
			t.Errorf("%s: missing %q in:\n%s", c.label, c.want, md)
		}
	}
}

// TestFormatOutputMarkdown_Empty verifies FormatOutputMarkdown when empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for zero-ID output, got %q", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Populated covers FormatListMarkdown with table-driven subtests for populated.
func TestFormatListMarkdown_Populated(t *testing.T) {
	out := ListOutput{
		Relations: []RelationOutput{
			{ID: 100, IID: 8, Title: "Related issue", State: "opened", LinkType: "relates_to", IssueLinkID: 1},
			{ID: 200, IID: 9, Title: "Blocking issue", State: "closed", LinkType: "blocks", IssueLinkID: 2},
		},
	}
	md := FormatListMarkdown(out)

	checks := []struct {
		label, want string
	}{
		{"header", "## Issue Relations (2)"},
		{"table header", "| ID | IID | Title | State | Link Type | Link ID |"},
		{"row1 id", "| 100 |"},
		{"row1 title", "Related issue"},
		{"row2 id", "| 200 |"},
		{"row2 link type", "blocks"},
	}
	for _, c := range checks {
		if !strings.Contains(md, c.want) {
			t.Errorf("%s: missing %q in:\n%s", c.label, c.want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No linked issues found") {
		t.Errorf("expected empty-state message, got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// toOutput converter
// ---------------------------------------------------------------------------.

// TestToOutput_FullFields verifies ToOutput when full fields.
func TestToOutput_FullFields(t *testing.T) {
	link := &gl.IssueLink{
		ID:       42,
		LinkType: "blocks",
		SourceIssue: &gl.Issue{
			IID:       5,
			ProjectID: 10,
		},
		TargetIssue: &gl.Issue{
			IID:       8,
			ProjectID: 20,
		},
	}
	out := toOutput(link)

	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.LinkType != "blocks" {
		t.Errorf("LinkType = %q, want %q", out.LinkType, "blocks")
	}
	if out.SourceIssueIID != 5 {
		t.Errorf("SourceIssueIID = %d, want 5", out.SourceIssueIID)
	}
	if out.SourceProjectID != 10 {
		t.Errorf("SourceProjectID = %d, want 10", out.SourceProjectID)
	}
	if out.TargetIssueIID != 8 {
		t.Errorf("TargetIssueIID = %d, want 8", out.TargetIssueIID)
	}
	if out.TargetProjectID != 20 {
		t.Errorf("TargetProjectID = %d, want 20", out.TargetProjectID)
	}
}

// TestToOutput_NilSourceIssue verifies ToOutput when nil source issue.
func TestToOutput_NilSourceIssue(t *testing.T) {
	link := &gl.IssueLink{
		ID:       1,
		LinkType: "relates_to",
		TargetIssue: &gl.Issue{
			IID:       8,
			ProjectID: 20,
		},
	}
	out := toOutput(link)

	if out.SourceIssueIID != 0 {
		t.Errorf("SourceIssueIID = %d, want 0 for nil source", out.SourceIssueIID)
	}
	if out.SourceProjectID != 0 {
		t.Errorf("SourceProjectID = %d, want 0 for nil source", out.SourceProjectID)
	}
	if out.TargetIssueIID != 8 {
		t.Errorf("TargetIssueIID = %d, want 8", out.TargetIssueIID)
	}
}

// TestToOutput_NilTargetIssue verifies ToOutput when nil target issue.
func TestToOutput_NilTargetIssue(t *testing.T) {
	link := &gl.IssueLink{
		ID:       2,
		LinkType: "is_blocked_by",
		SourceIssue: &gl.Issue{
			IID:       5,
			ProjectID: 10,
		},
	}
	out := toOutput(link)

	if out.TargetIssueIID != 0 {
		t.Errorf("TargetIssueIID = %d, want 0 for nil target", out.TargetIssueIID)
	}
	if out.TargetProjectID != 0 {
		t.Errorf("TargetProjectID = %d, want 0 for nil target", out.TargetProjectID)
	}
	if out.SourceIssueIID != 5 {
		t.Errorf("SourceIssueIID = %d, want 5", out.SourceIssueIID)
	}
}

// TestToOutputBoth_Nil verifies ToOutputBoth when nil.
func TestToOutputBoth_Nil(t *testing.T) {
	link := &gl.IssueLink{
		ID:       3,
		LinkType: "relates_to",
	}
	out := toOutput(link)

	if out.ID != 3 {
		t.Errorf("ID = %d, want 3", out.ID)
	}
	if out.SourceIssueIID != 0 || out.SourceProjectID != 0 {
		t.Errorf("expected zero source fields for nil source issue")
	}
	if out.TargetIssueIID != 0 || out.TargetProjectID != 0 {
		t.Errorf("expected zero target fields for nil target issue")
	}
}

// ---------------------------------------------------------------------------
// toRelationOutput converter
// ---------------------------------------------------------------------------.

// TestToRelationOutput_FullFields verifies ToRelationOutput when full fields.
func TestToRelationOutput_FullFields(t *testing.T) {
	r := &gl.IssueRelation{
		ID:          100,
		IID:         8,
		Title:       "Related issue",
		State:       "opened",
		ProjectID:   10,
		LinkType:    "relates_to",
		IssueLinkID: 1,
		WebURL:      "https://gitlab.example.com/group/project/-/issues/8",
	}
	out := toRelationOutput(r)

	if out.ID != 100 {
		t.Errorf("ID = %d, want 100", out.ID)
	}
	if out.IID != 8 {
		t.Errorf("IID = %d, want 8", out.IID)
	}
	if out.Title != "Related issue" {
		t.Errorf("Title = %q, want %q", out.Title, "Related issue")
	}
	if out.State != "opened" {
		t.Errorf("State = %q, want %q", out.State, "opened")
	}
	if out.ProjectID != 10 {
		t.Errorf("ProjectID = %d, want 10", out.ProjectID)
	}
	if out.LinkType != "relates_to" {
		t.Errorf("LinkType = %q, want %q", out.LinkType, "relates_to")
	}
	if out.IssueLinkID != 1 {
		t.Errorf("IssueLinkID = %d, want 1", out.IssueLinkID)
	}
	if out.WebURL != "https://gitlab.example.com/group/project/-/issues/8" {
		t.Errorf("WebURL = %q", out.WebURL)
	}
}

// ---------------------------------------------------------------------------
// Handler API error paths
// ---------------------------------------------------------------------------.

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", IssueIID: 10})
	if err == nil {
		t.Fatal("expected error for API 500 response")
	}
	if !strings.Contains(err.Error(), "list issue links") {
		t.Errorf("error should contain context, got: %v", err)
	}
}

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", IssueIID: 10, IssueLinkID: 100})
	if err == nil {
		t.Fatal("expected error for API 500 response")
	}
	if !strings.Contains(err.Error(), "get issue link") {
		t.Errorf("error should contain context, got: %v", err)
	}
}

// TestCreate_APIError verifies Create when API error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", IssueIID: 10, TargetProjectID: "42", TargetIssueIID: "20",
	})
	if err == nil {
		t.Fatal("expected error for API 500 response")
	}
	if !strings.Contains(err.Error(), "create issue link") {
		t.Errorf("error should contain context, got: %v", err)
	}
}

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", IssueIID: 10, IssueLinkID: 100})
	if err == nil {
		t.Fatal("expected error for API 500 response")
	}
	if !strings.Contains(err.Error(), "delete issue link") {
		t.Errorf("error should contain context, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// MCP integration — RegisterTools
// ---------------------------------------------------------------------------.

const (
	// pathIssueLinks identifies the path issue links constant used by this package.
	pathIssueLinks = "/api/v4/projects/42/issues/10/links"
	// pathIssueLink99 identifies the path issue link 99 constant used by this package.
	pathIssueLink99 = "/api/v4/projects/42/issues/10/links/99"

	// issueLinkJSON identifies the issue link JSON constant used by this package.
	issueLinkJSON = `{
		"id":99,
		"source_issue":{"id":50,"iid":10,"project_id":42,"title":"Source","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
		"target_issue":{"id":80,"iid":20,"project_id":42,"title":"Target","state":"opened","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
		"link_type":"relates_to"
	}`

	// issueRelationJSON identifies the issue relation JSON constant used by this package.
	issueRelationJSON = `[{
		"id":100,"iid":8,"title":"Related issue","state":"opened",
		"project_id":42,"issue_link_id":99,"link_type":"relates_to",
		"web_url":"https://gitlab.example.com/group/project/-/issues/8"
	}]`
)

// ---------------------------------------------------------------------------
// FormatListMarkdown with special characters
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_SpecialChars verifies FormatListMarkdown when special chars.
func TestFormatListMarkdown_SpecialChars(t *testing.T) {
	out := ListOutput{
		Relations: []RelationOutput{
			{ID: 1, IID: 2, Title: "Title with | pipe", State: "opened", LinkType: "relates_to", IssueLinkID: 1},
		},
	}
	md := FormatListMarkdown(out)
	if strings.Contains(md, "| pipe |") {
		t.Error("pipe character in title should be escaped in table cell")
	}
}
