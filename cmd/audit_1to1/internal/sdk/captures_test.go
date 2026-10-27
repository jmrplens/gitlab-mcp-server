// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// capturesSDK is a stand-in client-go for the captures section, in its
// "v3.30.0" state: a widget that models no delete_api_path and its size as a
// number, the details of a widget, which model neither, a gadget whose owner
// is a widget, methods that answer with nothing to decode or with no struct
// and an option that hands a request its context. A tilde stands for a
// backtick.
const capturesSDK = `package gitlab

import "context"

type Request struct{}

type RequestOptionFunc func(*Request) error

func WithContext(ctx context.Context) RequestOptionFunc { return nil }

type Response struct{}

type Widget struct {
	ID   int64  ~json:"id"~
	Name string ~json:"name"~
	Size int64  ~json:"size"~
}

type WidgetDetails struct {
	ID int64 ~json:"id"~
}

type Gadget struct {
	ID    int64   ~json:"id"~
	Owner *Widget ~json:"owner"~
}

type WidgetsServiceInterface interface {
	GetWidget(id int64, options ...RequestOptionFunc) (*Widget, *Response, error)
	ListWidgets(options ...RequestOptionFunc) ([]*Widget, *Response, error)
	GetWidgetDetails(id int64, options ...RequestOptionFunc) (*WidgetDetails, *Response, error)
	GetGadget(id int64, options ...RequestOptionFunc) (*Gadget, *Response, error)
	ListGadgets(options ...RequestOptionFunc) ([]*Gadget, *Response, error)
	DeleteWidget(id int64, options ...RequestOptionFunc) (*Response, error)
	CountWidgets(options ...RequestOptionFunc) (int, *Response, error)
	Ping(options ...RequestOptionFunc)
	helper(id int64) (*Response, *Response, error)
}

type Client struct {
	Widgets WidgetsServiceInterface
	Limit   int
}
`

// capturesGitLab is the stand-in for the package that captures a response:
// the capture, the function that starts one and the method that decodes it.
const capturesGitLab = `package gitlab

import "context"

type ResponseCapture struct{ body []byte }

func WithResponseCapture(ctx context.Context) (context.Context, *ResponseCapture) {
	return ctx, &ResponseCapture{}
}

func (c *ResponseCapture) Decode(v any) error { return nil }

func NotACapture(ctx context.Context) (context.Context, int) { return ctx, 0 }
`

// capturesToolutil is the stand-in shared package, whose bodies the walk
// reads as it reads a tool package's: two generic readers, one per answer
// shape; a reader of each shape that decodes the widget's extra through
// them; a reader of a list whose rows nest a widget under their owner, which
// decodes the row and hands back the widget inside it; a generic reader
// that takes a widget beside the capture; a function that decodes nothing;
// and the converter a widget's two halves are handed to.
const capturesToolutil = `package toolutil

import (
	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"
	gitlabclient "example.com/fixture/internal/gitlab"
)

type WidgetExtra struct {
	DeleteAPIPath string ~json:"delete_api_path"~
	Size          *int64 ~json:"size"~
}

func capturedOne[T any](capture *gitlabclient.ResponseCapture) (T, error) {
	var out T
	return out, capture.Decode(&out)
}

func capturedList[T any](capture *gitlabclient.ResponseCapture, decoded int) ([]T, error) {
	var out []T
	return out, capture.Decode(&out)
}

func CapturedWidget(capture *gitlabclient.ResponseCapture) (WidgetExtra, error) {
	return capturedOne[WidgetExtra](capture)
}

func CapturedWidgets(capture *gitlabclient.ResponseCapture, decoded int) ([]WidgetExtra, error) {
	return capturedList[WidgetExtra](capture, decoded)
}

type ownerRow struct {
	Owner WidgetExtra ~json:"owner"~
}

func CapturedOwners(capture *gitlabclient.ResponseCapture, decoded int) ([]WidgetExtra, error) {
	rows, err := capturedList[ownerRow](capture, decoded)
	out := make([]WidgetExtra, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Owner)
	}
	return out, err
}

func CapturedThread[T any](w *gl.Widget, capture *gitlabclient.ResponseCapture) (T, error) {
	return capturedOne[T](capture)
}

func Touch(capture *gitlabclient.ResponseCapture) {}

func WidgetOutput(w *gl.Widget, extra WidgetExtra) string { return extra.DeleteAPIPath }

const Version = 1
`

// capturesTool is a tool package that captures every way the walk follows:
// a get that hands the capture to a shared reader and both halves to the
// shared converter; a read of the details through a reader of its own that
// hands the capture to a generic one, a package variable holding a reader,
// and a reader called with a pair of values, which leaves what that reader
// decodes paired with nothing; a converter that takes the details beside the
// capture it decodes; a list of gadgets whose owners the shared reader hands
// back; a widget read through the shared generic reader that takes it; a
// function that starts two captures on one context, each paired with what is
// sent under it alone; a gadget decoded in place, whose owner holds a widget;
// a struct decoded in place and handed beside the gadget's owner to a
// converter, which is not the answer it was decoded from; two struct
// literals, one decoded in place and one through a generic
// reader; a delete, which answers with no struct; a struct decoded beside a
// delete and beside a request a helper sends through a method value, which
// the walk cannot see, and one decoded beside a delete and beside a request
// sent under a context of its own kept in a field, which it does not
// follow; a helper
// handed the captured context that answers
// with the widget it fetched; decodes into what no struct of the package's
// own is, a reader of client-go's own widget and a function that takes that
// widget beside its details among them; a method of a generic type, whose
// instance the walk cannot read; and what only looks like a capture to a
// looser walk.
const capturesTool = `package widgets

import (
	"context"

	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"
	gitlabclient "example.com/fixture/internal/gitlab"
	"example.com/fixture/internal/toolutil"
)

type detailsExtra struct {
	Archived bool ~json:"archived"~
}

type ownerExtra struct {
	Name   string ~json:"name"~
	Avatar string ~json:"avatar"~
}

type level int

type decoder struct{}

func (decoder) Decode(v any) error { return nil }

type pairDecoder struct{}

func (pairDecoder) Decode(v, w any) error { return nil }

type holder struct {
	ctx     context.Context
	capture *gitlabclient.ResponseCapture
}

type reader[T any] struct{}

func (reader[T]) read(capture *gitlabclient.ResponseCapture) (T, error) {
	var v T
	return v, capture.Decode(&v)
}

func noOption() gl.RequestOptionFunc { return nil }

type gadgetExtra struct {
	Owner *ownerExtra ~json:"owner"~
}

type countExtra struct {
	Total int ~json:"total"~
}

type threadExtra struct {
	Archived *bool ~json:"archived"~
}

type deleteExtra struct {
	Message string ~json:"message"~
}

type mixedExtra struct {
	Message string ~json:"message"~
}

type fetchedExtra struct {
	Archived bool ~json:"archived"~
}

func Get(ctx context.Context, c *gl.Client) (string, error) {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	w, _, err := c.Widgets.GetWidget(1, gl.WithContext(ctx))
	if err != nil {
		return "", err
	}
	extra, err := toolutil.CapturedWidget(captured)
	if err != nil {
		return "", err
	}
	return toolutil.WidgetOutput(w, extra), nil
}

func decodeOne[T any](capture *gitlabclient.ResponseCapture) (T, error) {
	var v T
	err := capture.Decode(&v)
	return v, err
}

func capturedDetails(capture *gitlabclient.ResponseCapture) (detailsExtra, error) {
	return decodeOne[detailsExtra](capture)
}

var detailsSeam = func(capture *gitlabclient.ResponseCapture) (detailsExtra, error) {
	return capturedDetails(capture)
}

func pairOf(capture *gitlabclient.ResponseCapture) (int, *gitlabclient.ResponseCapture) { return 1, capture }

func counted(n int, capture *gitlabclient.ResponseCapture) (countExtra, error) {
	var extra countExtra
	return extra, capture.Decode(&extra)
}

func Details(ctx context.Context, c *gl.Client) error {
	_, _, _ = c.Widgets.GetGadget(1, gl.WithContext(ctx))
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, _, err := c.Widgets.GetWidgetDetails(1, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	_, _ = c.Widgets.DeleteWidget(1, gl.WithContext(ctx))
	_, _, _ = c.Widgets.CountWidgets(gl.WithContext(ctx), nil)
	_, _ = detailsSeam(captured)
	_, _ = counted(pairOf(captured))
	return nil
}

func convertDetails(d *gl.WidgetDetails, captured *gitlabclient.ResponseCapture) (bool, error) {
	extra, err := capturedDetails(captured)
	return extra.Archived, err
}

func convertEarly(g *gl.Gadget, captured *gitlabclient.ResponseCapture) error {
	_, err := earlyReader(captured)
	return err
}

func earlyReader(capture *gitlabclient.ResponseCapture) (earlyExtra, error) {
	return lateReader(capture)
}

func lateReader(capture *gitlabclient.ResponseCapture) (earlyExtra, error) {
	var extra earlyExtra
	return extra, capture.Decode(&extra)
}

type earlyExtra struct {
	Flag bool ~json:"flag"~
}

func render(c *gl.Client, extra toolutil.WidgetExtra) string { return extra.DeleteAPIPath }

func Owners(ctx context.Context, c *gl.Client) error {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	gadgets, _, _ := c.Widgets.ListGadgets(gl.WithContext(ctx))
	_, err := toolutil.CapturedOwners(captured, len(gadgets))
	return err
}

func Thread(ctx context.Context, c *gl.Client) error {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	w, _, _ := c.Widgets.GetWidget(1, gl.WithContext(ctx))
	_, err := toolutil.CapturedThread[threadExtra](w, captured)
	return err
}

func Literal(ctx context.Context, c *gl.Client) error {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, _, _ = c.Widgets.GetWidget(1, gl.WithContext(ctx))
	var named struct {
		Name *string ~json:"name"~
	}
	_ = captured.Decode(&named)
	_, err := decodeOne[struct {
		Color string ~json:"color"~
	}](captured)
	return err
}

func Delete(ctx context.Context, c *gl.Client) error {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, _ = c.Widgets.DeleteWidget(1, gl.WithContext(ctx))
	var extra deleteExtra
	return captured.Decode(&extra)
}

type fieldedExtra struct {
	Message string ~json:"message"~
}

func DeleteFielded(ctx context.Context, c *gl.Client) error {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, _ = c.Widgets.DeleteWidget(1, gl.WithContext(ctx))
	var extra fieldedExtra
	return captured.Decode(&extra)
}

func Fielded(ctx context.Context, c *gl.Client) error {
	fieldCtx, captured := gitlabclient.WithResponseCapture(ctx)
	h := holder{ctx: fieldCtx}
	_, _, _ = c.Widgets.GetWidget(1, gl.WithContext(h.ctx))
	var extra fieldedExtra
	return captured.Decode(&extra)
}

func send(ctx context.Context, remove func(int64, ...gl.RequestOptionFunc) (*gl.Response, error)) error {
	_, err := remove(1, gl.WithContext(ctx))
	return err
}

func Mixed(ctx context.Context, c *gl.Client) error {
	ctx, first := gitlabclient.WithResponseCapture(ctx)
	_, _ = c.Widgets.DeleteWidget(1, gl.WithContext(ctx))
	var deleted mixedExtra
	_ = first.Decode(&deleted)
	ctx, second := gitlabclient.WithResponseCapture(ctx)
	_ = send(ctx, c.Widgets.DeleteWidget)
	var sent mixedExtra
	return second.Decode(&sent)
}

func fetch(ctx context.Context, c *gl.Client) (*gl.Widget, error) {
	w, _, err := c.Widgets.GetWidget(1, gl.WithContext(ctx))
	return w, err
}

type listedExtra struct {
	Avatar string ~json:"avatar"~
}

func Listed(ctx context.Context, c *gl.Client) string {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	g, _, _ := c.Widgets.GetGadget(1, gl.WithContext(ctx))
	var extra listedExtra
	_ = captured.Decode(&extra)
	return ownerOf(g.Owner, extra)
}

func ownerOf(w *gl.Widget, extra listedExtra) string { return extra.Avatar }

func widgetOf(capture *gitlabclient.ResponseCapture) (gl.Widget, error) {
	var w gl.Widget
	return w, capture.Decode(&w)
}

func sameWidget(w gl.Widget, d *gl.WidgetDetails) bool { return w.ID == d.ID }

func Fetched(ctx context.Context, c *gl.Client) error {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	if _, err := fetch(ctx, c); err != nil {
		return err
	}
	var extra fetchedExtra
	return captured.Decode(&extra)
}

func Twice(ctx context.Context, c *gl.Client) error {
	ctx, first := gitlabclient.WithResponseCapture(ctx)
	_, _, _ = c.Widgets.GetGadget(1, gl.WithContext(ctx))
	var gadget gadgetExtra
	_ = first.Decode(&gadget)
	ctx, second := gitlabclient.WithResponseCapture(ctx)
	_, _, _ = c.Widgets.ListWidgets(noOption(), gl.WithContext(ctx))
	c.Widgets.Ping(gl.WithContext(ctx))
	_, _ = toolutil.CapturedWidgets(second, 1)
	var widget gl.Widget
	_ = second.Decode(&widget)
	var loose map[string]any
	_ = second.Decode(&loose)
	var lv level
	_ = second.Decode(&lv)
	return nil
}

func Generic(ctx context.Context, c *gl.Client) error {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, _, _ = c.Widgets.GetWidgetDetails(1, gl.WithContext(ctx))
	_, _ = reader[detailsExtra]{}.read(captured)
	return nil
}

func Held(ctx context.Context, c *gl.Client) error {
	var h holder
	var captured *gitlabclient.ResponseCapture
	h.ctx, captured = gitlabclient.WithResponseCapture(ctx)
	_, _, _ = c.Widgets.GetGadget(1, gl.WithContext(h.ctx))
	var gadget gadgetExtra
	_ = captured.Decode(&gadget)
	_ = h.capture.Decode(&gadget)
	_, _ = toolutil.CapturedWidget(h.capture)
	return nil
}

func WithResponseCapture(ctx context.Context) (context.Context, *gitlabclient.ResponseCapture) {
	return ctx, nil
}

func WithContext(ctx context.Context) gl.RequestOptionFunc { return nil }

func Lookalikes(ctx context.Context, c *gl.Client) error {
	ctx, captured := WithResponseCapture(ctx)
	_, _, _ = c.Widgets.GetWidget(1, gl.WithContext(ctx))
	_, _ = toolutil.CapturedWidget(captured)
	ctx, actual := gitlabclient.WithResponseCapture(ctx)
	_, _, _ = c.Widgets.GetGadget(1, WithContext(ctx))
	var gadget gadgetExtra
	return actual.Decode(&gadget)
}

func Unrelated(ctx context.Context, c *gl.Client) error {
	ctx, n := gitlabclient.NotACapture(ctx)
	_, _, _ = c.Widgets.GetWidget(int64(n), gl.WithContext(ctx))
	var widgets []*gl.Widget
	_ = decoder{}.Decode(&widgets)
	_ = pairDecoder{}.Decode(&widgets, nil)
	var captured, other int
	captured, other = n, n
	_, _ = captured, other
	found := map[string]int{}
	value, ok := found["key"]
	_, _ = value, ok
	return nil
}
`

// capturesFixtureFiles are the files of the stand-in tree, the SDK put
// through sdk (nil keeps the "v3.30.0" state).
func capturesFixtureFiles(sdk *strings.Replacer) map[string]string {
	source := capturesSDK
	if sdk != nil {
		source = sdk.Replace(source)
	}
	return map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.27\n",
		"gitlab.com/gitlab-org/api/client-go/v3/sdk.go": strings.ReplaceAll(source, "~", "`"),
		"internal/gitlab/capture.go":                    capturesGitLab,
		"internal/toolutil/toolutil.go":                 strings.ReplaceAll(capturesToolutil, "~", "`"),
		"internal/tools/widgets/widgets.go":             strings.ReplaceAll(capturesTool, "~", "`"),
	}
}

// widgetGainsTheCapturedKeys is the "v3.38.0" state of the stand-in: the
// widget models the delete_api_path the capture reads, and its size as a
// pointer, which tells a size not sent from a size of zero.
var widgetGainsTheCapturedKeys = strings.NewReplacer(
	"Size int64  ~json:\"size\"~", "Size *int64 ~json:\"size\"~\n\tDeleteAPIPath string ~json:\"delete_api_path\"~",
)

// sizeKeptForPresence answers the one key the stand-in's capture shares with
// the widget on the "v3.30.0" state.
var sizeKeptForPresence = []captureDeclaration{{
	Type: "toolutil.WidgetExtra", Key: "size", Category: keptForPresence,
	Predicates: []waits.Predicate{waits.FieldKind{Type: "Widget", Field: "Size", Kind: "int64"}},
	Reason:     "a widget GitLab sends no size for reads as size 0",
}}

// analyzeCapturesFixture loads the stand-in tree with the SDK put through sdk
// and builds the section with declared.
func analyzeCapturesFixture(t *testing.T, sdk *strings.Replacer, extra map[string]string, declared []captureDeclaration) (capturesReport, []string) {
	t.Helper()
	files := capturesFixtureFiles(sdk)
	maps.Copy(files, extra)
	root := t.TempDir()
	writeFiles(t, root, files)
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load the fixture: %v", err)
	}
	clientGo, err := shared.ClientGoTypes(pkgs)
	if err != nil {
		t.Fatalf("client-go types: %v", err)
	}
	return analyzeCaptures(pkgs, clientGo, declared)
}

// standInDeclarations answer the keys the stand-in's captures share with
// client-go on the "v3.30.0" state: the widget's size, kept for its
// presence, on the widget's extra and below each gadget's owner, the name a
// struct literal reads, kept for its presence too, and the gadget's owner,
// read whole for an avatar the widget does not model.
var standInDeclarations = slices.Concat(sizeKeptForPresence, []captureDeclaration{
	{
		Type: "toolutil.ownerRow", Key: "owner.size", Category: keptForPresence,
		Predicates: []waits.Predicate{waits.FieldKind{Type: "Widget", Field: "Size", Kind: "int64"}},
		Reason:     "a widget GitLab sends no size for reads as size 0",
	},
	{
		Type: "widgets.Literal.struct{Name}", Key: "name", Category: keptForPresence,
		Predicates: []waits.Predicate{waits.FieldKind{Type: "Widget", Field: "Name", Kind: "string"}},
		Reason:     "a widget GitLab sends no name for reads as an empty name",
	},
	{
		Type: "widgets.gadgetExtra", Key: "owner", Category: keptWithItsObject,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "Widget", Key: "avatar"}},
		Reason:     "the owner is read whole for its avatar",
	},
})

// TestAnalyzeCaptures_AStandInTree_PairsEachCaptureWithWhatItWasDecodedBeside
// verifies the walk on a tree whose universe is known. A capture is decoded
// directly, through a reader of the shared package read from its body,
// through a reader of the package's own, through a generic one instantiated
// by its caller, two generic ones deep, through a package variable holding a
// reader and through a reader handed a pair of values; it is paired with the
// struct a service method answers with when the method is sent under the
// context the capture started, after it started and before another capture
// started on the same context, with the struct a function handed that
// context answers with, and with the struct a converter takes beside what a
// reader decoded, but not beside a struct decoded in place, which a
// converter may be handed with an object inside the answer. A reader that
// decodes the rows of a list and hands back an object inside each is paired
// at the rows' depth, and a struct literal is paired as a named struct is,
// spelled by the function it is written in. A method that
// answers with nothing, with nothing to decode or with no struct pairs with
// nothing, nor does one sent before the capture, under a context held in a
// field, under a context no capture started, under a capture started by a
// function of the package's own of the same name, or with an option of the
// package's own named like client-go's; client-go's own struct, a map and a
// named number a capture is decoded into are no captured struct, and the
// type parameter of a generic type's method stays one.
func TestAnalyzeCaptures_AStandInTree_PairsEachCaptureWithWhatItWasDecodedBeside(t *testing.T) {
	report, stale := analyzeCapturesFixture(t, nil, nil, standInDeclarations)
	want := []capturedType{
		{Type: "toolutil.WidgetExtra", Pairs: []string{"Widget"}, Sites: []string{"toolutil.WidgetOutput", "widgets.Get", "widgets.Twice"}},
		{Type: "toolutil.ownerRow", Pairs: []string{"Gadget"}, Sites: []string{"widgets.Owners"}},
		{Type: "widgets.Literal.struct{Color}", Pairs: []string{"Widget"}, Sites: []string{"widgets.Literal"}},
		{Type: "widgets.Literal.struct{Name}", Pairs: []string{"Widget"}, Sites: []string{"widgets.Literal"}},
		{Type: "widgets.detailsExtra", Pairs: []string{"WidgetDetails"}, Sites: []string{"widgets.Details", "widgets.convertDetails"}},
		{Type: "widgets.earlyExtra", Pairs: []string{"Gadget"}, Sites: []string{"widgets.convertEarly"}},
		{Type: "widgets.fetchedExtra", Pairs: []string{"Widget"}, Sites: []string{"widgets.Fetched"}},
		{Type: "widgets.gadgetExtra", Pairs: []string{"Gadget"}, Sites: []string{"widgets.Twice"}},
		{Type: "widgets.listedExtra", Pairs: []string{"Gadget"}, Sites: []string{"widgets.Listed"}},
		{Type: "widgets.threadExtra", Pairs: []string{"Widget"}, Sites: []string{"widgets.Thread"}},
	}
	if !reflect.DeepEqual(report.Types, want) {
		t.Errorf("types =\n%+v\nwant\n%+v", report.Types, want)
	}
	if len(stale) != 0 {
		t.Errorf("stale = %q, want none", stale)
	}
}

// TestAnalyzeCaptures_AStandInTree_ListsWhatItPairsWithNothing verifies the
// structs a capture is decoded into that the walk pairs with no client-go
// struct. One whose capture was sent under a delete, which answers with the
// response and the error alone, has no SDK half; one decoded from a capture
// no walked function starts, one decoded beside a request a helper sent
// through a method value as well as beside a delete, and one decoded beside
// a delete and beside a capture nothing the walk sees is sent under, its
// context kept in a field, are pairings the walk does not see, a finding
// each.
func TestAnalyzeCaptures_AStandInTree_ListsWhatItPairsWithNothing(t *testing.T) {
	report, _ := analyzeCapturesFixture(t, nil, nil, standInDeclarations)
	want := []unpairedCapture{
		{Type: "widgets.countExtra", Sites: []string{"widgets.counted"}, Status: captureUnpaired},
		{Type: "widgets.deleteExtra", Sites: []string{"widgets.Delete"}, Status: captureAnswersNoStruct},
		{Type: "widgets.fieldedExtra", Sites: []string{"widgets.DeleteFielded", "widgets.Fielded"}, Status: captureUnpaired},
		{Type: "widgets.mixedExtra", Sites: []string{"widgets.Mixed"}, Status: captureUnpaired},
	}
	if !reflect.DeepEqual(report.Unpaired, want) {
		t.Errorf("unpaired =\n%+v\nwant\n%+v", report.Unpaired, want)
	}
}

// TestAnalyzeCaptures_AReaderOfAnObjectInsideTheAnswer_PairsItAtThatDepth
// verifies that the widget's extra a reader hands back from inside each row
// of a gadget list is not paired with the gadget: what the capture is
// decoded into is the row, so a declaration that keeps a key of the widget's
// extra because the gadget does not model it pairs it with a struct it is
// never read beside, and lacks what its category requires.
func TestAnalyzeCaptures_AReaderOfAnObjectInsideTheAnswer_PairsItAtThatDepth(t *testing.T) {
	declared := []captureDeclaration{{
		Type: "toolutil.WidgetExtra", Key: "delete_api_path", Category: keptForSiblingPairing,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "Gadget", Key: "delete_api_path"}},
		Reason:     "a gadget does not model the widget's delete_api_path",
	}}
	_, stale := analyzeCapturesFixture(t, widgetGainsTheCapturedKeys, nil, declared)
	want := "capture declaration toolutil.WidgetExtra.delete_api_path: kept-for-sibling-pairing needs " + captureCategories[keptForSiblingPairing].lacks
	if !slices.Contains(stale, want) {
		t.Errorf("stale = %q, want it to hold %q", stale, want)
	}
}

// TestAnalyzeCaptures_TheSDKModelsACapturedKey_TheCaptureIsRedundant is the
// stand-in for the bump. On "v3.30.0" the widget models no delete_api_path
// and its size as a number, so the capture reads the first beside it and
// keeps the second for its presence, declared, on the widget's extra and
// below each gadget's owner, a struct literal keeps the name for its
// presence, and the gadget's owner is read whole for an avatar the widget
// does not model, its name below it answered by the declaration on the
// object. Once the widget models the delete_api_path and a size it can tell
// from zero, the first is a key read twice for nothing at both depths and
// the declarations of the second no longer hold: captures to retire.
func TestAnalyzeCaptures_TheSDKModelsACapturedKey_TheCaptureIsRedundant(t *testing.T) {
	cases := []struct {
		name string
		sdk  *strings.Replacer
		want []string
	}{
		{
			name: "v3.30.0",
			want: []string{
				"toolutil.WidgetExtra.size kept",
				"toolutil.ownerRow.owner.size kept",
				"widgets.Literal.struct{Name}.name kept",
				"widgets.gadgetExtra.owner.name kept",
			},
		},
		{
			name: "v3.38.0",
			sdk:  widgetGainsTheCapturedKeys,
			want: []string{
				"toolutil.WidgetExtra.delete_api_path redundant_capture",
				"toolutil.WidgetExtra.size obsolete_capture",
				"toolutil.ownerRow.owner.delete_api_path redundant_capture",
				"toolutil.ownerRow.owner.size obsolete_capture",
				"widgets.Literal.struct{Name}.name kept",
				"widgets.gadgetExtra.owner.name kept",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, stale := analyzeCapturesFixture(t, tc.sdk, nil, standInDeclarations)
			var got []string
			for _, key := range report.Keys {
				got = append(got, key.Type+"."+key.Key+" "+key.Status)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("keys = %q, want %q", got, tc.want)
			}
			if len(stale) != 0 {
				t.Errorf("stale = %q, want none", stale)
			}
		})
	}
}

// TestAnalyzeCaptures_ATreeWithoutTheSharedPackage_PairsWhatItsOwnReadersRead
// verifies that a tree whose tool packages capture without importing the
// shared package pairs what their own readers decode, and that a package
// that captures a response but declares no capture captures nothing.
func TestAnalyzeCaptures_ATreeWithoutTheSharedPackage_PairsWhatItsOwnReadersRead(t *testing.T) {
	const tool = `package widgets

import (
	"context"

	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"
	gitlabclient "example.com/fixture/internal/gitlab"
)

type detailsExtra struct {
	ID *int64 ~json:"id"~
}

func Details(ctx context.Context, c *gl.Client) error {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, _, _ = c.Widgets.GetWidgetDetails(1, gl.WithContext(ctx))
	var extra detailsExtra
	return captured.Decode(&extra)
}
`
	files := map[string]string{
		"internal/toolutil/toolutil.go":     "package toolutil\n",
		"internal/tools/widgets/widgets.go": strings.ReplaceAll(tool, "~", "`"),
	}
	report, _ := analyzeCapturesFixture(t, nil, files, nil)
	var got []string
	for _, key := range report.Keys {
		got = append(got, key.Type+"."+key.Key+" "+key.Status)
	}
	if !slices.Equal(got, []string{"widgets.detailsExtra.id redundant_capture"}) {
		t.Errorf("keys = %q, want the details' id read twice", got)
	}
	files["internal/gitlab/capture.go"] = "package gitlab\n\nimport \"context\"\n\nfunc WithResponseCapture(ctx context.Context) (context.Context, *int) { return ctx, nil }\n"
	files["internal/tools/widgets/widgets.go"] = "package widgets\n\nimport (\n\t\"context\"\n\n\tgl \"example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3\"\n\tgitlabclient \"example.com/fixture/internal/gitlab\"\n)\n\nfunc Details(ctx context.Context, c *gl.Client) { _, _ = gitlabclient.WithResponseCapture(ctx) }\n"
	if undeclared, _ := analyzeCapturesFixture(t, nil, files, nil); len(undeclared.Types) != 0 || len(undeclared.Keys) != 0 {
		t.Errorf("report = %+v, want nothing where no capture is declared", undeclared)
	}
}

// TestAnalyzeCaptures_NoPackageCaptures_FindsNothing verifies that a tree
// whose tool packages import no capture has nothing to pair, and that every
// declaration is then stale.
func TestAnalyzeCaptures_NoPackageCaptures_FindsNothing(t *testing.T) {
	report, stale := analyzeCapturesFixture(t, nil, map[string]string{
		"internal/tools/widgets/widgets.go": "package widgets\n\nimport gl \"example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3\"\n\nfunc Nothing(c *gl.Client) {}\n",
	}, sizeKeptForPresence)
	if len(report.Types) != 0 || len(report.Keys) != 0 {
		t.Errorf("report = %+v, want nothing", report)
	}
	if want := []string{"capture declaration toolutil.WidgetExtra.size answers no key a capture shares with the client-go struct beside it"}; !slices.Equal(stale, want) {
		t.Errorf("stale = %q, want %q", stale, want)
	}
}

// TestSummarizeCaptures_EachStatus_CountsOnce verifies the counters a
// captures section adds to the summary, and that gaps-only keeps the keys
// and the unpaired captures that are a finding and drops the types.
func TestSummarizeCaptures_EachStatus_CountsOnce(t *testing.T) {
	report := capturesReport{
		Types: []capturedType{{Type: "a"}, {Type: "b"}},
		Keys: []capturedKey{
			{Key: "kept", Status: captureKept},
			{Key: "redundant", Status: captureRedundant},
			{Key: "stale", Status: captureStalePredicate},
			{Key: "obsolete", Status: captureObsolete},
			{Key: "also kept", Status: captureKept},
		},
		Unpaired: []unpairedCapture{
			{Type: "no struct", Status: captureAnswersNoStruct},
			{Type: "unpaired", Status: captureUnpaired},
			{Type: "also no struct", Status: captureAnswersNoStruct},
		},
	}
	var summary reportSummary
	summarizeCaptures(&summary, report)
	want := reportSummary{
		CapturedTypes: 2, CapturedKeysShared: 5, CapturesKept: 2, CapturesRedundant: 1, CapturesStalePredicate: 1, CapturesObsolete: 1,
		CapturesAnswerNoStruct: 2, CapturesUnpaired: 1,
	}
	if summary != want {
		t.Errorf("summary = %+v, want %+v", summary, want)
	}
	kept := keepCaptureFindings(report)
	var keys []string
	for _, key := range kept.Keys {
		keys = append(keys, key.Key)
	}
	if !slices.Equal(keys, []string{"redundant", "stale", "obsolete"}) || kept.Types == nil || len(kept.Types) != 0 {
		t.Errorf("gaps-only = %+v, want the three findings and no type", kept)
	}
	if want := []unpairedCapture{{Type: "unpaired", Status: captureUnpaired}}; !reflect.DeepEqual(kept.Unpaired, want) {
		t.Errorf("gaps-only unpaired = %+v, want %+v", kept.Unpaired, want)
	}
	if none := keepCaptureFindings(capturesReport{}); none.Unpaired == nil || none.Keys == nil {
		t.Errorf("gaps-only of an empty section = %+v, want empty lists rather than null", none)
	}
}

// capturesWitness is the part of the sdk report the witness reads, decoded as
// plain JSON so that it decodes a report with no such section too.
type capturesWitness struct {
	Captures *struct {
		Types []struct {
			Type  string
			Pairs []string
		}
		Keys []struct {
			Type, Key, Status, Declared, Category string
		}
		Unpaired []struct {
			Type, Status string
		}
	} `json:"captures"`
}

// witnessKept is the status of a key a declaration answers while its
// predicates hold, spelled here so that the witness, copied into a tree
// without the section, compiles and fails there.
const witnessKept = "kept"

// witnessPairs is what the witness expects the walk to pair the structs two
// readers of the shared package hand back from inside the answer with: the
// list of file diffs a comparison carries under diffs, paired with each
// diff and not with the comparison, and the users a reviewer's and a
// starrer's row nest under user, paired at the row's depth, the users
// themselves only with the lists of users, and the comparison's wrapper the
// reader decodes, paired with the comparison; and the wiki blobs a search
// decodes, paired through the helper it hands the captured context to.
var witnessPairs = map[string]string{
	"toolutil.DiffExtra":                          "Diff",
	"toolutil.UserBasicExtra":                     "BasicUser ProjectUser",
	"toolutil.nestedUserExtra":                    "MergeRequestReviewer ProjectStarrer",
	"toolutil.CapturedCompareDiffs.struct{Diffs}": "Compare",
	"search.WikiBlobOutput":                       "Wiki",
}

// witnessUnpaired is what the witness expects of the structs a capture is
// decoded into that the walk pairs with nothing: the two decoded beside a
// service method that answers with the response and the error alone.
var witnessUnpaired = []string{
	"projects.shareLink answers_no_struct",
	"toolutil.DeploymentApprovalOutput answers_no_struct",
}

// witnessDeclared is what the witness expects of the captures on v3.30.0:
// each declaration by the type and key it is written on, with its category
// and how many keys both a capture and the client-go struct beside it read
// it answers.
var witnessDeclared = map[string]string{
	"branches.capturedBranch.commit.extended_trailers":                        "kept-for-value 1",
	"commits.Captured.extended_trailers":                                      "kept-for-value 1",
	"mrchanges.capturedDiffVersion.commits.extended_trailers":                 "kept-for-value 1",
	"mrcontextcommits.capturedCommit.extended_trailers":                       "kept-for-value 1",
	"repository.capturedCompare.commit.extended_trailers":                     "kept-for-value 1",
	"repository.capturedCompare.commits.extended_trailers":                    "kept-for-value 1",
	"toolutil.DiscussionExtra.notes.confidential":                             "kept-for-value 1",
	"toolutil.NoteExtra.confidential":                                         "kept-for-value 1",
	"dorametrics.capturedMetric.value":                                        "kept-for-null 1",
	"mergetrains.trainMeasures.duration":                                      "kept-for-null 1",
	"mergetrains.trainMeasures.pipeline.duration":                             "kept-for-null 1",
	"mergetrains.trainMeasures.pipeline.queued_duration":                      "kept-for-null 1",
	"resourceevents.capturedWeight.weight":                                    "kept-for-null 1",
	"toolutil.BoardExtra.weight":                                              "kept-for-null 1",
	"toolutil.BridgeExtra.coverage":                                           "kept-for-null 1",
	"toolutil.BridgeExtra.duration":                                           "kept-for-null 1",
	"toolutil.BridgeExtra.queued_duration":                                    "kept-for-null 1",
	"toolutil.JobMeasures.coverage":                                           "kept-for-null 1",
	"toolutil.JobMeasures.duration":                                           "kept-for-null 1",
	"toolutil.JobMeasures.queued_duration":                                    "kept-for-null 1",
	"toolutil.MergeRequestExtra.head_pipeline.duration":                       "kept-for-null 1",
	"toolutil.MergeRequestExtra.head_pipeline.queued_duration":                "kept-for-null 1",
	"toolutil.NamespaceExtra.additional_purchased_storage_size":               "kept-for-null 1",
	"toolutil.NamespaceExtra.extra_shared_runners_minutes_limit":              "kept-for-null 1",
	"toolutil.NamespaceExtra.shared_runners_minutes_limit":                    "kept-for-null 1",
	"toolutil.PipelineExtra.duration":                                         "kept-for-null 1",
	"toolutil.PipelineExtra.queued_duration":                                  "kept-for-null 1",
	"mrapprovals.enterpriseApprovalCapture.approvals_left":                    "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.approvals_required":                "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.description":                       "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.has_approval_rules":                "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.id":                                "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.iid":                               "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.merge_request_approvers_available": "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.merge_status":                      "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.multiple_approval_rules_available": "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.project_id":                        "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.require_password_to_approve":       "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.state":                             "kept-for-presence 1",
	"mrapprovals.enterpriseApprovalCapture.title":                             "kept-for-presence 1",
	"toolutil.MemberExtra.group_saml_identity":                                "kept-for-sibling-pairing 3",
	"toolutil.MemberExtra.public_email":                                       "kept-for-sibling-pairing 1",
	"toolutil.MergeRequestExtra.work_in_progress":                             "kept-for-sibling-pairing 1",
	"toolutil.RunnerExtra.token_expires_at":                                   "kept-for-sibling-pairing 1",
	"toolutil.ServiceAccountExtra.public_email":                               "kept-for-sibling-pairing 1",
	"mergerequests.approvalsExtra.approved_by":                                "kept-with-its-object 7",
	"mrapprovals.enterpriseApprovalCapture.approver_groups":                   "kept-with-its-object 11",
	"mrapprovals.enterpriseApprovalCapture.approvers":                         "kept-with-its-object 6",
	"mrapprovals.enterpriseApprovalCapture.suggested_approvers":               "kept-with-its-object 6",
}

// TestRun_Repository_DeclaresEveryKeyACaptureSharesWithClientGoV3300 is the
// witness on the real tree. A struct a handler decodes off a captured
// response (ADR-0021) reads keys the client-go struct decoded from the same
// answer does not model, and nothing asked whether it also reads keys the
// struct does: client-go v3.31 to v3.38 model thirty more of them, and on
// v3.30.0 seventy-seven are read twice already. Each is kept for a reason a
// predicate on the pinned SDK says, or the capture is retired: the approval
// state's two timestamps and its pending rules, which client-go decodes as
// GitLab sends them, are read off its decode now. A reader that hands back
// an object from inside the answer is paired at that object's depth, and
// every struct a capture is decoded into is paired or has no SDK half. The
// bump stack is where the thirty appear and this list changes.
//
// It reads the report as plain JSON and spells the status it expects, so
// copied on its own into a tree that has no such section it compiles and
// fails there, which is the point.
func TestRun_Repository_DeclaresEveryKeyACaptureSharesWithClientGoV3300(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, _, err := Run(root, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var rep capturesWitness
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	if rep.Captures == nil {
		t.Fatal("the sdk report has no captures section: nothing holds a key a capture reads to the client-go struct decoded beside it")
	}
	unseen := maps.Clone(witnessPairs)
	for _, captured := range rep.Captures.Types {
		if want, pinned := witnessPairs[captured.Type]; pinned && strings.Join(captured.Pairs, " ") != want {
			t.Errorf("%s pairs %q, want %q", captured.Type, captured.Pairs, want)
		}
		delete(unseen, captured.Type)
	}
	for _, missing := range slices.Sorted(maps.Keys(unseen)) {
		t.Errorf("%s is paired with nothing, want it paired", missing)
	}
	var unpaired []string
	for _, captured := range rep.Captures.Unpaired {
		unpaired = append(unpaired, captured.Type+" "+captured.Status)
	}
	if !slices.Equal(unpaired, witnessUnpaired) {
		t.Errorf("unpaired = %q, want %q", unpaired, witnessUnpaired)
	}
	got := map[string]string{}
	answered := map[string]int{}
	for _, key := range rep.Captures.Keys {
		if key.Status != witnessKept {
			t.Errorf("%s.%s is %s, want kept", key.Type, key.Key, key.Status)
		}
		id := key.Type + "." + key.Declared
		answered[id]++
		got[id] = key.Category + " " + strconv.Itoa(answered[id])
	}
	if !reflect.DeepEqual(got, witnessDeclared) {
		for _, id := range slices.Sorted(maps.Keys(witnessDeclared)) {
			if got[id] != witnessDeclared[id] {
				t.Errorf("%s = %q, want %q", id, got[id], witnessDeclared[id])
			}
		}
		for _, id := range slices.Sorted(maps.Keys(got)) {
			if _, expected := witnessDeclared[id]; !expected {
				t.Errorf("%s = %q, which the witness does not expect", id, got[id])
			}
		}
	}
}
