// markdown_test.go contains unit tests for the Markdown formatting functions
// in the markdown package.
package markdown

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestRender_Success verifies Render when success.
func TestRender_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/markdown" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"html":"<p>Hello <strong>world</strong></p>"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Render(t.Context(), client, RenderInput{Text: "Hello **world**"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.HTML != "<p>Hello <strong>world</strong></p>" {
		t.Errorf("unexpected HTML: %s", out.HTML)
	}
}

// TestRender_WithGFMAndProject verifies Render when with gfm and project.
func TestRender_WithGFMAndProject(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"html":"<p>Rendered with GFM</p>"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Render(t.Context(), client, RenderInput{
		Text:    "Hello",
		GFM:     true,
		Project: "my-group/my-project",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.HTML != "<p>Rendered with GFM</p>" {
		t.Errorf("unexpected HTML: %s", out.HTML)
	}
}

// TestRender_Error verifies Render when error.
func TestRender_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Render(t.Context(), client, RenderInput{Text: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatRenderMarkdown_Empty verifies that a render with no HTML produces
// the fixed sentence and nothing else, so a client is told the render was
// empty rather than shown an empty fence.
func TestFormatRenderMarkdown_Empty(t *testing.T) {
	if got := renderedText(t, FormatRenderMarkdown(RenderOutput{})); got != "Empty markdown rendered." {
		t.Errorf("text = %q, want the empty-render sentence", got)
	}
}

// TestFormatRenderMarkdown_WithData pins the whole text a populated render
// produces: the heading, the line saying whose HTML follows, and the HTML
// inside a fence.
//
// The fence is what the test is for. GitLab builds this HTML out of text
// anybody who can open an issue may have written, and concatenating it into
// the response hands the client whatever that document says: a client that
// renders raw HTML shows a live anchor to whatever host the content names,
// and a client that suppresses it drops the tag and says nothing about having
// done so. Escaping is not the answer for this one tool, since returning the
// rendered HTML is its whole job, so the fence is the containment and it is
// pinned here rather than described.
func TestFormatRenderMarkdown_WithData(t *testing.T) {
	html := `<p>Hello <a href="http://elsewhere.invalid">x</a></p>`

	got := renderedText(t, FormatRenderMarkdown(RenderOutput{HTML: html}))

	want := "## Rendered Markdown\n\nThe HTML the GitLab instance produced for the text it was given:\n\n" +
		"```html\n" + html + "\n```\n"
	if got != want {
		t.Errorf("text =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatRenderMarkdown_HTMLCarryingBackticks verifies that HTML holding a
// run of backticks of its own cannot close the fence around it: the fence is
// sized past the longest run inside, so everything after it stays contained
// instead of rendering as Markdown of the response.
func TestFormatRenderMarkdown_HTMLCarryingBackticks(t *testing.T) {
	got := renderedText(t, FormatRenderMarkdown(RenderOutput{HTML: "<p>```</p>"}))

	if !strings.Contains(got, "````html\n<p>```</p>\n````\n") {
		t.Errorf("text = %q, want a fence longer than the run inside it", got)
	}
}

// renderedText returns the one text block a formatter's result carries.
func renderedText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("the formatter returned no result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("the result carries %d content block(s), want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("the content block is %T, want text", result.Content[0])
	}
	return text.Text
}

// TestRender_CancelledContext verifies Render when cancelled context.
func TestRender_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"html":"<p>x</p>"}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Render(ctx, client, RenderInput{Text: "test"})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestActionSpecs_Metadata verifies canonical metadata for markdown rendering.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)

	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	spec := specs[0]
	if spec.OwnerPackage != "markdown" {
		t.Errorf("OwnerPackage = %q, want markdown", spec.OwnerPackage)
	}
	if spec.IndividualTool.Name != "gitlab_render_markdown" {
		t.Errorf("IndividualTool.Name = %q, want gitlab_render_markdown", spec.IndividualTool.Name)
	}
	if !spec.ReadOnly || !spec.Idempotent {
		t.Error("markdown render action should be read-only and idempotent")
	}
	if isGenericUsage(spec.Usage) {
		t.Errorf("Usage is generic/placeholder: %q", spec.Usage)
	}
	if hasOnlyToolnameAlias(spec) {
		t.Error("Aliases should include distinctive natural-language phrasing beyond the tool name")
	}
	if len(spec.RelatedActions) == 0 {
		t.Error("RelatedActions should cross-link related actions")
	}
	desc := spec.IndividualTool.Description
	if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
		t.Errorf("IndividualTool.Description must use the \"Returns: ... See also: ...\" form, got %q", desc)
	}
}

// genericUsageRe matches placeholder Usage sentences such as
// "Use to execute X domain action."; it mirrors the R-META auditor.
var genericUsageRe = regexp.MustCompile(`(?i)^use to execute\b.*\baction\.?\s*$`)

// isGenericUsage reports whether usage is empty or a placeholder sentence.
func isGenericUsage(usage string) bool {
	trimmed := strings.TrimSpace(usage)
	return trimmed == "" || genericUsageRe.MatchString(trimmed)
}

// hasOnlyToolnameAlias reports whether the spec exposes no natural-language
// alias beyond its canonical name and projected individual-tool name.
func hasOnlyToolnameAlias(spec toolutil.ActionSpec) bool {
	canonical := strings.ToLower(strings.TrimSpace(spec.Name))
	tool := strings.ToLower(strings.TrimSpace(spec.IndividualTool.Name))
	for _, alias := range spec.Aliases {
		normalized := strings.ToLower(strings.TrimSpace(alias))
		if normalized == "" || normalized == canonical || normalized == tool {
			continue
		}
		return false
	}
	return true
}

// TestActionSpecs_CallRoute verifies markdown rendering through the canonical route.
func TestActionSpecs_CallRoute(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/markdown" {
			testutil.RespondJSON(w, http.StatusOK, `{"html":"<p>rendered</p>"}`)
			return
		}
		http.NotFound(w, r)
	}))
	byTool := markdownSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_render_markdown"].Route.Handler(t.Context(), map[string]any{"text": "Hello **world**"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("Route.Handler returned nil")
	}
}

// markdownSpecsByTool supports markdown specs by tool assertions in markdown tests.
func markdownSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
