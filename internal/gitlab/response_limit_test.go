// response_limit_test.go contains unit tests for the ceiling this package puts
// on a single GitLab response body.
package gitlab

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// gzipJSONOfSize returns a gzip-compressed JSON object whose decompressed form
// is at least n bytes long. It is the shape of a decompression bomb: the
// compressed bytes are a rounding error next to what they expand to.
func gzipJSONOfSize(t *testing.T, n int) []byte {
	t.Helper()
	payload := `{"id":1,"name":"` + strings.Repeat("a", n) + `"}`
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(payload)); err != nil {
		t.Fatalf("gzip write unexpected error: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close unexpected error: %v", err)
	}
	return buf.Bytes()
}

// bombServer serves one gzip-compressed body under the given status code and
// reports how many compressed bytes it wrote, so a test can show the
// amplification it is defending against.
func bombServer(t *testing.T, status, decompressedSize int) (url string, wireBytes int) {
	t.Helper()
	body := gzipJSONOfSize(t, decompressedSize)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			t.Errorf("writing bomb body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, len(body)
}

// TestClient_OversizedResponse_IsBounded verifies that a response body which
// expands past the client's ceiling is abandoned with [ErrResponseTooLarge]
// rather than buffered, on both the success path and the error path.
//
// The error path is the larger amplifier and the one reachable without being
// authorized for the action at all: client-go's CheckResponse runs for every
// non-2xx, reads the whole body with io.ReadAll, unmarshals it into any and
// then formats the raw bytes into a message — three full copies of whatever
// the upstream sent.
//
// The ceiling has to bound the decompressed stream to be worth anything, so
// the table pairs each bounded case with a control that raises the ceiling
// above the payload and asserts the full decompressed value arrives. Without
// that control a test could pass merely because the transport never
// decompressed anything.
func TestClient_OversizedResponse_IsBounded(t *testing.T) {
	const (
		decompressed = 4 << 20  // 4 MiB of body from a few KB on the wire
		tightCap     = 64 << 10 // well under the payload
		looseCap     = 16 << 20 // well over it
	)

	tests := []struct {
		name     string
		status   int
		cap      int64
		wantSize bool // the call fails with ErrResponseTooLarge
	}{
		{name: "success body over the ceiling", status: http.StatusOK, cap: tightCap, wantSize: true},
		{name: "success body under the ceiling", status: http.StatusOK, cap: looseCap},
		{name: "error body over the ceiling", status: http.StatusBadRequest, cap: tightCap, wantSize: true},
		{name: "error body under the ceiling", status: http.StatusBadRequest, cap: looseCap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvURL, wire := bombServer(t, tt.status, decompressed)
			if wire > decompressed/10 {
				t.Fatalf("payload compressed to %d bytes from %d; the fixture is not a bomb", wire, decompressed)
			}

			client, err := NewClientWithTokenRetries(srvURL, testValidToken, false, true)
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			client.SetMaxResponseBytes(tt.cap)

			project, _, callErr := client.GL().Projects.GetProject("group/proj", nil)

			switch {
			case tt.status != http.StatusOK:
				assertErrorResponseSize(t, callErr, tt.status, tt.wantSize)
			case tt.wantSize:
				assertAbandonedAsTooLarge(t, project, callErr)
			default:
				assertFullyDecompressed(t, project, callErr, decompressed)
			}
		})
	}
}

// assertAbandonedAsTooLarge checks the call failed with the size error and
// produced no value, rather than a truncated one.
func assertAbandonedAsTooLarge(t *testing.T, project *gl.Project, err error) {
	t.Helper()
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("GetProject() error = %v, want %v", err, ErrResponseTooLarge)
	}
	if project != nil {
		t.Errorf("GetProject() returned a project, want a zero value alongside the size error")
	}
}

// assertFullyDecompressed checks that a ceiling above the payload lets the
// whole decompressed body through, which is what proves the ceiling — and not
// the transport failing to decompress — is what the tight rows measured.
func assertFullyDecompressed(t *testing.T, project *gl.Project, err error, want int) {
	t.Helper()
	if err != nil {
		t.Fatalf("GetProject() unexpected error under a loose ceiling: %v", err)
	}
	if project == nil {
		t.Fatal("GetProject() returned no project under a loose ceiling")
	}
	if len(project.Name) != want {
		t.Errorf("GetProject() name length = %d, want %d; the body was not decompressed in full",
			len(project.Name), want)
	}
}

// assertErrorResponseSize checks a non-2xx response is reported as a GitLab
// error carrying the status, and that the oversized body it arrived with is
// buffered into the message only when the ceiling allowed it.
//
// The loose-ceiling half is the control: it is the pre-fix behavior, and
// seeing the whole body land in an error message is what shows the ceiling —
// not some accident of the transport — is the thing that stops it.
func assertErrorResponseSize(t *testing.T, err error, wantStatus int, wantBounded bool) {
	t.Helper()
	var errResp *gl.ErrorResponse
	if !errors.As(err, &errResp) {
		t.Fatalf("error = %T (%v), want *gitlab.ErrorResponse", err, err)
	}
	if errResp.StatusCode != wantStatus {
		t.Errorf("ErrorResponse.StatusCode = %d, want %d", errResp.StatusCode, wantStatus)
	}
	const bounded = 4 << 10
	if (len(errResp.Error()) <= bounded) != wantBounded {
		t.Errorf("error message is %d bytes, want bounded under %d = %v",
			len(errResp.Error()), bounded, wantBounded)
	}
}

// TestClient_DefaultMaxResponseBytes verifies every constructor starts with
// the package default, so a client built anywhere in the server is bounded
// without the caller having to know the ceiling exists.
func TestClient_DefaultMaxResponseBytes(t *testing.T) {
	tests := []struct {
		name  string
		build func(baseURL string) (*Client, error)
	}{
		{name: "NewClient", build: func(u string) (*Client, error) { return NewClient(newTestConfig(u, testValidToken)) }},
		{name: "NewClientWithToken", build: func(u string) (*Client, error) { return NewClientWithToken(u, testValidToken, false) }},
		{name: "NewOAuthClientWithToken", build: func(u string) (*Client, error) {
			return NewOAuthClientWithToken(u, testValidToken, false)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := tt.build("https://gitlab.example.com")
			if err != nil {
				t.Fatalf("constructor error: %v", err)
			}
			if got := client.maxResponseBytes(); got != DefaultMaxResponseBytes {
				t.Errorf("maxResponseBytes() = %d, want %d", got, DefaultMaxResponseBytes)
			}
		})
	}
}

// TestMaxResponseBytes_NilClient verifies the accessor answers with the
// package default rather than panicking, so a transport holding no client
// still bounds what it reads.
func TestMaxResponseBytes_NilClient(t *testing.T) {
	var c *Client
	if got := c.maxResponseBytes(); got != DefaultMaxResponseBytes {
		t.Errorf("(*Client)(nil).maxResponseBytes() = %d, want %d", got, DefaultMaxResponseBytes)
	}
}

// TestLimitedBody_DeliversUpToTheCeiling verifies the reader hands over a body
// that fits in full, never delivers more than its limit, and fails with
// [ErrResponseTooLarge] as soon as one byte past the limit exists — including
// the boundary either side of the limit and a body read one byte at a time.
func TestLimitedBody_DeliversUpToTheCeiling(t *testing.T) {
	const limit = 100

	tests := []struct {
		name    string
		size    int
		chunk   int
		wantErr bool
	}{
		{name: "well under the limit", size: 10, chunk: 64},
		{name: "exactly at the limit", size: limit, chunk: 64},
		{name: "one byte over", size: limit + 1, chunk: 64, wantErr: true},
		{name: "far over", size: 10000, chunk: 64, wantErr: true},
		{name: "over the limit read one byte at a time", size: 200, chunk: 1, wantErr: true},
		{name: "zero-length body", size: 0, chunk: 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &limitedBody{
				inner:     io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), tt.size))),
				remaining: limit,
			}
			read, err := readInChunks(body, tt.chunk)

			if errors.Is(err, ErrResponseTooLarge) != tt.wantErr {
				t.Errorf("error = %v, want ErrResponseTooLarge = %v", err, tt.wantErr)
			}
			if read > limit {
				t.Errorf("read %d bytes, want no more than the limit of %d", read, limit)
			}
			if !tt.wantErr {
				if read != tt.size {
					t.Errorf("read %d bytes, want the whole %d-byte body", read, tt.size)
				}
				if err != nil && !errors.Is(err, io.EOF) {
					t.Errorf("unexpected error: %v", err)
				}
			}
			if closeErr := body.Close(); closeErr != nil {
				t.Errorf("Close() unexpected error: %v", closeErr)
			}
		})
	}
}

// errReaderStalled reports that a reader returned no bytes and no error, which
// io.Reader discourages and which a correct [limitedBody] never does.
var errReaderStalled = errors.New("reader returned no bytes and no error")

// readInChunks drains r in fixed-size reads and reports how many bytes it
// delivered before it stopped, plus the error that stopped it.
//
// A read that delivers nothing and reports nothing ends the loop with
// [errReaderStalled] rather than being retried. [limitedBody] hands its inner
// reader a slice of at least one byte on every call, so it cannot produce that
// answer; a version that shortened the slice to nothing would, and the loop
// would spin on it forever, which turns a wrong limiter into a suite that
// hangs instead of a test that fails.
func readInChunks(r io.Reader, chunk int) (int, error) {
	buf := make([]byte, chunk)
	total := 0
	for {
		n, err := r.Read(buf)
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, errReaderStalled
		}
	}
}

// TestLimitedBody_StaysFailedAfterOverflow verifies a reader that has already
// refused a body keeps refusing, so a caller that ignores the first error
// cannot resume reading the rest of an oversized response.
func TestLimitedBody_StaysFailedAfterOverflow(t *testing.T) {
	body := &limitedBody{
		inner:     io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("x"), 1000))),
		remaining: 10,
	}
	buf := make([]byte, 64)

	for attempt := range 3 {
		n, err := body.Read(buf)
		if attempt == 0 {
			// The first read fills up to the limit; the overflow is detected
			// on the same call because the reader asks for one byte past it.
			if !errors.Is(err, ErrResponseTooLarge) {
				t.Fatalf("read %d: n=%d err=%v, want ErrResponseTooLarge", attempt, n, err)
			}
			continue
		}
		if n != 0 || !errors.Is(err, ErrResponseTooLarge) {
			t.Errorf("read %d: n=%d err=%v, want 0, ErrResponseTooLarge", attempt, n, err)
		}
	}
}

// offeredSizes records the length of every buffer it is handed and fills it
// completely, which is what a body with more to give does.
type offeredSizes struct{ sizes []int }

// Read fills p and records how much room it was given.
func (r *offeredSizes) Read(p []byte) (int, error) {
	r.sizes = append(r.sizes, len(p))
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// Close satisfies io.ReadCloser and does nothing.
func (r *offeredSizes) Close() error { return nil }

// TestLimitedBody_OffersOneByteBeyondWhatIsLeft verifies the reader asks its
// inner body for exactly one byte more than the ceiling allows, and refuses
// the response on that same call.
//
// The window is the mechanism the whole limiter rests on, and the two ways of
// getting it wrong fail in opposite directions. Offering the ceiling exactly
// can never see a byte past it, so an oversized body is delivered truncated
// and reported as complete, which is the failure [limitedBody] exists to
// prevent. Offering less than the ceiling shrinks the window on every call
// until it reaches zero, and a reader handed an empty slice answers "no bytes,
// no error" for ever: the caller does not fail, it spins.
//
// Both are invisible to a test that only drains the body and checks the error,
// which is why the size the inner reader was offered is asserted directly.
func TestLimitedBody_OffersOneByteBeyondWhatIsLeft(t *testing.T) {
	const remaining = 10

	inner := &offeredSizes{}
	body := &limitedBody{inner: inner, remaining: remaining}

	n, err := body.Read(make([]byte, 100))

	t.Run("the inner body is offered the ceiling plus one", func(t *testing.T) {
		if len(inner.sizes) != 1 {
			t.Fatalf("inner body was read %d times, want once: %v", len(inner.sizes), inner.sizes)
		}
		if inner.sizes[0] != remaining+1 {
			t.Errorf("inner body was offered %d bytes, want %d", inner.sizes[0], remaining+1)
		}
	})
	t.Run("the overflow is reported on that same call", func(t *testing.T) {
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Errorf("Read() error = %v, want %v", err, ErrResponseTooLarge)
		}
		if n != 0 {
			t.Errorf("Read() delivered %d bytes alongside the size error, want none", n)
		}
	})
}

// TestLimitedBody_UnderTheCeiling_DeliversWithoutError verifies a read that
// fits hands back its bytes and no error, which is the case a limiter that
// compares the wrong way round breaks first.
//
// Asserting it here rather than only through a full drain is deliberate: a
// limiter that refuses every read is caught by the first call, before any
// retry, decompression or client-go error path has a chance to turn the defect
// into something slower and less legible.
func TestLimitedBody_UnderTheCeiling_DeliversWithoutError(t *testing.T) {
	body := &limitedBody{inner: io.NopCloser(strings.NewReader("hello")), remaining: 100}

	n, err := body.Read(make([]byte, 64))
	if err != nil {
		t.Errorf("Read() error = %v, want none for a body well inside the ceiling", err)
	}
	if n != len("hello") {
		t.Errorf("Read() = %d bytes, want %d", n, len("hello"))
	}
	if body.remaining != 100-int64(len("hello")) {
		t.Errorf("remaining = %d, want %d", body.remaining, 100-len("hello"))
	}
}

// The bodies GitLab answers a 401 with, as its own code writes them: Grape's
// unauthorized! for a permission refusal and for a token it cannot find, and
// rack-oauth2's rendering of the API guard's invalid_token for an expired one.
const (
	plainUnauthorizedBody = `{"message":"401 Unauthorized"}`
	expiredTokenBody      = `{"error":"invalid_token","error_description":"Token is expired. You can either do re-authorization or token refresh."}`
)

// restURL and graphQLURL are the two endpoints the credential signals differ
// between.
const (
	restURL    = "https://gitlab.example.com/api/v4/projects/1/merge_requests/1/approve"
	graphQLURL = "https://gitlab.example.com/api/graphql"
)

// answersHook registers a hook on client that records every answer it is
// told, and returns the record. The transport calls the hook on the goroutine
// that made the request, which in these tests is the test goroutine, so the
// record needs no lock.
func answersHook(client *Client) *[]UnauthorizedAnswer {
	answers := &[]UnauthorizedAnswer{}
	client.SetOnUnauthorized(func(answer UnauthorizedAnswer) { *answers = append(*answers, answer) })
	return answers
}

// roundTripDrained sends one request through transport and reads the answer's
// body to the end, which is what the SDK does with every answer it is given.
func roundTripDrained(t *testing.T, transport http.RoundTripper, url string) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	body, err := readBounded(resp.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return body
}

// readBounded reads r to its end like io.ReadAll, except that a reader which
// keeps answering nothing and no error is reported as a failure instead of
// being waited on forever. A body replayed wrongly can do exactly that, and a
// test that hangs on it reports nothing.
func readBounded(r io.Reader) ([]byte, error) {
	var out []byte
	buf := make([]byte, 512)
	for idle := 0; idle < 100; {
		n, err := r.Read(buf)
		out = append(out, buf[:n]...)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if n == 0 {
			idle++
			continue
		}
		idle = 0
	}
	return out, errors.New("the body answered nothing a hundred times in a row without ending")
}

// TestResponseLimitTransport_A401NamingTheCredential_ReportsItOnce verifies
// that a 401 GitLab said was about the credential reaches the hook as that,
// and only once however many follow.
//
// The two signals are the API guard's invalid_token and any 401 from the
// GraphQL endpoint. The verdict is final, so the server pool acts on the
// first and a second delivery would only repeat it.
func TestResponseLimitTransport_A401NamingTheCredential_ReportsItOnce(t *testing.T) {
	tests := []struct {
		name string
		url  string
		body string
	}{
		{name: "a REST 401 carrying invalid_token", url: restURL, body: expiredTokenBody},
		{name: "a GraphQL 401", url: graphQLURL, body: `{"errors":[{"message":"Invalid token"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			answers := answersHook(client)
			transport := &responseLimitTransport{
				base:                &stubRoundTripper{body: tt.body, status: http.StatusUnauthorized},
				client:              client,
				reportsUnauthorized: true,
			}
			for range 3 {
				roundTripDrained(t, transport, tt.url)
			}
			if len(*answers) != 1 || (*answers)[0] != UnauthorizedCredential {
				t.Errorf("the hook was told %v over three 401s naming the credential, want [UnauthorizedCredential] once", *answers)
			}
		})
	}
}

// TestResponseLimitTransport_A401NamingNothing_ReportsUnexplainedEveryTime
// verifies that a 401 naming no cause reaches the hook on every refusal, and
// never uses up the one delivery a verdict on the credential gets.
//
// Before, every 401 was the same event and the first one was spent as a
// revocation, so a permission refusal ended the credential and a revocation
// arriving after it was never delivered.
func TestResponseLimitTransport_A401NamingNothing_ReportsUnexplainedEveryTime(t *testing.T) {
	client := &Client{}
	answers := answersHook(client)
	plain := &responseLimitTransport{
		base:                &stubRoundTripper{body: plainUnauthorizedBody, status: http.StatusUnauthorized},
		client:              client,
		reportsUnauthorized: true,
	}
	for range 3 {
		roundTripDrained(t, plain, restURL)
	}
	expired := &responseLimitTransport{
		base:                &stubRoundTripper{body: expiredTokenBody, status: http.StatusUnauthorized},
		client:              client,
		reportsUnauthorized: true,
	}
	roundTripDrained(t, expired, restURL)

	want := []UnauthorizedAnswer{UnauthorizedUnexplained, UnauthorizedUnexplained, UnauthorizedUnexplained, UnauthorizedCredential}
	if !slices.Equal(*answers, want) {
		t.Errorf("the hook was told %v, want %v", *answers, want)
	}
}

// TestResponseLimitTransport_WhatIsNotReported_LeavesTheHookAlone verifies the
// cases the hook must not hear about: a status other than 401, a client that
// registered nothing or cleared what it registered, and a transport that does
// not report.
//
// The last is the health client's. Every request it makes is a probe whose
// caller acts on the answer, and one of them is the probe that confirms a 401
// naming nothing, whose own 401 must not raise another.
func TestResponseLimitTransport_WhatIsNotReported_LeavesTheHookAlone(t *testing.T) {
	t.Run("any other status", func(t *testing.T) {
		client := &Client{}
		answers := answersHook(client)
		// sequential: one hook records across every status, asserted once below
		for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
			roundTripDrained(t, &responseLimitTransport{
				base:                &stubRoundTripper{body: expiredTokenBody, status: status},
				client:              client,
				reportsUnauthorized: true,
			}, graphQLURL)
		}
		if len(*answers) != 0 {
			t.Errorf("the hook was told %v without a 401", *answers)
		}
	})
	t.Run("a client with no hook", func(t *testing.T) {
		body := roundTripDrained(t, &responseLimitTransport{
			base:                &stubRoundTripper{body: plainUnauthorizedBody, status: http.StatusUnauthorized},
			client:              &Client{},
			reportsUnauthorized: true,
		}, restURL)
		if string(body) != plainUnauthorizedBody {
			t.Errorf("body = %q, want %q", body, plainUnauthorizedBody)
		}
	})
	t.Run("a cleared hook", func(t *testing.T) {
		client := &Client{}
		answers := answersHook(client)
		client.SetOnUnauthorized(nil)
		roundTripDrained(t, &responseLimitTransport{
			base:                &stubRoundTripper{body: expiredTokenBody, status: http.StatusUnauthorized},
			client:              client,
			reportsUnauthorized: true,
		}, restURL)
		if len(*answers) != 0 {
			t.Errorf("a cleared hook was told %v", *answers)
		}
	})
	t.Run("a transport that does not report", func(t *testing.T) {
		client := &Client{}
		answers := answersHook(client)
		body := roundTripDrained(t, &responseLimitTransport{
			base:   &stubRoundTripper{body: expiredTokenBody, status: http.StatusUnauthorized},
			client: client,
		}, restURL)
		if len(*answers) != 0 {
			t.Errorf("a transport that does not report told the hook %v", *answers)
		}
		if string(body) != expiredTokenBody {
			t.Errorf("body = %q, want %q", body, expiredTokenBody)
		}
	})
}

// failAfterBody answers a fixed prefix together with an error, and afterwards,
// if read again, more bytes: the shape of a body whose error is not sticky,
// which nothing in [io.Reader]'s contract rules out. Closing it is recorded.
type failAfterBody struct {
	prefix string
	err    error
	later  string
	calls  int
	closed bool
}

// Read answers the prefix and the error first, then the later bytes, then EOF.
func (b *failAfterBody) Read(p []byte) (int, error) {
	b.calls++
	switch b.calls {
	case 1:
		return copy(p, b.prefix), b.err
	case 2:
		return copy(p, b.later), nil
	default:
		return 0, io.EOF
	}
}

// Close records that the body was closed.
func (b *failAfterBody) Close() error {
	b.closed = true
	return nil
}

// bodyRoundTripper answers 401 with the body it is given.
type bodyRoundTripper struct{ body io.ReadCloser }

// RoundTrip answers 401 with the stubbed body.
func (s *bodyRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusUnauthorized, Body: s.body, Header: make(http.Header)}, nil
}

// TestResponseLimitTransport_Peeked401Body_IsReplayedWhole verifies that the
// prefix read to classify a 401 is delivered again, so what the SDK and the
// response capture read is exactly what GitLab sent.
//
// A body longer than the peek is delivered whole and names nothing even when
// the code sits past the bound, which is the direction that asks GitLab again
// rather than acting. One that ends exactly at the bound is read whole by the
// peek and still classified.
func TestResponseLimitTransport_Peeked401Body_IsReplayedWhole(t *testing.T) {
	longPlain := `{"message":"` + strings.Repeat("x", 2*unauthorizedPeekBytes) + `"}`
	longWithCodeLate := `{"message":"` + strings.Repeat("x", 2*unauthorizedPeekBytes) + `","error":"invalid_token"}`

	tests := []struct {
		name string
		body string
		want UnauthorizedAnswer
	}{
		{name: "a short body naming the credential", body: expiredTokenBody, want: UnauthorizedCredential},
		{name: "a short body naming nothing", body: plainUnauthorizedBody, want: UnauthorizedUnexplained},
		{name: "a body longer than the peek", body: longPlain, want: UnauthorizedUnexplained},
		{name: "invalid_token past the bound", body: longWithCodeLate, want: UnauthorizedUnexplained},
		{name: "a body that ends exactly at the bound", body: expiredTokenBody + strings.Repeat(" ", unauthorizedPeekBytes-len(expiredTokenBody)), want: UnauthorizedCredential},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			client.SetMaxResponseBytes(1 << 20)
			answers := answersHook(client)
			got := roundTripDrained(t, &responseLimitTransport{
				base:                &stubRoundTripper{body: tt.body, status: http.StatusUnauthorized},
				client:              client,
				reportsUnauthorized: true,
			}, restURL)
			if string(got) != tt.body {
				t.Errorf("the caller read %d bytes, want the %d GitLab sent, byte for byte", len(got), len(tt.body))
			}
			if len(*answers) != 1 || (*answers)[0] != tt.want {
				t.Errorf("the hook was told %v, want [%v]", *answers, tt.want)
			}
		})
	}
}

// TestResponseLimitTransport_Peeked401Body_StaysUnderTheCeiling verifies that
// peeking a 401 does not take its body out from under the response ceiling:
// the peek happens first, and the limit still counts every byte of the replay,
// the peeked ones included.
func TestResponseLimitTransport_Peeked401Body_StaysUnderTheCeiling(t *testing.T) {
	client := &Client{}
	client.SetMaxResponseBytes(16)
	answers := answersHook(client)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, restURL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := (&responseLimitTransport{
		base:                &stubRoundTripper{body: plainUnauthorizedBody, status: http.StatusUnauthorized},
		client:              client,
		reportsUnauthorized: true,
	}).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if _, readErr := readBounded(resp.Body); !errors.Is(readErr, ErrResponseTooLarge) {
		t.Errorf("reading a 401 body past the ceiling = %v, want ErrResponseTooLarge", readErr)
	}
	if len(*answers) != 1 {
		t.Errorf("the hook was told %v, want one answer", *answers)
	}
}

// TestResponseLimitTransport_PeekThatFails_HandsTheFailureOn verifies that a
// read failing during the peek is replayed to the caller rather than
// swallowed, and that nothing is read past it: a body free to answer more after
// an error would otherwise have those bytes spliced in after the gap, and the
// caller would decode a body GitLab never sent. The prefix it did read names
// nothing, and closing the replay closes the body GitLab sent.
func TestResponseLimitTransport_PeekThatFails_HandsTheFailureOn(t *testing.T) {
	readErr := errors.New("connection reset mid-body")
	body := &failAfterBody{prefix: `{"error":"inv`, err: readErr, later: `alid_token"}`}
	client := &Client{}
	client.SetMaxResponseBytes(1 << 20)
	answers := answersHook(client)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, restURL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := (&responseLimitTransport{
		base:                &bodyRoundTripper{body: body},
		client:              client,
		reportsUnauthorized: true,
	}).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	got, gotErr := readBounded(resp.Body)
	if !errors.Is(gotErr, readErr) {
		t.Errorf("reading the body = %v, want the error the peek met", gotErr)
	}
	if string(got) != body.prefix {
		t.Errorf("the caller read %q, want the prefix %q and nothing spliced in after the failure", got, body.prefix)
	}
	if len(*answers) != 1 || (*answers)[0] != UnauthorizedUnexplained {
		t.Errorf("the hook was told %v, want [UnauthorizedUnexplained]", *answers)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Errorf("Close() = %v", closeErr)
	}
	if !body.closed {
		t.Error("closing the replayed body did not close the one GitLab sent")
	}
}

// TestResponseLimitTransport_PassesThroughUnlimited verifies a client whose
// ceiling is removed gets the untouched body back, and that a transport error
// is returned as-is rather than being wrapped in a limiter.
func TestResponseLimitTransport_PassesThroughUnlimited(t *testing.T) {
	tests := []struct {
		name    string
		limit   int64
		fail    bool
		wantErr bool
	}{
		{name: "ceiling disabled", limit: 0},
		{name: "negative ceiling", limit: -1},
		{name: "transport error", limit: 1 << 20, fail: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			client.SetMaxResponseBytes(tt.limit)
			transport := &responseLimitTransport{
				base:   &stubRoundTripper{fail: tt.fail, body: "hello"},
				client: client,
			}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://gitlab.example.com/x", http.NoBody)
			if err != nil {
				t.Fatalf("NewRequest unexpected error: %v", err)
			}
			resp, err := transport.RoundTrip(req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("RoundTrip() error = nil, want the base transport's error")
				}
				return
			}
			if err != nil {
				t.Fatalf("RoundTrip() unexpected error: %v", err)
			}
			defer resp.Body.Close()
			if _, ok := resp.Body.(*limitedBody); ok {
				t.Error("response body was wrapped in a limiter despite the ceiling being disabled")
			}
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading body: %v", err)
			}
			if string(got) != "hello" {
				t.Errorf("body = %q, want %q", got, "hello")
			}
		})
	}
}

// stubRoundTripper answers with a fixed body, or with an error when fail is
// set, without any network involvement.
type stubRoundTripper struct {
	fail bool
	body string
	// status is the answer's status code; zero means 200.
	status int
	// nilResponse answers (nil, nil), and nilBody answers a response whose
	// Body is nil. Both violate the [http.RoundTripper] contract; they exist
	// because this transport is the innermost layer and guards against them.
	nilResponse bool
	nilBody     bool
}

// RoundTrip returns the stubbed response or error.
func (s *stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	if s.fail {
		return nil, errors.New("stub transport failure")
	}
	if s.nilResponse {
		return nil, nil //nolint:nilnil // a contract-violating transport is what this stub imitates
	}
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	resp := &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}
	if s.nilBody {
		resp.Body = nil
	}
	return resp, nil
}

// TestResponseLimitTransport_ContractViolatingBase_IsPassedThrough verifies
// that the innermost transport survives a base that breaks the
// [http.RoundTripper] contract, rather than dereferencing what it was handed.
//
// Two shapes are pinned. A base that answers (nil, nil) must not reach the
// unauthorized hook, since there is no status to judge, and must not be
// wrapped, since there is no body to bound. A base that answers a response
// with a nil Body must be returned untouched for the same reason: wrapping
// nil would turn a malformed answer into a nil dereference in whichever
// layer above reads it.
//
// These are the branches an in-process transport stack can produce and a real
// GitLab cannot, so nothing else in this package reaches them: net/http's own
// transport always returns one of the two valid shapes.
func TestResponseLimitTransport_ContractViolatingBase_IsPassedThrough(t *testing.T) {
	t.Run("a base that answers nothing at all", func(t *testing.T) {
		respWasNil, _, answers := roundTripContractViolation(t, &stubRoundTripper{nilResponse: true}, restURL)
		if !respWasNil {
			t.Error("RoundTrip() invented a response, want the base's nil passed through")
		}
		if len(answers) != 0 {
			t.Errorf("the unauthorized hook was told %v for a response that never existed", answers)
		}
	})

	t.Run("a base that answers a response with no body", func(t *testing.T) {
		respWasNil, bodyWasNil, answers := roundTripContractViolation(t, &stubRoundTripper{nilBody: true}, restURL)
		if respWasNil {
			t.Fatal("RoundTrip() = nil, want the base's response passed through")
		}
		if !bodyWasNil {
			t.Error("the nil body was replaced, want it passed through unwrapped")
		}
		if len(answers) != 0 {
			t.Errorf("the unauthorized hook was told %v on a 200", answers)
		}
	})
}

// roundTripContractViolation sends one request through the innermost
// transport over base, and reports the shape of what came back rather than
// the response itself: every case it serves is about a response with no body
// to hand on, and returning one would only be a body nothing can close.
func roundTripContractViolation(t *testing.T, base http.RoundTripper, url string) (respWasNil, bodyWasNil bool, answers []UnauthorizedAnswer) {
	t.Helper()
	client := &Client{}
	client.SetMaxResponseBytes(1 << 20)
	told := answersHook(client)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest unexpected error: %v", err)
	}
	resp, err := (&responseLimitTransport{base: base, client: client, reportsUnauthorized: true}).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() unexpected error: %v", err)
	}
	if resp == nil {
		return true, true, *told
	}
	if resp.Body == nil {
		return false, true, *told
	}
	defer resp.Body.Close()
	return false, false, *told
}

// TestResponseLimitTransport_A401WithNoBody_IsStillReported verifies that a
// 401 whose response carries no body, which only a contract-violating base can
// hand over, still reaches the hook and is passed on untouched: as naming
// nothing over REST, since there is no body to carry a code, and as naming the
// credential over GraphQL, where the endpoint alone decides.
func TestResponseLimitTransport_A401WithNoBody_IsStillReported(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want UnauthorizedAnswer
	}{
		{name: "a REST 401 with no body names nothing", url: restURL, want: UnauthorizedUnexplained},
		{name: "a GraphQL 401 with no body names the credential", url: graphQLURL, want: UnauthorizedCredential},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respWasNil, bodyWasNil, answers := roundTripContractViolation(t, &stubRoundTripper{nilBody: true, status: http.StatusUnauthorized}, tt.url)
			if respWasNil {
				t.Fatal("RoundTrip() = nil, want the base's response passed through")
			}
			if !bodyWasNil {
				t.Error("the nil body was replaced, want it passed through unwrapped")
			}
			if len(answers) != 1 || answers[0] != tt.want {
				t.Errorf("the unauthorized hook was told %v on a 401, want [%v]", answers, tt.want)
			}
		})
	}
}
