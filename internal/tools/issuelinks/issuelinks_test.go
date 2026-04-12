// issuelinks_test.go contains unit tests for the issue link MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package issuelinks

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const errExpMissingProjectID = "expected error for missing project_id"

const errExpCancelledCtx = "expected error for canceled context"

const fmtUnexpErr = "unexpected error: %v"

const testPathIssueLinks = "/api/v4/projects/10/issues/5/links"

const errExpMissingIssueIID = "expected error for missing issue_iid"

const testProjectID = "10"

const fmtLinkTypeWant = "LinkType = %q, want %q"

const testLinkRelatesTo = "relates_to"

// ---------------------------------------------------------------------------
// Issue Link List
// ---------------------------------------------------------------------------.

// TestIssueLinkList_Success verifies the behavior of issue link list success.
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

// TestIssueLinkList_Empty verifies the behavior of issue link list empty.
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

// TestIssueLinkList_MissingProjectID verifies the behavior of issue link list missing project i d.
func TestIssueLinkList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(context.Background(), client, ListInput{IssueIID: 5})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestIssueLinkList_MissingIssueIID verifies the behavior of issue link list missing issue i i d.
func TestIssueLinkList_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: testProjectID})
	if err == nil {
		t.Fatal(errExpMissingIssueIID)
	}
}

// TestIssueLinkList_CancelledContext verifies the behavior of issue link list cancelled context.
func TestIssueLinkList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := List(ctx, client, ListInput{ProjectID: testProjectID, IssueIID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Issue Link Get
// ---------------------------------------------------------------------------.

// TestIssueLinkGet_Success verifies the behavior of issue link get success.
func TestIssueLinkGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/issues/5/links/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,
				"source_issue":{"id":50,"iid":5,"project_id":10,"title":"Source","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
				"target_issue":{"id":80,"iid":8,"project_id":10,"title":"Target","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
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

// TestIssueLinkGet_MissingProjectID verifies the behavior of issue link get missing project i d.
func TestIssueLinkGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Get(context.Background(), client, GetInput{IssueIID: 5, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestIssueLinkGet_MissingIssueIID verifies the behavior of issue link get missing issue i i d.
func TestIssueLinkGet_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpMissingIssueIID)
	}
}

// TestIssueLinkGet_MissingLinkID verifies the behavior of issue link get missing link i d.
func TestIssueLinkGet_MissingLinkID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: testProjectID, IssueIID: 5})
	if err == nil {
		t.Fatal("expected error for missing issue_link_id")
	}
}

// TestIssueLinkGet_CancelledContext verifies the behavior of issue link get cancelled context.
func TestIssueLinkGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Get(ctx, client, GetInput{ProjectID: testProjectID, IssueIID: 5, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Issue Link Create
// ---------------------------------------------------------------------------.

// TestIssueLinkCreate_Success verifies the behavior of issue link create success.
func TestIssueLinkCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathIssueLinks && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":2,
				"source_issue":{"id":50,"iid":5,"project_id":10,"title":"Source","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
				"target_issue":{"id":120,"iid":12,"project_id":20,"title":"Target","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
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

// TestIssueLinkCreate_WithoutLinkType verifies the behavior of issue link create without link type.
func TestIssueLinkCreate_WithoutLinkType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathIssueLinks && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":3,
				"source_issue":{"id":50,"iid":5,"project_id":10,"title":"Source","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
				"target_issue":{"id":70,"iid":7,"project_id":10,"title":"Target","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
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

// TestIssueLinkCreate_MissingProjectID verifies the behavior of issue link create missing project i d.
func TestIssueLinkCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Create(context.Background(), client, CreateInput{IssueIID: 5, TargetProjectID: "20", TargetIssueIID: "12"})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestIssueLinkCreate_MissingIssueIID verifies the behavior of issue link create missing issue i i d.
func TestIssueLinkCreate_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, TargetProjectID: "20", TargetIssueIID: "12"})
	if err == nil {
		t.Fatal(errExpMissingIssueIID)
	}
}

// TestIssueLinkCreate_MissingTargetProject verifies the behavior of issue link create missing target project.
func TestIssueLinkCreate_MissingTargetProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, IssueIID: 5, TargetIssueIID: "12"})
	if err == nil {
		t.Fatal("expected error for missing target_project_id")
	}
}

// TestIssueLinkCreate_MissingTargetIssue verifies the behavior of issue link create missing target issue.
func TestIssueLinkCreate_MissingTargetIssue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: testProjectID, IssueIID: 5, TargetProjectID: "20"})
	if err == nil {
		t.Fatal("expected error for missing target_issue_iid")
	}
}

// TestIssueLinkCreate_CancelledContext verifies the behavior of issue link create cancelled context.
func TestIssueLinkCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Create(ctx, client, CreateInput{ProjectID: testProjectID, IssueIID: 5, TargetProjectID: "20", TargetIssueIID: "12"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Issue Link Delete
// ---------------------------------------------------------------------------.

// TestIssueLinkDelete_Success verifies the behavior of issue link delete success.
func TestIssueLinkDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/issues/5/links/1" && r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,
				"source_issue":{"id":50,"iid":5,"project_id":10,"title":"Source","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
				"target_issue":{"id":80,"iid":8,"project_id":10,"title":"Target","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
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

// TestIssueLinkDelete_MissingProjectID verifies the behavior of issue link delete missing project i d.
func TestIssueLinkDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(context.Background(), client, DeleteInput{IssueIID: 5, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestIssueLinkDelete_MissingIssueIID verifies the behavior of issue link delete missing issue i i d.
func TestIssueLinkDelete_MissingIssueIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, IssueLinkID: 1})
	if err == nil {
		t.Fatal(errExpMissingIssueIID)
	}
}

// TestIssueLinkDelete_MissingLinkID verifies the behavior of issue link delete missing link i d.
func TestIssueLinkDelete_MissingLinkID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: testProjectID, IssueIID: 5})
	if err == nil {
		t.Fatal("expected error for missing issue_link_id")
	}
}

// TestIssueLinkDelete_CancelledContext verifies the behavior of issue link delete cancelled context.
func TestIssueLinkDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
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

// TestFormatOutputMarkdown_Populated validates format output markdown populated across multiple scenarios using table-driven subtests.
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

// TestFormatOutputMarkdown_Empty verifies the behavior of format output markdown empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for zero-ID output, got %q", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Populated validates format list markdown populated across multiple scenarios using table-driven subtests.
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

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No linked issues found") {
		t.Errorf("expected empty-state message, got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// toOutput converter
// ---------------------------------------------------------------------------.

// TestToOutput_FullFields verifies the behavior of to output full fields.
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

// TestToOutput_NilSourceIssue verifies the behavior of to output nil source issue.
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

// TestToOutput_NilTargetIssue verifies the behavior of to output nil target issue.
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

// TestToOutputBoth_Nil verifies the behavior of to output both nil.
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

// TestToRelationOutput_FullFields verifies the behavior of to relation output full fields.
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

// TestList_APIError verifies the behavior of list a p i error.
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

// TestGet_APIError verifies the behavior of get a p i error.
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

// TestCreate_APIError verifies the behavior of create a p i error.
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

// TestDelete_APIError verifies the behavior of delete a p i error.
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
	msgServerError  = "server error"
	pathIssueLinks  = "/api/v4/projects/42/issues/10/links"
	pathIssueLink99 = "/api/v4/projects/42/issues/10/links/99"

	issueLinkJSON = `{
		"id":99,
		"source_issue":{"id":50,"iid":10,"project_id":42,"title":"Source","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
		"target_issue":{"id":80,"iid":20,"project_id":42,"title":"Target","state":"opened","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"},
		"link_type":"relates_to"
	}`

	issueRelationJSON = `[{
		"id":100,"iid":8,"title":"Related issue","state":"opened",
		"project_id":42,"issue_link_id":99,"link_type":"relates_to",
		"web_url":"https://gitlab.example.com/group/project/-/issues/8"
	}]`
)

// newIssueLinksMCPSession is an internal helper for the issuelinks package.
func newIssueLinksMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == pathIssueLinks:
			testutil.RespondJSON(w, http.StatusOK, issueRelationJSON)
		case r.Method == http.MethodGet && path == pathIssueLink99:
			testutil.RespondJSON(w, http.StatusOK, issueLinkJSON)
		case r.Method == http.MethodPost && path == pathIssueLinks:
			testutil.RespondJSON(w, http.StatusCreated, issueLinkJSON)
		case r.Method == http.MethodDelete && path == pathIssueLink99:
			testutil.RespondJSON(w, http.StatusOK, issueLinkJSON)
		default:
			http.NotFound(w, r)
		}
	}))

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

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newIssueLinksMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_issue_link_list", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_issue_link_get", map[string]any{"project_id": "42", "issue_iid": 10, "issue_link_id": 99}},
		{"gitlab_issue_link_create", map[string]any{"project_id": "42", "issue_iid": 10, "target_project_id": "42", "target_issue_iid": "20", "link_type": "relates_to"}},
		{"gitlab_issue_link_delete", map[string]any{"project_id": "42", "issue_iid": 10, "issue_link_id": 99}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.name, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.name)
			}
		})
	}
}

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// FormatListMarkdown with special characters
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_SpecialChars verifies the behavior of format list markdown special chars.
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
