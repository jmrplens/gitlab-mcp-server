// Package projects implements MCP tools for GitLab project operations.
//
// The package covers project create, get, list, update, delete, restore, fork,
// star, archive, transfer, sharing, invited groups, forks, languages, members,
// starrers, hooks, push rules, pull mirroring, approval configuration, approval
// rules, and user contributed or starred project listings.
//
// # Catalog Surface
//
// Project actions are one of the broadest catalog domains. Their ActionSpecs
// feed individual tools, the gitlab_project meta-tool, and unified dynamic
// action IDs such as project.get, project.hook_add, and project.approval_rule_list.
//
// # GitLab API References
//
// The package wraps these GitLab APIs:
//
//   - https://docs.gitlab.com/api/projects/
//   - https://docs.gitlab.com/api/project_webhooks/
//   - https://docs.gitlab.com/api/project_badges/
//   - https://docs.gitlab.com/api/project_import_export/
package projects
