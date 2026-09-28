package mrcontextcommits

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// CommitItem is one context commit, the keys of lib/api/entities/commit.rb.
// create_context_commits answers with that entity; the list answers with
// lib/api/entities/commit_with_link.rb, which extends it.
//
// A context commit read back from the list is rebuilt from the row GitLab
// stored when it was pinned, and that row keeps no parents
// (MergeRequests::AddContextService drops them), so parent_ids arrives on the
// answer to create_context_commits and is empty on the list.
//
// extended_trailers maps each trailer to the list of its values, which is how
// Gitlab::Git::Commit#parse_commit_trailers builds it and how the entity
// documents it.
//
// Author, AuthorGravatarURL, DescriptionHTML and TitleHTML are keys
// lib/api/entities/commit_with_link.rb adds, so the list fills them and the
// answer to create_context_commits never does. The list route presents
// `with: Entities::CommitWithLink, type: :full, request: merge_request`
// (lib/api/merge_requests.rb) while its desc annotates Entities::Commit, which
// is why GitLab's generated reference, and the record R-PATH reads, know none
// of them. The rest of what CommitWithLink adds is left out on purpose:
// commit_url is web_url again and commit_path is its path, and signature_html,
// prev_commit_id, next_commit_id and pipeline_status_path are null on every
// commit of this route. The last three read presenter options the route does
// not pass, and signature_html renders only for a commit with a signature,
// which a context commit rebuilt from its stored row never has:
// Commit#raw_signature_type reads it off the Gitaly commit, and
// MergeRequestContextCommit#to_commit builds the commit from a hash of the row,
// which keeps no signature.
//
// author_gravatar_url is the instance's Gravatar image for author_email
// (GravatarService, which the instance can point elsewhere or turn off, and
// then it is null), the one picture of an author no account holds.
type CommitItem struct {
	ID                string              `json:"id"`
	ShortID           string              `json:"short_id"`
	Title             string              `json:"title"`
	Message           string              `json:"message,omitempty"`
	AuthorName        string              `json:"author_name"`
	AuthorEmail       string              `json:"author_email"`
	AuthoredDate      string              `json:"authored_date,omitempty"`
	CommitterName     string              `json:"committer_name,omitempty"`
	CommitterEmail    string              `json:"committer_email,omitempty"`
	CommittedDate     string              `json:"committed_date,omitempty"`
	CreatedAt         string              `json:"created_at,omitempty"`
	ParentIDs         []string            `json:"parent_ids,omitempty"`
	Trailers          map[string]string   `json:"trailers,omitempty"`
	ExtendedTrailers  map[string][]string `json:"extended_trailers,omitempty"`
	WebURL            string              `json:"web_url,omitempty"`
	Author            *CommitAuthor       `json:"author,omitempty"`
	AuthorGravatarURL string              `json:"author_gravatar_url,omitempty"`
	DescriptionHTML   string              `json:"description_html,omitempty"`
	TitleHTML         string              `json:"title_html,omitempty"`
}

// CommitAuthor is the GitLab account a listed context commit's author email
// belongs to, with the keys of lib/api/entities/user_path.rb: the basic user,
// the path of the user's profile, and show_status. GitLab sends null for an
// email no confirmed account holds, since Commit#lazy_author looks the author
// up with `User.by_any_email(emails, confirmed: true)`, and the key is then
// absent here.
//
// None of the entity's conditional keys reaches this route: avatar_path and
// custom_attributes wait on presenter options the route does not pass, and
// status_tooltip_html and availability on a status association that lookup
// does not preload. The same unloaded association is why show_status is false
// on every author this route sends; it is published because GitLab sends it.
type CommitAuthor struct {
	toolutil.UserBasicOutput
	Path       string `json:"path,omitempty"`
	ShowStatus bool   `json:"show_status"`
}

// capturedCommit is one commit of either context commit route's answer, read
// off the captured response as client-go's Commit reads it, except for
// extended_trailers, which the field here takes over: Commit declares it a map
// of strings, so it cannot hold the map of lists GitLab sends, and a page
// holding one commit with a trailer fails in client-go's decoder as a whole.
// encoding/json decodes a key into the shallowest field that names it, so the
// embedded Commit's own field stays empty. The four fields after it are keys
// of the list's CommitWithLink that Commit does not model (see [CommitItem]),
// and are empty on the answer to create_context_commits.
type capturedCommit struct {
	gl.Commit
	ExtendedTrailers  map[string][]string `json:"extended_trailers"`
	Author            *CommitAuthor       `json:"author"`
	AuthorGravatarURL string              `json:"author_gravatar_url"`
	DescriptionHTML   string              `json:"description_html"`
	TitleHTML         string              `json:"title_html"`
}

// misreadByClientGo reports whether err is client-go failing to decode an
// answer GitLab gave successfully into a struct of its own that cannot hold
// it, which is what a commit carrying a trailer produces (see
// [capturedCommit]). Such an answer is read from the capture by a type that
// can hold it, so the failure is not the handler's; any other error is.
func misreadByClientGo(err error) bool {
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &typeErr)
}

// capturedCommitItems decodes the commits a context commit route answered
// with from the captured response, in order.
func capturedCommitItems(captured *gitlabclient.ResponseCapture) ([]CommitItem, error) {
	var rows []capturedCommit
	if err := captured.Decode(&rows); err != nil {
		return nil, err
	}
	return toCommitItems(rows), nil
}

// toCommitItems converts the commits either context commit route answers with,
// in order. The three instants go out in RFC 3339, the wire form every other
// date here takes; Go's default layout, which created_at used to carry, is one
// the Markdown time helper cannot parse back, so the table printed it raw.
//
// It takes the page rather than one commit because a converter of one
// gl.Commit is read by the R-PATH type grain as answering every route client-go
// decodes a Commit from (a commit's detail, a cherry-pick, a revert), whose
// CommitDetail keys these two routes never send.
func toCommitItems(rows []capturedCommit) []CommitItem {
	items := make([]CommitItem, 0, len(rows))
	for i := range rows {
		c := &rows[i].Commit
		items = append(items, CommitItem{
			ID:                c.ID,
			ShortID:           c.ShortID,
			Title:             c.Title,
			Message:           c.Message,
			AuthorName:        c.AuthorName,
			AuthorEmail:       c.AuthorEmail,
			AuthoredDate:      toolutil.RFC3339Ptr(c.AuthoredDate),
			CommitterName:     c.CommitterName,
			CommitterEmail:    c.CommitterEmail,
			CommittedDate:     toolutil.RFC3339Ptr(c.CommittedDate),
			CreatedAt:         toolutil.RFC3339Ptr(c.CreatedAt),
			ParentIDs:         c.ParentIDs,
			Trailers:          c.Trailers,
			ExtendedTrailers:  rows[i].ExtendedTrailers,
			WebURL:            c.WebURL,
			Author:            rows[i].Author,
			AuthorGravatarURL: rows[i].AuthorGravatarURL,
			DescriptionHTML:   rows[i].DescriptionHTML,
			TitleHTML:         rows[i].TitleHTML,
		})
	}
	return items
}

// List.

// ListInput is the input for listing MR context commits, and the page of them
// to list.
//
// GitLab pages the list: the route builds
// paginate(merge_request.merge_request_context_commits), which reads page and
// per_page from the request although the route declares neither.
// client-go's ListMergeRequestContextCommits takes no options struct, so the
// page travels as a request option.
type ListInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"merge_request_iid"     jsonschema:"Merge request IID,required"`
	toolutil.PaginationInput
}

// ListOutput is the output for listing MR context commits: one page, and
// Pagination is where that page sits in the whole.
type ListOutput struct {
	toolutil.HintableOutput
	Commits    []CommitItem              `json:"commits"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// CreateOutput is the answer to create_context_commits: the commits it
// pinned, in the order GitLab answered with them.
//
// It is not [ListOutput], because the route is not paged: it answers with
// every commit it pinned and sends no pagination headers, so a pagination
// block here could only ever be zero, and its schema would tell a model that
// paging applies to a write.
type CreateOutput struct {
	toolutil.HintableOutput
	Commits []CommitItem `json:"commits"`
}

// List returns one page of the context commits of a merge request.
//
// The page is read from the captured body, since client-go's own decode fails
// on a commit carrying a trailer (see [capturedCommit]). The capture keeps no
// headers, so the pagination block comes from the response client-go returns,
// which it returns on that decode failure too.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("list_mr_context_commits: project_id is required")
	}
	if input.MergeRequest <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("list_mr_context_commits", "merge_request_iid")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, resp, err := client.GL().MergeRequestContextCommits.ListMergeRequestContextCommits(string(input.ProjectID), input.MergeRequest, gl.WithContext(ctx), toolutil.PaginationRequestOption(input.PaginationInput))
	if err != nil && !misreadByClientGo(err) {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_mr_context_commits", err, http.StatusNotFound, "verify project_id and merge_request_iid with merge_request.list")
	}
	items, err := capturedCommitItems(captured)
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("list_mr_context_commits", err)
	}
	return ListOutput{Commits: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Create.

// CreateInput is the input for creating MR context commits.
type CreateInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"merge_request_iid"     jsonschema:"Merge request IID,required"`
	Commits      []string             `json:"commits"    jsonschema:"List of commit SHAs to add as context,required"`
}

// Create adds context commits to a merge request.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (CreateOutput, error) {
	if err := ctx.Err(); err != nil {
		return CreateOutput{}, err
	}
	if input.ProjectID == "" {
		return CreateOutput{}, errors.New("create_mr_context_commits: project_id is required")
	}
	if input.MergeRequest <= 0 {
		return CreateOutput{}, toolutil.ErrRequiredInt64("create_mr_context_commits", "merge_request_iid")
	}
	opts := &gl.CreateMergeRequestContextCommitsOptions{
		Commits: &input.Commits,
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, _, err := client.GL().MergeRequestContextCommits.CreateMergeRequestContextCommits(string(input.ProjectID), input.MergeRequest, opts, gl.WithContext(ctx))
	if err != nil && !misreadByClientGo(err) {
		return CreateOutput{}, toolutil.WrapErrWithStatusHint("create_mr_context_commits", err, http.StatusBadRequest, "verify commit SHAs exist in the project repository")
	}
	items, err := capturedCommitItems(captured)
	if err != nil {
		return CreateOutput{}, toolutil.WrapErr("create_mr_context_commits", err)
	}
	return CreateOutput{Commits: items}, nil
}

// Delete.

// DeleteInput is the input for deleting MR context commits.
type DeleteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MergeRequest int64                `json:"merge_request_iid"     jsonschema:"Merge request IID,required"`
	Commits      []string             `json:"commits"    jsonschema:"List of commit SHAs to remove from context,required"`
}

// Delete removes context commits from a merge request.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("delete_mr_context_commits: project_id is required")
	}
	if input.MergeRequest <= 0 {
		return toolutil.ErrRequiredInt64("delete_mr_context_commits", "merge_request_iid")
	}
	opts := &gl.DeleteMergeRequestContextCommitsOptions{
		Commits: &input.Commits,
	}
	_, err := client.GL().MergeRequestContextCommits.DeleteMergeRequestContextCommits(string(input.ProjectID), input.MergeRequest, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete_mr_context_commits", err, http.StatusNotFound, "verify commit SHAs are valid context commits for this MR")
	}
	return nil
}

// Markdown Formatters.
