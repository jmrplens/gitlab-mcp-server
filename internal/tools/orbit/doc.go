// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package orbit implements MCP tools for the GitLab Orbit Knowledge Graph on
// GitLab.com.
//
// Six read-only handlers are exposed as individual tools (gitlab_orbit_*) and
// a consolidated meta-tool (gitlab_orbit). GitLab.com removed five of the six
// REST routes they read on 2026-10-27 and -28 (gitlab-org/gitlab!265633,
// !264745, !265998 and !266009), so each now reads the source GitLab offers in
// their place, a documented API where there is one:
//
//   - Status:      GraphQL Query.orbit.serviceStatus           (service health)
//   - Schema:      GraphQL Query.orbit.schema                  (graph schema),
//     and the get_graph_schema command of POST /api/v4/orbit/mcp for its llm text
//   - Tools:       tools/list of POST /api/v4/orbit/mcp        (MCP tool list)
//   - DSL:         list_commands of POST /api/v4/orbit/mcp     (query language, GQL)
//   - Query:       POST /api/v4/orbit/query                    (read-only graph query)
//   - GraphStatus: GraphQL Group.orbit and Project.orbit      (indexing status)
//
// The Orbit MCP endpoint is used only for what GitLab offers nowhere else: the
// tool list, the command catalog that describes the query language, and the
// schema as compact text. client-go's Orbit service still sends the removed
// routes for all but the query, which is why only Query calls it; each of its
// methods that does is a row of docs/development/upstream-bugs.md.
//
// Since Orbit's general availability on 2026-10-29, GitLab.com serves a
// namespace's index and its queries only for a top-level group with the
// orbit_promo add-on or a running Orbit trial. GitLab answers what it does not
// serve the token with 403 or with null, and the handlers report either as an
// error naming what Orbit needs ([ErrOrbitUnavailable] for null).
//
// Reference: https://docs.gitlab.com/api/orbit/
//
// The Query handler forwards a query as the caller wrote it, checking only
// that one was given. GQL, the read-only openCypher 9 based language, is what
// GitLab.com compiles every query in since it turned orbit_gql_queries on for
// everyone on 2026-10-15, and [DSL] describes it from the query_graph command
// GitLab.com publishes; the route that served the JSON query DSL before, GET
// /api/v4/orbit/schema/dsl, answers 404 since then, and nothing here asks it
// (issue 1414). gitlab-org/gitlab!261947 removed the flag and the JSON DSL
// with it on 2026-10-27, and GitLab's query route takes the query as text, so
// the handler takes text alone; a JSON query written as text is answered with
// a hint that GitLab.com reads GQL. The language keeps changing: GitLab
// compiles each query and refuses what it cannot run with a message naming
// the fault, which reaches the caller rather than being replaced by rules
// written here, which went stale once already (issue 1031).
package orbit
