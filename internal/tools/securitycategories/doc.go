// Package securitycategories implements MCP tools for GitLab security categories.
//
// The package exposes catalog-backed actions for creating, updating, and
// deleting the category records that group GitLab security attributes. Handlers
// validate namespace and category identifiers, optional update fields, and
// GraphQL mutation responses before returning MCP-friendly output with nested
// attribute summaries.
//
// Security categories are GraphQL-backed GitLab security taxonomy objects. This
// package keeps those mutations and conversions together so meta-tools, dynamic
// discovery, individual tool projection, and Markdown rendering all describe the
// same behavior.
//
// GitLab API docs:
//   - https://docs.gitlab.com/api/graphql/reference/#securitycategory
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorycreate
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategoryupdate
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecuritycategorydestroy
package securitycategories
