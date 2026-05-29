package features

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for instance feature flag tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		featureReadSpec("feature_list", toolutil.RouteAction(client, List), "gitlab_list_features"),
		featureReadSpec("feature_list_definitions", toolutil.RouteAction(client, ListDefinitions), "gitlab_list_feature_definitions"),
		featureSetSpec(client),
		featureDeleteSpec("feature_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_delete_feature_flag"),
	}
}

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

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("feature flag")
	return out, nil
}

func featureReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, featureOptions(name, individualTool))
}

func featureSetSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualIdempotent := false
	options := featureOptions("feature_set", "gitlab_set_feature_flag")
	options.IndividualTool.AnnotationOverrides.Idempotent = &individualIdempotent
	return toolutil.NewUpdateActionSpec("feature_set", SetRoute(client), options)
}

func featureDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, featureOptions(name, individualTool))
}

func featureOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	usage := "List instance feature flags and definitions."
	guidance := map[string]toolutil.ParameterGuidance{}
	if actionName == "feature_set" || actionName == "feature_delete" {
		guidance["name"] = toolutil.ParameterGuidance{
			SemanticRole:   "feature_flag_name",
			ValueSource:    "Feature flag name from list/definitions or requested admin toggle.",
			ExampleBinding: `params.name:"flag1"`,
		}
	}
	if actionName == "feature_set" {
		usage = "Set or update an instance feature flag value (bool/int/string)."
		guidance["value"] = toolutil.ParameterGuidance{
			SemanticRole:   "feature_flag_value",
			ValueSource:    "Desired value (boolean, integer, or string) for the feature flag gate.",
			ExampleBinding: "params.value:true",
		}
	}
	if actionName == "feature_delete" {
		usage = "Delete an instance feature flag override by name."
	}

	return toolutil.ActionSpecOptions{
		Aliases:           []string{individualTool},
		Tags:              []string{"admin", "feature"},
		Usage:             usage,
		RelatedActions:    []string{"admin.settings_get"},
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "features",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
