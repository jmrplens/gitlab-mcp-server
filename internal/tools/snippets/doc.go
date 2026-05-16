// Package snippets implements MCP tools for GitLab personal and project
// snippets.
//
// It covers personal snippet CRUD, project-scoped snippet CRUD, raw content
// retrieval, multi-file snippet content, threaded discussions, notes, award
// emoji, and Markdown rendering for snippet responses. The package wraps the
// GitLab SnippetsService and ProjectSnippetsService APIs.
//
// # Scope Handling
//
// Personal snippets and project snippets use different GitLab API services but
// share output and formatter types. Handlers therefore keep project_id optional
// where GitLab supports personal snippets and require it only for project-scoped
// operations.
//
// # GitLab API References
//
// The package wraps the GitLab Snippets API:
//
//   - https://docs.gitlab.com/api/snippets/
package snippets
