// Command audit_tokens measures the LLM context window overhead of all
// registered MCP tool definitions. It creates in-memory MCP servers in both
// individual and meta-tool modes, serializes tool definitions to JSON, and
// estimates token counts using a byte-based heuristic (bytes / 4).
//
// Usage:
//
//	go run ./cmd/audit_tokens/
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Token audit constants define the in-memory MCP session identity and the
// byte-to-token conversion heuristic used by the report.
const (
	serverName  = "audit-tokens"
	clientName  = "audit-tokens-client"
	auditVer    = "0.0.1"
	bytesPerTok = 4 // Approximate: 1 token ≈ 4 bytes for English text (cl100k_base average)
)

// toolTokenInfo stores the serialized size estimate for one MCP tool.
type toolTokenInfo struct {
	Name       string
	Domain     string
	Tokens     int
	Bytes      int
	Components toolComponentBytes
}

// toolComponentBytes stores byte totals for the expensive top-level fields in
// an advertised MCP tool definition.
type toolComponentBytes struct {
	Description  int
	InputSchema  int
	OutputSchema int
	Annotations  int
	Icons        int
	Other        int
}

// main creates the mock GitLab-backed client, measures all MCP catalog modes,
// and prints token overhead comparisons for tools, resources, and prompts.
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

	individualTools := listTools(client, false, true)
	metaBaseTools := listTools(client, true, false)
	metaEnterpriseTools := listTools(client, true, true)

	individualInfo := measureTools(individualTools)
	metaBaseInfo := measureTools(metaBaseTools)
	metaEnterpriseInfo := measureTools(metaEnterpriseTools)

	individualResourceTokens := measureResources(client, false)
	metaResourceTokens := measureResources(client, true)
	promptTokens := measurePrompts(client)

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("  gitlab-mcp-server — Token Overhead Audit")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println()

	// Mode comparison
	indTotal := totalTokens(individualInfo)
	metaTotal := totalTokens(metaBaseInfo)
	metaEntTotal := totalTokens(metaEnterpriseInfo)

	fmt.Println("## Mode Comparison")
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Mode\tTools\tTokens\tBytes\n")
	fmt.Fprintf(tw, "  ────\t─────\t──────\t─────\n")
	fmt.Fprintf(tw, "  Individual (all)\t%d\t%s\t%s\n", len(individualInfo), fmtNum(indTotal), fmtNum(indTotal*bytesPerTok))
	fmt.Fprintf(tw, "  Meta-tools (base)\t%d\t%s\t%s\n", len(metaBaseInfo), fmtNum(metaTotal), fmtNum(metaTotal*bytesPerTok))
	fmt.Fprintf(tw, "  Meta-tools (enterprise)\t%d\t%s\t%s\n", len(metaEnterpriseInfo), fmtNum(metaEntTotal), fmtNum(metaEntTotal*bytesPerTok))
	_ = tw.Flush()
	fmt.Println()

	if indTotal > 0 {
		savings := float64(indTotal-metaTotal) / float64(indTotal) * 100
		fmt.Printf("  Meta-tools reduce token overhead by %.1f%% vs individual mode\n", savings)
		fmt.Println()
	}

	// Shared overhead (resources + prompts)
	fmt.Println("## Shared Overhead (Resources + Prompts)")
	fmt.Println()
	fmt.Printf("  Resources (individual): ~%s tokens (%s bytes)\n", fmtNum(individualResourceTokens), fmtNum(individualResourceTokens*bytesPerTok))
	fmt.Printf("  Resources (meta-tools): ~%s tokens (%s bytes)\n", fmtNum(metaResourceTokens), fmtNum(metaResourceTokens*bytesPerTok))
	fmt.Printf("  Prompts:   ~%s tokens (%s bytes)\n", fmtNum(promptTokens), fmtNum(promptTokens*bytesPerTok))
	fmt.Printf("  Individual total: ~%s tokens\n", fmtNum(individualResourceTokens+promptTokens))
	fmt.Printf("  Meta-tool total:  ~%s tokens\n", fmtNum(metaResourceTokens+promptTokens))
	fmt.Println()

	// Top 30 individual tools by token cost
	fmt.Println("## Top 30 Individual Tools by Token Cost")
	fmt.Println()
	printTopTools(individualInfo, 30)

	// Top 20 meta-tools by token cost
	fmt.Println("## Meta-Tools by Token Cost (base)")
	fmt.Println()
	printTopTools(metaBaseInfo, len(metaBaseInfo))

	fmt.Println("## Tool Definition Components (meta-tools enterprise)")
	fmt.Println()
	printComponentTotals(metaEnterpriseInfo)

	// Domain aggregation for individual tools
	fmt.Println("## Domain Totals (Individual Mode, Top 20)")
	fmt.Println()
	printDomainTotals(individualInfo, 20)

	// Grand total
	fmt.Println("## Grand Total (what an LLM sees)")
	fmt.Println()
	fmt.Printf("  Individual mode: ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(indTotal), fmtNum(individualResourceTokens+promptTokens), fmtNum(indTotal+individualResourceTokens+promptTokens))
	fmt.Printf("  Meta-tool mode:  ~%s tokens (tools) + ~%s tokens (resources+prompts) = ~%s tokens\n",
		fmtNum(metaTotal), fmtNum(metaResourceTokens+promptTokens), fmtNum(metaTotal+metaResourceTokens+promptTokens))
	fmt.Println()
}

// listTools registers either individual tools or meta-tools on an in-memory MCP
// server and returns the published tool definitions for measurement.
func listTools(client *gitlabclient.Client, meta, enterprise bool) []*mcp.Tool {
	opts := &mcp.ServerOptions{PageSize: 2000}
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, opts)

	if meta {
		tools.RegisterAllMeta(server, client, enterprise)
		tools.RegisterMCPMeta(server, client, nil)
	} else {
		tools.RegisterAll(server, client, enterprise)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
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

// measureTools serializes each tool definition to JSON and estimates its token
// cost using the audit's byte-based heuristic.
func measureTools(toolList []*mcp.Tool) []toolTokenInfo {
	infos := make([]toolTokenInfo, 0, len(toolList))
	for _, t := range toolList {
		b, err := json.Marshal(t)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal tool %s: %v\n", t.Name, err)
			os.Exit(1)
		}
		tokens := len(b) / bytesPerTok
		domain := extractDomain(t.Name)
		infos = append(infos, toolTokenInfo{
			Name:       t.Name,
			Domain:     domain,
			Tokens:     tokens,
			Bytes:      len(b),
			Components: measureToolComponents(b),
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Tokens != infos[j].Tokens {
			return infos[i].Tokens > infos[j].Tokens
		}
		return infos[i].Name < infos[j].Name
	})
	return infos
}

// measureToolComponents breaks a serialized MCP tool definition into the
// fields most relevant to token-budget decisions. Other includes name, title,
// untracked fields, and JSON separator overhead.
func measureToolComponents(raw []byte) toolComponentBytes {
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return toolComponentBytes{Other: len(raw)}
	}
	components := toolComponentBytes{
		Description:  topLevelFieldBytes(decoded, "description"),
		InputSchema:  topLevelFieldBytes(decoded, "inputSchema"),
		OutputSchema: topLevelFieldBytes(decoded, "outputSchema"),
		Annotations:  topLevelFieldBytes(decoded, "annotations"),
		Icons:        topLevelFieldBytes(decoded, "icons"),
	}
	tracked := components.Description + components.InputSchema + components.OutputSchema + components.Annotations + components.Icons
	components.Other = max(len(raw)-tracked, 0)
	return components
}

// topLevelFieldBytes returns the byte cost of one serialized top-level JSON
// field, including the field name and colon but excluding surrounding object
// braces and comma separators.
func topLevelFieldBytes(decoded map[string]json.RawMessage, key string) int {
	value, ok := decoded[key]
	if !ok {
		return 0
	}
	entry, err := json.Marshal(map[string]json.RawMessage{key: value})
	if err != nil || len(entry) < 2 {
		return 0
	}
	return len(entry) - 2
}

// measureResources registers static, template, workflow, and optionally
// meta-schema MCP resources, then estimates the token cost of their advertised
// definitions.
func measureResources(client *gitlabclient.Client, includeMetaSchema bool) int {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, nil)
	resources.Register(server, client)
	if includeMetaSchema {
		metaRoutes := toolutil.CaptureMetaRoutes(func() {
			tools.RegisterAllMeta(server, client, false)
			tools.RegisterMCPMeta(server, client, nil)
		})
		resources.RegisterMetaSchemaResources(server, metaRoutes)
	}
	resources.RegisterWorkflowGuides(server)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect (resources): %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect (resources): %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	totalBytes := 0

	res, err := session.ListResources(ctx, nil)
	if err == nil {
		for _, r := range res.Resources {
			b, mErr := json.Marshal(r)
			if mErr != nil {
				fmt.Fprintf(os.Stderr, "marshal resource %s: %v\n", r.Name, mErr)
				os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
			}
			totalBytes += len(b)
		}
	}

	tpl, err := session.ListResourceTemplates(ctx, nil)
	if err == nil {
		for _, t := range tpl.ResourceTemplates {
			b, mErr := json.Marshal(t)
			if mErr != nil {
				fmt.Fprintf(os.Stderr, "marshal template %s: %v\n", t.Name, mErr)
				os.Exit(1)
			}
			totalBytes += len(b)
		}
	}

	return totalBytes / bytesPerTok
}

// measurePrompts registers MCP prompts and estimates the token cost of their
// advertised definitions.
func measurePrompts(client *gitlabclient.Client) int {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: auditVer}, nil)
	prompts.Register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect (prompts): %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: auditVer}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client connect (prompts): %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	totalBytes := 0
	p, err := session.ListPrompts(ctx, nil)
	if err == nil {
		for _, pr := range p.Prompts {
			b, mErr := json.Marshal(pr)
			if mErr != nil {
				fmt.Fprintf(os.Stderr, "marshal prompt %s: %v\n", pr.Name, mErr)
				os.Exit(1) //nolint:gocritic // CLI tool: OS reclaims resources on exit
			}
			totalBytes += len(b)
		}
	}
	return totalBytes / bytesPerTok
}

// extractDomain returns the GitLab tool domain from names like
// gitlab_{domain}_{action}. It returns "unknown" for malformed names.
func extractDomain(name string) string {
	// gitlab_{domain}_{action} → domain
	parts := strings.SplitN(name, "_", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

// totalTokens sums token estimates across a measured tool list.
func totalTokens(infos []toolTokenInfo) int {
	total := 0
	for _, i := range infos {
		total += i.Tokens
	}
	return total
}

// printTopTools writes the n most expensive tool definitions to stdout in a
// stable tabular format.
func printTopTools(infos []toolTokenInfo, n int) {
	if n > len(infos) {
		n = len(infos)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  #\tTokens\tBytes\tTool Name\n")
	fmt.Fprintf(tw, "  ─\t──────\t─────\t─────────\n")
	for i := range n {
		fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\n", i+1, fmtNum(infos[i].Tokens), fmtNum(infos[i].Bytes), infos[i].Name)
	}
	_ = tw.Flush()
	fmt.Println()
}

// printDomainTotals aggregates token estimates by tool domain and prints the
// highest-cost domains first.
func printDomainTotals(infos []toolTokenInfo, n int) {
	domainTotals := map[string]int{}
	domainCounts := map[string]int{}
	for _, i := range infos {
		domainTotals[i.Domain] += i.Tokens
		domainCounts[i.Domain]++
	}

	type domainEntry struct {
		Domain string
		Tokens int
		Count  int
	}
	entries := make([]domainEntry, 0, len(domainTotals))
	for d, t := range domainTotals {
		entries = append(entries, domainEntry{Domain: d, Tokens: t, Count: domainCounts[d]})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Tokens > entries[j].Tokens
	})

	if n > len(entries) {
		n = len(entries)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  #\tDomain\tTools\tTokens\n")
	fmt.Fprintf(tw, "  ─\t──────\t─────\t──────\n")
	for i := range n {
		fmt.Fprintf(tw, "  %d\t%s\t%d\t%s\n", i+1, entries[i].Domain, entries[i].Count, fmtNum(entries[i].Tokens))
	}
	_ = tw.Flush()
	fmt.Println()
}

// printComponentTotals aggregates top-level tool-definition component costs.
func printComponentTotals(infos []toolTokenInfo) {
	totals := toolComponentBytes{}
	totalBytes := 0
	for _, info := range infos {
		totalBytes += info.Bytes
		totals.Description += info.Components.Description
		totals.InputSchema += info.Components.InputSchema
		totals.OutputSchema += info.Components.OutputSchema
		totals.Annotations += info.Components.Annotations
		totals.Icons += info.Components.Icons
		totals.Other += info.Components.Other
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Component\tTokens\tBytes\tShare\n")
	fmt.Fprintf(tw, "  ─────────\t──────\t─────\t─────\n")
	printComponentRow(tw, "description", totals.Description, totalBytes)
	printComponentRow(tw, "inputSchema", totals.InputSchema, totalBytes)
	printComponentRow(tw, "outputSchema", totals.OutputSchema, totalBytes)
	printComponentRow(tw, "annotations", totals.Annotations, totalBytes)
	printComponentRow(tw, "icons", totals.Icons, totalBytes)
	printComponentRow(tw, "other", totals.Other, totalBytes)
	_ = tw.Flush()
	fmt.Println()
}

func printComponentRow(tw *tabwriter.Writer, label string, bytes, totalBytes int) {
	share := 0.0
	if totalBytes > 0 {
		share = float64(bytes) * 100 / float64(totalBytes)
	}
	fmt.Fprintf(tw, "  %s\t%s\t%s\t%.1f%%\n", label, fmtNum(bytes/bytesPerTok), fmtNum(bytes), share)
}

// fmtNum formats integers with comma thousands separators for report tables.
func fmtNum(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}
	return string(result)
}
