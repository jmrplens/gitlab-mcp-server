// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package securitycategories implements MCP tools for GitLab security categories.
//
// The package exposes catalog-backed actions for creating, updating, and
// deleting the category records that group GitLab security attributes. Handlers
// validate namespace and category identifiers, optional update fields, and
// GraphQL mutation responses before returning MCP-friendly output with nested
// attribute summaries. The create and the update go through client-go's
// SecurityCategories service; the delete sends a document of its own, because
// client-go's DestroySecurityCategory does not select the attributes GitLab
// deleted with the category.
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
