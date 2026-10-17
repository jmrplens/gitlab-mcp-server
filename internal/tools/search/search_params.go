package search

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// maxContextLines is the most context lines a code search can ask GitLab for
// on each side of a match. GitLab Enterprise declares num_context_lines with
// values 0..::Gitlab::Elastic::SearchResults::MAX_NUM_CONTEXT_LINES
// (ee/lib/ee/api/search.rb), which is 20 (ee/lib/gitlab/elastic/search_results.rb),
// and exact search clamps to the same ceiling (ee/lib/search/zoekt/params.rb).
const maxContextLines = 20

// The search backends as GitLab names them, both in the search_type parameter
// and in the X-Search-Type header it answers every search with: the values
// client-go's SearchType gives them, which allowedSearchTypes publishes.
const (
	backendBasic    = string(gl.BasicSearch)
	backendAdvanced = string(gl.AdvancedSearch)
	backendExact    = string(gl.ZoektSearch)
)

// The hints a refusal of num_context_lines or regex reaches the caller with.
// GitLab Enterprise refuses either parameter with 400 on a backend that does
// not take it, naming it in its message (BLOB_SEARCH_PARAM_RULES,
// ee/lib/ee/api/search.rb), and decides on the backend that would answer, so
// a search sent without search_type is refused on an instance whose default
// backend is the wrong one. It checks regex first and answers the first
// refusal alone, so a call that sent both is told about both, or leaving regex
// out would only meet the refusal of num_context_lines. A project search of a
// ref other than the default branch is answered by basic search whatever
// search_type says (ee/app/services/ee/search/project_service.rb, search_type
// and use_default_branch?), so such a call is told so as well, since retrying
// with the backend the hint names would be refused the same way.
const (
	regexBackendHint           = "regex needs exact code search: retry with search_type zoekt where the instance has exact code search, or leave regex out, since GitLab refuses it on any other backend, regex=false included"
	regexAndContextBackendHint = "regex needs exact code search, and num_context_lines advanced or exact code search: retry with search_type zoekt where the instance has exact code search, or leave regex out, and num_context_lines with it on basic search, since GitLab refuses each on a backend that does not take it, regex=false included"
	contextBackendHint         = "num_context_lines needs advanced or exact code search: retry with search_type advanced or zoekt where the instance has either, or leave num_context_lines out, since basic search refuses it"
	refBackendClause           = ", and a project search of a ref other than the default branch runs basic search whatever search_type says, so leave ref out to search the default branch"
)

// regexpParseHint is the hint for exact search's refusal of a query it cannot
// parse as a regular expression, which is how it reads every query over the
// API when regex is left out (search_mode, ee/lib/search/zoekt/search_results.rb):
// a literal "(" in the query is answered with Zoekt's "error parsing regexp".
const regexpParseHint = "exact code search reads the query as a regular expression unless regex=false is sent: send regex=false to search for the text as written, or escape the characters a regular expression gives a meaning to"

// validateCodeSearchParams refuses a num_context_lines outside the range
// GitLab accepts, before any request: GitLab Enterprise would refuse it as an
// invalid value, and Community Edition, which declares no such parameter,
// would drop it and answer as if nothing had been asked for.
func validateCodeSearchParams(contextLines *int64) error {
	if contextLines != nil && (*contextLines < 0 || *contextLines > maxContextLines) {
		return fmt.Errorf("searchCode: num_context_lines must be between 0 and %d, the range GitLab accepts, got %d", maxContextLines, *contextLines)
	}
	return nil
}

// codeSearchParams adds num_context_lines and regex to the query client-go
// encoded, which its SearchOptions does not model (upstream register row
// 132), the way [toolutil.PaginationRequestOption] adds a page to a method
// that takes no options for one. Each is sent only when the caller set it:
// GitLab Enterprise refuses either parameter on a backend that does not take
// it whatever its value, so an unset one has to stay absent rather than go
// out as its zero.
func codeSearchParams(contextLines *int64, regex *bool) gl.RequestOptionFunc {
	return func(req *retryablehttp.Request) error {
		query := req.URL.Query()
		if contextLines != nil {
			query.Set("num_context_lines", strconv.FormatInt(*contextLines, 10))
		}
		if regex != nil {
			query.Set("regex", strconv.FormatBool(*regex))
		}
		req.URL.RawQuery = query.Encode()
		return nil
	}
}

// searchBackend is the backend GitLab says answered a search, read from the
// X-Search-Type header every search route sets (lib/api/search.rb,
// set_headers), or the empty string when there is no response to read.
func searchBackend(resp *gl.Response) string {
	if resp == nil {
		return ""
	}
	return resp.Header.Get("X-Search-Type")
}

// codeRefusal is what a code search sent that decides how a 400 GitLab answers
// it with is explained.
type codeRefusal struct {
	// contextLines reports that the search sent num_context_lines.
	contextLines bool
	// projectRef reports a project search of a named ref, which GitLab
	// answers with basic search unless the ref is the default branch.
	projectRef bool
}

// newCodeRefusal is what input sends that a refusal's hint depends on.
func newCodeRefusal(input CodeInput) codeRefusal {
	return codeRefusal{
		contextLines: input.NumContextLines != nil,
		projectRef:   input.ProjectID != "" && input.Ref != "",
	}
}

// hint is the hint for a 400 GitLab answers the code search with, and the
// empty string for a refusal it does not explain: a backend that does not
// take a parameter the call sent, or a query exact search could not parse as
// a regular expression. It reads GitLab's own message, which is where it
// names the parameter or the parse error.
func (c codeRefusal) hint(err error) string {
	message := refusalMessage(err)
	if strings.Contains(message, "error parsing regexp") {
		return regexpParseHint
	}
	hint := c.backendHint(message)
	if hint != "" && c.projectRef {
		return hint + refBackendClause
	}
	return hint
}

// refusalMessage is the message GitLab refused a search with, as client-go
// parsed it out of the body, and the empty string for an error that carries
// none. It is read for matching alone and reaches no caller, so it is taken
// whole rather than through [toolutil.ExtractGitLabMessage], which drops a
// message that opens with the status text: GitLab's bad_request! writes every
// reason as "400 Bad request - <reason>", which is how exact search's parse
// error arrives.
func refusalMessage(err error) string {
	if refusal, ok := errors.AsType[*gl.ErrorResponse](err); ok {
		return refusal.Message
	}
	return ""
}

// backendHint is the hint for GitLab's refusal of a parameter the backend
// that would answer does not take, as message names it, and the empty string
// for any other message.
func (c codeRefusal) backendHint(message string) string {
	if strings.Contains(message, "regex supported only") {
		if c.contextLines {
			return regexAndContextBackendHint
		}
		return regexBackendHint
	}
	if strings.Contains(message, "num_context_lines supported only") {
		return contextBackendHint
	}
	return ""
}
