package main

// tierException records the doc-grounded expected tier for an action whose
// owner-domain doc page does not, by itself, imply that tier. Two situations
// produce these:
//
//   - cross-page endpoints: the action is owned by one package (mapped to one
//     doc page) but its REST endpoint is documented on a different page with a
//     different tier (e.g. project push rules live in the Free-badged
//     projects.md owner domain but are documented on the Premium push-rules
//     page);
//   - per-section overrides: the doc page default differs from a specific
//     endpoint's tier (e.g. merge_request_approvals.md is page-Premium but the
//     basic approval-state endpoints are Free);
//   - live-verified corrections: the doc text misstates the tier and a check
//     against a real instance settled it (e.g. merge_request.approval_state).
//
// Each entry is the audited, intended tier. The auditor treats a matching
// action as correct; a divergence from the entry would surface as a mismatch.
type tierException struct {
	tier   tier
	reason string
}

// acceptedTierExceptions maps canonical action IDs to their audited tier and a
// rationale citing the real doc page/section.
const (
	docMRApprovalsPremium   = "merge_request_approvals.md = Premium"
	docProjPushRulesPremium = "project_push_rules.md = Premium"
	docGrpPushRulesPremium  = "group_push_rules.md = Premium"
	docTargetBranchPremium  = "target branch rules = Premium"
	docPullMirrorPremium    = "project_pull_mirroring.md = Premium"
	docBillableMembersPrem  = "billable members = Premium"

	docMRApprovalRulesPremium = "merge_request_approvals.md per-MR approval-rule sections = Premium override"
)

var acceptedTierExceptions = map[string]tierException{
	// merge_request_approvals.md is page-Free with Premium section overrides:
	// the basic per-MR approval STATE endpoints are Free; approval RULES are
	// Premium (each rule section carries its own Premium badge).
	"merge_request.approval_config": {tierFree, "basic approval state (GET /approvals) is Free; only approval rules are Premium per merge_request_approvals.md"},
	"merge_request.approval_reset":  {tierFree, "reset approvals is a basic approval-state mutation, Free per merge_request_approvals.md"},

	// Per-MR approval rules — merge_request_approvals.md 'Approval rules for a
	// merge request' subsections each carry a Premium override badge.
	"merge_request.approval_rules":       {tierPremium, docMRApprovalRulesPremium},
	"merge_request.approval_rule_create": {tierPremium, docMRApprovalRulesPremium},
	"merge_request.approval_rule_update": {tierPremium, docMRApprovalRulesPremium},
	"merge_request.approval_rule_delete": {tierPremium, docMRApprovalRulesPremium},

	// Per-MR approval state — merge_request_approvals.md lists 'Retrieve
	// approval state for a merge request' among its Free endpoints, but live
	// behavior contradicts that text: GitLab 19.0.1 CE returns 404 for
	// GET /projects/:id/merge_requests/:iid/approval_state while the sibling
	// /approvals endpoint returns 200 (verified 2026-07-07). Live behavior
	// wins over the misleading doc listing, so the Premium label stands.
	"merge_request.approval_state": {tierPremium, "live-verified Premium: 404 on GitLab 19.0.1 CE (verified 2026-07-07) despite merge_request_approvals.md listing approval_state as Free"},

	// Project push rules — owned by the projects package (projects.md = Free) but
	// documented on the Premium project push-rules page.
	"project.push_rule_get":    {tierPremium, docProjPushRulesPremium},
	"project.push_rule_add":    {tierPremium, docProjPushRulesPremium},
	"project.push_rule_edit":   {tierPremium, docProjPushRulesPremium},
	"project.push_rule_delete": {tierPremium, docProjPushRulesPremium},

	// Project-level approval configuration/rules — documented on the Premium
	// merge_request_approvals page, not on projects.md.
	"project.approval_config_get":    {tierPremium, docMRApprovalsPremium},
	"project.approval_config_change": {tierPremium, docMRApprovalsPremium},
	"project.approval_rule_list":     {tierPremium, docMRApprovalsPremium},
	"project.approval_rule_get":      {tierPremium, docMRApprovalsPremium},
	"project.approval_rule_create":   {tierPremium, docMRApprovalsPremium},
	"project.approval_rule_update":   {tierPremium, docMRApprovalsPremium},
	"project.approval_rule_delete":   {tierPremium, docMRApprovalsPremium},

	// Pull mirroring — documented on the Premium project_pull_mirroring page,
	// distinct from the Free remote_mirrors (push) page.
	"project.pull_mirror_get":       {tierPremium, docPullMirrorPremium},
	"project.pull_mirror_configure": {tierPremium, docPullMirrorPremium},
	"project.start_mirroring":       {tierPremium, docPullMirrorPremium},

	// Target branch rules — Premium, documented separately (GraphQL-backed).
	"project.target_branch_rule_list":   {tierPremium, docTargetBranchPremium},
	"project.target_branch_rule_create": {tierPremium, docTargetBranchPremium},
	"project.target_branch_rule_delete": {tierPremium, docTargetBranchPremium},

	// Group push rules — group_push_rules.md = Premium (groups.md is Free).
	"group.push_rule_get":    {tierPremium, docGrpPushRulesPremium},
	"group.push_rule_add":    {tierPremium, docGrpPushRulesPremium},
	"group.push_rule_edit":   {tierPremium, docGrpPushRulesPremium},
	"group.push_rule_delete": {tierPremium, docGrpPushRulesPremium},

	// Group billable members & provisioned users — Premium/Ultimate billing
	// endpoints documented under members/billing, not on groups.md.
	"group.group_billable_members_list":            {tierPremium, docBillableMembersPrem},
	"group.group_billable_member_memberships_list": {tierPremium, docBillableMembersPrem},
	"group.group_billable_member_remove":           {tierPremium, docBillableMembersPrem},
	"group.list_provisioned_users":                 {tierPremium, "provisioned users = Premium"},

	// Group issue board create/delete — group_boards.md page is Free but the
	// create/delete sections are Premium-overridden.
	"group.group_board_create": {tierPremium, "group_boards.md 'Create' section = Premium override"},

	// Group milestone burndown events — group_milestones.md page is Free but the
	// burndown-chart-events section carries a Premium override badge.
	"group.group_milestone_burndown": {tierPremium, "group_milestones.md 'burndown chart events' section = Premium override"},

	// Epic resource label events — owned via resourceevents (resource_label_events.md
	// = Free) but epic-scoped events are Premium (epics are Premium).
	"group.event_epic_label_get":  {tierPremium, "epic label events = Premium (epics)"},
	"group.event_epic_label_list": {tierPremium, "epic label events = Premium (epics)"},

	// Issue iteration & weight resource events — iterations and weight are Premium
	// issue features (issues.md is page-Free).
	"issue.event_issue_iteration_get":  {tierPremium, "issue iterations = Premium"},
	"issue.event_issue_iteration_list": {tierPremium, "issue iterations = Premium"},
	"issue.event_issue_weight_list":    {tierPremium, "issue weight = Premium"},
}
