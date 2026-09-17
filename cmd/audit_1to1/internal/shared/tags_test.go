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
//
// Two cases pin the ends of the letter range the word break is decided by.
// "timeZone" is the last letter of the alphabet as a word start, so a rule
// written `r < 'Z'` would leave the tag camelCase and match no MCP name; and
// "sha256" is the other end, where a run of characters below 'A' has to pass
// through untouched rather than be treated as the start of a word.
func TestNormalizeSDKTag_Notations_MapToTheMCPSnakeCaseName(t *testing.T) {
	cases := []struct {
		name string
		tag  string
		want string
	}{
		{name: "array_suffix", tag: "iids[]", want: "iids"},
		{name: "bracket_negation", tag: "not[author_id]", want: "not_author_id"},
		{name: "camel_case", tag: "createdAt", want: "created_at"},
		{name: "camel_case_capital_z", tag: "timeZone", want: "time_zone"},
		{name: "leading_capital", tag: "TargetBranch", want: "target_branch"},
		{name: "snake_case_unchanged", tag: "target_branch", want: "target_branch"},
		{name: "digits_start_no_word", tag: "sha256", want: "sha256"},
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
// "AssigneeIDx" is that rule's other side, and shows how narrow the exemption
// is: only a final "s" holds an acronym run together, so the same identifier
// with any other trailing letter breaks and comes back assignee_i_dx.
//
// The rest of the table pins the four character ranges the word break is
// decided by, each at the boundary value a comparison written one notch wrong
// would drop: "TimeZone" for the last capital, "AreaID" and "QuizID" for the
// two ends of the lowercase run, and "Level0Name" and "P99Latency" for the two
// ends of the digits. A character in none of those ranges is a word character
// for neither side, which is what "Foo_Bar", "Foo-Bar" and "CaféURL" state: an
// identifier that already separates its words keeps exactly the separator it
// has, and no underscore is added beside it.
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
		{name: "trailing_letter_that_is_not_the_plural_s", field: "AssigneeIDx", want: "assignee_i_dx"},
		{name: "acronym_then_word", field: "CRMContactIDs", want: "crm_contact_ids"},
		{name: "acronym_alone", field: "ID", want: "id"},
		{name: "plural_acronym_alone", field: "IDs", want: "ids"},
		{name: "leading_acronym_word", field: "HTTPServer", want: "http_server"},
		{name: "single_word", field: "Name", want: "name"},
		{name: "capital_z_starts_a_word", field: "TimeZone", want: "time_zone"},
		{name: "lowercase_a_ends_a_word", field: "AreaID", want: "area_id"},
		{name: "lowercase_z_ends_a_word", field: "QuizID", want: "quiz_id"},
		{name: "digit_boundary", field: "Sha256Sum", want: "sha256_sum"},
		{name: "digit_zero_ends_a_word", field: "Level0Name", want: "level0_name"},
		{name: "digit_nine_ends_a_word", field: "P99Latency", want: "p99_latency"},
		{name: "lowercase_start_then_capital", field: "aURL", want: "a_url"},
		{name: "underscore_already_separates", field: "Foo_Bar", want: "foo_bar"},
		{name: "punctuation_already_separates", field: "Foo-Bar", want: "foo-bar"},
		{name: "non_ascii_letter_separates_nothing", field: "CaféURL", want: "caféurl"},
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
