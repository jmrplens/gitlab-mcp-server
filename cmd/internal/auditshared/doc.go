// Package auditshared holds analysis helpers shared by the discovery-metadata
// auditors (cmd/audit_1to1 R-META and cmd/audit_discovery_completeness): the
// projected-description probe, owner-package resolution, and the shared
// usage/description quality checks.
//
// It also holds [NewStubGitLabClient], the offline GitLab client the audit
// commands construct. That one is a thin delegation to
// mcpsurface.NewStubClientWithToken rather than a client of its own, so the
// audits and the generators share a single definition of what "no instance, no
// credentials" means: the same compiled-in URL, the same constant token, and
// the same disabled retries.
package auditshared
