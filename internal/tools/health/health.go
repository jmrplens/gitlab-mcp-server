// Package health implements the gitlab_server_status diagnostic tool for checking
// MCP server health and GitLab connectivity.
package health

import (
	"context"
	"fmt"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Output represents the health/connectivity status of the MCP server.
type Output struct {
	toolutil.HintableOutput
	Status           string `json:"status"`
	MCPServerVersion string `json:"mcp_server_version,omitempty"`
	Author           string `json:"author,omitempty"`
	Department       string `json:"department,omitempty"`
	Repository       string `json:"repository,omitempty"`
	GitLabURL        string `json:"gitlab_url"`
	GitLabVersion    string `json:"gitlab_version,omitempty"`
	GitLabRevision   string `json:"gitlab_revision,omitempty"`
	Authenticated    bool   `json:"authenticated"`
	Username         string `json:"username,omitempty"`
	UserID           int64  `json:"user_id,omitempty"`
	ResponseTimeMS   int64  `json:"response_time_ms"`
	Error            string `json:"error,omitempty"`
}

// Input is an empty struct for the status tool (no parameters needed).
type Input struct{}

// ServerInfo holds static metadata about the MCP server binary,
// injected at registration time from build-time variables.
type ServerInfo struct {
	Version    string
	Author     string
	Department string
	Repository string
}

// serverInfo holds the current server metadata, set via SetServerInfo.
var serverInfo ServerInfo

// SetServerInfo configures the static metadata returned by the status tool.
// Call before RegisterTools.
func SetServerInfo(info ServerInfo) {
	serverInfo = info
}

// Check verifies GitLab connectivity, authentication, and retrieves
// server version and current user info for diagnostic purposes.
func Check(ctx context.Context, client *gitlabclient.Client, _ Input) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	out := Output{
		GitLabURL:        client.GL().BaseURL().String(),
		MCPServerVersion: serverInfo.Version,
		Author:           serverInfo.Author,
		Department:       serverInfo.Department,
		Repository:       serverInfo.Repository,
	}

	start := time.Now()

	v, _, err := client.GL().Version.GetVersion(gl.WithContext(ctx))
	out.ResponseTimeMS = time.Since(start).Milliseconds()

	if err != nil {
		out.Status = "unhealthy"
		out.Error = fmt.Sprintf("connectivity check failed: %v", err)
		return out, nil
	}

	out.GitLabVersion = v.Version
	out.GitLabRevision = v.Revision

	u, _, err := client.GL().Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		out.Status = "degraded"
		out.Authenticated = false
		out.Error = fmt.Sprintf("authenticated but user retrieval failed: %v", err)
		return out, nil
	}

	out.Status = "healthy"
	out.Authenticated = true
	out.Username = u.Username
	out.UserID = u.ID

	return out, nil
}
