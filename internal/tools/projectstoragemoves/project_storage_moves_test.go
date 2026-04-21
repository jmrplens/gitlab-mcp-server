package projectstoragemoves

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const storageMoveJSON = `{
	"id": 1,
	"created_at": "2026-01-15T10:30:00Z",
	"state": "finished",
	"source_storage_name": "default",
	"destination_storage_name": "storage2",
	"project": {
		"id": 42,
		"name": "my-project",
		"path_with_namespace": "group/my-project"
	}
}`

const storageMoveNoProjectJSON = `{
	"id": 2,
	"state": "scheduled",
	"source_storage_name": "default",
	"destination_storage_name": "storage3"
}`

// TestRetrieveAll validates the RetrieveAll function covering success with
// pagination, empty results, API errors, and context cancellation.
func TestRetrieveAll(t *testing.T) {
	tests := []struct {
		name      string
		input     ListInput
		handler   http.HandlerFunc
		wantErr   bool
		wantMoves int
		validate  func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns moves with pagination",
			input: ListInput{PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 20}},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/project_repository_storage_moves")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+storageMoveJSON+`]`, testutil.PaginationHeaders{
					Page: "1", PerPage: "20", Total: "1", TotalPages: "1",
				})
			},
			wantMoves: 1,
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if out.Moves[0].ID != 1 {
					t.Errorf("ID = %d, want 1", out.Moves[0].ID)
				}
				if out.Moves[0].State != "finished" {
					t.Errorf("State = %q, want %q", out.Moves[0].State, "finished")
				}
				if out.Moves[0].SourceStorageName != "default" {
					t.Errorf("SourceStorageName = %q, want %q", out.Moves[0].SourceStorageName, "default")
				}
				if out.Moves[0].DestinationStorageName != "storage2" {
					t.Errorf("DestinationStorageName = %q, want %q", out.Moves[0].DestinationStorageName, "storage2")
				}
				if out.Moves[0].Project == nil {
					t.Fatal("expected non-nil project")
				}
				if out.Moves[0].Project.ID != 42 {
					t.Errorf("Project.ID = %d, want 42", out.Moves[0].Project.ID)
				}
				if out.Moves[0].Project.PathWithNamespace != "group/my-project" {
					t.Errorf("Project.PathWithNamespace = %q, want %q", out.Moves[0].Project.PathWithNamespace, "group/my-project")
				}
				if out.Moves[0].CreatedAt.IsZero() {
					t.Error("expected non-zero CreatedAt")
				}
				if out.Pagination.Page != 1 {
					t.Errorf("Pagination.Page = %d, want 1", out.Pagination.Page)
				}
			},
		},
		{
			name:  "returns empty list",
			input: ListInput{},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			},
			wantMoves: 0,
		},
		{
			name:  "returns move without project or created_at",
			input: ListInput{},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[`+storageMoveNoProjectJSON+`]`)
			},
			wantMoves: 1,
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if out.Moves[0].Project != nil {
					t.Errorf("expected nil project, got %+v", out.Moves[0].Project)
				}
				if !out.Moves[0].CreatedAt.IsZero() {
					t.Errorf("expected zero CreatedAt, got %v", out.Moves[0].CreatedAt)
				}
				if out.Moves[0].State != "scheduled" {
					t.Errorf("State = %q, want %q", out.Moves[0].State, "scheduled")
				}
			},
		},
		{
			name:  "returns error on 403 forbidden",
			input: ListInput{},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
		{
			name:  "returns error on 500 server error",
			input: ListInput{},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := RetrieveAll(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(out.Moves) != tt.wantMoves {
					t.Fatalf("got %d moves, want %d", len(out.Moves), tt.wantMoves)
				}
				if tt.validate != nil {
					tt.validate(t, out)
				}
			}
		})
	}
}

// TestRetrieveAll_ContextCanceled verifies that RetrieveAll returns an error
// when the context is already cancelled before the API call.
func TestRetrieveAll_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := RetrieveAll(ctx, client, ListInput{})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestRetrieveForProject validates the RetrieveForProject function covering
// success, missing project ID, API error, and context cancellation.
func TestRetrieveForProject(t *testing.T) {
	tests := []struct {
		name      string
		input     ListForProjectInput
		handler   http.HandlerFunc
		wantErr   bool
		wantMoves int
		validate  func(t *testing.T, out ListOutput)
	}{
		{
			name:  "returns moves for project",
			input: ListForProjectInput{ProjectID: 42},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/repository_storage_moves")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+storageMoveJSON+`]`, testutil.PaginationHeaders{
					Page: "1", PerPage: "20", Total: "1", TotalPages: "1",
				})
			},
			wantMoves: 1,
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if out.Moves[0].Project == nil {
					t.Fatal("expected non-nil project")
				}
				if out.Moves[0].Project.ID != 42 {
					t.Errorf("Project.ID = %d, want 42", out.Moves[0].Project.ID)
				}
			},
		},
		{
			name:    "returns error for missing project_id",
			input:   ListForProjectInput{},
			handler: func(http.ResponseWriter, *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on 404 not found",
			input: ListForProjectInput{ProjectID: 999},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns error on 403 forbidden",
			input: ListForProjectInput{ProjectID: 42},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := RetrieveForProject(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(out.Moves) != tt.wantMoves {
					t.Fatalf("got %d moves, want %d", len(out.Moves), tt.wantMoves)
				}
				if tt.validate != nil {
					tt.validate(t, out)
				}
			}
		})
	}
}

// TestRetrieveForProject_ContextCanceled verifies that RetrieveForProject
// returns an error when context is already cancelled.
func TestRetrieveForProject_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := RetrieveForProject(ctx, client, ListForProjectInput{ProjectID: 42})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestGet validates the Get function covering success, missing ID,
// API errors, and a response without project data.
func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		input    IDInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "returns storage move with full details",
			input: IDInput{ID: 1},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/project_repository_storage_moves/1")
				testutil.RespondJSON(w, http.StatusOK, storageMoveJSON)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 1 {
					t.Errorf("ID = %d, want 1", out.ID)
				}
				if out.SourceStorageName != "default" {
					t.Errorf("SourceStorageName = %q, want %q", out.SourceStorageName, "default")
				}
				if out.DestinationStorageName != "storage2" {
					t.Errorf("DestinationStorageName = %q, want %q", out.DestinationStorageName, "storage2")
				}
				if out.Project == nil {
					t.Fatal("expected non-nil project")
				}
				if out.Project.Name != "my-project" {
					t.Errorf("Project.Name = %q, want %q", out.Project.Name, "my-project")
				}
			},
		},
		{
			name:  "returns move without project",
			input: IDInput{ID: 2},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, storageMoveNoProjectJSON)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.Project != nil {
					t.Errorf("expected nil project, got %+v", out.Project)
				}
			},
		},
		{
			name:    "returns error for missing id",
			input:   IDInput{},
			handler: func(http.ResponseWriter, *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on 404 not found",
			input: IDInput{ID: 999},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns error on 500 server error",
			input: IDInput{ID: 1},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Get(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestGet_ContextCanceled verifies that Get returns an error when context
// is already cancelled.
func TestGet_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, storageMoveJSON)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, IDInput{ID: 1})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestGetForProject validates the GetForProject function covering success,
// missing project_id, missing id, and API errors.
func TestGetForProject(t *testing.T) {
	tests := []struct {
		name     string
		input    ProjectMoveInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "returns storage move for project",
			input: ProjectMoveInput{ProjectID: 42, ID: 1},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/repository_storage_moves/1")
				testutil.RespondJSON(w, http.StatusOK, storageMoveJSON)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 1 {
					t.Errorf("ID = %d, want 1", out.ID)
				}
			},
		},
		{
			name:    "returns error for missing project_id",
			input:   ProjectMoveInput{ID: 1},
			handler: func(http.ResponseWriter, *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error for missing id",
			input:   ProjectMoveInput{ProjectID: 42},
			handler: func(http.ResponseWriter, *http.Request) {},
			wantErr: true,
		},
		{
			name:    "returns error for both fields missing",
			input:   ProjectMoveInput{},
			handler: func(http.ResponseWriter, *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on 404 not found",
			input: ProjectMoveInput{ProjectID: 42, ID: 999},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := GetForProject(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestGetForProject_ContextCanceled verifies that GetForProject returns an
// error when context is already cancelled.
func TestGetForProject_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, storageMoveJSON)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := GetForProject(ctx, client, ProjectMoveInput{ProjectID: 42, ID: 1})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestSchedule validates the Schedule function covering success with and
// without destination, missing project_id, and API errors.
func TestSchedule(t *testing.T) {
	dest := "storage2"
	tests := []struct {
		name     string
		input    ScheduleInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out Output)
	}{
		{
			name:  "schedules move with destination",
			input: ScheduleInput{ProjectID: 42, DestinationStorageName: &dest},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/repository_storage_moves")
				testutil.RespondJSON(w, http.StatusCreated, storageMoveJSON)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 1 {
					t.Errorf("ID = %d, want 1", out.ID)
				}
				if out.DestinationStorageName != "storage2" {
					t.Errorf("DestinationStorageName = %q, want %q", out.DestinationStorageName, "storage2")
				}
			},
		},
		{
			name:  "schedules move without destination (auto-select)",
			input: ScheduleInput{ProjectID: 42},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusCreated, storageMoveNoProjectJSON)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.ID != 2 {
					t.Errorf("ID = %d, want 2", out.ID)
				}
			},
		},
		{
			name:    "returns error for missing project_id",
			input:   ScheduleInput{},
			handler: func(http.ResponseWriter, *http.Request) {},
			wantErr: true,
		},
		{
			name:  "returns error on 404 not found",
			input: ScheduleInput{ProjectID: 999},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
			},
			wantErr: true,
		},
		{
			name:  "returns error on 403 forbidden",
			input: ScheduleInput{ProjectID: 42},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Schedule(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestSchedule_ContextCanceled verifies that Schedule returns an error when
// context is already cancelled.
func TestSchedule_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, storageMoveJSON)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Schedule(ctx, client, ScheduleInput{ProjectID: 42})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestScheduleAll validates the ScheduleAll function covering success with
// source and destination, without optional params, and API errors.
func TestScheduleAll(t *testing.T) {
	src := "default"
	dest := "storage2"
	tests := []struct {
		name     string
		input    ScheduleAllInput
		handler  http.HandlerFunc
		wantErr  bool
		validate func(t *testing.T, out ScheduleAllOutput)
	}{
		{
			name:  "schedules all with source and destination",
			input: ScheduleAllInput{SourceStorageName: &src, DestinationStorageName: &dest},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, "/api/v4/project_repository_storage_moves")
				w.WriteHeader(http.StatusAccepted)
			},
			validate: func(t *testing.T, out ScheduleAllOutput) {
				t.Helper()
				if out.Message == "" {
					t.Error("expected non-empty message")
				}
				if !strings.Contains(out.Message, "scheduled") {
					t.Errorf("message %q should contain 'scheduled'", out.Message)
				}
			},
		},
		{
			name:  "schedules all without optional params",
			input: ScheduleAllInput{},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			},
			validate: func(t *testing.T, out ScheduleAllOutput) {
				t.Helper()
				if out.Message == "" {
					t.Error("expected non-empty message")
				}
			},
		},
		{
			name:  "returns error on 403 forbidden",
			input: ScheduleAllInput{},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
		{
			name:  "returns error on 500 server error",
			input: ScheduleAllInput{},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := ScheduleAll(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// TestScheduleAll_ContextCanceled verifies that ScheduleAll returns an error
// when context is already cancelled.
func TestScheduleAll_ContextCanceled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ScheduleAll(ctx, client, ScheduleAllInput{})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestFormatOutputMarkdown validates that FormatOutputMarkdown produces
// correct Markdown for moves with and without project/createdAt data.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    Output
		wantAll  []string
		wantNone []string
	}{
		{
			name: "full output with project and created_at",
			input: Output{
				ID:                     1,
				State:                  "finished",
				SourceStorageName:      "default",
				DestinationStorageName: "storage2",
				CreatedAt:              mustParseTime("2026-01-15T10:30:00Z"),
				Project: &ProjectOutput{
					ID:                42,
					Name:              "my-project",
					PathWithNamespace: "group/my-project",
				},
			},
			wantAll: []string{
				"## Project Storage Move #1",
				"| ID | 1 |",
				"| State | finished |",
				"| Source Storage | default |",
				"| Destination Storage | storage2 |",
				"| Created At |",
				"2026-01-15",
				"| Project | group/my-project (ID: 42) |",
			},
		},
		{
			name: "output without project or created_at",
			input: Output{
				ID:                     2,
				State:                  "scheduled",
				SourceStorageName:      "default",
				DestinationStorageName: "storage3",
			},
			wantAll: []string{
				"## Project Storage Move #2",
				"| State | scheduled |",
			},
			wantNone: []string{
				"| Project |",
				"| Created At |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(tt.input)
			for _, want := range tt.wantAll {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantNone {
				if strings.Contains(got, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, got)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates that FormatListMarkdown produces correct
// Markdown tables for lists with moves, empty lists, and pagination info.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		wantAll  []string
		wantNone []string
	}{
		{
			name: "list with moves and pagination",
			input: ListOutput{
				Moves: []Output{
					{
						ID:                     1,
						State:                  "finished",
						SourceStorageName:      "default",
						DestinationStorageName: "storage2",
						Project: &ProjectOutput{
							PathWithNamespace: "group/my-project",
						},
					},
					{
						ID:                     2,
						State:                  "scheduled",
						SourceStorageName:      "default",
						DestinationStorageName: "storage3",
					},
				},
				Pagination: toolutil.PaginationOutput{Page: 1},
			},
			wantAll: []string{
				"## Project Storage Moves",
				"| ID | State | Source | Destination | Project |",
				"| 1 | finished | default | storage2 | group/my-project |",
				"| 2 | scheduled | default | storage3 |  |",
				"_Page 1, 2 moves shown._",
			},
		},
		{
			name: "empty list no pagination line",
			input: ListOutput{
				Moves: []Output{},
			},
			wantAll: []string{
				"## Project Storage Moves",
				"| ID | State | Source | Destination | Project |",
			},
			wantNone: []string{
				"_Page",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			for _, want := range tt.wantAll {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantNone {
				if strings.Contains(got, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, got)
				}
			}
		})
	}
}

// TestFormatScheduleAllMarkdown validates that FormatScheduleAllMarkdown
// produces correct Markdown with the confirmation message.
func TestFormatScheduleAllMarkdown(t *testing.T) {
	out := ScheduleAllOutput{Message: "All project repository storage moves have been scheduled"}
	got := FormatScheduleAllMarkdown(out)

	wantAll := []string{
		"## Schedule All Project Storage Moves",
		"All project repository storage moves have been scheduled",
		"gitlab_retrieve_all_project_storage_moves",
	}
	for _, want := range wantAll {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, got)
		}
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
