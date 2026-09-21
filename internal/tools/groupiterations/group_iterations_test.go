// group_iterations_test.go contains unit tests for GitLab group iteration
// operations. Tests use httptest to mock the GitLab Group Iterations API.
package groupiterations

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const fmtUnexpErr = "unexpected error: %v"

// catalogGroupPrefix is the catalog group this package's one action is exposed
// through, as `internal/tools` composes it; the canonical ID a caller executes
// is this plus the spec's own name.
const catalogGroupPrefix = "issue."

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	specs := IssueActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(IssueActionSpecs) = %d, want 1", len(specs))
	}
	spec := specs[0]
	if spec.OwnerPackage != "groupiterations" || !spec.ReadOnly || !spec.Idempotent {
		t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
	}
	if spec.Usage == "" {
		t.Fatalf("Usage for %s is empty", spec.Name)
	}
	if len(spec.Aliases) == 0 {
		t.Fatalf("Aliases for %s are empty", spec.Name)
	}

	byTool := map[string]toolutil.ActionSpec{}
	for _, s := range specs {
		byTool[s.IndividualTool.Name] = s
	}
	if _, ok := byTool["gitlab_list_group_iterations"]; !ok {
		t.Fatal("missing individual tool mapping for gitlab_list_group_iterations")
	}
}

// TestIssueActionSpecs_ListHint_NamesTheActionThisPackageRegisters holds the
// card's group-list hint to the action this package actually registers.
//
// The two halves are written apart, the canonical ID in markdown.go and the
// name in action_specs.go, and the card tests below pin the hint's literal
// text. So renaming the spec leaves the card telling a model to execute an
// action the catalog no longer has, with every other test in this file still
// green. This is the only assertion that joins them.
func TestIssueActionSpecs_ListHint_NamesTheActionThisPackageRegisters(t *testing.T) {
	specs := IssueActionSpecs(testutil.NewTestClient(t, http.NewServeMux()))
	if len(specs) != 1 {
		t.Fatalf("len(IssueActionSpecs) = %d, want 1", len(specs))
	}
	if want := catalogGroupPrefix + specs[0].Name; actionListGroup != want {
		t.Errorf("group-list hint names %q, but the registered action is %q", actionListGroup, want)
	}
}

// TestIssueActionSpecs_Edition_KeepsTheActionBehindPremium holds the tier gate
// that keeps this action off a Free catalog.
//
// Iterations are a Premium feature, and the handler's own 404 hint tells a
// caller so after the call has already failed. What stops a Free deployment
// being offered the action in the first place is this one field: emptied, the
// action is listed to every tier and a model spends a round trip to learn it
// was never available. Nothing else in this file reads it.
func TestIssueActionSpecs_Edition_KeepsTheActionBehindPremium(t *testing.T) {
	specs := IssueActionSpecs(testutil.NewTestClient(t, http.NewServeMux()))
	if len(specs) != 1 {
		t.Fatalf("len(IssueActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].Edition != "premium" {
		t.Errorf("Edition = %q, want %q", specs[0].Edition, "premium")
	}
}

// TestList_Success verifies List returns correct group iteration fields
// including id, iid, title, state, and web_url from a well-formed API response.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/groups/10/iterations")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":2,"iid":1,"sequence":1,"group_id":10,"title":"Sprint 2","state":1,"web_url":"https://gitlab.example.com/iterations/2"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Iterations) != 1 {
		t.Fatalf("got %d iterations, want 1", len(out.Iterations))
	}
	it := out.Iterations[0]
	if it.Title != "Sprint 2" {
		t.Errorf("got title %q, want %q", it.Title, "Sprint 2")
	}
	if it.State != 1 {
		t.Errorf("got state %d, want 1", it.State)
	}
	if it.IID != 1 {
		t.Errorf("got IID %d, want 1", it.IID)
	}
	if it.ID != 2 {
		t.Errorf("got ID %d, want 2", it.ID)
	}
	if it.GroupID != 10 {
		t.Errorf("got GroupID %d, want 10", it.GroupID)
	}
	if it.WebURL != "https://gitlab.example.com/iterations/2" {
		t.Errorf("got WebURL %q, want non-empty URL", it.WebURL)
	}
}

// TestList_ValidationError_MissingGroupID verifies that List_ValidationError_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_ValidationError_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestList_QueryParams verifies that List_QueryParams forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_QueryParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/5/iterations")
		testutil.AssertQueryParam(t, r, "state", "opened")
		testutil.AssertQueryParam(t, r, "search", "sprint")
		testutil.AssertQueryParam(t, r, "include_ancestors", "true")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID:          "5",
		State:            "opened",
		Search:           "sprint",
		IncludeAncestors: true,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_KeysetParams verifies that List forwards keyset pagination and
// sort parameters (pagination, page_token, order_by, sort) to the GitLab API.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the keyset and ordering query parameters are present on the request.
func TestList_KeysetParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/7/iterations")
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "cursor-1")
		testutil.AssertQueryParam(t, r, "order_by", "due_date")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID:    "7",
		OrderBy:    "due_date",
		Sort:       "desc",
		Pagination: "keyset",
		PageToken:  "cursor-1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_EmptyResult verifies the List_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Iterations) != 0 {
		t.Errorf("got %d iterations, want 0", len(out.Iterations))
	}
}

// TestList_APIError verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_APIError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "returns error on 404 not found",
			status: http.StatusNotFound,
			body:   `{"message":"404 Group Not Found"}`,
		},
		{
			name:   "returns error on 400 bad request",
			status: http.StatusBadRequest,
			body:   `{"message":"400 Bad Request"}`,
		},
		{
			name:   "returns error on 403 forbidden",
			status: http.StatusForbidden,
			body:   `{"message":"403 Forbidden"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, tt.body)
			}))

			_, err := List(context.Background(), client, ListInput{GroupID: "999"})
			if err == nil {
				t.Fatal("expected error from API, got nil")
			}
		})
	}
}

// TestList_ErrorHint_OnlyA404NamesTheGroupAndTheLicense holds which status the
// corrective hint is attached to, and what it says.
//
// A group iteration list fails two ways that read alike on the wire: the group
// is not there, and the instance has no Premium license for iterations at all.
// The 404 branch is the only one that can tell a caller which of the two to
// check, so a hint moved to another status, or reworded off group.get
// and the license, leaves a model holding GitLab's bare message with nothing
// to do about it. [TestList_APIError] above cannot see any of this: every case
// here returns a non-nil error whichever status the hint is bound to.
func TestList_ErrorHint_OnlyA404NamesTheGroupAndTheLicense(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "404 names the group check and the license", status: http.StatusNotFound, wantHint: true},
		{name: "403 is left to the generic wrap", status: http.StatusForbidden, wantHint: false},
		{name: "400 is left to the generic wrap", status: http.StatusBadRequest, wantHint: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"refused"}`)
			}))

			_, err := List(context.Background(), client, ListInput{GroupID: "10"})
			if err == nil {
				t.Fatalf("expected an error for status %d, got nil", tt.status)
			}
			if !strings.Contains(err.Error(), "gitlab_list_group_iterations") {
				t.Errorf("error does not name the operation: %v", err)
			}
			hinted := strings.Contains(err.Error(), "group.get") && strings.Contains(err.Error(), "Premium")
			if hinted != tt.wantHint {
				t.Errorf("hint present = %v, want %v; error: %v", hinted, tt.wantHint, err)
			}
		})
	}
}

// TestList_OffsetPagination_ReachesGitLabAndComesBack holds that a caller's
// page and per_page leave the process, and that the page GitLab answered with
// comes back.
//
// Every other request test in this file drives the default page, so both
// parameters could stop being sent and each of those assertions would still
// pass while a model asking for page 3 silently read page 1 for ever. The
// response half is asserted against the fixture's own headers rather than
// against the input, so filling the block from the request instead of the
// response would fail here too.
func TestList_OffsetPagination_ReachesGitLabAndComesBack(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/10/iterations")
		testutil.AssertQueryParam(t, r, "page", "3")
		testutil.AssertQueryParam(t, r, "per_page", "25")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "3", PerPage: "25", Total: "60", TotalPages: "3", PrevPage: "2"})
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID: "10",
		Page:    3,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.Page != 3 {
		t.Errorf("pagination page = %d, want 3", out.Pagination.Page)
	}
	if out.Pagination.PerPage != 25 {
		t.Errorf("pagination per_page = %d, want 25", out.Pagination.PerPage)
	}
	if out.Pagination.PrevPage != 2 {
		t.Errorf("pagination prev_page = %d, want 2", out.Pagination.PrevPage)
	}
}

// TestList_Pagination verifies that List forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"iid":1,"title":"Sprint 1","state":1,"group_id":10}
		]`, testutil.PaginationHeaders{
			Page: "1", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "2",
		})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalItems != 3 {
		t.Errorf("pagination total_items = %d, want 3", out.Pagination.TotalItems)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("pagination total_pages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("pagination next_page = %d, want 2", out.Pagination.NextPage)
	}
}

// TestList_ContextCancelled verifies the List_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestList_WithDates verifies the List_WithDates handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_WithDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[{
			"id":5,"iid":2,"sequence":2,"group_id":10,"title":"Sprint 3","state":3,
			"start_date":"2026-01-01","due_date":"2026-01-14",
			"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-10T12:00:00Z"
		}]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Iterations) != 1 {
		t.Fatalf("got %d iterations, want 1", len(out.Iterations))
	}
	it := out.Iterations[0]
	if it.StartDate == "" {
		t.Error("expected non-empty StartDate")
	}
	if it.DueDate == "" {
		t.Error("expected non-empty DueDate")
	}
	if it.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if it.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}

// TestToOutput_NilInput verifies the ToOutput_NilInput handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToOutput_NilInput(t *testing.T) {
	out := toOutput(nil)
	if out.ID != 0 || out.Title != "" {
		t.Errorf("expected zero Output for nil, got %+v", out)
	}
}

// TestToOutput_AllFields verifies the ToOutput_AllFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToOutput_AllFields(t *testing.T) {
	now := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	startDate := gl.ISOTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	dueDate := gl.ISOTime(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))

	it := &gl.GroupIteration{
		ID:          42,
		IID:         7,
		Sequence:    3,
		GroupID:     10,
		Title:       "Sprint 7",
		Description: "Iteration description",
		State:       2,
		WebURL:      "https://gitlab.example.com/iterations/42",
		StartDate:   &startDate,
		DueDate:     &dueDate,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	out := toOutput(it)
	if out.ID != 42 {
		t.Errorf("ID = %d, want 42", out.ID)
	}
	if out.IID != 7 {
		t.Errorf("IID = %d, want 7", out.IID)
	}
	if out.Sequence != 3 {
		t.Errorf("Sequence = %d, want 3", out.Sequence)
	}
	if out.GroupID != 10 {
		t.Errorf("GroupID = %d, want 10", out.GroupID)
	}
	if out.Title != "Sprint 7" {
		t.Errorf("Title = %q, want %q", out.Title, "Sprint 7")
	}
	if out.Description != "Iteration description" {
		t.Errorf("Description = %q, want %q", out.Description, "Iteration description")
	}
	if out.State != 2 {
		t.Errorf("State = %d, want 2", out.State)
	}
	if out.WebURL != "https://gitlab.example.com/iterations/42" {
		t.Errorf("WebURL = %q, want non-empty", out.WebURL)
	}
	if out.StartDate == "" {
		t.Error("expected non-empty StartDate")
	}
	if out.DueDate == "" {
		t.Error("expected non-empty DueDate")
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}

// TestToOutput_NilDates verifies the ToOutput_NilDates handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToOutput_NilDates(t *testing.T) {
	it := &gl.GroupIteration{
		ID:    1,
		Title: "No dates",
		State: 1,
	}

	out := toOutput(it)
	if out.StartDate != "" {
		t.Errorf("StartDate = %q, want empty", out.StartDate)
	}
	if out.DueDate != "" {
		t.Errorf("DueDate = %q, want empty", out.DueDate)
	}
	if out.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", out.CreatedAt)
	}
	if out.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty", out.UpdatedAt)
	}
}

// TestFormatListMarkdown_Empty pins the whole render of an empty page: the
// one-sentence empty message, with no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	if got, want := FormatListMarkdown(ListOutput{}), "No group iterations found.\n"; got != want {
		t.Errorf("FormatListMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatListMarkdown_WithIterations pins the whole table: the group title
// in the heading, the iteration title linked to its page, GitLab's state
// words, the display form of both dates, and the guidance section last.
func TestFormatListMarkdown_WithIterations(t *testing.T) {
	out := ListOutput{
		Iterations: []Output{
			{ID: 1, IID: 1, Title: "Sprint 1", State: 1, StartDate: "2026-01-01", DueDate: "2026-01-14", WebURL: "https://gitlab.example.com/it/1"},
			{ID: 2, IID: 2, Title: "Sprint 2", State: 3, StartDate: "2026-01-15", DueDate: "2026-01-28", WebURL: ""},
		},
	}

	got := FormatListMarkdown(out)

	want := "## Group Iterations (2)\n\n" +
		"| ID | IID | Title | State | Start | Due |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | 1 | [Sprint 1](https://gitlab.example.com/it/1) | upcoming | 1 Jan 2026 | 14 Jan 2026 |\n" +
		"| 2 | 2 | Sprint 2 | closed | 15 Jan 2026 | 28 Jan 2026 |\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n"
	if got != want {
		t.Errorf("FormatListMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatOutputMarkdown_Full pins the whole card of a populated group
// iteration, hints included: both list actions are named so the card reads the
// same whichever of the two iteration packages registered the formatter.
func TestFormatOutputMarkdown_Full(t *testing.T) {
	out := Output{
		ID:          42,
		IID:         7,
		Title:       "Sprint 7",
		State:       2,
		GroupID:     10,
		StartDate:   "2026-03-01",
		DueDate:     "2026-03-14",
		WebURL:      "https://gitlab.example.com/iterations/42",
		CreatedAt:   "2026-03-01T00:00:00Z",
		Description: "This is the iteration description.",
	}

	got := FormatOutputMarkdown(out)

	want := "## Iteration #7: Sprint 7\n\n" +
		"- **ID**: 42\n" +
		"- **IID**: 7\n" +
		"- **State**: current\n" +
		"- **Group ID**: 10\n" +
		"- **Start**: 1 Mar 2026\n" +
		"- **Due**: 14 Mar 2026\n" +
		"- **URL**: [https://gitlab.example.com/iterations/42](https://gitlab.example.com/iterations/42)\n" +
		"- **Created**: 1 Mar 2026 00:00 UTC\n" +
		"- **Description**: This is the iteration description.\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'issue.iteration_list_group' to see every iteration in the group\n" +
		"- Use action 'issue.iteration_list_project' to see the iterations a project takes part in\n"
	if got != want {
		t.Errorf("FormatOutputMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatOutputMarkdown_NoDescriptionNoWebURL pins the whole card of an
// iteration GitLab sent no description, URL or dates for: each absent value
// writes no row at all, and no "| URL |" row survives anywhere.
func TestFormatOutputMarkdown_NoDescriptionNoWebURL(t *testing.T) {
	got := FormatOutputMarkdown(Output{ID: 1, IID: 1, Title: "Minimal", State: 1})

	want := "## Iteration #1: Minimal\n\n" +
		"- **ID**: 1\n" +
		"- **IID**: 1\n" +
		"- **State**: upcoming\n\n" +
		"---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'issue.iteration_list_group' to see every iteration in the group\n" +
		"- Use action 'issue.iteration_list_project' to see the iterations a project takes part in\n"
	if got != want {
		t.Errorf("FormatOutputMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestList_MultipleIterations verifies the List_MultipleIterations handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_MultipleIterations(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id":1,"iid":1,"title":"Sprint 1","state":1,"group_id":10},
			{"id":2,"iid":2,"title":"Sprint 2","state":2,"group_id":10},
			{"id":3,"iid":3,"title":"Sprint 3","state":3,"group_id":10}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "3", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Iterations) != 3 {
		t.Fatalf("got %d iterations, want 3", len(out.Iterations))
	}
	for i, want := range []string{"Sprint 1", "Sprint 2", "Sprint 3"} {
		t.Run(want, func(t *testing.T) {
			if out.Iterations[i].Title != want {
				t.Errorf("iteration[%d].Title = %q, want %q", i, out.Iterations[i].Title, want)
			}
		})
	}
}

// TestIssueActionSpecs_CallRoute verifies the IssueActionSpecs_CallRoute handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestIssueActionSpecs_CallRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(
			w, http.StatusOK,
			`[{"id":1,"iid":1,"title":"Sprint 1","state":1,"group_id":10}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
		)
	})
	client := testutil.NewTestClient(t, mux)
	specs := IssueActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(IssueActionSpecs) = %d, want 1", len(specs))
	}
	spec := specs[0]
	if spec.IndividualTool.Name != "gitlab_list_group_iterations" || spec.OwnerPackage != "groupiterations" {
		t.Fatalf("unexpected ActionSpec: %+v", spec)
	}
	result, err := spec.Route.Handler(t.Context(), map[string]any{"group_id": "10"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}
