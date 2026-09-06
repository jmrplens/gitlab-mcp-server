// Package serverpool manages a pool of credential entries keyed by GitLab token and URL.
//
// Each unique GitLab Personal Access Token and GitLab URL pair gets its own
// [Entry]: a GitLab client, the configuration resolved for it (detected token
// scopes, detected CE/EE edition, any read-only narrowing), the user it belongs
// to, and an opaque [Entry.Owner] that names it wherever a shared component has
// to say whose work it is doing. The MCP server an entry is served by is *not*
// its own: one server is built per configuration shape and answers for every
// credential that resolves to that shape, since what a server holds — the tool
// catalog, the resources, the prompts — is decided by the configuration and
// never by the credential. Isolation is therefore not "a server each" but
// "a client each": every request runs under the client its own entry carries,
// and anything that was ever keyed on the server is keyed on the entry instead.
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
// HTTP requests are routed to per-identity entries:
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
//	per-token, per-URL entry -> the server built for its configuration shape
//
// This design keeps token scopes, edition detection, read-only mode and safe
// mode resolved per credential, while the tools, resources and prompts they
// select are built once and shared by every credential that selects the same
// ones.
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
