package gatewaycompat

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Middleware returns a receiving middleware that applies subs to the four
// catalog listings a gateway validates: tools/list, prompts/list,
// resources/list and resources/templates/list. Every other method passes
// through untouched — a prompt's message content or a tool call's result is
// payload, and rewriting payload would change what the server does rather
// than how it introduces itself.
//
// Results are cloned before modification: list results return pointers
// shared with the server's registries, so an in-place edit would corrupt the
// catalog for every other session of this process.
func Middleware(subs []Substitution) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil || res == nil {
				return res, err
			}
			switch typed := res.(type) {
			case *mcp.ListToolsResult:
				return rewriteListTools(subs, typed), nil
			case *mcp.ListPromptsResult:
				return rewriteListPrompts(subs, typed), nil
			case *mcp.ListResourcesResult:
				return rewriteListResources(subs, typed), nil
			case *mcp.ListResourceTemplatesResult:
				return rewriteListResourceTemplates(subs, typed), nil
			}
			return res, nil
		}
	}
}

// rewriteListTools rewrites each tool's description, title, annotation title,
// and the description and title keys embedded in its schemas. Names, icons
// and schema constraints survive verbatim.
func rewriteListTools(subs []Substitution, res *mcp.ListToolsResult) *mcp.ListToolsResult {
	clone := *res
	clone.Tools = make([]*mcp.Tool, len(res.Tools))
	for i, tool := range res.Tools {
		tc := *tool
		tc.Description = Apply(subs, tool.Description)
		tc.Title = Apply(subs, tool.Title)
		if tool.Annotations != nil {
			ac := *tool.Annotations
			ac.Title = Apply(subs, tool.Annotations.Title)
			tc.Annotations = &ac
		}
		tc.InputSchema = rewriteSchema(subs, tool.InputSchema)
		tc.OutputSchema = rewriteSchema(subs, tool.OutputSchema)
		clone.Tools[i] = &tc
	}
	return &clone
}

// rewriteListPrompts rewrites each prompt's description and title and each
// argument's description and title; argument names survive verbatim.
func rewriteListPrompts(subs []Substitution, res *mcp.ListPromptsResult) *mcp.ListPromptsResult {
	clone := *res
	clone.Prompts = make([]*mcp.Prompt, len(res.Prompts))
	for i, prompt := range res.Prompts {
		pc := *prompt
		pc.Description = Apply(subs, prompt.Description)
		pc.Title = Apply(subs, prompt.Title)
		pc.Arguments = make([]*mcp.PromptArgument, len(prompt.Arguments))
		for j, arg := range prompt.Arguments {
			argc := *arg
			argc.Description = Apply(subs, arg.Description)
			argc.Title = Apply(subs, arg.Title)
			pc.Arguments[j] = &argc
		}
		clone.Prompts[i] = &pc
	}
	return &clone
}

// rewriteListResources rewrites each resource's description and title; the
// URI and name survive verbatim.
func rewriteListResources(subs []Substitution, res *mcp.ListResourcesResult) *mcp.ListResourcesResult {
	clone := *res
	clone.Resources = make([]*mcp.Resource, len(res.Resources))
	for i, resource := range res.Resources {
		rc := *resource
		rc.Description = Apply(subs, resource.Description)
		rc.Title = Apply(subs, resource.Title)
		clone.Resources[i] = &rc
	}
	return &clone
}

// rewriteListResourceTemplates mirrors rewriteListResources for templates;
// the URI template survives verbatim.
func rewriteListResourceTemplates(subs []Substitution, res *mcp.ListResourceTemplatesResult) *mcp.ListResourceTemplatesResult {
	clone := *res
	clone.ResourceTemplates = make([]*mcp.ResourceTemplate, len(res.ResourceTemplates))
	for i, template := range res.ResourceTemplates {
		tc := *template
		tc.Description = Apply(subs, template.Description)
		tc.Title = Apply(subs, template.Title)
		clone.ResourceTemplates[i] = &tc
	}
	return &clone
}

// rewriteSchema applies subs to every description and title string a schema
// embeds, however deeply nested, and to nothing else. The schema field is
// typed any (the SDK accepts *jsonschema.Schema, json.RawMessage, or any
// marshalable value), so the one representation every variant shares is its
// JSON: marshal, walk the decoded value, and keep the decoded form only when
// something actually changed — an untouched schema keeps its original typed
// value, and both forms marshal to the same wire bytes.
func rewriteSchema(subs []Substitution, schema any) any {
	if schema == nil {
		return nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return schema
	}
	var decoded any
	if unmarshalErr := json.Unmarshal(raw, &decoded); unmarshalErr != nil {
		return schema
	}
	if !RewriteSchemaProse(decoded, func(text string) string { return Apply(subs, text) }) {
		return schema
	}
	return decoded
}
