package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/roots"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// WorkspaceRootOutput describes a single workspace root URI provided by
// the MCP client. Name is optional; clients may leave it empty when
// the directory only has a path.
type WorkspaceRootOutput struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// WorkspaceRootsOutput is the JSON payload returned by the
// "gitlab://workspace/roots" resource: the list of workspace roots
// currently known to the client and a hint explaining how to use them
// for project discovery.
type WorkspaceRootsOutput struct {
	Roots []WorkspaceRootOutput `json:"roots"`
	Hint  string                `json:"hint"`
}

// RegisterWorkspaceRoots registers the "gitlab://workspace/roots"
// resource, which exposes the client workspace root URIs. LLMs read
// the .git/config in those roots to discover the project via
// gitlab_discover_project.
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

// marshalWorkspaceRootsJSON serializes a [WorkspaceRootsOutput] as a JSON
// text resource suitable for returning from an MCP ReadResource handler.
// The marshaled value is wrapped in a single text contents block tagged
// with the JSON MIME type.
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
