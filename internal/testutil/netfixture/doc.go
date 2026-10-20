// Package netfixture gives a test an HTTP address that stops answering
// without giving its port away.
//
// A test that needed "a server that is gone" used to start an httptest
// server and close it, keeping the URL. Closing the server gives the port
// back to the kernel at once, and the next listener given that port can
// answer the request the test exists to see fail: one of the package's
// parallel tests starting a server of its own, or a test binary of another
// package running beside it in the same coverage run. cmd/bench_resources had
// a test fail that way in CI, and issue 1132 moved sixteen more sites off
// the same pattern.
//
// [DeadEndpoint] is an address that accepts every connection and closes it
// before reading a byte, for the whole test. [StoppableServer] is a server
// that answers through its handler until the test calls
// [StoppableServer.Stop], and from then on hangs up on every request without
// answering, keeping its port: it is for the test that needs the instance to
// answer first and then go away, which closing an httptest server used to
// stand for.
//
// What a client meets at either address is not "connection refused". The
// connection is made and then ends, with an EOF or a reset, before any
// response arrives. A test that held a closed server's address to refuse the
// connection has to say what it actually checks instead: that no answer
// came.
//
// cmd/audit_test_closed_servers keeps the old pattern from coming back: it
// reports an httptest server closed while its address, or a value built from
// it, is still used later in the same function.
//
// The package sits beside [github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil]
// rather than inside it because internal/gitlab's own tests use it, and
// testutil imports internal/gitlab: importing testutil from those tests is an
// import cycle the compiler refuses. It imports nothing of this module, so
// any test in it may use it.
package netfixture
