package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func dynamicSchemaSession(t *testing.T, catalog *actioncatalog.Catalog) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-schema-test", Version: "0.0.1"}, nil)
	RegisterDynamicSchemaResources(server, catalog)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestDynamicSchemaIndex_ListsCanonicalActionsSorted(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_widget", BaseDomain: "widget"})
	group.SetAction(actioncatalog.Action{Name: "delete", Route: toolutil.ActionRoute{
		InputSchema: map[string]any{
			"type":       "object",
			"required":   []any{"project_id"},
			"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
		},
		Destructive: true,
	}})
	group.SetAction(actioncatalog.Action{Name: "create", Route: toolutil.ActionRoute{InputSchema: map[string]any{"type": "object"}}})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	session := dynamicSchemaSession(t, catalog)
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/dynamic/"})
	if err != nil {
		t.Fatalf("read dynamic index: %v", err)
	}

	var index DynamicSchemaIndex
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &index); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	if index.URITemplate != "gitlab://schema/dynamic/{action}" || index.ExecuteAction != "gitlab_execute_action" {
		t.Fatalf("index metadata = %+v, want dynamic template and execute action", index)
	}
	if index.ActionCount != 2 || len(index.Actions) != 2 {
		t.Fatalf("actions = %+v, want 2", index.Actions)
	}
	if index.Actions[0].ID != "widget.create" || index.Actions[1].ID != "widget.delete" {
		t.Fatalf("actions not sorted by canonical ID: %+v", index.Actions)
	}
	deleteAction := index.Actions[1]
	if deleteAction.SchemaURI != "gitlab://schema/dynamic/widget.delete" || deleteAction.MetaSchemaURI != "gitlab://schema/meta/gitlab_widget/delete" {
		t.Fatalf("delete schema URIs = %+v", deleteAction)
	}
	if !deleteAction.Destructive || len(deleteAction.RequiredParams) != 1 || deleteAction.RequiredParams[0] != "project_id" {
		t.Fatalf("delete metadata = %+v, want destructive project_id", deleteAction)
	}
}

func TestDynamicSchemaTemplate_ReturnsDynamicParamsSchema(t *testing.T) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	session := dynamicSchemaSession(t, catalog)

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/dynamic/project/delete"})
	if err == nil {
		t.Fatalf("slash-separated dynamic action URI should be invalid, got result %+v", result)
	}
	result, err = session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/dynamic/project.delete"})
	if err != nil {
		t.Fatalf("read dynamic action schema: %v", err)
	}

	var schema map[string]any
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &schema); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	if !strings.Contains(result.Contents[0].Text, "project_id") {
		t.Fatalf("schema missing project_id: %s", result.Contents[0].Text)
	}
	properties, _ := schema["properties"].(map[string]any)
	if _, hasConfirmParam := properties["confirm"]; hasConfirmParam {
		t.Fatalf("dynamic params schema should not include meta confirm param: %+v", properties)
	}
	confirmation, ok := schema["x_confirmation"].(map[string]any)
	if !ok || confirmation["location"] != "gitlab_execute_action.confirm" {
		t.Fatalf("x_confirmation = %+v, want top-level dynamic confirmation guidance", schema["x_confirmation"])
	}
}

func TestDynamicSchemaTemplate_ReturnsDefaultSchemaWhenInputSchemaMissing(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_widget", BaseDomain: "widget"})
	group.SetAction(actioncatalog.Action{Name: "ping", Route: toolutil.ActionRoute{}})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	session := dynamicSchemaSession(t, catalog)

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/dynamic/widget.ping"})
	if err != nil {
		t.Fatalf("read default dynamic action schema: %v", err)
	}
	var schema map[string]any
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &schema); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != true {
		t.Fatalf("schema = %+v, want permissive object fallback", schema)
	}
}

func TestDynamicSchemaTemplate_NotFound(t *testing.T) {
	session := dynamicSchemaSession(t, nil)

	for _, uri := range []string{
		"gitlab://schema/dynamic/unknown.action",
		"gitlab://schema/dynamic/   ",
		"gitlab://schema/dynamic/a/b",
		"unrelated://uri",
	} {
		t.Run(uri, func(t *testing.T) {
			_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
			if err == nil {
				t.Error("expected ResourceNotFoundError")
			}
		})
	}
}

func TestReadDynamicSchemaResource_RejectsInvalidURI(t *testing.T) {
	if _, err := readDynamicSchemaResource(actioncatalog.NewCatalog(), "gitlab://schema/dynamic/   "); err == nil {
		t.Fatal("expected ResourceNotFoundError")
	}
}

func TestParseDynamicSchemaURI_NormalizesAndRejectsInvalidURIs(t *testing.T) {
	if got := parseDynamicSchemaURI("gitlab://schema/dynamic/Project.Get"); got != "project.get" {
		t.Fatalf("parseDynamicSchemaURI() = %q, want project.get", got)
	}
	for _, uri := range []string{"unrelated://uri", "gitlab://schema/dynamic/", "gitlab://schema/dynamic/a/b"} {
		if got := parseDynamicSchemaURI(uri); got != "" {
			t.Fatalf("parseDynamicSchemaURI(%q) = %q, want empty", uri, got)
		}
	}
}

func TestDynamicRequiredParams_IncludesAnyOfAndOneOf(t *testing.T) {
	if got := dynamicRequiredParams(nil); got != nil {
		t.Fatalf("dynamicRequiredParams(nil) = %v, want nil", got)
	}
	if got := dedupeDynamicStrings(nil); got != nil {
		t.Fatalf("dedupeDynamicStrings(nil) = %v, want nil", got)
	}
	if got := dedupeDynamicStrings([]string{"", "branch", "branch"}); strings.Join(got, ",") != "branch" {
		t.Fatalf("dedupeDynamicStrings() = %v, want branch", got)
	}
	schema := map[string]any{
		"anyOf": []any{
			"ignored",
			map[string]any{"required": []any{"project_id"}},
		},
		"oneOf": []any{
			map[string]any{"required": []any{"branch"}},
		},
	}

	got := dynamicRequiredParams(schema)
	want := []string{"branch", "project_id"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dynamicRequiredParams() = %v, want %v", got, want)
	}
}

// TestDynamicParameterGuidanceEntry_CoversAllFields verifies the per-entry
// projection includes every populated field of toolutil.ParameterGuidance
// (semantic_role, value_source, common_confusions, example_binding) and
// omits empty ones to keep the model-facing guidance compact.
func TestDynamicParameterGuidanceEntry_CoversAllFields(t *testing.T) {
	tests := []struct {
		name           string
		item           toolutil.ParameterGuidance
		wantKeys       []string
		wantAbsentKeys []string
	}{
		{
			name: "all fields populated",
			item: toolutil.ParameterGuidance{
				SemanticRole:     "project_id",
				ValueSource:      "from URL or numeric ID",
				CommonConfusions: []string{"namespace_id", "group_id"},
				ExampleBinding:   `params.project_id:"group/project"`,
			},
			wantKeys:       []string{"semantic_role", "value_source", "common_confusions", "example_binding"},
			wantAbsentKeys: nil,
		},
		{
			name: "only semantic role",
			item: toolutil.ParameterGuidance{
				SemanticRole: "branch_name",
			},
			wantKeys:       []string{"semantic_role"},
			wantAbsentKeys: []string{"value_source", "common_confusions", "example_binding"},
		},
		{
			name: "only example binding",
			item: toolutil.ParameterGuidance{
				ExampleBinding: `params.ref:"main"`,
			},
			wantKeys:       []string{"example_binding"},
			wantAbsentKeys: []string{"semantic_role", "value_source", "common_confusions"},
		},
		{
			name:           "all fields empty produces empty entry",
			item:           toolutil.ParameterGuidance{},
			wantKeys:       nil,
			wantAbsentKeys: []string{"semantic_role", "value_source", "common_confusions", "example_binding"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := dynamicParameterGuidanceEntry(tt.item)
			if tt.wantKeys == nil {
				if len(entry) != 0 {
					t.Fatalf("entry = %+v, want empty map for all-empty input", entry)
				}
				return
			}
			for _, key := range tt.wantKeys {
				if _, ok := entry[key]; !ok {
					t.Errorf("entry missing key %q: %+v", key, entry)
				}
			}
			for _, key := range tt.wantAbsentKeys {
				if _, ok := entry[key]; ok {
					t.Errorf("entry should not include key %q: %+v", key, entry)
				}
			}
		})
	}
}

// TestDynamicParameterGuidance_FiltersEmptyEntries verifies the outer
// guidance projection drops params whose entries are entirely empty so the
// guidance map only surfaces meaningful hints to the LLM.
func TestDynamicParameterGuidance_FiltersEmptyEntries(t *testing.T) {
	action := actioncatalog.Action{
		Name: "create",
		Route: toolutil.ActionRoute{
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:   "project_id",
					ExampleBinding: `params.project_id:"group/project"`,
				},
				"unused_param": {}, // all fields empty; should be dropped
				"branch": {
					SemanticRole: "branch_name",
				},
			},
		},
	}

	guidance := dynamicParameterGuidance(action)
	if _, ok := guidance["unused_param"]; ok {
		t.Errorf("guidance should drop entries with no populated fields: %+v", guidance)
	}
	for _, name := range []string{"project_id", "branch"} {
		if _, ok := guidance[name]; !ok {
			t.Errorf("guidance missing populated entry %q: %+v", name, guidance)
		}
	}
}

// TestDynamicParameterGuidance_NilWhenEmpty verifies the function returns
// nil when the action declares no guidance at all, so enrichDynamicSchema
// can skip the x_parameter_guidance key entirely.
func TestDynamicParameterGuidance_NilWhenEmpty(t *testing.T) {
	if got := dynamicParameterGuidance(actioncatalog.Action{Name: "noop", Route: toolutil.ActionRoute{}}); got != nil {
		t.Fatalf("dynamicParameterGuidance(empty) = %+v, want nil", got)
	}
}

// TestEnrichDynamicSchema_AttachesGuidanceAndDestructive verifies the schema
// returned by dynamicActionSchema carries the x_parameter_guidance extension
// when the action declares guidance, and the x_destructive / x_confirmation
// blocks when the route is destructive.
func TestEnrichDynamicSchema_AttachesGuidanceAndDestructive(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_widget", BaseDomain: "widget"})
	group.SetAction(actioncatalog.Action{
		Name: "remove",
		Route: toolutil.ActionRoute{
			Destructive: true,
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:   "project_id",
					ExampleBinding: `params.project_id:"group/project"`,
				},
			},
		},
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	session := dynamicSchemaSession(t, catalog)
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/dynamic/widget.remove"})
	if err != nil {
		t.Fatalf("read dynamic action schema: %v", err)
	}

	var schema map[string]any
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &schema); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}

	guidance, ok := schema["x_parameter_guidance"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing x_parameter_guidance: %+v", schema)
	}
	entry, ok := guidance["project_id"].(map[string]any)
	if !ok {
		t.Fatalf("x_parameter_guidance missing project_id: %+v", guidance)
	}
	if entry["semantic_role"] != "project_id" {
		t.Errorf("semantic_role = %v, want project_id", entry["semantic_role"])
	}
	if entry["example_binding"] != `params.project_id:"group/project"` {
		t.Errorf("example_binding = %v, want quoted project ID", entry["example_binding"])
	}

	if got, exists := schema["x_destructive"].(bool); !exists || !got {
		t.Errorf("x_destructive = %v, want true", schema["x_destructive"])
	}
	confirmation, ok := schema["x_confirmation"].(map[string]any)
	if !ok || confirmation["location"] != "gitlab_execute_action.confirm" {
		t.Errorf("x_confirmation = %+v, want confirm guidance", schema["x_confirmation"])
	}
}

// TestEnrichDynamicSchema_OmitsGuidanceWhenAbsent verifies the schema does
// not include x_parameter_guidance when the action has no guidance entries,
// keeping the contract explicit for downstream consumers.
func TestEnrichDynamicSchema_OmitsGuidanceWhenAbsent(t *testing.T) {
	action := actioncatalog.Action{
		Name: "noop",
		Route: toolutil.ActionRoute{
			Destructive: false,
		},
	}
	schema := map[string]any{"type": "object"}
	enriched := enrichDynamicSchema(schema, action)
	if _, ok := enriched["x_parameter_guidance"]; ok {
		t.Errorf("schema should not include x_parameter_guidance when no guidance declared: %+v", enriched)
	}
	if _, ok := enriched["x_destructive"]; ok {
		t.Errorf("schema should not include x_destructive for non-destructive action: %+v", enriched)
	}
}
