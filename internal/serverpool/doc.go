// Package serverpool manages a pool of MCP servers keyed by GitLab token hash.
//
// Each unique GitLab Personal Access Token gets its own [*mcp.Server] with
// independently registered tools, resources, and prompts. This provides zero
// cross-contamination between HTTP clients by construction — each client
// operates on its own server instance.
//
// The pool has a configurable maximum size ([WithMaxSize]) and uses LRU
// eviction when the limit is reached. Token hashes (SHA-256) are used as
// pool keys so that raw tokens are never stored in memory.
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
//	    srv, err := pool.GetOrCreate(token)
//	    if err != nil {
//	        return nil
//	    }
//	    return srv
//	}, opts)
package serverpool
