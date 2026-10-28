// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package params is R-PARAM: every param a route this server reaches
// declares, held to whether anything here can send it.
//
// R-INPUT compares an input with the client-go struct it fills, and R-PATH
// compares what was sent with what GitLab answers, so a param GitLab declares
// on a route this server already calls and that client-go's option struct does
// not model was seen by neither: client-go cannot send it, so no input offers
// it, so no recorded request carries it. Row 133 of the upstream register is
// the case: the params GitLab 19.5 added that client-go does not model came
// from a review of the diff between two records, which no gate read.
//
// # What it reads
//
// Every input is committed or built from the tree:
//
//   - docs/development/action-requests.json, the REST routes each action
//     sends, the client-go methods each request goes through and the
//     functions that build the ones that go through none;
//   - docs/development/gitlab-api-live.json, each route's params as a booted
//     GitLab declares them;
//   - docs/development/request-inventory.json, the params the unit suite
//     recorded each route being sent, which covers what a request option adds
//     and what a handler builds by hand;
//   - client-go's option structs, through [sdkoptions], walked into every
//     client-go struct a field carries and read with the encoder the
//     request's constructor uses, and, for a raw request, the static type of
//     its options as the sdk scope's walk of the tool packages reads it;
//   - the input schema of every action, through actionids.Catalogs, a param
//     with brackets looked up as an input spells it (`not[author_id]` as
//     `not_author_id`, `custom_headers[key]` under `custom_headers`), and a
//     GitLab name an input fills from another field through the aliases the
//     requiredness scope declares.
//
// # How a param is judged
//
// Per route, over the union of every action that reaches it: a param is
// covered when a recorded request of the route carries it, and otherwise it
// is a finding of one of three classes. The inventory records the top-level
// keys of a body, so a recorded key covers a param nested under it only
// where an option struct the route's requests carry models that param
// (`approval_rules[id]` under a recorded `approval_rules` is unmodeled while
// EnvironmentApprovalRuleOptions has no field for it). `unmodeled` is one no option struct
// those actions' requests carry models and no input of theirs offers, which
// is work for client-go before it is work here; `not_offered` is one an
// option struct they already hand client-go models and no input offers,
// which this repository can offer today; and `offered_unrecorded` is one an
// input offers that no recorded request carries, a hint of a handler that
// drops it or of a test that never sends it. A param of a route one of whose
// requests carries options the walk cannot read (a free map, or a struct of
// the tool package's own that embeds no client-go struct) and that no input
// offers is `unjudged` rather than unmodeled. The redirect GitLab answers
// another route with, which the HTTP client follows, carries no options of
// the action's and leaves its route judged.
//
// A path param is not judged, nor an attribute Workhorse fills for an
// uploaded file (`file.sha256`). A param that holds others (`custom_headers`
// and its `custom_headers[key]`) is covered when one of them is, and a
// container none of whose members is covered stands for them as one finding
// when they share its class.
//
// # What it gates
//
// Nothing a finding says: almost every one is a client-go merge request, and
// GitLab adds params every week. It gates the declaration table
// (declarations.go), which answers a finding with a category and a reason,
// and fails on a declaration that answers nothing, so an `upstream` one goes
// stale the day client-go models its param. It reads the committed request
// record, so in CI it runs beside R-GRANT under the FRESHNESS deferral.
package params
