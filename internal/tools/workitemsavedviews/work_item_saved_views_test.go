// work_item_saved_views_test.go contains unit tests for GitLab work item saved
// view operations.
//
// The tests mock the GitLab GraphQL query and mutations with
// [testutil.GraphQLHandler], then call the handlers directly to verify the
// request payloads, input validation, error wrapping, and output conversion.
package workitemsavedviews

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// savedViewNode is the GraphQL node shape the query and every mutation return.
const savedViewNode = `{
	"id": "gid://gitlab/WorkItems::SavedViews::SavedView/7",
	"name": "My open tasks",
	"description": "Everything assigned to me",
	"isPrivate": true,
	"subscribed": false,
	"filters": {"assigneeUsernames": ["alice"]},
	"sort": "CREATED_DESC",
	"displaySettings": {"viewMode": "board"}
}`

// mutationInput parses the GraphQL "input" variable from r and records a test
// failure if it is absent. It runs inside the httptest server goroutine, so it
// reports with t.Errorf and returns nil rather than aborting.
func mutationInput(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	vars, err := testutil.ParseGraphQLVariables(r)
	if err != nil {
		t.Errorf("ParseGraphQLVariables error: %v", err)
		return nil
	}
	input, ok := vars["input"].(map[string]any)
	if !ok {
		t.Errorf("GraphQL input = %#v, want map", vars["input"])
		return nil
	}
	return input
}

// TestGet_Success verifies that Get sends the namespace path and the saved
// view's global ID, and decodes both opaque JSON scalars into structured data.
func TestGet_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"savedViews(id:": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Errorf("ParseGraphQLVariables error: %v", err)
				return
			}
			if got := vars["fullPath"]; got != "my-group" {
				t.Errorf("fullPath = %v, want my-group", got)
			}
			if got := vars["id"]; got != "gid://gitlab/WorkItems::SavedViews::SavedView/7" {
				t.Errorf("id = %v, want the saved view global ID", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"savedViews":{"nodes":[`+savedViewNode+`]}}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Get(context.Background(), client, GetInput{NamespacePath: " my-group ", SavedViewID: 7})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.NamespacePath != "my-group" {
		t.Errorf("NamespacePath = %q, want the trimmed path", out.NamespacePath)
	}
	if out.SavedView.ID != 7 || out.SavedView.Name != "My open tasks" || out.SavedView.Sort != "CREATED_DESC" {
		t.Errorf("SavedView = %+v", out.SavedView)
	}
	if out.SavedView.GID != "gid://gitlab/WorkItems::SavedViews::SavedView/7" {
		t.Errorf("GID = %q", out.SavedView.GID)
	}
	filters, ok := out.SavedView.Filters.(map[string]any)
	if !ok {
		t.Fatalf("Filters = %#v, want a decoded object", out.SavedView.Filters)
	}
	if _, ok = filters["assigneeUsernames"]; !ok {
		t.Errorf("Filters = %#v, want assigneeUsernames", filters)
	}
	if settings, isMap := out.SavedView.DisplaySettings.(map[string]any); !isMap || settings["viewMode"] != "board" {
		t.Errorf("DisplaySettings = %#v", out.SavedView.DisplaySettings)
	}
}

// TestGet_NotFound verifies that an empty namespace result is reported with the
// hint naming the two identifiers most likely to be wrong, rather than as a bare
// SDK sentinel.
func TestGet_NotFound(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"savedViews(id:": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"savedViews":{"nodes":[]}}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Get(context.Background(), client, GetInput{NamespacePath: "my-group", SavedViewID: 404})
	if err == nil {
		t.Fatal("Get() error = nil, want a not-found error")
	}
	if !strings.Contains(err.Error(), "saved_view_id") {
		t.Errorf("Get() error = %q, want the identifier hint", err)
	}
}

// TestGet_Validation verifies that a missing namespace path or a non-positive
// saved view ID is rejected before any request is dispatched.
func TestGet_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input GetInput
		want  string
	}{
		{name: "empty namespace", input: GetInput{SavedViewID: 1}, want: "namespace_path"},
		{name: "blank namespace", input: GetInput{NamespacePath: "   ", SavedViewID: 1}, want: "namespace_path"},
		{name: "zero id", input: GetInput{NamespacePath: "g"}, want: "saved_view_id"},
		{name: "negative id", input: GetInput{NamespacePath: "g", SavedViewID: -3}, want: "saved_view_id"},
	}
	client := testutil.NewTestClient(t, refusingHandler(t))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Get(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Get() error = %v, want one naming %s", err, tc.want)
			}
		})
	}
}

// TestList_Success verifies that List forwards the namespace path, defaults the
// page size, and copies the connection's page info into the output.
func TestList_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"savedViews(first:": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Errorf("ParseGraphQLVariables error: %v", err)
				return
			}
			if got := vars["fullPath"]; got != "my-group" {
				t.Errorf("fullPath = %v, want my-group", got)
			}
			if got := vars["first"]; got != float64(toolutil.GraphQLDefaultFirst) {
				t.Errorf("first = %v, want the default page size", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"savedViews":{
				"nodes":[`+savedViewNode+`],
				"pageInfo":{"endCursor":"CURSOR","hasNextPage":true,"startCursor":"START","hasPreviousPage":false}
			}}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := List(context.Background(), client, ListInput{NamespacePath: "my-group"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.SavedViews) != 1 || out.SavedViews[0].ID != 7 {
		t.Fatalf("SavedViews = %+v", out.SavedViews)
	}
	if !out.Pagination.HasNextPage || out.Pagination.EndCursor != "CURSOR" || out.Pagination.StartCursor != "START" {
		t.Errorf("Pagination = %+v", out.Pagination)
	}
}

// TestList_PaginationSelection verifies that the caller's page-size choice
// decides which of the SDK's mutually exclusive first/last arguments is sent:
// the SDK ignores Last whenever First is set, so a request that only asked to
// page backward must not carry First at all.
func TestList_PaginationSelection(t *testing.T) {
	cases := []struct {
		name      string
		first     *int
		last      *int
		wantVar   string
		wantValue float64
		absentVar string
	}{
		{name: "explicit first", first: new(5), wantVar: "first", wantValue: 5, absentVar: "last"},
		{name: "first clamped to maximum", first: new(5000), wantVar: "first", wantValue: float64(toolutil.GraphQLMaxFirst), absentVar: "last"},
		{name: "last only", last: new(3), wantVar: "last", wantValue: 3, absentVar: "first"},
		{name: "last clamped to minimum", last: new(0), wantVar: "last", wantValue: 1, absentVar: "first"},
		{name: "last clamped to maximum", last: new(5000), wantVar: "last", wantValue: float64(toolutil.GraphQLMaxFirst), absentVar: "first"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
				"savedViews(first:": func(w http.ResponseWriter, r *http.Request) {
					vars, err := testutil.ParseGraphQLVariables(r)
					if err != nil {
						t.Errorf("ParseGraphQLVariables error: %v", err)
						return
					}
					if got := vars[tc.wantVar]; got != tc.wantValue {
						t.Errorf("%s = %v, want %v", tc.wantVar, got, tc.wantValue)
					}
					if got, ok := vars[tc.absentVar]; ok {
						t.Errorf("%s = %v, want it absent", tc.absentVar, got)
					}
					testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"savedViews":{"nodes":[]}}}`)
				},
			})
			client := testutil.NewTestClient(t, handler)

			input := ListInput{
				NamespacePath: "my-group",
				First:         tc.first,
				Last:          tc.last,
			}
			if _, err := List(context.Background(), client, input); err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}
		})
	}
}

// TestList_Cursors verifies that the after and before cursors reach the query
// only when the caller supplied them.
func TestList_Cursors(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"savedViews(first:": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Errorf("ParseGraphQLVariables error: %v", err)
				return
			}
			if got := vars["after"]; got != "AFTER" {
				t.Errorf("after = %v, want AFTER", got)
			}
			if got := vars["before"]; got != "BEFORE" {
				t.Errorf("before = %v, want BEFORE", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"savedViews":{"nodes":[]}}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	input := ListInput{
		NamespacePath: "my-group",
		After:         "AFTER",
		Before:        "BEFORE",
	}
	if _, err := List(context.Background(), client, input); err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
}

// TestList_Errors verifies that a missing namespace path is rejected locally and
// that an unknown namespace is wrapped with the identifier hint.
func TestList_Errors(t *testing.T) {
	client := testutil.NewTestClient(t, refusingHandler(t))
	if _, err := List(context.Background(), client, ListInput{}); err == nil ||
		!strings.Contains(err.Error(), "namespace_path") {
		t.Errorf("List() error = %v, want one naming namespace_path", err)
	}

	missing := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"savedViews(first:": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":null}`)
		},
	}))
	if _, err := List(context.Background(), missing, ListInput{NamespacePath: "nope"}); err == nil ||
		!strings.Contains(err.Error(), "namespace_path") {
		t.Errorf("List() error = %v, want the not-found hint", err)
	}
}

// TestCreate_Success verifies that Create sends the namespace path, the
// required scalars, and a fully converted filter object.
func TestCreate_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewCreate": func(w http.ResponseWriter, r *http.Request) {
			input := mutationInput(t, r)
			if input == nil {
				return
			}
			if got := input["namespacePath"]; got != "my-group" {
				t.Errorf("namespacePath = %v, want my-group", got)
			}
			if got := input["name"]; got != "My open tasks" {
				t.Errorf("name = %v", got)
			}
			if got := input["sort"]; got != "CREATED_DESC" {
				t.Errorf("sort = %v", got)
			}
			settings, ok := input["displaySettings"].(map[string]any)
			if !ok || settings["viewMode"] != "board" {
				t.Errorf("displaySettings = %#v", input["displaySettings"])
			}
			filters, ok := input["filters"].(map[string]any)
			if !ok {
				t.Errorf("filters = %#v, want an object", input["filters"])
				testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewCreate":{"savedView":`+savedViewNode+`,"errors":[]}}`)
				return
			}
			if got := filters["authorUsername"]; got != "alice" {
				t.Errorf("filters.authorUsername = %v", got)
			}
			if got := filters["createdAfter"]; got != "2025-01-01T00:00:00Z" {
				t.Errorf("filters.createdAfter = %v", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewCreate":{"savedView":`+savedViewNode+`,"errors":[]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Create(context.Background(), client, CreateInput{
		NamespacePath:   "my-group",
		Name:            " My open tasks ",
		Sort:            "CREATED_DESC",
		DisplaySettings: map[string]any{"viewMode": "board"},
		Filters:         &Filters{AuthorUsername: "alice", CreatedAfter: "2025-01-01"},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Status != "success" || out.SavedView.ID != 7 {
		t.Errorf("Create() output = %+v", out)
	}
	if !strings.Contains(out.Message, "My open tasks") {
		t.Errorf("Create() message = %q, want it to name the view", out.Message)
	}
}

// TestCreate_DefaultsDisplaySettings verifies that an omitted display_settings
// is sent as an empty object. The GraphQL server requires the field, and the SDK
// refuses to dispatch without it, so the caller would otherwise have to know
// that `{}` is how to say "nothing to store".
func TestCreate_DefaultsDisplaySettings(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewCreate": func(w http.ResponseWriter, r *http.Request) {
			input := mutationInput(t, r)
			if input == nil {
				return
			}
			settings, ok := input["displaySettings"].(map[string]any)
			if !ok || len(settings) != 0 {
				t.Errorf("displaySettings = %#v, want an empty object", input["displaySettings"])
			}
			if _, ok = input["filters"].(map[string]any); !ok {
				t.Errorf("filters = %#v, want an empty object rather than null", input["filters"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewCreate":{"savedView":`+savedViewNode+`,"errors":[]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := Create(context.Background(), client, CreateInput{
		NamespacePath: "my-group", Name: "Minimal", Sort: "ID",
	}); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
}

// TestCreate_Validation verifies that every locally checkable requirement is
// reported by name before a request is dispatched.
func TestCreate_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input CreateInput
		want  string
	}{
		{name: "no namespace", input: CreateInput{Name: "n", Sort: "ID"}, want: "namespace_path"},
		{name: "no name", input: CreateInput{NamespacePath: "g", Sort: "ID"}, want: "name"},
		{name: "blank name", input: CreateInput{NamespacePath: "g", Name: "  ", Sort: "ID"}, want: "name"},
		{name: "no sort", input: CreateInput{NamespacePath: "g", Name: "n"}, want: "sort"},
		{
			name:  "malformed filter timestamp",
			input: CreateInput{NamespacePath: "g", Name: "n", Sort: "ID", Filters: &Filters{DueBefore: "next tuesday"}},
			want:  "filters.due_before",
		},
	}
	client := testutil.NewTestClient(t, refusingHandler(t))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Create(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Create() error = %v, want one naming %s", err, tc.want)
			}
		})
	}
}

// TestCreate_ServerError verifies that a mutation-level error is wrapped with
// the hint naming the two inputs GitLab rejects most often.
func TestCreate_ServerError(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewCreate": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewCreate":{"savedView":null,"errors":["Name has already been taken"]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Create(context.Background(), client, CreateInput{NamespacePath: "g", Name: "n", Sort: "ID"})
	if err == nil {
		t.Fatal("Create() error = nil, want the mutation error")
	}
	if !strings.Contains(err.Error(), "WorkItemSort") {
		t.Errorf("Create() error = %q, want the sort hint", err)
	}
}

// TestUpdate_Success verifies that Update sends the saved view's global ID and
// only the fields the caller supplied.
func TestUpdate_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := mutationInput(t, r)
			if input == nil {
				return
			}
			if got := input["id"]; got != "gid://gitlab/WorkItems::SavedViews::SavedView/7" {
				t.Errorf("id = %v, want the saved view global ID", got)
			}
			if got := input["name"]; got != "Renamed" {
				t.Errorf("name = %v", got)
			}
			// Collected rather than asserted per key: this runs on the httptest
			// goroutine, where t.Run must not be called.
			var present []string
			for _, absent := range []string{"description", "sort", "filters", "displaySettings"} {
				if _, ok := input[absent]; ok {
					present = append(present, absent)
				}
			}
			if len(present) > 0 {
				t.Errorf("input carries %v, want every unset field absent", present)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewUpdate":{"savedView":`+savedViewNode+`,"errors":[]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Update(context.Background(), client, UpdateInput{SavedViewID: 7, Name: "Renamed"})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Status != "success" || out.SavedView.ID != 7 {
		t.Errorf("Update() output = %+v", out)
	}
}

// TestUpdate_ReplacesFiltersAndSettings verifies that supplied filters and
// display settings reach the mutation, since both replace the stored value.
func TestUpdate_ReplacesFiltersAndSettings(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := mutationInput(t, r)
			if input == nil {
				return
			}
			filters, ok := input["filters"].(map[string]any)
			if !ok || filters["state"] != "closed" {
				t.Errorf("filters = %#v, want state closed", input["filters"])
			}
			settings, ok := input["displaySettings"].(map[string]any)
			if !ok || settings["viewMode"] != "table" {
				t.Errorf("displaySettings = %#v", input["displaySettings"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewUpdate":{"savedView":`+savedViewNode+`,"errors":[]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Update(context.Background(), client, UpdateInput{
		SavedViewID:     7,
		Filters:         &Filters{State: "closed"},
		DisplaySettings: map[string]any{"viewMode": "table"},
	})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
}

// TestUpdate_Validation verifies that a non-positive ID and a malformed filter
// timestamp are both reported before dispatch.
func TestUpdate_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input UpdateInput
		want  string
	}{
		{name: "zero id", input: UpdateInput{}, want: "saved_view_id"},
		{
			name:  "malformed filter timestamp",
			input: UpdateInput{SavedViewID: 7, Filters: &Filters{UpdatedAfter: "yesterday"}},
			want:  "filters.updated_after",
		},
	}
	client := testutil.NewTestClient(t, refusingHandler(t))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Update(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Update() error = %v, want one naming %s", err, tc.want)
			}
		})
	}
}

// TestDelete_Success verifies that Delete sends the global ID and returns the
// standard destructive-operation confirmation.
func TestDelete_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewDelete": func(w http.ResponseWriter, r *http.Request) {
			input := mutationInput(t, r)
			if input == nil {
				return
			}
			if got := input["id"]; got != "gid://gitlab/WorkItems::SavedViews::SavedView/7" {
				t.Errorf("id = %v, want the saved view global ID", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewDelete":{"errors":[]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Delete(context.Background(), client, DeleteInput{SavedViewID: 7})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "saved view 7") {
		t.Errorf("Delete() output = %+v", out)
	}
}

// TestDelete_Errors verifies local ID validation and mutation-error wrapping.
func TestDelete_Errors(t *testing.T) {
	client := testutil.NewTestClient(t, refusingHandler(t))
	if _, err := Delete(context.Background(), client, DeleteInput{}); err == nil ||
		!strings.Contains(err.Error(), "saved_view_id") {
		t.Errorf("Delete() error = %v, want one naming saved_view_id", err)
	}

	failing := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewDelete": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewDelete":{"errors":["not authorized"]}}`)
		},
	}))
	if _, err := Delete(context.Background(), failing, DeleteInput{SavedViewID: 7}); err == nil ||
		!strings.Contains(err.Error(), "saved_view_id") {
		t.Errorf("Delete() error = %v, want the identifier hint", err)
	}
}

// TestSubscribeUnsubscribe_Success verifies that both subscription mutations
// send the global ID and return the resulting view.
func TestSubscribeUnsubscribe_Success(t *testing.T) {
	t.Run("subscribe", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
			"workItemSavedViewSubscribe": func(w http.ResponseWriter, r *http.Request) {
				input := mutationInput(t, r)
				if input == nil {
					return
				}
				if got := input["id"]; got != "gid://gitlab/WorkItems::SavedViews::SavedView/7" {
					t.Errorf("id = %v, want the saved view global ID", got)
				}
				testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewSubscribe":{"savedView":`+savedViewNode+`,"errors":[]}}`)
			},
		}))
		out, err := Subscribe(context.Background(), client, SubscribeInput{SavedViewID: 7})
		if err != nil {
			t.Fatalf("Subscribe() unexpected error: %v", err)
		}
		if out.SavedView.ID != 7 || !strings.Contains(out.Message, "subscribed to saved view 7") {
			t.Errorf("Subscribe() output = %+v", out)
		}
	})

	t.Run("unsubscribe", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
			"workItemSavedViewUnsubscribe": func(w http.ResponseWriter, r *http.Request) {
				input := mutationInput(t, r)
				if input == nil {
					return
				}
				if got := input["id"]; got != "gid://gitlab/WorkItems::SavedViews::SavedView/7" {
					t.Errorf("id = %v, want the saved view global ID", got)
				}
				testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewUnsubscribe":{"savedView":`+savedViewNode+`,"errors":[]}}`)
			},
		}))
		out, err := Unsubscribe(context.Background(), client, UnsubscribeInput{SavedViewID: 7})
		if err != nil {
			t.Fatalf("Unsubscribe() unexpected error: %v", err)
		}
		if out.SavedView.ID != 7 || !strings.Contains(out.Message, "unsubscribed from saved view 7") {
			t.Errorf("Unsubscribe() output = %+v", out)
		}
	})
}

// TestSubscribeUnsubscribe_Errors verifies local ID validation and error
// wrapping for both subscription mutations.
func TestSubscribeUnsubscribe_Errors(t *testing.T) {
	client := testutil.NewTestClient(t, refusingHandler(t))
	if _, err := Subscribe(context.Background(), client, SubscribeInput{}); err == nil ||
		!strings.Contains(err.Error(), "saved_view_id") {
		t.Errorf("Subscribe() error = %v, want one naming saved_view_id", err)
	}
	if _, err := Unsubscribe(context.Background(), client, UnsubscribeInput{}); err == nil ||
		!strings.Contains(err.Error(), "saved_view_id") {
		t.Errorf("Unsubscribe() error = %v, want one naming saved_view_id", err)
	}

	failing := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewSubscribe": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewSubscribe":{"savedView":null,"errors":["forbidden"]}}`)
		},
		"workItemSavedViewUnsubscribe": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewUnsubscribe":{"savedView":null,"errors":["forbidden"]}}`)
		},
	}))
	if _, err := Subscribe(context.Background(), failing, SubscribeInput{SavedViewID: 7}); err == nil {
		t.Error("Subscribe() error = nil, want the mutation error")
	}
	if _, err := Unsubscribe(context.Background(), failing, UnsubscribeInput{SavedViewID: 7}); err == nil {
		t.Error("Unsubscribe() error = nil, want the mutation error")
	}
}

// TestHandlers_CanceledContext verifies that every handler returns the context
// error without dispatching a request once the caller has gone away.
func TestHandlers_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, refusingHandler(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := map[string]func() error{
		"get":         func() error { _, err := Get(ctx, client, GetInput{}); return err },
		"list":        func() error { _, err := List(ctx, client, ListInput{}); return err },
		"create":      func() error { _, err := Create(ctx, client, CreateInput{}); return err },
		"update":      func() error { _, err := Update(ctx, client, UpdateInput{}); return err },
		"delete":      func() error { _, err := Delete(ctx, client, DeleteInput{}); return err },
		"subscribe":   func() error { _, err := Subscribe(ctx, client, SubscribeInput{}); return err },
		"unsubscribe": func() error { _, err := Unsubscribe(ctx, client, UnsubscribeInput{}); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, context.Canceled) {
				t.Errorf("%s error = %v, want context.Canceled", name, err)
			}
		})
	}
}

// TestToItem_NilAndUndecodableScalars verifies the two shapes the converter has
// to survive: no saved view at all, and an opaque scalar that is not valid JSON
// (which is preserved as text rather than dropped).
func TestToItem_NilAndUndecodableScalars(t *testing.T) {
	if got := toItem(nil); got.ID != 0 || got.Name != "" {
		t.Errorf("toItem(nil) = %+v, want the zero value", got)
	}
	if got := decodeRaw(nil); got != nil {
		t.Errorf("decodeRaw(nil) = %v, want nil", got)
	}
	if got := decodeRaw([]byte("not json")); got != "not json" {
		t.Errorf("decodeRaw(invalid) = %v, want the literal text", got)
	}
}

// TestEncodeDisplaySettings_Unmarshalable verifies that a value json cannot
// marshal is reported as a display_settings problem rather than a panic.
func TestEncodeDisplaySettings_Unmarshalable(t *testing.T) {
	_, err := encodeDisplaySettings(map[string]any{"bad": make(chan int)})
	if err == nil || !strings.Contains(err.Error(), "display_settings") {
		t.Errorf("encodeDisplaySettings() error = %v, want one naming display_settings", err)
	}
}

// refusingHandler fails the test if a request reaches it. It is used by the
// validation tests, which must reject before dispatch. It runs on the httptest
// goroutine, so it reports with t.Errorf and answers deterministically.
func refusingHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
}

// TestMutations_UnmarshalableDisplaySettings verifies that create and update
// both report a display_settings value json cannot marshal, rather than
// dispatching a request with a missing field.
func TestMutations_UnmarshalableDisplaySettings(t *testing.T) {
	client := testutil.NewTestClient(t, refusingHandler(t))
	bad := map[string]any{"bad": make(chan int)}

	t.Run("create", func(t *testing.T) {
		_, err := Create(context.Background(), client, CreateInput{
			NamespacePath: "g", Name: "n", Sort: "ID", DisplaySettings: bad,
		})
		if err == nil || !strings.Contains(err.Error(), "display_settings") {
			t.Errorf("Create() error = %v, want one naming display_settings", err)
		}
	})
	t.Run("update", func(t *testing.T) {
		_, err := Update(context.Background(), client, UpdateInput{SavedViewID: 7, DisplaySettings: bad})
		if err == nil || !strings.Contains(err.Error(), "display_settings") {
			t.Errorf("Update() error = %v, want one naming display_settings", err)
		}
	})
}

// TestReads_TransportError verifies that a failure that is not the SDK's
// not-found sentinel is wrapped generically, without the identifier hint that
// would misdiagnose it.
func TestReads_TransportError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	client := testutil.NewTestClient(t, handler)

	t.Run("get", func(t *testing.T) {
		_, err := Get(context.Background(), client, GetInput{NamespacePath: "g", SavedViewID: 7})
		if err == nil {
			t.Fatal("Get() error = nil, want the transport error")
		}
		if strings.Contains(err.Error(), "saved_view_id") {
			t.Errorf("Get() error = %q, should not carry the not-found hint", err)
		}
	})
	t.Run("list", func(t *testing.T) {
		_, err := List(context.Background(), client, ListInput{NamespacePath: "g"})
		if err == nil {
			t.Fatal("List() error = nil, want the transport error")
		}
		if strings.Contains(err.Error(), "saved_view_id") {
			t.Errorf("List() error = %q, should not carry the not-found hint", err)
		}
	})
}

// TestUpdate_ServerError verifies that a mutation-level failure is wrapped with
// the identifier hint, since a wrong saved_view_id is the usual cause.
func TestUpdate_ServerError(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItemSavedViewUpdate": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"workItemSavedViewUpdate":{"savedView":null,"errors":["not found"]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Update(context.Background(), client, UpdateInput{SavedViewID: 7, Name: "Renamed"})
	if err == nil {
		t.Fatal("Update() error = nil, want the mutation error")
	}
	if !strings.Contains(err.Error(), "saved_view_id") {
		t.Errorf("Update() error = %q, want the identifier hint", err)
	}
}
