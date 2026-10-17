// search_params_test.go holds the two code search parameters client-go's
// SearchOptions does not model, num_context_lines and regex, to what GitLab
// declares for them: each reaches the query of every scope exactly as the
// caller set it and is absent when the caller did not, a context outside the
// range GitLab accepts is refused before any request, GitLab's refusal of a
// parameter on a backend that does not take it reaches the caller naming the
// backend it needs, and the backend that answered is kept for the Markdown.
package search

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// searchTypeHeader is the response header GitLab names the backend that
// answered a search with (lib/api/search.rb, set_headers).
const searchTypeHeader = "X-Search-Type"

// oneBlobBody is a code search answer holding one content match.
const oneBlobBody = `[{"basename":"main","data":"func main()\n","path":"main.go","filename":"main.go","ref":"main","startline":4,"project_id":42}]`

// TestSearchCode_ContextAndRegex_ReachEveryScopeAsSet verifies that a code
// search carrying num_context_lines and regex sends both to GitLab at project,
// group and global scope with the values the caller set, the zero context and
// regex=false included, since GitLab reads either as a choice and not as an
// absence; and that a search setting neither sends neither key, because
// GitLab Enterprise refuses even regex=false on every backend but exact
// search.
func TestSearchCode_ContextAndRegex_ReachEveryScopeAsSet(t *testing.T) {
	scopes := []struct {
		name  string
		path  string
		input CodeInput
	}{
		{name: "project", path: pathSearchProject, input: CodeInput{ProjectID: "42"}},
		{name: "group", path: pathSearchGroup, input: CodeInput{GroupID: "7"}},
		{name: "global", path: pathSearchGlobal},
	}
	values := []struct {
		name         string
		contextLines *int64
		regex        *bool
		wantContext  string
		wantRegex    string
	}{
		{name: "neither"},
		{name: "most context and a regex", contextLines: new(int64(20)), regex: new(true), wantContext: "20", wantRegex: "true"},
		{name: "no context and a literal query", contextLines: new(int64(0)), regex: new(false), wantContext: "0", wantRegex: "false"},
		{name: "context alone", contextLines: new(int64(5)), wantContext: "5"},
		{name: "regex alone", regex: new(true), wantRegex: "true"},
	}
	for _, scope := range scopes {
		for _, v := range values {
			t.Run(scope.name+"/"+v.name, func(t *testing.T) {
				input := scope.input
				input.NumContextLines = v.contextLines
				input.Regex = v.regex
				assertCodeQuerySent(t, scope.path, input, v.wantContext, v.wantRegex)
			})
		}
	}
}

// assertCodeQuerySent runs a code search for "func main" with input against a
// GitLab that expects it at path, and checks that the query carries
// num_context_lines and regex as wantContext and wantRegex, an empty want
// meaning the key must be absent.
func assertCodeQuerySent(t *testing.T, path string, input CodeInput, wantContext, wantRegex string) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("request went to %s, want %s", r.URL.Path, path)
		}
		testutil.AssertQueryParam(t, r, queryScope, scopeBlobs)
		testutil.AssertQueryParam(t, r, "search", "func main")
		assertSentOnlyWhenSet(t, r.URL.Query(), "num_context_lines", wantContext)
		assertSentOnlyWhenSet(t, r.URL.Query(), "regex", wantRegex)
		testutil.RespondJSONWithPagination(w, http.StatusOK, oneBlobBody, defaultPagination)
	}))
	input.Query = "func main"
	out, err := Code(context.Background(), client, input)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Blobs) != 1 {
		t.Fatalf(fmtLenBlobsWant1, len(out.Blobs))
	}
}

// assertSentOnlyWhenSet checks that query carries key with the value want when
// want is set, and does not carry key at all when it is not.
func assertSentOnlyWhenSet(t *testing.T, query url.Values, key, want string) {
	t.Helper()
	got, sent := query[key]
	if sent != (want != "") || (sent && got[0] != want) {
		t.Errorf("query %s = %q (sent %v), want %q", key, got, sent, want)
	}
}

// TestSearchCode_ContextOutOfRange_RefusedBeforeAnyRequest verifies that a
// num_context_lines below 0 or above 20 is refused naming the range GitLab
// accepts, before a request is made: GitLab Enterprise refuses it as an
// invalid value, while Community Edition drops the parameter and would answer
// as if the caller had asked for nothing.
func TestSearchCode_ContextOutOfRange_RefusedBeforeAnyRequest(t *testing.T) {
	for _, n := range []int64{-1, 21} {
		t.Run(strconv.FormatInt(n, 10), func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("request sent to %s for a refused num_context_lines", r.URL.Path)
				testutil.RespondJSON(w, http.StatusOK, "[]")
			}))
			_, err := Code(context.Background(), client, CodeInput{ProjectID: "42", Query: "x", NumContextLines: new(n)})
			if err == nil {
				t.Fatalf("num_context_lines %d was accepted", n)
			}
			if !strings.Contains(err.Error(), "num_context_lines") || !strings.Contains(err.Error(), "between 0 and 20") {
				t.Errorf("refusal = %q, want it to name num_context_lines and the range 0 to 20", err)
			}
		})
	}
}

// regexpParseRefusal is the body GitLab.com answered an exact code search for
// `WikiBlobsByProject(` with on 2026-10-17: exact search reads the query as a
// regular expression when regex is left out, and Zoekt cannot parse one with
// an unclosed parenthesis.
const regexpParseRefusal = "400 Bad request - bad request: error building search request: failed to convert raw request to SearchRequest: failed to convert and child 0 to proto: failed to parse search query: error parsing regexp: missing closing ): `WikiBlobsByProject(`"

// TestSearchCode_BackendRefusesParameter_HintNamesTheBackendItNeeds verifies
// that GitLab's refusal of regex or num_context_lines on a backend that does
// not take it, the 400 ee/lib/ee/api/search.rb answers with the message of
// BLOB_SEARCH_PARAM_RULES, reaches the caller with GitLab's message and a hint
// naming the backend the parameter needs as search_type takes it. GitLab
// checks regex first and stops at the first refusal, so a call that sent both
// is told about both, or leaving regex out would meet the second. A project
// search of a ref other than the default branch runs basic search whatever
// search_type says (ee/app/services/ee/search/project_service.rb), so a call
// that named a project and a ref is told so, since retrying with the backend
// the hint names would be refused the same way. Exact search's refusal of a
// query it cannot parse as a regular expression is answered with regex=false,
// and any other 400 keeps the general hint about the query and the backend.
func TestSearchCode_BackendRefusesParameter_HintNamesTheBackendItNeeds(t *testing.T) {
	const (
		regexRefusal   = "regex supported only with exact code search"
		contextRefusal = "num_context_lines supported only with advanced and exact code search"
		refClause      = "a ref other than the default branch runs basic search"
	)
	tests := []struct {
		name    string
		message string
		input   CodeInput
		want    []string
		notWant []string
	}{
		{
			name:    "regex on basic or advanced search",
			message: regexRefusal,
			input:   CodeInput{ProjectID: "42", Query: "x", Regex: new(true)},
			want:    []string{regexRefusal, "regex needs exact code search", "search_type zoekt", "leave regex out"},
			notWant: []string{"advanced", "num_context_lines", refClause},
		},
		{
			name:    "regex refused where num_context_lines was sent too",
			message: regexRefusal,
			input:   CodeInput{ProjectID: "42", Query: "x", Regex: new(false), NumContextLines: new(int64(5))},
			want:    []string{regexRefusal, "regex needs exact code search", "num_context_lines advanced or exact code search", "search_type zoekt", "leave regex out, and num_context_lines with it on basic search"},
			notWant: []string{refClause},
		},
		{
			name:    "num_context_lines on basic search",
			message: contextRefusal,
			input:   CodeInput{ProjectID: "42", Query: "x", NumContextLines: new(int64(5))},
			want:    []string{contextRefusal, "num_context_lines needs advanced or exact code search", "search_type advanced or zoekt"},
			notWant: []string{refClause},
		},
		{
			name:    "regex on a project search of another ref",
			message: regexRefusal,
			input:   CodeInput{ProjectID: "42", Query: "x", Ref: "feature", Regex: new(false), SearchType: "zoekt"},
			want:    []string{regexRefusal, "regex needs exact code search", refClause, "whatever search_type says", "leave ref out to search the default branch"},
		},
		{
			name:    "num_context_lines on a project search of another ref",
			message: contextRefusal,
			input:   CodeInput{ProjectID: "42", Query: "x", Ref: "feature", NumContextLines: new(int64(5)), SearchType: "advanced"},
			want:    []string{contextRefusal, "num_context_lines needs advanced or exact code search", refClause, "leave ref out to search the default branch"},
		},
		{
			name:    "a ref on a group search changes no backend",
			message: contextRefusal,
			input:   CodeInput{GroupID: "7", Query: "x", Ref: "feature", NumContextLines: new(int64(5))},
			want:    []string{contextRefusal, "num_context_lines needs advanced or exact code search"},
			notWant: []string{refClause},
		},
		{
			name:    "a query exact search cannot parse as a regular expression",
			message: regexpParseRefusal,
			input:   CodeInput{ProjectID: "42", Query: "WikiBlobsByProject(", SearchType: "zoekt"},
			want:    []string{"error parsing regexp", "exact code search reads the query as a regular expression", "regex=false"},
			notWant: []string{"check query and scope parameters"},
		},
		{
			name:    "any other refusal",
			message: "Scope supported only with advanced search or exact code search",
			input:   CodeInput{GroupID: "7", Query: "x"},
			want:    []string{"check query and scope parameters"},
			notWant: []string{"needs exact code search", "needs advanced or exact code search", "regex=false"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"`+tt.message+`"}`)
			}))
			_, err := Code(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("a refused search returned no error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not say %q", err, want)
				}
			}
			for _, unwanted := range tt.notWant {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("error %q says %q", err, unwanted)
				}
			}
		})
	}
}

// TestSearchCode_Answer_RecordsTheBackendAndTheContextAsked verifies that the
// handler keeps, for the Markdown, the backend GitLab says answered (the
// X-Search-Type header, empty when GitLab sent none) and the context the
// request asked for, which is what decides where each snippet's match is.
func TestSearchCode_Answer_RecordsTheBackendAndTheContextAsked(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		context *int64
	}{
		{name: "advanced with a context", header: "advanced", context: new(int64(12))},
		{name: "exact with the default context", header: "zoekt"},
		{name: "no header", header: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.header != "" {
					w.Header().Set(searchTypeHeader, tt.header)
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, oneBlobBody, defaultPagination)
			}))
			out, err := Code(context.Background(), client, CodeInput{ProjectID: "42", Query: "func main", NumContextLines: tt.context})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if out.snippets.backend != tt.header {
				t.Errorf("backend = %q, want %q", out.snippets.backend, tt.header)
			}
			if got := out.snippets.contextLines; (got == nil) != (tt.context == nil) || (got != nil && *got != *tt.context) {
				t.Errorf("context = %s, want the input's %s", contextText(got), contextText(tt.context))
			}
		})
	}
}

// TestCodeRefusalHint_NotAGitLabRefusal_ExplainsNothing verifies that an error
// GitLab did not answer, which carries no message of GitLab's, gets no code
// search hint, so the hints every search shares are what it keeps.
func TestCodeRefusalHint_NotAGitLabRefusal_ExplainsNothing(t *testing.T) {
	refusal := codeRefusal{contextLines: true, projectRef: true}
	if got := refusal.hint(errors.New("connection reset by peer")); got != "" {
		t.Errorf("hint = %q, want none for an error GitLab did not answer", got)
	}
	glErr := &gl.ErrorResponse{Message: "{message: regex supported only with exact code search}"}
	if got := refusal.hint(fmt.Errorf("searchCode: %w", glErr)); got != regexAndContextBackendHint+refBackendClause {
		t.Errorf("hint for a wrapped refusal = %q, want the regex and context hint with the ref clause", got)
	}
}

// TestScopedSearchWrap_OnlyACodeSearch400_TakesTheCodeHint verifies which hint
// a failed search returns: a 400 the code search's refusal hint explains takes
// that hint, the same message under any other status keeps the error every
// search returns, and a search with no refusal hint of its own, such as a
// merge request search, keeps the 400 hint every search shares.
func TestScopedSearchWrap_OnlyACodeSearch400_TakesTheCodeHint(t *testing.T) {
	const (
		regexRefusal = "regex supported only with exact code search"
		sharedHint   = "check query and scope parameters"
	)
	codeHint := newCodeRefusal(CodeInput{Query: "x"}).hint
	tests := []struct {
		name    string
		hint    func(error) string
		status  int
		message string
		want    string
		notWant string
	}{
		{name: "code search, 400 the hint explains", hint: codeHint, status: http.StatusBadRequest, message: regexRefusal, want: regexBackendHint},
		{name: "code search, the same message under 500", hint: codeHint, status: http.StatusInternalServerError, message: regexRefusal, want: regexRefusal, notWant: regexBackendHint},
		{name: "code search, 400 the hint does not explain", hint: codeHint, status: http.StatusBadRequest, message: "scope is invalid", want: sharedHint, notWant: regexBackendHint},
		{name: "search with no refusal hint, 400", status: http.StatusBadRequest, message: regexRefusal, want: sharedHint, notWant: regexBackendHint},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			glErr := &gl.ErrorResponse{
				Response: &http.Response{
					StatusCode: tt.status,
					Request:    &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/api/v4/search"}},
				},
				Message: tt.message,
			}
			args := scopedSearchArgs[*gl.Blob]{operation: "searchCode", refusalHint: tt.hint}
			got := args.wrap(glErr).Error()
			if !strings.Contains(got, tt.want) {
				t.Errorf("error = %q, want it to contain %q", got, tt.want)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Errorf("error = %q, want it not to contain %q", got, tt.notWant)
			}
		})
	}
}

// contextText writes a context the way a failure message reads it: its value,
// or "none" when the request asked for none.
func contextText(contextLines *int64) string {
	if contextLines == nil {
		return "none"
	}
	return strconv.FormatInt(*contextLines, 10)
}

// TestSearchBackend_NoResponse_NamesNoBackend verifies that a search with no
// response to read names no backend rather than failing.
func TestSearchBackend_NoResponse_NamesNoBackend(t *testing.T) {
	if got := searchBackend(nil); got != "" {
		t.Errorf("searchBackend(nil) = %q, want none", got)
	}
	resp := &gl.Response{Response: &http.Response{Header: http.Header{searchTypeHeader: []string{"basic"}}}}
	if got := searchBackend(resp); got != "basic" {
		t.Errorf("searchBackend = %q, want basic", got)
	}
}

// TestCodeSearchParams_SetAndUnset_EncodesOnlyWhatIsSet verifies the request
// option on its own: it adds each parameter the caller set to the query
// client-go encoded, keeps what was there, and changes nothing when neither is
// set.
func TestCodeSearchParams_SetAndUnset_EncodesOnlyWhatIsSet(t *testing.T) {
	tests := []struct {
		name         string
		contextLines *int64
		regex        *bool
		want         string
	}{
		{name: "neither", want: "scope=blobs&search=x"},
		{name: "both", contextLines: new(int64(3)), regex: new(false), want: "num_context_lines=3&regex=false&scope=blobs&search=x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := retryablehttp.NewRequest(http.MethodGet, "https://gitlab.example.com/api/v4/search?scope=blobs&search=x", nil)
			if err != nil {
				t.Fatalf("building the request: %v", err)
			}
			if err = codeSearchParams(tt.contextLines, tt.regex)(req); err != nil {
				t.Fatalf("option error = %v", err)
			}
			if req.URL.RawQuery != tt.want {
				t.Errorf("query = %q, want %q", req.URL.RawQuery, tt.want)
			}
		})
	}
}

// TestSearchInputSchema_ContextAndRegex_PublishTheBoundAndTheBackends verifies
// that the code search schema publishes num_context_lines as an integer
// bounded 0 to 20, as GitLab bounds it, and regex as a boolean, each described
// with the backend that honors it, so a client can refuse a value before
// sending it and a model can choose the backend.
func TestSearchInputSchema_ContextAndRegex_PublishTheBoundAndTheBackends(t *testing.T) {
	schema := schemaMapFromAny(t, searchInputSchemaMap[CodeInput]())
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %#v", schema)
	}
	contextLines, ok := properties["num_context_lines"].(map[string]any)
	if !ok {
		t.Fatalf("num_context_lines missing from %v", properties)
	}
	if fmt.Sprint(contextLines["type"]) != "[null integer]" || contextLines["minimum"] != float64(0) || contextLines["maximum"] != float64(20) {
		t.Errorf("num_context_lines = %v, want an integer from 0 to 20, null standing for none", contextLines)
	}
	regex, ok := properties["regex"].(map[string]any)
	if !ok {
		t.Fatalf("regex missing from %v", properties)
	}
	if fmt.Sprint(regex["type"]) != "[null boolean]" {
		t.Errorf("regex = %v, want a boolean, null standing for none", regex)
	}
	for name, want := range map[string][]string{
		"num_context_lines": {
			"up to this many on each side", "advanced and exact code search", "search_type advanced or zoekt",
			"exact search only from GitLab 19.5", "GitLab Enterprise 18.11 and later refuses it on basic search",
			"ref other than the default branch", "mcp scope", "Community Edition ignore it",
		},
		"regex": {
			"exact code search", "search_type zoekt", "regular expression", "GitLab Enterprise 18.9 and later refuses it",
			"ref other than the default branch", "mcp scope", "Community Edition ignore it",
		},
	} {
		t.Run("description of "+name, func(t *testing.T) {
			description := fmt.Sprint(properties[name].(map[string]any)["description"])
			for _, phrase := range want {
				if !strings.Contains(description, phrase) {
					t.Errorf("%s description %q does not say %q", name, description, phrase)
				}
			}
		})
	}
	for _, other := range []string{"num_context_lines", "regex"} {
		t.Run("not on the merge request search/"+other, func(t *testing.T) {
			props := schemaMapFromAny(t, searchInputSchemaMap[MergeRequestsInput]())["properties"].(map[string]any)
			if _, found := props[other]; found {
				t.Errorf("merge request search publishes %s, which only code search sends", other)
			}
		})
	}
}

// codeClient is a client whose GitLab answers every code search with one
// content match.
func codeClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, oneBlobBody, defaultPagination)
	}))
}

// TestSearchCode_ContextAtTheBounds_Accepted verifies that 0 and 20, the two
// ends of the range GitLab accepts, are sent rather than refused.
func TestSearchCode_ContextAtTheBounds_Accepted(t *testing.T) {
	for _, n := range []int64{0, 20} {
		t.Run(strconv.FormatInt(n, 10), func(t *testing.T) {
			if _, err := Code(context.Background(), codeClient(t), CodeInput{ProjectID: "42", Query: "x", NumContextLines: new(n)}); err != nil {
				t.Errorf("num_context_lines %d refused: %v", n, err)
			}
		})
	}
}
