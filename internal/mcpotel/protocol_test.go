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

// withProtocolHeader builds a tools/call carrying the HTTP protocol header, and
// optionally a session and a _meta.
func withProtocolHeader(session *mcp.ServerSession, meta mcp.Meta, headerValue string) mcp.Request {
	h := http.Header{}
	h.Set(protocolHeader, headerValue)
	params := &mcp.CallToolParamsRaw{Name: "gitlab_execute_action"}
	if meta != nil {
		params.SetMeta(meta)
	}
	return &mcp.CallToolRequest{Session: session, Params: params, Extra: &mcp.RequestExtra{Header: h}}
}

// TestProtocolVersionFor_ASessionStillInItsHandshakeLeavesItToTheHeader
// verifies that a session which has not recorded its initialize parameters
// yet is passed over rather than read.
//
// That is the state of every session while its initialize request is being
// handled: the middleware sees the request before the SDK records what it
// carried. A client that sends no per-request version reaches the session
// source then with nothing in it, and has to fall through to the header.
func TestProtocolVersionFor_ASessionStillInItsHandshakeLeavesItToTheHeader(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	_, serverTransport := mcp.NewInMemoryTransports()
	session, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if session.InitializeParams() != nil {
		t.Fatal("the session already carries initialize parameters, so this test no longer drives the handshake")
	}

	got := protocolVersionFor(withProtocolHeader(session, nil, "2025-11-25"), allowedVersions([]string{"2025-11-25"}))

	if got != "2025-11-25" {
		t.Errorf("protocolVersionFor = %q, want the header's 2025-11-25 while the session has nothing to say", got)
	}
}

// TestAllowedVersions_ABlankEntryAdmitsNothing verifies that an empty entry in
// the configured list is dropped rather than admitted.
//
// A list split from a string with a trailing comma carries one. Admitted, it
// would let a client's empty _meta version end the search as an answer, so
// the header a well-behaved proxy set would never be read.
func TestAllowedVersions_ABlankEntryAdmitsNothing(t *testing.T) {
	t.Parallel()

	allowed := allowedVersions([]string{"", "2025-11-25"})

	if _, admitted := allowed[""]; admitted {
		t.Error("the empty string was admitted as a protocol version")
	}
	blankMeta := mcp.Meta{metaProtocolVersionKey: ""}
	if got := protocolVersionFor(withProtocolHeader(nil, blankMeta, "2025-11-25"), allowed); got != "2025-11-25" {
		t.Errorf("protocolVersionFor = %q, want the header's 2025-11-25 past an empty _meta version", got)
	}
}

// connectedServerSession returns a live [*mcp.ServerSession] whose initialize
// parameters have been negotiated with a real client.
//
// A real session because that is the only way to populate InitializeParams: the
// field the negotiated revision lands in is written by the SDK during the
// handshake and cannot be set from outside it, so a fake would leave the branch
// that reads it untested while looking covered.
func connectedServerSession(t *testing.T) *mcp.ServerSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	})
	return serverSession
}

// TestProtocolVersionFor_TheSessionSuppliesItWhenMetaDoesNot covers the middle
// source, which is where every revision before 2026-07-28 records what it
// negotiated.
//
// It sits between the two sources that already had cases: the per-request _meta
// key, which older clients do not send, and the HTTP header, which is present on
// one transport and set by a client that can disagree with what it negotiated.
// Without this source a stdio session speaking 2025-11-25 recorded no version at
// all, which is the one combination the other two cannot cover between them.
func TestProtocolVersionFor_TheSessionSuppliesItWhenMetaDoesNot(t *testing.T) {
	session := connectedServerSession(t)
	params := session.InitializeParams()
	if params == nil {
		t.Fatal("the connected session carries no initialize parameters")
	}
	negotiated := params.ProtocolVersion
	if negotiated == "" {
		t.Fatal("the handshake negotiated no protocol version")
	}

	// The negotiated value rather than a literal: which revision the SDK's own
	// client settles on is its business, and pinning one here would fail on an
	// SDK bump rather than on a defect.
	request := &mcp.CallToolRequest{
		Session: session,
		Params:  &mcp.CallToolParamsRaw{Name: "gitlab_execute_action"},
	}

	if got := protocolVersionFor(request, allowedVersions([]string{negotiated})); got != negotiated {
		t.Errorf("protocolVersionFor = %q, want the session's negotiated %q", got, negotiated)
	}
	// And the allow-list still governs this source, exactly as it governs the
	// other two: an admitted-elsewhere value is not admitted here.
	if got := protocolVersionFor(request, allowedVersions([]string{"1999-01-01"})); got != "" {
		t.Errorf("protocolVersionFor = %q for a session speaking an unadmitted revision, want empty", got)
	}
}
