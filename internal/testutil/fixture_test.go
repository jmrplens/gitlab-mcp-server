package testutil

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// PaginationOutput stands in for the shared offset pagination, which the
// filler recognizes by name and fields rather than by package.
type PaginationOutput struct {
	Page       int64
	PerPage    int64
	TotalItems int64
	TotalPages int64
	NextPage   int64
	HasMore    bool
}

// GraphQLPaginationOutput stands in for the shared cursor pagination, and
// GraphQLForwardPaginationOutput for the forward-only half of it, which the
// filler recognizes under its own name.
type GraphQLPaginationOutput struct {
	HasNextPage bool
	EndCursor   string
}

type GraphQLForwardPaginationOutput struct {
	HasNextPage bool
	EndCursor   string
}

// HintableOutput stands in for the shared hints holder, which the filler
// leaves empty.
type HintableOutput struct {
	NextSteps []string
}

// stamp is a named time, the shape client-go gives an ISO time.
type stamp time.Time

// fixtureUser is a nested object.
type fixtureUser struct {
	Username string `json:"username"`
	WebURL   string `json:"web_url,omitempty"`
}

// FixtureEmbedded is an embedded struct, whose fields take the path of the
// struct that embeds it. It is exported because an embedded field is named
// by its type, and an unexported one is not a field the filler may set.
type FixtureEmbedded struct {
	Ref string `json:"ref"`
}

// fixtureItem is the shape a GitLab response fills, with one of every kind
// the filler has a rule for.
type fixtureItem struct {
	FixtureEmbedded
	HintableOutput
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	WebURL      string            `json:"web_url"`
	AvatarHref  string            `json:"avatar_href"`
	CreatedAt   string            `json:"created_at"`
	DueDate     string            `json:"due_date,omitempty"`
	Description string            `json:"description,omitempty"`
	Locked      bool              `json:"locked"`
	Weight      float64           `json:"weight,omitempty"`
	Count       uint              `json:"count"`
	When        time.Time         `json:"when"`
	Stamped     stamp             `json:"stamped"`
	Author      *fixtureUser      `json:"author,omitempty"`
	Labels      []string          `json:"labels"`
	Users       []fixtureUser     `json:"users"`
	Meta        map[string]string `json:"meta"`
	Extra       any               `json:"extra"`
	Self        *fixtureItem      `json:"self,omitempty"`
	Coordinates [2]int            `json:"coordinates"`
	hidden      string
}

// TestFillFixture_Populated_GivesEveryKindItsSentinel checks the value each
// kind of field receives, since the scan rules read the render for exactly
// these values.
func TestFillFixture_Populated_GivesEveryKindItsSentinel(t *testing.T) {
	v := FillFixture(reflect.TypeFor[fixtureItem](), FixtureOptions{State: FixtureMultiPage})
	item, ok := reflect.TypeAssert[fixtureItem](v)
	if !ok {
		t.Fatalf("FillFixture returned %T, want fixtureItem", v.Interface())
	}

	cases := []struct {
		name string
		got  any
		want any
	}{
		{name: "a number", got: item.ID, want: int64(7)},
		{name: "a string", got: item.Title, want: "Title7"},
		{name: "an embedded field takes the embedding path", got: item.Ref, want: "Ref7"},
		{name: "the hints holder stays empty", got: item.NextSteps == nil, want: true},
		{name: "a URL", got: item.WebURL, want: "https://gitlab.example/WebURL/7"},
		{name: "an href", got: item.AvatarHref, want: "https://gitlab.example/AvatarHref/7"},
		{name: "an instant string", got: item.CreatedAt, want: FixtureSentinelRFC3339},
		{name: "a date string", got: item.DueDate, want: FixtureSentinelRFC3339},
		{name: "a flag", got: item.Locked, want: true},
		{name: "a float", got: item.Weight, want: 7.0},
		{name: "an unsigned number", got: item.Count, want: uint(7)},
		{name: "a time", got: item.When.Equal(FixtureSentinelTime), want: true},
		{name: "a named time", got: time.Time(item.Stamped).Equal(FixtureSentinelTime), want: true},
		{name: "a pointer's field", got: item.Author != nil && item.Author.Username == "Username7", want: true},
		{name: "a pointer's URL", got: item.Author != nil && item.Author.WebURL == "https://gitlab.example/WebURL/7", want: true},
		{name: "a slice of strings", got: strings.Join(item.Labels, "|"), want: "Labels07|Labels17"},
		{name: "a slice of structs", got: len(item.Users) == 2 && item.Users[1].Username == "Username7", want: true},
		{name: "a map", got: item.Meta["MetaKey7"], want: "Meta7"},
		{name: "an interface stays nil", got: item.Extra == nil, want: true},
		{name: "an array", got: item.Coordinates, want: [2]int{7, 7}},
		{name: "an unexported field stays empty", got: item.hidden, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

// TestFillFixture_SelfReference_StopsAtTheDepthBound checks that a type
// holding a pointer to itself is filled to a bounded depth and no further.
//
// The last level that is filled is named rather than bounded, because a bound
// alone passes for every depth under it: the whole point of the constant is
// that a fixture is deep enough for a nested render to have something to show
// and shallow enough to terminate, and a bound that only says "not deeper
// than" is satisfied by a fixture one level deep. Each step of the self chain
// costs two levels, a struct field and the pointer under it, so the third
// object is the last one whose own fields the bound admits.
func TestFillFixture_SelfReference_StopsAtTheDepthBound(t *testing.T) {
	v := FillFixture(reflect.TypeFor[fixtureItem](), FixtureOptions{State: FixtureMultiPage})
	item := v.Interface().(fixtureItem)

	depth := 0
	for cursor := &item; cursor != nil; cursor = cursor.Self {
		depth++
		if depth > maxFixtureDepth+2 {
			t.Fatalf("the fixture nests deeper than %d", maxFixtureDepth)
		}
	}
	if depth < 2 {
		t.Errorf("the self pointer was not allocated at all")
	}

	if item.Self == nil || item.Self.Self == nil {
		t.Fatalf("the self chain stops before the depth bound does: %+v", item.Self)
	}
	if item.Self.Self.Title != "Title7" {
		t.Errorf("the third object's title = %q, want it filled: the bound admits its fields", item.Self.Self.Title)
	}
	if item.Self.Self.Self == nil || item.Self.Self.Self.Title != "" {
		t.Errorf("the fourth object = %+v, want it allocated and left empty by the bound", item.Self.Self.Self)
	}
}

// sliceLoop contains itself through a slice, and mapLoop through a map value.
// Both are legal Go types and neither terminates unless the depth counted for
// a container element is the depth of the value that holds it plus one.
type sliceLoop struct {
	Name string
	Kids []sliceLoop
}

type mapLoop struct {
	Name string
	Kids map[string]mapLoop
}

// The array and map-key chains are spelled out level by level, because
// neither container can hold the type that declares it: an array is inline,
// and a map key must be comparable, which a struct holding a map is not.
type (
	arrayLevel3 struct{ Name string }
	arrayLevel2 struct {
		Name string
		Kids [1]arrayLevel3
	}
	arrayLevel1 struct {
		Name string
		Kids [1]arrayLevel2
	}
	arrayRoot struct {
		Kids [1]arrayLevel1
	}
)

type (
	keyLevel4 struct{ Name string }
	keyLevel3 struct {
		Name string
		Next keyLevel4
	}
	keyLevel2 struct{ Next keyLevel3 }
	keyLevel1 struct{ Next keyLevel2 }
	keyRoot   struct {
		Kids map[keyLevel1]string
	}
)

// The four tests below check the depth bound through each container the filler
// descends into: a slice element, a map value, an array element and a map key.
//
// The bound is the only thing between a fixture and a type that contains
// itself, and a container that hands its element the depth it was given rather
// than one more never reaches it: the two self-containing types would then be
// filled for ever. The two chains spelled out level by level ask the same
// question of the containers that cannot hold themselves, by naming the last
// level the bound admits.

// TestFillFixture_ASliceOfItsOwnType_StopsAtTheDepthBound checks the slice.
func TestFillFixture_ASliceOfItsOwnType_StopsAtTheDepthBound(t *testing.T) {
	v := FillFixture(reflect.TypeFor[sliceLoop](), FixtureOptions{State: FixtureMultiPage})
	root, ok := reflect.TypeAssert[sliceLoop](v)
	if !ok {
		t.Fatalf("FillFixture returned %T, want sliceLoop", v.Interface())
	}

	if len(root.Kids) != 2 || len(root.Kids[0].Kids) != 2 {
		t.Fatalf("the fixture stops before the bound does: %+v", root)
	}
	if root.Kids[0].Kids[0].Name == "" {
		t.Error("the third object's name is empty, want it filled: the bound admits its fields")
	}
	if got := root.Kids[0].Kids[0].Kids; len(got) != 2 || got[0].Name != "" {
		t.Errorf("the fourth level = %+v, want two elements the bound left empty", got)
	}
}

// TestFillFixture_AMapOfItsOwnType_StopsAtTheDepthBound checks the map value.
func TestFillFixture_AMapOfItsOwnType_StopsAtTheDepthBound(t *testing.T) {
	v := FillFixture(reflect.TypeFor[mapLoop](), FixtureOptions{State: FixtureMultiPage})
	root, ok := reflect.TypeAssert[mapLoop](v)
	if !ok {
		t.Fatalf("FillFixture returned %T, want mapLoop", v.Interface())
	}

	level1, ok := onlyMapLoop(t, root.Kids)
	if !ok {
		return
	}
	level2, ok := onlyMapLoop(t, level1.Kids)
	if !ok {
		return
	}
	if level2.Name == "" {
		t.Error("the third object's name is empty, want it filled: the bound admits its fields")
	}
	level3, ok := onlyMapLoop(t, level2.Kids)
	if !ok {
		return
	}
	if level3.Name != "" || len(level3.Kids) != 0 {
		t.Errorf("the fourth level = %+v, want the entry the bound left empty", level3)
	}
}

// TestFillFixture_AnArrayChain_StopsAtTheDepthBound checks the array element.
func TestFillFixture_AnArrayChain_StopsAtTheDepthBound(t *testing.T) {
	v := FillFixture(reflect.TypeFor[arrayRoot](), FixtureOptions{State: FixtureMultiPage})
	root, ok := reflect.TypeAssert[arrayRoot](v)
	if !ok {
		t.Fatalf("FillFixture returned %T, want arrayRoot", v.Interface())
	}

	if root.Kids[0].Name == "" || root.Kids[0].Kids[0].Name == "" {
		t.Fatalf("the array chain stops before the bound does: %+v", root)
	}
	if got := root.Kids[0].Kids[0].Kids[0].Name; got != "" {
		t.Errorf("the fourth level's name = %q, want the bound to have left it empty", got)
	}
}

// TestFillFixture_AMapKeyChain_StopsAtTheDepthBound checks the map key, which
// is filled apart from the value beside it.
func TestFillFixture_AMapKeyChain_StopsAtTheDepthBound(t *testing.T) {
	v := FillFixture(reflect.TypeFor[keyRoot](), FixtureOptions{State: FixtureMultiPage})
	root, ok := reflect.TypeAssert[keyRoot](v)
	if !ok {
		t.Fatalf("FillFixture returned %T, want keyRoot", v.Interface())
	}

	if len(root.Kids) != 1 {
		t.Fatalf("the map holds %d entries, want the one the filler writes: %+v", len(root.Kids), root.Kids)
	}
	for key := range root.Kids {
		if key.Next.Next.Name == "" {
			t.Error("the key's third level is empty, want it filled: the bound admits its fields")
		}
		if got := key.Next.Next.Next.Name; got != "" {
			t.Errorf("the key's fourth level = %q, want the bound to have left it empty", got)
		}
	}
}

// onlyMapLoop returns the single value of a one-entry map, reporting when the
// map does not hold exactly one.
func onlyMapLoop(t *testing.T, kids map[string]mapLoop) (mapLoop, bool) {
	t.Helper()
	if len(kids) != 1 {
		t.Errorf("the map holds %d entries, want the one the filler writes: %+v", len(kids), kids)
		return mapLoop{}, false
	}
	for _, value := range kids {
		return value, true
	}
	return mapLoop{}, false
}

// TestFillFixture_States_SetThePaginationEachRuleReads checks the three
// populated states on the three pagination shapes, recognized by name and
// fields as the shared ones are.
//
// The forward-only cursor shape is one of them, and is asserted beside the
// two-way one rather than assumed to follow it: it is a different type under a
// different name, so it is recognized by a rule of its own and a rule that
// stopped matching it would leave every forward-only list's fixture without
// the cursor its render reads.
func TestFillFixture_States_SetThePaginationEachRuleReads(t *testing.T) {
	type listOutput struct {
		Items      []fixtureItem
		Pagination PaginationOutput
		Cursor     GraphQLPaginationOutput
		Forward    GraphQLForwardPaginationOutput
	}
	cases := []struct {
		name    string
		state   FixtureState
		want    PaginationOutput
		cursor  GraphQLPaginationOutput
		forward GraphQLForwardPaginationOutput
		items   int
	}{
		{name: "zero", state: FixtureZero, want: PaginationOutput{}, items: 0},
		{name: "multi-page", state: FixtureMultiPage, want: PaginationOutput{Page: 1, PerPage: 20, TotalItems: 45, TotalPages: 3, NextPage: 2, HasMore: true}, cursor: GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, forward: GraphQLForwardPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, items: 2},
		{name: "single page", state: FixtureSinglePage, want: PaginationOutput{Page: 1, PerPage: 20, TotalItems: 2, TotalPages: 1}, cursor: GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, forward: GraphQLForwardPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, items: 2},
		{name: "keyset", state: FixtureKeyset, want: PaginationOutput{Page: 1, PerPage: 20, NextPage: 2, HasMore: true}, cursor: GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, forward: GraphQLForwardPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, items: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := FillFixture(reflect.TypeFor[listOutput](), FixtureOptions{State: tc.state})
			out := v.Interface().(listOutput)
			if len(out.Items) != tc.items {
				t.Errorf("%d item(s), want %d", len(out.Items), tc.items)
			}
			if out.Pagination != tc.want {
				t.Errorf("pagination = %+v, want %+v", out.Pagination, tc.want)
			}
			if out.Cursor != tc.cursor {
				t.Errorf("cursor pagination = %+v, want %+v", out.Cursor, tc.cursor)
			}
			if out.Forward != tc.forward {
				t.Errorf("forward pagination = %+v, want %+v", out.Forward, tc.forward)
			}
		})
	}
}

// narrowNumbers carries every numeric kind fixtureItem does not, so the
// sentinel is known to reach each width rather than only the widest.
type narrowNumbers struct {
	I8  int8
	I16 int16
	I32 int32
	U8  uint8
	U16 uint16
	U32 uint32
	U64 uint64
	F32 float32
}

// TestFillFixture_EveryNumericWidth_TakesTheSameSentinel checks the kinds the
// main fixture does not carry.
//
// The filler lists them by kind rather than by size, so a width left out of a
// list is a field that stays at zero while every other number in the same
// response is 7 — and a render that shows a zero where the scan expects the
// sentinel reports the formatter, not the fixture.
func TestFillFixture_EveryNumericWidth_TakesTheSameSentinel(t *testing.T) {
	v := FillFixture(reflect.TypeFor[narrowNumbers](), FixtureOptions{State: FixtureMultiPage})
	got, ok := reflect.TypeAssert[narrowNumbers](v)
	if !ok {
		t.Fatalf("FillFixture returned %T, want narrowNumbers", v.Interface())
	}

	want := narrowNumbers{I8: 7, I16: 7, I32: 7, U8: 7, U16: 7, U32: 7, U64: 7, F32: 7}
	if got != want {
		t.Errorf("FillFixture = %+v, want %+v", got, want)
	}
}

// TestIsShape_Types_MatchesNameAndFields checks the recognition of the
// shared shapes: the name alone is not enough, and neither is the field set
// under another name.
func TestIsShape_Types_MatchesNameAndFields(t *testing.T) {
	type PaginationOutput struct{ Page int64 }
	cases := []struct {
		name string
		typ  reflect.Type
		want bool
	}{
		{name: "the stand-in with every field", typ: reflect.TypeFor[PaginationOutput](), want: false},
		{name: "a struct with the fields and another name", typ: reflect.TypeFor[fixtureItem](), want: false},
		{name: "not a struct", typ: reflect.TypeFor[string](), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPaginationShape(tc.typ); got != tc.want {
				t.Errorf("isPaginationShape(%v) = %v, want %v", tc.typ, got, tc.want)
			}
		})
	}
}

// TestFillFixture_Text_ReplacesEverySentinel checks the hook a hostile
// render is built through: every string field, nested ones included, takes
// the value the caller supplies for its path.
func TestFillFixture_Text_ReplacesEverySentinel(t *testing.T) {
	var paths []string
	v := FillFixture(reflect.TypeFor[fixtureItem](), FixtureOptions{State: FixtureMultiPage, Text: func(path string) string {
		paths = append(paths, path)
		return "x|y"
	}})
	item := v.Interface().(fixtureItem)

	if item.Title != "x|y" || item.Author == nil || item.Author.Username != "x|y" || item.Labels[0] != "x|y" {
		t.Errorf("the supplied text did not reach every string: %+v", item)
	}
	for _, want := range []string{".Title", ".Author.Username", ".Labels0", ".Users1.WebURL", ".MetaKey"} {
		t.Run(want, func(t *testing.T) {
			found := false
			for _, path := range paths {
				if path == want {
					found = true
				}
			}
			if !found {
				t.Errorf("path %q was never asked for; asked: %v", want, paths)
			}
		})
	}
}

// TestFixtureText_Names_ChooseTheSentinelByShape checks the sentinel a field
// name selects.
func TestFixtureText_Names_ChooseTheSentinelByShape(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "a plain field", path: ".Title", want: "Title7"},
		{name: "a web URL", path: ".Item.WebURL", want: "https://gitlab.example/WebURL/7"},
		{name: "a lowercase url", path: ".Url", want: "https://gitlab.example/Url/7"},
		{name: "an instant", path: ".CreatedAt", want: FixtureSentinelRFC3339},
		{name: "a date", path: ".DueDate", want: FixtureSentinelRFC3339},
		{name: "an on date", path: ".ExpiresOn", want: FixtureSentinelRFC3339},
		{name: "an until date", path: ".ValidUntil", want: FixtureSentinelRFC3339},
		{name: "a slice element of an instant", path: ".Events0.CreatedAt", want: FixtureSentinelRFC3339},
		{name: "a bare suffix is not an instant", path: ".At", want: "At7"},
		{name: "an indexed element", path: ".Labels1", want: "Labels17"},
		{name: "no field at all", path: "", want: "Value7"},
		{name: "a name with nothing to keep", path: ".", want: "Value7"},
		{name: "a name of punctuation alone", path: ".!!", want: "Value7"},
		// The three character ranges are kept at both ends: 'a', 'z', 'A',
		// 'Z', '0' and '9' are all inside them, and a range that loses either
		// end silently drops a letter from a sentinel a scan is looking for.
		{name: "both ends of every range are kept", path: ".Aa0Zz9", want: "Aa0Zz97"},
		// One character from each gap between the ranges: ':' sits above '9'
		// and below 'A', '_' above 'Z' and below 'a', and the accented letter
		// above 'z'. None of them may reach a sentinel, since a value holding
		// punctuation is a value a render could be reading as Markdown.
		{name: "a character from each gap is dropped", path: ".a:b_céd", want: "abcd7"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FixtureText(tc.path); got != tc.want {
				t.Errorf("FixtureText(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestFixtureURLShaped_Names_ReadTheAddressFields checks the names a hostile
// render keeps as addresses.
func TestFixtureURLShaped_Names_ReadTheAddressFields(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "WebURL", want: true},
		{name: "url", want: true},
		{name: "AvatarHref", want: true},
		{name: "Title", want: false},
		{name: "Curl", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FixtureURLShaped(tc.name); got != tc.want {
				t.Errorf("FixtureURLShaped(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestOptionalFields_Type_ListsWhatGitLabMayOmit checks the oracle of the
// absent-value rule: pointers, slices, maps, interfaces and omitempty fields,
// nested ones included, and nothing GitLab always sends.
func TestOptionalFields_Type_ListsWhatGitLabMayOmit(t *testing.T) {
	var names []string
	for _, field := range OptionalFields(reflect.TypeFor[fixtureItem]()) {
		names = append(names, field.Name)
	}

	want := "DueDate Description Weight Author Author.WebURL Labels Users Meta Extra Self Self.DueDate Self.Description Self.Weight Self.Author Self.Author.WebURL Self.Labels Self.Users Self.Meta Self.Extra Self.Self"
	got := strings.Join(names, " ")
	if !strings.HasPrefix(got, want) {
		t.Errorf("OptionalFields = %q, want it to open with %q", got, want)
	}
	for _, always := range []string{"ID", "Title", "Locked", "When", "Ref", "NextSteps"} {
		t.Run(always, func(t *testing.T) {
			for _, name := range names {
				if name == always {
					t.Errorf("%s is listed as optional, and GitLab always sends it", always)
				}
			}
		})
	}

	// The walk stops at the same depth the fill does, and the last level it
	// reaches is named rather than bounded: a type that contains itself has no
	// last field otherwise, and a bound alone is satisfied by a walk that gave
	// up at the first level, which would quietly stop asking the absent-value
	// rule about everything nested.
	deepest := strings.TrimSuffix(strings.Repeat("Self.", maxFixtureDepth+1), ".")
	if !slices.Contains(names, deepest) {
		t.Errorf("OptionalFields stops before %q, so the walk is shallower than the fill", deepest)
	}
	if beyond := deepest + ".Self"; slices.Contains(names, beyond) {
		t.Errorf("OptionalFields reaches %q, which is past the depth bound", beyond)
	}
}

// TestZeroField_Paths_ZeroesOneFieldOrSaysWhyNot checks the differential's
// mutation: the named field alone is zeroed, through a pointer, and a path
// across a nil pointer or off the struct is refused rather than applied.
func TestZeroField_Paths_ZeroesOneFieldOrSaysWhyNot(t *testing.T) {
	v := FillFixture(reflect.TypeFor[fixtureItem](), FixtureOptions{State: FixtureMultiPage})
	fields := map[string]FixtureField{}
	for _, field := range OptionalFields(reflect.TypeFor[fixtureItem]()) {
		fields[field.Name] = field
	}

	if !ZeroField(v, fields["Author.WebURL"].Path) {
		t.Fatal("ZeroField refused a path through an allocated pointer")
	}
	item := v.Interface().(fixtureItem)
	if item.Author == nil || item.Author.WebURL != "" || item.Author.Username != "Username7" {
		t.Errorf("zeroing Author.WebURL left %+v", item.Author)
	}
	if item.Title != "Title7" {
		t.Errorf("zeroing one field changed another: %+v", item)
	}

	if !ZeroField(v, fields["Author"].Path) || v.Interface().(fixtureItem).Author != nil {
		t.Error("ZeroField did not zero the pointer itself")
	}
	if ZeroField(v, fields["Author.WebURL"].Path) {
		t.Error("ZeroField applied a path across the pointer it had just zeroed")
	}
	if ZeroField(v, []int{99}) {
		t.Error("ZeroField applied a path off the struct")
	}
	// One past the last field, which is the index an off-by-one in the bound
	// admits and reflect answers with a panic rather than an error.
	if ZeroField(v, []int{reflect.TypeFor[fixtureItem]().NumField()}) {
		t.Error("ZeroField applied a path one index past the last field")
	}
	if ZeroField(v, append(append([]int{}, fields["Labels"].Path...), 0)) {
		t.Error("ZeroField applied a path through a slice, which is not a struct")
	}
	if ZeroField(v, nil) {
		t.Error("ZeroField applied an empty path")
	}
	if ZeroField(reflect.ValueOf(item), fields["Description"].Path) {
		t.Error("ZeroField applied a path on a value it cannot set")
	}
}

// TestOptionalFields_PointerType_ListsTheFieldsOfWhatItPointsAt checks that
// a registered pointer type is walked through to the struct, as the registry
// dereferences it.
func TestOptionalFields_PointerType_ListsTheFieldsOfWhatItPointsAt(t *testing.T) {
	direct := OptionalFields(reflect.TypeFor[fixtureItem]())
	through := OptionalFields(reflect.TypeFor[**fixtureItem]())

	if len(direct) == 0 || len(direct) != len(through) {
		t.Errorf("OptionalFields lists %d field(s) through the pointer and %d directly", len(through), len(direct))
	}
}

// TestFixtureState_String_NamesEveryState checks the subtest names the
// harness prints.
func TestFixtureState_String_NamesEveryState(t *testing.T) {
	cases := []struct {
		state FixtureState
		want  string
	}{
		{state: FixtureZero, want: "zero"},
		{state: FixtureMultiPage, want: "multi-page"},
		{state: FixtureSinglePage, want: "single-page"},
		{state: FixtureKeyset, want: "keyset"},
		{state: FixtureState(9), want: "fixture-9"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.state.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// oddShape carries, under the names the pagination setters use, a field of
// the wrong kind and a field of the right kind that no caller may set.
type oddShape struct {
	Page      string
	HasMore   string
	EndCursor int
	page      int64
	hasMore   bool
	endCursor string
}

// TestSetField_AFieldTheSetterMayNotWrite_IsLeftAlone checks the three
// conditions each pagination setter is guarded by.
//
// They exist because the shapes are matched by name: a type this package never
// saw can carry a Page of its own that is a string, or an unexported one, and
// reflect answers a write to either with a panic rather than an error. Each
// condition must therefore hold on its own, and a setter that wrote when any
// one of them was satisfied would take the whole suite down with it rather
// than skipping the field.
//
// Every field starts holding something other than what the setter would write,
// so "left alone" is a real comparison rather than a zero value agreeing with
// itself.
func TestSetField_AFieldTheSetterMayNotWrite_IsLeftAlone(t *testing.T) {
	populated := oddShape{
		Page:      "not a number",
		HasMore:   "not a flag",
		EndCursor: 9,
		page:      1,
		hasMore:   true,
		endCursor: "held",
	}
	cases := []struct {
		name string
		set  func(reflect.Value)
	}{
		{name: "an int64 setter on a string field", set: func(v reflect.Value) { setInt(v, "Page", 7) }},
		{name: "an int64 setter on an unexported field", set: func(v reflect.Value) { setInt(v, "page", 7) }},
		{name: "an int64 setter on a name the struct lacks", set: func(v reflect.Value) { setInt(v, "Absent", 7) }},
		{name: "a bool setter on a string field", set: func(v reflect.Value) { setBool(v, "HasMore", false) }},
		{name: "a bool setter on an unexported field", set: func(v reflect.Value) { setBool(v, "hasMore", false) }},
		{name: "a bool setter on a name the struct lacks", set: func(v reflect.Value) { setBool(v, "Absent", false) }},
		{name: "a string setter on an int field", set: func(v reflect.Value) { setString(v, "EndCursor", "written") }},
		{name: "a string setter on an unexported field", set: func(v reflect.Value) { setString(v, "endCursor", "written") }},
		{name: "a string setter on a name the struct lacks", set: func(v reflect.Value) { setString(v, "Absent", "written") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shape := populated
			v := reflect.ValueOf(&shape).Elem()

			tc.set(v)

			if shape != populated {
				t.Errorf("the setter wrote %+v, want %+v", shape, populated)
			}
		})
	}
}
