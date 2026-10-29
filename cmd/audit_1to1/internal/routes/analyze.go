// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/actions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/golist"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
)

// The classes of a route something answers, reached or not.
const (
	// ClassReached is a route an action's request sends.
	ClassReached = "reached"
	// ClassOutsideAPI is a route not mounted under /api/:version.
	ClassOutsideAPI = "outside-api"
	// ClassSkip is a route that skips the fine-grained check for a reason
	// skipReasons excludes.
	ClassSkip = "skip"
	// ClassSlugFromInput is a route an action reaches with a segment the
	// caller names, which a slugRules row declares.
	ClassSlugFromInput = "slug-from-input"
	// ClassDeprecatedAlias is a /services/ route whose /integrations/ twin
	// is reached or answered.
	ClassDeprecatedAlias = "deprecated-alias"
	// ClassInherited is a route whose every client-go method is decided in
	// a way that holds for a route no request sends.
	ClassInherited = "inherited"
	// ClassDeclared is a route a routeFamilies row answers.
	ClassDeclared = "declared"
)

// The classes of a finding.
const (
	// ClassUnanswered is a route nothing reaches or answers.
	ClassUnanswered = "unanswered"
	// ClassUnknownSkipReason is a route that skips the fine-grained check
	// for a reason skipReasons does not hold.
	ClassUnknownSkipReason = "unknown-skip-reason"
	// ClassInternalUnskipped is a route under /internal that carries no skip
	// reason, which every route of the internal API does.
	ClassInternalUnskipped = "internal-without-skip"
	// ClassInternalNotExcluded is a route under /internal whose skip reason
	// excludes nothing (public, a user's token, a reason a reached route
	// carries) and that no exception takes out.
	ClassInternalNotExcluded = "internal-not-excluded"
	// ClassContradicted is a route no action reaches whose client-go method
	// is decided COVERED_VARIANT or COVERED_RAW, which say it is sent.
	ClassContradicted = "contradicted"
)

// findingClasses are the classes of a finding.
var findingClasses = map[string]bool{
	ClassUnanswered:          true,
	ClassUnknownSkipReason:   true,
	ClassInternalUnskipped:   true,
	ClassInternalNotExcluded: true,
	ClassContradicted:        true,
}

// Report is the output of this scope.
type Report struct {
	SchemaVersion int `json:"schema_version"`
	// Requests and Record name the two committed files the join reads.
	Requests string  `json:"requests"`
	Record   string  `json:"record"`
	Summary  Summary `json:"summary"`
	// Findings are the routes no action reaches and nothing answers.
	Findings []Route `json:"findings"`
	// Answered are the routes no action reaches that a rule, an inherited
	// decision or a family row answers, and Reached the routes an action
	// sends, by name. Both are left out of a gaps-only report.
	Answered []Route  `json:"answered,omitempty"`
	Reached  []string `json:"reached,omitempty"`
	// Stale are the declarations that no longer describe the tree, which
	// fail the gate.
	Stale []string `json:"stale_declarations"`
}

// Route is one route of the live record, with what answers it.
type Route struct {
	// Route is the route as the request record names it, a route outside
	// the API prefix with its whole path.
	Route string `json:"route"`
	Class string `json:"class"`
	// Group is the route's first literal segments, two under a scope such
	// as projects/, which the lists are ordered by so a family reads
	// together.
	Group   string `json:"group"`
	Summary string `json:"summary,omitempty"`
	// SDKMethods are the client-go methods that send the route, empty when
	// none does.
	SDKMethods []string `json:"sdk_methods"`
	// SkipReason is the reason the route skips the fine-grained check, when
	// it does.
	SkipReason string `json:"skip_reason,omitempty"`
	// Category, Reason and Issue are what answers the route: a skip row, a
	// slug rule, a family row.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Issue    int    `json:"issue,omitempty"`
	// Family names the family row or the slug rule that answers it.
	Family string `json:"family,omitempty"`
	// Actions are the actions that send it, or that a rule or a family row
	// names for it.
	Actions []string `json:"actions,omitempty"`
	// Twin is the other spelling of an integration route: the /integrations/
	// route a deprecated alias stands for, or, on an /integrations/ route
	// nothing answers, the deprecated /services/ route an action reaches in
	// its place.
	Twin string `json:"twin,omitempty"`
	// Adjudications are the decisions about the client-go methods that send
	// the route, whether or not they carry onto it.
	Adjudications []Adjudication `json:"adjudications,omitempty"`
}

// Summary counts what the parts hold.
type Summary struct {
	// Mounted is every route the record mounts, a route mounted twice
	// counted twice, and Routes the routes once each.
	Mounted int `json:"mounted"`
	Routes  int `json:"routes"`
	// The classes count the routes once each.
	Reached             int `json:"reached"`
	OutsideAPI          int `json:"outside_api"`
	Skipped             int `json:"skipped"`
	SlugFromInput       int `json:"slug_from_input"`
	DeprecatedAlias     int `json:"deprecated_alias"`
	Inherited           int `json:"inherited"`
	Declared            int `json:"declared"`
	Unanswered          int `json:"unanswered"`
	UnknownSkipReason   int `json:"unknown_skip_reason"`
	InternalUnskipped   int `json:"internal_without_skip"`
	InternalNotExcluded int `json:"internal_not_excluded"`
	Contradicted        int `json:"contradicted"`
	// Families is the number of family rows, and Stale the declarations
	// that fail the gate.
	Families int `json:"families"`
	Stale    int `json:"stale_declarations"`
	// SkipReasonsReached are the skip reasons a reached route carries,
	// which exclude none of their routes.
	SkipReasonsReached []string `json:"skip_reasons_reached"`
}

// clean reports whether the gate passes: no route left a finding, of any
// class findingClasses names, and no declaration that no longer describes
// the tree. A route no action reaches is answered by a rule, an inherited
// decision or a family row, so a re-pin that brings one, or an action that
// stops sending one, fails the gate until the change that causes it decides
// the route.
func (s *Summary) clean() bool {
	counters := s.counters()
	for class := range findingClasses {
		if *counters[class] != 0 {
			return false
		}
	}
	return s.Stale == 0
}

// count adds one route of class to the summary.
func (s *Summary) count(class string) {
	*s.counters()[class]++
}

// counters names the counter of each class.
func (s *Summary) counters() map[string]*int {
	return map[string]*int{
		ClassReached:             &s.Reached,
		ClassOutsideAPI:          &s.OutsideAPI,
		ClassSkip:                &s.Skipped,
		ClassSlugFromInput:       &s.SlugFromInput,
		ClassDeprecatedAlias:     &s.DeprecatedAlias,
		ClassInherited:           &s.Inherited,
		ClassDeclared:            &s.Declared,
		ClassUnanswered:          &s.Unanswered,
		ClassUnknownSkipReason:   &s.UnknownSkipReason,
		ClassInternalUnskipped:   &s.InternalUnskipped,
		ClassInternalNotExcluded: &s.InternalNotExcluded,
		ClassContradicted:        &s.Contradicted,
	}
}

// Options is what a run of this scope needs beyond the tree.
type Options struct {
	// GapsOnly drops the answered routes and the reached ones, keeping the
	// findings and the stale declarations.
	GapsOnly bool
}

// Seams for the inputs and for the JSON encoder that never fails on a report
// of strings, ints and bools. Each is a variable a test restores.
var (
	marshalIndent = json.MarshalIndent
	readRecord    = actionrequests.ReadRecord
	readLive      = func(root string) (apilive.Document, error) {
		return apilive.Read(filepath.Join(root, apilive.DefaultDir))
	}
	clientGoDir = func(root string) (string, error) {
		return golist.ModuleDir(context.Background(), root, actionrequests.ClientGoPath)
	}
	readSDK = func(dir string) []sdkroutes.Method {
		return sdkroutes.Read(dir).Methods()
	}
	catalogActions   = requestinventory.Actions
	acceptedMethods  = actions.AcceptedMissingMethods
	declaredServices = sdk.DeclaredServices
	waitEntries      = waits.Entries
)

// Run builds the report for the repository at root and returns it as indented
// JSON with a trailing newline, together with the gate's outcome.
func Run(root string, opts Options) (content []byte, clean bool, err error) {
	report, err := buildReport(root, opts)
	if err != nil {
		return nil, false, err
	}
	content, err = marshalIndent(report, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), report.Summary.clean(), nil
}

// buildReport reads the inputs and classifies every route.
func buildReport(root string, opts Options) (Report, error) {
	record, err := readRecord(root)
	if err != nil {
		return Report{}, err
	}
	live, err := readLive(root)
	if err != nil {
		return Report{}, fmt.Errorf("read the live record: %w", err)
	}
	dir, err := clientGoDir(root)
	if err != nil {
		return Report{}, fmt.Errorf("find client-go: %w", err)
	}
	catalog, err := catalogActions()
	if err != nil {
		return Report{}, fmt.Errorf("build the action catalog: %w", err)
	}
	ids := map[string]bool{}
	for _, action := range catalog {
		ids[action.ID] = true
	}
	report := judgeAll(judgeInput{
		record:     record,
		live:       live,
		methods:    readSDK(dir),
		accepted:   acceptedMethods(),
		services:   declaredServices(),
		waits:      waitEntries(),
		catalog:    ids,
		skips:      skipReasons,
		exceptions: skipExceptions,
		graphql:    graphqlActions,
		slugs:      slugRules,
		families:   routeFamilies,
	})
	report.Requests = actionrequests.RecordPath
	report.Record = filepath.ToSlash(apilive.Path(apilive.DefaultDir))
	if opts.GapsOnly {
		report.Answered = nil
		report.Reached = nil
	}
	return report, nil
}

// judgeInput is everything a classification of the tree reads.
type judgeInput struct {
	record     actionrequests.Record
	live       apilive.Document
	methods    []sdkroutes.Method
	accepted   map[string]string
	services   map[string]sdk.ServiceDeclaration
	waits      []waits.Entry
	catalog    map[string]bool
	skips      map[string]skipReason
	exceptions map[string]skipReason
	// graphql names the GraphQL actions each client-go method decided
	// COVERED_GRAPHQL stands for, which its decision carries with.
	graphql  map[string][]string
	slugs    []slugRule
	families []routeFamily
}

// judge is one classification under way: the indexes the rules read, the
// verdicts reached so far and what each declaration answered.
type judge struct {
	in judgeInput
	// routes holds each route once, by name, the first mounting winning,
	// and order the names in the record's order.
	routes map[string]*apilive.Route
	order  []string
	// reached names the actions sending each route, called the client-go
	// methods a request goes through, graphql the actions that send a
	// GraphQL request, and derived the spellings each action's requests
	// leave a segment of to the input.
	reached map[string][]string
	called  map[string]bool
	graphql map[string]bool
	derived map[string][]string
	// methodsOf names the client-go methods sending each route, raw the
	// waits entry replacing each method or service, carried the skip
	// reasons a route carries, and reachedReasons the ones a reached route
	// carries.
	methodsOf      map[string][]string
	raw            map[string]waits.Entry
	carried        map[string]bool
	reachedReasons map[string]bool
	verdicts       map[string]*Route
	// familyRoutes and slugRoutes are what each row answered, by its index,
	// excepted the routes a skip exception answered, graphqlAsked the
	// methods whose GraphQL actions a decision was read with, and problems
	// what is wrong with the tables as the tree reads them.
	familyRoutes [][]string
	slugRoutes   [][]string
	excepted     map[string]bool
	graphqlAsked map[string]bool
	problems     []string
}

// judgeAll classifies every route of the record and assembles the report.
func judgeAll(in judgeInput) Report {
	j := newJudge(in)
	report := Report{SchemaVersion: shared.SchemaVersion, Findings: []Route{}, Stale: []string{}}
	report.Summary.Mounted = len(in.live.Routes)
	report.Summary.Routes = len(j.order)
	for _, name := range j.order {
		verdict := j.classify(name)
		report.Summary.count(verdict.Class)
		if verdict.Class == ClassReached {
			report.Reached = append(report.Reached, name)
			continue
		}
		if findingClasses[verdict.Class] {
			report.Findings = append(report.Findings, *verdict)
			continue
		}
		report.Answered = append(report.Answered, *verdict)
	}
	byGroup := func(a, b Route) int { return cmp.Or(cmp.Compare(a.Group, b.Group), cmp.Compare(a.Route, b.Route)) }
	slices.SortFunc(report.Findings, byGroup)
	slices.SortFunc(report.Answered, byGroup)
	slices.Sort(report.Reached)
	j.checkTables()
	report.Stale = append(report.Stale, sortedUnique(j.problems)...)
	report.Summary.Families = len(in.families)
	report.Summary.Stale = len(report.Stale)
	report.Summary.SkipReasonsReached = append([]string{}, sortedKeys(j.reachedReasons)...)
	return report
}

// newJudge indexes the inputs.
func newJudge(in judgeInput) *judge {
	j := &judge{
		in:             in,
		routes:         map[string]*apilive.Route{},
		reached:        map[string][]string{},
		called:         map[string]bool{},
		graphql:        map[string]bool{},
		derived:        map[string][]string{},
		methodsOf:      map[string][]string{},
		raw:            map[string]waits.Entry{},
		carried:        map[string]bool{},
		reachedReasons: map[string]bool{},
		verdicts:       map[string]*Route{},
		familyRoutes:   make([][]string, len(in.families)),
		slugRoutes:     make([][]string, len(in.slugs)),
		excepted:       map[string]bool{},
		graphqlAsked:   map[string]bool{},
	}
	for index := range in.live.Routes {
		route := &in.live.Routes[index]
		name := apilive.RouteName(route)
		if _, twice := j.routes[name]; twice {
			continue
		}
		j.routes[name] = route
		j.order = append(j.order, name)
		if reason := skipOf(route); reason != "" {
			j.carried[reason] = true
		}
	}
	j.indexRecord()
	j.indexMethods()
	for _, entry := range in.waits {
		for _, key := range entry.Replaces {
			if _, first := j.raw[key]; !first {
				j.raw[key] = entry
			}
		}
	}
	for name := range j.reached {
		if reason := skipOf(j.routes[name]); reason != "" {
			j.reachedReasons[reason] = true
		}
	}
	return j
}

// indexRecord reads the request record: the routes each action sends, the
// methods its requests go through, the actions that send GraphQL, and the
// spellings a request leaves a segment of to the input. A route the live
// record does not hold reaches nothing here.
func (j *judge) indexRecord() {
	for _, action := range j.in.record.Actions {
		for _, request := range action.Requests {
			for _, method := range request.SDKMethods {
				j.called[method] = true
			}
			if request.Kind == actionrequests.KindGraphQL {
				j.graphql[action.ID] = true
			}
			if request.Kind != actionrequests.KindREST {
				continue
			}
			if request.Derived != "" {
				j.derived[action.ID] = append(j.derived[action.ID], request.Derived)
			}
			if _, held := j.routes[request.Route]; held && !slices.Contains(j.reached[request.Route], action.ID) {
				j.reached[request.Route] = append(j.reached[request.Route], action.ID)
			}
		}
	}
	for name := range j.reached {
		slices.Sort(j.reached[name])
	}
}

// indexMethods finds every route each client-go method sends among the
// record's routes, the way a request is found there.
func (j *judge) indexMethods() {
	index := apilive.NewRouteIndex(j.in.live.Routes)
	for _, method := range j.in.methods {
		for _, sent := range method.Routes {
			route, found := index.Match(sent.Method, sent.Path)
			if !found {
				continue
			}
			name := apilive.RouteName(route)
			if !slices.Contains(j.methodsOf[name], method.Key()) {
				j.methodsOf[name] = append(j.methodsOf[name], method.Key())
			}
		}
	}
	for name := range j.methodsOf {
		slices.Sort(j.methodsOf[name])
	}
}

// classify returns the verdict on one route, reaching it once: the alias
// rule asks for its twin's verdict while it reaches its own.
func (j *judge) classify(name string) *Route {
	if verdict, done := j.verdicts[name]; done {
		return verdict
	}
	verdict := j.verdict(name)
	j.verdicts[name] = verdict
	return verdict
}

// verdict applies the rules to one route in their order of precedence, the
// first that holds deciding.
func (j *judge) verdict(name string) *Route {
	route := j.routes[name]
	verdict := &Route{
		Route: name, Group: groupOf(route.Path), Summary: route.Summary,
		SDKMethods: append([]string{}, j.methodsOf[name]...),
	}
	if sent := j.reached[name]; len(sent) > 0 {
		verdict.Class, verdict.Actions = ClassReached, sent
		return verdict
	}
	path, mounted := strings.CutPrefix(route.Path, apilive.EndpointPrefix)
	if !mounted {
		verdict.Class = ClassOutsideAPI
		return verdict
	}
	// Each rule is an if of its own rather than a case of a switch: a case
	// expression carries no statement counter, so a mutation tool leaves
	// its mutant unrun.
	if j.skipped(route, verdict) {
		return verdict
	}
	if isInternal(path) {
		verdict.Class = ClassInternalUnskipped
		if verdict.SkipReason != "" {
			verdict.Class = ClassInternalNotExcluded
		}
		return verdict
	}
	if j.slugAnswers(route.Method, path, verdict) {
		return verdict
	}
	if j.aliasAnswers(route.Method, path, verdict) {
		return verdict
	}
	if j.inherits(verdict) {
		return verdict
	}
	if j.declares(route.Method, path, verdict) {
		return verdict
	}
	verdict.Class, verdict.Twin = ClassUnanswered, j.reachedAlias(route.Method, path)
	return verdict
}

// checkTables adds what is wrong with each table as the tree reads it.
func (j *judge) checkTables() {
	j.problems = append(j.problems, skipProblems(j.in.skips, j.carried)...)
	j.problems = append(j.problems, exceptionProblems(j.in.exceptions, j.excepted)...)
	j.problems = append(j.problems, j.graphqlProblems()...)
	j.problems = append(j.problems, slugProblems(j.in.slugs, j.slugRoutes, j.in.catalog)...)
	for index, family := range j.in.families {
		j.problems = append(j.problems, j.familyProblems(family, j.familyRoutes[index])...)
	}
}

// skipOf returns the reason a route skips the fine-grained check, empty when
// it does not.
func skipOf(route *apilive.Route) string {
	if route.Authorization == nil {
		return ""
	}
	return route.Authorization.Skip
}

// isInternal reports whether a path under the API prefix is GitLab's
// internal API, which its own components call.
func isInternal(path string) bool {
	return path == internalRoot || strings.HasPrefix(path, internalRoot+"/")
}

// internalRoot is where GitLab mounts the API its own components call.
const internalRoot = "/internal"

// scopedRoots are the first segments a route's group takes the second
// literal segment beside: a project's packages and a group's packages are
// two families, not one.
var scopedRoots = map[string]bool{
	"admin": true, "api": true, "groups": true, "namespaces": true, "projects": true, "user": true, "users": true,
}

// groupOf names the family a route reads in: its first literal segment, and
// the second beside it under a scope, spelled with every optional group left
// out.
func groupOf(path string) string {
	shortest := apilive.ExpandOptional(strings.TrimPrefix(path, apilive.EndpointPrefix))[0]
	var literal []string
	for segment := range strings.SplitSeq(strings.Trim(shortest, "/"), "/") {
		if segment == "" || segment == "-" || strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			continue
		}
		literal = append(literal, segment)
	}
	if len(literal) == 0 {
		return "/"
	}
	if len(literal) > 1 && scopedRoots[literal[0]] {
		return literal[0] + "/" + literal[1]
	}
	return literal[0]
}

// sortedKeys returns a set's keys, sorted.
func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// sortedUnique sorts a list and drops repeats.
func sortedUnique(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}
