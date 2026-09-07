// action_specs_test.go contains canonical-route tests for work item actions.
package workitems

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionSpecWorkItemGraphQLResponse       = `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"ActionSpec test","author":{"username":"dev"},"widgets":[]}}}}`
	actionSpecWorkItemsListGraphQLResponse  = `{"data":{"namespace":{"workItems":{"nodes":[{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"ActionSpec test","author":{"username":"dev"},"widgets":[]}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`
	actionSpecWorkItemCreateGraphQLResponse = `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"ActionSpec test","author":{"username":"dev"},"widgets":[]}}}}`
	actionSpecWorkItemUpdateGraphQLResponse = `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/10","iid":"10","workItemType":{"name":"Issue"},"state":"OPEN","title":"ActionSpec updated","author":{"username":"dev"},"widgets":[]}}}}`
	actionSpecWorkItemDeleteGraphQLResponse = `{"data":{"workItemDelete":{"errors":[]}}}`
)

// TestActionSpecs_CallAllRoutes exercises every work item tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := workItemSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, workItemActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_get_work_item", map[string]any{"full_path": testProjectPath, "work_item_iid": 10}},
		{"gitlab_list_work_items", map[string]any{"full_path": testProjectPath}},
		{"gitlab_create_work_item", map[string]any{"full_path": testProjectPath, "work_item_type_id": testTypeGID, "title": "Test"}},
		{"gitlab_update_work_item", map[string]any{"full_path": testProjectPath, "work_item_iid": 10, "title": "Updated"}},
		{"gitlab_delete_work_item", map[string]any{"full_path": testProjectPath, "work_item_iid": 10}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
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

// TestActionSpecs_WorkItemListSortEnum verifies the schema gitlab_list_work_items
// serves publishes exactly the WorkItemSort values the pinned GitLab schema
// declares, and neither half of the asc/desc pair.
//
// Both halves matter, and they fail in opposite directions. asc and desc are
// what toolutil injects into any string sort carrying no enum of its own, and
// GitLab refuses both here; the values GitLab does accept would then be refused
// by go-sdk before the request is built, since it validates arguments against
// the published schema. The expectation is read out of the pin rather than
// written down again, so a re-pin that changes the enum fails here instead of
// leaving two lists to drift apart.
func TestActionSpecs_WorkItemListSortEnum(t *testing.T) {
	byTool := workItemSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, workItemActionHandler())))
	served := servedEnum(t, byTool["gitlab_list_work_items"].Route.InputSchema, "sort")

	schema, err := graphqlschema.Schema()
	if err != nil {
		t.Fatalf("graphqlschema.Schema() error: %v", err)
	}
	definition, ok := schema.Types["WorkItemSort"]
	if !ok {
		t.Fatal("pinned schema declares no WorkItemSort enum")
	}
	// The pin still carries the lowercase aliases GitLab renamed in 13.5, and
	// the override leaves them out on purpose: they work, and offering both
	// spellings would double the list. The pin records no deprecation, so the
	// spelling is what separates them, and the aliases are collected rather
	// than skipped so a new one cannot slip past unnoticed.
	var want, aliases []string
	for _, value := range definition.EnumValues {
		if value.Name != strings.ToUpper(value.Name) {
			aliases = append(aliases, value.Name)
			continue
		}
		want = append(want, value.Name)
	}
	slices.Sort(aliases)
	if !slices.Equal(aliases, []string{"created_asc", "created_desc", "updated_asc", "updated_desc"}) {
		t.Errorf("pinned WorkItemSort aliases = %v, want the four renamed in 13.5", aliases)
	}

	slices.Sort(served)
	slices.Sort(want)
	if !slices.Equal(served, want) {
		t.Errorf("served sort enum = %v, want the pinned WorkItemSort values %v", served, want)
	}
	for _, rejected := range []string{"asc", "desc"} {
		t.Run(rejected, func(t *testing.T) {
			if slices.Contains(served, rejected) {
				t.Errorf("served sort enum offers %q, which GitLab refuses for WorkItemSort", rejected)
			}
		})
	}
}

// servedEnum returns the enum values a built input schema publishes for one
// top-level property, which is what a client is offered after the action's
// overrides and the canonical parameter enums have both been applied.
func servedEnum(t *testing.T, schema map[string]any, property string) []string {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("input schema has no properties: %v", schema)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("input schema has no %q property", property)
	}
	enum, ok := field["enum"].([]any)
	if !ok {
		t.Fatalf("%q publishes no enum: %v", property, field)
	}
	values := make([]string, 0, len(enum))
	for _, value := range enum {
		text, isText := value.(string)
		if !isText {
			t.Fatalf("%q enum carries a non-string value %v", property, value)
		}
		values = append(values, text)
	}
	return values
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := workItemSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, workItemActionHandler())))

	result, err := byTool["gitlab_delete_work_item"].Route.Handler(t.Context(), map[string]any{"full_path": testProjectPath, "work_item_iid": 10})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_delete_work_item) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_delete_work_item) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted work item #10 from ns/proj." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_DeleteError verifies delete route failures propagate.
func TestActionSpecs_DeleteError(t *testing.T) {
	byTool := workItemSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusForbidden, `{"errors":[{"message":"server error"}]}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	_, err := byTool["gitlab_delete_work_item"].Route.Handler(t.Context(), map[string]any{"full_path": testProjectPath, "work_item_iid": 10})
	if err == nil {
		t.Fatal("expected route error")
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := workItemSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_delete_work_item"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test work item destructive confirmation.",
		Icons:       toolutil.IconIssue,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_delete_work_item",
		Arguments: map[string]any{"full_path": testProjectPath, "work_item_iid": 10},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// TestActionSpecs_ListFilters_PublishTheirEnums verifies the served schema
// carries the closed value set of every enum and wildcard filter.
//
// Nothing injects these centrally: toolutil's canonical enums reach the
// property literally named sort and no other. Without them a model guesses
// "none" or "any", GitLab matches the enum case sensitively, and the document
// is refused before anything executes with a message the caller cannot act on.
func TestActionSpecs_ListFilters_PublishTheirEnums(t *testing.T) {
	props := workItemToolProperties(t, "gitlab_list_work_items")

	cases := []struct {
		property string
		want     []any
	}{
		{"assignee_wildcard_id", []any{"ANY", "ME", "NONE"}},
		{"health_status_filter", []any{"ANY", "NONE", "atRisk", "needsAttention", "onTrack"}},
		{"iteration_wildcard_id", []any{"ANY", "CURRENT", "NONE"}},
		{"milestone_wildcard_id", []any{"ANY", "NONE", "STARTED", "UPCOMING"}},
		{"release_tag_wildcard_id", []any{"ANY", "NONE"}},
		{"subscribed", []any{"EXPLICITLY_SUBSCRIBED", "EXPLICITLY_UNSUBSCRIBED"}},
		{"weight_wildcard_id", []any{"ANY", "NONE"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.property, func(t *testing.T) {
			property := schemaProperty(t, props, testCase.property)
			if got := property["enum"]; !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("%s enum = %#v, want %#v", testCase.property, got, testCase.want)
			}
		})
	}

	t.Run("in publishes its enum on the array items", func(t *testing.T) {
		items, ok := schemaProperty(t, props, "in")["items"].(map[string]any)
		if !ok {
			t.Fatal("in has no items schema")
		}
		want := []any{"DESCRIPTION", "TITLE"}
		if got := items["enum"]; !reflect.DeepEqual(got, want) {
			t.Errorf("in items enum = %#v, want %#v", got, want)
		}
	})
}

// TestActionSpecs_TimeFilters_PublishOneFormat holds the eight date-range
// filters to a single treatment. toolutil injects date-time into four of them
// by name, and without the overrides beside them the other four would publish
// no format at all, leaving one input struct advertising sibling filters as
// two different things while the handler parses all eight the same way.
func TestActionSpecs_TimeFilters_PublishOneFormat(t *testing.T) {
	props := workItemToolProperties(t, "gitlab_list_work_items")

	for _, name := range []string{
		"closed_after", "closed_before", "created_after", "created_before",
		"due_after", "due_before", "updated_after", "updated_before",
	} {
		t.Run(name, func(t *testing.T) {
			if got := schemaProperty(t, props, name)["format"]; got != "date-time" {
				t.Errorf("%s format = %#v, want date-time", name, got)
			}
		})
	}
}

// TestActionSpecs_CreateCreatedAt_PublishesDateTime covers the one create
// field toolutil does not know by name, beside two date-only siblings.
func TestActionSpecs_CreateCreatedAt_PublishesDateTime(t *testing.T) {
	props := workItemToolProperties(t, "gitlab_create_work_item")
	if got := schemaProperty(t, props, "created_at")["format"]; got != "date-time" {
		t.Errorf("created_at format = %#v, want date-time", got)
	}
}

// workItemToolProperties returns the served input-schema properties of one
// work item tool, with every override and canonical injection applied.
func workItemToolProperties(t *testing.T, tool string) map[string]any {
	t.Helper()
	byTool := workItemSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t))))
	spec, ok := byTool[tool]
	if !ok {
		t.Fatalf("no spec for %q", tool)
	}
	props, ok := spec.Route.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s input schema has no properties", tool)
	}
	return props
}

// schemaProperty returns one property schema, failing rather than returning a
// nil map a later assertion would read as an absent enum.
func schemaProperty(t *testing.T, props map[string]any, name string) map[string]any {
	t.Helper()
	property, ok := props[name].(map[string]any)
	if !ok {
		t.Fatalf("property %q = %#v, want an object", name, props[name])
	}
	return property
}

func workItemActionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"errors":[{"message":"bad body"}]}`)
			return
		}
		body := string(bodyBytes)

		switch {
		case strings.Contains(body, "workItemCreate"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemCreateGraphQLResponse)
		case strings.Contains(body, "workItemUpdate"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemUpdateGraphQLResponse)
		case strings.Contains(body, "workItemDelete"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemDeleteGraphQLResponse)
		case strings.Contains(body, "workItems"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemsListGraphQLResponse)
		case strings.Contains(body, "workItem"):
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemGraphQLResponse)
		default:
			testutil.RespondJSON(w, http.StatusOK, actionSpecWorkItemGraphQLResponse)
		}
	})
}

func workItemSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
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
