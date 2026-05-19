// cilint_test.go contains unit tests for the CI lint MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package cilint

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// CI Lint Project
// ---------------------------------------------------------------------------.

// TestCILintProject_Success verifies CILintProject when success.
func TestCILintProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/ci/lint" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"valid":true,
				"errors":[],
				"warnings":["warning1"],
				"merged_yaml":"stages:\n  - build",
				"includes":[{"type":"local","location":".gitlab-ci.yml","context_project":"my/project"}]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := LintProject(context.Background(), client, ProjectInput{
		ProjectID: "123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Valid {
		t.Error("expected valid=true")
	}
	if len(out.Warnings) != 1 || out.Warnings[0] != "warning1" {
		t.Errorf("warnings = %v, want [warning1]", out.Warnings)
	}
	if out.MergedYaml != "stages:\n  - build" {
		t.Errorf("merged_yaml = %q, unexpected", out.MergedYaml)
	}
	if len(out.Includes) != 1 || out.Includes[0].Type != "local" {
		t.Errorf("includes = %v, want 1 local include", out.Includes)
	}
}

// TestCILintProject_Invalid verifies CILintProject when invalid.
func TestCILintProject_Invalid(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/ci/lint" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"valid":false,
				"errors":["syntax error","unknown key: foo"],
				"warnings":[],
				"merged_yaml":"",
				"includes":[]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := LintProject(context.Background(), client, ProjectInput{
		ProjectID: "123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Valid {
		t.Error("expected valid=false")
	}
	if len(out.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(out.Errors))
	}
}

// TestCILintProject_WithOptions verifies CILintProject when with options.
func TestCILintProject_WithOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/ci/lint" && r.Method == http.MethodGet {
			if r.URL.Query().Get("content_ref") != "main" {
				t.Errorf("expected content_ref=main, got %s", r.URL.Query().Get("content_ref"))
			}
			if r.URL.Query().Get("include_jobs") != "true" {
				t.Errorf("expected include_jobs=true, got %s", r.URL.Query().Get("include_jobs"))
			}
			testutil.RespondJSON(w, http.StatusOK, `{"valid":true,"errors":[],"warnings":[],"merged_yaml":"","includes":[]}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	dryRun := false
	includeJobs := true
	_, err := LintProject(context.Background(), client, ProjectInput{
		ProjectID:   "123",
		ContentRef:  "main",
		DryRun:      &dryRun,
		IncludeJobs: &includeJobs,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCILintProject_MissingProjectID verifies CILintProject when missing project ID.
func TestCILintProject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	_, err := LintProject(context.Background(), client, ProjectInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestCILintProject_CancelledContext verifies CILintProject when cancelled context.
func TestCILintProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := LintProject(ctx, client, ProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// ---------------------------------------------------------------------------
// CI Lint (Namespace)
// ---------------------------------------------------------------------------.

// TestCILint_Success verifies CILint when success.
func TestCILint_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/ci/lint" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{
				"valid":true,
				"errors":[],
				"warnings":[],
				"merged_yaml":"stages:\n  - test\njob1:\n  script: echo",
				"includes":[]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := LintContent(context.Background(), client, ContentInput{
		ProjectID: "123",
		Content:   "stages:\n  - test\njob1:\n  script: echo",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Valid {
		t.Errorf("expected valid=true, got errors: %v", out.Errors)
	}
}

// TestCILint_InvalidYAML verifies CILint when invalid YAML.
func TestCILint_InvalidYAML(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/ci/lint" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{
				"valid":false,
				"errors":["Invalid configuration format"],
				"warnings":[],
				"merged_yaml":"",
				"includes":[]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := LintContent(context.Background(), client, ContentInput{
		ProjectID: "123",
		Content:   "not valid yaml ---",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Valid {
		t.Error("expected valid=false")
	}
	if len(out.Errors) == 0 {
		t.Error("expected at least 1 error")
	}
}

// TestCILint_MissingProjectID verifies CILint when missing project ID.
func TestCILint_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	_, err := LintContent(context.Background(), client, ContentInput{Content: "stages: [build]"})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestCILint_MissingContent verifies CILint when missing content.
func TestCILint_MissingContent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	_, err := LintContent(context.Background(), client, ContentInput{ProjectID: "123"})
	if err == nil {
		t.Fatal("expected error for missing content")
	}
}

// TestCILint_EmptyContent verifies CILint when empty content.
func TestCILint_EmptyContent(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	_, err := LintContent(context.Background(), client, ContentInput{ProjectID: "123", Content: "   "})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

// TestCILint_CancelledContext verifies CILint when cancelled context.
func TestCILint_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := LintContent(ctx, client, ContentInput{ProjectID: "1", Content: "stages: []"})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	// mdHeadingWarnings identifies the md heading warnings constant used by this package.
	mdHeadingWarnings = "### Warnings"
	// mdHeadingIncludes identifies the md heading includes constant used by this package.
	mdHeadingIncludes = "### Includes"
	// mdHeadingMergedYAML identifies the md heading merged YAML constant used by this package.
	mdHeadingMergedYAML = "### Merged YAML"
)

// ---------------------------------------------------------------------------
// LintProject — API error
// ---------------------------------------------------------------------------.

// TestCILintProject_APIError verifies CILintProject when API error.
func TestCILintProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := LintProject(context.Background(), client, ProjectInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// LintProject — all optional fields (DryRunRef, Ref)
// ---------------------------------------------------------------------------.

// TestCILintProject_AllOptionalFields verifies CILintProject when all optional fields.
func TestCILintProject_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/ci/lint" && r.Method == http.MethodGet {
			testutil.AssertQueryParam(t, r, "content_ref", "develop")
			testutil.AssertQueryParam(t, r, "dry_run", "true")
			testutil.AssertQueryParam(t, r, "dry_run_ref", "staging")
			testutil.AssertQueryParam(t, r, "include_jobs", "true")
			testutil.AssertQueryParam(t, r, "ref", "v1.0")
			testutil.RespondJSON(w, http.StatusOK, `{
				"valid":true,
				"errors":[],
				"warnings":[],
				"merged_yaml":"stages:\n  - build",
				"includes":[{"type":"remote","location":"https://example.com/ci.yml","context_project":""}]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	dryRun := true
	includeJobs := true
	out, err := LintProject(context.Background(), client, ProjectInput{
		ProjectID:   "42",
		ContentRef:  "develop",
		DryRun:      &dryRun,
		DryRunRef:   "staging",
		IncludeJobs: &includeJobs,
		Ref:         "v1.0",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Valid {
		t.Error("expected valid=true")
	}
	if len(out.Includes) != 1 || out.Includes[0].Type != "remote" {
		t.Errorf("includes = %v, want 1 remote include", out.Includes)
	}
}

// ---------------------------------------------------------------------------
// LintContent — API error
// ---------------------------------------------------------------------------.

// TestCILintContent_APIError verifies CILintContent when API error.
func TestCILintContent_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))
	_, err := LintContent(context.Background(), client, ContentInput{
		ProjectID: "1",
		Content:   "stages: [build]",
	})
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ---------------------------------------------------------------------------
// LintContent — all optional fields (DryRun, IncludeJobs, Ref)
// ---------------------------------------------------------------------------.

// TestCILintContent_AllOptionalFields verifies CILintContent when all optional fields.
func TestCILintContent_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/99/ci/lint" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{
				"valid":true,
				"errors":[],
				"warnings":["unused variable"],
				"merged_yaml":"stages:\n  - test",
				"includes":[]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	dryRun := true
	includeJobs := true
	out, err := LintContent(context.Background(), client, ContentInput{
		ProjectID:   "99",
		Content:     "stages:\n  - test",
		DryRun:      &dryRun,
		IncludeJobs: &includeJobs,
		Ref:         "main",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Valid {
		t.Error("expected valid=true")
	}
	if len(out.Warnings) != 1 || out.Warnings[0] != "unused variable" {
		t.Errorf("warnings = %v, want [unused variable]", out.Warnings)
	}
}

// ---------------------------------------------------------------------------
// toOutput — empty includes slice
// ---------------------------------------------------------------------------.

// TestToOutput_EmptyIncludes verifies ToOutput includes for empty.
func TestToOutput_EmptyIncludes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/ci/lint" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"valid":true,
				"errors":[],
				"warnings":[],
				"merged_yaml":"",
				"includes":[]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := LintProject(context.Background(), client, ProjectInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Includes) != 0 {
		t.Errorf("expected 0 includes, got %d", len(out.Includes))
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — valid with all sections
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_ValidAllSections verifies FormatOutputMarkdown when valid all sections.
func TestFormatOutputMarkdown_ValidAllSections(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Valid:      true,
		Errors:     nil,
		Warnings:   []string{"warn1", "warn2"},
		MergedYaml: "stages:\n  - build",
		Includes: []Include{
			{Type: "local", Location: ".gitlab-ci.yml", ContextProject: "my/project"},
			{Type: "remote", Location: "https://example.com/ci.yml", ContextProject: ""},
		},
	})

	for _, want := range []string{
		"## CI Lint: ✅ Valid",
		mdHeadingWarnings,
		"- warn1",
		"- warn2",
		mdHeadingIncludes,
		"| Type | Location | Context Project |",
		"| local |",
		"| remote |",
		mdHeadingMergedYAML,
		"```yaml",
		"stages:",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	if strings.Contains(md, "### Errors") {
		t.Error("should not contain Errors section when errors is nil")
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — invalid with errors
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_InvalidWithErrors verifies FormatOutputMarkdown when invalid with errors.
func TestFormatOutputMarkdown_InvalidWithErrors(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Valid:  false,
		Errors: []string{"syntax error on line 5", "unknown key: foo"},
	})

	for _, want := range []string{
		"## CI Lint: ❌ Invalid",
		"### Errors",
		"- syntax error on line 5",
		"- unknown key: foo",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	if strings.Contains(md, mdHeadingWarnings) {
		t.Error("should not contain Warnings section when no warnings")
	}
	if strings.Contains(md, mdHeadingIncludes) {
		t.Error("should not contain Includes section when no includes")
	}
	if strings.Contains(md, mdHeadingMergedYAML) {
		t.Error("should not contain Merged YAML section when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — empty output (all defaults)
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_Empty verifies FormatOutputMarkdown when empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	// Zero-value Output has Valid=false, which produces the Invalid header
	if !strings.Contains(md, "❌ Invalid") {
		t.Errorf("expected Invalid header for zero-value Output, got %q", md)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — valid but empty content returns empty
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_ValidNoContentReturnsMinimalMessage verifies FormatOutputMarkdown returns minimal message for valid no content.
func TestFormatOutputMarkdown_ValidNoContentReturnsMinimalMessage(t *testing.T) {
	md := FormatOutputMarkdown(Output{Valid: true})
	if md == "" {
		t.Error("expected non-empty string for valid output with no content")
	}
	if !strings.Contains(md, "Valid") {
		t.Errorf("expected 'Valid' in output, got %q", md)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — only merged yaml
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_OnlyMergedYaml verifies FormatOutputMarkdown when only merged YAML.
func TestFormatOutputMarkdown_OnlyMergedYaml(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Valid:      true,
		MergedYaml: "image: alpine",
	})
	if !strings.Contains(md, mdHeadingMergedYAML) {
		t.Errorf("expected Merged YAML section:\n%s", md)
	}
	if !strings.Contains(md, "```yaml") {
		t.Errorf("expected yaml code block:\n%s", md)
	}
	if !strings.Contains(md, "image: alpine") {
		t.Errorf("expected yaml content:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — only includes
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_OnlyIncludes verifies FormatOutputMarkdown includes for only.
func TestFormatOutputMarkdown_OnlyIncludes(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Valid: true,
		Includes: []Include{
			{Type: "template", Location: "Auto-DevOps.gitlab-ci.yml", ContextProject: "gitlab-org/gitlab"},
		},
	})
	if !strings.Contains(md, mdHeadingIncludes) {
		t.Errorf("expected Includes section:\n%s", md)
	}
	if !strings.Contains(md, "| template |") {
		t.Errorf("expected include row:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — only warnings
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_OnlyWarnings verifies FormatOutputMarkdown when only warnings.
func TestFormatOutputMarkdown_OnlyWarnings(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Valid:    true,
		Warnings: []string{"deprecated keyword"},
	})
	if !strings.Contains(md, mdHeadingWarnings) {
		t.Errorf("expected Warnings section:\n%s", md)
	}
	if !strings.Contains(md, "- deprecated keyword") {
		t.Errorf("expected warning entry:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — only errors (invalid)
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_OnlyErrors verifies FormatOutputMarkdown when only errors.
func TestFormatOutputMarkdown_OnlyErrors(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Valid:  false,
		Errors: []string{"config error"},
	})
	if !strings.Contains(md, "## CI Lint: ❌ Invalid") {
		t.Errorf("expected invalid header:\n%s", md)
	}
	if !strings.Contains(md, "- config error") {
		t.Errorf("expected error entry:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — includes with special characters in table cells
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_IncludesSpecialChars verifies FormatOutputMarkdown includes special chars.
func TestFormatOutputMarkdown_IncludesSpecialChars(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Valid: true,
		Includes: []Include{
			{Type: "local", Location: "path/with|pipe", ContextProject: "proj|etc"},
		},
	})
	if !strings.Contains(md, mdHeadingIncludes) {
		t.Errorf("expected Includes section:\n%s", md)
	}
	// Pipe characters should be escaped in table cells
	if strings.Contains(md, "path/with|pipe") {
		t.Errorf("pipe char in Location should be escaped:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates all CI lint canonical routes across table-driven subtests.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	lintResult := `{"valid":true,"errors":[],"warnings":[],"merged_yaml":"stages:\n  - build","includes":[]}`

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/ci/lint", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, lintResult)
	})
	handler.HandleFunc("POST /api/v4/projects/1/ci/lint", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, lintResult)
	})

	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"lint_project", "gitlab_ci_lint_project", map[string]any{
			"project_id": "1", "content_ref": "main", "dry_run": true,
			"dry_run_ref": "main", "include_jobs": false, "ref": "main",
		}},
		{"lint_content", "gitlab_ci_lint", map[string]any{
			"project_id": "1", "content": "stages: [build]",
			"dry_run": false, "include_jobs": false, "ref": "main",
		}},
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
