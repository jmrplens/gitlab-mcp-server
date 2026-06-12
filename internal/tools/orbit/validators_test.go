package orbit

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRequireScopedNodes_RejectsZeroNodes verifies that [requireScopedNodes]
// surfaces an actionable error when the query has no top-level `node` or
// `nodes` entries. The live API would otherwise reject the request with a
// confusing compile_error; the client-side check returns a precise message
// with an example payload.
func TestRequireScopedNodes_RejectsZeroNodes(t *testing.T) {
	err := requireScopedNodes(map[string]any{
		"query_type": "traversal",
	}, "traversal")
	if err == nil {
		t.Fatal("requireScopedNodes() error = nil, want missing-node error")
	}
	if !strings.Contains(err.Error(), "query.node or query.nodes is required") {
		t.Fatalf("requireScopedNodes() error = %q, want actionable missing-node message", err)
	}
}

// TestRequireScopedNodes_DetectsIdRangeSpanTooWide verifies that
// [requireScopedNodes] returns the precise id_range span error when the
// user supplies an id_range whose span exceeds the 100,000 limit. This
// matches the live API's "require node_ids or filters" rejection so the
// LLM can see the exact cause.
func TestRequireScopedNodes_DetectsIdRangeSpanTooWide(t *testing.T) {
	err := requireScopedNodes(map[string]any{
		"query_type": "traversal",
		"node": map[string]any{
			"id":       "p",
			"entity":   "Project",
			"id_range": map[string]any{"start": 1, "end": 500000},
		},
	}, "traversal")
	if err == nil {
		t.Fatal("requireScopedNodes() error = nil, want id_range span error")
	}
	if !strings.Contains(err.Error(), "exceeds the 100,000 limit") {
		t.Fatalf("requireScopedNodes() error = %q, want id_range span error", err)
	}
}

// TestNodeHasScope_AcceptsAllScopeShapes verifies that [nodeHasScope]
// returns true for every valid scope shape: node_ids as []any, []int, and
// []int64, non-empty filters, and id_range with a span within the
// 100,000 limit. Each shape is treated as a valid scope so the Orbit
// query validator accepts the query without falling through to the
// "no scope" error.
func TestNodeHasScope_AcceptsAllScopeShapes(t *testing.T) {
	tests := []struct {
		name string
		node map[string]any
	}{
		{
			name: "node_ids as []any",
			node: map[string]any{"node_ids": []any{1, 2, 3}},
		},
		{
			name: "node_ids as []int",
			node: map[string]any{"node_ids": []int{1, 2, 3}},
		},
		{
			name: "node_ids as []int64",
			node: map[string]any{"node_ids": []int64{1, 2, 3}},
		},
		{
			name: "non-empty filters",
			node: map[string]any{"filters": map[string]any{"id": 1}},
		},
		{
			name: "id_range within span limit",
			node: map[string]any{"id_range": map[string]any{"start": 1, "end": 50}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !nodeHasScope(tt.node) {
				t.Fatalf("nodeHasScope(%+v) = false, want true", tt.node)
			}
		})
	}
}

// TestNodeHasScope_RejectsInvalidIdRange verifies that [nodeHasScope]
// returns false for id_range maps whose span is exactly at the boundary
// (the spec uses <= 100000; the branch only succeeds with hasStart and
// hasEnd). An id_range with a single key (no `end`) should also be
// rejected so the validator does not silently accept malformed input.
func TestNodeHasScope_RejectsInvalidIdRange(t *testing.T) {
	if nodeHasScope(map[string]any{"id_range": map[string]any{"start": 1}}) {
		t.Fatal("nodeHasScope() with id_range{start} only = true, want false")
	}
	if nodeHasScope(map[string]any{"id_range": map[string]any{"end": 100}}) {
		t.Fatal("nodeHasScope() with id_range{end} only = true, want false")
	}
}

// TestNodeHasScope_RejectsEmptyAndOversizedScopes verifies that
// [nodeHasScope] returns false for empty node_ids, empty filters, and an
// id_range whose span exceeds the 100,000 limit. These shapes must not
// count as scope so the Orbit API's full edge table scan guard fires.
func TestNodeHasScope_RejectsEmptyAndOversizedScopes(t *testing.T) {
	tests := []struct {
		name string
		node map[string]any
	}{
		{name: "empty []any node_ids", node: map[string]any{"node_ids": []any{}}},
		{name: "empty []int node_ids", node: map[string]any{"node_ids": []int{}}},
		{name: "empty []int64 node_ids", node: map[string]any{"node_ids": []int64{}}},
		{name: "empty filters", node: map[string]any{"filters": map[string]any{}}},
		{name: "id_range span too wide", node: map[string]any{"id_range": map[string]any{"start": 1, "end": 200000}}},
		{name: "no scope at all", node: map[string]any{"columns": []string{"id"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if nodeHasScope(tt.node) {
				t.Fatalf("nodeHasScope(%+v) = true, want false", tt.node)
			}
		})
	}
}

// TestToInt64_CoercesAllSupportedTypes verifies that [toInt64] handles
// every numeric shape the Orbit query DSL can produce: int, int64,
// float64, and float32. Each shape must round-trip to the same int64
// value so the id_range span check sees consistent numbers regardless
// of the JSON decoder used upstream.
func TestToInt64_CoercesAllSupportedTypes(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{name: "int", in: int(7), want: 7, ok: true},
		{name: "int64", in: int64(7), want: 7, ok: true},
		{name: "float64", in: float64(7), want: 7, ok: true},
		{name: "float32", in: float32(7), want: 7, ok: true},
		{name: "string", in: "7", want: 0, ok: false},
		{name: "nil", in: nil, want: 0, ok: false},
		{name: "bool", in: true, want: 0, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toInt64(tt.in)
			if ok != tt.ok {
				t.Fatalf("toInt64(%v) ok = %t, want %t", tt.in, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("toInt64(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestRequireNeighborsShape_RejectsMissingNode verifies that
// [requireNeighborsShape] surfaces a precise error when the top-level
// `node` field is absent. The live API requires a singular top-level
// `node` (not `nodes`); the client-side check matches.
func TestRequireNeighborsShape_RejectsMissingNode(t *testing.T) {
	err := requireNeighborsShape(map[string]any{
		"query_type": "neighbors",
		"neighbors":  map[string]any{"node": "p"},
	})
	if err == nil {
		t.Fatal("requireNeighborsShape() error = nil, want missing-node error")
	}
	if !strings.Contains(err.Error(), "require a top-level `node`") {
		t.Fatalf("requireNeighborsShape() error = %q, want missing-node message", err)
	}
}

// TestRequireNeighborsShape_RejectsMissingNeighborsObject verifies that
// [requireNeighborsShape] returns an actionable error when the
// top-level `neighbors` object is absent. The live API requires a
// `neighbors` object with at least a `node` field that references a
// top-level node id.
func TestRequireNeighborsShape_RejectsMissingNeighborsObject(t *testing.T) {
	err := requireNeighborsShape(map[string]any{
		"query_type": "neighbors",
		"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
	})
	if err == nil {
		t.Fatal("requireNeighborsShape() error = nil, want missing-neighbors-object error")
	}
	if !strings.Contains(err.Error(), "require a `neighbors` object") {
		t.Fatalf("requireNeighborsShape() error = %q, want missing-neighbors-object message", err)
	}
}

// TestRequireNeighborsShape_RejectsMissingNodeRef verifies that
// [requireNeighborsShape] returns a precise error when the
// `neighbors` object does not have a string `node` field that
// references a top-level node id. The live API rejects neighbors
// expansions that do not point at a defined node.
func TestRequireNeighborsShape_RejectsMissingNodeRef(t *testing.T) {
	err := requireNeighborsShape(map[string]any{
		"query_type": "neighbors",
		"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		"neighbors":  map[string]any{"direction": "both"},
	})
	if err == nil {
		t.Fatal("requireNeighborsShape() error = nil, want missing-node-ref error")
	}
	if !strings.Contains(err.Error(), "neighbors.node must be a non-empty string") {
		t.Fatalf("requireNeighborsShape() error = %q, want missing-node-ref message", err)
	}
}

// TestRequireNeighborsShape_AcceptsValidShape verifies that
// [requireNeighborsShape] returns nil for the canonical neighbors query
// shape: a top-level `node` with node_ids, plus a `neighbors` object
// that references it by id. This guards against a false positive in
// the rejection paths above.
func TestRequireNeighborsShape_AcceptsValidShape(t *testing.T) {
	if err := requireNeighborsShape(map[string]any{
		"query_type": "neighbors",
		"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		"neighbors":  map[string]any{"node": "p", "direction": "both"},
	}); err != nil {
		t.Fatalf("requireNeighborsShape() error = %v, want nil", err)
	}
}

// TestRequireAtLeastTwoNodes_RejectsEmptyNodes verifies that
// [requireAtLeastTwoNodes] surfaces the actionable error when a
// path_finding query has no top-level node selectors at all (neither
// `node` nor `nodes`). The live API requires two top-level nodes to
// anchor the path's `from` and `to` aliases.
func TestRequireAtLeastTwoNodes_RejectsEmptyNodes(t *testing.T) {
	err := requireAtLeastTwoNodes(map[string]any{
		"query_type": "path_finding",
		"path":       map[string]any{"type": "shortest", "from": "u", "to": "p", "max_depth": 1},
	})
	if err == nil {
		t.Fatal("requireAtLeastTwoNodes() error = nil, want at-least-two-nodes error")
	}
	if !strings.Contains(err.Error(), "at least two top-level node definitions") {
		t.Fatalf("requireAtLeastTwoNodes() error = %q, want at-least-two-nodes message", err)
	}
}

// TestRequireAtLeastTwoNodes_AcceptsTwoNodes verifies that
// [requireAtLeastTwoNodes] returns nil when the path_finding query
// defines at least two top-level node selectors. This covers the
// success-return branch that the rejection-only test does not.
func TestRequireAtLeastTwoNodes_AcceptsTwoNodes(t *testing.T) {
	err := requireAtLeastTwoNodes(map[string]any{
		"query_type": "path_finding",
		"nodes": []any{
			map[string]any{"id": "u", "entity": "User", "node_ids": []int{1}},
			map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		},
		"path": map[string]any{"type": "shortest", "from": "u", "to": "p", "max_depth": 1},
	})
	if err != nil {
		t.Fatalf("requireAtLeastTwoNodes() error = %v, want nil", err)
	}
}

// TestValidateQuery_RejectsWhitespaceQueryType verifies that
// [validateQuery] rejects a query_type that is present but consists
// solely of whitespace. The live API treats whitespace-only types as
// missing; the client-side check matches by trimming before the
// "required" check fires.
func TestValidateQuery_RejectsWhitespaceQueryType(t *testing.T) {
	_, err := validateQuery(map[string]any{"query_type": "   "})
	if err == nil {
		t.Fatal("validateQuery() error = nil, want whitespace-only query_type error")
	}
	if !strings.Contains(err.Error(), "query_type is required") {
		t.Fatalf("validateQuery() error = %q, want required query_type message", err)
	}
}

// TestValidateQuery_RejectsNeighborsShape verifies that [validateQuery]
// dispatches the neighbors branch in its switch and surfaces the
// canonical neighbors-shape error from [requireNeighborsShape].
// Coverage must reach the switch's `case "neighbors"` arm, which the
// existing happy-path tests do not exercise.
func TestValidateQuery_RejectsNeighborsShape(t *testing.T) {
	_, err := validateQuery(map[string]any{
		"query_type": "neighbors",
		"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		// Missing `neighbors` object so the validator rejects the shape.
	})
	if err == nil {
		t.Fatal("validateQuery() error = nil, want neighbors shape error")
	}
	if !strings.Contains(err.Error(), "require a `neighbors` object") {
		t.Fatalf("validateQuery() error = %q, want neighbors shape message", err)
	}
}

// TestRequireNeighborsShape_RejectsUnboundedTopLevelNode verifies that
// [requireNeighborsShape] surfaces a precise error when the top-level
// `node` is present but not bounded by node_ids/filters/id_range.
// The docstring and the Orbit query language reference both require
// a bounded selector; without this check the live API returns a
// confusing generic 400.
func TestRequireNeighborsShape_RejectsUnboundedTopLevelNode(t *testing.T) {
	err := requireNeighborsShape(map[string]any{
		"query_type": "neighbors",
		"node":       map[string]any{"id": "p", "entity": "Project"},
		"neighbors":  map[string]any{"node": "p"},
	})
	if err == nil {
		t.Fatal("requireNeighborsShape() error = nil, want unbounded-node error")
	}
	if !strings.Contains(err.Error(), "bounded by node_ids, filters, or id_range") {
		t.Fatalf("requireNeighborsShape() error = %q, want unbounded-node message", err)
	}
}

// TestRequireNeighborsShape_RejectsNodeRefMismatch verifies that
// [requireNeighborsShape] surfaces a precise error when
// `neighbors.node` does not reference the top-level `node.id`.
// Mirrors the live API's compile_error for the same case so the
// LLM gets a precise actionable error before the round trip.
func TestRequireNeighborsShape_RejectsNodeRefMismatch(t *testing.T) {
	err := requireNeighborsShape(map[string]any{
		"query_type": "neighbors",
		"node":       map[string]any{"id": "p", "entity": "Project", "node_ids": []int{1}},
		"neighbors":  map[string]any{"node": "other"},
	})
	if err == nil {
		t.Fatal("requireNeighborsShape() error = nil, want node-ref mismatch error")
	}
	if !strings.Contains(err.Error(), "must match the top-level node's `id`") {
		t.Fatalf("requireNeighborsShape() error = %q, want node-ref mismatch message", err)
	}
}

// TestCollectQueryNodes_AcceptsTypedSliceShape verifies that
// [collectQueryNodes] recognises a `nodes` slice of `map[string]any`
// values (the shape produced when callers build the query
// programmatically in Go) in addition to the canonical `[]any`
// shape that json.Unmarshal produces. Without the second branch a
// programmatically-built query would be silently ignored and the
// live API would reject it as unscoped.
func TestCollectQueryNodes_AcceptsTypedSliceShape(t *testing.T) {
	nodes := collectQueryNodes(map[string]any{
		"nodes": []map[string]any{
			{"id": "u", "entity": "User", "node_ids": []int{1}},
			{"id": "p", "entity": "Project", "node_ids": []int{2}},
		},
	})
	if len(nodes) != 2 {
		t.Fatalf("collectQueryNodes() len = %d, want 2 (typed slice shape)", len(nodes))
	}
	if nodes[0]["id"] != "u" || nodes[1]["id"] != "p" {
		t.Fatalf("collectQueryNodes() = %+v, want u and p in order", nodes)
	}
}

// TestToInt64_CoercesJSONNumber verifies that [toInt64] understands
// the [json.Number] type emitted by `json.Decoder.UseNumber()`. The
// live API can be configured to decode the query body with UseNumber
// to avoid float64 precision loss on large ids; without this branch
// id_range span checks silently fail and scoped queries are rejected
// downstream as "unscoped".
func TestToInt64_CoercesJSONNumber(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{name: "integer json.Number", in: json.Number("42"), want: 42, ok: true},
		{name: "float json.Number", in: json.Number("42.7"), want: 42, ok: true},
		{name: "empty json.Number", in: json.Number(""), want: 0, ok: false},
		{name: "non-numeric json.Number", in: json.Number("not-a-number"), want: 0, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toInt64(tt.in)
			if ok != tt.ok {
				t.Fatalf("toInt64(%v) ok = %t, want %t", tt.in, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("toInt64(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
