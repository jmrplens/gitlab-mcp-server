// Package actionfixture holds the fixture packages the suites of
// cmd/internal/actionrequests and cmd/audit_readonly_graphql both load: Go
// source written as strings, put into the loader's overlay under [Dir], a
// directory that exists nowhere on disk, and type-checked against the real
// toolutil and client-go so a resolver is held to the shapes it meets in the
// tree rather than to a mock of them.
//
// It is a package of its own because both suites need the same packages: the
// resolver's tests hold the walk to them, and the audit's tests hold what the
// audit makes of the walk to them. Two copies would drift, and a fixture that
// drifted in one suite tests a shape the other no longer has.
//
// Nothing here is linked into a binary: every reader is a test.
package actionfixture
