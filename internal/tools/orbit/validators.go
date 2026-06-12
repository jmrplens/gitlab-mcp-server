package orbit

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// queryTypeAlternatives are the four query_type variants the live
// GitLab.com API exposes (traversal, aggregation, neighbors, path_finding).
// The set is enforced client-side so the LLM gets actionable errors
// instead of a generic compile_error from the server.
var queryTypeAlternatives = []string{"traversal", "aggregation", "neighbors", "path_finding"}

// validateQuery performs lightweight client-side validation of an Orbit
// query DSL object before sending it to GitLab. The full JSON Schema is
// served by the live API and changes frequently during the Orbit beta;
// this validator only catches the small set of errors the live schema
// rejects with a confusing 400 (missing query_type, unknown query_type,
// traversal/aggregation without node_ids/filters, path_finding with
// fewer than 2 nodes).
//
// Reference: https://docs.gitlab.com/orbit/remote/queries/
func validateQuery(query map[string]any) (json.RawMessage, error) {
	if query == nil {
		return nil, toolutil.ErrFieldRequired("query")
	}
	queryType, ok := query["query_type"].(string)
	if !ok || strings.TrimSpace(queryType) == "" {
		return nil, fmt.Errorf("query.query_type is required and must be one of: %s", strings.Join(queryTypeAlternatives, ", "))
	}
	if !slices.Contains(queryTypeAlternatives, queryType) {
		return nil, toolutil.ErrInvalidEnum("query.query_type", queryType, queryTypeAlternatives)
	}
	switch queryType {
	case "traversal", "aggregation":
		if err := requireScopedNodes(query, queryType); err != nil {
			return nil, err
		}
	case "neighbors":
		if err := requireNeighborsShape(query); err != nil {
			return nil, err
		}
	case "path_finding":
		if err := requireAtLeastTwoNodes(query); err != nil {
			return nil, err
		}
	}
	buf, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("query must be a JSON object: %w", err)
	}
	return json.RawMessage(buf), nil
}

// requireScopedNodes returns an actionable error when a traversal or
// aggregation query lacks `node_ids` or `filters` on at least one node.
// The live API rejects such queries to prevent full edge table scans;
// surfacing the same check client-side avoids a round trip.
func requireScopedNodes(query map[string]any, queryType string) error {
	nodes := collectQueryNodes(query)
	if len(nodes) == 0 {
		return fmt.Errorf("query.node or query.nodes is required for %s queries; example: "+
			`{"id":"p","entity":"Project","filters":{"full_path":{"op":"starts_with","value":"plens1/"}}}`, queryType)
	}
	if slices.ContainsFunc(nodes, nodeHasScope) {
		return nil
	}
	// Detect the common "id_range too wide" case specifically so the
	// error message tells the user the exact issue, not just "no scope".
	for _, n := range nodes {
		if r, ok := n["id_range"].(map[string]any); ok && len(r) > 0 {
			start, hasStart := toInt64(r["start"])
			end, hasEnd := toInt64(r["end"])
			if hasStart && hasEnd && (end-start) > 100000 {
				return fmt.Errorf("query.node.id_range span (%d) exceeds the 100,000 limit; narrow the range or add a filter; example: "+
					`{"id":"p","entity":"Project","id_range":{"start":1,"end":50000},"columns":["id"]}`, end-start)
			}
		}
	}
	return fmt.Errorf("%s queries require at least one node with node_ids, filters, or id_range (span <= 100,000) to avoid a full edge table scan; example: "+
		`{"id":"p","entity":"Project","filters":{"full_path":{"op":"starts_with","value":"plens1/"}}}`, queryType)
}

// collectQueryNodes returns every node selector from a query, accepting
// either a singular top-level `node` or a `nodes` array. Non-map entries
// are silently skipped to keep the validators resilient to user JSON.
//
// The `nodes` slot accepts both `[]any` (the canonical JSON-decoded
// shape) and `[]map[string]any` (the shape produced when the caller
// builds the query programmatically in Go). Without the second branch,
// a programmatically-constructed query would be silently ignored and
// the user would see a confusing "no scope" error from the live API.
func collectQueryNodes(query map[string]any) []map[string]any {
	var out []map[string]any
	if single, singleOK := query["node"].(map[string]any); singleOK {
		out = append(out, single)
	}
	switch list := query["nodes"].(type) {
	case []any:
		for _, n := range list {
			if m, isMap := n.(map[string]any); isMap {
				out = append(out, m)
			}
		}
	case []map[string]any:
		out = append(out, list...)
	}
	return out
}

// nodeHasScope reports whether a node selector carries an explicit
// scope: non-empty node_ids, non-empty filters, or an id_range whose
// span is within the 100,000 limit. The Orbit API rejects unscoped
// queries to prevent full edge table scans; this mirrors that check
// client-side to surface a precise actionable error.
func nodeHasScope(n map[string]any) bool {
	if ids, ok := n["node_ids"].([]any); ok && len(ids) > 0 {
		return true
	}
	if ids, ok := n["node_ids"].([]int); ok && len(ids) > 0 {
		return true
	}
	if ids, ok := n["node_ids"].([]int64); ok && len(ids) > 0 {
		return true
	}
	if f, ok := n["filters"].(map[string]any); ok && len(f) > 0 {
		return true
	}
	// id_range with start/end is also a valid scoping mechanism per the
	// Orbit query language reference — but only when the span is
	// <= 100,000. The live API rejects wider ranges with a confusing
	// "require node_ids or filters" error, so we mirror the check
	// here to surface a precise actionable error.
	if r, ok := n["id_range"].(map[string]any); ok && len(r) > 0 {
		start, hasStart := toInt64(r["start"])
		end, hasEnd := toInt64(r["end"])
		if hasStart && hasEnd && (end-start) <= 100000 && (end-start) >= 0 {
			return true
		}
	}
	return false
}

// toInt64 coerces common numeric Go/JSON shapes (int, int64, float32,
// float64, json.Number) into an int64. Used by scope validators and
// the id_range span check. The json.Number branch matters when the
// caller decodes the query with `json.Decoder.UseNumber()` — without
// it, id_range span checks silently fail and scoped queries are
// rejected as "unscoped" downstream.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float32:
		return int64(n), true
	case float64:
		return int64(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		if f, err := n.Float64(); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}

// requireNeighborsShape enforces the canonical neighbors query shape:
// a top-level `node` (singular, not `nodes`) that is bounded by
// node_ids or filters, plus a `neighbors` object whose `node` string
// references the top-level node's `id` exactly.
func requireNeighborsShape(query map[string]any) error {
	node, hasNode := query["node"].(map[string]any)
	if !hasNode {
		return errors.New("neighbors queries require a top-level `node` (singular) with a bounded node selector; example: " +
			`{"query_type":"neighbors","node":{"id":"p","entity":"Project","node_ids":[1]},"neighbors":{"node":"p"}}`)
	}
	if !nodeHasScope(node) {
		return errors.New("neighbors queries require a top-level `node` bounded by node_ids, filters, or id_range (span <= 100,000); example: " +
			`{"node":{"id":"p","entity":"Project","node_ids":[1]}}`)
	}
	neighbors, ok := query["neighbors"].(map[string]any)
	if !ok {
		return errors.New("neighbors queries require a `neighbors` object; example: " +
			`{"neighbors":{"node":"p","direction":"both"}}`)
	}
	ref, hasNodeRef := neighbors["node"].(string)
	if !hasNodeRef || ref == "" {
		return errors.New("neighbors.node must be a non-empty string that references a top-level node's `id`; example: " +
			`{"neighbors":{"node":"p"}}`)
	}
	if declared, hasDeclaredID := node["id"].(string); hasDeclaredID && declared != "" && declared != ref {
		return fmt.Errorf("neighbors.node %q must match the top-level node's `id` %q; example: "+
			`{"node":{"id":"p",...},"neighbors":{"node":"p"}}`, ref, declared)
	}
	return nil
}

// requireAtLeastTwoNodes enforces the path_finding constraint that the
// query must define at least two top-level nodes (one for `from`, one
// for `to`) referenced by id from inside `path`.
func requireAtLeastTwoNodes(query map[string]any) error {
	nodes := collectQueryNodes(query)
	if len(nodes) < 2 {
		return errors.New("path_finding queries require at least two top-level node definitions " +
			"(one for path.from, one for path.to); example: " +
			`nodes=[{"id":"u","entity":"User","node_ids":[...]},{"id":"p","entity":"Project","node_ids":[...]}]`)
	}
	return nil
}

// queryType returns the query_type field of an Orbit query as a
// string, or "" when the field is missing or of an unexpected type.
// Used to label the Query output envelope regardless of the chosen
// response format.
func queryType(query map[string]any) string {
	queryTypeValue, ok := query["query_type"].(string)
	if !ok {
		return ""
	}
	return queryTypeValue
}
