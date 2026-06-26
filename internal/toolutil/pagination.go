package toolutil

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
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
// Embed it alongside PaginationInput on a list input and wire it with
// ApplyListOptions. Keyset pagination is more efficient than offset pagination
// for deep pages of large, ordered result sets.
type KeysetPaginationInput struct {
	Pagination string `json:"pagination,omitempty" jsonschema:"Pagination method: 'keyset' for keyset-based pagination on large ordered result sets, or 'offset' (the default). Keyset avoids deep-offset cost."`
	PageToken  string `json:"page_token,omitempty" jsonschema:"Keyset pagination cursor: record id at which to fetch the next page, taken from the previous keyset response. Only used when pagination='keyset'."`
}

// ApplyListOptions copies offset (PaginationInput) and keyset
// (KeysetPaginationInput) parameters onto a gl.ListOptions, setting only the
// values the caller supplied. Pass a zero KeysetPaginationInput for endpoints
// that only support offset pagination.
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
	if keyset.PageToken != "" {
		opts.PageToken = keyset.PageToken
	}
}

// PaginationOutput holds pagination metadata extracted from GitLab API responses.
// Fields map to GitLab's X-Page, X-Per-Page, X-Total, X-Total-Pages, X-Next-Page,
// X-Prev-Page headers. HasMore is a derived convenience flag (NextPage > 0) so
// LLM clients can decide whether to paginate without inspecting NextPage.
type PaginationOutput struct {
	Page       int64 `json:"page"`
	PerPage    int64 `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
	NextPage   int64 `json:"next_page"`
	PrevPage   int64 `json:"prev_page"`
	HasMore    bool  `json:"has_more"`
}

// PaginationFromResponse extracts pagination metadata from a GitLab API response.
func PaginationFromResponse(resp *gl.Response) PaginationOutput {
	if resp == nil {
		return PaginationOutput{}
	}
	return PaginationOutput{
		Page:       resp.CurrentPage,
		PerPage:    resp.ItemsPerPage,
		TotalItems: resp.TotalItems,
		TotalPages: resp.TotalPages,
		NextPage:   resp.NextPage,
		PrevPage:   resp.PreviousPage,
		HasMore:    resp.NextPage > 0,
	}
}

// AdjustPagination corrects pagination metadata when the GitLab API does not
// return X-Total and X-Total-Pages headers (e.g., the Search API).
// It infers TotalItems and TotalPages from the actual item count received
// and the presence of a NextPage indicator.
func AdjustPagination(p *PaginationOutput, itemCount int) {
	if itemCount == 0 {
		return
	}
	if p.Page == 0 {
		p.Page = 1
	}
	if p.TotalItems == 0 {
		p.TotalItems = int64(itemCount)
	}
	if p.TotalPages == 0 {
		if p.NextPage > 0 {
			p.TotalPages = p.NextPage
		} else {
			p.TotalPages = p.Page
		}
	}
	p.HasMore = p.NextPage > 0
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
// was affected (e.g., "project 42", "branch feature/x").
func DeleteResult(resource string) (*mcp.CallToolResult, DeleteOutput, error) {
	out := DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted %s.", resource),
	}
	md := fmt.Sprintf(EmojiSuccess+" Successfully deleted **%s**.", resource)
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
// successful void operation. The message describes what happened.
func VoidResult(message string) (*mcp.CallToolResult, VoidOutput, error) {
	out := VoidOutput{
		Status:  "success",
		Message: message,
	}
	md := fmt.Sprintf(EmojiSuccess+" %s", message)
	return ToolResultAnnotated(md, ContentMutate), out, nil
}
