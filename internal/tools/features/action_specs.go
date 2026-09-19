// action_specs.go holds what this package contributes to the canonical action
// catalog. The specs themselves are declared in internal/tools/adminspecs,
// which is the one place the gitlab_admin group is assembled; what stays here
// is the route a spec takes, because setting a feature gate needs an input
// schema the reflected one cannot express.
//
// This package used to declare a full set of specs of its own that nothing
// ever aggregated, so they reached no surface and no validation.
package features

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
