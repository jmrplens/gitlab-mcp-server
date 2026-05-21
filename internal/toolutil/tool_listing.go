package toolutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	serverSession, err := server.Connect(ctx, serverTransport, nil)
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

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	if result == nil {
		return nil, nil
	}
	return result.Tools, nil
}
