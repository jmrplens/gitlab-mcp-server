package toolutil

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestExpandResourceURI_FillsTheTemplateFromTheParameters verifies the
// expansion every dispatcher relies on: parameters land in the URI escaped the
// way the resource templates read them, JSON numbers come out as integers, and
// a missing or empty variable yields no URI at all rather than one with a hole.
func TestExpandResourceURI_FillsTheTemplateFromTheParameters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		template string
		params   map[string]any
		want     string
		ok       bool
	}{
		{name: "numeric ids from json", template: "gitlab://project/{project_id}/issue/{issue_iid}", params: map[string]any{"project_id": float64(42), "issue_iid": float64(7)}, want: "gitlab://project/42/issue/7", ok: true},
		{name: "a project path is escaped", template: "gitlab://project/{project_id}", params: map[string]any{"project_id": "group/sub/project"}, want: "gitlab://project/group%2Fsub%2Fproject", ok: true},
		{name: "a branch name with a slash is escaped", template: "gitlab://project/{project_id}/branch/{branch_name}", params: map[string]any{"project_id": "42", "branch_name": "feature/login"}, want: "gitlab://project/42/branch/feature%2Flogin", ok: true},
		{name: "reserved expansion keeps the slashes of a path", template: "gitlab://project/{project_id}/file/{ref}/{+path}", params: map[string]any{"project_id": "42", "ref": "main", "path": "src/a b.go"}, want: "gitlab://project/42/file/main/src/a%20b.go", ok: true},
		{name: "a missing variable yields nothing", template: "gitlab://project/{project_id}/issue/{issue_iid}", params: map[string]any{"project_id": "42"}, ok: false},
		{name: "an empty variable yields nothing", template: "gitlab://project/{project_id}", params: map[string]any{"project_id": "  "}, ok: false},
		{name: "an empty template yields nothing", template: "", params: map[string]any{"project_id": "42"}, ok: false},
		{name: "no variables at all", template: "gitlab://user/current", params: nil, want: "gitlab://user/current", ok: true},
		{name: "an unterminated brace yields nothing", template: "gitlab://project/{project_id", params: map[string]any{"project_id": "42"}, ok: false},
		{name: "an int value", template: "gitlab://snippet/{snippet_id}", params: map[string]any{"snippet_id": 5}, want: "gitlab://snippet/5", ok: true},
		{name: "a boolean is not an identifier", template: "gitlab://snippet/{snippet_id}", params: map[string]any{"snippet_id": true}, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ExpandResourceURI(tt.template, tt.params)
			if ok != tt.ok || got != tt.want {
				t.Errorf("ExpandResourceURI(%q) = %q, %v; want %q, %v", tt.template, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestResourceTemplateVariables_ListsTheNames verifies the variable scan the
// validator uses, including the reserved-expansion prefix it strips.
func TestResourceTemplateVariables_ListsTheNames(t *testing.T) {
	t.Parallel()
	got, err := ResourceTemplateVariables("gitlab://project/{project_id}/file/{ref}/{+path}")
	if err != nil {
		t.Fatalf("ResourceTemplateVariables() error = %v", err)
	}
	if strings.Join(got, ",") != "project_id,ref,path" {
		t.Errorf("variables = %v", got)
	}
	constant, err := ResourceTemplateVariables("gitlab://user/current")
	if err != nil {
		t.Fatalf("ResourceTemplateVariables() error = %v for a constant template", err)
	}
	if constant != nil {
		t.Errorf("variables of a constant template = %v, want none", constant)
	}
}

// TestResourceTemplateVariables_RefusesMalformedBraces pins that a template
// the expander could never fill is an error at scan time, since the expander
// answers it by embedding nothing and a declaration that reached it would
// fail silently on every call.
func TestResourceTemplateVariables_RefusesMalformedBraces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		template string
		wantErr  string
	}{
		{name: "a brace that never closes", template: "gitlab://project/{project_id", wantErr: "unterminated variable"},
		{name: "a second variable left open", template: "gitlab://project/{project_id}/issue/{issue_iid", wantErr: "unterminated variable"},
		{name: "a pair with nothing between", template: "gitlab://project/{}", wantErr: "no name"},
		{name: "a reserved prefix and nothing else", template: "gitlab://project/{+}", wantErr: "no name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			names, err := ResourceTemplateVariables(tt.template)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ResourceTemplateVariables(%q) = %v, %v; want an error containing %q", tt.template, names, err, tt.wantErr)
			}
		})
	}
}

// TestEmbedCanonicalResource_AppendsOnlyWhenEverythingLinesUp verifies the
// dispatcher-facing helper: it embeds the expanded URI with the JSON output,
// and stays out of error results, results without an identifier, and
// deployments that turned embedding off.
func TestEmbedCanonicalResource_AppendsOnlyWhenEverythingLinesUp(t *testing.T) {
	const template = "gitlab://project/{project_id}/issue/{issue_iid}"
	params := map[string]any{"project_id": "group/project", "issue_iid": float64(3)}
	value := map[string]any{"iid": 3}

	t.Run("embeds on a success result", func(t *testing.T) {
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "issue"}}}
		EmbedCanonicalResource(result, template, params, value)
		if len(result.Content) != 2 {
			t.Fatalf("content blocks = %d, want 2", len(result.Content))
		}
		embedded, ok := result.Content[1].(*mcp.EmbeddedResource)
		if !ok {
			t.Fatalf("second block = %T, want *mcp.EmbeddedResource", result.Content[1])
		}
		if embedded.Resource.URI != "gitlab://project/group%2Fproject/issue/3" {
			t.Errorf("uri = %q", embedded.Resource.URI)
		}
		if embedded.Resource.MIMEType != "application/json" || !strings.Contains(embedded.Resource.Text, `"iid":3`) {
			t.Errorf("resource = %+v", embedded.Resource)
		}
	})
	t.Run("stays out of an error result", func(t *testing.T) {
		result := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not found"}}}
		EmbedCanonicalResource(result, template, params, value)
		if len(result.Content) != 1 {
			t.Errorf("content blocks = %d, want the error text alone", len(result.Content))
		}
	})
	t.Run("stays out without a template or an identifier", func(t *testing.T) {
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "issue"}}}
		EmbedCanonicalResource(result, "", params, value)
		EmbedCanonicalResource(result, template, map[string]any{"project_id": "42"}, value)
		EmbedCanonicalResource(nil, template, params, value)
		if len(result.Content) != 1 {
			t.Errorf("content blocks = %d, want 1", len(result.Content))
		}
	})
	t.Run("honors the global switch", func(t *testing.T) {
		EnableEmbeddedResources(false)
		t.Cleanup(func() { EnableEmbeddedResources(true) })
		result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "issue"}}}
		EmbedCanonicalResource(result, template, params, value)
		if len(result.Content) != 1 {
			t.Errorf("content blocks = %d, want 1 with embedding disabled", len(result.Content))
		}
	})
}

// TestValidateEmbeddedResource_RefusesDeclarationsThatCannotWork verifies the
// spec-time checks: a template needs a policy that embeds, "always" needs a
// template, the scheme must be gitlab://, and every variable must be a
// parameter the action accepts.
func TestValidateEmbeddedResource_RefusesDeclarationsThatCannotWork(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{"project_id": map[string]any{}, "issue_iid": map[string]any{}}}
	spec := func(policy, template string) ActionSpec {
		return ActionSpec{Name: "get", Route: ActionRoute{InputSchema: schema}, EmbeddedResourcePolicy: policy, EmbeddedResource: template}
	}
	tests := []struct {
		name    string
		spec    ActionSpec
		wantErr string
	}{
		{name: "nothing declared", spec: spec("", "")},
		{name: "always with a matching template", spec: spec(ActionSpecEmbeddedAlways, "gitlab://project/{project_id}/issue/{issue_iid}")},
		{name: "optional with a template", spec: spec(ActionSpecEmbeddedOptional, "gitlab://project/{project_id}")},
		{name: "a constant template", spec: spec(ActionSpecEmbeddedAlways, "gitlab://user/current")},
		{name: "always without a template", spec: spec(ActionSpecEmbeddedAlways, ""), wantErr: "no embedded resource template"},
		{name: "a template under a policy that never embeds", spec: spec(ActionSpecEmbeddedNone, "gitlab://project/{project_id}"), wantErr: "never embeds"},
		{name: "a template with no policy", spec: spec("", "gitlab://project/{project_id}"), wantErr: "never embeds"},
		{name: "another scheme", spec: spec(ActionSpecEmbeddedAlways, "https://gitlab.com/{project_id}"), wantErr: "not a gitlab:// URI"},
		{name: "a variable the action does not accept", spec: spec(ActionSpecEmbeddedAlways, "gitlab://project/{project_id}/mr/{merge_request_iid}"), wantErr: `parameter "merge_request_iid"`},
		{name: "no schema to check against", spec: ActionSpec{Name: "get", EmbeddedResourcePolicy: ActionSpecEmbeddedAlways, EmbeddedResource: "gitlab://project/{anything}"}},
		{name: "a brace that never closes", spec: spec(ActionSpecEmbeddedAlways, "gitlab://project/{project_id"), wantErr: "unterminated variable"},
		{name: "a variable with no name", spec: spec(ActionSpecEmbeddedAlways, "gitlab://project/{}"), wantErr: "no name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateEmbeddedResource(tt.spec)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateEmbeddedResource() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validateEmbeddedResource() error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestResourceParamValue_RendersEachType covers every branch of the parameter
// renderer: the types that carry an identifier, an integral float that reads as
// an integer, a fractional and an out-of-range float that keep their decimal
// form, and the values that are deliberately not identifiers (nil, a blank
// string, a bool) and so give nothing to substitute.
func TestResourceParamValue_RendersEachType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		value  any
		want   string
		wantOK bool
	}{
		{name: "nil is not a value", value: nil, want: "", wantOK: false},
		{name: "a string is trimmed", value: "  x/y  ", want: "x/y", wantOK: true},
		{name: "a blank string is not a value", value: "   ", want: "", wantOK: false},
		{name: "an integral float reads as an integer", value: float64(42), want: "42", wantOK: true},
		{name: "a fractional float keeps its decimal", value: 3.5, want: "3.5", wantOK: true},
		{name: "a float too large to be an id keeps its form", value: 1e15, want: "1000000000000000", wantOK: true},
		{name: "an int", value: 7, want: "7", wantOK: true},
		{name: "an int64", value: int64(9000000000), want: "9000000000", wantOK: true},
		{name: "a bool is not a value", value: true, want: "", wantOK: false},
		{name: "another type is rendered by fmt", value: uint(5), want: "5", wantOK: true},
		{name: "another type that renders blank is not a value", value: emptyStringer{}, want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := resourceParamValue(tt.value)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("resourceParamValue(%#v) = %q, %v; want %q, %v", tt.value, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// emptyStringer renders as whitespace, so it reaches the default branch of
// resourceParamValue and is trimmed away to nothing.
type emptyStringer struct{}

func (emptyStringer) String() string { return "  " }

// TestWithEmbeddedResource_DeclaresTheTemplateAndPolicy verifies the spec
// builder sets the always-embed policy and stores the template trimmed, so a
// declaration padded with whitespace still validates and expands.
func TestWithEmbeddedResource_DeclaresTheTemplateAndPolicy(t *testing.T) {
	t.Parallel()
	spec := ActionSpec{Name: "get"}.WithEmbeddedResource("  gitlab://project/{project_id}  ")
	if spec.EmbeddedResourcePolicy != ActionSpecEmbeddedAlways {
		t.Errorf("policy = %q, want %q", spec.EmbeddedResourcePolicy, ActionSpecEmbeddedAlways)
	}
	if spec.EmbeddedResource != "gitlab://project/{project_id}" {
		t.Errorf("template = %q, want it trimmed", spec.EmbeddedResource)
	}
}
