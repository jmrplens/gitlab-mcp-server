# Security & Monitoring — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Security & Monitoring
> **Individual tools**: 28
> **Meta-tools**: `gitlab_feature_flag`, `gitlab_ff_user_list`, `gitlab_secure_file`, `gitlab_error_tracking`, `gitlab_alert_management` (when `META_TOOLS=true`, default)
> **GitLab API**: [Feature Flags](https://docs.gitlab.com/ee/api/feature_flags.html) · [Feature Flag User Lists](https://docs.gitlab.com/ee/api/feature_flag_user_lists.html) · [Secure Files](https://docs.gitlab.com/ee/api/secure_files.html) · [Error Tracking](https://docs.gitlab.com/ee/api/error_tracking.html) · [Alert Management](https://docs.gitlab.com/ee/api/alert_management_alerts.html) · [Impersonation Tokens](https://docs.gitlab.com/ee/api/users.html#get-all-impersonation-tokens-of-a-user)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The security & monitoring domain covers project feature flags, feature flag user lists, CI/CD secure files, error tracking settings and client keys, alert management metric images, and user impersonation/personal access token management (admin only).

When `META_TOOLS=true` (the default), the individual tools below are consolidated into five meta-tools that dispatch by `action` parameter.

### Common Questions

> "Run a security scan on MR !15"
> "Check for vulnerabilities in my project"
> "Review the security of merge request !23"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Feature Flags

### `gitlab_feature_flag_list`

List feature flags for a project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_feature_flag_get`

Get a single feature flag by name.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_feature_flag_create`

Create a new feature flag for a project.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_feature_flag_update`

Update an existing feature flag.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_feature_flag_delete`

Delete a feature flag.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Feature Flag User Lists

### `gitlab_ff_user_list_list`

List feature flag user lists for a project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_ff_user_list_get`

Get a single feature flag user list by IID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_ff_user_list_create`

Create a new feature flag user list.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_ff_user_list_update`

Update a feature flag user list.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_ff_user_list_delete`

Delete a feature flag user list.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Secure Files

### `gitlab_list_secure_files`

List CI/CD secure files for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_show_secure_file`

Show details of a CI/CD secure file.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_secure_file`

Create a new CI/CD secure file.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_remove_secure_file`

Remove a CI/CD secure file.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Error Tracking

### `gitlab_get_error_tracking_settings`

Get error tracking settings for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_enable_disable_error_tracking`

Enable or disable error tracking for a GitLab project.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_list_error_tracking_client_keys`

List error tracking client keys for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_error_tracking_client_key`

Create a new error tracking client key for a GitLab project.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_error_tracking_client_key`

Delete an error tracking client key for a GitLab project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Alert Management

### `gitlab_list_alert_metric_images`

List metric images for a GitLab alert.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_upload_alert_metric_image`

Upload a metric image for a GitLab alert.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_alert_metric_image`

Update a metric image for a GitLab alert.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_alert_metric_image`

Delete a metric image from a GitLab alert.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Impersonation Tokens

### `gitlab_list_impersonation_tokens`

List all impersonation tokens for a GitLab user by user ID. Optionally filter by state.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `user_id` | int | Yes | GitLab user ID |
| `state` | string | No | Filter by state: `all`/`active`/`inactive` |
| `page` | int | No | Page number for pagination |
| `per_page` | int | No | Items per page (max 100) |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_impersonation_token`

Retrieve a specific impersonation token by user ID and token ID.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `user_id` | int | Yes | GitLab user ID |
| `token_id` | int | Yes | Impersonation token ID |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_impersonation_token`

Create an impersonation token for a GitLab user (admin only). Requires user ID, token name, and scopes.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `user_id` | int | Yes | GitLab user ID |
| `name` | string | Yes | Name of the impersonation token |
| `scopes` | []string | Yes | Array of scopes (api, read_user, read_api, read_repository, write_repository, etc.) |
| `expires_at` | string | No | Token expiration date (YYYY-MM-DD) |

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_revoke_impersonation_token`

Revoke an impersonation token for a GitLab user (admin only).

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `user_id` | int | Yes | GitLab user ID |
| `token_id` | int | Yes | Impersonation token ID to revoke |

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_create_personal_access_token`

Create a personal access token for a specific GitLab user (admin only). Requires user ID, token name, and scopes.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `user_id` | int | Yes | GitLab user ID |
| `name` | string | Yes | Name of the personal access token |
| `scopes` | []string | Yes | Array of scopes |
| `description` | string | No | Description for the token |
| `expires_at` | string | No | Token expiration date (YYYY-MM-DD) |

| Annotation | **Create** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_feature_flag_list` | Feature Flags | Read |
| 2 | `gitlab_feature_flag_get` | Feature Flags | Read |
| 3 | `gitlab_feature_flag_create` | Feature Flags | Create |
| 4 | `gitlab_feature_flag_update` | Feature Flags | Update |
| 5 | `gitlab_feature_flag_delete` | Feature Flags | Delete |
| 6 | `gitlab_ff_user_list_list` | FF User Lists | Read |
| 7 | `gitlab_ff_user_list_get` | FF User Lists | Read |
| 8 | `gitlab_ff_user_list_create` | FF User Lists | Create |
| 9 | `gitlab_ff_user_list_update` | FF User Lists | Update |
| 10 | `gitlab_ff_user_list_delete` | FF User Lists | Delete |
| 11 | `gitlab_list_secure_files` | Secure Files | Read |
| 12 | `gitlab_show_secure_file` | Secure Files | Read |
| 13 | `gitlab_create_secure_file` | Secure Files | Create |
| 14 | `gitlab_remove_secure_file` | Secure Files | Delete |
| 15 | `gitlab_get_error_tracking_settings` | Error Tracking | Read |
| 16 | `gitlab_enable_disable_error_tracking` | Error Tracking | Update |
| 17 | `gitlab_list_error_tracking_client_keys` | Error Tracking | Read |
| 18 | `gitlab_create_error_tracking_client_key` | Error Tracking | Create |
| 19 | `gitlab_delete_error_tracking_client_key` | Error Tracking | Delete |
| 20 | `gitlab_list_alert_metric_images` | Alert Management | Read |
| 21 | `gitlab_upload_alert_metric_image` | Alert Management | Create |
| 22 | `gitlab_update_alert_metric_image` | Alert Management | Update |
| 23 | `gitlab_delete_alert_metric_image` | Alert Management | Delete |
| 24 | `gitlab_list_impersonation_tokens` | Impersonation Tokens | Read |
| 25 | `gitlab_get_impersonation_token` | Impersonation Tokens | Read |
| 26 | `gitlab_create_impersonation_token` | Impersonation Tokens | Create |
| 27 | `gitlab_revoke_impersonation_token` | Impersonation Tokens | Delete |
| 28 | `gitlab_create_personal_access_token` | Impersonation Tokens | Create |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_feature_flag_delete` — deletes a feature flag
- `gitlab_ff_user_list_delete` — deletes a feature flag user list
- `gitlab_remove_secure_file` — removes a CI/CD secure file
- `gitlab_delete_error_tracking_client_key` — deletes an error tracking client key
- `gitlab_delete_alert_metric_image` — deletes a metric image from an alert
- `gitlab_revoke_impersonation_token` — revokes an impersonation token (admin only)

---

## Related

- [GitLab Feature Flags API](https://docs.gitlab.com/ee/api/feature_flags.html)
- [GitLab Feature Flag User Lists API](https://docs.gitlab.com/ee/api/feature_flag_user_lists.html)
- [GitLab Secure Files API](https://docs.gitlab.com/ee/api/secure_files.html)
- [GitLab Error Tracking API](https://docs.gitlab.com/ee/api/error_tracking.html)
- [GitLab Alert Management API](https://docs.gitlab.com/ee/api/alert_management_alerts.html)
- [GitLab Impersonation Tokens API](https://docs.gitlab.com/ee/api/users.html#get-all-impersonation-tokens-of-a-user)
