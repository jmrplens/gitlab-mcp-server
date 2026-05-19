// Package securityattributes implements MCP tools for GitLab security attributes.
//
// The package exposes catalog-backed actions for creating, updating, deleting,
// assigning, and bulk-applying attributes that GitLab uses to classify projects
// and groups for security workflows. Handlers validate numeric identifiers,
// security category references, hex color values, and bulk update modes before
// issuing GraphQL mutations through the GitLab client.
//
// Security attributes are available only on GitLab tiers and deployments that
// expose the underlying GraphQL security attribute schema. The package keeps the
// GraphQL payloads local so the MCP action catalog, dynamic search surface, and
// Markdown formatters all share the same typed input and output structures.
//
// GitLab API docs:
//   - https://docs.gitlab.com/api/graphql/reference/#securityattribute
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributecreate
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeupdate
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributedestroy
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecurityattributeprojectupdate
//   - https://docs.gitlab.com/api/graphql/reference/#mutationbulkupdatesecurityattributes
package securityattributes
