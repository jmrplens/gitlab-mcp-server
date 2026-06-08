// Package integrations implements MCP tools for GitLab project and group
// integrations.
//
// It wraps the GitLab Services service from client-go v2. The generic tools
// list, get, and delete project-level integrations by slug, while
// integration-specific tools handle configuration details such as Jira and
// group-level Datadog settings. The package also provides Markdown rendering
// for project and group integration responses.
//
// The package wraps two GitLab API surfaces:
//
//   - Project integrations: https://docs.gitlab.com/api/integrations/
//   - Group integrations:   https://docs.gitlab.com/api/group_integrations/
package integrations
