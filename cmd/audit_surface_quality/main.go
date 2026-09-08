package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
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

	client, cleanup := auditshared.NewStubGitLabClient(auditshared.StubToken)
	defer cleanup()

	if *view == "metadata" || *view == "all" {
		runMetadataAudit(client)
	}
	if *view == "output" || *view == "all" {
		runOutputAudit(client)
	}
}

// listTools returns the tool list one surface advertises at the widest tier,
// from [mcpsurface], which registers what cmd/server registers and applies the
// served-schema chain, so the audit judges the schemas clients actually see.
// When meta is true the meta surface is listed instead of the individual one.
func listTools(client *gitlabclient.Client, meta bool) []*mcp.Tool {
	if meta {
		return mcpsurface.MetaTools(client, edition.Ultimate)
	}
	return mcpsurface.IndividualTools(client, edition.Ultimate)
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
