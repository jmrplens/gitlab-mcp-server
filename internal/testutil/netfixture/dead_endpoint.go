package netfixture

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// listen opens the loopback listener a dead endpoint holds. It is a variable
// so the one failure real input cannot reach, a loopback port the kernel will
// not give out, is reachable from a test.
var listen = func(ctx context.Context) (net.Listener, error) {
	return (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
}

// DeadEndpoint returns the base URL, "http://127.0.0.1:port", of an address
// nothing answers on for the rest of the test.
//
// A listener holds the port and closes every connection it accepts before
// reading a byte, so every request sent there fails before any response,
// which code under test handles as it would an instance that is gone. Nothing
// else can be given the port while the test runs, which is the whole
// difference from a closed httptest server. The listener is closed by the
// cleanup DeadEndpoint registers, and cleanups run last registered first, so
// the port is held through every cleanup registered after the call and given
// back before any registered earlier: a cleanup registered before the call
// must not send a request to the address.
//
// The request does not fail with "connection refused": the connection is
// made, then ends with an EOF or a reset. A caller that needs only the host
// and port trims the scheme.
func DeadEndpoint(tb testing.TB) string {
	tb.Helper()
	url, err := deadEndpoint(tb)
	if err != nil {
		tb.Fatalf("netfixture: open a dead endpoint: %v", err)
	}
	return url
}

// deadEndpoint opens the listener and starts the loop that hangs up on each
// connection. The cleanup closes the listener and waits for the loop to end,
// so no goroutine of the fixture outlives the test that asked for it.
func deadEndpoint(tb testing.TB) (string, error) {
	tb.Helper()
	listener, err := listen(tb.Context())
	if err != nil {
		return "", err
	}
	done := make(chan struct{})
	go hangUpOnEach(listener, done)
	tb.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	return "http://" + listener.Addr().String(), nil
}

// hangUpOnEach accepts connections and closes each one at once, until the
// listener is closed, and then closes done. It never touches the test, so it
// stays within the rule for code that runs off the test goroutine.
//
// An accept that fails for any other reason, such as a process out of file
// descriptors while several test binaries run at once, is retried after a
// pause, as net/http's server does: a loop that ended there would leave the
// port held with nothing accepting, and the kernel would queue every
// connection made to it, so a request would wait out its deadline rather than
// fail at once.
func hangUpOnEach(listener net.Listener, done chan<- struct{}) {
	defer close(done)
	for {
		conn, err := listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		_ = conn.Close()
	}
}
