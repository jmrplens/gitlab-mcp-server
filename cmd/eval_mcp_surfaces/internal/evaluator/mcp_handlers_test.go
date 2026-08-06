package evaluator

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/elicitation"
)

// TestEvalElicitationHandler_DerivesNestedDefaults verifies elicitation schemas
// are auto-accepted with type-aware deterministic fixture values.
func TestEvalElicitationHandler_DerivesNestedDefaults(t *testing.T) {
	previousTag, _ := evalElicitationReleaseTag.Load().(string)
	setEvalElicitationReleaseTag("v-test")
	t.Cleanup(func() { setEvalElicitationReleaseTag(previousTag) })
	result, err := evalElicitationHandler(t.Context(), &mcp.ElicitRequest{Params: &mcp.ElicitParams{RequestedSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirmed":       map[string]any{"type": "boolean"},
			"selection":       map[string]any{"type": "string", "enum": []any{"private", "internal"}},
			"count":           map[string]any{"type": []any{"null", "integer"}},
			"ratio":           map[string]any{"type": "number"},
			"items":           map[string]any{"type": "array"},
			"tag_name":        map[string]any{"type": "string"},
			"nested":          map[string]any{"type": "object", "properties": map[string]any{"description": map[string]any{"type": "string"}}},
			"ignored_non_map": "bad",
		},
	}}})
	if err != nil {
		t.Fatalf("evalElicitationHandler() error = %v", err)
	}
	if result.Action != "accept" {
		t.Fatalf("Action = %q, want accept", result.Action)
	}
	if result.Content["confirmed"] != true || result.Content["selection"] != "private" || result.Content["tag_name"] != "v-test" {
		t.Fatalf("content = %#v, want confirmed, private selection, and prepared tag", result.Content)
	}
	nested, ok := result.Content["nested"].(map[string]any)
	if !ok || nested["description"] != "Created by eval_mcp_surfaces elicitation handler" {
		t.Fatalf("nested content = %#v", result.Content["nested"])
	}
}

// TestFirstJSONSchemaType_SelectsFirstNonNullType verifies union schema types
// pick the first meaningful JSON schema type.
func TestFirstJSONSchemaType_SelectsFirstNonNullType(t *testing.T) {
	if got := firstJSONSchemaType([]any{"null", "boolean"}); got != "boolean" {
		t.Fatalf("firstJSONSchemaType() = %q, want boolean", got)
	}
	if got := firstJSONSchemaType(nil); got != "string" {
		t.Fatalf("firstJSONSchemaType(nil) = %q, want string", got)
	}
}

// TestEvalElicitationSelection_DefaultsWhenEnumMissing verifies selection fields
// always get a stable value even without usable enum metadata.
func TestEvalElicitationSelection_DefaultsWhenEnumMissing(t *testing.T) {
	if got := evalElicitationSelection(map[string]any{"enum": []any{""}}); got != "default" {
		t.Fatalf("evalElicitationSelection(empty enum) = %q, want default", got)
	}
}

// TestEvalElicitationHandler_AdvertisesElicitationToMCPServer verifies that the evaluator client can drive interactive tools.
func TestEvalElicitationHandler_AdvertisesElicitationToMCPServer(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "elicitation-probe", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "elicitation_probe", Description: "elicitation probe"}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		flow, err := elicitation.FlowFromRequest(req)
		if err != nil {
			return nil, nil, err
		}
		if !flow.IsSupported() {
			return nil, nil, errors.New("elicitation capability not advertised")
		}
		content, err := flow.GatherData(ctx, "probe", "probe", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":     map[string]any{"type": "string"},
				"confirmed": map[string]any{"type": "boolean"},
				"count":     map[string]any{"type": "integer"},
				"enabled":   map[string]any{"type": "boolean"},
				"selection": map[string]any{"type": "string", "enum": []any{"private", "internal"}},
			},
		})
		if errors.Is(err, elicitation.ErrInputPending) {
			return flow.InputRequiredResult(), nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		if validationErr := validateElicitationProbeResult(content); validationErr != nil {
			return nil, nil, validationErr
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprint(content["title"])}}}, nil, nil
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "elicitation-probe-client", Version: "0"}, &mcp.ClientOptions{
		ElicitationHandler: evalElicitationHandler,
	})
	session, err := mcpClient.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "elicitation_probe", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if got := toolResultContent(result); !strings.Contains(got, "Evaluation elicitation test") {
		t.Fatalf("elicitation result = %q, want evaluator title", got)
	}
}

func validateElicitationProbeResult(content map[string]any) error {
	if content["confirmed"] != true {
		return fmt.Errorf("elicitation content = %+v, want accepted confirmation", content)
	}
	if _, ok := content["enabled"].(bool); !ok {
		return fmt.Errorf("elicitation enabled = %T, want bool", content["enabled"])
	}
	if err := validateElicitationNumericZero(content["count"]); err != nil {
		return err
	}
	if content["selection"] != "private" {
		return fmt.Errorf("elicitation selection = %v, want private", content["selection"])
	}
	return nil
}

func validateElicitationNumericZero(count any) error {
	switch typed := count.(type) {
	case float64:
		if typed == 0 {
			return nil
		}
		return fmt.Errorf("elicitation count = %v, want numeric zero", typed)
	case int:
		if typed == 0 {
			return nil
		}
		return fmt.Errorf("elicitation count = %v, want numeric zero", typed)
	case nil:
		return errors.New("elicitation count must be a numeric value")
	default:
		return fmt.Errorf("elicitation count must be a numeric value, got %T", count)
	}
}

// TestEvalElicitationSchemaValue_TypeAwareDefaults verifies fallback values
// match schema types, including nested object properties handled outside MCP's
// elicitation primitive-property subset.
func TestEvalElicitationSchemaValue_TypeAwareDefaults(t *testing.T) {
	metadata := evalElicitationSchemaValue("metadata", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"retries": map[string]any{"type": "integer"},
			"dry_run": map[string]any{"type": "boolean"},
		},
	}).(map[string]any)
	if metadata["retries"] != 0 || metadata["dry_run"] != false {
		t.Fatalf("metadata defaults = %#v, want integer and boolean defaults", metadata)
	}

	if got := evalElicitationSchemaValue("visibility", map[string]any{"enum": []any{"private", "internal"}}); got != "private" {
		t.Fatalf("enum default = %v, want private", got)
	}
	if got := evalElicitationSchemaValue("labels", map[string]any{"type": "array"}); !reflect.DeepEqual(got, []any{}) {
		t.Fatalf("array default = %#v, want empty array", got)
	}
}
