//go:build race

// main_race_test.go is the race-detector half of this package's test seam. The
// go tool sets the race build tag when -race is used, so this file is what
// `go test -race ./cmd/server/` compiles and main_norace_test.go is what every
// other run compiles.
package main

import "time"

// testCatalogBuildTimeout bounds a wait on a whole registration, the slow half
// of a server's start, so that one that never returns is reported by the test
// that waited for it rather than by the package timeout.
//
// It is not a liveness bound like testHTTPLivenessTimeout, because a
// registration is not fast under the detector. A cold registration of the
// individual surface takes about 1.0 second of an ordinary build and about 13
// of an instrumented one on an idle five-core machine: every phase of it, the
// catalog build, the schema resolution of a thousand AddTool calls and the
// listing the tool count and the manifest share, runs ten to fourteen times
// slower, and it runs on one core. Before registration stopped listing its
// tools a second time it took 16 instrumented seconds on that machine, a CI
// runner a third slower per core took it past the 30 seconds the ordinary
// build is given, and half a core took it to 43. Three minutes is four times
// that worst measurement and still a bound.
const testCatalogBuildTimeout = 3 * time.Minute
