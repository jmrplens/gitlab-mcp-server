// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package required is R-REQUIRED: every parameter an action's input schema
// requires, held to what GitLab requires of the parameter it is sent as
// (issue 1100). R-INPUT is the other question about an input, whether it
// carries every field of the client-go struct it fills.
//
// All three surfaces now read one answer to whether a call may leave a
// parameter out, the `,required` marker of the field's jsonschema tag, which
// the catalog route's schema carries and the individual projection no longer
// replaces. That makes the marker the one thing to hold to GitLab, and this
// scope does it without loading a program. What it reads is committed or
// built from the tree:
//
//   - the input schema of every action of the two Ultimate catalogs
//     (self-managed and GitLab.com), through actionids.Catalogs;
//   - docs/development/action-requests.json, the REST routes each action
//     sends, the ways it runs and the field of the input each call fills a
//     route's placeholders from, through actionrequests.ReadRecord;
//   - docs/development/gitlab-api-live.json, each route's params and whether
//     GitLab requires them, as a booted GitLab declared them.
//
// # How a field is judged
//
// A path placeholder is found through the field the request record says the
// calls fill it from, which the derivation reads off each call: GitLab's
// `:key_id` is the input's `deploy_key_id`. A parameter the record names no
// field for is found by name, since the inputs here spell a parameter as
// GitLab does, except for a path placeholder: GitLab writes the project of
// /projects/:id as `id` and every input writes it as `project_id`, so a bare
// `:id` is the collection in front of it in the singular, and a polymorphic
// `:noteable_id` is the identifier of the object the collection holds
// (place.go). A placeholder every spelling of the path carries is required
// whatever the record says, because the record leaves a path param it never
// redeclares unrequired. A param found by name under a field that fills a
// placeholder is not that field, since one field fills one parameter: the body
// `group_id` of /groups/:id/share is the group shared with, not the input's
// `group_id`. Where those rules would find no field or the wrong one, an alias
// names the right one (declarations.go).
//
// Workhorse rewrites the whole body of some requests into one required
// WorkhorseFile param, and where the endpoint parses that file and validates
// what it holds by hand the record declares none of what it holds.
// declaredWorkhorseBodies says what GitLab reads out of the body of each such
// route, citing lib/api, and those params are placed as if the record declared
// them. A declaration for a route no action sends, or whose record route
// declares a param of its body again or stops requiring the file, answers
// nothing.
//
// GitLab requires a field when every way the action runs sends a request that
// requires it; a way that does not is a call GitLab serves without it. A field
// no route of the action is found to carry is unplaced, and a field some way
// cannot be read for (a GraphQL request, an unresolved one) is unjudged.
// Neither is a finding, and both are listed so a reader sees what was not
// asked.
//
// # What it gates
//
// A field the schema leaves optional and GitLab requires (a call the surface
// accepts and GitLab refuses) and a field the schema requires and GitLab
// leaves optional (a call GitLab would serve that the surface refuses before
// sending it), unless a declaration answers it with a category and a reason;
// and a declaration, alias or Workhorse body that answers nothing, so the
// tables cannot outlive what they answer. It reads the committed request
// record, so in CI it runs beside R-GRANT under the FRESHNESS deferral.
//
// # What it cannot see
//
// A constraint across fields. GitLab's at_least_one_of and exactly_one_of
// (a member added by user_id or username, a project created with a name or a
// path) are not in the record, which writes each of those params as optional;
// the schemas say them with an anyOf a toolutil.SchemaAnyOfRequired override
// adds, and nothing here holds it to GitLab. Two exactly_one_of are published
// as a oneOf: an LDAP group link's cn or filter, which a
// toolutil.SchemaOneOfRequired override adds, and a snippet created from files
// or from file_name with content, which the snippets package writes by hand
// because a content brings its file_name with it. The others are still an
// anyOf or not stated at all, and this scope does not find them. The
// rules an element of a list is held to, such as a protected environment's
// deploy access level naming exactly one of a role, a user or a group, are
// stated in the item schema by a toolutil.SchemaItemsOneOfRequired override or
// one of its siblings, and the record writes each field of the element as an
// optional param, so this scope cannot hold them to GitLab either. A
// requirement GitLab states in a `given` block is written by the record as
// unconditional, which is what the conditional declarations answer. And a
// field of the body or the query an input names differently from GitLab
// without an alias is unplaced rather than judged, as is a path placeholder
// a call fills from a value GitLab answered or a helper produced, with no alias
// and no field of its name.
package required
