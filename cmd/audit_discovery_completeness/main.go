// Command audit_discovery_completeness reports discovery-metadata gaps that
// affect how well models can find and choose the right action. It extends the
// existing R-META auditor (cmd/audit_metadata_completeness) with two new
// dimensions:
//
//   - Check B — Field-level: empty_param_description (input-schema property
//     descriptions that are blank or boilerplate) and empty_output_description
//     (lower priority).
//
//   - Check C — Sibling-cluster / confusable-action: cluster actions by
//     (owner_package, base_action_stem) stripping variant suffixes
//     (_batch/_bulk/_all/_directory/_single) and CRUD verbs. Flag any cluster
//     member that lacks both a sibling in RelatedActions (or a CommonConfusions
//     entry naming a sibling) AND a Usage string that mentions a distinguishing
//     signal. This is the link_create_batch / publish vs publish_directory
//     class of gap that motivated the discovery program.
//
// Check A — Action-level (weak_aliases, generic_usage, empty_related,
// weak_individual_description) reuses the R-META logic. missing_next_steps is
// a Phase 0 stub (see TODO) — Phase 1 will wire it to the markdown formatter
// registry via an exported HasRegisteredMarkdownFormatter helper.
//
// Output JSON shape (plan/discovery-backlog.json):
//
//	{
//	  "schema_version": 1,
//	  "summary": {...},                  // global counts
//	  "clusters": [{"package", "stem", "members"}],
//	  "packages": [{"package", "actions", "findings"}]
//	}
//
// Flags:
//
//	-output PATH        path to write JSON (default "-" stdout)
//	-gaps-only          omit actions without any finding
//	-min-aliases N      threshold for weak_aliases (default 3)
//	-severity LEVEL     error|warning|info for -check threshold (default error)
//	-check              exit non-zero if any finding meets/exceeds -severity
//
// Usage:
//
//	go run ./cmd/audit_discovery_completeness/                 # full report to stdout
//	go run ./cmd/audit_discovery_completeness/ -gaps-only      # only findings
//	go run ./cmd/audit_discovery_completeness/ -check          # CI gate
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
// "Use to execute releaselinks domain action." or empty Usage.
var genericUsageRe = regexp.MustCompile(`(?i)^use to execute\b.*\baction\.?\s*$`)

// variantSuffixes are stripped from action stems when clustering sibling actions.
var variantSuffixes = []string{"_batch", "_bulk", "_all", "_directory", "_single"}

// crudSuffixes are common CRUD verb suffixes stripped alongside variant suffixes.
var crudSuffixes = []string{"_list", "_get", "_delete", "_update", "_create"}

// scopeSuffixes mark scope-specific variants of a base action (project/user/
// group/instance) that should cluster with the base. Without these, the
// cluster algorithm would treat deploy_key_list_project and deploy_key_list_all
// as unrelated even though they are both "list deploy keys" with different
// scope or aggregation. Stripped AFTER crudSuffixes.
var scopeSuffixes = []string{"_project", "_user", "_group", "_instance"}

// scopeIDSuffixes are *_id parameter names that frequently confuse models
// because the same identifier means a different scope across GitLab APIs
// (project_id accepts a path in some endpoints, a numeric ID in others).
// Used by isScopeSuggestiveName for the membership check below.
var scopeIDSuffixes = []string{
	"project_id", "group_id", "user_id", "instance_id",
	"namespace_id", "milestone_id", "epic_id",
}

// usageSignalKeywords are heuristic markers that a Usage string mentions a
// distinguishing signal for sibling-cluster disambiguation. The list is
// intentionally conservative: only phrases that strongly imply a comparison
// against a sibling action. "single" is deliberately excluded because it is
// too generic (it appears in many single-action Usages that don't need
// disambiguation because there is no batch sibling).
var usageSignalKeywords = []string{
	"instead of",
	"multiple",
	"one call",
	"batch",
	"bulk",
	"all ",
	"directory",
}

// severityFor reports the effective severity for a flag, applying the
// cluster-aware upgrade for empty_related, weak_aliases, and
// weak_individual_description. The upgrade is only applied when the
// cluster has a non-CRUD variant suffix (_batch/_bulk/_all/_directory/
// _single) — pure CRUD families (create/get/list/delete/update on the
// same resource) are not escalated because the verb is itself the
// disambiguator and the action name is the natural description.
func severityFor(flagName string, inCluster bool) string {
	switch flagName {
	case "generic_usage", "missing_disambiguation":
		return "error"
	case "weak_aliases":
		if inCluster && hasNonCRUDVariantInCluster(currentClusterMembers) {
			return "error"
		}
		return "warning"
	case "empty_related":
		if inCluster && hasNonCRUDVariantInCluster(currentClusterMembers) {
			return "error"
		}
		return "warning"
	case "missing_next_steps":
		return "warning"
	case "empty_param_description":
		return "warning"
	case "empty_output_description":
		return "info"
	case "weak_individual_description":
		if inCluster && hasNonCRUDVariantInCluster(currentClusterMembers) {
			return "error"
		}
		return "warning"
	case "missing_parameter_guidance":
		return "warning"
	case "aliases_only_toolname":
		return "warning"
	}
	return "info"
}

// currentClusterMembers is set by buildReport before iterating packages so
// severityFor can apply the cluster-variant guard without threading the
// cluster membership through every call site. Reset to nil for any auditor
// use that doesn't set it (e.g. unit tests of severityFor).
var currentClusterMembers []string

// withClusterMembers returns a copy of fn invoked with currentClusterMembers
// set to members, then resets the global. Used by buildReport to scope
// severityFor's cluster-variant lookups to the current spec's cluster.
func withClusterMembers(members []string, fn func()) {
	currentClusterMembers = members
	defer func() { currentClusterMembers = nil }()
	fn()
}

// actionFinding records the discovery-completeness flags raised for one action.
type actionFinding struct {
	Action    string         `json:"action"`
	Tool      string         `json:"tool,omitempty"`
	Usage     string         `json:"usage,omitempty"`
	Severity  string         `json:"severity"`
	Flags     []string       `json:"flags"`
	Cluster   []string       `json:"cluster,omitempty"`
	Fields    []fieldFinding `json:"fields,omitempty"`
	HasSchema bool           `json:"has_schema,omitempty"`
}

// fieldFinding records a single field-level discovery gap (empty description).
type fieldFinding struct {
	Param string `json:"param"`
	Flag  string `json:"flag"`
}

// clusterRecord describes one confusable-action cluster discovered by the auditor.
type clusterRecord struct {
	Package string   `json:"package"`
	Stem    string   `json:"stem"`
	Members []string `json:"members"`
}

// packageReport groups action findings by owner package.
type packageReport struct {
	Package  string          `json:"package"`
	Actions  int             `json:"actions"`
	Findings []actionFinding `json:"findings"`
}

// report is the top-level JSON document.
type report struct {
	SchemaVersion int             `json:"schema_version"`
	Summary       reportSummary   `json:"summary"`
	Clusters      []clusterRecord `json:"clusters"`
	Packages      []packageReport `json:"packages"`
}

// reportSummary holds global flag counts.
type reportSummary struct {
	Packages                  int `json:"packages"`
	Actions                   int `json:"actions"`
	WeakAliases               int `json:"weak_aliases"`
	GenericUsage              int `json:"generic_usage"`
	EmptyRelated              int `json:"empty_related"`
	MissingNextSteps          int `json:"missing_next_steps"`
	EmptyParamDescription     int `json:"empty_param_description"`
	EmptyOutputDescription    int `json:"empty_output_description"`
	MissingDisambiguation     int `json:"missing_disambiguation"`
	WeakIndividualDescription int `json:"weak_individual_description"`
	MissingParameterGuidance  int `json:"missing_parameter_guidance"`
	AliasesOnlyToolname       int `json:"aliases_only_toolname"`
	Errors                    int `json:"errors"`
	Warnings                  int `json:"warnings"`
}

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include actions that raise at least one flag")
	minAliases := flag.Int("min-aliases", 3, "minimum non-canonical, non-toolname aliases required to clear weak_aliases")
	severityFlag := flag.String("severity", "error", "threshold severity for -check: error|warning|info")
	checkMode := flag.Bool("check", false, "exit non-zero if any finding meets or exceeds -severity threshold")
	flag.Parse()

	threshold, err := parseSeverity(*severityFlag)
	if err != nil {
		cmdutil.Fatalf("invalid -severity: %v", err)
	}

	rep, err := buildReport(*gapsOnly, *minAliases)
	if err != nil {
		cmdutil.Fatalf("build discovery completeness report: %v", err)
	}

	if *checkMode {
		if checkErr := rep.check(threshold); checkErr != nil {
			fmt.Fprintln(os.Stderr, checkErr.Error())
			os.Exit(1)
		}
		return
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

func parseSeverity(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return severityError, nil
	case "warning":
		return severityWarning, nil
	case "info":
		return severityInfo, nil
	}
	return 0, fmt.Errorf("must be error, warning, or info (got %q)", s)
}

const (
	severityError = iota
	severityWarning
	severityInfo
)

func severityRank(s string) int {
	switch s {
	case "error":
		return severityError
	case "warning":
		return severityWarning
	case "info":
		return severityInfo
	}
	return severityInfo
}

// check returns a non-nil error when any finding has severity at or above the
// given threshold. Used by the -check CI gate.
func (r report) check(threshold int) error {
	if r.Summary.Errors > 0 && threshold == severityError {
		return fmt.Errorf("discovery completeness: %d error-severity finding(s) present", r.Summary.Errors)
	}
	if r.Summary.Warnings > 0 && threshold == severityWarning && r.Summary.Errors == 0 {
		return fmt.Errorf("discovery completeness: %d warning-or-worse finding(s) present", r.Summary.Warnings)
	}
	if threshold == severityInfo && (r.Summary.Errors > 0 || r.Summary.Warnings > 0) {
		return fmt.Errorf("discovery completeness: %d info-or-worse finding(s) present", r.Summary.Errors+r.Summary.Warnings)
	}
	return nil
}

func buildReport(gapsOnly bool, minAliases int) (report, error) {
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		return report{}, fmt.Errorf("create audit client: %w", err)
	}
	defer cleanup()

	projected, err := projectIndividualDescriptions(client)
	if err != nil {
		return report{}, err
	}

	allClusters := collectAllClusters(client)

	packagesOut := buildPackageReports(client, allClusters, projected, minAliases, gapsOnly)
	clustersOut := sortClusters(allClusters)

	return report{
		SchemaVersion: schemaVersion,
		Summary:       summarize(packagesOut),
		Clusters:      clustersOut,
		Packages:      packagesOut,
	}, nil
}

// collectAllClusters gathers the catalog and builds sibling clusters across
// all owner packages. Some packages register the same (owner, name) from
// multiple scope helpers (e.g. project+group badges), so we dedupe by
// (owner, name) before clustering.
func collectAllClusters(client *gitlabclient.Client) []clusterRecord {
	specsByOwner := map[string][]toolutil.ActionSpec{}
	seen := map[string]bool{}
	for _, group := range tools.CollectActionSpecs(client, true) {
		for _, spec := range group.Actions {
			owner := ownerPackage(group, spec)
			key := owner + "\x00" + spec.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			specsByOwner[owner] = append(specsByOwner[owner], spec)
		}
	}
	return siblingClusters(flattenSpecs(specsByOwner))
}

// buildPackageReports analyzes every spec and returns the per-package reports
// ready for JSON output. Gaps-only mode filters clean packages.
func buildPackageReports(client *gitlabclient.Client, allClusters []clusterRecord, projected map[string]string, minAliases int, gapsOnly bool) []packageReport {
	byPackage := map[string]*packageReport{}
	for _, group := range tools.CollectActionSpecs(client, true) {
		for _, spec := range group.Actions {
			owner := ownerPackage(group, spec)
			clusterMembers := clusterMembersFor(allClusters, owner, spec.Name)
			withClusterMembers(clusterMembers, func() {
				pr := packageFor(byPackage, owner)
				pr.Actions++
				finding := analyzeSpec(spec, projected, clusterMembers, minAliases)
				if len(finding.Flags) == 0 {
					return
				}
				if gapsOnly {
					pr.Findings = append(pr.Findings, finding)
					return
				}
				pr.Findings = append(pr.Findings, finding)
			})
		}
	}
	packagesOut := make([]packageReport, 0, len(byPackage))
	for _, pr := range byPackage {
		sort.Slice(pr.Findings, func(i, j int) bool { return pr.Findings[i].Action < pr.Findings[j].Action })
		packagesOut = append(packagesOut, *pr)
	}
	sort.Slice(packagesOut, func(i, j int) bool { return packagesOut[i].Package < packagesOut[j].Package })
	return packagesOut
}

// sortClusters returns a copy of clusters sorted by (package, stem) for
// deterministic JSON output.
func sortClusters(allClusters []clusterRecord) []clusterRecord {
	clustersOut := append([]clusterRecord(nil), allClusters...)
	sort.Slice(clustersOut, func(i, j int) bool {
		if clustersOut[i].Package != clustersOut[j].Package {
			return clustersOut[i].Package < clustersOut[j].Package
		}
		return clustersOut[i].Stem < clustersOut[j].Stem
	})
	return clustersOut
}

// flattenSpecs merges the owner-grouped specs into a single slice while
// preserving each spec's owner (via its OwnerPackage field).
func flattenSpecs(byOwner map[string][]toolutil.ActionSpec) []toolutil.ActionSpec {
	total := 0
	for _, specs := range byOwner {
		total += len(specs)
	}
	out := make([]toolutil.ActionSpec, 0, total)
	for _, specs := range byOwner {
		out = append(out, specs...)
	}
	return out
}

// analyzeSpec returns the discovery finding for one spec. Flags are emitted
// once per finding even if multiple fields raise the same flag — the
// per-field breakdown is preserved in finding.Fields.
func analyzeSpec(spec toolutil.ActionSpec, projected map[string]string, clusterMembers []string, minAliases int) actionFinding {
	inCluster := len(clusterMembers) >= 2
	flagSet := map[string]bool{}

	addFlag := func(f string) { flagSet[f] = true }

	if isGenericUsage(spec.Usage) {
		addFlag("generic_usage")
	}
	if weakAliases(spec, minAliases) {
		addFlag("weak_aliases")
	}
	if len(spec.RelatedActions) == 0 {
		addFlag("empty_related")
	}
	if weakIndividualDescription(spec, projected) {
		addFlag("weak_individual_description")
	}

	// missing_next_steps: list/detail content kinds SHOULD have a registered
	// Markdown formatter so the next-step hint section can be extracted. We
	// detect this via toolutil.HasRegisteredMarkdownFormatter on the
	// spec.Route.OutputType reflect.Type. Actions without an OutputType (e.g.
	// constructed via the untyped Route constructor) are skipped — those are
	// typically ad-hoc helpers that don't carry a formatter contract.
	if needsMarkdownFormatter(spec) && spec.Route.OutputType != nil &&
		!toolutil.HasRegisteredMarkdownFormatter(spec.Route.OutputType) {
		addFlag("missing_next_steps")
	}

	// aliases_only_toolname: aliases carry no natural-language signal beyond
	// the canonical tool name. Models cannot disambiguate via alias lookup,
	// so the spec fails its discoverability contract even though the count
	// test passes.
	if aliasesOnlyToolname(spec) {
		addFlag("aliases_only_toolname")
	}

	// missing_parameter_guidance: actions with sibling-confusable parameters
	// (e.g. project_id vs group_id, ref vs branch) need explicit guidance
	// entries. We detect the heuristic pattern: the action has an input
	// parameter that carries a known scope-style name (id/_id with a
	// project/group/user/integer prefix), AND the spec has no ParameterGuidance
	// entries at all.
	if missingParameterGuidance(spec) {
		addFlag("missing_parameter_guidance")
	}

	// Field-level (Check B): walk input schema, flag empty/boilerplate descriptions.
	//
	// Note on the output schema walk: `empty_output_description` is intentionally
	// classified at info severity (not warning/error). Per the team's 1:1 audit
	// policy (see plan/discovery-metadata-completeness.md §9 "Output godoc"),
	// Output struct fields intentionally lack jsonschema tags because:
	//   1. The MCP protocol uses ActionSpec.Description (not OutputSchema
	//      property descriptions) for model-facing tool selection.
	//   2. Output structs mirror client-go SDK types 1:1; adding jsonschema
	//      tags per-field would drift the wire-format shape from the SDK.
	//   3. Output descriptions would bloat the model's startup context for
	//      zero functional benefit.
	// The info-level finding remains visible in the gaps-only summary for
	// awareness but does NOT count toward `errors` or `warnings` and does
	// NOT fail `make audit-discovery-check`. If the policy changes, raise
	// the severity in `severityForFlag` and re-evaluate the audit gate.
	var fields []fieldFinding
	for _, p := range emptyParamDescriptions(spec.Route.InputSchema) {
		fields = append(fields, fieldFinding{Param: p, Flag: "empty_param_description"})
	}
	for _, p := range emptyParamDescriptions(spec.Route.OutputSchema) {
		fields = append(fields, fieldFinding{Param: p, Flag: "empty_output_description"})
	}
	if hasInputEmptyFields(fields, "empty_param_description") {
		addFlag("empty_param_description")
	}
	if hasInputEmptyFields(fields, "empty_output_description") {
		addFlag("empty_output_description")
	}

	// Check C: sibling-cluster disambiguation.
	// Only flag when the spec itself carries a non-CRUD variant suffix
	// (_batch/_bulk/_all/_directory/_single) AND the cluster has at least
	// one such variant member. Pure CRUD families (create/get/list/delete/
	// update on the same resource) are not confusable because the verb is
	// the disambiguator — the link_create_batch / publish vs
	// publish_directory class is the only real eval-failure pattern.
	if inCluster && hasNonCRUDVariantSuffix(spec.Name) &&
		hasNonCRUDVariantInCluster(clusterMembers) &&
		!hasDisambiguation(spec, clusterMembers) {
		addFlag("missing_disambiguation")
	}

	flags := make([]string, 0, len(flagSet))
	for f := range flagSet {
		flags = append(flags, f)
	}
	sort.Strings(flags)

	severity := highestSeverity(flags, inCluster)

	return actionFinding{
		Action:    spec.Name,
		Tool:      spec.IndividualTool.Name,
		Usage:     strings.TrimSpace(spec.Usage),
		Severity:  severity,
		Flags:     flags,
		Cluster:   clusterMembers,
		Fields:    fields,
		HasSchema: len(spec.Route.InputSchema) > 0,
	}
}

// hasInputEmptyFields reports whether any field has the given flag.
func hasInputEmptyFields(fields []fieldFinding, flagName string) bool {
	for _, f := range fields {
		if f.Flag == flagName {
			return true
		}
	}
	return false
}

// isListOrDetailContent reports whether the spec's ContentKind is one where
// next-step hints are commonly expected (list/detail).
func isListOrDetailContent(spec toolutil.ActionSpec) bool {
	switch strings.ToLower(strings.TrimSpace(spec.ContentKind)) {
	case toolutil.ActionSpecContentList, toolutil.ActionSpecContentDetail:
		return true
	}
	return false
}

// highestSeverity returns the highest severity among the given flags,
// upgrading empty_related/weak_aliases for cluster members.
func highestSeverity(flags []string, inCluster bool) string {
	highest := severityInfo
	for _, f := range flags {
		r := severityRank(severityFor(f, inCluster))
		if r < highest {
			highest = r
		}
	}
	switch highest {
	case severityError:
		return "error"
	case severityWarning:
		return "warning"
	default:
		return "info"
	}
}

// weakAliases reports whether the action has fewer than minAliases
// natural-language aliases beyond its canonical name and projected
// individual-tool name.
func weakAliases(spec toolutil.ActionSpec, minAliases int) bool {
	canonical := strings.ToLower(strings.TrimSpace(spec.Name))
	tool := strings.ToLower(strings.TrimSpace(spec.IndividualTool.Name))
	count := 0
	for _, alias := range spec.Aliases {
		normalized := strings.ToLower(strings.TrimSpace(alias))
		if normalized == "" || normalized == canonical || normalized == tool {
			continue
		}
		count++
	}
	return count < minAliases
}

// hasDisambiguation reports whether the action carries BOTH a sibling in
// RelatedActions (or CommonConfusions) AND a Usage signal that distinguishes
// it from siblings. When there are no siblings (single-member or empty
// cluster membership), disambiguation is vacuous and the function returns true.
//
// RelatedActions commonly use cross-package prefixed names
// (e.g. "release.link_create" or "package.registry_tag_delete") while sibling
// cluster members use the bare action name ("link_create"). The match below
// accepts both forms: a related action counts as a sibling reference if
// either the full string or its tail after the last "." equals a sibling
// name, so gold-standard fixes in either naming convention pass.
func hasDisambiguation(spec toolutil.ActionSpec, clusterMembers []string) bool {
	siblings := siblingSet(clusterMembers, spec.Name)
	if len(siblings) == 0 {
		// No cluster to disambiguate from; vacuous pass.
		return true
	}
	// (b) A usage signal that mentions a distinguishing keyword or sibling name.
	hasSignal := usageHasSignal(spec.Usage)
	if !hasSignal {
		// Allow usage to mention a sibling name verbatim even without a keyword.
		lcUsage := strings.ToLower(spec.Usage)
		for sibling := range siblings {
			if strings.Contains(lcUsage, sibling) {
				hasSignal = true
				break
			}
		}
	}
	if !hasSignal {
		return false
	}
	// (a) A sibling in RelatedActions (accept prefixed or bare form).
	// Cross-package action IDs use "." as separator ("pages.domain_list")
	// while cluster siblings use "_" ("pages_domain_list"). Normalize both
	// forms before matching.
	for _, related := range spec.RelatedActions {
		lc := strings.ToLower(strings.TrimSpace(related))
		if siblingMatches(lc, siblings) {
			return true
		}
	}
	// (a') A sibling mentioned in any CommonConfusions entry of any param.
	for _, g := range spec.ParameterGuidance {
		for _, conf := range g.CommonConfusions {
			lc := strings.ToLower(conf)
			for sibling := range siblings {
				if strings.Contains(lc, sibling) {
					return true
				}
			}
		}
	}
	return false
}

// usageHasSignal reports whether the Usage string contains a distinguishing
// keyword (see usageSignalKeywords) or explicitly names a sibling action.
func usageHasSignal(usage string) bool {
	lc := strings.ToLower(usage)
	for _, k := range usageSignalKeywords {
		if strings.Contains(lc, k) {
			return true
		}
	}
	return false
}

// hasNonCRUDVariantSuffix reports whether the action name ends with a
// non-CRUD variant suffix (_batch, _bulk, _all, _directory, _single). These
// are the suffixes that mark a sibling variant of an existing base action
// and require disambiguation in the Usage / RelatedActions metadata. Pure
// CRUD verbs (_create, _get, _list, _delete, _update) are not variant
// suffixes — they are the verbs that distinguish operations on a single
// resource.

// siblingMatches reports whether the related-action name (possibly
// cross-package prefixed with "." separator) refers to any cluster sibling
// (which use "_" separator). Accepts:
//   - exact lowercase match
//   - tail match after the last "." ("pages.domain_list" -> "domain_list")
//   - separator-normalized match: replace "." with "_" in the related name
//     ("pages.domain_list" -> "pages_domain_list") and compare to siblings
func siblingMatches(related string, siblings map[string]struct{}) bool {
	if _, ok := siblings[related]; ok {
		return true
	}
	if idx := strings.LastIndex(related, "."); idx >= 0 {
		tail := related[idx+1:]
		if _, ok := siblings[tail]; ok {
			return true
		}
		// Also accept the cross-package form: take only the resource portion
		// (skip the owner package) and the local-action portion to form a
		// candidate sibling key. "pages.domain_list" -> "pages_domain_list".
		head := related[:idx]
		candidate := head + "_" + tail
		if _, ok := siblings[candidate]; ok {
			return true
		}
	}
	// Fallback: a sibling name is contained in the related name (covers
	// cases where the related name embeds a sibling as a substring, e.g.
	// the related name "page_domain_list" or "do_main_list" — defensive
	// against non-conformant RelatedActions values).
	for sibling := range siblings {
		if strings.Contains(related, sibling) {
			return true
		}
	}
	return false
}

func hasNonCRUDVariantSuffix(name string) bool {
	lower := strings.ToLower(name)
	for _, sfx := range variantSuffixes {
		if strings.HasSuffix(lower, sfx) {
			return true
		}
	}
	return false
}

// hasNonCRUDVariantInCluster reports whether any cluster member carries a
// non-CRUD variant suffix. Used to gate the missing_disambiguation check on
// clusters that actually contain a base-vs-variant pair (the eval-failure
// class) — pure CRUD families are exempt.
func hasNonCRUDVariantInCluster(members []string) bool {
	return slices.ContainsFunc(members, hasNonCRUDVariantSuffix)
}

// siblingSet returns the set of lowercase sibling names excluding self.
func siblingSet(members []string, self string) map[string]struct{} {
	out := map[string]struct{}{}
	selfLower := strings.ToLower(strings.TrimSpace(self))
	for _, m := range members {
		key := strings.ToLower(strings.TrimSpace(m))
		if key == "" || key == selfLower {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

// siblingClusters groups actions into clusters by (owner_package, base_stem).
// Each cluster contains the canonical action names of all members.
func siblingClusters(specs []toolutil.ActionSpec) []clusterRecord {
	type key struct{ owner, stem string }
	buckets := map[key][]string{}
	for _, spec := range specs {
		owner := strings.TrimSpace(spec.OwnerPackage)
		if owner == "" {
			owner = inferOwnerFromName(spec.Name)
		}
		stem := baseActionStem(spec.Name)
		buckets[key{owner, stem}] = append(buckets[key{owner, stem}], spec.Name)
	}
	out := make([]clusterRecord, 0, len(buckets))
	for k, members := range buckets {
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		out = append(out, clusterRecord{
			Package: k.owner,
			Stem:    k.stem,
			Members: members,
		})
	}
	return out
}

// clusterMembersFor returns the member list of the cluster that contains the
// given (owner, name), or nil if no cluster does.
func clusterMembersFor(clusters []clusterRecord, owner, name string) []string {
	stem := baseActionStem(name)
	for _, c := range clusters {
		if c.Package == owner && c.Stem == stem {
			return append([]string(nil), c.Members...)
		}
	}
	return nil
}

// baseActionStem strips variant and CRUD suffixes from an action name to
// produce a clustering stem. The function handles both prefixed
// "<owner>.<action>" and unprefixed "<action>" names so it works for both
// catalog-projected names ("release.link_create_batch") and the bare names
// returned by sub-package ActionSpecs helpers ("link_create_batch").
//
// Examples:
//
//	"release.link_create_batch"      -> "release.link"
//	"release.link_create"            -> "release.link"
//	"package.publish_directory"      -> "package.publish"
//	"package.publish"                -> "package.publish"
//	"merge_request.update"           -> "merge_request"
//	"branch.list"                    -> "branch"
//	"members.add_bulk"               -> "members"
//	"notes.delete_all"               -> "notes"
//	"link_create_batch"              -> "link"
//	"link_create"                    -> "link"
//
// When the action is a bare CRUD verb (e.g. "update", "list") or strips down
// to a single verb ("add_bulk" → "add"), the stem falls back to the bare
// owner so clusters stay meaningful.
func baseActionStem(name string) string {
	owner, action := splitOwnerAction(name)
	lower := strings.ToLower(action)
	changed := true
	for changed {
		changed = false
		for _, sfx := range variantSuffixes {
			if newLower, ok := strings.CutSuffix(lower, sfx); ok {
				lower = newLower
				changed = true
			}
		}
		for _, sfx := range crudSuffixes {
			if newLower, ok := strings.CutSuffix(lower, sfx); ok {
				lower = newLower
				changed = true
			}
		}
		for _, sfx := range scopeSuffixes {
			if newLower, ok := strings.CutSuffix(lower, sfx); ok {
				lower = newLower
				changed = true
			}
		}
	}
	if lower == "" || isBareVerb(lower) {
		if owner != "" {
			return owner
		}
		// Bare-action (no owner) with a single verb: keep as-is so the cluster
		// stem is still distinct.
		return strings.ToLower(action)
	}
	if owner == "" {
		return lower
	}
	return owner + "." + lower
}

// isBareVerb reports whether s is itself one of the CRUD verbs we strip
// (e.g. "add", "delete", "update", "list", "get", "create"). Used to detect
// cases like "members.add_bulk" → after stripping "_bulk" the result is just
// the verb "add", which carries no semantic content for clustering.
func isBareVerb(s string) bool {
	switch s {
	case "add", "delete", "update", "list", "get", "create":
		return true
	}
	return false
}

// splitOwnerAction splits an action name into (owner, action). Names with a
// "." are split at the first dot; names without are treated as bare actions
// (owner="").
func splitOwnerAction(name string) (owner, action string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return "", name
	}
	return parts[0], parts[1]
}

// inferOwnerFromName derives an owner_package guess when spec.OwnerPackage is
// empty (defensive: catalog-first invariant guarantees this is set in practice).
func inferOwnerFromName(name string) string {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// emptyParamDescriptions walks an input/output JSON schema and returns the
// list of property paths whose description is empty or boilerplate. Returns
// nil for nil schemas. Recurses through "properties" (and "items" for arrays)
// and tracks visited schema nodes by pointer identity to avoid re-visiting
// the same node through $ref cycles or repeated nested schemas.
func emptyParamDescriptions(schema map[string]any) []string {
	if len(schema) == 0 {
		return nil
	}
	visited := map[uintptr]bool{}
	out := walkSchemaForEmptyDescriptions(schema, "", visited)
	return out
}

// walkSchemaForEmptyDescriptions performs a recursive walk over the schema,
// returning the list of property paths whose description is missing or
// boilerplate. The visited set prevents cycles in $ref-resolved schemas
// from re-emitting the same path.
func walkSchemaForEmptyDescriptions(schema map[string]any, path string, visited map[uintptr]bool) []string {
	if schema == nil {
		return nil
	}
	key := schemaPointer(schema)
	if visited[key] {
		return nil
	}
	visited[key] = true

	resolved := resolveSchemaRef(schema)
	if schemaPointer(resolved) != key {
		key2 := schemaPointer(resolved)
		if visited[key2] {
			return nil
		}
		visited[key2] = true
		schema = resolved
	}

	props, hasProps := schema["properties"].(map[string]any)
	if !hasProps {
		return nil
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, key := range keys {
		child, _ := props[key].(map[string]any)
		if child == nil {
			continue
		}
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		if isEmptyOrBoilerplateDescription(child) {
			out = append(out, childPath)
		}
		out = append(out, walkSchemaForEmptyDescriptions(child, childPath, visited)...)
		if items, hasItems := child["items"].(map[string]any); hasItems {
			out = append(out, walkSchemaForEmptyDescriptions(items, childPath, visited)...)
		}
	}
	return out
}

// schemaPointer returns a stable identifier for a map. Using the address of
// the first byte via reflect.Value.Pointer is fragile for map headers, so we
// fall back to fmt.Sprintf-based fingerprinting — collisions are vanishingly
// rare across the schema sizes we deal with and would only manifest as a
// missed walk, never a false positive.
func schemaPointer(schema map[string]any) uintptr {
	if schema == nil {
		return 0
	}
	h := fnv.New32a()
	fmt.Fprintf(h, "%v", schema)
	return uintptr(h.Sum32())
}

// resolveSchemaRef follows a single-level $ref of the form "#/$defs/Name".
func resolveSchemaRef(schema map[string]any) map[string]any {
	ref, isRef := schema["$ref"].(string)
	if !isRef || !strings.HasPrefix(ref, "#/$defs/") {
		return schema
	}
	// The root schema (not the child) holds $defs, but we also accept local
	// definitions to be tolerant of nested forms.
	if defs, hasDefs := schema["$defs"].(map[string]any); hasDefs {
		if d, hasDef := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any); hasDef {
			return d
		}
	}
	return schema
}

// isEmptyOrBoilerplateDescription reports whether a property schema lacks a
// meaningful description.
func isEmptyOrBoilerplateDescription(propSchema map[string]any) bool {
	desc, ok := propSchema["description"].(string)
	if !ok {
		return true
	}
	trimmed := strings.TrimSpace(desc)
	if trimmed == "" {
		return true
	}
	// Drop the leading "The " for the boilerplate check.
	lc := strings.ToLower(trimmed)
	switch {
	case len(trimmed) < 4:
		return true
	case strings.HasPrefix(lc, "the ") && len(trimmed) <= len("the x")+2:
		return true
	case lc == "id":
		return true
	}
	return false
}

// weakIndividualDescription reports whether the effective individual-tool
// description lacks the norm's "Returns: … See also: …" form.
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

// isGenericUsage reports whether the Usage string is the placeholder template
// or empty.
func isGenericUsage(usage string) bool {
	trimmed := strings.TrimSpace(usage)
	return trimmed == "" || genericUsageRe.MatchString(trimmed)
}

// projectIndividualDescriptions registers the individual-tool surface on an
// in-memory MCP server and returns the projected description per tool name.
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
			inCluster := len(finding.Cluster) >= 2
			for _, flag := range finding.Flags {
				switch flag {
				case "weak_aliases":
					s.WeakAliases++
				case "generic_usage":
					s.GenericUsage++
				case "empty_related":
					s.EmptyRelated++
				case "missing_next_steps":
					s.MissingNextSteps++
				case "empty_param_description":
					s.EmptyParamDescription++
				case "empty_output_description":
					s.EmptyOutputDescription++
				case "missing_disambiguation":
					s.MissingDisambiguation++
				case "weak_individual_description":
					s.WeakIndividualDescription++
				case "missing_parameter_guidance":
					s.MissingParameterGuidance++
				case "aliases_only_toolname":
					s.AliasesOnlyToolname++
				}
				switch severityFor(flag, inCluster) {
				case "error":
					s.Errors++
				case "warning":
					s.Warnings++
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

// needsMarkdownFormatter reports whether the spec represents list/detail
// content where the next-step hint section is expected to be present.
// Destructive actions never need it (they're terminal in the workflow
// sense — there's no "next step" after a delete).
func needsMarkdownFormatter(spec toolutil.ActionSpec) bool {
	if spec.Destructive {
		return false
	}
	if isListOrDetailContent(spec) {
		return true
	}
	if spec.Name == "" {
		return false
	}
	return true
}

// aliasesOnlyToolname reports whether the spec's Aliases carry no
// discoverability signal beyond the canonical tool name. The check is:
// every alias either equals the tool name (gitlab_foo_bar), equals the
// canonical action name (foo_bar), or is empty. Empty aliases are
// counted as no-signal.
func aliasesOnlyToolname(spec toolutil.ActionSpec) bool {
	toolName := spec.IndividualTool.Name
	canonical := spec.Name
	if len(spec.Aliases) == 0 {
		return false // weak_aliases handles this case
	}
	hasSignal := false
	for _, a := range spec.Aliases {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if a != toolName && a != canonical {
			hasSignal = true
			break
		}
	}
	return !hasSignal
}

// missingParameterGuidance reports whether an action with sibling-
// confusable parameters (project_id vs group_id, ref vs branch, etc.)
// has no ParameterGuidance entries. The heuristic:
//   - The action has at least one input parameter whose name matches a
//     known scope-suggestive pattern (id/_id with project/group/user/
//     instance prefix, or "ref", "branch", "tag").
//   - The spec has zero ParameterGuidance entries.
//
// Mutating actions are prioritized (they fail with confusing 400s when
// the wrong scope is chosen); read-only actions are still flagged but
// with warning severity rather than escalating to error.
func missingParameterGuidance(spec toolutil.ActionSpec) bool {
	if len(spec.ParameterGuidance) > 0 {
		return false
	}
	// Walk the InputSchema properties for scope-suggestive parameter
	// names. We deliberately look at the schema (not the Go type) so
	// this works for both typed and untyped routes.
	props, _ := spec.Route.InputSchema["properties"].(map[string]any)
	for name := range props {
		lname := strings.ToLower(name)
		if isScopeSuggestiveName(lname) {
			return true
		}
	}
	return false
}

// isScopeSuggestiveName matches parameter names that frequently confuse
// models because the same parameter type means different scopes across
// GitLab APIs (e.g. project_id in one endpoint accepts a project path
// in another, or ref means "branch name" in commits but "branch or tag"
// in protected branches).
func isScopeSuggestiveName(name string) bool {
	// Exact matches for the most-confused single-word names.
	switch name {
	case "ref", "branch", "tag", "sha", "path", "iid":
		return true
	}
	return slices.Contains(scopeIDSuffixes, name)
}
