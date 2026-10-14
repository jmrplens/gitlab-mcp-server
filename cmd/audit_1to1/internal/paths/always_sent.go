package paths

import (
	"cmp"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// AlwaysSentCheck is the other half of R-PATH's blind spot about values: the
// inventory records which field names a body carried and nothing about what was
// in them, so no rule here could see a field this server sends as null on every
// call.
//
// The defect it was built against is `package.protection_rule_update`.
// client-go's UpdatePackageProtectionRulesOptions declares package_name_pattern
// and package_type without omitempty, so encoding/json writes both keys
// whatever the handler filled in, and a caller changing only the minimum access
// level sends GitLab `"package_name_pattern": null`. GitLab marks both optional
// on that PATCH route, which is exactly what makes sending them unconditionally
// wrong: on the POST beside it they are required, and there sending them always
// is correct. Five dimensions of this audit were green on it, because the
// surface is perfect and the request is not, and the sixth recorded
// `body: [package_name_pattern, package_type, …]` and could not tell a name
// that was sent from a value that was meant.
//
// # The oracle, and why it separates the defect from the innocent cases
//
// Two records already here answer it between them. client-go's own source says
// which option struct a service method takes and which json keys it writes
// unconditionally; the live record says, per mounted route, which params GitLab
// declares and which of them it requires. A field written on every call whose
// param GitLab requires anyway is correct, and one whose param GitLab marks
// optional is a value the caller cannot decline to send.
//
// That is a sharp line rather than a heuristic, and it was measured before it
// was written: over the option structs the 41 packages of one review reach, it
// separates the one defect from eight benign always-sent fields with no
// declaration table doing any of the work. CreateDraftNoteOptions.Note,
// PositionOptions.PositionType (required as position[position_type]), both
// fields of CreateProjectAliasOptions, the three runner registration tokens and
// EnableProjectRunnerOptions.RunnerID are all always sent and all required.
//
// # What it cannot see
//
// The rule reads the json tag, so it sees exactly what encoding/json's
// omitempty decides. A field whose type is a plain struct carrying omitempty is
// still written on every call, because under v1 semantics a struct is never
// empty, and this does not report it. client-go has no such field today: its
// nullable values are Nullable[T], which is a map and which omitempty does omit.
// The blind spot is stated rather than absorbed because a future SDK change
// could open it silently.
//
// The join is the package rather than the action: the inventory says which
// package reached which route, the action request record
// (docs/development/action-requests.json) says which client-go methods that
// package's handlers call, and the SDK says which option struct each of those
// methods takes to that route. A route two methods reach with different
// structs is asked about under the struct the package's own method takes. It
// used to be asked about under both, and so charged a package with a struct
// it never passes (issue 1104): POST /groups/:id/share is reached by
// GroupMembers.ShareWithGroup and Groups.ShareGroupWithGroup, and the two
// packages that call the second were reported for the first's expires_at.
// Where the record names no method of the package that reaches the route (a
// body the handler builds itself, a package that owns no action, or a call
// the derivation missed) the endpoint is asked about under every struct the
// route accepts, as it was, and listed in NotNarrowed so a reader can tell
// which findings rest on that. The record names a package by its Go name and
// the inventory by its directory, and the two meet on the directory's last
// element, which every package here shares with its name. The join is still
// the package, so a package two of whose handlers reach one route through two
// methods is asked about under both structs on that route.
//
// The rule reads the struct's tags and nothing the handler does to the body
// afterwards. A request option that edits the JSON client-go wrote
// (toolutil.EditJSONBody) is invisible to it, so releases.Update, which drops
// the name and the description a caller left out, is still reported for both.
//
// # It reports and does not gate
//
// A finding here is a request this server sends wrongly, which sounds like
// something to fail on, and the fix for almost all of them is upstream: the tag
// belongs to client-go. What this repository can do locally is build the body
// itself, which is a handler rewrite rather than a line. Failing a build on
// somebody else's struct tag is the shape [SDKGraphQLCheck] already declined.
type AlwaysSentCheck struct {
	// Ran is false when the client-go source or the live record could not be
	// read, which are the only two ways this check is skipped. Both render as
	// no findings, and so does a clean tree.
	Ran bool `json:"ran"`
	// Record names the artifact that answered about requiredness.
	Record string `json:"record,omitempty"`
	// Requests names the artifact that answered which client-go methods each
	// package's handlers call. It is empty when that record could not be
	// read, and then every endpoint is judged against every option struct
	// its route accepts and listed in NotNarrowed.
	Requests string `json:"requests_record,omitempty"`
	// Endpoints counts the recorded body-carrying endpoints this was asked of.
	Endpoints int `json:"endpoints"`
	// OptionTypes counts the distinct client-go option structs those endpoints
	// reach, which is the size of what was walked.
	OptionTypes int `json:"option_types"`
	// Fields counts the params those structs write unconditionally.
	Fields int `json:"always_sent_fields"`
	// Required counts how many of them GitLab requires anyway, which is the
	// benign majority and the evidence that the rule is not just counting
	// missing tags.
	Required int `json:"required_by_gitlab"`
	// Unmatched counts the always-sent params the route declares nothing for,
	// which is a comparison that could not be made rather than a finding: the
	// record's params come from the route's own declaration and a handful of
	// endpoints declare none.
	Unmatched int `json:"not_declared_by_the_route"`
	// Optional is the finding list: a param the SDK writes on every call that
	// GitLab lets a caller leave out.
	Optional []AlwaysSentField `json:"optional_but_always_sent,omitempty"`
	// NotNarrowed lists the endpoints judged against every option struct
	// their route accepts, because the action request record names no method
	// of the package's handlers that reaches the route. A finding on one of
	// them may be about a struct the package never passes.
	NotNarrowed []AlwaysSentEndpoint `json:"not_narrowed_to_the_package,omitempty"`
}

// AlwaysSentEndpoint is one endpoint a package reached that was judged
// against every option struct its route accepts.
type AlwaysSentEndpoint struct {
	Package     string   `json:"package"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	OptionTypes []string `json:"option_types"`
}

// AlwaysSentField is one param an option struct writes unconditionally on a
// route where GitLab marks it optional.
type AlwaysSentField struct {
	// Package is the package the inventory recorded reaching the endpoint.
	Package string `json:"package"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	// OptionType is the client-go struct declaring the field, which is not
	// necessarily the one the method takes: a nested option struct is named
	// here and its parent's key is in Param.
	OptionType string `json:"option_type"`
	// Field is the Go field name, so the finding can be grepped for.
	Field string `json:"field"`
	// Param is what GitLab calls it, nested keys in subscripts.
	Param string `json:"param"`
	// ParamType is what the route says the param is, which is what a reader
	// needs to judge what a null in that position does.
	ParamType string `json:"param_type,omitempty"`
}

// bodyMethods are the verbs whose options client-go encodes as a JSON body, and
// so the verbs whose omitempty decides what GitLab receives. A GET carries its
// options in the query string, where a nil pointer is left out whatever the
// json tag says.
var bodyMethods = map[string]bool{
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodPatch:  true,
	http.MethodDelete: true,
}

// readOptions is the seam for the client-go source, which lives in a module
// cache a test has none of. It is a variable a test restores, like readRoutes
// beside it.
var readOptions = readSDKOptions

// alwaysSentCheck reports every param an option struct writes on every call
// that GitLab marks optional on the route receiving it.
func alwaysSentCheck(root string, requests []requestinventory.Row) AlwaysSentCheck {
	record, err := apilive.Read(recordDir(root))
	if err != nil {
		return AlwaysSentCheck{}
	}
	pairings, pairErr := collectPairings(root)
	if pairErr != nil || pairings.ClientGoDir == "" {
		return AlwaysSentCheck{}
	}
	options := readOptions(pairings.ClientGoDir)
	if len(options.Types) == 0 {
		return AlwaysSentCheck{}
	}

	index := newOperationIndex(record)
	byRoute := optionTypesByRoute(options)
	check := AlwaysSentCheck{Ran: true, Record: apilive.FileName, Optional: []AlwaysSentField{}}
	var calls map[string]map[string]bool
	if derived, readErr := actionrequests.ReadRecord(root); readErr == nil {
		check.Requests = actionrequests.RecordPath
		calls = packageMethods(derived)
	}

	walked := map[string]bool{}
	for _, endpoint := range bodyEndpoints(requests) {
		operation, quality, _ := index.lookup(endpoint.method, endpoint.path)
		if quality == matchNone || len(operation.Params) == 0 {
			continue
		}
		takers := byRoute[endpoint.method+" "+pathShape(endpoint.path)]
		if len(takers) == 0 {
			continue
		}
		check.Endpoints++
		types := takers.takenBy(calls[path.Base(endpoint.pkg)])
		if len(types) == 0 {
			types = takers.optionTypes()
			check.NotNarrowed = append(check.NotNarrowed, AlwaysSentEndpoint{
				Package: endpoint.pkg, Method: endpoint.method, Path: endpoint.path, OptionTypes: types,
			})
		}
		for _, name := range types {
			walked[name] = true
			check.judge(endpoint, name, options.Types, operation.Params)
		}
	}
	check.OptionTypes = len(walked)
	slices.SortFunc(check.Optional, compareAlwaysSent)
	return check
}

// packageMethods reads, from the action request record, the client-go methods
// each package's handlers call, keyed by the Go package name the record
// qualifies its symbols with.
//
// A request is credited to the package of every site that builds it and of
// every handler of its action, because the inventory files a request under
// the package whose test drove it, which is either: a guided flow in
// elicitationtools that creates its issue through issues.Create is recorded
// under elicitationtools, and a helper in toolutil tested on its own under
// toolutil. Crediting a package with more requests than its own tests drove
// holds it to more structs, never to fewer, except where it lifts an endpoint
// off the not-narrowed list, and that takes a method the record says reaches
// the route.
func packageMethods(record actionrequests.Record) map[string]map[string]bool {
	calls := map[string]map[string]bool{}
	for _, action := range record.Actions {
		for _, request := range action.Requests {
			for _, symbol := range slices.Concat(action.Handlers, request.Sites) {
				name, _, _ := strings.Cut(symbol, ".")
				if calls[name] == nil {
					calls[name] = map[string]bool{}
				}
				for _, method := range request.SDKMethods {
					calls[name][method] = true
				}
			}
		}
	}
	return calls
}

// judge walks one option struct against one route's declared params.
func (c *AlwaysSentCheck) judge(endpoint bodyEndpoint, name string, types map[string]sdkOptionType, params map[string]apilive.Param) {
	walkOptionParams(types, name, "", nil, 0, func(found optionParam) {
		if !found.Field.Always || found.Field.Nested != "" {
			// A nested struct's own key is the object around the fields this
			// walk goes on to judge. Whether that object is written when it is
			// nil is a question about the parent's tag rather than about a
			// value GitLab was asked to accept, and the fields under it are
			// reported on their own terms.
			return
		}
		c.Fields++
		param, declared := lookupParam(params, found)
		switch {
		case !declared:
			c.Unmatched++
		case param.Required:
			c.Required++
		default:
			c.Optional = append(c.Optional, AlwaysSentField{
				Package:    endpoint.pkg,
				Method:     endpoint.method,
				Path:       endpoint.path,
				OptionType: found.Owner,
				Field:      found.Field.GoName,
				Param:      found.Param,
				ParamType:  param.Type,
			})
		}
	})
}

// lookupParam finds what a route declares about one param.
//
// A list of nested objects is tried under both spellings GitLab uses for one,
// since Grape declares the members of an array of hashes with an empty
// subscript and several routes flatten it away.
func lookupParam(params map[string]apilive.Param, found optionParam) (apilive.Param, bool) {
	if param, declared := params[found.Param]; declared {
		return param, true
	}
	if flattened := strings.ReplaceAll(found.Param, "[][", "["); flattened != found.Param {
		if param, declared := params[flattened]; declared {
			return param, true
		}
	}
	return apilive.Param{}, false
}

// bodyEndpoint is one recorded request reduced to what this check joins on.
type bodyEndpoint struct {
	pkg    string
	method string
	path   string
}

// bodyEndpoints lists the distinct recorded REST endpoints whose verb carries a
// body, one per package that reached it.
func bodyEndpoints(requests []requestinventory.Row) []bodyEndpoint {
	seen := map[bodyEndpoint]bool{}
	found := make([]bodyEndpoint, 0, len(requests))
	for _, request := range requests {
		if !strings.EqualFold(request.Kind, requestinventory.KindREST) || !bodyMethods[strings.ToUpper(request.Method)] {
			continue
		}
		endpoint := bodyEndpoint{pkg: request.Package, method: strings.ToUpper(request.Method), path: request.Path}
		if seen[endpoint] {
			continue
		}
		seen[endpoint] = true
		found = append(found, endpoint)
	}
	slices.SortFunc(found, func(left, right bodyEndpoint) int {
		return cmp.Or(
			strings.Compare(left.pkg, right.pkg),
			strings.Compare(left.path, right.path),
			strings.Compare(left.method, right.method),
		)
	})
	return found
}

// compareAlwaysSent orders two findings. Every field of the identity takes
// part, so the order is total and two runs over one tree print the same list.
func compareAlwaysSent(a, b AlwaysSentField) int {
	return cmp.Or(
		strings.Compare(a.Package, b.Package),
		strings.Compare(a.Path, b.Path),
		strings.Compare(a.Method, b.Method),
		strings.Compare(a.OptionType, b.OptionType),
		strings.Compare(a.Param, b.Param),
	)
}
