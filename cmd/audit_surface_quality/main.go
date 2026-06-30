// Command audit_surface_quality is the consolidated MCP tool surface quality
// audit. It combines the metadata-quality checks (naming, annotations, schema
// shape — formerly audit_tools) and the output-quality checks (OutputSchema,
// "Returns:"/"See also:", Title — formerly audit_output) behind a single
// -view flag.
//
// Usage:
//
//	go run ./cmd/audit_surface_quality/                  # both views
//	go run ./cmd/audit_surface_quality/ -view=metadata   # metadata only
//	go run ./cmd/audit_surface_quality/ -view=output
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// outputJSON switches stdout output from human-readable markdown to structured
// JSON. Set by the -json flag.
var outputJSON bool

func main() {
	view := flag.String("view", "all", "which audit view to run: metadata, output, or all")
	flag.BoolVar(&outputJSON, "json", false, "emit JSON instead of markdown")
	flag.Parse()

	switch *view {
	case "metadata", "output", "all":
	default:
		fmt.Fprintf(os.Stderr, "invalid -view %q (valid: metadata, output, all)\n", *view)
		os.Exit(2)
	}

	// -json runs a single audit view: -view=all would emit two top-level JSON
	// documents back-to-back (unparseable), so require an explicit view.
	if outputJSON && *view == "all" {
		fmt.Fprintln(os.Stderr, "-json requires -view=metadata or -view=output")
		os.Exit(2)
	}

	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	if *view == "metadata" || *view == "all" {
		runMetadataAudit(client)
	}
	if *view == "output" || *view == "all" {
		runOutputAudit(client)
	}
}

// listTools registers all MCP tools on an in-memory server and returns
// the tool list. When meta is true, meta-tools are registered instead of
// individual tools. LockdownInputSchemas is applied so the audit reflects
// the schemas clients actually see.
func listTools(client *gitlabclient.Client, meta bool) []*mcp.Tool {
	server := mcp.NewServer(&mcp.Implementation{Name: "audit", Version: "0.0.1"}, nil)
	if meta {
		if err := tools.RegisterAllMeta(server, client, edition.Ultimate); err != nil {
			fmt.Fprintf(os.Stderr, "register meta tools: %v\n", err)
			os.Exit(1)
		}
	} else {
		tools.RegisterAll(server, client, edition.Ultimate)
	}
	toolutil.LockdownInputSchemas(server)

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

// jsonEntry is the JSON representation of a violation or finding.
type jsonEntry struct {
	Tool     string `json:"tool"`
	Category string `json:"category"`
	Detail   string `json:"detail"`
}

func toEntries(vs []violation) []jsonEntry {
	out := make([]jsonEntry, len(vs))
	for i, v := range vs {
		out[i] = jsonEntry{v.tool, v.category, v.detail}
	}
	return out
}
