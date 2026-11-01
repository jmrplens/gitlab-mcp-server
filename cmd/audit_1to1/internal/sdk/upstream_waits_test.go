// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"encoding/json"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// waitsSDK is a stand-in client-go for the upstream waits section, in its
// "v3.30.0" state: a flag option struct whose scope fields lack omitempty
// (row 6's shape), an update whose name is a pointer tagged omitempty, whose
// title lacks it and whose tags are a slice tagged omitempty, a list whose
// options model no entity filter, a widget that models no archived, a list
// of gadgets that takes no options struct, a service whose one implemented
// method sends the widget list and none the archive, and the request
// constructors, the upload's options variadic. A tilde stands for a
// backtick.
const waitsSDK = `package gitlab

import "net/http"

type Request struct {
	Method string
}

func (r *Request) Clone() *Request { return r }

func (r *Request) Do(req, v any) {}

type RequestOptionFunc func(*Request) error

func WithContext(ctx any) RequestOptionFunc { return nil }

func WithHeader(name, value string) RequestOptionFunc { return nil }

type Response struct{}

type SetWidgetFlagOptions struct {
	Value any    ~json:"value"~
	Key   string ~json:"key"~
	Group string ~json:"group"~
}

type UpdateWidgetOptions struct {
	Name  *string  ~json:"name,omitempty"~
	Title *string  ~json:"title"~
	Tags  []string ~json:"tags,omitempty"~
}

type ListWidgetsOptions struct {
	Search *string ~url:"search,omitempty" json:"search,omitempty"~
}

type Widget struct {
	ID   int64  ~json:"id"~
	Name string ~json:"name"~
}

type WidgetsServiceInterface interface {
	SetWidgetFlag(name string, opt *SetWidgetFlagOptions, options ...RequestOptionFunc) (*Widget, *Response, error)
	UpdateWidget(id int64, opt *UpdateWidgetOptions, options ...RequestOptionFunc) (*Widget, *Response, error)
	ListWidgets(opt *ListWidgetsOptions, options ...RequestOptionFunc) ([]*Widget, *Response, error)
	ListGadgets(filter any, options ...RequestOptionFunc) ([]*Widget, *Response, error)
	ArchiveWidget(id int64, options ...RequestOptionFunc) (*Response, error)
}

type WidgetsService struct {
	client *Client
}

func (s *WidgetsService) ListWidgets(opt *ListWidgetsOptions, options ...RequestOptionFunc) ([]*Widget, *Response, error) {
	req, err := s.client.NewRequest(http.MethodGet, "widgets", opt, options)
	if err != nil {
		return nil, nil, err
	}
	var widgets []*Widget
	resp, err := s.client.Do(req, &widgets)
	return widgets, resp, err
}

type Client struct {
	Widgets WidgetsServiceInterface
}

func (c *Client) NewRequest(method, path string, opt any, options []RequestOptionFunc) (*Request, error) {
	return &Request{Method: method}, nil
}

func (c *Client) UploadRequest(method, path string, content any, filename, uploadType string, opt any, options ...RequestOptionFunc) (*Request, error) {
	return &Request{Method: method}, nil
}

func (c *Client) Do(req *Request, v any) (*Response, error) { return nil, nil }

var _ = http.MethodGet
`

// waitsToolutil is the stand-in shared package: the body edit, its null
// helper and a function that builds one, three request options, an
// unexported one no tool package can call, which is no site, and a function
// that returns none.
const waitsToolutil = `package toolutil

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"

type BodyEdit struct {
	Set  map[string]any
	Drop []string
}

func NullWhenZero[T comparable](value *T, key string, edit *BodyEdit) *T {
	var zero T
	if value == nil || *value != zero {
		return value
	}
	if edit.Set == nil {
		edit.Set = map[string]any{}
	}
	edit.Set[key] = nil
	return nil
}

func EditJSONBody(edit BodyEdit) gl.RequestOptionFunc { return nil }

func PresetEdit() BodyEdit { return BodyEdit{Set: map[string]any{"name": nil}} }

func TimestampParam(name string) gl.RequestOptionFunc { return nil }

func Paginate() gl.RequestOptionFunc { return nil }

func Continue() gl.RequestOptionFunc { return nil }

func nullOption() gl.RequestOptionFunc { return Continue() }

func Do(req, v any) error { return nil }

func Helper() int { return 1 }

const Version = 1
`

// waitsTool is the tool package that carries one workaround of each form,
// the way the real tree carries it, and functions that carry none but read
// like one to a looser walk. Two raw requests decode into a struct that
// embeds client-go's widget and reads archived beside it, one through a
// generic function that forwards its verb and its options, whose callers
// pass a struct that embeds client-go's update options and writes shade
// beside them; the request options are handed to the methods they page.
const waitsTool = `package widgets

import (
	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"
	"example.com/fixture/internal/toolutil"
)

const flagRoute = "features/"

type lister struct{}

var (
	newRequest              = (*gl.Client).NewRequest
	newUpload               = (*gl.Client).UploadRequest
	notAConstructor, answer = (*gl.Client).Do, 2
	withContext             = gl.WithContext
	bound                   = (&gl.Client{}).NewRequest
)

var always gl.RequestOptionFunc = func(*gl.Request) error { return nil }

func SetFlag(c *gl.Client, name string, value bool) error {
	req, err := c.NewRequest("POST", flagRoute+name, map[string]any{"value": value}, nil)
	if err != nil {
		return err
	}
	_, err = c.Do(req.Clone(), nil)
	return err
}

func Archive(c *gl.Client, method string) error {
	_, err := newRequest(c, method, "widgets/archive", nil, nil)
	return err
}

func importWidgets(c *gl.Client, opts *gl.ListWidgetsOptions) error {
	_, err := c.UploadRequest("POST", "widgets/import", nil, "widgets.json", "file", opts)
	return err
}

func importThroughSeam(c *gl.Client, opts *gl.ListWidgetsOptions) error {
	_, err := newUpload(c, "POST", "widgets/import", nil, "widgets.json", "file", opts)
	return err
}

type widgetAPI struct {
	gl.Widget
	Archived bool ~json:"archived"~
}

type widgetBody struct {
	*gl.UpdateWidgetOptions
	Shade string ~json:"shade"~
}

func (lister) Fetch(c *gl.Client) error {
	req, err := c.NewRequest("GET", "widgets", (*gl.ListWidgetsOptions)(nil), nil)
	if err != nil {
		return err
	}
	var widgets []*widgetAPI
	_, err = c.Do(req, &widgets)
	return err
}

func doWidget[T any](c *gl.Client, method, path string, opts any) (T, error) {
	var out T
	req, err := c.NewRequest(method, path, opts, nil)
	if err != nil {
		return out, err
	}
	_, err = c.Do(req, &out)
	return out, err
}

func sendWidget(c *gl.Client, opts any) error {
	_, err := c.NewRequest("PATCH", "widgets", opts, nil)
	return err
}

func PatchWidget(c *gl.Client) error { return sendWidget(c, &widgetBody{}) }

func ArchiveNow(c *gl.Client) error { return Archive(c, "POST") }

func verbFirst(method string, c *gl.Client) error {
	_, err := c.NewRequest(method, "widgets/3", nil, nil)
	return err
}

func VerbCaller(c *gl.Client) error { return verbFirst("DELETE", c) }

func optsFirst(opts any, c *gl.Client) error {
	_, err := c.NewRequest("PUT", "widgets/4", opts, nil)
	return err
}

func OptsCaller(c *gl.Client) error { return optsFirst(&widgetBody{}, c) }

func Reshade(c *gl.Client) error {
	_, err := c.NewRequest("PUT", "widgets/5", &widgetBody{Shade: "blue"}, nil)
	return err
}

func GetWidget(c *gl.Client) error {
	_, err := doWidget[widgetAPI](c, "GET", "widgets/1", (*gl.ListWidgetsOptions)(nil))
	return err
}

func PostWidget(c *gl.Client, verb string, opts *gl.UpdateWidgetOptions) error {
	if _, err := doWidget[[]*gl.Widget](c, "POST", "widgets", opts); err != nil {
		return err
	}
	if _, err := doWidget[*gl.Widget](c, "PUT", "widgets/1", &widgetBody{Shade: "red"}); err != nil {
		return err
	}
	_, err := doWidget[*gl.Widget](c, verb, "widgets/2", nil)
	return err
}

func Gadgets(c *gl.Client) error {
	_, _, err := c.Widgets.ListGadgets(nil, toolutil.Paginate())
	return err
}

func Hint() string { return "send title on every update" }

func entityFilter(kind string) gl.RequestOptionFunc {
	return func(req *gl.Request) error {
		req.Method = kind
		return nil
	}
}

func headers() gl.RequestOptionFunc { return gl.WithHeader("X-Widget", "1") }

func twoFilters() []gl.RequestOptionFunc {
	return []gl.RequestOptionFunc{
		func(*gl.Request) error { return nil },
		func(*gl.Request) error { return nil },
	}
}

// BodyEdit and EditJSONBody share the shared package's names and are not it.
type BodyEdit struct{ Set map[string]any }

func EditJSONBody(edit BodyEdit) error { return nil }

func List(c *gl.Client) error {
	_, _, err := c.Widgets.ListWidgets(nil, withContext(nil), gl.WithContext(nil), entityFilter("x"), headers(), always)
	return err
}

func Update(c *gl.Client, name *string) error {
	var edit toolutil.BodyEdit
	opts := updateOptions(name, &edit)
	edit.Set = map[string]any{"tags": []any{}}
	_, _, err := c.Widgets.UpdateWidget(1, opts, toolutil.EditJSONBody(edit))
	return err
}

func updateOptions(name *string, edit *toolutil.BodyEdit) *gl.UpdateWidgetOptions {
	return &gl.UpdateWidgetOptions{Name: toolutil.NullWhenZero(name, "name", edit)}
}

func Rename(c *gl.Client, keep bool, key string) error {
	var unset []string
	if !keep {
		unset = append(unset, "title")
	}
	unset = append(unset, key)
	others := append([]string{}, "ignored")
	others = append(others, "kept")
	_ = copy(others, unset)
	_ = toolutil.BodyEdit{Drop: nil}
	edit := toolutil.BodyEdit{Drop: unset}
	edit.Drop = others[:0]
	_, _, err := c.Widgets.UpdateWidget(1, nil, toolutil.EditJSONBody(edit))
	return err
}

func Reset(c *gl.Client) error {
	edit := toolutil.BodyEdit{Drop: []string{"title"}, Set: map[string]any{"name": nil}}
	pointer := &edit
	pointer.Set = map[string]any{"title": nil}
	var err error
	err, edit.Set = errAndSet()
	_ = toolutil.BodyEdit{nil, nil}
	var unrelated struct{ Set map[string]any }
	unrelated.Set = map[string]any{"unrelated": 1}
	local := BodyEdit{Set: map[string]any{"local": 1}}
	local.Set = map[string]any{"local": 2}
	_ = local
	_, _, err = c.Widgets.UpdateWidget(1, nil, toolutil.EditJSONBody(edit))
	return err
}

func errAndSet() (error, map[string]any) { return nil, nil }

func pair(a, b string) string { return a + b }

func Stamp(c *gl.Client) error {
	_, _, err := c.Widgets.ListWidgets(nil, toolutil.TimestampParam("at"), toolutil.Continue())
	return err
}

type pairDoer struct{}

func (pairDoer) Do(req, v any) {}

type singleDoer struct{}

func (singleDoer) Do(v any) {}

func (lister) option() gl.RequestOptionFunc { return nil }

func Plain(c *gl.Client) int {
	var into widgetAPI
	_, _, _ = c.Widgets.ListWidgets(nil, lister{}.option())
	_ = lister{}.Fetch(c)
	_, _ = (*gl.Client).Do(c, nil, &into)
	(&gl.Request{}).Do(nil, &into)
	pairDoer{}.Do(nil, &into)
	singleDoer.Do(singleDoer{}, &into)
	_ = toolutil.Do(nil, &into)
	_ = pair("a", "b")
	_ = EditJSONBody(BodyEdit{})
	return toolutil.Helper() + answer
}
`

// waitsFixtureFeature is the e2e fixture: a raw request of row 6's shape and
// a probe that reaches every branch of the syntax walk. It is read for its
// syntax alone, as the e2e build tag leaves it out of every load.
const waitsFixtureFeature = `//go:build e2e

package fixture

import "net/http"

type server struct{}

func setFeature(e *Env, name string) error {
	req, err := e.Client().GL().NewRequest(http.MethodPost, "features/"+name, map[string]any{"value": true}, nil)
	if err != nil {
		return err
	}
	_, err = e.Client().GL().Do(req, nil)
	return err
}

func probe(c *Client, method, path string, query any) {
	_, _ = c.GL().UploadRequest("PUT", path, nil, "f", "t", query, nil)
	_, _ = c.GL().NewRequest(method, path, nil, nil)
	_, _ = c.GL().NewRequest(5, path, nil, nil)
	_, _ = c.GL().NewRequest(~bad~)
	_, _ = c.GL().NewRequest("GET", path)
	_, _ = GL().NewRequest(http.Get, path, nil, nil)
	_, _ = Other().NewRequest("GET", path, nil, nil)
	_, _ = c.Other().NewRequest("GET", path, nil, nil)
	_, _ = c.GL().Do(nil, nil)
	_, _ = c.client.NewRequest("GET", path, nil, nil)
	_, _ = c.GL()[0].NewRequest("GET", path, nil, nil)
	helper()
}

func (s *server) handle() {}

func declared()
`

// waitsScenario is an e2e scenario file, a test file of a package of
// test/e2e/gitlab, with a raw request of the shape the model registry's
// file upload sends.
const waitsScenario = `//go:build e2e

package common

import "net/http"

func uploadWidgetFile(e *Env) {
	req, _ := e.Client().GL().NewRequest(http.MethodGet, "widgets/1/files/a", nil, nil)
	_ = req
}
`

// waitsRegister is the stand-in register, with the two rows the fixture's
// entries wait on, each linking to the section that names what its entries
// are about.
const waitsRegister = "| # | Project | Title |\n" +
	"| - | ------- | ----- |\n" +
	"| 6 | client-go | [`SetWidgetFlagOptions` writes its scope every call](#setwidgetflagoptions-writes-its-scope-every-call) |\n" +
	"| 121 | client-go | [`UpdateWidgetOptions` cannot clear a field](#updatewidgetoptions-cannot-clear-a-field) |\n" +
	"\n" +
	"### `SetWidgetFlagOptions` writes its scope every call\n" +
	"\n" +
	"`ListWidgetsOptions` has no entity filter, no method sends `POST /widgets/archive` or uploads a file (uploadWidgetFile), " +
	"a `Widget` reads no archived and `Widgets.ListGadgets` takes no options.\n" +
	"\n" +
	"### `UpdateWidgetOptions` cannot clear a field\n" +
	"\n" +
	"Its title is written on every call.\n"

// The "v3.38.0" states of the stand-in, each a set of changes client-go
// makes.
var (
	// flagKeyGainsOmitempty gives one of the flag's two scope fields
	// omitempty, models the entity filter and the widget's archived, adds
	// the method that sends the archive, gives the gadget list an options
	// struct, and makes the tags a pointer to a list, which can send one
	// empty.
	flagKeyGainsOmitempty = strings.NewReplacer(
		"Key   string ~json:\"key\"~", "Key   string ~json:\"key,omitempty\"~",
		"Search *string ~url:\"search,omitempty\" json:\"search,omitempty\"~", "Search *string ~url:\"search,omitempty\" json:\"search,omitempty\"~\n\tEntityType *string ~url:\"entity_type,omitempty\"~",
		"Tags  []string ~json:\"tags,omitempty\"~", "Tags  *[]string ~json:\"tags,omitempty\"~",
		"Name string ~json:\"name\"~\n}", "Name string ~json:\"name\"~\n\tArchived bool ~json:\"archived\"~\n}",
		"ListGadgets(filter any,", "ListGadgets(filter *ListWidgetsOptions,",
		"var _ = http.MethodGet\n", "func (s *WidgetsService) ArchiveWidget(id int64, options ...RequestOptionFunc) (*Response, error) {\n\treq, err := s.client.NewRequest(http.MethodPost, \"widgets/archive\", nil, options)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn s.client.Do(req, nil)\n}\n",
	)
	// flagScopesGainOmitempty gives both scope fields omitempty.
	flagScopesGainOmitempty = strings.NewReplacer(
		"Key   string ~json:\"key\"~", "Key   string ~json:\"key,omitempty\"~",
		"Group string ~json:\"group\"~", "Group string ~json:\"group,omitempty\"~",
	)
	// anyPathGained adds a method that sends whatever path it is handed,
	// which the reading of client-go folds to no route.
	anyPathGained = strings.NewReplacer(
		"var _ = http.MethodGet\n", "func (s *WidgetsService) FetchAny(path string, options ...RequestOptionFunc) (*Response, error) {\n\treq, err := s.client.NewRequest(http.MethodPost, path, nil, options)\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn s.client.Do(req, nil)\n}\n",
	)
)

// fixtureWaits are the entries that answer every workaround of the stand-in
// tree, the way the real table answers the tree's.
func fixtureWaits() []waits.Entry {
	return []waits.Entry{
		{
			Row: 6, Category: waits.CategoryUpstream,
			Sites:    []string{"widgets.SetFlag", "fixture.setFeature"},
			Replaces: []string{"Widgets.SetWidgetFlag", "Widgets"},
			Predicates: []waits.Predicate{
				waits.WritesAlways{Type: "SetWidgetFlagOptions", Field: "Key", Encoder: waits.EncoderJSON},
				waits.WritesAlways{Type: "SetWidgetFlagOptions", Field: "Group", Encoder: waits.EncoderJSON},
			},
			Reason: "the scope fields lack omitempty",
		},
		{
			Row: 6, Category: waits.CategoryUpstream,
			Sites:      []string{"widgets.entityFilter"},
			Predicates: []waits.Predicate{waits.LacksKey{Type: "ListWidgetsOptions", Key: "entity_type"}},
			Reason:     "the list models no entity filter",
		},
		{
			Row: 6, Category: waits.CategoryUpstream,
			Sites:      []string{"widgets.Archive", "widgets.verbFirst", "widgets.importWidgets", "widgets.importThroughSeam", "widgets.lister.Fetch", "widgets.headers", "widgets.always", "widgets.twoFilters", "toolutil.TimestampParam", "common.uploadWidgetFile"},
			Predicates: []waits.Predicate{waits.NoMethodFor{Route: "POST /widgets/archive"}},
			Reason:     "no method sends the archive",
		},
		{
			Row: 6, Category: waits.CategoryUpstream,
			Sites: []string{"widgets.lister.Fetch", "widgets.doWidget", "widgets.sendWidget", "widgets.optsFirst", "widgets.Reshade"},
			Predicates: []waits.Predicate{
				waits.LacksKey{Type: "Widget", Key: "archived"},
				waits.LacksKey{Type: "UpdateWidgetOptions", Key: "shade"},
			},
			Reason: "the widget models no archived and the update no shade",
		},
		{
			Row: 121, Category: waits.CategoryUpstream,
			Sites: []string{"widgets.Update", "widgets.Rename", "widgets.Reset"},
			Predicates: []waits.Predicate{
				waits.NotNullable{Type: "UpdateWidgetOptions", Field: "Name"},
				waits.WritesAlways{Type: "UpdateWidgetOptions", Field: "Title", Encoder: waits.EncoderJSON},
				waits.NoEmptyList{Type: "UpdateWidgetOptions", Field: "Tags"},
			},
			Reason: "the update cannot clear what the edit clears",
		},
		{
			Row: 121, Category: waits.CategoryUpstream,
			Unseen:     []string{"widgets.Hint"},
			Predicates: []waits.Predicate{waits.WritesAlways{Type: "UpdateWidgetOptions", Field: "Title", Encoder: waits.EncoderJSON}},
			Reason:     "the hint tells the caller to send the title",
		},
		{
			Row: 6, Category: waits.CategoryUpstream,
			Sites:      []string{"toolutil.Paginate"},
			Predicates: []waits.Predicate{waits.TakesNoOptions{Method: "Widgets.ListGadgets"}},
			Reason:     "the gadget list takes no options to page with",
		},
		{Category: waits.CategoryProbe, Sites: []string{"fixture.probe"}, Reason: "a probe"},
		{Category: waits.CategoryPagination, Sites: []string{"toolutil.Continue"}, Reason: "paging"},
	}
}

// waitsFixture is one stand-in tree, loaded.
type waitsFixture struct {
	root     string
	pkgs     []*packages.Package
	clientGo *types.Package
	usage    map[string]*shared.ServiceUsage
}

// newWaitsFixture writes the stand-in module with the SDK put through sdk
// (nil keeps the "v3.30.0" state) and loads it.
func newWaitsFixture(t *testing.T, sdk *strings.Replacer) waitsFixture {
	t.Helper()
	return newWaitsFixtureWith(t, sdk, nil)
}

// newWaitsFixtureWith is newWaitsFixture with extra files written beside the
// stand-in's own.
func newWaitsFixtureWith(t *testing.T, sdk *strings.Replacer, extra map[string]string) waitsFixture {
	t.Helper()
	source := waitsSDK
	if sdk != nil {
		source = sdk.Replace(source)
	}
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.27\n",
		"gitlab.com/gitlab-org/api/client-go/v3/sdk.go": strings.ReplaceAll(source, "~", "`"),
		"internal/toolutil/toolutil.go":                 waitsToolutil,
		"internal/tools/widgets/widgets.go":             strings.ReplaceAll(waitsTool, "~", "`"),
		fixtureDir + "/feature.go":                      strings.ReplaceAll(waitsFixtureFeature, "~", "`"),
		fixtureDir + "/feature_test.go":                 "package fixture\n\nfunc testOnly(c *Client) { _, _ = c.GL().NewRequest(\"GET\", \"x\", nil, nil) }\n",
		fixtureDir + "/README.txt":                      "c.GL().NewRequest is not Go here\n",
		fixtureDir + "/nested/nested.go":                "package nested\n\nfunc deeper(c *Client) { _, _ = c.GL().NewRequest(\"GET\", \"x\", nil, nil) }\n",
		scenarioDir + "/common/widgets_test.go":         waitsScenario,
		scenarioDir + "/ee/deeper/quiet_test.go":        "//go:build e2e\n\npackage deeper\n\nfunc quiet() {}\n",
		scenarioDir + "/README.md":                      "e.Client().GL().NewRequest is not Go here\n",
		waits.RegisterPath:                              waitsRegister,
	}
	maps.Copy(files, extra)
	writeFiles(t, root, files)
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load the fixture: %v", err)
	}
	clientGo, err := shared.ClientGoTypes(pkgs)
	if err != nil {
		t.Fatalf("client-go types: %v", err)
	}
	return waitsFixture{root: root, pkgs: pkgs, clientGo: clientGo, usage: shared.CollectServiceUsage(pkgs)}
}

// analyze builds the section over the fixture with entries, reading the
// register and the e2e fixture under root.
func (f waitsFixture) analyze(t *testing.T, root string, entries []waits.Entry) (upstreamWaitsReport, []string) {
	t.Helper()
	report, stale, err := analyzeUpstreamWaits(root, f.pkgs, f.clientGo, f.usage, entries)
	if err != nil {
		t.Fatalf("analyzeUpstreamWaits: %v", err)
	}
	return report, stale
}

// entryStatuses lists each entry's label and status, in table order.
func entryStatuses(report upstreamWaitsReport) []string {
	out := make([]string, 0, len(report.Entries))
	for _, entry := range report.Entries {
		out = append(out, (waits.Entry{Row: entry.Row, Category: entry.Category}).Label()+" "+entry.Status)
	}
	return out
}

// TestBuildUpstreamWaits_AStandInTree_AnswersEveryWorkaroundAndHoldsEveryPredicate
// verifies the whole section on a tree whose universe is known: every raw
// request (built on the client, through a package variable that holds the
// constructor, by a method, or by an upload whose options are variadic), every
// request option written by hand (a literal, a constructor client-go exports
// other than WithContext, a package variable holding one, and the toolutil
// functions that return one but EditJSONBody), every body edit with the keys
// it writes (a null read off the helper it calls, a map set on it or on a
// pointer to it, the names appended to its Drop or listed in place) and every
// raw request the e2e fixture and an e2e scenario build through a GL()
// accessor is found and answered, and every predicate holds on the
// "v3.30.0" stand-in. A raw request records the struct its answer is decoded
// into and the one it sends when either embeds a client-go struct, with the
// predicate each key beside it needs, a type parameter read through every
// instantiation of its function; a function that forwards its verb or its
// options records each call of it with what the request then carries; and a
// request option records the methods it is handed to. What looks like one to
// a looser walk (a method of another struct, a method expression that builds
// nothing, WithContext held in a variable, a call of a toolutil option) is
// not found.
func TestBuildUpstreamWaits_AStandInTree_AnswersEveryWorkaroundAndHoldsEveryPredicate(t *testing.T) {
	fixture := newWaitsFixture(t, nil)
	report, stale := fixture.analyze(t, fixture.root, fixtureWaits())

	row6, row121, probe := []string{"row 6"}, []string{"row 121"}, []string{"probe"}
	listWidgets := []string{"Widgets.ListWidgets"}
	widgetDecode := []waits.Superset{{Type: "widgets.widgetAPI", Root: "Widget", Keys: []string{"archived"}, Needs: []string{"LacksKey{Widget, archived}"}}}
	widgetBody := []waits.Superset{{Type: "widgets.widgetBody", Root: "UpdateWidgetOptions", Keys: []string{"shade"}, Needs: []string{"LacksKey{UpdateWidgetOptions, shade}"}}}
	fixtureNewRequest := workaround{Form: formFixtureRawRequest, Site: "fixture.probe", Constructor: "NewRequest", AnsweredBy: probe}
	want := []workaround{
		{Form: formScenarioRawRequest, Site: "common.uploadWidgetFile", Constructor: "NewRequest", Verb: "GET", AnsweredBy: row6},
		fixtureNewRequest, fixtureNewRequest, fixtureNewRequest, fixtureNewRequest, fixtureNewRequest,
		{Form: formFixtureRawRequest, Site: "fixture.probe", Constructor: "UploadRequest", Verb: "PUT", Options: "query", AnsweredBy: probe},
		{Form: formFixtureRawRequest, Site: "fixture.setFeature", Constructor: "NewRequest", Verb: "POST", Options: "map[string]any{…}", AnsweredBy: row6},
		{Form: formRequestOption, Site: "toolutil.Continue", PassedTo: listWidgets, AnsweredBy: []string{"pagination"}},
		{Form: formRequestOption, Site: "toolutil.Paginate", PassedTo: []string{"Widgets.ListGadgets"}, AnsweredBy: row6},
		{Form: formRequestOption, Site: "toolutil.TimestampParam", PassedTo: listWidgets, AnsweredBy: row6},
		{Form: formRawRequest, Site: "widgets.Archive", Constructor: "NewRequest", Callers: []rawCaller{{Site: "widgets.ArchiveNow", Verb: "POST"}}, AnsweredBy: row6},
		{Form: formBodyEdit, Site: "widgets.Rename", Keys: []string{"title"}, AnsweredBy: row121},
		{Form: formBodyEdit, Site: "widgets.Reset", Keys: []string{"name", "title"}, AnsweredBy: row121},
		{Form: formRawRequest, Site: "widgets.Reshade", Constructor: "NewRequest", Verb: "PUT", Options: "*widgets.widgetBody", Sends: widgetBody, AnsweredBy: row6},
		{Form: formRawRequest, Site: "widgets.SetFlag", Constructor: "NewRequest", Verb: "POST", Options: "map[string]any", AnsweredBy: row6},
		{Form: formBodyEdit, Site: "widgets.Update", Keys: []string{"name", "tags"}, AnsweredBy: row121},
		{Form: formRequestOption, Site: "widgets.always", AnsweredBy: row6},
		{
			Form: formRawRequest, Site: "widgets.doWidget", Constructor: "NewRequest", Options: "any",
			Callers: []rawCaller{
				{Site: "widgets.GetWidget", Verb: "GET", Options: "*gitlab.ListWidgetsOptions"},
				{Site: "widgets.PostWidget"},
				{Site: "widgets.PostWidget", Verb: "POST", Options: "*gitlab.UpdateWidgetOptions"},
				{Site: "widgets.PostWidget", Verb: "PUT", Options: "*widgets.widgetBody"},
			},
			Decodes:    widgetDecode,
			Sends:      widgetBody,
			AnsweredBy: row6,
		},
		{Form: formRequestOption, Site: "widgets.entityFilter", PassedTo: listWidgets, AnsweredBy: row6},
		{Form: formRequestOption, Site: "widgets.headers", PassedTo: listWidgets, AnsweredBy: row6},
		{Form: formRawRequest, Site: "widgets.importThroughSeam", Constructor: "UploadRequest", Verb: "POST", Options: "*gitlab.ListWidgetsOptions", UploadField: "file", AnsweredBy: row6},
		{Form: formRawRequest, Site: "widgets.importWidgets", Constructor: "UploadRequest", Verb: "POST", Options: "*gitlab.ListWidgetsOptions", UploadField: "file", AnsweredBy: row6},
		{Form: formRawRequest, Site: "widgets.lister.Fetch", Constructor: "NewRequest", Verb: "GET", Options: "*gitlab.ListWidgetsOptions", Decodes: widgetDecode, AnsweredBy: row6},
		{
			Form: formRawRequest, Site: "widgets.optsFirst", Constructor: "NewRequest", Verb: "PUT", Options: "any",
			Callers:    []rawCaller{{Site: "widgets.OptsCaller", Verb: "PUT", Options: "*widgets.widgetBody"}},
			Sends:      widgetBody,
			AnsweredBy: row6,
		},
		{
			Form: formRawRequest, Site: "widgets.sendWidget", Constructor: "NewRequest", Verb: "PATCH", Options: "any",
			Callers:    []rawCaller{{Site: "widgets.PatchWidget", Verb: "PATCH", Options: "*widgets.widgetBody"}},
			Sends:      widgetBody,
			AnsweredBy: row6,
		},
		{Form: formRequestOption, Site: "widgets.twoFilters", AnsweredBy: row6},
		{Form: formRawRequest, Site: "widgets.verbFirst", Constructor: "NewRequest", Callers: []rawCaller{{Site: "widgets.VerbCaller", Verb: "DELETE"}}, AnsweredBy: row6},
	}
	if !reflect.DeepEqual(report.Workarounds, want) {
		t.Errorf("workarounds =\n%+v\nwant\n%+v", report.Workarounds, want)
	}
	if len(stale) != 0 {
		t.Errorf("stale = %q, want none", stale)
	}
	wantStatuses := []string{"row 6 holds", "row 6 holds", "row 6 holds", "row 6 holds", "row 121 holds", "row 121 holds", "row 6 holds", "probe declared", "pagination declared"}
	if got := entryStatuses(report); !reflect.DeepEqual(got, wantStatuses) {
		t.Errorf("statuses = %q, want %q", got, wantStatuses)
	}
	var summary reportSummary
	summarizeUpstreamWaits(&summary, report)
	wantSummary := reportSummary{UpstreamWaits: 9, WaitsHolding: 7, Workarounds: len(want)}
	if summary != wantSummary || !summary.clean() {
		t.Errorf("summary = %+v, want %+v and a clean gate", summary, wantSummary)
	}
	if gaps := keepWaitFindings(report); len(gaps.Entries) != 0 || len(gaps.Workarounds) != 0 {
		t.Errorf("gaps-only kept %+v, want nothing", gaps)
	}
}

// TestBuildUpstreamWaits_AClientGoThatFixesThings_RetiresOrEditsEachEntry is
// row 6's case on the stand-in, with the other predicates' beside it. With
// one of the flag's two scope fields gaining omitempty the entry has a
// predicate to edit, and with both it is a workaround to retire, naming its
// sites. An option struct that models the filter, a method that sends the
// archive and a gadget list that gains options retire their entries; a list
// that can send [] leaves the body edit's entry a predicate to edit, and so
// does a widget that models archived while the update still models no
// shade. The archived the decode still reads beside the widget is then
// read for nothing, which both raw reads of it report.
func TestBuildUpstreamWaits_AClientGoThatFixesThings_RetiresOrEditsEachEntry(t *testing.T) {
	t.Run("one scope field gains omitempty", func(t *testing.T) {
		fixture := newWaitsFixture(t, flagKeyGainsOmitempty)
		report, stale := fixture.analyze(t, fixture.root, fixtureWaits())
		want := []string{
			"row 6 stale_predicate", "row 6 obsolete_workaround", "row 6 obsolete_workaround", "row 6 stale_predicate",
			"row 121 stale_predicate", "row 121 holds", "row 6 obsolete_workaround", "probe declared", "pagination declared",
		}
		if got := entryStatuses(report); !reflect.DeepEqual(got, want) {
			t.Errorf("statuses = %q, want %q", got, want)
		}
		if len(stale) != 0 {
			t.Errorf("stale = %q, want none", stale)
		}
		gaps := keepWaitFindings(report)
		var shadowed []string
		for _, found := range gaps.Workarounds {
			shadowed = append(shadowed, found.Site+": "+strings.Join(found.UnansweredNeeds, ", "))
		}
		wantShadowed := []string{
			"widgets.doWidget: shadows archived, which Widget models as bool",
			"widgets.lister.Fetch: shadows archived, which Widget models as bool",
		}
		if len(gaps.Entries) != 6 || !reflect.DeepEqual(shadowed, wantShadowed) {
			t.Errorf("gaps-only kept %d entries and workarounds %q, want 6 and %q", len(gaps.Entries), shadowed, wantShadowed)
		}
		var summary reportSummary
		summarizeUpstreamWaits(&summary, report)
		if summary.WaitsStalePredicate != 3 || summary.WaitsObsolete != 3 || summary.WaitsHolding != 1 || summary.SupersetNeedsUnanswered != 2 || summary.clean() {
			t.Errorf("summary = %+v, want three predicates to edit, three workarounds to retire, two keys read for nothing and a failing gate", summary)
		}
	})
	t.Run("both scope fields gain omitempty", func(t *testing.T) {
		fixture := newWaitsFixture(t, flagScopesGainOmitempty)
		report, _ := fixture.analyze(t, fixture.root, fixtureWaits())
		first := report.Entries[0]
		if first.Status != waitObsolete || !slices.Equal(first.Sites, []string{"widgets.SetFlag", "fixture.setFeature"}) {
			t.Errorf("row 6 = %+v, want obsolete_workaround naming widgets.SetFlag and fixture.setFeature", first)
		}
	})
}

// TestAnalyzeUpstreamWaits_AClientGoRequestWhosePathIsNotRead_LeavesNoRouteUnsent
// verifies that a request of client-go whose path the reading folds to no
// route is handed to the predicates beside the routes it read: it could be
// the archive no method sends, so the entry waiting on that route does not
// hold, and its verdict names the request rather than calling the route
// unsent.
func TestAnalyzeUpstreamWaits_AClientGoRequestWhosePathIsNotRead_LeavesNoRouteUnsent(t *testing.T) {
	fixture := newWaitsFixture(t, anyPathGained)

	routes, unresolved := sdkRoutes(structs.ClientGoDir(fixture.pkgs))
	if !routes["GET /widgets"] || len(unresolved) != 1 || !strings.HasPrefix(unresolved[0], "WidgetsService.FetchAny: ") {
		t.Fatalf("sdkRoutes = %v, %q, want GET /widgets read and WidgetsService.FetchAny unresolved", routes, unresolved)
	}
	report, _ := fixture.analyze(t, fixture.root, fixtureWaits())
	archive := report.Entries[2]
	if archive.Status == waitHolds || len(archive.Predicates) != 1 || !strings.Contains(archive.Predicates[0].Error, "cannot be told unsent: WidgetsService.FetchAny: ") {
		t.Errorf("the archive's entry = %+v, want a verdict naming WidgetsService.FetchAny rather than the route unsent", archive)
	}
}

// pairsTool is a tool package whose functions build two raw requests each:
// one decoding into client-go's widget and one into a struct that reads
// archived beside it while sending one that writes shade, a variable
// assigned both, and a function forwarding its options to both.
const pairsTool = `package pairs

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"

type widgetAPI struct {
	gl.Widget
	Archived bool ~json:"archived"~
}

type widgetBody struct {
	*gl.UpdateWidgetOptions
	Shade string ~json:"shade"~
}

func Both(c *gl.Client) error {
	plain, err := c.NewRequest("GET", "widgets/1", nil, nil)
	if err != nil {
		return err
	}
	var widget gl.Widget
	if _, err := c.Do(plain, &widget); err != nil {
		return err
	}
	var read, _ = c.NewRequest("PUT", "widgets/2", &widgetBody{}, nil)
	var archived widgetAPI
	_, err = c.Do(read, &archived)
	return err
}

func Reused(c *gl.Client) error {
	req, err := c.NewRequest("GET", "widgets/1", nil, nil)
	if err != nil {
		return err
	}
	req, err = c.NewRequest("GET", "widgets/2", nil, nil)
	if err != nil {
		return err
	}
	var archived widgetAPI
	_, err = c.Do(req, &archived)
	return err
}

func Swapped(c *gl.Client) error {
	first, err := c.NewRequest("GET", "widgets/1", nil, nil)
	if err != nil {
		return err
	}
	second, err := c.NewRequest("GET", "widgets/2", nil, nil)
	if err != nil {
		return err
	}
	first = second
	var archived widgetAPI
	_, err = c.Do(first, &archived)
	return err
}

func Aliased(c *gl.Client) error {
	if _, err := c.NewRequest("GET", "widgets/1", nil, nil); err != nil {
		return err
	}
	built, err := c.NewRequest("GET", "widgets/2", nil, nil)
	if err != nil {
		return err
	}
	var sent = built
	var archived widgetAPI
	_, err = c.Do(sent, &archived)
	return err
}

func twice(c *gl.Client, opts any) error {
	if _, err := c.NewRequest("PUT", "widgets/1", opts, nil); err != nil {
		return err
	}
	_, err := c.NewRequest("PUT", "widgets/2", opts, nil)
	return err
}

func Twice(c *gl.Client) error { return twice(c, &widgetBody{}) }
`

// TestWalkToolWorkarounds_TwoRawRequestsOfOneFunction_EachCarryTheirOwnSupersets
// verifies that what a raw request decodes and sends is read for that
// request, not for every request its function builds: the request a call of
// Do sends is the one the variable it reads was assigned, so a request
// decoding client-go's own widget carries no superset beside one decoding a
// struct that reads archived, and neither does a request sending nothing
// beside one sending a struct that writes shade. A variable assigned two
// requests could send either, so what Do decodes through it is counted
// against both, and so is one assigned a request of its own and then
// another variable's; one assigned only another variable's holds that
// request alone. A function forwarding its options to two requests hands
// each one every call of it.
func TestWalkToolWorkarounds_TwoRawRequestsOfOneFunction_EachCarryTheirOwnSupersets(t *testing.T) {
	fixture := newWaitsFixtureWith(t, nil, map[string]string{"internal/tools/pairs/pairs.go": strings.ReplaceAll(pairsTool, "~", "`")})

	found, _, _ := walkToolWorkarounds(fixture.pkgs, fixture.clientGo)

	decode := []waits.Superset{{Type: "pairs.widgetAPI", Root: "Widget", Keys: []string{"archived"}, Needs: []string{"LacksKey{Widget, archived}"}}}
	body := []waits.Superset{{Type: "pairs.widgetBody", Root: "UpdateWidgetOptions", Keys: []string{"shade"}, Needs: []string{"LacksKey{UpdateWidgetOptions, shade}"}}}
	twiceCaller := []rawCaller{{Site: "pairs.Twice", Verb: "PUT", Options: "*pairs.widgetBody"}}
	want := []workaround{
		{Form: formRawRequest, Site: "pairs.Both", Constructor: "NewRequest", Verb: "GET"},
		{Form: formRawRequest, Site: "pairs.Both", Constructor: "NewRequest", Verb: "PUT", Options: "*pairs.widgetBody", Decodes: decode, Sends: body},
		{Form: formRawRequest, Site: "pairs.Reused", Constructor: "NewRequest", Verb: "GET", Decodes: decode},
		{Form: formRawRequest, Site: "pairs.Reused", Constructor: "NewRequest", Verb: "GET", Decodes: decode},
		{Form: formRawRequest, Site: "pairs.Swapped", Constructor: "NewRequest", Verb: "GET", Decodes: decode},
		{Form: formRawRequest, Site: "pairs.Swapped", Constructor: "NewRequest", Verb: "GET", Decodes: decode},
		{Form: formRawRequest, Site: "pairs.Aliased", Constructor: "NewRequest", Verb: "GET"},
		{Form: formRawRequest, Site: "pairs.Aliased", Constructor: "NewRequest", Verb: "GET", Decodes: decode},
		{Form: formRawRequest, Site: "pairs.twice", Constructor: "NewRequest", Verb: "PUT", Options: "any", Callers: twiceCaller, Sends: body},
		{Form: formRawRequest, Site: "pairs.twice", Constructor: "NewRequest", Verb: "PUT", Options: "any", Callers: twiceCaller, Sends: body},
	}
	var got []workaround
	for _, raw := range found {
		if raw.Form == formRawRequest && strings.HasPrefix(raw.Site, "pairs.") {
			got = append(got, raw)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("raw requests =\n%+v\nwant\n%+v", got, want)
	}
}

// editorsTool is a tool package whose body edits write keys by assignment
// as well as in literals: into the edit's Set and onto its Drop, and into a
// map and a list handed to the edit whole.
const editorsTool = `package editors

import (
	"slices"

	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"
	"example.com/fixture/internal/toolutil"
)

func Clear(c *gl.Client, key string) error {
	edit := toolutil.BodyEdit{Set: map[string]any{"name": nil}}
	edit.Set["title"] = nil
	edit.Set[key] = nil
	edit.Drop = append(edit.Drop, "tags")
	edit.Drop[0] = "ignored"
	_, _, err := c.Widgets.UpdateWidget(1, nil, toolutil.EditJSONBody(edit))
	return err
}

func Built(c *gl.Client) error {
	nulls := map[string]any{}
	nulls["shade"] = nil
	delete(nulls, "gone")
	unset := []string{}
	unset = append(unset, "hue")
	if slices.Contains(unset, "gone") || len(unset) == 0 {
		return nil
	}
	_, _, err := c.Widgets.UpdateWidget(1, nil, toolutil.EditJSONBody(toolutil.BodyEdit{Set: nulls, Drop: unset}))
	return err
}

func Extended(c *gl.Client) error {
	existing := []string{"name"}
	edit := toolutil.BodyEdit{}
	edit.Drop = append(existing, "title")
	_, _, err := c.Widgets.UpdateWidget(1, nil, toolutil.EditJSONBody(edit))
	return err
}

func Seeded(c *gl.Client) error {
	var seeded = map[string]any{"tint": nil}
	nulls := seeded
	_, _, err := c.Widgets.UpdateWidget(1, nil, toolutil.EditJSONBody(toolutil.BodyEdit{Set: nulls}))
	return err
}
`

// TestWalkToolWorkarounds_KeysABodyEditIsAssigned_AreEachRead verifies the
// keys a body edit writes outside a literal: one assigned into its Set
// beside a literal's, so the edit is not taken as answered by the literal's
// alone, one appended onto its Drop, and one assigned into a map or
// appended to a list handed to the edit whole. A Drop appended onto
// another list drops what that list holds too, and a map handed to the edit
// through another variable sets the keys the map was filled with. A key
// that is no constant adds nothing, and neither does a name written over an
// element of the Drop or one a call other than append is handed beside a
// list or a map the edit takes, a call of another builtin included.
func TestWalkToolWorkarounds_KeysABodyEditIsAssigned_AreEachRead(t *testing.T) {
	fixture := newWaitsFixtureWith(t, nil, map[string]string{"internal/tools/editors/editors.go": editorsTool})

	found, _, _ := walkToolWorkarounds(fixture.pkgs, fixture.clientGo)

	got := map[string][]string{}
	for _, edit := range found {
		if edit.Form == formBodyEdit && strings.HasPrefix(edit.Site, "editors.") {
			got[edit.Site] = edit.Keys
		}
	}
	want := map[string][]string{
		"editors.Built":    {"hue", "shade"},
		"editors.Clear":    {"name", "tags", "title"},
		"editors.Extended": {"name", "title"},
		"editors.Seeded":   {"tint"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("body edit keys = %v, want %v", got, want)
	}
}

// presetsTool is a tool package whose body edit is built by a function of
// another package, so the walk cannot read the keys it writes.
const presetsTool = `package presets

import (
	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"
	"example.com/fixture/internal/toolutil"
)

func Preset(c *gl.Client) error {
	_, _, err := c.Widgets.UpdateWidget(1, nil, toolutil.EditJSONBody(toolutil.PresetEdit()))
	return err
}
`

// TestBuildUpstreamWaits_EntriesThatAnswerLess_LeaveTheirWorkaroundsFindings
// verifies the findings of the walk: a workaround no entry lists; a body key
// no predicate of an entry listing the body edit answers; a body edit whose
// keys the walk cannot read, which no predicate can be tied to; a key a raw
// read decodes beside client-go's struct that no predicate of an entry
// listing it explains; and a method a request option is handed to that no
// TakesNoOptions of an entry listing the option names, once one does.
func TestBuildUpstreamWaits_EntriesThatAnswerLess_LeaveTheirWorkaroundsFindings(t *testing.T) {
	fixture := newWaitsFixtureWith(t, nil, map[string]string{"internal/tools/presets/presets.go": presetsTool})
	entries := append(fixtureWaits(), waits.Entry{
		Row: 121, Category: waits.CategoryUpstream, Sites: []string{"presets.Preset"},
		Predicates: []waits.Predicate{waits.NotNullable{Type: "UpdateWidgetOptions", Field: "Name"}}, Reason: "a preset",
	})
	entries[0].Sites = []string{"fixture.setFeature"}
	entries[3].Sites = []string{"widgets.doWidget", "widgets.sendWidget", "widgets.optsFirst", "widgets.Reshade"}
	entries[4].Predicates = entries[4].Predicates[:1]
	entries[6].Predicates = []waits.Predicate{waits.TakesNoOptions{Method: "Widgets.ArchiveWidget"}}
	report, _ := fixture.analyze(t, fixture.root, entries)

	gaps := keepWaitFindings(report)
	got := map[string][]string{}
	for _, found := range gaps.Workarounds {
		var unread []string
		if found.KeysUnread {
			unread = []string{"keys unread"}
		}
		got[found.Site] = slices.Concat(found.UnansweredKeys, found.UnansweredNeeds, found.UnansweredMethods, unread, found.AnsweredBy)
	}
	want := map[string][]string{
		"widgets.SetFlag":      nil,
		"widgets.Rename":       {"title", "row 121"},
		"widgets.Reset":        {"title", "row 121"},
		"widgets.Update":       {"tags", "row 121"},
		"presets.Preset":       {"keys unread", "row 121"},
		"widgets.lister.Fetch": {"LacksKey{Widget, archived}", "row 6"},
		"toolutil.Paginate":    {"Widgets.ListGadgets", "row 6"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("gaps = %v, want %v", got, want)
	}
	var summary reportSummary
	summarizeUpstreamWaits(&summary, report)
	if summary.WorkaroundsUndeclared != 1 || summary.BodyEditKeysUnanswered != 3 || summary.BodyEditsUnread != 1 ||
		summary.SupersetNeedsUnanswered != 1 || summary.OptionMethodsUnanswered != 1 || summary.clean() {
		t.Errorf("summary = %+v, want one undeclared workaround, three unanswered keys, one unread body edit, one unexplained decoded key, one unnamed method and a failing gate", summary)
	}
}

// TestBuildUpstreamWaits_EntriesThatNoLongerDescribeTheTree_AreStale verifies
// every way an entry stops describing the tree: a site that does not exist,
// a site the walk finds nothing at, an unseen site that does not exist or
// that the walk does see, an unseen site that holds no string naming a key
// a field predicate of the entry names, a row the register lacks or whose
// text names nothing the entry is about, a method client-go does not
// declare or a handler calls, a predicate naming what client-go lacks, a
// LacksKey or FieldKind on a superset's root naming a key no superset at the
// entry's sites carries, a TakesNoOptions naming a method no site of the
// entry is handed to, and a shape Problems refuses. A whole service is the
// service rule's to judge and is passed over here, and an upstream entry
// with no row is Problems' to name.
func TestBuildUpstreamWaits_EntriesThatNoLongerDescribeTheTree_AreStale(t *testing.T) {
	fixture := newWaitsFixture(t, nil)
	entries := append(fixtureWaits(),
		waits.Entry{
			Row: 7, Category: waits.CategoryUpstream,
			Sites:      []string{"widgets.Gone", "widgets.Plain"},
			Unseen:     []string{"widgets.Vanished", "widgets.SetFlag", "widgets.Stamp"},
			Replaces:   []string{"Widgets.Gone", "Widgets.ListWidgets", "Gadgets"},
			Predicates: []waits.Predicate{waits.LacksKey{Type: "Gone", Key: "x"}},
			Reason:     "stale on purpose",
		},
		waits.Entry{Category: "decided", Sites: []string{"widgets.Plain"}, Reason: "no such category"},
		waits.Entry{
			Category: waits.CategoryUpstream, Unseen: []string{"widgets.Plain"},
			Predicates: []waits.Predicate{waits.LacksKey{Type: "ListWidgetsOptions", Key: "gone"}}, Reason: "no row",
		},
		waits.Entry{
			Row: 121, Category: waits.CategoryUpstream, Sites: []string{"widgets.Update"},
			Predicates: []waits.Predicate{waits.LacksKey{Type: "Widget", Key: "shade"}}, Reason: "a row about something else",
		},
		waits.Entry{
			Row: 6, Category: waits.CategoryUpstream, Sites: []string{"widgets.lister.Fetch"},
			Predicates: []waits.Predicate{
				waits.LacksKey{Type: "Widget", Key: "archived"},
				waits.LacksKey{Type: "Widget", Key: "shade"},
				waits.FieldKind{Type: "Widget", Field: "ID", Kind: "int64"},
				waits.LacksKey{Type: "ListWidgetsOptions", Key: "shade"},
				waits.TagName{Type: "Widget", Field: "Name", Encoder: waits.EncoderJSON, Want: "title"},
			},
			Reason: "keys the decode does not read",
		},
		waits.Entry{
			Row: 6, Category: waits.CategoryUpstream, Sites: []string{"toolutil.Paginate"},
			Predicates: []waits.Predicate{waits.TakesNoOptions{Method: "Widgets.ListGadgets"}, waits.TakesNoOptions{Method: "Widgets.ArchiveWidget"}},
			Reason:     "a method the option is not handed to",
		},
		waits.Entry{
			Row: 121, Category: waits.CategoryUpstream, Unseen: []string{"widgets.Plain", "widgets.Hint"},
			Predicates: []waits.Predicate{
				waits.NotNullable{Type: "UpdateWidgetOptions", Field: "Name"},
				waits.WritesAlways{Type: "UpdateWidgetOptions", Field: "Title", Encoder: waits.EncoderJSON},
				waits.NoMethodFor{Route: "POST /widgets/archive"},
			},
			Reason: "a hint that names no key",
		},
	)
	_, stale := fixture.analyze(t, fixture.root, entries)
	want := []string{
		"upstream wait row 0 names no register row",
		"wait decided: category \"decided\" is not one the table defines",
		"wait decided: site widgets.Plain carries no workaround the walk finds",
		"wait row 121: row 121 of docs/development/upstream-bugs.md names nothing the entry is about (Widget, widgets.Update)",
		"wait row 121: unseen site widgets.Hint holds no string naming name, the key NotNullable{UpdateWidgetOptions.Name} is about",
		"wait row 121: unseen site widgets.Plain holds no string naming name, the key NotNullable{UpdateWidgetOptions.Name} is about",
		"wait row 121: unseen site widgets.Plain holds no string naming title, the key WritesAlways{UpdateWidgetOptions.Title, json} is about",
		"wait row 6: FieldKind{Widget.ID, int64} names a key no superset at the entry's sites carries beside Widget",
		"wait row 6: LacksKey{Widget, shade} names a key no superset at the entry's sites carries beside Widget",
		"wait row 6: TakesNoOptions{Widgets.ArchiveWidget} names a method no site of the entry is handed to",
		"wait row 7: LacksKey{Gone, x} names what client-go lacks: client-go declares no type Gone",
		"wait row 7: docs/development/upstream-bugs.md has no row 7",
		"wait row 7: replaces Widgets.Gone: client-go's Widgets has no method Gone",
		"wait row 7: replaces Widgets.ListWidgets, which a handler calls",
		"wait row 7: site widgets.Gone no longer exists",
		"wait row 7: site widgets.Plain carries no workaround the walk finds",
		"wait row 7: site widgets.Vanished no longer exists",
		"wait row 7: the walk finds a workaround at widgets.SetFlag, so it belongs among the sites",
	}
	if !reflect.DeepEqual(stale, want) {
		t.Errorf("stale =\n%s\nwant\n%s", strings.Join(stale, "\n"), strings.Join(want, "\n"))
	}

	t.Run("a tree without the register or the e2e fixture", func(t *testing.T) {
		bareReport, bareStale := fixture.analyze(t, t.TempDir(), fixtureWaits())
		var missing []string
		for _, want := range []string{
			"upstream register: read the upstream register",
			"wait row 6: docs/development/upstream-bugs.md has no row 6",
			"wait row 6: site fixture.setFeature no longer exists",
		} {
			if !slices.ContainsFunc(bareStale, func(line string) bool { return strings.HasPrefix(line, want) }) {
				missing = append(missing, want)
			}
		}
		if len(missing) != 0 {
			t.Errorf("stale = %q, want lines starting %q", bareStale, missing)
		}
		if slices.ContainsFunc(bareReport.Workarounds, func(found workaround) bool { return found.Form == formFixtureRawRequest }) {
			t.Error("a tree without the e2e fixture reported a fixture request")
		}
	})
	for _, tc := range []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{name: "an e2e fixture that does not parse", files: map[string]string{fixtureDir + "/broken.go": "package fixture\n\nfunc {\n"}, wantErr: "parse the e2e fixture"},
		{name: "an e2e fixture path that is no directory", files: map[string]string{fixtureDir: "not a package\n"}, wantErr: "read the e2e fixture"},
		{name: "an e2e scenario that does not parse", files: map[string]string{scenarioDir + "/ce/deeper/broken_test.go": "package deeper\n\nfunc {\n"}, wantErr: "parse the e2e scenarios"},
		{name: "an e2e scenario path that is no directory", files: map[string]string{scenarioDir: "not a tree\n"}, wantErr: "read the e2e scenarios"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, tc.files)
			if _, _, err := analyzeUpstreamWaits(root, fixture.pkgs, fixture.clientGo, fixture.usage, fixtureWaits()); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("analyzeUpstreamWaits = %v, want an error naming %q", err, tc.wantErr)
			}
		})
	}
	// A dangling link reads as a directory that does not exist on every
	// platform, which is how Windows reads a regular file too: the tree is
	// there, so the run fails rather than skipping it.
	t.Run("an e2e fixture path that is a dangling link", func(t *testing.T) {
		root := t.TempDir()
		link := filepath.Join(root, filepath.FromSlash(fixtureDir))
		if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "nowhere"), link); err != nil {
			t.Skipf("making a link needs a privilege this platform does not grant: %v", err)
		}
		if _, _, err := analyzeUpstreamWaits(root, fixture.pkgs, fixture.clientGo, fixture.usage, fixtureWaits()); err == nil || !strings.Contains(err.Error(), "cannot be read as a directory") {
			t.Errorf("analyzeUpstreamWaits = %v, want an error saying the fixture cannot be read as a directory", err)
		}
	})
}

// TestWithCoveredRawServices_EntriesReplacingAService_DeclareItCoveredRaw
// verifies the service rule's half of the derivation: a service an entry
// replaces whole is declared COVERED_RAW with the entry's reason, the
// declarations already written are kept, the table handed in is not
// written to, a service declared both ways is a declaration said twice, and
// a COVERED_RAW written into declaredServices by hand, which no entry holds
// to the pinned SDK, is a finding as well.
func TestWithCoveredRawServices_EntriesReplacingAService_DeclareItCoveredRaw(t *testing.T) {
	declaredBy := map[string]declaration{
		"Kept":  {coveredGraphQL, "over GraphQL"},
		"Twice": {supersededUpstream, "replaced upstream"},
		"Hand":  {coveredRaw, "a raw request nothing holds"},
	}
	entries := []waits.Entry{{Row: 7, Category: waits.CategoryUpstream, Replaces: []string{"Stats", "Twice", "Stats.Get"}, Reason: "counts"}}
	got, twice := withCoveredRawServices(declaredBy, entries)
	want := map[string]declaration{
		"Kept":  {coveredGraphQL, "over GraphQL"},
		"Twice": {supersededUpstream, "replaced upstream"},
		"Hand":  {coveredRaw, "a raw request nothing holds"},
		"Stats": {coveredRaw, "row 7: counts"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("declarations = %v, want %v", got, want)
	}
	wantTwice := []string{
		"service Hand (declared COVERED_RAW in declaredServices, which only a wait declares)",
		"service Twice (declared in declaredServices and replaced by a wait)",
	}
	if !slices.Equal(twice, wantTwice) {
		t.Errorf("twice = %q, want %q", twice, wantTwice)
	}
	if _, written := declaredBy["Stats"]; written {
		t.Error("withCoveredRawServices wrote into the table it was handed")
	}
}

// TestSyntaxVerb_Expressions_ReadOnlyAConstantOrAString covers the syntax
// walk's verb reading, and the accessor check's refusal of a call through an
// index expression.
func TestSyntaxVerb_Expressions_ReadOnlyAConstantOrAString(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want string
	}{
		{name: "net/http constant", expr: "http.MethodDelete", want: "DELETE"},
		{name: "another selector", expr: "http.Get"},
		{name: "string", expr: `"PATCH"`, want: "PATCH"},
		{name: "number", expr: "5"},
		{name: "identifier", expr: "method"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parse %s: %v", tc.expr, err)
			}
			if got := syntaxVerb(expr); got != tc.want {
				t.Errorf("syntaxVerb(%s) = %q, want %q", tc.expr, got, tc.want)
			}
		})
	}
	call, err := parser.ParseExpr("x[0]()")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if callsNamed(call.(*ast.CallExpr), glAccessor) {
		t.Error("callsNamed accepted a call of an index expression")
	}
}

// checkGitlab type-checks source as a stand-in client-go root package.
func checkGitlab(t *testing.T, source string) *types.Package {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sdk.go", source, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pkg, err := (&types.Config{}).Check("gitlab", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return pkg
}

// TestArgNamed_ACallThatStopsShort_HandsNothing verifies the argument a call
// hands a constructor's parameter: the one in its place, nothing when the
// call stops before it (a variadic tail left out), on a Client and through a
// package variable holding the method expression, whose first argument is
// the Client and shifts every other by one, and nothing for a parameter the
// constructor does not take, read past every argument.
func TestArgNamed_ACallThatStopsShort_HandsNothing(t *testing.T) {
	pkg := checkGitlab(t, "package gitlab\n\ntype Client struct{}\n\nfunc (c *Client) UploadRequest(method string, opt any, uploadType string, options ...any) {}\n")
	client := pkg.Scope().Lookup("Client").Type().(*types.Named)
	upload := client.Method(0)
	verb, options := ast.NewIdent("verb"), ast.NewIdent("opts")
	short := &ast.CallExpr{Args: []ast.Expr{verb, options}}
	if got := argNamed(upload, short, 0, rawRequestParam); got != options {
		t.Errorf("argNamed(opt) = %v, want the second argument", got)
	}
	if got := argNamed(upload, short, 0, uploadTypeParam); got != nil {
		t.Errorf("argNamed(uploadType) of a call that stops before it = %v, want nothing", got)
	}
	throughVariable := &ast.CallExpr{Args: []ast.Expr{ast.NewIdent("client"), verb, options}}
	if got := argNamed(upload, throughVariable, 1, rawRequestParam); got != options {
		t.Errorf("argNamed(opt) through a package variable = %v, want the third argument", got)
	}
	if got := argNamed(upload, throughVariable, 1, uploadTypeParam); got != nil {
		t.Errorf("argNamed(uploadType) of a call through a package variable that stops before it = %v, want nothing", got)
	}
	full := &ast.CallExpr{Args: []ast.Expr{verb, options, ast.NewIdent("kind"), ast.NewIdent("extra")}}
	if got := argNamed(upload, full, 0, "absent"); got != nil {
		t.Errorf("argNamed(absent) = %v, want nothing", got)
	}
}

// TestWalkGuards_WhatTheTreeCannotProduce_FindNothing covers the guards the
// stand-in tree cannot reach: an SDK that declares no RequestOptionFunc or
// one that is no function type, a function signature or a constructor
// compared with nothing, a method of the universe, a toolutil import that
// carries no types, an untyped expression and a constant that is no string.
func TestWalkGuards_WhatTheTreeCannotProduce_FindNothing(t *testing.T) {
	toolutilImport := func(typed *types.Package) []*packages.Package {
		return []*packages.Package{{Imports: map[string]*packages.Package{"x" + toolutilPathSuffix: {Types: typed}}}}
	}
	// A toolutil with a function in it, so a walk that went on past the
	// guard would name a site.
	toolutilWithAFunc := types.NewPackage("x"+toolutilPathSuffix, "toolutil")
	toolutilWithAFunc.Scope().Insert(types.NewFunc(token.NoPos, toolutilWithAFunc, "Helper", types.NewSignatureType(nil, nil, nil, nil, nil, false)))
	for _, tc := range []struct{ name, source string }{
		{name: "absent", source: "package gitlab\n"},
		{name: "not a function", source: "package gitlab\n\ntype RequestOptionFunc int\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := checkGitlab(t, tc.source)
			if requestOptionFunc(pkg) != nil {
				t.Fatal("requestOptionFunc found one")
			}
			scan := bodyScan{clientGo: pkg}
			if scan.isOptionSignature(types.NewSignatureType(nil, nil, nil, nil, nil, false)) {
				t.Error("a signature was compared with no RequestOptionFunc")
			}
			if scan.isOptionConstructor(types.NewFunc(token.NoPos, pkg, "WithHeader", types.NewSignatureType(nil, nil, nil, nil, nil, false))) {
				t.Error("a constructor was compared with no RequestOptionFunc")
			}
			if found, sites := toolutilWorkarounds(toolutilImport(toolutilWithAFunc), pkg); len(found) != 0 || len(sites) != 0 {
				t.Errorf("toolutilWorkarounds = %v, %v, want nothing", found, sites)
			}
		})
	}

	pkg := checkGitlab(t, "package gitlab\n\ntype RequestOptionFunc func() error\n")
	if found, sites := toolutilWorkarounds(nil, pkg); len(found) != 0 || len(sites) != 0 {
		t.Errorf("toolutilWorkarounds with no toolutil import = %v, %v, want nothing", found, sites)
	}
	scan := bodyScan{clientGo: pkg, optionFunc: requestOptionFunc(pkg)}
	errorMethod := types.Universe.Lookup("error").Type().Underlying().(*types.Interface).Method(0)
	if scan.isOptionSignature(nil) || scan.isOptionConstructor(errorMethod) || inToolutil(errorMethod) {
		t.Error("a missing signature or a method of the universe was read as an option or as toolutil's")
	}
	if importedPackage(toolutilImport(nil), toolutilPathSuffix) != nil {
		t.Error("importedPackage returned an import that carries no types")
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	number := &ast.BasicLit{Kind: token.INT, Value: "1"}
	info.Types[number] = types.TypeAndValue{Type: types.Typ[types.Int], Value: constant.MakeInt64(1)}
	if constantString(info, number) != "" || spellOptions(info, &ast.Ident{Name: "untyped"}) != "" {
		t.Error("a constant that is no string or an expression with no type was spelled")
	}
}

// witnessReport is the part of the sdk report the witness reads, decoded as
// plain JSON so that it decodes a report with no such section too.
type witnessReport struct {
	UpstreamWaits *struct {
		Entries     []witnessEntry
		Workarounds []struct {
			Form       string
			Site       string
			AnsweredBy []string `json:"answered_by"`
		}
	} `json:"upstream_waits"`
}

// witnessEntry is one entry of the section as the witness reads it.
type witnessEntry struct {
	Row        int
	Sites      []string
	Unseen     []string
	Status     string
	Predicates []struct {
		Predicate string
		Holds     bool
	}
}

// checkWitnessEntry holds one entry to the predicates and sites the witness
// expects, every predicate holding.
func checkWitnessEntry(t *testing.T, entry witnessEntry, predicates, sites []string) {
	t.Helper()
	var got []string
	for _, verdict := range entry.Predicates {
		if !verdict.Holds {
			t.Errorf("%s does not hold on the pinned client-go", verdict.Predicate)
		}
		got = append(got, verdict.Predicate)
	}
	if !slices.Equal(got, predicates) {
		t.Errorf("predicates = %q, want %q", got, predicates)
	}
	if listed := slices.Concat(entry.Sites, entry.Unseen); !slices.Equal(listed, sites) || entry.Status != waitHolds {
		t.Errorf("sites %q and status %q, want %q and holds", listed, entry.Status, sites)
	}
}

// TestRun_Repository_HoldsRowSixAndRowFiftyTwoToWhatClientGoV3300Does is
// the witness on the real tree. client-go's SetFeatureFlagOptions gained
// omitempty on its scope fields in v3.34.0, and internal/tools/features.Set
// and the e2e fixture's setFeature went on posting the body by hand with
// nothing to say so; UpdatePackageProtectionRulesOptions gained it on its
// pattern and type, and the protected package update went on answering a
// 422 with a hint to send both. On v3.30.0 every predicate of the two rows
// holds; the bump stack is where they stop holding and the entries retire.
//
// It reads the report as plain JSON, so it says the same thing on a tree
// that has no such section: there it fails, which is the point.
func TestRun_Repository_HoldsRowSixAndRowFiftyTwoToWhatClientGoV3300Does(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, _, err := Run(root, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var rep witnessReport
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	if rep.UpstreamWaits == nil {
		t.Fatal("the sdk report has no upstream_waits section: nothing holds a workaround that waits on client-go to the SDK it is pinned to")
	}
	wantPredicates := map[int][]string{
		6: {
			"WritesAlways{SetFeatureFlagOptions.Key, json}", "WritesAlways{SetFeatureFlagOptions.FeatureGroup, json}",
			"WritesAlways{SetFeatureFlagOptions.User, json}", "WritesAlways{SetFeatureFlagOptions.Group, json}",
			"WritesAlways{SetFeatureFlagOptions.Namespace, json}", "WritesAlways{SetFeatureFlagOptions.Project, json}",
			"WritesAlways{SetFeatureFlagOptions.Repository, json}", "WritesAlways{SetFeatureFlagOptions.Force, json}",
		},
		52: {"WritesAlways{UpdatePackageProtectionRulesOptions.PackageNamePattern, json}", "WritesAlways{UpdatePackageProtectionRulesOptions.PackageType, json}"},
	}
	wantSites := map[int][]string{6: {"features.Set", "fixture.setFeature"}, 52: {"protectedpackages.Update"}}
	for _, entry := range rep.UpstreamWaits.Entries {
		predicates, watched := wantPredicates[entry.Row]
		if !watched {
			continue
		}
		delete(wantPredicates, entry.Row)
		t.Run("row "+strconv.Itoa(entry.Row), func(t *testing.T) {
			checkWitnessEntry(t, entry, predicates, wantSites[entry.Row])
		})
	}
	if len(wantPredicates) != 0 {
		t.Errorf("no entry for rows %v", wantPredicates)
	}
	answered := map[string][]string{}
	for _, found := range rep.UpstreamWaits.Workarounds {
		answered[found.Form+" "+found.Site] = found.AnsweredBy
	}
	for _, key := range []string{"raw-request features.Set", "fixture-raw-request fixture.setFeature"} {
		t.Run(key, func(t *testing.T) {
			if !slices.Equal(answered[key], []string{"row 6"}) {
				t.Errorf("%s answered by %q, want row 6", key, answered[key])
			}
		})
	}
}

// supersetWitness is the part of a workaround the superset witness reads,
// decoded as plain JSON into types of its own so that it compiles, and
// decodes, against a report without those keys too.
type supersetWitness struct {
	Form       string
	Site       string
	AnsweredBy []string `json:"answered_by"`
	PassedTo   []string `json:"passed_to"`
	Callers    []witnessCaller
	Decodes    []struct {
		Type  string
		Needs []string
	}
}

// witnessCaller is one caller of a forwarder as the witness reads it.
type witnessCaller struct {
	Site, Verb, Options string
}

// TestRun_Repository_HoldsEveryKeyARawRequestCarriesBesideClientGo is the
// witness of what a review of the table found the walk did not ask on the
// real tree, at client-go v3.30.0. The job reads decode a runner with five
// keys JobRunner lacks and three measures Job types float64, the epic board
// reads labels and lists whose element structs lack eight keys, and the
// epic list an author with two keys EpicAuthor lacks: each is a reason the
// raw read cannot go back to client-go, and none was a predicate, so a pin
// modeling only the keys the entry named would have called the read
// obsolete. The sixteen methods PaginationRequestOption pages take no
// options and wait on client-go like the upload and certificate lists, and
// the project hook forwarder is called with the struct and verb each hook
// action sends, which R-PARAM reads in place of its any. The bump stack
// updates this list.
//
// It reads the report as plain JSON and spells the forms as the report does,
// so it compiles and says the same thing on a tree whose walk reads none of
// this: there it fails, which is the point.
func TestRun_Repository_HoldsEveryKeyARawRequestCarriesBesideClientGo(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, _, err := Run(root, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var rep struct {
		UpstreamWaits struct{ Workarounds []supersetWitness } `json:"upstream_waits"`
	}
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	bySite := map[string]supersetWitness{}
	for _, found := range rep.UpstreamWaits.Workarounds {
		bySite[found.Form+" "+found.Site] = found
	}
	jobNeeds := []string{
		"FieldKind{Job.Coverage, float64}", "FieldKind{Job.Duration, float64}", "FieldKind{Job.QueuedDuration, float64}",
		"LacksKey{Job, archived}", "LacksKey{Job, runner.ip_address}", "LacksKey{Job, runner.online}", "LacksKey{Job, runner.paused}",
		"LacksKey{Job, runner.runner_type}", "LacksKey{Job, runner.status}", "LacksKey{Job, runner_manager}",
	}
	boardNeeds := []string{
		"LacksKey{GroupEpicBoard, hide_backlog_list}", "LacksKey{GroupEpicBoard, hide_closed_list}",
		"LacksKey{GroupEpicBoard, labels.created_at}", "LacksKey{GroupEpicBoard, labels.group_id}", "LacksKey{GroupEpicBoard, labels.project_id}",
		"LacksKey{GroupEpicBoard, labels.template}", "LacksKey{GroupEpicBoard, labels.title}", "LacksKey{GroupEpicBoard, labels.updated_at}",
		"LacksKey{GroupEpicBoard, lists.collapsed}", "LacksKey{GroupEpicBoard, lists.list_type}",
	}
	decodes := []struct {
		site, decoded string
		needs         []string
	}{
		{site: "jobs.rawListJobs", decoded: "jobs.jobAPI", needs: jobNeeds},
		{site: "jobs.rawGetJob", decoded: "jobs.jobAPI", needs: jobNeeds},
		{site: "groupepicboards.rawListBoards", decoded: "groupepicboards.groupEpicBoardAPI", needs: boardNeeds},
		{site: "groupepicboards.rawGetBoard", decoded: "groupepicboards.groupEpicBoardAPI", needs: boardNeeds},
		{site: "epics.rawListEpics", decoded: "epics.epicAPI", needs: []string{"LacksKey{Epic, author.locked}", "LacksKey{Epic, author.public_email}"}},
		{site: "projects.doProjectRequest", decoded: "projects.projectHookAPI", needs: []string{"LacksKey{ProjectHook, duo_flow_callback_enabled}"}},
	}
	for _, tc := range decodes {
		t.Run(tc.site, func(t *testing.T) {
			found := bySite["raw-request "+tc.site]
			var needs []string
			for _, decode := range found.Decodes {
				if decode.Type == tc.decoded {
					needs = decode.Needs
				}
			}
			if missingNeeds := slices.DeleteFunc(slices.Clone(tc.needs), func(need string) bool { return slices.Contains(needs, need) }); len(missingNeeds) != 0 {
				t.Errorf("%s decodes %s with needs %q, which lack %q", tc.site, tc.decoded, needs, missingNeeds)
			}
		})
	}
	t.Run("project hook callers", func(t *testing.T) {
		callers := bySite["raw-request projects.doProjectRequest"].Callers
		for _, want := range []witnessCaller{
			{Site: "projects.AddHook", Verb: "POST", Options: "*gitlab.AddProjectHookOptions"},
			{Site: "projects.EditHook", Verb: "PUT", Options: "*gitlab.EditProjectHookOptions"},
		} {
			t.Run(want.Site, func(t *testing.T) {
				if !slices.Contains(callers, want) {
					t.Errorf("doProjectRequest callers %+v lack %+v", callers, want)
				}
			})
		}
	})
	t.Run("paging without options", func(t *testing.T) {
		paging := bySite["request-option toolutil.PaginationRequestOption"]
		if len(paging.PassedTo) != 16 || !slices.Equal(paging.AnsweredBy, []string{"row 150"}) {
			t.Errorf("PaginationRequestOption is handed to %q and answered by %q, want sixteen methods and row 150", paging.PassedTo, paging.AnsweredBy)
		}
	})
}

// TestRun_Repository_EveryWorkaroundIsAnsweredAndEveryEntryHolds verifies
// the section on the real tree in full: the walk finds the four forms in
// the numbers the table answers, every one answered and every key of a body
// edit explained, and every upstream entry holds on the pinned client-go.
func TestRun_Repository_EveryWorkaroundIsAnsweredAndEveryEntryHolds(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	rep, err := buildReport(root, false)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	forms := map[string]int{}
	for _, found := range rep.UpstreamWaits.Workarounds {
		forms[found.Form]++
		if len(found.AnsweredBy) == 0 || len(found.UnansweredKeys)+len(found.UnansweredNeeds)+len(found.UnansweredMethods) != 0 || found.KeysUnread {
			t.Errorf("%s %s is answered by %q with keys %q, needs %q and methods %q unanswered, keys unread %v",
				found.Form, found.Site, found.AnsweredBy, found.UnansweredKeys, found.UnansweredNeeds, found.UnansweredMethods, found.KeysUnread)
		}
	}
	want := map[string]int{formRawRequest: 28, formRequestOption: 13, formBodyEdit: 9, formFixtureRawRequest: 5, formScenarioRawRequest: 1}
	if !reflect.DeepEqual(forms, want) {
		t.Errorf("forms = %v, want %v", forms, want)
	}
	for _, entry := range rep.UpstreamWaits.Entries {
		if entry.Category == waits.CategoryUpstream && entry.Status != waitHolds {
			t.Errorf("row %d is %s: %+v", entry.Row, entry.Status, entry.Predicates)
		}
	}
	if rep.Summary.UpstreamWaits != len(waits.Entries()) || rep.Summary.Workarounds != len(rep.UpstreamWaits.Workarounds) {
		t.Errorf("summary = %+v", rep.Summary)
	}
	gaps, err := buildReport(root, true)
	if err != nil {
		t.Fatalf("gaps-only buildReport: %v", err)
	}
	if upstream := gaps.UpstreamWaits; len(upstream.Entries) != 0 || len(upstream.Workarounds) != 0 {
		t.Errorf("gaps-only report kept upstream waits %+v, want none", upstream)
	}
	for _, dir := range []string{fixtureDir, scenarioDir} {
		t.Run(dir, func(t *testing.T) {
			if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); statErr != nil {
				t.Errorf("the e2e tree the walk reads is not at %s: %v", dir, statErr)
			}
		})
	}
}
