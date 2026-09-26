# CI/CD Catalog — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: CI/CD Catalog
> **Individual tools**: 2
> **Meta-tool**: `gitlab_ci_catalog` (`GITLAB_MCP_TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `ci_catalog.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [CI/CD Catalog GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#querycicatalogresources)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The CI/CD Catalog domain provides discovery and inspection of reusable CI/CD components published to the GitLab CI/CD Catalog. The Catalog is a GraphQL-only feature with no REST API equivalent. Resources in the catalog are GitLab projects that publish reusable CI/CD components — pipeline templates, jobs, and steps that can be included in `.gitlab-ci.yml` files.

On the default dynamic surface, these operations are the `ci_catalog.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `GITLAB_MCP_TOOL_SURFACE=meta`, both individual tools below are consolidated into a single `gitlab_ci_catalog` meta-tool that dispatches by `action` parameter.

### Common Questions

> "Search the CI/CD catalog for Docker build components"
> "Show me the details of the auto-deploy catalog resource"
> "What components are available in the latest version?"
> "List all catalog resources sorted by star count"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description              |
| ---------- | :------: | :---------: | :--------: | ------------------------ |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation |

---

## Tools

### `gitlab_list_catalog_resources`

Search and list CI/CD Catalog resources. Supports text search, scope filtering, and multiple sort orders. Returns a paginated list with each resource's fields as the detail below lists them, up to `latest_released_at`, and the name of its latest version (`latest_version_name`). A listing reads nothing else of a version: its components, inputs and README are what the get action is for.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type   | Required | Description                                                                                                                                                                                                   |
| --------- | ------ | :------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `search`  | string |    No    | Search resources by name or description                                                                                                                                                                       |
| `scope`   | string |    No    | Filter scope: `ALL` (default) or `NAMESPACES`                                                                                                                                                                 |
| `sort`    | string |    No    | Sort order: `NAME_ASC` (default), `NAME_DESC`, `LATEST_RELEASED_AT_ASC`, `LATEST_RELEASED_AT_DESC`, `STAR_COUNT_ASC`, `STAR_COUNT_DESC`, `CREATED_ASC`, `CREATED_DESC`, `USAGE_COUNT_ASC`, `USAGE_COUNT_DESC` |
| `first`   | int    |    No    | Number of items per page (default: 20)                                                                                                                                                                        |
| `after`   | string |    No    | Cursor for forward pagination                                                                                                                                                                                 |
| `last`    | int    |    No    | Number of items per page when paging backward. Cannot be combined with `first`                                                                                                                                |
| `before`  | string |    No    | Cursor for backward pagination, from a previous response's `start_cursor`                                                                                                                                     |

### `gitlab_get_catalog_resource`

Get full details of a CI/CD Catalog resource by GID or project full path. Returns complete resource information including README content, all released versions, and component details with their input parameters and include paths.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter   | Type   | Required | Description                                                |
| ----------- | ------ | :------: | ---------------------------------------------------------- |
| `id`        | string |    No    | Resource GID (e.g. `gid://gitlab/Ci::Catalog::Resource/1`) |
| `full_path` | string |    No    | Project full path (e.g. `my-group/my-catalog-project`)     |

> **Note**: At least one of `id` or `full_path` must be provided.

### Output fields (detail)

| Field                     | Type   | Description                                                                 |
| ------------------------- | ------ | --------------------------------------------------------------------------- |
| `id`                      | string | Resource GID                                                                |
| `name`                    | string | Resource name                                                               |
| `description`             | string | Resource description                                                        |
| `icon`                    | string | Resource icon                                                               |
| `full_path`               | string | Project full path                                                           |
| `web_path`                | string | Path to the resource in GitLab (relative to the instance)                   |
| `star_count`              | int    | Number of stars                                                             |
| `starrers_path`           | string | Path to the page listing who starred the project (relative to the instance) |
| `last_30_day_usage_count` | int    | Unique projects that used a component in the last 30 days                   |
| `archived`                | bool   | Whether the hosting project is archived                                     |
| `topics`                  | array  | Project topics                                                              |
| `verification_level`      | string | Catalog verification level (e.g. `UNVERIFIED`, `GITLAB_MAINTAINED`)         |
| `visibility_level`        | string | Project visibility (`private`, `internal`, `public`)                        |
| `latest_released_at`      | string | Date of latest release                                                      |
| `latest_version_name`     | string | Name of the newest version                                                  |
| `readme`                  | string | README of the newest version, as its Markdown source                        |
| `readme_html`             | string | README of the newest version, as GitLab renders it                          |
| `versions`                | array  | Up to ten released versions (see below)                                     |
| `components`              | array  | Components in the newest version                                            |

GitLab resolves a version's README for one version per request and refuses it for any other, so the README is read once, for the newest version, and the older versions carry none.

### Version structure

| Field         | Type   | Description                                                               |
| ------------- | ------ | ------------------------------------------------------------------------- |
| `id`          | string | Version GID                                                               |
| `name`        | string | Version name                                                              |
| `released_at` | string | Release date                                                              |
| `created_at`  | string | Creation date                                                             |
| `semver`      | string | `major.minor.patch`, when GitLab sends all three                          |
| `path`        | string | Path to the release (relative to the instance)                            |
| `author`      | object | User who published it (`id`, `username`, `name`, `web_url`, `avatar_url`) |
| `commit`      | object | Commit it was released from (`sha`, `short_id`, `title`, `web_url`)       |
| `components`  | array  | Components of that version                                                |

### Component structure

Each component includes:

| Field                     | Type   | Description                                                          |
| ------------------------- | ------ | -------------------------------------------------------------------- |
| `id`                      | string | Component GID                                                        |
| `name`                    | string | Component name                                                       |
| `description`             | string | Component description                                                |
| `include_path`            | string | Path to include in `.gitlab-ci.yml`                                  |
| `last_30_day_usage_count` | int    | Unique projects that included this version of it in the last 30 days |
| `inputs`                  | array  | Input parameters (see below)                                         |

### Input structure

| Field         | Type   | Description                                                                                                |
| ------------- | ------ | ---------------------------------------------------------------------------------------------------------- |
| `name`        | string | Input name                                                                                                 |
| `type`        | string | `STRING`, `NUMBER`, `BOOLEAN` or `ARRAY`                                                                   |
| `required`    | bool   | Whether the input has no default                                                                           |
| `default`     | any    | Default value, as the JSON value of the input's type (a boolean input's default is `false`, not `"false"`) |
| `options`     | any    | Values the input may take, when it restricts them                                                          |
| `regex`       | string | Pattern the value must match, when it has one                                                              |
| `rules`       | array  | Conditional rules: `if` (the expression), and the `default` and `options` that apply when it holds         |
| `description` | string | Input description                                                                                          |

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
- The projects using a resource's components (`projectComponentUsages`) are not part of the detail view: GitLab marks the field as an experiment, answers it only to maintainers of the resource project on Premium and above, and resolves it for one resource per request
- GitLab refuses a whole GraphQL document that names a field it does not have, so the newest field a document selects is the oldest release it works on: the detail view needs GitLab 18.10 (a component's description) and the listing 18.1 (a resource's archived flag)

## Related

- [GitLab CI/CD Catalog](https://docs.gitlab.com/ee/ci/components/)
- [GitLab CI/CD Catalog GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#querycicatalogresources)
- [CI/CD Components](https://docs.gitlab.com/ee/ci/components/)
