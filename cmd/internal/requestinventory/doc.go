// Package requestinventory holds the committed record of what this server
// sends GitLab, and the one definition of which catalog actions it covers.
//
// The artifact itself is written by cmd/gen_request_inventory out of the shards
// internal/testutil records, and read by cmd/audit_1to1's paths scope, which
// audits it. Both need the same answer to "which package was seen issuing
// nothing", and two answers to that question would be worse than none: the
// generator's summary and the auditor's gate would disagree about the number
// while both looked right.
//
// # What a row is, and what it is not
//
// One row per package, method and templated endpoint, carrying the union of
// the parameter names that package was seen to send it. A row is not keyed by
// action, because nothing on the wire names one: the httptest server answers on
// its own goroutine while the test goroutine that called the handler is out of
// reach, and the catalog's route is a closure over its handler, so neither the
// stack nor the catalog can hand back an action to attribute a request to.
//
// The consequence runs through everything here. Coverage is per owning
// package, so "covered" means the package that owns this action issued some
// request, never that this action's request was seen. It is the strongest
// statement the recording can support, and saying more would be a guess
// dressed as a measurement.
package requestinventory
