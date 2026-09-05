// Package dynamiccatalog builds the catalog the dynamic surface executes over,
// for every caller that needs one built the way the server builds it.
//
// It is a package of its own because the two halves it joins cannot import
// each other: internal/tools owns the catalog and its filter, and its tests
// import internal/tools/dynamic, whose tests import internal/tools. A build
// function in either would be an import cycle in that package's tests. Here
// it is reachable from cmd/server and from the end-to-end suite alike, which
// is the point: the suite used to assemble its own catalog, in the other
// order and with no withheld bookkeeping, and its read-only session answered a
// withheld write with "unknown action" while the shipped binary answered
// "exists but is not available".
package dynamiccatalog
