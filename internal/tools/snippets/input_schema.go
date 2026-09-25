package snippets

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

func snippetCreateInputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("snippet create input schema: %v", err))
	}
	addSnippetCreateFileRequirement(schema)
	return schema
}

// schemaToJSON and schemaFromJSON are the round trip a create schema takes
// into the map the catalog publishes. A schema jsonschema-go reflected from a
// Go type always marshals, and what it writes is an object, which always
// decodes into a map, so the panics reading their errors guard against a
// library change no input can bring about. They are package variables so a
// test can take those branches.
var (
	schemaToJSON   = json.Marshal
	schemaFromJSON = json.Unmarshal
)

func snippetCreateInputSchemaMap[T any]() map[string]any {
	data, err := schemaToJSON(snippetCreateInputSchema[T]())
	if err != nil {
		panic(fmt.Sprintf("marshal snippet create input schema: %v", err))
	}
	var schema map[string]any
	if unmarshalErr := schemaFromJSON(data, &schema); unmarshalErr != nil {
		panic(fmt.Sprintf("unmarshal snippet create input schema: %v", unmarshalErr))
	}
	return schema
}

// CreateInputSchemaMap returns the input schema for personal snippet creation.
func CreateInputSchemaMap() map[string]any {
	return snippetCreateInputSchemaMap[CreateInput]()
}

// ProjectCreateInputSchemaMap returns the input schema for project snippet creation.
func ProjectCreateInputSchemaMap() map[string]any {
	return snippetCreateInputSchemaMap[ProjectCreateInput]()
}

func addSnippetCreateFileRequirement(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if schema.Properties == nil {
		schema.Properties = make(map[string]*jsonschema.Schema)
	}
	if files := schema.Properties["files"]; files != nil {
		minItems := 1
		files.MinItems = &minItems
	}
	schema.AnyOf = []*jsonschema.Schema{
		{Required: []string{"file_name", "content"}},
		{Required: []string{"files"}},
	}
}
