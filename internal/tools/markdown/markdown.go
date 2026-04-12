// Package markdown implements the MCP tool handler for rendering
// GitLab-flavored markdown. It wraps the MarkdownService from client-go v2.
package markdown

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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

// Render renders arbitrary markdown text to HTML using the GitLab API.
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
		return RenderOutput{}, toolutil.WrapErrWithMessage("render_markdown", err)
	}
	return RenderOutput{HTML: md.HTML}, nil
}

// FormatRenderMarkdown formats the rendered markdown output.
func FormatRenderMarkdown(out RenderOutput) *mcp.CallToolResult {
	if out.HTML == "" {
		return toolutil.ToolResultWithMarkdown("Empty markdown rendered.")
	}
	return toolutil.ToolResultWithMarkdown("## Rendered Markdown\n\n" + out.HTML)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatRenderMarkdown)
}
