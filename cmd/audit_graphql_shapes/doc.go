// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_graphql_shapes fails when a struct this repository decodes a
// GraphQL response into cannot hold what the document it is sent with asks
// for.
//
// Every other GraphQL gate reads the request. The pinned schema judges the
// document (make check-graphql-documents) and the test transport judges the
// variables, and neither looks at the struct the answer is decoded into,
// because the fixture a test answers with is written to match that struct. So
// a decoder that disagrees with GitLab is invisible to all of them: the
// licensed run found startLine and endLine declared as String by the schema
// and decoded into int by two response structs, every located finding failed
// to decode, and every gate was green.
//
// This command pairs the two halves. It loads the program with go/packages,
// finds every call whose first argument is a GraphQLQuery, folds the document
// that call sends, resolves what the pointer it decodes into points at, and
// walks the validated selection set against that type through encoding/json's
// own rules: a field is matched by its json tag or, failing that, its name
// case-insensitively, embedded structs are flattened, a type that unmarshals
// itself is trusted, json.Number holds a number as it is written and a string
// only when its text may be one, and a type parameter is read as whatever the
// caller bound it to: a generic function's by the type arguments of the call,
// and a method's of a generic type by the type arguments of the receiver it is
// called on.
//
// Three disagreements fail the gate: a field whose Go kind cannot hold what
// the schema says GitLab sends (a String decoded into an int, an object into a
// string, a list into a struct), a Go field the document never selects and so
// is always empty, and a scalar this audit has no serialization for. A field
// the document selects and no Go field reads is reported and does not fail:
// it is transfer, not truth. A position typed by a type parameter no caller
// binds does not fail either, and is counted apart on the summary line of a
// passing and a failing run alike rather than with the selections nothing
// reads, since it was never judged at all; the pairing holding it is not
// counted among those that agree, and -v names each position. A document that
// reaches its call through a wrapper's parameter is paired at every call of
// that wrapper, where the document is named; a document built at run time, or
// one the sibling audit finds that no call this audit can see sends, is a
// failure rather than a silence, because a document nobody judges is the
// shape this gate exists to refuse.
//
// # What the schema offers and nobody asks for
//
// The same walk answers the reverse question, which had no oracle at all. For
// REST, R-PATH holds GitLab's own OpenAPI record against our output types and
// reports the fields we do not publish; the eleven GraphQL-only domains
// contribute nothing to it, because their output types pair with no client-go
// struct and GitLab's REST record says nothing about them. They were invisible
// to every gate that asks what we fail to expose. -report writes what this
// walk finds instead: at every object a Go struct decodes, the fields the
// pinned schema offers that no document of the package that decodes it ever
// selects.
//
// The grain of that claim is the package and the schema type, and it is a
// claim about what was computed rather than about one document. A package
// sends several documents at one object and they differ on purpose: a CE
// document omits a Premium field and the EE document beside it selects it,
// and a mutation payload echoes an id where the query reads the whole object.
// So the walk records what each document did select alongside what it did not,
// and a field any document of the package selects is not reported. The
// document a finding names is the witness the field was offered at. A finding
// is filed against the package whose struct decodes the object, not the
// package the call is in, because the two differ wherever a shared wrapper
// sends the document and only the decoding package could publish the field.
//
// Three conditions bound it, and each is the difference between a backlog and
// a dump of the schema. The position must be one a Go struct decodes, so the
// walk is bounded by our own decoders. It must not be the operation root,
// whose fields are other requests rather than this response and which alone
// offers seventeen thousand of the twenty-two thousand fields an unbounded
// walk would report. And the object must be read rather than traversed, which
// a struct decoding at least one scalar or enum shows. On top of those, four
// exclusions each cost what sent.go records: connection plumbing, which is the
// six names that move a page rather than carry it and includes the cursor, the
// meta field, a field you must supply an argument to fetch, and the mutation id
// nobody supplies. A union is asked once per member the
// document names in a fragment and never about one it does not, which is what
// keeps a security finding's location family without reporting every variant
// of it.
//
// # How much of the surface the question was put to
//
// The report publishes a coverage figure rather than one number, because the
// positions asked about are fewer than the positions reached and a single
// count would say otherwise. A schema type is asked about once per pairing
// however often that pairing reaches it, so a later position selecting
// strictly less is skipped; an object decoding no leaf is traversed rather
// than read. Three further positions are left in silence and are counted so
// the silence is visible: an object selection no Go field decodes stops the
// walk with a note before anything under it is reached, an object decoded into
// a map has neither fields to judge nor a package to file a finding against
// (and is the one place the mutation-errors gate is not applied either), and a
// type that unmarshals itself is trusted with its own decoding and walked no
// further.
//
// One limit sits outside this run altogether. It loads only the packages under
// ./internal, so it sees only documents this repository's source writes; the
// operations client-go builds have their text and their decoder in the SDK,
// where there is nothing here to pair. The report names that set, read from the
// committed request inventory, and the client-go run below asks the same
// questions of it.
//
// It reports and does not gate. A field GitLab offers that this server does
// not surface is a candidate for the surface rather than a defect in it,
// GitLab adds fields weekly, and this dimension has neither a tier oracle nor
// a deprecation oracle to sort the candidates with, both of which the report
// states rather than leaving a reader to notice. One sub-class does gate, and
// on every run rather than only under -report: a mutation payload whose errors
// no field of the decoder reads, which drops GitLab's account of a refused
// mutation and reports success. What the document asks for is not the
// condition, since a payload that selects its errors and decodes none loses
// them just as completely; the reverse, a decoder field the document never
// selects, is already a hard failure of the leg above.
//
// A finding is answered rather than fixed by an entry in sent_declarations.go,
// keyed by package, schema type and field, and a declaration that answers
// nothing fails the run on the same terms as every other declaration table
// here: an excuse that outlives what it excused is a claim about the tree that
// is no longer true.
//
// # Documents assembled before they are sent
//
// A document written as the format of a fmt.Sprintf call, or as the text a
// text/template set renders into a builder whose String the request is given,
// is paired through the rendering the document inventory makes of it
// (graphqldocs.Assembly), under the name the call reaches it by. A shell the
// inventory could not render is a problem naming the inventory's reason, and
// each step of following a builder to its template set (the one Execute call
// that writes it, the package variable that call executes, the text that
// variable's set is parsed from) fails with a reason of its own.
//
// # The client-go half
//
// -sdk asks the same questions, the three disagreements above and what the
// schema offers that nothing selects, of the documents client-go builds and
// the structs it decodes them into, for the client-go go.mod requires, read
// from the module cache under the environment a load of somebody else's module
// needs (graphqldocs.ForeignModuleEnv). It asks them only of the sends this
// server reaches: a client-go function is reached when a package of ours names
// it, or names an interface method it implements (which is how a handler holds
// a service), or when a reached client-go function names it in turn, so a send
// in a helper only a service method calls is judged too. A send nothing reaches
// is listed and not judged, since it is a document this server never puts on
// the wire.
//
// Two things differ from the run over our own documents, each for a reason of
// the dependency. A field the schema offers is filed under the client-go
// service whose method sends the document rather than under a package, since
// every client-go decoder is declared in one package and the package grain
// would let one service's selection cancel another's gap. And an object that
// decodes only the next object is read rather than skipped when the object it
// hangs from is read: a work item's features and each widget under them are
// that shape, and skipping them hides every widget client-go leaves out, which
// is where GitLab grows the type. Applied to our own documents the same rule
// adds eighteen findings in the epic notes, discussions and issues packages,
// which is a question for those packages; the run over our own documents keeps
// the narrower rule.
//
// Every finding of the client-go run fails it unless a declaration answers it:
// a disagreement between a document and its decoder and a reached send whose
// document cannot be read before a request is made, answered in
// sdk_declarations.go, and a field nothing selects, answered in
// sdk_declarations_sent.go. A declaration that answers nothing fails it too.
// That is the difference from the report over our own documents, and it is
// what makes the one-time triage of issue 1022 persist: the run reads the
// client-go go.mod requires, so a bump that starts selecting a declared field
// leaves its declaration stale, and a bump that stops selecting one, or a
// re-pin that adds one, leaves a finding nothing answers. The exception is a
// type declared whole rather than field by field: the star
// already answers a field a re-pin adds to it. A recorded-upstream answer
// names the row of docs/development/upstream-bugs.md that records the gap.
//
// Usage:
//
//	go run ./cmd/audit_graphql_shapes/
//	go run ./cmd/audit_graphql_shapes/ -v
//	go run ./cmd/audit_graphql_shapes/ -schema /tmp/live/gitlab-schema.graphql
//	go run ./cmd/audit_graphql_shapes/ -report plan/graphql-sent.json
//	go run ./cmd/audit_graphql_shapes/ -sdk
//	go run ./cmd/audit_graphql_shapes/ -sdk -v -report plan/graphql-sdk-sent.json
package main
