# Security Categories — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Security Categories
> **Individual tools**: 3
> **Meta-tool**: `gitlab_security_category` (`TOOL_SURFACE=meta` catalog)
> **GitLab API**: [SecurityCategory GraphQL object](https://docs.gitlab.com/api/graphql/reference/#securitycategory) · [Create](https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorycreate) · [Update](https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategoryupdate) · [Delete](https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorydestroy)
> **Audience**: End users, AI assistant users
> **Requires**: GitLab Ultimate or Premium

---

## Overview

Security categories group namespace-level security attributes. A category controls whether multiple attributes can be selected for the same target, and deleting a category also deletes the security attributes associated with it.

Use security categories before creating security attributes. Use security attributes to classify projects and groups once the category exists.

### Common Questions

> "Create a security category named Business impact"
> "Rename security category 7"
> "Delete a custom security category and its attributes"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                                       |
| ---------- | :------: | :---------: | :--------: | ----------------------------------------------------------------- |
| **Create** |    —     |     No      |     No     | Creates a new category                                            |
| **Update** |    —     |     No      |    Yes     | Modifies an existing category                                     |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a category and its attributes; protected by confirmation |

Tools marked **Delete** require confirmation before execution.

---

## Tools

### `gitlab_create_security_category`

Create a security category in a namespace.

| Annotation | **Create** |
| ---------- | ---------- |

| Parameter            | Type   | Required | Description                                                  |
| -------------------- | ------ | :------: | ------------------------------------------------------------ |
| `namespace_id`       | int    |   Yes    | Numeric namespace ID                                         |
| `name`               | string |   Yes    | Category name                                                |
| `description`        | string |    No    | Category description                                         |
| `multiple_selection` | bool   |    No    | Whether multiple attributes can be selected for the category |

### `gitlab_update_security_category`

Update a security category name or description.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter      | Type   | Required | Description                  |
| -------------- | ------ | :------: | ---------------------------- |
| `category_id`  | int    |   Yes    | Numeric security category ID |
| `namespace_id` | int    |   Yes    | Numeric namespace ID         |
| `name`         | string |    No    | New category name            |
| `description`  | string |    No    | New category description     |

At least one of `name` or `description` must be provided.

### `gitlab_delete_security_category`

Delete a custom security category and its associated security attributes.

| Annotation | **Delete** |
| ---------- | ---------- |

| Parameter     | Type | Required | Description                  |
| ------------- | ---- | :------: | ---------------------------- |
| `category_id` | int  |   Yes    | Numeric security category ID |

> **Destructive**: Protected by confirmation prompt because associated security attributes are also deleted.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_create_security_category` | Mutation | Create |
| 2 | `gitlab_update_security_category` | Mutation | Update |
| 3 | `gitlab_delete_security_category` | Mutation | Delete |

---

## Related

- [GitLab SecurityCategory GraphQL object](https://docs.gitlab.com/api/graphql/reference/#securitycategory)
- [Security Attributes](security-attributes.md) — attribute management and assignment
- [Groups](groups.md) — namespace and group management
- [Projects](projects.md) — project management and metadata
