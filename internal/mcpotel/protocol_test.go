// protocol_test.go verifies where the recorded protocol version comes from
// when several sources could supply one, and that only an admitted value is
// ever recorded.
package mcpotel

import (
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestProtocolVersionFor_TheHTTPHeaderIsTheLastResort covers the source that is
// consulted only when nothing better exists.
//
// The order is deliberate. _meta is what protocol 2026-07-28 defines and the
// only source that works on stdio; the session's initialize parameters are
// where the older revisions record what was negotiated; the header is present
// on one transport only and is set by a client that can disagree with what it
// negotiated. So the header is read last, and — like every other source — only
// a value on the allow-list is recorded, because this attribute reaches a
// metric and an unbounded one there is a caller minting time series.
func TestProtocolVersionFor_TheHTTPHeaderIsTheLastResort(t *testing.T) {
	t.Parallel()

	allowed := allowedVersions([]string{"2026-07-28", "2025-11-25"})

	header := func(value string) http.Header {
		// Set rather than a map literal: Get canonicalizes the key it looks
		// for, and a literal written the way the header is spelled on the wire
		// would never be found.
		h := http.Header{}
		h.Set(protocolHeader, value)
		return h
	}
	withHeader := func(value string) mcp.Request {
		return &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{Name: "gitlab_execute_action"},
			Extra:  &mcp.RequestExtra{Header: header(value)},
		}
	}
	withMetaAndHeader := func(meta, headerValue string) mcp.Request {
		params := &mcp.CallToolParamsRaw{Name: "gitlab_execute_action"}
		params.SetMeta(mcp.Meta{metaProtocolVersionKey: meta})
		return &mcp.CallToolRequest{
			Params: params,
			Extra:  &mcp.RequestExtra{Header: header(headerValue)},
		}
	}

	tests := []struct {
		name    string
		req     mcp.Request
		allowed map[string]struct{}
		want    string
	}{
		{name: "an admitted header is recorded", req: withHeader("2025-11-25"), allowed: allowed, want: "2025-11-25"},
		{name: "an unadmitted header is dropped", req: withHeader("1999-01-01"), allowed: allowed, want: ""},
		{name: "meta outranks the header", req: withMetaAndHeader("2026-07-28", "2025-11-25"), allowed: allowed, want: "2026-07-28"},
		{name: "no request records nothing", req: nil, allowed: allowed, want: ""},
		{name: "no allow-list records nothing", req: withHeader("2025-11-25"), allowed: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := protocolVersionFor(tt.req, tt.allowed); got != tt.want {
				t.Errorf("protocolVersionFor = %q, want %q", got, tt.want)
			}
		})
	}
}
