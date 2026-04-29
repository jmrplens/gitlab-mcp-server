// Command audit_meta_schema reports the size impact of each
// META_PARAM_SCHEMA mode on the meta-tool catalog.
//
// It registers all base + enterprise meta-tools on an in-memory MCP server,
// retrieves the per-action route maps via toolutil.MetaRoutes(), and computes
// three candidate schema sizes per tool:
//
//   - opaque:  current production schema (action enum + params:any).
//   - full:    oneOf with per-action params reusing the InputSchema captured
//     by RouteAction[T,R] (descriptions, jsonschema tags, $defs).
//   - compact: oneOf with per-action params reduced to property names + types
//     (descriptions stripped, additionalProperties:true, no required).
//
// Usage: go run ./cmd/audit_meta_schema/
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	defer srv.Close()

	cfg := &config.Config{GitLabURL: srv.URL, GitLabToken: "spike-token"}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client: %v\n", err)
		os.Exit(1)
	}

	// Register both base and enterprise meta-tools so MetaRoutes() returns
	// every route map. Use a fresh registry to avoid cross-run pollution.
	toolutil.ClearMetaRoutes()
	server := mcp.NewServer(&mcp.Implementation{Name: "spike", Version: "0"}, &mcp.ServerOptions{PageSize: 2000})
	tools.RegisterAllMeta(server, client, true)

	// Connect once so we can retrieve the published InputSchema (the
	// "opaque" baseline) via tools/list. The schema we send equals the
	// schema served, so this is the same number a client sees.
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		os.Exit(1)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "spike-cli", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list tools: %v\n", err)
		os.Exit(1)
	}
	currentByName := map[string]map[string]any{}
	for _, t := range listed.Tools {
		if t.InputSchema == nil {
			continue
		}
		// Re-marshal/unmarshal so we measure the same JSON bytes the
		// transport ships, and so our candidate schemas use comparable
		// generic map types.
		raw, _ := json.Marshal(t.InputSchema)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		currentByName[t.Name] = m
	}

	type row struct {
		name      string
		actions   int
		opaque    int
		full      int
		compact   int
		fullDelta int
	}
	rows := []row{}

	routes := toolutil.MetaRoutes()
	names := make([]string, 0, len(routes))
	for n := range routes {
		names = append(names, n)
	}
	sort.Strings(names)

	totalOpaque, totalFull, totalCompact := 0, 0, 0

	for _, name := range names {
		rmap := routes[name]
		opaque := currentByName[name]
		opaqueJSON, _ := json.Marshal(opaque)

		full := buildOneOfSchema(rmap, false /*compact=*/)
		compact := buildOneOfSchema(rmap, true)

		fullJSON, _ := json.Marshal(full)
		compactJSON, _ := json.Marshal(compact)

		r := row{
			name:      name,
			actions:   len(rmap),
			opaque:    len(opaqueJSON),
			full:      len(fullJSON),
			compact:   len(compactJSON),
			fullDelta: len(fullJSON) - len(opaqueJSON),
		}
		rows = append(rows, r)
		totalOpaque += r.opaque
		totalFull += r.full
		totalCompact += r.compact
	}

	// Sort by full size descending to highlight the heaviest meta-tools.
	sort.Slice(rows, func(i, j int) bool { return rows[i].full > rows[j].full })

	fmt.Println("============================================================")
	fmt.Println(" Meta-tool InputSchema sizing spike")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("%-46s %7s %10s %10s %10s %10s\n",
		"meta-tool", "actions", "opaque", "full", "compact", "Δ full")
	fmt.Println(repeat("-", 96))
	for _, r := range rows {
		fmt.Printf("%-46s %7d %10s %10s %10s %+10s\n",
			r.name, r.actions,
			human(r.opaque), human(r.full), human(r.compact),
			human(r.fullDelta))
	}
	fmt.Println(repeat("-", 96))
	fmt.Printf("%-46s %7s %10s %10s %10s\n",
		"TOTAL", "",
		human(totalOpaque), human(totalFull), human(totalCompact))
	fmt.Println()
	fmt.Printf("Full / opaque   ratio: %.1fx\n", float64(totalFull)/float64(totalOpaque))
	fmt.Printf("Compact / opaque ratio: %.1fx\n", float64(totalCompact)/float64(totalOpaque))
}

// buildOneOfSchema constructs a JSON Schema with a oneOf branch per action.
// When compact is true, each branch's params object only enumerates property
// names with their declared type (descriptions, $defs and required dropped),
// and additionalProperties is left true.
func buildOneOfSchema(rmap toolutil.ActionMap, compact bool) map[string]any {
	actionNames := make([]string, 0, len(rmap))
	for n := range rmap {
		actionNames = append(actionNames, n)
	}
	sort.Strings(actionNames)

	branches := make([]any, 0, len(actionNames))
	for _, action := range actionNames {
		route := rmap[action]
		params := route.InputSchema
		if compact && params != nil {
			params = compactSchema(params)
		}
		if params == nil {
			params = map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			}
		}
		branch := map[string]any{
			"properties": map[string]any{
				"action": map[string]any{"const": action},
				"params": params,
			},
			"required": []any{"action"},
		}
		branches = append(branches, branch)
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": actionNames,
			},
			"params": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		},
		"required": []any{"action"},
		"oneOf":    branches,
	}
}

// compactSchema returns a copy of the given JSON Schema with descriptions
// removed and only property name/type retained. Inlines top-level $ref by
// looking it up in $defs once, then drops $defs entirely. Best-effort: any
// shape we don't recognise is preserved verbatim.
func compactSchema(s map[string]any) map[string]any {
	if s == nil {
		return nil
	}
	resolved := resolveTopRef(s)
	props, _ := resolved["properties"].(map[string]any)
	if props == nil {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	compactProps := make(map[string]any, len(props))
	for k, v := range props {
		pm, ok := v.(map[string]any)
		if !ok {
			compactProps[k] = map[string]any{}
			continue
		}
		entry := map[string]any{}
		if t, ok := pm["type"]; ok {
			entry["type"] = t
		}
		if e, ok := pm["enum"]; ok {
			entry["enum"] = e
		}
		compactProps[k] = entry
	}
	return map[string]any{
		"type":                 "object",
		"properties":           compactProps,
		"additionalProperties": true,
	}
}

// resolveTopRef returns the schema with a top-level "$ref" replaced by the
// referenced $defs entry. If no top-level $ref is present, returns s.
func resolveTopRef(s map[string]any) map[string]any {
	ref, _ := s["$ref"].(string)
	if ref == "" {
		return s
	}
	defs, _ := s["$defs"].(map[string]any)
	if defs == nil {
		return s
	}
	// $ref like "#/$defs/Foo"
	const prefix = "#/$defs/"
	if len(ref) <= len(prefix) || ref[:len(prefix)] != prefix {
		return s
	}
	target, _ := defs[ref[len(prefix):]].(map[string]any)
	if target == nil {
		return s
	}
	return target
}

func human(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}
