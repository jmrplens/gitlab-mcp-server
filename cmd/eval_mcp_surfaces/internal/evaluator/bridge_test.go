package evaluator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestAppendCapabilityBridgeTools_AddsSupportedBridgeSchemas verifies bridge tools
// are appended with strict object schemas and deterministic sorting.
func TestAppendCapabilityBridgeTools_AddsSupportedBridgeSchemas(t *testing.T) {
	catalog := []modelTool{modelToolFromParts("gitlab_execute_action", "execute", map[string]any{"type": "object"})}
	tools := appendCapabilityBridgeTools(catalog, mcpBridgeSupport{Capabilities: true, Resources: true, Prompts: true, Completion: true})

	got := make(map[string]modelTool, len(tools))
	for _, tool := range tools {
		got[tool.Name] = tool
	}
	for _, name := range []string{capabilityListTool, resourceListTool, resourceReadTool, promptListTool, promptGetTool, completionTool, dynamicExecuteActionTool} {
		if _, ok := got[name]; !ok {
			t.Fatalf("appendCapabilityBridgeTools() missing %s in %#v", name, got)
		}
	}
	if schema := got[capabilityListTool].InputSchema.(map[string]any); schema["additionalProperties"] != false {
		t.Fatalf("capability schema = %#v, want strict object", schema)
	}
	if !strings.Contains(got[capabilityListTool].Description, "MCP server capability metadata") {
		t.Fatalf("capability description = %q, want metadata guidance", got[capabilityListTool].Description)
	}
	if !strings.Contains(got[resourceListTool].Description, "valid first step for resource-manifest discovery") {
		t.Fatalf("resource list description = %q, want resource-manifest guidance", got[resourceListTool].Description)
	}
	if required := strings.Join(stringSliceFromAny(t, got[completionTool].InputSchema.(map[string]any)["required"]), ","); required != "ref_type,argument_name" {
		t.Fatalf("completion required = %q, want ref_type,argument_name", required)
	}
	if tools[len(tools)-1].CacheControl == nil || tools[len(tools)-1].CacheControl.Type != "ephemeral" {
		t.Fatalf("last tool cache control = %#v, want ephemeral", tools[len(tools)-1].CacheControl)
	}
}

// TestActiveBridgeTools_ReflectsSupportedCapabilities verifies capability lists
// only include tools backed by active MCP features.
func TestActiveBridgeTools_ReflectsSupportedCapabilities(t *testing.T) {
	if got := activeBridgeTools(mcpBridgeSupport{}); got != nil {
		t.Fatalf("activeBridgeTools(no support) = %#v, want nil", got)
	}
	got := activeBridgeTools(mcpBridgeSupport{Capabilities: true, Resources: true, Completion: true})
	want := strings.Join([]string{capabilityListTool, resourceListTool, resourceReadTool, completionTool}, ",")
	if strings.Join(got, ",") != want {
		t.Fatalf("activeBridgeTools() = %v, want %s", got, want)
	}
}

// TestCompletionParamsFromInput_BuildsPromptAndResourceReferences verifies MCP
// completion bridge arguments are validated and converted to SDK params.
func TestCompletionParamsFromInput_BuildsPromptAndResourceReferences(t *testing.T) {
	cases := []struct {
		name    string
		input   map[string]any
		wantRef string
		wantErr string
	}{
		{
			name: "prompt ref with context",
			input: map[string]any{
				"ref_type":          "ref/prompt",
				"name":              "my_open_mrs",
				"argument_name":     "project_id",
				"argument_value":    "gitlab",
				"context_arguments": map[string]any{"group_id": "my-org", "ignored": 42},
			},
			wantRef: "ref/prompt:my_open_mrs",
		},
		{
			name:    "resource ref",
			input:   map[string]any{"ref_type": "ref/resource", "uri": "gitlab://project/{project_id}", "argument_name": "project_id"},
			wantRef: "ref/resource:gitlab://project/{project_id}",
		},
		{name: "missing ref type", input: map[string]any{"argument_name": "project_id"}, wantErr: "ref_type"},
		{name: "unsupported ref type", input: map[string]any{"ref_type": "bad", "argument_name": "project_id"}, wantErr: "unsupported ref_type"},
		{name: "missing prompt name", input: map[string]any{"ref_type": "ref/prompt", "argument_name": "project_id"}, wantErr: "name"},
		{name: "missing resource uri", input: map[string]any{"ref_type": "ref/resource", "argument_name": "project_id"}, wantErr: "uri"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, err := completionParamsFromInput(tc.input)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("completionParamsFromInput() error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("completionParamsFromInput() error = %v", err)
			}
			gotRef := params.Ref.Type + ":" + params.Ref.Name + params.Ref.URI
			if gotRef != tc.wantRef {
				t.Fatalf("ref = %q, want %q", gotRef, tc.wantRef)
			}
			if params.Argument.Name != "project_id" {
				t.Fatalf("argument name = %q, want project_id", params.Argument.Name)
			}
		})
	}
}

// TestStringMapFromAny_DropsNonStringValues verifies completion context values
// are limited to MCP-compatible strings.
func TestStringMapFromAny_DropsNonStringValues(t *testing.T) {
	got := stringMapFromAny(map[string]any{"keep": "value", "drop": 1})
	if len(got) != 1 || got["keep"] != "value" {
		t.Fatalf("stringMapFromAny() = %#v, want only keep=value", got)
	}
	wrongType := stringMapFromAny(map[string]string{"wrong": "type"})
	if wrongType != nil {
		t.Fatalf("stringMapFromAny(non map[string]any) = %#v, want nil", wrongType)
	}
}

// TestMarshalResourceBridgeResult_RecordsTraceExchange verifies bridge result
// serialization populates both model-facing content and trace metadata.
func TestMarshalResourceBridgeResult_RecordsTraceExchange(t *testing.T) {
	exchange := &traceMCPExchange{}
	content, err := marshalResourceBridgeResult(map[string]any{"ok": true}, exchange)
	if err != nil {
		t.Fatalf("marshalResourceBridgeResult() error = %v", err)
	}
	if content != `{"ok":true}` {
		t.Fatalf("content = %q, want JSON", content)
	}
	var decoded map[string]bool
	decodeErr := json.Unmarshal(exchange.Response, &decoded)
	if decodeErr != nil || !decoded["ok"] {
		t.Fatalf("exchange response = %s, %v; want ok true", exchange.Response, decodeErr)
	}
}

// TestCapabilityBridgeResult_RequiresSession verifies bridge execution fails
// clearly when no MCP session is available.
func TestCapabilityBridgeResult_RequiresSession(t *testing.T) {
	runner := &modelRunner{}
	result := runner.capabilityBridgeResult(t.Context(), modelContentBlock{Name: capabilityListTool})
	if result.Err == nil || !strings.Contains(result.Content, "not available") {
		t.Fatalf("capabilityBridgeResult() = %+v, want unavailable error", result)
	}
}

// TestCapabilityBridgeResult_UsesLiveMCPCapabilities verifies bridge tools can
// inspect a real in-memory MCP session, including resources and completions.
func TestCapabilityBridgeResult_UsesLiveMCPCapabilities(t *testing.T) {
	client, cleanup, clientErr := newMockGitLabClient()
	if clientErr != nil {
		t.Fatalf("newMockGitLabClient() error = %v", clientErr)
	}
	defer cleanup()
	session, closeSession, _, _, sessionErr := buildCatalogSession(client, config.ToolSurfaceDynamic)
	if sessionErr != nil {
		t.Fatalf("buildCatalogSession() error = %v", sessionErr)
	}
	defer closeSession()
	support := probeCapabilityBridgeSupport(session)
	runner := &modelRunner{mcpSession: session, mcpBridge: support}

	capabilities := runner.capabilityBridgeResult(t.Context(), modelContentBlock{Name: capabilityListTool})
	if capabilities.Err != nil || !strings.Contains(capabilities.Content, capabilityListTool) || capabilities.MCP == nil || len(capabilities.MCP.Response) == 0 {
		t.Fatalf("capability bridge result = %+v, want capabilities payload", capabilities)
	}

	resources := runner.capabilityBridgeResult(t.Context(), modelContentBlock{Name: resourceListTool})
	if resources.Err != nil || !strings.Contains(resources.Content, "gitlab://tools") {
		t.Fatalf("resource list result = %+v, want tools resource", resources)
	}

	read := runner.capabilityBridgeResult(t.Context(), modelContentBlock{Name: resourceReadTool, Input: map[string]any{"uri": "gitlab://tools/project.get"}})
	if read.Err != nil || !strings.Contains(read.Content, "project.get") {
		t.Fatalf("resource read result = %+v, want project.get detail", read)
	}

	prompts := runner.capabilityBridgeResult(t.Context(), modelContentBlock{Name: promptListTool})
	if prompts.Err != nil || !strings.Contains(prompts.Content, "prompts") {
		t.Fatalf("prompt list result = %+v, want prompt list", prompts)
	}

	completion := runner.capabilityBridgeResult(t.Context(), modelContentBlock{Name: completionTool, Input: map[string]any{"ref_type": "ref/resource", "uri": "gitlab://tools/{id}", "argument_name": "id", "argument_value": "project"}})
	if completion.Err != nil || !strings.Contains(completion.Content, "completion") {
		t.Fatalf("completion result = %+v, want completion payload", completion)
	}
}

// TestCapabilityBridgeResult_ReportsUnsupportedTool verifies unknown bridge
// names are traced as protocol errors.
func TestCapabilityBridgeResult_ReportsUnsupportedTool(t *testing.T) {
	runner := &modelRunner{mcpSession: newResourceLookupSessionForTest(t)}
	result := runner.capabilityBridgeResult(t.Context(), modelContentBlock{Name: "gitlab_unknown_bridge"})
	if result.Err == nil || result.MCP == nil || !strings.Contains(result.MCP.ProtocolError, "unsupported") {
		t.Fatalf("capabilityBridgeResult(unknown) = %+v, want protocol error", result)
	}
}

// TestPromptAndCompletionBridgeInputs_ReportMissingRequiredFields verifies bridge
// tools fail before contacting MCP when required standalone fields are absent.
func TestPromptAndCompletionBridgeInputs_ReportMissingRequiredFields(t *testing.T) {
	runner := &modelRunner{mcpSession: newResourceLookupSessionForTest(t)}
	if content, err := runner.getPromptLookupContent(t.Context(), map[string]any{}, &traceMCPExchange{}); err == nil || content != "" || !strings.Contains(err.Error(), "requires name") {
		t.Fatalf("getPromptLookupContent(missing) = %q, %v; want name error", content, err)
	}
	if content, err := runner.completionLookupContent(t.Context(), map[string]any{"argument_name": "id"}, &traceMCPExchange{}); err == nil || content != "" || !strings.Contains(err.Error(), "requires ref_type") {
		t.Fatalf("completionLookupContent(missing) = %q, %v; want ref_type error", content, err)
	}
}

func stringSliceFromAny(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]string)
	if ok {
		return items
	}
	rawItems, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %T, want []string or []any", value)
	}
	out := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		text, isString := item.(string)
		if !isString {
			t.Fatalf("item = %T, want string", item)
		}
		out = append(out, text)
	}
	return out
}
