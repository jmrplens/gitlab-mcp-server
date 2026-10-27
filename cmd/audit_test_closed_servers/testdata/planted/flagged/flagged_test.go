// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package flagged

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// use stands for anything that sends a request to an address.
func use(string) {}

// TestPlanted_URLReadAfterTheClose_Reported reads the URL off the server once
// it is closed.
func TestPlanted_URLReadAfterTheClose_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	use(srv.URL)
}

// TestPlanted_SavedURL_Reported keeps the URL in a variable first, the shape
// issue 1132 found most often.
func TestPlanted_SavedURL_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	use(url)
}

// TestPlanted_ServerFromAHelper_Reported gets its server from a helper and
// keeps only the host and port.
func TestPlanted_ServerFromAHelper_Reported(t *testing.T) {
	srv := NewServer()
	addr := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()
	use(addr)
}

// address is a string type of the test's own.
type address string

// TestPlanted_AddressBuiltInSteps_Reported derives the address it uses from
// another derived one, through concatenation, formatting and a conversion.
func TestPlanted_AddressBuiltInSteps_Reported(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	var base = srv.URL + "/api/v4"
	endpoint := address(fmt.Sprintf("%s/version", base))
	srv.Close()
	use(string(endpoint))
}

// TestPlanted_SavedInAPairedAssignment_Reported saves the URL as the second
// of two names one assignment gives values to.
func TestPlanted_SavedInAPairedAssignment_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	kind, url := "plain", srv.URL
	srv.Close()
	use(kind + url)
}

// TestPlanted_ClientBuiltOnTheLiveURL_Reported builds a client on the live
// server and closes it for one case only, so the client goes on to dial a
// freed port. The constructor returns an error beside the client, and the
// error is not what carries the address on.
func TestPlanted_ClientBuiltOnTheLiveURL_Reported(t *testing.T) {
	for _, unreachable := range []bool{false, true} {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		client, err := Dial(srv.URL)
		if unreachable {
			srv.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		use(client.Get())
	}
}

// TestPlanted_OneNameReboundAnotherUsed_Reported gives one saved address a
// new value after the close and goes on to use the other.
func TestPlanted_OneNameReboundAnotherUsed_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	first := srv.URL
	second := NewClient(srv.URL)
	srv.Close()
	first = "http://example.com"
	use(first)
	use(second.Get())
}

// TestPlanted_FunctionBoundToTheURL_Reported keeps a function that dials the
// URL and calls it after the close.
func TestPlanted_FunctionBoundToTheURL_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	ping := func() { use(srv.URL) }
	srv.Close()
	ping()
}

// env holds its server in a field.
type env struct{ server *httptest.Server }

// TestPlanted_ServerInAField_Reported closes the server through the field
// that holds it.
func TestPlanted_ServerInAField_Reported(t *testing.T) {
	e := env{server: httptest.NewServer(http.NotFoundHandler())}
	e.server.Close()
	use(e.server.URL)
}

// fake embeds its server, so Close and URL are promoted from it.
type fake struct{ *httptest.Server }

// TestPlanted_EmbeddedServer_Reported closes a server through a type that
// embeds it.
func TestPlanted_EmbeddedServer_Reported(t *testing.T) {
	f := fake{httptest.NewServer(http.NotFoundHandler())}
	f.Close()
	use(f.URL)
}

// TestPlanted_ClosedInASubtest_Reported closes the server in a subtest and
// uses the address after it.
func TestPlanted_ClosedInASubtest_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Run("close", func(*testing.T) { srv.Close() })
	use(srv.URL)
}

// note stands for a call of the test's own that returns.
func note(error) {}

// TestPlanted_ClosedOnABranchThatGoesOn_Reported closes a server on each of
// several branches whose last statement does not end the test, so the uses
// after them can follow the close: a testing method that logs, a builtin
// other than panic, a receive, an assignment, a method of the predeclared
// error, and a function of the test's own.
func TestPlanted_ClosedOnABranchThatGoesOn_Reported(t *testing.T) {
	logged := httptest.NewServer(http.NotFoundHandler())
	builtin := httptest.NewServer(http.NotFoundHandler())
	received := httptest.NewServer(http.NotFoundHandler())
	assigned := httptest.NewServer(http.NotFoundHandler())
	described := httptest.NewServer(http.NotFoundHandler())
	noted := httptest.NewServer(http.NotFoundHandler())
	done := make(chan struct{}, 1)
	done <- struct{}{}
	var err error
	if err == nil {
		logged.Close()
		t.Log("closed")
	}
	if err == nil {
		builtin.Close()
		close(done)
	}
	if err == nil {
		received.Close()
		<-done
	}
	if err == nil {
		assigned.Close()
		err = nil
	}
	if err == nil {
		described.Close()
		err.Error()
	}
	if err == nil {
		noted.Close()
		note(err)
	}
	use(logged.URL)
	use(builtin.URL)
	use(received.URL)
	use(assigned.URL)
	use(described.URL)
	use(noted.URL)
}

// TestPlanted_AddressFromAnInvokedLiteral_Reported takes the address from a
// function literal it calls in place.
func TestPlanted_AddressFromAnInvokedLiteral_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := func() string { return srv.URL }()
	srv.Close()
	use(addr)
}

// TestPlanted_DeferredInASubtest_Reported defers the close inside a subtest,
// so the server is closed when the subtest returns and the use after it meets
// a freed port. The second server is closed by a Cleanup the subtest
// registers, which runs when the subtest ends.
func TestPlanted_DeferredInASubtest_Reported(t *testing.T) {
	deferred := httptest.NewServer(http.NotFoundHandler())
	cleaned := httptest.NewServer(http.NotFoundHandler())
	t.Run("uses and closes", func(t *testing.T) {
		defer deferred.Close()
		t.Cleanup(func() { cleaned.Close() })
		use(deferred.URL)
		use(cleaned.URL)
	})
	use(deferred.URL)
	use(cleaned.URL)
}

// registry has a Cleanup of its own, which runs what it is handed at once.
type registry struct{}

// Cleanup runs f now; it is not a test's.
func (registry) Cleanup(f func()) { f() }

// hooks holds a Cleanup as a field rather than a method.
type hooks struct{ Cleanup func(func()) }

// TestPlanted_CleanupThatIsNotATests_Reported hands the close to a Cleanup
// that is not a test's, a method and a field, so neither waits for the test
// to end.
func TestPlanted_CleanupThatIsNotATests_Reported(t *testing.T) {
	byMethod := httptest.NewServer(http.NotFoundHandler())
	byField := httptest.NewServer(http.NotFoundHandler())
	registry{}.Cleanup(func() { byMethod.Close() })
	h := hooks{Cleanup: func(f func()) { f() }}
	h.Cleanup(func() { byField.Close() })
	use(byMethod.URL)
	use(byField.URL)
}

// TestPlanted_ClosedBeforeAFunctionValue_Reported closes a server on a branch
// whose last statement calls a function held in a variable, which returns.
func TestPlanted_ClosedBeforeAFunctionValue_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	stop := func() {}
	var err error
	if err == nil {
		srv.Close()
		stop()
	}
	use(srv.URL)
}

// TestPlanted_ReassignedFromItself_Reported trims the saved address into the
// same name after the close: the new value is read from the old one, so the
// assignment that ends the name's tracking is itself a use.
func TestPlanted_ReassignedFromItself_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	addr = strings.TrimPrefix(addr, "http://")
	use(addr)
}

// TestPlanted_CompoundAssignment_Reported extends the saved address in place
// after the close, which reads the old value and keeps it.
func TestPlanted_CompoundAssignment_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	base += "/api/v4"
	use(base)
}

// TestPlanted_ClosedOnACaseThatGoesOn_Reported closes a server in a case of a
// switch, and another in a case of a select, neither of which ends the test,
// so the uses after them can follow the close.
func TestPlanted_ClosedOnACaseThatGoesOn_Reported(t *testing.T) {
	switched := httptest.NewServer(http.NotFoundHandler())
	selected := httptest.NewServer(http.NotFoundHandler())
	ready := make(chan struct{}, 1)
	ready <- struct{}{}
	switch {
	case len(ready) == 1:
		switched.Close()
		t.Log("closed")
	}
	select {
	case <-ready:
		selected.Close()
		t.Log("closed")
	default:
	}
	use(switched.URL)
	use(selected.URL)
}

// TestPlanted_HelperHandsOutAClosedURL_Clean is not itself a finding: the
// helpers it calls are, where they close the server.
func TestPlanted_HelperHandsOutAClosedURL_Clean(t *testing.T) {
	use(closedURL())
	use(closedLater())
}

// TestPlanted_ClosedThroughACopy_Reported saves the URL, copies the server
// into another name and closes it through the copy.
func TestPlanted_ClosedThroughACopy_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	other := srv
	other.Close()
	use(url)
}

// holder is given its server in a composite literal.
type holder struct{ server *httptest.Server }

// TestPlanted_ClosedThroughAHolder_Reported saves the URL, hands the server to
// a struct and closes it through the struct's field.
func TestPlanted_ClosedThroughAHolder_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	h := holder{server: srv}
	h.server.Close()
	use(url)
}

// TestPlanted_ServerAndAddressFromAHelper_Reported takes the server and its
// address from the struct a helper builds, the shape of a helper that returns
// both, and closes the server through its field.
func TestPlanted_ServerAndAddressFromAHelper_Reported(t *testing.T) {
	f := newFakeGitLab()
	f.srv.Close()
	use(f.url)
}

// config is bound to an address once a field of it is set.
type config struct{ base string }

// dialConfig stands for a request sent to the address a config names.
func dialConfig(config) {}

// TestPlanted_AddressStoredInAField_Reported stores each address in a field
// of a struct, one held as a value and one through a pointer, and uses the
// whole struct after the close.
func TestPlanted_AddressStoredInAField_Reported(t *testing.T) {
	byValue := httptest.NewServer(http.NotFoundHandler())
	byPointer := httptest.NewServer(http.NotFoundHandler())
	var cfg config
	cfg.base = byValue.URL
	ptr := &config{}
	ptr.base = byPointer.URL
	byValue.Close()
	byPointer.Close()
	dialConfig(cfg)
	dialConfig(*ptr)
}

// TestPlanted_RangeVariable_Reported ranges over a list holding the address
// and closes the server before the variable is used.
func TestPlanted_RangeVariable_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	for _, u := range []string{srv.URL} {
		srv.Close()
		use(u)
	}
}

// TestPlanted_AddressAsBytesAndInARequest_Reported keeps one address as
// bytes, which are text like a string, and builds a request on the other, and
// uses both after the close.
func TestPlanted_AddressAsBytesAndInARequest_Reported(t *testing.T) {
	asBytes := httptest.NewServer(http.NotFoundHandler())
	asRequest := httptest.NewServer(http.NotFoundHandler())
	raw := []byte(asBytes.URL)
	req, err := http.NewRequest(http.MethodGet, asRequest.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	asBytes.Close()
	asRequest.Close()
	use(string(raw))
	_, _ = http.DefaultClient.Do(req)
}

// TestPlanted_CleanupInASubtest_Reported registers each close as a method
// value on its subtest's own test, under the usual name and under another, so
// the server is closed when the subtest ends and the use after it meets a
// freed port.
func TestPlanted_CleanupInASubtest_Reported(t *testing.T) {
	named := httptest.NewServer(http.NotFoundHandler())
	renamed := httptest.NewServer(http.NotFoundHandler())
	t.Run("registers on its own test", func(t *testing.T) {
		t.Cleanup(named.Close)
	})
	t.Run("under another name", func(sub *testing.T) {
		sub.Cleanup(renamed.Close)
	})
	use(named.URL)
	use(renamed.URL)
}

// TestPlanted_ParallelInsideASubtest_Reported registers the close in a
// parallel subtest of a subtest. A parallel subtest goes on once the body of
// the test that started it has returned, so the server is closed when the
// outer subtest ends.
func TestPlanted_ParallelInsideASubtest_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Run("group", func(t *testing.T) {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			t.Cleanup(srv.Close)
		})
	})
	use(srv.URL)
}

// TestPlanted_ServerWrappedInAnEmbedding_Reported wraps the server it started
// in a type that embeds it, and reads the address the wrapper promotes after
// the close.
func TestPlanted_ServerWrappedInAnEmbedding_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	wrapped := fake{srv}
	srv.Close()
	use(wrapped.URL)
}

// TestPlanted_LiteralDefersItsClose_Reported calls a function literal that
// defers the close of the server it starts, so the address it returns is a
// closed server's.
func TestPlanted_LiteralDefersItsClose_Reported(t *testing.T) {
	closedAddress := func() string {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		return srv.URL
	}
	use(closedAddress())
}

// TestPlanted_DeferredCloseBeforeAParallelSubtest_Reported defers the close
// and starts a parallel subtest that uses the address. The subtest goes on
// only once the body of the test has returned, which is when the deferred
// close has run.
func TestPlanted_DeferredCloseBeforeAParallelSubtest_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	t.Run("parallel", func(t *testing.T) {
		t.Parallel()
		use(srv.URL)
	})
}

// TestPlanted_ClosedAfterAParallelSubtest_Reported closes the server after
// starting a parallel subtest. The subtest copies the address before it waits
// for the body of the test to return, and dials the copy once it has.
func TestPlanted_ClosedAfterAParallelSubtest_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	t.Run("parallel", func(t *testing.T) {
		saved := url
		t.Parallel()
		use(saved)
	})
	srv.Close()
}

// TestPlanted_SubtestDefersBeforeItsParallelSubtest_Reported is the first
// shape one level down: a subtest defers the close of its server and starts a
// parallel subtest of its own that uses it.
func TestPlanted_SubtestDefersBeforeItsParallelSubtest_Reported(t *testing.T) {
	t.Run("group", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			use(srv.URL)
		})
	})
}

// TestPlanted_ClosedInASubtestAfterAParallelOne_Reported closes the server in
// the Cleanup of a later subtest, which runs when that subtest ends, while the
// body of the test is still running and before the parallel subtest started
// earlier goes on to use the address.
func TestPlanted_ClosedInASubtestAfterAParallelOne_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Run("parallel", func(t *testing.T) {
		t.Parallel()
		use(srv.URL)
	})
	t.Run("closes", func(t *testing.T) {
		t.Cleanup(srv.Close)
	})
}

// TestPlanted_ClosedOnAStoppingBranchAfterAParallelSubtest_Reported closes
// the server on a branch that stops the test. A parallel subtest started
// before the branch still runs once the test has stopped, and uses the
// address; so would one the branch starts after the close.
func TestPlanted_ClosedOnAStoppingBranchAfterAParallelSubtest_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Run("parallel", func(t *testing.T) {
		t.Parallel()
		use(srv.URL)
	})
	if t.Failed() {
		srv.Close()
		t.Run("started by the branch", func(t *testing.T) {
			t.Parallel()
		})
		t.FailNow()
	}
}

// TestPlanted_ReboundOnABranch_Reported gives the saved address a new value
// on a branch only one case takes, so the case that does not take it extends
// and uses the closed server's address on the very next line.
func TestPlanted_ReboundOnABranch_Reported(t *testing.T) {
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	for _, live := range []bool{false, true} {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		if live {
			url = other.URL
		}
		url += "/version"
		use(url)
	}
}

// TestPlanted_ReboundOnTheOtherBranch_Reported gives the saved address a new
// value on one arm of an if and uses it on the other, where it is still the
// closed server's.
func TestPlanted_ReboundOnTheOtherBranch_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if t.Failed() {
		url = "http://example.com"
	} else {
		use(url)
	}
}

// TestPlanted_ReboundByARangeThatMayNotRun_Reported gives the saved address a
// new value from a range over a list that may be empty, which leaves the
// closed server's address in the name after the loop.
func TestPlanted_ReboundByARangeThatMayNotRun_Reported(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	var others []string
	for _, url = range others {
		use(url)
	}
	use(url)
}

// TestPlanted_ClosedThroughAMethodValue_Reported closes one server through
// its Close held in a variable and called in place, and another through one
// declared in a subtest and registered on its Cleanup, so the uses after them
// meet a freed port.
func TestPlanted_ClosedThroughAMethodValue_Reported(t *testing.T) {
	called := httptest.NewServer(http.NotFoundHandler())
	registered := httptest.NewServer(http.NotFoundHandler())
	url := called.URL
	stop := called.Close
	stop()
	t.Run("registers", func(t *testing.T) {
		var cleanup = registered.Close
		t.Cleanup(cleanup)
	})
	use(url)
	use(registered.URL)
}
