// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_test_closed_servers reports every httptest server closed while
// its address is still used later in the same function.
//
// # Why this needs a rule of its own
//
// Closing an httptest server gives its port back to the kernel at once, and
// whatever is given the port next can answer a request sent to the old
// address: a parallel test of the same package starting a server of its own,
// or a test binary of another package running beside it in the same coverage
// run. A test that closed a server to get "an instance that is gone" therefore
// passed or failed on what else happened to be listening. cmd/bench_resources
// failed that way in CI, and issue 1132 moved sixteen more sites in seven
// packages, seven of them in parallel tests. They now use
// internal/testutil/netfixture: DeadEndpoint, an address that hangs up on every
// connection and keeps its port for the whole test, and StoppableServer, a
// server that answers until the test stops it and then hangs up, keeping its
// port. This gate keeps the pattern from coming back.
//
// # The rule
//
// A call to an httptest server's Close is reported when the server, or a name
// its address was given to, is used after the close in the same top-level
// function. The server is known by the method the type checker resolves, so
// one returned by a helper (stubVersionServer in internal/gitlab was the case
// that made this a requirement), held in a field, or embedded in a type of the
// test's own is followed like one started in place. A variable given that
// Close as a method value, `stop := srv.Close`, closes the server where it is
// called or handed to a Cleanup, until an assignment gives it something else.
//
// The server is also known by every other name it was given before the close:
// one it was copied from, `other := srv` closed as `other.Close()`, and what
// a field it is closed through was given anywhere in the load, by an
// assignment or a composite literal, so `holder{server: srv}` closed as
// `h.server.Close()` is srv's close, and a helper's `&fake{srv: s, url: s.URL}`
// makes the url field carry the address of the server closed as
// `f.srv.Close()`.
//
// The address is followed through assignments, to a fixed point: a URL saved
// before the close, a host and port trimmed from it, a client, a request or a
// function built on it, a struct holding it, whether it was built with it or
// given it later through a field (`cfg.base = srv.URL` makes cfg carry it),
// and the variables of a range statement over it. A string, or a slice of
// bytes, is followed only as an address is built, through selectors,
// operators, conversions and the functions of the standard packages that
// format, trim or join one (fmt, strings, net, net/url, path, path/filepath);
// text some other call returned is taken to be what it answered rather than
// the address it was handed. An error, a boolean, a number and an answer are
// never followed, since none of them can be dialed: an answer is a response
// or a header of net/http, a stream of io, or a struct that embeds one, as a
// client's own response type does.
//
// A name an assignment gives a new value to after the close is no longer
// followed from the end of that assignment to the end of the block, or case
// of a switch or a select, the assignment is in, since past that branch the
// old value may still be there: `if live { url = other.URL }` leaves the
// closed server's address in url for the case that does not take the branch.
// A new value built from the old one, as in
// `addr = strings.TrimPrefix(addr, "http://")`, is a use of it, and a value
// built from the name on the branch afterwards, `u := srv.URL` once srv holds
// a new server, is not; an operator such as `+=` reads the value it changes
// and gives the name no new one. A range statement's `=` gives its names
// their new values once the expression it ranges over has been read, and
// only until the loop ends, since a loop over nothing gives them none.
//
// A deferred close happens when the function that deferred it returns. In a
// function literal that is when the literal returns, so a use after the
// literal in the enclosing function is reported. In the top-level function
// nothing written after the defer follows it, but two things still can: what
// the function returns after the defer, which its caller holds once the close
// has run, so a helper that defers the close of the server whose address it
// returns is reported at the defer; and a parallel subtest, below. A close
// handed to a subtest's Cleanup happens when the subtest ends, which is when
// its literal returns. One handed to the Cleanup of the top-level function's
// test, as a method value or in a function literal, happens after everything
// the function runs, and is not judged; nor is one registered on a test a
// function literal was handed by its caller, the helper literal
// `newServer(t)` shape, which happens whenever that caller's test ends.
//
// A subtest, a function literal handed to Run, that calls Parallel on its own
// test goes on from that call only once the body that started it has
// returned, the closes that body defers included, and before that test's
// Cleanup runs. What it reads from the call on is therefore read after every
// close that body makes, in place, deferred, or in a subtest it runs and what
// that subtest defers or registers, and before a close registered on the
// Cleanup of that test or of an outer one. What such a subtest defers or
// registers itself happens when the test that started it ends.
//
// A close on a branch that then leaves the function, through a return, a
// panic, or a test's Fatal, FailNow or Skip, can only be followed by what is
// left of that branch: the error path of a helper that closes the server it
// could not use and panics is not followed into the success path below it,
// nor into a parallel subtest written after the branch, which is never
// started once the close is made. A branch is a block or a case of a switch
// or a select. That reading stops at a function literal, since a literal that
// returns leaves the function around it running.
//
// A finding names the close, the server as written, the first name the
// address is used through after it and that line, and the function. A
// function that closes a server and hands out its address, the shape of a
// helper returning "a URL nothing answers on", is reported where it closes
// it, whether it closes the server before the return or defers the close.
//
// # Declarations
//
// A function allowed to close a server and go on using its address is declared
// in declarations.go, keyed `package:Func` with a category and a reason. A
// declaration that excuses nothing is reported like every stale declaration in
// this repository, scoped to the packages the run loaded, and so is one naming
// a category nobody defined. The table is empty.
//
// # What it reads, and what it cannot see
//
// ./... with every package's tests, through cmd/internal/goprogram, under the
// build tags of every end-to-end suite (e2e, httpe2e, stdioe2e, collectore2e,
// orbitlive), so the test files behind each are read. A file a build
// constraint still leaves out, a platform or race variant, is not read, and is
// not silent: any such file that imports net/http/httptest is reported as
// unjudged and fails the gate. None exists today. A directory every file of
// which a constraint leaves out, a suite behind a tag not in that list or a
// package for one platform only, is not listed by ./... at all, so its files
// are neither read nor reported.
//
// It covers httptest servers only, as decided on issue 1132: a net.Listener
// closed and then dialed is not followed. Beyond the branch that leaves the
// function, it reads positions rather than control flow, so a close in one
// iteration of a loop and a use earlier in the body, reached on the next
// iteration, is not seen, a close inside a function literal that is neither
// deferred nor registered is taken to have happened where it is written, and
// a server copied under another name is taken to be the same server whether
// the copy was made before the close or after a rebind. A name given a new
// value on every branch of an if is still followed after it, since each of
// those values holds only to the end of its own branch. A parallel subtest
// reads each name as it stands where the subtest is written, so a value given
// to the name later in the body that started it is not seen there. A
// subtest whose literal reaches Run through a variable is not known to be
// parallel. A close made inside a helper the server is handed to is judged in
// the helper, not where the helper is called. A server held in a field is
// known by the field, so two values of one type are not told apart, and a
// field is given the same values wherever in the load it was given them. A
// server closed through anything but a name or a field, such as an index, a
// dereferenced pointer or the result of a call, is not judged and its close is
// not counted, and so is one closed through its Close held anywhere but a
// variable of the function, such as a field or a parameter. An address stored
// through an index, an explicit dereference or the result of a call is not
// followed, and text a function value returned is taken to be an answer. A
// value other than text or an answer that a call over the address returned,
// such as a decoded object, is taken to keep the address, so reading one
// after the close is reported. Each of these is a stated limit rather than a
// guarantee.
//
// Usage:
//
//	go run ./cmd/audit_test_closed_servers/          # report
//	go run ./cmd/audit_test_closed_servers/ -check   # fail on a finding (CI gate)
//	go run ./cmd/audit_test_closed_servers/ -v       # also list what a declaration excuses
//	go run ./cmd/audit_test_closed_servers/ ./internal/oauth  # one package
package main
