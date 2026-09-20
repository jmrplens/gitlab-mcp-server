// action_specs_test.go contains integration tests for the user email tool
// closures in ActionSpecs routes with a mock GitLab API.
package useremails

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// defaultUserEmailUsage is the usage line every action falls back to when its
// metadata entry names none.
const defaultUserEmailUsage = "Use to execute useremails domain action."

const registerEmailJSON = `{"id":1,"email":"test@example.com","confirmed_at":"2026-01-01T00:00:00Z"}`

// TestActionSpecs_Metadata verifies user email action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "useremails" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestUserEmailOptions_MetadataReachesTheOptions verifies that the discovery
// metadata a tool declares is what its options carry: the usage line served
// into the group description, the aliases find matches on, the canonical
// related IDs a model is pointed at next, and the individual tool's
// description. Nothing asserted any of it, so all four guards in
// userEmailOptions could be inverted with the suite still green while every
// tool fell back to the generic usage and to its own name as its only alias.
func TestUserEmailOptions_MetadataReachesTheOptions(t *testing.T) {
	for tool, meta := range userEmailActionMeta {
		t.Run(tool, func(t *testing.T) {
			options := userEmailOptions(tool)
			if options.Usage != meta.usage {
				t.Errorf("Usage = %q, want %q", options.Usage, meta.usage)
			}
			if !slices.Equal(options.Aliases, meta.aliases) {
				t.Errorf("Aliases = %v, want %v", options.Aliases, meta.aliases)
			}
			if !slices.Equal(options.RelatedActions, meta.related) {
				t.Errorf("RelatedActions = %v, want %v", options.RelatedActions, meta.related)
			}
			if options.IndividualTool.Description != meta.description {
				t.Errorf("Description = %q, want %q", options.IndividualTool.Description, meta.description)
			}
			if options.IndividualTool.Name != tool {
				t.Errorf("IndividualTool.Name = %q, want %q", options.IndividualTool.Name, tool)
			}
		})
	}
}

// TestUserEmailOptions_IncompleteMetadata_KeepsTheDefaults verifies the
// fallback the guards in userEmailOptions exist for: a tool the table does not
// name, and a tool whose entry leaves fields empty, both keep the generic
// usage and the single alias naming the tool itself. Every entry in the table
// fills every field today, so without an entry planted here the guards have no
// false side at all and nothing states what a half-written one would serve.
func TestUserEmailOptions_IncompleteMetadata_KeepsTheDefaults(t *testing.T) {
	const planted = "gitlab_useremails_planted_for_this_test"
	userEmailActionMeta[planted] = userEmailMeta{}
	t.Cleanup(func() { delete(userEmailActionMeta, planted) })

	for _, tool := range []string{"gitlab_absent_from_the_table", planted} {
		t.Run(tool, func(t *testing.T) {
			options := userEmailOptions(tool)
			if options.Usage != defaultUserEmailUsage {
				t.Errorf("Usage = %q, want the default %q", options.Usage, defaultUserEmailUsage)
			}
			if !slices.Equal(options.Aliases, []string{tool}) {
				t.Errorf("Aliases = %v, want the default %v", options.Aliases, []string{tool})
			}
			if len(options.RelatedActions) != 0 {
				t.Errorf("RelatedActions = %v, want none", options.RelatedActions)
			}
			if options.IndividualTool.Description != "" {
				t.Errorf("Description = %q, want empty", options.IndividualTool.Description)
			}
		})
	}
}

// TestActionSpecs_CallRoutes verifies all registered user email routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.HasSuffix(r.URL.Path, "/emails") {
				testutil.RespondJSON(w, http.StatusOK, `[`+registerEmailJSON+`]`)
			} else {
				testutil.RespondJSON(w, http.StatusOK, registerEmailJSON)
			}
		case http.MethodPost:
			testutil.RespondJSON(w, http.StatusCreated, registerEmailJSON)
		case http.MethodDelete:
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
		{"gitlab_list_emails_for_user", map[string]any{"user_id": 1}},
		{"gitlab_get_email", map[string]any{"email_id": 1}},
		{"gitlab_add_email", map[string]any{"email": "new@example.com"}},
		{"gitlab_add_email_for_user", map[string]any{"user_id": 1, "email": "new@example.com"}},
		{"gitlab_delete_email", map[string]any{"email_id": 1}},
		{"gitlab_delete_email_for_user", map[string]any{"user_id": 1, "email_id": 1}},
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
