// Command audit_meta_schema reports the size impact of each
// META_PARAM_SCHEMA mode on the meta-tool catalog.
//
// It registers all base + enterprise meta-tools on an in-memory MCP server,
// captures the per-action route maps, and computes
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
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	defer srv.Close()

	cfg := &config.Config{GitLabURL: srv.URL, GitLabToken: "spike-token"}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}

	// Register both base and enterprise meta-tools and capture every route map.
	server := mcp.NewServer(&mcp.Implementation{Name: "spike", Version: "0"}, &mcp.ServerOptions{PageSize: 2000})
	routes := toolutil.CaptureMetaRoutes(func() {
		tools.RegisterAllMeta(server, client, true)
	})

	// Connect once so we can retrieve the published InputSchema (the
	// "opaque" baseline) via tools/list. The schema we send equals the
	// schema served, so this is the same number a client sees.
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, cerr := server.Connect(ctx, st, nil); cerr != nil {
		return fmt.Errorf("server connect: %w", cerr)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "spike-cli", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	currentByName := map[string]map[string]any{}
	for _, t := range listed.Tools {
		if t.InputSchema == nil {
			continue
		}
		// Re-marshal/unmarshal so we measure the same JSON bytes the
		// transport ships, and so our candidate schemas use comparable
		// generic map types.
		raw, mErr := json.Marshal(t.InputSchema)
		if mErr != nil {
			return fmt.Errorf("marshal input schema for %s: %w", t.Name, mErr)
		}
		var m map[string]any
		if uErr := json.Unmarshal(raw, &m); uErr != nil {
			return fmt.Errorf("unmarshal input schema for %s: %w", t.Name, uErr)
		}
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

	names := make([]string, 0, len(routes))
	for n := range routes {
		names = append(names, n)
	}
	sort.Strings(names)

	totalOpaque, totalFull, totalCompact := 0, 0, 0

	for _, name := range names {
		rmap := routes[name]
		opaque := currentByName[name]
		opaqueJSON, mErr := json.Marshal(opaque)
		if mErr != nil {
			return fmt.Errorf("marshal opaque schema for %s: %w", name, mErr)
		}

		// Reuse the production schema builder so the audit always reflects
		// what the server actually publishes for each mode.
		full := toolutil.BuildMetaToolSchema(rmap, toolutil.MetaParamSchemaFull)
		compact := toolutil.BuildMetaToolSchema(rmap, toolutil.MetaParamSchemaCompact)

		fullJSON, mErr := json.Marshal(full)
		if mErr != nil {
			return fmt.Errorf("marshal full schema for %s: %w", name, mErr)
		}
		compactJSON, mErr := json.Marshal(compact)
		if mErr != nil {
			return fmt.Errorf("marshal compact schema for %s: %w", name, mErr)
		}

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
	return nil
}

// buildOneOfSchema constructs a JSON Schema with a oneOf branch per action.
// When compact is true, each branch's params object only enumerates property
// names with their declared type (descriptions, $defs and required dropped),
// and additionalProperties is left true.
//
// Removed: this function and its helpers (compactSchema, resolveTopRef) were
// duplicates of toolutil.BuildMetaToolSchema. Callers now invoke
// toolutil.BuildMetaToolSchema directly so the audit always tracks production
// behavior; see the loop above.

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
