// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// TestHasDotSegment_PathForms_ReportOnlyDotSegments asserts that a "." or ".."
// component is found in the text as given and, failing that, once its
// percent-encoding is undone, whether "/" or "\" delimits it and whatever the
// case of the hex digits of an escape, and that the segment reports which of
// the two forms it was found in. Text whose components each name a resource
// (an escaped value, a dotted name, a component that only contains dots, a
// value encoded twice) is not reported, and an escape that cannot be undone
// neither hides a segment written as given nor one written escaped elsewhere
// in the text.
func TestHasDotSegment_PathForms_ReportOnlyDotSegments(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		want  DotSegment
		found bool
	}{
		{"plain nested path", "/api/v4/projects/42/jobs/7/artifacts/build/report.txt", DotSegment{}, false},
		{"escaped project path", "/api/v4/projects/group%2Fsub%2Fproj/jobs/7/artifacts/report.txt", DotSegment{}, false},
		{"version with dots", "/api/v4/projects/42/releases/v1.2.3", DotSegment{}, false},
		{"dot in component not a segment", "/api/v4/x/a..b", DotSegment{}, false},
		{"three dots", "/api/v4/x/.../b", DotSegment{}, false},
		{"encoded twice", "/api/v4/x/a/%252e%252e/b", DotSegment{}, false},
		{"literal dot dot", "/api/v4/x/a/../b", DotSegment{Name: ".."}, true},
		{"leading dot dot", "/../a/b", DotSegment{Name: ".."}, true},
		{"single dot segment", "/api/v4/x/./y", DotSegment{Name: "."}, true},
		{"backslash separated", `/api/v4/x/a\..\b`, DotSegment{Name: ".."}, true},
		{"trailing dot dot", "/api/v4/x/a/..", DotSegment{Name: ".."}, true},
		{"bare value", "..", DotSegment{Name: ".."}, true},
		{"literal segment before an encoded one", "a/../%2e", DotSegment{Name: ".."}, true},
		{"lower encoded dot dot", "/api/v4/x/a/%2e%2e/b", DotSegment{Name: "..", Decoded: true}, true},
		{"upper encoded dot dot", "/api/v4/x/a/%2E%2E/b", DotSegment{Name: "..", Decoded: true}, true},
		{"mixed case escapes", "/api/v4/x/a/%2e%2E/b", DotSegment{Name: "..", Decoded: true}, true},
		{"one dot escaped", "/api/v4/x/a/.%2E/b", DotSegment{Name: "..", Decoded: true}, true},
		{"encoded single dot", "/api/v4/x/%2e/y", DotSegment{Name: ".", Decoded: true}, true},
		{"encoded separator after dots", "/api/v4/x/..%2Fb", DotSegment{Name: "..", Decoded: true}, true},
		{"dot between encoded separators", "/api/v4/x/a%2F.%2Fb", DotSegment{Name: ".", Decoded: true}, true},
		{"dot dot between encoded backslashes", "/api/v4/x/a%5C..%5Cb", DotSegment{Name: "..", Decoded: true}, true},
		{"invalid encoding, no dot segment", "/api/v4/x/%zz/y", DotSegment{}, false},
		{"invalid encoding with a literal dot dot", "/api/v4/x/%zz/../y", DotSegment{Name: ".."}, true},
		{"invalid encoding before an encoded separator", "%zz/..%2Fx", DotSegment{Name: "..", Decoded: true}, true},
		{"invalid encoding after an encoded dot dot", "/api/v4/x/%2e%2e/%zz", DotSegment{Name: "..", Decoded: true}, true},
		{"invalid encoding in the same component", "/api/v4/x/..%2F%zz", DotSegment{Name: "..", Decoded: true}, true},
		{"invalid encoding beside an encoded single dot", `%zz\%2E`, DotSegment{Name: ".", Decoded: true}, true},
		{"invalid encoding beside an encoded name", "/api/v4/x/%zz/v1%2E2", DotSegment{}, false},
		{"escape of another byte beside dots", "/api/v4/x/%2e%2e%41/y", DotSegment{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found := HasDotSegment(tc.text)
			if found != tc.found {
				t.Fatalf("HasDotSegment(%q) found = %v, want %v", tc.text, found, tc.found)
			}
			if got != tc.want {
				t.Errorf("HasDotSegment(%q) = %+v, want %+v", tc.text, got, tc.want)
			}
		})
	}
}

// TestHasDotSegment_DecodableText_FindsWhatPathUnescapeWould asserts that on
// text whose percent-encoding can be undone, the decoded form [HasDotSegment]
// reads through [dotSegmentEscapes] reports the segment that the text
// url.PathUnescape returns would: the same name, or none. The texts mix the
// six escapes it undoes with escapes of other bytes, an escaped "%" and both
// cases of hex digits, which are the ways the two readings could part.
func TestHasDotSegment_DecodableText_FindsWhatPathUnescapeWould(t *testing.T) {
	texts := []string{
		"%2e%2e", "%2E", "a%2F%2e%2e%2Fb", `a%5C.%5cb`, "..%2f", "%2e%2e%41", "%41%2e",
		"%25%2e", "%252e%252e", "%25%32%65", "%c3%a9/..%2Fx", "v1%2E2", "%2e%2e%2e", "%2F%2F", "%2e%20",
	}
	for _, text := range texts {
		t.Run(text, func(t *testing.T) {
			decoded, err := url.PathUnescape(text)
			if err != nil {
				t.Fatalf("url.PathUnescape(%q) = %v, want a text it can decode", text, err)
			}
			wantName, wantFound := dotComponent(decoded)
			gotName, gotFound := dotComponent(dotSegmentEscapes.Replace(text))
			if gotName != wantName || gotFound != wantFound {
				t.Errorf("dotComponent of %q read through dotSegmentEscapes = %q, %v; through url.PathUnescape = %q, %v",
					text, gotName, gotFound, wantName, wantFound)
			}
		})
	}
}

// TestDotSegment_String_NamesTheFormItWasFoundIn asserts that a segment found
// in the text as given is described by its quoted name alone, and one found
// only once the percent-encoding is undone says so, so a message built from it
// is right for both forms.
func TestDotSegment_String_NamesTheFormItWasFoundIn(t *testing.T) {
	cases := []struct {
		name    string
		segment DotSegment
		want    string
	}{
		{"as given", DotSegment{Name: ".."}, `".."`},
		{"single dot as given", DotSegment{Name: "."}, `"."`},
		{"once decoded", DotSegment{Name: "..", Decoded: true}, `".." once its percent-encoding is undone`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.segment.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// countingRoundTripper records whether its base was reached, so a refusal can
// be told apart from a pass-through.
type countingRoundTripper struct {
	calls int
}

func (c *countingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls++
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

// TestPathTraversalGuardTransport_RoundTrip_RefusesDotSegmentsAndSendsCleanPaths
// asserts that the guard refuses a request whose path carries a dot segment
// without reaching the base, with an [ErrRequestPathRefused] that names the
// path without its query, the segment and the form it was found in, and hands
// a request with a clean path to the base unchanged.
func TestPathTraversalGuardTransport_RoundTrip_RefusesDotSegmentsAndSendsCleanPaths(t *testing.T) {
	refusals := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "a segment in the path as given",
			url:  "https://gitlab.example/api/v4/x/a/../b?page=2",
			want: `request path refused: the path "/api/v4/x/a/../b" has the segment ".."`,
		},
		{
			name: "a segment only once the path is decoded",
			url:  "https://gitlab.example/api/v4/x/a%2F.%2Fb?page=2",
			want: `request path refused: the path "/api/v4/x/a%2F.%2Fb" has the segment "." once its percent-encoding is undone`,
		},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			calls, rtErr := roundTripThroughGuard(t, tc.url)
			if rtErr == nil {
				t.Fatal("RoundTrip() = nil error, want a refusal")
			}
			if calls != 0 {
				t.Errorf("base reached %d time(s); a refused request must not be sent", calls)
			}
			if !errors.Is(rtErr, ErrRequestPathRefused) {
				t.Errorf("RoundTrip() error %v does not wrap ErrRequestPathRefused", rtErr)
			}
			if got := rtErr.Error(); got != tc.want {
				t.Errorf("RoundTrip() error = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("passes a clean path to the base", func(t *testing.T) {
		calls, rtErr := roundTripThroughGuard(t, "https://gitlab.example/api/v4/projects/42/jobs/7/artifacts/build/report.txt")
		if rtErr != nil {
			t.Fatalf("RoundTrip() clean path = %v, want nil", rtErr)
		}
		if calls != 1 {
			t.Errorf("base reached %d time(s); a clean request must be sent once", calls)
		}
	})
}

// roundTripThroughGuard sends a GET for rawURL through a guard in front of a
// counting base and returns how many times the base was reached and the
// error. A response is closed here; one returned beside an error is reported,
// since a refusal must carry none.
func roundTripThroughGuard(t *testing.T, rawURL string) (int, error) {
	t.Helper()
	base := &countingRoundTripper{}
	guard := &pathTraversalGuardTransport{base: base}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, rtErr := guard.RoundTrip(req)
	if resp != nil {
		resp.Body.Close()
		if rtErr != nil {
			t.Error("RoundTrip() returned a response alongside the refusal")
		}
	}
	return base.calls, rtErr
}

// TestAPITransport_UnescapedDotSegmentArgument_IsRefusedBeforeSending drives
// client-go routes that place an argument into the path without escaping it
// through the full client transport chain against a recording server: an
// argument with a dot segment is refused and no request reaches the server,
// while the same route with a clean argument reaches it unchanged. It holds
// the rule to the chain every handler's request passes, not to one handler's
// own check.
func TestAPITransport_UnescapedDotSegmentArgument_IsRefusedBeforeSending(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.RequestURI)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	newClient := func() *Client {
		client, err := NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: testValidToken, DisableRetries: true})
		if err != nil {
			t.Fatalf(fmtNewClientErr, err)
		}
		client.MarkInitialized()
		return client
	}

	reset := func() {
		mu.Lock()
		seen = nil
		mu.Unlock()
	}
	saw := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}

	// Each refusal reaches the caller through client-go and net/http, which
	// wrap it, so the sentinel has to survive both for a caller to classify
	// the refusal; the segment's description has to match the form the value
	// took on the wire, whether client-go escaped it or not.
	refusals := []struct {
		name string
		send func(*Client) error
		want string
	}{
		{
			name: "artifact path with a dot segment, placed unescaped",
			send: func(client *Client) error {
				_, _, err := client.GL().Jobs.DownloadSingleArtifactsFile("group/sub/proj", 7,
					"a/../b", gl.WithContext(context.Background()))
				return err
			},
			want: `has the segment ".."`,
		},
		{
			name: "hook URL variable key with a dot segment, placed unescaped",
			send: func(client *Client) error {
				value := "v"
				_, err := client.GL().SystemHooks.SetHookURLVariable(5, "a/../b",
					&gl.SetHookURLVariableOptions{Value: &value}, gl.WithContext(context.Background()))
				return err
			},
			want: `has the segment ".."`,
		},
		{
			name: "escaped label name that is a dot segment as a whole",
			send: func(client *Client) error {
				_, _, err := client.GL().Labels.GetLabel(1, "..", gl.WithContext(context.Background()))
				return err
			},
			want: `has the segment ".."`,
		},
		{
			name: "escaped label name with a dot segment between its slashes",
			send: func(client *Client) error {
				_, _, err := client.GL().Labels.GetLabel(1, "x/./y", gl.WithContext(context.Background()))
				return err
			},
			want: `has the segment "." once its percent-encoding is undone`,
		},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			reset()
			err := tc.send(newClient())
			if err == nil {
				t.Fatal("the call = nil error, want a refusal")
			}
			if !errors.Is(err, ErrRequestPathRefused) {
				t.Errorf("the call's error %v does not wrap ErrRequestPathRefused", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the call's error %q does not say it %s", err, tc.want)
			}
			if got := saw(); len(got) != 0 {
				t.Errorf("server received %v; a refused request must never leave the process", got)
			}
		})
	}

	t.Run("a clean artifact path still reaches the server", func(t *testing.T) {
		reset()
		client := newClient()
		_, _, err := client.GL().Jobs.DownloadSingleArtifactsFile("group/sub/proj", 7,
			"build/output/report.txt", gl.WithContext(context.Background()))
		if err != nil {
			t.Fatalf("DownloadSingleArtifactsFile with a clean path = %v, want it sent", err)
		}
		got := saw()
		if len(got) != 1 {
			t.Fatalf("server received %v; a clean request must be sent once", got)
		}
		if !strings.Contains(got[0], "/artifacts/build/output/report.txt") {
			t.Errorf("server received %q, want the clean nested artifact path", got[0])
		}
	})
}
