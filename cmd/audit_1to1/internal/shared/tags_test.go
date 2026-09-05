package shared

import (
	"reflect"
	"testing"
)

// TestNormalizeSDKTag_Notations_MapToTheMCPSnakeCaseName verifies the three
// rewrites client-go tag names need before they can meet an MCP json tag:
// the array suffix, bracketed negation, and camelCase from GraphQL structs.
// A tag already in the MCP form must come back unchanged, since the helper
// runs on every fallback lookup.
func TestNormalizeSDKTag_Notations_MapToTheMCPSnakeCaseName(t *testing.T) {
	cases := []struct {
		name string
		tag  string
		want string
	}{
		{name: "array_suffix", tag: "iids[]", want: "iids"},
		{name: "bracket_negation", tag: "not[author_id]", want: "not_author_id"},
		{name: "camel_case", tag: "createdAt", want: "created_at"},
		{name: "leading_capital", tag: "TargetBranch", want: "target_branch"},
		{name: "snake_case_unchanged", tag: "target_branch", want: "target_branch"},
		{name: "empty", tag: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeSDKTag(tc.tag); got != tc.want {
				t.Errorf("NormalizeSDKTag(%q) = %q, want %q", tc.tag, got, tc.want)
			}
		})
	}
}

// TestTagName_Keys_PrefersTheFirstKeyAndStripsOptions verifies tag selection
// across the preferred key order, the stripping of ",omitempty"-style
// suffixes, the "-" exclusion sentinel, and the empty result for a field
// none of the keys names.
func TestTagName_Keys_PrefersTheFirstKeyAndStripsOptions(t *testing.T) {
	cases := []struct {
		name string
		raw  reflect.StructTag
		keys []string
		want string
	}{
		{name: "url_first", raw: `url:"search,omitempty" json:"search_query,omitempty"`, keys: []string{"url", "json"}, want: "search"},
		{name: "json_only", raw: `url:"search,omitempty" json:"search_query,omitempty"`, keys: []string{"json"}, want: "search_query"},
		{name: "dash_sentinel", raw: `json:"-"`, keys: []string{"json"}, want: "-"},
		{name: "no_tag", raw: ``, keys: []string{"json"}, want: ""},
		{name: "empty_name_falls_through", raw: `url:",omitempty" json:"name"`, keys: []string{"url", "json"}, want: "name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TagName(tc.raw, tc.keys); got != tc.want {
				t.Errorf("TagName(%q, %v) = %q, want %q", tc.raw, tc.keys, got, tc.want)
			}
		})
	}
}
