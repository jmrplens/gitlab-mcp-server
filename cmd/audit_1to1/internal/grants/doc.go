// Package grants is R-GRANT, the dimension of the 1:1 audit that says what a
// fine-grained personal access token needs for each action this server offers
// (issue 952).
//
// The other dimensions compare what the server publishes or sends with what
// client-go and GitLab offer. This one reads the answer the fine-grained
// derivation (cmd/gen_action_grants) committed and holds it to GitLab and to
// what the suites were seen sending. It deliberately loads no program: the
// questions that need the handlers' source are asked by the generator, which
// loads it and gates them (make check-action-grants-derivation). What is read
// here is committed:
//
//   - the table the server compiles in, through actiongrants.Table;
//   - docs/development/action-requests.json, what each action sends and what
//     shaped it, through actionrequests.ReadRecord;
//   - docs/development/gitlab-api-live.json, what GitLab 19.4.1 declares;
//   - docs/development/request-inventory.json, what the unit suite sent;
//   - and, when a directory is named, the dispatch lines an end-to-end run
//     recorded.
//
// # What it gates
//
// The committed artifacts' consistency with the live record, with each other
// and with the catalog: the table was joined at the record's release, names
// the record's permission vocabulary and public sets, denies nothing on an
// element the record does not hold as the kind its cause says (the
// derivation's gate 2, read back from the table), demands on every REST
// operation what the record's route declares (the join's REST half, read back
// through the same apilive.Route.Requirements), and covers the actions the
// request record and the catalog cover. A table that fails one of these was
// joined from another record or left behind by one, and every answer below
// would be read from it. What a GraphQL operation demands is not read back,
// since that needs the documents walked against the schema, which is the
// derivation's work. It runs where the other committed-artifact gates run,
// under the FRESHNESS deferral in CI.
//
// # What it reports and never gates
//
// Per action, the permissions in GitLab's words, worded by the same
// finegrained.Table.Describe the reference page and gitlab://tools/{id} use,
// with the directive or declaration that shaped each request; the actions no
// fine-grained token reaches at the recorded release, by cause (the set a
// fine-grained session is withheld, and issue 1054's inventory where the cause
// is GraphQL's); the positions served empty; the REST operations a public
// project or group serves with no grant at all; the GraphQL worklist of issue
// 1055, each undeclared type or mutation this server reaches with the actions
// it blocks and, as a lead and never a verdict, the REST routes this server
// already calls that declare a permission for the same resource; and two
// cross-checks of the derivation, at package grain against the request
// inventory and at action grain against an end-to-end run's request counts
// and the routes its client spans reached. Both cross-checks are leads: the
// inventory names a package and never an action, a count is a floor, and a
// route a trace reached that the derivation does not name is a positive claim
// that the derivation under-approximates the action, read from a record the
// run does not commit.
package grants
