//go:build httpe2e

// redirect_test.go pins what happens to this deployment's GitLab credential
// when the instance answers a tool call with a redirect somewhere else.
//
// Following the redirect is deliberate and cannot be given up: GitLab answers
// artifact, trace and package reads with a 302 to object storage whenever
// object storage is configured, which is GitLab.com and most self-managed
// instances, and several shipped read-only actions exist only because that hop
// is followed. Those destinations authenticate through the query string, so the
// credential headers cost nothing to withhold.
//
// net/http withholds Authorization, Cookie and a few others across a change of
// host, and nothing else. It never touches PRIVATE-TOKEN, which is the only
// credential stdio mode has and legacy HTTP mode's default; and because it
// compares hostnames alone, it keeps even Authorization across an https-to-http
// downgrade. Both gaps are closed by this project's own redirect policy, which
// is why the case below is a downgrade on one host: the standard library would
// carry the credential through it.
//
// The policy has unit tests. What they cannot show is that the policy is
// installed on the client a tool call actually uses, that it survives the
// assembled request path, and that the line it writes reaches the operator's
// log without dragging a presigned URL along with it. Those need a process.
package httpe2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// redirectPresignedQuery is the query string the instance attaches to the
// destination it names.
//
// It stands in for the presigned parameters real object storage uses, which
// are themselves a working credential: the whole point of recording the hop as
// a host and a scheme rather than as a URL is that logging the URL would write
// one credential to stderr while reporting that another was withheld.
const redirectPresignedQuery = "X-Amz-Signature=thisvalueisitselfacredential"

// redirectDestination records what arrived at the host the instance redirected
// to.
type redirectDestination struct {
	url string

	mu       sync.Mutex
	arrived  bool
	carried  []string
	rawQuery string
}

// startRedirectDestination serves the object-storage stand-in: it records the
// credential headers it received and answers with the project, so the tool
// call completes and the assertions are about the credential rather than about
// a failure.
func startRedirectDestination(t *testing.T) *redirectDestination {
	t.Helper()

	dest := &redirectDestination{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dest.mu.Lock()
		dest.arrived = true
		dest.rawQuery = r.URL.RawQuery
		for _, name := range []string{"PRIVATE-TOKEN", "Authorization", "Sudo", "Job-Token"} {
			if r.Header.Get(name) != "" {
				dest.carried = append(dest.carried, name)
			}
		}
		dest.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"name":"widgets","path_with_namespace":"acme/widgets"}`))
	}))
	t.Cleanup(srv.Close)
	dest.url = srv.URL
	return dest
}

// observed returns what reached the destination.
func (d *redirectDestination) observed() (arrived bool, carried []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.arrived, append([]string(nil), d.carried...)
}

// startRedirectingGitLab serves an https instance whose project endpoint
// answers a 302 to destination over plain http.
//
// https for the instance and http for the destination is what makes this a
// downgrade rather than a change of host, and a downgrade on the same host is
// precisely the hop net/http considers safe. Choosing it means a passing case
// cannot be explained by the standard library doing the work.
func startRedirectingGitLab(t *testing.T, destination string) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":7,"username":"someone","name":"Some One","state":"active"}`))
	})
	mux.HandleFunc("/api/v4/projects/42", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination+"/storage/project-42?"+redirectPresignedQuery, http.StatusFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// The certificate httptest generates is self-signed, which is what
	// --skip-tls-verify exists for; the server under test is given that flag,
	// so nothing here depends on the certificate being trusted.
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestRedirect_OffTheInstanceCarriesNoCredentialAndIsRecorded drives a tool
// call whose instance answers with a redirect that leaves the credential scope,
// and asserts three things about the hop that follows.
//
// The credential must not arrive: that is the fix, and the destination is a
// host the operator never configured. The call must still succeed: refusing
// cross-scope redirects outright would be simpler and would break every
// artifact, trace and package read on an instance with object storage. And the
// server must say it withheld something, because an operator looking at a 401
// from object storage otherwise cannot tell a credential this server
// deliberately dropped from a credential that was never valid, and those two
// call for opposite responses.
func TestRedirect_OffTheInstanceCarriesNoCredentialAndIsRecorded(t *testing.T) {
	destination := startRedirectDestination(t)
	instance := startRedirectingGitLab(t, destination.url)
	srv := startServer(t, nil, "--gitlab-url="+instance, "--skip-tls-verify")

	body, isError := toolResultsText(t, toolResultsCall(t, srv, "project.get", `{"project_id":"42"}`))
	if isError {
		t.Fatalf("the redirect was not followed, so no hop was made to test: %s", toolResultsTruncate(body))
	}

	arrived, carried := destination.observed()
	if !arrived {
		t.Fatal("nothing reached the redirect destination, so the assertions below have nothing to prove")
	}
	if len(carried) > 0 {
		t.Errorf("the redirect carried %v to a host the operator never configured", carried)
	}

	logs := awaitLog(t, srv, "dropped credential headers on redirect")
	if logs == "" {
		t.Fatalf("the server withheld the credential and never said so:\n%s", truncate(srv.logs()))
	}
	if !strings.Contains(logs, "redirect downgrades https to http") {
		t.Errorf("the log does not say which rule withheld the credential: %s", truncate(logs))
	}
	if !strings.Contains(logs, "PRIVATE-TOKEN") {
		t.Errorf("the log does not name the header it withheld, which is the detail that makes it actionable: %s",
			truncate(logs))
	}
	// The destination is recorded as a host and a scheme. Recording the URL
	// would put the presigned parameters, which are themselves a credential,
	// into the log that reports a credential being withheld.
	if strings.Contains(logs, redirectPresignedQuery) {
		t.Errorf("the log carries the destination's query string, which authenticates the request: %s", truncate(logs))
	}
}
