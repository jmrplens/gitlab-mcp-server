// Package harness is the only route from an end-to-end test to an MCP server.
//
// It starts the real cmd/server binary and speaks to it over stdio, the way a
// client does. Nothing here assembles a server: a test that built its own
// would be testing its own assembly rather than the program, which is exactly
// what the suite this replaces did. The binary decides its own catalog from
// the environment it is given, so every knob a test turns is a variable or a
// flag the released program reads.
//
// The tests carry the e2e build tag; this file is what a plain build sees of
// the package, the convention test/e2e/http/doc.go states.
//
// The library is deliberately not importable outside test/e2e: Go's internal
// rule keeps it there, so no production package can grow a dependency on test
// scaffolding.
package harness
