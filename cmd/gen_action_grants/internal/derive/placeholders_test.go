// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package derive

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests/actionfixture"
)

// placedSource is the fixture the placeholder reading is held to: one
// handler per shape an argument filling a placeholder takes in the tree, each
// routed through an ActionSpec, so the walk meets them the way it meets a
// domain package.
const placedSource = `package placed

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

type Scope struct {
	ProjectID toolutil.StringOrInt @@json:"project_id"@@
}

type Position struct {
	BaseSHA string @@json:"base_sha"@@
}

type Input struct {
	Scope
	KeyID    int64    @@json:"deploy_key_id"@@
	Name     string   @@json:"name,omitempty"@@
	Other    string   @@json:"other"@@
	Hidden   string   @@json:"-"@@
	Untagged string
	Position Position @@json:"position"@@
	Pointer  *string  @@json:"pointer"@@
	Lookup   func() string @@json:"-"@@
}

type Output struct {
	OK bool @@json:"ok"@@
}

const fixed = "fixed"

var shared = "shared"

func key(ctx context.Context, client *gitlabclient.Client, project any, id int64) error {
	_, _, err := client.GL().DeployKeys.GetDeployKey(project, id, gl.WithContext(ctx))
	return err
}

func get(ctx context.Context, client *gitlabclient.Client, project any) error {
	_, _, err := client.GL().Projects.GetProject(project, nil, gl.WithContext(ctx))
	return err
}

func resolve(name string) (int64, error) {
	return strconv.ParseInt(name, 10, 64)
}

func pick(fn func() string) string {
	return fn()
}

func count() int {
	return 1
}

func Direct(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, _, err := client.GL().DeployKeys.GetDeployKey(input.ProjectID, input.KeyID, gl.WithContext(ctx))
	return Output{}, err
}

func Helpers(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, key(ctx, client, input.ProjectID.String(), input.KeyID)
}

func Locals(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	project := input.ProjectID
	var id = input.KeyID
	var unused string
	unused = input.Name
	_ = unused
	return Output{}, key(ctx, client, project, id)
}

func Resolved(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	id, err := resolve(input.Name)
	if err != nil {
		return Output{}, err
	}
	return Output{}, key(ctx, client, input.ProjectID, id)
}

func resolveWithin(ctx context.Context, client *gitlabclient.Client, project any, names ...string) (int64, error) {
	return resolve(names[0])
}

func Scoped(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	id, err := resolveWithin(ctx, client, input.ProjectID, input.Name)
	if err != nil {
		return Output{}, err
	}
	return Output{}, key(ctx, client, input.ProjectID, id)
}

func ScopedTwice(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	id, err := resolveWithin(ctx, client, input.ProjectID, input.Name, input.Other)
	if err != nil {
		return Output{}, err
	}
	return Output{}, key(ctx, client, input.ProjectID, id)
}

func Unscoped(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	id, err := resolveWithin(ctx, client, input.Other, input.Name)
	if err != nil {
		return Output{}, err
	}
	return Output{}, key(ctx, client, fixed, id)
}

func Mixed(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, key(ctx, client, input.Name+"/"+input.Other, int64(len(fixed)))
}

func Declared(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	var unrelated = input.Other
	_ = unrelated
	var project string
	project = input.Name
	out := Output{}
	out.OK = true
	return out, get(ctx, client, project)
}

func LocalField(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	out := Output{}
	return Output{}, get(ctx, client, out.OK)
}

func Chain(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, get(ctx, client, input.Position.BaseSHA)
}

func Hidden(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, get(ctx, client, input.Hidden)
}

func Untagged(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, get(ctx, client, input.Untagged)
}

func Pointers(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := get(ctx, client, *input.Pointer); err != nil {
		return Output{}, err
	}
	_, err := client.GL().Projects.DeleteProject(&input.Name, nil, gl.WithContext(ctx))
	return Output{}, err
}

func inner(ctx context.Context, client *gitlabclient.Client, in Input) error {
	return get(ctx, client, in.Other)
}

func innerPointer(ctx context.Context, client *gitlabclient.Client, in *Input) error {
	_, err := client.GL().Projects.DeleteProject(in.Other, nil, gl.WithContext(ctx))
	return err
}

func innerPosition(ctx context.Context, client *gitlabclient.Client, position Position) error {
	_, _, err := client.GL().Projects.EditProject(position.BaseSHA, nil, gl.WithContext(ctx))
	return err
}

func Literals(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := inner(ctx, client, Input{Other: input.Name}); err != nil {
		return Output{}, err
	}
	if err := innerPointer(ctx, client, &Input{Other: strconv.FormatInt(input.KeyID, 10)}); err != nil {
		return Output{}, err
	}
	return Output{}, innerPosition(ctx, client, Position{input.Name})
}

func lookup(name string) (int, string) {
	return len(name), name
}

func SecondResult(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, project := lookup(input.Name)
	return Output{}, get(ctx, client, project)
}

func FuncField(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, get(ctx, client, input.Lookup())
}

func Unmatched(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, inner(ctx, client, Input{Name: input.Name})
}

func whole(ctx context.Context, client *gitlabclient.Client, in Input) error {
	return get(ctx, client, in.Name)
}

func wholePointer(ctx context.Context, client *gitlabclient.Client, in *Input) error {
	if _, err := client.GL().Projects.DeleteProject(in.Other, nil, gl.WithContext(ctx)); err != nil {
		return err
	}
	_, _, err := client.GL().Projects.EditProject((*in).Name, nil, gl.WithContext(ctx))
	return err
}

func Whole(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := whole(ctx, client, input); err != nil {
		return Output{}, err
	}
	return Output{}, wholePointer(ctx, client, &input)
}

func Foreign(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, get(ctx, client, req.Params.Name)
}

func Response(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	project, _, err := client.GL().Projects.GetProject(input.Name, nil, gl.WithContext(ctx))
	if err != nil {
		return Output{}, err
	}
	_, err = client.GL().Projects.DeleteProject(project.ID, nil, gl.WithContext(ctx))
	return Output{}, err
}

func Unread(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := get(ctx, client, http.MethodGet); err != nil {
		return Output{}, err
	}
	if _, err := client.GL().Projects.DeleteProject(shared, nil, gl.WithContext(ctx)); err != nil {
		return Output{}, err
	}
	if _, _, err := client.GL().Projects.EditProject(fixed, nil, gl.WithContext(ctx)); err != nil {
		return Output{}, err
	}
	_, _, err := client.GL().Projects.StarProject([]string{input.Name}[0], gl.WithContext(ctx))
	return Output{}, err
}

func MethodValue(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	return Output{}, get(ctx, client, pick(input.ProjectID.String))
}

func SelfAssigned(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	project := input.Name
	project = project + "-copy"
	return Output{}, get(ctx, client, project)
}

func Conflict(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := get(ctx, client, input.Name); err != nil {
		return Output{}, err
	}
	return Output{}, get(ctx, client, input.Other)
}

func Agree(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := get(ctx, client, input.Name); err != nil {
		return Output{}, err
	}
	return Output{}, get(ctx, client, input.Name)
}

func apply(fn func(any, *gl.GetProjectOptions, ...gl.RequestOptionFunc) (*gl.Project, *gl.Response, error), project any) error {
	_, _, err := fn(project, nil)
	return err
}

func Unknown(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if err := get(ctx, client, input.Name); err != nil {
		return Output{}, err
	}
	return Output{}, apply(client.GL().Projects.GetProject, input.Name)
}

func Segments(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if _, _, err := client.GL().DeployKeys.EnableDeployKey(input.KeyID, input.KeyID, gl.WithContext(ctx)); err != nil {
		return Output{}, err
	}
	_, _, err := client.GL().Projects.UnstarProject(input.Name, gl.WithContext(ctx))
	return Output{}, err
}

func Split(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, _, err := client.GL().DeployKeys.EnableDeployKey(input.Name, input.KeyID, gl.WithContext(ctx))
	return Output{}, err
}

func RawFields(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	path := fmt.Sprintf("projects/%s/hooks/%d", url.PathEscape(input.ProjectID.String()), input.KeyID)
	_, err := client.GL().NewRequest(http.MethodGet, path, nil, nil)
	return Output{}, err
}

func RawMixed(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	if _, err := client.GL().NewRequest(http.MethodPut, "projects/"+url.PathEscape(input.Name)+"-"+input.Other, nil, nil); err != nil {
		return Output{}, err
	}
	_, err := client.GL().NewRequest(http.MethodDelete, fmt.Sprintf("projects/%d", count()), nil, nil)
	return Output{}, err
}

func RawBranches(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	path := "projects/" + input.Name
	if input.Other != "" {
		path = "projects/" + input.Other
	}
	_, err := client.GL().NewRequest(http.MethodGet, path, nil, nil)
	return Output{}, err
}

func RawAgree(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	path := "projects/" + input.Name
	if input.Other != "" {
		path = "/projects/" + input.Name + "?with=other"
	}
	_, err := client.GL().NewRequest(http.MethodGet, path, nil, nil)
	return Output{}, err
}

func RawUnknown(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_, err := client.GL().NewRequest(http.MethodGet, fmt.Sprintf("projects/%s/hooks/%s", input.Name), nil, nil)
	return Output{}, err
}

func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		spec("direct", toolutil.RouteAction(client, Direct)),
		spec("helpers", toolutil.RouteAction(client, Helpers)),
		spec("locals", toolutil.RouteAction(client, Locals)),
		spec("resolved", toolutil.RouteAction(client, Resolved)),
		spec("scoped", toolutil.RouteAction(client, Scoped)),
		spec("scoped_twice", toolutil.RouteAction(client, ScopedTwice)),
		spec("unscoped", toolutil.RouteAction(client, Unscoped)),
		spec("mixed", toolutil.RouteAction(client, Mixed)),
		spec("declared", toolutil.RouteAction(client, Declared)),
		spec("local_field", toolutil.RouteAction(client, LocalField)),
		spec("chain_at_the_limit", toolutil.RouteAction(client, ChainAtTheLimit)),
		spec("chain_past_the_limit", toolutil.RouteAction(client, ChainPastTheLimit)),
		spec("chain", toolutil.RouteAction(client, Chain)),
		spec("hidden", toolutil.RouteAction(client, Hidden)),
		spec("untagged", toolutil.RouteAction(client, Untagged)),
		spec("pointers", toolutil.RouteAction(client, Pointers)),
		spec("literals", toolutil.RouteAction(client, Literals)),
		spec("unmatched", toolutil.RouteAction(client, Unmatched)),
		spec("whole", toolutil.RouteAction(client, Whole)),
		spec("foreign", toolutil.RouteActionWithRequest(client, Foreign)),
		spec("response", toolutil.RouteAction(client, Response)),
		spec("unread", toolutil.RouteAction(client, Unread)),
		spec("method_value", toolutil.RouteAction(client, MethodValue)),
		spec("self_assigned", toolutil.RouteAction(client, SelfAssigned)),
		spec("conflict", toolutil.RouteAction(client, Conflict)),
		spec("agree", toolutil.RouteAction(client, Agree)),
		spec("unknown", toolutil.RouteAction(client, Unknown)),
		spec("segments", toolutil.RouteAction(client, Segments)),
		spec("split", toolutil.RouteAction(client, Split)),
		spec("raw_fields", toolutil.RouteAction(client, RawFields)),
		spec("raw_mixed", toolutil.RouteAction(client, RawMixed)),
		spec("raw_branches", toolutil.RouteAction(client, RawBranches)),
		spec("raw_agree", toolutil.RouteAction(client, RawAgree)),
		spec("raw_unknown", toolutil.RouteAction(client, RawUnknown)),
		spec("second_result", toolutil.RouteAction(client, SecondResult)),
		spec("func_field", toolutil.RouteAction(client, FuncField)),
	}
}

func spec(name string, route toolutil.ActionRoute) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{Usage: "fixture"})
}
`

// chainHandler writes a handler that hands a helper the input's name through
// links locals, each assigned from the one before it, so the read of the field
// sits links+2 reads down: the helper's parameter, every local, then the field.
func chainHandler(name string, links int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nfunc %s(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {\n\tlink1 := input.Name\n", name)
	for i := 2; i <= links; i++ {
		fmt.Fprintf(&b, "\tlink%d := link%d\n", i, i-1)
	}
	fmt.Fprintf(&b, "\treturn Output{}, get(ctx, client, link%d)\n}\n", links)
	return b.String()
}

// placedCache memoizes the loaded placeholder fixture, which every test of
// this file reads without changing it.
var placedCache *actionrequests.Program

// loadPlaced loads the placeholder fixture program.
func loadPlaced(t *testing.T) *actionrequests.Program {
	t.Helper()
	if placedCache != nil {
		return placedCache
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	root, err := actionfixture.Root(dir)
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	source := placedSource + chainHandler("ChainAtTheLimit", maxFoldDepth-2) + chainHandler("ChainPastTheLimit", maxFoldDepth-1)
	prog, err := actionrequests.Load(root, []string{actionfixture.Pattern}, actionfixture.Overlay(root, map[string]string{"placed": source}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	placedCache = prog
	return prog
}

// placedSDK is the client-go the placeholder fixture calls: where each
// method's arguments land, two of them placed by hand where the reading of
// client-go never would, to reach the rules for an argument past the call's
// end and two arguments filling one segment.
func placedSDK() stubSDK {
	route := func(method, path string) Request { return Request{Kind: KindREST, Method: method, Path: path} }
	return stubSDK{
		"DeployKeys.GetDeployKey": {
			routes: []Request{route("GET", "/projects/:/deploy_keys/:")},
			placements: []Placement{
				{Index: 0, Route: "GET /projects/:/deploy_keys/:", Segment: 1},
				{Index: 1, Route: "GET /projects/:/deploy_keys/:", Segment: 3},
			},
		},
		"DeployKeys.EnableDeployKey": {
			routes: []Request{route("POST", "/projects/:/deploy_keys/:/enable")},
			placements: []Placement{
				{Index: 0, Route: "POST /projects/:/deploy_keys/:/enable", Segment: 1},
				{Index: 1, Route: "POST /projects/:/deploy_keys/:/enable", Segment: 1},
			},
		},
		"Projects.GetProject":    {routes: []Request{route("GET", "/projects/:")}, placements: []Placement{{Index: 0, Route: "GET /projects/:", Segment: 1}}},
		"Projects.DeleteProject": {routes: []Request{route("DELETE", "/projects/:")}, placements: []Placement{{Index: 0, Route: "DELETE /projects/:", Segment: 1}}},
		"Projects.EditProject":   {routes: []Request{route("PUT", "/projects/:")}, placements: []Placement{{Index: 0, Route: "PUT /projects/:", Segment: 1}}},
		"Projects.StarProject":   {routes: []Request{route("POST", "/projects/:/star")}, placements: []Placement{{Index: 0, Route: "POST /projects/:/star", Segment: 1}}},
		"Projects.UnstarProject": {
			routes:     []Request{route("POST", "/projects/:/unstar")},
			placements: []Placement{{Index: 2, Route: "POST /projects/:/unstar", Segment: 1}},
		},
	}
}

// derivePlaced derives the named actions of the placeholder fixture.
func derivePlaced(t *testing.T) map[string]Action {
	t.Helper()
	names := []string{
		"direct", "helpers", "locals", "resolved", "scoped", "scoped_twice", "unscoped", "mixed", "declared", "local_field",
		"chain_at_the_limit", "chain_past_the_limit", "chain", "hidden", "untagged", "pointers", "literals",
		"unmatched", "whole", "foreign", "response", "unread", "method_value", "self_assigned", "conflict", "agree",
		"unknown", "segments", "split", "raw_fields", "raw_mixed", "raw_branches", "raw_agree", "raw_unknown",
		"second_result", "func_field",
	}
	actions := make([]actionrequests.Action, len(names))
	for i, name := range names {
		actions[i] = actionrequests.Action{ID: "placed." + name, Name: name, Owner: "placed"}
	}
	result := Derive(loadPlaced(t), actions, placedSDK(), nil)
	byID := map[string]Action{}
	for _, act := range result.Actions {
		byID[act.ID] = act
	}
	return byID
}

// describeFills renders what each request of an action is filled from, one
// line per request in the order the walk met them.
func describeFills(act *Action) string {
	var b strings.Builder
	for _, use := range act.Uses {
		fmt.Fprintf(&b, "%s", use.Key())
		for _, segment := range slices.Sorted(maps.Keys(use.Fills)) {
			fmt.Fprintf(&b, " %d=%s", segment, use.Fills[segment])
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TestDerive_EachArgumentShape_FillsThePlaceholdersFromTheFieldItIsBuiltOutOf
// verifies, for every shape an argument filling a placeholder takes, which
// field of the input the derivation reads it to: a field selected on the
// input (promoted from an embedded struct, through a chain, behind & or *,
// spelled by its tag or by its Go name, none for one the JSON leaves out);
// through a helper's parameter, a local, a declaration, a call taking several
// results, a method call's receiver, a conversion, a method value and a
// composite literal handed down; a lookup handed the field another segment is
// filled from as well as the one it looks up; a var declared with no value and
// assigned later, beside another var the argument does not read and an
// assignment to a field; a chain of
// locals down to the deepest read allowed; and none for a chain past it, a
// field of a local of the module's own type, a lookup left with two fields or
// scoped by no segment, a field of a value GitLab
// answered, a field of a struct outside the module, a constant, a package
// variable, an index, an expression of two fields, a literal that gives the
// field nothing, two calls filling one segment from two fields, a method
// handed on as a value, and an argument past the call's end. A raw request's
// path is read the same way, segment by segment.
func TestDerive_EachArgumentShape_FillsThePlaceholdersFromTheFieldItIsBuiltOutOf(t *testing.T) {
	derived := derivePlaced(t)
	cases := map[string]string{
		"direct":       "GET /projects/:/deploy_keys/: 1=project_id 3=deploy_key_id\n",
		"helpers":      "GET /projects/:/deploy_keys/: 1=project_id 3=deploy_key_id\n",
		"locals":       "GET /projects/:/deploy_keys/: 1=project_id 3=deploy_key_id\n",
		"resolved":     "GET /projects/:/deploy_keys/: 1=project_id 3=name\n",
		"scoped":       "GET /projects/:/deploy_keys/: 1=project_id 3=name\n",
		"scoped_twice": "GET /projects/:/deploy_keys/: 1=project_id\n",
		"unscoped":     "GET /projects/:/deploy_keys/:\n",
		"mixed":        "GET /projects/:/deploy_keys/:\n",
		"declared":     "GET /projects/: 1=name\n",
		"local_field":  "GET /projects/:\n",
		// The read of the field sits at the deepest read allowed, and one past it.
		"chain_at_the_limit":   "GET /projects/: 1=name\n",
		"chain_past_the_limit": "GET /projects/:\n",
		"chain":                "GET /projects/: 1=position\n",
		"hidden":               "GET /projects/:\n",
		"untagged":             "GET /projects/: 1=Untagged\n",
		"pointers":             "GET /projects/: 1=pointer\nDELETE /projects/: 1=name\n",
		"literals":             "GET /projects/: 1=name\nDELETE /projects/: 1=deploy_key_id\nPUT /projects/:\n",
		"unmatched":            "GET /projects/:\n",
		"whole":                "GET /projects/: 1=name\nDELETE /projects/: 1=other\nPUT /projects/: 1=name\n",
		"second_result":        "GET /projects/: 1=name\n",
		"func_field":           "GET /projects/:\n",
		"raw_agree":            "GET /projects/: 1=name\n",
		"foreign":              "GET /projects/:\n",
		"response":             "GET /projects/: 1=name\nDELETE /projects/:\n",
		"unread":               "GET /projects/:\nDELETE /projects/:\nPUT /projects/:\nPOST /projects/:/star\n",
		"method_value":         "GET /projects/: 1=project_id\n",
		"self_assigned":        "GET /projects/: 1=name\n",
		"conflict":             "GET /projects/:\n",
		"agree":                "GET /projects/: 1=name\n",
		"unknown":              "GET /projects/:\n",
		"segments":             "POST /projects/:/deploy_keys/:/enable 1=deploy_key_id\nPOST /projects/:/unstar\n",
		"split":                "POST /projects/:/deploy_keys/:/enable\n",
		"raw_fields":           "GET /projects/:/hooks/: 1=project_id 3=deploy_key_id\n",
		"raw_mixed":            "PUT /projects/:\nDELETE /projects/:\n",
		"raw_branches":         "GET /projects/:\n",
		"raw_unknown":          "unresolved raw-path placed.RawUnknown\n",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			act, ok := derived["placed."+name]
			if !ok {
				t.Fatalf("placed.%s was not derived", name)
			}
			if len(act.Findings) > 0 && name != "raw_unknown" {
				t.Fatalf("placed.%s has findings: %v", name, act.Findings)
			}
			if got := describeFills(&act); got != want {
				t.Errorf("fills of placed.%s =\n%s\nwant\n%s", name, got, want)
			}
		})
	}
}

// TestFrame_WithNoParameterList_DeclaresNone verifies a frame entered through
// a package-level initializer that is no function literal, which has no
// parameter list, reads no variable as a parameter.
func TestFrame_WithNoParameterList_DeclaresNone(t *testing.T) {
	if (&frame{}).declares(nil) {
		t.Error("a frame with no parameter list declares a parameter")
	}
}

// TestFillState_MergesCallsSegmentBySegment verifies the fold of what several
// calls fill one request from: a segment keeps the field every call agreed
// on, loses it to a call that fills it from another field or from none, and a
// call that could not be read leaves the request with nothing.
func TestFillState_MergesCallsSegmentBySegment(t *testing.T) {
	cases := []struct {
		name  string
		calls []map[int]string
		want  map[int]string
	}{
		{name: "one call", calls: []map[int]string{{1: "a", 3: "b"}}, want: map[int]string{1: "a", 3: "b"}},
		{name: "two agreeing", calls: []map[int]string{{1: "a"}, {1: "a"}}, want: map[int]string{1: "a"}},
		{name: "two disagreeing", calls: []map[int]string{{1: "a", 3: "b"}, {1: "c", 3: "b"}}, want: map[int]string{3: "b"}},
		{name: "a later call fills a segment from none", calls: []map[int]string{{1: "a"}, {}}, want: nil},
		{name: "a later call fills a new segment", calls: []map[int]string{{}, {1: "a"}}, want: nil},
		{name: "an unread call first", calls: []map[int]string{nil, {1: "a"}}, want: nil},
		{name: "an unread call last", calls: []map[int]string{{1: "a"}, nil}, want: nil},
		{name: "a call filling nothing", calls: []map[int]string{{1: ""}}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var state fillState
			for _, call := range tc.calls {
				state.merge(call)
			}
			if got := state.placeholders(); !maps.Equal(got, tc.want) || (got == nil) != (tc.want == nil) {
				t.Errorf("placeholders = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRouteFills_Of_TellsAnUnreadCallFromARouteNoArgumentFills verifies a
// call that could not be read answers nothing for every route, and one that
// could answers an empty set for a route no argument of its method lands in,
// which is a request known to be filled from no field.
func TestRouteFills_Of_TellsAnUnreadCallFromARouteNoArgumentFills(t *testing.T) {
	route := Request{Kind: KindREST, Method: "GET", Path: "/projects/:"}
	if got := routeFills(nil).of(route); got != nil {
		t.Errorf("an unread call answers %v, want nil", got)
	}
	read := routeFills{"GET /projects/:": {1: "project_id"}}
	if got := read.of(route); !maps.Equal(got, map[int]string{1: "project_id"}) {
		t.Errorf("a read call answers %v for its route", got)
	}
	if got := read.of(Request{Kind: KindREST, Method: "GET", Path: "/user"}); got == nil || len(got) != 0 {
		t.Errorf("a read call answers %v for a route no argument fills, want an empty set", got)
	}
}
