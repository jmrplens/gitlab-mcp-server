// Package containerregistry implements MCP tools for GitLab container registry
// operations.
//
// It wraps the GitLab ContainerRegistry service to list project and group
// repositories, list and delete tags, inspect tag details, and manage container
// registry protection rules. The package also registers MCP tools and provides
// Markdown rendering for container registry responses.
//
// # Safety Model
//
// Tag and protection-rule deletions are destructive catalog actions. Their
// ActionSpecs mark that behavior so read-only mode, safe mode previews, and
// dynamic confirmation handling all see the same metadata.
//
// # GitLab API References
//
// The package wraps the GitLab Container Registry API:
//
//   - https://docs.gitlab.com/api/container_registry/
package containerregistry
