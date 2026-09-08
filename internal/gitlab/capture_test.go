package gitlab

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestCaptureTransport_CopiesTheBodyForARequestThatAsked verifies the whole
// contract of the capture: the SDK still reads the complete body, and the
// capture decodes the same bytes into a type of the handler's own, which is
// how a field client-go does not model reaches an output.
func TestCaptureTransport_CopiesTheBodyForARequestThatAsked(t *testing.T) {
	transport := &captureTransport{base: &stubRoundTripper{body: `{"id":7,"unmodeled":"here"}`}}
	ctx, capture := WithResponseCapture(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.example.com/x", http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest unexpected error: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() unexpected error: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(got) != `{"id":7,"unmodeled":"here"}` {
		t.Errorf("the SDK's body = %q, want the whole answer", got)
	}
	var extra struct {
		Unmodeled string `json:"unmodeled"`
	}
	if decodeErr := capture.Decode(&extra); decodeErr != nil {
		t.Fatalf("Decode() error = %v", decodeErr)
	}
	if extra.Unmodeled != "here" {
		t.Errorf("Decode() read %+v, want the field the SDK's struct would drop", extra)
	}
}

// TestCaptureTransport_ARequestThatDidNotAsk_IsLeftAlone verifies that the
// capture costs nothing to a request made without one: the body is handed on
// as it came, and a capture nothing was made under has nothing to decode.
func TestCaptureTransport_ARequestThatDidNotAsk_IsLeftAlone(t *testing.T) {
	transport := &captureTransport{base: &stubRoundTripper{body: "hello"}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://gitlab.example.com/x", http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest unexpected error: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if got, _ := io.ReadAll(resp.Body); string(got) != "hello" {
		t.Errorf("body = %q, want it passed through", got)
	}
	_, capture := WithResponseCapture(t.Context())
	if decodeErr := capture.Decode(&struct{}{}); !errors.Is(decodeErr, ErrNoResponseCaptured) {
		t.Errorf("Decode() on a capture nothing ran under = %v, want ErrNoResponseCaptured", decodeErr)
	}
}

// TestCaptureTransport_PassesThroughWhatItCannotCopy verifies the shapes a
// transport below can hand up that carry no body to copy: a failure, a nil
// response, a response with a nil body. Each is returned as it came, and the
// capture stays empty rather than recording an empty body as an answer.
func TestCaptureTransport_PassesThroughWhatItCannotCopy(t *testing.T) {
	cases := []struct {
		name    string
		base    *stubRoundTripper
		wantErr bool
	}{
		{name: "the base fails", base: &stubRoundTripper{fail: true}, wantErr: true},
		{name: "the base answers no response", base: &stubRoundTripper{nilResponse: true}},
		{name: "the base answers a response with no body", base: &stubRoundTripper{nilBody: true}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &captureTransport{base: testCase.base}
			ctx, capture := WithResponseCapture(t.Context())
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.example.com/x", http.NoBody)
			if err != nil {
				t.Fatalf("NewRequest unexpected error: %v", err)
			}

			resp, err := transport.RoundTrip(req)

			if (err != nil) != testCase.wantErr {
				t.Errorf("RoundTrip() error = %v, want an error: %v", err, testCase.wantErr)
			}
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
				t.Errorf("RoundTrip() = %+v, want the base's answer handed on unchanged", resp)
			}
			if decodeErr := capture.Decode(&struct{}{}); !errors.Is(decodeErr, ErrNoResponseCaptured) {
				t.Errorf("Decode() = %v, want nothing captured", decodeErr)
			}
		})
	}
}

// TestCaptureTransport_ABodyThatCannotBeRead_IsAnError verifies the two
// failures reading the body can meet. An answer over the client's ceiling
// fails with the ceiling's own error, since the capture reads through the
// limiter and the SDK's decoder would have met the same one; a body whose
// close fails is reported too, since a connection the pool cannot reuse is
// worth knowing about.
func TestCaptureTransport_ABodyThatCannotBeRead_IsAnError(t *testing.T) {
	client := &Client{}
	client.SetMaxResponseBytes(4)
	cases := []struct {
		name string
		base http.RoundTripper
		want error
		says string
	}{
		{
			name: "over the ceiling",
			base: &responseLimitTransport{base: &stubRoundTripper{body: "0123456789"}, client: client},
			want: ErrResponseTooLarge,
			says: "read the response to capture it",
		},
		{
			name: "close fails",
			base: closeFailingTransport{},
			want: errCloseFailed,
			says: "close the captured response",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &captureTransport{base: testCase.base}
			ctx, capture := WithResponseCapture(t.Context())
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.example.com/x", http.NoBody)
			if err != nil {
				t.Fatalf("NewRequest unexpected error: %v", err)
			}

			resp, err := transport.RoundTrip(req)

			if resp != nil {
				if resp.Body != nil {
					_ = resp.Body.Close()
				}
				t.Errorf("RoundTrip() = %+v, want no response from a body that could not be read", resp)
			}
			if !errors.Is(err, testCase.want) || !strings.Contains(err.Error(), testCase.says) {
				t.Errorf("RoundTrip() error = %v; want one wrapping %v that says %q", err, testCase.want, testCase.says)
			}
			if decodeErr := capture.Decode(&struct{}{}); !errors.Is(decodeErr, ErrNoResponseCaptured) {
				t.Errorf("Decode() = %v, want nothing captured from a body that could not be read", decodeErr)
			}
		})
	}
}

// errCloseFailed is what closeFailingTransport's body fails with.
var errCloseFailed = errors.New("close failed")

// closeFailingTransport answers a readable body whose Close fails.
type closeFailingTransport struct{}

func (closeFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: closeFailingBody{Reader: strings.NewReader("{}")}, Header: make(http.Header)}, nil
}

type closeFailingBody struct{ io.Reader }

func (closeFailingBody) Close() error { return errCloseFailed }

// TestResponseCapture_Decode_ReportsABodyTheTypeCannotHold verifies that a
// captured body which does not fit the handler's type is an error the
// handler sees, named as a decode of the capture: the SDK decoded the same
// bytes, so the fault is in the type and not in GitLab's answer.
func TestResponseCapture_Decode_ReportsABodyTheTypeCannotHold(t *testing.T) {
	capture := &ResponseCapture{}
	capture.record([]byte(`{"n":"text"}`))

	err := capture.Decode(&struct {
		N int `json:"n"`
	}{})

	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("Decode() error = %v, want one naming the capture", err)
	}
}

// TestCapturedBody_IsACaptureAlreadyAnswered verifies the seam a reader's
// test decodes from: the capture holds the body as if a request under it had
// been answered with it, so Decode reads it and never reports the empty case.
func TestCapturedBody_IsACaptureAlreadyAnswered(t *testing.T) {
	var got struct {
		N int `json:"n"`
	}

	err := CapturedBody([]byte(`{"n":4}`)).Decode(&got)

	if err != nil || got.N != 4 {
		t.Errorf("CapturedBody().Decode() = %+v, %v; want the body decoded", got, err)
	}
}

// TestClient_ResponseCapture_ReadsWhatTheSDKDecoded verifies the mechanism
// end to end through the real client: an SDK call made under a capture
// answers with its own struct as always, the capture holds the same body for
// a field that struct does not carry, and a second call made without one
// records nothing, so the capture is per request and not per client.
func TestClient_ResponseCapture_ReadsWhatTheSDKDecoded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v4/version":
			_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abc"}`))
		case "/api/v4/user":
			_, _ = w.Write([]byte(`{"id":1,"username":"u","unmodeled":"here"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := NewClientWithTokenRetries(srv.URL, testValidToken, false, true)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}

	ctx, capture := WithResponseCapture(t.Context())
	user, _, err := client.GL().Users.CurrentUser(gl.WithContext(ctx))
	if err != nil {
		t.Fatalf("CurrentUser() error = %v", err)
	}
	var extra struct {
		Unmodeled string `json:"unmodeled"`
	}
	if decodeErr := capture.Decode(&extra); decodeErr != nil {
		t.Fatalf("Decode() error = %v", decodeErr)
	}
	if user.Username != "u" || extra.Unmodeled != "here" {
		t.Errorf("SDK read %+v and the capture %+v, want both halves of the one answer", user, extra)
	}

	_, untouched := WithResponseCapture(t.Context())
	if _, _, againErr := client.GL().Users.CurrentUser(gl.WithContext(t.Context())); againErr != nil {
		t.Fatalf("CurrentUser() without a capture error = %v", againErr)
	}
	if decodeErr := untouched.Decode(&extra); !errors.Is(decodeErr, ErrNoResponseCaptured) {
		t.Errorf("a capture no request ran under decoded %v, want ErrNoResponseCaptured", decodeErr)
	}
}
