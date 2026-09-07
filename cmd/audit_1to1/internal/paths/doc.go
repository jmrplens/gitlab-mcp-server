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
// That grain is the check's limit and it is worth saying plainly, because the
// number it prints reads stronger than it is: 990 of 1082 actions observed
// means 990 actions whose owning package issued some request, not 990 actions
// whose own request anybody has seen. Summary.Grain says so beside the number,
// since the number is what gets quoted. As a regression guard it is real and
// as per-action assurance it is nothing, and the layer that closes that gap is
// a live instance rather than a stronger reading of this file.
//
// An action whose declared owner names no package at all is a finding of its
// own rather than a curiosity. Nothing in the catalog validates that an owner
// is a package, so such an action used to be classified unmapped, which was
// gated by nothing and dropped from the -gaps-only report: it could be neither
// counted nor seen. It is counted now. The owner "tools" is the exception the
// catalog itself defines, for the orchestration package rather than a domain
// under it, and it resolves to internal/tools, which records requests of its
// own.
//
// What no version of this can catch is an owner that names a real package and
// the wrong one, because the recording joins on that name and nothing else.
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
// This check judges the document and never the values sent with it, and the
// two halves of that family are worth keeping straight because neither
// substitutes for the other. Of the nine tools that shipped unable to work,
// four sent a document the schema refuses and this is what catches them; the
// other five sent a document the schema accepts carrying a value GitLab does
// not have, an enum miscased or a filter typed as a string, and what catches
// those is the validating transport in internal/testutil, which validates the
// variables with the document every time a test drives one. That check lives
// under every unit test and belongs to whoever keeps NewTestClient wrapping
// the handler it is given: removing that wrapper would remove half the
// coverage of this defect family, silently, which is why
// TestNewTestClient_Recording_WritesTheRequestTheClientMade and its GraphQL
// counterpart exist there.
//
// # Does the endpoint exist
//
// Each recorded REST endpoint against GitLab's own API documentation. It runs
// only when asked for, because the oracle is 250 pages over the network, and
// when it runs it gates: an endpoint no declaration in endpoint_declarations.go
// accounts for fails the run, which is what the issue behind this dimension
// asked for. The declarations are what make that safe, since the oracle is
// prose and a handful of endpoints GitLab serves are written down in a way no
// comparison can match; [EndpointCheck] has the categories and the reasons.
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
//
// Two more limits, both deliberate and neither obvious.
//
// The inventory records which parameter names an endpoint was sent and never
// which of them were sent together, so a combination of optional inputs that
// GitLab refuses is invisible here. gitlab_get_catalog_resource was in that
// state: id and full_path are both optional, both were published, GitLab
// accepts exactly one, and nothing sent both because no test did. The answer
// to that class is a handler that refuses the combination and a schema that
// says so, which is where the fix went, rather than an inventory of every
// combination the tests happen to use: a combination is a property of the
// fixtures, and a file that recorded them would be read as a contract.
//
// The issue that asked for this dimension also asked for the mock to refuse a
// request whose path is not in the committed inventory, and that is not built.
// It was declined while the inventory had never been read by anybody, on the
// grounds that a baseline nobody has checked is not one to gate against; the
// inventory has since been read and corrected, so the objection is spent and
// the reason it is still not here is only that an allow-list of paths fails
// every new test before its endpoint is committed, which is the deadlock the
// observation check already had to be taught to avoid.
package paths
