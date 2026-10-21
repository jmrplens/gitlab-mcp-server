package main

import "github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"

// categoryOutlivesTheCall is the one reason a call may be excused: a request
// that is meant to finish whatever becomes of the call that started it, and is
// bounded by something other than the caller's context.
const categoryOutlivesTheCall = "outlives-the-call"

// categories are the reasons a declaration may give, each with what it means.
// A declaration naming anything else is reported rather than trusted, since a
// category nobody defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryOutlivesTheCall: "the request must finish even if the call that started it is abandoned, and something other than the caller's context bounds it",
}

// withoutContextOnPurpose are the functions this repository lets reach
// client-go without the caller's context, each with the reason that is right:
// why the function passes no context, in a category and in words.
//
// The table is meant to stay empty. A request built without the context is
// one the action deadline cannot end and an abandoned HTTP POST cannot stop,
// and every call that has done it so far was an omission rather than a
// choice. An entry is for the day one is a choice, and it names the function
// rather than the line so it survives an edit above it.
//
// The key is the package the repository names, a colon, and the function as
// its file spells it (`internal/tools/x:Func`, `internal/tools/x:Type.Method`),
// which is also how a finding names where it sits. An entry that excuses
// nothing is reported, on the terms every declaration table here is held to:
// a declaration that has stopped describing the tree is itself a finding.
var withoutContextOnPurpose = map[string]sourcegate.Declaration{}

// declarationKey is how a finding is named in the declaration table.
func declarationKey(finding Finding) string {
	return finding.Package + ":" + finding.Func
}
