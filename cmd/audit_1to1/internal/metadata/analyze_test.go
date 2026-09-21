package metadata

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestIsGenericUsage_FlagsPlaceholders verifies the placeholder-Usage detector
// matches the generated "Use to execute … action." templates and empty strings
// while leaving purpose-specific sentences untouched.
func TestIsGenericUsage_FlagsPlaceholders(t *testing.T) {
	generic := []string{
		"",
		"   ",
		"Use to execute branches domain action.",
		"Use to execute list action.",
		"use to execute markdown_render action",
	}
	for _, usage := range generic {
		t.Run(usage, func(t *testing.T) {
			if !auditshared.IsGenericUsage(usage) {
				t.Errorf("isGenericUsage(%q) = false, want true", usage)
			}
		})
	}
	specific := []string{
		"List branches for a project with optional search and pagination.",
		"Create a new branch from a ref. Returns the created branch.",
	}
	for _, usage := range specific {
		t.Run(usage, func(t *testing.T) {
			if auditshared.IsGenericUsage(usage) {
				t.Errorf("isGenericUsage(%q) = true, want false", usage)
			}
		})
	}
}

// TestAliasesOnlyToolname verifies that an action with no alias beyond its
// canonical name and individual-tool name is flagged, while a natural-language
// alias clears the flag.
func TestAliasesOnlyToolname(t *testing.T) {
	bare := toolutil.ActionSpec{
		Name: "branch.create",
		// The blank entries are the third way an alias says nothing: a spec
		// whose alias list holds only empty strings is as bare as one holding
		// only its own two names, and nothing else here has a blank alias.
		Aliases:        []string{"gitlab_branch_create", "branch.create", "", "   "},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_branch_create"},
	}
	if !aliasesOnlyToolname(bare) {
		t.Error("expected bare aliases to be flagged")
	}
	rich := toolutil.ActionSpec{
		Name:           "branch.create",
		Aliases:        []string{"gitlab_branch_create", "create branch", "new branch"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_branch_create"},
	}
	if aliasesOnlyToolname(rich) {
		t.Error("expected natural-language aliases to clear the flag")
	}
}

// TestWeakIndividualDescription verifies the effective-description quality check
// against the projected description map and the norm's "Returns:/See also:"
// structure.
func TestWeakIndividualDescription(t *testing.T) {
	spec := toolutil.ActionSpec{IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_branch_get"}}
	good := map[string]string{"gitlab_branch_get": "Fetch a branch. Returns: branch fields. See also: gitlab_branch_list."}
	if auditshared.WeakIndividualDescription(spec, good) {
		t.Error("structured description should not be flagged")
	}
	weak := map[string]string{"gitlab_branch_get": "Fetch a branch."}
	if !auditshared.WeakIndividualDescription(spec, weak) {
		t.Error("unstructured description should be flagged")
	}
	// Meta-only action (no individual tool) is never flagged.
	metaOnly := toolutil.ActionSpec{Name: "server.health_check"}
	if auditshared.WeakIndividualDescription(metaOnly, weak) {
		t.Error("meta-only action should not be flagged")
	}
	// Unknown tool name (not projected) is not flagged.
	if auditshared.WeakIndividualDescription(spec, map[string]string{}) {
		t.Error("unprojected tool should not be flagged")
	}
}

// TestBuildReport_DetectsKnownMetadataGaps runs the auditor and asserts the
// fix-agnostic invariants plus the presence of the known generic-Usage backlog.
// This is the R-META methodology regression guard.
func TestBuildReport_DetectsKnownMetadataGaps(t *testing.T) {
	rep := buildReport(false)
	if rep.Summary.Actions == 0 {
		t.Fatalf("no actions analyzed: %+v", rep.Summary)
	}
	// The projected individual descriptions are curated, so weak descriptions
	// must be rare — guards against the earlier false-positive overcount.
	if rep.Summary.WeakIndividualDescription > 50 {
		t.Errorf("weak_individual_description = %d, expected a small number (curated descriptions)", rep.Summary.WeakIndividualDescription)
	}
	// The header must be the count of what the report carries rather than a
	// number of its own: the two figures are far apart on the real catalog,
	// so a summary that read one off the other would say so here.
	var actions int
	var prev string
	for _, pr := range rep.Packages {
		actions += pr.Actions
		if pr.Package < prev {
			t.Errorf("packages not sorted: %q before %q", prev, pr.Package)
		}
		prev = pr.Package
	}
	if rep.Summary.Packages != len(rep.Packages) || rep.Summary.Actions != actions {
		t.Errorf("summary = %d packages / %d actions, want %d / %d", rep.Summary.Packages, rep.Summary.Actions, len(rep.Packages), actions)
	}
}

// TestBuildReport_GapsOnly_DropsThePackagesThatRaiseNothing verifies the flag
// the command takes reaches the grouping rather than being forwarded wrongly
// or not at all, which no gate can see because it is one argument passed
// along. The full report carries every owner package of the catalog and the
// gaps-only one only those that raise a finding, so the two disagree while any
// package is clean; a forward that dropped or inverted the flag makes them
// agree. The curated catalog raises nothing today, which is why the second
// assertion holds vacuously and the first is what does the work.
func TestBuildReport_GapsOnly_DropsThePackagesThatRaiseNothing(t *testing.T) {
	full := buildReport(false)
	gaps := buildReport(true)
	if len(full.Packages) <= len(gaps.Packages) {
		t.Fatalf("full report = %d packages, gaps-only = %d; want the full one to keep packages gaps-only drops", len(full.Packages), len(gaps.Packages))
	}
	kept := map[string]bool{}
	for _, pr := range gaps.Packages {
		kept[pr.Package] = true
		if len(pr.Findings) == 0 {
			t.Errorf("gaps-only kept %q, which raises no finding", pr.Package)
		}
	}
	for _, pr := range full.Packages {
		if len(pr.Findings) > 0 && !kept[pr.Package] {
			t.Errorf("gaps-only dropped %q, which raises %d findings", pr.Package, len(pr.Findings))
		}
	}
}

// TestBuildReport_Deterministic verifies repeated runs are identical.
func TestBuildReport_Deterministic(t *testing.T) {
	first := buildReport(true)
	second := buildReport(true)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("buildReport is not deterministic across runs")
	}
}

// curatedProjection is the projected description map the synthetic specs
// below are judged against: one structured description and one bare one.
var curatedProjection = map[string]string{
	"gitlab_branch_get":  "Fetch a branch. Returns: branch fields. See also: gitlab_branch_list.",
	"gitlab_branch_list": "List branches.",
}

// cleanSpec is an action that raises no R-META flag.
var cleanSpec = toolutil.ActionSpec{
	Name:           "branch.get",
	Usage:          "  Fetch one branch by name.  ",
	Aliases:        []string{"gitlab_branch_get", "get branch"},
	RelatedActions: []string{"branch.list"},
	IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_branch_get"},
}

// withSpec returns a copy of cleanSpec with one field changed.
func withSpec(mutate func(spec *toolutil.ActionSpec)) toolutil.ActionSpec {
	spec := cleanSpec
	spec.Aliases = append([]string(nil), spec.Aliases...)
	spec.RelatedActions = append([]string(nil), spec.RelatedActions...)
	mutate(&spec)
	return spec
}

// TestAnalyzeSpec_Flags_RaiseEachFinding verifies every R-META flag on its
// own and together, that a clean spec raises none, and that gaps-only mode
// drops the clean finding entirely while the full mode still records it.
func TestAnalyzeSpec_Flags_RaiseEachFinding(t *testing.T) {
	cases := []struct {
		name      string
		spec      toolutil.ActionSpec
		gapsOnly  bool
		wantFlags []string
		wantOK    bool
		wantEmpty bool
	}{
		{name: "clean_spec_is_recorded_without_flags", spec: cleanSpec, wantFlags: nil, wantOK: false},
		{name: "clean_spec_is_dropped_in_gaps_only", spec: cleanSpec, gapsOnly: true, wantOK: false, wantEmpty: true},
		{
			name:      "generic_usage",
			spec:      withSpec(func(s *toolutil.ActionSpec) { s.Usage = "Use to execute branches domain action." }),
			wantFlags: []string{"generic_usage"},
			wantOK:    true,
		},
		{
			name:      "aliases_only_toolname",
			spec:      withSpec(func(s *toolutil.ActionSpec) { s.Aliases = []string{"gitlab_branch_get", " branch.get "} }),
			wantFlags: []string{"aliases_only_toolname"},
			wantOK:    true,
		},
		{
			name:      "empty_related",
			spec:      withSpec(func(s *toolutil.ActionSpec) { s.RelatedActions = nil }),
			wantFlags: []string{"empty_related"},
			wantOK:    true,
		},
		{
			name:      "weak_individual_description",
			spec:      withSpec(func(s *toolutil.ActionSpec) { s.IndividualTool.Name = "gitlab_branch_list" }),
			wantFlags: []string{"weak_individual_description"},
			wantOK:    true,
		},
		{
			name: "every_flag_in_report_order",
			spec: toolutil.ActionSpec{
				Name:           "branch.list",
				Usage:          "",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_branch_list"},
			},
			gapsOnly:  true,
			wantFlags: []string{"generic_usage", "aliases_only_toolname", "empty_related", "weak_individual_description"},
			wantOK:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finding, ok := analyzeSpec(tc.spec, curatedProjection, tc.gapsOnly)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantEmpty {
				if !reflect.DeepEqual(finding, actionFinding{}) {
					t.Errorf("finding = %+v, want the zero finding in gaps-only mode", finding)
				}
				return
			}
			if !reflect.DeepEqual(finding.Flags, tc.wantFlags) {
				t.Errorf("flags = %v, want %v", finding.Flags, tc.wantFlags)
			}
			if finding.Action != tc.spec.Name || finding.Tool != tc.spec.IndividualTool.Name || finding.Usage != strings.TrimSpace(tc.spec.Usage) {
				t.Errorf("finding = %+v, want action %q, tool %q, trimmed usage", finding, tc.spec.Name, tc.spec.IndividualTool.Name)
			}
		})
	}
}

// TestSummarize_Findings_CountsEachFlag verifies the summary tallies the
// packages, their action counts and every flag by name, and ignores a flag
// it does not know rather than miscounting it.
//
// No two of the six counts agree, because a figure a sibling shares is one
// the two fields could trade places over unseen: three flags reading 1 let any
// pair of the four cases swap counters, and an empty_related tally equal to the
// package count let the summary's own header trade places with it.
func TestSummarize_Findings_CountsEachFlag(t *testing.T) {
	s := summarize([]packageReport{
		{Package: "a", Actions: 3, Findings: []actionFinding{
			{Action: "a.x", Flags: []string{"generic_usage", "empty_related", "empty_related"}},
			{Action: "a.y", Flags: []string{"aliases_only_toolname", "aliases_only_toolname", "weak_individual_description", "unknown_flag"}},
		}},
		{Package: "b", Actions: 2},
		{Package: "c", Actions: 1, Findings: []actionFinding{
			{Action: "c.z", Flags: []string{"empty_related", "empty_related", "empty_related", "weak_individual_description", "weak_individual_description", "weak_individual_description"}},
		}},
	})
	want := reportSummary{Packages: 3, Actions: 6, GenericUsage: 1, AliasesOnlyToolname: 2, EmptyRelated: 5, WeakIndividualDescription: 4}
	if s != want {
		t.Errorf("summarize = %+v, want %+v", s, want)
	}
}

// TestCollectPackages_Groups_GroupByOwnerAndSort verifies the grouping the
// report is built from: actions are filed under the spec's owner package
// override, then the group's owner, then its base domain; packages are
// sorted by name and findings by action; and gaps-only drops the packages
// without a finding while keeping their action counts out of the report.
func TestCollectPackages_Groups_GroupByOwnerAndSort(t *testing.T) {
	flagged := func(name, owner string) toolutil.ActionSpec {
		return withSpec(func(s *toolutil.ActionSpec) {
			s.Name = name
			s.OwnerPackage = owner
			s.RelatedActions = nil
		})
	}
	groups := []tools.ActionSpecGroup{
		{BaseDomain: "zeta", Actions: []toolutil.ActionSpec{cleanSpec, flagged("zeta.b", "")}},
		// grouped.a is the middle tier of the precedence: a spec with no owner
		// of its own in a group that has one. Without it the group's owner is
		// never the answer, since both of its siblings override it and the
		// base domain would have served them just as well.
		{BaseDomain: "alpha", OwnerPackage: "grouped", Actions: []toolutil.ActionSpec{flagged("custom.b", "custom"), flagged("custom.a", "custom"), flagged("grouped.a", "")}},
		// omega's two flagged actions arrive in the order the report wants
		// them, which is the case custom cannot show: a sort is only known to
		// decide anything once it has also left something alone.
		{BaseDomain: "omega", Actions: []toolutil.ActionSpec{flagged("omega.a", ""), flagged("omega.b", "")}},
		{BaseDomain: "clean", Actions: []toolutil.ActionSpec{cleanSpec}},
	}

	cases := []struct {
		name         string
		gapsOnly     bool
		wantPackages []string
	}{
		{name: "full_report_keeps_clean_packages", gapsOnly: false, wantPackages: []string{"clean", "custom", "grouped", "omega", "zeta"}},
		{name: "gaps_only_drops_clean_packages", gapsOnly: true, wantPackages: []string{"custom", "grouped", "omega", "zeta"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collectPackages(groups, curatedProjection, tc.gapsOnly)
			names := make([]string, 0, len(got))
			for _, pr := range got {
				names = append(names, pr.Package)
			}
			if !reflect.DeepEqual(names, tc.wantPackages) {
				t.Fatalf("packages = %v, want %v", names, tc.wantPackages)
			}
			for _, pr := range got {
				assertPackageShape(t, pr)
			}
		})
	}
}

// groupedPackageShapes is what each package of the grouping fixture must come
// out as: the actions it collected and the findings it kept, in the order the
// report writes them.
var groupedPackageShapes = map[string]struct {
	actions  int
	findings []string
}{
	// Its one action raises nothing, so it is the package the full report
	// keeps and the gaps-only one drops.
	"clean": {actions: 1},
	// Both specs override their group's owner, and they arrive reversed, so
	// this is the package whose findings the sort has to move.
	"custom": {actions: 2, findings: []string{"custom.a", "custom.b"}},
	// The one action that reaches the middle tier of the owner precedence.
	"grouped": {actions: 1, findings: []string{"grouped.a"}},
	// Already in order on arrival: a sort is only known to decide anything
	// once it has also left something alone.
	"omega": {actions: 2, findings: []string{"omega.a", "omega.b"}},
	// Two actions, one of them clean, so the count and the findings cannot be
	// read off each other.
	"zeta": {actions: 2, findings: []string{"zeta.b"}},
}

// assertPackageShape checks one grouped package against the whole record
// groupedPackageShapes declares for it, and reports a package the fixture
// never planted rather than passing over it.
func assertPackageShape(t *testing.T, pr packageReport) {
	t.Helper()
	want, planted := groupedPackageShapes[pr.Package]
	if !planted {
		t.Errorf("unexpected package %q in the grouped report", pr.Package)
		return
	}
	found := make([]string, 0, len(pr.Findings))
	for _, finding := range pr.Findings {
		found = append(found, finding.Action)
	}
	if pr.Actions != want.actions || !slices.Equal(found, want.findings) {
		t.Errorf("%s = %d actions with findings %v, want %d with %v", pr.Package, pr.Actions, found, want.actions, want.findings)
	}
}

// TestRun_GapsOnly_EmitsIndentedJSON verifies the command-facing entry
// point: the report is indented JSON with a trailing newline, carries the
// schema version, and lists the packages as an array even when the catalog
// raises no finding, which is the state the curated metadata is held to.
func TestRun_GapsOnly_EmitsIndentedJSON(t *testing.T) {
	content := Run(true)
	if !strings.HasSuffix(string(content), "}\n") {
		t.Error("report lacks the trailing newline")
	}
	var rep report
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	if rep.SchemaVersion != shared.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", rep.SchemaVersion, shared.SchemaVersion)
	}
	if !strings.Contains(string(content), "\"packages\": [") {
		t.Errorf("report should list packages as an array, got:\n%s", content)
	}
}
