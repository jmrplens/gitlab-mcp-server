// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package routes is R-ROUTE: every route the live GitLab record mounts, held
// to an action of the catalog that sends it or to a reason none does.
//
// R-ACTION and the sdk scope start from client-go, and R-PATH from what the
// unit suite recorded, so a route client-go does not model was seen by no
// dimension: no method to call, so no action, so no request recorded. GET
// /pipelines and the two placeholder reassignment routes were in the 19.5.0
// record from the day it was pinned and surfaced only when client-go modeled
// them. This scope starts from the record.
//
// # What it reads
//
// Every input is committed or pinned:
//
//   - docs/development/gitlab-api-live.json, every route a booted GitLab
//     mounts, with the reason it skips the fine-grained check when it does;
//   - docs/development/action-requests.json, the REST routes each action
//     sends, spelled as the live record spells them, the client-go methods
//     its requests go through, the actions that send a GraphQL request, and
//     the spelling a request whose route the input chooses derives;
//   - the pinned client-go, through cmd/internal/sdkroutes, the routes each
//     method sends, found in the record the way a request is
//     (apilive.RouteIndex), its directory asked of `go list -m` rather than
//     of a typed load of the tool packages;
//   - the decisions already taken about client-go methods no handler calls:
//     acceptedMissingMethods of R-ACTION, the service declarations of the sdk
//     scope and the waits table;
//   - the catalog, which an action a declaration names must be in.
//
// # How a route is classified
//
// A route mounted twice is one route. In order, the first that holds wins:
//
//  1. reached: an action's request sends it.
//  2. outside-api: it is not mounted under /api/:version (SCIM).
//  3. skip: it skips the fine-grained check for a reason the closed table
//     skipReasons excludes, or a skipExceptions row takes it out. The table
//     is closed over the whole record: a reason a route carries that it does
//     not hold fails the gate whatever the route's class, and on a route
//     this step reaches it is a finding too. public_endpoint, a reason
//     GitLab's code lets a user's token call, and a reason some reached
//     route carries exclude nothing, so their routes go on to the rest of
//     the rules: a credential this server already sends to one route is one
//     it can send to the routes beside it. An exception names a route such a
//     reason leaves in whose handler refuses a user's token all the same
//     (the routes that check a Duo flow's composite identity).
//  4. A route under /internal no skip excludes is a finding:
//     internal-without-skip when it carries no skip reason, which every
//     route of the internal API does, internal-not-excluded when its reason
//     excludes nothing.
//  5. slug-from-input: a slugRules row names the action whose request
//     derives a spelling that leaves the segment to the input (an
//     integration's slug), and the route is one that spelling meets.
//  6. deprecated-alias: a /services/ route whose /integrations/ twin is
//     reached or answered.
//  7. inherited: every client-go method that sends the route is decided in
//     a way that holds for a route no request sends: COVERED_GRAPHQL with
//     the GraphQL actions graphqlActions names for the method, each in the
//     catalog and sending GraphQL, SUPERSEDED_UPSTREAM, UNWRAPPED_TRACKED,
//     or a waits entry whose handler builds the URL and sends nothing. A
//     COVERED_GRAPHQL decision no action stands for does not carry, since
//     the operation is then no action's. COVERED_GENERIC says another route
//     covers the capability, which is the question asked here, and does not
//     carry; COVERED_VARIANT and COVERED_RAW say this route is sent, so on a
//     route no action reaches they are a contradiction, a finding; a method
//     an action calls, an intentional skip and a method nothing decided do
//     not carry either. A COVERED_GENERIC or INTENTIONAL_SKIP_* decision on
//     a route a tracked row answers is a problem of the tables: the row
//     says an action is still to implement and the decision that none is
//     needed, and an action still to implement is no method R-ACTION may
//     settle.
//  8. declared: a routeFamilies row answers it, with its category
//     (client-protocol, covered-by-graphql or tracked), its reason and the
//     number of routes it pins.
//
// Anything else is a finding, with the client-go methods that send it: none
// means the fix starts upstream or with a raw request (ADR-0021). An
// /integrations/ finding whose deprecated /services/ twin an action reaches
// names that twin, since the capability is offered through it.
//
// # What it gates
//
// Every finding, of any class: a route no action reaches is answered by a
// rule, an inherited decision or a family row, so a re-pin that brings one,
// or an action that stops sending one, fails the gate until the change that
// causes it decides the route. And every declaration that no longer
// describes the tree: a family row that answers no route, answers another
// number of routes than it pins, overlaps another row, lacks what its
// category needs, is client-protocol over a route client-go sends, or is
// tracked over a route a COVERED_GENERIC or INTENTIONAL_SKIP_* decision
// settles; a
// skipReasons row no route carries or that lacks its category, and a reason
// a route carries that no row holds; a skipExceptions row that answers no
// route, gives no reason or excludes nothing; a graphqlActions row that
// answers no route, names no action, or names one the catalog does not hold
// or that sends no GraphQL; a slugRules row that answers nothing or names an
// action the catalog does not hold. It reads the committed request record,
// so in CI it runs beside R-GRANT under the FRESHNESS deferral.
//
// # How the family rows are decided
//
// A REST route whose operation an action of the catalog does over GraphQL is
// covered-by-graphql, with that action: a group's work item's discussions,
// notes and children among them, which the group.epic_* actions read by the
// work item's iid whatever its type. The protocol a package manager's or
// MLflow's own client speaks is client-protocol, except a download: a route
// that serves what a package or a model version holds (one of its files, its
// archive, its source or a file inside it, such as a Go module's go.mod) is
// tracked, so the action that downloads it stays in view, while an index or
// a metadata document a client reads to find and resolve packages (Debian's
// Release and Packages files, Helm's index.yaml, RPM's repodata, RubyGems'
// spec indexes and gemspecs), and the URL or the location of the content,
// are the protocol's. Everything else GitLab serves to a caller with a token
// is tracked by the issue of its domain, an administrator's, a GitLab.com
// and a tier's route among them, and so is a public_endpoint route, which a
// token reaches as any other.
package routes
