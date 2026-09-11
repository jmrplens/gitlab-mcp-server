package markdown

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// RenderInput is the input for rendering markdown.
type RenderInput struct {
	Text    string `json:"text" jsonschema:"Markdown text to render,required"`
	GFM     bool   `json:"gfm,omitempty" jsonschema:"Use GitLab Flavored Markdown (default false)"`
	Project string `json:"project,omitempty" jsonschema:"Project path for resolving references (e.g. group/project)"`
}

// RenderOutput is the output containing the rendered HTML.
type RenderOutput struct {
	toolutil.HintableOutput
	HTML string `json:"html"`
}

// Render sends arbitrary Markdown text to the GitLab Markdown render
// API (POST /markdown) and returns the resulting HTML. When GFM is
// true, GitLab Flavored Markdown extensions are applied; an optional
// Project enables reference resolution (e.g. issue/MR links).
func Render(ctx context.Context, client *gitlabclient.Client, input RenderInput) (RenderOutput, error) {
	opts := &gl.RenderOptions{
		Text: new(input.Text),
	}
	if input.GFM {
		opts.GitlabFlavouredMarkdown = new(true)
	}
	if input.Project != "" {
		opts.Project = new(input.Project)
	}
	md, _, err := client.GL().Markdown.Render(opts, gl.WithContext(ctx))
	if err != nil {
		return RenderOutput{}, toolutil.WrapErrWithStatusHint("render_markdown", err, http.StatusBadRequest, "verify the markdown text is valid and project is accessible if using project context")
	}
	return RenderOutput{HTML: md.HTML}, nil
}

// FormatRenderMarkdown wraps the [RenderOutput] HTML in an
// [mcp.CallToolResult] with a Markdown heading so downstream agents
// see a stable structure even when the rendered HTML is empty.
//
// The HTML is fenced rather than concatenated into the page. What GitLab
// returns here is a document of tags built from text anybody who can write an
// issue or a comment could have supplied, and a response that pastes it in
// hands the client whatever that document says: with raw HTML enabled a
// client renders a live anchor to whatever host the content names, and with
// raw HTML suppressed it silently drops the tag and shows neither the link
// nor the fact that one was removed. Escaping is the wrong answer for this
// one tool, because returning the rendered HTML is what it is for, so the
// answer is containment: a fence sized by [toolutil.MarkdownFencedBlock],
// which no run of backticks in the document can close, under a line saying
// whose HTML it is.
func FormatRenderMarkdown(out RenderOutput) *mcp.CallToolResult {
	if out.HTML == "" {
		return toolutil.ToolResultWithMarkdown("Empty markdown rendered.")
	}
	return toolutil.ToolResultWithMarkdown("## Rendered Markdown\n\nThe HTML the GitLab instance produced for the text it was given:\n\n" +
		toolutil.MarkdownFencedBlock("html", out.HTML))
}

func init() {
	toolutil.RegisterMarkdownResult(FormatRenderMarkdown)
}
