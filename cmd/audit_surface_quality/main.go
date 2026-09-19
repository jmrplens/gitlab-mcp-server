package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// outputJSON switches stdout output from human-readable markdown to structured
// JSON. Set by the -json flag.
var outputJSON bool

// checkMode makes the command a gate: it prints the violations that gate and
// nothing else, and the exit code says whether there were any. Set by -check.
//
// What gates is every rule that reads the served surface. The
// result-envelope section is excluded and stays a report: a formatter that
// renders nothing for a zero value is a guard rather than a defect, and the
// duplicate registrations it lists are a backlog nobody has worked through,
// so failing on them would fail every run from the day the flag was added.
// The constant-index rule is the exception, and is counted here rather than
// there, being a defect in what a client reads.
var checkMode bool

func main() {
	view := flag.String("view", "all", "which audit view to run: metadata, output, or all")
	flag.BoolVar(&outputJSON, "json", false, "emit JSON instead of markdown")
	flag.BoolVar(&checkMode, "check", false, "print only the violations that gate and exit 1 when there are any")
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

	// -check reports one compact list per view, so the two spellings would
	// contradict each other on what stdout carries.
	if outputJSON && checkMode {
		fmt.Fprintln(os.Stderr, "-check and -json are alternatives: -check prints the violations that gate")
		os.Exit(2)
	}

	if auditViews(*view) > 0 && checkMode {
		os.Exit(1)
	}
}

// auditViews runs the views the flag asked for and returns how many
// violations gate. It owns the stub client rather than main, because
// os.Exit runs no deferred call and the exit code is decided from what this
// returns.
func auditViews(view string) int {
	client, cleanup := auditshared.NewStubGitLabClient(auditshared.StubToken)
	defer cleanup()

	failed := 0
	if view == "metadata" || view == "all" {
		failed += runMetadataAudit(client)
	}
	if view == "output" || view == "all" {
		failed += runOutputAudit(client)
	}
	return failed
}

// listTools returns the tool list one surface advertises at the widest tier,
// from [mcpsurface], which registers what cmd/server registers and applies the
// served-schema chain, so the audit judges the schemas clients actually see.
// When meta is true the meta surface is listed instead of the individual one.
func listTools(client *gitlabclient.Client, meta bool) []*mcp.Tool {
	if meta {
		return mcpsurface.MetaTools(client, edition.Ultimate)
	}
	return listIndividualTools(client, edition.Ultimate)
}

// listIndividualTools returns the individual surface as one tier is served
// it. The listing is memoized per tier, so the edition-tier rule reading all
// three costs three registrations and no more.
func listIndividualTools(client *gitlabclient.Client, tier edition.Tier) []*mcp.Tool {
	return mcpsurface.IndividualTools(client, tier)
}

// reportGate prints the violations one view gates on, and returns how many
// there were. It is what -check writes instead of the report.
func reportGate(view string, vs []violation) int {
	if len(vs) == 0 {
		fmt.Printf("%s: no violations\n", view)
		return 0
	}
	fmt.Printf("%s: %d violation(s)\n", view, len(vs))
	for _, v := range vs {
		fmt.Printf("  %s [%s]: %s\n", v.tool, v.category, v.detail)
	}
	return len(vs)
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
