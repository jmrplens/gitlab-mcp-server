// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SurfaceToolRegisterOptions controls how an ActionSpec is exposed as a
// standalone visible MCP tool.
type SurfaceToolRegisterOptions struct {
	Description  string
	Icons        []mcp.Icon
	FormatResult FormatResultFunc
}

type surfaceToolTextOnlyMarker interface {
	// SurfaceToolTextOnly marks this result as text-only for standalone surface tools.
	SurfaceToolTextOnly()
}

// RegisterSurfaceToolFromSpec registers one visible MCP tool by projecting an
// ActionSpec and executing its route handler directly.
func RegisterSurfaceToolFromSpec(server *mcp.Server, spec ActionSpec, opts SurfaceToolRegisterOptions) {
	if server == nil {
		return
	}
	tool, err := IndividualToolFromActionSpec(spec, IndividualToolProjectionOptions{
		Description: opts.Description,
		Icons:       opts.Icons,
	})
	if err != nil {
		panic(err)
	}
	formatResult := opts.FormatResult
	if formatResult == nil {
		formatResult = MarkdownForResult
	}
	// Standalone utility schemas are static per tool name (no tier pruning
	// at this layer), so the compiled form is safely shared across servers.
	CompileToolSchemas(tool, "standalone|"+tool.Name)
	// No catalog projected this spec, and the catalog is where every other
	// action's content kind is resolved, so the route is given it here. This
	// is the meta and individual surfaces' one dispatcher of a spec, and
	// without it the guided flows and project discovery would answer with the
	// assistant default that no catalog action is served any more.
	route := spec.Route
	route.ContentKind = ResolveContentKind(spec)
	mcp.AddTool[map[string]any, any](server, tool, surfaceToolHandler(tool.Name, route, formatResult))
}

func surfaceToolHandler(toolName string, route ActionRoute, formatResult FormatResultFunc) mcp.ToolHandlerFor[map[string]any, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		if withheld := FineGrainedRefusal(ctx, req, toolName, route.ActionID, ""); withheld != nil {
			return withheld, nil, nil
		}
		if route.Destructive {
			message := fmt.Sprintf("Confirm destructive action %q?", toolName)
			guard, guardErr := ConfirmDestructiveAction(ctx, req, input, message)
			if guardErr != nil {
				LogToolCallAll(ctx, req, toolName, start, nil, guardErr)
				return nil, nil, guardErr
			}
			if guard != nil {
				// A guard is a refusal, not a completed call. This was the one
				// place a blocked destructive action was logged, and it was
				// logged as a success.
				LogToolRefusal(ctx, req, toolName, RefusalNeedsConfirmation)
				return guard, nil, nil
			}
		}
		result, err := route.Handler(ContextWithRequest(ctx, req), input)
		// Not every handler goes through the wrapping helpers, and one that
		// wraps a client-go error with %w itself hands the whole upstream body
		// to the response text and to the log line. gitlab_discover_project is
		// registered here and is exactly that shape. See [SanitizeError].
		err = SanitizeError(err)
		if inputResult, needsInput := InputRequiredResultFromError(err); needsInput {
			return inputResult, nil, nil
		}
		LogToolCallAll(ctx, req, toolName, start, result, err)
		if err != nil {
			return nil, nil, FineGrainedErrorNote(ctx, route.ActionID, err)
		}
		callResult, structured := FinishToolResult(FineGrainedNotes(ctx, route.ActionID, formatResult(result), result), result, route, input)
		if callResult.IsError {
			return callResult, nil, nil
		}
		if _, ok := result.(surfaceToolTextOnlyMarker); ok {
			return callResult, nil, nil
		}
		return callResult, structured, nil
	}
}
