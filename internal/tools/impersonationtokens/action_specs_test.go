// action_specs_test.go contains unit tests for the impersonation token [toolutil.ActionSpec] entries.
package impersonationtokens

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	registerTokenJSON  = `{"id":1,"name":"tok","scopes":["api"],"active":true,"impersonation":true,"revoked":false}`
	registerTokensJSON = `[{"id":1,"name":"tok","scopes":["api"],"active":true,"impersonation":true,"revoked":false}]`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "impersonationtokens" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_RMetaDescriptions validates that every user-token action
// carries a non-generic individual-tool description in the "Returns: … See
// also: …" form and a RelatedActions list (R-META; 1:1 audit).
func TestActionSpecs_RMetaDescriptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, spec := range ActionSpecs(client) {
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: description missing Returns:/See also: form: %q", spec.IndividualTool.Name, desc)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: expected RelatedActions to be set", spec.IndividualTool.Name)
		}
		if spec.Usage == "Use to execute impersonationtokens domain action." {
			t.Errorf("%s: Usage is still the generic placeholder", spec.IndividualTool.Name)
		}
	}
}

// TestActionSpecs_EveryActionCarriesANaturalLanguageAlias holds the discovery
// metadata to replacing the fallback alias list rather than sitting unread
// beside it. The fallback is the individual tool's own name, which
// gitlab_find_action already matches on; what a plain-English query needs is a
// phrase, so an action whose only alias is its identifier is undiscoverable by
// anything a model would actually type.
func TestActionSpecs_EveryActionCarriesANaturalLanguageAlias(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, spec := range ActionSpecs(client) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			phrases := 0
			for _, alias := range spec.Aliases {
				if strings.Contains(alias, " ") {
					phrases++
				}
			}
			if phrases == 0 {
				t.Errorf("Aliases = %q, want at least one multi-word phrase beside the tool name", spec.Aliases)
			}
		})
	}
}

// TestActionSpecs_OnlyTheListActionConstrainsState pins the one input-schema
// override this package declares to the one action whose endpoint takes the
// parameter. GitLab accepts state on the impersonation token listing alone, so
// publishing the enum anywhere else would advertise a filter that endpoint
// ignores, and publishing it nowhere would let a model send a fourth value and
// learn about it from a 400.
func TestActionSpecs_OnlyTheListActionConstrainsState(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, spec := range ActionSpecs(client) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			got := schemaEnum(t, spec.Route.InputSchema, "state")
			want := []string{"all", "active", "inactive"}
			if spec.IndividualTool.Name != "gitlab_list_impersonation_tokens" {
				want = nil
			}
			if !slices.Equal(got, want) {
				t.Errorf("state enum = %q, want %q", got, want)
			}
		})
	}
}

// schemaEnum returns the enum values a JSON Schema publishes for property, or
// nil when the property or its enum is absent.
func schemaEnum(t *testing.T, schema map[string]any, property string) []string {
	t.Helper()
	props, _ := schema["properties"].(map[string]any)
	prop, _ := props[property].(map[string]any)
	raw, _ := prop["enum"].([]any)
	if raw == nil {
		return nil
	}
	values := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Errorf("enum value %v is %T, want a string", v, v)
			continue
		}
		values = append(values, s)
	}
	return values
}

// TestUserTokenOptions_AToolWithNoMetadataEntry_KeepsTheGenericOptions covers
// the miss side of the metadata lookup, which the five registered tools never
// take. A tool added to ActionSpecs and forgotten in userTokenActionMeta must
// still produce a usable spec rather than one built from a zero entry.
func TestUserTokenOptions_AToolWithNoMetadataEntry_KeepsTheGenericOptions(t *testing.T) {
	options := userTokenOptions("gitlab_not_in_the_metadata_table")
	if options.Usage != "Use to execute impersonationtokens domain action." {
		t.Errorf("Usage = %q, want the generic placeholder", options.Usage)
	}
	if !slices.Equal(options.Aliases, []string{"gitlab_not_in_the_metadata_table"}) {
		t.Errorf("Aliases = %q, want only the tool's own name", options.Aliases)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions = %q, want none", options.RelatedActions)
	}
	if options.IndividualTool.Description != "" {
		t.Errorf("Description = %q, want empty", options.IndividualTool.Description)
	}
}

// TestApplyActionMeta_AnEntryThatNamesSomeFields_LeavesTheRestAlone states the
// per-field contract of the four guards in applyActionMeta. Each one is there
// so a half-filled entry degrades to the generic surface: assigning
// unconditionally would blank the usage, drop the tool's own alias and discard
// the related actions the caller already set, and nothing downstream would say
// so, because an empty alias list and an empty related list are both legal.
func TestApplyActionMeta_AnEntryThatNamesSomeFields_LeavesTheRestAlone(t *testing.T) {
	base := func() toolutil.ActionSpecOptions {
		return toolutil.ActionSpecOptions{
			Usage:          "generic usage",
			Aliases:        []string{"gitlab_some_tool"},
			RelatedActions: []string{"impersonationtokens.list_impersonation_tokens"},
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_some_tool", Description: "generic description"},
		}
	}
	cases := []struct {
		name string
		meta userTokenActionMetaEntry
		want toolutil.ActionSpecOptions
	}{
		{
			name: "an entry naming nothing changes nothing",
			meta: userTokenActionMetaEntry{},
			want: base(),
		},
		{
			name: "an entry naming only aliases keeps the other three",
			meta: userTokenActionMetaEntry{aliases: []string{"mint a token"}},
			want: func() toolutil.ActionSpecOptions {
				o := base()
				o.Aliases = []string{"mint a token"}
				return o
			}(),
		},
		{
			name: "an entry naming only related actions keeps the other three",
			meta: userTokenActionMetaEntry{related: []string{"impersonationtokens.revoke_impersonation_token"}},
			want: func() toolutil.ActionSpecOptions {
				o := base()
				o.RelatedActions = []string{"impersonationtokens.revoke_impersonation_token"}
				return o
			}(),
		},
		{
			name: "an entry naming only usage and description keeps the two lists",
			meta: userTokenActionMetaEntry{usage: "mint one", description: "Returns: a token."},
			want: func() toolutil.ActionSpecOptions {
				o := base()
				o.Usage = "mint one"
				o.IndividualTool.Description = "Returns: a token."
				return o
			}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := base()
			applyActionMeta(&got, tc.meta)
			if got.Usage != tc.want.Usage {
				t.Errorf("Usage = %q, want %q", got.Usage, tc.want.Usage)
			}
			if !slices.Equal(got.Aliases, tc.want.Aliases) {
				t.Errorf("Aliases = %q, want %q", got.Aliases, tc.want.Aliases)
			}
			if !slices.Equal(got.RelatedActions, tc.want.RelatedActions) {
				t.Errorf("RelatedActions = %q, want %q", got.RelatedActions, tc.want.RelatedActions)
			}
			if got.IndividualTool.Description != tc.want.IndividualTool.Description {
				t.Errorf("Description = %q, want %q", got.IndividualTool.Description, tc.want.IndividualTool.Description)
			}
		})
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/impersonation_tokens"):
			testutil.RespondJSON(w, http.StatusOK, registerTokensJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/impersonation_tokens/"):
			testutil.RespondJSON(w, http.StatusOK, registerTokenJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/impersonation_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerTokenJSON)
		case r.Method == http.MethodPost && strings.Contains(path, "/personal_access_tokens"):
			testutil.RespondJSON(w, http.StatusCreated, registerTokenJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_impersonation_tokens", map[string]any{"user_id": 1}},
		{"gitlab_get_impersonation_token", map[string]any{"user_id": 1, "token_id": 1}},
		{"gitlab_create_impersonation_token", map[string]any{"user_id": 1, "name": "tok", "scopes": []any{"api"}}},
		{"gitlab_revoke_impersonation_token", map[string]any{"user_id": 1, "token_id": 1}},
		{"gitlab_create_personal_access_token", map[string]any{"user_id": 1, "name": "tok", "scopes": []any{"api"}}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}
