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

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

// ProjectIndividualDescriptions registers the individual-tool surface on an
// in-memory MCP server and returns the projected description per tool name —
// the exact text the model consumes.
func ProjectIndividualDescriptions(client *gitlabclient.Client) (map[string]string, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "audit", Version: "0.0.1"}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}})
	tools.RegisterAll(server, client, edition.Ultimate)
	toolutil.LockdownInputSchemas(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		return nil, fmt.Errorf("connect server: %w", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "audit-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect client: %w", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	descriptions := make(map[string]string, len(result.Tools))
	for _, tool := range result.Tools {
		descriptions[tool.Name] = tool.Description
	}
	return descriptions, nil
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
func NewStubGitLabClient(token string) (*gitlabclient.Client, func(), error) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))

	cfg := &config.Config{
		GitLabURL:   srv.URL,
		GitLabToken: token,
	}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		srv.Close()
		return nil, nil, fmt.Errorf("create stub gitlab client: %w", err)
	}
	return client, srv.Close, nil
}
