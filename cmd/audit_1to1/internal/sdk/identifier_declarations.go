// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import "strconv"

// The reasons an input field may hand client-go an integer where the live
// API record says the placeholder takes a name or a path, or says nothing.
const (
	// identifierAcceptsIDOnly: GitLab takes an ID alone there, whatever the
	// record's description says, which the declaration's evidence shows
	// from GitLab's code.
	identifierAcceptsIDOnly = "gitlab-accepts-id-only"
	// identifierTracked: the input is to be widened to an ID or a name
	// (toolutil.StringOrInt, additive in the schema), which the issue the
	// declaration names tracks.
	identifierTracked = "tracked"
)

// identifierDeclaration answers an input field whose integer a handler
// hands a client-go parameter that takes a name or a path too.
type identifierDeclaration struct {
	// Input is the field, spelled package.Type.Field.
	Input    string
	Category string
	// Issue is the issue a tracked declaration names, and Evidence what of
	// GitLab's code a declaration that GitLab takes an ID alone cites.
	Issue    int
	Evidence string
	Reason   string
}

// userIdentifiersIssue is the issue that widens the three user inputs
// below to an ID or a username.
const userIdentifiersIssue = 1602

// identifierDeclarations answers the input fields that narrow a placeholder
// GitLab takes a name or a path in.
var identifierDeclarations = []identifierDeclaration{
	{
		Input: "users.ListContributionEventsInput.UserID", Category: identifierTracked, Issue: userIdentifiersIssue,
		Reason: "GET /users/:id/events takes the ID or the username of the user (gitlab-api-live.json, 19.5.0-ee; doc/api/events.md) and ListUserContributionEvents takes any, while user.contribution_events offers user_id as an integer alone",
	},
	{
		Input: "users.GetStatusInput.UserID", Category: identifierTracked, Issue: userIdentifiersIssue,
		Reason: "GET /users/:user_id/status takes the ID or the username of the user (gitlab-api-live.json, 19.5.0-ee; doc/api/users.md) and GetUserStatus takes any, while user.get_status offers user_id as an integer alone",
	},
	{
		Input: "users.ListSSHKeysForUserInput.UserID", Category: identifierTracked, Issue: userIdentifiersIssue,
		Reason: "GET /users/:user_id/keys takes the ID or the username of the user (gitlab-api-live.json, 19.5.0-ee; doc/api/user_keys.md) and ListSSHKeysForUser takes any, while user.ssh_keys_for_user offers user_id as an integer alone",
	},
}

// identifierNeeds says what a declaration lacks, empty when it lacks
// nothing: a reason, a category the table defines, and what the category
// requires.
func identifierNeeds(declaration identifierDeclaration) string {
	if declaration.Category != identifierTracked && declaration.Category != identifierAcceptsIDOnly {
		return "category " + strconv.Quote(declaration.Category) + " is not one the table defines"
	}
	if declaration.Reason == "" {
		return "it gives no reason"
	}
	if declaration.Category == identifierTracked && declaration.Issue <= 0 {
		return "a tracked declaration needs the issue that tracks it"
	}
	if declaration.Category == identifierAcceptsIDOnly && declaration.Evidence == "" {
		return "a gitlab-accepts-id-only declaration needs the evidence of GitLab's code"
	}
	return ""
}

// applyIdentifierDeclarations marks each argument a declaration answers as
// declared, with the declaration's category, issue and reason, and returns
// the stale declarations: one declared twice, one that lacks what it needs,
// and one that answers no argument that would otherwise fail the gate. An
// argument the record says takes an ID alone needs no declaration, so one
// answering it answers nothing.
func applyIdentifierDeclarations(arguments []identifierArgument, declared []identifierDeclaration) []string {
	var stale []string
	byInput := map[string]identifierDeclaration{}
	var order []string
	for _, declaration := range declared {
		label := "identifier declaration " + declaration.Input
		if _, twice := byInput[declaration.Input]; twice {
			stale = append(stale, label+" is declared twice")
			continue
		}
		byInput[declaration.Input] = declaration
		order = append(order, declaration.Input)
		if lacks := identifierNeeds(declaration); lacks != "" {
			stale = append(stale, label+": "+lacks)
		}
	}
	answered := map[string]bool{}
	for i := range arguments {
		argument := &arguments[i]
		declaration, isDeclared := byInput[argument.Input]
		if !isDeclared || (argument.Status != identifierNarrowed && argument.Status != identifierUndescribed) {
			continue
		}
		argument.Status, argument.Category, argument.Issue, argument.Reason = identifierDeclared, declaration.Category, declaration.Issue, declaration.Reason
		answered[argument.Input] = true
	}
	for _, input := range order {
		if !answered[input] {
			stale = append(stale, "identifier declaration "+input+" answers no integer handed where the record does not say an ID alone is taken")
		}
	}
	return stale
}
