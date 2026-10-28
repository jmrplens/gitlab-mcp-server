// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdkoptions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// tick spells a backquote, so a struct tag can sit inside a raw string.
const tick = "`"

// mergedAfterField is where the stand-in client-go grows the filter the
// "v3.38.0" state models; the "v3.30.0" state leaves it out.
const mergedAfterField = "\t// MERGED_AFTER\n"

// standInSDK is a stand-in for client-go's root package: a list whose
// options lack a filter GitLab declares, a create that uploads its avatar
// when one is given, an upload that always uploads, and the hook options a
// raw request carries.
const standInSDK = `package gitlab

import "net/http"

var (
	routeWidgets      = route("projects/%s/widgets")
	routeWidgetAvatar = route("projects/%s/widgets/avatar")
)

type UploadType string

const UploadAvatar UploadType = "avatar"

type ListOptions struct {
	Page *int64 ` + tick + `url:"page,omitempty" json:"page,omitempty"` + tick + `
}

type ListWidgetsOptions struct {
	ListOptions
	Search         *string   ` + tick + `url:"search,omitempty" json:"search,omitempty"` + tick + `
	Labels         *[]string ` + tick + `url:"labels[],omitempty" json:"labels,omitempty"` + tick + `
	NotLabels      *[]string ` + tick + `url:"not[labels],omitempty" json:"not[labels],omitempty"` + tick + `
` + mergedAfterField + `}

type WidgetHeader struct {
	Key   string ` + tick + `json:"key"` + tick + `
	Value string ` + tick + `json:"value"` + tick + `
}

type CreateWidgetOptions struct {
	Name    *string          ` + tick + `url:"name,omitempty" json:"name,omitempty"` + tick + `
	Avatar  *string          ` + tick + `url:"-" json:"-"` + tick + `
	Headers *[]*WidgetHeader ` + tick + `url:"headers,omitempty" json:"headers,omitempty"` + tick + `
}

type AddHookOptions struct {
	URL           *string          ` + tick + `url:"url,omitempty" json:"url,omitempty"` + tick + `
	CustomHeaders *[]*WidgetHeader ` + tick + `url:"custom_headers,omitempty" json:"custom_headers,omitempty"` + tick + `
}

type ImportOptions struct {
	Path *string ` + tick + `url:"path,omitempty" json:"path,omitempty"` + tick + `
}

func (s *WidgetsService) ListWidgets(pid any, opt *ListWidgetsOptions, options ...RequestOptionFunc) ([]*Widget, *Response, error) {
	return do[[]*Widget](s.client, withPath(routeWidgets, ProjectID{pid}), withAPIOpts(opt))
}

func (s *WidgetsService) CreateWidget(pid any, opt *CreateWidgetOptions, options ...RequestOptionFunc) (*Widget, *Response, error) {
	reqOpts := []doOption{withMethod(http.MethodPost), withPath(routeWidgets, ProjectID{pid}), withAPIOpts(opt)}
	if opt.Avatar != nil {
		reqOpts = append(reqOpts, withUpload(nil, "avatar.png", UploadAvatar))
	}
	return do[*Widget](s.client, reqOpts...)
}

func (s *WidgetsService) UploadWidgetAvatar(pid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withMethod(http.MethodPut), withPath(routeWidgetAvatar, ProjectID{pid}), withUpload(nil, "avatar.png", UploadAvatar))
}
`

// standInDir writes the stand-in client-go in one of its two states and
// returns its directory: "v3.30.0" without the merged_after filter, and
// "v3.38.0" with it.
func standInDir(t *testing.T, modelsMergedAfter bool) string {
	t.Helper()
	field := ""
	if modelsMergedAfter {
		field = "\tMergedAfter *string " + tick + `url:"merged_after,omitempty" json:"merged_after,omitempty"` + tick + "\n"
	}
	source := strings.NewReplacer(mergedAfterField, field).Replace(standInSDK)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sdk.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("write the stand-in: %v", err)
	}
	return dir
}

// rest builds a REST request of the record.
func rest(route string, sdkMethods []string, sites ...string) actionrequests.RecordRequest {
	return actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: route, SDKMethods: sdkMethods, Sites: sites}
}

// param builds a declared param of the live record.
func param(kind string) apilive.Param { return apilive.Param{Type: kind} }

// fixtureRecord is the request record of the stand-in tree: a list and a
// create through client-go, a hook and an import built by hand, an
// integration whose slug the caller names, the list again as the target of a
// redirect another action's request follows, a request through no site at
// all, and requests the scope does not read (GraphQL, a route the live record
// does not hold).
func fixtureRecord() actionrequests.Record {
	set := rest("PUT /projects/:id/integrations/slack", nil, "widgets.setIntegration")
	set.Derived = "PUT /projects/:/integrations/:"
	redirected := rest("GET /projects/:id/widgets", nil, "widgets.Latest")
	redirected.Declaration = declarationFollowsRedirect
	return actionrequests.Record{Actions: []actionrequests.RecordAction{
		{ID: "widget.list", Handlers: []string{"widgets.List"}, Requests: []actionrequests.RecordRequest{
			rest("GET /projects/:id/widgets", []string{"Widgets.ListWidgets"}),
		}},
		{ID: "widget.latest", Handlers: []string{"widgets.Latest"}, Requests: []actionrequests.RecordRequest{redirected}},
		{ID: "widget.create", Handlers: []string{"widgets.Create"}, Requests: []actionrequests.RecordRequest{
			rest("POST /projects/:id/widgets", []string{"Widgets.CreateWidget"}),
			{Kind: actionrequests.KindGraphQL, Operation: "query widgets"},
			rest("GET /not/in/the/record", nil, "widgets.anywhere"),
		}},
		{ID: "widget.avatar", Handlers: []string{"widgets.Avatar"}, Requests: []actionrequests.RecordRequest{
			rest("PUT /projects/:id/widgets/avatar", []string{"Widgets.UploadWidgetAvatar"}),
		}},
		{ID: "widget.hook_add", Handlers: []string{"widgets.AddHook"}, Requests: []actionrequests.RecordRequest{
			rest("POST /projects/:id/hooks", nil, "widgets.doHook"),
		}},
		{ID: "widget.import", Handlers: []string{"widgets.Import"}, Requests: []actionrequests.RecordRequest{
			rest("POST /projects/import", nil, "widgets.rawImport"),
		}},
		{ID: "widget.set", Handlers: []string{"widgets.Set"}, Requests: []actionrequests.RecordRequest{set}},
		{ID: "widget.version", Handlers: []string{"widgets.Version"}, Requests: []actionrequests.RecordRequest{
			rest("GET /version", nil),
			rest("GET /version", []string{"Widgets.Vanished"}),
		}},
	}}
}

// fixtureLive is the live record of the stand-in tree.
func fixtureLive() apilive.Document {
	route := func(method, path string, params map[string]apilive.Param) apilive.Route {
		return apilive.Route{Method: method, Path: apilive.EndpointPrefix + path, Params: params}
	}
	str, array, hash, file := param("String"), param("Array"), param("Hash"), param(workhorseFile)
	return apilive.Document{Routes: []apilive.Route{
		route("GET", "/projects/:id/widgets", map[string]apilive.Param{
			"id": param("[String, Integer]"), "page": param("Integer"), "search": str, "labels": str,
			"merged_after": param("DateTime"), "not": hash, "not[labels]": str, "not[author_id]": param("Integer"),
		}),
		route("POST", "/projects/:id/widgets", map[string]apilive.Param{
			"name": str, "avatar": file, "headers": array, "headers[key]": str, "headers[value]": str,
			"url_variables": array, "url_variables[key]": str, "url_variables[value]": str,
		}),
		route("PUT", "/projects/:id/widgets/avatar", map[string]apilive.Param{"avatar": file}),
		route("POST", "/projects/:id/hooks", map[string]apilive.Param{
			"url": str, "custom_headers": array, "custom_headers[key]": str, "custom_headers[value]": str,
			"custom_headers[id]": param("Integer"),
		}),
		route("POST", "/projects/import", map[string]apilive.Param{
			"file": file, "file.sha256": str, "path": str, "override": hash, "override[name]": str,
		}),
		route("PUT", "/projects/:id/integrations/slack", map[string]apilive.Param{"webhook": str}),
		route("PUT", "/projects/:id/integrations/jira", map[string]apilive.Param{"url": str}),
		route("GET", "/version", map[string]apilive.Param{"detail": str}),
	}}
}

// fixtureInventory is what the stand-in's unit suite recorded.
func fixtureInventory() requestinventory.Inventory {
	return requestinventory.Inventory{Requests: []requestinventory.Row{
		{Package: "internal/tools/widgets", Kind: "rest", Method: "get", Path: "/projects/:project_id/widgets", Query: []string{"page", "labels[]", "not[labels]"}},
		{Package: "internal/tools/widgets", Kind: "rest", Method: "POST", Path: "/projects/:project_id/hooks", Body: []string{"url", "custom_headers"}},
		{Package: "internal/tools/widgets", Kind: "graphql", Method: "POST", Path: "/graphql"},
		{Package: "internal/tools/widgets", Kind: "rest", Method: "GET", Path: "/nowhere"},
	}}
}

// fixtureSchemas are the input schemas of the stand-in's actions.
func fixtureSchemas() map[string]map[string]any {
	properties := func(names ...string) map[string]any {
		out := map[string]any{}
		for _, name := range names {
			out[name] = map[string]any{"type": "string"}
		}
		return map[string]any{"properties": out}
	}
	create := properties("project_id", "title")
	create["properties"].(map[string]any)["headers"] = map[string]any{
		"type": "array", "items": map[string]any{"properties": map[string]any{"key": map[string]any{}, "value": map[string]any{}}},
	}
	return map[string]map[string]any{
		"widget.list":     properties("project_id", "search", "not_author_id"),
		"widget.create":   create,
		"widget.avatar":   properties("project_id", "avatar_path"),
		"widget.hook_add": properties("project_id", "url", "custom_headers"),
		"widget.import":   properties("path"),
		"widget.set":      properties("project_id", "slug", "config"),
		"widget.version":  properties(),
	}
}

// fixtureRaw is what the walk of the stand-in's tool packages read of its
// raw requests.
func fixtureRaw() []sdk.RawRequest {
	return []sdk.RawRequest{
		{Site: "widgets.doHook", Constructor: "NewRequest", Options: "any", Callers: []sdk.RawCaller{
			{Site: "widgets.AddHook", Verb: "POST", Options: "*gitlab.AddHookOptions"},
			{Site: "widgets.ListHooks", Verb: "GET", Options: "*gitlab.ListWidgetsOptions"},
		}},
		{Site: "widgets.rawImport", Constructor: "UploadRequest", Verb: "POST", Options: "*widgets.importBody", UploadField: "file", Sends: []waits.Superset{
			{Type: "widgets.importBody", Root: "ImportOptions", Keys: []string{"override.name"}},
		}},
		{Site: "widgets.setIntegration", Constructor: "NewRequestToURL", Verb: "PUT", Options: "map[string]any"},
		{Site: "widgets.setIntegration", Constructor: "NewRequest", Verb: "PUT", Options: "map[string]any"},
	}
}

// fixtureDeclarations answer the stand-in's findings, the upstream one the
// "v3.30.0" filter, which the "v3.38.0" state models.
func fixtureDeclarations() []paramDeclaration {
	return []paramDeclaration{
		{Route: "GET /projects/:id/widgets", Param: "merged_after", Category: categoryUpstream, Reason: "the stand-in's", Row: 7},
		{Route: "PUT /projects/:id/integrations/*", Param: anyParam, Category: categoryMapInput, Reason: "the stand-in's"},
	}
}

// useFixtures points every seam at the stand-in tree, client-go in the
// "v3.30.0" state unless modelsMergedAfter, and restores them afterwards.
func useFixtures(t *testing.T, modelsMergedAfter bool) {
	t.Helper()
	dir := standInDir(t, modelsMergedAfter)
	originals := struct {
		readRecord    func(string) (actionrequests.Record, error)
		readLive      func(string) (apilive.Document, error)
		readInventory func(string) (requestinventory.Inventory, error)
		inputSchemas  func() (map[string]map[string]any, error)
		rawRequests   func(string) ([]sdk.RawRequest, string, error)
		readOptions   func(string) sdkoptions.SDK
		readRows      func(string) (map[int]string, error)
		declarations  func() []paramDeclaration
		aliases       func() map[string]map[string]string
	}{readRecord, readLive, readInventory, inputSchemas, rawRequests, readOptions, readRows, declarations, aliases}
	t.Cleanup(func() {
		readRecord, readLive, readInventory = originals.readRecord, originals.readLive, originals.readInventory
		inputSchemas, rawRequests, readOptions = originals.inputSchemas, originals.rawRequests, originals.readOptions
		readRows, declarations, aliases = originals.readRows, originals.declarations, originals.aliases
	})
	readRecord = func(string) (actionrequests.Record, error) { return fixtureRecord(), nil }
	readLive = func(string) (apilive.Document, error) { return fixtureLive(), nil }
	readInventory = func(string) (requestinventory.Inventory, error) { return fixtureInventory(), nil }
	inputSchemas = func() (map[string]map[string]any, error) { return fixtureSchemas(), nil }
	rawRequests = func(string) ([]sdk.RawRequest, string, error) { return fixtureRaw(), dir, nil }
	readRows = func(string) (map[int]string, error) { return map[int]string{7: "row seven"}, nil }
	declarations = fixtureDeclarations
	aliases = func() map[string]map[string]string {
		return map[string]map[string]string{"widget.create": {"name": "title"}}
	}
}

// decode reads a report back.
func decode(t *testing.T, content []byte) Report {
	t.Helper()
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("decode the report: %v", err)
	}
	return report
}

// verdicts spells each finding and declared finding "route param class",
// with its category when a declaration answers it.
func verdicts(report Report) []string {
	var out []string
	for _, finding := range slices.Concat(report.Findings, report.Declared) {
		line := finding.Route + " " + finding.Param + " " + finding.Class
		if finding.Category != "" {
			line += " " + finding.Category
		}
		if len(finding.Children) > 0 {
			line += " [" + strings.Join(finding.Children, " ") + "]"
		}
		out = append(out, line)
	}
	slices.Sort(out)
	return out
}

// TestRun_TheStandInTree_JudgesEveryParamOfEveryRouteReached verifies the
// whole judgement on the stand-in, one case per way a param is sent or not.
//
// On the list, page and the label filters are recorded, the query's labels[]
// as the labels the record declares, search is offered and modeled but never
// recorded, not[author_id] is offered under the name an input flattens it to,
// and merged_after is modeled by no option struct and offered by no input,
// which is the finding R-PARAM exists for, answered here by an upstream
// declaration. The not container is covered by its recorded member. The
// redirect another action follows to the list carries no options of its own,
// so it leaves the list's params judged rather than unjudged. On the create,
// the name is offered through the alias that fills it from title, the
// headers through the nested schema of the input's list, the avatar is the
// file an upload sends when one is given, and url_variables, with nothing to
// send any of its members, stands for them. The avatar upload sends its file
// on every call. The hook's raw request is read through the call the action's
// handler makes, so the struct of the list call the hook list makes does not
// cover the hook's params; the custom headers recorded at the top level cover
// the keys nested under them that the hook's struct models, and the one it
// does not model is unmodeled, since the inventory records a body's top-level
// keys and not what was nested in them. The import's
// struct of the package's own carries the client-go struct it embeds and the
// key beside it, its file is the field the upload always sends, and the
// attributes Workhorse fills for the file are not judged; its override
// container, modeled by no struct and offered by no input, is dropped for its
// member, which is of another class. The integration's options are a free
// map, on the route the record places it and on the one its derived spelling
// meets, and the map declaration answers both. A request through no site and
// one through a method client-go does not declare leave the version's param
// unjudged.
func TestRun_TheStandInTree_JudgesEveryParamOfEveryRouteReached(t *testing.T) {
	useFixtures(t, false)

	content, clean, err := Run("root", Options{})
	if err != nil || !clean {
		t.Fatalf("Run = clean %t, error %v; want a clean run", clean, err)
	}
	if !strings.HasSuffix(string(content), "}\n") {
		t.Error("the report does not end with a newline")
	}
	report := decode(t, content)
	want := []string{
		"GET /projects/:id/widgets merged_after unmodeled upstream",
		"GET /projects/:id/widgets not[author_id] offered_unrecorded",
		"GET /projects/:id/widgets search offered_unrecorded",
		"GET /version detail unjudged",
		"POST /projects/:id/hooks custom_headers[id] unmodeled",
		"POST /projects/:id/widgets avatar not_offered",
		"POST /projects/:id/widgets headers offered_unrecorded [headers[key] headers[value]]",
		"POST /projects/:id/widgets name offered_unrecorded",
		"POST /projects/:id/widgets url_variables unmodeled [url_variables[key] url_variables[value]]",
		"POST /projects/import override[name] not_offered",
		"POST /projects/import path offered_unrecorded",
		"PUT /projects/:id/integrations/jira url unjudged map-input",
		"PUT /projects/:id/integrations/slack webhook unjudged map-input",
	}
	if got := verdicts(report); !slices.Equal(got, want) {
		t.Errorf("verdicts =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if report.Requests != actionrequests.RecordPath || report.Record != "docs/development/gitlab-api-live.json" || report.Inventory != requestinventory.Path {
		t.Errorf("inputs = %q, %q, %q; want the three committed paths", report.Requests, report.Record, report.Inventory)
	}
	wantUnread := []string{
		"GET /version widget.version  " + reasonNoSite,
		"GET /version widget.version Widgets.Vanished " + reasonUnknownMethod,
		"PUT /projects/:id/integrations/jira widget.set widgets.setIntegration " + reasonFreeMap,
		"PUT /projects/:id/integrations/jira widget.set widgets.setIntegration " + reasonFreeMap,
		"PUT /projects/:id/integrations/slack widget.set widgets.setIntegration " + reasonFreeMap,
		"PUT /projects/:id/integrations/slack widget.set widgets.setIntegration " + reasonFreeMap,
	}
	var unread []string
	for _, request := range report.Unread {
		unread = append(unread, request.Route+" "+request.Action+" "+request.Site+" "+request.Reason)
	}
	slices.Sort(unread)
	if !slices.Equal(unread, wantUnread) {
		t.Errorf("unread =\n%s\nwant\n%s", strings.Join(unread, "\n"), strings.Join(wantUnread, "\n"))
	}
	summary := Summary{
		Routes: 8, RoutesWithFindings: 7, Params: 28, Covered: 9,
		Unmodeled: 2, NotOffered: 2, OfferedUnrecorded: 5, Unjudged: 1, Declared: 3, UnreadRequests: 6,
	}
	if report.Summary != summary {
		t.Errorf("summary = %+v, want %+v", report.Summary, summary)
	}
}

// TestRun_AClientGoThatModelsTheParam_RetiresItsUpstreamDeclaration
// verifies the life of an upstream declaration: in the "v3.30.0" state the
// stand-in's list options lack merged_after and the declaration answers the
// unmodeled finding; once client-go models it ("v3.38.0") the finding is one
// an input can offer today, and the declaration, which answers an unmodeled
// param alone, is stale and fails the gate. Without the declaration the
// "v3.30.0" finding is reported and the gate still passes, since a finding
// is a report.
func TestRun_AClientGoThatModelsTheParam_RetiresItsUpstreamDeclaration(t *testing.T) {
	t.Run("v3.30.0 without the declaration", func(t *testing.T) {
		useFixtures(t, false)
		declarations = func() []paramDeclaration { return fixtureDeclarations()[1:] }
		content, clean, err := Run("root", Options{GapsOnly: true})
		if err != nil || !clean {
			t.Fatalf("Run = clean %t, error %v; want a clean run reporting the finding", clean, err)
		}
		report := decode(t, content)
		if !slices.Contains(verdicts(report), "GET /projects/:id/widgets merged_after unmodeled") || report.Declared != nil || report.Unread != nil {
			t.Errorf("report = %+v, want merged_after unmodeled and the declared and unread lists dropped", report)
		}
		if report.Summary.Unmodeled != 3 {
			t.Errorf("unmodeled = %d, want merged_after, url_variables and custom_headers[id]", report.Summary.Unmodeled)
		}
	})
	t.Run("v3.38.0", func(t *testing.T) {
		useFixtures(t, true)
		content, clean, err := Run("root", Options{})
		if err != nil || clean {
			t.Fatalf("Run = clean %t, error %v; want the gate to fail on the stale declaration", clean, err)
		}
		report := decode(t, content)
		if !slices.Contains(verdicts(report), "GET /projects/:id/widgets merged_after not_offered") {
			t.Errorf("verdicts = %v, want merged_after one an input can offer", verdicts(report))
		}
		if !slices.Equal(report.Stale, []string{"GET /projects/:id/widgets merged_after (upstream): answers no finding"}) {
			t.Errorf("stale = %v, want the upstream declaration", report.Stale)
		}
	})
}

// TestRun_AnInputThatCannotBeRead_StopsTheRunNamingIt verifies each input's
// failure surfaces with what failed, and that a report that cannot be
// encoded is an error rather than an empty file.
func TestRun_AnInputThatCannotBeRead_StopsTheRunNamingIt(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		arrange func(t *testing.T)
		wantErr string
	}{
		{name: "record", arrange: func(*testing.T) {
			readRecord = func(string) (actionrequests.Record, error) { return actionrequests.Record{}, boom }
		}, wantErr: "boom"},
		{name: "live", arrange: func(*testing.T) {
			readLive = func(string) (apilive.Document, error) { return apilive.Document{}, boom }
		}, wantErr: "read the live record: boom"},
		{name: "inventory", arrange: func(*testing.T) {
			readInventory = func(string) (requestinventory.Inventory, error) { return requestinventory.Inventory{}, boom }
		}, wantErr: "read the request inventory: boom"},
		{name: "catalog", arrange: func(*testing.T) {
			inputSchemas = func() (map[string]map[string]any, error) { return nil, boom }
		}, wantErr: "build the action catalog: boom"},
		{name: "raw requests", arrange: func(*testing.T) {
			rawRequests = func(string) ([]sdk.RawRequest, string, error) { return nil, "", boom }
		}, wantErr: "walk the raw requests: boom"},
		{name: "register", arrange: func(*testing.T) {
			readRows = func(string) (map[int]string, error) { return nil, boom }
		}, wantErr: "boom"},
		{name: "marshal", arrange: func(t *testing.T) {
			t.Helper()
			original := marshalIndent
			t.Cleanup(func() { marshalIndent = original })
			marshalIndent = func(any, string, string) ([]byte, error) { return nil, boom }
		}, wantErr: "marshal report: boom"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			useFixtures(t, false)
			testCase.arrange(t)
			content, clean, err := Run("root", Options{})
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) || clean || content != nil {
				t.Errorf("Run = %q, clean %t, error %v; want no report and an error containing %q", content, clean, err, testCase.wantErr)
			}
		})
	}
}

// TestDefaultReaders_ReadWhereTheRepositoryKeepsThem verifies the readers
// the seams default to look under the root they are given, so a root without
// the files is an error naming what is missing, and that the catalog the
// default schema reader builds holds an input schema for its actions.
func TestDefaultReaders_ReadWhereTheRepositoryKeepsThem(t *testing.T) {
	if _, err := readLive(t.TempDir()); err == nil || !strings.Contains(err.Error(), apilive.FileName) {
		t.Errorf("readLive of an empty root = %v, want an error naming %s", err, apilive.FileName)
	}
	if _, err := readRows(t.TempDir()); err == nil || !strings.Contains(err.Error(), "upstream register") {
		t.Errorf("readRows of an empty root = %v, want an error naming the register", err)
	}
	schemas, err := catalogSchemas()
	if err != nil {
		t.Fatalf("catalogSchemas: %v", err)
	}
	if properties := schemaProperties(schemas["merge_request.list"]); properties["project_id"] == nil {
		t.Errorf("merge_request.list schema properties = %v, want project_id among them", properties)
	}
}

// TestCatalogSchemas_ACatalogThatWillNotBuild_IsAnError verifies a failure
// to build the catalog is returned rather than read as no schema.
func TestCatalogSchemas_ACatalogThatWillNotBuild_IsAnError(t *testing.T) {
	original := catalogs
	t.Cleanup(func() { catalogs = original })
	catalogs = func() ([]*actioncatalog.Catalog, error) { return nil, errors.New("boom") }
	if schemas, err := catalogSchemas(); err == nil || schemas != nil {
		t.Errorf("catalogSchemas = %v, %v; want the error", schemas, err)
	}
}

// repositoryReport runs the scope on the repository's own tree.
func repositoryReport(t *testing.T) Report {
	t.Helper()
	freshness.SkipIfDeferred(t)
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("find the repository root: %v", err)
	}
	content, clean, err := Run(root, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := decode(t, content)
	if !clean {
		t.Fatalf("stale param declarations: %v", report.Stale)
	}
	return report
}

// TestRun_Repository_GateIsGreen runs the gate on the repository's own
// request record, live record, inventory and catalog, which is what make
// audit-1to1-params does: no declaration answers nothing. It reads the
// committed request record, so a stacked layer that defers that record's
// refresh skips it.
func TestRun_Repository_GateIsGreen(t *testing.T) {
	report := repositoryReport(t)
	if report.Summary.Routes == 0 || report.Summary.Params == 0 || report.Summary.Declared == 0 {
		t.Errorf("summary = %+v, want the tree's routes judged and its declarations used", report.Summary)
	}
}

// TestRun_Repository_ReportsTheParamsClientGoV3300CannotSend fixes the cases
// R-PARAM was built to see on the pinned client-go. It is sensitive to the
// version on purpose: the stack that bumps client-go to v3.38.0 changes it,
// and that change is the evidence the scope was run again.
//
// In v3.30.0 the four merge request lists (the global one, the project's,
// the group's and a deployment's) take source_project_id and the filters
// GitLab 19.5 added, which no option struct models: thirteen on every list,
// the deployment filters and environment on the three ListMergeRequestsOptions
// and ListGroupMergeRequestsOptions serve, and in on the project's, which
// ListProjectMergeRequestsOptions leaves out. The system hooks take
// url_variables and custom_headers, and the project and group hooks
// url_variables; a project edit takes the three duo_* flags v3.38.0 adds to
// EditProjectOptions; a registry repository read takes size; every
// issues_statistics route and the group epic list take not[author_id]; a
// release read takes include_html_description, which GetRelease, taking no
// options, cannot send and the redirect release.get_latest follows to the
// route does not either; and a protected environment's approval rule takes
// an id, nested under the approval_rules the inventory records, which
// EnvironmentApprovalRuleOptions does not model. The service account external
// flag and the four duo flags a create drops are declared, and so is every
// param of the four label collection routes, which client-go takes only for
// a label argument the handlers never leave nil.
//
// The custom header keys of a project hook are carried by the struct its raw
// request sends: on the add, whose recorded body carries custom_headers,
// they are covered only because the walk of AddProjectHookOptions models
// them, and on the edit, whose recorded body does not, the container stands
// for them with the struct EditProjectHookOptions modeling it.
func TestRun_Repository_ReportsTheParamsClientGoV3300CannotSend(t *testing.T) {
	report := repositoryReport(t)
	reported := map[string]Finding{}
	for _, finding := range slices.Concat(report.Findings, report.Declared) {
		reported[finding.Route+" "+finding.Param] = finding
		for _, child := range finding.Children {
			reported[finding.Route+" "+child] = finding
		}
	}
	for _, tc := range witnessCases() {
		t.Run(tc.route+" "+tc.param, func(t *testing.T) {
			finding, found := reported[tc.route+" "+tc.param]
			if !found || finding.Class != tc.class || finding.Category != tc.category {
				t.Errorf("%s %s = %+v (found %t), want class %s category %q", tc.route, tc.param, finding, found, tc.class, tc.category)
			}
		})
	}
	checkProjectHookHeaders(t, reported)
}

// witnessCase is one param the witness test expects reported, with its class
// and, when a declaration answers it, its category.
type witnessCase struct{ route, param, class, category string }

// witnessCases are the params the witness test expects on v3.30.0.
func witnessCases() []witnessCase {
	type want = witnessCase
	var cases []want
	everyList := []string{
		"assignee_username", "merge_user_id", "merge_user_username", "merged_after", "merged_before",
		"not[assignee_id]", "not[assignee_username]", "not[author_id]", "not[milestone]", "not[my_reaction_emoji]",
		"not[reviewer_id]", "not[reviewer_username]", "source_project_id",
	}
	deployments := []string{"deployed_after", "deployed_before", "environment"}
	for route, params := range map[string][]string{
		"GET /merge_requests":                                         slices.Concat(everyList, deployments),
		"GET /projects/:id/merge_requests":                            slices.Concat(everyList, []string{"in"}),
		"GET /groups/:id/merge_requests":                              slices.Concat(everyList, deployments),
		"GET /projects/:id/deployments/:deployment_id/merge_requests": slices.Concat(everyList, deployments),
	} {
		for _, param := range params {
			cases = append(cases, want{route, param, ClassUnmodeled, ""})
		}
	}
	for _, param := range []string{"duo_sast_fp_detection_enabled", "duo_sast_vr_workflow_enabled", "duo_secret_detection_fp_enabled"} {
		cases = append(cases, want{"PUT /projects/:id", param, ClassUnmodeled, ""})
	}
	for _, route := range []string{"POST /hooks", "PUT /hooks/:hook_id"} {
		cases = append(cases, want{route, "url_variables", ClassUnmodeled, ""}, want{route, "custom_headers", ClassUnmodeled, ""})
	}
	for _, route := range []string{"POST /projects/:id/hooks", "PUT /projects/:id/hooks/:hook_id", "POST /groups/:id/hooks", "PUT /groups/:id/hooks/:hook_id"} {
		cases = append(cases, want{route, "url_variables", ClassUnmodeled, ""})
	}
	for _, route := range []string{"GET /issues_statistics", "GET /groups/:id/issues_statistics", "GET /projects/:id/issues_statistics", "GET /groups/:id/epics"} {
		cases = append(cases, want{route, "not[author_id]", ClassUnmodeled, ""})
	}
	for _, route := range []string{"POST /projects", "POST /projects/user/:user_id"} {
		for _, param := range []string{
			"duo_dependency_bump_breaking_changes_enabled", "duo_sast_fp_detection_enabled",
			"duo_sast_vr_workflow_enabled", "duo_secret_detection_fp_enabled",
		} {
			cases = append(cases, want{route, param, ClassUnmodeled, categoryDeclaredNotApplied})
		}
	}
	cases = append(cases,
		want{"GET /registry/repositories/:id", "size", ClassUnmodeled, ""},
		want{"GET /projects/:id/releases/:tag_name", "include_html_description", ClassUnmodeled, ""},
		want{"POST /projects/:id/protected_environments", "approval_rules[id]", ClassUnmodeled, ""},
		want{"POST /service_accounts", "external", ClassUnmodeled, categoryUpstream},
		want{"PATCH /service_accounts/:user_id", "external", ClassUnmodeled, categoryUpstream},
		want{"PUT /projects/:id/labels", "label_id", ClassOfferedUnrecorded, categoryBranchNotTaken},
		want{"DELETE /projects/:id/labels", "label_id", ClassOfferedUnrecorded, categoryBranchNotTaken},
		want{"PUT /groups/:id/labels", "label_id", ClassOfferedUnrecorded, categoryBranchNotTaken},
		want{"DELETE /groups/:id/labels", "name", ClassNotOffered, categoryBranchNotTaken},
	)
	return cases
}

// checkProjectHookHeaders holds the custom header keys of a project hook to
// the struct its raw request sends: covered on the add, read through
// AddProjectHookOptions, and under the container EditProjectHookOptions
// models on the edit.
func checkProjectHookHeaders(t *testing.T, reported map[string]Finding) {
	t.Helper()
	const hookAdd, hookEdit = "POST /projects/:id/hooks", "PUT /projects/:id/hooks/:hook_id"
	t.Run(hookAdd+" is read through AddProjectHookOptions", func(t *testing.T) {
		if finding := reported[hookAdd+" url_variables"]; !slices.Contains(finding.OptionTypes, "AddProjectHookOptions") {
			t.Errorf("%s url_variables option types = %v, want the AddProjectHookOptions the raw request sends", hookAdd, finding.OptionTypes)
		}
	})
	for _, param := range []string{"custom_headers", "custom_headers[key]", "custom_headers[value]"} {
		t.Run(hookAdd+" "+param+" is covered", func(t *testing.T) {
			if finding, found := reported[hookAdd+" "+param]; found {
				t.Errorf("%s %s = %+v, want it covered by the recorded custom_headers and the struct that models it", hookAdd, param, finding)
			}
		})
	}
	for _, param := range []string{"custom_headers[key]", "custom_headers[value]"} {
		t.Run(hookEdit+" "+param+" stands under the modeled container", func(t *testing.T) {
			finding, found := reported[hookEdit+" "+param]
			if !found || finding.Param != "custom_headers" || !finding.SDKModels || !slices.Contains(finding.OptionTypes, "EditProjectHookOptions") {
				t.Errorf("%s %s = %+v (found %t), want it under the custom_headers container EditProjectHookOptions models", hookEdit, param, finding, found)
			}
		})
	}
}
