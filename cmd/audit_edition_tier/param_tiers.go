// The parameter half of the audit: a request parameter GitLab's API pages mark
// for a paid tier, against the input schema a lower tier is served.

package main

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// paramFinding is one request parameter GitLab documents for a paid tier that
// the input schema of a lower tier offers anyway.
type paramFinding struct {
	Action     string `json:"action"`
	Route      string `json:"route"`
	Param      string `json:"param"`
	Documented string `json:"documented"` // the tier the documentation row names
	OfferedAt  string `json:"offered_at"` // the lowest tier whose input schema carries it
	Doc        string `json:"doc"`        // the page the row was read from
}

// docEndpoint is one endpoint a documentation section spells out, as its
// method and the segments of its path.
type docEndpoint struct {
	method   string
	segments []string
}

// docParam is one request parameter a section's table marks for a paid tier.
type docParam struct {
	name string
	tier tier
}

// docSection is what one section of an API page says about the endpoints it
// documents: the endpoints, and the request parameters its tables mark for a
// paid tier. Only a section holding both is kept, since one without an
// endpoint has nothing to join its rows to.
type docSection struct {
	endpoints []docEndpoint
	params    []docParam
}

// endpointLine matches the way GitLab's documentation spells an endpoint inside
// a code block: a method, then the path. It is the reading R-PATH applies to
// the same pages (cmd/audit_1to1/internal/paths), which no command outside it
// can import. It is one interpreted literal, since a raw string cannot hold the
// backtick the path stops at.
var endpointLine = regexp.MustCompile("^(GET|POST|PUT|DELETE|PATCH|HEAD)\\s+(/?[A-Za-z:][^\\s\"'`]*)")

// paramName matches the parameter name a request table's first cell carries in
// backticks.
var paramName = regexp.MustCompile("^`([^`]+)`")

// paramTierMarker matches the phrases a request table row writes to say its
// parameter needs a paid tier: "Premium and Ultimate only", "Ultimate only",
// "Available on Premium and Ultimate", "(in the Premium and Ultimate tier)" and
// the "**(PREMIUM ALL)**" badge an older row carries.
var paramTierMarker = regexp.MustCompile(`(?i)premium and ultimate|ultimate only|premium only|\(premium all\)|\(ultimate all\)`)

// otherSubjectTier matches a tier phrase whose subject the row names in
// backticks, "`member_role_id` is Ultimate only", which is about that key of
// the parameter's value rather than about the parameter.
var otherSubjectTier = regexp.MustCompile("`([^`]+)` is (?i:premium and ultimate|ultimate) only")

// freeElsewhere is the clause a row adds when the tier it names is not the
// whole answer: "Premium and Ultimate. Also available on the Free tier on
// GitLab.com with GitLab Credits." Such a row grades nothing, since a Free
// GitLab.com instance is served the parameter.
const freeElsewhere = "also available on the free tier"

// apiPrefix is what a documentation line sometimes carries in front of the
// path, and the recorded route never does.
const apiPrefix = "/api/v4"

// readActionRoutes reads the REST routes each catalog action sends, keyed by
// canonical action ID, from the record cmd/gen_action_grants derives from the
// handlers. A GraphQL request carries no route and is left out.
func readActionRoutes(root string) (map[string][]string, error) {
	record, err := actionrequests.ReadRecord(root)
	if err != nil {
		return nil, err
	}
	routes := map[string][]string{}
	for _, action := range record.Actions {
		for _, request := range action.Requests {
			if request.Kind == actionrequests.KindREST {
				routes[action.ID] = append(routes[action.ID], request.Route)
			}
		}
	}
	return routes, nil
}

// offeredTiers maps each action to the lowest tier whose input schema carries
// each of its parameters, read from the catalogs a Free, a Premium and an
// Ultimate instance are served. The tier filter prunes a parameter tagged for
// a higher tier out of the lower tiers' schemas, so this is what a client of
// each tier is offered rather than what the input struct declares.
func offeredTiers(free, premium, ultimate *actioncatalog.Catalog) map[string]map[string]tier {
	offered := map[string]map[string]tier{}
	for _, level := range []struct {
		tier    tier
		catalog *actioncatalog.Catalog
	}{{tierFree, free}, {tierPremium, premium}, {tierUltimate, ultimate}} {
		for _, action := range level.catalog.Actions() {
			properties, _ := action.Route.InputSchema["properties"].(map[string]any)
			for name := range properties {
				params := offered[string(action.ID)]
				if params == nil {
					params = map[string]tier{}
					offered[string(action.ID)] = params
				}
				if _, seen := params[name]; !seen {
					params[name] = level.tier
				}
			}
		}
	}
	return offered
}

// parseParamSections splits an API page into its sections and keeps, for each
// one, the endpoints its code blocks spell and the request parameters its
// tables mark for a paid tier.
//
// A section runs from one heading to the next. A request table is one whose
// header has a Required column, which is what tells it from a response
// attribute table on the same section: a response attribute marked Premium
// says what GitLab sends, not what a caller may ask for. A line starting with
// a hash inside a code block is a shell comment, not a heading.
func parseParamSections(content string) []docSection {
	var sections []docSection
	var current docSection
	inFence, inTable, requestTable := false, false, false
	flush := func() {
		if len(current.endpoints) > 0 && len(current.params) > 0 {
			sections = append(sections, current)
		}
		current = docSection{}
	}
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			inFence = !inFence
		case inFence:
			if endpoint, ok := parseEndpoint(trimmed); ok {
				current.endpoints = append(current.endpoints, endpoint)
			}
		case strings.HasPrefix(trimmed, "#"):
			flush()
		case !strings.HasPrefix(trimmed, "|"):
			inTable = false
		case !inTable:
			inTable, requestTable = true, isRequestHeader(trimmed)
		case requestTable:
			if param, ok := parseParamRow(trimmed); ok {
				current.params = append(current.params, param)
			}
		}
	}
	flush()
	return sections
}

// parseEndpoint reads one endpoint line. A line has to carry a slash to be an
// endpoint, so a sentence opening with a method name is not read as one.
func parseEndpoint(line string) (docEndpoint, bool) {
	match := endpointLine.FindStringSubmatch(line)
	if len(match) == 0 || !strings.Contains(match[2], "/") {
		return docEndpoint{}, false
	}
	return docEndpoint{method: match[1], segments: pathSegments(match[2])}, true
}

// pathSegments turns a written path into its segments, dropping the query, the
// /api/v4 prefix a documentation line sometimes carries and the slashes at
// either end.
func pathSegments(path string) []string {
	path, _, _ = strings.Cut(path, "?")
	path = strings.TrimPrefix(path, apiPrefix)
	return strings.Split(strings.Trim(path, "/"), "/")
}

// isRequestHeader reports whether a table's header row has a Required column.
func isRequestHeader(header string) bool {
	return slices.ContainsFunc(strings.Split(header, "|"), func(cell string) bool {
		return strings.EqualFold(strings.TrimSpace(cell), "required")
	})
}

// parseParamRow reads one request table row: the parameter its first cell
// names and the tier the rest of the row marks it for. A nested name,
// `position[base_sha]` or `assignee_ids[]`, is read as the parameter it
// belongs to, which is the name an input schema carries. A row starts with a
// pipe, so splitting it always leaves the first cell at index 1, and a row
// with nothing after that cell marks no tier.
func parseParamRow(row string) (docParam, bool) {
	cells := strings.Split(row, "|")
	match := paramName.FindStringSubmatch(strings.TrimSpace(cells[1]))
	if match == nil {
		return docParam{}, false
	}
	name, _, _ := strings.Cut(match[1], "[")
	paramTier, marked := paramRowTier(name, strings.Join(cells[2:], "|"))
	if !marked {
		return docParam{}, false
	}
	return docParam{name: name, tier: paramTier}, true
}

// paramRowTier reads the tier the rest of a row marks its parameter for, and
// reports false when the row marks none or marks something other than the
// parameter: a tier whose subject is another name in backticks, or one the row
// says the Free tier is also served.
func paramRowTier(name, rest string) (tier, bool) {
	if strings.Contains(strings.ToLower(rest), freeElsewhere) {
		return tierFree, false
	}
	for _, match := range otherSubjectTier.FindAllStringSubmatch(rest, -1) {
		if match[1] != name {
			return tierFree, false
		}
	}
	marker := strings.ToLower(paramTierMarker.FindString(rest))
	if marker == "" {
		return tierFree, false
	}
	if strings.Contains(marker, "premium") {
		return tierPremium, true
	}
	return tierUltimate, true
}

// documents reports whether the section spells out the route, a recorded
// "METHOD /path": the same method and a path of the same shape, where a
// placeholder on both sides matches whatever either names it. A route recorded
// with an optional group joins an endpoint written in any of its forms.
func (s docSection) documents(route string) bool {
	method, path, _ := strings.Cut(route, " ")
	return slices.ContainsFunc(optionalForms(path), func(form string) bool {
		segments := pathSegments(form)
		return slices.ContainsFunc(s.endpoints, func(endpoint docEndpoint) bool {
			return endpoint.method == method && slices.EqualFunc(segments, endpoint.segments, sameSegment)
		})
	})
}

// optionalForms expands every optional group Grape writes into a recorded
// route, "(-/)search" or "(ref/:ref/)trigger/pipeline", into the paths the
// route answers at: the first group with its contents and without them, and
// each of those expanded in turn, so a nested group is read once its enclosing
// one is kept. Split as written, "(-" and ")search" are segments no documented
// endpoint has. A group that is never closed is left as it is written.
func optionalForms(path string) []string {
	before, rest, opened := strings.Cut(path, "(")
	if !opened {
		return []string{path}
	}
	inner, after, closed := closeGroup(rest)
	if !closed {
		return []string{path}
	}
	return append(optionalForms(before+inner+after), optionalForms(before+after)...)
}

// closeGroup splits what follows an opening parenthesis into the group's
// contents and what follows the parenthesis that closes it, counting the
// groups opened inside it, and reports false when nothing closes it.
func closeGroup(rest string) (inner, after string, closed bool) {
	depth := 1
	for i := range len(rest) {
		if rest[i] == '(' {
			depth++
		}
		if rest[i] == ')' {
			depth--
			if depth == 0 {
				return rest[:i], rest[i+1:], true
			}
		}
	}
	return "", "", false
}

// sameSegment reports whether a recorded path segment and a documented one name
// the same thing: the same literal, or a placeholder on both sides.
func sameSegment(recorded, documented string) bool {
	return recorded == documented || isPlaceholder(recorded) && isPlaceholder(documented)
}

// isPlaceholder reports whether a path segment stands for a value the caller
// supplies, which GitLab writes as :id, *path or <id>.
func isPlaceholder(segment string) bool {
	return strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") || strings.HasPrefix(segment, "<")
}

// pageSections is one page's sections with the page they were read from, so a
// finding can name it.
type pageSections struct {
	ref      docRef
	sections []docSection
}

// gradeParams joins every action of a domain to the tier-marked request rows
// of the sections documenting the routes it sends, on the owner page and on
// the override page the action is redirected to, and reports each row whose
// parameter a lower tier's input schema offers. It also returns how many rows
// were joined to an action, so a report with no finding still says whether
// anything was compared.
func (r *docResolver) gradeParams(ctx context.Context, owner pageSections, actions []actionDetail, offered map[string]map[string]tier) (findings []paramFinding, joined int) {
	for _, action := range actions {
		pages := []pageSections{owner}
		if ref, ok := docOverrideForAction(action.ID); ok {
			if page := r.page(ctx, ref); page.err == nil {
				pages = append(pages, pageSections{ref: ref, sections: page.sections})
			}
		}
		for _, route := range r.routes[action.ID] {
			for _, page := range pages {
				found, compared := gradeRoute(action.ID, route, page, offered[action.ID])
				joined += compared
				for _, finding := range found {
					if !slices.Contains(findings, finding) {
						findings = append(findings, finding)
					}
				}
			}
		}
	}
	return findings, joined
}

// gradeRoute compares one route of one action with the tier-marked rows of the
// sections of one page documenting it.
func gradeRoute(id, route string, page pageSections, offered map[string]tier) (findings []paramFinding, compared int) {
	for _, section := range page.sections {
		if !section.documents(route) {
			continue
		}
		for _, param := range section.params {
			compared++
			if at, ok := offered[param.name]; ok && at < param.tier {
				findings = append(findings, paramFinding{
					Action: id, Route: route, Param: param.name,
					Documented: param.tier.String(), OfferedAt: at.String(), Doc: page.ref.docPath(),
				})
			}
		}
	}
	return findings, compared
}
