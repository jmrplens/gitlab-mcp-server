package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// gateFindings holds the derivation to the three rules a person cannot be
// trusted to keep by reading a diff:
//
//  1. a way of running an action that sends more than one request says why
//     each extra one runs (a directive or a declaration), since the default
//     reading of syntax is what makes two lookups look like a conjunction;
//  2. a denial, of an action or of one way of running it, names something the
//     record holds, so no action and no input is withheld on a name GitLab
//     never declared;
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
		findings = append(findings, unheldDenials(joined[i].Row, record)...)
	}
	slices.Sort(findings)
	return slices.Compact(findings)
}

// classicFindings holds the classic scope of every joined action to the two
// rules a diff of the table would not show:
//
//  4. every way of running an action needs the same classic scope, and no
//     optional request needs more than the action, since a read_api token is
//     served or withheld an action whole while GitLab judges each request;
//  5. an action the catalog classifies as a read is one a read_api token
//     reaches, and a write one it does not, since the classification decides
//     read-only mode and safe mode while the reach decides what a read_api
//     token is served.
//
// An action a declaration answers passes its gate, and a declaration that
// answers nothing is a finding.
func classicFindings(joined []join.Action, catalog []actionrequests.Action, variations, disagreements []classicDeclaration) []string {
	readOnly := make(map[string]bool, len(catalog))
	for _, action := range catalog {
		readOnly[action.ID] = action.ReadOnly
	}
	varied, departed := declaredActions(variations), declaredActions(disagreements)
	var findings []string
	for i := range joined {
		act := &joined[i]
		if act.Row == nil {
			continue
		}
		if variation := classicVariation(act); variation != "" && !settled(varied, act.ID) {
			findings = append(findings, "gate 4: "+variation)
		}
		if reach := act.Row.Classic.ReachableWith(finegrained.ClassicReadAPI); reach != readOnly[act.ID] && !settled(departed, act.ID) {
			findings = append(findings, fmt.Sprintf(
				"gate 5: %s is classified read-only=%t and a read_api token reaching it is %t; declare why the two depart",
				act.ID, readOnly[act.ID], reach,
			))
		}
	}
	findings = append(findings, unsettled("gate 4", varied)...)
	findings = append(findings, unsettled("gate 5", departed)...)
	slices.Sort(findings)
	return findings
}

// classicVariation says how one action's classic scope varies, "" when it
// does not: two ways of running it needing different scopes, or an optional
// request needing more than the action.
func classicVariation(act *join.Action) string {
	var ways []string
	for _, path := range act.Paths {
		way := join.WayClassic(act.Requests, path).String()
		if !slices.Contains(ways, way) {
			ways = append(ways, way)
		}
	}
	if len(ways) > 1 {
		return fmt.Sprintf("%s runs ways needing %s; a read_api token would be served or withheld it whole", act.ID, strings.Join(ways, " and "))
	}
	for i := range act.Requests {
		request := &act.Requests[i]
		if request.Class == derive.ClassOptional && request.Classic > act.Row.Classic {
			return fmt.Sprintf("%s may send %s, which needs %s, more than the %s the action needs", act.ID, request.Key(), request.Classic, act.Row.Classic)
		}
	}
	return ""
}

// declaredActions indexes declarations by action, each marked unused until a
// finding it answers is met.
func declaredActions(declarations []classicDeclaration) map[string]bool {
	used := make(map[string]bool, len(declarations))
	for _, declaration := range declarations {
		used[declaration.Action] = false
	}
	return used
}

// settled reports whether a declaration of the action answers its finding,
// marking the declaration used when one does.
func settled(declared map[string]bool, id string) bool {
	_, ok := declared[id]
	if ok {
		declared[id] = true
	}
	return ok
}

// unsettled reports each declaration of a gate that answered no finding.
func unsettled(gate string, declared map[string]bool) []string {
	var findings []string
	for id, used := range declared {
		if !used {
			findings = append(findings, fmt.Sprintf("%s: the declaration of %s answers nothing; remove it", gate, id))
		}
	}
	return findings
}

// unheldDenials reports each denial of a row, of the action or of one way of
// running it, that names an element the record does not hold as the kind its
// cause says. R-GRANT asks the same question of the committed table through
// the same function, so the two cannot disagree on any denial either reads.
func unheldDenials(row *finegrained.Requirement, record *apilive.Document) []string {
	if row == nil {
		return nil
	}
	var findings []string
	if row.Denied != nil && !record.HoldsDenial(row.Denied) {
		findings = append(findings, fmt.Sprintf("gate 2: %s is denied by %s, which the live record does not hold as a %s",
			row.ID, row.Denied.Element, row.Denied.Cause))
	}
	for j := range row.DeniedWays {
		way := &row.DeniedWays[j]
		if !record.HoldsDenial(way) {
			findings = append(findings, fmt.Sprintf("gate 2: a way of running %s is denied by %s, which the live record does not hold as a %s",
				row.ID, way.Element, way.Cause))
		}
	}
	return findings
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
	// An action sending nothing has no request to name, which the loop below
	// answers on its own.
	if !slices.ContainsFunc(act.Paths, func(path []int) bool { return len(path) == 0 }) {
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
