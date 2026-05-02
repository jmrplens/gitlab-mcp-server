// Package integrations implements MCP tools for GitLab project integrations.
//
// It wraps the GitLab Services service from client-go v2. The generic tools
// list, get, and delete integrations by slug, while integration-specific tools
// handle configuration details such as Jira settings. The package also provides
// Markdown rendering for project integration responses.
package integrations
