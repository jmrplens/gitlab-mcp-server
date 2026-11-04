// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package auditshared holds analysis helpers shared by the auditors: the
// projected-description probe, owner-package resolution, and the
// usage/description quality checks cmd/audit_discovery_completeness applies,
// which the R-META stream of cmd/audit_1to1 applied too until it was retired.
//
// It also holds [NewStubGitLabClient], the offline GitLab client the audit
// commands construct. That one is a thin delegation to
// mcpsurface.NewStubClientWithToken rather than a client of its own, so the
// audits and the generators share a single definition of what "no instance, no
// credentials" means: the same compiled-in URL, the same constant token, and
// the same disabled retries.
package auditshared
