// Package tenancy is the register of every decision this server makes about
// who a caller is and what it may hold, spend or be told.
//
// It decides and never enforces. The layers that count streams, fill token
// buckets, evict pool entries and write refusals keep doing so where the
// resource is counted; what lives here is the answer they apply, written once:
//
//   - the key vocabulary ([Key]), where each key says what it costs a caller to
//     mint another value of it, which is the fact issues 540 and 561 each had
//     to rediscover by hand;
//   - one [Decision] row for each requirement of the tenant policy
//     specification, with its key, class, kind, disposition, value source,
//     zero meaning, capacity behavior and refusals ([Decisions]);
//   - the policy values and refusal codes the layers read (values.go,
//     codes.go);
//   - the rules promoted here from the layers, so that changing one is an
//     edit of the register: which bucket each MCP method is charged to
//     ([MeterFor], meter.go), which holdings make a pool entry busy
//     ([Busy], busy.go), and which settings switch an authentication budget
//     off and when the transport-source budget exists ([BudgetOn],
//     [EscalationOn], [TransportSourceBudgetOn], budget.go);
//   - the table of authentication failures and what each is charged
//     ([Failures]);
//   - the channels go-sdk v1.8.0 carries for each method ([Carriages]);
//   - [Validate] and [ValidateFailures], which turn the specification's
//     invariants into errors, so that a share on a key a caller can mint, a
//     per-caller ceiling on a process resource with no process partner, or a
//     refusal on a channel the method cannot carry is refused before review.
//
// The specification it implements is docs/development/tenant-policy-spec.md,
// and the decision to hold policy this way is ADR-0023
// (docs/development/adr/adr-0023-tenant-policy-is-declared-once.md). The dated
// record the rows were taken from, with its line citations, is
// plan/issue-565/spec.md, and it is the document the comments here cite by
// number: a section ("spec section 3.3", "spec 4.1") and a requirement ID
// that is not a row, an invariant or a validation item (TEN-, CON-, AC-) are
// the dated record's, which numbers them. The row IDs, INV- and VAL- are in
// the durable specification too.
//
// # A leaf with nothing to initialize
//
// The package imports only errors, fmt, strings and time, all of which the
// server binary already links, and it declares no package-level variable and
// no init function: every table is built by the function that returns it, on
// each call. So importing it adds no initialization work to a binary, a
// function nothing reachable calls is dropped by the linker, and a layer that
// replaces a literal with one of the constants here compiles to the same
// instructions and data. The tests in doc_test.go hold all three properties,
// because the proof that moving a value here changed nothing rests on them.
//
// A promoted rule is the one thing here a layer calls, so the proof that
// promoting it changed nothing is not the binary. It is an oracle test and a
// fuzz target holding the function to a verbatim copy of the code it replaced,
// and the function allocates nothing and leaves the site's lock profile as it
// was: Busy reads the watcher count through [Holdings], which takes the
// subscription manager's lock, and it does so exactly when the code it
// replaced did, only when no stream is open.
//
// # Findings
//
// A row that disagrees with the specification's invariants today carries the
// finding that records the disagreement, and [Validate] accepts it only
// because it does. No finding is fixed here: each is filed as an issue of its
// own, and [FindingIssue] names it.
package tenancy
