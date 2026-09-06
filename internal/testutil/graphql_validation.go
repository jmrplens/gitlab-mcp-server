package testutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// graphQLPath is the endpoint client-go rewrites every GraphQL request onto.
const graphQLPath = "/api/graphql"

// labelLimit keeps a document's first line from filling the failure message
// when the operation signature declares a dozen variables.
const labelLimit = 160

// validatingHandler wraps a mock so every GraphQL document a test sends is
// judged by the pinned GitLab schema before the mock answers it.
//
// This is where the economics change. A mock stands in for GitLab, and ours
// answered whatever it was asked, so a GraphQL test proved only that our
// handler agreed with our own fixture: four registered tools shipped documents
// no instance accepts with every test green. Wrapping the handler that
// [NewTestClient] serves makes every GraphQL test this repository already has
// into a document validator, at no cost to the tests themselves.
//
// It wraps the handler rather than the client's [http.RoundTripper] because
// gitlabclient.NewClient builds its transport from the configuration and
// exposes no seam to install one, while the handler observes the very same
// bytes one hop later.
//
// The request always proceeds. A refused document is reported and then
// answered as before, so the test's own assertions still run and still report,
// and one broken document produces one clear failure rather than a cascade of
// nil dereferences downstream.
func validatingHandler(tb testing.TB, handler http.Handler) http.Handler {
	tb.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validateGraphQLRequest(tb, r)
		handler.ServeHTTP(w, r)
	})
}

// graphQLReporter is the part of [testing.TB] the validation uses. It is an
// interface so the reporting paths are reachable from a test with a recorder,
// rather than only by failing the test that exercises them.
type graphQLReporter interface {
	Helper()
	Name() string
	Errorf(format string, args ...any)
}

// validateGraphQLRequest checks one request against the pinned schema, doing
// nothing for a request that carries no GraphQL document.
//
// It never calls Fatal or FailNow: this runs on the httptest server's
// goroutine, where those abort the wrong goroutine and leave the test hanging
// or passing (.github/instructions/test-goroutines.instructions.md).
func validateGraphQLRequest(reporter graphQLReporter, r *http.Request) {
	reporter.Helper()
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, graphQLPath) {
		return
	}
	if invalidGraphQLAllowed(reporter.Name()) {
		return
	}

	body, err := io.ReadAll(r.Body)
	// The mock still has to be able to read the body, whether or not this
	// managed to.
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		reporter.Errorf("could not read the GraphQL request body to validate it: %v", err)
		return
	}

	var request graphqlRequest
	if json.Unmarshal(body, &request) != nil || strings.TrimSpace(request.Query) == "" {
		// Not the JSON envelope: a query carrying an upload is multipart, and
		// a test posting anything else to this path is testing the transport
		// rather than a document. Neither is this gate's business.
		return
	}

	if err = graphqlschema.Validate(request.Query, request.Variables); err != nil {
		reporter.Errorf("GitLab would refuse this GraphQL request\n  operation: %s\n%s",
			documentLabel(request.Query), reasonLines(err))
	}
}

// reasonLines renders a validation failure one reason per line, indented under
// the operation it belongs to. A failure that is not a refusal is a failure to
// load the pinned schema at all, which has one line and no reasons.
func reasonLines(err error) string {
	var refusal *graphqlschema.ValidationError
	if !errors.As(err, &refusal) {
		return "  " + err.Error()
	}
	lines := make([]string, 0, len(refusal.Reasons))
	for _, reason := range refusal.Reasons {
		lines = append(lines, "  - "+reason)
	}
	return strings.Join(lines, "\n")
}

// documentLabel names a document by its first line, which for every document
// in this repository is the operation signature and identifies it uniquely.
func documentLabel(document string) string {
	for line := range strings.SplitSeq(document, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > labelLimit {
			return trimmed[:labelLimit] + "..."
		}
		return trimmed
	}
	return "(empty document)"
}

// allowedInvalid holds the tests that have declared they send a document the
// schema is expected to refuse, keyed by test name.
var (
	allowedInvalidMu sync.Mutex
	allowedInvalid   = map[string]bool{}
)

// AllowInvalidGraphQL exempts tb, and every subtest under it, from GraphQL
// document validation.
//
// It exists for the test that deliberately sends a malformed document, which
// is the only honest reason to want it: a document the pinned schema refuses
// is a document GitLab refuses, so silencing the gate anywhere else silences a
// real defect. Say in a comment at the call site which malformed document the
// test is sending and why it has to.
//
// The exemption lasts for tb's lifetime and is released on cleanup.
func AllowInvalidGraphQL(tb testing.TB) {
	tb.Helper()
	name := tb.Name()
	allowedInvalidMu.Lock()
	allowedInvalid[name] = true
	allowedInvalidMu.Unlock()
	tb.Cleanup(func() {
		allowedInvalidMu.Lock()
		delete(allowedInvalid, name)
		allowedInvalidMu.Unlock()
	})
}

// invalidGraphQLAllowed reports whether the named test, or any test it runs
// under, declared the exemption. A parent's exemption covers its subtests
// because the client is often built once in the parent and driven from them.
func invalidGraphQLAllowed(name string) bool {
	allowedInvalidMu.Lock()
	defer allowedInvalidMu.Unlock()
	for {
		if allowedInvalid[name] {
			return true
		}
		slash := strings.LastIndex(name, "/")
		if slash < 0 {
			return false
		}
		name = name[:slash]
	}
}
