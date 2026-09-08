package health

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// publicBaseURL renders base with the three parts a secret can hide in
// removed: the userinfo, the query and the fragment. What survives is the
// scheme, the host and the path, which is what identifies the instance and
// all this diagnostics field was ever meant to report.
//
// GITLAB_URL is validated for scheme and host only, so an operator may
// configure any of the three (a proxy credential in the userinfo is the
// realistic one), and this value is handed to whichever MCP client called the
// tool and copied into whatever it logs. client-go builds its own error text
// the same way, from scheme, host and path (gitlab.go, ErrorResponse.Error),
// so this leaves the two agreeing rather than the field being the one place
// that says more.
func publicBaseURL(base *url.URL) string {
	return base.Scheme + "://" + base.Host + base.EscapedPath()
}

// withoutUserinfo removes user from text, for the error strings that come
// back from a request made against a base URL carrying one.
//
// Three renderings have to be caught, because the credential reaches the
// message through three different writers: url.URL.String writes the userinfo
// verbatim, the *url.Error net/http returns replaces the password with ***,
// and url.URL.Redacted replaces it with xxxxx. Only the password is hidden by
// any of them, so the username reaches the caller unless it is stripped here.
//
// The username itself comes in two spellings, and a name needing no escaping
// hides that: net/http's mask is built from Userinfo.Username, which is
// decoded, while Redacted rebuilds the userinfo through url.UserPassword and
// so escapes it. A proxy user called "proxy@corp" is "proxy@corp:***" in one
// message and "proxy%40corp:xxxxx" in the other, so both are stripped.
//
// The forms are ordered longest first so a shorter one cannot match inside a
// longer one, and an empty or repeated one is skipped: the empty one would
// otherwise turn into a stray "@" that eats every at-sign in the message.
func withoutUserinfo(text string, user *url.Userinfo) string {
	if user == nil {
		return text
	}
	decoded := user.Username()
	escaped := url.User(decoded).String()
	seen := make(map[string]bool, 7)
	for _, form := range []string{
		user.String(),
		escaped + ":***", decoded + ":***",
		escaped + ":xxxxx", decoded + ":xxxxx",
		escaped, decoded,
	} {
		if form == "" || seen[form] {
			continue
		}
		seen[form] = true
		text = strings.ReplaceAll(text, form+"@", "")
	}
	return text
}

// Check verifies GitLab connectivity, authentication, and retrieves
// server version and current user info for diagnostic purposes.
func Check(ctx context.Context, client *gitlabclient.Client, _ Input) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	base := client.GL().BaseURL()

	out := Output{
		GitLabURL:        publicBaseURL(base),
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
		out.Error = withoutUserinfo(fmt.Sprintf("connectivity check failed: %v", err), base.User)
		return out, nil
	}

	out.GitLabVersion = v.Version
	out.GitLabRevision = v.Revision

	u, _, err := client.GL().Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		out.Status = "degraded"
		out.Authenticated = false
		out.Error = withoutUserinfo(fmt.Sprintf("authenticated but user retrieval failed: %v", err), base.User)
		return out, nil
	}

	out.Status = "healthy"
	out.Authenticated = true
	out.Username = u.Username
	out.UserID = u.ID

	return out, nil
}
