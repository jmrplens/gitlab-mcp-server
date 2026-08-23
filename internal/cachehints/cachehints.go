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
//
// One hour is the ceiling for anything catalog-shaped. The only event that
// legitimately changes the tool set under a running server is a new binary,
// and auto-update checks hourly by default, so a longer window could outlive
// the build that issued it.
const (
	// staticListTTLMs marks the catalogs compiled into the binary — prompts,
	// resources and resource templates — fresh for 1 hour. None of them
	// depends on the licensing tier or on the token's scopes, so nothing at
	// runtime can change them.
	staticListTTLMs = 3_600_000
	// detectedListTTLMs marks the tool catalog fresh for 5 minutes when the
	// licensing tier is detected per instance rather than configured. A tier
	// change adds or removes whole tool families, and the shorter window
	// bounds how long a client keeps calling a catalog that no longer matches
	// its license.
	detectedListTTLMs = 300_000
	// staticReadTTLMs marks static resource reads (docs, manifests, guides,
	// schemas) fresh for 1 hour.
	staticReadTTLMs = 3_600_000
)

// scopePrivate is the only scope this server emits; catalogs and reads are
// token/tier-dependent, so public intermediary caching would leak data
// across users (the SDK default is "public").
const scopePrivate = "private"

// staticResourcePrefixes lists the URI prefixes served entirely from process
// memory and independent of the catalog: workflow guides
// ([resources.RegisterWorkflowGuides]) and the dynamic and meta action
// schemas. Every other gitlab:// URI registered in internal/resources is
// backed by a live GitLab API call, so it gets no freshness window.
var staticResourcePrefixes = []string{
	"gitlab://guides/",
	"gitlab://schema/",
}

// toolManifestPrefix covers gitlab://tools and gitlab://tools/{id}. Both are
// served from process memory, but they describe the visible tool catalog, so
// they vary with the same tier input as tools/list and share its window rather
// than the static one. Caching the manifest for an hour while the tool list
// refreshes every five minutes would let a client hold two disagreeing views
// of the same catalog.
const toolManifestPrefix = "gitlab://tools"

// Options configures the freshness windows the middleware stamps.
type Options struct {
	// TierPinned reports that the licensing tier was configured rather than
	// detected from the instance license, so it cannot change while the
	// server runs. It lifts the tool catalog to the same one-hour window as
	// the catalogs compiled into the binary.
	//
	// A token's scopes are still detected per pool entry and also filter the
	// catalog, but that variation is bounded by the client's own token and
	// every hint is stamped "private", so no other client can observe it.
	TierPinned bool
}

// Middleware returns a receiving middleware that stamps cache hints on every
// CacheableResult according to the package policy.
func Middleware(opts Options) mcp.Middleware {
	toolListTTL := detectedListTTLMs
	if opts.TierPinned {
		toolListTTL = staticListTTLMs
	}
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil || res == nil {
				return res, err
			}
			applyHints(method, req, res, toolListTTL)
			return res, nil
		}
	}
}

// applyHints mutates the Cacheable fields embedded in res for the methods
// covered by the policy; other results pass through untouched.
func applyHints(method string, req mcp.Request, res mcp.Result, toolListTTL int) {
	switch method {
	case "prompts/list", "resources/list", "resources/templates/list":
		// Compiled into the binary; no runtime input can change them.
		setCacheable(res, scopePrivate, staticListTTLMs)
	case "tools/list", "server/discover":
		// Tier-dependent: the tool catalog, and the instructions and
		// capabilities that discover carries alongside it.
		setCacheable(res, scopePrivate, toolListTTL)
	case "resources/read":
		setCacheable(res, scopePrivate, readTTL(req, toolListTTL))
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

// readTTL returns the freshness window for a resources/read result: the tool
// catalog's own window for the tool manifest, one hour for static content
// independent of the catalog, and 0 (immediately stale) for resources backed
// by live GitLab data or for requests whose URI cannot be determined.
func readTTL(req mcp.Request, toolListTTL int) int {
	readReq, ok := req.(*mcp.ReadResourceRequest)
	if !ok || readReq.Params == nil {
		return 0
	}
	uri := readReq.Params.URI
	if strings.HasPrefix(uri, toolManifestPrefix) {
		return toolListTTL
	}
	if isStaticResourceURI(uri) {
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
