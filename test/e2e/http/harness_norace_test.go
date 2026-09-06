//go:build httpe2e && !race

// harness_norace_test.go is the ordinary half of the harness build seam, used
// by every run that is not `go test -race`. See harness_race_test.go for the
// other half and for why the seam exists.
package httpe2e

import "time"

// serverBuildArgs returns the `go build` arguments for the server under test.
func serverBuildArgs(out string) []string {
	return []string{"build", "-o", out, "./cmd/server"}
}

// serverBuildTimeout bounds that build.
const serverBuildTimeout = 5 * time.Minute

// raceEnviron adds nothing to the server's environment when the detector is
// not in play.
func raceEnviron() []string {
	return nil
}
