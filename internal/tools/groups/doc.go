// Package groups implements MCP tools for GitLab group operations.
//
// The package wraps client-go group APIs for listing groups, retrieving group
// details, listing members and subgroups, and managing group webhooks through
// list, get, add, edit, and delete operations. It also provides Markdown
// formatting for group tool outputs.
//
// Group-adjacent domains such as labels, milestones, boards, variables,
// service accounts, SAML, LDAP, protected environments, protected branches,
// epics, group wikis, and group releases live in dedicated sibling packages and
// share the same catalog-first registration path.
//
// # GitLab API References
//
// The package wraps the GitLab Groups API:
//
//   - https://docs.gitlab.com/api/groups/
package groups
