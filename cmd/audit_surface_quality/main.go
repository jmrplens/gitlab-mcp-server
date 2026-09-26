package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// toolName is the command's own name, used as the flag set's name so a usage
// message names the command rather than the test binary that drove it.
const toolName = "audit_surface_quality"

// osExit is os.Exit behind a variable, so the one line main carries is
// reachable from a test rather than only from a process.
var osExit = os.Exit

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
	osExit(runMain(os.Args[1:], os.Stderr))
}

// runMain parses args, the command line with the program name already
// removed, runs the views it asked for and returns the process exit code: 0
// when nothing gates, 1 when -check found something, and 2 for a command line
// this command refuses.
//
// The flag set is one of its own rather than the package-level [flag]
// CommandLine, so a bad flag is an exit code this function returns instead of
// an os.Exit the seam above never sees, and so a second call in one process
// does not redeclare a flag. flag has already printed the error and the usage
// by the time Parse returns; -h is the one failure that exits clean, as
// ExitOnError would.
func runMain(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	view := fs.String("view", "all", "which audit view to run: metadata, output, or all")
	fs.BoolVar(&outputJSON, "json", false, "emit JSON instead of markdown")
	fs.BoolVar(&checkMode, "check", false, "print only the violations that gate and exit 1 when there are any")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	switch *view {
	case "metadata", "output", "all":
	default:
		fmt.Fprintf(stderr, "invalid -view %q (valid: metadata, output, all)\n", *view)
		return 2
	}

	// -json runs a single audit view: -view=all would emit two top-level JSON
	// documents back-to-back (unparseable), so require an explicit view.
	if outputJSON && *view == "all" {
		fmt.Fprintln(stderr, "-json requires -view=metadata or -view=output")
		return 2
	}

	// -check reports one compact list per view, so the two spellings would
	// contradict each other on what stdout carries.
	if outputJSON && checkMode {
		fmt.Fprintln(stderr, "-check and -json are alternatives: -check prints the violations that gate")
		return 2
	}

	if auditViews(*view) > 0 && checkMode {
		return 1
	}
	return 0
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

// listSurface is where both views and the edition-tier rule read a surface
// from: the meta surface when meta is true and the individual one otherwise,
// as tier is served it, from [mcpsurface], which registers what cmd/server
// registers and applies the served-schema chain, so the audit judges the
// schemas clients actually see.
//
// It is a variable so a test can hand the views a listing that carries a
// violation of every rule, which the served surface, carrying none, cannot.
// Until it was, a rule dropped from either view, or handed the other
// surface or the other surface's label, failed no test.
var listSurface = func(client *gitlabclient.Client, tier edition.Tier, meta bool) []*mcp.Tool {
	if meta {
		return mcpsurface.MetaTools(client, tier)
	}
	return mcpsurface.IndividualTools(client, tier)
}

// listTools returns the tool list one surface advertises at the widest tier.
// When meta is true the meta surface is listed instead of the individual one.
func listTools(client *gitlabclient.Client, meta bool) []*mcp.Tool {
	return listSurface(client, edition.Ultimate, meta)
}

// listIndividualTools returns the individual surface as one tier is served
// it. The listing is memoized per tier, so the edition-tier rule reading all
// three costs three registrations and no more.
func listIndividualTools(client *gitlabclient.Client, tier edition.Tier) []*mcp.Tool {
	return listSurface(client, tier, false)
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
