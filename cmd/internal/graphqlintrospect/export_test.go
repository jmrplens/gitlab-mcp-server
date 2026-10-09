// export_test.go exposes the host fold to the external test package, which
// holds it byte for byte to the fold the server compares hosts with.
package graphqlintrospect

// FoldASCIICaseForTest exposes foldASCIICase.
var FoldASCIICaseForTest = foldASCIICase
