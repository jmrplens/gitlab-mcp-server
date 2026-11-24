// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The operations a command catalog read is reported under.
const (
	orbitCommandListOp    = "orbit_command_list"
	orbitResponseFormatOp = "orbit_response_format"
)

// responseFormatCommand is the Orbit command that answers the JSON Schema of
// a query's raw answer.
const responseFormatCommand = "get_response_format"

// maxCommandNames bounds how many command names one [CommandList] call sends.
// GitLab served three commands on 2026-10-29; a list longer than this names
// commands that do not exist.
const maxCommandNames = 20

// CommandListInput selects the commands to describe and the shape of the
// answer.
type CommandListInput struct {
	// CommandNames names the commands to describe; empty describes every
	// command GitLab serves.
	CommandNames []string `json:"command_names,omitempty" jsonschema:"Names of the Orbit commands to describe (query_graph, get_graph_schema, get_response_format). Empty describes every command."`
	ResponseFormatInput
}

// Command is one command of GitLab's Orbit MCP endpoint.
type Command struct {
	// Name is the command's name as GitLab spells it.
	Name string `json:"name"`
	// Description is GitLab's description of the command.
	Description string `json:"description,omitempty"`
	// Parameters is the JSON Schema of the command's input, as GitLab sent it.
	Parameters any `json:"parameters,omitempty"`
	// PerformedBy is the action of this server that runs the command, empty
	// for a command none runs.
	PerformedBy string `json:"performed_by,omitempty"`
}

// CommandListOutput is the command catalog of GitLab's Orbit MCP endpoint.
type CommandListOutput struct {
	toolutil.HintableOutput
	// ResponseFormat is the format that produced the answer.
	ResponseFormat string `json:"response_format,omitempty"`
	// Commands are the commands described, for the raw format.
	Commands []Command `json:"commands,omitempty"`
	// FormattedText is the compact text GitLab answered the llm format with.
	FormattedText string `json:"formatted_text,omitempty"`
}

// QueryResponseFormatInput selects the shape of the answer.
type QueryResponseFormatInput struct {
	ResponseFormatInput
}

// QueryResponseFormatOutput is the JSON Schema of the raw answer of a query.
type QueryResponseFormatOutput struct {
	toolutil.HintableOutput
	// ResponseFormat is the format that produced the answer.
	ResponseFormat string `json:"response_format,omitempty"`
	// Version is the version of the response format the schema describes.
	Version string `json:"version,omitempty"`
	// Schema is the JSON Schema document of a query's raw answer, as GitLab
	// sent it.
	Schema any `json:"schema,omitempty"`
	// FormattedText is the compact text GitLab answered the llm format with.
	FormattedText string `json:"formatted_text,omitempty"`
}

// commandActions names the action of this server that runs each command
// GitLab's Orbit MCP endpoint served on 2026-10-29, so a model reading the
// catalog is pointed at an action it can call: no action invokes a command by
// name (decided on issue 1209).
var commandActions = map[string]string{
	queryCommand:          orbitActionQuery,
	schemaCommand:         orbitActionSchema,
	responseFormatCommand: orbitActionResponseFormat,
}

// CommandList retrieves the command catalog of GitLab's Orbit MCP endpoint:
// the name, description and input schema of each command, and the action of
// this server that runs it.
//
// Source: the list_commands tool of POST /api/v4/orbit/mcp, which replaced GET
// /api/v4/orbit/agent/commands when gitlab-org/gitlab!266009 removed the route
// on 2026-10-28. The tool defaults to llm and the route defaulted to raw, so
// raw is sent when the caller chose none. A name GitLab does not serve is
// answered by the tool as an error naming it.
func CommandList(ctx context.Context, client *gitlabclient.Client, input CommandListInput) (CommandListOutput, error) {
	if err := ctx.Err(); err != nil {
		return CommandListOutput{}, err
	}
	names, err := commandNames(input.CommandNames)
	if err != nil {
		return CommandListOutput{}, err
	}
	format, sent, err := mcpFormat(input.ResponseFormat)
	if err != nil {
		return CommandListOutput{}, err
	}
	arguments := map[string]any{"format": string(sent)}
	if len(names) > 0 {
		arguments["command_names"] = names
	}
	text, err := callMCPTool(ctx, client, mcpPath, "list_commands", arguments)
	if err != nil {
		return CommandListOutput{}, wrapCommandErr(orbitCommandListOp, err)
	}
	out := CommandListOutput{ResponseFormat: responseFormatName(format)}
	if sent == gl.OrbitResponseFormatLLM {
		out.FormattedText = text
		return out, nil
	}
	var commands []agentCommand
	if err = json.Unmarshal([]byte(text), &commands); err != nil {
		return CommandListOutput{}, toolutil.WrapErr(orbitCommandListOp, fmt.Errorf("decode the Orbit command list: %w", err))
	}
	out.Commands = make([]Command, 0, len(commands))
	for _, command := range commands {
		out.Commands = append(out.Commands, Command{
			Name:        command.Name,
			Description: command.Description,
			Parameters:  decodeRaw(command.Parameters),
			PerformedBy: commandActions[command.Name],
		})
	}
	return out, nil
}

// QueryResponseFormat retrieves the JSON Schema of the raw answer of an Orbit
// query, the envelope orbit.query decodes.
//
// Source: the get_response_format command of POST /api/v4/orbit/mcp, invoked
// through invoke_command, which replaced GET /api/v4/orbit/schema/format when
// gitlab-org/gitlab!265633 removed the route on 2026-10-27. With raw the
// command answers {schema, version}; with llm the same schema as text. raw is
// sent when the caller chose none, the route's default.
func QueryResponseFormat(ctx context.Context, client *gitlabclient.Client, input QueryResponseFormatInput) (QueryResponseFormatOutput, error) {
	if err := ctx.Err(); err != nil {
		return QueryResponseFormatOutput{}, err
	}
	format, sent, err := mcpFormat(input.ResponseFormat)
	if err != nil {
		return QueryResponseFormatOutput{}, err
	}
	text, err := callMCPTool(ctx, client, mcpPath, "invoke_command", map[string]any{
		"command_name": responseFormatCommand,
		"parameters":   map[string]any{"format": string(sent)},
	})
	if err != nil {
		return QueryResponseFormatOutput{}, wrapCommandErr(orbitResponseFormatOp, err)
	}
	out := QueryResponseFormatOutput{ResponseFormat: responseFormatName(format)}
	if sent == gl.OrbitResponseFormatLLM {
		out.FormattedText = text
		return out, nil
	}
	var answer struct {
		Schema  json.RawMessage `json:"schema"`
		Version string          `json:"version"`
	}
	if err = json.Unmarshal([]byte(text), &answer); err != nil {
		return QueryResponseFormatOutput{}, toolutil.WrapErr(orbitResponseFormatOp, fmt.Errorf("decode the Orbit response format: %w", err))
	}
	out.Version = answer.Version
	out.Schema = decodeRaw(answer.Schema)
	return out, nil
}

// mcpFormat normalizes the response format of a read from the Orbit MCP
// endpoint: the format the caller named (nil for none), and the one sent,
// raw when the caller named none.
func mcpFormat(format string) (named *gl.OrbitResponseFormatValue, sent gl.OrbitResponseFormatValue, err error) {
	named, _, err = responseFormat(format, "response_format")
	if err != nil {
		return nil, "", err
	}
	if named == nil {
		return nil, gl.OrbitResponseFormatRaw, nil
	}
	return named, *named, nil
}

// commandNames trims the command names, drops the empty ones and refuses a
// list longer than [maxCommandNames].
func commandNames(names []string) ([]string, error) {
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	if len(kept) > maxCommandNames {
		return nil, fmt.Errorf("%s: command_names names %d commands and at most %d are taken: GitLab serves three", orbitCommandListOp, len(kept), maxCommandNames)
	}
	return kept, nil
}

// commandRefusedHint follows a command catalog read GitLab refused as a
// command error, which is how it answers a command name it does not serve.
const commandRefusedHint = "orbit.command_list with no command_names lists every command GitLab serves"

// wrapCommandErr classifies a failed read of the Orbit command catalog: a
// refusal GitLab answered as a tool error names what it refused and is
// followed by [commandRefusedHint], and anything else is classified the way
// every Orbit failure is, by [wrapOrbitErr].
func wrapCommandErr(op string, err error) error {
	if errors.Is(err, errToolRefused) {
		return fmt.Errorf("%s: %w. Suggestion: %s", op, err, commandRefusedHint)
	}
	return wrapOrbitErr(op, err)
}
