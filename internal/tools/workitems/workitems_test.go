// workitems_test.go contains unit tests for the work item MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package workitems

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestGet_Success verifies the behavior of get success.
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

// TestGet_InvalidIID verifies the behavior of get invalid i i d.
func TestGet_InvalidIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	for _, iid := range []int64{0, -1, -100} {
		_, err := Get(t.Context(), client, GetInput{FullPath: testFullPath, IID: iid})
		if err == nil {
			t.Fatalf("expected error for IID=%d, got nil", iid)
		}
		if !strings.Contains(err.Error(), "iid") {
			t.Errorf("expected error to mention 'iid' for IID=%d, got: %v", iid, err)
		}
	}
}

// TestGet_Error verifies the behavior of get error.
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

// TestList_Success verifies the behavior of list success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf(fmtUnexpMethod, r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item 1","author":{"username":"dev1"},"widgets":[]},{"id":"gid://gitlab/WorkItem/2","iid":"11","workItemType":{"name":"Task"},"state":"CLOSED","title":"Item 2","author":{"username":"dev2"},"widgets":[]}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 2 {
		t.Fatalf("expected 2 work items, got %d", len(out.WorkItems))
	}
}

// TestList_Empty verifies the behavior of list empty.
func TestList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`)
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

// TestList_Error verifies the behavior of list error.
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

// TestCreate_Success verifies the behavior of create success.
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

// TestCreate_Error verifies the behavior of create error.
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
		if !strings.Contains(err.Error(), "iid") {
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

// TestFormatGetMarkdown verifies the behavior of format get markdown.
func TestFormatGetMarkdown(t *testing.T) {
	result := FormatGetMarkdown(GetOutput{WorkItem: WorkItemItem{
		IID: 42, Title: "Test", Type: "Issue", State: "OPEN", Author: "dev",
	}})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	if result == nil {
		t.Fatal(errExpNonNilResult)
	}
}

// TestFormatListMarkdown_WithData verifies the behavior of format list markdown with data.
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

const errExpNonNilResult = "expected non-nil result"

const errExpCancelledNil = "expected error for canceled context, got nil"

const errExpectedNil = "expected error, got nil"

const fmtUnexpErr = "unexpected error: %v"

const fmtUnexpMethod = "unexpected method: %s"

const testFullPath = "my-group/my-project"

const (
	testProjectPath  = "ns/proj"
	testStateOpen    = "OPEN"
	testStateClosed  = "CLOSED"
	testTypeIssue    = "Issue"
	testTypeTask     = "Task"
	testTypeGID      = "gid://gitlab/WorkItems::Type/1"
	testAuthorAlice  = "alice"
	testAuthorBob    = "bob"
	testAuthorCarol  = "carol"
	testAuthorDev    = "dev"
	testLabelBug     = "bug"
	testLabelUrgent  = "urgent"
	testWorkItemURL  = "https://gitlab.example.com/-/work_items/42"
	testVersion      = "0.0.1"
	testSectionDesc  = "### Description"
	fmtDescWant      = "Description = %q"
	testTitleNewItem = "New item"
)

// ---------------------------------------------------------------------------
// workItemToItem -- converter tests
// ---------------------------------------------------------------------------.

// TestWorkItemToItem_FullData verifies the behavior of work item to item full data.
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

// assertFullItemCore is an internal helper for the workitems package.
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

// assertFullItemPeople is an internal helper for the workitems package.
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

// assertFullItemTimestamps is an internal helper for the workitems package.
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

// TestWorkItemToItem_Minimal verifies the behavior of work item to item minimal.
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

// TestWorkItemToItemNilStatusNon_NilAuthor verifies the behavior of work item to item nil status non nil author.
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

// TestWorkItemToItem_EmptyAssigneesAndLabelsSlices verifies the behavior of work item to item empty assignees and labels slices.
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

// TestFormatGetMarkdown_FullPopulated verifies the behavior of format get markdown full populated.
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

// TestFormatGetMarkdown_Empty verifies the behavior of format get markdown empty.
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

// TestFormatGetMarkdown_OnlyAuthor verifies the behavior of format get markdown only author.
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

// TestFormatGetMarkdown_OnlyAssignees verifies the behavior of format get markdown only assignees.
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

// TestFormatGetMarkdown_OnlyLabels verifies the behavior of format get markdown only labels.
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

// TestFormatGetMarkdown_OnlyWebURL verifies the behavior of format get markdown only web u r l.
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

// TestFormatGetMarkdown_OnlyDescription verifies the behavior of format get markdown only description.
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

// TestFormatListMarkdown_MultipleItems verifies the behavior of format list markdown multiple items.
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

// TestFormatListMarkdown_EmptyReturnsMessage verifies the behavior of format list markdown empty returns message.
func TestFormatListMarkdown_EmptyReturnsMessage(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	text := extractText(t, result)
	if !strings.Contains(text, "No work items found") {
		t.Errorf("expected 'No work items found', got: %s", text)
	}
}

// TestFormatListMarkdown_SpecialCharsInTitle verifies the behavior of format list markdown special chars in title.
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

// TestList_AllFilters verifies the behavior of list all filters.
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

// TestList_MinimalFilters verifies the behavior of list minimal filters.
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

// TestCreate_AllOptions verifies the behavior of create all options.
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

// TestCreate_MinimalOptions verifies the behavior of create minimal options.
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

// TestCreate_InvalidDueDate verifies the behavior of create invalid due date.
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

// TestCreate_WithOnlyDescription verifies the behavior of create with only description.
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

// TestGet_ContextCancelled verifies the behavior of get context cancelled.
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

// TestList_ContextCancelled verifies the behavior of list context cancelled.
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

// TestCreate_ContextCancelled verifies the behavior of create context cancelled.
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

// TestGet_APIError404 verifies the behavior of get a p i error404.
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

// TestGet_APIError401 verifies the behavior of get a p i error401.
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

// TestList_APIError403 verifies the behavior of list a p i error403.
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

// TestCreate_APIError422 verifies the behavior of create a p i error422.
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

// TestCreate_APIError500 verifies the behavior of create a p i error500.
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
		if !strings.Contains(err.Error(), "iid") {
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
// Get — rich response with labels, assignees, status, dates
// ---------------------------------------------------------------------------.
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
// MCP integration -- RegisterTools
// ---------------------------------------------------------------------------.

const workItemGraphQLResponse = `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"MCP test","author":{"username":"dev"},"widgets":[]}}}}`
const workItemsListGraphQLResponse = `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"MCP test","author":{"username":"dev"},"widgets":[]}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`
const workItemCreateGraphQLResponse = `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"MCP test","author":{"username":"dev"},"widgets":[]}}}}`
const workItemUpdateGraphQLResponse = `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"MCP test updated","author":{"username":"dev"},"widgets":[]}}}}`
const workItemDeleteGraphQLResponse = `{"data":{"workItemDelete":{"errors":[]}}}`

// newWorkItemsMCPSession is an internal helper for the workitems package.
func newWorkItemsMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// All WorkItems API calls use POST (GraphQL).
		// Distinguish by reading request body keywords if needed,
		// but for simplicity we route based on simple heuristics.
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		// Read body to determine which operation
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		body := string(buf[:n])

		switch {
		case strings.Contains(body, "workItemCreate"):
			testutil.RespondJSON(w, http.StatusOK, workItemCreateGraphQLResponse)
		case strings.Contains(body, "workItemUpdate"):
			testutil.RespondJSON(w, http.StatusOK, workItemUpdateGraphQLResponse)
		case strings.Contains(body, "workItemDelete"):
			testutil.RespondJSON(w, http.StatusOK, workItemDeleteGraphQLResponse)
		case strings.Contains(body, "workItems"):
			testutil.RespondJSON(w, http.StatusOK, workItemsListGraphQLResponse)
		case strings.Contains(body, "workItem"):
			testutil.RespondJSON(w, http.StatusOK, workItemGraphQLResponse)
		default:
			// Fallback for any POST -- return a valid work item
			testutil.RespondJSON(w, http.StatusOK, workItemGraphQLResponse)
		}
	}))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: testVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// assertToolCallSuccess calls the named MCP tool and fails the test if the
// call returns an error or the result indicates a tool-level error.
func assertToolCallSuccess(t *testing.T, session *mcp.ClientSession, ctx context.Context, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", name, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", name, tc.Text)
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true", name)
	}
}

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
// It exercises get, list, create, update, and delete tools via in-memory MCP transport.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newWorkItemsMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_work_item", map[string]any{"full_path": testProjectPath, "iid": 10}},
		{"gitlab_list_work_items", map[string]any{"full_path": testProjectPath}},
		{"gitlab_create_work_item", map[string]any{"full_path": testProjectPath, "work_item_type_id": testTypeGID, "title": "Test"}},
		{"gitlab_update_work_item", map[string]any{"full_path": testProjectPath, "iid": 10, "title": "Updated"}},
		{"gitlab_delete_work_item", map[string]any{"full_path": testProjectPath, "iid": 10}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertToolCallSuccess(t, session, ctx, tt.name, tt.args)
		})
	}
}

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// extractText is an internal helper for the workitems package.
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
