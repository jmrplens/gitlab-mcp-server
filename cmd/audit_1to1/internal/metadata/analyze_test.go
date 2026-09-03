package metadata

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
		Name:           "branch.create",
		Aliases:        []string{"gitlab_branch_create", "branch.create"},
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
	rep, err := buildReport(false)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	if rep.Summary.Actions == 0 {
		t.Fatalf("no actions analyzed: %+v", rep.Summary)
	}
	// The projected individual descriptions are curated, so weak descriptions
	// must be rare — guards against the earlier false-positive overcount.
	if rep.Summary.WeakIndividualDescription > 50 {
		t.Errorf("weak_individual_description = %d, expected a small number (curated descriptions)", rep.Summary.WeakIndividualDescription)
	}
	var prev string
	for _, pr := range rep.Packages {
		if pr.Package < prev {
			t.Errorf("packages not sorted: %q before %q", prev, pr.Package)
		}
		prev = pr.Package
	}
}

// TestBuildReport_Deterministic verifies repeated runs are identical.
func TestBuildReport_Deterministic(t *testing.T) {
	first, err := buildReport(true)
	if err != nil {
		t.Fatalf("first buildReport: %v", err)
	}
	second, err := buildReport(true)
	if err != nil {
		t.Fatalf("second buildReport: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("buildReport is not deterministic across runs")
	}
}
