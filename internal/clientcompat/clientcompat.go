// Package clientcompat applies per-client response compatibility profiles to
// MCP results. Most MCP clients ignore fields they do not understand, so the
// server ships its full surface (icons, content annotations, structured
// content) unconditionally. OpenAI Codex is the exception: the Codex builds
// bundled with ChatGPT.app (verified on codex-cli 0.148.0-alpha.9, rmcp
// 3.0.0) fail any result whose annotations carry a non-integer priority —
// the response degrades to rmcp's CustomResult and every affected call
// surfaces as "Unexpected response type". This package detects Codex from
// the session's initialize clientInfo and rounds the priority to the nearest
// spec-legal integer (0 or 1) for that session; audience, structuredContent,
// outputSchema, icons, and every other field are preserved, and every other
// client keeps the exact float values.
package clientcompat

import (
	"context"
	"math"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Profile identifies the response compatibility profile negotiated for a
// client session.
type Profile int

const (
	// ProfileDefault leaves responses untouched.
	ProfileDefault Profile = iota
	// ProfileCodex rounds the float priority on content and resource
	// annotations to 0 or 1 (the bundled rmcp parser rejects non-integer
	// priorities).
	// Everything else — audience, structuredContent, outputSchema, icons,
	// and the tool annotations Codex's approval policy depends on — is
	// preserved.
	ProfileCodex
)

// envDisable is the environment kill-switch: CLIENT_COMPAT=off disables the
// middleware entirely (both stdio and HTTP modes read the process env).
const envDisable = "CLIENT_COMPAT"

// Enabled reports whether the compatibility middleware should be installed.
// Any value other than "off" (default empty included) enables it.
func Enabled() bool {
	return !strings.EqualFold(os.Getenv(envDisable), "off")
}

// profileFromClientInfo maps the initialize clientInfo to a Profile. Codex
// has identified itself as name "codex-mcp-client" / title "Codex" since
// v0.20, so a case-insensitive "codex" substring over both fields is stable
// and future-proof.
func profileFromClientInfo(impl *mcp.Implementation) Profile {
	if impl == nil {
		return ProfileDefault
	}
	name := strings.ToLower(impl.Name)
	title := strings.ToLower(impl.Title)
	if strings.Contains(name, "codex") || strings.Contains(title, "codex") {
		return ProfileCodex
	}
	return ProfileDefault
}

// profileForRequest resolves the Profile for the session that issued req.
// Sessions without initialize params (e.g. synthesized stateless-HTTP
// sessions) fall back to ProfileDefault.
func profileForRequest(req mcp.Request) Profile {
	if req == nil {
		return ProfileDefault
	}
	ss, ok := req.GetSession().(*mcp.ServerSession)
	if !ok || ss == nil {
		return ProfileDefault
	}
	params := ss.InitializeParams()
	if params == nil {
		return ProfileDefault
	}
	return profileFromClientInfo(params.ClientInfo)
}

// Middleware returns a receiving middleware that rewrites results according
// to the session's client profile. Results are cloned before modification:
// list results return pointers shared with the server's registries, so
// in-place edits would leak into concurrent sessions of other clients.
func Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil || res == nil {
				return res, err
			}
			if profileForRequest(req) != ProfileCodex {
				return res, nil
			}
			return sanitizeForCodex(res), nil
		}
	}
}

// sanitizeForCodex rewrites the results that can carry float-priority
// annotations; every other result passes through untouched.
func sanitizeForCodex(res mcp.Result) mcp.Result {
	switch typed := res.(type) {
	case *mcp.CallToolResult:
		return sanitizeCallToolResult(typed)
	case *mcp.ListResourcesResult:
		return sanitizeListResources(typed)
	case *mcp.ListResourceTemplatesResult:
		return sanitizeListResourceTemplates(typed)
	case *mcp.GetPromptResult:
		return sanitizeGetPrompt(typed)
	}
	return res
}

// roundPriority returns a copy of a with the priority rounded to the
// nearest integer the spec allows (0 or 1) — Codex's parser accepts integer
// priorities, and rounding keeps the field while staying inside the spec's
// 0–1 range. A rounded 0 is omitted from the wire format. Audience and
// lastModified survive. Nil input passes through.
func roundPriority(a *mcp.Annotations) *mcp.Annotations {
	if a == nil || a.Priority == float64(int64(a.Priority)) {
		return a
	}
	clone := *a
	clone.Priority = math.Round(a.Priority)
	return &clone
}

// sanitizeCallToolResult rounds the float priority on every content
// block's annotations; text, structuredContent, and isError are untouched.
func sanitizeCallToolResult(res *mcp.CallToolResult) *mcp.CallToolResult {
	clone := *res
	clone.Content = roundContentPriorities(res.Content)
	return &clone
}

// roundContentPriorities returns a copy of blocks whose annotation
// priorities are rounded. Unknown content types are passed through
// unchanged.
func roundContentPriorities(blocks []mcp.Content) []mcp.Content {
	if len(blocks) == 0 {
		return blocks
	}
	out := make([]mcp.Content, len(blocks))
	for i, block := range blocks {
		switch c := block.(type) {
		case *mcp.TextContent:
			cc := *c
			cc.Annotations = roundPriority(c.Annotations)
			out[i] = &cc
		case *mcp.ImageContent:
			cc := *c
			cc.Annotations = roundPriority(c.Annotations)
			out[i] = &cc
		case *mcp.AudioContent:
			cc := *c
			cc.Annotations = roundPriority(c.Annotations)
			out[i] = &cc
		case *mcp.ResourceLink:
			cc := *c
			cc.Annotations = roundPriority(c.Annotations)
			out[i] = &cc
		case *mcp.EmbeddedResource:
			cc := *c
			cc.Annotations = roundPriority(c.Annotations)
			out[i] = &cc
		default:
			out[i] = block
		}
	}
	return out
}

// sanitizeListResources rounds the float priority on resource annotations.
func sanitizeListResources(res *mcp.ListResourcesResult) *mcp.ListResourcesResult {
	clone := *res
	clone.Resources = make([]*mcp.Resource, len(res.Resources))
	for i, r := range res.Resources {
		rc := *r
		rc.Annotations = roundPriority(r.Annotations)
		clone.Resources[i] = &rc
	}
	return &clone
}

// sanitizeListResourceTemplates mirrors sanitizeListResources for templates.
func sanitizeListResourceTemplates(res *mcp.ListResourceTemplatesResult) *mcp.ListResourceTemplatesResult {
	clone := *res
	clone.ResourceTemplates = make([]*mcp.ResourceTemplate, len(res.ResourceTemplates))
	for i, r := range res.ResourceTemplates {
		rc := *r
		rc.Annotations = roundPriority(r.Annotations)
		clone.ResourceTemplates[i] = &rc
	}
	return &clone
}

// sanitizeGetPrompt rounds the float priority on prompt message content.
func sanitizeGetPrompt(res *mcp.GetPromptResult) *mcp.GetPromptResult {
	clone := *res
	clone.Messages = make([]*mcp.PromptMessage, len(res.Messages))
	for i, m := range res.Messages {
		mc := *m
		mc.Content = roundContentPriorities([]mcp.Content{m.Content})[0]
		clone.Messages[i] = &mc
	}
	return &clone
}
