package main

import "github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"

// categoryOnlyAsText is the one reason a function may close a server and go
// on reading its address: afterwards the address is only text, compared or
// printed, and nothing sends a request to it.
const categoryOnlyAsText = "only-as-text"

// categories are the reasons a declaration may give, each with what it means.
// A declaration naming anything else is reported rather than trusted, since a
// category nobody defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryOnlyAsText: "after the close the address is only read as text and nothing dials it, so no listener given the port can answer in its place",
}

// usedAfterTheCloseOnPurpose are the functions this repository lets close an
// httptest server and go on using its address, each with the reason that is
// right: why the function reads a closed server's address, in a category and
// in words.
//
// The table is meant to stay empty. A server closed in the middle of a test
// gives its port back at once, and a test that needs an address nothing
// answers on has netfixture.DeadEndpoint, which keeps the port, and one that
// needs an instance to answer and then go away has
// netfixture.NewStoppableServer. An entry is for the day reading the address
// after the close is a choice rather than an omission, and it names the
// function rather than the line so it survives an edit above it.
//
// The key is the package the repository names, a colon, and the function as
// its file spells it (`internal/x:TestY`, `internal/x:Type.Method`), which is
// also how a finding names where it sits. An entry that excuses nothing is
// reported, on the terms every declaration table here is held to: a
// declaration that has stopped describing the tree is itself a finding.
var usedAfterTheCloseOnPurpose = map[string]sourcegate.Declaration{}

// declarationKey is how a finding is named in the declaration table.
func declarationKey(finding Finding) string {
	return finding.Package + ":" + finding.Func
}
