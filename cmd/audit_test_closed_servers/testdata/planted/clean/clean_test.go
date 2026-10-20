package clean

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// base is the address TestMain publishes to the package's tests.
var base string

// use stands for anything that sends a request to an address.
func use(string) {}

// statusOf, ping and fetch stand for calls that return what an address
// answered rather than the address.
func statusOf(string) int     { return http.StatusOK }
func ping(string) error       { return errors.New("no answer") }
func fetch(url string) string { return "body of " + url }

// TestMain closes the package's server after every test has run, and uses
// nothing of it after the close.
func TestMain(m *testing.M) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base = srv.URL
	code := m.Run()
	srv.Close()
	os.Exit(code)
}

// TestPlanted_DeferredClose_Clean closes when the function returns, the
// literal form included.
func TestPlanted_DeferredClose_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	other := httptest.NewServer(http.NotFoundHandler())
	defer func() { other.Close() }()
	use(srv.URL)
	use(other.URL)
}

// TestPlanted_CleanupClose_Clean closes when the test ends, as a method value
// and inside a literal.
func TestPlanted_CleanupClose_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	other := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(func() { other.Close() })
	use(srv.URL)
	use(other.URL)
}

// TestPlanted_ClosedToWaitForItsHandler_Clean closes the server to wait for
// its handler, then reads only what the handler recorded.
func TestPlanted_ClosedToWaitForItsHandler_Clean(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	use(srv.URL)
	srv.Close()
	if hits != 1 {
		t.Errorf("hits = %d", hits)
	}
}

// TestPlanted_AnswersReadAfterTheClose_Clean keeps what the address answered,
// none of which can carry the address on to anything that dials it.
func TestPlanted_AnswersReadAfterTheClose_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	status := statusOf(srv.URL)
	err := ping(srv.URL)
	body := fetch(srv.URL)
	ok := status == http.StatusOK
	srv.Close()
	srv.Close()
	if !ok || err == nil || body == "" {
		t.Error("unexpected answers")
	}
}

// TestPlanted_Rebound_Clean gives the server's variable, and the address's,
// new values after the close, so what is used afterwards is not the closed
// server's.
func TestPlanted_Rebound_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	url = base
	srv = httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	use(url)
	use(srv.URL)
}

// TestPlanted_ReboundThenExtended_Clean gives two saved addresses new values
// after the close, one by an assignment and one by a short declaration that
// redeclares it, and extends each new value in place on the very next line:
// what is extended is not the closed server's.
func TestPlanted_ReboundThenExtended_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	host := srv.URL
	srv.Close()
	url = base
	url += "/api/v4"
	host, port := base, ":80"
	host += port
	use(url)
	use(host)
}

// server is a type of this package with a Close and a URL of its own.
type server struct{ URL string }

// Close does nothing; it is not httptest's.
func (server) Close() {}

// TestPlanted_AnotherTypesClose_Clean closes something that is not an httptest
// server.
func TestPlanted_AnotherTypesClose_Clean(t *testing.T) {
	srv := server{URL: base}
	srv.Close()
	use(srv.URL)
}

// TestPlanted_UsedBeforeTheCloseOnly_Clean uses the address and then closes.
func TestPlanted_UsedBeforeTheCloseOnly_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	use(srv.URL)
	srv.Close()
}

// TestPlanted_ServerNotHeldInAVariable_Clean closes a server nothing names.
func TestPlanted_ServerNotHeldInAVariable_Clean(t *testing.T) {
	newServer().Close()
}

// newServer returns a server the caller does not keep.
func newServer() *httptest.Server { return httptest.NewServer(http.NotFoundHandler()) }

// client is bound to an address when it is built.
type client struct{ base string }

// dial binds a client to base the way a constructor that can fail does.
func dial(base string) (*client, error) { return &client{base: base}, nil }

// newStubClient closes its server only on the branch that panics, the shape
// of the stub client cmd/internal/mcpsurface builds: the client returned after
// the branch never meets the close.
func newStubClient() (*client, func()) {
	srv := httptest.NewServer(http.NotFoundHandler())
	built, err := dial(srv.URL)
	if err != nil {
		srv.Close()
		panic(err)
	}
	return built, srv.Close
}

// closedBeforeReturning closes its server only on the branch that returns,
// and otherwise defers the close and returns what the server answered, which
// is not its address.
func closedBeforeReturning() string {
	srv := httptest.NewServer(http.NotFoundHandler())
	c, err := dial(srv.URL)
	if err != nil {
		srv.Close()
		return ""
	}
	defer srv.Close()
	return fetch(c.base)
}

// closedOnACaseThatReturns closes its server only in the case of a switch
// that returns, and its second only in the case of a select that panics, so
// neither close is followed into what comes after the switch or the select;
// what it returns once both closes are deferred is an answer.
func closedOnACaseThatReturns(ready chan struct{}) string {
	srv := httptest.NewServer(http.NotFoundHandler())
	c, err := dial(srv.URL)
	switch {
	case err != nil:
		srv.Close()
		return ""
	}
	defer srv.Close()
	other := httptest.NewServer(http.NotFoundHandler())
	select {
	case <-ready:
		other.Close()
		panic("closed")
	default:
	}
	defer other.Close()
	return fetch(c.base + other.URL)
}

// closedOnNestedEndingBranches closes its server on a branch that returns,
// inside another that panics, and returns an answer once the close is
// deferred.
func closedOnNestedEndingBranches() string {
	srv := httptest.NewServer(http.NotFoundHandler())
	c, err := dial(srv.URL)
	if err != nil {
		if c == nil {
			srv.Close()
			return ""
		}
		panic(err)
	}
	defer srv.Close()
	return fetch(c.base)
}

// TestPlanted_ServerPerSubtest_Clean starts and defers the close of a server
// inside each subtest, which is the ordinary shape: nothing after the subtest
// names the server.
func TestPlanted_ServerPerSubtest_Clean(t *testing.T) {
	t.Run("one", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		use(srv.URL)
	})
	use(closedOnNestedEndingBranches())
}

// closer holds a Close as a field.
type closer struct{ Close func() }

// TestPlanted_NotFollowed_Clean holds the shapes the gate does not follow by
// design: a Close that is a field of the test's own, a string a function value
// returned, an address stored through an index, a declaration that gives no
// value, and a server closed through an index, which is not judged at all.
func TestPlanted_NotFollowed_Clean(t *testing.T) {
	c := closer{Close: func() {}}
	c.Close()
	srv := httptest.NewServer(http.NotFoundHandler())
	get := func(u string) string { return u }
	body := get(srv.URL)
	stored := map[string]string{}
	stored["url"] = srv.URL
	var later string
	srv.Close()
	use(body)
	use(stored["url"])
	use(later)
	servers := []*httptest.Server{httptest.NewServer(http.NotFoundHandler())}
	indexed := servers[0].URL
	servers[0].Close()
	use(indexed)
}

// TestPlanted_ClosedOnABranchThatStopsTheTest_Clean closes its server only on
// the branch that stops the test, inside a branch of its own.
func TestPlanted_ClosedOnABranchThatStopsTheTest_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	c, err := dial(srv.URL)
	if err != nil {
		if c == nil {
			srv.Close()
		}
		t.Fatalf("dial: %v", err)
	}
	defer srv.Close()
	use(c.base)
	built, _ := newStubClient()
	use(built.base)
	use(closedBeforeReturning())
	use(closedOnACaseThatReturns(nil))
}

// TestPlanted_ReboundBeforeItsAddressIsSaved_Clean closes a server, gives its
// variable a new one, and only then saves the address it uses.
func TestPlanted_ReboundBeforeItsAddressIsSaved_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	srv = httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	u := srv.URL
	use(u)
}

// TestPlanted_RangeReassignsTheName_Clean gives the saved address a new value
// from a range after the close, so what the loop body uses is not the closed
// server's.
func TestPlanted_RangeReassignsTheName_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	for _, u = range []string{base} {
		use(u)
	}
}

// TestPlanted_HelperRegistersTheCloseOnItsCaller_Clean starts each server in a
// function literal that registers the close on the test it is handed, as a
// literal and as a method value, so the server lives until that test ends.
func TestPlanted_HelperRegistersTheCloseOnItsCaller_Clean(t *testing.T) {
	newServer := func(t *testing.T) *httptest.Server {
		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(func() { srv.Close() })
		return srv
	}
	newOther := func(tb testing.TB) *httptest.Server {
		srv := httptest.NewServer(http.NotFoundHandler())
		tb.Cleanup(srv.Close)
		return srv
	}
	use(newServer(t).URL)
	use(newOther(t).URL)
}

// TestPlanted_ClosedWhenTheParentEnds_Clean registers closes that wait for
// the test itself to end: one on the parent's test from inside a subtest, and
// a deferred close and a cleanup in a parallel subtest, which goes on only
// once the parent's body has returned.
func TestPlanted_ClosedWhenTheParentEnds_Clean(t *testing.T) {
	onParent := httptest.NewServer(http.NotFoundHandler())
	deferred := httptest.NewServer(http.NotFoundHandler())
	cleaned := httptest.NewServer(http.NotFoundHandler())
	t.Run("registers on the parent", func(*testing.T) {
		t.Cleanup(onParent.Close)
	})
	t.Run("parallel", func(t *testing.T) {
		t.Parallel()
		defer deferred.Close()
		t.Cleanup(cleaned.Close)
	})
	use(onParent.URL)
	use(deferred.URL)
	use(cleaned.URL)
}

// response wraps an answer the way a client's own response type does.
type response struct{ *http.Response }

// bodyOf, bytesOf, headerOf and call stand for calls that return what an
// address answered: a body, its bytes, a header, and a response of a client's
// own type.
func bodyOf(string) io.ReadCloser   { return io.NopCloser(strings.NewReader("")) }
func bytesOf(string) []byte         { return nil }
func headerOf(string) http.Header   { return http.Header{} }
func call(string) (*response, bool) { return &response{Response: &http.Response{}}, true }

// TestPlanted_ResponsesReadAfterTheClose_Clean reads, after the close, the
// response a request to the address returned and the other forms an answer
// takes, none of which carries the address on to anything that dials it.
func TestPlanted_ResponsesReadAfterTheClose_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body := bodyOf(srv.URL)
	raw := bytesOf(srv.URL)
	header := headerOf(srv.URL)
	wrapped, _ := call(srv.URL)
	srv.Close()
	if resp.StatusCode != http.StatusNotFound || body == nil || raw != nil || header == nil || wrapped == nil {
		t.Error("unexpected answers")
	}
}

// TestPlanted_ForgottenAfterTheClose_Clean closes its server and drops it,
// using nothing of it afterwards.
func TestPlanted_ForgottenAfterTheClose_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	srv = nil
	if srv != nil {
		t.Error("the server is still held")
	}
}

// settings is a struct a function returns, so a store into it has no
// variable at its root.
type settings struct{ base string }

// newSettings returns settings nothing else holds.
func newSettings() *settings { return &settings{} }

// TestPlanted_StoresNotFollowed_Clean stores the address where no variable of
// the test holds it, in the settings a call returned, gives a variable of
// another package a value, and registers a Cleanup on something that is not
// an httptest server.
func TestPlanted_StoresNotFollowed_Clean(t *testing.T) {
	const path = "/api/v4"
	srv := httptest.NewServer(http.NotFoundHandler())
	newSettings().base = srv.URL + path
	http.DefaultClient = &http.Client{Transport: http.DefaultTransport}
	c := closer{Close: func() {}}
	t.Cleanup(c.Close)
	srv.Close()
}

// answeredWithADeferredClose defers the close of its server and returns what
// it answered: a number and an error, neither of which carries the address.
// The literal it reads the address through returns it to this function, not
// to its caller.
func answeredWithADeferredClose() (int, error) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	read := func() string { return srv.URL }
	resp, err := http.Get(read())
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// getWithADeferredClose returns both results of a request to the server whose
// close it defers, a response and an error, in one call.
func getWithADeferredClose() (*http.Response, error) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	return http.Get(srv.URL)
}

// namedStatusWithADeferredClose gives what the server answered to its named
// result and returns it bare.
func namedStatusWithADeferredClose() (status int) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	status = statusOf(srv.URL)
	return
}

// returnedBeforeTheDefer hands out the address only on a return made before
// the close is deferred, so that close never runs for it.
func returnedBeforeTheDefer(early bool) string {
	srv := httptest.NewServer(http.NotFoundHandler())
	if early {
		return srv.URL
	}
	defer srv.Close()
	return fetch(srv.URL)
}

// deferredOnABranchThatReturns defers the close only on a branch that
// returns an answer, so the return after the branch hands out the address of
// a server whose close was never deferred.
func deferredOnABranchThatReturns(fail bool) string {
	srv := httptest.NewServer(http.NotFoundHandler())
	if fail {
		defer srv.Close()
		return fetch(srv.URL)
	}
	return srv.URL
}

// TestPlanted_ParallelSubtestsBeforeTheirClose_Clean starts parallel subtests
// that run before the close they meet: one before the test's own Cleanup,
// which waits for them, one started by a subtest that has returned before the
// later close, one that reads the address only before a literal it calls
// waits, one that closes its own server once it is done with it, and one
// started by a subtest whose own Cleanup closes the server.
func TestPlanted_ParallelSubtestsBeforeTheirClose_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	t.Run("parallel", func(t *testing.T) {
		t.Parallel()
		use(srv.URL)
	})
	other := httptest.NewServer(http.NotFoundHandler())
	url := other.URL
	t.Run("group", func(t *testing.T) {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			use(url)
		})
	})
	t.Run("reads before it waits", func(t *testing.T) {
		use(url)
		wait := func() { t.Parallel() }
		wait()
	})
	t.Run("closes its own", func(t *testing.T) {
		t.Parallel()
		own := httptest.NewServer(http.NotFoundHandler())
		use(own.URL)
		own.Close()
	})
	other.Close()
	t.Run("cleans up after its parallel subtest", func(t *testing.T) {
		group := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(group.Close)
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			use(group.URL)
		})
	})
}

// TestPlanted_ParallelSubtestAfterAStoppingBranch_Clean closes the server on a
// branch that stops the test before any parallel subtest is started, and
// otherwise when the test ends, after its parallel subtests. The test is
// parallel itself, and a literal that calls Parallel on the test it takes is
// not a subtest unless it is handed to Run.
func TestPlanted_ParallelSubtestAfterAStoppingBranch_Clean(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	if t.Failed() {
		srv.Close()
		t.FailNow()
	}
	t.Run("parallel", func(t *testing.T) {
		t.Parallel()
		use(srv.URL)
	})
	_ = func(t *testing.T) { t.Parallel() }
}

// TestPlanted_ReboundOnABranchAndUsedThere_Clean gives a saved address a new
// value on a branch and uses it only on that branch, gives another one a new
// value for the rest of the test, which a second value given on a branch does
// not cut short, and gives a third one a new value in the statement that
// opens an if, which runs whichever way the if goes.
func TestPlanted_ReboundOnABranchAndUsedThere_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if base != "" {
		url = base
		use(url)
	}
	other := httptest.NewServer(http.NotFoundHandler())
	saved := other.URL
	other.Close()
	saved = base
	if base != "" {
		saved = base + "/api/v4"
	}
	use(saved)
	third := httptest.NewServer(http.NotFoundHandler())
	addr := third.URL
	third.Close()
	if addr = base; addr != "" {
		use(addr)
	}
	use(addr)
}

// TestPlanted_MethodValueCloses_Clean closes through a Close held in a
// variable: called twice once the address has been used, given a new value
// before it is called, and deferred.
func TestPlanted_MethodValueCloses_Clean(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	stop := srv.Close
	use(srv.URL)
	stop()
	stop()
	other := httptest.NewServer(http.NotFoundHandler())
	halt := other.Close
	halt = func() {}
	halt()
	use(other.URL)
	deferred := httptest.NewServer(http.NotFoundHandler())
	end := deferred.Close
	defer end()
	use(deferred.URL)
}
