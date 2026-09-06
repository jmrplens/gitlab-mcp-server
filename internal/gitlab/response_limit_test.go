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
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
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

// readInChunks drains r in fixed-size reads and reports how many bytes it
// delivered before it stopped, plus the error that stopped it.
func readInChunks(r io.Reader, chunk int) (int, error) {
	buf := make([]byte, chunk)
	total := 0
	for {
		n, err := r.Read(buf)
		total += n
		if err != nil {
			return total, err
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

// TestResponseLimitTransport_PassesThroughUnlimited verifies a client whose
// ceiling is removed gets the untouched body back, and that a transport error
// is returned as-is rather than being wrapped in a limiter.
// TestResponseLimitTransport_A401FiresTheUnauthorizedHookOnce verifies the
// innermost transport reports the first 401 GitLab answers to whoever
// registered for it, once, and never for any other status. The server pool
// registers to drop its entry, so a revoked token stops being served on the
// first call GitLab refuses rather than on the next periodic check.
func TestResponseLimitTransport_A401FiresTheUnauthorizedHookOnce(t *testing.T) {
	t.Parallel()

	roundTrip := func(t *testing.T, transport *responseLimitTransport) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://gitlab.example.com/x", http.NoBody)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		_ = resp.Body.Close()
	}

	t.Run("three refusals fire it once", func(t *testing.T) {
		t.Parallel()
		client := &Client{}
		fired := 0
		client.SetOnUnauthorized(func() { fired++ })
		transport := &responseLimitTransport{base: &stubRoundTripper{body: "{}", status: http.StatusUnauthorized}, client: client}
		for range 3 {
			roundTrip(t, transport)
		}
		if fired != 1 {
			t.Errorf("the hook fired %d times over three 401s, want once", fired)
		}
	})
	t.Run("any other status leaves it alone", func(t *testing.T) {
		t.Parallel()
		client := &Client{}
		fired := 0
		client.SetOnUnauthorized(func() { fired++ })
		// sequential: one hook counts across every status, asserted once below
		for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
			roundTrip(t, &responseLimitTransport{base: &stubRoundTripper{body: "{}", status: status}, client: client})
		}
		if fired != 0 {
			t.Errorf("the hook fired %d times without a 401", fired)
		}
	})
	t.Run("a client with no hook is untouched by a 401", func(t *testing.T) {
		t.Parallel()
		roundTrip(t, &responseLimitTransport{base: &stubRoundTripper{body: "{}", status: http.StatusUnauthorized}, client: &Client{}})
	})
	t.Run("a nil hook clears it", func(t *testing.T) {
		t.Parallel()
		client := &Client{}
		fired := 0
		client.SetOnUnauthorized(func() { fired++ })
		client.SetOnUnauthorized(nil)
		roundTrip(t, &responseLimitTransport{base: &stubRoundTripper{body: "{}", status: http.StatusUnauthorized}, client: client})
		if fired != 0 {
			t.Errorf("a cleared hook fired %d times", fired)
		}
	})
}

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
	// roundTrip reports the shape of what came back rather than the response
	// itself: every case here is about a response with no body to hand on,
	// and returning one would only be a body nothing can close.
	roundTrip := func(t *testing.T, base http.RoundTripper) (respWasNil, bodyWasNil bool, fired int) {
		t.Helper()
		client := &Client{}
		client.SetMaxResponseBytes(1 << 20)
		client.SetOnUnauthorized(func() { fired++ })

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://gitlab.example.com/x", http.NoBody)
		if err != nil {
			t.Fatalf("NewRequest unexpected error: %v", err)
		}
		resp, err := (&responseLimitTransport{base: base, client: client}).RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip() unexpected error: %v", err)
		}
		if resp == nil {
			return true, true, fired
		}
		if resp.Body == nil {
			return false, true, fired
		}
		defer resp.Body.Close()
		return false, false, fired
	}

	t.Run("a base that answers nothing at all", func(t *testing.T) {
		respWasNil, _, fired := roundTrip(t, &stubRoundTripper{nilResponse: true})
		if !respWasNil {
			t.Error("RoundTrip() invented a response, want the base's nil passed through")
		}
		if fired != 0 {
			t.Errorf("the unauthorized hook fired %d times for a response that never existed", fired)
		}
	})

	t.Run("a base that answers a response with no body", func(t *testing.T) {
		respWasNil, bodyWasNil, fired := roundTrip(t, &stubRoundTripper{nilBody: true})
		if respWasNil {
			t.Fatal("RoundTrip() = nil, want the base's response passed through")
		}
		if !bodyWasNil {
			t.Error("the nil body was replaced, want it passed through unwrapped")
		}
		if fired != 0 {
			t.Errorf("the unauthorized hook fired %d times on a 200", fired)
		}
	})

	t.Run("a 401 with no body still fires the hook", func(t *testing.T) {
		respWasNil, bodyWasNil, fired := roundTrip(t, &stubRoundTripper{nilBody: true, status: http.StatusUnauthorized})
		if respWasNil {
			t.Fatal("RoundTrip() = nil, want the base's response passed through")
		}
		if !bodyWasNil {
			t.Error("the nil body was replaced, want it passed through unwrapped")
		}
		if fired != 1 {
			t.Errorf("the unauthorized hook fired %d times on a 401, want once", fired)
		}
	})
}
