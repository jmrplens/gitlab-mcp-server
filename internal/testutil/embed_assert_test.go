package testutil

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestAssertEmbeddedResource_TogglesEmbeddedContent drives
// [AssertEmbeddedResource] through a complete enabled/disabled cycle to make
// sure both subtests pass and the toggle is restored to enabled afterwards.
//
// The test spins up an in-memory MCP server whose "test_embed" tool embeds
// a JSON resource, connects a client session over [mcp.NewInMemoryTransports],
// and hands the session to AssertEmbeddedResource together with the real
// [toolutil.EnableEmbeddedResources] toggle. After the assertion the test
// re-checks [toolutil.EmbeddedResourcesEnabled] so the suite leaves the
// global flag in its production default state.
func TestAssertEmbeddedResource_TogglesEmbeddedContent(t *testing.T) {
	const resourceURI = "gitlab://test/resources/1"

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "test_embed", Description: "Returns an embedded resource."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}
			toolutil.EmbedResourceJSON(result, resourceURI, map[string]any{"id": 1})
			return result, nil, nil
		})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		_ = serverSession.Wait()
	})

	AssertEmbeddedResource(t, ctx, clientSession, "test_embed", map[string]any{}, resourceURI, toolutil.EnableEmbeddedResources)
	if !toolutil.EmbeddedResourcesEnabled() {
		t.Fatal("AssertEmbeddedResource did not restore embedded resources to enabled")
	}
}

// TestAssertEmbeddedResource_ToolThatAnswersWrongly_IsReported covers what the
// helper says when the tool under it does not do what the toggle promises.
//
// These are the paths a passing suite never takes, and they are the only reason
// the helper exists: a missing embed, an embed that should have been suppressed,
// a tool that answered with an error, and a session that is no longer there.
// Each is driven with a recorder, since a real *testing.T would fail this test
// for reporting exactly what it is meant to report.
func TestAssertEmbeddedResource_ToolThatAnswersWrongly_IsReported(t *testing.T) {
	session := connectEmbedTestSession(t)

	cases := []struct {
		name   string
		assert func(embedReporter)
		want   string
	}{
		{
			name: "no embed where one was expected",
			assert: func(reporter embedReporter) {
				assertResourceEmbedded(t.Context(), reporter, session, "plain", map[string]any{}, "gitlab://test/resources/1")
			},
			want: "expected EmbeddedResource for plain",
		},
		{
			name: "an embed where none was expected",
			assert: func(reporter embedReporter) {
				assertResourceNotEmbedded(t.Context(), reporter, session, "embeds", map[string]any{})
			},
			want: "expected no EmbeddedResource when disabled",
		},
		{
			name: "a tool that answered with an error",
			assert: func(reporter embedReporter) {
				assertResourceEmbedded(t.Context(), reporter, session, "failing", map[string]any{}, "gitlab://test/resources/1")
			},
			want: "expected successful result, got IsError=true",
		},
		{
			name: "a tool that answered with an error while the embed was off",
			assert: func(reporter embedReporter) {
				assertResourceNotEmbedded(t.Context(), reporter, session, "failing", map[string]any{})
			},
			want: "expected successful result, got IsError=true",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reporter := &recordingEmbedReporter{}

			testCase.assert(reporter)

			if !strings.Contains(reporter.joined(), testCase.want) {
				t.Errorf("report = %q, want it to contain %q", reporter.joined(), testCase.want)
			}
		})
	}
}

// TestCallToolSuccessfully_ClosedSession_IsReported covers the transport half
// of the same guard: a call that never reached the server is reported as the
// call it was, rather than as a nil result the assertions would trip over.
func TestCallToolSuccessfully_ClosedSession_IsReported(t *testing.T) {
	session := connectEmbedTestSession(t)
	session.Close()
	reporter := &recordingEmbedReporter{}

	if result := callToolSuccessfully(t.Context(), reporter, session, "embeds", map[string]any{}); result != nil {
		t.Errorf("callToolSuccessfully() = %v, want nil after a transport failure", result)
	}
	if !strings.Contains(reporter.joined(), "CallTool(embeds)") {
		t.Errorf("report = %q, want it to name the call", reporter.joined())
	}
}

// connectEmbedTestSession serves the three answers these assertions have to
// tell apart: a tool that embeds a resource, one that does not, and one that
// fails.
func connectEmbedTestSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "embed-test-server", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "embeds", Description: "Returns an embedded resource."},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}
			toolutil.EmbedResourceJSON(result, "gitlab://test/resources/1", map[string]any{"id": 1})
			return result, nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "plain", Description: "Returns text only."},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "failing", Description: "Answers with an error result."},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "no"}}}, nil, nil
		})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "embed-test-client", Version: "0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		_ = serverSession.Wait()
	})
	return clientSession
}

// recordingEmbedReporter stands in for *testing.T so a reported failure is
// collected rather than failing the test that provoked it.
type recordingEmbedReporter struct {
	messages []string
}

// Helper satisfies the reporter and does nothing.
func (*recordingEmbedReporter) Helper() {}

// Error records what would have been reported.
func (r *recordingEmbedReporter) Error(args ...any) {
	r.messages = append(r.messages, fmt.Sprint(args...))
}

// Errorf records what would have been reported.
func (r *recordingEmbedReporter) Errorf(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// Fatalf records what would have been reported.
func (r *recordingEmbedReporter) Fatalf(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

// joined returns every recorded message as one string for substring assertions.
func (r *recordingEmbedReporter) joined() string { return strings.Join(r.messages, "\n") }

// TestFirstEmbeddedResource_Found confirms that [firstEmbeddedResource]
// returns the first [*mcp.EmbeddedResource] in the content slice when one is
// present.
//
// The test builds a result containing a text block followed by an embedded
// resource, calls the helper, and asserts that the returned pointer equals
// the embedded resource we constructed. It protects against regressions
// where non-embedded content blocks would be returned instead.
func TestFirstEmbeddedResource_Found(t *testing.T) {
	embed := &mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "u", MIMEType: "application/json"}}
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "x"}, embed}}
	if got := firstEmbeddedResource(result); got != embed {
		t.Errorf("firstEmbeddedResource = %v, want embed", got)
	}
}

// TestFirstEmbeddedResource_None confirms that [firstEmbeddedResource]
// returns nil when the result has no embedded resource blocks.
//
// The test constructs a result with only a text content block and verifies
// that the helper yields a nil pointer rather than panicking or returning
// a zero value. This guards callers that range over the returned block.
func TestFirstEmbeddedResource_None(t *testing.T) {
	result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "x"}}}
	if got := firstEmbeddedResource(result); got != nil {
		t.Errorf("firstEmbeddedResource = %v, want nil", got)
	}
}

// TestAssertEmbeddedResourcePayload_MismatchFields is a table-driven check
// that [assertEmbeddedResourcePayload] records a failure for each kind of
// embedded-resource mismatch the helper guards against.
//
// Each subtest feeds a hand-crafted [*mcp.ResourceContents] whose URI,
// MIME type, or Text field disagrees with the expected values, then verifies
// that the helper marked a sentinel [*testing.T] as failed. The cases cover
// URI mismatch, MIME type mismatch, and an empty Text payload — the three
// failure modes a real MCP server response can produce.
func TestAssertEmbeddedResourcePayload_MismatchFields(t *testing.T) {
	tests := []struct {
		name      string
		resource  *mcp.ResourceContents
		wantURI   string
		wantField string
	}{
		{
			name:      "URI mismatch",
			resource:  &mcp.ResourceContents{URI: "got", MIMEType: "application/json", Text: "x"},
			wantURI:   "want",
			wantField: "URI",
		},
		{
			name:      "MIMEType mismatch",
			resource:  &mcp.ResourceContents{URI: "u", MIMEType: "text/plain", Text: "x"},
			wantURI:   "u",
			wantField: "MIMEType",
		},
		{
			name:      "Text empty",
			resource:  &mcp.ResourceContents{URI: "u", MIMEType: "application/json", Text: ""},
			wantURI:   "u",
			wantField: "Text",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeT := &testing.T{}
			assertEmbeddedResourcePayload(fakeT, tt.resource, tt.wantURI)
			if !fakeT.Failed() {
				t.Errorf("assertEmbeddedResourcePayload should fail for %s mismatch", tt.wantField)
			}
		})
	}
}
