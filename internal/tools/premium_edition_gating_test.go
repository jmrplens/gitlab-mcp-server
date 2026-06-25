package tools

import (
	"sort"
	"strings"
	"testing"
)

// premiumGatingExempt lists individual tools whose model-facing text mentions
// "Premium/Ultimate" but which are genuine CE (Free-tier) features. For these
// the premium wording describes an OPTIONAL parameter, not the action itself,
// so the action must NOT be Edition-gated (gating would hide a CE feature from
// CE clients). Each entry carries a one-line rationale.
var premiumGatingExempt = map[string]string{
	// Adding a group member is CE; only the optional member_role_id custom role
	// is Premium/Ultimate. The tool itself works on Free.
	"gitlab_group_member_add": "CE feature; only the optional member_role_id custom role is Premium/Ultimate.",
	// Adding a project member is CE; only the optional member_role_id custom role
	// is Premium/Ultimate. The tool itself works on Free.
	"gitlab_project_member_add": "CE feature; only the optional member_role_id custom role is Premium/Ultimate.",
	// Feature flags are available on Free, Premium, and Ultimate per current
	// GitLab docs (docs.gitlab.com/operations/feature_flags). The
	// "Requires Premium/Ultimate" wording in the spec is legacy/over-conservative.
	"gitlab_feature_flag_list":   "Feature flags are a Free-tier feature; the Premium/Ultimate wording is legacy and inaccurate.",
	"gitlab_feature_flag_create": "Feature flags are a Free-tier feature; the Premium/Ultimate wording is legacy and inaccurate.",
	"gitlab_feature_flag_update": "Feature flags are a Free-tier feature; the Premium/Ultimate wording is legacy and inaccurate.",
	"gitlab_feature_flag_delete": "Feature flags are a Free-tier feature; the Premium/Ultimate wording is legacy and inaccurate.",
	// Push (remote) mirror creation is Free per doc/api/remote_mirrors.md (page
	// tier = Free, Premium, Ultimate); only pull mirroring is Premium. The
	// "Requires Premium/Ultimate" usage wording is legacy and inaccurate.
	"gitlab_add_project_mirror": "Push mirroring is Free per remote_mirrors.md; the Premium/Ultimate wording is legacy. Pull mirroring stays Premium.",
	// Group integration management is Free per doc/api/group_integrations.md
	// (page tier = Free, Premium, Ultimate); only specific integrations need a
	// paid tier. The wording references that sub-feature nuance, not the API tier.
	"gitlab_list_group_integrations":          "Group integration management API is Free per group_integrations.md; only some integrations need a paid tier.",
	"gitlab_set_group_integration":            "Group integration management API is Free per group_integrations.md; only some integrations need a paid tier.",
	"gitlab_get_group_datadog_integration":    "Group integration management API is Free per group_integrations.md; the Premium/Ultimate wording is over-stated.",
	"gitlab_set_group_datadog_integration":    "Group integration management API is Free per group_integrations.md; the Premium/Ultimate wording is over-stated.",
	"gitlab_delete_group_datadog_integration": "Group integration management API is Free per group_integrations.md; the Premium/Ultimate wording is over-stated.",
}

// TestPremiumDescribedActionsAreEditionGated guards the premium-tool gating
// contract: any action whose model-facing text advertises a GitLab
// Premium/Ultimate requirement MUST also set ActionSpec.Edition, because the
// individual catalog hides an action from non-enterprise (CE) clients only when
// Edition != "" (see individualCatalogActionEligible). A premium action with an
// empty Edition leaks into the CE surface, where it would only 403 at runtime.
//
// Actions in premiumGatingExempt are skipped: they are CE features whose
// Premium/Ultimate text describes only an optional parameter.
func TestPremiumDescribedActionsAreEditionGated(t *testing.T) {
	var leaks []string
	for _, group := range CollectActionSpecs(nil, true) {
		for _, spec := range group.Actions {
			if spec.Edition != "" {
				continue
			}
			text := strings.ToLower(spec.Usage + " " + spec.IndividualTool.Description)
			if strings.Contains(text, "premium/ultimate") ||
				strings.Contains(text, "premium or ultimate") ||
				strings.Contains(text, "ultimate only") {
				name := spec.IndividualTool.Name
				if name == "" {
					name = group.ToolName + "/" + spec.Name
				}
				if _, ok := premiumGatingExempt[name]; ok {
					continue
				}
				leaks = append(leaks, name)
			}
		}
	}
	if len(leaks) > 0 {
		sort.Strings(leaks)
		t.Fatalf("%d premium-described action(s) have empty Edition and leak into the CE surface; gate them via the package's premium spec helper (e.g. groupPremiumSpec/projectPremiumSpec) or set opts.Edition:\n  %s",
			len(leaks), strings.Join(leaks, "\n  "))
	}
}
