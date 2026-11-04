// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	got := placements(route, nil, nil, map[string]string{"name": "thing"}, aliased)
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
	if got := placements(route, nil, nil, nil, map[string]bool{}); !got["project_id"] {
		t.Errorf("placements = %v, want project_id required by the path", got)
	}
}

// TestPlacements_TwoRequiredParamsOnOneField_StayRequired verifies a field a
// param placed as required stays required when a second param lands on it.
// The test above cannot fix that order, since the map hands either param over
// first; with both required, the second always meets a field already placed.
func TestPlacements_TwoRequiredParamsOnOneField_StayRequired(t *testing.T) {
	route := &apilive.Route{
		Method: "GET",
		Path:   apilive.EndpointPrefix + "/projects/:id",
		Params: map[string]apilive.Param{"project_id": {Required: true}, "id": {}},
	}
	got := placements(route, nil, nil, nil, map[string]bool{})
	if !maps.Equal(got, map[string]bool{"project_id": true}) {
		t.Errorf("placements = %v, want project_id required once", got)
	}
}

// TestPlacements_APlaceholderTheCallsFill_IsFoundUnderTheirField verifies a
// path parameter the request record names a filling field for is found under
// that field: the deploy key of /projects/:id/deploy_keys/:key_id is the
// input's deploy_key_id, which no rule of names finds and the record without
// its placeholders leaves under key_id, a field no input publishes.
func TestPlacements_APlaceholderTheCallsFill_IsFoundUnderTheirField(t *testing.T) {
	route := &apilive.Route{
		Method: "GET",
		Path:   apilive.EndpointPrefix + "/projects/:id/deploy_keys/:key_id",
		Params: map[string]apilive.Param{"id": {}, "key_id": {Required: true}},
	}
	before := placements(route, nil, nil, nil, map[string]bool{})
	if !maps.Equal(before, map[string]bool{"project_id": true, "key_id": true}) {
		t.Errorf("by name alone = %v, want key_id under its own name", before)
	}
	filled := map[string]string{"id": "project_id", "key_id": "deploy_key_id"}
	after := placements(route, filled, nil, nil, map[string]bool{})
	if !maps.Equal(after, map[string]bool{"project_id": true, "deploy_key_id": true}) {
		t.Errorf("through the record's placeholders = %v, want the deploy key under deploy_key_id", after)
	}
}

// TestPlacements_TwoPlaceholdersNamedForOneField_AreToldApart verifies the
// collision of DELETE /groups/:id/share/:group_id: by names, the bare :id is
// the group_id the collection names and :group_id is group_id too, so the
// group shared with landed on the group that shares and its own field was
// never judged; through the fields the calls fill each from, the two land
// apart.
func TestPlacements_TwoPlaceholdersNamedForOneField_AreToldApart(t *testing.T) {
	route := &apilive.Route{
		Method: "DELETE",
		Path:   apilive.EndpointPrefix + "/groups/:id/share/:group_id",
		Params: map[string]apilive.Param{"id": {}, "group_id": {}},
	}
	if got := placements(route, nil, nil, nil, map[string]bool{}); !maps.Equal(got, map[string]bool{"group_id": true}) {
		t.Errorf("by name alone = %v, want both on group_id", got)
	}
	filled := map[string]string{"id": "group_id", "group_id": "share_group_id"}
	got := placements(route, filled, nil, nil, map[string]bool{})
	if !maps.Equal(got, map[string]bool{"group_id": true, "share_group_id": true}) {
		t.Errorf("through the record's placeholders = %v, want each on its own field", got)
	}
}

// TestPlacements_ABodyParamNamedLikeAFillingField_IsNotThatField verifies a
// param found by name under a field a placeholder is filled from is left
// out, since one field of the input fills one parameter: the body group_id
// of POST /groups/:id/share is the group shared with, and before the rule its
// requirement landed on the input's group_id, the group the path names. The
// path's group is written in an optional group here so the two answers
// differ: by name, the body's requirement made group_id required; with the
// rule, group_id keeps the path's answer.
func TestPlacements_ABodyParamNamedLikeAFillingField_IsNotThatField(t *testing.T) {
	route := &apilive.Route{
		Method: "POST",
		Path:   apilive.EndpointPrefix + "/groups/(:id/)share",
		Params: map[string]apilive.Param{"id": {}, "group_id": {Required: true}, "group_access": {Required: true}},
	}
	if got := placements(route, nil, nil, nil, map[string]bool{}); !maps.Equal(got, map[string]bool{"group_id": true, "group_access": true}) {
		t.Errorf("by name alone = %v, want the body's requirement on group_id", got)
	}
	got := placements(route, map[string]string{"id": "group_id"}, nil, nil, map[string]bool{})
	if !maps.Equal(got, map[string]bool{"group_id": false, "group_access": true}) {
		t.Errorf("through the record's placeholders = %v, want group_id with the path's answer alone", got)
	}
}

// TestPlacements_TheSharedGroupOfAGroupShare_IsFoundUnderItsAlias is the
// planted defect of #1688 (P8) on POST /groups/:id/share: by name alone the
// body's group_id is the input's group_id and the field naming the group
// shared with is never judged; through the record's placeholders and the alias
// the body's requirement lands on that field, and the path's on group_id.
func TestPlacements_TheSharedGroupOfAGroupShare_IsFoundUnderItsAlias(t *testing.T) {
	route := &apilive.Route{
		Method: "POST",
		Path:   apilive.EndpointPrefix + "/groups/:id/share",
		Params: map[string]apilive.Param{"id": {Required: true}, "group_id": {Required: true}, "expires_at": {}},
	}
	before := placements(route, nil, nil, nil, map[string]bool{})
	if !maps.Equal(before, map[string]bool{"group_id": true, "expires_at": false}) {
		t.Errorf("by name alone = %v, want both on group_id and no share_group_id", before)
	}
	aliased := map[string]bool{}
	after := placements(route, map[string]string{"id": "group_id"}, nil, map[string]string{"group_id": "share_group_id"}, aliased)
	if !maps.Equal(after, map[string]bool{"group_id": true, "share_group_id": true, "expires_at": false}) || !aliased["group_id"] {
		t.Errorf("through the placeholders and the alias = %v, aliased %v; want each on its own field and the alias used", after, aliased)
	}
}

// TestPlacements_AFilledPlaceholder_WinsOverItsAlias verifies the record's
// placeholders decide before a declared alias: an alias for a parameter the
// calls are read to fill is not used, which is how the alias table tells an
// entry the record has made unnecessary.
func TestPlacements_AFilledPlaceholder_WinsOverItsAlias(t *testing.T) {
	route := &apilive.Route{
		Method: "PATCH",
		Path:   apilive.EndpointPrefix + "/projects/:id/protected_branches/:name",
		Params: map[string]apilive.Param{"id": {}, "name": {Required: true}},
	}
	aliased := map[string]bool{}
	got := placements(route, map[string]string{"name": "branch_name"}, nil, map[string]string{"name": "branch"}, aliased)
	if !maps.Equal(got, map[string]bool{"project_id": true, "branch_name": true}) || len(aliased) != 0 {
		t.Errorf("placements = %v, aliased %v; want branch_name from the record and the alias unused", got, aliased)
	}
}

// TestPlacements_AWorkhorseBody_PlacesTheParamsItCarries verifies the params
// a declaration says GitLab reads out of a body Workhorse rewrote are placed
// beside the record's own, each with its answer: on the commit route the
// record declares the file and the project alone.
func TestPlacements_AWorkhorseBody_PlacesTheParamsItCarries(t *testing.T) {
	route := &apilive.Route{
		Method: "POST",
		Path:   apilive.EndpointPrefix + "/projects/:id/repository/commits",
		Params: map[string]apilive.Param{"id": {}, "file": {Required: true, Type: apilive.WorkhorseFile}},
	}
	body := map[string]bool{"branch": true, "commit_message": true, "stats": false}
	got := placements(route, nil, body, nil, map[string]bool{})
	want := map[string]bool{"project_id": true, "file": true, "branch": true, "commit_message": true, "stats": false}
	if !maps.Equal(got, want) {
		t.Errorf("placements = %v, want %v", got, want)
	}
}
