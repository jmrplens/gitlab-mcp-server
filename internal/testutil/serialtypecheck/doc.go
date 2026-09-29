// Package serialtypecheck makes golang.org/x/tools/go/packages type-check one
// package at a time in a race build, and does nothing in any other build.
//
// It is imported for that side effect alone, with a blank import in a _test.go
// file of every package whose test binary links go/packages. It exports
// nothing, and nothing but a test binary ever links it:
// TestDependencies_TestSupport_NeverReachesTheServerBinary in cmd/server names
// it beside internal/testutil.
//
// # The race it works around
//
// go/types has a data race, reported upstream as golang/go#81122
// (https://github.com/golang/go/issues/81122) and unfixed in every Go release
// this repository can build with. (*Checker).isComplete reads the Named.fromRHS
// field of a named type without calling unpack first, while another Checker
// may be expanding the same type through unpack. The type is shared whenever
// two checkers import one package from export data and both reach one of its
// generic instances: iter.Seq[string], the result type of strings.SplitSeq, is
// the one this repository's tests hit. go/packages type-checks every package
// it loads from source in parallel, so any load that type-checks two or more
// of them can race, and under the detector a test that does so fails at
// random: 13 of 80 runs of internal/testutil/modelcorpus's boundary test did
// with five processors, and that test is what failed a release rehearsal's
// race gate.
//
// Serializing the checkers removes it, and go/packages already has the knob:
// it bounds its parsing and type-checking with one semaphore, sized once from
// runtime.GOMAXPROCS(0) when the package is initialized. With that at one, the
// checkers run one after another and hand over through the semaphore's
// channel, which is a happens-before edge the detector sees.
//
// # Why a package initializer, and not a test or a second process
//
// Setting GOMAXPROCS inside a test is too late, because the semaphore was
// sized before the first test ran. What sizes it is whatever GOMAXPROCS is when
// go/packages is initialized, and this package runs first by the rule of the
// language rather than by luck: since Go 1.21 the specification initializes,
// at every step, the first package in import path order whose imports are all
// initialized. This package imports only runtime, which go/packages imports
// too, so at the step where go/packages becomes ready this one is ready as
// well, and its path sorts first ("github.com/..." before "golang.org/..."),
// so it has already been initialized. That argument names nothing about the
// binary, which is why one blank import is enough in every package that needs
// it.
//
// The alternative this replaced on paper was a TestMain that starts the test
// binary again with GOMAXPROCS=1 in its environment. It would have to carry
// every flag, the exit status, the output streams, the coverage profile and a
// signal through a parent that runs nothing, be merged into the TestMain two
// of these packages already have, and stay out of the way of the tests that
// start this same binary as the command they test. It would also hand
// GOMAXPROCS=1 to every go command the loader starts, since that is how a
// child learns it; setting the value in the process leaves the environment as
// it was, so the go list behind each load keeps the parallelism it had.
//
// # What it costs
//
// A race build of a test binary that links this package starts with one
// processor, and keeps it unless -cpu asks for more; the loader's semaphore
// stays at one whatever -cpu says, because it was sized before. Measured
// with five processors on one machine, the 22 packages that link it took
// 1134 s under the detector before and 1359 s after, and the whole
// difference is cmd/audit_action_ids, 347 s before and 707 s after, the one
// that type-checks most (96 loads of up to 518 packages); the others moved
// within the noise of a shared machine. An ordinary build is unchanged: the
// initializer reads the setting and changes nothing.
//
// # What holds it in place
//
// The tests of this package hold the three things the argument above rests
// on: that go/packages parses one file at a time in a race build of a binary
// linking this package and more than one otherwise, which fails if x/tools
// stops sizing its semaphore at initialization; that this package's import
// path sorts before go/packages' and that it imports nothing go/packages does
// not, which is what makes the initialization order hold in every binary; and
// that every test binary in the module that links go/packages links this
// package too, which is what keeps a new loader from being added without it.
// The tests are an external package, because a test file inside this one
// that imported go/packages would break the order in the one binary they run
// in; export_test.go says so where the next test would be added.
//
// # When it goes
//
// The upstream fix is a guarded unpack in isComplete for a type the checker
// does not own, the same unpack hasVarSize already does before reading the
// field. When the Go release go.mod pins carries it, this package, its blank
// imports and its entry in the cmd/server dependency test are deleted
// together. docs/development/upstream-bugs.md records the entry and that
// condition.
package serialtypecheck
