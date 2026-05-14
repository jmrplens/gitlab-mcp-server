package features

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func setInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[SetInput](nil)
	if err != nil {
		panic(fmt.Sprintf("build feature set input schema: %v; check SetInput struct tags, unsupported field types, or circular schema references", err))
	}
	if property := schema.Properties["value"]; property != nil {
		property.Type = ""
		property.OneOf = []*jsonschema.Schema{
			{Type: "boolean"},
			{Type: "integer"},
			{Type: "string"},
		}
	}
	return schema
}

func setInputSchemaMap() map[string]any {
	data, err := json.Marshal(setInputSchema())
	if err != nil {
		panic(fmt.Sprintf("marshal feature set input schema: %v; check schema serialization for unsupported values", err))
	}
	var schema map[string]any
	if unmarshalErr := json.Unmarshal(data, &schema); unmarshalErr != nil {
		panic(fmt.Sprintf("unmarshal feature set input schema: %v; check generated schema JSON shape", unmarshalErr))
	}
	return schema
}

// SetRoute returns the meta-tool route for setting instance feature flags.
func SetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Set)
	route.InputSchema = setInputSchemaMap()
	return route
}

// RegisterTools registers all feature flag tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	featureTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconConfig})
	}

	mcp.AddTool(server, featureTool("gitlab_list_features", "List all feature flags (admin). Returns name, state and gates for each flag.\n\nReturns: JSON array of feature flags.\n\nSee also: gitlab_set_feature_flag."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_features", start, err)
		if err != nil {
			return nil, ListOutput{}, err
		}
		return toolutil.WithHints(FormatListMarkdown(out), out, nil)
	})

	mcp.AddTool(server, featureTool("gitlab_list_feature_definitions", "List all feature definitions (admin). Returns name, type, group, milestone and default_enabled for each definition.\n\nReturns: JSON array of feature definitions.\n\nSee also: gitlab_list_features."), func(ctx context.Context, req *mcp.CallToolRequest, input ListDefinitionsInput) (*mcp.CallToolResult, ListDefinitionsOutput, error) {
		start := time.Now()
		out, err := ListDefinitions(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_feature_definitions", start, err)
		if err != nil {
			return nil, ListDefinitionsOutput{}, err
		}
		return toolutil.WithHints(FormatListDefinitionsMarkdown(out), out, nil)
	})

	mcp.AddTool(server, featureTool("gitlab_set_feature_flag", "Set or create a feature flag (admin). Requires name and value. Supports scoping to user, group, project, namespace, or repository.\n\nReturns: JSON with the feature flag details.\n\nSee also: gitlab_list_features."), func(ctx context.Context, req *mcp.CallToolRequest, input SetInput) (*mcp.CallToolResult, SetOutput, error) {
		start := time.Now()
		out, err := Set(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_set_feature_flag", start, err)
		if err != nil {
			return nil, SetOutput{}, err
		}
		return toolutil.WithHints(FormatFeatureMarkdown(out), out, nil)
	})

	mcp.AddTool(server, featureTool("gitlab_delete_feature_flag", "Delete a feature flag (admin). Requires the flag name.\n\nReturns: confirmation message.\n\nSee also: gitlab_feature_flag_list."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete feature flag %q?", input.Name)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_feature_flag", start, err)
		r, o, _ := toolutil.DeleteResult("feature flag")
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return r, o, nil
	})
}
