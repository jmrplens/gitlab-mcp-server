// pagination_test.go contains unit tests for PaginationFromResponse,
// PaginationRequestOption, KeysetContinuation and DeleteResult. Tests cover
// nil response, fully populated headers, single-page results, the keyset
// continuation a Link header names and its round trip back onto a request,
// the page and per_page a request option carries onto a request, and the
// standard delete output builder.
package toolutil

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
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
		// A keyset page has no page number and no total to infer, and its
		// successor is the token: recomputing has_more from next_page alone
		// would report its last page wherever it has one.
		{
			name:      "keyset page is left as GitLab described it",
			input:     PaginationOutput{HasMore: true, NextPageToken: keysetNextLink},
			itemCount: 2,
			wantItems: 0,
			wantPages: 0,
			wantPage:  0,
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

// TestDeleteResult_HostileResource_StaysInsideTheConfirmation verifies that
// the resource a handler names cannot end the confirmation sentence the two
// result builders write, whole output for each.
//
// The resource is composed from what the caller sent and what GitLab holds, so
// a branch called "x\n## injected" used to write a heading of the response
// under a successful delete, and one holding a tag reached the page as raw
// HTML. [VoidResult] writes the same sentence without the bold, and is held to
// the same containment.
func TestDeleteResult_HostileResource_StaysInsideTheConfirmation(t *testing.T) {
	const hostile = "branch x\n## injected"
	const escaped = "branch x ## injected"

	result, out, err := DeleteResult(hostile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "Successfully deleted " + hostile + "."; out.Message != want {
		t.Errorf("Message = %q, want %q", out.Message, want)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if want := EmojiSuccess + " Successfully deleted **" + escaped + "**."; text != want {
		t.Errorf("delete markdown = %q, want %q", text, want)
	}

	voidResult, voidOut, err := VoidResult(hostile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if voidOut.Message != hostile {
		t.Errorf("Message = %q, want %q", voidOut.Message, hostile)
	}
	voidText := voidResult.Content[0].(*mcp.TextContent).Text
	if want := EmojiSuccess + " " + escaped; voidText != want {
		t.Errorf("void markdown = %q, want %q", voidText, want)
	}
}

// TestApplyListOptions_SetsOffsetAndKeyset verifies ApplyListOptions copies only
// the supplied offset and keyset pagination values onto a gl.ListOptions and
// leaves unset fields at their zero value.
func TestApplyListOptions_SetsOffsetAndKeyset(t *testing.T) {
	var opts gl.ListOptions
	ApplyListOptions(&opts, PaginationInput{Page: 3, PerPage: 50}, KeysetPaginationInput{Pagination: "keyset", PageToken: "abc"})
	if opts.Page != 3 || opts.PerPage != 50 {
		t.Errorf("offset = %d/%d, want 3/50", opts.Page, opts.PerPage)
	}
	if opts.Pagination != "keyset" || opts.PageToken != "abc" {
		t.Errorf("keyset = %q/%q, want keyset/abc", opts.Pagination, opts.PageToken)
	}
}

// TestApplyListOptions_OnlyNonZero verifies that zero-valued pagination inputs
// leave the corresponding gl.ListOptions fields untouched, and that a nil opts
// pointer is a safe no-op.
func TestApplyListOptions_OnlyNonZero(t *testing.T) {
	opts := gl.ListOptions{Page: 7, PerPage: 10, Pagination: "offset", PageToken: "keep"}
	ApplyListOptions(&opts, PaginationInput{}, KeysetPaginationInput{})
	if opts.Page != 7 || opts.PerPage != 10 || opts.Pagination != "offset" || opts.PageToken != "keep" {
		t.Errorf("zero inputs mutated opts: %+v", opts)
	}
	ApplyListOptions(nil, PaginationInput{Page: 1}, KeysetPaginationInput{}) // must not panic
}

// keysetNextLink is a next link the way GitLab writes one for a keyset page of
// GET /projects: every parameter the request carried, as Grape read it, plus
// the position to resume from.
const keysetNextLink = "https://gitlab.example.com/api/v4/projects?id_after=42&order_by=id&pagination=keyset&per_page=2&sort=asc"

// responseTo builds a client-go response to a request for rawURL, carrying
// next as its keyset next link, the way client-go fills one from a Link
// header.
func responseTo(t *testing.T, rawURL, next string) *gl.Response {
	t.Helper()
	sent, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	return &gl.Response{
		Response: &http.Response{Request: &http.Request{URL: sent}},
		NextLink: next,
	}
}

// TestPaginationFromResponse_KeysetLink_PublishesTheContinuation verifies the
// pagination block of each shape a response can name its successor in. Under
// keyset pagination GitLab answers with a Link header naming the next page and
// no X-Next-Page, so the block used to report has_more false on a page that
// had a successor and carry nothing a caller could send back (issue 1165).
// The next link is now published as next_page_token, verbatim, since it is
// exactly what gl.WithKeysetPaginationParameters takes, and has_more follows
// it. An offset page keeps next_page as its continuation and publishes no
// token, and a link that cannot carry a position, or names the page just
// read, is not one.
func TestPaginationFromResponse_KeysetLink_PublishesTheContinuation(t *testing.T) {
	const sentFirst = "https://gitlab.example.com/api/v4/projects?order_by=id&pagination=keyset&per_page=2&sort=asc"
	cases := []struct {
		name      string
		resp      *gl.Response
		wantToken string
		wantMore  bool
	}{
		{name: "a keyset page names its successor in the link alone", resp: &gl.Response{NextLink: keysetNextLink}, wantToken: keysetNextLink, wantMore: true},
		{name: "a keyset page answering a request names its successor", resp: responseTo(t, sentFirst, keysetNextLink), wantToken: keysetNextLink, wantMore: true},
		{name: "a response that kept no request still names its successor", resp: &gl.Response{Response: &http.Response{}, NextLink: keysetNextLink}, wantToken: keysetNextLink, wantMore: true},
		{name: "an offset page keeps next_page and publishes no token", resp: &gl.Response{NextPage: 3, NextLink: "https://gitlab.example.com/api/v4/projects?page=3&per_page=2"}, wantMore: true},
		{name: "the last keyset page names no successor", resp: &gl.Response{}},
		{name: "a link with no query carries no position", resp: &gl.Response{NextLink: "https://gitlab.example.com/api/v4/projects"}},
		{name: "a link that does not parse is not published", resp: &gl.Response{NextLink: "https://gitlab.example.com/%zz?id_after=1"}},
		// GitLab's contributed projects route answers keyset pages with an
		// id_after its finder never reads, so the page after the first is the
		// first again, under the same link. Publishing that link would send a
		// caller round the same page for ever.
		{name: "a link naming the page just read does not advance", resp: responseTo(t, keysetNextLink, keysetNextLink)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PaginationFromResponse(tc.resp)
			if got.NextPageToken != tc.wantToken {
				t.Errorf("NextPageToken = %q, want %q", got.NextPageToken, tc.wantToken)
			}
			if got.HasMore != tc.wantMore {
				t.Errorf("HasMore = %v, want %v", got.HasMore, tc.wantMore)
			}
		})
	}
}

// TestPaginationFromResponse_KeysetToken_IsWhatTheContinuationTakes verifies
// the round trip the issue asked for: the token a keyset page publishes,
// handed back as page_token, makes the next request carry exactly the next
// link's parameters, so each endpoint is sent the one it pages by.
func TestPaginationFromResponse_KeysetToken_IsWhatTheContinuationTakes(t *testing.T) {
	token := PaginationFromResponse(&gl.Response{NextLink: keysetNextLink}).NextPageToken
	req, err := retryablehttp.NewRequest(http.MethodGet, "https://gitlab.example.com/api/v4/projects?per_page=20", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err = KeysetContinuation(KeysetPaginationInput{PageToken: token})(req); err != nil {
		t.Fatalf("continuation returned %v, want nil", err)
	}
	want, err := url.Parse(keysetNextLink)
	if err != nil {
		t.Fatalf("parse next link: %v", err)
	}
	if got := req.URL.Query(); !reflect.DeepEqual(got, want.Query()) {
		t.Errorf("query = %v, want the next link's %v", got, want.Query())
	}
}

// TestKeysetContinuation_Cases_CarriesTheListingOn verifies the request
// option every keyset-capable handler passes on its SDK call. A token a
// previous response published becomes the request's whole query, since
// GitLab writes every parameter it read into the link; nothing else changes
// the request, since a raw GitLab page token is already on it through
// [ApplyListOptions]. A token carrying a credential or an identity is refused,
// because passing it on would let a page_token authenticate or impersonate
// where no argument of the action can.
func TestKeysetContinuation_Cases_CarriesTheListingOn(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		token   string
		want    url.Values
		wantErr string
	}{
		{name: "no token leaves the request alone", query: "order_by=id&pagination=keyset", want: url.Values{"order_by": {"id"}, "pagination": {"keyset"}}},
		{name: "a raw page token is left where ApplyListOptions put it", query: "page_token=main&pagination=keyset", token: "main", want: url.Values{"page_token": {"main"}, "pagination": {"keyset"}}},
		// Git allows a percent sign in a branch name, and such a name does
		// not parse as a URL at all, so it is a raw token like any other.
		{name: "a raw page token that does not parse as a link is left alone", query: "page_token=100%25&pagination=keyset", token: "100%", want: url.Values{"page_token": {"100%"}, "pagination": {"keyset"}}},
		// GitLab writes the link from every parameter it read, so the link is
		// the whole of the next request: a parameter sent beside the token
		// and missing from it was not part of the listing the token resumes.
		{
			name:  "an id_after continuation is the whole of the next query",
			query: "order_by=id&per_page=20&search=api",
			token: keysetNextLink,
			want:  url.Values{"id_after": {"42"}, "order_by": {"id"}, "pagination": {"keyset"}, "per_page": {"2"}, "sort": {"asc"}},
		},
		// GitLab writes an Array parameter of the listing in Rack's bracket
		// form, while client-go sends the same filter comma-separated under
		// the bare name. Both forms on one request make Rack refuse the query
		// before the API runs, so the filter the caller sends again beside
		// the token must not survive it.
		{
			name:  "an Array filter in GitLab's form replaces client-go's comma form",
			query: "labels=bug%2Cx&not%5Blabels%5D=wontfix&pagination=keyset&per_page=20",
			token: "https://gitlab.example.com/api/v4/projects/42/issues?cursor=NEXT&id=42&labels%5B%5D=bug&labels%5B%5D=x&not%5Blabels%5D%5B%5D=wontfix&pagination=keyset&per_page=20",
			want:  url.Values{"cursor": {"NEXT"}, "id": {"42"}, "labels[]": {"bug", "x"}, "not[labels][]": {"wontfix"}, "pagination": {"keyset"}, "per_page": {"20"}},
		},
		{
			name:  "a cursor continuation",
			query: "pagination=keyset",
			token: "https://gitlab.example.com/api/v4/projects/7/issues?cursor=eyJpZCI6IjQyIn0&pagination=keyset&per_page=20",
			want:  url.Values{"cursor": {"eyJpZCI6IjQyIn0"}, "pagination": {"keyset"}, "per_page": {"20"}},
		},
		{
			name:  "a page_token continuation",
			query: "pagination=keyset",
			token: "https://gitlab.example.com/api/v4/projects/7/repository/tree?page_token=a1e8f8d7&pagination=keyset&per_page=20",
			want:  url.Values{"page_token": {"a1e8f8d7"}, "pagination": {"keyset"}, "per_page": {"20"}},
		},
		{name: "a private_token is refused", token: "https://gitlab.example.com/api/v4/projects?id_after=1&private_token=glpat-x", wantErr: "private_token"},
		{name: "an access_token is refused", token: "https://gitlab.example.com/api/v4/projects?access_token=x&id_after=1", wantErr: "access_token"},
		{name: "a job_token is refused", token: "https://gitlab.example.com/api/v4/projects?id_after=1&job_token=x", wantErr: "job_token"},
		{name: "sudo is refused", token: "https://gitlab.example.com/api/v4/projects?id_after=1&sudo=root", wantErr: "sudo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := retryablehttp.NewRequest(http.MethodGet, "https://gitlab.example.com/api/v4/projects?"+tc.query, nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			err = KeysetContinuation(KeysetPaginationInput{Pagination: "keyset", PageToken: tc.token})(req)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("continuation error = %v, want one naming %q", err, tc.wantErr)
				}
				if req.URL.RawQuery != tc.query {
					t.Errorf("query = %q after a refusal, want it untouched as %q", req.URL.RawQuery, tc.query)
				}
				return
			}
			if err != nil {
				t.Fatalf("continuation returned %v, want nil", err)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("query = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestApplyListOptions_Continuation_IsNotAPageToken verifies that a token a
// previous keyset response published stays off gl.ListOptions.PageToken. It is
// a link carrying the listing's parameters, which [KeysetContinuation] applies
// one by one; sent whole as page_token it would reach GitLab as a parameter
// of its own, which GitLab copies into the next link it writes, so every page
// would carry the links of all the pages before it.
func TestApplyListOptions_Continuation_IsNotAPageToken(t *testing.T) {
	var opts gl.ListOptions
	ApplyListOptions(&opts, PaginationInput{}, KeysetPaginationInput{Pagination: "keyset", PageToken: keysetNextLink})
	if opts.PageToken != "" {
		t.Errorf("PageToken = %q, want it left empty for a continuation", opts.PageToken)
	}
	if opts.Pagination != "keyset" {
		t.Errorf("Pagination = %q, want keyset", opts.Pagination)
	}
}

// TestPaginationRequestOption_AddsWhatTheCallerAskedFor verifies the request
// option carries a caller's page and per_page onto a request whose client-go
// method has no options struct: a value left at zero is not sent, a value the
// query already holds is replaced rather than repeated, and the rest of the
// query client-go encoded survives.
func TestPaginationRequestOption_AddsWhatTheCallerAskedFor(t *testing.T) {
	tests := []struct {
		name  string
		query string
		page  PaginationInput
		want  url.Values
	}{
		{
			name:  "nothing asked for sends neither parameter",
			query: "state=all",
			want:  url.Values{"state": {"all"}},
		},
		{
			name:  "both asked for are both sent beside the existing query",
			query: "state=all",
			page:  PaginationInput{Page: 3, PerPage: 50},
			want:  url.Values{"state": {"all"}, "page": {"3"}, "per_page": {"50"}},
		},
		{
			name: "page alone leaves per_page to GitLab",
			page: PaginationInput{Page: 2},
			want: url.Values{"page": {"2"}},
		},
		{
			name: "per_page alone leaves page to GitLab",
			page: PaginationInput{PerPage: 1},
			want: url.Values{"per_page": {"1"}},
		},
		{
			name:  "a value already in the query is replaced rather than repeated",
			query: "page=1&per_page=20",
			page:  PaginationInput{Page: 4, PerPage: 100},
			want:  url.Values{"page": {"4"}, "per_page": {"100"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := retryablehttp.NewRequest(http.MethodGet, "https://gitlab.example.com/api/v4/user/gpg_keys?"+tt.query, nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if err = PaginationRequestOption(tt.page)(req); err != nil {
				t.Fatalf("option returned %v, want nil", err)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("query = %v, want %v", got, tt.want)
			}
		})
	}
}
