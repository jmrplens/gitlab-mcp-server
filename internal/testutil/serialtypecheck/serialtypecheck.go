package serialtypecheck

import "runtime"

// processors is the value golang.org/x/tools/go/packages sizes its
// type-checking semaphore from in a binary that links this package: one in a
// race build, and whatever the process started with in any other.
//
// It is a package-level variable rather than an init function so that the
// value is there for this package's tests to hold the loader to. Its
// initializer runs in this package's initialization step, which the package
// documentation shows comes before go/packages' own.
var processors = serializeUnderRace(raceEnabled)

// serializeUnderRace sets GOMAXPROCS to one when race is true and leaves it as
// it is otherwise, and returns the setting it leaves in force.
func serializeUnderRace(race bool) int {
	if race {
		runtime.GOMAXPROCS(1)
	}
	return runtime.GOMAXPROCS(0)
}
