// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
// A badge grades an endpoint and never one of its parameters, and GitLab marks
// a paid parameter of a Free endpoint in the parameter's own table row
// instead: "Premium and Ultimate only" beside a board update's scope, "Ultimate
// only" beside a member's custom role. Reading badges alone, this auditor
// passed eleven such board inputs that every tier was offered (issue 1233), so
// it reads those rows too (param_tiers.go). Each section of a page is split
// into the endpoints its code blocks spell and the rows of its request tables,
// a table whose header has a Required column, that name a tier; the routes
// each action sends, read from docs/development/action-requests.json, join a
// section to the actions that call it, a route Grape records with an optional
// group ("(-/)search") in each of its forms; and a row whose parameter the
// input schema of a lower tier carries, read from the catalogs built at Free,
// Premium and Ultimate, is a finding naming the action, the route, the
// parameter, both tiers and the page.
//
// What the parameter pass cannot see is stated here rather than discovered: a
// row under a subheading of its endpoint (the options of a nested parameter)
// has no endpoint in its section and is not joined; a parameter our input
// names differently from GitLab is not matched; an action whose requests are
// GraphQL or that the record could not resolve has no route to join; a row
// whose tier names another key of the value ("`member_role_id` is Ultimate
// only" beside `allowed_to_merge`) or that GitLab.com serves on Free as well
// grades nothing; and only the owner page and the override page an action is
// redirected to are read, so a route documented on a third page is not seen.
// A finding is a lead rather than a verdict, since a row can mark a whole
// parameter for a tier that gates only some of its values, and the report
// gates nothing.
//
// A lead read against GitLab's source is answered in one of two places
// (issue 1240). A tier tag on the input field answers it in the schema: the
// tier filter prunes the parameter from the lower tiers and the finding goes
// away by itself. Any other answer is a declaration in param_declarations.go,
// keyed "action param", naming the tier its row names, a category and a
// reason: a value-level gate whose description says which values need the
// tier, a gated value the action does not offer, or a row GitLab's source
// contradicts. A declaration answers the findings of its tier, which the
// report moves to param_findings_answered, and is stale when it answers
// nothing. Half of that rule needs no page and is held by the unit suite: the
// action has to offer the parameter below the row's tier, and the schema the
// lowest tier offering it is served has to hold for the category: for a
// value-level gate, a description naming the tier beside each gated value the
// declaration names, which it has to name when the parameter enumerates its
// values, or anywhere for a gate on something other than a value; for a
// value not offered, an enum leaving out the gated values. The other half, a
// declaration whose action's pages were all read, the owner page and any
// override page, and that answered no finding, is reported under
// stale_param_declarations, since it rests on GitLab's pages, which a gate
// would have to fetch.
//
// Usage:
//
//	go run ./cmd/audit_edition_tier/                 # full report to stdout
//	go run ./cmd/audit_edition_tier/ -gaps-only      # only domains needing work
//	go run ./cmd/audit_edition_tier/ -offline        # use only cached docs
//	go run ./cmd/audit_edition_tier/ -output dist/edition-tier.json
package main
