// Package orbit implements MCP tools for the experimental GitLab Orbit
// Knowledge Graph API on GitLab.com.
//
// Six read-only handlers wrap the public Orbit endpoints and are exposed as
// individual tools (gitlab_orbit_*) and a consolidated meta-tool
// (gitlab_orbit):
//
//   - Status:      GET  /api/v4/orbit/status          (cluster health)
//   - Schema:      GET  /api/v4/orbit/schema          (graph ontology)
//   - Tools:       GET  /api/v4/orbit/tools           (MCP tool manifest)
//   - DSL:         GET  /api/v4/orbit/agent/commands  (query language, GQL)
//   - Query:       POST /api/v4/orbit/query           (read-only graph query)
//   - GraphStatus: GET  /api/v4/orbit/graph_status    (indexing status)
//
// Orbit is gated to GitLab.com Premium/Ultimate with the knowledge_graph
// feature flag enabled. The package returns informative not-found results
// when the experimental feature is unavailable or the token cannot access
// a Knowledge Graph-enabled namespace or project.
//
// Reference: https://docs.gitlab.com/api/orbit/
//
// The Query handler forwards a query as the caller wrote it, checking only
// that one was given as GQL text or a JSON object. GQL, the read-only
// openCypher 9 based language, is what GitLab.com compiles every query in
// since it turned orbit_gql_queries on for everyone on 2026-10-15, and [DSL]
// describes it from the query_graph command GitLab.com publishes; the route
// that served the JSON query DSL before, GET /api/v4/orbit/schema/dsl, answers
// 404 since then, and nothing here asks it (issue 1414). An object is the
// JSON DSL, still forwarded rather than refused so it works if GitLab.com
// turns GQL back off, and answered with a hint that GitLab.com reads GQL when
// GitLab refuses it for its language. The language changes during the Orbit
// beta: GitLab compiles each query and refuses what it cannot run with a
// message naming the fault, which reaches the caller rather than being
// replaced by rules written here, which went stale once already (issue 1031).
package orbit
