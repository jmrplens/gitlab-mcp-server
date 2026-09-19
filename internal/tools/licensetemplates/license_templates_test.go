// license_templates_test.go contains unit tests for the license template MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package licensetemplates

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestList verifies List.
func TestList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/licenses")
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"mit","name":"MIT License","featured":true,"popular":true}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Licenses) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Licenses))
	}
	if !out.Licenses[0].Popular {
		t.Error("Popular = false, want the popular template the answer describes")
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet verifies Get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/templates/licenses/mit")
		testutil.RespondJSON(w, http.StatusOK, `{"key":"mit","name":"MIT License","content":"MIT License\n\nCopyright...","permissions":["commercial-use"],"conditions":["include-copyright"],"limitations":["no-liability"]}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{Key: "mit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "MIT License" {
		t.Errorf("Name = %q", out.Name)
	}
	if len(out.Permissions) != 1 {
		t.Errorf("Permissions len = %d", len(out.Permissions))
	}
}

// TestGet_EmptyKey verifies that Get returns a validation error when the key is empty.
func TestGet_EmptyKey(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(t.Context(), client, GetInput{Key: ""})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "key is required") {
		t.Errorf("error = %q, want mention of key", err.Error())
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(t.Context(), client, GetInput{Key: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatListMarkdown verifies the whole list render: the heading counts
// what the page shows, the popular column is the flag glyph rather than the
// word "true", and one hint closes the page.
func TestFormatListMarkdown(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Licenses: []LicenseItem{{Key: "mit", Name: "MIT", Popular: true}}})
	want := "## License Templates (1)\n\n" +
		"| Key | Name | Popular |\n| --- | --- | --- |\n" +
		"| mit | MIT | " + toolutil.EmojiSuccess + " |\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Use `gitlab_get_license_template` to view a specific template\n"
	if md != want {
		t.Errorf("license list:\n got %q\nwant %q", md, want)
	}
}

// TestFormatGetMarkdown verifies the whole card of one license template,
// including the key row the formatter used to drop.
func TestFormatGetMarkdown(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Name: "MIT", Key: "mit", Content: "text", Permissions: []string{"use"}})
	want := "## License: MIT\n\n" +
		"- **Key**: mit\n" +
		"- **Permissions**: use\n" +
		"\n```\ntext\n```\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Copy this template to your LICENSE file and customize it\n"
	if md != want {
		t.Errorf("license card:\n got %q\nwant %q", md, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies that an empty page is the one sentence
// and nothing else: no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Licenses: nil})
	if want := "No license templates found.\n"; md != want {
		t.Errorf("empty license list:\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — all optional fields populated
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_AllFields verifies FormatGetMarkdown when all fields.
func TestFormatGetMarkdown_AllFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Name:        "Apache 2.0",
		Description: "A permissive license",
		Permissions: []string{"commercial-use", "modification"},
		Conditions:  []string{"include-copyright", "document-changes"},
		Limitations: []string{"no-liability", "no-warranty"},
		Content:     "Apache License text here",
	})
	want := "## License: Apache 2.0\n\n" +
		"- **Description**: A permissive license\n" +
		"- **Permissions**: commercial-use, modification\n" +
		"- **Conditions**: include-copyright, document-changes\n" +
		"- **Limitations**: no-liability, no-warranty\n" +
		"\n```\nApache License text here\n```\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Copy this template to your LICENSE file and customize it\n"
	if md != want {
		t.Errorf("license card:\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown — minimal fields (no description, no conditions, no content)
// ---------------------------------------------------------------------------.

// TestFormatGetMarkdown_MinimalFields verifies that a template GitLab answered
// with nothing but a name renders the heading and the hints and no absent
// value at all: no empty description row and no empty fence.
func TestFormatGetMarkdown_MinimalFields(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{
		Name: "Minimal",
	})
	want := "## License: Minimal\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Copy this template to your LICENSE file and customize it\n"
	if md != want {
		t.Errorf("minimal license card:\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// List — API error 400
// ---------------------------------------------------------------------------.

// TestList_APIError400 verifies List when API error 400.
func TestList_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Get — API error 400
// ---------------------------------------------------------------------------.

// TestGet_APIError400 verifies Get when API error 400.
func TestGet_APIError400(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Get(context.Background(), client, GetInput{Key: "bad"})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// List — with Popular filter
// ---------------------------------------------------------------------------.

// TestList_WithPopularFilter verifies List when with popular filter.
func TestList_WithPopularFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("popular") != "true" {
			t.Errorf("expected popular=true, got %s", r.URL.Query().Get("popular"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"mit","name":"MIT License","featured":true}]`)
	}))
	pop := true
	out, err := List(context.Background(), client, ListInput{Popular: &pop})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Licenses) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Licenses))
	}
}

// ---------------------------------------------------------------------------
// List — with pagination
// ---------------------------------------------------------------------------.

// TestList_WithPagination verifies List when with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "10" {
			t.Errorf("expected page=2&per_page=10, got %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"apache-2.0","name":"Apache License 2.0"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{Page: 2, PerPage: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Licenses) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Licenses))
	}
}

// ---------------------------------------------------------------------------
// List — with keyset pagination, order_by, and sort
// ---------------------------------------------------------------------------.

// TestList_WithKeysetAndOrdering verifies that List forwards the keyset
// pagination cursor, pagination method, order_by, and sort query parameters.
func TestList_WithKeysetAndOrdering(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("pagination") != "keyset" {
			t.Errorf("expected pagination=keyset, got %s", q.Get("pagination"))
		}
		if q.Get("page_token") != "tok-123" {
			t.Errorf("expected page_token=tok-123, got %s", q.Get("page_token"))
		}
		if q.Get("order_by") != "name" {
			t.Errorf("expected order_by=name, got %s", q.Get("order_by"))
		}
		if q.Get("sort") != "asc" {
			t.Errorf("expected sort=asc, got %s", q.Get("sort"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"mit","name":"MIT License"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{
		OrderBy:    "name",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "tok-123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Licenses) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Licenses))
	}
}

// ---------------------------------------------------------------------------
// Get — with optional Project and Fullname fields
// ---------------------------------------------------------------------------.

// TestGet_WithOptionalFields verifies Get when with optional fields.
func TestGet_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("project") != "my-project" {
			t.Errorf("expected project=my-project, got %s", r.URL.Query().Get("project"))
		}
		if r.URL.Query().Get("fullname") != "John Doe" {
			t.Errorf("expected fullname=John Doe, got %s", r.URL.Query().Get("fullname"))
		}
		testutil.RespondJSON(w, http.StatusOK, `{"key":"mit","name":"MIT License","content":"MIT License\nCopyright (c) John Doe"}`)
	}))
	proj := "my-project"
	fullname := "John Doe"
	out, err := Get(context.Background(), client, GetInput{Key: "mit", Project: &proj, Fullname: &fullname})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "MIT License" {
		t.Errorf("Name = %q, want MIT License", out.Name)
	}
}

// TestActionSpecs_Metadata verifies license template action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "licensetemplates" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", spec.Name)
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Fatalf("IndividualTool.Description for %s should follow Returns:/See also: form, got %q", spec.Name, desc)
		}
	}
	if specByTool["gitlab_get_license_template"].ParameterGuidance["key"].SemanticRole == "" {
		t.Fatal("gitlab_get_license_template should define key parameter guidance")
	}
	if specByTool["gitlab_list_license_templates"].ParameterGuidance["order_by"].SemanticRole == "" {
		t.Fatal("gitlab_list_license_templates should define order_by parameter guidance")
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates license template canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := newLicenseRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_license_templates", map[string]any{}},
		{"get", "gitlab_get_license_template", map[string]any{"key": "mit"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution error paths
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteErrors validates license template route errors.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/licenses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})
	handler.HandleFunc("GET /api/v4/templates/licenses/bad", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	})

	client := testutil.NewTestClient(t, handler)
	specByTool := licenseTemplateSpecsByTool(ActionSpecs(client))

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list_error", "gitlab_list_license_templates", map[string]any{}},
		{"get_error", "gitlab_get_license_template", map[string]any{"key": "bad"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — unpopular license
// ---------------------------------------------------------------------------

// TestFormatListMarkdown_UnpopularLicense verifies that a template GitLab does
// not list among the popular ones renders the cross glyph, not the word
// "false", which is what the column showed before the flag went through
// BoolEmoji.
func TestFormatListMarkdown_UnpopularLicense(t *testing.T) {
	md := FormatListMarkdown(ListOutput{Licenses: []LicenseItem{
		{Key: "gpl-3.0", Name: "GPL 3.0", Popular: false},
	}})
	want := "## License Templates (1)\n\n" +
		"| Key | Name | Popular |\n| --- | --- | --- |\n" +
		"| gpl-3.0 | GPL 3.0 | " + toolutil.EmojiCross + " |\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n- Use `gitlab_get_license_template` to view a specific template\n"
	if md != want {
		t.Errorf("license list:\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// Helper: route specs factory
// ---------------------------------------------------------------------------.

// newLicenseRouteSpecs constructs license route specs test fixtures.
func newLicenseRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	licenseJSON := `{"key":"mit","name":"MIT License","featured":true,"description":"A short license","permissions":["commercial-use"],"conditions":["include-copyright"],"limitations":["no-liability"],"content":"MIT License text"}`

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/templates/licenses", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+licenseJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/templates/licenses/mit", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, licenseJSON)
	})

	client := testutil.NewTestClient(t, handler)
	return licenseTemplateSpecsByTool(ActionSpecs(client))
}

// TestLicenseTemplates_UnreadablePopular verifies that both license template
// handlers return an error rather than a half-filled template when GitLab
// sends popular as something that is not a boolean. client-go models popular
// on its own LicenseTemplate as of v3.12.0, so the SDK's decoder is what
// refuses it now that the captured read is retired.
func TestLicenseTemplates_UnreadablePopular(t *testing.T) {
	// A list answers with an array and a get with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertUnreadableBodyRefused(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"key":"mit","name":"MIT License","popular":"yes"}]`)
			_, err := List(context.Background(), client, ListInput{})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"key":"mit","name":"MIT License","popular":"yes"}`)
			_, err := Get(context.Background(), client, GetInput{Key: "mit"})
			return err
		}},
	})
}

// ---------------------------------------------------------------------------
// The popular flag is GitLab's own key, per template, in order
// ---------------------------------------------------------------------------.

// TestList_PopularFollowsGitLabsOwnKeyPerTemplate verifies that each published
// template's Popular flag is the `popular` key GitLab sent for that same
// template, and neither the `featured` key the SDK decodes beside it nor
// another template's answer.
//
// Both halves are why the handler reads the captured response at all
// (ADR-0021). client-go's LicenseTemplate models `featured` and not `popular`,
// so a conversion taking l.Featured compiles, publishes a plausible boolean,
// and silently answers a different question than the field name promises. And
// the extras come back as a list indexed in step with the decoded templates, so
// this fixture makes the two templates disagree on both keys: a flag read off
// the wrong template is then a wrong answer rather than the right one by luck,
// which a single-template fixture can never tell apart.
func TestList_PopularFollowsGitLabsOwnKeyPerTemplate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+
			`{"key":"mit","name":"MIT License","featured":true,"popular":false},`+
			`{"key":"gpl-3.0","name":"GPL 3.0","featured":false,"popular":true}`+
			`]`)
	}))
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Licenses) != 2 {
		t.Fatalf("len(Licenses) = %d, want 2", len(out.Licenses))
	}
	cases := []struct {
		name    string
		index   int
		key     string
		popular bool
	}{
		{"featured but not popular", 0, "mit", false},
		{"popular but not featured", 1, "gpl-3.0", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := out.Licenses[tt.index]
			if got.Key != tt.key {
				t.Fatalf("Licenses[%d].Key = %q, want %q: templates are published in the order GitLab sent them", tt.index, got.Key, tt.key)
			}
			if got.Popular != tt.popular {
				t.Errorf("%s: Popular = %v, want %v", got.Key, got.Popular, tt.popular)
			}
		})
	}
}

// TestGet_PopularFollowsGitLabsOwnKey verifies that the single-template read
// publishes the `popular` key GitLab sent rather than the `featured` one the
// SDK decodes. The fixture makes the two disagree, so a read that took the
// SDK's field, or that dropped the captured extra on the floor and passed a
// zero one, answers false where GitLab said true.
func TestGet_PopularFollowsGitLabsOwnKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"mit","name":"MIT License","featured":false,"popular":true}`)
	}))
	out, err := Get(t.Context(), client, GetInput{Key: "mit"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Popular {
		t.Error("Popular = false, want the popular template GitLab's own key describes")
	}
}

// ---------------------------------------------------------------------------
// Every field of a template reaches the caller under its own name
// ---------------------------------------------------------------------------.

// TestGet_PublishesEveryFieldGitLabSent verifies that each field of the
// published template carries the value GitLab sent under that same name.
//
// Every one of these is a plain copy, so transposing two of them changes no
// control flow and nothing downstream complains. The html_url and source_url
// pair is the case worth naming: both are URLs of the same template, a reader
// handed them the wrong way round has no way to notice, and until this test
// neither was asserted anywhere in the package at all.
func TestGet_PublishesEveryFieldGitLabSent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{`+
			`"key":"mit","name":"MIT License","nickname":"MIT",`+
			`"html_url":"https://gitlab.example.com/licenses/mit",`+
			`"source_url":"https://opensource.org/licenses/MIT",`+
			`"description":"A short permissive license",`+
			`"conditions":["include-copyright"],`+
			`"permissions":["commercial-use","modification"],`+
			`"limitations":["no-liability"],`+
			`"content":"MIT License\n\nCopyright (c) 2026 Jane Doe"`+
			`}`)
	}))
	out, err := Get(t.Context(), client, GetInput{Key: "mit"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	scalars := []struct {
		field string
		got   string
		want  string
	}{
		{"Key", out.Key, "mit"},
		{"Name", out.Name, "MIT License"},
		{"Nickname", out.Nickname, "MIT"},
		{"HTMLURL", out.HTMLURL, "https://gitlab.example.com/licenses/mit"},
		{"SourceURL", out.SourceURL, "https://opensource.org/licenses/MIT"},
		{"Description", out.Description, "A short permissive license"},
		{"Content", out.Content, "MIT License\n\nCopyright (c) 2026 Jane Doe"},
	}
	for _, tt := range scalars {
		t.Run(tt.field, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.field, tt.got, tt.want)
			}
		})
	}
	lists := []struct {
		field string
		got   []string
		want  []string
	}{
		{"Conditions", out.Conditions, []string{"include-copyright"}},
		{"Permissions", out.Permissions, []string{"commercial-use", "modification"}},
		{"Limitations", out.Limitations, []string{"no-liability"}},
	}
	for _, tt := range lists {
		t.Run(tt.field, func(t *testing.T) {
			if !slices.Equal(tt.got, tt.want) {
				t.Errorf("%s = %q, want %q", tt.field, tt.got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The pagination block describes the listing that carried it
// ---------------------------------------------------------------------------.

// TestList_PaginationDescribesTheListResponse verifies that the pagination
// block a listing publishes is built from the headers GitLab put on that very
// response, rather than left at its zero value.
//
// This is the one part of the output nothing else in the package depends on,
// which is exactly why it can go missing unnoticed: the licenses themselves are
// correct either way. A caller reads this block to decide whether to ask for
// another page, so a zeroed one makes every page look like the last and puts
// the remaining templates out of reach. The whole struct is compared rather
// than a field or two, so a block filled from the wrong place cannot pass by
// agreeing on the fields somebody happened to check.
func TestList_PaginationDescribesTheListResponse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"key":"mit","name":"MIT License"}]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1"},
		)
	}))
	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := toolutil.PaginationOutput{Page: 2, PerPage: 1, TotalItems: 3, TotalPages: 3, NextPage: 3, PrevPage: 1, HasMore: true}
	if out.Pagination != want {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, want)
	}
}

// ---------------------------------------------------------------------------
// Each hint is attached to the one refusal it explains
// ---------------------------------------------------------------------------.

// TestList_ForbiddenSuggestsTheScopeAndOtherFailuresDoNot verifies that a
// listing GitLab refuses carries the scope hint, and that a failure of another
// kind does not.
//
// Both halves are the property. A hint offered on every failure sends a caller
// to widen a token that was never the problem; a hint wired to the wrong status
// is never offered when it would have helped. The status the handler named is
// invisible from outside, so the suggestion it produces is the only evidence of
// which one it is — asserting merely that the call failed, as this package did
// before, holds neither half.
func TestList_ForbiddenSuggestsTheScopeAndOtherFailuresDoNot(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{"forbidden", http.StatusForbidden, true},
		{"bad request", http.StatusBadRequest, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			_, err := List(t.Context(), client, ListInput{})
			if err == nil {
				t.Fatalf("status %d: expected an error", tt.status)
			}
			got := strings.Contains(err.Error(), "Suggestion: verify your token has read_api scope")
			if got != tt.wantHint {
				t.Errorf("status %d: read_api suggestion present = %v, want %v; error was %q", tt.status, got, tt.wantHint, err.Error())
			}
		})
	}
}

// TestGet_NotFoundSuggestsTheListAndOtherFailuresDoNot verifies the same of the
// single-template read: the hint pointing a caller back at the listing belongs
// to the missing-key refusal that it answers, and to no other failure. A key
// that does not exist is the one thing the listing can fix.
func TestGet_NotFoundSuggestsTheListAndOtherFailuresDoNot(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{"not found", http.StatusNotFound, true},
		{"bad request", http.StatusBadRequest, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			_, err := Get(t.Context(), client, GetInput{Key: "nope"})
			if err == nil {
				t.Fatalf("status %d: expected an error", tt.status)
			}
			got := strings.Contains(err.Error(), "Suggestion: verify key with gitlab_list_license_templates")
			if got != tt.wantHint {
				t.Errorf("status %d: list suggestion present = %v, want %v; error was %q", tt.status, got, tt.wantHint, err.Error())
			}
		})
	}
}

// licenseTemplateSpecsByTool supports license template specs by tool assertions in licensetemplates tests.
func licenseTemplateSpecsByTool(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}
