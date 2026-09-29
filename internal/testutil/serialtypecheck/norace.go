//go:build !race

// norace.go is the ordinary half of the build seam, compiled by every build
// that is not a race build. See race.go for the other half.

package serialtypecheck

// raceEnabled is false outside a race build, where the loader keeps the
// parallelism it had.
const raceEnabled = false
