package serialtypecheck

// The tests live in the external package serialtypecheck_test, and this file
// is all of the internal one. It must import nothing: a file of this package
// that imported go/packages, as a test of the probe would, makes the package
// depend on the loader in its own test binary, where it is then initialized
// after the loader and the semaphore is sized before GOMAXPROCS is set. That
// is the one binary the package documentation's argument would not cover, and
// TestInitialization_PrecedesGoPackages_InEveryBinary reads this file as part
// of the package for that reason.

// SerializeUnderRace is serializeUnderRace, for the tests.
var SerializeUnderRace = serializeUnderRace

// Processors is the value this package's initializer left in force.
var Processors = processors

// RaceEnabled reports which half of the build seam this binary compiled.
const RaceEnabled = raceEnabled
