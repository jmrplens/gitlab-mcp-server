// Package groupwikis implements MCP tool handlers for GitLab group wiki
// operations including list, get, create, edit, and delete pages.
package groupwikis

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

type Output struct {
	toolutil.HintableOutput
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Format   string `json:"format"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

type ListOutput struct {
	toolutil.HintableOutput
	WikiPages []Output `json:"wiki_pages"`
}

func toOutput(w *gl.GroupWiki) Output {
	return Output{
		Title:    w.Title,
		Slug:     w.Slug,
		Format:   string(w.Format),
		Content:  w.Content,
		Encoding: w.Encoding,
	}
}

type ListInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id"              jsonschema:"Group ID or URL-encoded path,required"`
	WithContent bool                 `json:"with_content,omitempty" jsonschema:"Include page content in the response"`
}

type GetInput struct {
	GroupID    toolutil.StringOrInt `json:"group_id"             jsonschema:"Group ID or URL-encoded path,required"`
	Slug       string               `json:"slug"                  jsonschema:"URL-encoded slug of the wiki page,required"`
	RenderHTML bool                 `json:"render_html,omitempty" jsonschema:"Return HTML-rendered content"`
	Version    string               `json:"version,omitempty"     jsonschema:"Wiki page version SHA"`
}

type CreateInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Title   string               `json:"title"            jsonschema:"Title of the wiki page,required"`
	Content string               `json:"content"          jsonschema:"Content of the wiki page,required"`
	Format  string               `json:"format,omitempty" jsonschema:"Content format: markdown (default), rdoc, asciidoc, or org"`
}

type EditInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Slug    string               `json:"slug"              jsonschema:"URL-encoded slug of the wiki page to edit,required"`
	Title   string               `json:"title,omitempty"   jsonschema:"New title"`
	Content string               `json:"content,omitempty" jsonschema:"New content"`
	Format  string               `json:"format,omitempty"  jsonschema:"Content format: markdown, rdoc, asciidoc, or org"`
}

type DeleteInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Slug    string               `json:"slug"     jsonschema:"URL-encoded slug of the wiki page to delete,required"`
}

// List retrieves all wiki pages for a GitLab group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupWikisOptions{}
	if input.WithContent {
		opts.WithContent = new(true)
	}
	pages, _, err := client.GL().GroupWikis.ListGroupWikis(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("listGroupWikis", err)
	}
	out := make([]Output, len(pages))
	for i, w := range pages {
		out[i] = toOutput(w)
	}
	return ListOutput{WikiPages: out}, nil
}

// Get retrieves a single wiki page from a GitLab group.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Slug == "" {
		return Output{}, toolutil.ErrFieldRequired("slug")
	}
	opts := &gl.GetGroupWikiPageOptions{}
	if input.RenderHTML {
		opts.RenderHTML = new(true)
	}
	if input.Version != "" {
		opts.Version = new(input.Version)
	}
	w, _, err := client.GL().GroupWikis.GetGroupWikiPage(string(input.GroupID), input.Slug, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("getGroupWikiPage", err)
	}
	return toOutput(w), nil
}

// Create creates a new wiki page in a GitLab group.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Title == "" {
		return Output{}, toolutil.ErrFieldRequired("title")
	}
	if input.Content == "" {
		return Output{}, toolutil.ErrFieldRequired("content")
	}
	opts := &gl.CreateGroupWikiPageOptions{
		Title:   new(input.Title),
		Content: new(input.Content),
	}
	if input.Format != "" {
		f := gl.WikiFormatValue(input.Format)
		opts.Format = &f
	}
	w, _, err := client.GL().GroupWikis.CreateGroupWikiPage(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("createGroupWikiPage", err)
	}
	return toOutput(w), nil
}

// Edit updates an existing wiki page in a GitLab group.
func Edit(ctx context.Context, client *gitlabclient.Client, input EditInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Slug == "" {
		return Output{}, toolutil.ErrFieldRequired("slug")
	}
	opts := &gl.EditGroupWikiPageOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.Content != "" {
		opts.Content = new(input.Content)
	}
	if input.Format != "" {
		f := gl.WikiFormatValue(input.Format)
		opts.Format = &f
	}
	w, _, err := client.GL().GroupWikis.EditGroupWikiPage(string(input.GroupID), input.Slug, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("editGroupWikiPage", err)
	}
	return toOutput(w), nil
}

// Delete removes a wiki page from a GitLab group.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.Slug == "" {
		return toolutil.ErrFieldRequired("slug")
	}
	_, err := client.GL().GroupWikis.DeleteGroupWikiPage(string(input.GroupID), input.Slug, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("deleteGroupWikiPage", err)
	}
	return nil
}
