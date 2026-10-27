// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// optionSDK is a stand-in client-go for the option structs section, in its
// "v3.30.0" state: two hook structs with the same fields, a struct a helper
// builds out of scalars, one every call sends as nil, one decoded from a map,
// a request that reaches a nested struct an input element fills and one built
// out of a list of integers, a defined type over an options struct, a struct
// only a raw request sends, two a raw request takes as any from the function
// that hands it its options, one of them embedded in a struct of the tool
// package's own, a search a helper builds out of an intermediate, a copy that
// shares no field with what it reads, an upload's content and the GraphQL
// document. A tilde stands for a backtick.
const optionSDK = `package gitlab

import "io"

type RequestOptionFunc func()

type ListOptions struct {
	Page int64 ~url:"page,omitempty" json:"page,omitempty"~
}

type AddWidgetHookOptions struct {
	URL    *string ~url:"url,omitempty" json:"url,omitempty"~
	Name   *string ~url:"name,omitempty" json:"name,omitempty"~
	Token  *string ~url:"token,omitempty" json:"token,omitempty"~
	secret int
}

type EditWidgetHookOptions struct {
	URL    *string ~url:"url,omitempty" json:"url,omitempty"~
	Name   *string ~url:"name,omitempty" json:"name,omitempty"~
	secret int
}

type WidgetPermissionOptions struct {
	UserID *int64 ~url:"user_id,omitempty" json:"user_id,omitempty"~
}

type WidgetLevel struct {
	Level *int64 ~url:"level,omitempty" json:"level,omitempty"~
	order int
}

type WidgetAvatar struct {
	Filename string
	Image    io.Reader
}

type ProtectWidgetOptions struct {
	Name          *string                     ~url:"name,omitempty" json:"name,omitempty"~
	AllowedToPush *[]*WidgetPermissionOptions ~url:"allowed_to_push,omitempty" json:"allowed_to_push,omitempty"~
	Levels        []*WidgetLevel              ~url:"levels,omitempty" json:"levels,omitempty"~
	Avatar        *WidgetAvatar               ~url:"-" json:"-"~
	Notify        *NotifyWidgetOptions        ~url:"notify,omitempty" json:"notify,omitempty"~
}

type WidgetTheme struct {
	Dark *bool ~json:"dark,omitempty"~
}

type ListWidgetsOptions struct {
	ListOptions
	Search *string ~url:"search,omitempty" json:"search,omitempty"~
}

type ListGadgetsOptions struct {
	ListOptions
}

type WidgetSettingsOptions struct {
	Color *string      ~json:"color,omitempty"~
	Theme *WidgetTheme ~json:"theme,omitempty"~
}

type CreateWidgetOptions struct {
	Name *string ~url:"name,omitempty" json:"name,omitempty"~
}

type CopyWidgetOptions CreateWidgetOptions

type ArchiveWidgetOptions CreateWidgetOptions

type NotifyWidgetOptions struct {
	Payload any ~json:"payload,omitempty"~
}

type WidgetPayload struct {
	Payload *string ~json:"payload"~
}

type ExportWidgetOptions struct {
	Format *string ~url:"format,omitempty" json:"format,omitempty"~
}

type PurgeWidgetOptions struct {
	Name *string ~url:"name,omitempty" json:"name,omitempty"~
}

type RestoreWidgetOptions struct {
	Name *string ~url:"name,omitempty" json:"name,omitempty"~
}

type PairWidgetOptions struct {
	Name *string ~url:"name,omitempty" json:"name,omitempty"~
}

type SearchWidgetsOptions struct {
	Query *string ~url:"query,omitempty" json:"query,omitempty"~
}

type WidgetHookFilter struct {
	Match func() string ~json:"-"~
}

type Widget struct {
	ID int64 ~json:"id"~
}

type WidgetCopy Widget

func (w Widget) Match() string { return "" }

func (w Widget) String() string { return "" }

type WidgetsServiceInterface interface {
	AddWidgetHook(pid any, opt *AddWidgetHookOptions, options ...RequestOptionFunc) error
	EditWidgetHook(pid any, hook int64, opt *EditWidgetHookOptions, options ...RequestOptionFunc) error
	ListWidgets(opt *ListWidgetsOptions, options ...RequestOptionFunc) ([]*Widget, error)
	ListGadgets(pid any, opt *ListGadgetsOptions, options ...RequestOptionFunc) error
	ProtectWidget(pid any, opt *ProtectWidgetOptions, options ...RequestOptionFunc) error
	UpdateWidgetSettings(opt *WidgetSettingsOptions, options ...RequestOptionFunc) error
	CreateWidget(opt *CreateWidgetOptions, options ...RequestOptionFunc) error
	CopyWidget(pid any, opt *CopyWidgetOptions, options ...RequestOptionFunc) error
	NotifyWidget(opt *NotifyWidgetOptions, options ...RequestOptionFunc) error
	ArchiveWidget(opt *ArchiveWidgetOptions, options ...RequestOptionFunc) error
	SearchWidgets(opt *SearchWidgetsOptions, options ...RequestOptionFunc) error
}

type GraphQLQuery struct {
	Query string ~json:"query"~
}

type GraphQLInterface interface {
	Do(query GraphQLQuery, target any, options ...RequestOptionFunc) error
}

type Client struct {
	GraphQL GraphQLInterface
	Widgets WidgetsServiceInterface
}

func (c *Client) NewRequest(method, path string, opt any, options []RequestOptionFunc) (*Widget, error) {
	return nil, nil
}

func (c *Client) BaseURL() string { return "" }
`

// optionDataSource is a tool package of helpers, which builds the list
// options out of scalars.
const optionDataSource = `package widgetdata

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"

func ListOptions(page int, search string) *gl.ListWidgetsOptions {
	opts := &gl.ListWidgetsOptions{Search: &search}
	opts.Page = int64(page)
	return opts
}

var Name *string
`

// optionToolSource is the tool package whose handlers send every struct of
// optionSDK, each the way the case it stands for does on the real tree.
const optionToolSource = `package widgets

import (
	"encoding/json"

	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"
	"example.com/fixture/internal/tools/widgetdata"
)

type HookInput struct {
	URL  string ~json:"url"~
	Name string ~json:"name"~
}

func addOptions(in HookInput) *gl.AddWidgetHookOptions {
	return &gl.AddWidgetHookOptions{URL: &in.URL, Name: &in.Name}
}

func editFromAdd(opts *gl.AddWidgetHookOptions) *gl.EditWidgetHookOptions {
	return &gl.EditWidgetHookOptions{URL: opts.URL, Name: opts.Name}
}

func AddHook(c *gl.Client, in HookInput) error { return c.Widgets.AddWidgetHook(1, addOptions(in)) }

func EditHook(c *gl.Client, in HookInput) error {
	add := addOptions(in)
	return c.Widgets.EditWidgetHook(1, 2, editFromAdd(add))
}

type ListInput struct {
	Page   int    ~json:"page"~
	Search string ~json:"search"~
}

func List(c *gl.Client, in ListInput) error {
	_, err := c.Widgets.ListWidgets(widgetdata.ListOptions(in.Page, in.Search))
	return err
}

func Gadgets(c *gl.Client) error { return c.Widgets.ListGadgets(1, nil) }

type SettingsInput struct {
	Settings map[string]any ~json:"settings"~
}

func Settings(c *gl.Client, in SettingsInput) error {
	raw, _ := json.Marshal(in.Settings)
	var opts gl.WidgetSettingsOptions
	_ = json.Unmarshal(raw, &opts)
	if err := c.Widgets.UpdateWidgetSettings(&opts); err != nil {
		return err
	}
	return c.Widgets.UpdateWidgetSettings(nil)
}

func Lister(c *gl.Client) any { return c.Widgets.ListWidgets }

type PermissionInput struct {
	UserID int64 ~json:"user_id"~
}

type ProtectInput struct {
	Name          string            ~json:"name"~
	AllowedToPush []PermissionInput ~json:"allowed_to_push"~
	Levels        []int64           ~json:"levels"~
}

func Protect(c *gl.Client, in ProtectInput) error {
	allowed := make([]*gl.WidgetPermissionOptions, 0, len(in.AllowedToPush))
	for _, p := range in.AllowedToPush {
		allowed = append(allowed, &gl.WidgetPermissionOptions{UserID: &p.UserID})
	}
	return c.Widgets.ProtectWidget(1, &gl.ProtectWidgetOptions{Name: &in.Name, AllowedToPush: &allowed, Levels: levels(in.Levels)})
}

func levels(values []int64) []*gl.WidgetLevel {
	out := make([]*gl.WidgetLevel, 0, len(values))
	for _, value := range values {
		out = append(out, &gl.WidgetLevel{Level: &value})
	}
	return out
}

type CreateInput struct {
	Name string ~json:"name"~
}

func create(in CreateInput) *gl.CreateWidgetOptions { return &gl.CreateWidgetOptions{Name: &in.Name} }

func Create(c *gl.Client, in CreateInput) error { return c.Widgets.CreateWidget(create(in)) }

func Copy(c *gl.Client, in CreateInput) error {
	opts := gl.CopyWidgetOptions(*create(in))
	_ = gl.CopyWidgetOptions(*create(in))
	return c.Widgets.CopyWidget(1, &opts)
}

type NotifyInput struct {
	Payload string ~json:"payload"~
}

func Notify(c *gl.Client, in NotifyInput) error {
	payload := gl.WidgetPayload{Payload: &in.Payload}
	return c.Widgets.NotifyWidget(&gl.NotifyWidgetOptions{Payload: payload.Payload})
}

type ExportInput struct {
	Format string ~json:"format"~
}

func Export(c *gl.Client, in ExportInput) error {
	_, err := c.NewRequest("GET", "widgets/export", &gl.ExportWidgetOptions{Format: &in.Format}, nil)
	return err
}

func raw(c *gl.Client, opts any) error {
	_, err := c.NewRequest("GET", "widgets", opts, nil)
	_, _ = c.NewRequest("GET", "widgets", nil, nil)
	_ = c.BaseURL()
	return err
}

type PurgeInput struct {
	Name string ~json:"name"~
}

func Purge(c *gl.Client, in PurgeInput) error {
	if err := raw(c, nil); err != nil {
		return err
	}
	return raw(c, &gl.PurgeWidgetOptions{Name: &in.Name})
}

var rawVar = func(c *gl.Client, opts any, retries int) error {
	_, err := c.NewRequest("PUT", "widgets", opts, nil)
	return err
}

var count, limit = 1, 2

type restoreBody struct {
	gl.RestoreWidgetOptions
	Theme *gl.WidgetTheme ~json:"theme,omitempty"~
	Force *bool           ~url:"force,omitempty" json:"force,omitempty"~
}

type RestoreInput struct {
	Name string ~json:"name"~
}

func Restore(c *gl.Client, in RestoreInput) error {
	return rawVar(c, &restoreBody{RestoreWidgetOptions: gl.RestoreWidgetOptions{Name: &in.Name}}, count+limit)
}

func rawPair[T, U any](c *gl.Client, opts any) (T, U, error) {
	var first T
	var second U
	_, err := c.NewRequest("POST", "widgets/pair", opts, nil)
	return first, second, err
}

type PairInput struct {
	Name string ~json:"name"~
}

func Pair(c *gl.Client, in PairInput) error {
	_, _, err := rawPair[int, string](c, &gl.PairWidgetOptions{Name: &in.Name})
	return err
}

func rawMany(c *gl.Client, opts ...any) error {
	_, err := c.NewRequest("GET", "widgets", opts, nil)
	return err
}

func Many(c *gl.Client) error { return rawMany(c) }

type searchArgs struct {
	Query string ~json:"query"~
}

func searchOptions(a searchArgs) *gl.SearchWidgetsOptions { return &gl.SearchWidgetsOptions{Query: &a.Query} }

func withSearch(a searchArgs, run func(searchArgs) error) error { return run(a) }

type SearchInput struct {
	Query string ~json:"query"~
}

func Search(c *gl.Client, in SearchInput) error {
	return withSearch(searchArgs{Query: in.Query}, func(a searchArgs) error { return c.Widgets.SearchWidgets(searchOptions(a)) })
}

func Query(c *gl.Client) error { return c.GraphQL.Do(gl.GraphQLQuery{Query: "{}"}, nil) }

func odd(export *gl.ExportWidgetOptions, w gl.Widget, created gl.CreateWidgetOptions) {
	_ = gl.CreateWidgetOptions{Name: export.Format}
	_ = gl.CreateWidgetOptions{Name: nil}
	_ = gl.CreateWidgetOptions{Name: widgetdata.Name}
	_ = gl.CreateWidgetOptions{Name: created.Name}
	_ = gl.CopyWidgetOptions{Name: created.Name}
	_ = gl.ArchiveWidgetOptions(created)
	_ = gl.WidgetCopy(w)
	_ = gl.WidgetHookFilter{Match: w.Match}
	_ = gl.Widget{1}
	_ = gl.Widget(w)
	_ = w.String()
	_ = map[string]*string{"name": export.Format}
	_ = gl.WidgetsServiceInterface.ListWidgets
}
`

// The "v3.38.0" states of the stand-in, each one change client-go makes.
var (
	// hookGainsURLVariables adds a field to both hook structs.
	hookGainsURLVariables = strings.NewReplacer(
		"type AddWidgetHookOptions struct {\n", "type AddWidgetHookOptions struct {\n\tURLVariables *[]string ~url:\"url_variables,omitempty\" json:\"url_variables,omitempty\"~\n",
		"type EditWidgetHookOptions struct {\n", "type EditWidgetHookOptions struct {\n\tURLVariables *[]string ~url:\"url_variables,omitempty\" json:\"url_variables,omitempty\"~\n",
	)
	// hookConvertible is that change with the add options' own field gone,
	// so the two hook structs are convertible.
	hookConvertible = strings.NewReplacer(
		"type AddWidgetHookOptions struct {\n", "type AddWidgetHookOptions struct {\n\tURLVariables *[]string ~url:\"url_variables,omitempty\" json:\"url_variables,omitempty\"~\n",
		"type EditWidgetHookOptions struct {\n", "type EditWidgetHookOptions struct {\n\tURLVariables *[]string ~url:\"url_variables,omitempty\" json:\"url_variables,omitempty\"~\n",
		"\tToken  *string ~url:\"token,omitempty\" json:\"token,omitempty\"~\n", "",
	)
	// permissionGainsMemberRole adds a field to the nested permission struct.
	permissionGainsMemberRole = strings.NewReplacer(
		"type WidgetPermissionOptions struct {\n", "type WidgetPermissionOptions struct {\n\tMemberRoleID *int64 ~url:\"member_role_id,omitempty\" json:\"member_role_id,omitempty\"~\n",
	)
	// levelGainsWeight adds a second field to the struct built per integer.
	levelGainsWeight = strings.NewReplacer(
		"type WidgetLevel struct {\n", "type WidgetLevel struct {\n\tWeight *int64 ~url:\"weight,omitempty\" json:\"weight,omitempty\"~\n",
	)
)

// And the two changes of the tool package a case turns on.
var (
	// editByConversion replaces the field-by-field copy with a conversion.
	editByConversion = strings.NewReplacer(
		"return &gl.EditWidgetHookOptions{URL: opts.URL, Name: opts.Name}",
		"edit := gl.EditWidgetHookOptions(*opts)\n\treturn &edit",
	)
	// listWithConstants hands the helper constants instead of the input.
	listWithConstants = strings.NewReplacer("widgetdata.ListOptions(in.Page, in.Search)", `widgetdata.ListOptions(1, "x")`)
	// searchWithConstants fills the intermediate with a constant, so the
	// search options are paired with the intermediate alone.
	searchWithConstants = strings.NewReplacer("searchArgs{Query: in.Query}", `searchArgs{Query: "x"}`)
)

// fixtureDeclarations answers the three structs of the stand-in nothing
// fills, the way optionStructDeclarations answers their likes on the tree.
func fixtureDeclarations() map[string]optionStructDeclaration {
	return map[string]optionStructDeclaration{
		"WidgetSettingsOptions": {categoryMapInput, "decoded from the settings map"},
		"ListGadgetsOptions":    {categorySDKModelsUnused, "the route declares no page"},
		"WidgetLevel":           {categoryScalarList, "offered as a list of integers"},
	}
}

// optionFixture writes the stand-in module with the SDK and the tool package
// put through their replacers, and returns the section built over it with
// declared.
func optionFixture(t *testing.T, sdk, tool *strings.Replacer, declared map[string]optionStructDeclaration) (optionStructsReport, []string) {
	t.Helper()
	identity := strings.NewReplacer()
	sdkSource, toolSource := optionSDK, optionToolSource
	if sdk != nil {
		sdkSource = sdk.Replace(sdkSource)
	}
	if tool == nil {
		tool = identity
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.27\n",
		"gitlab.com/gitlab-org/api/client-go/v3/sdk.go": strings.ReplaceAll(sdkSource, "~", "`"),
		"internal/tools/widgetdata/widgetdata.go":       optionDataSource,
		"internal/tools/widgets/widgets.go":             strings.ReplaceAll(tool.Replace(toolSource), "~", "`"),
	})
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load the fixture: %v", err)
	}
	return buildOptionStructs(collectOptionUniverse(pkgs), structs.DiffInputPairs(pkgs), declared)
}

// structByType indexes a section's structs by name.
func structByType(report optionStructsReport) map[string]optionStruct {
	out := map[string]optionStruct{}
	for _, entry := range report.Structs {
		out[entry.Type] = entry
	}
	return out
}

// TestBuildOptionStructs_AStandInTree_HoldsEveryStructAndJudgesTheCopy
// verifies the whole section on a tree whose universe is known: every
// struct a service method takes, and a raw request sends (as its typed
// options, through a function or a variable that hands it its parameter of
// type any, or embedded in a struct of the tool package's own), is held by
// the input its handler pairs it with, by the input of the handler that
// hands a helper its fields or an intermediate filled from them, by a
// conversion of a held struct, by a copy of one or by a declaration; every
// struct a held one reaches is held by the input element it is built from or
// by a declaration; the copy is judged against the fields the two hook
// structs share and the conversion is listed. The GraphQL document, the
// upload's content, a struct a local one holds in a field of its own, what a
// variadic parameter hands on and the structs no call sends are not in the
// section, and a copy that shares no field with what it reads is no copy.
func TestBuildOptionStructs_AStandInTree_HoldsEveryStructAndJudgesTheCopy(t *testing.T) {
	report, stale := optionFixture(t, nil, nil, fixtureDeclarations())

	want := optionStructsReport{
		Structs: []optionStruct{
			{Type: "AddWidgetHookOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.HookInput"}},
			{Type: "CopyWidgetOptions", Level: levelReceived, Status: optionConverted, From: "CreateWidgetOptions", Sites: []string{"widgets.Copy"}},
			{Type: "CreateWidgetOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.CreateInput"}},
			{Type: "EditWidgetHookOptions", Level: levelReceived, Status: optionCopied, From: "AddWidgetHookOptions", Sites: []string{"widgets.editFromAdd"}},
			{Type: "ExportWidgetOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.ExportInput"}},
			{
				Type: "ListGadgetsOptions", Level: levelReceived, Status: optionDeclared, Sites: []string{"widgets.Gadgets"}, NilOnly: true,
				Category: categorySDKModelsUnused, Reason: "the route declares no page",
			},
			{Type: "ListWidgetsOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.ListInput (caller)"}},
			{Type: "NotifyWidgetOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.NotifyInput"}},
			{Type: "PairWidgetOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.PairInput"}},
			{Type: "ProtectWidgetOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.ProtectInput"}},
			{Type: "PurgeWidgetOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.PurgeInput"}},
			{Type: "RestoreWidgetOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.RestoreInput"}},
			{Type: "SearchWidgetsOptions", Level: levelReceived, Status: optionPaired, Inputs: []string{"widgets.SearchInput (caller)", "widgets.searchArgs"}},
			{
				Type: "WidgetLevel", Level: levelNested, Under: []string{"ProtectWidgetOptions"}, Status: optionDeclared,
				Category: categoryScalarList, Reason: "offered as a list of integers",
			},
			{Type: "WidgetPermissionOptions", Level: levelNested, Under: []string{"ProtectWidgetOptions"}, Status: optionPaired, Inputs: []string{"widgets.PermissionInput (nested)"}},
			{
				Type: "WidgetSettingsOptions", Level: levelReceived, Status: optionDeclared, Sites: []string{"widgets.Settings"},
				Category: categoryMapInput, Reason: "decoded from the settings map",
			},
		},
		Copies: []optionCopy{
			{From: "AddWidgetHookOptions", To: "EditWidgetHookOptions", Site: "widgets.editFromAdd", Shared: 2, Copied: 2},
			{From: "CreateWidgetOptions", To: "CopyWidgetOptions", Site: "widgets.odd", Shared: 1, Copied: 1},
		},
		Conversions: []optionConversion{
			{From: "CreateWidgetOptions", To: "ArchiveWidgetOptions", Site: "widgets.odd"},
			{From: "CreateWidgetOptions", To: "CopyWidgetOptions", Site: "widgets.Copy"},
		},
	}
	if !reflect.DeepEqual(report, want) {
		t.Errorf("section =\n%+v\nwant\n%+v", report, want)
	}
	if len(stale) != 0 {
		t.Errorf("stale = %v, want none", stale)
	}
	var summary reportSummary
	summarizeOptionStructs(&summary, report)
	if want := (reportSummary{OptionStructs: 16, OptionPaired: 11, OptionConverted: 1, OptionCopied: 1, OptionDeclared: 3}); summary != want {
		t.Errorf("summary = %+v, want %+v", summary, want)
	}
	if !summary.clean() {
		t.Error("a held section failed the gate")
	}
}

// TestBuildOptionStructs_NothingDeclared_EachStructNothingFillsIsAFinding
// verifies rule B and rule C without their declarations: a struct every call
// sends as nil, one decoded from a map and a nested one built out of
// integers are each unpaired, a received one with the functions that send it
// and whether all of them send nil, and the summary fails the gate on them.
func TestBuildOptionStructs_NothingDeclared_EachStructNothingFillsIsAFinding(t *testing.T) {
	report, _ := optionFixture(t, nil, nil, nil)

	byType := structByType(report)
	cases := map[string]optionStruct{
		"ListGadgetsOptions":    {Type: "ListGadgetsOptions", Level: levelReceived, Status: optionUnpaired, Sites: []string{"widgets.Gadgets"}, NilOnly: true},
		"WidgetSettingsOptions": {Type: "WidgetSettingsOptions", Level: levelReceived, Status: optionUnpaired, Sites: []string{"widgets.Settings"}},
		"WidgetLevel":           {Type: "WidgetLevel", Level: levelNested, Under: []string{"ProtectWidgetOptions"}, Status: optionUnpaired},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := byType[name]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s = %+v, want %+v", name, got, want)
			}
		})
	}
	var summary reportSummary
	summarizeOptionStructs(&summary, report)
	if summary.OptionUnpaired != 3 || summary.clean() {
		t.Errorf("summary = %+v, want 3 unpaired and a failing gate", summary)
	}
	if kept := keepOptionFindings(report); len(kept.Structs) != 3 || len(kept.Copies) != 0 || kept.Conversions == nil || len(kept.Conversions) != 0 {
		t.Errorf("keepOptionFindings = %+v, want the three unpaired structs and empty lists", kept)
	}
}

// TestBuildOptionStructs_AFieldBothHookStructsGain_IsAFindingOfTheCopyUntilAConversionBuildsIt
// verifies rule A on the change v3.38.0 brings the group hooks: once both
// structs carry URLVariables the field-by-field copy leaves it out and the
// gate fails on it, a declaration that the copy drops it on purpose answers
// it, and the conversion the compiler holds leaves no copy to judge.
func TestBuildOptionStructs_AFieldBothHookStructsGain_IsAFindingOfTheCopyUntilAConversionBuildsIt(t *testing.T) {
	t.Run("the copy leaves the new field out", func(t *testing.T) {
		report, _ := optionFixture(t, hookGainsURLVariables, nil, fixtureDeclarations())

		want := optionCopy{From: "AddWidgetHookOptions", To: "EditWidgetHookOptions", Site: "widgets.editFromAdd", Shared: 3, Copied: 2, Missing: []string{"URLVariables"}}
		if len(report.Copies) != 2 || !reflect.DeepEqual(report.Copies[0], want) {
			t.Errorf("copies = %+v, want %+v first", report.Copies, want)
		}
		var summary reportSummary
		summarizeOptionStructs(&summary, report)
		if summary.OptionCopyMissing != 1 || summary.clean() {
			t.Errorf("summary = %+v, want one missing field and a failing gate", summary)
		}
		if kept := keepOptionFindings(report); !reflect.DeepEqual(kept.Copies, []optionCopy{want}) || len(kept.Structs) != 0 {
			t.Errorf("keepOptionFindings = %+v, want the copy alone", kept)
		}
	})

	t.Run("a declaration drops it on purpose", func(t *testing.T) {
		declared := fixtureDeclarations()
		declared[copyKey("AddWidgetHookOptions", "EditWidgetHookOptions", "URLVariables")] = optionStructDeclaration{categoryDroppedOnPurpose, "the edit must not send it"}

		report, stale := optionFixture(t, hookGainsURLVariables, nil, declared)

		want := optionCopy{From: "AddWidgetHookOptions", To: "EditWidgetHookOptions", Site: "widgets.editFromAdd", Shared: 3, Copied: 2, Dropped: []string{"URLVariables"}}
		if len(report.Copies) != 2 || !reflect.DeepEqual(report.Copies[0], want) || len(stale) != 0 {
			t.Errorf("copies = %+v, stale = %v; want %+v first and none", report.Copies, stale, want)
		}
	})

	t.Run("a declaration of another category answers nothing", func(t *testing.T) {
		declared := fixtureDeclarations()
		key := copyKey("AddWidgetHookOptions", "EditWidgetHookOptions", "URLVariables")
		declared[key] = optionStructDeclaration{categoryMapInput, "a struct category on a field"}

		report, stale := optionFixture(t, hookGainsURLVariables, nil, declared)

		if len(report.Copies) != 2 || !slices.Equal(report.Copies[0].Missing, []string{"URLVariables"}) || len(report.Copies[0].Dropped) != 0 {
			t.Errorf("copies = %+v, want URLVariables still missing", report.Copies)
		}
		if want := []string{"option struct copy " + key + " (category map-input does not answer a field a copy leaves out)"}; !slices.Equal(stale, want) {
			t.Errorf("stale = %v, want %v", stale, want)
		}
	})

	t.Run("a conversion leaves nothing to judge", func(t *testing.T) {
		report, _ := optionFixture(t, hookConvertible, editByConversion, fixtureDeclarations())

		edit := structByType(report)["EditWidgetHookOptions"]
		if len(report.Copies) != 1 || report.Copies[0].To != "CopyWidgetOptions" || edit.Status != optionConverted || edit.From != "AddWidgetHookOptions" || !slices.Equal(edit.Sites, []string{"widgets.editFromAdd"}) {
			t.Errorf("copies = %+v, edit = %+v; want no copy into the edit options and a conversion from the add options", report.Copies, edit)
		}
		var summary reportSummary
		summarizeOptionStructs(&summary, report)
		if !summary.clean() {
			t.Errorf("summary = %+v, want a passing gate", summary)
		}
	})
}

// TestBuildOptionStructs_AFieldANestedStructGains_IsAMissingInputOnTheElement
// verifies rule C: a field client-go adds to a nested struct is a missing
// input on the input element the struct is built from, which fails the gate
// where the structs scope would only list it.
func TestBuildOptionStructs_AFieldANestedStructGains_IsAMissingInputOnTheElement(t *testing.T) {
	report, _ := optionFixture(t, permissionGainsMemberRole, nil, fixtureDeclarations())

	if got := structByType(report)["WidgetPermissionOptions"].Missing; !slices.Equal(got, []string{"widgets.PermissionInput.member_role_id"}) {
		t.Errorf("WidgetPermissionOptions missing = %v, want the element's member_role_id", got)
	}
	var summary reportSummary
	summarizeOptionStructs(&summary, report)
	if summary.OptionNestedMissing != 1 || summary.clean() {
		t.Errorf("summary = %+v, want one nested missing field and a failing gate", summary)
	}
	if kept := keepOptionFindings(report); len(kept.Structs) != 1 || kept.Structs[0].Type != "WidgetPermissionOptions" || len(kept.Copies) != 0 {
		t.Errorf("keepOptionFindings = %+v, want the nested struct alone", kept)
	}
}

// TestResolveHeld_Builds_AreFollowedFromAHeldStructOnly verifies the
// resolution a conversion or a copy holds a struct by: one built from a held
// struct is held, however many passes the chain needs, and lists each
// function that builds it that way once; a build from another struct, or of
// the other kind, adds no function to it; and one built from a struct
// nothing holds stays unheld.
func TestResolveHeld_Builds_AreFollowedFromAHeldStructOnly(t *testing.T) {
	pkg := types.NewPackage(shared.ClientGoPkgPath+"/v3", "gitlab")
	named := func(name string) *types.Named {
		return types.NewNamed(types.NewTypeName(token.NoPos, pkg, name, nil), types.NewStruct(nil, nil), nil)
	}
	a, b, c, d, f, g := named("A"), named("B"), named("C"), named("D"), named("F"), named("G")
	universe := optionUniverse{
		conversions: []optionBuild{
			{to: f, from: b, site: "p.fromB"},
			{to: b, from: a, site: "p.one"},
			{to: b, from: a, site: "p.two"},
			{to: b, from: a, site: "p.one"},
			{to: d, from: c, site: "p.unheld"},
		},
		copies: []optionBuild{
			{to: b, from: a, site: "p.copy", keys: map[string]bool{"X": true}},
			{to: b, from: g, site: "p.fromG"},
		},
	}
	inputs := map[*types.TypeName]bool{a.Obj(): true, g.Obj(): true}

	resolved := resolveHeld(universe, inputs)

	want := map[*types.TypeName]held{
		a.Obj(): {status: optionPaired},
		g.Obj(): {status: optionPaired},
		b.Obj(): {status: optionConverted, from: "A", sites: []string{"p.one", "p.two"}},
		f.Obj(): {status: optionConverted, from: "B", sites: []string{"p.fromB"}},
	}
	if !reflect.DeepEqual(resolved, want) {
		t.Errorf("resolveHeld = %+v, want %+v", resolved, want)
	}
}

// TestCollectOptionUniverse_ADeclarationWithNoBody_IsPassedOver verifies the
// declarations the walk skips that a type-checked package never carries: a
// function declared outside Go, which has no body to walk, and a declaration
// the parser could not read.
func TestCollectOptionUniverse_ADeclarationWithNoBody_IsPassedOver(t *testing.T) {
	file := &ast.File{Decls: []ast.Decl{&ast.FuncDecl{Name: ast.NewIdent("external"), Type: &ast.FuncType{}}, &ast.BadDecl{}}}
	universe := collectOptionUniverse([]*packages.Package{{PkgPath: "example.com/x/internal/tools/p", Syntax: []*ast.File{file}}})

	if len(universe.received) != 0 || len(universe.conversions) != 0 || len(universe.copies) != 0 {
		t.Errorf("collectOptionUniverse = %+v, want nothing", universe)
	}
}

// TestBuildOptionStructs_AHelperHandedConstants_LeavesItsStructUnpaired
// verifies the pairing through a call is what holds the helper's struct: when
// the handler hands the helper constants rather than its input's fields, the
// struct is compared with no input, and it is a finding.
func TestBuildOptionStructs_AHelperHandedConstants_LeavesItsStructUnpaired(t *testing.T) {
	report, _ := optionFixture(t, nil, listWithConstants, fixtureDeclarations())

	want := optionStruct{Type: "ListWidgetsOptions", Level: levelReceived, Status: optionUnpaired, Sites: []string{"widgets.List", "widgets.Lister"}}
	if got := structByType(report)["ListWidgetsOptions"]; !reflect.DeepEqual(got, want) {
		t.Errorf("ListWidgetsOptions = %+v, want %+v", got, want)
	}
}

// TestBuildOptionStructs_AStructPairedWithAnIntermediateAlone_IsUnpaired
// verifies what holds a struct: an input a model sends. When the handler
// fills the intermediate it hands the helper with a constant, the search
// options are compared with that unexported struct of arguments alone, which
// is listed among the inputs and holds nothing, so the struct is a finding.
func TestBuildOptionStructs_AStructPairedWithAnIntermediateAlone_IsUnpaired(t *testing.T) {
	report, _ := optionFixture(t, nil, searchWithConstants, fixtureDeclarations())

	want := optionStruct{Type: "SearchWidgetsOptions", Level: levelReceived, Status: optionUnpaired, Inputs: []string{"widgets.searchArgs"}, Sites: []string{"widgets.Search"}}
	if got := structByType(report)["SearchWidgetsOptions"]; !reflect.DeepEqual(got, want) {
		t.Errorf("SearchWidgetsOptions = %+v, want %+v", got, want)
	}
}

// TestBuildOptionStructs_DeclarationsThatNoLongerHold_AreStale verifies every
// way a declaration stops describing the tree: a struct now held, one no
// call sends, a category that answers no struct or that answers a copy, a
// struct that is no longer what its category requires (sent with something
// other than nil, given a second field), a field the copy now fills, and a
// category that answers no field a copy leaves out. A declaration that does
// not hold leaves its struct a finding.
func TestBuildOptionStructs_DeclarationsThatNoLongerHold_AreStale(t *testing.T) {
	declared := map[string]optionStructDeclaration{
		"AddWidgetHookOptions":  {categoryMapInput, "held by its input"},
		"MissingOptions":        {categoryMapInput, "no call sends it"},
		"CopyWidgetOptions":     {categoryDroppedOnPurpose, "a copy category on a struct"},
		"ListGadgetsOptions":    {"made-up", "no such category"},
		"WidgetSettingsOptions": {categorySDKModelsUnused, "sent with what the map decodes to"},
		"WidgetLevel":           {categoryScalarList, "two fields now"},
		copyKey("AddWidgetHookOptions", "EditWidgetHookOptions", "URL"):  {categoryDroppedOnPurpose, "the copy fills it"},
		copyKey("AddWidgetHookOptions", "EditWidgetHookOptions", "Name"): {categoryMapInput, "a struct category on a field"},
	}

	report, stale := optionFixture(t, levelGainsWeight, nil, declared)

	want := []string{
		"option struct AddWidgetHookOptions (now paired)",
		"option struct CopyWidgetOptions (category dropped-on-purpose does not answer a struct)",
		"option struct ListGadgetsOptions (category made-up does not answer a struct)",
		"option struct MissingOptions (no call sends it)",
		"option struct WidgetLevel (category scalar-list requires that it carries exactly one field)",
		"option struct WidgetSettingsOptions (category sdk-models-unused requires that every call sends it as nil)",
		"option struct copy AddWidgetHookOptions->EditWidgetHookOptions.Name (category map-input does not answer a field a copy leaves out)",
		"option struct copy AddWidgetHookOptions->EditWidgetHookOptions.URL (no copy leaves the field out)",
	}
	if !slices.Equal(stale, want) {
		t.Errorf("stale =\n%s\nwant\n%s", strings.Join(stale, "\n"), strings.Join(want, "\n"))
	}
	byType := structByType(report)
	for _, name := range []string{"ListGadgetsOptions", "WidgetSettingsOptions", "WidgetLevel"} {
		t.Run(name, func(t *testing.T) {
			if byType[name].Status != optionUnpaired || byType[name].Category != "" {
				t.Errorf("%s = %+v, want unpaired and no category", name, byType[name])
			}
		})
	}
}

// TestCopiedField_ElementsThatAreNoCopy_AreRefused verifies the one element
// shape the type checker never produces in a struct literal: a key that is
// not a field name.
func TestCopiedField_ElementsThatAreNoCopy_AreRefused(t *testing.T) {
	element := &ast.KeyValueExpr{Key: &ast.BasicLit{Kind: token.STRING, Value: `"url"`}, Value: &ast.SelectorExpr{X: ast.NewIdent("opts"), Sel: ast.NewIdent("URL")}}
	if from, field, isCopy := copiedField(&types.Info{}, element); isCopy || from != nil || field != "" {
		t.Errorf("copiedField = %v/%q/%v, want no copy for a key that names no field", from, field, isCopy)
	}
}

// TestServiceMethod_ASelectorTheTypeCheckerDidNotResolve_NamesNoMethod
// verifies the selector on a service interface that a type-checked package
// always resolves, as a method value or a method expression, names no method
// when its resolution is missing rather than failing the walk.
func TestServiceMethod_ASelectorTheTypeCheckerDidNotResolve_NamesNoMethod(t *testing.T) {
	sdk := types.NewPackage(shared.ClientGoPkgPath+"/v3", "gitlab")
	service := types.NewNamed(types.NewTypeName(token.NoPos, sdk, "WidgetsServiceInterface", nil), types.NewInterfaceType(nil, nil), nil)
	receiver := ast.NewIdent("widgets")
	sel := &ast.SelectorExpr{X: receiver, Sel: ast.NewIdent("ListWidgets")}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{receiver: {Type: service}}}

	if method := serviceMethod(info, sel); method != nil {
		t.Errorf("serviceMethod = %v, want nil for a selector with no selection", method)
	}
}

// TestRun_Repository_HoldsEveryOptionStructAHandlerSends is the witness of
// the option structs section on the tree as it stands. The group hook edit is
// a field-by-field copy of the add options, complete over the thirty fields
// the two share in v3.30.0, and is paired with the edit input that reaches it
// through that copy; the project and system hook edits are conversions; the
// structs a helper builds out of scalars are each paired with the input of
// the handler that hands the helper its fields; the settings struct a map is
// decoded into and the integrations list every call sends as nil are
// declared; and a nested struct is paired with the input element it is built
// from, the group access level built per integer declared.
func TestRun_Repository_HoldsEveryOptionStructAHandlerSends(t *testing.T) {
	section := readRepositorySection(t)
	byType := section.byType

	cases := []struct{ name, got, want string }{
		{"the group hook edit, which its input reaches through the copy", byType["EditGroupHookOptions"], "received paired    groups.EditHookInput (caller) "},
		{"the system hook conversion", byType["EditHookOptions"], "received converted AddHookOptions  systemhooks.hookEditOptions  "},
		{"the project hook edit a raw request sends", byType["EditProjectHookOptions"], "received converted AddProjectHookOptions  projects.editProjectHookOptions  "},
		{"the settings map", byType["UpdateSettingsOptions"], "received declared  map-input settings.Update  "},
		{"the integrations sent as nil", byType["ListActiveIntegrationsOptions"], "received declared  sdk-models-unused integrations.ListGroupIntegrations  nil-only"},
		{"a branch permission", byType["BranchPermissionOptions"], "nested paired    branches.BranchPermissionInput (nested) "},
		{"a group access level", byType["GroupAccessLevel"], "nested declared  scalar-list   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
	for _, name := range []string{
		"GetDORAMetricsOptions", "ListAccessRequestsOptions", "ListAuditEventsOptions", "ListGroupIterationsOptions",
		"ListGroupLabelsOptions", "ListLabelsOptions", "ListProjectIterationsOptions", "SearchOptions",
	} {
		t.Run(name+" through its helper", func(t *testing.T) {
			if !strings.Contains(byType[name], "received paired ") || !strings.Contains(byType[name], "(caller)") {
				t.Errorf("%s = %q, want paired through the caller of its helper", name, byType[name])
			}
		})
	}
	t.Run("the copy and the conversions", func(t *testing.T) {
		copied := witnessCopy{From: "AddGroupHookOptions", To: "EditGroupHookOptions", Site: "groups.groupEditHookOptionsFromAdd", Shared: 30, Copied: 30}
		if !slices.Contains(section.copies, copied) {
			t.Errorf("copies = %+v, want %+v among them", section.copies, copied)
		}
		for _, conversion := range []witnessConversion{
			{From: "AddProjectHookOptions", To: "EditProjectHookOptions", Site: "projects.editProjectHookOptions"},
			{From: "AddHookOptions", To: "EditHookOptions", Site: "systemhooks.hookEditOptions"},
		} {
			t.Run(conversion.To, func(t *testing.T) {
				if !slices.Contains(section.conversions, conversion) {
					t.Errorf("conversions = %+v, want %+v among them", section.conversions, conversion)
				}
			})
		}
	})
}

// TestRun_Repository_HoldsWhatAnIntermediateOrARawRequestCarries is the
// witness of the two routes to a request the section first missed on the
// tree as it stands. A struct an intermediate the handler fills carries to a
// helper is paired with the handler's input, where it was paired with the
// unexported intermediate alone and so with nothing a model sends; and a
// struct a raw request takes as any, from the function that hands it its
// options, is in the section and held, the custom header the project hooks
// reach among the nested structs.
func TestRun_Repository_HoldsWhatAnIntermediateOrARawRequestCarries(t *testing.T) {
	section := readRepositorySection(t)
	byType, inputsOf, underOf := section.byType, section.inputsOf, section.underOf

	for name, input := range map[string]string{
		"CreateGroupAccessTokenOptions":     "accesstokens.GroupCreateInput (caller)",
		"CreateProjectAccessTokenOptions":   "accesstokens.ProjectCreateInput (caller)",
		"GetGroupIssuesStatisticsOptions":   "issuestatistics.GetGroupInput (caller)",
		"GetProjectIssuesStatisticsOptions": "issuestatistics.GetProjectInput (caller)",
		"ListGroupsRunnersOptions":          "runners.ListGroupInput (caller)",
		"ListProjectRunnersOptions":         "runners.ListProjectInput (caller)",
	} {
		t.Run(name+" through the intermediate its handler fills", func(t *testing.T) {
			if !strings.HasPrefix(byType[name], "received paired ") || !slices.Contains(inputsOf[name], input) {
				t.Errorf("%s = %q, want paired with %s", name, byType[name], input)
			}
		})
	}
	for _, name := range []string{
		"AddProjectHookOptions", "CreateMergeRequestApprovalRuleOptions", "ListProjectHooksOptions",
		"UpdateGroupIssueBoardOptions", "UpdateMergeRequestApprovalRuleOptions",
	} {
		t.Run(name+" a raw request takes as any", func(t *testing.T) {
			if !strings.HasPrefix(byType[name], "received paired ") {
				t.Errorf("%s = %q, want received and paired", name, byType[name])
			}
		})
	}
	t.Run("the custom header every hook options struct reaches", func(t *testing.T) {
		if want := []string{"AddGroupHookOptions", "AddProjectHookOptions", "EditGroupHookOptions", "EditProjectHookOptions"}; !slices.Equal(underOf["HookCustomHeader"], want) {
			t.Errorf("HookCustomHeader under = %v, want %v", underOf["HookCustomHeader"], want)
		}
	})
}

// witnessCopy and witnessConversion read a copy and a conversion of the
// published section through types of the test's own, so the witness reads
// the published shape rather than the section's Go types.
type witnessCopy struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Site   string `json:"site"`
	Shared int    `json:"shared"`
	Copied int    `json:"copied"`
}

type witnessConversion struct {
	From string `json:"from"`
	To   string `json:"to"`
	Site string `json:"site"`
}

// witnessSection is the option structs section of a run over the tree, each
// struct by name: byType as "<level> <status> <from> <category> <sites>
// <inputs> <nil-only>", and its inputs and the received structs it sits
// under on their own.
type witnessSection struct {
	byType      map[string]string
	inputsOf    map[string][]string
	underOf     map[string][]string
	copies      []witnessCopy
	conversions []witnessConversion
}

// readRepositorySection runs the scope over the repository and reads its
// option structs section.
func readRepositorySection(t *testing.T) witnessSection {
	t.Helper()
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, _, err := Run(root, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var rep struct {
		OptionStructs struct {
			Structs []struct {
				Type     string   `json:"type"`
				Level    string   `json:"level"`
				Under    []string `json:"under"`
				Status   string   `json:"status"`
				Inputs   []string `json:"inputs"`
				From     string   `json:"from"`
				Sites    []string `json:"sites"`
				NilOnly  bool     `json:"nil_only"`
				Category string   `json:"category"`
			} `json:"structs"`
			Copies      []witnessCopy       `json:"copies"`
			Conversions []witnessConversion `json:"conversions"`
		} `json:"option_structs"`
	}
	if decodeErr := json.Unmarshal(content, &rep); decodeErr != nil {
		t.Fatalf("report is not JSON: %v", decodeErr)
	}
	section := witnessSection{
		byType: map[string]string{}, inputsOf: map[string][]string{}, underOf: map[string][]string{},
		copies: rep.OptionStructs.Copies, conversions: rep.OptionStructs.Conversions,
	}
	for _, entry := range rep.OptionStructs.Structs {
		section.byType[entry.Type] = strings.Join([]string{entry.Level, entry.Status, entry.From, entry.Category, strings.Join(entry.Sites, ","), strings.Join(entry.Inputs, ","), map[bool]string{true: "nil-only"}[entry.NilOnly]}, " ")
		section.inputsOf[entry.Type], section.underOf[entry.Type] = entry.Inputs, entry.Under
	}
	return section
}
