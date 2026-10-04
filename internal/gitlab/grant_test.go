// grant_test.go holds the reads a fine-grained token's authority is judged
// from: the grant, bounded where it is read, and the version beside it.
package gitlab

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// grantBody is a fine-grained token's answer to GET
// /personal_access_tokens/:id, holding one project scope.
const grantBody = `{"id":9,"scopes":["granular"],"granular":true,` +
	`"granular_scopes":[{"access":"selected_memberships","permissions":["project_read"],"project_id":7}]}`

// grantServer answers the grant read with status and body, the version read
// with version (a refusal when version is "refused"), and counts both.
type grantServer struct {
	url      string
	grants   atomic.Int64
	versions atomic.Int64
}

// newGrantServer starts the stand-in for one test.
func newGrantServer(t *testing.T, status int, body, version string) *grantServer {
	t.Helper()
	stand := &grantServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/personal_access_tokens/9", func(w http.ResponseWriter, _ *http.Request) {
		stand.grants.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	mux.HandleFunc("GET /api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		stand.versions.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if version == "refused" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":"insufficient_granular_scope","error_description":"Access denied: This operation `+
				`requires a fine-grained personal access token with the following instance permissions: [read_metadata]"}`)
			return
		}
		if version == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, `{"version":"`+version+`"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	stand.url = srv.URL
	return stand
}

// grantClient is a client with retries off pointed at the stand-in.
func grantClient(t *testing.T, url string) *Client {
	t.Helper()
	client, err := NewClientWithTokenRetries(url, testValidToken, false, true)
	if err != nil {
		t.Fatalf("NewClientWithTokenRetries: %v", err)
	}
	return client
}

// TestReadGrant_ReadsTheAnswerOrSaysWhyNot verifies each answer the grant read
// can meet: a grant decoded, a refusal of the token's own description read as
// unreadable, a token that says it is not fine-grained or holds a scope that
// cannot be placed read as of unknown shape, a body decoded by client-go
// whose captured copy does not decode read the same way, and a status that is
// no verdict on the read reported as no answer, with an error.
func TestReadGrant_ReadsTheAnswerOrSaysWhyNot(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		reason   finegrained.FallbackReason
		answered bool
		scopes   int
	}{
		{name: "a grant", status: http.StatusOK, body: grantBody, reason: finegrained.FallbackNone, answered: true, scopes: 1},
		{name: "refused", status: http.StatusForbidden, body: `{"message":"403 Forbidden"}`, reason: finegrained.FallbackGrantUnreadable, answered: true},
		{name: "not fine-grained", status: http.StatusOK, body: `{"id":9,"granular":false}`, reason: finegrained.FallbackGrantShape, answered: true},
		{name: "trailing bytes", status: http.StatusOK, body: grantBody + ` trailing`, reason: finegrained.FallbackGrantShape, answered: true},
		{name: "no scopes", status: http.StatusOK, body: `{"id":9,"granular":true}`, reason: finegrained.FallbackGrantUnreadable, answered: true},
		{name: "server error", status: http.StatusInternalServerError, body: `{}`, reason: finegrained.FallbackNone},
		{name: "not found", status: http.StatusNotFound, body: `{}`, reason: finegrained.FallbackNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stand := newGrantServer(t, tc.status, tc.body, "19.4.1-ee")
			grant, reason, err := ReadGrant(t.Context(), grantClient(t, stand.url).GL(), 9)
			if (err == nil) != tc.answered || reason != tc.reason || len(grant.Scopes) != tc.scopes {
				t.Errorf("ReadGrant = %d scopes, %q, %v; want %d scopes, %q, answered %v", len(grant.Scopes), reason, err, tc.scopes, tc.reason, tc.answered)
			}
			if err != nil && strings.Contains(err.Error(), "personal_access_tokens/9") && !strings.Contains(err.Error(), "read the token's grant") {
				t.Errorf("error %v does not say what failed", err)
			}
		})
	}
}

// TestReadGrant_ABodyPastTheBound_IsReadAsTooLarge verifies a grant answer
// larger than GrantMaxBytes is read as too large, with no error, through the
// client's real transport chain.
func TestReadGrant_ABodyPastTheBound_IsReadAsTooLarge(t *testing.T) {
	huge := `{"id":9,"granular":true,"granular_scopes":[{"access":"user","permissions":["` +
		strings.Repeat("a", int(GrantMaxBytes)) + `"]}]}`
	stand := newGrantServer(t, http.StatusOK, huge, "19.4.1-ee")
	_, reason, err := ReadGrant(t.Context(), grantClient(t, stand.url).GL(), 9)
	if err != nil || reason != finegrained.FallbackGrantTooLarge {
		t.Errorf("ReadGrant = %q, %v; want too large and no error", reason, err)
	}
}

// countingBody is a body of size bytes that counts what was read of it.
type countingBody struct {
	left int64
	read *atomic.Int64
}

// Read hands out zeros until the body's size is reached.
func (b *countingBody) Read(p []byte) (int, error) {
	if b.left <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), b.left)
	clear(p[:n])
	b.left -= n
	b.read.Add(n)
	return int(n), nil
}

// Close does nothing.
func (b *countingBody) Close() error { return nil }

// countingTransport answers every request with a 200 whose body is a
// countingBody of size bytes.
type countingTransport struct {
	size int64
	read atomic.Int64
}

// RoundTrip answers the request with the counting body.
func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{}, Request: req,
		Body: &countingBody{left: t.size, read: &t.read},
	}, nil
}

// TestReadGrant_A64MiBBody_IsReadToTheBoundAndNoFurther verifies the bound
// applies where the body is read: a 64 MiB answer, the client's own ceiling,
// is read through the capture and the response ceiling to at most
// GrantMaxBytes+1 bytes and abandoned with the ceiling's error, rather than
// buffered whole and measured afterwards.
func TestReadGrant_A64MiBBody_IsReadToTheBoundAndNoFurther(t *testing.T) {
	base := &countingTransport{size: DefaultMaxResponseBytes}
	client := &Client{}
	client.SetMaxResponseBytes(DefaultMaxResponseBytes)
	transport := &captureTransport{base: &responseLimitTransport{base: base, client: client}}
	ctx, capture := WithResponseCapture(WithResponseLimit(t.Context(), GrantMaxBytes))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.example.com/api/v4/personal_access_tokens/9", http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := transport.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("RoundTrip error = %v, want %v", err, ErrResponseTooLarge)
	}
	if got := base.read.Load(); got > GrantMaxBytes+1 {
		t.Errorf("read %d bytes of the body, want at most %d", got, GrantMaxBytes+1)
	}
	if decodeErr := capture.Decode(&struct{}{}); !errors.Is(decodeErr, ErrNoResponseCaptured) {
		t.Errorf("Decode = %v, want nothing captured from a body past the bound", decodeErr)
	}
}

// TestReadVersion_AnswersAndNonAnswers verifies what the version read reports:
// a version the instance named, readable or not, and GitLab's refusal of
// Metadata: Read are answers; a server error is none.
func TestReadVersion_AnswersAndNonAnswers(t *testing.T) {
	cases := []struct {
		name, served, want string
		answered           bool
	}{
		{"a version", "19.4.1-ee", "19.4.1-ee", true},
		{"not a version", "nineteen", "", true},
		{"refused", "refused", "", true},
		{"no answer", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stand := newGrantServer(t, http.StatusOK, grantBody, tc.served)
			got, answered := grantClient(t, stand.url).ReadVersion(t.Context())
			if got != tc.want || answered != tc.answered {
				t.Errorf("ReadVersion = %q, %v; want %q, %v", got, answered, tc.want, tc.answered)
			}
		})
	}
}

// TestClient_ReadFineGrained_AVersionThatLeavesNothingToJudgeAt_SaysWhyAndSkipsTheGrant
// verifies the reading when the version read leaves no version to judge the
// grant at: an instance that did not answer is read as unanswered, one that
// refused Metadata: Read or named no readable version as unreadable, and a
// version already read and empty as unreadable, each without asking for the
// grant, since no grant is evaluated without a version.
func TestClient_ReadFineGrained_AVersionThatLeavesNothingToJudgeAt_SaysWhyAndSkipsTheGrant(t *testing.T) {
	readable := TokenFacts{Scopes: []string{ScopeGranular}, ID: 9, FineGrained: true, GrantReadable: true}
	cases := []struct {
		name, served string
		want         finegrained.FallbackReason
	}{
		{"no answer", "", finegrained.FallbackVersionUnanswered},
		{"refused", "refused", finegrained.FallbackVersionUnreadable},
		{"not a version", "nineteen", finegrained.FallbackVersionUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stand := newGrantServer(t, http.StatusOK, grantBody, tc.served)
			got := grantClient(t, stand.url).ReadFineGrained(t.Context(), readable)
			if got.Fallback != tc.want || got.Version != "" || len(got.Grant.Scopes) != 0 || stand.grants.Load() != 0 || stand.versions.Load() != 1 {
				t.Errorf("reading = %+v after %d grant and %d version reads; want %q, no grant read and one version read",
					got, stand.grants.Load(), stand.versions.Load(), tc.want)
			}
		})
	}
	t.Run("an empty version already read", func(t *testing.T) {
		stand := newGrantServer(t, http.StatusOK, grantBody, "19.4.1-ee")
		got := grantClient(t, stand.url).ReadFineGrainedAt(t.Context(), readable, "")
		if got.Fallback != finegrained.FallbackVersionUnreadable || stand.grants.Load()+stand.versions.Load() != 0 {
			t.Errorf("reading = %+v after %d requests; want unreadable and none", got, stand.grants.Load()+stand.versions.Load())
		}
	})
}

// TestClient_ReadFineGrained_ReadsWhatTheTokenMayRead verifies the reading
// handed to the judgement: a token that may not read its grant is asked
// nothing; one that may is asked the version and the grant, or only the grant
// when the version was already read; and a grant read nobody answered is
// read as unanswered, a reason a later read lifts.
func TestClient_ReadFineGrained_ReadsWhatTheTokenMayRead(t *testing.T) {
	readable := TokenFacts{Scopes: []string{ScopeGranular}, ID: 9, FineGrained: true, GrantReadable: true}

	stand := newGrantServer(t, http.StatusOK, grantBody, "19.4.1-ee")
	client := grantClient(t, stand.url)
	unreadable := client.ReadFineGrained(t.Context(), TokenFacts{Scopes: []string{ScopeGranular}, FineGrained: true})
	if unreadable.Fallback != finegrained.FallbackGrantUnreadable || stand.grants.Load()+stand.versions.Load() != 0 {
		t.Errorf("unreadable reading = %+v after %d requests, want unreadable and none", unreadable, stand.grants.Load()+stand.versions.Load())
	}
	if again := client.ReadFineGrainedAt(t.Context(), TokenFacts{FineGrained: true}, "19.4.1"); again.Fallback != finegrained.FallbackGrantUnreadable {
		t.Errorf("unreadable reading at a version = %+v", again)
	}
	read := client.ReadFineGrained(t.Context(), readable)
	if read.Fallback != finegrained.FallbackNone || read.Version != "19.4.1-ee" || len(read.Grant.Scopes) != 1 ||
		stand.grants.Load() != 1 || stand.versions.Load() != 1 {
		t.Errorf("reading = %+v after %d grant and %d version reads", read, stand.grants.Load(), stand.versions.Load())
	}
	at := client.ReadFineGrainedAt(t.Context(), readable, "19.4.0")
	if at.Version != "19.4.0" || stand.versions.Load() != 1 {
		t.Errorf("reading at a known version = %+v after %d version reads, want no new one", at, stand.versions.Load())
	}

	failing := newGrantServer(t, http.StatusBadGateway, `{}`, "19.4.1-ee")
	if got := grantClient(t, failing.url).ReadFineGrained(t.Context(), readable); got.Fallback != finegrained.FallbackGrantUnanswered {
		t.Errorf("reading with no answer = %+v, want the grant unanswered", got)
	}
}

// refreshTable is a table at 19.4 with one action a project read reaches.
func refreshTable() *finegrained.Table {
	return &finegrained.Table{
		Version: "19.4.1-ee", Bucket: "19.4",
		Permissions: []string{"read_project"}, Display: []string{"Project: Read"},
		Assignables: []finegrained.Assignable{{Name: "project_read", Permissions: []uint16{0}, Boundaries: finegrained.BoundaryProject, Grantable: true}},
		Groups:      []finegrained.Group{{Perms: []uint16{0}, Any: finegrained.BoundaryProject}},
		Operations:  []finegrained.Operation{{Name: "GET /projects/:id", Groups: []uint32{0}}},
		Actions:     []finegrained.Requirement{{ID: "project.get", Paths: [][]uint32{{0}}}},
	}
}

// refreshReadable is a fine-grained token that may read its grant, as the
// self endpoint describes it.
var refreshReadable = TokenFacts{Scopes: []string{ScopeGranular}, ID: 9, FineGrained: true, GrantReadable: true}

// refreshedClient is a client pointed at a stand-in answering the grant read
// with status and body and the version read with version, carrying current.
func refreshedClient(t *testing.T, status int, body, version string, current *finegrained.Authority) *Client {
	t.Helper()
	client := grantClient(t, newGrantServer(t, status, body, version).url)
	client.SetAuthority(current)
	return client
}

// TestClient_RefreshAuthority_ReplacesOnlyOnReadsThatAnswered verifies the
// re-read a revalidation makes: nothing asked of a token that may not read its
// grant; the authority replaced when the reads answered at the same release,
// with nothing returned since no verdict moved; and kept, with the reason,
// when the version was not answered or not readable or the grant read failed
// at the same release.
func TestClient_RefreshAuthority_ReplacesOnlyOnReadsThatAnswered(t *testing.T) {
	table := refreshTable()
	current := finegrained.Judge(table, finegrained.Reading{Grant: finegrained.Grant{}, Version: "19.4.1-ee"})

	t.Run("a token that may not read its grant", func(t *testing.T) {
		client := refreshedClient(t, http.StatusOK, grantBody, "19.4.1-ee", current)
		if moved, reason := client.RefreshAuthority(t.Context(), TokenFacts{FineGrained: true}, table); moved != nil || reason != "" || client.Authority() != current {
			t.Errorf("RefreshAuthority = %v, %q", moved, reason)
		}
	})
	t.Run("reads that answered at the same release", func(t *testing.T) {
		client := refreshedClient(t, http.StatusOK, grantBody, "19.4.1-ee", current)
		moved, reason := client.RefreshAuthority(t.Context(), refreshReadable, table)
		if moved != nil || reason != "" || client.Authority() == current || !client.Authority().Decide("project.get").Listed {
			t.Errorf("RefreshAuthority = %v, %q; authority lists project.get %v", moved, reason, client.Authority().Decide("project.get").Listed)
		}
	})
	for _, tc := range []struct {
		name, served string
		status       int
		want         finegrained.FallbackReason
	}{
		{"no answer to the version", "", http.StatusOK, finegrained.FallbackVersionUnanswered},
		{"a refused version", "refused", http.StatusOK, finegrained.FallbackVersionUnreadable},
		{"a grant read that failed", "19.4.1-ee", http.StatusServiceUnavailable, finegrained.FallbackGrantUnanswered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := refreshedClient(t, tc.status, grantBody, tc.served, current)
			if moved, reason := client.RefreshAuthority(t.Context(), refreshReadable, table); moved != nil || reason != string(tc.want) || client.Authority() != current {
				t.Errorf("RefreshAuthority = %v, %q; want it kept with %q", moved, reason, tc.want)
			}
		})
	}
}

// TestClient_RefreshAuthority_AnUpgradePastTheRecord_ReturnsTheMoveOnce
// verifies a re-read that moves the token to another verdict, an instance
// upgraded to a release the table does not record, returns the phase A
// authority it moved the token to, and that the next re-read at that release
// replaces it without returning it again, so its caller logs the move once.
func TestClient_RefreshAuthority_AnUpgradePastTheRecord_ReturnsTheMoveOnce(t *testing.T) {
	table := refreshTable()
	current := finegrained.Judge(table, finegrained.Reading{Grant: finegrained.Grant{}, Version: "19.4.1-ee"})
	client := refreshedClient(t, http.StatusOK, grantBody, "19.6.0-ee", current)
	moved, reason := client.RefreshAuthority(t.Context(), refreshReadable, table)
	if moved == nil || moved != client.Authority() || reason != "" ||
		moved.Phase() != finegrained.PhaseUnknown || moved.Fallback() != finegrained.FallbackVersionOutside {
		t.Fatalf("RefreshAuthority = %+v, %q; want the phase A authority it moved the token to", moved, reason)
	}
	again, reason := client.RefreshAuthority(t.Context(), refreshReadable, table)
	if again != nil || reason != "" || client.Authority() == moved {
		t.Errorf("a second re-read at the same release = %v, %q; want it replaced and not reported as moved", again, reason)
	}
}

// TestReadGrant_ACancelledRead_IsNoAnswer verifies the read carries the
// caller's context: a context that has ended is no answer, with its error.
func TestReadGrant_ACancelledRead_IsNoAnswer(t *testing.T) {
	stand := newGrantServer(t, http.StatusOK, grantBody, "19.4.1-ee")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := ReadGrant(ctx, grantClient(t, stand.url).GL(), 9); !errors.Is(err, context.Canceled) {
		t.Errorf("ReadGrant error = %v, want the cancellation", err)
	}
}
