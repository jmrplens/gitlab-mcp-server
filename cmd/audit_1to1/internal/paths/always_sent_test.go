// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdkoptions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
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
	readOptions = sdkoptions.Read
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

// shareSource is pretend client-go source in which two service methods reach
// one route with different option structs, the shape issue 1104 was found on.
// GroupMembers.ShareWithGroup writes expires_at on every call and
// Groups.ShareGroupWithGroup omits every empty field, and both POST to
// groups/:id/share.
func shareSource(t *testing.T) string {
	t.Helper()
	return sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

var routeGroupsIDShare = route("groups/%s/share")
`,
		"options.go": `package gitlab

type ShareWithGroupOptions struct {
	GroupID     *int64  ` + "`" + `json:"group_id"` + "`" + `
	GroupAccess *int    ` + "`" + `json:"group_access"` + "`" + `
	ExpiresAt   *string ` + "`" + `json:"expires_at"` + "`" + `
}

type ShareGroupWithGroupOptions struct {
	GroupID     *int64  ` + "`" + `json:"group_id,omitempty"` + "`" + `
	GroupAccess *int    ` + "`" + `json:"group_access,omitempty"` + "`" + `
	ExpiresAt   *string ` + "`" + `json:"expires_at,omitempty"` + "`" + `
}
`,
		"service.go": `package gitlab

import "net/http"

func (s *GroupMembersService) ShareWithGroup(gid any, opt *ShareWithGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error) {
	return do[*Group](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDShare, GroupID{gid}),
		withAPIOpts(opt),
	)
}

func (s *GroupsService) ShareGroupWithGroup(gid any, opt *ShareGroupWithGroupOptions, options ...RequestOptionFunc) (*Group, *Response, error) {
	return do[*Group](s.client,
		withMethod(http.MethodPost),
		withPath(routeGroupsIDShare, GroupID{gid}),
		withAPIOpts(opt),
	)
}
`,
	})
}

// shareRecord is a live record holding the one route both methods reach,
// with the two params GitLab requires and the one it lets a caller leave out.
func shareRecord(t *testing.T) string {
	t.Helper()
	return recordIn(t, map[string]response{
		"POST /groups/:id/share": {
			Params:         []string{"expires_at"},
			ParamsRequired: []string{"group_id", "group_access"},
		},
	})
}

// writeActionRequests commits an action request record under root, the way
// cmd/gen_action_grants writes it.
func writeActionRequests(t *testing.T, root string, actions []actionrequests.RecordAction) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(actionrequests.RecordPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatalf("prepare the action request record: %v", err)
	}
	if err := os.WriteFile(target, actionrequests.RenderRecord(actions), 0o600); err != nil {
		t.Fatalf("write the action request record: %v", err)
	}
}

// shareRequests are the inventory rows of three packages reaching the share
// route: one whose handler calls Groups.ShareGroupWithGroup, one whose handler
// calls GroupMembers.ShareWithGroup, and one the action request record says
// nothing about, which stands for a body a handler builds itself.
func shareRequests() []requestinventory.Row {
	row := func(pkg string) requestinventory.Row {
		return requestinventory.Row{
			Package: pkg, Kind: "rest", Method: http.MethodPost,
			Path: "/groups/:group_id/share", Body: []string{"group_id", "group_access"},
		}
	}
	return []requestinventory.Row{
		row("internal/tools/groups"),
		row("internal/tools/groupmembers"),
		row("internal/tools/handbuilt"),
	}
}

// TestAlwaysSentCheck_TwoMethodsOnOneRoute_JudgeOnlyTheOptionsThePackageSends
// verifies the join issue 1104 asked for: a package is held to the option
// structs of the client-go methods its handlers call, not to every struct
// whose method reaches the route it was recorded on.
//
// POST /groups/:id/share is reached by two methods. A package whose handler
// calls Groups.ShareGroupWithGroup sends ShareGroupWithGroupOptions, whose
// every field is omitempty, so it writes nothing on every call and is no
// finding; the check used to charge it with the expires_at of
// ShareWithGroupOptions as well, a request it never sends. A package whose
// handler calls GroupMembers.ShareWithGroup keeps its finding. A package the
// action request record names no method of reaching the route, which is what
// a body built by hand looks like, is judged against both structs as before.
func TestAlwaysSentCheck_TwoMethodsOnOneRoute_JudgeOnlyTheOptionsThePackageSends(t *testing.T) {
	withSDKOptions(t, shareSource(t))
	root := shareRecord(t)
	writeActionRequests(t, root, []actionrequests.RecordAction{
		{
			ID:       "group.share_with_group",
			Handlers: []string{"groups.ShareGroupWithGroup"},
			Requests: []actionrequests.RecordRequest{{
				Kind: actionrequests.KindREST, Route: "POST /groups/:id/share", Class: actionrequests.ClassMandatory,
				Sites: []string{"groups.ShareGroupWithGroup"}, SDKMethods: []string{"Groups.ShareGroupWithGroup"},
			}},
			Paths: [][]int{{0}},
		},
		{
			ID:       "group.group_member_share",
			Handlers: []string{"groupmembers.ShareGroup"},
			Requests: []actionrequests.RecordRequest{{
				Kind: actionrequests.KindREST, Route: "POST /groups/:id/share", Class: actionrequests.ClassMandatory,
				Sites: []string{"groupmembers.ShareGroup"}, SDKMethods: []string{"GroupMembers.ShareWithGroup"},
			}},
			Paths: [][]int{{0}},
		},
	})

	check := alwaysSentCheck(root, shareRequests())

	if !check.Ran || check.Endpoints != 3 || check.OptionTypes != 2 {
		t.Fatalf("check = %+v, want three endpoints over the two option types", check)
	}
	if check.Fields != 6 || check.Required != 4 || check.Unmatched != 0 {
		t.Errorf("fields = %d, required = %d, unmatched = %d, want 6, 4 and 0: three fields each for the two packages held to ShareWithGroupOptions",
			check.Fields, check.Required, check.Unmatched)
	}
	want := []AlwaysSentField{
		{Package: "internal/tools/groupmembers", Method: http.MethodPost, Path: "/groups/:group_id/share", OptionType: "ShareWithGroupOptions", Field: "ExpiresAt", Param: "expires_at", ParamType: "String", Encoder: "json"},
		{Package: "internal/tools/handbuilt", Method: http.MethodPost, Path: "/groups/:group_id/share", OptionType: "ShareWithGroupOptions", Field: "ExpiresAt", Param: "expires_at", ParamType: "String", Encoder: "json"},
	}
	if !reflect.DeepEqual(check.Optional, want) {
		t.Errorf("findings = %+v, want %+v: none for the package that sends ShareGroupWithGroupOptions", check.Optional, want)
	}
	if check.Requests != actionrequests.RecordPath {
		t.Errorf("requests record = %q, want %q named as what narrowed the join", check.Requests, actionrequests.RecordPath)
	}
	wantUnnarrowed := []AlwaysSentEndpoint{{
		Package: "internal/tools/handbuilt", Method: http.MethodPost, Path: "/groups/:group_id/share",
		OptionTypes: []string{"ShareGroupWithGroupOptions", "ShareWithGroupOptions"},
	}}
	if !reflect.DeepEqual(check.NotNarrowed, wantUnnarrowed) {
		t.Errorf("not narrowed = %+v, want %+v: only the package the record names no method of", check.NotNarrowed, wantUnnarrowed)
	}
}

// TestAlwaysSentCheck_NoActionRequestRecord_JudgesEveryEndpointUnnarrowed
// verifies that a tree whose action request record cannot be read still runs
// the check, as it ran before the record was joined, and says on every
// endpoint that it did: each is asked about under every struct its route
// accepts, the phantom of issue 1104 included, and listed as not narrowed.
func TestAlwaysSentCheck_NoActionRequestRecord_JudgesEveryEndpointUnnarrowed(t *testing.T) {
	withSDKOptions(t, shareSource(t))

	check := alwaysSentCheck(shareRecord(t), shareRequests())

	if !check.Ran || check.Requests != "" {
		t.Fatalf("check = %+v, want a run that names no requests record", check)
	}
	if len(check.Optional) != 3 || len(check.NotNarrowed) != 3 {
		t.Errorf("findings = %+v, not narrowed = %+v, want three of each: every package held to both structs", check.Optional, check.NotNarrowed)
	}
}

// TestOptionTypesByRoute_AMethodGivenAStruct_TakesItToEachRouteItSends
// verifies the index the join is cut from: an endpoint is keyed by its verb
// and shape, holds each option struct a method reaching it takes with the
// methods that take it there, and a route no method takes a struct to has
// no entry.
func TestOptionTypesByRoute_AMethodGivenAStruct_TakesItToEachRouteItSends(t *testing.T) {
	byRoute := optionTypesByRoute(sdkoptions.Read(alwaysSentSource(t)))

	want := map[string]routeTakers{
		"PATCH /projects/:/packages/protection/rules/:": {"UpdateRulesOptions": {"ProtectedPackages.UpdateRules": true}},
	}
	if !reflect.DeepEqual(byRoute, want) {
		t.Errorf("byRoute = %+v, want %+v", byRoute, want)
	}
}

// TestRouteTakers_TheStructsAPackageCanSend_AreTheOnesItsMethodsTake verifies
// the narrowing issue 1104 asked for at its smallest: of the option structs a
// route accepts, a package is held to those a method it calls takes there,
// one method taking two structs and two methods taking one struct included,
// and to none when it calls no method that reaches the route.
func TestRouteTakers_TheStructsAPackageCanSend_AreTheOnesItsMethodsTake(t *testing.T) {
	takers := routeTakers{
		"AOptions": {"Svc.TakesAAndB": true},
		"BOptions": {"Svc.TakesAAndB": true, "Svc.TakesB": true},
		"COptions": {"Svc.TakesC": true},
	}
	if got := takers.optionTypes(); !reflect.DeepEqual(got, []string{"AOptions", "BOptions", "COptions"}) {
		t.Errorf("optionTypes() = %v, want every struct the route accepts, sorted", got)
	}

	tests := []struct {
		name   string
		called map[string]bool
		want   []string
	}{
		{name: "a method taking two structs", called: map[string]bool{"Svc.TakesAAndB": true}, want: []string{"AOptions", "BOptions"}},
		{name: "one of two methods taking a struct", called: map[string]bool{"Svc.TakesB": true}, want: []string{"BOptions"}},
		{name: "methods taking different structs", called: map[string]bool{"Svc.TakesB": true, "Svc.TakesC": true}, want: []string{"BOptions", "COptions"}},
		{name: "a method recorded as not called", called: map[string]bool{"Svc.TakesC": false}},
		{name: "a method that does not reach the route", called: map[string]bool{"Svc.Elsewhere": true}},
		{name: "no method at all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := takers.takenBy(tt.called); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("takenBy(%v) = %v, want %v", tt.called, got, tt.want)
			}
		})
	}
}

// TestRouteTakers_AStructTwoMethodsTake_IsJudgedByEveryEncoderEitherUses
// verifies the encoders a struct is judged by on one endpoint: those of
// every method that takes it there, each once, so a struct one method sends
// as a body and another uploads is held to both tags.
func TestRouteTakers_AStructTwoMethodsTake_IsJudgedByEveryEncoderEitherUses(t *testing.T) {
	takers := routeTakers{"AOptions": {"Svc.Body": true, "Svc.Upload": true, "Svc.Both": true}}
	methods := map[string]sdkoptions.Method{
		"Svc.Body":   {Constructors: []string{"NewRequest"}},
		"Svc.Upload": {Constructors: []string{"UploadRequest"}},
		"Svc.Both":   {Constructors: []string{"NewRequest", "UploadRequest"}},
	}
	if got := takers.encoders("AOptions", http.MethodPost, methods); !reflect.DeepEqual(got, []string{"json", "url"}) {
		t.Errorf("encoders(POST) = %v, want [json url]", got)
	}
	if got := takers.encoders("AOptions", http.MethodGet, methods); !reflect.DeepEqual(got, []string{"url"}) {
		t.Errorf("encoders(GET) = %v, want [url]", got)
	}
	if got := takers.encoders("BOptions", http.MethodPost, methods); got != nil {
		t.Errorf("encoders(a struct no method takes) = %v, want none", got)
	}
}

// TestPackageMethods_ARequest_IsCreditedToItsSitesAndItsHandlers verifies the
// attribution the narrowing rests on. The inventory files a request under the
// package whose test drove it, which is the handler's package when a guided
// flow calls another domain's function and the site's package when a shared
// helper is tested on its own, so a request's methods are credited to both;
// a symbol with no qualifier is credited to a name no package has.
func TestPackageMethods_ARequest_IsCreditedToItsSitesAndItsHandlers(t *testing.T) {
	calls := packageMethods(actionrequests.Record{Actions: []actionrequests.RecordAction{
		{
			ID:       "interactive.issue_create",
			Handlers: []string{"elicitationtools.IssueCreate"},
			Requests: []actionrequests.RecordRequest{{
				Kind: actionrequests.KindREST, Sites: []string{"issues.Create"}, SDKMethods: []string{"Issues.CreateIssue"},
			}},
		},
		{
			ID:       "merge_request.draft_note_publish",
			Handlers: []string{"mrdraftnotes.Publish"},
			Requests: []actionrequests.RecordRequest{
				{Kind: actionrequests.KindREST, Sites: []string{"toolutil.Publish"}, SDKMethods: []string{"DraftNotes.PublishAllDraftNotes"}},
				{Kind: actionrequests.KindREST, Sites: []string{"unqualified"}},
			},
		},
	}})

	want := map[string]map[string]bool{
		"elicitationtools": {"Issues.CreateIssue": true},
		"issues":           {"Issues.CreateIssue": true},
		"mrdraftnotes":     {"DraftNotes.PublishAllDraftNotes": true},
		"toolutil":         {"DraftNotes.PublishAllDraftNotes": true},
		"unqualified":      {},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("packageMethods = %v, want %v", calls, want)
	}
}

// verbSource is pretend client-go source with one request per encoding
// NewRequest and UploadRequest choose between: a GET and a DELETE, whose
// options NewRequest writes as a query, and a POST built as an upload, whose
// options UploadRequest writes as multipart fields. Each struct tags a field
// to be written always under the url tag and omitted under the json one, and
// the GET's struct another the other way round.
func verbSource(t *testing.T) string {
	t.Helper()
	return sdkSourceIn(t, map[string]string{
		"routes.go": `package gitlab

var (
	routeRules       = route("projects/%s/rules")
	routeRulesID     = route("projects/%s/rules/%d")
	routeRulesUpload = route("projects/%s/rules/upload")
)
`,
		"options.go": `package gitlab

type ListRulesOptions struct {
	PackageNamePattern *string ` + "`" + `url:"package_name_pattern" json:"package_name_pattern,omitempty"` + "`" + `
	Name               *string ` + "`" + `url:"name,omitempty" json:"name"` + "`" + `
}

type DeleteRuleOptions struct {
	FullPath *string ` + "`" + `url:"full_path" json:"full_path,omitempty"` + "`" + `
}

type UploadRuleOptions struct {
	Title *string ` + "`" + `url:"title" json:"title,omitempty"` + "`" + `
}
`,
		"service.go": `package gitlab

import "net/http"

func (s *RulesService) ListRules(pid any, opt *ListRulesOptions, options ...RequestOptionFunc) ([]*Rule, *Response, error) {
	return do[[]*Rule](s.client, withPath(routeRules, ProjectID{pid}), withAPIOpts(opt))
}

func (s *RulesService) DeleteRule(pid any, id int64, opt *DeleteRuleOptions, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client, withMethod(http.MethodDelete), withPath(routeRulesID, ProjectID{pid}, id), withAPIOpts(opt))
	return resp, err
}

func (s *RulesService) UploadRule(pid any, opt *UploadRuleOptions, options ...RequestOptionFunc) (*Rule, *Response, error) {
	return do[*Rule](s.client, withMethod(http.MethodPost), withPath(routeRulesUpload, ProjectID{pid}), withUpload(nil, "rule", UploadFile), withAPIOpts(opt))
}
`,
	})
}

// TestAlwaysSentCheck_EachRequest_IsJudgedByTheTagItsEncoderReads verifies
// that the rule reads what the request carries, not the json tag alone.
//
// NewRequest writes a GET's and a DELETE's options as a query through
// go-querystring, which reads the url tag and writes `name=` for a nil
// pointer that carries no omitempty, and UploadRequest writes a POST's as
// multipart fields through the same encoder. The rule used to read the json
// tag for every body verb, DELETE included, and to pass over a GET on the
// premise that a query leaves a nil pointer out whatever the tag says: it
// reported the GET's name, which the query omits, and missed the three keys
// each of these requests writes on every call.
func TestAlwaysSentCheck_EachRequest_IsJudgedByTheTagItsEncoderReads(t *testing.T) {
	withSDKOptions(t, verbSource(t))
	root := recordIn(t, map[string]response{
		"GET /projects/:id/rules":         {Params: []string{"package_name_pattern", "name"}},
		"DELETE /projects/:id/rules/:id2": {Params: []string{"full_path"}},
		"POST /projects/:id/rules/upload": {Params: []string{"title"}},
	})
	row := func(method, path string) requestinventory.Row {
		return requestinventory.Row{Package: "internal/tools/rules", Kind: "rest", Method: method, Path: path}
	}

	check := alwaysSentCheck(root, []requestinventory.Row{
		row(http.MethodGet, "/projects/:project_id/rules"),
		row(http.MethodDelete, "/projects/:project_id/rules/:rule_id"),
		row(http.MethodPost, "/projects/:project_id/rules/upload"),
	})

	if !check.Ran || check.Endpoints != 3 || check.Fields != 3 {
		t.Fatalf("check = %+v, want three endpoints writing one key each on every call", check)
	}
	finding := func(method, path, optionType, field, param string) AlwaysSentField {
		return AlwaysSentField{
			Package: "internal/tools/rules", Method: method, Path: path, OptionType: optionType,
			Field: field, Param: param, ParamType: "String", Encoder: "url",
		}
	}
	want := []AlwaysSentField{
		finding(http.MethodGet, "/projects/:project_id/rules", "ListRulesOptions", "PackageNamePattern", "package_name_pattern"),
		finding(http.MethodDelete, "/projects/:project_id/rules/:rule_id", "DeleteRuleOptions", "FullPath", "full_path"),
		finding(http.MethodPost, "/projects/:project_id/rules/upload", "UploadRuleOptions", "Title", "title"),
	}
	if !reflect.DeepEqual(check.Optional, want) {
		t.Errorf("findings =\n%+v\nwant\n%+v", check.Optional, want)
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
		readOptions = func(string) sdkoptions.SDK { return sdkoptions.Read(dir) }
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
		readOptions = func(dir string) sdkoptions.SDK {
			t.Errorf("readOptions(%q) was called, want no read of a module the load did not find", dir)
			return sdkoptions.SDK{}
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

// TestRESTEndpoints_RecordedRows_AreDeduplicatedPerPackage verifies the join's
// own input: one endpoint a package reached twenty times is asked about once,
// two packages reaching it are two questions, since a finding names the
// package whose handler built the request, and a GET is asked about beside
// the POST, since its query writes the url tag's always-sent keys as well.
func TestRESTEndpoints_RecordedRows_AreDeduplicatedPerPackage(t *testing.T) {
	endpoints := restEndpoints([]requestinventory.Row{
		{Package: "internal/tools/issues", Kind: "rest", Method: "post", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/groups", Kind: "rest", Method: "POST", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "GET", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/epics", Kind: "graphql", Method: "POST", Path: "/graphql", Operation: "mutation createEpic"},
	})

	want := []restEndpoint{
		{pkg: "internal/tools/groups", method: "POST", path: "/projects/:project_id/issues"},
		{pkg: "internal/tools/issues", method: "GET", path: "/projects/:project_id/issues"},
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

// TestRESTEndpoints_AreOrderedByPackagePathAndMethod verifies the order the
// join asks its questions in, which is the order a report lists them in:
// by package, then path, then method, whatever order the inventory recorded
// them in.
func TestRESTEndpoints_AreOrderedByPackagePathAndMethod(t *testing.T) {
	endpoints := restEndpoints([]requestinventory.Row{
		{Package: "internal/tools/issues", Kind: "rest", Method: "PUT", Path: "/projects/:project_id/issues/:issue_id"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/projects/:project_id/issues/:issue_id"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/groups", Kind: "rest", Method: "POST", Path: "/groups"},
	})

	want := []restEndpoint{
		{pkg: "internal/tools/groups", method: "POST", path: "/groups"},
		{pkg: "internal/tools/issues", method: "POST", path: "/projects/:project_id/issues"},
		{pkg: "internal/tools/issues", method: "POST", path: "/projects/:project_id/issues/:issue_id"},
		{pkg: "internal/tools/issues", method: "PUT", path: "/projects/:project_id/issues/:issue_id"},
	}
	if !reflect.DeepEqual(endpoints, want) {
		t.Errorf("endpoints = %+v, want %+v", endpoints, want)
	}
}

// TestRESTEndpoints_OnePackage_IsOrderedByPathThenMethod verifies the order
// within a package, from rows recorded the other way round so that every
// comparison of the sort is asked in both directions.
func TestRESTEndpoints_OnePackage_IsOrderedByPathThenMethod(t *testing.T) {
	endpoints := restEndpoints([]requestinventory.Row{
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/b"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "PUT", Path: "/a"},
		{Package: "internal/tools/issues", Kind: "rest", Method: "POST", Path: "/a"},
	})

	want := []restEndpoint{
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
	base := AlwaysSentField{Package: "internal/tools/x", Path: "/a", Method: "PUT", OptionType: "XOptions", Param: "b", Encoder: "json"}
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
		{name: "encoder", later: func(f *AlwaysSentField) { f.Encoder = "url" }},
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
			param, declared := lookupParam(params, sdkoptions.Param{Param: tt.param})
			if declared != tt.declared || param.Required != tt.required {
				t.Errorf("lookupParam(%q) = %+v, %t, want required=%t declared=%t", tt.param, param, declared, tt.required, tt.declared)
			}
		})
	}
}
