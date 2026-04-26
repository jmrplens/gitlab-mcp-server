// pagination_test.go contains unit tests for PaginationFromResponse and
// DeleteResult. Tests cover nil response, fully populated headers, single-page
// results, and the standard delete output builder.

package toolutil

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestPaginationFromResponse_NilResponse verifies that PaginationFromResponse
// returns a zero-value PaginationOutput when given a nil response.
func TestPaginationFromResponse_NilResponse(t *testing.T) {
	got := PaginationFromResponse(nil)

	if got.Page != 0 {
		t.Errorf("Page = %d, want 0", got.Page)
	}
	if got.PerPage != 0 {
		t.Errorf("PerPage = %d, want 0", got.PerPage)
	}
	if got.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", got.TotalItems)
	}
	if got.TotalPages != 0 {
		t.Errorf("TotalPages = %d, want 0", got.TotalPages)
	}
	if got.NextPage != 0 {
		t.Errorf("NextPage = %d, want 0", got.NextPage)
	}
	if got.PrevPage != 0 {
		t.Errorf("PrevPage = %d, want 0", got.PrevPage)
	}
	if got.HasMore {
		t.Errorf("HasMore = true, want false on nil response")
	}
}

// TestPaginationFromResponse_WithPaginationHeaders verifies that all pagination
// fields are correctly extracted from a multi-page GitLab API response.
func TestPaginationFromResponse_WithPaginationHeaders(t *testing.T) {
	resp := &gl.Response{
		CurrentPage:  2,
		ItemsPerPage: 20,
		TotalItems:   55,
		TotalPages:   3,
		NextPage:     3,
		PreviousPage: 1,
	}

	got := PaginationFromResponse(resp)

	if got.Page != 2 {
		t.Errorf("Page = %d, want 2", got.Page)
	}
	if got.PerPage != 20 {
		t.Errorf("PerPage = %d, want 20", got.PerPage)
	}
	if got.TotalItems != 55 {
		t.Errorf("TotalItems = %d, want 55", got.TotalItems)
	}
	if got.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", got.TotalPages)
	}
	if got.NextPage != 3 {
		t.Errorf("NextPage = %d, want 3", got.NextPage)
	}
	if got.PrevPage != 1 {
		t.Errorf("PrevPage = %d, want 1", got.PrevPage)
	}
	if !got.HasMore {
		t.Errorf("HasMore = false, want true when NextPage > 0")
	}
}

// TestPaginationFromResponse_SinglePage verifies that NextPage and PrevPage
// are zero when the result fits on a single page.
func TestPaginationFromResponse_SinglePage(t *testing.T) {
	resp := &gl.Response{
		CurrentPage:  1,
		ItemsPerPage: 20,
		TotalItems:   5,
		TotalPages:   1,
		NextPage:     0,
		PreviousPage: 0,
	}

	got := PaginationFromResponse(resp)

	if got.Page != 1 {
		t.Errorf("Page = %d, want 1", got.Page)
	}
	if got.TotalPages != 1 {
		t.Errorf("TotalPages = %d, want 1", got.TotalPages)
	}
	if got.NextPage != 0 {
		t.Errorf("NextPage = %d, want 0 (no next page)", got.NextPage)
	}
	if got.PrevPage != 0 {
		t.Errorf("PrevPage = %d, want 0 (no previous page)", got.PrevPage)
	}
	if got.HasMore {
		t.Errorf("HasMore = true, want false on single-page result")
	}
}

// TestAdjustPagination verifies that AdjustPagination corrects pagination
// metadata when the GitLab API does not return X-Total/X-Total-Pages headers.
func TestAdjustPagination(t *testing.T) {
	tests := []struct {
		name      string
		input     PaginationOutput
		itemCount int
		wantItems int64
		wantPages int64
		wantPage  int64
		wantMore  bool
	}{
		{
			name:      "no items — no adjustment",
			input:     PaginationOutput{Page: 1, PerPage: 20},
			itemCount: 0,
			wantItems: 0,
			wantPages: 0,
			wantPage:  1,
			wantMore:  false,
		},
		{
			name:      "single page with missing totals",
			input:     PaginationOutput{Page: 1, PerPage: 20},
			itemCount: 3,
			wantItems: 3,
			wantPages: 1,
			wantPage:  1,
			wantMore:  false,
		},
		{
			name:      "page zero corrected to 1",
			input:     PaginationOutput{Page: 0, PerPage: 20},
			itemCount: 5,
			wantItems: 5,
			wantPages: 1,
			wantPage:  1,
			wantMore:  false,
		},
		{
			name:      "multi-page with next page",
			input:     PaginationOutput{Page: 1, PerPage: 20, NextPage: 2},
			itemCount: 20,
			wantItems: 20,
			wantPages: 2,
			wantPage:  1,
			wantMore:  true,
		},
		{
			name:      "existing totals not overwritten",
			input:     PaginationOutput{Page: 1, PerPage: 20, TotalItems: 55, TotalPages: 3},
			itemCount: 20,
			wantItems: 55,
			wantPages: 3,
			wantPage:  1,
			wantMore:  false,
		},
		{
			name:      "last page — NextPage 0 infers TotalPages from Page",
			input:     PaginationOutput{Page: 3, PerPage: 20, NextPage: 0},
			itemCount: 5,
			wantItems: 5,
			wantPages: 3,
			wantPage:  3,
			wantMore:  false,
		},
		{
			name:      "middle page — NextPage set infers TotalPages",
			input:     PaginationOutput{Page: 2, PerPage: 20, NextPage: 3},
			itemCount: 20,
			wantItems: 20,
			wantPages: 3,
			wantPage:  2,
			wantMore:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.input
			AdjustPagination(&p, tt.itemCount)
			if p.TotalItems != tt.wantItems {
				t.Errorf("TotalItems = %d, want %d", p.TotalItems, tt.wantItems)
			}
			if p.TotalPages != tt.wantPages {
				t.Errorf("TotalPages = %d, want %d", p.TotalPages, tt.wantPages)
			}
			if p.Page != tt.wantPage {
				t.Errorf("Page = %d, want %d", p.Page, tt.wantPage)
			}
			if p.HasMore != tt.wantMore {
				t.Errorf("HasMore = %v, want %v", p.HasMore, tt.wantMore)
			}
		})
	}
}

// TestDeleteResult verifies that DeleteResult returns a well-formed MCP result,
// a DeleteOutput with "success" status and descriptive message, and nil error.
func TestDeleteResult(t *testing.T) {
	result, out, err := DeleteResult("branch feature/x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "success" {
		t.Errorf("Status = %q, want %q", out.Status, "success")
	}
	if !strings.Contains(out.Message, "branch feature/x") {
		t.Errorf("Message = %q, want resource name included", out.Message)
	}
	if result == nil {
		t.Fatal("expected non-nil CallToolResult")
	}
	if result.IsError {
		t.Error("expected IsError = false for successful delete")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content block")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "branch feature/x") {
		t.Errorf("markdown = %q, want resource name included", text)
	}
}

// TestVoidResult verifies the successful void operation result returned by
// mutating tools that produce no domain-specific output (e.g. start mirroring).
func TestVoidResult(t *testing.T) {
	callResult, out, err := VoidResult("Mirroring started successfully.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callResult == nil {
		t.Fatal("expected non-nil CallToolResult")
	}
	if out.Status != "success" {
		t.Errorf("Status = %q, want %q", out.Status, "success")
	}
	if out.Message != "Mirroring started successfully." {
		t.Errorf("Message = %q, want %q", out.Message, "Mirroring started successfully.")
	}
}
