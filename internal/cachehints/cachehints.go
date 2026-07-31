// Package cachehints applies SEP-2549 cache hints (ttlMs/cacheScope) to MCP
// results. Every catalog in this server is token- and tier-dependent (PAT
// scope filtering, licensing-tier gating), so all cacheable results are
// marked "private": shared intermediaries must never serve one user's
// catalog to another. TTLs distinguish stable catalogs from live GitLab data.
package cachehints

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TTL hints in milliseconds.
const (
	// listTTLMs marks list catalogs fresh for 5 minutes: they only change
	// when the server process (or its auto-updated binary) changes.
	listTTLMs = 300_000
	// staticReadTTLMs marks static resource reads (docs, manifests, guides,
	// schemas) fresh for 1 hour.
	staticReadTTLMs = 3_600_000
)

// scopePrivate is the only scope this server emits; catalogs and reads are
// token/tier-dependent, so public intermediary caching would leak data
// across users (the SDK default is "public").
const scopePrivate = "private"

// staticResourcePrefixes lists the URI prefixes served entirely from process
// memory: workflow guides ([resources.RegisterWorkflowGuides]), the dynamic
// and meta action schemas, and the tool manifest. Every other gitlab:// URI
// registered in internal/resources is backed by a live GitLab API call, so it
// gets no freshness window.
var staticResourcePrefixes = []string{
	"gitlab://guides/",
	"gitlab://schema/",
	"gitlab://tools",
}

// Middleware returns a receiving middleware that stamps cache hints on every
// CacheableResult according to the package policy.
func Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil || res == nil {
				return res, err
			}
			applyHints(method, req, res)
			return res, nil
		}
	}
}

// applyHints mutates the Cacheable fields embedded in res for the methods
// covered by the policy; other results pass through untouched.
func applyHints(method string, req mcp.Request, res mcp.Result) {
	switch method {
	case "tools/list", "prompts/list", "resources/list", "resources/templates/list", "server/discover":
		setCacheable(res, scopePrivate, listTTLMs)
	case "resources/read":
		setCacheable(res, scopePrivate, readTTL(req))
	}
}

// setCacheable writes the hint into the concrete result type. The Cacheable
// getters have value receivers, so the embedded struct must be assigned
// through the concrete pointer rather than through [mcp.CacheableResult].
// Result types without an embedded Cacheable are left untouched.
func setCacheable(res mcp.Result, scope string, ttlMs int) {
	hint := mcp.Cacheable{TTLMs: ttlMs, CacheScope: scope}
	switch typed := res.(type) {
	case *mcp.ListToolsResult:
		typed.Cacheable = hint
	case *mcp.ListPromptsResult:
		typed.Cacheable = hint
	case *mcp.ListResourcesResult:
		typed.Cacheable = hint
	case *mcp.ListResourceTemplatesResult:
		typed.Cacheable = hint
	case *mcp.DiscoverResult:
		typed.Cacheable = hint
	case *mcp.ReadResourceResult:
		typed.Cacheable = hint
	}
}

// readTTL returns the freshness window for a resources/read result: one hour
// for static content, and 0 (immediately stale) for resources backed by live
// GitLab data or for requests whose URI cannot be determined.
func readTTL(req mcp.Request) int {
	readReq, ok := req.(*mcp.ReadResourceRequest)
	if !ok || readReq.Params == nil {
		return 0
	}
	if isStaticResourceURI(readReq.Params.URI) {
		return staticReadTTLMs
	}
	return 0
}

// isStaticResourceURI reports whether uri is served from process memory and is
// therefore stable for the lifetime of the server instance.
func isStaticResourceURI(uri string) bool {
	for _, prefix := range staticResourcePrefixes {
		if strings.HasPrefix(uri, prefix) {
			return true
		}
	}
	return false
}
