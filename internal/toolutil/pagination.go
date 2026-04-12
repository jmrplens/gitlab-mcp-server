// pagination.go defines input and output structs for paginated GitLab API
// responses, and a helper to extract pagination metadata from API response headers.

package toolutil

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// PaginationInput holds common pagination query parameters for list endpoints.
type PaginationInput struct {
	Page    int `json:"page,omitempty"     jsonschema:"Page number (default 1)"`
	PerPage int `json:"per_page,omitempty" jsonschema:"Items per page (default 20, max 100)"`
}

// PaginationOutput holds pagination metadata extracted from GitLab API responses.
// Fields map to GitLab's X-Page, X-Per-Page, X-Total, X-Total-Pages, X-Next-Page, X-Prev-Page headers.
type PaginationOutput struct {
	Page       int64 `json:"page"`
	PerPage    int64 `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
	NextPage   int64 `json:"next_page"`
	PrevPage   int64 `json:"prev_page"`
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
