package paths

import (
	"net/http"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// AlwaysSentCheck is the other half of R-PATH's blind spot about values: the
// inventory records which field names a body carried and nothing about what was
// in them, so no rule here could see a field this server sends as null on every
// call.
//
// The defect it was built against is `package_protection_rule.update`.
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
// The join is the endpoint rather than the action: the inventory says which
// package reached which route, the SDK says which option struct reaches that
// route, and a route reached by two option structs is asked about under both.
// That can name an option struct a given package never passes, which costs a
// finding that is about the SDK rather than about that package's handler, and
// the endpoint is on every finding so a reader can see which.
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

	walked := map[string]bool{}
	for _, endpoint := range bodyEndpoints(requests) {
		operation, quality, _ := index.lookup(endpoint.method, endpoint.path)
		if quality == matchNone || len(operation.Params) == 0 {
			continue
		}
		types := byRoute[endpoint.method+" "+pathShape(endpoint.path)]
		if len(types) == 0 {
			continue
		}
		check.Endpoints++
		for _, name := range types {
			walked[name] = true
			check.judge(endpoint, name, options.Types, operation.Params)
		}
	}
	check.OptionTypes = len(walked)
	sort.Slice(check.Optional, func(i, j int) bool { return lessAlwaysSent(check.Optional[i], check.Optional[j]) })
	return check
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
	sort.Slice(found, func(i, j int) bool {
		left, right := found[i], found[j]
		if left.pkg != right.pkg {
			return left.pkg < right.pkg
		}
		if left.path != right.path {
			return left.path < right.path
		}
		return left.method < right.method
	})
	return found
}

// lessAlwaysSent orders two findings. Every field of the identity takes part,
// so the order is total and two runs over one tree print the same list.
func lessAlwaysSent(a, b AlwaysSentField) bool {
	left := []string{a.Package, a.Path, a.Method, a.OptionType, a.Param}
	right := []string{b.Package, b.Path, b.Method, b.OptionType, b.Param}
	for i := range left {
		if left[i] != right[i] {
			return left[i] < right[i]
		}
	}
	return false
}
