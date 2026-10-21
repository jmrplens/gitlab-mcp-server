// Package sourcegate is the scaffolding the gates that judge this repository's
// own source share above the load: the command line they read, the exit code
// they end on, the declaration table they are held to, and the name a finding
// gives the function it sits in.
//
// Four gates are built on it, each asking its own question of the program
// cmd/internal/goprogram loads. cmd/audit_test_closed_servers holds every
// httptest server to being closed only once its address is no longer used,
// cmd/audit_sdk_context holds every call into client-go to the caller's
// context, cmd/audit_dead_consts holds every unexported constant to something
// that reads it, and cmd/audit_tenancy holds the tenant policy register to the
// code that enforces it. What each keeps is what makes it that gate: its load
// options, its scanner, its findings and their order, the words of its report
// and its summary, its declarations and their categories, and its binary.
//
// What they shared they used to share as copies, written once per gate in the
// same words: the flag set and its two exit codes, the end of a run, the split
// of a run's findings into the excused and the reported, the rules that report
// a stale declaration and an undefined category, and the spelling of a method
// by its receiver. A change to any of them was a change to every gate, with
// one copy easy to forget, which is how two copies of a rule come to answer
// the same tree differently; the receiver's spelling had already begun to, one
// of its four copies guarding a receiver list with nothing in it and the other
// three indexing past its end.
//
// # The command line and the exit code
//
// [Main] is a gate's main function: it hands the gate's command line the
// process's arguments and standard streams and ends the process through
// [Exit], the one seam a gate's test replaces to read the code instead of
// ending the test binary. [Run] parses a command line with the three flags
// every gate reads, -dir, -check and -v, named after the gate and described in
// its own words ([Usage]), and starts the gate on what they say; a request for
// help ends the run with 0 and arguments it cannot parse with 2.
// cmd/audit_tenancy, which adds -compare-binaries and refuses patterns, builds
// the same command line with [NewCommand] and defines its flag on
// [Command.Flags] before parsing. cmd/audit_dead_consts still reads the
// process's own flag set, so its help names the binary rather than the gate,
// and moving it here would change what its -h prints.
//
// [Finish] is the end of every run: a run that could not be made says so on
// standard error and exits 1, and a run that was made writes its [Report] and
// exits 1 only under -check when the report is not clean.
//
// # The declaration table
//
// A gate that lets a function do on purpose what it otherwise reports keeps a
// [Table] of those functions, each with a [Declaration] giving a category and
// a reason. [Judge] holds a run's findings against it and returns the
// [Verdict] a report embeds: what the table excused, what it did not, and the
// two ways the table itself fails, an entry that excused nothing in a package
// the run scanned and an entry whose category nobody defined.
// cmd/audit_test_closed_servers and cmd/audit_sdk_context are held to one this
// way. cmd/audit_dead_consts keys its table by constant rather than by
// function and passes over a key that names no package, and cmd/audit_tenancy
// holds its exemptions to rules of its own, so both keep theirs.
//
// # The name of the place a finding sits
//
// [FuncDeclName] spells a declaration the way its file does, `Type.Method`
// with the receiver's pointer and type parameters taken off, which is the
// function half of a declaration key and of what a finding prints. All four
// gates name functions by it: cmd/audit_dead_consts adds the constant's name
// after it, and cmd/audit_tenancy keys by it the functions its register names
// as sites. [IdentsAsExprs] is the one-line widening two of their walks need to
// read a declaration and an assignment alike.
package sourcegate
