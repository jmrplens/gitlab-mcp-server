# Orbit — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Orbit Knowledge Graph
> **Individual tools**: 5
> **Meta-tool**: `gitlab_orbit` (`TOOL_SURFACE=meta` catalog)
> **GitLab API**: [Orbit API](https://docs.gitlab.com/api/orbit/)
> **Availability**: GitLab.com only; Enterprise/Premium catalog; experimental `knowledge_graph` feature
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The Orbit domain exposes GitLab's experimental Knowledge Graph API for GitLab.com. It is registered only when the MCP server is connected to `https://gitlab.com` and the Enterprise/Premium catalog is enabled; self-managed GitLab instances and non-enterprise catalogs do not advertise these tools. GitLab may still return `404 Not Found` when the `knowledge_graph` feature flag is disabled, `403 Forbidden` when the token cannot access a Knowledge Graph-enabled namespace or project, or `503 Service Unavailable` when the Orbit backend is unavailable.

The upstream Orbit API is moving quickly. This MCP surface follows the latest GitLab client and CLI coverage, including `graph_status`; GitLab's public API reference may lag behind that endpoint. For schema formatting, the live API currently uses the `format` query parameter, while this server also accepts `response_format` as an input alias for compatibility with public documentation wording.

With `TOOL_SURFACE=meta`, all five individual tools below are consolidated into the `gitlab_orbit` meta-tool with an `action` parameter.

| Meta-tool Action | Individual Tool | Purpose |
| --- | --- | --- |
| `status` | `gitlab_orbit_status` | Check Orbit service health and backend components |
| `schema` | `gitlab_orbit_schema` | Inspect the graph ontology and optionally expand node definitions |
| `tools` | `gitlab_orbit_tools` | Discover the live Orbit query manifest |
| `query` | `gitlab_orbit_query` | Run a read-only Knowledge Graph query object |
| `graph_status` | `gitlab_orbit_graph_status` | Inspect indexing status for one namespace, project, or full path |

### Common Questions

> "Is Orbit available for this GitLab.com token?"
> "Show the Knowledge Graph schema"
> "Check indexing status for gitlab-org/gitlab"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |

All Orbit tools are read-only.

---

## Status

### `gitlab_orbit_status`

Get Orbit cluster health and component status. Optional `response_format` accepts `raw` or `llm`; omitting it defaults to `raw`.

| Annotation | **Read** |
| ---------- | -------- |

---

## Schema

### `gitlab_orbit_schema`

Get the Orbit graph ontology, including schema version, domains, node summaries, and edges. Optional `expand` requests expanded node definitions for named node types, and optional `format` accepts `raw` or `llm`. The input also accepts `response_format` as an alias; if both are set, they must match.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Manifest

### `gitlab_orbit_tools`

Get the Orbit MCP tool manifest served by GitLab.com. Use this before `gitlab_orbit_query` to discover the live query shapes and parameter schemas supported by the Orbit backend.

| Annotation | **Read** |
| ---------- | -------- |

---

## Query

### `gitlab_orbit_query`

Execute a read-only Orbit Knowledge Graph query. The `query` parameter must be a JSON object matching the schema returned by `gitlab_orbit_tools`. Optional `response_format` accepts `raw` or `llm`.

| Annotation | **Read** |
| ---------- | -------- |

---

## Graph Status

### `gitlab_orbit_graph_status`

Get graph indexing status for exactly one scope: `namespace_id`, `project_id`, or `full_path`. Optional `response_format` accepts `raw` or `llm`.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_orbit_status` | Status | Read |
| 2 | `gitlab_orbit_schema` | Schema | Read |
| 3 | `gitlab_orbit_tools` | Tool Manifest | Read |
| 4 | `gitlab_orbit_query` | Query | Read |
| 5 | `gitlab_orbit_graph_status` | Graph Status | Read |

### Destructive Tools (Require Confirmation)

None — all Orbit tools are read-only.

---

## Related

- [GitLab Orbit API](https://docs.gitlab.com/api/orbit/)
- [GraphQL Integration](../graphql.md)
- [Search Tools](search.md)
