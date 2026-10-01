package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// gateFindings holds the derivation to the three rules a person cannot be
// trusted to keep by reading a diff:
//
//  1. a way of running an action that sends more than one request says why
//     each extra one runs (a directive or a declaration), since the default
//     reading of syntax is what makes two lookups look like a conjunction;
//  2. a denial names something the record holds, so no action is withheld on
//     a name GitLab never declared;
//  3. an action that sends something sends something on every way of running
//     it, or a directive says it may not, since a loop the default reads as
//     optional is otherwise a silent wrong "yes".
func gateFindings(derived []derive.Action, joined []join.Action, record *apilive.Document) []string {
	var findings []string
	for i := range derived {
		act := &derived[i]
		if act.Declaration != "" {
			continue
		}
		findings = append(findings, unqualified(act)...)
		if finding := pathWithoutRequest(act); finding != "" {
			findings = append(findings, finding)
		}
	}
	for i := range joined {
		row := joined[i].Row
		if row != nil && row.Denied != nil && !join.Known(record, row.Denied) {
			findings = append(findings, fmt.Sprintf("gate 2: %s is denied by %s, which the live record does not hold as a %s",
				row.ID, row.Denied.Element, row.Denied.Cause))
		}
	}
	slices.Sort(findings)
	return slices.Compact(findings)
}

// unqualified reports each way of running an action that sends more than one
// request nothing qualifies.
func unqualified(act *derive.Action) []string {
	var findings []string
	for _, path := range act.Paths {
		var bare []string
		for _, use := range path {
			if !act.Uses[use].Qualified {
				bare = append(bare, describeUse(&act.Uses[use]))
			}
		}
		if len(bare) > 1 {
			findings = append(findings, fmt.Sprintf("gate 1: %s sends %s on one path and no directive or declaration says why each runs",
				act.ID, strings.Join(bare, " and ")))
		}
	}
	return findings
}

// describeUse names a request with the functions that send it, which is where
// a directive goes.
func describeUse(use *derive.Use) string {
	return use.Key() + " (from " + strings.Join(use.Sites, ", ") + ")"
}

// pathWithoutRequest reports an action that sends a request and has a way of
// running that sends none, unless every request it sends was declared
// optional.
func pathWithoutRequest(act *derive.Action) string {
	if len(act.Uses) == 0 || !slices.ContainsFunc(act.Paths, func(path []int) bool { return len(path) == 0 }) {
		return ""
	}
	for i := range act.Uses {
		if !act.Uses[i].DeclaredOptional {
			return fmt.Sprintf("gate 3: %s can run sending none of its requests (%s is never mandatory); "+
				"mark the request mandatory, or optional with the reason it may not run", act.ID, describeUse(&act.Uses[i]))
		}
	}
	return ""
}
