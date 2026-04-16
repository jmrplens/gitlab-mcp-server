package mergetrains

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestListProjectMergeTrains validates the ListProjectMergeTrains handler.
// Covers success with all fields, empty project_id validation, scope/sort query
// params, API errors, and empty results.
func TestListProjectMergeTrains(t *testing.T) {
	tests := []struct {
		name     string
		input    ListProjectInput
		handler  func(t *testing.T, w http.ResponseWriter, r *http.Request)
		wantErr  bool
		validate func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns trains with all fields populated",
			input: ListProjectInput{ProjectID: "42"},
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/merge_trains")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[
					{"id":1,"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug","state":"merged","web_url":"https://gitlab.example.com/-/merge_requests/5","created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-16T10:00:00Z"},"user":{"id":1,"username":"admin"},"pipeline":{"id":200},"target_branch":"main","status":"merged","duration":120,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-16T10:00:00Z","merged_at":"2026-01-17T10:00:00Z"}
				]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			},
			validate: func(t *testing.T, out ListOutput) {
				if len(out.Trains) != 1 {
					t.Fatalf("got %d trains, want 1", len(out.Trains))
				}
				tr := out.Trains[0]
				if tr.ID != 1 {
					t.Errorf("got ID %d, want 1", tr.ID)
				}
				if tr.TargetBranch != "main" {
					t.Errorf("got target_branch %q, want %q", tr.TargetBranch, "main")
				}
				if tr.Status != "merged" {
					t.Errorf("got status %q, want %q", tr.Status, "merged")
				}
				if tr.User != "admin" {
					t.Errorf("got user %q, want %q", tr.User, "admin")
				}
				if tr.PipelineID != 200 {
					t.Errorf("got pipeline_id %d, want 200", tr.PipelineID)
				}
				if tr.Duration != 120 {
					t.Errorf("got duration %d, want 120", tr.Duration)
				}
				if tr.MergeRequest.IID != 5 {
					t.Errorf("got MR IID %d, want 5", tr.MergeRequest.IID)
				}
				if tr.MergeRequest.WebURL != "https://gitlab.example.com/-/merge_requests/5" {
					t.Errorf("got web_url %q, want non-empty", tr.MergeRequest.WebURL)
				}
				if tr.MergeRequest.CreatedAt == "" {
					t.Error("expected MR created_at to be set")
				}
				if tr.MergeRequest.UpdatedAt == "" {
					t.Error("expected MR updated_at to be set")
				}
				if tr.CreatedAt == "" {
					t.Error("expected created_at to be set")
				}
				if tr.UpdatedAt == "" {
					t.Error("expected updated_at to be set")
				}
				if tr.MergedAt == "" {
					t.Error("expected merged_at to be set")
				}
			},
		},
		{
			name:    "returns error when project_id is empty",
			input:   ListProjectInput{},
			wantErr: true,
		},
		{
			name:  "passes scope and sort query parameters",
			input: ListProjectInput{ProjectID: "42", Scope: "active", Sort: "asc"},
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "scope", "active")
				testutil.AssertQueryParam(t, r, "sort", "asc")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			},
			validate: func(t *testing.T, out ListOutput) {
				if len(out.Trains) != 0 {
					t.Errorf("got %d trains, want 0", len(out.Trains))
				}
			},
		},
		{
			name:  "returns error on API 500",
			input: ListProjectInput{ProjectID: "42"},
			handler: func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns empty list for no results",
			input: ListProjectInput{ProjectID: "42"},
			handler: func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			},
			validate: func(t *testing.T, out ListOutput) {
				if len(out.Trains) != 0 {
					t.Errorf("got %d trains, want 0", len(out.Trains))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.handler != nil {
					tt.handler(t, w, r)
				} else {
					t.Fatal("handler should not be called")
				}
			}))
			out, err := ListProjectMergeTrains(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestListMergeRequestInMergeTrain validates the ListMergeRequestInMergeTrain handler.
// Covers success, missing project_id, missing target_branch, scope/sort params,
// API errors, and empty results.
func TestListMergeRequestInMergeTrain(t *testing.T) {
	tests := []struct {
		name     string
		input    ListBranchInput
		handler  func(t *testing.T, w http.ResponseWriter, r *http.Request)
		wantErr  bool
		validate func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns trains for branch",
			input: ListBranchInput{ProjectID: "42", TargetBranch: "main"},
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/merge_trains/main")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[
					{"id":1,"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug","state":"merged","web_url":"https://gitlab.example.com/-/merge_requests/5"},"user":{"id":1,"username":"admin"},"pipeline":{"id":200},"target_branch":"main","status":"merged","duration":120,"created_at":"2026-01-15T10:00:00Z"}
				]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			},
			validate: func(t *testing.T, out ListOutput) {
				if len(out.Trains) != 1 {
					t.Fatalf("got %d trains, want 1", len(out.Trains))
				}
				if out.Trains[0].TargetBranch != "main" {
					t.Errorf("got target_branch %q, want %q", out.Trains[0].TargetBranch, "main")
				}
				if out.Trains[0].Status != "merged" {
					t.Errorf("got status %q, want %q", out.Trains[0].Status, "merged")
				}
			},
		},
		{
			name:    "returns error when project_id is empty",
			input:   ListBranchInput{TargetBranch: "main"},
			wantErr: true,
		},
		{
			name:    "returns error when target_branch is empty",
			input:   ListBranchInput{ProjectID: "42"},
			wantErr: true,
		},
		{
			name:  "passes scope and sort query parameters",
			input: ListBranchInput{ProjectID: "42", TargetBranch: "develop", Scope: "complete", Sort: "desc"},
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "scope", "complete")
				testutil.AssertQueryParam(t, r, "sort", "desc")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			},
			validate: func(t *testing.T, out ListOutput) {
				if len(out.Trains) != 0 {
					t.Errorf("got %d trains, want 0", len(out.Trains))
				}
			},
		},
		{
			name:  "returns error on API 404",
			input: ListBranchInput{ProjectID: "999", TargetBranch: "main"},
			handler: func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.handler != nil {
					tt.handler(t, w, r)
				} else {
					t.Fatal("handler should not be called")
				}
			}))
			out, err := ListMergeRequestInMergeTrain(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestGetMergeRequestOnMergeTrain validates the GetMergeRequestOnMergeTrain handler.
// Covers success, missing project_id, invalid MR ID (zero/negative), and API errors.
func TestGetMergeRequestOnMergeTrain(t *testing.T) {
	tests := []struct {
		name     string
		input    GetInput
		handler  func(t *testing.T, w http.ResponseWriter, r *http.Request)
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "returns merge train entry",
			input: GetInput{ProjectID: "42", MergeRequestID: 5},
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/merge_trains/merge_requests/5")
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug","state":"merged"},"target_branch":"main","status":"merged","duration":60}`)
			},
			validate: func(t *testing.T, out Output) {
				if out.TargetBranch != "main" {
					t.Errorf("got target_branch %q, want %q", out.TargetBranch, "main")
				}
				if out.Status != "merged" {
					t.Errorf("got status %q, want %q", out.Status, "merged")
				}
				if out.Duration != 60 {
					t.Errorf("got duration %d, want 60", out.Duration)
				}
				if out.MergeRequest.IID != 5 {
					t.Errorf("got MR IID %d, want 5", out.MergeRequest.IID)
				}
			},
		},
		{
			name:    "returns error when project_id is empty",
			input:   GetInput{MergeRequestID: 5},
			wantErr: true,
		},
		{
			name:    "returns error when merge_request_iid is zero",
			input:   GetInput{ProjectID: "42", MergeRequestID: 0},
			wantErr: true,
		},
		{
			name:    "returns error when merge_request_iid is negative",
			input:   GetInput{ProjectID: "42", MergeRequestID: -1},
			wantErr: true,
		},
		{
			name:  "returns error on API 404",
			input: GetInput{ProjectID: "42", MergeRequestID: 999},
			handler: func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.handler != nil {
					tt.handler(t, w, r)
				} else {
					t.Fatal("handler should not be called")
				}
			}))
			out, err := GetMergeRequestOnMergeTrain(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestAddMergeRequestToMergeTrain validates the AddMergeRequestToMergeTrain handler.
// Covers success, missing project_id, invalid MR ID, optional fields
// (AutoMerge, SHA, Squash), and API errors.
func TestAddMergeRequestToMergeTrain(t *testing.T) {
	tests := []struct {
		name     string
		input    AddInput
		handler  func(t *testing.T, w http.ResponseWriter, r *http.Request)
		wantErr  bool
		validate func(t *testing.T, out ListOutput)
	}{
		{
			name:  "adds MR to merge train",
			input: AddInput{ProjectID: "42", MergeRequestID: 5},
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/merge_trains/merge_requests/5")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[
					{"id":2,"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug","state":"opened"},"target_branch":"main","status":"idle","duration":0}
				]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			},
			validate: func(t *testing.T, out ListOutput) {
				if len(out.Trains) != 1 {
					t.Fatalf("got %d trains, want 1", len(out.Trains))
				}
				if out.Trains[0].Status != "idle" {
					t.Errorf("got status %q, want %q", out.Trains[0].Status, "idle")
				}
			},
		},
		{
			name:  "sends optional fields in request body",
			input: AddInput{ProjectID: "42", MergeRequestID: 5, AutoMerge: true, SHA: "abc123", Squash: true},
			handler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				var opts map[string]any
				if unmarshalErr := json.Unmarshal(body, &opts); unmarshalErr != nil {
					t.Fatalf("failed to parse body: %v", unmarshalErr)
				}
				if opts["auto_merge"] != true {
					t.Errorf("auto_merge = %v, want true", opts["auto_merge"])
				}
				if opts["sha"] != "abc123" {
					t.Errorf("sha = %v, want %q", opts["sha"], "abc123")
				}
				if opts["squash"] != true {
					t.Errorf("squash = %v, want true", opts["squash"])
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[
					{"id":3,"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug","state":"opened"},"target_branch":"main","status":"idle","duration":0}
				]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			},
			validate: func(t *testing.T, out ListOutput) {
				if len(out.Trains) != 1 {
					t.Fatalf("got %d trains, want 1", len(out.Trains))
				}
			},
		},
		{
			name:    "returns error when project_id is empty",
			input:   AddInput{MergeRequestID: 5},
			wantErr: true,
		},
		{
			name:    "returns error when merge_request_iid is zero",
			input:   AddInput{ProjectID: "42", MergeRequestID: 0},
			wantErr: true,
		},
		{
			name:    "returns error when merge_request_iid is negative",
			input:   AddInput{ProjectID: "42", MergeRequestID: -1},
			wantErr: true,
		},
		{
			name:  "returns error on API 422",
			input: AddInput{ProjectID: "42", MergeRequestID: 5},
			handler: func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"MR is not mergeable"}`)
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.handler != nil {
					tt.handler(t, w, r)
				} else {
					t.Fatal("handler should not be called")
				}
			}))
			out, err := AddMergeRequestToMergeTrain(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestToOutput_NilInput verifies toOutput handles a nil MergeTrain gracefully.
func TestToOutput_NilInput(t *testing.T) {
	out := toOutput(nil)
	if out.ID != 0 {
		t.Errorf("got ID %d, want 0 for nil input", out.ID)
	}
}

// TestToOutput_MinimalFields verifies toOutput with nil optional sub-objects
// (no User, no Pipeline, no MergeRequest, no timestamps) returns zero values.
func TestToOutput_MinimalFields(t *testing.T) {
	mt := &gl.MergeTrain{
		ID:           10,
		TargetBranch: "main",
		Status:       "idle",
		Duration:     0,
	}
	out := toOutput(mt)
	if out.ID != 10 {
		t.Errorf("got ID %d, want 10", out.ID)
	}
	if out.User != "" {
		t.Errorf("got user %q, want empty for nil User", out.User)
	}
	if out.PipelineID != 0 {
		t.Errorf("got pipeline_id %d, want 0 for nil Pipeline", out.PipelineID)
	}
	if out.MergeRequest.IID != 0 {
		t.Errorf("got MR IID %d, want 0 for nil MergeRequest", out.MergeRequest.IID)
	}
	if out.CreatedAt != "" {
		t.Errorf("got created_at %q, want empty for nil time", out.CreatedAt)
	}
	if out.MergedAt != "" {
		t.Errorf("got merged_at %q, want empty for nil time", out.MergedAt)
	}
}

// TestFormatListMarkdown validates Markdown formatting for merge train lists.
// Covers empty trains, trains with WebURL links, and trains without WebURL.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		contains []string
		equals   string
	}{
		{
			name:   "empty list returns no-results message",
			input:  ListOutput{Trains: []Output{}},
			equals: "No merge trains found.\n",
		},
		{
			name: "renders table with WebURL link",
			input: ListOutput{
				Trains: []Output{
					{
						ID:           1,
						TargetBranch: "main",
						Status:       "merged",
						User:         "admin",
						Duration:     120,
						MergeRequest: MergeRequestOutput{IID: 5, Title: "Fix bug", WebURL: "https://gitlab.example.com/-/merge_requests/5"},
					},
				},
			},
			contains: []string{
				"## Merge Trains",
				"[!5](https://gitlab.example.com/-/merge_requests/5)",
				"| 1 |",
				"| main |",
				"| merged |",
				"| admin |",
				"| 120s |",
			},
		},
		{
			name: "renders MR without WebURL as plain text",
			input: ListOutput{
				Trains: []Output{
					{
						ID:           2,
						TargetBranch: "develop",
						Status:       "idle",
						User:         "dev",
						Duration:     0,
						MergeRequest: MergeRequestOutput{IID: 10, Title: "Add feature"},
					},
				},
			},
			contains: []string{
				"!10",
				"Add feature",
				"| develop |",
				"| idle |",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			if tt.equals != "" && got != tt.equals {
				t.Errorf("got %q, want %q", got, tt.equals)
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}

// TestFormatOutputMarkdown validates Markdown formatting for a single merge train entry.
// Covers minimal output, full output with all fields, and output without WebURL.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    Output
		contains []string
		absent   []string
	}{
		{
			name: "renders full output with all optional fields",
			input: Output{
				ID:           1,
				TargetBranch: "main",
				Status:       "merged",
				User:         "admin",
				PipelineID:   200,
				Duration:     120,
				CreatedAt:    "2026-01-15T10:00:00Z",
				MergedAt:     "2026-01-17T10:00:00Z",
				MergeRequest: MergeRequestOutput{IID: 5, Title: "Fix bug", WebURL: "https://gitlab.example.com/-/merge_requests/5"},
			},
			contains: []string{
				"## Merge Train #1",
				"| Status | merged |",
				"| Target Branch | main |",
				"[!5](https://gitlab.example.com/-/merge_requests/5)",
				"| User | admin |",
				"| Pipeline | #200 |",
				"| Duration | 120s |",
				"| Merged At |",
			},
		},
		{
			name: "renders minimal output without optional fields",
			input: Output{
				ID:           2,
				TargetBranch: "develop",
				Status:       "idle",
				Duration:     0,
				MergeRequest: MergeRequestOutput{IID: 10, Title: "Add feature"},
			},
			contains: []string{
				"## Merge Train #2",
				"| Status | idle |",
				"!10",
			},
			absent: []string{
				"| User |",
				"| Pipeline |",
				"| Merged At |",
			},
		},
		{
			name: "renders MR without WebURL as plain text",
			input: Output{
				ID:           3,
				TargetBranch: "main",
				Status:       "active",
				MergeRequest: MergeRequestOutput{IID: 7, Title: "Update docs"},
			},
			contains: []string{
				"!7 — Update docs",
			},
			absent: []string{
				"[!7](",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
			for _, notWant := range tt.absent {
				if strings.Contains(got, notWant) {
					t.Errorf("output should not contain %q\ngot:\n%s", notWant, got)
				}
			}
		})
	}
}
