package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func appendCapabilityBridgeTools(catalog []modelTool, support mcpBridgeSupport) []modelTool {
	extra := 0
	if support.Capabilities {
		extra++
	}
	if support.Resources {
		extra += 2
	}
	if support.Prompts {
		extra += 2
	}
	if support.Completion {
		extra++
	}
	out := make([]modelTool, 0, len(catalog)+extra)
	out = append(out, catalog...)
	if support.Capabilities {
		out = append(out, modelToolFromParts(capabilityListTool, "Inspect MCP server capability metadata, including server info, instructions, and evaluator bridge tools active for this run. Use this when the task asks about MCP capabilities or the bridge itself; use gitlab_list_resources directly when the task only needs resource URIs.", emptyObjectSchema()))
	}
	if support.Resources {
		out = append(
			out,
			modelToolFromParts(resourceListTool, "List MCP resources and resource templates exposed by this GitLab MCP server instance. Use this to discover resource URIs such as gitlab://tools and URI templates before reading them; it is a valid first step for resource-manifest discovery.", emptyObjectSchema()),
			modelToolFromParts(resourceReadTool, "Read one MCP resource URI exposed by this GitLab MCP server instance. Use URIs returned by gitlab_list_resources, including gitlab://tools and concrete gitlab://tools/{id} values.", resourceReadSchema()),
		)
	}
	if support.Prompts {
		out = append(
			out,
			modelToolFromParts(promptListTool, "List MCP prompt templates exposed by this GitLab MCP server instance.", emptyObjectSchema()),
			modelToolFromParts(promptGetTool, "Render one MCP prompt by name with optional string arguments.", promptGetSchema()),
		)
	}
	if support.Completion {
		out = append(out, modelToolFromParts(completionTool, "Request MCP argument completions for a prompt or resource reference.", completionSchema()))
	}
	return sortedModelTools(out)
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}

func promptGetSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "MCP prompt name returned by gitlab_list_prompts.",
			},
			"arguments": map[string]any{
				"type":                 "object",
				"description":          "Optional prompt arguments as string values.",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	}
}

func completionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ref_type": map[string]any{
				"type":        "string",
				"enum":        []string{"ref/prompt", "ref/resource"},
				"description": "MCP completion reference type.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Prompt name when ref_type is ref/prompt.",
			},
			"uri": map[string]any{
				"type":        "string",
				"description": "Resource URI template or URI when ref_type is ref/resource.",
			},
			"argument_name": map[string]any{
				"type":        "string",
				"description": "Argument name to complete.",
			},
			"argument_value": map[string]any{
				"type":        "string",
				"description": "Current partial value for the argument.",
			},
			"context_arguments": map[string]any{
				"type":                 "object",
				"description":          "Previously resolved prompt/resource arguments as string values.",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
		"required":             []string{"ref_type", "argument_name"},
		"additionalProperties": false,
	}
}

func resourceReadSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"uri": map[string]any{
				"type":        "string",
				"description": "MCP resource URI to read, for example gitlab://tools or a tool detail URI from the active manifest such as gitlab://tools/{id}.",
			},
		},
		"required":             []string{"uri"},
		"additionalProperties": false,
	}
}

// catalogToolNames resolves catalog tool names for evaluator execution.

func (r *modelRunner) capabilityBridgeResult(ctx context.Context, toolUse modelContentBlock) resourceLookupResult {
	if r.mcpSession == nil {
		err := errors.New("MCP capability bridge is not available in this evaluation run")
		return resourceLookupResult{Content: err.Error(), Err: err}
	}
	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	exchange := &traceMCPExchange{Request: traceMCPRequest{Name: toolUse.Name, Arguments: toolUse.Input}}
	started := time.Now()
	var content string
	var err error
	switch toolUse.Name {
	case capabilityListTool:
		content, err = r.listCapabilityLookupContent(exchange)
	case resourceListTool:
		content, err = r.listResourceLookupContent(callCtx, exchange)
	case resourceReadTool:
		content, err = r.readResourceLookupContent(callCtx, toolUse.Input, exchange)
	case promptListTool:
		content, err = r.listPromptLookupContent(callCtx, exchange)
	case promptGetTool:
		content, err = r.getPromptLookupContent(callCtx, toolUse.Input, exchange)
	case completionTool:
		content, err = r.completionLookupContent(callCtx, toolUse.Input, exchange)
	default:
		err = fmt.Errorf("unsupported MCP capability bridge tool %q", toolUse.Name)
	}
	exchange.DurationMillis = time.Since(started).Milliseconds()
	if err != nil {
		exchange.ProtocolError = err.Error()
		return resourceLookupResult{Content: content, Err: err, MCP: exchange}
	}
	return resourceLookupResult{Content: content, MCP: exchange}
}

func (r *modelRunner) listCapabilityLookupContent(exchange *traceMCPExchange) (string, error) {
	initResult := r.mcpSession.InitializeResult()
	if initResult == nil {
		return "", errors.New("list MCP capabilities: empty initialize result")
	}
	output := evalCapabilitiesOutput{
		ProtocolVersion: initResult.ProtocolVersion,
		ServerInfo:      initResult.ServerInfo,
		Capabilities:    initResult.Capabilities,
		Instructions:    initResult.Instructions,
		BridgeTools:     activeBridgeTools(r.mcpBridge),
	}
	return marshalResourceBridgeResult(output, exchange)
}

func (r *modelRunner) listResourceLookupContent(ctx context.Context, exchange *traceMCPExchange) (string, error) {
	resourcesResult, err := r.mcpSession.ListResources(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list MCP resources: %w", err)
	}
	if resourcesResult == nil {
		return "", errors.New("list MCP resources: empty result")
	}
	templatesResult, err := r.mcpSession.ListResourceTemplates(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list MCP resource templates: %w", err)
	}
	if templatesResult == nil {
		return "", errors.New("list MCP resource templates: empty result")
	}
	output := evalResourceListOutput{
		Resources:         make([]evalResourceRef, 0, len(resourcesResult.Resources)),
		ResourceTemplates: make([]evalResourceTemplateRef, 0, len(templatesResult.ResourceTemplates)),
	}
	for _, resource := range resourcesResult.Resources {
		output.Resources = append(output.Resources, evalResourceRef{
			URI:         resource.URI,
			Name:        resource.Name,
			Title:       resource.Title,
			Description: resource.Description,
			MIMEType:    resource.MIMEType,
			Annotations: resource.Annotations,
		})
	}
	for _, template := range templatesResult.ResourceTemplates {
		output.ResourceTemplates = append(output.ResourceTemplates, evalResourceTemplateRef{
			URITemplate: template.URITemplate,
			Name:        template.Name,
			Title:       template.Title,
			Description: template.Description,
			MIMEType:    template.MIMEType,
			Annotations: template.Annotations,
		})
	}
	return marshalResourceBridgeResult(output, exchange)
}

func (r *modelRunner) readResourceLookupContent(ctx context.Context, input map[string]any, exchange *traceMCPExchange) (string, error) {
	uri, _ := input["uri"].(string)
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", errors.New("gitlab_read_resource requires uri")
	}
	result, err := r.mcpSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return "", fmt.Errorf("read MCP resource %s: %w", uri, err)
	}
	if result == nil {
		return "", fmt.Errorf("read MCP resource %s: empty result", uri)
	}
	output := evalResourceReadOutput{URI: uri, Contents: make([]evalResourceContent, 0, len(result.Contents))}
	for _, content := range result.Contents {
		if content == nil {
			continue
		}
		output.Contents = append(output.Contents, evalResourceContent{
			URI:      content.URI,
			MIMEType: content.MIMEType,
			Text:     truncateToolResult(content.Text),
		})
	}
	return marshalResourceBridgeResult(output, exchange)
}

func (r *modelRunner) listPromptLookupContent(ctx context.Context, exchange *traceMCPExchange) (string, error) {
	result, err := r.mcpSession.ListPrompts(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list MCP prompts: %w", err)
	}
	if result == nil {
		return "", errors.New("list MCP prompts: empty result")
	}
	return marshalResourceBridgeResult(evalPromptListOutput{Prompts: result.Prompts}, exchange)
}

func (r *modelRunner) getPromptLookupContent(ctx context.Context, input map[string]any, exchange *traceMCPExchange) (string, error) {
	name, _ := input["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("gitlab_get_prompt requires name")
	}
	result, err := r.mcpSession.GetPrompt(ctx, &mcp.GetPromptParams{Name: name, Arguments: stringMapFromAny(input["arguments"])})
	if err != nil {
		return "", fmt.Errorf("get MCP prompt %s: %w", name, err)
	}
	if result == nil {
		return "", fmt.Errorf("get MCP prompt %s: empty result", name)
	}
	return marshalResourceBridgeResult(result, exchange)
}

func (r *modelRunner) completionLookupContent(ctx context.Context, input map[string]any, exchange *traceMCPExchange) (string, error) {
	params, err := completionParamsFromInput(input)
	if err != nil {
		return "", err
	}
	result, err := r.mcpSession.Complete(ctx, params)
	if err != nil {
		return "", fmt.Errorf("complete MCP argument %s: %w", params.Argument.Name, err)
	}
	if result == nil {
		return "", fmt.Errorf("complete MCP argument %s: empty result", params.Argument.Name)
	}
	return marshalResourceBridgeResult(result, exchange)
}

func activeBridgeTools(support mcpBridgeSupport) []string {
	if !support.any() {
		return nil
	}
	bridgeTools := []string{capabilityListTool}
	if support.Resources {
		bridgeTools = append(bridgeTools, resourceListTool, resourceReadTool)
	}
	if support.Prompts {
		bridgeTools = append(bridgeTools, promptListTool, promptGetTool)
	}
	if support.Completion {
		bridgeTools = append(bridgeTools, completionTool)
	}
	return bridgeTools
}

func completionParamsFromInput(input map[string]any) (*mcp.CompleteParams, error) {
	refType, _ := input["ref_type"].(string)
	refType = strings.TrimSpace(refType)
	argumentName, _ := input["argument_name"].(string)
	argumentName = strings.TrimSpace(argumentName)
	if refType == "" {
		return nil, errors.New("gitlab_complete requires ref_type")
	}
	if argumentName == "" {
		return nil, errors.New("gitlab_complete requires argument_name")
	}
	ref := &mcp.CompleteReference{Type: refType}
	switch refType {
	case "ref/prompt":
		ref.Name, _ = input["name"].(string)
		ref.Name = strings.TrimSpace(ref.Name)
		if ref.Name == "" {
			return nil, errors.New("gitlab_complete requires name for ref/prompt")
		}
	case "ref/resource":
		ref.URI, _ = input["uri"].(string)
		ref.URI = strings.TrimSpace(ref.URI)
		if ref.URI == "" {
			return nil, errors.New("gitlab_complete requires uri for ref/resource")
		}
	default:
		return nil, fmt.Errorf("gitlab_complete unsupported ref_type %q", refType)
	}
	argumentValue, _ := input["argument_value"].(string)
	params := &mcp.CompleteParams{
		Ref: ref,
		Argument: mcp.CompleteParamsArgument{
			Name:  argumentName,
			Value: argumentValue,
		},
	}
	if contextArgs := stringMapFromAny(input["context_arguments"]); len(contextArgs) > 0 {
		params.Context = &mcp.CompleteContext{Arguments: contextArgs}
	}
	return params, nil
}

func stringMapFromAny(raw any) map[string]string {
	values, ok := raw.(map[string]any)
	if !ok || len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if text, isString := value.(string); isString {
			out[key] = text
		}
	}
	return out
}

func marshalResourceBridgeResult(value any, exchange *traceMCPExchange) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal MCP resource result: %w", err)
	}
	content := truncateToolResult(string(data))
	if exchange != nil {
		exchange.Response = append(json.RawMessage(nil), data...)
		exchange.ResponseText = content
	}
	return content, nil
}

// toolExecutionNote converts the GitLab API response to the tool output format.
