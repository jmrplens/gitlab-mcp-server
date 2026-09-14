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
// inventory through five checks.
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
// as per-action assurance it is nothing.
//
// The layer that closes that gap is a live instance, and it is here now as a
// report beside the gate. An end-to-end run has the one thing the unit
// recording lacks: the harness stamps a trace id into each MCP call, the
// server's span carries it back with the route the dispatcher actually chose,
// and every GitLab request the handler made is a child of that span. One trace
// is one call is one action. [E2EObservation] reads the dispatch lines of such
// a run when one is offered (-e2e-calls, the shards a Docker session left
// behind) and says which actions were themselves seen issuing a request and
// which ran and issued nothing.
//
// It reports and never gates, for two reasons that are both about what a
// missing record means. The shards are a byproduct of a run CI does not
// schedule and never commits, so an absent record is the ordinary case and
// failing on it would fail every push. And the counts are floors: the spans
// travel through a batching processor that drops silently when its queue
// overflows, so a trace whose client spans were dropped reads as an action
// that issued nothing. A positive claim here is therefore solid and a negative
// one is a lead.
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
// Until recently the question was asked only of the documents this repository
// writes, which left half the server's GraphQL surface unjudged: client-go
// builds another 42 inside its own module, for the achievement, work item,
// saved view, security attribute, security category, scan profile, target
// branch rule and Terraform state services, and each of them reaches GitLab
// through this server. [SDKGraphQLCheck] reads them out of the module source
// the typed shape comparison already resolves a directory for, and puts them
// to the same pinned schema.
//
// Six of the 42 are not sendable text and are counted apart rather than
// refused: client-go writes the work item documents as text/template shells
// and the Terraform state queries as printf format strings, so what the type
// checker folds carries a placeholder where a value belongs. No schema can
// judge those, and a refusal list with permanent entries in it is a list a
// reader learns to skip.
//
// That section reports and never gates, for a reason unlike the usual one: the
// pin is what GitLab serves, so a refusal there is real. What it is not is
// something this repository can fix — the answer is an upstream merge request
// and a version bump — and that is not a state to fail a build on. It also
// keeps this gate's verdict a statement about this repository's own tree,
// which is what everything else it fails on is.
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
// # Does GitLab say it sends what we publish
//
// The one check here whose oracle is GitLab itself: the record a booted GitLab
// produced, pinned by cmd/gen_api_live. It reports and never gates, and it asks
// its question at two grains, both published so a reader can compare them.
//
// One record answers both halves of it, which is why a finding can always say
// when GitLab sends a field. The two records this replaced could not: a
// generated document said what an endpoint returned and a scan of the Grape
// source said what gated each field, so a response could name an entity the
// scan had never read.
//
// The package grain unions the responses of every endpoint a package was
// recorded calling and holds each of that package's top-level output types
// against the union. It is exact for a package with one endpoint and weaker as
// the package grows, which is the honest shape available while the inventory
// records a package and never an action. It finds 610 fields across 130
// packages, and most of them are not phantoms: our own wrappers around a JSON
// array, our own answers to a 204 and to a not-found, and an endpoint the
// document gives no schema for sitting in a package where another endpoint has
// one.
//
// The type grain asks only about the endpoints a type actually models, along a
// chain in which every link already existed. A converter pairs an output type
// with a client-go struct, which is what structs.CollectOutputPairings reads
// out of the same pass the field diff runs over; client-go's service methods
// say which endpoints answer with that struct, which readSDKRoutes reads out of
// the SDK source the handlers compile against; and the document says what those
// endpoints send. Nothing in it consults the inventory, which is the point: the
// inventory cannot be sharpened, because it records a package by construction.
// Of 432 top-level output types it compares 24 and reports 3 fields, skipping
// 394 that no converter pairs, 1 that no method answers with, and 13 whose
// endpoints the document describes no response for. mrapprovals.ConfigOutput,
// the first confirmed phantom this repository found, is the case the join was
// built against: its old shape produces exactly the twenty findings the fix
// removed, and its current one produces none.
//
// It asks the same question one level down, since schema version 2 of the
// record carries the properties of each object a response nests. A nested
// output type is held against the properties the document gives the response
// property it sits under, and only when the document describes an object there
// at all: 11 nested types compared, 25 fields reported. The reticence is what
// makes that level usable, since it is the level whose first, unguarded attempt
// produced 1418 findings. None of those 25 is declared yet, so
// typed_undeclared_fields reads 25 while the three top-level findings are all
// answered: that counter spans both levels. They sit in issuelinks and
// pipelinetriggers, and adjudicating one means reading its page first.
//
// A finding at type grain can be answered rather than fixed, because the oracle
// is generated and is not always complete: an endpoint rendering a bare hash
// gets no useful schema, and a nested property can be given a narrower entity
// than the one the endpoint renders. shape_declarations.go is where such a
// finding is written down with its category and its evidence, on the terms
// every other declaration table here works on: a declaration that stops
// matching is itself a finding.
//
// # Does a list say where it ends
//
// R-PAGE, and the reason it is a rule of this package rather than a scope of its
// own: it needs exactly the three inputs this one already reads in a single
// pass, and it asks about a part of GitLab's answer that is in no entity.
//
// Pagination arrives in headers. An offset page comes with X-Page, X-Next-Page,
// X-Per-Page, X-Total and X-Total-Pages; a keyset page comes with a Link. So
// every rule that compares a published field against client-go's struct, the
// documentation or the entity record is looking in the one place the answer is
// not, and all six of them were green on internal/tools/impersonationtokens,
// which returns a bare array of tokens while GitLab serves twenty at a time. A
// caller cannot tell it has one page and cannot ask for the next.
//
// The oracle is the live record's params: 308 of its 2110 mounted routes declare
// per_page, 304 of those declare page beside it and the other four take a cursor
// or a page_token. That is a far stronger statement than a guess from an
// endpoint's name, and it is available because the record asks the router rather
// than reading prose.
//
// An action is judged when its output is a collection envelope, which is exactly
// one content field that is a list of objects, with this server's own framing
// taken out first. That strictness is where "the route declares the params but
// the action reads a single object" is answered: a project carrying
// shared_with_groups is a single-object read, and admitting it would have turned
// 24 findings into 79.
//
// The join from an action to an endpoint is the package, for the same reason the
// observation check's is, and it costs the same way: of the 24 findings, 14 name
// an action whose own route declares per_page and 10 matched a sibling's
// endpoint. Those 10 are written down in pagination_declarations.go with the
// route the record holds for each, on the terms every declaration table here
// works on. It reports and does not gate, since a finding is a surface change;
// its declaration table gates, since a claim that has stopped being true is not
// a candidate. See [PaginationCheck].
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
