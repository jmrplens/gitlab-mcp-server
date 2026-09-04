// Package auditshared holds analysis helpers shared by the discovery-metadata
// auditors (cmd/audit_1to1 R-META and cmd/audit_discovery_completeness): the
// projected-description probe, owner-package resolution, and the shared
// usage/description quality checks.
package auditshared

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// genericUsageRe matches the placeholder Usage template ("Use to execute …
// action.") that indicates the action has no curated usage text.
var genericUsageRe = regexp.MustCompile(`(?i)^use to execute\b.*\baction\.?\s*$`)

// IsGenericUsage reports whether the Usage string is the placeholder template
// or empty.
func IsGenericUsage(usage string) bool {
	trimmed := strings.TrimSpace(usage)
	return trimmed == "" || genericUsageRe.MatchString(trimmed)
}

// WeakIndividualDescription reports whether the effective individual-tool
// description the model sees lacks the norm's "Returns: … See also: …" form.
// The effective description is the projected mcp.Tool.Description (which
// already resolves the curated-description fallback chain), so this avoids
// false positives from specs that omit IndividualTool.Description yet still
// project a good curated one.
func WeakIndividualDescription(spec toolutil.ActionSpec, projected map[string]string) bool {
	tool := strings.TrimSpace(spec.IndividualTool.Name)
	if tool == "" {
		return false
	}
	description, ok := projected[tool]
	if !ok {
		return false
	}
	return !strings.Contains(description, "Returns:") || !strings.Contains(description, "See also:")
}

// projectionCache memoizes ProjectIndividualDescriptions. Projecting the
// individual surface registers ~1071 tools with schema compilation (~10s),
// the output depends only on the compiled-in catalog (the client is an
// offline stub), and the audits only read the map. One projection per
// process therefore serves every analyzer and every test in a package; the
// one-shot CLIs are unaffected.
var projectionCache struct {
	once         sync.Once
	descriptions map[string]string
}

// specsCache memoizes CachedActionSpecs per enterprise flag, same contract:
// the returned groups are shared and must be treated as read-only.
var specsCache sync.Map // enterprise bool -> *specsResult

type specsResult struct {
	once   sync.Once
	groups []tools.ActionSpecGroup
}

// CachedIndividualDescriptions returns the projected individual-tool
// descriptions, computed once per process. The map is shared: read-only.
func CachedIndividualDescriptions(client *gitlabclient.Client) map[string]string {
	projectionCache.once.Do(func() {
		projectionCache.descriptions = ProjectIndividualDescriptions(client)
	})
	return projectionCache.descriptions
}

// CachedActionSpecs returns the collected action specs for the given tier
// selector, computed once per process and per flag. The slice and everything
// it references are shared: read-only.
func CachedActionSpecs(client *gitlabclient.Client, enterprise bool) []tools.ActionSpecGroup {
	entry, _ := specsCache.LoadOrStore(enterprise, &specsResult{})
	result, _ := entry.(*specsResult)
	result.once.Do(func() {
		result.groups = tools.CollectActionSpecs(client, enterprise)
	})
	return result.groups
}

// ProjectIndividualDescriptions registers the individual-tool surface on an
// in-memory MCP server and returns the projected description per tool name —
// the exact text the model consumes. Prefer CachedIndividualDescriptions
// unless a fresh projection is the point.
//
// Registration projects the catalog compiled into this binary and both ends of
// the transport are this process, so none of the three steps can fail and no
// auditor could do anything with the failure but print it.
func ProjectIndividualDescriptions(client *gitlabclient.Client) map[string]string {
	server := mcp.NewServer(&mcp.Implementation{Name: "audit", Version: "0.0.1"}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}})
	tools.RegisterAll(server, client, edition.Ultimate)
	toolutil.LockdownInputSchemas(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	cmdutil.Must(server.Connect(ctx, serverTransport, nil))

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "audit-client", Version: "0.0.1"}, nil)
	session := cmdutil.Must(mcpClient.Connect(ctx, clientTransport, nil))
	defer func() { _ = session.Close() }()

	result := cmdutil.Must(session.ListTools(ctx, nil))
	descriptions := make(map[string]string, len(result.Tools))
	for _, tool := range result.Tools {
		descriptions[tool.Name] = tool.Description
	}
	return descriptions
}

// OwnerPackage resolves the owning package for an action: the spec override
// wins, then the group owner, then the group base domain.
func OwnerPackage(group tools.ActionSpecGroup, spec toolutil.ActionSpec) string {
	if owner := strings.TrimSpace(spec.OwnerPackage); owner != "" {
		return owner
	}
	if owner := strings.TrimSpace(group.OwnerPackage); owner != "" {
		return owner
	}
	return strings.TrimSpace(group.BaseDomain)
}

// NewStubGitLabClient builds a GitLab client pointed at an in-process HTTP
// stub that answers every request with a fixed version payload. Generators
// and auditors use it to register the tool catalog offline. The returned
// cleanup func shuts the stub server down.
//
// The only thing client construction validates is the base URL, and that one
// comes from the httptest server started two lines earlier.
func NewStubGitLabClient(token string) (client *gitlabclient.Client, cleanup func()) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))

	return cmdutil.Must(gitlabclient.NewClient(&config.Config{
		GitLabURL:   srv.URL,
		GitLabToken: token,
	})), srv.Close
}
