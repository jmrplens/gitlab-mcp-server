//go:build e2e && !race

// server_norace.go is the ordinary half of the build seam, used by every run
// that is not `go test -race`. See server_race.go for the other half and for
// why the seam exists.

package harness

import "time"

// serverBuildArgs returns the go build arguments for the server under test.
func serverBuildArgs(out string) []string {
	return []string{"build", "-o", out, "./cmd/server"}
}

// serverBuildTimeout bounds that build.
const serverBuildTimeout = 5 * time.Minute

// raceChildEnv adds nothing to a child's environment when the detector is not
// in play.
func raceChildEnv() map[string]string { return nil }
