//go:build race

// race.go is the race-detector half of the build seam: the go tool sets the
// race build tag when -race is used, so this file is what a race build
// compiles and norace.go is what every other build compiles.

package serialtypecheck

// raceEnabled is true in a race build, the one build whose type-checking has
// to be serial.
const raceEnabled = true
