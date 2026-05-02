// Package serverpool manages a pool of MCP servers keyed by GitLab token and URL.
//
// Each unique GitLab Personal Access Token and GitLab URL pair gets its own
// [*mcp.Server], GitLab client, and server-scoped configuration snapshot with
// independently registered tools, resources, prompts, detected token scopes,
// and detected CE/EE edition. This provides zero cross-contamination between
// HTTP clients by construction: each client operates on its own server entry.
//
// The pool has a configurable maximum size ([WithMaxSize]) and uses LRU
// eviction when the limit is reached. Token plus URL hashes (SHA-256) are used
// as pool keys so that raw tokens are never stored in memory.
//
// The package also extracts GitLab tokens and per-request GitLab URLs from HTTP
// headers and includes an authentication-failure rate limiter for the HTTP MCP
// endpoint.
//
// # Isolation Model
//
// HTTP requests are routed to per-identity server entries:
//
//	HTTP request
//	    |
//	    v
//	ExtractToken and ExtractGitLabURL
//	    |
//	    v
//	ServerPool.GetOrCreate
//	    |
//	    v
//	per-token, per-URL MCP server
//
// This design keeps token scopes, edition detection, read-only mode, safe mode,
// tools, resources, and prompts isolated between concurrent HTTP clients.
//
// # Usage
//
// Create a pool with [New], retrieve or create servers with
// [ServerPool.GetOrCreate], and extract tokens from HTTP requests with
// [ExtractToken]:
//
//	pool := serverpool.New(cfg, factory, serverpool.WithMaxSize(100))
//	defer pool.Close()
//
//	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
//	    token := serverpool.ExtractToken(r)
//	    gitlabURL, err := serverpool.ExtractGitLabURL(r, cfg.GitLabURL)
//	    if err != nil {
//	        return nil
//	    }
//	    srv, err := pool.GetOrCreate(token, gitlabURL)
//	    if err != nil {
//	        return nil
//	    }
//	    return srv
//	}, opts)
package serverpool
