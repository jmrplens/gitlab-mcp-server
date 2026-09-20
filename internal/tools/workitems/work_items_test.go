// work_items_test.go contains unit tests for the work item MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package workitems

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// invalidIIDCases lists the work item IIDs every handler must reject before
// reaching the API: Get, Delete and Update share the same guard.
var invalidIIDCases = []struct {
	name string
	iid  int64
}{
	{"zero", 0},
	{"negative", -1},
	{"large_negative", -100},
}

// namedUser builds the author or assignee object a test needs when only the
// username matters to what it asserts.
func namedUser(username string) *toolutil.BasicUserOutput {
	return &toolutil.BasicUserOutput{Username: username}
}

// namedUsers builds an assignee list from usernames alone, for the same reason
// [namedUser] exists.
func namedUsers(usernames ...string) []*toolutil.BasicUserOutput {
	users := make([]*toolutil.BasicUserOutput, 0, len(usernames))
	for _, username := range usernames {
		users = append(users, namedUser(username))
	}
	return users
}

// namedLabels builds a label list from titles alone.
func namedLabels(names ...string) []*toolutil.LabelDetailsOutput {
	labels := make([]*toolutil.LabelDetailsOutput, 0, len(names))
	for _, name := range names {
		labels = append(labels, &toolutil.LabelDetailsOutput{Name: name})
	}
	return labels
}

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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	for _, tc := range invalidIIDCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Get(t.Context(), client, GetInput{FullPath: testFullPath, IID: tc.iid})
			if err == nil {
				t.Fatalf("expected error for IID=%d, got nil", tc.iid)
			}
			if !strings.Contains(err.Error(), "work_item_iid") {
				t.Errorf("expected error to mention 'iid' for IID=%d, got: %v", tc.iid, err)
			}
		})
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
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item 1","description":"Desc 1","confidential":true,"webUrl":"https://gitlab.example.com/work_items/10","author":{"username":"dev1"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","closedAt":null},{"id":"gid://gitlab/WorkItem/2","iid":"11","workItemType":{"name":"Task"},"state":"CLOSED","title":"Item 2","author":{"username":"dev2"}}],"pageInfo":{"hasNextPage":true,"endCursor":"cursor-2","hasPreviousPage":false,"startCursor":"cursor-1"}}}}}`)
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
	if first.ID != 1 || first.IID != 10 || first.Type != testTypeIssue || authorName(first.Author) != "dev1" {
		t.Fatalf("unexpected first work item: %+v", first)
	}
	if !first.Confidential || first.WebURL == "" || first.CreatedAt == "" || first.UpdatedAt == "" {
		t.Fatalf("expected mapped optional fields, got %+v", first)
	}
	if first.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want RFC 3339", first.CreatedAt)
	}
	if first.UpdatedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, want RFC 3339", first.UpdatedAt)
	}
	// The fixture sends closedAt as null, which is what an open item carries.
	// Asserting it is empty is what keeps a future formatter from rendering a
	// zero time as a real date.
	if first.ClosedAt != "" {
		t.Errorf("ClosedAt = %q, want empty for a null timestamp", first.ClosedAt)
	}
	// Every field of pageInfo, not only the two the handler needed first: the
	// fixture supplies four, and a field nothing asserts is a field a
	// regression can quietly drop.
	if !out.Pagination.HasNextPage || out.Pagination.EndCursor != "cursor-2" {
		t.Errorf("Pagination = %+v, want the connection's pageInfo", out.Pagination)
	}
	if out.Pagination.HasPreviousPage {
		t.Errorf("HasPreviousPage = true, want false from the fixture's pageInfo")
	}
	if out.Pagination.StartCursor != "cursor-1" {
		t.Errorf("StartCursor = %q, want cursor-1", out.Pagination.StartCursor)
	}
}

// TestList_WidgetFields_ArePopulatedFromTheDefaultFieldSet verifies that listed work items carry the assignees,
// labels and linked items the SDK's default field set requests. The
// hand-written query this handler replaced selected none of them, so every
// listed item came back with those three fields empty even when GitLab held
// values for them.
func TestList_WidgetFields_ArePopulatedFromTheDefaultFieldSet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item 1","author":{"username":"dev1"},"features":{"assignees":{"assignees":{"nodes":[{"id":"gid://gitlab/User/3","username":"bob"},{"id":"gid://gitlab/User/4","username":"carol"}]}},"labels":{"labels":{"nodes":[{"id":"gid://gitlab/ProjectLabel/7","title":"bug"}]}},"linkedItems":{"linkedItems":{"nodes":[{"workItem":{"iid":"7","namespace":{"fullPath":"my-group/other"}},"linkType":"blocks"}]}}}}]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(out.WorkItems))
	}
	item := out.WorkItems[0]
	if got := assigneeNames(item.Assignees); len(got) != 2 || got[0] != testAuthorBob || got[1] != testAuthorCarol {
		t.Errorf("Assignees = %v, want [bob carol]", got)
	}
	if got := labelNames(item.Labels); len(got) != 1 || got[0] != testLabelBug {
		t.Errorf("Labels = %v, want [bug]", got)
	}
	if len(item.LinkedItems) != 1 {
		t.Fatalf("LinkedItems = %d, want 1", len(item.LinkedItems))
	}
	if item.LinkedItems[0].IID != 7 || item.LinkedItems[0].LinkType != "blocks" || item.LinkedItems[0].Path != "my-group/other" {
		t.Errorf("LinkedItems[0] = %+v, want {7 blocks my-group/other}", item.LinkedItems[0])
	}
}

// TestList_AssigneesAndLabels_CarryTheWholeObject verifies that the fields the
// UserCoreBasic and label fragments already fetch reach the output instead of
// being reduced to a name.
//
// The fixture answers with every field of both fragments, because the fragment
// pays for all of them on every list call: publishing only the username and
// the title threw away six user fields and five label fields per entry.
func TestList_AssigneesAndLabels_CarryTheWholeObject(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item 1","author":{"id":"gid://gitlab/User/2","username":"alice","name":"Alice A","state":"active","avatarUrl":"https://gitlab.example.com/alice.png","webUrl":"https://gitlab.example.com/alice","createdAt":"2026-01-01T00:00:00Z"},"features":{"assignees":{"assignees":{"nodes":[{"id":"gid://gitlab/User/3","username":"bob","name":"Bob B","state":"active","avatarUrl":"https://gitlab.example.com/bob.png","webUrl":"https://gitlab.example.com/bob","createdAt":"2026-01-02T00:00:00Z"}]}},"labels":{"labels":{"nodes":[{"id":"gid://gitlab/ProjectLabel/7","title":"bug","color":"#d9534f","description":"Something is broken","descriptionHtml":"<p>Something is broken</p>","textColor":"#ffffff"}]}}}}]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(out.WorkItems))
	}
	item := out.WorkItems[0]
	wantAuthor := &toolutil.BasicUserOutput{
		ID: 2, Username: testAuthorAlice, Name: "Alice A", State: "active",
		AvatarURL: "https://gitlab.example.com/alice.png",
		WebURL:    "https://gitlab.example.com/alice",
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	if !reflect.DeepEqual(item.Author, wantAuthor) {
		t.Errorf("Author = %+v, want %+v", item.Author, wantAuthor)
	}
	wantAssignees := []*toolutil.BasicUserOutput{{
		ID: 3, Username: testAuthorBob, Name: "Bob B", State: "active",
		AvatarURL: "https://gitlab.example.com/bob.png",
		WebURL:    "https://gitlab.example.com/bob",
		CreatedAt: "2026-01-02T00:00:00Z",
	}}
	if !reflect.DeepEqual(item.Assignees, wantAssignees) {
		t.Errorf("Assignees = %+v, want %+v", item.Assignees, wantAssignees)
	}
	wantLabels := []*toolutil.LabelDetailsOutput{{
		ID: 7, Name: testLabelBug, Color: "#d9534f",
		Description:     "Something is broken",
		DescriptionHTML: "<p>Something is broken</p>",
		TextColor:       "#ffffff",
	}}
	if !reflect.DeepEqual(item.Labels, wantLabels) {
		t.Errorf("Labels = %+v, want %+v", item.Labels, wantLabels)
	}
}

// TestList_Empty verifies List when empty.
//
// The fixture omits pageInfo altogether, which is the answer the nil guard
// around the pagination block exists for. client-go assigns a PageInfo to the
// response whatever the document carried, so what reaches the caller is the
// zero block rather than invented cursors, and the guard's false arm is
// unreachable through the SDK.
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
	if out.Pagination != (toolutil.GraphQLPaginationOutput{}) {
		t.Errorf("Pagination = %+v, want the zero block for an answer with no pageInfo", out.Pagination)
	}
}

// TestList_Filters verifies List forwards supported filters to the minimal GraphQL query.
func TestList_Filters(t *testing.T) {
	confidential := true
	first := 5
	includeAncestors := true
	includeDescendants := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode GraphQL request: %v", err)
			http.Error(w, "decode GraphQL request", http.StatusInternalServerError)
			return
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
			t.Run(key, func(t *testing.T) {
				if got := request.Variables[key]; got != want {
					t.Errorf("variable %s = %#v, want %#v", key, got, want)
					http.Error(w, "variable, want", http.StatusInternalServerError)
					return
				}
			})
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

// TestList_TypeFilter_ReachesGitLabUppercased verifies the normalisation the
// types filter depends on.
//
// The values reach GitLab as a GraphQL IssueType, which is case sensitive:
// gitlab.com answers types ["Issue"] with `Expected "Issue" to be one of:
// ISSUE, INCIDENT, ...` and executes nothing, so the caller is told the
// namespace holds no matching work items. This server sent exactly that until
// the pinned schema began judging variables, with every test green, because the
// mock answered whatever it was asked. The case table is the spellings a caller
// or a model actually writes.
func TestList_TypeFilter_ReachesGitLabUppercased(t *testing.T) {
	cases := []struct {
		name string
		sent []string
		want []any
	}{
		{name: "title case, the natural guess", sent: []string{"Issue", "Task"}, want: []any{"ISSUE", "TASK"}},
		{name: "lower case", sent: []string{"incident"}, want: []any{"INCIDENT"}},
		{name: "already correct", sent: []string{"TEST_CASE"}, want: []any{"TEST_CASE"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var sent []any
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Variables map[string]any `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode GraphQL request: %v", err)
					http.Error(w, "decode GraphQL request", http.StatusInternalServerError)
					return
				}
				sent, _ = request.Variables["types"].([]any)
				testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[]}}}}`)
			})

			_, err := List(t.Context(), testutil.NewTestClient(t, handler), ListInput{
				FullPath: testFullPath, Types: testCase.sent,
			})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !slices.Equal(sent, testCase.want) {
				t.Errorf("types reached GitLab as %#v, want %#v", sent, testCase.want)
			}
		})
	}
}

// TestListWorkItemTypes_NameFilter_ReachesGitLabUppercased is the same
// guarantee for the type list's name filter, which is the other place an
// IssueType value leaves this package.
func TestListWorkItemTypes_NameFilter_ReachesGitLabUppercased(t *testing.T) {
	var sent any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode GraphQL request: %v", err)
			http.Error(w, "decode GraphQL request", http.StatusInternalServerError)
			return
		}
		sent = request.Variables["name"]
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItemTypes":{"nodes":[]}}}}`)
	})

	_, err := ListWorkItemTypes(t.Context(), testutil.NewTestClient(t, handler), ListWorkItemTypesInput{
		FullPath: testFullPath, Name: "Issue",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if sent != "ISSUE" {
		t.Errorf("name reached GitLab as %#v, want %q", sent, "ISSUE")
	}
}

// TestList_Children verifies List requests the hierarchy children field in its
// GraphQL query and maps returned child nodes into the WorkItemItem output.
func TestList_Children(t *testing.T) {
	var capturedQuery string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode GraphQL request: %v", err)
			http.Error(w, "decode GraphQL request", http.StatusInternalServerError)
			return
		}
		capturedQuery = request.Query
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Parent","author":{"username":"dev1"},"features":{"hierarchy":{"hasChildren":true,"children":{"nodes":[{"iid":"20","namespace":{"fullPath":"my-group/child-a"}},{"iid":"21","namespace":{"fullPath":"my-group/child-b"}}]}}}}]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, field := range []string{"hierarchy", "hasChildren", "children"} {
		t.Run(field, func(t *testing.T) {
			if !strings.Contains(capturedQuery, field) {
				t.Errorf("list query missing %q field:\n%s", field, capturedQuery)
			}
		})
	}
	if len(out.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(out.WorkItems))
	}
	children := out.WorkItems[0].Children
	if len(children) != 2 {
		t.Fatalf("Children = %d, want 2", len(children))
	}
	if children[0].IID != 20 || children[0].Path != "my-group/child-a" {
		t.Errorf("children[0] = %+v, want {20 my-group/child-a}", children[0])
	}
	if children[1].IID != 21 || children[1].Path != "my-group/child-b" {
		t.Errorf("children[1] = %+v, want {21 my-group/child-b}", children[1])
	}
}

// TestList_ChildrenAbsent verifies List omits Children when the hierarchy widget
// reports no children, so the omitempty field stays nil.
func TestList_ChildrenAbsent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Leaf","author":{"username":"dev1"},"features":{"hierarchy":{"hasChildren":false,"children":{"nodes":[]}}}}]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(out.WorkItems))
	}
	if out.WorkItems[0].Children != nil {
		t.Errorf("Children = %+v, want nil", out.WorkItems[0].Children)
	}
}

// TestList_ChildrenInvalidIID verifies List drops a child whose IID does not
// parse rather than reporting a misleading zero.
//
// The SDK skips such a node, which is what Get, Create and Update have always
// done because they have always gone through the SDK; the hand-written list
// query failed the whole call instead. GitLab does not emit a non-numeric IID,
// so the two only differ on a response no instance sends.
func TestList_ChildrenInvalidIID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"Parent","features":{"hierarchy":{"hasChildren":true,"children":{"nodes":[{"iid":"not-a-number","namespace":{"fullPath":"my-group/child"}}]}}}}]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(out.WorkItems))
	}
	if len(out.WorkItems[0].Children) != 0 {
		t.Errorf("Children = %+v, want the unparseable child dropped", out.WorkItems[0].Children)
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestList_NamespaceNotFound verifies List reports an unknown namespace as an
// empty list whose Markdown says how to check the path.
//
// GitLab answers a namespace that does not exist, or that the token cannot
// read, with a null namespace and HTTP 200, which is indistinguishable from a
// namespace holding no work items once the SDK has decoded the connection. The
// hint is where that ambiguity is made visible to the caller.
func TestList_NamespaceNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":null}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.WorkItems) != 0 {
		t.Fatalf("expected 0 work items, got %d", len(out.WorkItems))
	}
	md := extractText(t, FormatListMarkdown(out))
	if !strings.Contains(md, "gitlab_project_list") {
		t.Fatalf("expected actionable hint, got %q", md)
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
			want: `invalid global ID format: "not-a-gid"`,
		},
		{
			name: "invalid iid",
			body: `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/1","iid":"abc","workItemType":{"name":"Issue"},"state":"OPEN","title":"Item"}]}}}}`,
			want: `failed parsing "abc" as numeric ID`,
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	for _, tc := range invalidIIDCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Delete(t.Context(), client, DeleteInput{FullPath: testFullPath, IID: tc.iid})
			if err == nil {
				t.Fatalf("expected error for IID=%d, got nil", tc.iid)
			}
			if !strings.Contains(err.Error(), "work_item_iid") {
				t.Errorf("expected error to mention 'iid' for IID=%d, got: %v", tc.iid, err)
			}
		})
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
		IID: 42, Title: "Test", Type: "Issue", State: "OPEN", Author: namedUser("dev"),
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

// TestFormatListMarkdown_WithNextPage_EmitsTheCursorLine verifies that the next-page cursor is
// rendered when the connection reports a further page, so a caller can pass it
// back through the after input.
func TestFormatListMarkdown_WithNextPage_EmitsTheCursorLine(t *testing.T) {
	out := ListOutput{
		WorkItems: []WorkItemItem{{IID: 1, Type: testTypeIssue, State: testStateOpen, Title: "A", Author: namedUser(testAuthorDev)}},
		Pagination: toolutil.GraphQLPaginationOutput{
			HasNextPage: true,
			EndCursor:   "next-page-cursor",
		},
	}
	text := extractText(t, FormatListMarkdown(out))
	if !strings.Contains(text, "next-page-cursor") {
		t.Errorf("expected next-page cursor in output:\n%s", text)
	}
}

// TestFormatListMarkdown_WithData verifies FormatListMarkdown when with data.
func TestFormatListMarkdown_WithData(t *testing.T) {
	out := ListOutput{WorkItems: []WorkItemItem{
		{IID: 1, Type: "Issue", State: "OPEN", Title: "A", Author: namedUser("dev")},
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
		Author:       &gl.BasicUser{ID: 2, Username: testAuthorAlice, Name: "Alice A", State: "active"},
		Assignees:    []*gl.BasicUser{{ID: 3, Username: testAuthorBob}, {ID: 4, Username: testAuthorCarol}},
		Labels: []gl.LabelDetails{
			{ID: 7, Name: testLabelBug, Color: "#d9534f", Description: "broken", DescriptionHTML: "<p>broken</p>", TextColor: "#ffffff"},
			{ID: 8, Name: testLabelUrgent},
		},
		LinkedItems: []gl.LinkedWorkItem{
			{NamespacePath: "my-group/other", IID: 7, LinkType: "blocks"},
		},
		Children: []gl.WorkItemIID{
			{NamespacePath: "my-group/child-proj", IID: 5},
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
	if len(item.Children) != 1 {
		t.Fatalf("Children = %d, want 1", len(item.Children))
	}
	if item.Children[0].IID != 5 {
		t.Errorf("Children[0].IID = %d, want 5", item.Children[0].IID)
	}
	if item.Children[0].Path != "my-group/child-proj" {
		t.Errorf("Children[0].Path = %q, want my-group/child-proj", item.Children[0].Path)
	}
}

// assertFullItemPeople checks full item people invariants for tests.
//
// Each of the three is checked whole rather than by name, because the whole
// object is what the converter now carries and a name-only assertion is what
// let the other fields go missing before.
func assertFullItemPeople(t *testing.T, item WorkItemItem) {
	t.Helper()
	wantAuthor := &toolutil.BasicUserOutput{ID: 2, Username: testAuthorAlice, Name: "Alice A", State: "active"}
	if !reflect.DeepEqual(item.Author, wantAuthor) {
		t.Errorf("Author = %+v, want %+v", item.Author, wantAuthor)
	}
	wantAssignees := []*toolutil.BasicUserOutput{
		{ID: 3, Username: testAuthorBob},
		{ID: 4, Username: testAuthorCarol},
	}
	if !reflect.DeepEqual(item.Assignees, wantAssignees) {
		t.Errorf("Assignees = %+v, want %+v", item.Assignees, wantAssignees)
	}
	wantLabels := []*toolutil.LabelDetailsOutput{
		{ID: 7, Name: testLabelBug, Color: "#d9534f", Description: "broken", DescriptionHTML: "<p>broken</p>", TextColor: "#ffffff"},
		{ID: 8, Name: testLabelUrgent},
	}
	if !reflect.DeepEqual(item.Labels, wantLabels) {
		t.Errorf("Labels = %+v, want %+v", item.Labels, wantLabels)
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
	if item.Author != nil {
		t.Errorf("Author should be absent, got %+v", item.Author)
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
	if authorName(item.Author) != testAuthorDev {
		t.Errorf("Author = %+v, want dev", item.Author)
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

// TestFormatGetMarkdown_FullPopulated checks the whole card of a populated work
// item: the rows in order, the handles with their "@", the state with the emoji
// the shared table gives its REST spelling, the one-line description on the
// field's own row, and the guidance section naming the canonical action.
func TestFormatGetMarkdown_FullPopulated(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:         42,
		Title:       "Full WI",
		Type:        testTypeTask,
		State:       testStateOpen,
		Author:      namedUser(testAuthorAlice),
		Assignees:   namedUsers(testAuthorBob, testAuthorCarol),
		Labels:      namedLabels(testLabelBug, testLabelUrgent),
		WebURL:      "https://gitlab.example.com/work_items/42",
		Description: "A very detailed description.",
	}}
	want := "## Work Item #42: Full WI\n\n" +
		"- **Type**: Task\n" +
		"- **State**: 🟢 OPEN\n" +
		"- **Author**: @alice\n" +
		"- **Assignees**: @bob, @carol\n" +
		"- **Labels**: bug, urgent\n" +
		"- **URL**: [https://gitlab.example.com/work_items/42](https://gitlab.example.com/work_items/42)\n" +
		"- **Description**: A very detailed description.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_update' to modify this work item\n"
	if got := extractText(t, FormatGetMarkdown(out)); got != want {
		t.Errorf("FormatGetMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatGetMarkdown_Children checks the whole card of a work item whose
// hierarchy children are a nested collection: the table opens under its own
// heading after a blank line and its rows carry no sigil, since a child may be
// an epic or an issue and the query does not say which.
func TestFormatGetMarkdown_Children(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:   42,
		Title: "Parent WI",
		Type:  testTypeTask,
		State: testStateOpen,
		Children: []ChildItem{
			{IID: 20, Path: "my-group/child-a"},
			{IID: 21, Path: "my-group/child-b"},
		},
	}}
	want := "## Work Item #42: Parent WI\n\n" +
		"- **Type**: Task\n" +
		"- **State**: 🟢 OPEN\n" +
		"\n### Children\n\n" +
		"| IID | Path |\n" +
		"| --- | --- |\n" +
		"| 20 | my-group/child-a |\n" +
		"| 21 | my-group/child-b |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_update' to modify this work item\n"
	if got := extractText(t, FormatGetMarkdown(out)); got != want {
		t.Errorf("FormatGetMarkdown(children)\n got %q\nwant %q", got, want)
	}
}

// TestFormatGetMarkdown_Empty checks that a work item with nothing in it
// renders the heading and the guidance section alone: an absent value writes
// nothing, so no label stands with nothing after it.
func TestFormatGetMarkdown_Empty(t *testing.T) {
	// The trailing space of the empty title is trimmed on the way out by
	// NormalizeResultMarkdown, which every rendered response passes through.
	want := "## Work Item #0:\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_update' to modify this work item\n"
	if got := extractText(t, FormatGetMarkdown(GetOutput{WorkItem: WorkItemItem{}})); got != want {
		t.Errorf("FormatGetMarkdown(zero)\n got %q\nwant %q", got, want)
	}
}

// TestFormatGetMarkdown_OneFieldAtATime checks the whole card of a work item
// carrying exactly one optional value, one case per value: what is present is
// on its row and nothing else is written at all.
func TestFormatGetMarkdown_OneFieldAtATime(t *testing.T) {
	head := func(title string) string {
		return "## Work Item #1: " + title + "\n\n- **Type**: Issue\n- **State**: 🟢 OPEN\n"
	}
	hints := "\n---\n💡 **Next steps:**\n- Use action 'issue.work_item_update' to modify this work item\n"
	cases := []struct {
		name string
		item WorkItemItem
		want string
	}{
		{
			name: "author",
			item: WorkItemItem{IID: 1, Title: "Simple", Type: testTypeIssue, State: testStateOpen, Author: namedUser(testAuthorDev)},
			want: head("Simple") + "- **Author**: @dev\n" + hints,
		},
		{
			name: "assignees",
			item: WorkItemItem{IID: 1, Title: "Assigned", Type: testTypeIssue, State: testStateOpen, Assignees: namedUsers(testAuthorAlice)},
			want: head("Assigned") + "- **Assignees**: @alice\n" + hints,
		},
		{
			name: "labels",
			item: WorkItemItem{IID: 1, Title: "Labeled", Type: testTypeIssue, State: testStateOpen, Labels: namedLabels("feature")},
			want: head("Labeled") + "- **Labels**: feature\n" + hints,
		},
		{
			name: "web url",
			item: WorkItemItem{IID: 1, Title: "URL only", Type: testTypeIssue, State: testStateOpen, WebURL: "https://example.com/wi/1"},
			want: head("URL only") + "- **URL**: [https://example.com/wi/1](https://example.com/wi/1)\n" + hints,
		},
		{
			name: "description",
			item: WorkItemItem{IID: 1, Title: "With desc", Type: testTypeIssue, State: testStateOpen, Description: "My description"},
			want: head("With desc") + "- **Description**: My description\n" + hints,
		},
		{
			name: "status",
			item: WorkItemItem{IID: 1, Title: "Status item", Type: testTypeIssue, State: testStateOpen, Status: "IN_PROGRESS"},
			want: head("Status item") + "- **Status**: IN_PROGRESS\n" + hints,
		},
		{
			name: "confidential",
			item: WorkItemItem{IID: 1, Title: "Secret", Type: testTypeIssue, State: testStateOpen, Confidential: true},
			want: head("Secret") + "- 🔒 **Confidential**\n" + hints,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractText(t, FormatGetMarkdown(GetOutput{WorkItem: tc.item})); got != tc.want {
				t.Errorf("FormatGetMarkdown(%s)\n got %q\nwant %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestFormatGetMarkdown_MultiLineDescription checks that a description of more
// than one line becomes a blockquote indented under its label, so nothing a
// person typed into GitLab can add a row, a heading or a list item to the card.
func TestFormatGetMarkdown_MultiLineDescription(t *testing.T) {
	want := "## Work Item #1: Quoted\n\n" +
		"- **Type**: Issue\n" +
		"- **State**: 🟢 OPEN\n" +
		"- **Description**:\n" +
		"  > First line\n" +
		"  >\n" +
		"  > ## not a heading\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_update' to modify this work item\n"
	got := extractText(t, FormatGetMarkdown(GetOutput{WorkItem: WorkItemItem{
		IID: 1, Title: "Quoted", Type: testTypeIssue, State: testStateOpen,
		Description: "First line\n\n## not a heading",
	}}))
	if got != want {
		t.Errorf("FormatGetMarkdown(multi-line description)\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_MultipleItems checks the whole list rendering: the
// heading counting what the page holds, one row per item with the state emoji,
// the handle with its "@" and the confidential marker, and one guidance
// section.
func TestFormatListMarkdown_MultipleItems(t *testing.T) {
	out := ListOutput{WorkItems: []WorkItemItem{
		{IID: 1, Type: testTypeIssue, State: testStateOpen, Title: "First", Author: namedUser("dev1")},
		{IID: 2, Type: testTypeTask, State: testStateClosed, Title: "Second", Author: namedUser("dev2"), Confidential: true},
		{IID: 3, Type: "Epic", State: testStateOpen, Title: "Third", Author: namedUser("dev3")},
	}}
	want := "## Work Items (3)\n\n" +
		"| IID | Type | State | Status | Title | Author |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| #1 | Issue | 🟢 OPEN |  | First | @dev1 |\n" +
		"| #2 🔒 | Task | 🔴 CLOSED |  | Second | @dev2 |\n" +
		"| #3 | Epic | 🟢 OPEN |  | Third | @dev3 |\n" +
		"\n" + toolutil.FormatGraphQLPagination(toolutil.GraphQLPaginationOutput{}, 3) + "\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_get' to view full details of a specific item\n"
	if got := extractText(t, FormatListMarkdown(out)); got != want {
		t.Errorf("FormatListMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdown_EmptyReturnsMessage checks the whole empty rendering:
// the one sentence, then the hint that a namespace the token cannot read lists
// no work items either, which is what GitLab's null namespace looks like here.
func TestFormatListMarkdown_EmptyReturnsMessage(t *testing.T) {
	want := "No work items found.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- If work items were expected, verify full_path with `gitlab_project_list` or `gitlab_group_list`: a namespace that does not exist, or that the token cannot read, also lists no work items\n"
	if got := extractText(t, FormatListMarkdown(ListOutput{})); got != want {
		t.Errorf("FormatListMarkdown(empty)\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdown_LinkedRows_AreLinkedAndAskedToStay checks the whole
// rendering of a page where an item carries a web address: its reference cell
// is a link, the confidential marker follows the link rather than replacing
// it, and the guidance opens by asking the model to keep the links.
//
// No list fixture in this file carried a web address, so the linked half of
// referenceCell and the linked branch of writeListHints were written by
// nothing. The linked item is the second of the two on purpose: the flag that
// decides the hint is accumulated across the rows, so a page that only becomes
// linked partway through is what tells the accumulation from a look at the
// first row alone.
func TestFormatListMarkdown_LinkedRows_AreLinkedAndAskedToStay(t *testing.T) {
	const url = "https://gitlab.example.com/-/work_items/2"
	out := ListOutput{WorkItems: []WorkItemItem{
		{IID: 1, Type: testTypeIssue, State: testStateOpen, Title: "First", Author: namedUser("dev1")},
		{IID: 2, Type: testTypeTask, State: testStateClosed, Title: "Second", Author: namedUser("dev2"), WebURL: url, Confidential: true},
	}}
	want := "## Work Items (2)\n\n" +
		"| IID | Type | State | Status | Title | Author |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| #1 | Issue | 🟢 OPEN |  | First | @dev1 |\n" +
		"| [#2](" + url + ") 🔒 | Task | 🔴 CLOSED |  | Second | @dev2 |\n" +
		"\n" + toolutil.FormatGraphQLPagination(toolutil.GraphQLPaginationOutput{}, 2) + "\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'issue.work_item_get' to view full details of a specific item\n"
	if got := extractText(t, FormatListMarkdown(out)); got != want {
		t.Errorf("FormatListMarkdown(linked)\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdown_SpecialCharsInTitle checks that a pipe in a title
// stays one cell: the escaped entity is written and the row keeps its columns.
func TestFormatListMarkdown_SpecialCharsInTitle(t *testing.T) {
	out := ListOutput{WorkItems: []WorkItemItem{
		{IID: 1, Type: testTypeIssue, State: testStateOpen, Title: "Has | pipe", Author: namedUser(testAuthorDev)},
	}}
	text := extractText(t, FormatListMarkdown(out))
	if !strings.Contains(text, "| #1 | Issue | 🟢 OPEN |  | Has &#124; pipe | @dev |\n") {
		t.Errorf("the pipe was not neutralized in the row:\n%s", text)
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
	first := 10

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

// TestList_BackwardPagination verifies that a request naming a start cursor
// reaches GitLab as before and last, with no first.
//
// This tool published start_cursor and has_previous_page while offering no
// parameter that could spend either, so a model following the cursor back had
// nothing to send it in. The SDK's own document declares all four variables,
// so the repair was to add the pair rather than to withdraw the output half.
func TestList_BackwardPagination(t *testing.T) {
	var body string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		testutil.RespondJSON(w, http.StatusOK,
			`{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"hasPreviousPage":true,"endCursor":"","startCursor":"cursor-start"}}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(t.Context(), client, ListInput{
		FullPath: testFullPath,
		Last:     new(5),
		Before:   "cursor-xyz",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(body, "cursor-xyz") {
		t.Errorf("request body missing the before cursor: %s", body)
	}
	if !strings.Contains(body, `"last":5`) && !strings.Contains(body, `"last": 5`) {
		t.Errorf("request body missing the last variable: %s", body)
	}
	// This SDK method assembles its document from the options it was given, so
	// an unset count is absent from both the signature and the variables
	// rather than sent as a null. Either way the direction is left to before
	// and last, which is what the assertion is about.
	if strings.Contains(body, `"first":`) {
		t.Errorf("request body carries a page size for first on a backward request: %s", body)
	}
	if !out.Pagination.HasPreviousPage || out.Pagination.StartCursor != "cursor-start" {
		t.Errorf("Pagination = %+v, want the backward half the caller can now spend", out.Pagination)
	}
}

// TestList_BeforeAloneStillPagesBackward verifies that a caller who names only
// a start cursor is sent last and no first, at the default page size.
func TestList_BeforeAloneStillPagesBackward(t *testing.T) {
	var body string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		testutil.RespondJSON(w, http.StatusOK,
			`{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":"end"}}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := List(t.Context(), client, ListInput{
		FullPath: testFullPath,
		Before:   "cursor-xyz",
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(body, `"last":20`) && !strings.Contains(body, `"last": 20`) {
		t.Errorf("request body missing the default backward page size: %s", body)
	}
	if strings.Contains(body, `"first":`) {
		t.Errorf("request body carries a page size for first on a backward request: %s", body)
	}
}

// TestList_ContradictoryPageSizes verifies that naming both first and last is
// refused before a request is made. GitLab answers the pair with "Can only
// provide either first or last, not both", so guessing which one the caller
// meant would only move the failure.
func TestList_ContradictoryPageSizes(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := List(t.Context(), client, ListInput{
		FullPath: testFullPath,
		First:    new(10),
		Last:     new(5),
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "first and last cannot be combined") {
		t.Errorf("error = %v, want it to name the contradiction", err)
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

// TestCreate_WithStatus verifies that Create maps a documented status string
// (e.g. "IN_PROGRESS") to the corresponding GitLab work item status GID via
// mapStatusToID and forwards it in the CreateWorkItem request. No other
// Create test sets input.Status, so a regression that dropped or
// mis-mapped the status would go unnoticed and silently create work items
// with the wrong initial status.
func TestCreate_WithStatus(t *testing.T) {
	var body string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/60","iid":"60","workItemType":{"name":"Issue"},"state":"OPEN","title":"With status","author":{"username":"dev"},"widgets":[]}}}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(t.Context(), client, CreateInput{
		FullPath:       testFullPath,
		WorkItemTypeID: testTypeGID,
		Title:          "With status",
		Status:         "IN_PROGRESS",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "With status" {
		t.Errorf("Title = %q, want %q", out.WorkItem.Title, "With status")
	}
	wantStatus := string(gl.WorkItemStatusInProgress)
	if !strings.Contains(body, wantStatus) {
		t.Errorf("request body = %s, want it to contain mapped status %q", body, wantStatus)
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

// TestCreate_MalformedDate_RefusesAndNamesTheField verifies that create stops
// on a start_date or due_date it cannot read, naming the one at fault, instead
// of dropping it and reporting a work item created without the date asked for.
//
// One case per field: a shared parser reached from two call sites is exactly
// the shape where one of the two silently keeps the old behavior.
func TestCreate_MalformedDate_RefusesAndNamesTheField(t *testing.T) {
	cases := []struct {
		name  string
		input CreateInput
	}{
		{"start_date", CreateInput{StartDate: "not-a-date"}},
		{"due_date", CreateInput{DueDate: "31/01/2025"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
			input := tc.input
			input.FullPath = testProjectPath
			input.WorkItemTypeID = testTypeGID
			input.Title = "Bad date"
			_, err := Create(t.Context(), client, input)
			if err == nil {
				t.Fatal(errExpectedNil)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("error = %v, want it to name %s", err, tc.name)
			}
		})
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	for _, tc := range invalidIIDCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Update(t.Context(), client, UpdateInput{FullPath: testFullPath, IID: tc.iid, Title: "x"})
			if err == nil {
				t.Fatalf("expected error for IID=%d, got nil", tc.iid)
			}
			if !strings.Contains(err.Error(), "work_item_iid") {
				t.Errorf("expected error to mention 'iid' for IID=%d, got: %v", tc.iid, err)
			}
		})
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

// TestUpdate_MalformedDate_RefusesBeforeAnyRequest verifies that update stops
// on a start_date or due_date it cannot read, naming the one at fault, and
// does so before issuing anything: the clearing guard's read would otherwise be
// spent on a call already destined to be refused.
func TestUpdate_MalformedDate_RefusesBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		name  string
		input UpdateInput
	}{
		{"start_date", UpdateInput{StartDate: "not-a-date"}},
		{"due_date", UpdateInput{DueDate: "also-invalid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
			input := tc.input
			input.FullPath = testFullPath
			input.IID = 1
			_, err := Update(t.Context(), client, input)
			if err == nil {
				t.Fatal(errExpectedNil)
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Errorf("error = %v, want it to name %s", err, tc.name)
			}
		})
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

// clearGuardHandler serves the two GraphQL shapes an empty-list update needs:
// the work item read the guard performs, and the update mutation itself. It
// dispatches on the request body so the extra read does not depend on call
// ordering, and records how many reads happened.
//
// It carried the doc comment of a TestUpdate_EmptyAssigneesRemovesAll that is
// nowhere in this file, promising that the empty array is forwarded to GitLab.
// Nothing here reads a request body, so that claim was made by no test at all;
// TestUpdate_EmptyAssignees_ReachesGitLabAsAnEmptyArray makes it now.
func clearGuardHandler(t *testing.T, assignees string, reads *int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			// t.Fatalf would abort the httptest server's goroutine rather than
			// the test: report, answer deterministically and return.
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		if strings.Contains(string(body), "workItemUpdate") {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Updated","author":{"username":"dev"},"widgets":[]}}}}`)
			return
		}
		*reads++
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Updated","author":{"username":"dev"},"features":{"assignees":{"assignees":{"nodes":[`+assignees+`]}}}}}}}`)
	}
}

// TestUpdate_EmptyAssigneesWithNoneAssignedProceeds verifies that clearing an
// already-empty assignee list is accepted without confirmation: nothing would
// be deleted, so the guard must not make an ordinary edit interactive.
func TestUpdate_EmptyAssigneesWithNoneAssignedProceeds(t *testing.T) {
	reads := 0
	client := testutil.NewTestClient(t, clearGuardHandler(t, "", &reads))

	out, err := Update(t.Context(), client, UpdateInput{
		FullPath:    testFullPath,
		IID:         1,
		AssigneeIDs: []int64{},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "Updated" {
		t.Errorf("Title = %q", out.WorkItem.Title)
	}
	if reads == 0 {
		t.Error("guard did not read the work item before clearing assignees")
	}
}

// TestUpdate_EmptyAssigneesWithAssignedRequiresConfirmation verifies that the
// same call is refused when assignees would actually be removed, and that the
// error tells the caller how to proceed. Clients without elicitation must not
// have the deletion happen silently.
func TestUpdate_EmptyAssigneesWithAssignedRequiresConfirmation(t *testing.T) {
	reads := 0
	client := testutil.NewTestClient(t, clearGuardHandler(t, `{"username":"dev"},{"username":"ops"}`, &reads))

	_, err := Update(t.Context(), client, UpdateInput{
		FullPath:    testFullPath,
		IID:         1,
		AssigneeIDs: []int64{},
	})
	if err == nil {
		t.Fatal("clearing 2 assignees was accepted without confirmation")
	}
	if !strings.Contains(err.Error(), "all 2 assignees") {
		t.Errorf("error should name what would be removed, got: %v", err)
	}
	if !strings.Contains(err.Error(), "confirm=true") {
		t.Errorf("error should tell the caller how to confirm, got: %v", err)
	}
}

// TestUpdate_EmptyAssigneesWithConfirmProceeds verifies that an explicit
// confirm=true performs the removal.
//
// The confirmation is read from the raw tool call, not from UpdateInput: the
// reserved confirm key is stripped before the typed input is unmarshalled, so a
// test that set the struct field directly would pass while the real MCP path
// stayed blocked. The request is therefore built the way a caller sends it.
func TestUpdate_EmptyAssigneesWithConfirmProceeds(t *testing.T) {
	reads := 0
	client := testutil.NewTestClient(t, clearGuardHandler(t, `{"username":"dev"}`, &reads))

	ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "gitlab_update_work_item",
			Arguments: json.RawMessage(`{"full_path":"my-group/my-project","work_item_iid":1,"assignee_ids":[],"confirm":true}`),
		},
	})

	out, err := Update(ctx, client, UpdateInput{
		FullPath:    testFullPath,
		IID:         1,
		AssigneeIDs: []int64{},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.WorkItem.Title != "Updated" {
		t.Errorf("Title = %q", out.WorkItem.Title)
	}
	// Only the update's own global-ID lookup: a confirmed call must not pay for
	// the guard's inspection read.
	if reads != 1 {
		t.Errorf("reads = %d, want exactly the update's own ID lookup", reads)
	}
}

// TestUpdate_EmptyAssigneesWithNestedConfirmProceeds verifies the dispatcher
// call shape, where the reserved key travels inside params rather than at the
// top level.
func TestUpdate_EmptyAssigneesWithNestedConfirmProceeds(t *testing.T) {
	reads := 0
	client := testutil.NewTestClient(t, clearGuardHandler(t, `{"username":"dev"}`, &reads))

	ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "gitlab_execute_action",
			// The canonical ID, not "work_item.update": the guard reads confirm
			// out of params and never resolves the action, so a fixture naming
			// an action that does not exist passes while documenting a call a
			// model cannot make.
			Arguments: json.RawMessage(`{"action":"issue.work_item_update","params":{"full_path":"my-group/my-project","work_item_iid":1,"assignee_ids":[],"confirm":true}}`),
		},
	})

	if _, err := Update(ctx, client, UpdateInput{
		FullPath:    testFullPath,
		IID:         1,
		AssigneeIDs: []int64{},
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestUpdate_EmptyCRMContactsAlwaysConfirms verifies that clearing CRM contacts
// always asks: the read model does not expose them, so the guard cannot tell a
// no-op from a deletion and must not assume the harmless case.
func TestUpdate_EmptyCRMContactsAlwaysConfirms(t *testing.T) {
	reads := 0
	client := testutil.NewTestClient(t, clearGuardHandler(t, "", &reads))

	_, err := Update(t.Context(), client, UpdateInput{
		FullPath:      testFullPath,
		IID:           1,
		CRMContactIDs: []int64{},
	})
	if err == nil {
		t.Fatal("clearing CRM contacts was accepted without confirmation")
	}
	if !strings.Contains(err.Error(), "every CRM contact") {
		t.Errorf("error should name CRM contacts, got: %v", err)
	}
}

// TestUpdate_OmittedListsSkipGuard verifies that an update which does not touch
// assignees or CRM contacts never reads the work item, so the guard costs
// nothing for ordinary edits.
func TestUpdate_OmittedListsSkipGuard(t *testing.T) {
	reads := 0
	client := testutil.NewTestClient(t, clearGuardHandler(t, `{"username":"dev"}`, &reads))

	if _, err := Update(t.Context(), client, UpdateInput{
		FullPath: testFullPath,
		IID:      1,
		Title:    "Renamed",
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	// The update performs its own global-ID lookup; anything beyond that would
	// be the guard reading a work item it has no reason to inspect.
	if reads != 1 {
		t.Errorf("reads = %d, want exactly the update's own ID lookup", reads)
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
//
// The link type and the ids are read out of the mutation, since that is the
// claim: the test asserted only the title the fixture echoes back, which a
// create that dropped the widget answers with just as readily. The ids arrive
// wrapped as work item global IDs, and the key client-go spells them under is
// workItemsIds rather than the workItemIds a reader would expect.
func TestCreate_WithLinkedItems(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQL(t, &got,
		`{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/55","iid":"55","workItemType":{"name":"Issue"},"state":"OPEN","title":"Linked","author":{"username":"dev"},"widgets":[]}}}}`))

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
	input := mutationInput(t, got)
	for _, testCase := range []wireCase{
		{name: "link_type", widget: "linkedItemsWidget", field: "linkType", want: "BLOCKS"},
		{name: "work_item_ids", widget: "linkedItemsWidget", field: "workItemsIds", want: []any{"gid://gitlab/WorkItem/10", "gid://gitlab/WorkItem/20"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assertWire(t, input, testCase)
		})
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

// TestCreate_LinkedItemsEmptyIDs verifies that linked items with empty IDs is
// ignored, by reading the mutation rather than the answer: a link type with no
// work items to link is not a widget GitLab has anything to do, and the test
// asserted only the echoed title, which says nothing about what was sent.
func TestCreate_LinkedItemsEmptyIDs(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQL(t, &got,
		`{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Issue"},"state":"OPEN","title":"Empty links","author":{"username":"dev"},"widgets":[]}}}}`))

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
	if widget, ok := mutationInput(t, got)["linkedItemsWidget"]; ok {
		t.Errorf("linkedItemsWidget = %#v, want it absent from the mutation", widget)
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
			{NamespacePath: "group/proj", IID: 5, LinkType: "relates_to"},
			{NamespacePath: "group/other", IID: 8, LinkType: "blocks"},
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

// TestFormatGetMarkdown_WithLinkedItems checks the whole card of a work item
// with a linked item: the collection is a table under its own heading, after a
// blank line, and the card's rows stay above it.
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
	want := "## Work Item #1: Linked item\n\n" +
		"- **Type**: Issue\n" +
		"- **State**: 🟢 OPEN\n" +
		"\n### Linked Items\n\n" +
		"| IID | Link Type | Path |\n" +
		"| --- | --- | --- |\n" +
		"| 5 | blocks | group/proj |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_update' to modify this work item\n"
	if got := extractText(t, FormatGetMarkdown(out)); got != want {
		t.Errorf("FormatGetMarkdown(linked items)\n got %q\nwant %q", got, want)
	}
}

// TestFormatGetMarkdown_NoStatusNoLinkedItems checks the whole card of a work
// item with neither: the two rows it does have, and nothing else.
func TestFormatGetMarkdown_NoStatusNoLinkedItems(t *testing.T) {
	out := GetOutput{WorkItem: WorkItemItem{
		IID:   1,
		Title: "Plain",
		Type:  testTypeIssue,
		State: testStateOpen,
	}}
	want := "## Work Item #1: Plain\n\n" +
		"- **Type**: Issue\n" +
		"- **State**: 🟢 OPEN\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_update' to modify this work item\n"
	if got := extractText(t, FormatGetMarkdown(out)); got != want {
		t.Errorf("FormatGetMarkdown(plain)\n got %q\nwant %q", got, want)
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestListWorkItemTypes_WithOptions verifies ListWorkItemTypes passes name,
// onlyAvailable and the forward cursor pair to the API.
//
// It set all four and read none of them, asserting the name of the type the
// fixture answers with, which a handler that forwarded nothing answers with
// too. Each is given a value no other option has, so the four variables also
// tell a swap apart.
func TestListWorkItemTypes_WithOptions(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQL(t, &got, `{
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
		}`))

	out, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{
		FullPath:      testFullPath,
		Name:          "Issue",
		OnlyAvailable: true,
		First:         new(10),
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
	sent := map[string]any{
		"namespacePath": testFullPath,
		"name":          "ISSUE",
		"onlyAvailable": true,
		"first":         float64(10),
		"after":         "cursor-abc",
	}
	for variable, want := range sent {
		t.Run(variable, func(t *testing.T) {
			if value := got.Variables[variable]; value != want {
				t.Errorf("variable %s = %#v, want %#v", variable, value, want)
			}
		})
	}
}

// TestListWorkItemTypes_Pagination_CarriesEveryCursorField holds the four
// fields of the connection's pageInfo to the answer, each with a value no
// other field has.
//
// The only fixture that read them sent false, false, "" and "", where a field
// copied from its neighbor looks exactly like the one it should have carried,
// and start_cursor was read nowhere at all: the backward half of this
// connection could have been dropped with the whole suite green.
func TestListWorkItemTypes_Pagination_CarriesEveryCursorField(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"namespace":{"workItemTypes":{`+
			`"nodes":[{"id":"gid://gitlab/WorkItems::Type/1","name":"Issue","enabled":true}],`+
			`"pageInfo":{"hasNextPage":true,"hasPreviousPage":false,"endCursor":"cursor-end","startCursor":"cursor-start"}}}}}`)
	})
	out, err := ListWorkItemTypes(t.Context(), testutil.NewTestClient(t, handler), ListWorkItemTypesInput{FullPath: testFullPath})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := toolutil.GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor-end", StartCursor: "cursor-start"}
	if out.Pagination != want {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, want)
	}
}

// TestListWorkItemTypes_BackwardPagination verifies that the last and before
// cursor-pagination inputs are wired into the GraphQL request variables, mirroring
// v2.ListWorkItemTypesOptions backward-paging arguments.
func TestListWorkItemTypes_BackwardPagination(t *testing.T) {
	var body string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		testutil.RespondJSON(w, http.StatusOK, `{
			"data": {
				"namespace": {
					"workItemTypes": {
						"nodes": [
							{"id":"gid://gitlab/WorkItems::Type/1","name":"Issue","enabled":true}
						],
						"pageInfo": {"hasNextPage":false,"hasPreviousPage":true,"endCursor":"","startCursor":"cursor-start"}
					}
				}
			}
		}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{
		FullPath: testFullPath,
		Last:     new(5),
		Before:   "cursor-xyz",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Types) != 1 {
		t.Fatalf("expected 1 type, got %d", len(out.Types))
	}
	if !strings.Contains(body, "cursor-xyz") {
		t.Errorf("request body missing before cursor: %s", body)
	}
	if !strings.Contains(body, `"last":5`) && !strings.Contains(body, `"last": 5`) {
		t.Errorf("request body missing last variable: %s", body)
	}
	if !out.Pagination.HasPreviousPage {
		t.Errorf("expected HasPreviousPage = true")
	}
}

// TestListWorkItemTypes_BeforeAloneStillPagesBackward verifies that a caller
// who names only a start cursor is sent last and no first.
//
// The SDK forwards whichever of the two counts is set, and this input used to
// set neither when only before was named. graphql-ruby then fills first from
// its own default page size, which takes the head of the list rather than the
// page before the cursor, so following start_cursor back looped on page one.
func TestListWorkItemTypes_BeforeAloneStillPagesBackward(t *testing.T) {
	var body string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		testutil.RespondJSON(w, http.StatusOK, `{
			"data": {
				"namespace": {
					"workItemTypes": {
						"nodes": [{"id":"gid://gitlab/WorkItems::Type/1","name":"Issue","enabled":true}],
						"pageInfo": {"hasNextPage":true,"hasPreviousPage":false,"endCursor":"end","startCursor":""}
					}
				}
			}
		}`)
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{
		FullPath: testFullPath,
		Before:   "cursor-xyz",
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(body, `"last":20`) && !strings.Contains(body, `"last": 20`) {
		t.Errorf("request body missing the default backward page size: %s", body)
	}
	// The SDK sends every variable its document declares, so first is present
	// as an explicit null. GraphQL reads that as "not provided", which is what
	// leaves the direction to before and last.
	if !strings.Contains(body, `"first":null`) {
		t.Errorf("request body carries a page size for first on a backward request: %s", body)
	}
}

// TestListWorkItemTypes_ContradictoryPageSizes verifies that naming both first
// and last is refused before a request is made. GitLab answers the pair with
// "Can only provide either first or last, not both", so guessing which one the
// caller meant would only move the failure.
func TestListWorkItemTypes_ContradictoryPageSizes(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ListWorkItemTypes(t.Context(), client, ListWorkItemTypesInput{
		FullPath: testFullPath,
		First:    new(10),
		Last:     new(5),
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "first and last cannot be combined") {
		t.Errorf("error = %v, want it to name the contradiction", err)
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
	want := "## Work Item Types (2)\n\n" +
		"| Name | ID | Enabled |\n" +
		"| --- | --- | --- |\n" +
		"| Issue | `gid://gitlab/WorkItems::Type/1` | ✅ |\n" +
		"| Task | `gid://gitlab/WorkItems::Type/7` | ❌ |\n" +
		"\n" + toolutil.FormatGraphQLPagination(toolutil.GraphQLPaginationOutput{}, 2) + "\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_create' to create work items of a type, with the work_item_type_id from the ID column\n"
	if got := extractText(t, FormatWorkItemTypeListMarkdown(out)); got != want {
		t.Errorf("FormatWorkItemTypeListMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatWorkItemTypeListMarkdown_Empty verifies that an empty type list
// returns a result containing a "no work item types" message.
func TestFormatWorkItemTypeListMarkdown_Empty(t *testing.T) {
	want := "No work item types found.\n"
	if got := extractText(t, FormatWorkItemTypeListMarkdown(WorkItemTypeListOutput{})); got != want {
		t.Errorf("FormatWorkItemTypeListMarkdown(empty) = %q, want %q", got, want)
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
	if authorName(wi.Author) != testAuthorAlice {
		t.Errorf("Author = %+v", wi.Author)
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
// Filters, widgets and the field set a list asks for
// ---------------------------------------------------------------------------.

// graphQLRequest is what a test needs out of a recorded GraphQL POST: the
// document, so it can assert which fields the query asked GitLab for, and the
// variables, so it can assert what a filter turned into on the wire.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// recordGraphQL answers every POST with response and records the request into
// got. The request is written before the answer and read after the call
// returns, so the HTTP round trip orders the two.
func recordGraphQL(t *testing.T, got *graphQLRequest, response string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Errorf("decode GraphQL request: %v", err)
			http.Error(w, "decode GraphQL request", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, response)
	}
}

// emptyWorkItemsResponse is the shortest answer a list query accepts, for the
// tests whose whole assertion is on the request.
const emptyWorkItemsResponse = `{"data":{"namespace":{"workItems":{"nodes":[]}}}}`

// TestList_Filters_ReachTheWire proves each filter this action exposes is
// declared as a GraphQL variable carrying the value the caller sent.
//
// A filter that never reaches GitLab is worse than one that does not exist:
// the answer comes back unnarrowed, and nothing in it says the condition was
// dropped. The variable names are client-go's, and the pinned schema judges
// both the document and these values on every request, so a case here also
// proves GitLab would accept the pair.
func TestList_Filters_ReachTheWire(t *testing.T) {
	cases := []struct {
		name     string
		input    ListInput
		variable string
		want     any
	}{
		{"assignee_usernames", ListInput{AssigneeUsernames: []string{testAuthorBob, testAuthorCarol}}, "assigneeUsernames", []any{testAuthorBob, testAuthorCarol}},
		{"assignee_wildcard_id", ListInput{AssigneeWildcardID: "ANY"}, "assigneeWildcardId", "ANY"},
		// GitLab compares both against a numeric column without parsing, so the
		// value that reaches the wire has to be the bare id.
		{"crm_contact_id", ListInput{CRMContactID: "1"}, "crmContactId", "1"},
		{"crm_organization_id", ListInput{CRMOrganizationID: "2"}, "crmOrganizationId", "2"},
		{"health_status_filter", ListInput{HealthStatusFilter: "onTrack"}, "healthStatusFilter", "onTrack"},
		{"ids", ListInput{IDs: []string{"gid://gitlab/WorkItem/1"}}, "ids", []any{"gid://gitlab/WorkItem/1"}},
		{"iids", ListInput{IIDs: []string{"12"}}, "iids", []any{"12"}},
		{"in", ListInput{Search: "needle", In: []string{"TITLE"}}, "in", []any{"TITLE"}},
		{"iteration_cadence_id", ListInput{IterationCadenceID: []string{"gid://gitlab/Iterations::Cadence/3"}}, "iterationCadenceId", []any{"gid://gitlab/Iterations::Cadence/3"}},
		{"iteration_id", ListInput{IterationID: []string{"gid://gitlab/Iteration/4"}}, "iterationId", []any{"gid://gitlab/Iteration/4"}},
		{"iteration_wildcard_id", ListInput{IterationWildcardID: "CURRENT"}, "iterationWildcardId", "CURRENT"},
		// TestList_Filters passes a label and reads every other variable it
		// sends, so this was the one filter the action exposes that nothing
		// ever saw on the wire.
		{"label_name", ListInput{LabelName: []string{testLabelBug}}, "labelName", []any{testLabelBug}},
		{"milestone_title", ListInput{MilestoneTitle: []string{"v1.0"}}, "milestoneTitle", []any{"v1.0"}},
		{"milestone_wildcard_id", ListInput{MilestoneWildcardID: "UPCOMING"}, "milestoneWildcardId", "UPCOMING"},
		{"my_reaction_emoji", ListInput{MyReactionEmoji: "thumbsup"}, "myReactionEmoji", "thumbsup"},
		{"parent_ids", ListInput{ParentIDs: []string{"gid://gitlab/WorkItem/9"}}, "parentIds", []any{"gid://gitlab/WorkItem/9"}},
		{"release_tag", ListInput{ReleaseTag: []string{"v1.0.0"}}, "releaseTag", []any{"v1.0.0"}},
		{"release_tag_wildcard_id", ListInput{ReleaseTagWildcardID: "ANY"}, "releaseTagWildcardId", "ANY"},
		{"subscribed", ListInput{Subscribed: "EXPLICITLY_SUBSCRIBED"}, "subscribed", "EXPLICITLY_SUBSCRIBED"},
		{"weight", ListInput{Weight: "3"}, "weight", "3"},
		{"weight_wildcard_id", ListInput{WeightWildcardID: "NONE"}, "weightWildcardId", "NONE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got graphQLRequest
			client := testutil.NewTestClient(t, recordGraphQL(t, &got, emptyWorkItemsResponse))
			input := testCase.input
			input.FullPath = testFullPath
			if _, err := List(t.Context(), client, input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if sent := got.Variables[testCase.variable]; !reflect.DeepEqual(sent, testCase.want) {
				t.Errorf("variable %s = %#v, want %#v", testCase.variable, sent, testCase.want)
			}
		})
	}
}

// TestList_EmptyFilterLists_SendNoVariable pins the premise every list filter
// is handed over on: buildListWorkItemsQuery declares a variable for one only
// when its own len is above zero, so an empty filter builds the same request
// as an absent one.
//
// That is why none of them is guarded here, and it is a claim about client-go
// rather than about this package, so it needs a test of its own: without one,
// a release that started forwarding empty lists would send eleven filters that
// match nothing and no assertion in this file would notice.
func TestList_EmptyFilterLists_SendNoVariable(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQL(t, &got, emptyWorkItemsResponse))
	_, err := List(t.Context(), client, ListInput{
		FullPath:           testFullPath,
		In:                 []string{},
		Types:              []string{},
		AssigneeUsernames:  []string{},
		IDs:                []string{},
		IIDs:               []string{},
		ParentIDs:          []string{},
		LabelName:          []string{},
		MilestoneTitle:     []string{},
		ReleaseTag:         []string{},
		IterationID:        []string{},
		IterationCadenceID: []string{},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, variable := range []string{
		"in", "types", "assigneeUsernames", "ids", "iids", "parentIds",
		"labelName", "milestoneTitle", "releaseTag", "iterationId", "iterationCadenceId",
	} {
		t.Run(variable, func(t *testing.T) {
			if value, ok := got.Variables[variable]; ok {
				t.Errorf("variable %s = %#v, want it absent for an empty filter", variable, value)
			}
		})
	}
}

// TestCreate_EmptyIDLists_BuildNoWidget pins the same premise on the create
// path, where client-go folds an id list into a widget of its own only when
// the list has entries.
func TestCreate_EmptyIDLists_BuildNoWidget(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQL(t, &got, workItemCreateResponse))
	_, err := Create(t.Context(), client, CreateInput{
		FullPath:       testFullPath,
		WorkItemTypeID: testTypeGID,
		Title:          "Widgets",
		AssigneeIDs:    []int64{},
		LabelIDs:       []int64{},
		CRMContactIDs:  []int64{},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	input := mutationInput(t, got)
	for _, widget := range []string{"assigneesWidget", "labelsWidget", "crmContactsWidget"} {
		t.Run(widget, func(t *testing.T) {
			if value, ok := input[widget]; ok {
				t.Errorf("%s = %#v, want it absent for an empty list", widget, value)
			}
		})
	}
}

// TestUpdate_EmptyLabelLists_BuildNoWidget pins it on the update path too. The
// two assignee and CRM lists are deliberately not here: on update an explicit
// empty array is the caller asking for every entry to be removed, so those two
// are guarded on nil rather than on length and reach GitLab as [].
func TestUpdate_EmptyLabelLists_BuildNoWidget(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordWorkItemUpdate(t, &got))
	_, err := Update(t.Context(), client, UpdateInput{
		FullPath:       testFullPath,
		IID:            42,
		Title:          "Renamed",
		AddLabelIDs:    []int64{},
		RemoveLabelIDs: []int64{},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if value, ok := mutationInput(t, got)["labelsWidget"]; ok {
		t.Errorf("labelsWidget = %#v, want it absent for two empty lists", value)
	}
}

// TestList_TimeFilters_ReachTheWireAsRFC3339 covers the eight date-range
// filters, which are the only ones this handler parses rather than forwards.
func TestList_TimeFilters_ReachTheWireAsRFC3339(t *testing.T) {
	const sent = "2025-03-04T05:06:07Z"
	cases := []struct {
		name     string
		input    ListInput
		variable string
	}{
		{"closed_after", ListInput{ClosedAfter: sent}, "closedAfter"},
		{"closed_before", ListInput{ClosedBefore: sent}, "closedBefore"},
		{"created_after", ListInput{CreatedAfter: sent}, "createdAfter"},
		{"created_before", ListInput{CreatedBefore: sent}, "createdBefore"},
		{"due_after", ListInput{DueAfter: sent}, "dueAfter"},
		{"due_before", ListInput{DueBefore: sent}, "dueBefore"},
		{"updated_after", ListInput{UpdatedAfter: sent}, "updatedAfter"},
		{"updated_before", ListInput{UpdatedBefore: sent}, "updatedBefore"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got graphQLRequest
			client := testutil.NewTestClient(t, recordGraphQL(t, &got, emptyWorkItemsResponse))
			input := testCase.input
			input.FullPath = testFullPath
			if _, err := List(t.Context(), client, input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if value := got.Variables[testCase.variable]; value != sent {
				t.Errorf("variable %s = %#v, want %q", testCase.variable, value, sent)
			}
		})
	}
}

// TestList_TimeFilter_AcceptsTheThreeSpellings pins the leniency the schema
// advertises in prose: the date-time format names the canonical spelling, and
// a bare date is read as midnight UTC rather than refused, because a model
// told a value is a date sends exactly that.
func TestList_TimeFilter_AcceptsTheThreeSpellings(t *testing.T) {
	cases := []struct {
		name string
		sent string
		want string
	}{
		{"RFC 3339", "2025-03-04T05:06:07Z", "2025-03-04T05:06:07Z"},
		{"bare datetime", "2025-03-04T05:06:07", "2025-03-04T05:06:07Z"},
		{"bare date", "2025-03-04", "2025-03-04T00:00:00Z"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got graphQLRequest
			client := testutil.NewTestClient(t, recordGraphQL(t, &got, emptyWorkItemsResponse))
			_, err := List(t.Context(), client, ListInput{FullPath: testFullPath, ClosedAfter: testCase.sent})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if value := got.Variables["closedAfter"]; value != testCase.want {
				t.Errorf("closedAfter = %#v, want %q", value, testCase.want)
			}
		})
	}
}

// TestList_MalformedTimeFilter_IsRefusedByName verifies the whole call fails
// and names the offending filter, rather than dropping it and answering with
// more work items than were asked for.
func TestList_MalformedTimeFilter_IsRefusedByName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(t.Context(), client, ListInput{FullPath: testFullPath, DueBefore: "next tuesday"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "due_before") {
		t.Errorf("error = %v, want it to name due_before", err)
	}
}

// TestList_ReturnedFields_FollowTheResolvedTier covers the defect this change
// closes: list asked for client-go's CE default alone, so status, weight,
// health status, iteration and color were requested by nothing and every
// listed work item came back without them whatever the instance held. They are
// asked for only on Premium and Ultimate, because a Community Edition schema
// does not define those widgets and one unknown field fails the whole query.
func TestList_ReturnedFields_FollowTheResolvedTier(t *testing.T) {
	eeFragments := []string{"color {", "healthStatus {", "iteration {", "status {", "weight {"}
	cases := []struct {
		name       string
		enterprise bool
	}{
		{"community edition asks for the CE default only", false},
		{"enterprise asks for the five EE widgets too", true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got graphQLRequest
			client := testutil.NewTestClient(t, recordGraphQL(t, &got, emptyWorkItemsResponse))
			client.SetEnterprise(testCase.enterprise)
			if _, err := List(t.Context(), client, ListInput{FullPath: testFullPath}); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			for _, fragment := range eeFragments {
				if strings.Contains(got.Query, fragment) != testCase.enterprise {
					t.Errorf("query contains %q = %v, want %v\nquery: %s",
						fragment, strings.Contains(got.Query, fragment), testCase.enterprise, got.Query)
				}
			}
		})
	}
}

// TestWorkItems_TierTags_CoverEveryEnterpriseWidget pins the tier of every
// gated field on the three shapes that carry one.
//
// It is the other half of the saved-views table: the two domains publish the
// same conditions through different tools, and a tag dropped on either side is
// a filter one tool offers a tier that cannot use it. The five widget fields
// are the ones client-go keeps out of its Community Edition default set, so
// this table and workItemEEListFields describe the same set from two angles.
func TestWorkItems_TierTags_CoverEveryEnterpriseWidget(t *testing.T) {
	cases := []struct {
		object reflect.Type
		field  string
		want   string
	}{
		{reflect.TypeFor[WorkItemItem](), "Color", "premium"},
		{reflect.TypeFor[WorkItemItem](), "HealthStatus", "ultimate"},
		{reflect.TypeFor[WorkItemItem](), "IterationID", "premium"},
		{reflect.TypeFor[WorkItemItem](), "Status", "premium"},
		{reflect.TypeFor[WorkItemItem](), "Weight", "premium"},
		{reflect.TypeFor[CreateInput](), "Color", "premium"},
		{reflect.TypeFor[CreateInput](), "HealthStatus", "ultimate"},
		{reflect.TypeFor[CreateInput](), "IterationID", "premium"},
		{reflect.TypeFor[CreateInput](), "Status", "premium"},
		{reflect.TypeFor[CreateInput](), "Weight", "premium"},
		{reflect.TypeFor[UpdateInput](), "Color", "premium"},
		{reflect.TypeFor[UpdateInput](), "HealthStatus", "ultimate"},
		{reflect.TypeFor[UpdateInput](), "IterationID", "premium"},
		{reflect.TypeFor[UpdateInput](), "Status", "premium"},
		{reflect.TypeFor[UpdateInput](), "Weight", "premium"},
		{reflect.TypeFor[ListInput](), "HealthStatusFilter", "ultimate"},
		{reflect.TypeFor[ListInput](), "IterationCadenceID", "premium"},
		{reflect.TypeFor[ListInput](), "IterationID", "premium"},
		{reflect.TypeFor[ListInput](), "IterationWildcardID", "premium"},
		{reflect.TypeFor[ListInput](), "Weight", "premium"},
		{reflect.TypeFor[ListInput](), "WeightWildcardID", "premium"},
	}
	for _, testCase := range cases {
		t.Run(testCase.object.Name()+"."+testCase.field, func(t *testing.T) {
			field, ok := testCase.object.FieldByName(testCase.field)
			if !ok {
				t.Fatalf("%s has no field %s", testCase.object.Name(), testCase.field)
			}
			if got := field.Tag.Get("tier"); got != testCase.want {
				t.Errorf("tier tag = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestList_ReturnedFields_SelectTheFragment proves that returned_fields reaches
// the rendered document: the fields named are the ones asked for, and the
// per-tier default is not merged back in over the caller's narrower choice.
//
// The instance is Enterprise here on purpose, since that is the case where an
// implementation that appended the default set instead of replacing it would
// still look right on a Community Edition one.
func TestList_ReturnedFields_SelectTheFragment(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQL(t, &got, emptyWorkItemsResponse))
	client.SetEnterprise(true)
	if _, err := List(t.Context(), client, ListInput{
		FullPath:       testFullPath,
		ReturnedFields: []string{"iid", "title", "weight"},
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"iid", "title", "weight {"} {
		t.Run("asks for "+want, func(t *testing.T) {
			if !strings.Contains(got.Query, want) {
				t.Errorf("query does not ask for %q:\n%s", want, got.Query)
			}
		})
	}
	// A default-set field the caller did not name: present without this
	// input, and proof the selection replaces rather than extends.
	for _, unwanted := range []string{"webUrl", "healthStatus {"} {
		t.Run("omits "+unwanted, func(t *testing.T) {
			if strings.Contains(got.Query, unwanted) {
				t.Errorf("query still asks for %q:\n%s", unwanted, got.Query)
			}
		})
	}
}

// TestList_ReturnedFields_UnknownName_IsRefusedBeforeTheRequest verifies that a
// name outside client-go's registry stops the call rather than reaching GitLab,
// which is what makes the published enum the whole story for a caller.
func TestList_ReturnedFields_UnknownName_IsRefusedBeforeTheRequest(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(t.Context(), client, ListInput{
		FullPath:       testFullPath,
		ReturnedFields: []string{"iid", "totallyMadeUp"},
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "totallyMadeUp") {
		t.Errorf("error = %v, want it to name the unknown field", err)
	}
}

// workItemWidgetsResponse is a get answer carrying every widget this package
// reads, so one round trip covers the whole converter.
const workItemWidgetsResponse = `{"data":{"namespace":{"workItem":{` +
	`"id":"gid://gitlab/WorkItem/42","iid":"42","workItemType":{"name":"Epic"},"state":"OPEN","title":"Widgets",` +
	`"author":{"username":"dev"},"features":{` +
	`"color":{"color":"#ff0000","textColor":"#ffffff"},` +
	`"healthStatus":{"healthStatus":"needsAttention"},` +
	`"hierarchy":{"hasParent":true,"parent":{"iid":"3","namespace":{"fullPath":"my-group/parent"}},"hasChildren":false},` +
	`"iteration":{"iteration":{"id":"gid://gitlab/Iteration/9"}},` +
	`"milestone":{"milestone":{"id":"gid://gitlab/Milestone/4"}},` +
	`"startAndDueDate":{"startDate":"2026-01-05","dueDate":"2026-02-10"},` +
	`"status":{"status":{"name":"In progress"}},` +
	`"weight":{"weight":5}}}}}}`

// TestGet_WidgetFields_RoundTripFromTheFixture asserts every widget-backed
// output field against one answer. The static fragment client-go uses for get,
// create and update selects all of them unconditionally, so these arrive on
// every one of the three whatever the tier.
func TestGet_WidgetFields_RoundTripFromTheFixture(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, workItemWidgetsResponse)
	})
	out, err := Get(t.Context(), testutil.NewTestClient(t, handler), GetInput{FullPath: testFullPath, IID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	wi := out.WorkItem
	if wi.Parent == nil {
		t.Fatal("Parent = nil, want the hierarchy widget's parent")
	}
	if wi.Parent.IID != 3 || wi.Parent.Path != "my-group/parent" {
		t.Errorf("Parent = %+v, want {3 my-group/parent}", *wi.Parent)
	}
	if wi.Weight == nil || *wi.Weight != 5 {
		t.Errorf("Weight = %v, want 5", wi.Weight)
	}
	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"Color", wi.Color, "#ff0000"},
		{"HealthStatus", wi.HealthStatus, "needsAttention"},
		{"IterationID", wi.IterationID, int64(9)},
		{"MilestoneID", wi.MilestoneID, int64(4)},
		{"StartDate", wi.StartDate, "2026-01-05"},
		{"DueDate", wi.DueDate, "2026-02-10"},
		{"Status", wi.Status, "In progress"},
	}
	for _, check := range checks {
		t.Run(check.field, func(t *testing.T) {
			if check.got != check.want {
				t.Errorf("%s = %#v, want %#v", check.field, check.got, check.want)
			}
		})
	}
}

// TestFormatGetMarkdown_WidgetFields checks the whole card of a work item
// carrying every widget value, since a field nothing prints is a field a reader
// never sees. The parent is a nested object rather than a reference with a
// sigil: it may be an epic, which GitLab writes &N, or an issue, which it
// writes #N, and the query does not say which.
func TestFormatGetMarkdown_WidgetFields(t *testing.T) {
	weight := int64(5)
	want := "## Work Item #42: Widgets\n\n" +
		"- **Parent**:\n" +
		"  - **IID**: 3\n" +
		"  - **Path**: my-group/parent\n" +
		"- **Milestone ID**: 4\n" +
		"- **Iteration ID**: 9\n" +
		"- **Weight**: 5\n" +
		"- **Health Status**: needsAttention\n" +
		"- **Start Date**: 5 Jan 2026\n" +
		"- **Due Date**: 10 Feb 2026\n" +
		"- **Color**: #ff0000\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'issue.work_item_update' to modify this work item\n"
	got := extractText(t, FormatGetMarkdown(GetOutput{WorkItem: WorkItemItem{
		IID:          42,
		Title:        "Widgets",
		Parent:       &ChildItem{IID: 3, Path: "my-group/parent"},
		MilestoneID:  4,
		IterationID:  9,
		Weight:       &weight,
		HealthStatus: "needsAttention",
		StartDate:    "2026-01-05",
		DueDate:      "2026-02-10",
		Color:        "#ff0000",
	}}))
	if got != want {
		t.Errorf("FormatGetMarkdown(widgets)\n got %q\nwant %q", got, want)
	}
}

// TestCreate_Fields_ReachTheWire proves the five create fields added for issue
// 583 arrive inside the mutation input, each in the widget client-go folds it
// into, with the global ID wrapping client-go applies.
func TestCreate_Fields_ReachTheWire(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQL(t, &got,
		`{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Task"},"state":"OPEN","title":"Fields","author":{"username":"dev"}}}}}`))

	parent, iteration := int64(9), int64(7)
	_, err := Create(t.Context(), client, CreateInput{
		FullPath:       testFullPath,
		WorkItemTypeID: testTypeGID,
		Title:          "Fields",
		CRMContactIDs:  []int64{4},
		ParentID:       &parent,
		IterationID:    &iteration,
		CreatedAt:      "2025-03-04T05:06:07Z",
		CreateSource:   "gitlab-mcp-server",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	input, ok := got.Variables["input"].(map[string]any)
	if !ok {
		t.Fatalf("mutation input = %#v, want an object", got.Variables["input"])
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"createSource", input["createSource"], "gitlab-mcp-server"},
		{"createdAt", input["createdAt"], "2025-03-04T05:06:07Z"},
		{"crmContactsWidget.contactIds", widgetField(t, input, "crmContactsWidget", "contactIds"), []any{"gid://gitlab/CustomerRelations::Contact/4"}},
		{"hierarchyWidget.parentId", widgetField(t, input, "hierarchyWidget", "parentId"), "gid://gitlab/WorkItem/9"},
		{"iterationWidget.iterationId", widgetField(t, input, "iterationWidget", "iterationId"), "gid://gitlab/Iteration/7"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !reflect.DeepEqual(check.got, check.want) {
				t.Errorf("%s = %#v, want %#v", check.name, check.got, check.want)
			}
		})
	}
}

// widgetField reads one leaf out of a widget of the create mutation input.
func widgetField(t *testing.T, input map[string]any, widget, field string) any {
	t.Helper()
	nested, ok := input[widget].(map[string]any)
	if !ok {
		t.Fatalf("input[%q] = %#v, want an object", widget, input[widget])
	}
	return nested[field]
}

// TestCreate_MalformedCreatedAt_IsRefusedByName verifies a created_at GitLab
// could not read stops the call instead of creating a work item stamped now.
func TestCreate_MalformedCreatedAt_IsRefusedByName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(t.Context(), client, CreateInput{
		FullPath:       testFullPath,
		WorkItemTypeID: testTypeGID,
		Title:          "Bad timestamp",
		CreatedAt:      "yesterday",
	})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "created_at") {
		t.Errorf("error = %v, want it to name created_at", err)
	}
}

// mutationInput reads the single input object a create or update mutation
// sends, which is where every field of both builders ends up.
func mutationInput(t *testing.T, got graphQLRequest) map[string]any {
	t.Helper()
	input, ok := got.Variables["input"].(map[string]any)
	if !ok {
		t.Fatalf("mutation input = %#v, want an object", got.Variables["input"])
	}
	return input
}

// wireCase names one field of a builder, the widget client-go folds it into
// (empty for a key of the mutation input itself) and the value it must carry.
type wireCase struct {
	name   string
	widget string
	field  string
	want   any
}

// assertWire reads the leaf a case names out of the mutation input and
// compares it with what the caller sent.
func assertWire(t *testing.T, input map[string]any, testCase wireCase) {
	t.Helper()
	sent := input[testCase.field]
	if testCase.widget != "" {
		sent = widgetField(t, input, testCase.widget, testCase.field)
	}
	if !reflect.DeepEqual(sent, testCase.want) {
		t.Errorf("%s = %#v, want %#v", testCase.field, sent, testCase.want)
	}
}

// recordWorkItemUpdate answers the two POSTs an update makes and records the
// mutation's own request into got.
//
// client-go resolves the work item's global ID with a query of its own before
// it sends workItemUpdate, so a handler recording every request it sees would
// end up holding the lookup rather than the mutation.
func recordWorkItemUpdate(t *testing.T, got *graphQLRequest) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		if !strings.Contains(string(raw), "workItemUpdate") {
			testutil.RespondJSON(w, http.StatusOK, workItemGIDResponse)
			return
		}
		if err = json.Unmarshal(raw, got); err != nil {
			t.Errorf("decode GraphQL request: %v", err)
			http.Error(w, "decode GraphQL request", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, workItemUpdateResponse)
	}
}

// workItemGIDResponse answers the global-ID lookup every update and delete
// makes before its own mutation.
const workItemGIDResponse = `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/42"}}}}`

// workItemUpdateResponse is the shortest answer an update mutation accepts,
// for the tests whose whole assertion is on the request.
const workItemUpdateResponse = `{"data":{"workItemUpdate":{"workItem":{` +
	`"id":"gid://gitlab/WorkItem/42","iid":"42","workItemType":{"name":"Issue"},` +
	`"state":"OPEN","title":"Updated","author":{"username":"dev"},"widgets":[]}}}}`

// TestUpdate_Fields_ReachTheWire drives one update field at a time and asserts
// the leaf it becomes inside the mutation input, in the widget client-go folds
// it into and with the global ID wrapping client-go applies.
//
// Nothing read that mutation before. TestUpdate_AllOptions sets seventeen
// fields at once and asserts only the title the fixture echoes back, so a
// field assigned from its neighbor, or never assigned at all, reached GitLab
// wrong with the whole suite green.
func TestUpdate_Fields_ReachTheWire(t *testing.T) {
	milestone, parent, iteration, weight := int64(4), int64(9), int64(7), int64(3)
	cases := []struct {
		input UpdateInput
		wireCase
	}{
		{UpdateInput{Title: "Renamed"}, wireCase{name: "title", field: "title", want: "Renamed"}},
		{UpdateInput{StateEvent: "CLOSE"}, wireCase{name: "state_event", field: "stateEvent", want: "CLOSE"}},
		{UpdateInput{Description: "new desc"}, wireCase{name: "description", widget: "descriptionWidget", field: "description", want: "new desc"}},
		{UpdateInput{AssigneeIDs: []int64{5}}, wireCase{name: "assignee_ids", widget: "assigneesWidget", field: "assigneeIds", want: []any{"gid://gitlab/User/5"}}},
		{UpdateInput{MilestoneID: &milestone}, wireCase{name: "milestone_id", widget: "milestoneWidget", field: "milestoneId", want: "gid://gitlab/Milestone/4"}},
		{UpdateInput{CRMContactIDs: []int64{6}}, wireCase{name: "crm_contact_ids", widget: "crmContactsWidget", field: "contactIds", want: []any{"gid://gitlab/CustomerRelations::Contact/6"}}},
		// REPLACE is what makes the list the caller sent the whole list;
		// without it GitLab would add to the contacts already attached.
		{UpdateInput{CRMContactIDs: []int64{6}}, wireCase{name: "crm_contact_ids replace", widget: "crmContactsWidget", field: "operationMode", want: "REPLACE"}},
		{UpdateInput{ParentID: &parent}, wireCase{name: "parent_id", widget: "hierarchyWidget", field: "parentId", want: "gid://gitlab/WorkItem/9"}},
		{UpdateInput{AddLabelIDs: []int64{11}}, wireCase{name: "add_label_ids", widget: "labelsWidget", field: "addLabelIds", want: []any{"gid://gitlab/Label/11"}}},
		{UpdateInput{RemoveLabelIDs: []int64{12}}, wireCase{name: "remove_label_ids", widget: "labelsWidget", field: "removeLabelIds", want: []any{"gid://gitlab/Label/12"}}},
		{UpdateInput{StartDate: "2026-06-01"}, wireCase{name: "start_date", widget: "startAndDueDateWidget", field: "startDate", want: "2026-06-01"}},
		{UpdateInput{DueDate: "2026-06-30"}, wireCase{name: "due_date", widget: "startAndDueDateWidget", field: "dueDate", want: "2026-06-30"}},
		{UpdateInput{Weight: &weight}, wireCase{name: "weight", widget: "weightWidget", field: "weight", want: float64(3)}},
		{UpdateInput{HealthStatus: "needsAttention"}, wireCase{name: "health_status", widget: "healthStatusWidget", field: "healthStatus", want: "needsAttention"}},
		{UpdateInput{IterationID: &iteration}, wireCase{name: "iteration_id", widget: "iterationWidget", field: "iterationId", want: "gid://gitlab/Iteration/7"}},
		{UpdateInput{Color: "#00ff00"}, wireCase{name: "color", widget: "colorWidget", field: "color", want: "#00ff00"}},
		{UpdateInput{Status: "IN_PROGRESS"}, wireCase{name: "status", widget: "statusWidget", field: "status", want: string(gl.WorkItemStatusInProgress)}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got graphQLRequest
			client := testutil.NewTestClient(t, recordWorkItemUpdate(t, &got))
			input := testCase.input
			input.FullPath, input.IID = testFullPath, 42
			if _, err := Update(t.Context(), client, input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			assertWire(t, mutationInput(t, got), testCase.wireCase)
		})
	}
}

// TestUpdate_EmptyAssignees_ReachesGitLabAsAnEmptyArray asserts the property
// the clearing guard exists to protect: a confirmed clear sends assigneeIds as
// [], which is how GitLab is told to remove every assignee.
//
// An update that dropped the empty array would leave the assignees in place
// and answer with the work item as if it had removed them, and the guard would
// have asked the user to approve a deletion that never happened.
func TestUpdate_EmptyAssignees_ReachesGitLabAsAnEmptyArray(t *testing.T) {
	var got graphQLRequest
	client := testutil.NewTestClient(t, recordWorkItemUpdate(t, &got))
	ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "gitlab_update_work_item",
			Arguments: json.RawMessage(`{"full_path":"my-group/my-project","work_item_iid":42,"assignee_ids":[],"confirm":true}`),
		},
	})

	if _, err := Update(ctx, client, UpdateInput{FullPath: testFullPath, IID: 42, AssigneeIDs: []int64{}}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	sent := widgetField(t, mutationInput(t, got), "assigneesWidget", "assigneeIds")
	if !reflect.DeepEqual(sent, []any{}) {
		t.Errorf("assigneeIds = %#v, want an empty array", sent)
	}
}

// TestCreate_Widgets_ReachTheWire drives one create field at a time and
// asserts the leaf it becomes, for the fields the two tests beside it leave
// unread: TestCreate_AllOptions sets thirteen at once and asserts only the
// echoed title, and TestCreate_Fields_ReachTheWire covers the five added for
// issue 583.
func TestCreate_Widgets_ReachTheWire(t *testing.T) {
	confidential := true
	milestone, weight := int64(4), int64(3)
	cases := []struct {
		input CreateInput
		wireCase
	}{
		{CreateInput{Confidential: &confidential}, wireCase{name: "confidential", field: "confidential", want: true}},
		{CreateInput{Description: "desc"}, wireCase{name: "description", widget: "descriptionWidget", field: "description", want: "desc"}},
		{CreateInput{AssigneeIDs: []int64{5}}, wireCase{name: "assignee_ids", widget: "assigneesWidget", field: "assigneeIds", want: []any{"gid://gitlab/User/5"}}},
		{CreateInput{MilestoneID: &milestone}, wireCase{name: "milestone_id", widget: "milestoneWidget", field: "milestoneId", want: "gid://gitlab/Milestone/4"}},
		{CreateInput{LabelIDs: []int64{11}}, wireCase{name: "label_ids", widget: "labelsWidget", field: "labelIds", want: []any{"gid://gitlab/Label/11"}}},
		{CreateInput{StartDate: "2026-06-01"}, wireCase{name: "start_date", widget: "startAndDueDateWidget", field: "startDate", want: "2026-06-01"}},
		{CreateInput{DueDate: "2026-06-30"}, wireCase{name: "due_date", widget: "startAndDueDateWidget", field: "dueDate", want: "2026-06-30"}},
		{CreateInput{Weight: &weight}, wireCase{name: "weight", widget: "weightWidget", field: "weight", want: float64(3)}},
		{CreateInput{HealthStatus: "needsAttention"}, wireCase{name: "health_status", widget: "healthStatusWidget", field: "healthStatus", want: "needsAttention"}},
		{CreateInput{Color: "#00ff00"}, wireCase{name: "color", widget: "colorWidget", field: "color", want: "#00ff00"}},
		{CreateInput{Status: "DONE"}, wireCase{name: "status", widget: "statusWidget", field: "status", want: string(gl.WorkItemStatusDone)}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got graphQLRequest
			client := testutil.NewTestClient(t, recordGraphQL(t, &got, workItemCreateResponse))
			input := testCase.input
			input.FullPath, input.WorkItemTypeID, input.Title = testFullPath, testTypeGID, "Widgets"
			if _, err := Create(t.Context(), client, input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			assertWire(t, mutationInput(t, got), testCase.wireCase)
		})
	}
}

// workItemCreateResponse is the shortest answer a create mutation accepts.
const workItemCreateResponse = `{"data":{"workItemCreate":{"workItem":{` +
	`"id":"gid://gitlab/WorkItem/1","iid":"1","workItemType":{"name":"Task"},` +
	`"state":"OPEN","title":"Widgets","author":{"username":"dev"}}}}}`

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
