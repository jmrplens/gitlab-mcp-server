// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go contains unit tests for the snippets ActionSpec
// metadata: the decorator no-op on unknown tools, populated metadata, the
// expected action count, and the choice of files or content the two create
// actions publish.
package snippets

import (
	"encoding/json"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestDecorateSnippetMeta_UnknownToolIsNoop verifies decorateSnippetMeta leaves
// options untouched for a tool with no metadata entry.
func TestDecorateSnippetMeta_UnknownToolIsNoop(t *testing.T) {
	options := snippetOptions("gitlab_snippet_unknown")
	before := options.Usage
	decorateSnippetMeta(&options, "gitlab_snippet_unknown")
	if options.Usage != before {
		t.Errorf("Usage mutated for unknown tool: %q", options.Usage)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions set for unknown tool: %v", options.RelatedActions)
	}
}

// TestActionSpecs_MetadataPopulated verifies every snippet action spec carries
// non-generic discovery metadata (R-META): a custom usage, natural-language
// aliases, related actions, and a "Returns: … See also: …" description.
func TestActionSpecs_MetadataPopulated(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	for _, spec := range ActionSpecs(client) {
		tool := spec.IndividualTool.Name
		t.Run(tool, func(t *testing.T) {
			meta, ok := snippetActionMeta[tool]
			if !ok {
				t.Fatalf("no metadata entry for %s", tool)
			}
			if meta.usage == "" || spec.Usage == "Use to execute snippets domain action." {
				t.Errorf("%s has generic usage: %q", tool, spec.Usage)
			}
			if len(spec.Aliases) == 0 || spec.Aliases[0] == tool {
				t.Errorf("%s missing natural-language aliases: %v", tool, spec.Aliases)
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("%s missing related actions", tool)
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s description missing Returns:/See also: form: %q", tool, desc)
			}
		})
	}
}

// TestActionSpecs_Count guards that the snippet catalog still projects all 15
// canonical snippet actions after the audit.
func TestActionSpecs_Count(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if n := len(ActionSpecs(client)); n != 15 {
		t.Fatalf("ActionSpecs count = %d, want 15", n)
	}
}

// snippetCreateTools are the individual tools of the two snippet create
// actions, with the params each needs beside its files or content.
var snippetCreateTools = map[string]map[string]any{
	"gitlab_snippet_create":         {"title": "notes"},
	"gitlab_project_snippet_create": {"project_id": "42", "title": "notes"},
}

// snippetCreateSpecs returns the specs of the two snippet create actions,
// keyed by individual tool.
func snippetCreateSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()
	specs := make(map[string]toolutil.ActionSpec, len(snippetCreateTools))
	for _, spec := range ActionSpecs(testutil.NewTestClient(t, http.NewServeMux())) {
		if _, isCreate := snippetCreateTools[spec.IndividualTool.Name]; isCreate {
			specs[spec.IndividualTool.Name] = spec
		}
	}
	if len(specs) != len(snippetCreateTools) {
		t.Fatalf("ActionSpecs() holds the create tools %v, want every one of %v", slices.Sorted(maps.Keys(specs)), slices.Sorted(maps.Keys(snippetCreateTools)))
	}
	return specs
}

// TestCreateActions_Schema_StatesExactlyOneOfFilesOrContent verifies that the
// personal and the project snippet create publish the rule GitLab holds both
// routes to, exactly_one_of :files, :content with a file_name required beside
// a content (create_file_params in lib/api/helpers/snippets_helpers.rb),
// where a caller reads it: the alternatives of the input schema are file_name
// with content or files, of which a call completes exactly one, as find, the
// gitlab://tools manifest and the tool reference read them, and the Usage
// says not both.
//
// The schema stated the two as an anyOf, which read as a choice a call could
// complete twice. A call that did was refused by GitLab on the personal
// create, and on the project create, which sends the single file as files,
// lost its content and file_name without a word.
func TestCreateActions_Schema_StatesExactlyOneOfFilesOrContent(t *testing.T) {
	wantGroups := [][]string{{"file_name", "content"}, {"files"}}
	for name, spec := range snippetCreateSpecs(t) {
		t.Run(name, func(t *testing.T) {
			schema := spec.Route.InputSchema
			if got, has := schema["anyOf"]; has {
				t.Errorf("input schema anyOf = %#v, want none: a call completes exactly one of the alternatives", got)
			}
			if got := actioncatalog.RequiredParamAlternatives(schema); !reflect.DeepEqual(got, wantGroups) {
				t.Errorf("RequiredParamAlternatives() = %v, want %v", got, wantGroups)
			}
			if !actioncatalog.RequiredParamAlternativesExclusive(schema) {
				t.Error("RequiredParamAlternativesExclusive() = false, want true: GitLab refuses files beside a content")
			}
			if !strings.Contains(spec.Usage, "not both") {
				t.Errorf("Usage = %q, want it to say not both", spec.Usage)
			}
		})
	}
}

// TestCreateActions_ActionSchema_RefusesWhatGitLabRefuses verifies the
// catalog's input schema of the two snippet creates, as the meta tool's full
// parameter-schema mode holds a call's params to it: the files alone, or a
// file name with a content, pass; the files beside a content, with or
// without its file name, are refused, and so are neither and a content
// without its file name; and a field with an empty value counts as left out,
// as the handler counts it, so the files beside an empty content pass. It
// also verifies the individual tools are served that schema without its
// oneOf, since the Anthropic Messages API refuses a tool list holding a root
// combinator.
func TestCreateActions_ActionSchema_RefusesWhatGitLabRefuses(t *testing.T) {
	files := []any{map[string]any{"file_path": "notes.md", "content": "# notes"}}
	calls := []struct {
		name      string
		fields    map[string]any
		wantValid bool
	}{
		{name: "the files alone", fields: map[string]any{"files": files}, wantValid: true},
		{name: "a file name with a content", fields: map[string]any{"file_name": "notes.md", "content": "# notes"}, wantValid: true},
		{name: "the files beside an empty content", fields: map[string]any{"files": files, "content": ""}, wantValid: true},
		{name: "the files beside a file name", fields: map[string]any{"files": files, "file_name": "notes.md"}, wantValid: true},
		{name: "the files beside a file name with a content", fields: map[string]any{"files": files, "file_name": "notes.md", "content": "# notes"}},
		{name: "the files beside a content", fields: map[string]any{"files": files, "content": "# notes"}},
		{name: "a content without a file name", fields: map[string]any{"content": "# notes"}},
		{name: "a file name beside an empty content", fields: map[string]any{"file_name": "notes.md", "content": ""}},
		{name: "neither", fields: map[string]any{}},
	}
	for name, spec := range snippetCreateSpecs(t) {
		t.Run(name, func(t *testing.T) {
			projected, err := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{})
			if err != nil {
				t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
			}
			if served := toolutil.RootCombinators(projected.InputSchema); len(served) > 0 {
				t.Errorf("the individual tool is served %v at the root of its input schema", served)
			}
			resolved := resolvedInputSchema(t, toolutil.MetaActionSchema(spec.Route))
			for _, call := range calls {
				t.Run(call.name, func(t *testing.T) {
					arguments := maps.Clone(snippetCreateTools[name])
					maps.Copy(arguments, call.fields)
					if validateErr := resolved.Validate(arguments); (validateErr == nil) != call.wantValid {
						t.Errorf("Validate(%v) error = %v, want valid %t", arguments, validateErr, call.wantValid)
					}
				})
			}
		})
	}
}

// resolvedInputSchema decodes an input schema into a resolved jsonschema-go
// schema, the validator the SDK and the meta tool's full mode put a call's
// arguments to.
func resolvedInputSchema(t *testing.T, schema any) *jsonschema.Resolved {
	t.Helper()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var decoded jsonschema.Schema
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode input schema: %v", err)
	}
	resolved, err := decoded.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve input schema: %v", err)
	}
	return resolved
}
