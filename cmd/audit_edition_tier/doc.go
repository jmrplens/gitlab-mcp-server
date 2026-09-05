// Command audit_edition_tier reports the doc-grounded licensing tier of every
// canonical MCP action and compares it to the action's current gating.
//
// GitLab marks the required licensing tier per API endpoint in its
// documentation: a page-level `{{< details >}}` block carries a default
// `- Tier:` badge, and individual endpoint sections may override it with their
// own details block. The values seen are "Free, Premium, Ultimate" (all tiers),
// "Premium, Ultimate" (Premium minimum) and "Ultimate" (Ultimate only).
//
// Not every endpoint family is graded by its owner package's page: some are
// documented on a different API page (group webhooks) or carry their tier badge
// only on a user-facing page (merge request dependencies). actionDocOverrides
// in doc_map.go redirects those actions to the page that actually states their
// tier, and the tier is still parsed from that page's badge.
//
// The current MCP gating is binary and positional: an action is either present
// in the Community Edition catalog (effectively Free) or only in the Enterprise
// catalog (an undifferentiated Premium-or-Ultimate). This auditor surfaces, per
// owner domain, the doc tier distribution against that current gating so the
// per-action tier-assignment waves can be planned and later verified.
//
// Usage:
//
//	go run ./cmd/audit_edition_tier/                 # full report to stdout
//	go run ./cmd/audit_edition_tier/ -gaps-only      # only domains needing work
//	go run ./cmd/audit_edition_tier/ -offline        # use only cached docs
//	go run ./cmd/audit_edition_tier/ -output dist/edition-tier.json
package main
