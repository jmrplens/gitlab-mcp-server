package testutil

import (
	"reflect"
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

// GraphQLPaginationOutput stands in for the shared cursor pagination.
type GraphQLPaginationOutput struct {
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
}

// TestFillFixture_States_SetThePaginationEachRuleReads checks the three
// populated states on the two pagination shapes, recognized by name and
// fields as the shared ones are.
func TestFillFixture_States_SetThePaginationEachRuleReads(t *testing.T) {
	type listOutput struct {
		Items      []fixtureItem
		Pagination PaginationOutput
		Cursor     GraphQLPaginationOutput
	}
	cases := []struct {
		name   string
		state  FixtureState
		want   PaginationOutput
		cursor GraphQLPaginationOutput
		items  int
	}{
		{name: "zero", state: FixtureZero, want: PaginationOutput{}, items: 0},
		{name: "multi-page", state: FixtureMultiPage, want: PaginationOutput{Page: 1, PerPage: 20, TotalItems: 45, TotalPages: 3, NextPage: 2, HasMore: true}, cursor: GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, items: 2},
		{name: "single page", state: FixtureSinglePage, want: PaginationOutput{Page: 1, PerPage: 20, TotalItems: 2, TotalPages: 1}, cursor: GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, items: 2},
		{name: "keyset", state: FixtureKeyset, want: PaginationOutput{Page: 1, PerPage: 20, NextPage: 2, HasMore: true}, cursor: GraphQLPaginationOutput{HasNextPage: true, EndCursor: "cursor7"}, items: 2},
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
		})
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
