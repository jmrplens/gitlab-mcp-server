// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package protectedenvdata contains the protected environment shapes the
// project and the group protected environment tools share: the four entry
// shapes a protect and an update send, the deploy access levels and approval
// rules GitLab answers with, and the Markdown both packages render them in.
//
// GitLab declares the project and the group routes with the same param blocks
// (ee/lib/api/protected_environments.rb) and answers both with
// API::Entities::ProtectedEnvironment, and client-go models each side with
// structs that carry the same fields, so the shapes are declared once here and
// each package names them under its own name. Each package keeps its
// handlers, its converters to and from its own client-go structs, and its
// formatters, which differ in their headings and hints.
package protectedenvdata
