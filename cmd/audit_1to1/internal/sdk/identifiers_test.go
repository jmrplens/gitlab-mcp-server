// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// identifiersSDK is the stand-in client-go of the identifiers section: a
// service whose methods take an identifier as any, one routed where the
// record says a name is taken, one where an ID alone is, ones the record
// describes only by type, only through another route of the namespace, or
// not at all, and one the record does not hold; and methods that take an
// integer where the record says a name or a path is taken, one sending two
// such routes and one filling two such placeholders.
const identifiersSDK = `package gitlab

import (
	"context"
	"net/http"
)

type RequestOptionFunc func() error

func WithContext(ctx context.Context) RequestOptionFunc { return nil }

type Response struct{}

type Widget struct{ ID int64 }

// WidgetID wraps an identifier for a path, the way client-go's ProjectID does.
type WidgetID struct{ Value any }

const DefaultWidget = 1

type option func()

func route(template string) string                { return template }
func withPath(path string, args ...any) option    { return nil }
func withMethod(method string) option             { return nil }
func do[T any](c *Client, options ...option) (T, *Response, error) {
	var zero T
	return zero, nil, nil
}

var (
	routeWidgetEvents  = route("widgets/%s/events")
	routeWidgetDetails = route("widgets/%s/details")
	routeWidgetOwner   = route("widgets/%d/owner")
	routeGadget        = route("gadgets/%s")
	routeGadgetPart    = route("gadgets/%s/parts/%d")
	routeGadgetClone   = route("gadgets/%d/clone")
	routeGadgetCopy    = route("gadgets/%d/copy")
	routeCog           = route("cogs/%s")
	routeBolt          = route("bolts/%s")
	routeBoltThreads   = route("bolts/%s/threads")
	routeBoltLatest    = route("bolts/%s/threads/latest")
	routeGadgetMove    = route("gadgets/%d/move/%d")
	routeNut           = route("nuts/%s")
	routeWasher        = route("washers/%s")
	routeSprocket      = route("sprockets/%s")
	routeSprocketTeeth = route("sprockets/%d/teeth")
)

type WidgetsServiceInterface interface {
	ListWidgetEvents(uid any, options ...RequestOptionFunc) ([]*Widget, *Response, error)
	GetWidgetDetails(wid any, options ...RequestOptionFunc) (*Widget, *Response, error)
	GetWidgetOwner(wid int64, options ...RequestOptionFunc) (*Widget, *Response, error)
	GetGadget(gid any, options ...RequestOptionFunc) (*Widget, *Response, error)
	UpdateGadget(gid int64, options ...RequestOptionFunc) (*Widget, *Response, error)
	DeleteGadget(gid int64, options ...RequestOptionFunc) (*Response, error)
	GetGadgetPart(gid any, part int64, options ...RequestOptionFunc) (*Widget, *Response, error)
	CloneGadget(gid int64, options ...RequestOptionFunc) (*Widget, *Response, error)
	CopyGadget(gid int64, options ...RequestOptionFunc) (*Widget, *Response, error)
	GetCog(cid any, options ...RequestOptionFunc) (*Widget, *Response, error)
	GetBolt(bid any, options ...RequestOptionFunc) (*Widget, *Response, error)
	ListBoltThreads(bid any, options ...RequestOptionFunc) ([]*Widget, *Response, error)
	GetLatestBoltThread(bid any, options ...RequestOptionFunc) (*Widget, *Response, error)
	MoveGadget(gid, target int64, options ...RequestOptionFunc) (*Widget, *Response, error)
	GetNut(nid any, options ...RequestOptionFunc) (*Widget, *Response, error)
	GetWasher(wid any, options ...RequestOptionFunc) (*Widget, *Response, error)
	GetSprocket(sid any, options ...RequestOptionFunc) (*Widget, *Response, error)
	ListSprocketTeeth(sid int64, options ...RequestOptionFunc) ([]*Widget, *Response, error)
	Count(n int) int
}

type WidgetsService struct{ client *Client }

func (s *WidgetsService) ListWidgetEvents(uid any, options ...RequestOptionFunc) ([]*Widget, *Response, error) {
	return do[[]*Widget](s.client, withPath(routeWidgetEvents, WidgetID{uid}))
}

func (s *WidgetsService) GetWidgetDetails(wid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeWidgetDetails, WidgetID{wid}))
}

func (s *WidgetsService) GetWidgetOwner(wid int64, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeWidgetOwner, wid))
}

func (s *WidgetsService) GetGadget(gid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeGadget, WidgetID{gid}))
}

func (s *WidgetsService) UpdateGadget(gid int64, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withMethod(http.MethodPut), withPath(routeGadget, gid))
}

func (s *WidgetsService) DeleteGadget(gid int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[*Widget](s.client, withMethod(http.MethodDelete), withPath(routeGadget, gid))
	return resp, err
}

func (s *WidgetsService) GetGadgetPart(gid any, part int64, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeGadgetPart, WidgetID{gid}, part))
}

func (s *WidgetsService) CloneGadget(gid int64, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withMethod(http.MethodPost), withPath(routeGadgetClone, gid), withPath(routeGadgetCopy, gid))
}

func (s *WidgetsService) CopyGadget(gid int64, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withMethod(http.MethodPost), withPath(routeGadgetCopy, gid))
}

func (s *WidgetsService) GetCog(cid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeCog, WidgetID{cid}))
}

func (s *WidgetsService) GetBolt(bid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeBolt, WidgetID{bid}))
}

func (s *WidgetsService) ListBoltThreads(bid any, options ...RequestOptionFunc) ([]*Widget, *Response, error) {
	return do[[]*Widget](s.client, withPath(routeBoltThreads, WidgetID{bid}))
}

func (s *WidgetsService) GetLatestBoltThread(bid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeBoltLatest, WidgetID{bid}))
}

func (s *WidgetsService) MoveGadget(gid, target int64, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withMethod(http.MethodPut), withPath(routeGadgetMove, gid, target))
}

func (s *WidgetsService) GetNut(nid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeNut, WidgetID{nid}))
}

func (s *WidgetsService) GetWasher(wid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeWasher, WidgetID{wid}))
}

func (s *WidgetsService) GetSprocket(sid any, options ...RequestOptionFunc) (*Widget, *Response, error) {
	return do[*Widget](s.client, withPath(routeSprocket, WidgetID{sid}))
}

func (s *WidgetsService) ListSprocketTeeth(sid int64, options ...RequestOptionFunc) ([]*Widget, *Response, error) {
	return do[[]*Widget](s.client, withPath(routeSprocketTeeth, sid))
}

func (s *WidgetsService) Count(n int) int { return n }

type Client struct {
	Widgets WidgetsServiceInterface
}
`

// identifiersTool is the stand-in tool package: handlers that hand the
// service an integer read from an input field in each way the trace
// follows (in place, in parentheses, converted into a local, through a
// helper's parameter, through a pair assignment and a declaration, from two
// fields, through a function that calls itself, a range, an element, a
// pointer, an operator, an assertion, a parse, a method of the input's
// type, a variadic parameter, a helper of another package, and a method
// value handed to a helper, put in a struct or kept in a local), integers
// read from no input field (a client-go struct, a constant, an untyped
// literal, an unnamed struct, the results of a call handed on whole, a
// count, a position), integers the trace stops at (a closure's parameter, a
// local whose address is taken, a package's variable, a function value's
// result, a bare return, the parameters of a function named other than by
// a call), method values it does not follow, and calls that hand it no
// integer at all.
const identifiersTool = `package widgets

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"
	"example.com/fixture/internal/tools/widgetdata"
)

// StringOrInt stands in for toolutil.StringOrInt, what an input is widened to.
type StringOrInt string

// Int64 reads the stand-in as an ID, the way toolutil.StringOrInt does.
func (s StringOrInt) Int64() int64 {
	n, _ := strconv.ParseInt(string(s), 10, 64)
	return n
}

type EventsInput struct {
	UserID int64 ~json:"user_id"~
}

func Events(ctx context.Context, c *gl.Client, input EventsInput) error {
	_, _, err := c.Widgets.ListWidgetEvents((input.UserID), gl.WithContext(ctx))
	return err
}

type DetailsInput struct{ WidgetID int64 }

func Details(c *gl.Client, input *DetailsInput) error {
	id := int(input.WidgetID)
	_, _, err := c.Widgets.GetWidgetDetails(id)
	return err
}

type OwnerInput struct{ WidgetID int64 }

func Owner(c *gl.Client, input OwnerInput) error {
	_, _, err := c.Widgets.GetWidgetOwner(input.WidgetID)
	return err
}

type GadgetInput struct{ GadgetID int64 }

func Gadget(c *gl.Client, input GadgetInput) error { return showGadget(input.GadgetID, c) }

func showGadget(gid int64, c *gl.Client) error {
	_, _, err := c.Widgets.GetGadget(gid)
	return err
}

type PartInput struct{ GadgetID, PartID int64 }

func Part(c *gl.Client, input PartInput) error {
	gid, part := input.GadgetID, input.PartID
	_, _, err := c.Widgets.GetGadgetPart(gid, part)
	return err
}

type CogInput struct{ CogID int64 }

func Cog(c *gl.Client, input CogInput) error {
	var cid = input.CogID
	var p, q = two()
	var spare int64 = 2
	_, _, _ = p, q, spare
	_, _, err := c.Widgets.GetCog(cid)
	return err
}

type BoltInput struct{ BoltID int64 }

func Bolt(c *gl.Client, input BoltInput) error {
	if _, _, err := c.Widgets.GetBolt(input.BoltID); err != nil {
		return err
	}
	if _, _, err := c.Widgets.ListBoltThreads(input.BoltID); err != nil {
		return err
	}
	_, _, err := c.Widgets.GetLatestBoltThread(input.BoltID)
	return err
}

type NutInput struct{ NutID int64 }

func Nut(c *gl.Client, input NutInput) error {
	_, _, err := c.Widgets.GetNut(input.NutID)
	return err
}

type WasherInput struct{ WasherID int64 }

func Washer(c *gl.Client, input WasherInput) error {
	_, _, err := c.Widgets.GetWasher(input.WasherID)
	return err
}

type SprocketInput struct{ SprocketID int64 }

func Sprocket(c *gl.Client, input SprocketInput) error {
	_, _, err := c.Widgets.GetSprocket(input.SprocketID)
	return err
}

type TwiceInput struct{ First, Second int64 }

func Twice(c *gl.Client, input TwiceInput, second bool) error {
	uid := input.First
	if second {
		uid = input.Second
	}
	var holder struct{ id int64 }
	holder.id = uid
	x, y := two()
	_, _ = x, y
	_, _, err := c.Widgets.ListWidgetEvents(uid)
	return err
}

type RecurseInput struct{ UserID int64 }

func Recurse(c *gl.Client, input RecurseInput) error { return recurse(c, input.UserID, 3) }

func recurse(c *gl.Client, uid int64, depth int) error {
	if depth > 0 {
		return recurse(c, uid, depth-1)
	}
	_, _, err := c.Widgets.ListWidgetEvents(uid)
	return err
}

func FromAnswer(c *gl.Client) error {
	w, _, err := c.Widgets.GetWidgetDetails("first")
	if err != nil {
		return err
	}
	if _, _, err = c.Widgets.ListWidgetEvents(w.ID); err != nil {
		return err
	}
	var first, second = two()
	_ = first
	if _, _, err = c.Widgets.ListWidgetEvents(second); err != nil {
		return err
	}
	_, third := twoAgain()
	if _, _, err = c.Widgets.ListWidgetEvents(third); err != nil {
		return err
	}
	_, _, err = c.Widgets.ListWidgetEvents(idOf(w))
	return err
}

func idOf(w *gl.Widget) int64 { return w.ID }

func twoAgain() (int64, int64) { return two() }

const fallbackOwner = 4

func Constants(c *gl.Client) int {
	c.Widgets.ListWidgetEvents(gl.DefaultWidget)
	c.Widgets.ListWidgetEvents(fallbackOwner)
	c.Widgets.ListWidgetEvents(7)
	anonymous := struct{ ID int64 }{ID: 1}
	c.Widgets.ListWidgetEvents(anonymous.ID)
	positional := struct{ ID int64 }{2}
	c.Widgets.ListWidgetEvents(positional.ID)
	c.Widgets.ListWidgetEvents(strconv.Itoa(1))
	return c.Widgets.Count(2)
}

type handler struct{ c *gl.Client }

func (h handler) Pair() error { return h.pairShow(two()) }

func (h handler) pairShow(a, b int64) error {
	_, _, err := h.c.Widgets.ListWidgetEvents(b)
	return err
}

func two() (int64, int64) { return 1, 2 }

type ManyInput struct {
	UserIDs []int64
	ByOwner map[int64]string
	Count   int64
	Name    string
	Offset  int64
	Raw     any
	Slots   [2]int64
}

func Many(c *gl.Client, input ManyInput) {
	for _, uid := range input.UserIDs {
		c.Widgets.ListWidgetEvents(uid)
	}
	for position := range input.UserIDs {
		c.Widgets.ListWidgetEvents(position)
	}
	for owner := range input.ByOwner {
		c.Widgets.ListWidgetEvents(owner)
	}
	for n := range input.Count {
		c.Widgets.ListWidgetEvents(n)
	}
	for i := range input.Name {
		c.Widgets.ListWidgetEvents(i)
	}
	for slot := range input.Slots {
		c.Widgets.ListWidgetEvents(slot)
	}
	for slot := range &input.Slots {
		c.Widgets.ListWidgetEvents(slot)
	}
	c.Widgets.ListWidgetEvents(input.UserIDs[0])
	c.Widgets.ListWidgetEvents(input.UserIDs[1:][0])
	c.Widgets.ListWidgetEvents(-input.Offset)
	c.Widgets.ListWidgetEvents(input.Raw.(int64))
	c.Widgets.ListWidgetEvents(input.Offset + 1)
	c.Widgets.ListWidgetEvents(len(input.UserIDs))
	c.Widgets.ListWidgetEvents(max(input.Count, 1))
	if raw, isInt := input.Raw.(int64); isInt {
		c.Widgets.ListWidgetEvents(raw)
	}
}

type PtrInput struct{ UserID *int64 }

func Ptr(c *gl.Client, input PtrInput) {
	c.Widgets.ListWidgetEvents(*input.UserID)
	c.Widgets.ListWidgetEvents(ownerOf(*input.UserID))
}

// ownerOf hands its parameter back; the return of the closure in it is the
// closure's own.
func ownerOf(uid int64) int64 {
	fallback := func() int64 { return defaultOwner }
	_ = fallback
	return uid
}

type ParsedInput struct {
	User  string
	Owner StringOrInt
}

func Parsed(c *gl.Client, input ParsedInput) error {
	uid, err := strconv.ParseInt(input.User, 10, 64)
	if err != nil {
		return err
	}
	c.Widgets.ListWidgetEvents(uid)
	c.Widgets.ListWidgetEvents(input.Owner.Int64())
	c.Widgets.ListWidgetEvents(StringOrInt.Int64(input.Owner))
	return nil
}

type PairInput struct{ First, Second int64 }

func Pair(c *gl.Client, input PairInput) {
	_, second := ownersOf(input)
	c.Widgets.ListWidgetEvents(second)
}

func ownersOf(input PairInput) (int64, int64) { return input.First, input.Second }

type OwnerNameInput struct{ Owner StringOrInt }

func OwnerNamed(c *gl.Client, input OwnerNameInput) {
	c.Widgets.ListWidgetEvents(input.Owner.Int64())
}

func Scanned(c *gl.Client, input ParsedInput) error {
	var uid, other int64
	if _, err := fmt.Sscan(input.User, &uid, &other); err != nil {
		return err
	}
	c.Widgets.ListWidgetEvents(uid)
	c.Widgets.ListWidgetEvents(-other)
	return nil
}

type ClosureInput struct{ UserID int64 }

func Closure(c *gl.Client, input ClosureInput) {
	show := func(uid int64) { c.Widgets.ListWidgetEvents(uid) }
	show(input.UserID)
}

var defaultOwner int64 = 3

func Default(c *gl.Client, h handler) {
	c.Widgets.ListWidgetEvents(defaultOwner)
	c.Widgets.ListWidgetEvents(widgetdata.DefaultOwner)
	c.Widgets.ListWidgetEvents(func() int64 { return 5 }())
	pick := two
	first, _ := pick()
	c.Widgets.ListWidgetEvents(first)
	c.Widgets.ListWidgetEvents(int64(sort.Search(10, isLarge) + sort.Search(10, func(i int) bool { return i > 3 }) + sort.Search(10, h.less)))
	c.Widgets.ListWidgetEvents(nakedOwner(ClosureInput{UserID: 9}))
}

func isLarge(i int) bool { return i > 5 }

func (h handler) less(i int) bool { return i > 1 }

func nakedOwner(input ClosureInput) (uid int64) {
	uid = input.UserID
	return
}

type SharedInput struct{ UserID int64 }

func Shared(c *gl.Client, input SharedInput) error { return widgetdata.Events(c, input.UserID) }

type ShownInput struct{ UserID int64 }

func Shown(c *gl.Client, input ShownInput) func(*gl.Client, int64) {
	showOwner(c, input.UserID)
	return showOwner
}

func showOwner(c *gl.Client, uid int64) { c.Widgets.ListWidgetEvents(uid) }

func orphan(c *gl.Client, uid int64) { c.Widgets.ListWidgetEvents(uid) }

func (h handler) owner(uid int64) { h.c.Widgets.ListWidgetEvents(uid) }

func Owned(h handler, input ShownInput) { handler.owner(h, input.UserID) }

type ownerID int64

func (o ownerID) events(c *gl.Client) { c.Widgets.ListWidgetEvents(o) }

func (o ownerID) unused(c *gl.Client) { c.Widgets.ListWidgetEvents(o) }

type OwnerIDInput struct{ UserID int64 }

func OwnerEvents(c *gl.Client, input OwnerIDInput) { ownerID(input.UserID).events(c) }

type EachInput struct{ First, Second int64 }

func Each(c *gl.Client, input EachInput) {
	everyOwner(c, input.First, input.Second)
	everyOwner(c, input.First)
}

func everyOwner(c *gl.Client, first int64, rest ...int64) {
	c.Widgets.ListWidgetEvents(first)
	for _, uid := range rest {
		c.Widgets.ListWidgetEvents(uid)
	}
}

// getFunc and listFunc are the signatures of the stand-in's reads and lists.
type (
	getFunc  = func(any, ...gl.RequestOptionFunc) (*gl.Widget, *gl.Response, error)
	listFunc = func(any, ...gl.RequestOptionFunc) ([]*gl.Widget, *gl.Response, error)
)

type ValueInput struct{ UserID int64 }

func Value(c *gl.Client, input ValueInput) error { return viaValue(input.UserID, c.Widgets.ListWidgetEvents) }

type BoltValueInput struct{ UserID int64 }

func BoltValue(c *gl.Client, input BoltValueInput) error {
	return viaValue(input.UserID, (c.Widgets.ListBoltThreads))
}

func viaValue(uid int64, list listFunc) error {
	if list == nil {
		return nil
	}
	_, _, err := list(uid)
	return err
}

type eventsRead[O any] struct {
	id   int64
	opts O
	list listFunc
}

type FieldInput struct{ UserID int64 }

func Field(c *gl.Client, input FieldInput) error {
	return readEvents(eventsRead[string]{id: input.UserID, list: c.Widgets.ListWidgetEvents})
}

func readEvents[O any](read eventsRead[O]) error {
	_, _, err := read.list(read.id)
	return err
}

type (
	assignedRead   struct{ list listFunc }
	positionalRead struct{ list listFunc }
	keyedRead      struct{ list listFunc }
)

type LocalInput struct{ Assigned, Positional, Keyed, Later int64 }

func Local(c *gl.Client, input LocalInput) {
	list := c.Widgets.ListWidgetEvents
	var again = list
	if again == nil {
		return
	}
	again = list
	var assigned assignedRead
	assigned.list = again
	positional := &positionalRead{again}
	keyed := keyedRead{list: again}
	var later listFunc
	later = again
	assigned.list(input.Assigned)
	positional.list(input.Positional)
	keyed.list(input.Keyed)
	later(input.Later)
}

func Escaped(c *gl.Client) { escape(c.Widgets.GetWidgetDetails) }

func escape(get getFunc) []getFunc { return []getFunc{get} }

func lister(c *gl.Client) listFunc { return c.Widgets.ListWidgetEvents }

type ListedInput struct{ First, Second, Third int64 }

func Listed(c *gl.Client, input ListedInput) {
	lister(c)(input.First)
	kept := lister(c)
	kept(input.Second)
	func() listFunc { return c.Widgets.ListBoltThreads }()(input.Third)
}

func listerAndErr(c *gl.Client) (listFunc, error) { return c.Widgets.ListBoltThreads, nil }

func idleLister(c *gl.Client) getFunc { return c.Widgets.GetGadget }

func escapedGetter(c *gl.Client) getFunc { return c.Widgets.GetLatestBoltThread }

func Escapes() { _ = escapedGetter }

func cogPicker() func(gl.WidgetsServiceInterface) getFunc {
	return func(s gl.WidgetsServiceInterface) getFunc { return s.GetCog }
}

// pickFunc picks a method off the service, the way mergerequests'
// subscriptionCall does, so the service is reached only when it is called.
type pickFunc func(gl.WidgetsServiceInterface) listFunc

type PickedInput struct{ UserID int64 }

func Picked(c *gl.Client, input PickedInput) error {
	return picked(c, input.UserID, func(s gl.WidgetsServiceInterface) listFunc { return s.ListWidgetEvents })
}

func picked(c *gl.Client, uid int64, pick pickFunc) error {
	fmt.Println(pick)
	relay := pick
	if _, _, err := relay(c.Widgets)(uid + 1); err != nil {
		return err
	}
	_, _, err := pick(c.Widgets)(uid)
	return err
}

func PickedValue(c *gl.Client) {
	pickedValue(func(s gl.WidgetsServiceInterface) listFunc { return s.ListBoltThreads }, c)
}

func pickedValue(pick pickFunc, c *gl.Client) listFunc { return pick(c.Widgets) }

func spread(first getFunc, rest ...getFunc) int {
	first(7)
	return len(rest)
}

var localHook getFunc

func Printed(c *gl.Client, name string) {
	localHook = c.Widgets.GetBolt
	_ = c.Widgets.GetWasher
	fmt.Println(c.Widgets.GetCog, c.Widgets.GetWidgetOwner)
	spread(c.Widgets.GetWasher)
	spread(nil, c.Widgets.GetNut)
	gl.WidgetsServiceInterface.GetBolt(c.Widgets, 5)
	_ = map[string]getFunc{"gadget": c.Widgets.GetGadget, name: c.Widgets.GetSprocket}
	hooks := make([]getFunc, 1)
	hooks[0] = c.Widgets.GetLatestBoltThread
	widgetdata.Hook = c.Widgets.GetWidgetDetails
}
`

// identifiersData is the stand-in of another tool package: a helper a
// handler of the first hands an input field, and the variables of its own
// the first reads and writes.
const identifiersData = `package widgetdata

import gl "example.com/fixture/gitlab.com/gitlab-org/api/client-go/v3"

var DefaultOwner int64 = 2

var Hook func(any, ...gl.RequestOptionFunc) (*gl.Widget, *gl.Response, error)

func Events(c *gl.Client, uid int64) error {
	_, _, err := c.Widgets.ListWidgetEvents(uid)
	return err
}
`

// identifiersRecord is the stand-in record: the events route under an
// optional group that keeps its length and the details route under one
// that changes it, so a placeholder is named off the spelling the request
// meets; the gadget's routes, one describing nothing of its id, its
// siblings describing it two ways at one depth and one way deeper; and
// routes described by their type alone, by another route of their
// namespace, by a type that says nothing of an ID, and by nothing.
func identifiersRecord() []apilive.Route {
	describe := func(kind, desc string) map[string]apilive.Param {
		return map[string]apilive.Param{"id": {Required: true, Type: kind, Desc: desc}}
	}
	route := func(method, path string, params map[string]apilive.Param) apilive.Route {
		return apilive.Route{Method: method, Path: apilive.EndpointPrefix + path, Params: params}
	}
	return []apilive.Route{
		route("GET", "/widget(s)/:id/events", describe("String", "ID or username of the widget's owner.")),
		route("GET", "/widgets/:id/(-/)details", describe("[String, Integer]", "ID of the widget.")),
		route("GET", "/widgets/:id/owner", describe("String", "The ID or URL-encoded path of the widget")),
		route("GET", "/gadgets/:id", map[string]apilive.Param{"id": {}}),
		route("DELETE", "/gadgets/:id", describe("Integer", "ID of the gadget.")),
		route("PUT", "/gadgets/:id", describe("String", "Name of the gadget.")),
		route("POST", "/gadgets/:id/ping", describe("Integer", "ID of the gadget.")),
		route("GET", "/gadgets/:id/parts", describe("[String, Integer]", "ID of the gadget.")),
		route("GET", "/gadgets/:id/parts/:part_id", map[string]apilive.Param{"part_id": {Type: "Integer", Desc: "ID of the part."}}),
		route("POST", "/gadgets/:id/clone", describe("String", "Path of the gadget.")),
		route("POST", "/gadgets/:id/copy", describe("String", "Path of the gadget.")),
		route("GET", "/cogs/:id", describe("String", "")),
		route("GET", "/bolts/:id", describe("Integer", "")),
		route("GET", "/bolts/:id/threads", nil),
		route("GET", "/bolts/:id/threads/:thread_id", nil),
		route("PUT", "/gadgets/:id/move/:target_id", map[string]apilive.Param{"target_id": {Type: "String", Desc: "Path of the target group."}}),
		route("GET", "/nuts/:id", describe("Hash", "")),
		route("GET", "/washers", describe("String", "Slug of the washer to start from.")),
		route("GET", "/washers/:id", nil),
	}
}

// identifiersFixture writes the stand-in tree with the tool package put
// through tool and the record's routes, and loads it.
func identifiersFixture(t *testing.T, tool *strings.Replacer, routes []apilive.Route) (string, identifiersInputs) {
	t.Helper()
	root := t.TempDir()
	source := identifiersTool
	if tool != nil {
		source = tool.Replace(source)
	}
	writeFiles(t, root, map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.27\n",
		"gitlab.com/gitlab-org/api/client-go/v3/sdk.go": identifiersSDK,
		"internal/tools/widgets/widgets.go":             strings.ReplaceAll(source, "~", "`"),
		"internal/tools/widgetdata/widgetdata.go":       identifiersData,
	})
	if routes != nil {
		if err := apilive.Write(filepath.Join(root, filepath.FromSlash(apilive.DefaultDir)), apilive.Document{SchemaVersion: apilive.SchemaVersion, Routes: routes}); err != nil {
			t.Fatalf("write the record: %v", err)
		}
	}
	pkgs, err := shared.LoadToolPackages(root)
	if err != nil {
		t.Fatalf("load the fixture: %v", err)
	}
	return root, identifiersInputs{pkgs: pkgs, usage: shared.CollectServiceUsage(pkgs)}
}

// identifiersInputs is what the section is built from, besides the root.
type identifiersInputs struct {
	pkgs  []*packages.Package
	usage map[string]*shared.ServiceUsage
}

// analyzeIdentifiersFixture builds the section over the stand-in tree with
// the tool package put through tool, the record's routes (none written
// when nil) and declared.
func analyzeIdentifiersFixture(t *testing.T, tool *strings.Replacer, routes []apilive.Route, declared []identifierDeclaration) (identifiersReport, []string) {
	t.Helper()
	root, inputs := identifiersFixture(t, tool, routes)
	return analyzeIdentifiers(root, inputs.pkgs, inputs.usage, declared)
}

// standInIdentifierDeclarations answer three of the stand-in's inputs, one
// of each status a declaration answers and of each category, and declare a
// fourth whose placeholder takes an ID alone.
var standInIdentifierDeclarations = []identifierDeclaration{
	{Input: "widgets.EventsInput.UserID", Category: identifierTracked, Issue: 7, Reason: "the owner's events take a username"},
	{Input: "widgets.CogInput.CogID", Category: identifierAcceptsIDOnly, Evidence: "find_cog! reads Cog.find(params[:id])", Reason: "the type says String, the finder takes an ID"},
	{Input: "widgets.NutInput.NutID", Category: identifierTracked, Issue: 8, Reason: "the nut's id is a hash nothing describes"},
	{Input: "widgets.DetailsInput.WidgetID", Category: identifierTracked, Issue: 9, Reason: "the details take an ID alone"},
}

// argumentLine spells an argument the way the stand-in's expectations read.
func argumentLine(argument identifierArgument) string {
	return strings.Join([]string{argument.Site, argument.Method, argument.Param, argument.Argument, argument.Type, argument.Input, argument.Status}, " ")
}

// standInArguments is every integer the stand-in tree hands a client-go
// parameter of type any, as argumentLine spells it, with the stand-in
// declarations applied.
var standInArguments = []string{
	"widgetdata.Events Widgets.ListWidgetEvents uid uid int64 widgets.SharedInput.UserID narrowed",
	"widgets.Bolt Widgets.GetBolt bid input.BoltID int64 widgets.BoltInput.BoltID id_only",
	"widgets.Bolt Widgets.GetLatestBoltThread bid input.BoltID int64 widgets.BoltInput.BoltID id_only",
	"widgets.Bolt Widgets.ListBoltThreads bid input.BoltID int64 widgets.BoltInput.BoltID id_only",
	"widgets.Closure Widgets.ListWidgetEvents uid uid int64  untraced",
	"widgets.Cog Widgets.GetCog cid cid int64 widgets.CogInput.CogID declared",
	"widgets.Constants Widgets.ListWidgetEvents uid 7 int  not_from_input",
	"widgets.Constants Widgets.ListWidgetEvents uid anonymous.ID int64  not_from_input",
	"widgets.Constants Widgets.ListWidgetEvents uid fallbackOwner int  not_from_input",
	"widgets.Constants Widgets.ListWidgetEvents uid gl.DefaultWidget int  not_from_input",
	"widgets.Constants Widgets.ListWidgetEvents uid positional.ID int64  not_from_input",
	"widgets.Default Widgets.ListWidgetEvents uid (func() int64 literal)() int64  untraced",
	"widgets.Default Widgets.ListWidgetEvents uid defaultOwner int64  untraced",
	"widgets.Default Widgets.ListWidgetEvents uid first int64  untraced",
	"widgets.Default Widgets.ListWidgetEvents uid int64(sort.Search(10, isLarge) + sort.Search(10, (func(i int) bool literal)) + sort.Search(10, h.less)) int64  untraced",
	"widgets.Default Widgets.ListWidgetEvents uid nakedOwner(ClosureInput{…}) int64  untraced",
	"widgets.Default Widgets.ListWidgetEvents uid widgetdata.DefaultOwner int64  untraced",
	"widgets.Details Widgets.GetWidgetDetails wid id int widgets.DetailsInput.WidgetID id_only",
	"widgets.Events Widgets.ListWidgetEvents uid (input.UserID) int64 widgets.EventsInput.UserID declared",
	"widgets.FromAnswer Widgets.ListWidgetEvents uid idOf(w) int64  not_from_input",
	"widgets.FromAnswer Widgets.ListWidgetEvents uid second int64  not_from_input",
	"widgets.FromAnswer Widgets.ListWidgetEvents uid third int64  not_from_input",
	"widgets.FromAnswer Widgets.ListWidgetEvents uid w.ID int64  not_from_input",
	"widgets.Listed Widgets.ListBoltThreads bid input.Third int64 widgets.ListedInput.Third id_only",
	"widgets.Listed Widgets.ListWidgetEvents uid input.First int64 widgets.ListedInput.First narrowed",
	"widgets.Listed Widgets.ListWidgetEvents uid input.Second int64 widgets.ListedInput.Second narrowed",
	"widgets.Local Widgets.ListWidgetEvents uid input.Assigned int64 widgets.LocalInput.Assigned narrowed",
	"widgets.Local Widgets.ListWidgetEvents uid input.Keyed int64 widgets.LocalInput.Keyed narrowed",
	"widgets.Local Widgets.ListWidgetEvents uid input.Later int64 widgets.LocalInput.Later narrowed",
	"widgets.Local Widgets.ListWidgetEvents uid input.Positional int64 widgets.LocalInput.Positional narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid i int  not_from_input",
	"widgets.Many Widgets.ListWidgetEvents uid len(input.UserIDs) int  not_from_input",
	"widgets.Many Widgets.ListWidgetEvents uid position int  not_from_input",
	"widgets.Many Widgets.ListWidgetEvents uid slot int  not_from_input",
	"widgets.Many Widgets.ListWidgetEvents uid owner int64 widgets.ManyInput.ByOwner narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid max(input.Count, 1) int64 widgets.ManyInput.Count narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid n int64 widgets.ManyInput.Count narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid -input.Offset int64 widgets.ManyInput.Offset narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid input.Offset + 1 int64 widgets.ManyInput.Offset narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid input.Raw.(int64) int64 widgets.ManyInput.Raw narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid raw int64 widgets.ManyInput.Raw narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid input.UserIDs[0] int64 widgets.ManyInput.UserIDs narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid input.UserIDs[1:][0] int64 widgets.ManyInput.UserIDs narrowed",
	"widgets.Many Widgets.ListWidgetEvents uid uid int64 widgets.ManyInput.UserIDs narrowed",
	"widgets.Nut Widgets.GetNut nid input.NutID int64 widgets.NutInput.NutID declared",
	"widgets.OwnerNamed Widgets.ListWidgetEvents uid input.Owner.Int64() int64 widgets.OwnerNameInput.Owner narrowed",
	"widgets.Pair Widgets.ListWidgetEvents uid second int64 widgets.PairInput.Second narrowed",
	"widgets.Parsed Widgets.ListWidgetEvents uid StringOrInt.Int64(input.Owner) int64 widgets.ParsedInput.Owner narrowed",
	"widgets.Parsed Widgets.ListWidgetEvents uid input.Owner.Int64() int64 widgets.ParsedInput.Owner narrowed",
	"widgets.Parsed Widgets.ListWidgetEvents uid uid int64 widgets.ParsedInput.User narrowed",
	"widgets.Part Widgets.GetGadgetPart gid gid int64 widgets.PartInput.GadgetID id_only",
	"widgets.Ptr Widgets.ListWidgetEvents uid *input.UserID int64 widgets.PtrInput.UserID narrowed",
	"widgets.Ptr Widgets.ListWidgetEvents uid ownerOf(*input.UserID) int64 widgets.PtrInput.UserID narrowed",
	"widgets.Scanned Widgets.ListWidgetEvents uid -other int64  untraced",
	"widgets.Scanned Widgets.ListWidgetEvents uid uid int64  untraced",
	"widgets.Sprocket Widgets.GetSprocket sid input.SprocketID int64 widgets.SprocketInput.SprocketID unplaced",
	"widgets.Twice Widgets.ListWidgetEvents uid uid int64 widgets.TwiceInput.First narrowed",
	"widgets.Twice Widgets.ListWidgetEvents uid uid int64 widgets.TwiceInput.Second narrowed",
	"widgets.Washer Widgets.GetWasher wid input.WasherID int64 widgets.WasherInput.WasherID undescribed",
	"widgets.everyOwner Widgets.ListWidgetEvents uid first int64 widgets.EachInput.First narrowed",
	"widgets.everyOwner Widgets.ListWidgetEvents uid uid int64 widgets.EachInput.Second narrowed",
	"widgets.handler.owner Widgets.ListWidgetEvents uid uid int64  untraced",
	"widgets.handler.pairShow Widgets.ListWidgetEvents uid b int64  not_from_input",
	"widgets.orphan Widgets.ListWidgetEvents uid uid int64  untraced",
	"widgets.ownerID.events Widgets.ListWidgetEvents uid o widgets.ownerID widgets.OwnerIDInput.UserID narrowed",
	"widgets.ownerID.unused Widgets.ListWidgetEvents uid o widgets.ownerID  untraced",
	"widgets.picked Widgets.ListWidgetEvents uid uid int64 widgets.PickedInput.UserID narrowed",
	"widgets.picked Widgets.ListWidgetEvents uid uid + 1 int64 widgets.PickedInput.UserID narrowed",
	"widgets.readEvents Widgets.ListWidgetEvents uid read.id int64 widgets.eventsRead.id narrowed",
	"widgets.recurse Widgets.ListWidgetEvents uid uid int64 widgets.RecurseInput.UserID narrowed",
	"widgets.showGadget Widgets.GetGadget gid gid int64 widgets.GadgetInput.GadgetID narrowed",
	"widgets.showOwner Widgets.ListWidgetEvents uid uid int64  untraced",
	"widgets.showOwner Widgets.ListWidgetEvents uid uid int64 widgets.ShownInput.UserID narrowed",
	"widgets.spread Widgets.GetWasher wid 7 int  not_from_input",
	"widgets.viaValue Widgets.ListBoltThreads bid uid int64 widgets.BoltValueInput.UserID id_only",
	"widgets.viaValue Widgets.ListWidgetEvents uid uid int64 widgets.ValueInput.UserID narrowed",
}

// standInStoppedAt is where the trace stops for each untraced argument of
// the stand-in tree, by site and argument.
var standInStoppedAt = map[string]string{
	"widgets.Closure uid":                      "widgets.Closure: uid",
	"widgets.Default (func() int64 literal)()": "widgets.Default: (func() int64 literal)()",
	"widgets.Default defaultOwner":             "widgets.Default: defaultOwner",
	"widgets.Default first":                    "widgets.Default: pick()",
	"widgets.Default int64(sort.Search(10, isLarge) + sort.Search(10, (func(i int) bool literal)) + sort.Search(10, h.less))": "widgets.Default: (func(i int) bool literal); widgets.Default: h.less; widgets.Default: isLarge",
	"widgets.Default nakedOwner(ClosureInput{…})": "widgets.nakedOwner: return",
	"widgets.Default widgetdata.DefaultOwner":     "widgets.Default: widgetdata.DefaultOwner",
	"widgets.Scanned -other":                      "widgets.Scanned: &other",
	"widgets.Scanned uid":                         "widgets.Scanned: &uid",
	"widgets.handler.owner uid":                   "widgets.handler.owner: uid",
	"widgets.orphan uid":                          "widgets.orphan: uid",
	"widgets.ownerID.unused o":                    "widgets.ownerID.unused: o",
	"widgets.showOwner uid":                       "widgets.showOwner: uid",
}

// standInUnfollowed is every method value of the stand-in tree the walk
// does not follow.
var standInUnfollowed = []unfollowedValue{
	{Site: "widgets.Printed", Method: "Widgets.GetBolt", Value: "c.Widgets.GetBolt"},
	{Site: "widgets.Printed", Method: "Widgets.GetBolt", Value: "gl.WidgetsServiceInterface.GetBolt"},
	{Site: "widgets.Printed", Method: "Widgets.GetCog", Value: "c.Widgets.GetCog"},
	{Site: "widgets.Printed", Method: "Widgets.GetGadget", Value: "c.Widgets.GetGadget"},
	{Site: "widgets.Printed", Method: "Widgets.GetLatestBoltThread", Value: "c.Widgets.GetLatestBoltThread"},
	{Site: "widgets.Printed", Method: "Widgets.GetNut", Value: "c.Widgets.GetNut"},
	{Site: "widgets.Printed", Method: "Widgets.GetSprocket", Value: "c.Widgets.GetSprocket"},
	{Site: "widgets.Printed", Method: "Widgets.GetWasher", Value: "c.Widgets.GetWasher"},
	{Site: "widgets.Printed", Method: "Widgets.GetWidgetDetails", Value: "c.Widgets.GetWidgetDetails"},
	{Site: "widgets.cogPicker", Method: "Widgets.GetCog", Value: "s.GetCog"},
	{Site: "widgets.escape", Method: "Widgets.GetWidgetDetails", Value: "get"},
	{Site: "widgets.escapedGetter", Method: "Widgets.GetLatestBoltThread", Value: "c.Widgets.GetLatestBoltThread"},
	{Site: "widgets.idleLister", Method: "Widgets.GetGadget", Value: "c.Widgets.GetGadget"},
	{Site: "widgets.listerAndErr", Method: "Widgets.ListBoltThreads", Value: "c.Widgets.ListBoltThreads"},
	{Site: "widgets.picked", Method: "Widgets.ListWidgetEvents", Value: "pick"},
	{Site: "widgets.pickedValue", Method: "Widgets.ListBoltThreads", Value: "pick(c.Widgets)"},
}

// TestAnalyzeIdentifiers_AStandInTree_HoldsEachIntegerToWhatItsPlaceholderTakes
// verifies the section on a tree whose universe is known. Every integer a
// handler hands a client-go parameter of type any, by name or through a
// method value handed to a helper, put in a struct, generic or not, by key
// or by position, or kept in a local, is found, once per input field it is
// read from: in place, in parentheses, converted, through locals, pair
// assignments and declarations, a range's key or value, an element, a
// slice, a pointer, an operator, an assertion, a parse, a method of the
// input's own type or a call of one through its type, a helper's parameter
// in the package or in another, a variadic one, the receiver of a method,
// and a function that calls itself; one read from a client-go struct, a
// function's result, a qualified or a package constant, an untyped literal,
// an unnamed struct, a count, a position, or a call whose results are
// handed on whole is read from no input; and one the trace stops at (a
// closure's parameter, a local whose address is taken, a package's
// variable, a function value's or a function literal's result, a function
// or a method handed on as a value, a bare return, the parameters and the
// receiver of a function named other than by a call or by none) is
// untraced, with where it stopped. A method value handed on through one
// call carries the arguments of that call alone. A string, a parameter of
// another type and a method that is no service's are none. A record
// placeholder stands for a literal the request carries, as the route index
// has it. Each is held to what the record says the placeholder takes: the
// description first, the type when there is none, then the route of the
// namespace that shares the most segments and, among as many, takes the
// most; a placeholder nothing describes, a route the record does not hold
// and an untraced integer are findings, and so is a placeholder that takes
// a name or a path unless a declaration answers its input. Every method
// value the walk does not follow (returned, handed to a function it does
// not read or to a variadic parameter, put in a slice or a map, assigned to
// an element, the blank identifier or a package's variable of either
// package, or taken through the interface type) is listed.
func TestAnalyzeIdentifiers_AStandInTree_HoldsEachIntegerToWhatItsPlaceholderTakes(t *testing.T) {
	report, stale := analyzeIdentifiersFixture(t, nil, identifiersRecord(), standInIdentifierDeclarations)
	got := make([]string, 0, len(report.Arguments))
	placed := map[string]placeholder{}
	stoppedAt := map[string]string{}
	for _, argument := range report.Arguments {
		got = append(got, argumentLine(argument))
		if len(argument.Placeholders) == 1 {
			placed[argument.Site+" "+argument.Method] = argument.Placeholders[0]
		}
		if argument.StoppedAt != "" {
			stoppedAt[argument.Site+" "+argument.Argument] = argument.StoppedAt
		}
	}
	if !slices.Equal(got, standInArguments) {
		t.Errorf("arguments =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(standInArguments, "\n"))
	}
	if !maps.Equal(stoppedAt, standInStoppedAt) {
		t.Errorf("stopped at = %q, want %q", stoppedAt, standInStoppedAt)
	}
	if !slices.Equal(report.Unfollowed, standInUnfollowed) {
		t.Errorf("unfollowed =\n%+v\nwant\n%+v", report.Unfollowed, standInUnfollowed)
	}
	wantPlaced := map[string]placeholder{
		"widgets.Events Widgets.ListWidgetEvents":     {Route: "GET /widget(s)/:id/events", Name: "id", Type: "String", Desc: "ID or username of the widget's owner.", Takes: takesName},
		"widgets.Details Widgets.GetWidgetDetails":    {Route: "GET /widgets/:id/(-/)details", Name: "id", Type: "[String, Integer]", Desc: "ID of the widget.", Takes: takesID},
		"widgets.showGadget Widgets.GetGadget":        {Route: "GET /gadgets/:id", Name: "id", Type: "String", Desc: "Name of the gadget.", DescribedBy: "PUT /gadgets/:id", Takes: takesName},
		"widgets.Part Widgets.GetGadgetPart":          {Route: "GET /gadgets/:id/parts/:part_id", Name: "id", Type: "[String, Integer]", Desc: "ID of the gadget.", DescribedBy: "GET /gadgets/:id/parts", Takes: takesID},
		"widgets.Cog Widgets.GetCog":                  {Route: "GET /cogs/:id", Name: "id", Type: "String", Takes: takesName},
		"widgets.Bolt Widgets.ListBoltThreads":        {Route: "GET /bolts/:id/threads", Name: "id", Type: "Integer", DescribedBy: "GET /bolts/:id", Takes: takesID},
		"widgets.Nut Widgets.GetNut":                  {Route: "GET /nuts/:id", Name: "id", Type: "Hash", Takes: takesUndescribed},
		"widgets.Washer Widgets.GetWasher":            {Route: "GET /washers/:id", Name: "id", Takes: takesUndescribed},
		"widgets.FromAnswer Widgets.ListWidgetEvents": {Route: "GET /widget(s)/:id/events", Name: "id", Type: "String", Desc: "ID or username of the widget's owner.", Takes: takesName},
	}
	for key, want := range wantPlaced {
		t.Run(key, func(t *testing.T) {
			if placed[key] != want {
				t.Errorf("placeholder = %+v, want %+v", placed[key], want)
			}
		})
	}
	for _, argument := range report.Arguments {
		if argument.Input == "widgets.EventsInput.UserID" && (argument.Category != identifierTracked || argument.Issue != 7 || argument.Reason == "") {
			t.Errorf("the declared events argument carries %q, %d and %q, want the declaration's category, issue and reason", argument.Category, argument.Issue, argument.Reason)
		}
	}
	wantStale := []string{"identifier declaration widgets.DetailsInput.WidgetID answers no integer handed where the record does not say an ID alone is taken"}
	if !slices.Equal(stale, wantStale) {
		t.Errorf("stale = %q, want %q", stale, wantStale)
	}
	wantSDK := []sdkNarrowing{
		{Method: "Widgets.CloneGadget", Param: "gid", Type: "int64", Placeholders: []placeholder{
			{Route: "POST /gadgets/:id/clone", Name: "id", Type: "String", Desc: "Path of the gadget.", Takes: takesName},
			{Route: "POST /gadgets/:id/copy", Name: "id", Type: "String", Desc: "Path of the gadget.", Takes: takesName},
		}},
		{Method: "Widgets.CopyGadget", Param: "gid", Type: "int64", Placeholders: []placeholder{
			{Route: "POST /gadgets/:id/copy", Name: "id", Type: "String", Desc: "Path of the gadget.", Takes: takesName},
		}},
		{Method: "Widgets.GetWidgetOwner", Param: "wid", Type: "int64", Called: true, Placeholders: []placeholder{
			{Route: "GET /widgets/:id/owner", Name: "id", Type: "String", Desc: "The ID or URL-encoded path of the widget", Takes: takesName},
		}},
		{Method: "Widgets.MoveGadget", Param: "gid", Type: "int64", Placeholders: []placeholder{
			{Route: "PUT /gadgets/:id/move/:target_id", Name: "id", Type: "String", Desc: "Name of the gadget.", DescribedBy: "PUT /gadgets/:id", Takes: takesName},
		}},
		{Method: "Widgets.MoveGadget", Param: "target", Type: "int64", Placeholders: []placeholder{
			{Route: "PUT /gadgets/:id/move/:target_id", Name: "target_id", Type: "String", Desc: "Path of the target group.", Takes: takesName},
		}},
		{Method: "Widgets.UpdateGadget", Param: "gid", Type: "int64", Placeholders: []placeholder{
			{Route: "PUT /gadgets/:id", Name: "id", Type: "String", Desc: "Name of the gadget.", Takes: takesName},
		}},
	}
	if !reflect.DeepEqual(report.SDKNarrowed, wantSDK) {
		t.Errorf("sdk_narrowed =\n%+v\nwant\n%+v", report.SDKNarrowed, wantSDK)
	}
}

// TestAnalyzeIdentifiers_TheInputOrTheRecordMoves_TheNarrowingComesAndGoes
// verifies the rule from both of its ends: an integer user ID handed where
// the record says the owner's ID or username is taken is a finding; the
// input widened to a StringOrInt takes it away, and so does a record whose
// description says an ID alone. A declaration that answered it then
// answers nothing, and is stale.
func TestAnalyzeIdentifiers_TheInputOrTheRecordMoves_TheNarrowingComesAndGoes(t *testing.T) {
	widened := strings.NewReplacer("UserID int64 ~json:\"user_id\"~", "UserID StringOrInt ~json:\"user_id\"~")
	idAlone := identifiersRecord()
	idAlone[0].Params = map[string]apilive.Param{"id": {Type: "String", Desc: "ID of the user."}}
	tracked := []identifierDeclaration{{Input: "widgets.EventsInput.UserID", Category: identifierTracked, Issue: 7, Reason: "the owner's events take a username"}}
	answersNothing := "identifier declaration widgets.EventsInput.UserID answers no integer handed where the record does not say an ID alone is taken"
	cases := []struct {
		name       string
		tool       *strings.Replacer
		routes     []apilive.Route
		declared   []identifierDeclaration
		wantStatus string
		wantStale  []string
	}{
		{name: "an integer where a username is taken", routes: identifiersRecord(), wantStatus: identifierNarrowed},
		{name: "declared tracked", routes: identifiersRecord(), declared: tracked, wantStatus: identifierDeclared},
		{name: "the input widened", tool: widened, routes: identifiersRecord(), declared: tracked, wantStale: []string{answersNothing}},
		{name: "the record says an ID alone", routes: idAlone, wantStatus: identifierIDOnly},
		{name: "the record says an ID alone, declared", routes: idAlone, declared: tracked, wantStatus: identifierIDOnly, wantStale: []string{answersNothing}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, stale := analyzeIdentifiersFixture(t, tc.tool, tc.routes, tc.declared)
			status := ""
			for _, argument := range report.Arguments {
				if argument.Site == "widgets.Events" {
					status = argument.Status
				}
			}
			if status != tc.wantStatus {
				t.Errorf("the events argument is %q, want %q", status, tc.wantStatus)
			}
			if !slices.Equal(stale, tc.wantStale) {
				t.Errorf("stale = %q, want %q", stale, tc.wantStale)
			}
		})
	}
}

// TestAnalyzeIdentifiers_NoRecord_LeavesEveryArgumentUnplacedAndSaysSo
// verifies a record that cannot be read fails the gate rather than passing
// it empty: the run says it could not read it, every argument is unplaced,
// and nothing narrows client-go's own parameters.
func TestAnalyzeIdentifiers_NoRecord_LeavesEveryArgumentUnplacedAndSaysSo(t *testing.T) {
	report, stale := analyzeIdentifiersFixture(t, nil, nil, nil)
	if len(stale) != 1 || !strings.HasPrefix(stale[0], "identifiers: reading the live API record") {
		t.Errorf("stale = %q, want the record's read error", stale)
	}
	for _, argument := range report.Arguments {
		if argument.Status != identifierUnplaced {
			t.Errorf("%s is %s, want unplaced", argumentLine(argument), argument.Status)
		}
	}
	if len(report.Arguments) != len(standInArguments) || len(report.SDKNarrowed) != 0 {
		t.Errorf("the section kept %d arguments and %d narrowings, want %d and none", len(report.Arguments), len(report.SDKNarrowed), len(standInArguments))
	}
}

// TestTakes_EachDeclaration_SaysWhatThePlaceholderTakes verifies the
// reading of a placeholder's declaration: the description decides when
// there is one, a word ending in name or path or the word slug saying a
// name or a path is taken and anything else an ID alone, whatever the type;
// with none, a String type says a name, an Integer one an ID, and anything
// else nothing.
func TestTakes_EachDeclaration_SaysWhatThePlaceholderTakes(t *testing.T) {
	cases := []struct {
		param apilive.Param
		want  string
	}{
		{param: apilive.Param{Type: "String", Desc: "ID or username of the user."}, want: takesName},
		{param: apilive.Param{Type: "[String, Integer]", Desc: "ID or URL-encoded path of the project."}, want: takesName},
		{param: apilive.Param{Type: "String", Desc: "Group's ID or slug."}, want: takesName},
		{param: apilive.Param{Type: "String", Desc: "Filename of the upload."}, want: takesName},
		{param: apilive.Param{Type: "[String, Integer]", Desc: "ID of the repository."}, want: takesID},
		{param: apilive.Param{Type: "String", Desc: "The ID of the namespace"}, want: takesID},
		{param: apilive.Param{Type: "Integer", Desc: "  "}, want: takesID},
		{param: apilive.Param{Type: "[String, Integer]"}, want: takesName},
		{param: apilive.Param{Type: "Integer"}, want: takesID},
		{param: apilive.Param{Type: "Hash"}, want: takesUndescribed},
		{param: apilive.Param{}, want: takesUndescribed},
	}
	for _, tc := range cases {
		t.Run(tc.param.Type+" "+tc.param.Desc, func(t *testing.T) {
			if got := takes(tc.param); got != tc.want {
				t.Errorf("takes(%+v) = %q, want %q", tc.param, got, tc.want)
			}
		})
	}
}

// TestIdentifierStatus_EachPlaceholderSet_ComesToOneStatus verifies the
// status before any declaration: no placeholder is unplaced whatever the
// integer is read from, a name or a path anywhere narrows over a
// placeholder nothing describes, which in turn is undescribed over an ID
// alone, and an integer read from no input that would be narrowed or
// undescribed is untraced when the trace stopped and read from no input
// when it did not; an ID alone needs neither.
func TestIdentifierStatus_EachPlaceholderSet_ComesToOneStatus(t *testing.T) {
	id, name, undescribed := placeholder{Takes: takesID}, placeholder{Takes: takesName}, placeholder{Takes: takesUndescribed}
	cases := []struct {
		name    string
		input   string
		stopped string
		held    []placeholder
		want    string
	}{
		{name: "nothing placed", input: "a.B.C", want: identifierUnplaced},
		{name: "nothing placed from no input", want: identifierUnplaced},
		{name: "nothing placed, untraced", stopped: "a.F: x", want: identifierUnplaced},
		{name: "an ID alone", input: "a.B.C", held: []placeholder{id, id}, want: identifierIDOnly},
		{name: "a name after an undescribed one", input: "a.B.C", held: []placeholder{undescribed, name}, want: identifierNarrowed},
		{name: "a name before an undescribed one", input: "a.B.C", held: []placeholder{name, undescribed}, want: identifierNarrowed},
		{name: "undescribed", input: "a.B.C", held: []placeholder{id, undescribed}, want: identifierUndescribed},
		{name: "a name from no input", held: []placeholder{name}, want: identifierNotFromInput},
		{name: "undescribed from no input", held: []placeholder{undescribed}, want: identifierNotFromInput},
		{name: "an ID from no input", held: []placeholder{id}, want: identifierIDOnly},
		{name: "a name untraced", stopped: "a.F: x", held: []placeholder{name}, want: identifierUntraced},
		{name: "undescribed untraced", stopped: "a.F: x", held: []placeholder{undescribed}, want: identifierUntraced},
		{name: "an ID untraced", stopped: "a.F: x", held: []placeholder{id}, want: identifierIDOnly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := identifierStatus(tc.input, tc.stopped, tc.held); got != tc.want {
				t.Errorf("identifierStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSummarizeIdentifiers_EachStatus_CountsOnceAndTheFindingsAloneAreKept
// verifies every status is counted under its own counter, the unfollowed
// method values and the client-go narrowings beside them, and that the
// gaps-only filter keeps the four statuses that are findings and every
// unfollowed method value, and none of the narrowings, as an empty list.
func TestSummarizeIdentifiers_EachStatus_CountsOnceAndTheFindingsAloneAreKept(t *testing.T) {
	statuses := []string{identifierIDOnly, identifierDeclared, identifierNotFromInput, identifierNarrowed, identifierUndescribed, identifierUnplaced, identifierUntraced, identifierNarrowed}
	unfollowed := []unfollowedValue{{Site: "a.F", Method: "A.B", Value: "c.A.B"}, {Site: "a.G", Method: "A.C", Value: "c.A.C"}}
	report := identifiersReport{Unfollowed: unfollowed, SDKNarrowed: []sdkNarrowing{{Method: "A.B"}}}
	for _, status := range statuses {
		report.Arguments = append(report.Arguments, identifierArgument{Status: status})
	}
	var summary reportSummary
	summarizeIdentifiers(&summary, report)
	want := reportSummary{
		IdentifierArguments: 8, IdentifiersIDOnly: 1, IdentifiersDeclared: 1, IdentifiersNotFromInput: 1,
		IdentifiersNarrowed: 2, IdentifiersUndescribed: 1, IdentifiersUnplaced: 1, IdentifiersUntraced: 1,
		IdentifiersUnfollowed: 2, SDKNarrowed: 1,
	}
	if summary != want {
		t.Errorf("summary = %+v, want %+v", summary, want)
	}
	kept := keepIdentifierFindings(report)
	var keptStatuses []string
	for _, argument := range kept.Arguments {
		keptStatuses = append(keptStatuses, argument.Status)
	}
	if !slices.Equal(keptStatuses, []string{identifierNarrowed, identifierUndescribed, identifierUnplaced, identifierUntraced, identifierNarrowed}) || kept.SDKNarrowed == nil || len(kept.SDKNarrowed) != 0 {
		t.Errorf("kept %q and %v, want the five findings and an empty list of narrowings", keptStatuses, kept.SDKNarrowed)
	}
	if !slices.Equal(kept.Unfollowed, unfollowed) {
		t.Errorf("kept unfollowed %+v, want %+v", kept.Unfollowed, unfollowed)
	}
}

// witnessIdentifierArguments is what the witness expects of every integer a
// handler hands a client-go parameter of type any on v3.30.0: by site,
// method and input field, its status and the record's route and
// placeholder, spelled as the report spells them.
var witnessIdentifierArguments = map[string]string{
	"users.GetStatus Users.GetUserStatus users.GetStatusInput.UserID":                                                                 "declared tracked GET /users/:user_id/status user_id name-or-path",
	"users.ListContributionEvents Users.ListUserContributionEvents users.ListContributionEventsInput.UserID":                          "declared tracked GET /users/:id/events id name-or-path",
	"users.ListSSHKeysForUser Users.ListSSHKeysForUser users.ListSSHKeysForUserInput.UserID":                                          "declared tracked GET /users/:user_id/keys user_id name-or-path",
	"containerregistry.GetRepository ContainerRegistry.GetSingleRegistryRepository containerregistry.GetRepositoryInput.RepositoryID": "id_only  GET /registry/repositories/:id id id",
	"runners.Get Runners.GetRunnerDetails runners.GetInput.RunnerID":                                                                  "id_only  GET /runners/:id id id",
	"runners.ListJobs Runners.ListRunnerJobs runners.ListJobsInput.RunnerID":                                                          "id_only  GET /runners/:id/jobs id id",
	"runners.ListManagers Runners.ListRunnerManagers runners.ListManagersInput.RunnerID":                                              "id_only  GET /runners/:id/managers id id",
	"runners.Remove Runners.RemoveRunner runners.RemoveInput.RunnerID":                                                                "id_only  DELETE /runners/:id id id",
	"runners.Update Runners.UpdateRunnerDetails runners.UpdateInput.RunnerID":                                                         "id_only  PUT /runners/:id id id",
}

// witnessSDKNarrowed is what the witness expects client-go v3.30.0 to
// narrow: each method and parameter that takes an int64 where the record
// says the group's, the project's or the user's ID or path is taken, every
// one of them called by a handler.
var witnessSDKNarrowed = []string{
	"CustomAttributes.DeleteCustomGroupAttribute group", "CustomAttributes.DeleteCustomProjectAttribute project",
	"CustomAttributes.DeleteCustomUserAttribute user", "CustomAttributes.GetCustomGroupAttribute group",
	"CustomAttributes.GetCustomProjectAttribute project", "CustomAttributes.GetCustomUserAttribute user",
	"CustomAttributes.ListCustomGroupAttributes group", "CustomAttributes.ListCustomProjectAttributes project",
	"CustomAttributes.ListCustomUserAttributes user", "CustomAttributes.SetCustomGroupAttribute group",
	"CustomAttributes.SetCustomProjectAttribute project", "CustomAttributes.SetCustomUserAttribute user",
	"GroupRepositoryStorageMove.GetStorageMoveForGroup group", "GroupRepositoryStorageMove.RetrieveAllStorageMovesForGroup group",
	"GroupRepositoryStorageMove.ScheduleStorageMoveForGroup group", "ProjectRepositoryStorageMove.GetStorageMoveForProject project",
	"ProjectRepositoryStorageMove.RetrieveAllStorageMovesForProject project", "ProjectRepositoryStorageMove.ScheduleStorageMoveForProject project",
}

// identifiersWitness is the part of the sdk report the identifiers witness
// reads, decoded as plain JSON into types of its own so that it compiles,
// and decodes, against a report without the section too.
type identifiersWitness struct {
	Identifiers *struct {
		Arguments []struct {
			Site, Method, Input, Status, Category string
			Issue                                 int
			Placeholders                          []struct{ Route, Name, Takes string }
		}
		SDKNarrowed []struct {
			Method, Param string
			Called        bool
		} `json:"sdk_narrowed"`
	}
}

// TestRun_Repository_DeclaresTheUserIdentifiersThatTakeAUsernameAtClientGoV3300
// is the witness on the real tree. GitLab takes the ID or the username of a
// user at GET /users/:id/events, /users/:user_id/status and
// /users/:user_id/keys, and client-go takes any there, while
// user.contribution_events, user.get_status and user.ssh_keys_for_user
// offer user_id as an integer alone: nothing compared what a handler hands
// such a parameter with what the record says the placeholder takes. The
// three are declared tracked until the inputs are widened, and the six
// other integers handed to a parameter of type any fill a placeholder the
// record says takes an ID alone. Beside them client-go narrows eighteen
// parameters itself, the custom attributes and repository storage moves of
// a group, a project or a user taking an int64 where the record says an ID
// or a path, which is backlog upstream and gates nothing. Both lists come
// out the same on client-go v3.38.0: B4 of the bump stack widens the three
// inputs, which turns their declarations stale and drops them from the
// arguments, and the narrowings change only when client-go widens those
// parameters.
//
// It reads the report as plain JSON and spells what it expects, so copied
// on its own into a tree that has no such section it compiles and fails
// there, which is the point.
func TestRun_Repository_DeclaresTheUserIdentifiersThatTakeAUsernameAtClientGoV3300(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	content, _, err := Run(root, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var rep identifiersWitness
	if unmarshalErr := json.Unmarshal(content, &rep); unmarshalErr != nil {
		t.Fatalf("report is not JSON: %v", unmarshalErr)
	}
	if rep.Identifiers == nil {
		t.Fatal("the sdk report has no identifiers section: nothing holds an integer a handler hands client-go to what the record says the placeholder takes")
	}
	got := map[string]string{}
	for _, argument := range rep.Identifiers.Arguments {
		line := []string{argument.Status, argument.Category}
		for _, held := range argument.Placeholders {
			line = append(line, held.Route, held.Name, held.Takes)
		}
		got[argument.Site+" "+argument.Method+" "+argument.Input] = strings.Join(line, " ")
		if argument.Category == "tracked" && argument.Issue <= 0 {
			t.Errorf("%s %s is tracked with no issue", argument.Site, argument.Input)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(witnessIdentifierArguments)) {
		if got[key] != witnessIdentifierArguments[key] {
			t.Errorf("%s = %q, want %q", key, got[key], witnessIdentifierArguments[key])
		}
	}
	for _, key := range slices.Sorted(maps.Keys(got)) {
		if _, expected := witnessIdentifierArguments[key]; !expected {
			t.Errorf("%s = %q, which the witness does not expect", key, got[key])
		}
	}
	var narrowed []string
	for _, narrowing := range rep.Identifiers.SDKNarrowed {
		narrowed = append(narrowed, narrowing.Method+" "+narrowing.Param)
		if !narrowing.Called {
			t.Errorf("%s %s is called by no handler, want every narrowing called", narrowing.Method, narrowing.Param)
		}
	}
	if !slices.Equal(narrowed, witnessSDKNarrowed) {
		t.Errorf("sdk_narrowed = %q, want %q", narrowed, witnessSDKNarrowed)
	}
}
