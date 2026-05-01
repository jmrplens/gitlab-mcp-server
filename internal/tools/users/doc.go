// Package users implements GitLab user MCP tools for the current authenticated
// user, administrator user management, user CRUD, SSH keys, service accounts,
// personal access tokens, memberships, activities, runner details, and identity
// deletion.
//
// The package registers both read-only profile tools and administrative actions
// such as block, unblock, ban, unban, activate, deactivate, approve, reject,
// disable two-factor authentication, and user-scoped SSH key management. It also
// provides Markdown formatters for user tool outputs.
package users
