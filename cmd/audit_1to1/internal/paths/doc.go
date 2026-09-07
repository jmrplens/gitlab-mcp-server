// Package paths holds the 1:1 audit rule about the request an action actually
// issues (R-PATH).
//
// The other five rules all describe the surface: the fields we accept
// (R-INPUT), the fields we return (R-OUTPUT), the actions we register
// (R-ACTION), the discovery metadata we attach (R-META) and the enum values we
// advertise (R-ENUM). Every one of them compares what we publish against what
// the SDK and the API documentation offer, and not one of them looks at the
// request a handler builds. That is how nine registered tools shipped while
// being unable to work: a perfect input struct, a perfect output struct, a
// registered action, complete enums, and a request GitLab refuses.
//
// This rule reads the request instead. Its input is the inventory
// internal/testutil records and cmd/gen_request_inventory commits, which is the
// first honest answer to what this server sends GitLab, and it puts that
// inventory through three checks.
//
// # Has the path ever been observed
//
// An action whose owning package issued no request at all has never had its
// request seen by anything, which is exactly the state the broken documents
// were in. This is the gate. It is held at package grain because nothing on
// the wire names an action, so a package that recorded something covers every
// action it owns, and a package that recorded nothing is a finding against all
// of them.
//
// A package may nevertheless be silent for a reason, and internal/tools/adminspecs
// is the whole of it today: it declares specs whose handlers live in other
// packages, so its requests are recorded under the package that made them.
// Such a package is held to a declaration with a reason, the way -scope=sdk
// holds a client-go service to one, and a declaration that no longer describes
// the tree is itself a finding.
//
// # Does the document validate
//
// Every raw GraphQL document in the source, judged against the pinned schema.
// The reading and the judging are cmd/internal/graphqldocs's, shared with the
// standalone gate cmd/audit_graphql_documents, so there is one answer to
// whether a document is one GitLab would refuse.
//
// # Does the endpoint exist
//
// Each recorded REST endpoint against GitLab's own API documentation. This one
// is a candidate list a human adjudicates and deliberately not a gate, and the
// reasons are in [CheckEndpoints]: the oracle is prose, and a false failure
// here would poison the whole dimension.
//
// # Where the report goes
//
// Into this scope's own JSON, not into the merged backlog, for the reason
// -scope=sdk stays out of it: the merged backlog accumulates candidates a
// human adjudicates per package, and this scope gates. Adding a fifth stream to
// the merge would also change the shape of plan/1to1-backlog.json for the
// tooling that already reads it, in exchange for entries that are empty
// whenever the gate passes.
//
// # What none of it can do
//
// It checks that the request is well formed and that the endpoint is
// documented. It cannot check that GitLab answers it the way our output struct
// expects, because only a real GitLab can. The three layers stack: the schema
// and the inventory catch a request that cannot work, a real instance catches a
// response we misread, and the other five rules keep catching the surface we
// failed to expose.
package paths
