package toolutil

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// PaginationInput holds common pagination query parameters for list endpoints.
// Constraints (page>=1, per_page in [1,100]) are also enforced at the JSON
// Schema level by EnrichPaginationConstraints so LLM clients see the bounds
// directly in tools/list responses.
type PaginationInput struct {
	Page    int `json:"page,omitempty"     jsonschema:"Page number to fetch, 1-based. Defaults to 1. Use the next_page field from the previous response to paginate forward."`
	PerPage int `json:"per_page,omitempty" jsonschema:"Items per page. Defaults to 20, minimum 1, maximum 100. Use 100 to minimize round trips when the result set is large."`
}

// KeysetPaginationInput holds GitLab keyset-pagination parameters for list
// endpoints that support keyset pagination in addition to offset pagination.
// Embed it alongside PaginationInput on a list input, wire it with
// ApplyListOptions, and pass [KeysetContinuation] on the SDK call so the
// next_page_token a keyset page publishes can be followed. Keyset pagination
// is more efficient than offset pagination for deep pages of large, ordered
// result sets.
type KeysetPaginationInput struct {
	Pagination string `json:"pagination,omitempty" jsonschema:"Pagination method: 'keyset' for keyset-based pagination on large ordered result sets, or 'offset' (the default). Keyset avoids deep-offset cost, and a keyset page names the next one in next_page_token rather than next_page."`
	PageToken  string `json:"page_token,omitempty" jsonschema:"Next page of a keyset listing: the next_page_token of the previous response of this action, passed back unchanged. It carries the position and every filter of the listing as GitLab read them, and those are what the next page is asked with, whatever filters are sent beside it. Leave it empty for the first page."`
}

// ApplyListOptions copies offset (PaginationInput) and keyset
// (KeysetPaginationInput) parameters onto a gl.ListOptions, setting only the
// values the caller supplied. Pass a zero KeysetPaginationInput for endpoints
// that only support offset pagination.
//
// A page_token that is a next_page_token a previous response published is not
// copied: it is a link carrying the listing's parameters, which
// [KeysetContinuation] makes the query of the request, and sent whole as
// page_token it would reach GitLab as a parameter of its own, which GitLab
// copies into the next link it writes. Only a raw GitLab page token goes on
// PageToken.
func ApplyListOptions(opts *gl.ListOptions, page PaginationInput, keyset KeysetPaginationInput) {
	if opts == nil {
		return
	}
	if page.Page > 0 {
		opts.Page = int64(page.Page)
	}
	if page.PerPage > 0 {
		opts.PerPage = int64(page.PerPage)
	}
	if keyset.Pagination != "" {
		opts.Pagination = keyset.Pagination
	}
	if _, continuation := keysetContinuationLink(keyset.PageToken); keyset.PageToken != "" && !continuation {
		opts.PageToken = keyset.PageToken
	}
}

// PaginationRequestOption carries a caller's page and per_page to a client-go
// method that takes no options struct to put them in. client-go models
// several of GitLab's paginated lists that way (a user's GPG keys and emails,
// system hooks, project aliases, a commit's merge requests), and without this
// such a method can only ever ask for GitLab's first page.
//
// A value the caller left at zero is not sent, so GitLab applies its own
// default, exactly as [ApplyListOptions] leaves a zero out of a
// [gl.ListOptions]. The option runs after client-go has encoded the request's
// own query, so it adds to that query rather than replacing it.
func PaginationRequestOption(page PaginationInput) gl.RequestOptionFunc {
	return func(req *retryablehttp.Request) error {
		query := req.URL.Query()
		if page.Page > 0 {
			query.Set("page", strconv.Itoa(page.Page))
		}
		if page.PerPage > 0 {
			query.Set("per_page", strconv.Itoa(page.PerPage))
		}
		req.URL.RawQuery = query.Encode()
		return nil
	}
}

// PaginationOutput holds pagination metadata extracted from GitLab API responses.
// Fields map to GitLab's X-Page, X-Per-Page, X-Total, X-Total-Pages, X-Next-Page,
// X-Prev-Page headers, and NextPageToken to the next link of the Link header
// on a keyset page. HasMore is a derived convenience flag (NextPage > 0, or a
// NextPageToken) so LLM clients can decide whether to paginate without
// inspecting either.
type PaginationOutput struct {
	Page       int64 `json:"page"`
	PerPage    int64 `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
	NextPage   int64 `json:"next_page"`
	PrevPage   int64 `json:"prev_page"`
	HasMore    bool  `json:"has_more"`

	// NextPageToken is the continuation of a page GitLab served by keyset
	// pagination: the next link of its Link header, verbatim, which is exactly
	// what [gl.WithKeysetPaginationParameters] takes. A keyset page carries no
	// X-Next-Page, so NextPage stays 0 on it and this is the one way to ask
	// for its successor, by passing it back as page_token. It is empty on an
	// offset page and on the last keyset page.
	NextPageToken string `json:"next_page_token,omitempty"`
}

// PaginationFromResponse extracts pagination metadata from a GitLab API
// response: the offset headers as client-go parsed them, and the keyset
// continuation [keysetNextPageToken] reads from the Link header.
func PaginationFromResponse(resp *gl.Response) PaginationOutput {
	if resp == nil {
		return PaginationOutput{}
	}
	token := keysetNextPageToken(resp)
	return PaginationOutput{
		Page:          resp.CurrentPage,
		PerPage:       resp.ItemsPerPage,
		TotalItems:    resp.TotalItems,
		TotalPages:    resp.TotalPages,
		NextPage:      resp.NextPage,
		PrevPage:      resp.PreviousPage,
		HasMore:       resp.NextPage > 0 || token != "",
		NextPageToken: token,
	}
}

// keysetNextPageToken returns the next link of a page GitLab served by keyset
// pagination, or "" when the response names no keyset successor.
//
// GitLab names the next page in the Link header under both strategies, and
// only an offset page also sends X-Next-Page, so a next link with no next
// page number is a keyset continuation: an id_before or id_after on projects,
// by the sort, a cursor on issues, jobs and audit events, a page_token on the
// repository tree. The link is returned whole because it carries every
// parameter of the listing as GitLab read it, which is what lets
// [KeysetContinuation] resume it. A link with no query carries no position,
// and one that would send again exactly the request just answered names the
// page that was read rather than the next one; publishing either would send a
// caller round the same page for ever. That guard stops such a list on the
// second reading of a page, since the first request carries no position to
// compare the link's with, which is why the one route known to write such a
// link, a user's contributed projects, is never asked for by keyset at all.
func keysetNextPageToken(resp *gl.Response) string {
	if resp.NextPage > 0 || resp.NextLink == "" {
		return ""
	}
	next, err := url.Parse(resp.NextLink)
	if err != nil || next.RawQuery == "" {
		return ""
	}
	if resp.Response != nil && resp.Request != nil && !linkAdvances(resp.Request.URL.Query(), next.Query()) {
		return ""
	}
	return resp.NextLink
}

// linkAdvances reports whether following a next link changes the request
// that was just answered. [KeysetContinuation] sends the link's query as the
// whole query of the next request, so a link whose query is the one the
// request already carried would send that request again.
func linkAdvances(sent, next url.Values) bool {
	return !maps.EqualFunc(sent, next, slices.Equal)
}

// credentialParameters are the query parameters GitLab authenticates a request
// by, or acts as another user under: a personal, OAuth or CI job token, and
// sudo. GitLab copies every parameter of a request into the next link it
// writes, but this server never sends one of these in a query, so no link
// GitLab writes for it carries one, and a page_token that does was not
// published by a response.
var credentialParameters = []string{"private_token", "access_token", "job_token", "sudo"}

// KeysetContinuation returns the request option every handler of a list GitLab
// pages by keyset passes on its SDK call, last, after [gl.WithContext]. When
// page_token is a next_page_token a previous response published, the next
// link's query becomes the whole query of the request through
// [gl.WithKeysetPaginationParameters], so each endpoint is sent the parameter
// it pages by (id_before or id_after, cursor, page_token or last) and the rest
// of the listing's arguments exactly as GitLab read them and wrote them into
// the link. Whatever else the call sends in its query is dropped: GitLab
// writes every parameter of the listing into its link, an Array one such as
// labels or topic in Rack's bracket form (labels[]=a), while client-go sends
// the same filter comma-separated under the bare name, and Rack refuses with
// 400 a query that carries both. Any other page_token is a raw GitLab page
// token, which [ApplyListOptions] has already put on the request, and an
// empty one is the first page: neither changes it.
//
// A continuation that carries a credential or sudo is refused rather than
// applied, because passing it on would let a page_token authenticate, or act
// as another user, where no argument of the action can.
func KeysetContinuation(keyset KeysetPaginationInput) gl.RequestOptionFunc {
	return func(req *retryablehttp.Request) error {
		next, ok := keysetContinuationLink(keyset.PageToken)
		if !ok {
			return nil
		}
		query := next.Query()
		for _, name := range credentialParameters {
			if query.Has(name) {
				return fmt.Errorf("page_token carries %s, a parameter no next_page_token holds: pass back the next_page_token of the previous response unchanged", name)
			}
		}
		req.URL.RawQuery = ""
		return gl.WithKeysetPaginationParameters(keyset.PageToken)(req)
	}
}

// keysetContinuationLink parses a page_token as the next link a previous
// keyset response published, and reports whether it is one. A raw GitLab page
// token is a branch or tag name or a record ID, none of which can hold the
// '?' a link's query starts with, since Git refuses it in a ref name.
func keysetContinuationLink(token string) (*url.URL, bool) {
	next, err := url.Parse(token)
	if err != nil || next.RawQuery == "" {
		return nil, false
	}
	return next, true
}

// AdjustPagination completes the pagination of a list GitLab answers without
// the X-Total and X-Total-Pages headers, which is what its search API does. It
// numbers an unnumbered page 1 and derives HasMore from the next page, and it
// fills the two totals only on a lone page (no next page, no previous one, page
// 1 at most), the one page whose rows are the whole result. Anywhere else it
// leaves them at 0, the value [PaginationOutput] documents for a total GitLab
// did not send: search redacts the rows a caller may not read after it pages,
// so neither this page's row count nor the next page number counts the result,
// and an earlier page need not have been full. The Markdown then says "shown,
// more available" instead of a count nothing measured.
//
// A keyset page is left as [PaginationFromResponse] described it: it has no
// page number or total to infer from, and its successor is its next_page_token
// rather than a page.
func AdjustPagination(p *PaginationOutput, itemCount int) {
	if itemCount == 0 || p.NextPageToken != "" {
		return
	}
	if p.Page == 0 {
		p.Page = 1
	}
	p.HasMore = p.NextPage > 0
	if p.NextPage > 0 || p.PrevPage > 0 || p.Page > 1 {
		return
	}
	if p.TotalItems == 0 {
		p.TotalItems = int64(itemCount)
	}
	if p.TotalPages == 0 {
		p.TotalPages = 1
	}
}

// DeleteOutput is a confirmation message returned by destructive tool handlers
// (delete, unprotect, unapprove) so the LLM receives explicit feedback instead
// of empty content when the operation succeeds.
type DeleteOutput struct {
	HintableOutput
	Status  string `json:"status"`
	Message string `json:"message"`
}

// DeleteResult builds a DeleteOutput and its Markdown representation for
// a successful destructive operation. The resource parameter describes what
// was affected (e.g., "project 42", "branch feature/x"), and reaches the
// Markdown through the inline escaper a card row uses, since the branch,
// package or file name inside it is the caller's and GitLab's rather than the
// server's: unescaped, a name carrying a line break wrote structure of its own
// under the confirmation, and one carrying a tag reached the page as raw HTML.
// The structured message keeps the sentence as it reads.
func DeleteResult(resource string) (*mcp.CallToolResult, DeleteOutput, error) {
	out := DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted %s.", resource),
	}
	md := fmt.Sprintf(EmojiSuccess+" Successfully deleted **%s**.", cardInline(resource))
	return ToolResultAnnotated(md, ContentMutate), out, nil
}

// VoidOutput is a confirmation message returned by tool handlers that perform
// an action without returning domain data (e.g., set header, start mirroring).
type VoidOutput struct {
	HintableOutput
	Status  string `json:"status"`
	Message string `json:"message"`
}

// VoidResult builds a VoidOutput and its Markdown representation for a
// successful void operation. The message describes what happened, and reaches
// the Markdown through the same escaper [DeleteResult] uses and for the same
// reason.
func VoidResult(message string) (*mcp.CallToolResult, VoidOutput, error) {
	out := VoidOutput{
		Status:  "success",
		Message: message,
	}
	md := EmojiSuccess + " " + cardInline(message)
	return ToolResultAnnotated(md, ContentMutate), out, nil
}
