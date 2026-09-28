package paths

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// withSDKOptions points the always-sent check at a fixture directory of pretend
// client-go source, restoring the real readers afterwards.
//
// Both seams have to move together: the directory comes out of a twenty-second
// load of the tool packages, and what is in it comes out of a module cache.
func withSDKOptions(t *testing.T, dir string) {
	t.Helper()
	originalPairings, originalOptions := collectPairings, readOptions
	collectPairings = func(string) (structs.Pairings, error) {
		return structs.Pairings{ClientGoDir: dir}, nil
	}
	readOptions = readSDKOptions
	t.Cleanup(func() { collectPairings, readOptions = originalPairings, originalOptions })
}

// alwaysSentSource is one directory of pretend client-go source carrying the
// three shapes this rule has to tell apart on one route: a field written
// unconditionally that GitLab requires, one it does not, and one that is
// omitted when empty.
func alwaysSentSource(t *testing.T) string {
	t.Helper()
	return sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

var routeProjectsIDRulesID = route("projects/%s/packages/protection/rules/%d")
`,
		"options.go": `package gitlab

type UpdateRulesOptions struct {
	PackageNamePattern *string ` + "`" + `json:"package_name_pattern"` + "`" + `
	PackageType        *string ` + "`" + `json:"package_type"` + "`" + `
	MinimumAccessLevel *string ` + "`" + `json:"minimum_access_level,omitempty"` + "`" + `
	Unknown            *string ` + "`" + `json:"a_param_the_route_never_declares"` + "`" + `
}
`,
		"service.go": `package gitlab

import "net/http"

func (s *ProtectedPackagesService) UpdateRules(pid any, id int64, opt *UpdateRulesOptions, options ...RequestOptionFunc) (*Rule, *Response, error) {
	return do[*Rule](s.client,
		withMethod(http.MethodPatch),
		withPath(routeProjectsIDRulesID, ProjectID{pid}, id),
		withAPIOpts(opt),
	)
}
`,
	})
}

// TestAlwaysSentCheck_AnOptionalParamWrittenOnEveryCall_IsAFinding verifies the
// line this rule draws, on the defect it was built against.
//
// client-go's update options for a package protection rule declare
// package_name_pattern and package_type without omitempty, so encoding/json
// writes both keys whatever the handler filled in and a caller changing only
// the access level sends GitLab a null pattern. GitLab marks them optional on
// the PATCH, which is what makes that wrong; the same fields are required on
// the POST beside it, where writing them always is correct. Nothing else in
// this audit can see either half: the surface is perfect and the inventory
// records that a name was sent, never what was in it.
//
// The three benign outcomes are asserted beside the finding because the rule is
// worth nothing if it cannot separate them. A required param is correct to send
// always; an omitempty field is not sent at all; and a param the route declares
// nothing about is a comparison that could not be made rather than a finding.
func TestAlwaysSentCheck_AnOptionalParamWrittenOnEveryCall_IsAFinding(t *testing.T) {
	withSDKOptions(t, alwaysSentSource(t))
	root := recordIn(t, map[string]response{
		"PATCH /projects/:id/packages/protection/rules/:package_protection_rule_id": {
			Params:         []string{"package_name_pattern", "minimum_access_level"},
			ParamsRequired: []string{"package_type"},
		},
	})

	check := alwaysSentCheck(root, []requestinventory.Row{{
		Package: "internal/tools/protectedpackages", Kind: "rest", Method: "PATCH",
		Path: "/projects/:project_id/packages/protection/rules/:rule_id",
		Body: []string{"package_name_pattern", "package_type"},
	}})

	if !check.Ran || check.Endpoints != 1 || check.OptionTypes != 1 {
		t.Fatalf("check = %+v, want one endpoint and one option type", check)
	}
	if check.Fields != 3 || check.Required != 1 || check.Unmatched != 1 {
		t.Errorf("fields = %d, required = %d, unmatched = %d, want 3, 1 and 1", check.Fields, check.Required, check.Unmatched)
	}
	if len(check.Optional) != 1 {
		t.Fatalf("findings = %+v, want exactly the optional param", check.Optional)
	}
	found := check.Optional[0]
	if found.Param != "package_name_pattern" || found.Field != "PackageNamePattern" || found.OptionType != "UpdateRulesOptions" {
		t.Errorf("finding = %+v, want UpdateRulesOptions.PackageNamePattern as package_name_pattern", found)
	}
	if found.Package != "internal/tools/protectedpackages" || found.Method != http.MethodPatch {
		t.Errorf("finding = %+v, want it attributed to the package the inventory recorded", found)
	}
}

// TestAlwaysSentCheck_AGetEndpoint_IsNeverJudged verifies the one restriction
// that keeps the rule truthful about what it reads. A GET carries its options
// in the query string, where client-go's encoder leaves a nil pointer out
// whatever the json tag says, so judging one would report a field nobody sends.
func TestAlwaysSentCheck_AGetEndpoint_IsNeverJudged(t *testing.T) {
	withSDKOptions(t, alwaysSentSource(t))
	root := recordIn(t, map[string]response{
		"GET /projects/:id/packages/protection/rules/:package_protection_rule_id": {
			Params: []string{"package_name_pattern"},
		},
	})

	check := alwaysSentCheck(root, []requestinventory.Row{{
		Package: "internal/tools/protectedpackages", Kind: "rest", Method: "GET",
		Path: "/projects/:project_id/packages/protection/rules/:rule_id",
	}})

	if check.Endpoints != 0 || len(check.Optional) != 0 {
		t.Errorf("check = %+v, want a GET to be passed over", check)
	}
}

// TestAlwaysSentCheck_ARouteDeclaringNoParams_IsNotJudged verifies that a route
// the record holds and declares nothing on is passed over rather than asked
// about. With no param to hold a field against, every field of the option
// struct would read as a comparison that could not be made, and the endpoint
// would be counted as judged when nothing about it was.
func TestAlwaysSentCheck_ARouteDeclaringNoParams_IsNotJudged(t *testing.T) {
	withSDKOptions(t, alwaysSentSource(t))
	root := recordIn(t, map[string]response{
		"PATCH /projects/:id/packages/protection/rules/:package_protection_rule_id": {Response: []string{"id"}},
	})

	check := alwaysSentCheck(root, []requestinventory.Row{{
		Package: "internal/tools/protectedpackages", Kind: "rest", Method: "PATCH",
		Path: "/projects/:project_id/packages/protection/rules/:rule_id",
		Body: []string{"package_name_pattern"},
	}})

	if !check.Ran || check.Endpoints != 0 || check.Fields != 0 || len(check.Optional) != 0 {
		t.Errorf("check = %+v, want a run that judged no endpoint", check)
	}
}

// TestAlwaysSentCheck_MissingInputs_SayTheCheckDidNotRun verifies that each way
// of having nothing to compare renders as a check that never ran rather than as
// a clean one.
//
// The difference is the whole value of the section. A reader told "no findings"
// by a run that never opened client-go is worse off than one told nothing, and
// all three of these render identically in the finding list.
func TestAlwaysSentCheck_MissingInputs_SayTheCheckDidNotRun(t *testing.T) {
	requests := []requestinventory.Row{{
		Package: "internal/tools/protectedpackages", Kind: "rest", Method: "PATCH",
		Path: "/projects/:project_id/packages/protection/rules/:rule_id",
	}}

	t.Run("no live record", func(t *testing.T) {
		withSDKOptions(t, alwaysSentSource(t))
		if check := alwaysSentCheck(t.TempDir(), requests); check.Ran {
			t.Errorf("check = %+v, want ran=false with no record on disk", check)
		}
	})

	t.Run("no client-go directory", func(t *testing.T) {
		dir := alwaysSentSource(t)
		withSDKOptions(t, dir)
		original := collectPairings
		// The failed load still names a directory holding option structs, so
		// only the error can be what stops the check.
		collectPairings = func(string) (structs.Pairings, error) {
			return structs.Pairings{ClientGoDir: dir}, errors.New("no load")
		}
		t.Cleanup(func() { collectPairings = original })

		root := recordIn(t, map[string]response{"PATCH /projects/:id": {Params: []string{"name"}}})
		if check := alwaysSentCheck(root, requests); check.Ran {
			t.Errorf("check = %+v, want ran=false when the load failed", check)
		}
	})

	t.Run("a load that names no client-go directory", func(t *testing.T) {
		dir := alwaysSentSource(t)
		withSDKOptions(t, dir)
		original, originalOptions := collectPairings, readOptions
		collectPairings = func(string) (structs.Pairings, error) { return structs.Pairings{}, nil }
		// A reader that would find option structs wherever it was pointed, so
		// only the empty directory can be what stops the check.
		readOptions = func(string) sdkOptions { return readSDKOptions(dir) }
		t.Cleanup(func() { collectPairings, readOptions = original, originalOptions })

		root := recordIn(t, map[string]response{"PATCH /projects/:id": {Params: []string{"name"}}})
		if check := alwaysSentCheck(root, requests); check.Ran {
			t.Errorf("check = %+v, want ran=false when the load found no client-go module", check)
		}
	})

	t.Run("a client-go directory holding no option struct", func(t *testing.T) {
		withSDKOptions(t, t.TempDir())
		root := recordIn(t, map[string]response{"PATCH /projects/:id": {Params: []string{"name"}}})
		if check := alwaysSentCheck(root, requests); check.Ran {
			t.Errorf("check = %+v, want ran=false when the source held nothing", check)
		}
	})

	t.Run("a load that found no client-go module", func(t *testing.T) {
		withSDKOptions(t, alwaysSentSource(t))
		original := collectPairings
		collectPairings = func(string) (structs.Pairings, error) { return structs.Pairings{}, nil }
		readOptions = func(dir string) sdkOptions {
			t.Errorf("readOptions(%q) was called, want no read of a module the load did not find", dir)
			return sdkOptions{}
		}
		t.Cleanup(func() { collectPairings = original })

		root := recordIn(t, map[string]response{"PATCH /projects/:id": {Params: []string{"name"}}})
		if check := alwaysSentCheck(root, requests); check.Ran {
			t.Errorf("check = %+v, want ran=false when the load named no client-go directory", check)
		}
	})
}

// TestAlwaysSentCheck_ARouteDeclaringNoParam_IsNotAskedAbout verifies that an
// endpoint whose route the record matches and whose params it declares none of
// is passed over rather than walked: every field of its option struct would
// read as a comparison that could not be made, which is a count about the
// record and not about the endpoint.
func TestAlwaysSentCheck_ARouteDeclaringNoParam_IsNotAskedAbout(t *testing.T) {
	withSDKOptions(t, alwaysSentSource(t))
	root := recordIn(t, map[string]response{
		"PATCH /projects/:id/packages/protection/rules/:package_protection_rule_id": {},
	})

	check := alwaysSentCheck(root, []requestinventory.Row{{
		Package: "internal/tools/protectedpackages", Kind: "rest", Method: "PATCH",
		Path: "/projects/:project_id/packages/protection/rules/:rule_id",
	}})

	if !check.Ran || check.Endpoints != 0 || check.Fields != 0 || check.Unmatched != 0 || check.OptionTypes != 0 {
		t.Errorf("check = %+v, want it to run and ask nothing of a route that declares no param", check)
	}
}

// TestBodyEndpoints_RecordedRows_AreDeduplicatedPerPackage verifies the join's
// own input: one endpoint a package reached twenty times is asked about once,
// and two packages reaching it are two questions, since a finding names the
// package whose handler built the request.
func TestBodyEndpoints_RecordedRows_AreDeduplicatedPerPackage(t *testing.T) {
	endpoints := bodyEndpoints([]requestinventory.Row{
		{Package: "internal/tools/issues", Kind: "rest", Method: "post", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/groups", Kind: "rest", Method: "POST", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "GET", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/epics", Kind: "graphql", Method: "POST", Path: "/graphql", Operation: "mutation createEpic"},
	})

	want := []bodyEndpoint{
		{pkg: "internal/tools/groups", method: "POST", path: "/projects/:project_id/issues"},
		{pkg: "internal/tools/issues", method: "POST", path: "/projects/:project_id/issues"},
	}
	if len(endpoints) != len(want) {
		t.Fatalf("endpoints = %+v, want %+v", endpoints, want)
	}
	for i, got := range endpoints {
		if got != want[i] {
			t.Errorf("endpoint %d = %+v, want %+v", i, got, want[i])
		}
	}
}

// TestBodyEndpoints_AreOrderedByPackagePathAndMethod verifies the order the
// join asks its questions in, which is the order a report lists them in:
// by package, then path, then method, whatever order the inventory recorded
// them in.
func TestBodyEndpoints_AreOrderedByPackagePathAndMethod(t *testing.T) {
	endpoints := bodyEndpoints([]requestinventory.Row{
		{Package: "internal/tools/issues", Kind: "rest", Method: "PUT", Path: "/projects/:project_id/issues/:issue_id"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/projects/:project_id/issues/:issue_id"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/groups", Kind: "rest", Method: "POST", Path: "/groups"},
	})

	want := []bodyEndpoint{
		{pkg: "internal/tools/groups", method: "POST", path: "/groups"},
		{pkg: "internal/tools/issues", method: "POST", path: "/projects/:project_id/issues"},
		{pkg: "internal/tools/issues", method: "POST", path: "/projects/:project_id/issues/:issue_id"},
		{pkg: "internal/tools/issues", method: "PUT", path: "/projects/:project_id/issues/:issue_id"},
	}
	if !reflect.DeepEqual(endpoints, want) {
		t.Errorf("endpoints = %+v, want %+v", endpoints, want)
	}
}

// TestBodyEndpoints_OnePackage_IsOrderedByPathThenMethod verifies the order
// within a package, from rows recorded the other way round so that every
// comparison of the sort is asked in both directions.
func TestBodyEndpoints_OnePackage_IsOrderedByPathThenMethod(t *testing.T) {
	endpoints := bodyEndpoints([]requestinventory.Row{
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/b"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "PUT", Path: "/a"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/a"},
	})

	want := []bodyEndpoint{
		{pkg: "internal/tools/issues", method: "POST", path: "/a"},
		{pkg: "internal/tools/issues", method: "PUT", path: "/a"},
		{pkg: "internal/tools/issues", method: "POST", path: "/b"},
	}
	if !reflect.DeepEqual(endpoints, want) {
		t.Errorf("endpoints = %+v, want %+v", endpoints, want)
	}
}

// TestCompareAlwaysSent_IsATotalOrder verifies the finding order compares
// every field of the identity in turn, and holds a finding equal only to
// itself, which is what slices.SortFunc asks of a comparison and what keeps two
// runs over one tree printing one list.
//
// Each case differs from the base in one field alone and every field before it
// is equal, so a comparison that stopped early, skipped a field or read one
// backwards answers zero or the wrong sign for exactly that case.
func TestCompareAlwaysSent_IsATotalOrder(t *testing.T) {
	base := AlwaysSentField{Package: "internal/tools/x", Path: "/a", Method: "PUT", OptionType: "XOptions", Param: "b"}
	if got := compareAlwaysSent(base, base); got != 0 {
		t.Errorf("compareAlwaysSent(f, f) = %d, want 0", got)
	}
	tests := []struct {
		name  string
		later func(*AlwaysSentField)
	}{
		{name: "package", later: func(f *AlwaysSentField) { f.Package = "internal/tools/y" }},
		{name: "path", later: func(f *AlwaysSentField) { f.Path = "/b" }},
		{name: "method", later: func(f *AlwaysSentField) { f.Method = "PUTX" }},
		{name: "option type", later: func(f *AlwaysSentField) { f.OptionType = "YOptions" }},
		{name: "param", later: func(f *AlwaysSentField) { f.Param = "c" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			later := base
			tt.later(&later)
			if compareAlwaysSent(base, later) >= 0 || compareAlwaysSent(later, base) <= 0 {
				t.Errorf("compareAlwaysSent did not order two findings that differ only in the %s", tt.name)
			}
		})
	}
}

// TestLookupParam_AListOfObjects_IsTriedUnderBothSpellings verifies the one
// accommodation the param naming makes. Grape declares the members of an array
// of hashes with an empty subscript between the parent and the key, and several
// routes flatten that away, so both spellings are tried and a name matching
// neither is an unanswered comparison rather than a finding.
func TestLookupParam_AListOfObjects_IsTriedUnderBothSpellings(t *testing.T) {
	params := map[string]apilive.Param{
		"actions[][action]": {Required: true, Type: "String"},
		"position[x]":       {Type: "Integer"},
	}

	tests := []struct {
		name     string
		param    string
		declared bool
		required bool
	}{
		{name: "the subscripted spelling", param: "actions[][action]", declared: true, required: true},
		{name: "the flattened spelling of a declared param", param: "position[][x]", declared: true},
		{name: "a plain nested param", param: "position[x]", declared: true},
		{name: "a param the route never declares", param: "position[y]"},
		{name: "a list member the route declares under neither spelling", param: "position[][y]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param, declared := lookupParam(params, optionParam{Param: tt.param})
			if declared != tt.declared || param.Required != tt.required {
				t.Errorf("lookupParam(%q) = %+v, %t, want required=%t declared=%t", tt.param, param, declared, tt.required, tt.declared)
			}
		})
	}
}
