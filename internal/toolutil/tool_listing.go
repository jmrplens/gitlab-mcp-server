package toolutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectInspected and listInspectedTools are the two calls ListRegisteredTools
// makes that the in-memory pair it builds never fails: go-sdk's Server.Connect
// fails only when its transport's Connect does, which an in-memory transport
// never does, and ListTools answers a nil result without an error only for a
// server that sent a null one, which a go-sdk server does not. The branches
// guarding them stay, since the listing is only as good as what the SDK
// promises, and these are package variables so a test can take them.
var (
	connectInspected = func(ctx context.Context, server *mcp.Server, transport mcp.Transport) (*mcp.ServerSession, error) {
		return server.Connect(ctx, transport, nil)
	}
	listInspectedTools = func(ctx context.Context, session *mcp.ClientSession) (*mcp.ListToolsResult, error) {
		return session.ListTools(ctx, nil)
	}
)

// ListRegisteredTools lists tools registered on a server through an ephemeral
// in-memory MCP client session.
func ListRegisteredTools(ctx context.Context, server *mcp.Server, clientName string) ([]*mcp.Tool, error) {
	if server == nil {
		return nil, errors.New("server is nil")
	}
	if clientName == "" {
		clientName = "tool-list-client"
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	// Marked so the rate limiter knows this listing is the server asking
	// itself, and leaves the caller's catalog bucket alone.
	serverSession, err := connectInspected(WithInternalInspection(ctx), server, serverTransport)
	if err != nil {
		return nil, fmt.Errorf("connect server: %w", err)
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: "0"}, nil)
	clientSession, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect client: %w", err)
	}
	defer clientSession.Close()

	result, err := listInspectedTools(ctx, clientSession)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	if result == nil {
		return nil, nil
	}
	return result.Tools, nil
}
