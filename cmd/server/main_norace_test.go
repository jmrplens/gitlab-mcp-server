//go:build !race

// main_norace_test.go is the ordinary half of this package's test seam, used
// by every run that is not `go test -race`. See main_race_test.go for the other
// half and for why the seam exists.
package main

import "time"

// testCatalogBuildTimeout bounds a wait on a whole registration. An ordinary
// build registers the largest surface in under two seconds, so this only
// matters to a registration that never returns.
const testCatalogBuildTimeout = 30 * time.Second
