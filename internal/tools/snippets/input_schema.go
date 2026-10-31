// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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

// addSnippetCreateFileRequirement states the rule GitLab holds both snippet
// create routes to (create_file_params in lib/api/helpers/snippets_helpers.rb):
// exactly_one_of :files, :content, with a file_name required beside a
// content. It is a root oneOf of two branches, the single file as file_name
// with content and the files, which find, the gitlab://tools manifest and the
// tool reference read as two alternatives of which a call completes exactly
// one. No tool's served input schema carries it at its root, since the
// Anthropic Messages API refuses a tool list holding a root combinator, so
// the handler refuses a call completing none or both before it sends anything
// ([validateCreateSnippetContent]).
//
// A field counts in a branch only with a value, a string that is not empty or
// a list that is not, which is how the handler counts it and what it sends.
// Each branch also leaves the other's field empty or out, so the files beside
// a content complete neither, with or without a file_name, and a validator
// holding a call to the oneOf, the meta tool's full parameter-schema mode,
// refuses what GitLab refuses. Each leaves the call's other properties open,
// for the reason toolutil's valuedBranch gives.
func addSnippetCreateFileRequirement(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if schema.Properties == nil {
		schema.Properties = make(map[string]*jsonschema.Schema)
	}
	none, one := 0, 1
	if files := schema.Properties["files"]; files != nil {
		files.MinItems = &one
	}
	schema.OneOf = []*jsonschema.Schema{
		{
			Required: []string{"file_name", "content"},
			Properties: map[string]*jsonschema.Schema{
				"file_name": {MinLength: &one},
				"content":   {MinLength: &one},
				"files":     {MaxItems: &none},
			},
			AdditionalProperties: &jsonschema.Schema{},
		},
		{
			Required: []string{"files"},
			Properties: map[string]*jsonschema.Schema{
				"files":   {MinItems: &one},
				"content": {MaxLength: &none},
			},
			AdditionalProperties: &jsonschema.Schema{},
		},
	}
}
