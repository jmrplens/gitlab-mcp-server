// Package workitemsavedviews implements MCP tools for GitLab work item saved
// views through the GraphQL API.
//
// A saved view stores a named, reusable work item filter under a namespace: the
// filter itself, the sort order, and the display settings the consuming UI
// renders it with. The package lists, reads, creates, updates, deletes,
// subscribes to, and unsubscribes from saved views, and renders the results as
// Markdown.
//
// The package wraps the GitLab GraphQL saved view query and mutations:
//
//   - https://docs.gitlab.com/api/graphql/reference/#namespacesavedviews
//   - https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewcreate
//   - https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewupdate
//   - https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewdelete
//   - https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewsubscribe
//   - https://docs.gitlab.com/api/graphql/reference/#mutationworkitemsavedviewunsubscribe
//
// Experimental: upstream marks the Work Item Saved Views API as a work in
// progress that may introduce breaking changes even between minor versions.
package workitemsavedviews
