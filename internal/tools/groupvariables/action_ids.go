package groupvariables

// Canonical action IDs this package publishes, the one form every surface
// resolves.
//
// The domain is ci_variable and not group_variable: the group-scoped variable
// actions are aggregated into the CI variable group beside the project and
// instance ones, so the package name is not the domain. Spelling it
// "group_variable.group_get" reads as a perfectly plausible pair and resolves
// to nothing, which answers "unknown action" the moment a model follows it.
const (
	actionGroupVariableList   = "ci_variable.group_list"
	actionGroupVariableGet    = "ci_variable.group_get"
	actionGroupVariableUpdate = "ci_variable.group_update"
	actionGroupVariableDelete = "ci_variable.group_delete"
)

// publishedActionIDs returns every canonical action ID this package hands a
// model as a cross-link, which is what the catalog test holds against the
// catalog.
func publishedActionIDs() []string {
	return []string{
		actionGroupVariableList,
		actionGroupVariableGet,
		actionGroupVariableUpdate,
		actionGroupVariableDelete,
	}
}
