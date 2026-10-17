//go:build !race

// jobs_norace_test.go is the ordinary half of this package's test seam, used
// by every run that is not `go test -race`. See jobs_race_test.go for the
// other half and for why the seam exists.
package jobs

// raceEnabled reports that this test binary was built with -race. An ordinary
// build allocates what the server allocates, so allocation bounds hold here.
const raceEnabled = false
