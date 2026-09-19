package mrapprovalsettings

// Canonical action IDs this package publishes, the one form every surface
// resolves.
//
// The hints a Markdown formatter writes and the relatedActions the metadata
// table carries read this one block rather than each keeping its own. Kept
// apart, the two drifted: the formatters named the catalog IDs and the table
// named the bare action halves, with no domain in front of them, so the
// cross-link a model was invited to follow answered "unknown action".
const (
	actionGroupGet   = "merge_request.approval_settings_group_get"
	actionProjectGet = "merge_request.approval_settings_project_get"

	// Both update actions are named when a formatter does not know which scope
	// it is rendering, because the Markdown registry dispatches on the Go type
	// and the group and project handlers answer with the same one.
	actionGroupUpdate   = "merge_request.approval_settings_group_update"
	actionProjectUpdate = "merge_request.approval_settings_project_update"

	// These settings are the policy; the approval state is the policy applied
	// to one merge request, which is the read worth naming from here. It is a
	// Premium action, as every action in this package is.
	actionMRApprovalState = "merge_request.approval_state"

	actionGroupGetScope   = "group.get"
	actionProjectGetScope = "project.get"
)

// publishedActionIDs returns every canonical action ID this package hands a
// model as a cross-link, which is what the catalog test holds against the
// catalog.
func publishedActionIDs() []string {
	return []string{
		actionGroupGet,
		actionGroupUpdate,
		actionProjectGet,
		actionProjectUpdate,
		actionMRApprovalState,
		actionGroupGetScope,
		actionProjectGetScope,
	}
}
