package prompts

import (
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestAuditCommitHygiene_Success verifies that audit_commit_hygiene summarizes
// Conventional Commit usage, merge commits, bodies, and linked work references.
func TestAuditCommitHygiene_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathRepoCompare, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("from") != "v1.0.0" {
			t.Errorf("expected from=v1.0.0, got %q", r.URL.Query().Get("from"))
		}
		respondJSON(w, http.StatusOK, `{
			"commits": [
				{"id":"abc123456789","title":"feat(api): add project prompt #12","message":"feat(api): add project prompt #12\n\nAdds a reusable prompt.","author_name":"Alice","parent_ids":["p1"]},
				{"id":"def123456789","title":"Merge branch 'feature' into main","message":"Merge branch 'feature' into main","author_name":"Bob","parent_ids":["p1","p2"]},
				{"id":"999999999999","title":"fix!: change auth flow","message":"fix!: change auth flow\n\nBREAKING CHANGE: token shape changed","author_name":"Carol","parent_ids":["p3"]}
			],
			"diffs": [],
			"compare_same_ref": false
		}`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "audit_commit_hygiene",
		Arguments: map[string]string{"project_id": "42", "from": "v1.0.0", "to": "main"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	checks := []string{
		"Commit Hygiene Audit: v1.0.0 → main",
		"Conventional titles | 2",
		"Merge commits | 1",
		"Breaking-change markers | 1",
		"Commit bodies/details present | 2",
		"Linked work references | 1",
		"needs title",
	}
	assertContainsAll(t, text, checks)
}

// TestAuditCommitHygiene_MissingArgs verifies required argument validation.
func TestAuditCommitHygiene_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "audit_commit_hygiene",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal("expected error for missing from")
	}
}

// TestMRDescriptionQuality_Success verifies that mr_description_quality reports
// description completeness signals and changed-file context.
func TestMRDescriptionQuality_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(pathMR5, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{
			"id":55,
			"iid":5,
			"title":"Improve login flow",
			"source_branch":"feature/login",
			"target_branch":"main",
			"description":"Closes #12\n\nThis updates the login flow with enough context for reviewers to understand the behavior change and the user impact.\n\nTests: go test ./...\n\nRisk: behind a feature flag.\n\n- [x] rollout checked"
		}`)
	})
	mux.HandleFunc(pathMR5Diffs, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `[
			{"old_path":"auth/login.go","new_path":"auth/login.go","diff":"+code","new_file":false,"renamed_file":false,"deleted_file":false},
			{"old_path":"auth/login_test.go","new_path":"auth/login_test.go","diff":"+test","new_file":false,"renamed_file":false,"deleted_file":false},
			{"old_path":"docs/login.md","new_path":"docs/login.md","diff":"+docs","new_file":false,"renamed_file":false,"deleted_file":false}
		]`)
	})

	session := newMCPSession(t, mux)
	result, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "mr_description_quality",
		Arguments: map[string]string{"project_id": "42", "merge_request_iid": "5"},
	})
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	text := result.Messages[0].Content.(*mcp.TextContent).Text
	checks := []string{
		"MR Description Quality: !5",
		"Files changed**: 3",
		"Tests changed**: 1",
		"Docs changed**: 1",
		"Clear context (>120 chars) | ✅",
		"Linked issue/MR/work item | ✅",
		"Test or verification evidence | ✅",
		"Rollout, risk, or rollback notes | ✅",
		"Checklist present | ✅",
	}
	assertContainsAll(t, text, checks)
}

// TestMRDescriptionQuality_MissingArgs verifies required argument validation.
func TestMRDescriptionQuality_MissingArgs(t *testing.T) {
	session := newMCPSession(t, http.NewServeMux())
	_, err := session.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "mr_description_quality",
		Arguments: map[string]string{"project_id": "42"},
	})
	if err == nil {
		t.Fatal("expected error for missing merge_request_iid")
	}
}

// TestCommitHygieneHelpers covers the local commit classification helpers.
func TestCommitHygieneHelpers(t *testing.T) {
	if !isConventionalCommit("fix(parser): handle empty input") {
		t.Fatal("expected conventional commit title")
	}
	if isConventionalCommit("update parser") {
		t.Fatal("expected non-conventional commit title")
	}
	if firstLine("title\nbody") != "title" {
		t.Fatal("expected first line extraction")
	}
	if !strings.Contains(commitHygieneLabel(testCommit("fix!: change API", "fix!: change API\n\nBREAKING CHANGE: changed", []string{"p1"})), "breaking") {
		t.Fatal("expected breaking hygiene label")
	}
}

func testCommit(title, message string, parents []string) *gl.Commit {
	return &gl.Commit{Title: title, Message: message, ParentIDs: parents}
}
