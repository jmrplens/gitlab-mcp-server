# Security Attributes — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Security Attributes
> **Individual tools**: 5
> **Meta-tool**: `gitlab_security_attribute` (`TOOL_SURFACE=meta` catalog)
> **GitLab API**: [SecurityAttribute GraphQL object](https://docs.gitlab.com/api/graphql/reference/#securityattribute) · [Create](https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributecreate) · [Update](https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeupdate) · [Delete](https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributedestroy) · [Project update](https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeprojectupdate) · [Bulk update](https://docs.gitlab.com/api/graphql/reference/#mutationbulkupdatesecurityattributes)
> **Audience**: End users, AI assistant users
> **Requires**: GitLab Ultimate or Premium

---

## Overview

Security attributes are namespace-level labels that classify GitLab groups and projects. Attributes belong to a security category, can be applied directly to a project, and can be added, removed, or replaced across multiple groups and projects in bulk.

This domain is distinct from security findings and vulnerabilities: security attributes are classification metadata, while findings and vulnerabilities represent scanner output and triage state.

### Common Questions

> "Create a security attribute called High under category 7"
> "Apply security attribute 9 to project 42"
> "Replace the security attributes on these projects"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                                                   |
| ---------- | :------: | :---------: | :--------: | ----------------------------------------------------------------------------- |
| **Create** |    —     |     No      |     No     | Creates one or more new attributes                                            |
| **Update** |    —     |     No      |    Yes     | Modifies metadata or project assignments                                      |
| **Delete** |    —     |     Yes     |    Yes     | Destroys an attribute; protected by confirmation                              |
| **Bulk**   |    —     |     Yes     |    Yes     | Applies, removes, or replaces assignments at scale; protected by confirmation |

Tools marked **Delete** or **Bulk** require confirmation before execution.

---

## Tools

### `gitlab_create_security_attribute`

Create one or more security attributes under an existing security category.

| Annotation | **Create** |
| ---------- | ---------- |

| Parameter      | Type  | Required | Description                                                            |
| -------------- | ----- | :------: | ---------------------------------------------------------------------- |
| `namespace_id` | int   |   Yes    | Numeric namespace ID                                                   |
| `category_id`  | int   |   Yes    | Numeric security category ID                                           |
| `attributes`   | array |   Yes    | Attributes to create; each item has `name`, `description`, and `color` |

### `gitlab_update_security_attribute`

Update a security attribute name, description, or color.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter      | Type   | Required | Description                                |
| -------------- | ------ | :------: | ------------------------------------------ |
| `attribute_id` | int    |   Yes    | Numeric security attribute ID              |
| `name`         | string |    No    | New attribute name                         |
| `description`  | string |    No    | New attribute description                  |
| `color`        | string |    No    | New color as a hex code, such as `#FF0000` |

At least one of `name`, `description`, or `color` must be provided.

### `gitlab_delete_security_attribute`

Delete a custom security attribute.

| Annotation | **Delete** |
| ---------- | ---------- |

| Parameter      | Type | Required | Description                   |
| -------------- | ---- | :------: | ----------------------------- |
| `attribute_id` | int  |   Yes    | Numeric security attribute ID |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_update_project_security_attributes`

Add or remove security attributes on a project.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter              | Type  | Required | Description                      |
| ---------------------- | ----- | :------: | -------------------------------- |
| `project_id`           | int   |   Yes    | Numeric project ID               |
| `add_attribute_ids`    | int[] |    No    | Security attribute IDs to add    |
| `remove_attribute_ids` | int[] |    No    | Security attribute IDs to remove |

At least one of `add_attribute_ids` or `remove_attribute_ids` must be provided.

### `gitlab_bulk_update_security_attributes`

Add, remove, or replace security attributes across multiple groups and projects.

| Annotation | **Bulk** |
| ---------- | -------- |

| Parameter       | Type   | Required | Description                              |
| --------------- | ------ | :------: | ---------------------------------------- |
| `group_ids`     | int[]  |    No    | Numeric group IDs to update              |
| `project_ids`   | int[]  |    No    | Numeric project IDs to update            |
| `attribute_ids` | int[]  |   Yes    | Security attribute IDs to apply          |
| `mode`          | string |   Yes    | Bulk mode: `ADD`, `REMOVE`, or `REPLACE` |

At least one of `group_ids` or `project_ids` must be provided.

> **Destructive**: Protected by confirmation prompt because `REMOVE` and `REPLACE` can remove existing assignments.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_create_security_attribute` | Mutation | Create |
| 2 | `gitlab_update_security_attribute` | Mutation | Update |
| 3 | `gitlab_delete_security_attribute` | Mutation | Delete |
| 4 | `gitlab_update_project_security_attributes` | Mutation | Update |
| 5 | `gitlab_bulk_update_security_attributes` | Mutation | Bulk |

---

## Related

- [GitLab SecurityAttribute GraphQL object](https://docs.gitlab.com/api/graphql/reference/#securityattribute)
- [Security Categories](security-categories.md) — category management for security attributes
- [Security Findings](security-findings.md) — pipeline scan findings
- [Vulnerabilities](vulnerabilities.md) — tracked vulnerability lifecycle management
