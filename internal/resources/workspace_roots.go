// workspace_roots.go registers the "gitlab://workspace/roots" resource that exposes
// MCP client workspace roots to LLMs, enabling project discovery from local
// repository paths.
package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/roots"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// WorkspaceRootOutput describes a single workspace root provided by the MCP client.
type WorkspaceRootOutput struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// WorkspaceRootsOutput holds the list of workspace roots and a hint for project discovery.
type WorkspaceRootsOutput struct {
	Roots []WorkspaceRootOutput `json:"roots"`
	Hint  string                `json:"hint"`
}

// RegisterWorkspaceRoots registers the "gitlab://workspace/roots" resource.
// It exposes the client workspace root URIs so LLMs can read .git/config
// and use gitlab_discover_project to discover the project.
func RegisterWorkspaceRoots(server *mcp.Server, rootsMgr *roots.Manager) {
	server.AddResource(&mcp.Resource{
		URI:      "gitlab://workspace/roots",
		Name:     "workspace_roots",
		MIMEType: mimeJSON,
		Icons:    toolutil.IconProject,
		Description: "List workspace root directories provided by the MCP client. " +
			"Use these paths to locate .git/config files and extract git remote URLs " +
			"for project discovery via gitlab_discover_project.",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		cachedRoots := rootsMgr.GetRoots()
		out := WorkspaceRootsOutput{
			Roots: make([]WorkspaceRootOutput, 0, len(cachedRoots)),
			Hint: "To discover the GitLab project: " +
				"1) Read .git/config from a root directory to find [remote \"origin\"] url = ... " +
				"2) Call gitlab_discover_project with that URL to get the project_id.",
		}
		for _, r := range cachedRoots {
			out.Roots = append(out.Roots, WorkspaceRootOutput{
				URI:  r.URI,
				Name: r.Name,
			})
		}
		return marshalWorkspaceRootsJSON(out)
	})
}

// marshalWorkspaceRootsJSON serializes a WorkspaceRootsOutput as a JSON text
// resource suitable for returning from an MCP ReadResource handler.
func marshalWorkspaceRootsJSON(v WorkspaceRootsOutput) (*mcp.ReadResourceResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal workspace roots: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			MIMEType: mimeJSON,
			Text:     string(data),
		}},
	}, nil
}
