// Package dependencyproxy implements MCP tools for GitLab Dependency Proxy cache
// management.
//
// The package currently exposes the group cache purge operation and registers it
// as both an individual tool and a meta-tool action.
//
// The package wraps the GitLab Dependency Proxy API:
//
//   - https://docs.gitlab.com/api/dependency_proxy/
package dependencyproxy
