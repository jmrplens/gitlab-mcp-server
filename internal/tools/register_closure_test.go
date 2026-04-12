// register_closure_test.go contains integration tests that verify every tool
// closure in register.go works end-to-end through a full MCP session.
// A route-aware mock HTTP handler serves responses for all GitLab API endpoints.
package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// pathReleases is the URL path segment for release endpoints.
	pathReleases = "/releases/"
	// pathDiscussions is the URL path segment for discussion endpoints.
	pathDiscussions = "/discussions"
	// pathNotes is the URL path segment for note endpoints.
	pathNotes = "/notes"
	// suffixIssues is the URL path segment for issue endpoints.
	suffixIssues = "/issues"
)

// mockBodies holds all JSON response bodies used by the mock GitLab API handler.
type mockBodies struct {
	project, branch, protectedBranch, tag string
	release, releaseLink                  string
	mr, mrNote, discussion, mrChanges     string
	commit, file                          string
	issue, issueNote                      string
}

// newMockBodies returns a freshly populated mockBodies with valid JSON
// response payloads for all supported GitLab API entities.
func newMockBodies() mockBodies {
	return mockBodies{
		project:         `{"id":42,"name":"test","path_with_namespace":"ns/test","visibility":"private","web_url":"https://example.com/ns/test","description":"desc","default_branch":"main","namespace":{"id":1,"name":"ns","path":"ns","full_path":"ns"}}`,
		branch:          `{"name":"dev","merged":false,"protected":false,"default":false,"web_url":"https://example.com","commit":{"id":"abc123","short_id":"abc1","title":"init","message":"init","author_name":"test"}}`,
		protectedBranch: `{"id":1,"name":"main","push_access_levels":[{"access_level":40}],"merge_access_levels":[{"access_level":40}],"allow_force_push":false}`,
		tag:             `{"name":"v1.0","message":"tag","target":"abc123","commit":{"id":"abc123","short_id":"abc1","title":"init","message":"init","author_name":"test"}}`,
		release:         `{"tag_name":"v1.0","name":"v1.0","description":"notes","created_at":"2024-01-01T00:00:00Z","released_at":"2024-01-01T00:00:00Z","author":{"username":"test"},"commit":{"id":"abc123"},"assets":{"links":[]}}`,
		releaseLink:     `{"id":1,"name":"bin","url":"https://example.com/bin","link_type":"package"}`,
		mr:              `{"id":1,"iid":1,"title":"MR","state":"opened","source_branch":"dev","target_branch":"main","web_url":"https://example.com/mr/1","author":{"username":"test"},"description":"d","labels":[],"assignees":[],"reviewers":[],"detailed_merge_status":"mergeable","has_conflicts":false,"changes_count":"1"}`,
		mrNote:          `{"id":1,"body":"note","author":{"username":"test"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z","system":false,"resolvable":false}`,
		discussion:      `{"id":"abc","individual_note":false,"notes":[{"id":1,"body":"disc","author":{"username":"test"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z","system":false,"resolvable":true,"resolved":false}]}`,
		mrChanges:       `{"id":1,"iid":1,"title":"MR","state":"opened","changes":[{"old_path":"a.go","new_path":"a.go","diff":"@@ -1 +1 @@\\n-old\\n+new","new_file":false,"renamed_file":false,"deleted_file":false}]}`,
		commit:          `{"id":"abc123","short_id":"abc1","title":"msg","message":"msg","author_name":"test","author_email":"t@e.com","created_at":"2024-01-01T00:00:00Z","web_url":"https://example.com/c/abc","stats":{"additions":1,"deletions":0,"total":1}}`,
		file:            `{"file_name":"README.md","file_path":"README.md","size":100,"encoding":"base64","content_sha256":"abc","ref":"main","blob_id":"def","commit_id":"abc123","last_commit_id":"abc123","content":"SGVsbG8="}`,
		issue:           `{"id":1,"iid":10,"title":"Test issue","description":"desc","state":"opened","labels":["bug"],"assignees":[{"username":"alice"}],"milestone":{"title":"v1.0"},"author":{"username":"test"},"web_url":"https://example.com/issues/10","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`,
		issueNote:       `{"id":1,"body":"note","author":{"username":"test"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z","system":false,"internal":false}`,
	}
}

// routeAwareMockHandler returns an HTTP handler that serves mock responses
// for every GitLab API endpoint used by the 52 tools.
func routeAwareMockHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	b := newMockBodies()
	return func(w http.ResponseWriter, r *http.Request) {
		if routeProjects(w, r, b) ||
			routeBranches(w, r, b) ||
			routeTags(w, r, b) ||
			routeReleases(w, r, b) ||
			routeMergeRequests(w, r, b) ||
			routeIssues(w, r, b) ||
			routeNotes(w, r, b) ||
			routeDiscussions(w, r, b) ||
			routeCommitsAndFiles(w, r, b) ||
			routeMembersAndGroups(w, r) ||
			routeUploads(w, r) {
			return
		}
		t.Logf("unhandled: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}
}

// projectPath42 is the URL path for project ID 42, used across route helpers.
const projectPath42 = "/api/v4/projects/42"

// routeProjects handles mock GitLab project API endpoints.
func routeProjects(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && p == "/api/v4/projects":
		respondJSON(w, http.StatusCreated, b.project)
	case r.Method == http.MethodGet && p == projectPath42:
		respondJSON(w, http.StatusOK, b.project)
	case r.Method == http.MethodGet && p == "/api/v4/projects":
		respondJSON(w, http.StatusOK, "["+b.project+"]")
	case r.Method == http.MethodDelete && p == projectPath42:
		w.WriteHeader(http.StatusAccepted)
	case r.Method == http.MethodPut && p == projectPath42:
		respondJSON(w, http.StatusOK, b.project)
	default:
		return false
	}
	return true
}

// routeBranches handles mock GitLab branch and protected branch API endpoints.
func routeBranches(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/repository/branches"):
		respondJSON(w, http.StatusCreated, b.branch)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/repository/branches"):
		respondJSON(w, http.StatusOK, "["+b.branch+"]")
	case r.Method == http.MethodPost && strings.Contains(p, "/protected_branches"):
		respondJSON(w, http.StatusCreated, b.protectedBranch)
	case r.Method == http.MethodDelete && strings.Contains(p, "/protected_branches/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/protected_branches"):
		respondJSON(w, http.StatusOK, "["+b.protectedBranch+"]")
	default:
		return false
	}
	return true
}

// routeTags handles mock GitLab tag API endpoints.
func routeTags(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/repository/tags"):
		respondJSON(w, http.StatusCreated, b.tag)
	case r.Method == http.MethodDelete && strings.Contains(p, "/repository/tags/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/repository/tags"):
		respondJSON(w, http.StatusOK, "["+b.tag+"]")
	default:
		return false
	}
	return true
}

// routeReleases handles mock GitLab release and asset link API endpoints.
func routeReleases(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/releases"):
		respondJSON(w, http.StatusCreated, b.release)
	case r.Method == http.MethodPut && strings.Contains(p, pathReleases):
		respondJSON(w, http.StatusOK, b.release)
	case r.Method == http.MethodDelete && strings.Contains(p, pathReleases) && !strings.Contains(p, "/assets/"):
		respondJSON(w, http.StatusOK, b.release)
	case r.Method == http.MethodGet && strings.Contains(p, pathReleases) && !strings.Contains(p, "/assets/"):
		respondJSON(w, http.StatusOK, b.release)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/releases"):
		respondJSON(w, http.StatusOK, "["+b.release+"]")
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/assets/links"):
		respondJSON(w, http.StatusCreated, b.releaseLink)
	case r.Method == http.MethodDelete && strings.Contains(p, "/assets/links/"):
		respondJSON(w, http.StatusOK, b.releaseLink)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/assets/links"):
		respondJSON(w, http.StatusOK, "["+b.releaseLink+"]")
	default:
		return false
	}
	return true
}

// routeMergeRequests handles mock GitLab merge request API endpoints.
func routeMergeRequests(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	hasMR1 := strings.Contains(p, "/merge_requests/1")
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/merge_requests"):
		respondJSON(w, http.StatusCreated, b.mr)
	case r.Method == http.MethodGet && hasMR1 && !strings.Contains(p, pathNotes) && !strings.Contains(p, pathDiscussions) && !strings.Contains(p, "/changes"):
		respondJSON(w, http.StatusOK, b.mr)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/merge_requests"):
		respondJSON(w, http.StatusOK, "["+b.mr+"]")
	case r.Method == http.MethodPut && hasMR1 && !strings.Contains(p, "/merge") && !strings.Contains(p, pathDiscussions):
		respondJSON(w, http.StatusOK, b.mr)
	case r.Method == http.MethodPut && strings.HasSuffix(p, "/merge"):
		respondJSON(w, http.StatusOK, b.mr)
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/approve"):
		respondJSON(w, http.StatusOK, `{}`)
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/unapprove"):
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && strings.Contains(p, "/changes"):
		respondJSON(w, http.StatusOK, b.mrChanges)
	default:
		return false
	}
	return true
}

// routeNotes handles mock GitLab note (comment) API endpoints for merge requests.
func routeNotes(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	if !strings.Contains(p, pathNotes) || strings.Contains(p, pathDiscussions) {
		return false
	}
	switch r.Method {
	case http.MethodPost:
		respondJSON(w, http.StatusCreated, b.mrNote)
	case http.MethodGet:
		respondJSON(w, http.StatusOK, "["+b.mrNote+"]")
	case http.MethodPut:
		respondJSON(w, http.StatusOK, b.mrNote)
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		return false
	}
	return true
}

// routeDiscussions handles mock GitLab discussion thread API endpoints.
func routeDiscussions(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	if !strings.Contains(p, pathDiscussions) {
		return false
	}
	switch {
	case r.Method == http.MethodPost && !strings.Contains(p, pathNotes):
		respondJSON(w, http.StatusCreated, b.discussion)
	case r.Method == http.MethodPut:
		respondJSON(w, http.StatusOK, b.discussion)
	case r.Method == http.MethodPost && strings.Contains(p, pathNotes):
		respondJSON(w, http.StatusCreated, b.mrNote)
	case r.Method == http.MethodGet:
		respondJSON(w, http.StatusOK, "["+b.discussion+"]")
	default:
		return false
	}
	return true
}

// routeCommitsAndFiles handles mock GitLab commit and repository file API endpoints.
func routeCommitsAndFiles(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/repository/commits"):
		respondJSON(w, http.StatusCreated, b.commit)
	case r.Method == http.MethodGet && strings.Contains(p, "/repository/files/"):
		respondJSON(w, http.StatusOK, b.file)
	default:
		return false
	}
	return true
}

// routeMembersAndGroups handles mock GitLab member and group API endpoints.
func routeMembersAndGroups(w http.ResponseWriter, r *http.Request) bool {
	p := r.URL.Path
	member := `{"id":1,"username":"jdoe","name":"John Doe","state":"active","access_level":30,"web_url":"https://gitlab.example.com/jdoe"}`
	group := `{"id":99,"name":"test-group","path":"test-group","full_path":"test-group","description":"","visibility":"private","web_url":"https://gitlab.example.com/groups/test-group"}`
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/members/all"):
		respondJSON(w, http.StatusOK, "["+member+"]")
	case r.Method == http.MethodGet && p == "/api/v4/groups":
		respondJSON(w, http.StatusOK, "["+group+"]")
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/v4/groups/") && strings.HasSuffix(p, "/descendant_groups"):
		respondJSON(w, http.StatusOK, "["+group+"]")
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/v4/groups/"):
		respondJSON(w, http.StatusOK, group)
	default:
		return false
	}
	return true
}

// routeIssues handles mock GitLab issue API endpoints.
func routeIssues(w http.ResponseWriter, r *http.Request, b mockBodies) bool {
	p := r.URL.Path
	if !strings.Contains(p, suffixIssues) {
		return false
	}
	hasIssueID := strings.Contains(p, "/issues/10")
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, suffixIssues):
		respondJSON(w, http.StatusCreated, b.issue)
	case r.Method == http.MethodGet && hasIssueID && strings.HasSuffix(p, pathNotes):
		respondJSON(w, http.StatusOK, "["+b.issueNote+"]")
	case r.Method == http.MethodPost && hasIssueID && strings.HasSuffix(p, pathNotes):
		respondJSON(w, http.StatusCreated, b.issueNote)
	case r.Method == http.MethodGet && hasIssueID:
		respondJSON(w, http.StatusOK, b.issue)
	case r.Method == http.MethodGet && strings.HasSuffix(p, suffixIssues):
		respondJSON(w, http.StatusOK, "["+b.issue+"]")
	case r.Method == http.MethodPut && hasIssueID:
		respondJSON(w, http.StatusOK, b.issue)
	case r.Method == http.MethodDelete && hasIssueID:
		w.WriteHeader(http.StatusNoContent)
	default:
		return false
	}
	return true
}

// routeUploads handles mock GitLab project upload API endpoints.
func routeUploads(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/uploads") {
		respondJSON(w, http.StatusCreated, `{"alt":"file","url":"/uploads/abc/file.png","full_path":"/g/p/uploads/abc/file.png","markdown":"![file](/uploads/abc/file.png)"}`)
		return true
	}
	return false
}

// TestRegisterAll_AllToolsThroughMCP exercises every tool closure in register.go
// by calling each of the 52 tools via an MCP session.
func TestRegisterAll_AllToolsThroughMCP(t *testing.T) {
	session := newMCPSession(t, routeAwareMockHandler(t))

	tools := []struct {
		name  string
		input map[string]any
	}{
		{"gitlab_project_create", map[string]any{"name": "test"}},
		{"gitlab_project_get", map[string]any{"project_id": "42"}},
		{"gitlab_project_list", map[string]any{}},
		{"gitlab_project_delete", map[string]any{"project_id": "42"}},
		{"gitlab_project_update", map[string]any{"project_id": "42", "name": "t2"}},
		{"gitlab_branch_create", map[string]any{"project_id": "42", "branch_name": "dev", "ref": "main"}},
		{"gitlab_branch_list", map[string]any{"project_id": "42"}},
		{"gitlab_branch_protect", map[string]any{"project_id": "42", "branch_name": "main"}},
		{"gitlab_branch_unprotect", map[string]any{"project_id": "42", "branch_name": "main"}},
		{"gitlab_protected_branches_list", map[string]any{"project_id": "42"}},
		{"gitlab_tag_create", map[string]any{"project_id": "42", "tag_name": "v1.0", "ref": "main"}},
		{"gitlab_tag_delete", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_tag_list", map[string]any{"project_id": "42"}},
		{"gitlab_release_create", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_release_update", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_release_delete", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_release_get", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_release_list", map[string]any{"project_id": "42"}},
		{"gitlab_release_link_create", map[string]any{"project_id": "42", "tag_name": "v1.0", "name": "bin", "url": "https://example.com/bin"}},
		{"gitlab_release_link_delete", map[string]any{"project_id": "42", "tag_name": "v1.0", "link_id": 1}},
		{"gitlab_release_link_list", map[string]any{"project_id": "42", "tag_name": "v1.0"}},
		{"gitlab_mr_create", map[string]any{"project_id": "42", "source_branch": "dev", "target_branch": "main", "title": "test"}},
		{"gitlab_mr_get", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_list", map[string]any{"project_id": "42"}},
		{"gitlab_mr_update", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_merge", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_approve", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_unapprove", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_note_create", map[string]any{"project_id": "42", "mr_iid": 1, "body": "test"}},
		{"gitlab_mr_notes_list", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_note_update", map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 1, "body": "upd"}},
		{"gitlab_mr_note_delete", map[string]any{"project_id": "42", "mr_iid": 1, "note_id": 1}},
		{"gitlab_mr_discussion_create", map[string]any{"project_id": "42", "mr_iid": 1, "body": "disc"}},
		{"gitlab_mr_discussion_resolve", map[string]any{"project_id": "42", "mr_iid": 1, "discussion_id": "abc", "resolved": true}},
		{"gitlab_mr_discussion_reply", map[string]any{"project_id": "42", "mr_iid": 1, "discussion_id": "abc", "body": "reply"}},
		{"gitlab_mr_discussion_list", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_mr_changes_get", map[string]any{"project_id": "42", "mr_iid": 1}},
		{"gitlab_commit_create", map[string]any{"project_id": "42", "branch": "main", "commit_message": "test", "actions": []map[string]any{{"action": "create", "file_path": "f.txt", "content": "x"}}}},
		{"gitlab_file_get", map[string]any{"project_id": "42", "file_path": "README.md", "ref": "main"}},
		{"gitlab_project_members_list", map[string]any{"project_id": "42"}},
		{"gitlab_group_list", map[string]any{}},
		{"gitlab_group_get", map[string]any{"group_id": "99"}},
		{"gitlab_group_members_list", map[string]any{"group_id": "99"}},
		{"gitlab_subgroups_list", map[string]any{"group_id": "99"}},
		{"gitlab_project_upload", map[string]any{"project_id": "42", "filename": "test.png", "content_base64": "aGVsbG8="}},
		{"gitlab_issue_create", map[string]any{"project_id": "42", "title": "Test issue"}},
		{"gitlab_issue_get", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_issue_list", map[string]any{"project_id": "42"}},
		{"gitlab_issue_update", map[string]any{"project_id": "42", "issue_iid": 10, "title": "Updated"}},
		{"gitlab_issue_delete", map[string]any{"project_id": "42", "issue_iid": 10}},
		{"gitlab_issue_note_create", map[string]any{"project_id": "42", "issue_iid": 10, "body": "note"}},
		{"gitlab_issue_note_list", map[string]any{"project_id": "42", "issue_iid": 10}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("marshal input: %v", err)
			}
			var params map[string]any
			if err = json.Unmarshal(inputJSON, &params); err != nil {
				t.Fatalf("unmarshal params: %v", err)
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      tt.name,
				Arguments: params,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("CallTool(%s) returned nil result", tt.name)
			}
		})
	}
}
