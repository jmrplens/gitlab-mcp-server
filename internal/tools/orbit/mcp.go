// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// mcpPath is GitLab's Orbit MCP endpoint, below /api/v4. It speaks MCP's
// Streamable HTTP transport statelessly: a POST carrying one JSON-RPC request
// is answered with one JSON object, with no initialize before it and no
// session, which is how every request here is sent (measured on GitLab.com on
// 2026-10-29: tools/list and tools/call answered without one). client-go has
// no method for it.
const mcpPath = "orbit/mcp"

// The operations an MCP Orbit failure is reported under.
const (
	orbitToolsOp = "orbit_tools"
	orbitDSLOp   = "orbit_dsl"
)

// queryCommand is the Orbit command a graph query runs as, the one whose
// description is the grammar of the query language.
const queryCommand = "query_graph"

// schemaCommand is the Orbit command that answers the graph schema, the one
// source of its compact text since the REST schema route was removed.
const schemaCommand = "get_graph_schema"

// maxToolPages bounds how many tools/list pages one [Tools] call follows.
const maxToolPages = 20

// mcpRequest is one JSON-RPC request to the Orbit MCP endpoint.
type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// mcpResponse is the JSON-RPC answer: a result, or an error object, which
// GitLab sends with 400 for a request it refuses (an unknown tool) and so
// reaches here only when it is sent with 200.
type mcpResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// callMCP sends one JSON-RPC request to the Orbit MCP endpoint at path and
// returns its result. The path is a parameter so a test reaches the request
// that cannot be built, which no constant path produces.
func callMCP(ctx context.Context, client *gitlabclient.Client, path, method string, params any) (json.RawMessage, error) {
	body := mcpRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
	req, err := client.GL().NewRequest(http.MethodPost, path, body, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var answer mcpResponse
	if _, err = client.GL().Do(req, &answer); err != nil {
		return nil, err
	}
	if answer.Error != nil {
		return nil, fmt.Errorf("orbit MCP %s answered error %d: %s", method, answer.Error.Code, answer.Error.Message)
	}
	return answer.Result, nil
}

// mcpToolResult is the result of tools/call: the text the tool answered, and
// whether it is a refusal.
type mcpToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// errToolRefused is what [callMCPTool] wraps a tool result GitLab marked as
// an error in: an unknown command, or a query the Knowledge Graph refused.
var errToolRefused = errors.New("refused")

// callMCPTool calls one tool of the Orbit MCP endpoint and returns the text
// it answered, the text blocks joined. A result marked isError is an error
// carrying that text, which says what GitLab refused.
func callMCPTool(ctx context.Context, client *gitlabclient.Client, path, name string, arguments map[string]any) (string, error) {
	raw, err := callMCP(ctx, client, path, "tools/call", map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return "", err
	}
	var result mcpToolResult
	if err = json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode the result of Orbit MCP tool %s: %w", name, err)
	}
	texts := make([]string, 0, len(result.Content))
	for _, block := range result.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		}
	}
	text := strings.Join(texts, "\n")
	if result.IsError {
		return "", fmt.Errorf("orbit MCP tool %s %w: %s", name, errToolRefused, text)
	}
	return text, nil
}

// mcpTool is one tool of a tools/list answer.
type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Trusted     *bool           `json:"trusted"`
}

// Tools retrieves the tools GitLab's Orbit MCP endpoint serves, with the JSON
// Schema of each tool's input.
//
// Source: tools/list at POST /api/v4/orbit/mcp, which replaced GET
// /api/v4/orbit/tools when gitlab-org/gitlab!265998 removed the route on
// 2026-10-28 (client-go's Orbit.GetTools still sends that GET). GitLab offers
// the tool list nowhere else. A list GitLab answers in pages is followed by
// its nextCursor.
func Tools(ctx context.Context, client *gitlabclient.Client, _ ToolsInput) (ToolsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ToolsOutput{}, err
	}
	page, err := toolsPage(ctx, client, map[string]any{})
	if err != nil {
		return ToolsOutput{}, err
	}
	out := ToolsOutput{}
	for pages := 1; ; pages++ {
		for _, tool := range page.Tools {
			out.Tools = append(out.Tools, ToolDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  decodeRaw(tool.InputSchema),
				Trusted:     tool.Trusted,
			})
		}
		if page.NextCursor == "" {
			return out, nil
		}
		if pages == maxToolPages {
			return ToolsOutput{}, toolutil.WrapErr(orbitToolsOp, fmt.Errorf("the Orbit MCP tool list did not end within %d pages", maxToolPages))
		}
		//gitlab:request optional: a later tools/list page is asked only when GitLab names a cursor, which it did not on 2026-10-29
		page, err = toolsPage(ctx, client, map[string]any{"cursor": page.NextCursor})
		if err != nil {
			return ToolsOutput{}, err
		}
	}
}

// mcpToolsPage is one page of a tools/list answer.
type mcpToolsPage struct {
	Tools      []mcpTool `json:"tools"`
	NextCursor string    `json:"nextCursor"`
}

// toolsPage asks the Orbit MCP endpoint for one page of its tool list.
func toolsPage(ctx context.Context, client *gitlabclient.Client, params map[string]any) (mcpToolsPage, error) {
	raw, err := callMCP(ctx, client, mcpPath, "tools/list", params)
	if err != nil {
		return mcpToolsPage{}, wrapOrbitErr(orbitToolsOp, err)
	}
	var page mcpToolsPage
	if err = json.Unmarshal(raw, &page); err != nil {
		return mcpToolsPage{}, toolutil.WrapErr(orbitToolsOp, fmt.Errorf("decode the Orbit MCP tool list: %w", err))
	}
	return page, nil
}

// DSL retrieves the query language GitLab.com compiles orbit.query in, as the
// query_graph command describes it.
//
// Source: the list_commands tool of GitLab's Orbit MCP endpoint, called with
// command_names [query_graph], which replaced GET /api/v4/orbit/agent/commands
// when gitlab-org/gitlab!266009 removed the route on 2026-10-28. With format
// raw the tool answers the same list of commands the route did, and the
// query_graph entry gives the grammar's description and the JSON Schema of the
// command's input; with llm it answers the command as compact text. The tool
// defaults to llm and the route defaulted to raw, so raw is sent when the
// caller chose none, which keeps the answer this action has always given.
func DSL(ctx context.Context, client *gitlabclient.Client, input DSLInput) (DSLOutput, error) {
	if err := ctx.Err(); err != nil {
		return DSLOutput{}, err
	}
	format, _, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return DSLOutput{}, err
	}
	sent := gl.OrbitResponseFormatRaw
	if format != nil {
		sent = *format
	}
	text, err := callMCPTool(ctx, client, mcpPath, "list_commands", map[string]any{
		"command_names": []string{queryCommand},
		"format":        string(sent),
	})
	if err != nil {
		return DSLOutput{}, wrapOrbitErr(orbitDSLOp, err)
	}
	if sent == gl.OrbitResponseFormatLLM {
		return DSLOutput{ResponseFormat: responseFormatName(format), FormattedText: text}, nil
	}
	out, err := decodeQueryCommand(text)
	if err != nil {
		return DSLOutput{}, toolutil.WrapErr(orbitDSLOp, err)
	}
	out.ResponseFormat = responseFormatName(format)
	return out, nil
}

// agentCommand is one command of the raw answer of list_commands.
type agentCommand struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// decodeQueryCommand reads the query_graph command out of the raw answer of
// list_commands, a list of commands. A list that holds no query_graph command
// answers nothing, which the card says rather than describing another command
// as the query language.
func decodeQueryCommand(text string) (DSLOutput, error) {
	var commands []agentCommand
	if err := json.Unmarshal([]byte(text), &commands); err != nil {
		return DSLOutput{}, fmt.Errorf("decode the query command: %w", err)
	}
	for _, command := range commands {
		if command.Name == queryCommand {
			return DSLOutput{Name: command.Name, Description: command.Description, Parameters: decodeRaw(command.Parameters)}, nil
		}
	}
	return DSLOutput{}, nil
}

// schemaText reads the Orbit graph schema as compact text: the get_graph_schema
// command of GitLab's Orbit MCP endpoint with format llm, and the nodes named
// in expand as its expand_nodes. GitLab's GraphQL schema has no text rendering
// and no other source of one remains, so this is the one MCP call
// [Schema] makes.
func schemaText(ctx context.Context, client *gitlabclient.Client, path string, expand []string) (SchemaOutput, error) {
	parameters := map[string]any{"format": string(gl.OrbitResponseFormatLLM)}
	if len(expand) > 0 {
		parameters["expand_nodes"] = expand
	}
	text, err := callMCPTool(ctx, client, path, "invoke_command", map[string]any{
		"command_name": schemaCommand,
		"parameters":   parameters,
	})
	if err != nil {
		return SchemaOutput{}, wrapOrbitErr(orbitSchemaOp, err)
	}
	return SchemaOutput{FormattedText: text}, nil
}
