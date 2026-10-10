// Package required is the requiredness rule of R-INPUT: every parameter an
// action's input schema requires, held to what GitLab requires of the
// parameter it is sent as (issue 1100).
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
//     sends and the ways it runs, through actionrequests.ReadRecord;
//   - docs/development/gitlab-api-live.json, each route's params and whether
//     GitLab requires them, as a booted GitLab 19.4.1 declared them.
//
// # How a field is judged
//
// A field is found among a route's params by name, since the inputs here
// spell a parameter as GitLab does, except for a path placeholder: GitLab
// writes the project of /projects/:id as `id` and every input writes it as
// `project_id`, so a bare `:id` is the collection in front of it in the
// singular, and a polymorphic `:noteable_id` is the identifier of the object
// the collection holds (place.go). A placeholder every spelling of the path
// carries is required whatever the record says, because the record leaves a
// path param it never redeclares unrequired. Where those rules would find the
// wrong field, an alias names the right one (declarations.go).
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
// and a declaration or alias that answers nothing, so the table cannot outlive
// what it answers. It reads the committed request record, so in CI it runs
// beside R-GRANT under the FRESHNESS deferral.
//
// # What it cannot see
//
// A constraint across fields. GitLab's at_least_one_of and exactly_one_of
// (a member added by user_id or username, a project created with a name or a
// path) are not in the record, which writes each of those params as optional;
// the schemas say them with an anyOf a toolutil.SchemaAnyOfRequired override adds,
// and nothing here holds the anyOf to GitLab. A requirement GitLab states in a
// `given` block is written by the record as unconditional, which is what the
// conditional declarations answer. And a field an input names differently
// from GitLab without an alias is unplaced rather than judged.
package required
