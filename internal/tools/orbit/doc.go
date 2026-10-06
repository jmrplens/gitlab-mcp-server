// Package orbit implements MCP tools for the experimental GitLab Orbit
// Knowledge Graph API on GitLab.com.
//
// Six read-only handlers wrap the public Orbit endpoints and are exposed as
// individual tools (gitlab_orbit_*) and a consolidated meta-tool
// (gitlab_orbit):
//
//   - Status       — GET  /api/v4/orbit/status          (cluster health)
//   - Schema       — GET  /api/v4/orbit/schema          (graph ontology)
//   - Tools        — GET  /api/v4/orbit/tools           (MCP tool manifest)
//   - DSL          — GET  /api/v4/orbit/schema/dsl      (query DSL grammar)
//   - Query        — POST /api/v4/orbit/query           (read-only graph query)
//   - GraphStatus  — GET  /api/v4/orbit/graph_status    (indexing status)
//
// Orbit is gated to GitLab.com Premium/Ultimate with the knowledge_graph
// feature flag enabled. The package returns informative not-found results
// when the experimental feature is unavailable or the token cannot access
// a Knowledge Graph-enabled namespace or project.
//
// Reference: https://docs.gitlab.com/api/orbit/
//
// The Query handler forwards a query in the Orbit query DSL (traversal,
// aggregation, neighbors, path_finding) as the caller wrote it, checking
// only that one was given and that it encodes as JSON. The DSL is the JSON
// Schema [DSL] serves, version 12 at the time of writing, and it changes
// during the Orbit beta: GitLab compiles each query against it and refuses
// what it cannot run with a message naming the fault, which reaches the
// caller rather than being replaced by rules written here, which went stale
// once already (issue 1031).
package orbit
