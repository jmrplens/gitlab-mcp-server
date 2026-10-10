package required

import (
	"maps"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// TestPlaceholderName_NamesTheParameterASegmentHolds verifies a segment names
// a parameter when it is a `:name` placeholder or a `*name` wildcard with a
// name after the sigil, and that a literal segment or a bare sigil names none.
func TestPlaceholderName_NamesTheParameterASegmentHolds(t *testing.T) {
	cases := []struct {
		name    string
		segment string
		want    string
		isParam bool
	}{
		{name: "placeholder", segment: ":issue_iid", want: "issue_iid", isParam: true},
		{name: "wildcard", segment: "*file_path", want: "file_path", isParam: true},
		{name: "literal", segment: "projects"},
		{name: "bare colon", segment: ":"},
		{name: "bare star", segment: "*"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, isParam := placeholderName(testCase.segment)
			if got != testCase.want || isParam != testCase.isParam {
				t.Errorf("placeholderName(%q) = %q, %t; want %q, %t", testCase.segment, got, isParam, testCase.want, testCase.isParam)
			}
		})
	}
}

// TestSingular_TakesEachEnglishPluralShape verifies the three plural shapes
// GitLab's collections use, and that a name in none of them is kept: the
// `ies` shape is tried before the plain `s`, and `sses` and `xes` lose their
// `es` rather than only the `s`.
func TestSingular_TakesEachEnglishPluralShape(t *testing.T) {
	cases := map[string]string{
		"repositories": "repository",
		"classes":      "class",
		"boxes":        "box",
		"projects":     "project",
		"staff":        "staff",
	}
	for plural, want := range cases {
		t.Run(plural, func(t *testing.T) {
			if got := singular(plural); got != want {
				t.Errorf("singular(%q) = %q, want %q", plural, got, want)
			}
		})
	}
}

// TestIDField_NamesABareIDAfterTheCollectionBeforeIt verifies a bare `:id` is
// the nearest literal before it in the singular with `_id`, placeholders and
// the `-` scope separator skipped, and stays `id` when no literal precedes it.
func TestIDField_NamesABareIDAfterTheCollectionBeforeIt(t *testing.T) {
	cases := []struct {
		name   string
		before []string
		want   string
	}{
		{name: "collection", before: []string{"projects"}, want: "project_id"},
		{name: "past placeholder and dash", before: []string{"groups", ":group_id", "-"}, want: "group_id"},
		{name: "nested", before: []string{"projects", ":id", "geo_sites"}, want: "geo_site_id"},
		{name: "nothing before"},
		{name: "only placeholders", before: []string{":x", "-"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			want := testCase.want
			if want == "" {
				want = "id"
			}
			if got := idField(testCase.before); got != want {
				t.Errorf("idField(%v) = %q, want %q", testCase.before, got, want)
			}
		})
	}
}

// TestPolymorphicField_NamesTheObjectTheCollectionHolds verifies a
// polymorphic placeholder becomes the identifier of the collection right
// before it, and keeps its own name for a collection the table does not hold
// or when nothing precedes it.
func TestPolymorphicField_NamesTheObjectTheCollectionHolds(t *testing.T) {
	cases := []struct {
		name   string
		before []string
		want   string
	}{
		{name: "issue", before: []string{"projects", ":id", "issues"}, want: "issue_iid"},
		{name: "commit", before: []string{"projects", ":id", "repository", "commits"}, want: "sha"},
		{name: "unknown collection", before: []string{"projects", ":id", "widgets"}, want: "noteable_id"},
		{name: "nothing before", want: "noteable_id"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := polymorphicField("noteable_id", testCase.before); got != testCase.want {
				t.Errorf("polymorphicField(noteable_id, %v) = %q, want %q", testCase.before, got, testCase.want)
			}
		})
	}
}

// TestPathFields_NamesEachPlaceholderAndWhetherEveryPathCarriesIt verifies
// the field each placeholder of a path is named by and whether every spelling
// of the path carries it: a placeholder in an optional group is carried by
// one spelling of two, and every other by both.
func TestPathFields_NamesEachPlaceholderAndWhetherEveryPathCarriesIt(t *testing.T) {
	cases := []struct {
		name string
		path string
		want map[string]pathParam
	}{
		{
			name: "plain",
			path: "/projects/:id/issues/:issue_iid",
			want: map[string]pathParam{"id": {field: "project_id", always: true}, "issue_iid": {field: "issue_iid", always: true}},
		},
		{
			name: "optional group",
			path: "/projects/:id/(ref/:ref/)trigger/pipeline",
			want: map[string]pathParam{"id": {field: "project_id", always: true}, "ref": {field: "ref"}},
		},
		{
			name: "polymorphic and wildcard",
			path: "/projects/:id/issues/:noteable_id/notes/*rest",
			want: map[string]pathParam{
				"id":          {field: "project_id", always: true},
				"noteable_id": {field: "issue_iid", always: true},
				"rest":        {field: "rest", always: true},
			},
		},
		{name: "no placeholder", path: "/topics", want: map[string]pathParam{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := pathFields(testCase.path); !maps.Equal(got, testCase.want) {
				t.Errorf("pathFields(%q) = %v, want %v", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestPlacements_FindsEachParamUnderItsFieldWithGitLabsAnswer verifies the
// map a route's params become: a nested param is left to its parent, a path
// placeholder is found under its input name and is required when every
// spelling carries it even though the record leaves it unrequired, one in an
// optional group keeps the record's answer, a body param keeps its own name
// and answer, and an alias moves a param to the field it names and is
// reported as used.
func TestPlacements_FindsEachParamUnderItsFieldWithGitLabsAnswer(t *testing.T) {
	route := &apilive.Route{
		Method: "POST",
		Path:   apilive.EndpointPrefix + "/projects/:id/(ref/:ref/)things/:name",
		Params: map[string]apilive.Param{
			"id":             {},
			"ref":            {},
			"name":           {Required: true},
			"title":          {Required: true},
			"description":    {},
			"position[base]": {Required: true},
		},
	}
	aliased := map[string]bool{}
	got := placements(route, map[string]string{"name": "thing"}, aliased)
	want := map[string]bool{"project_id": true, "ref": false, "thing": true, "title": true, "description": false}
	if !maps.Equal(got, want) {
		t.Errorf("placements = %v, want %v", got, want)
	}
	if !maps.Equal(aliased, map[string]bool{"name": true}) {
		t.Errorf("aliased = %v, want the one alias the route declares", aliased)
	}
}

// TestPlacements_TwoParamsOnOneField_AreRequiredIfEitherIs verifies a field
// two params of one route are found under is required when either of them is,
// whichever the map hands over first.
func TestPlacements_TwoParamsOnOneField_AreRequiredIfEitherIs(t *testing.T) {
	route := &apilive.Route{
		Method: "GET",
		Path:   apilive.EndpointPrefix + "/projects/:id",
		Params: map[string]apilive.Param{"project_id": {}, "id": {}},
	}
	if got := placements(route, nil, map[string]bool{}); !got["project_id"] {
		t.Errorf("placements = %v, want project_id required by the path", got)
	}
}
