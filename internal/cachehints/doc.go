// Package cachehints applies SEP-2549 cache hints (ttlMs/cacheScope) to MCP
// results. Almost every catalog in this server is token- and tier-dependent
// (PAT scope filtering, licensing-tier gating), so almost every cacheable
// result is marked "private": shared intermediaries must never serve one
// user's catalog to another. The prompt catalog is the exception and is
// "public"; see [scopePublic]. TTLs distinguish stable catalogs from live
// GitLab data.
package cachehints
