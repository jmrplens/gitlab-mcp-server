package clientcompat

import (
	"context"
	"math"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// Profile identifies the response compatibility profile negotiated for a
// client session.
type Profile int

const (
	// ProfileDefault leaves responses untouched.
	ProfileDefault Profile = iota
	// ProfileCodex rounds the float priority on content and resource
	// annotations to 0 or 1, because Codex's build decodes a fractional
	// number into a float field through serde_json's arbitrary_precision
	// and fails the result (see the package comment).
	// Everything else — audience, structuredContent, outputSchema, icons,
	// and the tool annotations Codex's approval policy depends on — is
	// preserved.
	ProfileCodex
)

// envDisable is the environment kill-switch: GITLAB_MCP_CLIENT_COMPAT=off
// disables the middleware entirely (both stdio and HTTP modes read the process
// env). It spells the suffix alone because [config.Getenv] is the one way a
// setting this project defines is read and supplies the prefix itself.
//
// The bare CLIENT_COMPAT spelling this setting also answered to from 2.8.0 was
// removed in 3.1.0, and nothing reads it now; [config.RetiredEnvUses] is what
// tells a deployment that still sets it, rather than quietly ignoring an "off"
// somebody meant.
const envDisable = "CLIENT_COMPAT"

// Enabled reports whether the compatibility middleware should be installed.
// Any value other than "off" (default empty included) enables it.
func Enabled() bool {
	return !strings.EqualFold(config.Getenv(envDisable), "off")
}

const (
	// codexClientName is the clientInfo name Codex has reported since v0.20.
	// It is matched as a case-insensitive prefix.
	codexClientName = "codex-mcp-client"
	// codexClientTitle is the clientInfo title Codex has reported since v0.20.
	// It is matched exactly.
	codexClientTitle = "Codex"
	// codexUserAgentPrefix opens the User-Agent Codex's MCP client sends on
	// every Streamable HTTP request, followed by its version
	// (codex-rs/rmcp-client/src/utils.rs). It is matched as a case-insensitive
	// prefix.
	codexUserAgentPrefix = "codex-mcp-client/"
)

// hasPrefixFold reports whether s begins with prefix, ignoring case.
func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// profileFromClientInfo maps the clientInfo a session reported, in initialize
// or, at protocol 2026-07-28, in the request's _meta, to a Profile. Codex has
// identified itself as name "codex-mcp-client" and title "Codex" since v0.20,
// and the match is held to those two spellings: a "codex" substring would also
// catch an openai-mcp client whose label carries the word, which reads a
// fractional priority without error and needs no profile (issue 1043).
func profileFromClientInfo(impl *mcp.Implementation) Profile {
	if impl == nil {
		return ProfileDefault
	}
	if hasPrefixFold(impl.Name, codexClientName) || impl.Title == codexClientTitle {
		return ProfileCodex
	}
	return ProfileDefault
}

// profileFromUserAgent maps the User-Agent of an HTTP request to a Profile:
// Codex's MCP client sends "codex-mcp-client/<version>". ChatGPT web's tool
// calls send "openai-mcp/1.0.0 (Codex)", which the prefix does not match.
func profileFromUserAgent(userAgent string) Profile {
	if hasPrefixFold(userAgent, codexUserAgentPrefix) {
		return ProfileCodex
	}
	return ProfileDefault
}

// sessionClientInfo returns the clientInfo of the session that issued req, or
// nil when the session knows no client. Over stateless HTTP at protocol
// 2025-11-25 or earlier each POST is a session of its own, whose initialize
// params the SDK synthesizes with a protocol version and no clientInfo.
func sessionClientInfo(req mcp.Request) *mcp.Implementation {
	ss, ok := req.GetSession().(*mcp.ServerSession)
	if !ok || ss == nil {
		return nil
	}
	params := ss.InitializeParams()
	if params == nil {
		return nil
	}
	return params.ClientInfo
}

// requestUserAgent returns the User-Agent of the HTTP request that carried
// req, or "" when it came over stdio or carried none.
func requestUserAgent(req mcp.Request) string {
	extra := req.GetExtra()
	if extra == nil {
		return ""
	}
	return extra.Header.Get("User-Agent")
}

// profileForRequest resolves the Profile for req. The clientInfo of its
// session decides whenever the session has one; only a session that knows no
// client falls back to the request's User-Agent (issue 1043), which is how a
// Codex client on protocol 2025-11-25 or earlier is recognized over the
// default stateless HTTP transport. Both are self-reported, and neither ever
// decides who a caller is or what it may do (register row IDN-013).
func profileForRequest(req mcp.Request) Profile {
	if req == nil {
		return ProfileDefault
	}
	if impl := sessionClientInfo(req); impl != nil {
		return profileFromClientInfo(impl)
	}
	return profileFromUserAgent(requestUserAgent(req))
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
