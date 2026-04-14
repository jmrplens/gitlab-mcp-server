// Command audit_tools generates a markdown report of all MCP tool metadata
// violations. It creates an in-memory MCP server with all tools registered
// (both individual and meta-tools), inspects their metadata, and outputs
// findings to stdout.
//
// Usage:
//
//	go run ./cmd/audit_tools/
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
)

// Naming patterns for MCP tool name validation.
var (
	// toolNameRe matches individual tool names: gitlab_{word}_{word}[_{word}...].
	toolNameRe = regexp.MustCompile(`^gitlab_[a-z][a-z0-9]*(_[a-z0-9]+)+$`)
	// metaToolNameRe matches meta-tool names: gitlab_{word}[_{word}...].
	metaToolNameRe = regexp.MustCompile(`^gitlab_[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
)

// minDescLen is the minimum acceptable description length for an MCP tool.
const minDescLen = 20

// readSuffixes indicate read-only operations based on tool name endings.
var readSuffixes = []string{
	"_list", "_lists", "_get", "_search",
	"_latest", "_blame", "_raw", "_diff", "_refs",
	"_statuses", "_signature", "_languages", "_statistics",
}

// violation records a single metadata rule infraction for a tool.
type violation struct {
	tool     string // MCP tool name that violated the rule.
	category string // Rule category (naming, description, annotations, etc.).
	detail   string // Human-readable explanation of the violation.
}

// main audits MCP tool metadata for violations such as missing annotations.
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

	individualTools := listTools(client, false)
	metaTools := listTools(client, true)

	violations := make([]violation, 0, len(individualTools)+len(metaTools))

	violations = append(violations, auditNaming(individualTools, toolNameRe, "individual")...)
	violations = append(violations, auditNaming(metaTools, metaToolNameRe, "meta")...)
	violations = append(violations, auditDescriptions(individualTools, "individual")...)
	violations = append(violations, auditDescriptions(metaTools, "meta")...)
	violations = append(violations, auditAnnotations(individualTools, "individual")...)
	violations = append(violations, auditAnnotations(metaTools, "meta")...)
	violations = append(violations, auditAnnotationTypes(individualTools)...)
	violations = append(violations, auditInputSchema(individualTools)...)
	violations = append(violations, auditDuplicates(individualTools, "individual")...)
	violations = append(violations, auditDuplicates(metaTools, "meta")...)

	printReport(individualTools, metaTools, violations)
}

// listTools registers all MCP tools on an in-memory server and returns
// the tool list. When meta is true, meta-tools are registered instead of
// individual tools.
func listTools(client *gitlabclient.Client, meta bool) []*mcp.Tool {
	server := mcp.NewServer(&mcp.Implementation{Name: "audit", Version: "0.0.1"}, nil)
	if meta {
		tools.RegisterAllMeta(server, client, true)
	} else {
		tools.RegisterAll(server, client, true)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server connect: %v\n", err)
		os.Exit(1)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "audit-client", Version: "0.0.1"}, nil)
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

// auditNaming checks that every tool name matches the given regex pattern.
// kind is a label ("individual" or "meta") used in violation messages.
func auditNaming(tls []*mcp.Tool, re *regexp.Regexp, kind string) []violation {
	var vs []violation
	for _, t := range tls {
		if !re.MatchString(t.Name) {
			vs = append(vs, violation{t.Name, "naming", fmt.Sprintf("%s tool name does not match %s", kind, re.String())})
		}
	}
	return vs
}

// auditDescriptions flags tools whose description is shorter than minDescLen.
func auditDescriptions(tls []*mcp.Tool, kind string) []violation {
	var vs []violation
	for _, t := range tls {
		if len(t.Description) < minDescLen {
			vs = append(vs, violation{t.Name, "description", fmt.Sprintf("%s description too short (%d chars): %q", kind, len(t.Description), t.Description)})
		}
	}
	return vs
}

// auditAnnotations checks that every tool has non-nil Annotations and
// that ReadOnlyHint and DestructiveHint are not both true simultaneously.
func auditAnnotations(tls []*mcp.Tool, kind string) []violation {
	var vs []violation
	for _, t := range tls {
		if t.Annotations == nil {
			vs = append(vs, violation{t.Name, "annotations", fmt.Sprintf("%s tool has nil Annotations", kind)})
			continue
		}
		if t.Annotations.ReadOnlyHint && t.Annotations.DestructiveHint != nil && *t.Annotations.DestructiveHint {
			vs = append(vs, violation{t.Name, "annotations", "ReadOnlyHint=true conflicts with DestructiveHint=true"})
		}
	}
	return vs
}

// auditAnnotationTypes verifies consistency between tool name suffixes and
// their annotation hints: read-like names should have ReadOnlyHint=true,
// delete-like names should have DestructiveHint=true.
func auditAnnotationTypes(tls []*mcp.Tool) []violation {
	var vs []violation
	for _, t := range tls {
		if t.Annotations == nil {
			continue
		}
		isRead := isReadToolName(t.Name)
		isDelete := isDeleteToolName(t.Name)

		if isRead && !t.Annotations.ReadOnlyHint {
			vs = append(vs, violation{t.Name, "annotation-type", "name suggests read-only but ReadOnlyHint is false"})
		}
		if isDelete {
			if t.Annotations.DestructiveHint == nil || !*t.Annotations.DestructiveHint {
				vs = append(vs, violation{t.Name, "annotation-type", "name suggests delete but DestructiveHint is not true"})
			}
		}
	}
	return vs
}

// auditInputSchema validates that each tool's InputSchema is a valid JSON
// Schema object with type "object" and at least one property defined.
func auditInputSchema(tls []*mcp.Tool) []violation {
	var vs []violation
	for _, t := range tls {
		schema, ok := t.InputSchema.(map[string]any)
		if !ok {
			vs = append(vs, violation{t.Name, "input-schema", "InputSchema is not a map"})
			continue
		}
		typ, _ := schema["type"].(string)
		if typ != "object" {
			vs = append(vs, violation{t.Name, "input-schema", fmt.Sprintf("InputSchema type=%q, expected \"object\"", typ)})
		}
	}
	return vs
}

// auditDuplicates detects tools with the same name registered more than once.
func auditDuplicates(tls []*mcp.Tool, kind string) []violation {
	var vs []violation
	seen := make(map[string]bool, len(tls))
	for _, t := range tls {
		if seen[t.Name] {
			vs = append(vs, violation{t.Name, "duplicate", fmt.Sprintf("duplicate %s tool name", kind)})
		}
		seen[t.Name] = true
	}
	return vs
}

// isReadToolName reports whether name ends with a read-only suffix such as
// "_list", "_get", or "_search".
func isReadToolName(name string) bool {
	for _, sfx := range readSuffixes {
		if strings.HasSuffix(name, sfx) {
			return true
		}
	}
	return false
}

// isDeleteToolName reports whether name contains or ends with "delete".
func isDeleteToolName(name string) bool {
	if strings.HasSuffix(name, "_delete") {
		return true
	}
	return slices.Contains(strings.Split(name, "_"), "delete")
}

// printReport writes the full markdown audit report to stdout,
// including summary counts, violations grouped by category, and a
// complete listing of all individual and meta-tools with their annotations.
func printReport(individual, meta []*mcp.Tool, vs []violation) {
	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("# MCP Tool Metadata Audit Report\n\n")
	fmt.Printf("Generated: %s\n\n", now)
	fmt.Printf("## Summary\n\n")
	fmt.Printf("| Metric | Count |\n")
	fmt.Printf("| --- | --- |\n")
	fmt.Printf("| Individual tools | %d |\n", len(individual))
	fmt.Printf("| Meta-tools | %d |\n", len(meta))
	fmt.Printf("| Total violations | %d |\n\n", len(vs))

	if len(vs) == 0 {
		fmt.Println("**No violations found.**")
		return
	}

	// Group by category
	categories := make(map[string][]violation)
	for _, v := range vs {
		categories[v.category] = append(categories[v.category], v)
	}

	fmt.Printf("## Violations by Category\n\n")
	for cat, catVs := range categories {
		fmt.Printf("### %s (%d)\n\n", cat, len(catVs))
		fmt.Printf("| Tool | Detail |\n")
		fmt.Printf("| --- | --- |\n")
		for _, v := range catVs {
			fmt.Printf("| `%s` | %s |\n", v.tool, v.detail)
		}
		fmt.Println()
	}

	fmt.Printf("## All Tools\n\n")
	fmt.Printf("### Individual Tools (%d)\n\n", len(individual))
	fmt.Printf("| # | Name | Description (first 60 chars) | Annotations |\n")
	fmt.Printf("| --- | --- | --- | --- |\n")
	for i, t := range individual {
		desc := t.Description
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		ann := "nil"
		if t.Annotations != nil {
			ann = fmt.Sprintf("RO=%v D=%v I=%v OW=%v",
				t.Annotations.ReadOnlyHint,
				ptrBool(t.Annotations.DestructiveHint),
				t.Annotations.IdempotentHint,
				ptrBool(t.Annotations.OpenWorldHint))
		}
		fmt.Printf("| %d | `%s` | %s | %s |\n", i+1, t.Name, desc, ann)
	}

	fmt.Printf("\n### Meta-Tools (%d)\n\n", len(meta))
	fmt.Printf("| # | Name | Description (first 60 chars) | Annotations |\n")
	fmt.Printf("| --- | --- | --- | --- |\n")
	for i, t := range meta {
		desc := t.Description
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		ann := "nil"
		if t.Annotations != nil {
			ann = fmt.Sprintf("RO=%v D=%v I=%v OW=%v",
				t.Annotations.ReadOnlyHint,
				ptrBool(t.Annotations.DestructiveHint),
				t.Annotations.IdempotentHint,
				ptrBool(t.Annotations.OpenWorldHint))
		}
		fmt.Printf("| %d | `%s` | %s | %s |\n", i+1, t.Name, desc, ann)
	}
}

// ptrBool formats a *bool as "true", "false", or "nil".
func ptrBool(p *bool) string {
	if p == nil {
		return "nil"
	}
	if *p {
		return "true"
	}
	return "false"
}
