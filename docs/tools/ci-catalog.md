# CI/CD Catalog — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: CI/CD Catalog
> **Individual tools**: 2
> **Meta-tool**: `gitlab_ci_catalog` (when `META_TOOLS=true`, default)
> **GitLab API**: [CI/CD Catalog GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#querycicatalogresources)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The CI/CD Catalog domain provides discovery and inspection of reusable CI/CD components published to the GitLab CI/CD Catalog. The Catalog is a GraphQL-only feature with no REST API equivalent. Resources in the catalog are GitLab projects that publish reusable CI/CD components — pipeline templates, jobs, and steps that can be included in `.gitlab-ci.yml` files.

When `META_TOOLS=true` (the default), both individual tools below are consolidated into a single `gitlab_ci_catalog` meta-tool that dispatches by `action` parameter.

### Common Questions

> "Search the CI/CD catalog for Docker build components"
> "Show me the details of the auto-deploy catalog resource"
> "What components are available in the latest version?"
> "List all catalog resources sorted by star count"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |

---

## Tools

### `gitlab_list_catalog_resources`

Search and list CI/CD Catalog resources. Supports text search, scope filtering, and multiple sort orders. Returns a paginated list with resource name, description, star/fork counts, and latest version.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `search` | string | No | Search resources by name or description |
| `scope` | string | No | Filter scope: `ALL` (default) or `NAMESPACED` |
| `sort` | string | No | Sort order: `NAME_ASC` (default), `NAME_DESC`, `LATEST_RELEASED_AT_ASC`, `LATEST_RELEASED_AT_DESC`, `STAR_COUNT_ASC`, `STAR_COUNT_DESC` |
| `first` | int | No | Number of items per page (default: 20) |
| `after` | string | No | Cursor for forward pagination |

### `gitlab_get_catalog_resource`

Get full details of a CI/CD Catalog resource by GID or project full path. Returns complete resource information including README content, all released versions, and component details with their input parameters and include paths.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `id` | string | No | Resource GID (e.g. `gid://gitlab/Ci::Catalog::Resource/1`) |
| `full_path` | string | No | Project full path (e.g. `my-group/my-catalog-project`) |

> **Note**: At least one of `id` or `full_path` must be provided.

### Output fields (detail)

| Field | Type | Description |
| ----- | ---- | ----------- |
| `id` | string | Resource GID |
| `name` | string | Resource name |
| `description` | string | Resource description |
| `icon` | string | Resource icon |
| `full_path` | string | Project full path |
| `web_url` | string | URL to the resource in GitLab |
| `star_count` | int | Number of stars |
| `forks_count` | int | Number of forks |
| `open_issues_count` | int | Open issue count |
| `open_merge_requests_count` | int | Open MR count |
| `latest_released_at` | string | Date of latest release |
| `readme_html` | string | Rendered README content |
| `versions` | array | Released versions with components |
| `components` | array | Components in the latest version |

### Component structure

Each component includes:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | Component name |
| `description` | string | Component description |
| `include_path` | string | Path to include in `.gitlab-ci.yml` |
| `inputs` | array | Input parameters (name, type, required, default, description) |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_catalog_resources` | Query | Read |
| 2 | `gitlab_get_catalog_resource` | Query | Read |

---

## Notes

- The CI/CD Catalog is a GraphQL-only feature — there is no REST API for catalog resources
- Resource versions correspond to GitLab releases on the underlying project
- Component `include_path` values can be used directly in `.gitlab-ci.yml` `include:` directives
- Up to 10 most recent versions are returned in the detail view

## Related

- [GitLab CI/CD Catalog](https://docs.gitlab.com/ee/ci/components/)
- [GitLab CI/CD Catalog GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#querycicatalogresources)
- [CI/CD Components](https://docs.gitlab.com/ee/ci/components/)
