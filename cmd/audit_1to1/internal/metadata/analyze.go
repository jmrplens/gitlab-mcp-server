// Package metadata reports discovery-metadata gaps (R-META)
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
package metadata

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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

// Run builds the report and returns it as indented JSON (with a trailing
// newline). gapsOnly filters to actions that raise at least one flag, matching
// the original -gaps-only flag.
func Run(gapsOnly bool) ([]byte, error) {
	rep, err := buildReport(gapsOnly)
	if err != nil {
		return nil, err
	}
	content, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), nil
}

func buildReport(gapsOnly bool) (report, error) {
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		return report{}, fmt.Errorf("create audit client: %w", err)
	}
	defer cleanup()

	projected, err := auditshared.CachedIndividualDescriptions(client)
	if err != nil {
		return report{}, err
	}

	packagesOut := collectPackages(auditshared.CachedActionSpecs(client, true), projected, gapsOnly)
	return report{
		SchemaVersion: shared.SchemaVersion,
		Summary:       summarize(packagesOut),
		Packages:      packagesOut,
	}, nil
}

// collectPackages analyzes every spec of every group and returns the per-owner
// package reports, sorted by package and, within one, by action. gapsOnly
// drops the packages that raise no finding.
func collectPackages(groups []tools.ActionSpecGroup, projected map[string]string, gapsOnly bool) []packageReport {
	byPackage := map[string]*packageReport{}
	for _, group := range groups {
		for _, spec := range group.Actions {
			owner := auditshared.OwnerPackage(group, spec)
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
	return packagesOut
}

// analyzeSpec returns the R-META finding for one spec. The boolean is false when
// the action raises no flags (used to skip clean actions in gaps-only mode).
func analyzeSpec(spec toolutil.ActionSpec, projected map[string]string, gapsOnly bool) (actionFinding, bool) {
	var flags []string
	if auditshared.IsGenericUsage(spec.Usage) {
		flags = append(flags, "generic_usage")
	}
	if aliasesOnlyToolname(spec) {
		flags = append(flags, "aliases_only_toolname")
	}
	if len(spec.RelatedActions) == 0 {
		flags = append(flags, "empty_related")
	}
	if auditshared.WeakIndividualDescription(spec, projected) {
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
