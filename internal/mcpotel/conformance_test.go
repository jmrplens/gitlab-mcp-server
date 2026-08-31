package mcpotel

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// admitted is the list a deployment passes in, kept short so a test that adds
// a version has to say so.
var admitted = []string{"2026-07-28", "2025-11-25"}

// TestMiddleware_ProtocolVersionIsRecordedFromMeta covers the attribute the
// convention marks Recommended and this package declared and then never set.
//
// The constant existed in attributes.go from the first commit, which is the
// failure mode worth naming: a declared-but-unused attribute key reads exactly
// like an implemented one, and nothing in Go complains about an unused package
// level constant. It took a collector with real traffic to notice the value was
// absent from every span.
func TestMiddleware_ProtocolVersionIsRecordedFromMeta(t *testing.T) {
	req := callToolRequest("gitlab_execute_action", nil, map[string]any{
		metaProtocolVersionKey: "2026-07-28",
	})

	span := runOnce(t, Options{ProtocolVersions: admitted}, "tools/call", req, nil, nil)

	value, ok := attrOf(span, AttrMCPProtocolVersion)
	if !ok {
		t.Fatal("mcp.protocol.version is absent; the convention marks it Recommended")
	}
	if value.AsString() != "2026-07-28" {
		t.Errorf("mcp.protocol.version = %q, want %q", value.AsString(), "2026-07-28")
	}
}

// TestMiddleware_UnadmittedProtocolVersionIsDropped is the guard that makes the
// attribute safe to put on a metric.
//
// The value arrives from the caller. Recording whatever it says would let a
// client mint a time series per spelling, and the SDK answers an exhausted
// series budget by collapsing the overflow into one otel.metric.overflow
// bucket, first-come-wins under cumulative temporality. That is silent data
// destruction, so the allow-list is load-bearing rather than defensive.
func TestMiddleware_UnadmittedProtocolVersionIsDropped(t *testing.T) {
	req := callToolRequest("gitlab_execute_action", nil, map[string]any{
		metaProtocolVersionKey: "1999-01-01-not-a-revision",
	})

	span := runOnce(t, Options{ProtocolVersions: admitted}, "tools/call", req, nil, nil)

	if _, ok := attrOf(span, AttrMCPProtocolVersion); ok {
		t.Error("an unadmitted version was recorded; a caller can then mint one time series per spelling")
	}
}

// TestMiddleware_NoAllowListRecordsNothing pins the default for a caller that
// never configured the list, so the unsafe case is the one that needs an
// explicit opt-in rather than the safe one.
func TestMiddleware_NoAllowListRecordsNothing(t *testing.T) {
	req := callToolRequest("gitlab_execute_action", nil, map[string]any{
		metaProtocolVersionKey: "2026-07-28",
	})

	span := runOnce(t, Options{}, "tools/call", req, nil, nil)

	if _, ok := attrOf(span, AttrMCPProtocolVersion); ok {
		t.Error("a version was recorded with no allow-list configured")
	}
}

// TestMiddleware_NoSessionMeansNoSessionID covers the condition attached to
// mcp.session.id, which is a condition rather than a preference: "When the MCP
// request or notification is part of a session."
//
// Under the default stateless HTTP transport there is no session id, and the
// right answer is to omit the attribute rather than invent a per-POST value.
// A request built without a session is the same case.
func TestMiddleware_NoSessionMeansNoSessionID(t *testing.T) {
	req := callToolRequest("gitlab_execute_action", nil, nil)

	span := runOnce(t, Options{}, "tools/call", req, nil, nil)

	if _, ok := attrOf(span, AttrMCPSessionID); ok {
		t.Error("mcp.session.id was recorded for a request that is part of no session")
	}
}

// TestMiddleware_LinksTheAmbientContextWhenMetaSuppliesTheParent is the
// regression for the half of the propagation rule that was missing.
//
// The convention asks for two relationships, not one: parent on the context
// from params._meta, "and SHOULD link current ambient context, if it's
// present". Extract replaces the ambient context in ctx, so an implementation
// that reads it afterwards finds the extracted one and links nothing. On HTTP
// the ambient span is this server's own HTTP span, so without the link a trace
// arriving through _meta has no record of which HTTP request carried it.
func TestMiddleware_LinksTheAmbientContextWhenMetaSuppliesTheParent(t *testing.T) {
	recorder := newRecorder(t)

	// An ambient span, standing in for the HTTP server span.
	ambientCtx, ambient := otel.Tracer("test").Start(context.Background(), "POST")
	defer ambient.End()

	req := callToolRequest("gitlab_execute_action", nil, map[string]any{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	})

	handler := Middleware(Options{})(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	_, _ = handler(ambientCtx, "tools/call", req)

	var span trace.SpanContext
	links := 0
	for _, s := range recorder.Ended() {
		if s.Name() != "tools/call gitlab_execute_action" {
			continue
		}
		span = s.Parent()
		links = len(s.Links())
		for _, l := range s.Links() {
			if !l.SpanContext.Equal(ambient.SpanContext()) {
				t.Errorf("link points at %v, want the ambient span %v", l.SpanContext, ambient.SpanContext())
			}
		}
	}

	if got := span.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("parent trace = %s, want the one from _meta", got)
	}
	if links != 1 {
		t.Errorf("recorded %d links, want 1: the ambient context is dropped rather than linked", links)
	}
}

// TestMiddleware_NoLinkWhenTheAmbientContextIsAlreadyTheParent keeps the link
// from becoming noise.
//
// With no trace context in _meta the ambient span is already the parent, and a
// span linked to its own parent states a relationship the tree already carries.
func TestMiddleware_NoLinkWhenTheAmbientContextIsAlreadyTheParent(t *testing.T) {
	recorder := newRecorder(t)

	ambientCtx, ambient := otel.Tracer("test").Start(context.Background(), "POST")
	defer ambient.End()

	handler := Middleware(Options{})(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	_, _ = handler(ambientCtx, "tools/call", callToolRequest("gitlab_execute_action", nil, nil))

	for _, s := range recorder.Ended() {
		if s.Name() != "tools/call gitlab_execute_action" {
			continue
		}
		if got := len(s.Links()); got != 0 {
			t.Errorf("recorded %d links, want 0: the ambient span is the parent already", got)
		}
		if !s.Parent().Equal(ambient.SpanContext()) {
			t.Errorf("parent = %v, want the ambient span", s.Parent())
		}
	}
}
