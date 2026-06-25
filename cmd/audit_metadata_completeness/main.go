// Command audit_metadata_completeness reports discovery-metadata gaps (R-META)
// across the canonical ActionSpec catalog. For every action it flags:
//
//   - generic_usage: a placeholder Usage string such as "Use to execute X domain
//     action." (or an empty Usage) instead of a purpose-specific sentence.
//   - aliases_only_toolname: no natural-language aliases beyond the canonical
//     action name and the projected individual-tool name.
//   - empty_related: no RelatedActions cross-links.
//   - missing_individual_description: the individual-tool surface has no
//     "Returns: … See also: …" Description.
//
// The output is the R-META slice of the 1:1 backlog, grouped by owner package.
//
// Usage:
//
//	go run ./cmd/audit_metadata_completeness/                 # full report to stdout
//	go run ./cmd/audit_metadata_completeness/ -gaps-only      # only actions with findings
//	go run ./cmd/audit_metadata_completeness/ -output dist/metadata-completeness.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const schemaVersion = 1

// genericUsageRe matches placeholder Usage sentences such as
// "Use to execute branches domain action." or "Use to execute list action.".
var genericUsageRe = regexp.MustCompile(`(?i)^use to execute\b.*\baction\.?\s*$`)

// actionFinding records the R-META flags raised for one action.
type actionFinding struct {
	Action string   `json:"action"`
	Tool   string   `json:"tool,omitempty"`
	Usage  string   `json:"usage,omitempty"`
	Flags  []string `json:"flags"`
}

// packageReport groups action findings by owner package.
type packageReport struct {
	Package  string          `json:"package"`
	Actions  int             `json:"actions"`
	Findings []actionFinding `json:"findings"`
}

type report struct {
	SchemaVersion int             `json:"schema_version"`
	Summary       reportSummary   `json:"summary"`
	Packages      []packageReport `json:"packages"`
}

type reportSummary struct {
	Packages                  int `json:"packages"`
	Actions                   int `json:"actions"`
	GenericUsage              int `json:"generic_usage"`
	AliasesOnlyToolname       int `json:"aliases_only_toolname"`
	EmptyRelated              int `json:"empty_related"`
	WeakIndividualDescription int `json:"weak_individual_description"`
}

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include actions that raise at least one flag")
	flag.Parse()

	rep, err := buildReport(*gapsOnly)
	if err != nil {
		cmdutil.Fatalf("build metadata completeness report: %v", err)
	}
	content, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		cmdutil.Fatalf("marshal report: %v", err)
	}
	content = append(content, '\n')
	if writeErr := writeReport(*outputPath, content); writeErr != nil {
		cmdutil.Fatalf("write report: %v", writeErr)
	}
}

func buildReport(gapsOnly bool) (report, error) {
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		return report{}, fmt.Errorf("create audit client: %w", err)
	}
	defer cleanup()

	projected, err := projectIndividualDescriptions(client)
	if err != nil {
		return report{}, err
	}

	byPackage := map[string]*packageReport{}
	for _, group := range tools.CollectActionSpecs(client, true) {
		for _, spec := range group.Actions {
			owner := ownerPackage(group, spec)
			pr := packageFor(byPackage, owner)
			pr.Actions++
			if finding, ok := analyzeSpec(spec, projected, gapsOnly); ok {
				pr.Findings = append(pr.Findings, finding)
			}
		}
	}

	packagesOut := make([]packageReport, 0, len(byPackage))
	for _, pr := range byPackage {
		if gapsOnly && len(pr.Findings) == 0 {
			continue
		}
		sort.Slice(pr.Findings, func(i, j int) bool { return pr.Findings[i].Action < pr.Findings[j].Action })
		packagesOut = append(packagesOut, *pr)
	}
	sort.Slice(packagesOut, func(i, j int) bool { return packagesOut[i].Package < packagesOut[j].Package })
	return report{
		SchemaVersion: schemaVersion,
		Summary:       summarize(packagesOut),
		Packages:      packagesOut,
	}, nil
}

// analyzeSpec returns the R-META finding for one spec. The boolean is false when
// the action raises no flags (used to skip clean actions in gaps-only mode).
func analyzeSpec(spec toolutil.ActionSpec, projected map[string]string, gapsOnly bool) (actionFinding, bool) {
	var flags []string
	if isGenericUsage(spec.Usage) {
		flags = append(flags, "generic_usage")
	}
	if aliasesOnlyToolname(spec) {
		flags = append(flags, "aliases_only_toolname")
	}
	if len(spec.RelatedActions) == 0 {
		flags = append(flags, "empty_related")
	}
	if weakIndividualDescription(spec, projected) {
		flags = append(flags, "weak_individual_description")
	}
	if len(flags) == 0 && gapsOnly {
		return actionFinding{}, false
	}
	return actionFinding{
		Action: spec.Name,
		Tool:   spec.IndividualTool.Name,
		Usage:  strings.TrimSpace(spec.Usage),
		Flags:  flags,
	}, len(flags) > 0
}

// weakIndividualDescription reports whether the effective individual-tool
// description the model sees lacks the norm's "Returns: … See also: …" form.
// The effective description is the projected mcp.Tool.Description (which already
// resolves the curated-description fallback chain), so this avoids false
// positives from specs that omit IndividualTool.Description yet still project a
// good curated one.
func weakIndividualDescription(spec toolutil.ActionSpec, projected map[string]string) bool {
	tool := strings.TrimSpace(spec.IndividualTool.Name)
	if tool == "" {
		return false
	}
	description, ok := projected[tool]
	if !ok {
		return false
	}
	return !strings.Contains(description, "Returns:") || !strings.Contains(description, "See also:")
}

// projectIndividualDescriptions registers the individual-tool surface on an
// in-memory MCP server and returns the projected description per tool name —
// the exact text the model consumes.
func projectIndividualDescriptions(client *gitlabclient.Client) (map[string]string, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "audit", Version: "0.0.1"}, nil)
	tools.RegisterAll(server, client, edition.Ultimate)
	toolutil.LockdownInputSchemas(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		return nil, fmt.Errorf("connect server: %w", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "audit-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect client: %w", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	descriptions := make(map[string]string, len(result.Tools))
	for _, tool := range result.Tools {
		descriptions[tool.Name] = tool.Description
	}
	return descriptions, nil
}

func isGenericUsage(usage string) bool {
	trimmed := strings.TrimSpace(usage)
	return trimmed == "" || genericUsageRe.MatchString(trimmed)
}

// aliasesOnlyToolname reports whether the action has no natural-language alias
// beyond its canonical name and projected individual-tool name.
func aliasesOnlyToolname(spec toolutil.ActionSpec) bool {
	canonical := strings.ToLower(strings.TrimSpace(spec.Name))
	tool := strings.ToLower(strings.TrimSpace(spec.IndividualTool.Name))
	for _, alias := range spec.Aliases {
		normalized := strings.ToLower(strings.TrimSpace(alias))
		if normalized == "" || normalized == canonical || normalized == tool {
			continue
		}
		return false
	}
	return true
}

func ownerPackage(group tools.ActionSpecGroup, spec toolutil.ActionSpec) string {
	if owner := strings.TrimSpace(spec.OwnerPackage); owner != "" {
		return owner
	}
	if owner := strings.TrimSpace(group.OwnerPackage); owner != "" {
		return owner
	}
	return strings.TrimSpace(group.BaseDomain)
}

func packageFor(byPackage map[string]*packageReport, owner string) *packageReport {
	pr, ok := byPackage[owner]
	if !ok {
		pr = &packageReport{Package: owner}
		byPackage[owner] = pr
	}
	return pr
}

func summarize(packages []packageReport) reportSummary {
	s := reportSummary{Packages: len(packages)}
	for _, pr := range packages {
		s.Actions += pr.Actions
		for _, finding := range pr.Findings {
			for _, flag := range finding.Flags {
				switch flag {
				case "generic_usage":
					s.GenericUsage++
				case "aliases_only_toolname":
					s.AliasesOnlyToolname++
				case "empty_related":
					s.EmptyRelated++
				case "weak_individual_description":
					s.WeakIndividualDescription++
				}
			}
		}
	}
	return s
}

func writeReport(outputPath string, content []byte) error {
	if outputPath == "-" {
		_, err := os.Stdout.Write(content)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(outputPath, content, 0o600)
}
