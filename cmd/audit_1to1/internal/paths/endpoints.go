// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// Endpoint is one recorded REST request, named the way a report lists it.
type Endpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Packages are the packages seen issuing it, sorted, since one endpoint is
	// reached from several packages often enough that naming only one would
	// send a reader to the wrong file.
	Packages []string `json:"packages"`
	// Category and Reason are the declaration that accounts for this endpoint
	// being absent from the documentation, and are empty for one nothing
	// accounts for, which is what the gate fails on.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// declared reports whether a declaration accounts for this endpoint.
func (e Endpoint) declared() bool { return e.Category != "" }

// EndpointCheck is what the documentation comparison found, and which oracle
// it read.
//
// A recorded endpoint GitLab does not document is a finding, and the issue this
// dimension answers says so: a path GitLab does not have is a failure, not a
// report. What stops that from being true of the raw comparison is that the
// oracle is prose. GitLab writes its endpoints as `METHOD /path` lines in
// fenced blocks, which is regular enough to compare against, and then:
//
//   - 60 of those lines omit the leading slash at 19.5.0-ee, and
//     project_access_tokens.md writes every one of its endpoints that way;
//   - emoji_reactions.md gives the note reactions one plaintext block, for
//     issues, and leaves the merge request and snippet variants to prose;
//   - usage_data.md documents /usage_data/track_events in a sentence and a curl
//     example, never as a line;
//   - the generic package registry and Terraform state are documented outside
//     doc/api entirely, under doc/user;
//   - the job artifacts of a ref are documented under :ref_name, which the
//     route lets carry a slash, so a branch named feature/x fills two segments
//     where the line has one;
//   - client-go still spells the integrations endpoints /services/, an alias
//     GitLab replaced with /integrations/ and no longer documents, and the
//     Orbit Knowledge Graph endpoints are documented only in a generated
//     reference their doc/api page points at and spells no line of.
//
// None of those is a defect in this server, and every one of them looks exactly
// like one. A gate that failed a release over the spelling of a documentation
// page would be worse than no check at all, so each of those shapes is
// declared in endpoint_declarations.go with a category and a reason, the way a
// silent package is, and the gate fails on what no declaration accounts for. A
// declaration that stops matching anything is a finding too, so the excuse
// cannot outlive the thing it excuses.
//
// The oracle is read once and committed ([DocEndpointsFile], written by make
// gen-api-doc-endpoints), at the tag of the release the live record was taken
// from, so the comparison needs no network and runs with the rest of the
// scope, in CI and in make analyze, whenever -check-endpoints asks for it.
// Until it was committed it read 250 pages over the network on every run, so
// nothing ran it: no cache held a listing, and the declarations in
// endpoint_declarations.go went unjudged. A committed oracle has no unread
// page by construction, because its generator refuses to write one with a
// hole, and an oracle read at another release than the live record's is
// refused rather than compared, since the two would speak for two GitLabs.
type EndpointCheck struct {
	// Ran says whether the comparison was asked for at all.
	Ran bool `json:"ran"`
	// Oracle and Ref name the file that answered and the tag its pages were
	// read at.
	Oracle string `json:"oracle,omitempty"`
	Ref    string `json:"ref,omitempty"`
	// Areas is how many documentation pages the oracle was read from.
	Areas int `json:"areas,omitempty"`
	// DocumentedEndpoints is how many distinct method-and-path shapes the pages
	// spell out.
	DocumentedEndpoints int `json:"documented_endpoints,omitempty"`
	// RecordedEndpoints is how many distinct ones this server was recorded
	// issuing.
	RecordedEndpoints int `json:"recorded_endpoints,omitempty"`
	// Undocumented are the recorded endpoints no page spells out, each
	// carrying the declaration that accounts for it when one does.
	Undocumented []Endpoint `json:"undocumented,omitempty"`
	// UnusedDeclarations names the declarations that matched no undocumented
	// endpoint in this run, sorted.
	UnusedDeclarations []string `json:"unused_declarations,omitempty"`
}

// undeclared counts the undocumented endpoints no declaration accounts for.
func (c EndpointCheck) undeclared() int {
	count := 0
	for _, endpoint := range c.Undocumented {
		if !endpoint.declared() {
			count++
		}
	}
	return count
}

// staleDeclarations renders this run's unused declarations as the findings the
// report lists beside the silent-owner ones.
//
// A run that did not compare anything has nothing to say about them: every
// declaration would look unused, and the loudest wrong answer this could give
// is that they are all stale.
func (c EndpointCheck) staleDeclarations() []string {
	if !c.Ran {
		return nil
	}
	stale := make([]string, 0, len(c.UnusedDeclarations))
	for _, shape := range c.UnusedDeclarations {
		stale = append(stale, shape+" is declared undocumented and is not: the documentation now spells it out, or nothing issues it any more")
	}
	return stale
}

// skippedAreaPrefix is the one part of doc/api this does not read: the GraphQL
// reference pages are generated SDL dumps with no REST endpoint anywhere in
// them, and the largest is several megabytes.
const skippedAreaPrefix = "graphql/reference/"

// endpointLine matches the way GitLab's documentation spells an endpoint: a
// method, then the path.
//
// The leading slash is optional because 60 lines across the documentation do
// not have one at 19.5.0-ee, all of project_access_tokens.md among them, and
// demanding it would report every endpoint on those pages as undocumented.
var endpointLine = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD)\s+(/?[A-Za-z:][^\s"'` + "`" + `]*)`)

// apiPrefix is what a documentation line sometimes carries in front of the
// path, and the recorder always trims.
const apiPrefix = "/api/v4"

// checkEndpoints compares every recorded REST endpoint with GitLab's own
// documentation, as the committed oracle under root records it.
//
// An oracle it cannot read, and one read at another release than the live
// record's, are errors rather than findings: neither says anything about an
// endpoint, and a comparison against either would report the oracle as the
// server's defect.
func checkEndpoints(root string, rows []requestinventory.Row) (EndpointCheck, error) {
	oracle, err := readDocEndpoints(root)
	if err != nil {
		return EndpointCheck{}, err
	}
	release, err := ReleaseRef(root)
	if err != nil {
		return EndpointCheck{}, fmt.Errorf("read the release the documentation oracle must have been read at: %w", err)
	}
	if oracle.Source.Ref != release {
		return EndpointCheck{}, fmt.Errorf("%s was read at %s and the live API record is of %s: regenerate it with make gen-api-doc-endpoints",
			DocEndpointsFile, oracle.Source.Ref, release)
	}

	check := EndpointCheck{
		Ran:                 true,
		Oracle:              DocEndpointsFile,
		Ref:                 oracle.Source.Ref,
		Areas:               oracle.Source.Areas,
		DocumentedEndpoints: len(oracle.Endpoints),
	}
	documented := oracle.documented()
	recorded := recordedEndpoints(rows)
	check.RecordedEndpoints = len(recorded)
	found := make([]Endpoint, 0)
	for _, endpoint := range recorded {
		if !isDocumented(documented[endpoint.Method], endpoint.Path) {
			found = append(found, endpoint)
		}
	}
	check.Undocumented, check.UnusedDeclarations = classifyUndocumented(found)
	return check, nil
}

// normalizePath turns a written path into its segments, dropping the query, the
// fragment, the /api/v4 prefix a documentation line sometimes carries and the
// trailing slash a few of them end with.
func normalizePath(path string) []string {
	path, _, _ = strings.Cut(path, "?")
	path, _, _ = strings.Cut(path, "#")
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimPrefix(path, apiPrefix)
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// recordedEndpoints folds the inventory into one entry per REST method and
// path, naming every package seen issuing it.
func recordedEndpoints(rows []requestinventory.Row) []Endpoint {
	packages := map[string]map[string]struct{}{}
	for _, row := range rows {
		if row.Kind != requestinventory.KindREST {
			continue
		}
		key := row.Method + " " + row.Path
		if packages[key] == nil {
			packages[key] = map[string]struct{}{}
		}
		packages[key][row.Package] = struct{}{}
	}

	endpoints := make([]Endpoint, 0, len(packages))
	for key, seen := range packages {
		method, path, _ := strings.Cut(key, " ")
		named := make([]string, 0, len(seen))
		for pkg := range seen {
			named = append(named, pkg)
		}
		sort.Strings(named)
		endpoints = append(endpoints, Endpoint{Method: method, Path: path, Packages: named})
	}
	slices.SortFunc(endpoints, func(a, b Endpoint) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.Method, b.Method))
	})
	return endpoints
}

// isDocumented reports whether any documented path of the same method has this
// one's shape.
func isDocumented(documented [][]string, path string) bool {
	segments := normalizePath(path)
	return slices.ContainsFunc(documented, func(known []string) bool { return matchesShape(segments, known) })
}

// matchesShape compares a recorded path with a documented one segment by
// segment, where a documented placeholder matches whatever we put there.
//
// The tolerance is deliberate and it is one-sided. Our own templating replaces
// a segment it can recognize as an identifier, and a branch name, a wiki slug,
// a CI variable key and a template name all look like ordinary words, so the
// recorded path carries the fixture's value where the documentation carries
// :branch. Letting the documentation's placeholder match anything is what makes
// those comparable; the cost is that a genuinely wrong literal in a placeholder
// position is accepted, which is the right way round for a list whose whole
// value is that a reader trusts its entries.
//
// An empty segment is the exception, and it has to be: a placeholder stands for
// a value the caller supplies, and no value at all is not one. Without this,
// /projects//statistics matched the documented /projects/:id/statistics and
// the eight requests this server makes with an empty identifier were counted
// as endpoints GitLab documents, which is precisely the shape of request this
// dimension exists to notice.
func matchesShape(recorded, documented []string) bool {
	for i, want := range documented {
		if isGlob(want) {
			// A glob swallows the rest of the path, which is how the model
			// registry documents a file path that may have slashes in it.
			return i < len(recorded)
		}
		if i >= len(recorded) || recorded[i] == "" {
			return false
		}
		if isPlaceholder(want) || want == recorded[i] {
			continue
		}
		return false
	}
	return len(recorded) == len(documented)
}

// digits matches a segment written as a number, which is how the documentation
// spells an identifier when it writes a worked example rather than a template.
var digits = regexp.MustCompile(`^\d+$`)

// isPlaceholder reports whether a documented segment stands for a value the
// caller supplies.
func isPlaceholder(segment string) bool {
	return strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "<") || digits.MatchString(segment)
}

// isGlob reports whether a documented segment stands for the whole rest of the
// path, which GitLab writes as (*path) where a file path may carry slashes.
func isGlob(segment string) bool {
	return strings.HasPrefix(segment, "*") || strings.HasPrefix(segment, "(*")
}

// String renders one endpoint as the line a text report prints.
func (e Endpoint) String() string {
	return fmt.Sprintf("%s %s (%s)", e.Method, e.Path, strings.Join(e.Packages, ", "))
}
