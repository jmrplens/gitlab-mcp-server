// gatewaycompat_wiring_test.go verifies that createServer installs the
// description-substitution middleware on the production path when
// GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS is set, that an unset variable
// installs nothing, and that an invalid value fails both createServer and
// runHTTP's early validation instead of surfacing on somebody's first pool
// request.
package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/gatewaycompat"
)

// escapeSubstitutionHalf escapes one half of an old=new pair so arbitrary
// served text can be used as the pattern.
func escapeSubstitutionHalf(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ",", `\,`)
	return strings.ReplaceAll(s, "=", `\=`)
}

// firstDescribedResource returns the URI and description of the first
// resource in the server's listing that carries a description; resources are
// served from the compiled-in registry, so no GitLab backend is involved.
func firstDescribedResource(t *testing.T, session *mcp.ClientSession) (uri, description string) {
	t.Helper()
	res, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, r := range res.Resources {
		if r.Description != "" {
			return r.URI, r.Description
		}
	}
	t.Fatal("no resource with a description; the fixture surface changed")
	return "", ""
}

// TestCreateServer_GatewayCompatWiring verifies the middleware end to end on
// a server built by createServer: a substitution whose pattern is a real
// served description rewrites that description, and only servers built while
// the variable is set are affected. The pattern is read from a control
// server first, so the test holds whatever the catalog's wording becomes.
func TestCreateServer_GatewayCompatWiring(t *testing.T) {
	client := newMockGitLabClient(t)
	cfg := &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic}

	control, err := createServer(t.Context(), client, cfg)
	if err != nil {
		t.Fatalf("createServer (control): %v", err)
	}
	controlSession := connectSessionAs(t, control, &mcp.Implementation{Name: "test-client", Version: "1.0.0"})
	uri, original := firstDescribedResource(t, controlSession)

	const rewritten = "REWRITTEN BY GATEWAYCOMPAT"
	t.Setenv(gatewaycompat.EnvVar, escapeSubstitutionHalf(original)+"="+escapeSubstitutionHalf(rewritten))

	server, err := createServer(t.Context(), client, cfg)
	if err != nil {
		t.Fatalf("createServer (substituted): %v", err)
	}
	session := connectSessionAs(t, server, &mcp.Implementation{Name: "test-client", Version: "1.0.0"})
	res, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources (substituted): %v", err)
	}
	for _, r := range res.Resources {
		if r.URI != uri {
			continue
		}
		if r.Description != rewritten {
			t.Errorf("resource %s description = %q, want %q", uri, r.Description, rewritten)
		}
		return
	}
	t.Fatalf("resource %s missing from the substituted server's listing", uri)
}

// TestCreateServer_GatewayCompatInvalidValue verifies that a malformed
// substitution value fails the server build with an error naming the
// variable, rather than installing a half-parsed rewrite.
func TestCreateServer_GatewayCompatInvalidValue(t *testing.T) {
	t.Setenv(gatewaycompat.EnvVar, "=x")
	client := newMockGitLabClient(t)
	_, err := createServer(t.Context(), client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic})
	if err == nil {
		t.Fatal("createServer accepted a malformed substitution value")
	}
	if !strings.Contains(err.Error(), gatewaycompat.EnvVar) {
		t.Errorf("error = %q, want it to name %s", err, gatewaycompat.EnvVar)
	}
}

// TestRunHTTP_InvalidDescriptionSubstitutions verifies the early validation:
// in HTTP mode the pool builds servers lazily, so the malformed value must
// fail at startup, before the listener comes up.
func TestRunHTTP_InvalidDescriptionSubstitutions(t *testing.T) {
	t.Setenv(gatewaycompat.EnvVar, "abc")
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:      "https://gitlab.example.com",
		maxHTTPClients: config.DefaultMaxHTTPClients,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err == nil {
		t.Fatal("runHTTP accepted a malformed substitution value")
	}
	if !strings.Contains(err.Error(), gatewaycompat.EnvVar) {
		t.Errorf("error = %q, want it to name %s", err, gatewaycompat.EnvVar)
	}
}
