package finegrained

import (
	"fmt"
	"strings"
)

// The notes a fine-grained session is given beside an answer it was served,
// for what GitLab does to that answer without saying so. Over REST a call
// outside the grant is GitLab's own 403, which names what is missing; over
// GraphQL it is not: a position the grant does not reach comes back null and a
// connection drops the items it does not reach, with no error either way. So
// the answers where that can happen carry a next step that says it, written
// here once for every surface, naming no tool and no action ID.

// DegradedNote is the next step a served answer gets when GitLab leaves parts
// of it empty for this credential, or "" when it leaves none. Each part is
// written as the GraphQL selection that reaches it, with the type GitLab
// checks there and why it comes back empty: a type that declares no
// fine-grained permission is empty for every fine-grained token, and a
// declared one is empty unless the grant holds what the type needs.
func (a *Authority) DegradedNote(decision Decision) string {
	if len(decision.Degraded) == 0 {
		return ""
	}
	parts := make([]string, 0, len(decision.Degraded))
	for _, index := range decision.Degraded {
		element := a.table.Elements[index]
		why := "on which it declares no fine-grained permission"
		if !element.Undeclared {
			needs := make([]string, 0, len(element.Groups))
			for _, group := range element.Groups {
				words, _ := a.table.groupWords(group)
				needs = append(needs, words)
			}
			why = "which needs " + strings.Join(needs, ", and ") + " this token was not granted"
		}
		parts = append(parts, fmt.Sprintf("`%s` (the GraphQL type %s, %s)", Selection(element.Path), element.Type, why))
	}
	return fmt.Sprintf("GitLab %s leaves part of this answer empty for a fine-grained personal access token, with no error: %s. "+
		"Empty there does not mean there is nothing.", a.table.DisplayVersion(), strings.Join(parts, "; "))
}

// heldElsewhereBoundaries are the boundaries GitLab's missing-permissions
// refusal names at which a grant can hold a permission for some projects or
// groups and not for others, each with its index into the covered sets.
var heldElsewhereBoundaries = map[string]int{"project": 0, "group": 1}

// HeldElsewhereNote is the sentence added to GitLab's missing-permissions
// refusal of a call when this session's grant does hold, at the boundary
// type the refusal names, every permission it quotes, or "" when it does not.
//
// GitLab writes the same sentence whether the grant lacks a permission or
// holds it for other projects or groups, and whether the token's user is a
// member of the project or group the call targets, so the description of the
// refusal names every cause. A session whose grant holds the permission can
// rule the first one out, and this says so: what the call is missing is the
// project or group in the grant, or its user's membership there.
//
// boundary is the refusal's boundary word and words its quoted permissions,
// both GitLab's text. A word is matched against the words the table displays
// each raw permission by, and the grant is asked whether it covers at that
// boundary type any raw permission displayed that way, which is the most a
// grant's covered sets can say since they keep no namespace. The words the
// sentence repeats are therefore always the table's own. A phase A authority
// covers nothing, so it never writes the sentence.
func (a *Authority) HeldElsewhereNote(boundary string, words []string) string {
	index, membership := heldElsewhereBoundaries[boundary]
	if !membership || len(words) == 0 {
		return ""
	}
	for _, word := range words {
		if !a.holdsAt(index, word) {
			return ""
		}
	}
	return "This session's token does hold [" + strings.Join(words, ", ") + "], for other " + boundary +
		"s, so what this call is missing is this " + boundary + " in the grant, or a group above it, or its user's membership here."
}

// holdsAt reports whether the grant covers, at the boundary type of index b,
// a raw permission the table displays as words.
func (a *Authority) holdsAt(b int, words string) bool {
	for perm, display := range a.table.Display {
		if a.table.Displays[display] == words && a.covered[b].has(perm) {
			return true
		}
	}
	return false
}

// NullNote is the next step a not-found answer gets when the action reads
// GitLab over GraphQL, or "" when it does not. GitLab answers null, with no
// error, for an object a fine-grained token is not granted or that sits in a
// project or group outside its grant, so a reader told "not found" has to be
// told that it may be the token rather than the object.
func (a *Authority) NullNote(id string) string {
	row := a.table.Requirement(id)
	if row == nil || !row.GraphQL {
		return ""
	}
	return "Over GraphQL, GitLab answers null with no error for an object a fine-grained personal access token is not granted, " +
		"or that sits in a project or group outside its grant, so not found may mean this token cannot see it rather than " +
		"that it does not exist."
}

// EmptyNote is the next step an empty list gets when the action's answer is a
// GraphQL list or connection, or "" when it is not. GitLab leaves out of such
// a list the items a fine-grained token is not granted, with no error, so an
// empty answer may be the token rather than an absence.
func (a *Authority) EmptyNote(id string) string {
	row := a.table.Requirement(id)
	if row == nil || !row.Collection {
		return ""
	}
	return "Over GraphQL, GitLab leaves out of a list, with no error, the items a fine-grained personal access token is not " +
		"granted, so an empty answer may mean this token cannot see them rather than that there are none."
}
