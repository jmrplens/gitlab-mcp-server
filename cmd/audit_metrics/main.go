// Command audit_metrics generates a comprehensive metrics summary for the
// gitlab-mcp-server MCP server. It creates in-memory MCP servers to count tools,
// meta-tools, resources, and prompts at runtime — the only reliable counting
// method. It also scans the filesystem for sub-packages, source files, and
// test files.
//
// Usage:
//
//	go run ./cmd/audit_metrics/
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	auditServerName = "audit-metrics"
	auditClientName = "audit-metrics-client"
	auditVersion    = "0.0.1"
)

func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	defer srv.Close()

	cfg := &config.Config{
		GitLabURL:   srv.URL,
		GitLabToken: "audit-token",
	}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
	}

	individualTools := listServerTools(client, false, false)
	metaBase := listServerTools(client, true, false)
	metaEnterprise := listServerTools(client, true, true)
	staticResources, templateResources := countResources(client)
	resourceCount := staticResources + templateResources + 1 // +1 for workspace_roots
	promptCount := countPrompts(client)

	subPkgs := countSubPackages()
	srcFiles, testFiles := countSourceFiles()

	// Classify individual tools by domain.
	samplingCount := 0
	elicitationCount := 0
	domains := map[string]int{}
	for _, t := range individualTools {
		parts := strings.SplitN(t.Name, "_", 3) // gitlab_{domain}_{action}
		if len(parts) >= 2 {
			domains[parts[1]]++
		}
		if strings.HasPrefix(t.Name, "gitlab_analyze_") || strings.HasPrefix(t.Name, "gitlab_summarize_") ||
			strings.HasPrefix(t.Name, "gitlab_generate_") || strings.HasPrefix(t.Name, "gitlab_review_mr_security") ||
			strings.HasPrefix(t.Name, "gitlab_find_technical_debt") {
			samplingCount++
		}
		if strings.HasPrefix(t.Name, "gitlab_interactive_") {
			elicitationCount++
		}
	}

	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println("  gitlab-mcp-server — MCP Server Metrics Audit")
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println()

	fmt.Println("## Core Metrics")
	fmt.Println()
	printRow("Individual MCP tools", len(individualTools))
	printRow("Meta-tools (base)", len(metaBase))
	printRow("Meta-tools (enterprise)", len(metaEnterprise))
	printRow("Enterprise-only meta-tools", len(metaEnterprise)-len(metaBase))
	printRow("MCP Resources (total)", resourceCount)
	printRow("  Static resources", staticResources)
	printRow("  Resource templates", templateResources)
	printRow("  Workspace roots", 1)
	printRow("MCP Prompts", promptCount)
	fmt.Println()

	fmt.Println("## Tool Categories")
	fmt.Println()
	printRow("Sampling tools", samplingCount)
	printRow("Elicitation tools", elicitationCount)
	printRow("Standard tools", len(individualTools)-samplingCount-elicitationCount)
	fmt.Println()

	fmt.Println("## Meta-Tool Schema Modes")
	fmt.Println()
	printMetaSchemaModes(client)
	fmt.Println()

	fmt.Println("## Codebase Metrics")
	fmt.Println()
	printRow("Tool sub-packages", subPkgs)
	printRow("Source files (.go)", srcFiles)
	printRow("Test files (_test.go)", testFiles)
	fmt.Println()

	fmt.Println("## Domain Breakdown (top 20)")
	fmt.Println()
	printDomainTable(domains)
	fmt.Println()

	fmt.Println("## Meta-tools List")
	fmt.Println()
	fmt.Println("### Base (" + strconv.Itoa(len(metaBase)) + ")")
	for _, t := range metaBase {
		fmt.Printf("  - %s\n", t.Name)
	}
	fmt.Println()
	fmt.Println("### Enterprise-only (" + strconv.Itoa(len(metaEnterprise)-len(metaBase)) + ")")
	baseNames := map[string]bool{}
	for _, t := range metaBase {
		baseNames[t.Name] = true
	}
	for _, t := range metaEnterprise {
		if !baseNames[t.Name] {
			fmt.Printf("  - %s\n", t.Name)
		}
	}
}

// listServerTools registers tools on an in-memory MCP server and returns
// the full tool list. When meta is true, meta-tools are registered.
// Enterprise controls whether Enterprise/Premium meta-tools are included.
func listServerTools(client *gitlabclient.Client, meta, enterprise bool) []*mcp.Tool {
	opts := &mcp.ServerOptions{PageSize: 2000}
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, opts)

	if meta {
		tools.RegisterAllMeta(server, client, enterprise)
	} else {
		tools.RegisterAll(server, client, true)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: auditClientName, Version: auditVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListTools: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
	}
	return result.Tools
}

// countResources registers all MCP resources and returns static and template counts.
// This includes resources from Register() and RegisterWorkflowGuides().
// Workspace roots (+1) are counted separately because they need a roots.Manager.
func countResources(client *gitlabclient.Client) (static, templates int) {
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, nil)
	metaRoutes := toolutil.CaptureMetaRoutes(func() {
		tools.RegisterAllMeta(server, client, false)
	})
	resources.Register(server, client)
	resources.RegisterMetaSchemaResources(server, metaRoutes)
	resources.RegisterWorkflowGuides(server)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect (resources): %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: auditClientName, Version: auditVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect (resources): %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	res, err := session.ListResources(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListResources: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
	}
	static = len(res.Resources)

	tpl, tplErr := session.ListResourceTemplates(ctx, nil)
	if tplErr != nil {
		fmt.Fprintf(os.Stderr, "ListResourceTemplates: %v\n", tplErr)
		os.Exit(1)
	}
	templates = len(tpl.ResourceTemplates)
	return static, templates
}

// countPrompts registers all MCP prompts and returns the count.
func countPrompts(client *gitlabclient.Client) int {
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, nil)
	prompts.Register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect (prompts): %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: auditClientName, Version: auditVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect (prompts): %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	result, err := session.ListPrompts(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListPrompts: %v\n", err)
		os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
	}
	return len(result.Prompts)
}

// countSubPackages counts directories under internal/tools/ that contain
// a register.go file (the convention for tool sub-packages).
func countSubPackages() int {
	toolsDir := filepath.Join("internal", "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadDir %s: %v\n", toolsDir, err)
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		regFile := filepath.Join(toolsDir, e.Name(), "register.go")
		if _, statErr := os.Stat(regFile); statErr == nil {
			count++
		}
	}
	return count
}

// countSourceFiles walks the internal/ directory and counts .go source files
// and _test.go test files separately.
func countSourceFiles() (src, test int) {
	err := filepath.Walk("internal", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			test++
		} else {
			src++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Walk: %v\n", err)
	}
	return src, test
}

// printRow prints a metric row with aligned formatting.
func printRow(label string, value int) {
	fmt.Printf("  %-30s %d\n", label, value)
}

// printMetaSchemaModes reports the active META_PARAM_SCHEMA mode and the
// total meta-tool InputSchema byte size each mode would produce. Useful
// for ops to size the impact of META_PARAM_SCHEMA before flipping it.
func printMetaSchemaModes(client *gitlabclient.Client) {
	// Read META_PARAM_SCHEMA directly: config.Load() requires GITLAB_URL +
	// GITLAB_TOKEN and would silently fall back to "opaque" if they are
	// missing, misreporting the active mode in environments where this
	// tool is invoked without full GitLab credentials (e.g., audits).
	active := strings.ToLower(strings.TrimSpace(os.Getenv("META_PARAM_SCHEMA")))
	switch active {
	case config.MetaParamSchemaCompact, config.MetaParamSchemaFull, config.MetaParamSchemaOpaque:
		// recognized — keep as-is
	default:
		active = config.MetaParamSchemaOpaque
	}
	fmt.Printf("  Active mode (env): %s\n\n", active)
	fmt.Printf("  %-12s %12s\n", "mode", "total bytes")
	fmt.Printf("  %-12s %12s\n", strings.Repeat("-", 12), strings.Repeat("-", 12))
	for _, mode := range []string{"opaque", "compact", "full"} {
		tools.SetMetaParamSchema(mode)
		metaTools := listServerTools(client, true, true)
		total := 0
		for _, t := range metaTools {
			if t.InputSchema == nil {
				continue
			}
			raw, err := json.Marshal(t.InputSchema)
			if err != nil {
				continue
			}
			total += len(raw)
		}
		fmt.Printf("  %-12s %12d\n", mode, total)
	}
	tools.SetMetaParamSchema("opaque")
}

// printDomainTable prints the top 20 tool domains sorted by count.
func printDomainTable(domains map[string]int) {
	type kv struct {
		key string
		val int
	}
	var sorted []kv
	for k, v := range domains {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].val != sorted[j].val {
			return sorted[i].val > sorted[j].val
		}
		return sorted[i].key < sorted[j].key
	})
	limit := min(20, len(sorted))
	fmt.Printf("  %-25s %s\n", "Domain", "Tools")
	fmt.Printf("  %-25s %s\n", strings.Repeat("-", 25), strings.Repeat("-", 5))
	for _, kv := range sorted[:limit] {
		fmt.Printf("  %-25s %d\n", kv.key, kv.val)
	}
	if len(sorted) > limit {
		fmt.Printf("  ... and %d more domains\n", len(sorted)-limit)
	}
}
