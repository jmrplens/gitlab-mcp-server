package paths

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/requestinventory"
)

// Endpoint is one recorded REST request, named the way a report lists it.
type Endpoint struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Packages are the packages seen issuing it, sorted, since one endpoint is
	// reached from several packages often enough that naming only one would
	// send a reader to the wrong file.
	Packages []string `json:"packages"`
}

// EndpointCheck is what the documentation comparison found, and how much of the
// documentation it managed to read.
//
// It is a candidate list and never a gate, and the reason is that the oracle is
// prose. GitLab writes its endpoints as `METHOD /path` lines in fenced blocks,
// which is regular enough to compare against, and then:
//
//   - 57 of those lines omit the leading slash, and project_access_tokens.md
//     writes every one of its endpoints that way;
//   - emoji_reactions.md gives the note reactions one plaintext block, for
//     issues, and leaves the merge request and snippet variants to prose;
//   - usage_data.md documents /usage_data/track_events in a sentence and a curl
//     example, never as a line;
//   - the generic package registry and Terraform state are documented outside
//     doc/api entirely, under doc/user;
//   - client-go still spells the integrations endpoints /services/, an alias
//     GitLab replaced with /integrations/ and no longer documents, and the
//     Orbit Knowledge Graph endpoints are experimental and documented nowhere.
//
// None of those is a defect in this server, and every one of them looks exactly
// like one. A gate that fails a release over the spelling of a documentation
// page would be worse than no check, so this reports and does not fail.
//
// The second half of the honesty is coverage. UnreadAreas counts the pages the
// fetch could not get, and a hole there produces candidates that are only about
// the hole: a reader weighing this list has to read that number first.
type EndpointCheck struct {
	// Ran says whether the comparison was asked for at all.
	Ran bool `json:"ran"`
	// Areas is how many documentation pages were read.
	Areas int `json:"areas,omitempty"`
	// UnreadAreas names the pages that could not be fetched, sorted. Every
	// endpoint documented only on one of them is reported as undocumented, so
	// this is the first thing a reader of the candidates needs.
	UnreadAreas []string `json:"unread_areas,omitempty"`
	// DocumentedEndpoints is how many distinct method-and-path shapes the pages
	// spell out.
	DocumentedEndpoints int `json:"documented_endpoints,omitempty"`
	// RecordedEndpoints is how many distinct ones this server was recorded
	// issuing.
	RecordedEndpoints int `json:"recorded_endpoints,omitempty"`
	// Undocumented are the recorded endpoints no page spells out.
	Undocumented []Endpoint `json:"undocumented,omitempty"`
}

// skippedAreaPrefix is the one part of doc/api this does not read: the GraphQL
// reference pages are generated SDL dumps with no REST endpoint anywhere in
// them, and the largest is several megabytes.
const skippedAreaPrefix = "graphql/reference/"

// endpointLine matches the way GitLab's documentation spells an endpoint: a
// method, then the path.
//
// The leading slash is optional because 57 lines across the documentation do
// not have one, all of project_access_tokens.md among them, and demanding it
// would report every endpoint on those pages as undocumented.
var endpointLine = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD)\s+(/?[A-Za-z:][^\s"'` + "`" + `]*)`)

// apiPrefix is what a documentation line sometimes carries in front of the
// path, and the recorder always trims.
const apiPrefix = "/api/v4"

// checkEndpoints compares every recorded REST endpoint with GitLab's own
// documentation.
//
// A page that cannot be fetched is recorded and skipped rather than failing the
// run: the documentation is 250 pages and one renamed page must not turn a
// candidate list into a page of noise about a fetch.
func checkEndpoints(ctx context.Context, fetcher *apidocs.Fetcher, rows []requestinventory.Row) (EndpointCheck, error) {
	areas, err := fetcher.Areas(ctx)
	if err != nil {
		return EndpointCheck{}, err
	}

	check := EndpointCheck{Ran: true, UnreadAreas: []string{}}
	documented := map[string][][]string{}
	for _, area := range areas {
		if strings.HasPrefix(area, skippedAreaPrefix) {
			continue
		}
		page, fetchErr := fetcher.Fetch(ctx, area)
		if fetchErr != nil {
			// A cancelled sweep is not a documentation gap. Without this, one
			// Ctrl+C halfway through 250 pages reports every endpoint below the
			// interruption as undocumented, which is a page of findings about
			// the interruption.
			if ctx.Err() != nil {
				return EndpointCheck{}, fmt.Errorf("read %s: %w", area, fetchErr)
			}
			check.UnreadAreas = append(check.UnreadAreas, area)
			continue
		}
		check.Areas++
		addDocumentedEndpoints(documented, page)
	}
	for _, shapes := range documented {
		check.DocumentedEndpoints += len(shapes)
	}

	recorded := recordedEndpoints(rows)
	check.RecordedEndpoints = len(recorded)
	check.Undocumented = make([]Endpoint, 0)
	for _, endpoint := range recorded {
		if !isDocumented(documented[endpoint.Method], endpoint.Path) {
			check.Undocumented = append(check.Undocumented, endpoint)
		}
	}
	return check, nil
}

// addDocumentedEndpoints records every endpoint one documentation page spells
// out, as the segments of its path.
func addDocumentedEndpoints(documented map[string][][]string, page string) {
	for line := range strings.SplitSeq(page, "\n") {
		match := endpointLine.FindStringSubmatch(strings.TrimSpace(line))
		// A line has to carry a slash to be an endpoint. Without that, a
		// sentence beginning "GET is the method this endpoint takes" is read as
		// an endpoint called "is", and every recorded single-segment path then
		// matches something the documentation never said.
		if len(match) == 0 || !strings.Contains(match[2], "/") {
			continue
		}
		segments := normalizePath(match[2])
		if len(segments) == 0 {
			continue
		}
		method := match[1]
		if !slices.ContainsFunc(documented[method], func(known []string) bool { return slices.Equal(known, segments) }) {
			documented[method] = append(documented[method], segments)
		}
	}
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
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return endpoints[i].Method < endpoints[j].Method
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
func matchesShape(recorded, documented []string) bool {
	for i, want := range documented {
		if isGlob(want) {
			// A glob swallows the rest of the path, which is how the model
			// registry documents a file path that may have slashes in it.
			return i < len(recorded)
		}
		if i >= len(recorded) {
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
