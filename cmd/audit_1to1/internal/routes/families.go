// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import (
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// The categories of a family row.
const (
	// categoryClientProtocol: the protocol of a package manager's or a
	// model registry's own client (npm, Maven, PyPI, NuGet, Conan,
	// Composer, Helm, RubyGems, Go, Debian, Terraform modules, MLflow, the
	// protocol side of the virtual registries), which a client of that
	// protocol speaks and an action does not. A route client-go sends is
	// R-ACTION's, never this.
	categoryClientProtocol = "client-protocol"
	// categoryCoveredByGraphQL: the same operation is an action over
	// GraphQL, whose Actions the row names.
	categoryCoveredByGraphQL = "covered-by-graphql"
	// categoryTracked: GitLab serves the route to a caller with a token and
	// no action sends it yet, which the row's Issue tracks. An administrator's
	// route, a GitLab.com one and a tier's are tracked like any other.
	categoryTracked = "tracked"
)

// familyCategories are the categories a family row takes.
var familyCategories = map[string]bool{
	categoryClientProtocol:   true,
	categoryCoveredByGraphQL: true,
	categoryTracked:          true,
}

// The segments a pattern spells a wildcard with.
const (
	// anySegment stands for one segment.
	anySegment = "*"
	// anyTail, the last segment of a pattern, stands for any number of
	// segments, none included.
	anyTail = "**"
)

// routeFamily answers the routes no action reaches under one pattern.
type routeFamily struct {
	// Methods are the verbs the row answers, every verb when empty.
	Methods []string
	// Pattern is the record's path without /api/:version, "*" standing for
	// one segment and a last "**" for any tail. It is matched against every
	// spelling apilive.ExpandOptional gives the path, since an optional
	// group (`(-/)`) and NuGet's escaped parentheses split the text badly.
	Pattern  string
	Category string
	Reason   string
	// Issue tracks a tracked row, and Actions are the GraphQL actions a
	// covered-by-graphql row stands for.
	Issue   int
	Actions []string
	// Routes is how many routes no action reaches the row answers, pinned:
	// a re-pin that adds a route under the family, or an action that
	// starts sending one, changes the count and the gate names the routes.
	Routes int
}

// label names the row in a report.
func (f routeFamily) label() string {
	if len(f.Methods) == 0 {
		return "* " + f.Pattern
	}
	return strings.Join(f.Methods, ",") + " " + f.Pattern
}

// matches reports whether the row answers a route of a verb and a path under
// the API prefix.
func (f routeFamily) matches(method, path string) bool {
	if len(f.Methods) > 0 && !slices.Contains(f.Methods, method) {
		return false
	}
	for _, variant := range apilive.ExpandOptional(path) {
		if patternMatches(f.Pattern, variant) {
			return true
		}
	}
	return false
}

// patternMatches reports whether a path meets a pattern segment by segment,
// "*" standing for one segment and "**" for the rest of the path.
func patternMatches(pattern, path string) bool {
	want := strings.Split(strings.Trim(pattern, "/"), "/")
	got := strings.Split(strings.Trim(path, "/"), "/")
	for index, segment := range want {
		if segment == anyTail {
			return true
		}
		if index >= len(got) || (segment != anySegment && segment != got[index]) {
			return false
		}
	}
	return len(got) == len(want)
}

// declares decides a route a family row answers, the first row that matches
// answering it and a second that matches too being a problem of the table,
// and so is a tracked row over a route a decision of acceptedMissingMethods
// settles. It reports whether the route is decided.
func (j *judge) declares(method, path string, verdict *Route) bool {
	var matched []int
	for index, family := range j.in.families {
		if family.matches(method, path) {
			matched = append(matched, index)
		}
	}
	if len(matched) == 0 {
		return false
	}
	for _, other := range matched[1:] {
		j.problems = append(j.problems, "family "+j.in.families[other].label()+" answers "+verdict.Route+", which family "+j.in.families[matched[0]].label()+" answers too")
	}
	family := j.in.families[matched[0]]
	j.familyRoutes[matched[0]] = append(j.familyRoutes[matched[0]], verdict.Route)
	verdict.Class, verdict.Family = ClassDeclared, family.label()
	verdict.Category, verdict.Reason, verdict.Issue = family.Category, family.Reason, family.Issue
	verdict.Actions = slices.Clone(family.Actions)
	for _, decision := range verdict.Adjudications {
		if family.Category == categoryTracked && settlingAdjudications[decision.Category] {
			j.problems = append(j.problems, "family "+family.label()+" tracks "+verdict.Route+", and "+decision.Method+" is decided "+decision.Category+
				", which says the route needs no action of its own: an action still to implement is no method acceptedMissingMethods settles")
		}
	}
	return true
}

// familyProblems lists what is wrong with one family row as the tree reads
// it: what it lacks, the number of routes it answers against the number it
// pins, and a client-protocol row over a route client-go sends.
func (j *judge) familyProblems(family routeFamily, answered []string) []string {
	label := "family " + family.label()
	var out []string
	for _, need := range j.familyNeeds(family) {
		out = append(out, label+": "+need)
	}
	if len(answered) == 0 {
		out = append(out, label+" answers no route")
	}
	if len(answered) > 0 && len(answered) != family.Routes {
		out = append(out, label+" answers "+strconv.Itoa(len(answered))+" routes and pins "+strconv.Itoa(family.Routes)+": "+strings.Join(answered, ", "))
	}
	if family.Category != categoryClientProtocol {
		return out
	}
	for _, name := range answered {
		if methods := j.methodsOf[name]; len(methods) > 0 {
			out = append(out, label+" is client-protocol, and client-go sends "+name+" through "+strings.Join(methods, ", ")+": that route is R-ACTION's")
		}
	}
	return out
}

// familyNeeds says what a family row lacks: a category the table defines, a
// reason, a pattern under the API prefix whose "**" is its last segment, the
// issue a tracked row names and only it, and the GraphQL actions a
// covered-by-graphql row names, each in the catalog and sending GraphQL,
// and only it.
func (j *judge) familyNeeds(family routeFamily) []string {
	var out []string
	if !familyCategories[family.Category] {
		out = append(out, "category "+strconv.Quote(family.Category)+" is not one the table defines")
	}
	if family.Reason == "" {
		out = append(out, "it gives no reason")
	}
	if !strings.HasPrefix(family.Pattern, "/") || strings.HasPrefix(family.Pattern, apilive.EndpointPrefix) {
		out = append(out, "its pattern is not a path under "+apilive.EndpointPrefix)
	}
	if tail := strings.Index(family.Pattern, anyTail); tail >= 0 && tail != len(family.Pattern)-len(anyTail) {
		out = append(out, "its pattern puts "+anyTail+" before its last segment")
	}
	tracked := family.Category == categoryTracked
	if tracked != (family.Issue > 0) {
		out = append(out, "an issue is what a tracked row names, and only a tracked row")
	}
	graphql := family.Category == categoryCoveredByGraphQL
	if graphql != (len(family.Actions) > 0) {
		out = append(out, "actions are what a covered-by-graphql row names, and only a covered-by-graphql row")
	}
	for _, action := range family.Actions {
		out = append(out, j.actionNeeds(action)...)
	}
	return out
}

// actionNeeds says what a GraphQL action a declaration names lacks: a place
// in the catalog, and a GraphQL request in the request record.
func (j *judge) actionNeeds(action string) []string {
	var out []string
	if !j.in.catalog[action] {
		out = append(out, "it names "+action+", which the catalog does not hold")
	}
	if !j.graphql[action] {
		out = append(out, "it names "+action+", which sends no GraphQL request")
	}
	return out
}
