// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"strings"
	"testing"
)

// TestRootedAnswer_TakesThePartTheHandlerReads verifies where each kind of
// call's key tree is taken from: a REST body whole and as it came; a GraphQL
// answer at the field below data, its connections recorded as the lists they
// page; an MCP answer's JSON-RPC result; and a tool result's text, as JSON when
// it is JSON and as text otherwise, its text blocks joined and any other block
// left out.
func TestRootedAnswer_TakesThePartTheHandlerReads(t *testing.T) {
	cases := []struct {
		name        string
		root        string
		contentType string
		payload     string
		wantType    string
		wantBody    string
	}{
		{name: "a REST body", contentType: "text/plain", payload: "@header", wantType: "text/plain", wantBody: "@header"},
		{
			name: "a GraphQL field", root: "data.orbit.schema", contentType: mediaJSON,
			payload:  `{"data":{"orbit":{"schema":{"nodes":{"pageInfo":{"hasNextPage":false},"nodes":[{"name":"A","properties":{"pageInfo":{},"nodes":[{"name":"id"}]}}]},"edges":{"nodes":[]}}}}}`,
			wantType: mediaJSON, wantBody: `{"edges":{"nodes":[]},"nodes":[{"name":"A","properties":[{"name":"id"}]}]}`,
		},
		{
			name: "an MCP result", root: rootMCPResult, contentType: mediaJSON,
			payload:  `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"t"}]}}`,
			wantType: mediaJSON, wantBody: `{"tools":[{"name":"t"}]}`,
		},
		{
			name: "a tool's JSON text", root: rootMCPText, contentType: mediaJSON,
			payload:  `{"result":{"content":[{"type":"text","text":"[{\"name\":\"query_graph\"}]"}]}}`,
			wantType: mediaJSON, wantBody: `[{"name":"query_graph"}]`,
		},
		{
			name: "a tool's text", root: rootMCPText, contentType: mediaJSON,
			payload:  `{"result":{"content":[{"type":"text","text":"domains[9]:"},{"type":"image","data":"AA=="},{"type":"text","text":42},{"type":"text","text":"edges[60]:"}]}}`,
			wantType: mediaText, wantBody: "domains[9]:\nedges[60]:",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotBody, err := rootedAnswer(tc.root, tc.contentType, []byte(tc.payload))
			if err != nil {
				t.Fatalf("rootedAnswer() error: %v", err)
			}
			if gotType != tc.wantType || string(gotBody) != tc.wantBody {
				t.Fatalf("rootedAnswer() = %q, %s, want %q, %s", gotType, gotBody, tc.wantType, tc.wantBody)
			}
		})
	}
}

// TestRootedAnswer_WhatCannotBeRead_IsAnError verifies the answers a call
// cannot be recorded from: one that does not decode, one with nothing or null
// at the root, a path through a value that is not an object, and a tool
// result with no content list.
func TestRootedAnswer_WhatCannotBeRead_IsAnError(t *testing.T) {
	cases := []struct {
		name    string
		root    string
		payload string
		noRoot  bool
		want    string
	}{
		{name: "not JSON", root: rootMCPResult, payload: `{`, want: "the answer does not decode, so nothing can be read at result"},
		{name: "no such key", root: "data.orbit.schema", payload: `{"data":{"orbit":{}}}`, noRoot: true},
		{name: "null at the root", root: "data.orbit.schema", payload: `{"data":{"orbit":{"schema":null}}}`, noRoot: true},
		{name: "through a list", root: "data.orbit.schema", payload: `{"data":[]}`, noRoot: true},
		{name: "no content", root: rootMCPText, payload: `{"result":{"content":"text"}}`, noRoot: true},
		{name: "no result", root: rootMCPText, payload: `{"error":{}}`, noRoot: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := rootedAnswer(tc.root, mediaJSON, []byte(tc.payload))
			if err == nil || errors.Is(err, errNoRoot) != tc.noRoot || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("rootedAnswer() error = %v, want errNoRoot %t and %q", err, tc.noRoot, tc.want)
			}
		})
	}
}

// TestElideConnections_LeavesWhatIsNotAConnection verifies that an object
// with nodes and no pageInfo, a list and a scalar are left as they are.
func TestElideConnections_LeavesWhatIsNotAConnection(t *testing.T) {
	value := map[string]any{"nodes": []any{"a"}, "count": 1.0, "list": []any{map[string]any{"pageInfo": 1}}}
	got := elideConnections(value)
	object, isObject := got.(map[string]any)
	if !isObject || object["count"] != 1.0 || len(object["nodes"].([]any)) != 1 || len(object["list"].([]any)) != 1 {
		t.Fatalf("elideConnections() = %#v, want it unchanged", got)
	}
}
