// Package users implements GitLab user MCP tools for the current authenticated
// user, administrator user management, user CRUD, SSH keys, service accounts,
// personal access tokens, memberships, activities, runner details, and identity
// deletion.
//
// The package registers both read-only profile tools and administrative actions
// such as block, unblock, ban, unban, activate, deactivate, approve, reject,
// disable two-factor authentication, and user-scoped SSH key management. It also
// provides Markdown formatters for user tool outputs.
//
// # Permissions
//
// Read-only current-user actions work with ordinary authenticated tokens, while
// administrative actions require GitLab administrator permissions. Handlers keep
// these paths separate so catalog metadata can preserve read-only and mutating
// behavior for each action.
//
// # GitLab API References
//
// The package wraps the GitLab Users API:
//
//   - https://docs.gitlab.com/api/users/
package users
