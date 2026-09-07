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

// TestFieldNameTag_GoIdentifiers_BecomeTheNameEncodingJSONWouldUse verifies the
// conversion the struct comparison falls back to when an SDK struct tags
// nothing, which is every GraphQL-backed type client-go declares.
//
// The case that decides the algorithm is the plural acronym. The usual rule,
// breaking before the last capital of a run followed by a lowercase letter, is
// right for CRMContact and renders AssigneeIDs as assignee_i_ds, which matches
// no tag anybody writes and would report a field we do have as one we lack.
func TestFieldNameTag_GoIdentifiers_BecomeTheNameEncodingJSONWouldUse(t *testing.T) {
	cases := []struct {
		name  string
		field string
		want  string
	}{
		{name: "plain_camel", field: "CreatedAt", want: "created_at"},
		{name: "trailing_acronym", field: "NamespaceID", want: "namespace_id"},
		{name: "trailing_acronym_three", field: "AvatarURL", want: "avatar_url"},
		{name: "plural_acronym", field: "AssigneeIDs", want: "assignee_ids"},
		{name: "acronym_then_word", field: "CRMContactIDs", want: "crm_contact_ids"},
		{name: "acronym_alone", field: "ID", want: "id"},
		{name: "plural_acronym_alone", field: "IDs", want: "ids"},
		{name: "leading_acronym_word", field: "HTTPServer", want: "http_server"},
		{name: "single_word", field: "Name", want: "name"},
		{name: "digit_boundary", field: "Sha256Sum", want: "sha256_sum"},
		{name: "already_lower", field: "name", want: "name"},
		{name: "empty", field: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FieldNameTag(tc.field); got != tc.want {
				t.Errorf("FieldNameTag(%q) = %q, want %q", tc.field, got, tc.want)
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
