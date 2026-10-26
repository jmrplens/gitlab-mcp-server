package tools_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrchanges"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mrdraftnotes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestMRReview_InlinePositionsAcrossDiffPages exercises the served tools,
// including their pre-check and actual writes, against a 201-file MR.
func TestMRReview_InlinePositionsAcrossDiffPages(t *testing.T) {
	tools := []struct {
		name, method, path, bodyField, response string
	}{
		{"gitlab_mr_discussion_create", http.MethodPost, "/discussions", "body", `{"id":"thread","notes":[{"id":10,"body":"Keep this text"}]}`},
		{"gitlab_mr_draft_note_create", http.MethodPost, "/draft_notes", "note", `{"id":10,"note":"Keep this text"}`},
		{"gitlab_mr_draft_note_update", http.MethodPut, "/draft_notes/10", "note", `{"id":10,"note":"Keep this text"}`},
	}
	positions := []struct {
		name, path, oldPath string
		newLine, oldLine    int
	}{
		{"FirstPageAdded", "file-000.go", "file-000.go", 2, 0},
		{"SecondPageRemoved", "file-100.go", "file-100.go", 0, 2},
		{"ThirdPageRenamedContext", "renamed.go", "file-200.go", 1, 1},
	}
	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			for _, position := range positions {
				t.Run(position.name, func(t *testing.T) {
					var pages []int
					var writes []map[string]any
					session := newMRPaginationSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/api/v4/projects/42/merge_requests/1/diffs" {
							pages = append(pages, respondMRDiffPage(w, r))
							return
						}
						if r.Method != tool.method || r.URL.Path != "/api/v4/projects/42/merge_requests/1"+tool.path {
							http.NotFound(w, r)
							return
						}
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						writes = append(writes, body)
						testutil.RespondJSON(w, http.StatusOK, tool.response)
					}))
					pos := map[string]any{
						"base_sha": strings.Repeat("a", 40), "start_sha": strings.Repeat("b", 40),
						"head_sha": strings.Repeat("c", 40), "position_type": "text",
						"new_path": position.path, "old_path": position.oldPath,
					}
					if position.newLine != 0 {
						pos["new_line"] = position.newLine
					}
					if position.oldLine != 0 {
						pos["old_line"] = position.oldLine
					}
					args := map[string]any{
						"project_id": "42", "merge_request_iid": 1,
						tool.bodyField: "Keep this text", "position": pos,
					}
					if tool.method == http.MethodPut {
						args["note_id"] = 10
					}

					result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool.name, Arguments: args})

					if err != nil || result.IsError {
						t.Fatalf("CallTool() = (%+v, %v), want a successful inline write", result, err)
					}
					if !slices.Equal(pages, []int{1, 2, 3}) {
						t.Errorf("diff pages = %v, want [1 2 3]", pages)
					}
					if len(writes) != 1 {
						t.Fatalf("writes = %d, want 1", len(writes))
					}
					if writes[0][tool.bodyField] != "Keep this text" {
						t.Errorf("written body = %v", writes[0])
					}
					expected, err := json.Marshal(pos)
					if err != nil {
						t.Fatal(err)
					}
					var want map[string]any
					if err := json.Unmarshal(expected, &want); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(writes[0]["position"], want) {
						t.Errorf("written position = %#v, want %#v", writes[0]["position"], want)
					}
				})
			}
		})
	}
}

// TestMRReview_InvalidPositionAfterCompleteListing keeps real validation
// failures separate from a valid file that was absent from the first page.
func TestMRReview_InvalidPositionAfterCompleteListing(t *testing.T) {
	cases := []struct {
		name, path string
		line       int
	}{
		{"MissingFile", "missing.go", 2},
		{"OutsideDiff", "renamed.go", 99},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pages []int
			session := newMRPaginationSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v4/projects/42/merge_requests/1/diffs" {
					pages = append(pages, respondMRDiffPage(w, r))
					return
				}
				t.Error("invalid position reached the write endpoint")
				w.WriteHeader(http.StatusBadRequest)
			}))

			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name: "gitlab_mr_draft_note_create",
				Arguments: map[string]any{
					"project_id": "42", "merge_request_iid": 1, "note": "Invalid",
					"position": map[string]any{
						"base_sha": strings.Repeat("a", 40), "start_sha": strings.Repeat("b", 40),
						"head_sha": strings.Repeat("c", 40), "new_path": tc.path, "new_line": tc.line,
					},
				},
			})

			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatal("invalid position was accepted")
			}
			if !slices.Equal(pages, []int{1, 2, 3}) {
				t.Errorf("diff pages = %v, want [1 2 3]", pages)
			}
		})
	}
}

// TestMRReview_ChangesPagination follows only the continuation the tool
// publishes, so omission of pagination cannot silently read the first page.
func TestMRReview_ChangesPagination(t *testing.T) {
	session := newMRPaginationSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondMRDiffPage(w, r)
	}))
	page, count := int64(1), 0
	var pages []int64
	// sequential: each page number is supplied by the preceding tool response.
	for {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "gitlab_mr_changes_get",
			Arguments: map[string]any{"project_id": "42", "merge_request_iid": 1, "page": page, "per_page": 100},
		})
		if err != nil || result.IsError {
			t.Fatalf("CallTool() = (%+v, %v)", result, err)
		}
		body, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out mrchanges.Output
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		pages = append(pages, out.Pagination.Page)
		count += len(out.Changes)
		if !out.Pagination.HasMore {
			if out.Pagination.NextPage != 0 {
				t.Errorf("last next_page = %d, want 0", out.Pagination.NextPage)
			}
			break
		}
		if out.Pagination.NextPage <= page {
			t.Fatalf("next_page = %d does not advance page %d", out.Pagination.NextPage, page)
		}
		page = out.Pagination.NextPage
	}

	if count != 201 || !slices.Equal(pages, []int64{1, 2, 3}) {
		t.Errorf("listing = (%d files, pages %v), want (201, [1 2 3])", count, pages)
	}
}

// newMRPaginationSession connects an MCP client to the actual individual
// projections of the four review actions this regression exercises.
func newMRPaginationSession(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "pagination-test", Version: "1"}, nil)
	specs := append(mrchanges.ActionSpecs(client), mrdiscussions.ActionSpecs(client)...)
	specs = append(specs, mrdraftnotes.ActionSpecs(client)...)
	for _, spec := range specs {
		toolutil.RegisterSurfaceToolFromSpec(server, spec, toolutil.SurfaceToolRegisterOptions{
			Description: spec.IndividualTool.Description,
		})
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "pagination-client", Version: "1"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// respondMRDiffPage serves 201 file diffs in offset pages with no totals,
// matching GitLab instances that publish only their next-page headers.
func respondMRDiffPage(w http.ResponseWriter, r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 || page > 3 || r.URL.Query().Get("per_page") != "100" {
		http.Error(w, "unexpected pagination", http.StatusBadRequest)
		return 0
	}
	var files []map[string]any
	for i := (page - 1) * 100; i < min(page*100, 201); i++ {
		path := fmt.Sprintf("file-%03d.go", i)
		nextPath := path
		if i == 200 {
			nextPath = "renamed.go"
		}
		files = append(files, map[string]any{
			"old_path": path, "new_path": nextPath, "renamed_file": i == 200,
			"diff": "@@ -1,2 +1,2 @@\n context\n-old\n+new\n",
		})
	}
	body, err := json.Marshal(files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return page
	}
	next := ""
	if page < 3 {
		next = strconv.Itoa(page + 1)
	}
	testutil.RespondJSONWithPagination(w, http.StatusOK, string(body), testutil.PaginationHeaders{
		Page: strconv.Itoa(page), PerPage: "100", NextPage: next,
	})
	return page
}
