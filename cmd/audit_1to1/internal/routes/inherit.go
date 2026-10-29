// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import (
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

// The categories a decision about a client-go method takes, as the tables
// that hold them spell them, and the three this scope adds for a method no
// table decides.
const (
	adjudicationGraphQL    = "COVERED_GRAPHQL"
	adjudicationSuperseded = "SUPERSEDED_UPSTREAM"
	adjudicationUnwrapped  = "UNWRAPPED_TRACKED"
	adjudicationVariant    = "COVERED_VARIANT"
	adjudicationRaw        = "COVERED_RAW"
	// adjudicationURLBuilt is a waits entry whose handler builds the URL for
	// the caller to fetch and sends nothing, which holds for a route no
	// request sends.
	adjudicationURLBuilt = waits.CategoryURLBuilt
	// adjudicationCalled is a method an action calls, whose other routes it
	// sends and this one not.
	adjudicationCalled = "called"
	// adjudicationNone is a method nothing decides, a finding of R-ACTION.
	adjudicationNone = "unadjudicated"
)

// carryingAdjudications are the decisions whose claim holds for a route no
// request sends: the operation goes over GraphQL, GitLab replaced the API,
// the service is tracked as not wrapped, or the handler builds the URL. A
// COVERED_GRAPHQL decision carries only with the GraphQL actions
// graphqlActions names for the method.
var carryingAdjudications = map[string]bool{
	adjudicationGraphQL:    true,
	adjudicationSuperseded: true,
	adjudicationUnwrapped:  true,
	adjudicationURLBuilt:   true,
}

// contradictingAdjudications are the decisions that say the route is sent,
// which a route no action reaches makes false.
var contradictingAdjudications = map[string]bool{
	adjudicationVariant: true,
	adjudicationRaw:     true,
}

// settlingAdjudications are the decisions of acceptedMissingMethods that say
// a route needs no action of its own: another route covers the capability,
// or the route is left out on purpose. Neither carries onto a route no
// action reaches, and a tracked row over such a route says the opposite, an
// action still to implement, so the two are a problem of the tables.
var settlingAdjudications = map[string]bool{
	"COVERED_GENERIC":         true,
	"INTENTIONAL_SKIP_BINARY": true,
	"INTENTIONAL_SKIP_OTHER":  true,
}

// graphqlActions names, for each client-go method decided COVERED_GRAPHQL
// whose route no action reaches, the actions of the catalog that do the same
// operation over GraphQL, keyed "Service.Method". The decision says the
// domain goes over GraphQL; what carries it onto a route is an action that
// does the operation the route does, so a method this table does not name
// leaves its route to the family rows, a finding until one answers it. A
// row whose method no such route is sent by answers nothing, and every
// action it names is in the catalog and sends GraphQL in the request record.
var graphqlActions = map[string][]string{
	"Epics.CreateEpic":                                  {"group.epic_create"},
	"Epics.DeleteEpic":                                  {"group.epic_delete"},
	"Epics.GetEpic":                                     {"group.epic_get"},
	"Epics.UpdateEpic":                                  {"group.epic_update"},
	"EpicIssues.AssignEpicIssue":                        {"group.epic_issue_assign"},
	"EpicIssues.ListEpicIssues":                         {"group.epic_issue_list"},
	"EpicIssues.RemoveEpicIssue":                        {"group.epic_issue_remove"},
	"EpicIssues.UpdateEpicIssueAssignment":              {"group.epic_issue_update"},
	"Notes.CreateEpicNote":                              {"group.epic_note_create"},
	"Notes.DeleteEpicNote":                              {"group.epic_note_delete"},
	"Notes.GetEpicNote":                                 {"group.epic_note_get"},
	"Notes.ListEpicNotes":                               {"group.epic_note_list"},
	"Notes.UpdateEpicNote":                              {"group.epic_note_update"},
	"Discussions.AddEpicDiscussionNote":                 {"group.epic_discussion_add_note"},
	"Discussions.CreateEpicDiscussion":                  {"group.epic_discussion_create"},
	"Discussions.DeleteEpicDiscussionNote":              {"group.epic_discussion_delete_note"},
	"Discussions.GetEpicDiscussion":                     {"group.epic_discussion_get"},
	"Discussions.ListGroupEpicDiscussions":              {"group.epic_discussion_list"},
	"Discussions.UpdateEpicDiscussionNote":              {"group.epic_discussion_update_note"},
	"ProjectVulnerabilities.ListProjectVulnerabilities": {"vulnerability.list"},
}

// Adjudication is the decision about one client-go method that sends a
// route.
type Adjudication struct {
	// Method is "Service.Method".
	Method string `json:"method"`
	// Category is the decision's, or called or unadjudicated.
	Category string `json:"category"`
	Reason   string `json:"reason,omitempty"`
	// Actions are the GraphQL actions a COVERED_GRAPHQL decision stands
	// for, from graphqlActions.
	Actions []string `json:"actions,omitempty"`
	// Carries says whether the decision holds for a route no request sends.
	Carries bool `json:"carries"`
}

// inherits decides a route by the decisions about the client-go methods that
// send it: inherited when every one carries, contradicted when one says the
// route is sent. Either way the decisions are kept on the verdict. It reports
// whether the route is decided.
func (j *judge) inherits(verdict *Route) bool {
	if len(verdict.SDKMethods) == 0 {
		return false
	}
	carries, contradicted := true, false
	for _, method := range verdict.SDKMethods {
		adjudication := j.adjudicate(method)
		verdict.Adjudications = append(verdict.Adjudications, adjudication)
		carries = carries && adjudication.Carries
		contradicted = contradicted || contradictingAdjudications[adjudication.Category]
	}
	if contradicted {
		verdict.Class = ClassContradicted
		return true
	}
	if carries {
		verdict.Class = ClassInherited
		return true
	}
	return false
}

// adjudicate finds the decision about one method: a method an action calls
// first, then acceptedMissingMethods, the waits entry that replaces it, the
// declaration of its service and the waits entry that replaces the service.
func (j *judge) adjudicate(method string) Adjudication {
	if j.called[method] {
		return Adjudication{Method: method, Category: adjudicationCalled}
	}
	if accepted, held := j.in.accepted[method]; held {
		category, reason, _ := strings.Cut(accepted, ". ")
		return j.graphqlCover(decided(method, category, reason))
	}
	if entry, replaced := j.raw[method]; replaced {
		return rawAdjudication(method, entry)
	}
	service, _, _ := strings.Cut(method, ".")
	if declared, held := j.in.services[service]; held {
		return j.graphqlCover(decided(method, declared.Category, declared.Reason))
	}
	if entry, replaced := j.raw[service]; replaced {
		return rawAdjudication(method, entry)
	}
	return Adjudication{Method: method, Category: adjudicationNone}
}

// graphqlCover holds a COVERED_GRAPHQL decision to the GraphQL actions
// graphqlActions names for its method: it carries only when it names one and
// each is in the catalog and sends GraphQL. A decision of another category
// is returned as it is.
func (j *judge) graphqlCover(adjudication Adjudication) Adjudication {
	if adjudication.Category != adjudicationGraphQL {
		return adjudication
	}
	j.graphqlAsked[adjudication.Method] = true
	adjudication.Actions = slices.Clone(j.in.graphql[adjudication.Method])
	adjudication.Carries = len(adjudication.Actions) > 0
	for _, action := range adjudication.Actions {
		if len(j.actionNeeds(action)) > 0 {
			adjudication.Carries = false
		}
	}
	return adjudication
}

// graphqlProblems lists what is wrong with graphqlActions: a row that names
// no action, an action not in the catalog or sending no GraphQL, and a row
// no decision was read with.
func (j *judge) graphqlProblems() []string {
	var out []string
	for method, actions := range j.in.graphql {
		label := "graphql actions of " + method
		if len(actions) == 0 {
			out = append(out, label+" name none")
		}
		for _, action := range actions {
			for _, need := range j.actionNeeds(action) {
				out = append(out, label+": "+need)
			}
		}
		if !j.graphqlAsked[method] {
			out = append(out, label+" answer no route: no route no action reaches is sent by it under a COVERED_GRAPHQL decision")
		}
	}
	return out
}

// rawAdjudication is the decision a waits entry makes about a method it
// replaces: COVERED_RAW, or the URL the handler builds.
func rawAdjudication(method string, entry waits.Entry) Adjudication {
	category := adjudicationRaw
	if entry.Category == waits.CategoryURLBuilt {
		category = adjudicationURLBuilt
	}
	return decided(method, category, entry.Label()+": "+entry.Reason)
}

// decided builds a decision, carrying when its category does.
func decided(method, category, reason string) Adjudication {
	return Adjudication{Method: method, Category: category, Reason: reason, Carries: carryingAdjudications[category]}
}
