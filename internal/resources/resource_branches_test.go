// resource_branches_test.go contains edge-case and branch-coverage tests
// for MCP resources, exercising nil-pointer branches (e.g. nil author on
// merge requests) and optional field handling (e.g. milestone due dates).
package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMilestonesResource_WithDueDate exercises the DueDate != nil branch.
func TestMilestonesResource_WithDueDate(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/milestones" {
			respondJSON(w, http.StatusOK, `[
				{"id":1,"iid":1,"title":"v1.0","description":"First","state":"active","web_url":"https://x.com/m/1","due_date":"2025-06-30"},
				{"id":2,"iid":2,"title":"v2.0","description":"Second","state":"active","web_url":"https://x.com/m/2"}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://project/42/milestones",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var milestones []MilestoneResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &milestones); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if milestones[0].DueDate == "" {
		t.Error("expected DueDate to be set for first milestone")
	}
	if milestones[1].DueDate != "" {
		t.Error("expected DueDate to be empty for second milestone (no due_date)")
	}
}

// TestMergeRequestResource_NilAuthor exercises the nil author branch.
func TestMergeRequestResource_NilAuthor(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/merge_requests/1" {
			respondJSON(w, http.StatusOK, `{"id":1,"iid":1,"title":"No Author MR","state":"opened","source_branch":"dev","target_branch":"main","author":null,"web_url":"https://x.com/mr/1","detailed_merge_status":"mergeable"}`)
			return
		}
		http.NotFound(w, r)
	}))

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://project/42/mr/1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var mr MRResourceOutput
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &mr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mr.Author != "" {
		t.Errorf("expected empty author for nil author, got %q", mr.Author)
	}
	if mr.Title != "No Author MR" {
		t.Errorf("title = %q, want %q", mr.Title, "No Author MR")
	}
}
