//go:build stdioe2e && !race

// harness_norace_test.go is the ordinary half of the harness build seam, used
// by every run that is not `go test -race`. See harness_race_test.go for the
// other half and for why the seam exists.
package stdioe2e

import "time"

// serverBuildArgs returns the `go build` arguments for the server under test.
func serverBuildArgs(out string) []string {
	return []string{"build", "-o", out, "./cmd/server"}
}

// serverBuildTimeout bounds that build.
const serverBuildTimeout = 5 * time.Minute

// callTimeout bounds how long call waits for its response.
const callTimeout = 30 * time.Second

// registrationTimeout bounds a wait that ends only once the server has
// registered its catalog. An ordinary build registers the largest surface in
// under two seconds, so this only matters to a server that never finishes.
const registrationTimeout = 60 * time.Second

// raceEnviron adds nothing to the server's environment when the detector is
// not in play.
func raceEnviron() []string {
	return nil
}

// prebuiltBinaryRefusal reports that a staged server is usable: an ordinary run
// drives whatever binary it is pointed at.
func prebuiltBinaryRefusal() string {
	return ""
}
