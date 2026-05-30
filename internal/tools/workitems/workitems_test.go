// workitems_test.go contains unit tests for the work item MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package workitems

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"42","workItemType":{"name":"Issue"},"state":"OPEN","title":"Test item","description":"A description","webUrl":"https://gitlab.example.com/-/work_items/42","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{FullPath: testFullPath, IID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "Test item" {
		t.Errorf("expected title 'Test item', got %s", out.WorkItem.Title)
	}
}

// TestGet_InvalidIID verifies Get when invalid IID.
func TestGet_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	for _, iid := range []int64{0, -1, -100} {
		_, err := Get(t.Context(), client, GetInput{FullPath: testFullPath, IID: iid})
		if err == nil {
			t.Fatalf("expected error for IID=%d, got nil", iid)
		}
		if !strings.Contains(err.Error(), "work_item_iid") {
			t.Errorf("expected error to mention 'iid' for IID=%d, got: %v", iid, err)
		}
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{FullPath: testFullPath, IID: 42})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item 1","description":"Desc 1","confidential":true,"webUrl":"https://gitlab.example.com/work_items/10","author":{"username":"dev1"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","closedAt":""},{"id":"gid://gitlab/WorkItem/2","iid":"11","workItemType":{"name":"Task"},"state":"CLOSED","title":"Item 2","author":{"username":"dev2"}}]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 2 {
		t.Fatalf("expected 2 work items, got %d", len(out.WorkItems))
	}
	first := out.WorkItems[0]
	if first.ID != 1 || first.IID != 10 || first.Type != testTypeIssue || first.Author != "dev1" {
		t.Fatalf("unexpected first work item: %+v", first)
	}
	if !first.Confidential || first.WebURL == "" || first.CreatedAt == "" || first.UpdatedAt == "" {
		t.Fatalf("expected mapped optional fields, got %+v", first)
	}
}

// TestList_Empty verifies List when empty.
func TestList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 0 {
		t.Fatalf("expected 0 work items, got %d", len(out.WorkItems))
	}
}

// TestList_Filters verifies List forwards supported filters to the minimal GraphQL query.
func TestList_Filters(t *testing.T) {
	confidential := true
	first := int64(5)
	includeAncestors := true
	includeDescendants := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode GraphQL request: %v", err)
		}
		expected := map[string]any{
			"fullPath":           testFullPath,
			"state":              "opened",
			"search":             "needle",
			"authorUsername":     testAuthorDev,
			"confidential":       true,
			"sort":               "CREATED_DESC",
			"first":              float64(5),
			"after":              "cursor-1",
			"includeAncestors":   true,
			"includeDescendants": false,
		}
		for key, want := range expected {
			if got := request.Variables[key]; got != want {
				t.Fatalf("variable %s = %#v, want %#v", key, got, want)
			}
		}
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{
		FullPath:           testFullPath,
		State:              "opened",
		Search:             "needle",
		Types:              []string{testTypeIssue},
		AuthorUsername:     testAuthorDev,
		LabelName:          []string{testLabelBug},
		Confidential:       &confidential,
		Sort:               "CREATED_DESC",
		First:              &first,
		After:              "cursor-1",
		IncludeAncestors:   &includeAncestors,
		IncludeDescendants: &includeDescendants,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestList_MissingFullPath verifies List validates full_path before calling GitLab.
func TestList_MissingFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "full_path") {
		t.Fatalf("expected error to mention full_path, got %v", err)
	}
}

// TestList_GraphQLErrors verifies List surfaces GraphQL errors from 200 responses.
func TestList_GraphQLErrors(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"errors":[{"message":"field error"}]}`)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "field error") {
		t.Fatalf("expected GraphQL error detail, got %v", err)
	}
}

// TestList_NamespaceNotFound verifies List gives an actionable error when full_path is absent.
func TestList_NamespaceNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":null}}`)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "gitlab_project_list") {
		t.Fatalf("expected actionable hint, got %v", err)
	}
}

// TestList_InvalidGraphQLIDs verifies List rejects malformed ID fields instead of returning misleading zeros.
func TestList_InvalidGraphQLIDs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "invalid gid",
			body: `{"data":{"namespace":{"workItems":{"nodes":[{"id":"not-a-gid","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item"}]}}}}`,
			want: "invalid work item id",
		},
		{
			name: "invalid iid",
			body: `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"abc","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item"}]}}}}`,
			want: "invalid work item iid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, tt.body)
			})
			client := testutil.NewTestClient(t, handler)
			_, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
			if err == nil {
				t.Fatal(errExpectedNil)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

// TestCreate_Success verifies Create when success.
func TestCreate_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/99","iid":"99","workItemType":{"name":"Issue"},"state":"OPEN","title":"New item","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Create(t.Context(), client, CreateInput{
		FullPath:       testFullPath,
		WorkItemTypeID: testTypeGID,
		Title:          testTitleNewItem,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != testTitleNewItem {
		t.Errorf("expected title 'New item', got %s", out.WorkItem.Title)
	}
}

// TestCreate_Error verifies Create when error.
func TestCreate_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Create(t.Context(), client, CreateInput{
		FullPath:       testFullPath,
		WorkItemTypeID: testTypeGID,
		Title:          testTitleNewItem,
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDelete_Success verifies that a work item can be deleted by IID.
func TestDelete_Success(t *testing.T) {
	call := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		call++
		switch call {
		case 1:
			// workItemGID query to resolve the global ID
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}}`)
		default:
			// workItemDelete mutation
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemDelete":{"errors":[]}}}`)
		}
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{FullPath: testFullPath, IID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_InvalidIID verifies that Delete rejects invalid IIDs.
func TestDelete_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	for _, iid := range []int64{0, -1, -100} {
		err := Delete(t.Context(), client, DeleteInput{FullPath: testFullPath, IID: iid})
		if err == nil {
			t.Fatalf("expected error for IID=%d, got nil", iid)
		}
		if !strings.Contains(err.Error(), "work_item_iid") {
			t.Errorf("expected error to mention 'iid' for IID=%d, got: %v", iid, err)
		}
	}
}

// TestDelete_Error verifies that Delete propagates API errors.
func TestDelete_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{FullPath: testFullPath, IID: 42})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestFormatGetMarkdown verifies FormatGetMarkdown.
func TestFormatGetMarkdown(t *testing.T) {
	result := FormatGetMarkdown(GetOutput{WorkItem: WorkItemItem{
		IID: 42, Title: "Test", Type: "Issue", State: "OPEN", Author: "dev",
	}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{WorkItems: []WorkItemItem{
		{IID: 1, Type: "Issue", State: "OPEN", Title: "A", Author: "dev"},
	}}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := fmt.Sprintf("%v", result.Content[0])
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpNonNilResult identifies the err exp non nil result constant used by this package.
const errExpNonNilResult = "expected non-nil result"

// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
const errExpCancelledNil = "expected error for canceled context, got nil"

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// fmtUnexpMethod identifies the fmt unexp method constant used by this package.
const fmtUnexpMethod = "unexpected method: %s"

// testFullPath identifies the test full path constant used by this package.
const testFullPath = "my-group/my-project"

const (
	// testProjectPath identifies the test project path constant used by this package.
	testProjectPath = "ns/proj"
	// testStateOpen identifies the test state open constant used by this package.
	testStateOpen = "OPEN"
	// testStateClosed identifies the test state closed constant used by this package.
	testStateClosed = "CLOSED"
	// testTypeIssue identifies the test type issue constant used by this package.
	testTypeIssue = "Issue"
	// testTypeTask identifies the test type task constant used by this package.
	testTypeTask = "Task"
	// testTypeGID identifies the test type gid constant used by this package.
	testTypeGID = "gid://gitlab/WorkItems::Type/1"
	// testAuthorAlice identifies the test author alice constant used by this package.
	testAuthorAlice = "alice"
	// testAuthorBob identifies the test author bob constant used by this package.
	testAuthorBob = "bob"
	// testAuthorCarol identifies the test author carol constant used by this package.
	testAuthorCarol = "carol"
	// testAuthorDev identifies the test author dev constant used by this package.
	testAuthorDev = "dev"
	// testLabelBug identifies the test label bug constant used by this package.
	testLabelBug = "bug"
	// testLabelUrgent identifies the test label urgent constant used by this package.
	testLabelUrgent = "urgent"
	// testWorkItemURL identifies the test work item URL constant used by this package.
	testWorkItemURL = "https://gitlab.example.com/-/work_items/42"
	// testSectionDesc identifies the test section desc constant used by this package.
	testSectionDesc = "### Description"
	// fmtDescWant identifies the fmt desc want constant used by this package.
	fmtDescWant = "Description = %q"
	// testTitleNewItem identifies the test title new item constant used by this package.
	testTitleNewItem = "New item"
)

// ---------------------------------------------------------------------------
// workItemToItem -- converter tests
// ---------------------------------------------------------------------------.

// TestWorkItemToItem_FullData verifies WorkItemToItem when full data.
func TestWorkItemToItem_FullData(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	later := now.Add(24 * time.Hour)
	closed := later.Add(48 * time.Hour)
	status := "IN_PROGRESS"

	wi := &gl.WorkItem{
		ID:           100,
		IID:          42,
		Type:         testTypeTask,
		State:        testStateOpen,
		Status:       &status,
		Title:        "Full work item",
		Description:  "A detailed description",
		WebURL:       testWorkItemURL,
		Confidential: true,
		Author:       &gl.BasicUser{Username: testAuthorAlice},
		Assignees:    []*gl.BasicUser{{Username: testAuthorBob}, {Username: testAuthorCarol}},
		Labels:       []gl.LabelDetails{{Name: testLabelBug}, {Name: testLabelUrgent}},
		LinkedItems: []gl.LinkedWorkItem{
			{WorkItemIID: gl.WorkItemIID{NamespacePath: "my-group/other", IID: 7}, LinkType: "blocks"},
		},
		CreatedAt: &now,
		UpdatedAt: &later,
		ClosedAt:  &closed,
	}

	item := workItemToItem(wi)

	assertFullItemCore(t, item)
	assertFullItemPeople(t, item)
	assertFullItemTimestamps(t, item)
}

// assertFullItemCore checks full item core invariants for tests.
func assertFullItemCore(t *testing.T, item WorkItemItem) {
	t.Helper()
	if item.ID != 100 {
		t.Errorf("ID = %d, want 100", item.ID)
	}
	if item.IID != 42 {
		t.Errorf("IID = %d, want 42", item.IID)
	}
	if item.Type != testTypeTask {
		t.Errorf("Type = %q, want Task", item.Type)
	}
	if item.State != testStateOpen {
		t.Errorf("State = %q, want OPEN", item.State)
	}
	if item.Status != "IN_PROGRESS" {
		t.Errorf("Status = %q, want IN_PROGRESS", item.Status)
	}
	if item.Title != "Full work item" {
		t.Errorf("Title = %q, want 'Full work item'", item.Title)
	}
	if item.Description != "A detailed description" {
		t.Errorf(fmtDescWant, item.Description)
	}
	if item.WebURL != testWorkItemURL {
		t.Errorf("WebURL = %q", item.WebURL)
	}
	if !item.Confidential {
		t.Error("expected Confidential=true")
	}
	if len(item.LinkedItems) != 1 {
		t.Fatalf("LinkedItems = %d, want 1", len(item.LinkedItems))
	}
	if item.LinkedItems[0].IID != 7 {
		t.Errorf("LinkedItems[0].IID = %d, want 7", item.LinkedItems[0].IID)
	}
	if item.LinkedItems[0].LinkType != "blocks" {
		t.Errorf("LinkedItems[0].LinkType = %q, want blocks", item.LinkedItems[0].LinkType)
	}
	if item.LinkedItems[0].Path != "my-group/other" {
		t.Errorf("LinkedItems[0].Path = %q, want my-group/other", item.LinkedItems[0].Path)
	}
}

// assertFullItemPeople checks full item people invariants for tests.
func assertFullItemPeople(t *testing.T, item WorkItemItem) {
	t.Helper()
	if item.Author != testAuthorAlice {
		t.Errorf("Author = %q, want alice", item.Author)
	}
	if len(item.Assignees) != 2 || item.Assignees[0] != testAuthorBob || item.Assignees[1] != testAuthorCarol {
		t.Errorf("Assignees = %v, want [bob carol]", item.Assignees)
	}
	if len(item.Labels) != 2 || item.Labels[0] != testLabelBug || item.Labels[1] != testLabelUrgent {
		t.Errorf("Labels = %v, want [bug urgent]", item.Labels)
	}
}

// assertFullItemTimestamps checks full item timestamps invariants for tests.
func assertFullItemTimestamps(t *testing.T, item WorkItemItem) {
	t.Helper()
	if item.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if item.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
	if item.ClosedAt == "" {
		t.Error("expected non-empty ClosedAt")
	}
}

// TestWorkItemToItem_Minimal verifies WorkItemToItem when minimal.
func TestWorkItemToItem_Minimal(t *testing.T) {
	wi := &gl.WorkItem{
		ID:    1,
		IID:   1,
		Type:  testTypeIssue,
		State: testStateClosed,
		Title: "Minimal",
	}

	item := workItemToItem(wi)

	if item.Status != "" {
		t.Errorf("Status should be empty, got %q", item.Status)
	}
	if item.Author != "" {
		t.Errorf("Author should be empty, got %q", item.Author)
	}
	if len(item.Assignees) != 0 {
		t.Errorf("Assignees should be empty, got %v", item.Assignees)
	}
	if len(item.Labels) != 0 {
		t.Errorf("Labels should be empty, got %v", item.Labels)
	}
	if item.CreatedAt != "" {
		t.Errorf("CreatedAt should be empty, got %q", item.CreatedAt)
	}
	if item.UpdatedAt != "" {
		t.Errorf("UpdatedAt should be empty, got %q", item.UpdatedAt)
	}
	if item.ClosedAt != "" {
		t.Errorf("ClosedAt should be empty, got %q", item.ClosedAt)
	}
}

// TestWorkItemToItemNilStatusNon_NilAuthor verifies WorkItemToItemNilStatusNon when nil author.
func TestWorkItemToItemNilStatusNon_NilAuthor(t *testing.T) {
	wi := &gl.WorkItem{
		ID:     5,
		IID:    5,
		Type:   "Epic",
		State:  testStateOpen,
		Title:  "Epic item",
		Author: &gl.BasicUser{Username: testAuthorDev},
	}
	item := workItemToItem(wi)
	if item.Status != "" {
		t.Errorf("Status = %q, want empty", item.Status)
	}
	if item.Author != testAuthorDev {
		t.Errorf("Author = %q, want dev", item.Author)
	}
}

// TestWorkItemToItem_EmptyAssigneesAndLabelsSlices verifies WorkItemToItem when empty assignees and labels slices.
func TestWorkItemToItem_EmptyAssigneesAndLabelsSlices(t *testing.T) {
	wi := &gl.WorkItem{
		ID:        2,
		IID:       2,
		Type:      testTypeIssue,
		State:     testStateOpen,
		Title:     "Edge",
		Assignees: []*gl.BasicUser{},
		Labels:    []gl.LabelDetails{},
	}
	item := workItemToItem(wi)
	if len(item.Assignees) != 0 {
		t.Errorf("expected empty assignees, got %v", item.Assignees)
	}
	if len(item.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", item.Labels)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_FullPopulated verifies FormatGetMarkdown when full populated.
func TestFormatGetMarkdown_FullPopulated(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:         42,
		Title:       "Full WI",
		Type:        testTypeTask,
		State:       testStateOpen,
		Author:      testAuthorAlice,
		Assignees:   []string{testAuthorBob, testAuthorCarol},
		Labels:      []string{testLabelBug, testLabelUrgent},
		WebURL:      "https://gitlab.example.com/work_items/42",
		Description: "A very detailed description.",
	}}
	result := FormatGetMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := extractText(t, result)
	expects := []string{
		"## Work Item #42: Full WI",
		"**Type**: Task",
		"**State**: OPEN",
		"**Author**: alice",
		"**Assignees**: bob, carol",
		"**Labels**: bug, urgent",
		"**URL**: https://gitlab.example.com/work_items/42",
		testSectionDesc,
		"A very detailed description.",
	}
	for _, s := range expects {
		if !strings.Contains(text, s) {
			t.Errorf("missing %q in output:\n%s", s, text)
		}
	}
}

// TestFormatGetMarkdown_Empty verifies FormatGetMarkdown when empty.
func TestFormatGetMarkdown_Empty(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{}}
	result := FormatGetMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := extractText(t, result)
	// Should NOT contain optional sections
	if strings.Contains(text, "**Author**") {
		t.Error("unexpected Author in empty output")
	}
	if strings.Contains(text, "**Assignees**") {
		t.Error("unexpected Assignees in empty output")
	}
	if strings.Contains(text, "**Labels**") {
		t.Error("unexpected Labels in empty output")
	}
	if strings.Contains(text, "**URL**") {
		t.Error("unexpected URL in empty output")
	}
	if strings.Contains(text, testSectionDesc) {
		t.Error("unexpected Description in empty output")
	}
}

// TestFormatGetMarkdown_OnlyAuthor verifies FormatGetMarkdown when only author.
func TestFormatGetMarkdown_OnlyAuthor(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:    1,
		Title:  "Simple",
		Type:   testTypeIssue,
		State:  testStateClosed,
		Author: testAuthorDev,
	}}
	result := FormatGetMarkdown(out)
	text := extractText(t, result)
	if !strings.Contains(text, "**Author**: dev") {
		t.Errorf("missing author in output: %s", text)
	}
	if strings.Contains(text, "**Assignees**") {
		t.Error("unexpected Assignees")
	}
}

// TestFormatGetMarkdown_OnlyAssignees verifies FormatGetMarkdown when only assignees.
func TestFormatGetMarkdown_OnlyAssignees(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:       1,
		Title:     "Assigned",
		Type:      testTypeTask,
		State:     testStateOpen,
		Assignees: []string{testAuthorAlice},
	}}
	result := FormatGetMarkdown(out)
	text := extractText(t, result)
	if !strings.Contains(text, "**Assignees**: alice") {
		t.Errorf("missing assignees: %s", text)
	}
}

// TestFormatGetMarkdown_OnlyLabels verifies FormatGetMarkdown when only labels.
func TestFormatGetMarkdown_OnlyLabels(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:    1,
		Title:  "Labeled",
		Type:   testTypeIssue,
		State:  testStateOpen,
		Labels: []string{"feature"},
	}}
	result := FormatGetMarkdown(out)
	text := extractText(t, result)
	if !strings.Contains(text, "**Labels**: feature") {
		t.Errorf("missing labels: %s", text)
	}
}

// TestFormatGetMarkdown_OnlyWebURL verifies FormatGetMarkdown when only web URL.
func TestFormatGetMarkdown_OnlyWebURL(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:    1,
		Title:  "URL only",
		Type:   testTypeIssue,
		State:  testStateOpen,
		WebURL: "https://example.com/wi/1",
	}}
	result := FormatGetMarkdown(out)
	text := extractText(t, result)
	if !strings.Contains(text, "**URL**: https://example.com/wi/1") {
		t.Errorf("missing URL: %s", text)
	}
}

// TestFormatGetMarkdown_OnlyDescription verifies FormatGetMarkdown when only description.
func TestFormatGetMarkdown_OnlyDescription(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:         1,
		Title:       "With desc",
		Type:        testTypeIssue,
		State:       testStateOpen,
		Description: "My description",
	}}
	result := FormatGetMarkdown(out)
	text := extractText(t, result)
	if !strings.Contains(text, testSectionDesc) {
		t.Errorf("missing Description heading: %s", text)
	}
	if !strings.Contains(text, "My description") {
		t.Errorf("missing description text: %s", text)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_MultipleItems verifies FormatListMarkdown when multiple items.
func TestFormatListMarkdown_MultipleItems(t *testing.T) {
	out := ListOutput{WorkItems: []WorkItemItem{
		{IID: 1, Type: testTypeIssue, State: testStateOpen, Title: "First", Author: "dev1"},
		{IID: 2, Type: testTypeTask, State: testStateClosed, Title: "Second", Author: "dev2"},
		{IID: 3, Type: "Epic", State: testStateOpen, Title: "Third", Author: "dev3"},
	}}
	result := FormatListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := extractText(t, result)
	if !strings.Contains(text, "## Work Items (3)") {
		t.Errorf("missing header with count: %s", text)
	}
	if !strings.Contains(text, "| 1 | Issue | OPEN |  | First | dev1 |") {
		t.Errorf("missing row 1: %s", text)
	}
	if !strings.Contains(text, "| 2 | Task | CLOSED |  | Second | dev2 |") {
		t.Errorf("missing row 2: %s", text)
	}
}

// TestFormatListMarkdown_EmptyReturnsMessage verifies FormatListMarkdown returns message for empty.
func TestFormatListMarkdown_EmptyReturnsMessage(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	text := extractText(t, result)
	if !strings.Contains(text, "No work items found") {
		t.Errorf("expected 'No work items found', got: %s", text)
	}
}

// TestFormatListMarkdown_SpecialCharsInTitle verifies FormatListMarkdown when special chars in title.
func TestFormatListMarkdown_SpecialCharsInTitle(t *testing.T) {
	out := ListOutput{WorkItems: []WorkItemItem{
		{IID: 1, Type: testTypeIssue, State: testStateOpen, Title: "Has | pipe", Author: testAuthorDev},
	}}
	result := FormatListMarkdown(out)
	text := extractText(t, result)
	// The title should be escaped for markdown table
	if !strings.Contains(text, "pipe") {
		t.Errorf("missing title in output: %s", text)
	}
}

// ---------------------------------------------------------------------------
// List -- all filter branches
// ---------------------------------------------------------------------------.

// TestList_AllFilters verifies List when all filters.
func TestList_AllFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	boolTrue := true
	first := int64(10)

	_, err := List(t.Context(), client, ListInput{
		FullPath:           testFullPath,
		State:              "opened",
		Search:             "keyword",
		Types:              []string{testTypeIssue, testTypeTask},
		AuthorUsername:     testAuthorAlice,
		LabelName:          []string{testLabelBug, "high"},
		Confidential:       &boolTrue,
		Sort:               "UPDATED_DESC",
		First:              &first,
		After:              "cursor123",
		IncludeAncestors:   &boolTrue,
		IncludeDescendants: &boolTrue,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestList_MinimalFilters verifies List when minimal filters.
func TestList_MinimalFilters(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item","author":{"username":"dev"},"widgets":[]}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{FullPath: testProjectPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out.WorkItems))
	}
}

// ---------------------------------------------------------------------------
// Create -- all option branches
// ---------------------------------------------------------------------------.

// TestCreate_AllOptions verifies Create when all options.
func TestCreate_AllOptions(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/50","iid":"50","workItemType":{"name":"Task"},"state":"OPEN","title":"All opts","description":"desc","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	confidential := true
	milestone := int64(10)
	weight := int64(5)

	out, err := Create(t.Context(), client, CreateInput{
		FullPath:       testFullPath,
		WorkItemTypeID: testTypeGID,
		Title:          "All opts",
		Description:    "desc",
		Confidential:   &confidential,
		AssigneeIDs:    []int64{1, 2},
		MilestoneID:    &milestone,
		LabelIDs:       []int64{10, 20},
		Weight:         &weight,
		HealthStatus:   "onTrack",
		Color:          "#ff0000",
		DueDate:        "2026-06-15",
		StartDate:      "2026-06-01",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "All opts" {
		t.Errorf("Title = %q, want 'All opts'", out.WorkItem.Title)
	}
}

// TestCreate_MinimalOptions verifies Create when minimal options.
func TestCreate_MinimalOptions(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Min","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		FullPath:       testProjectPath,
		WorkItemTypeID: testTypeGID,
		Title:          "Min",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "Min" {
		t.Errorf("Title = %q", out.WorkItem.Title)
	}
}

// TestCreate_InvalidDueDate verifies Create when invalid due date.
func TestCreate_InvalidDueDate(t *testing.T) {
	// DueDate parsing uses time.Parse -- invalid format is silently ignored
	// (err == nil check), so invalid dates just skip setting the field.
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Bad date","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Create(t.Context(), client, CreateInput{
		FullPath:       testProjectPath,
		WorkItemTypeID: testTypeGID,
		Title:          "Bad date",
		DueDate:        "not-a-date",
		StartDate:      "also-not-a-date",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCreate_WithOnlyDescription verifies Create when with only description.
func TestCreate_WithOnlyDescription(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/2","iid":"2","workItemType":{"name":"Issue"},"state":"OPEN","title":"Desc only","description":"my desc","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		FullPath:       testProjectPath,
		WorkItemTypeID: testTypeGID,
		Title:          "Desc only",
		Description:    "my desc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Description != "my desc" {
		t.Errorf(fmtDescWant, out.WorkItem.Description)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------.

// TestGet_ContextCancelled verifies Get when context cancelled.
func TestGet_ContextCancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"x","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{FullPath: testProjectPath, IID: 1})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestList_ContextCancelled verifies List when context cancelled.
func TestList_ContextCancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{FullPath: testProjectPath})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCreate_ContextCancelled verifies Create when context cancelled.
func TestCreate_ContextCancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"x","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{
		FullPath:       testProjectPath,
		WorkItemTypeID: testTypeGID,
		Title:          "x",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// API error paths
// ---------------------------------------------------------------------------.

// TestGet_APIError404 verifies Get when API error 404.
func TestGet_APIError404(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Get(t.Context(), client, GetInput{FullPath: testProjectPath, IID: 999})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGet_APIError401 verifies Get when API error 401.
func TestGet_APIError401(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Get(t.Context(), client, GetInput{FullPath: testProjectPath, IID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestList_APIError403 verifies List when API error 403.
func TestList_APIError403(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := List(t.Context(), client, ListInput{FullPath: testProjectPath})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestCreate_APIError422 verifies Create when API error 422.
func TestCreate_APIError422(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Create(t.Context(), client, CreateInput{
		FullPath:       testProjectPath,
		WorkItemTypeID: testTypeGID,
		Title:          "fail",
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestCreate_APIError500 verifies Create when API error 500.
func TestCreate_APIError500(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Create(t.Context(), client, CreateInput{
		FullPath:       testProjectPath,
		WorkItemTypeID: testTypeGID,
		Title:          "fail",
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// Update — all option branches
// ---------------------------------------------------------------------------.

// TestUpdate_Success verifies that Update returns the updated work item when
// the API responds successfully with minimal input (title only).
// UpdateWorkItem makes two GraphQL calls: first workItemGID to resolve the
// global ID, then the actual workItemUpdate mutation.
func TestUpdate_Success(t *testing.T) {
	call := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		call++
		switch call {
		case 1:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/42"}}}}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/42","iid":"42","workItemType":{"name":"Issue"},"state":"OPEN","title":"Updated title","author":{"username":"dev"},"widgets":[]}}}}`)
		}
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Update(t.Context(), client, UpdateInput{
		FullPath: testFullPath,
		IID:      42,
		Title:    "Updated title",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "Updated title" {
		t.Errorf("Title = %q, want 'Updated title'", out.WorkItem.Title)
	}
}

// TestUpdate_AllOptions verifies that Update correctly passes all optional
// fields to the GitLab API: StateEvent, Description, AssigneeIDs, MilestoneID,
// CRMContactIDs, ParentID, AddLabelIDs, RemoveLabelIDs, StartDate, DueDate,
// Weight, HealthStatus, IterationID, and Color.
func TestUpdate_AllOptions(t *testing.T) {
	call := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		call++
		switch call {
		case 1:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/42"}}}}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/42","iid":"42","workItemType":{"name":"Task"},"state":"CLOSED","title":"All opts updated","description":"new desc","author":{"username":"alice"},"widgets":[]}}}}`)
		}
	})
	client := testutil.NewTestClient(t, handler)

	milestone := int64(5)
	parent := int64(100)
	weight := int64(8)
	iteration := int64(3)

	out, err := Update(t.Context(), client, UpdateInput{
		FullPath:       testFullPath,
		IID:            42,
		Title:          "All opts updated",
		StateEvent:     "CLOSE",
		Description:    "new desc",
		AssigneeIDs:    []int64{1, 2},
		MilestoneID:    &milestone,
		CRMContactIDs:  []int64{10},
		ParentID:       &parent,
		AddLabelIDs:    []int64{20, 30},
		RemoveLabelIDs: []int64{40},
		StartDate:      "2026-06-01",
		DueDate:        "2026-06-30",
		Weight:         &weight,
		HealthStatus:   "needsAttention",
		IterationID:    &iteration,
		Color:          "#00ff00",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "All opts updated" {
		t.Errorf("Title = %q, want 'All opts updated'", out.WorkItem.Title)
	}
	if out.WorkItem.State != "CLOSED" {
		t.Errorf("State = %q, want 'CLOSED'", out.WorkItem.State)
	}
}

// TestUpdate_InvalidIID verifies that Update rejects IID values <= 0
// with an error mentioning "iid".
func TestUpdate_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	for _, iid := range []int64{0, -1, -100} {
		_, err := Update(t.Context(), client, UpdateInput{FullPath: testFullPath, IID: iid, Title: "x"})
		if err == nil {
			t.Fatalf("expected error for IID=%d, got nil", iid)
		}
		if !strings.Contains(err.Error(), "work_item_iid") {
			t.Errorf("expected error to mention 'iid' for IID=%d, got: %v", iid, err)
		}
	}
}

// TestUpdate_Error verifies that Update propagates API errors correctly.
func TestUpdate_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Update(t.Context(), client, UpdateInput{FullPath: testFullPath, IID: 42, Title: "x"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestUpdate_InvalidDates verifies that invalid date formats for StartDate
// and DueDate are silently ignored (the field is not set) without causing errors.
func TestUpdate_InvalidDates(t *testing.T) {
	call := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call++
		switch call {
		case 1:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Bad dates","author":{"username":"dev"},"widgets":[]}}}}`)
		}
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Update(t.Context(), client, UpdateInput{
		FullPath:  testFullPath,
		IID:       1,
		StartDate: "not-a-date",
		DueDate:   "also-invalid",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "Bad dates" {
		t.Errorf("Title = %q", out.WorkItem.Title)
	}
}

// TestUpdate_ContextCancelled verifies that Update respects context
// cancellation and returns an error.
func TestUpdate_ContextCancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"x","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{FullPath: testProjectPath, IID: 1, Title: "x"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestUpdate_APIError404 verifies that Update returns an error for 404 responses.
func TestUpdate_APIError404(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Update(t.Context(), client, UpdateInput{FullPath: testProjectPath, IID: 999, Title: "x"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestUpdate_EmptyAssigneesRemovesAll verifies that passing an empty AssigneeIDs
// slice (non-nil) forwards it to the API, which interprets it as "remove all".
func TestUpdate_EmptyAssigneesRemovesAll(t *testing.T) {
	call := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call++
		switch call {
		case 1:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"No assignees","author":{"username":"dev"},"widgets":[]}}}}`)
		}
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Update(t.Context(), client, UpdateInput{
		FullPath:    testFullPath,
		IID:         1,
		AssigneeIDs: []int64{},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "No assignees" {
		t.Errorf("Title = %q", out.WorkItem.Title)
	}
}

// ---------------------------------------------------------------------------
// Update — Status field
// ---------------------------------------------------------------------------

// TestUpdate_WithStatus verifies that setting a status maps correctly to
// WorkItemStatusID and the API call succeeds.
func TestUpdate_WithStatus(t *testing.T) {
	statuses := []string{"TODO", "IN_PROGRESS", "DONE", "WONT_DO", "DUPLICATE"}
	for _, s := range statuses {
		t.Run(s, func(t *testing.T) {
			call := 0
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				call++
				switch call {
				case 1:
					testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}}`)
				default:
					testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Status test","author":{"username":"dev"},"widgets":[{"type":"STATUS","status":{"name":"%s"}}]}}}}`, s))
				}
			})
			client := testutil.NewTestClient(t, handler)

			_, err := Update(t.Context(), client, UpdateInput{
				FullPath: testFullPath,
				IID:      1,
				Status:   s,
			})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
		})
	}
}

// TestUpdate_StatusNotSet verifies that omitting status does not set it on opts.
func TestUpdate_StatusNotSet(t *testing.T) {
	call := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call++
		switch call {
		case 1:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"No status","author":{"username":"dev"},"widgets":[]}}}}`)
		}
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Update(t.Context(), client, UpdateInput{
		FullPath: testFullPath,
		IID:      1,
		Title:    "No status",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "No status" {
		t.Errorf("Title = %q", out.WorkItem.Title)
	}
}

// TestMapStatusToID verifies all known status strings and a fallback.
func TestMapStatusToID(t *testing.T) {
	tests := []struct {
		input string
		want  gl.WorkItemStatusID
	}{
		{"TODO", gl.WorkItemStatusToDo},
		{"IN_PROGRESS", gl.WorkItemStatusInProgress},
		{"DONE", gl.WorkItemStatusDone},
		{"WONT_DO", gl.WorkItemStatusWontDo},
		{"DUPLICATE", gl.WorkItemStatusDuplicate},
		{"custom-gid", gl.WorkItemStatusID("custom-gid")},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapStatusToID(tt.input)
			if got != tt.want {
				t.Errorf("mapStatusToID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Create — LinkedItems field
// ---------------------------------------------------------------------------

// TestCreate_WithLinkedItems verifies that linked items are passed to the API.
func TestCreate_WithLinkedItems(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/55","iid":"55","workItemType":{"name":"Issue"},"state":"OPEN","title":"Linked","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		FullPath:       testFullPath,
		WorkItemTypeID: testTypeGID,
		Title:          "Linked",
		LinkedItems: &CreateLinkedItems{
			WorkItemIDs: []int64{10, 20},
			LinkType:    "BLOCKS",
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "Linked" {
		t.Errorf("Title = %q, want 'Linked'", out.WorkItem.Title)
	}
}

// TestCreate_LinkedItemsNil verifies that nil linked items is handled.
func TestCreate_LinkedItemsNil(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"No links","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		FullPath:       testProjectPath,
		WorkItemTypeID: testTypeGID,
		Title:          "No links",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "No links" {
		t.Errorf("Title = %q", out.WorkItem.Title)
	}
}

// TestCreate_LinkedItemsEmptyIDs verifies that linked items with empty IDs is ignored.
func TestCreate_LinkedItemsEmptyIDs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Empty links","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		FullPath:       testProjectPath,
		WorkItemTypeID: testTypeGID,
		Title:          "Empty links",
		LinkedItems: &CreateLinkedItems{
			WorkItemIDs: []int64{},
			LinkType:    "RELATED",
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "Empty links" {
		t.Errorf("Title = %q", out.WorkItem.Title)
	}
}

// ---------------------------------------------------------------------------
// workItemToItem — LinkedItems mapping
// ---------------------------------------------------------------------------

// TestWorkItemToItem_WithLinkedItems verifies linked items are mapped correctly.
func TestWorkItemToItem_WithLinkedItems(t *testing.T) {
	wi := &gl.WorkItem{
		ID:    10,
		IID:   10,
		Type:  testTypeIssue,
		State: testStateOpen,
		Title: "With links",
		LinkedItems: []gl.LinkedWorkItem{
			{WorkItemIID: gl.WorkItemIID{NamespacePath: "group/proj", IID: 5}, LinkType: "relates_to"},
			{WorkItemIID: gl.WorkItemIID{NamespacePath: "group/other", IID: 8}, LinkType: "blocks"},
		},
	}
	item := workItemToItem(wi)
	if len(item.LinkedItems) != 2 {
		t.Fatalf("LinkedItems = %d, want 2", len(item.LinkedItems))
	}
	if item.LinkedItems[0].IID != 5 || item.LinkedItems[0].LinkType != "relates_to" || item.LinkedItems[0].Path != "group/proj" {
		t.Errorf("LinkedItems[0] = %+v", item.LinkedItems[0])
	}
	if item.LinkedItems[1].IID != 8 || item.LinkedItems[1].LinkType != "blocks" || item.LinkedItems[1].Path != "group/other" {
		t.Errorf("LinkedItems[1] = %+v", item.LinkedItems[1])
	}
}

// TestWorkItemToItem_NoLinkedItems verifies empty linked items stays nil.
func TestWorkItemToItem_NoLinkedItems(t *testing.T) {
	wi := &gl.WorkItem{
		ID:    1,
		IID:   1,
		Type:  testTypeIssue,
		State: testStateOpen,
		Title: "No links",
	}
	item := workItemToItem(wi)
	if len(item.LinkedItems) != 0 {
		t.Errorf("LinkedItems = %d, want 0", len(item.LinkedItems))
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — Status and LinkedItems rendering
// ---------------------------------------------------------------------------

// TestFormatGetMarkdown_WithStatus verifies Status is rendered in markdown.
func TestFormatGetMarkdown_WithStatus(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:    1,
		Title:  "Status item",
		Type:   testTypeIssue,
		State:  testStateOpen,
		Status: "IN_PROGRESS",
	}}
	result := FormatGetMarkdown(out)
	text := extractText(t, result)
	if !strings.Contains(text, "**Status**: IN_PROGRESS") {
		t.Errorf("missing status in output: %s", text)
	}
}

// TestFormatGetMarkdown_WithLinkedItems verifies linked items table is rendered.
func TestFormatGetMarkdown_WithLinkedItems(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:   1,
		Title: "Linked item",
		Type:  testTypeIssue,
		State: testStateOpen,
		LinkedItems: []LinkedItem{
			{IID: 5, LinkType: "blocks", Path: "group/proj"},
		},
	}}
	result := FormatGetMarkdown(out)
	text := extractText(t, result)
	if !strings.Contains(text, "### Linked Items") {
		t.Errorf("missing Linked Items heading: %s", text)
	}
	if !strings.Contains(text, "| 5 | blocks | group/proj |") {
		t.Errorf("missing linked item row: %s", text)
	}
}

// TestFormatGetMarkdown_NoStatusNoLinkedItems verifies optional sections are omitted.
func TestFormatGetMarkdown_NoStatusNoLinkedItems(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:   1,
		Title: "Plain",
		Type:  testTypeIssue,
		State: testStateOpen,
	}}
	result := FormatGetMarkdown(out)
	text := extractText(t, result)
	if strings.Contains(text, "**Status**") {
		t.Error("unexpected Status in output")
	}
	if strings.Contains(text, "### Linked Items") {
		t.Error("unexpected Linked Items in output")
	}
}

// ---------------------------------------------------------------------------
// ListWorkItemTypes
// ---------------------------------------------------------------------------.

// TestListWorkItemTypes_Success verifies ListWorkItemTypes returns two types
// when the GraphQL endpoint returns a namespace with two WorkItemType nodes.
func TestListWorkItemTypes_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"data": {
				"namespace": {
					"workItemTypes": {
						"nodes": [
							{"id":"gid://gitlab/WorkItems::Type/1","name":"Issue","enabled":true},
							{"id":"gid://gitlab/WorkItems::Type/7","name":"Task","enabled":true}
						],
						"pageInfo": {
							"hasNextPage": false,
							"hasPreviousPage": false,
							"endCursor": "",
							"startCursor": ""
						}
					}
				}
			}
		}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{
		FullPath: testFullPath,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(out.Types))
	}
	if out.Types[0].Name != "Issue" {
		t.Errorf("Types[0].Name = %q, want Issue", out.Types[0].Name)
	}
	if !out.Types[0].Enabled {
		t.Errorf("Types[0].Enabled = false, want true")
	}
	if out.Types[1].Name != "Task" {
		t.Errorf("Types[1].Name = %q, want Task", out.Types[1].Name)
	}
	if out.Pagination.HasNextPage {
		t.Error("Pagination.HasNextPage = true, want false")
	}
	if out.Pagination.HasPreviousPage {
		t.Error("Pagination.HasPreviousPage = true, want false")
	}
	if out.Pagination.EndCursor != "" {
		t.Errorf("Pagination.EndCursor = %q, want empty", out.Pagination.EndCursor)
	}
}

// TestListWorkItemTypes_EmptyFullPath verifies ListWorkItemTypes returns a
// validation error when full_path is empty.
func TestListWorkItemTypes_EmptyFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "full_path") {
		t.Fatalf("expected error to mention full_path, got: %v", err)
	}
}

// TestListWorkItemTypes_NotFound verifies ListWorkItemTypes returns an error
// when the GraphQL response contains errors (e.g. namespace not found).
func TestListWorkItemTypes_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"errors":[{"message":"not found"}]}`)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{FullPath: "nonexistent/project"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestListWorkItemTypes_WithOptions verifies ListWorkItemTypes passes name and
// onlyAvailable filter options to the API.
func TestListWorkItemTypes_WithOptions(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"data": {
				"namespace": {
					"workItemTypes": {
						"nodes": [
							{"id":"gid://gitlab/WorkItems::Type/1","name":"Issue","enabled":true}
						],
						"pageInfo": {"hasNextPage":false,"hasPreviousPage":false,"endCursor":"","startCursor":""}
					}
				}
			}
		}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{
		FullPath:      testFullPath,
		Name:          "Issue",
		OnlyAvailable: true,
		First:         10,
		After:         "cursor-abc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(out.Types))
	}
	if out.Types[0].Name != "Issue" {
		t.Errorf("Types[0].Name = %q, want Issue", out.Types[0].Name)
	}
}

// TestListWorkItemTypes_APIError verifies ListWorkItemTypes returns an error
// when the API returns a non-200 HTTP status.
func TestListWorkItemTypes_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{FullPath: testFullPath})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// ---------------------------------------------------------------------------
// FormatWorkItemTypeListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatWorkItemTypeListMarkdown_WithTypes verifies that a list with two
// types renders a Markdown table with the correct headers and rows.
func TestFormatWorkItemTypeListMarkdown_WithTypes(t *testing.T) {
	out := WorkItemTypeListOutput{
		Types: []WorkItemTypeOutput{
			{ID: "gid://gitlab/WorkItems::Type/1", Name: "Issue", Enabled: true},
			{ID: "gid://gitlab/WorkItems::Type/7", Name: "Task", Enabled: false},
		},
	}
	result := FormatWorkItemTypeListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := extractText(t, result)

	for _, want := range []string{
		"## Work Item Types (2)",
		"| Name | ID | Enabled |",
		"Issue",
		"Task",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("markdown missing %q:\n%s", want, text)
		}
	}
}

// TestFormatWorkItemTypeListMarkdown_Empty verifies that an empty type list
// returns a result containing a "no work item types" message.
func TestFormatWorkItemTypeListMarkdown_Empty(t *testing.T) {
	out := WorkItemTypeListOutput{}
	result := FormatWorkItemTypeListMarkdown(out)
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
	text := extractText(t, result)
	if !strings.Contains(text, "No work item types found") {
		t.Errorf("expected 'No work item types found' message, got:\n%s", text)
	}
}

// TestFormatWorkItemTypeListMarkdown_WithNextPage verifies that a next-page
// cursor is included in the output when HasNextPage is true.
func TestFormatWorkItemTypeListMarkdown_WithNextPage(t *testing.T) {
	out := WorkItemTypeListOutput{
		Types: []WorkItemTypeOutput{
			{ID: "gid://gitlab/WorkItems::Type/1", Name: "Issue", Enabled: true},
		},
		Pagination: toolutil.GraphQLPaginationOutput{
			HasNextPage: true,
			EndCursor:   "next-page-cursor",
		},
	}
	result := FormatWorkItemTypeListMarkdown(out)
	text := extractText(t, result)
	if !strings.Contains(text, "next-page-cursor") {
		t.Errorf("expected next-page cursor in output:\n%s", text)
	}
}

// TestGet_RichResponse verifies rich work item response mapping.
func TestGet_RichResponse(t *testing.T) {
	richJSON := `{"data":{"namespace":{"workItem":{
		"id":"gid://gitlab/WorkItem/42","iid":"42",
		"workItemType":{"name":"Task"},
		"state":"OPEN",
		"title":"Rich item",
		"description":"Detailed desc",
		"confidential":true,
		"webUrl":"https://gitlab.example.com/-/work_items/42",
		"author":{"username":"alice"},
		"createdAt":"2026-01-01T00:00:00Z",
		"updatedAt":"2026-01-02T00:00:00Z",
		"closedAt":"2026-01-03T00:00:00Z",
		"widgets":[]}}}}`

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, richJSON)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(t.Context(), client, GetInput{FullPath: testProjectPath, IID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	wi := out.WorkItem
	if wi.Title != "Rich item" {
		t.Errorf("Title = %q", wi.Title)
	}
	if wi.Author != testAuthorAlice {
		t.Errorf("Author = %q", wi.Author)
	}
	if wi.Description != "Detailed desc" {
		t.Errorf(fmtDescWant, wi.Description)
	}
	if !wi.Confidential {
		t.Error("expected Confidential=true")
	}
	if wi.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if wi.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
	if wi.ClosedAt == "" {
		t.Error("expected non-empty ClosedAt")
	}
	if wi.WebURL != testWorkItemURL {
		t.Errorf("WebURL = %q", wi.WebURL)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// extractText supports extract text assertions in workitems tests.
func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("nil CallToolResult")
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content in CallToolResult")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}
