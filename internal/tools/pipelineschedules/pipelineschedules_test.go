// pipelineschedules_test.go contains unit tests for the pipeline schedule MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package pipelineschedules

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpMissingProjectID = "expected error for missing project_id"

const errExpZeroScheduleID = "expected error for zero schedule_id"

const errExpMissingKey = "expected error for missing key"

const (
	testPathSchedules = "/api/v4/projects/123/pipeline_schedules"
	testPathSchedule1 = "/api/v4/projects/123/pipeline_schedules/1"
	testUpdatedDesc   = "Updated desc"
)

// ---------------------------------------------------------------------------
// Pipeline Schedule List
// ---------------------------------------------------------------------------.

// TestPipelineScheduleList_Success verifies the behavior of pipeline schedule list success.
func TestPipelineScheduleList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedules && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"description":"Nightly build","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,"owner":{"username":"admin"}}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Schedules) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(out.Schedules))
	}
	if out.Schedules[0].Description != "Nightly build" {
		t.Errorf("description = %q, want %q", out.Schedules[0].Description, "Nightly build")
	}
	if out.Schedules[0].OwnerName != "admin" {
		t.Errorf("owner = %q, want %q", out.Schedules[0].OwnerName, "admin")
	}
}

// TestPipelineScheduleList_WithScope verifies the behavior of pipeline schedule list with scope.
func TestPipelineScheduleList_WithScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedules {
			if r.URL.Query().Get("scope") != "active" {
				t.Errorf("expected scope=active, got %q", r.URL.Query().Get("scope"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "123",
		Scope:     "active",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPipelineScheduleList_MissingProjectID verifies the behavior of pipeline schedule list missing project i d.
func TestPipelineScheduleList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestPipelineScheduleList_CancelledContext verifies the behavior of pipeline schedule list cancelled context.
func TestPipelineScheduleList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Pipeline Schedule Get
// ---------------------------------------------------------------------------.

// TestPipelineScheduleGet_Success verifies the behavior of pipeline schedule get success.
func TestPipelineScheduleGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedule1 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"Nightly build","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,"owner":{"username":"admin"}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:  "123",
		ScheduleID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("id = %d, want 1", out.ID)
	}
	if out.Cron != "0 1 * * *" {
		t.Errorf("cron = %q, want %q", out.Cron, "0 1 * * *")
	}
}

// TestPipelineSchedule_GetZeroID verifies the behavior of pipeline schedule get zero i d.
func TestPipelineSchedule_GetZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "123", ScheduleID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// TestPipelineScheduleGet_CancelledContext verifies the behavior of pipeline schedule get cancelled context.
func TestPipelineScheduleGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Pipeline Schedule Create
// ---------------------------------------------------------------------------.

// TestPipelineScheduleCreate_Success verifies the behavior of pipeline schedule create success.
func TestPipelineScheduleCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedules && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":10,"description":"Weekly deploy","ref":"main","cron":"0 9 * * 1","cron_timezone":"UTC","active":true
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "123",
		Description: "Weekly deploy",
		Ref:         "main",
		Cron:        "0 9 * * 1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("id = %d, want 10", out.ID)
	}
}

// TestPipelineScheduleCreate_MissingFields validates pipeline schedule create missing fields across multiple scenarios using table-driven subtests.
func TestPipelineScheduleCreate_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	tests := []struct {
		name  string
		input CreateInput
	}{
		{"missing project_id", CreateInput{Description: "d", Ref: "r", Cron: "c"}},
		{"missing description", CreateInput{ProjectID: "1", Ref: "r", Cron: "c"}},
		{"missing ref", CreateInput{ProjectID: "1", Description: "d", Cron: "c"}},
		{"missing cron", CreateInput{ProjectID: "1", Description: "d", Ref: "r"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Create(context.Background(), client, tc.input)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestPipelineScheduleCreate_CancelledContext verifies the behavior of pipeline schedule create cancelled context.
func TestPipelineScheduleCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{
		ProjectID: "1", Description: "d", Ref: "r", Cron: "c",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Pipeline Schedule Update
// ---------------------------------------------------------------------------.

// TestPipelineScheduleUpdate_Success verifies the behavior of pipeline schedule update success.
func TestPipelineScheduleUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedule1 && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"Updated desc","ref":"develop","cron":"0 2 * * *","cron_timezone":"UTC","active":false
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:   "123",
		ScheduleID:  1,
		Description: testUpdatedDesc,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Description != testUpdatedDesc {
		t.Errorf("description = %q, want %q", out.Description, testUpdatedDesc)
	}
}

// TestPipelineSchedule_UpdateZeroID verifies the behavior of pipeline schedule update zero i d.
func TestPipelineSchedule_UpdateZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "123", ScheduleID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// TestPipelineScheduleUpdate_CancelledContext verifies the behavior of pipeline schedule update cancelled context.
func TestPipelineScheduleUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{
		ProjectID: "1", ScheduleID: 1,
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Pipeline Schedule Delete
// ---------------------------------------------------------------------------.

// TestPipelineScheduleDelete_Success verifies the behavior of pipeline schedule delete success.
func TestPipelineScheduleDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedule1 && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "123", ScheduleID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPipelineSchedule_DeleteZeroID verifies the behavior of pipeline schedule delete zero i d.
func TestPipelineSchedule_DeleteZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "123", ScheduleID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// TestPipelineScheduleDelete_CancelledContext verifies the behavior of pipeline schedule delete cancelled context.
func TestPipelineScheduleDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{
		ProjectID: "1", ScheduleID: 1,
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Pipeline Schedule Run
// ---------------------------------------------------------------------------.

// TestPipelineScheduleRun_Success verifies the behavior of pipeline schedule run success.
func TestPipelineScheduleRun_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/projects/123/pipeline_schedules/1/play" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == testPathSchedule1 && r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"Triggered","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true
			}`)
		default:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
		}
	}))

	out, err := Run(context.Background(), client, RunInput{
		ProjectID: "123", ScheduleID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 {
		t.Errorf("id = %d, want 1", out.ID)
	}
}

// TestPipelineSchedule_RunZeroID verifies the behavior of pipeline schedule run zero i d.
func TestPipelineSchedule_RunZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := Run(context.Background(), client, RunInput{
		ProjectID: "123", ScheduleID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// TestPipelineScheduleRun_CancelledContext verifies the behavior of pipeline schedule run cancelled context.
func TestPipelineScheduleRun_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	ctx := testutil.CancelledCtx(t)
	_, err := Run(ctx, client, RunInput{
		ProjectID: "1", ScheduleID: 1,
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Take Ownership
// ---------------------------------------------------------------------------.

const scheduleJSON = `{"id":1,"description":"Nightly","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,"owner":{"username":"newowner"}}`

// TestTakeOwnership_Success verifies the behavior of take ownership success.
func TestTakeOwnership_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/pipeline_schedules/1/take_ownership" {
			testutil.RespondJSON(w, http.StatusOK, scheduleJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := TakeOwnership(context.Background(), client, TakeOwnershipInput{ProjectID: "42", ScheduleID: 1})
	if err != nil {
		t.Fatalf("TakeOwnership() error: %v", err)
	}
	if out.OwnerName != "newowner" {
		t.Errorf("OwnerName = %q, want %q", out.OwnerName, "newowner")
	}
}

// TestTakeOwnership_MissingProjectID verifies the behavior of take ownership missing project i d.
func TestTakeOwnership_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := TakeOwnership(context.Background(), client, TakeOwnershipInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestTakeOwnership_ZeroScheduleID verifies the behavior of take ownership zero schedule i d.
func TestTakeOwnership_ZeroScheduleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := TakeOwnership(context.Background(), client, TakeOwnershipInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// ---------------------------------------------------------------------------
// Create Variable
// ---------------------------------------------------------------------------.

// TestCreateVariable_Success verifies the behavior of create variable success.
func TestCreateVariable_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/pipeline_schedules/1/variables" {
			testutil.RespondJSON(w, http.StatusCreated, `{"key":"DEPLOY_ENV","value":"production","variable_type":"env_var"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ProjectID: "42", ScheduleID: 1, Key: "DEPLOY_ENV", Value: "production",
	})
	if err != nil {
		t.Fatalf("CreateVariable() error: %v", err)
	}
	if out.Key != "DEPLOY_ENV" {
		t.Errorf("Key = %q, want %q", out.Key, "DEPLOY_ENV")
	}
	if out.Value != "production" {
		t.Errorf("Value = %q, want %q", out.Value, "production")
	}
}

// TestCreateVariable_MissingKey verifies the behavior of create variable missing key.
func TestCreateVariable_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ProjectID: "42", ScheduleID: 1, Value: "val",
	})
	if err == nil {
		t.Fatal(errExpMissingKey)
	}
}

// TestCreateVariable_MissingValue verifies the behavior of create variable missing value.
func TestCreateVariable_MissingValue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ProjectID: "42", ScheduleID: 1, Key: "K",
	})
	if err == nil {
		t.Fatal("expected error for missing value")
	}
}

// ---------------------------------------------------------------------------
// Edit Variable
// ---------------------------------------------------------------------------.

// TestEditVariable_Success verifies the behavior of edit variable success.
func TestEditVariable_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/pipeline_schedules/1/variables/DEPLOY_ENV" {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"DEPLOY_ENV","value":"staging","variable_type":"env_var"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := EditVariable(context.Background(), client, EditVariableInput{
		ProjectID: "42", ScheduleID: 1, Key: "DEPLOY_ENV", Value: "staging",
	})
	if err != nil {
		t.Fatalf("EditVariable() error: %v", err)
	}
	if out.Value != "staging" {
		t.Errorf("Value = %q, want %q", out.Value, "staging")
	}
}

// TestEditVariable_MissingKey verifies the behavior of edit variable missing key.
func TestEditVariable_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := EditVariable(context.Background(), client, EditVariableInput{
		ProjectID: "42", ScheduleID: 1, Value: "val",
	})
	if err == nil {
		t.Fatal(errExpMissingKey)
	}
}

// ---------------------------------------------------------------------------
// Delete Variable
// ---------------------------------------------------------------------------.

// TestDeleteVariable_Success verifies the behavior of delete variable success.
func TestDeleteVariable_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/pipeline_schedules/1/variables/DEPLOY_ENV" {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"DEPLOY_ENV","value":"production","variable_type":"env_var"}`)
			return
		}
		http.NotFound(w, r)
	}))
	err := DeleteVariable(context.Background(), client, DeleteVariableInput{
		ProjectID: "42", ScheduleID: 1, Key: "DEPLOY_ENV",
	})
	if err != nil {
		t.Fatalf("DeleteVariable() error: %v", err)
	}
}

// TestDeleteVariable_MissingKey verifies the behavior of delete variable missing key.
func TestDeleteVariable_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	err := DeleteVariable(context.Background(), client, DeleteVariableInput{
		ProjectID: "42", ScheduleID: 1,
	})
	if err == nil {
		t.Fatal(errExpMissingKey)
	}
}

// TestDeleteVariable_APIError verifies the behavior of delete variable a p i error.
func TestDeleteVariable_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	err := DeleteVariable(context.Background(), client, DeleteVariableInput{
		ProjectID: "42", ScheduleID: 1, Key: "K",
	})
	if err == nil {
		t.Fatal("expected error for API error")
	}
}

// ---------------------------------------------------------------------------
// List Triggered Pipelines
// ---------------------------------------------------------------------------.

// TestListTriggeredPipelines_Success verifies the behavior of list triggered pipelines success.
func TestListTriggeredPipelines_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipeline_schedules/1/pipelines" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":100,"iid":10,"ref":"main","sha":"abc","status":"success","source":"schedule","web_url":"https://example.com/p/100"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListTriggeredPipelines(context.Background(), client, ListTriggeredPipelinesInput{
		ProjectID: "42", ScheduleID: 1,
	})
	if err != nil {
		t.Fatalf("ListTriggeredPipelines() error: %v", err)
	}
	if len(out.Pipelines) != 1 {
		t.Fatalf("len(Pipelines) = %d, want 1", len(out.Pipelines))
	}
	if out.Pipelines[0].ID != 100 {
		t.Errorf("ID = %d, want 100", out.Pipelines[0].ID)
	}
	if out.Pipelines[0].Source != "schedule" {
		t.Errorf("Source = %q, want %q", out.Pipelines[0].Source, "schedule")
	}
}

// TestListTriggeredPipelines_MissingProjectID verifies the behavior of list triggered pipelines missing project i d.
func TestListTriggeredPipelines_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := ListTriggeredPipelines(context.Background(), client, ListTriggeredPipelinesInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestListTriggeredPipelines_ZeroScheduleID verifies the behavior of list triggered pipelines zero schedule i d.
func TestListTriggeredPipelines_ZeroScheduleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := ListTriggeredPipelines(context.Background(), client, ListTriggeredPipelinesInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// ---------------------------------------------------------------------------
// assertContains verifies that err is non-nil and its message contains substr.
// ---------------------------------------------------------------------------.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestScheduleIDRequired_Validation ensures all handlers that require schedule_id
// reject zero and negative values.
func TestScheduleIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when schedule_id is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Get_zero", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, ScheduleID: 0}); return e }},
		{"Get_negative", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, ScheduleID: -1}); return e }},
		{"Update_zero", func() error { _, e := Update(ctx, client, UpdateInput{ProjectID: pid, ScheduleID: 0}); return e }},
		{"Update_negative", func() error { _, e := Update(ctx, client, UpdateInput{ProjectID: pid, ScheduleID: -3}); return e }},
		{"Delete_zero", func() error { return Delete(ctx, client, DeleteInput{ProjectID: pid, ScheduleID: 0}) }},
		{"Delete_negative", func() error { return Delete(ctx, client, DeleteInput{ProjectID: pid, ScheduleID: -1}) }},
		{"Run_zero", func() error { _, e := Run(ctx, client, RunInput{ProjectID: pid, ScheduleID: 0}); return e }},
		{"Run_negative", func() error { _, e := Run(ctx, client, RunInput{ProjectID: pid, ScheduleID: -5}); return e }},
		{"TakeOwnership_zero", func() error {
			_, e := TakeOwnership(ctx, client, TakeOwnershipInput{ProjectID: pid, ScheduleID: 0})
			return e
		}},
		{"TakeOwnership_negative", func() error {
			_, e := TakeOwnership(ctx, client, TakeOwnershipInput{ProjectID: pid, ScheduleID: -1})
			return e
		}},
		{"CreateVariable_zero", func() error {
			_, e := CreateVariable(ctx, client, CreateVariableInput{ProjectID: pid, ScheduleID: 0, Key: "k", Value: "v"})
			return e
		}},
		{"CreateVariable_negative", func() error {
			_, e := CreateVariable(ctx, client, CreateVariableInput{ProjectID: pid, ScheduleID: -2, Key: "k", Value: "v"})
			return e
		}},
		{"EditVariable_zero", func() error {
			_, e := EditVariable(ctx, client, EditVariableInput{ProjectID: pid, ScheduleID: 0, Key: "k", Value: "v"})
			return e
		}},
		{"EditVariable_negative", func() error {
			_, e := EditVariable(ctx, client, EditVariableInput{ProjectID: pid, ScheduleID: -1, Key: "k", Value: "v"})
			return e
		}},
		{"DeleteVariable_zero", func() error {
			return DeleteVariable(ctx, client, DeleteVariableInput{ProjectID: pid, ScheduleID: 0, Key: "k"})
		}},
		{"DeleteVariable_negative", func() error {
			return DeleteVariable(ctx, client, DeleteVariableInput{ProjectID: pid, ScheduleID: -1, Key: "k"})
		}},
		{"ListTriggeredPipelines_negative", func() error {
			_, e := ListTriggeredPipelines(ctx, client, ListTriggeredPipelinesInput{ProjectID: pid, ScheduleID: -1})
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "schedule_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpCancelledCtx = "expected error for canceled context"

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error
// ---------------------------------------------------------------------------.

// TestPipelineScheduleList_APIError verifies the behavior of pipeline schedule list a p i error.
func TestPipelineScheduleList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestPipelineScheduleGet_APIError verifies the behavior of pipeline schedule get a p i error.
func TestPipelineScheduleGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleGet_MissingProjectID verifies the behavior of pipeline schedule get missing project i d.
func TestPipelineScheduleGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(context.Background(), client, GetInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Create — API error, with optional fields
// ---------------------------------------------------------------------------.

// TestPipelineScheduleCreate_APIError verifies the behavior of pipeline schedule create a p i error.
func TestPipelineScheduleCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "1", Description: "d", Ref: "main", Cron: "0 * * * *",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleCreate_WithOptionalFields verifies the behavior of pipeline schedule create with optional fields.
func TestPipelineScheduleCreate_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/1/pipeline_schedules" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":5,"description":"Deploy","ref":"main","cron":"0 9 * * 1","cron_timezone":"America/New_York","active":false
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	active := false
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:    "1",
		Description:  "Deploy",
		Ref:          "main",
		Cron:         "0 9 * * 1",
		CronTimezone: "America/New_York",
		Active:       &active,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CronTimezone != "America/New_York" {
		t.Errorf("CronTimezone = %q, want %q", out.CronTimezone, "America/New_York")
	}
	if out.Active {
		t.Error("Active should be false")
	}
}

// ---------------------------------------------------------------------------
// Update — API error, missing project_id, with optional fields
// ---------------------------------------------------------------------------.

// TestPipelineScheduleUpdate_APIError verifies the behavior of pipeline schedule update a p i error.
func TestPipelineScheduleUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleUpdate_MissingProjectID verifies the behavior of pipeline schedule update missing project i d.
func TestPipelineScheduleUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPipelineScheduleUpdate_AllOptionalFields verifies the behavior of pipeline schedule update all optional fields.
func TestPipelineScheduleUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/pipeline_schedules/1" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"New desc","ref":"develop","cron":"30 2 * * *","cron_timezone":"Europe/Berlin","active":true
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	active := true
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:    "1",
		ScheduleID:   1,
		Description:  "New desc",
		Ref:          "develop",
		Cron:         "30 2 * * *",
		CronTimezone: "Europe/Berlin",
		Active:       &active,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CronTimezone != "Europe/Berlin" {
		t.Errorf("CronTimezone = %q, want %q", out.CronTimezone, "Europe/Berlin")
	}
	if out.Ref != "develop" {
		t.Errorf("Ref = %q, want %q", out.Ref, "develop")
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestPipelineScheduleDelete_APIError verifies the behavior of pipeline schedule delete a p i error.
func TestPipelineScheduleDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleDelete_MissingProjectID verifies the behavior of pipeline schedule delete missing project i d.
func TestPipelineScheduleDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := Delete(context.Background(), client, DeleteInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Run — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestPipelineScheduleRun_APIError verifies the behavior of pipeline schedule run a p i error.
func TestPipelineScheduleRun_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Run(context.Background(), client, RunInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleRun_MissingProjectID verifies the behavior of pipeline schedule run missing project i d.
func TestPipelineScheduleRun_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Run(context.Background(), client, RunInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPipelineSchedule_RunGetAfterPlayFails verifies the behavior of pipeline schedule run get after play fails.
func TestPipelineSchedule_RunGetAfterPlayFails(t *testing.T) {
	callCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// Play succeeds
			w.WriteHeader(http.StatusCreated)
			return
		}
		// Get after play fails
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Run(context.Background(), client, RunInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal("expected error when get after run fails, got nil")
	}
}

// ---------------------------------------------------------------------------
// TakeOwnership — API error, canceled context
// ---------------------------------------------------------------------------.

// TestTakeOwnership_APIError verifies the behavior of take ownership a p i error.
func TestTakeOwnership_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := TakeOwnership(context.Background(), client, TakeOwnershipInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestTakeOwnership_CancelledContext verifies the behavior of take ownership cancelled context.
func TestTakeOwnership_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := TakeOwnership(ctx, client, TakeOwnershipInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CreateVariable — API error, missing fields, canceled context
// ---------------------------------------------------------------------------.

// TestCreateVariable_APIError verifies the behavior of create variable a p i error.
func TestCreateVariable_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateVariable_MissingProjectID verifies the behavior of create variable missing project i d.
func TestCreateVariable_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ScheduleID: 1, Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreateVariable_ZeroScheduleID verifies the behavior of create variable zero schedule i d.
func TestCreateVariable_ZeroScheduleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ProjectID: "1", Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal("expected error for zero schedule_id")
	}
}

// TestCreateVariable_CancelledContext verifies the behavior of create variable cancelled context.
func TestCreateVariable_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateVariable(ctx, client, CreateVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestCreateVariable_WithVariableType verifies the behavior of create variable with variable type.
func TestCreateVariable_WithVariableType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/1/pipeline_schedules/1/variables" {
			testutil.RespondJSON(w, http.StatusCreated, `{"key":"SECRET","value":"/tmp/secret","variable_type":"file"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "SECRET", Value: "/tmp/secret", VariableType: "file",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.VariableType != "file" {
		t.Errorf("VariableType = %q, want %q", out.VariableType, "file")
	}
}

// ---------------------------------------------------------------------------
// EditVariable — API error, missing fields, canceled context
// ---------------------------------------------------------------------------.

// TestEditVariable_APIError verifies the behavior of edit variable a p i error.
func TestEditVariable_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := EditVariable(context.Background(), client, EditVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEditVariable_MissingProjectID verifies the behavior of edit variable missing project i d.
func TestEditVariable_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := EditVariable(context.Background(), client, EditVariableInput{
		ScheduleID: 1, Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEditVariable_ZeroScheduleID verifies the behavior of edit variable zero schedule i d.
func TestEditVariable_ZeroScheduleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := EditVariable(context.Background(), client, EditVariableInput{
		ProjectID: "1", Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal("expected error for zero schedule_id")
	}
}

// TestEditVariable_MissingValue verifies the behavior of edit variable missing value.
func TestEditVariable_MissingValue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := EditVariable(context.Background(), client, EditVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "K",
	})
	if err == nil {
		t.Fatal("expected error for missing value")
	}
}

// TestEditVariable_CancelledContext verifies the behavior of edit variable cancelled context.
func TestEditVariable_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := EditVariable(ctx, client, EditVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestEditVariable_WithVariableType verifies the behavior of edit variable with variable type.
func TestEditVariable_WithVariableType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/pipeline_schedules/1/variables/SECRET" {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"SECRET","value":"new-val","variable_type":"file"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := EditVariable(context.Background(), client, EditVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "SECRET", Value: "new-val", VariableType: "file",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.VariableType != "file" {
		t.Errorf("VariableType = %q, want %q", out.VariableType, "file")
	}
}

// ---------------------------------------------------------------------------
// DeleteVariable — missing project_id, zero schedule_id, canceled context
// ---------------------------------------------------------------------------.

// TestDeleteVariable_MissingProjectID verifies the behavior of delete variable missing project i d.
func TestDeleteVariable_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteVariable(context.Background(), client, DeleteVariableInput{
		ScheduleID: 1, Key: "K",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDeleteVariable_ZeroScheduleID verifies the behavior of delete variable zero schedule i d.
func TestDeleteVariable_ZeroScheduleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteVariable(context.Background(), client, DeleteVariableInput{
		ProjectID: "1", Key: "K",
	})
	if err == nil {
		t.Fatal("expected error for zero schedule_id")
	}
}

// TestDeleteVariable_CancelledContext verifies the behavior of delete variable cancelled context.
func TestDeleteVariable_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteVariable(ctx, client, DeleteVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "K",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListTriggeredPipelines — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListTriggeredPipelines_APIError verifies the behavior of list triggered pipelines a p i error.
func TestListTriggeredPipelines_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListTriggeredPipelines(context.Background(), client, ListTriggeredPipelinesInput{
		ProjectID: "1", ScheduleID: 1,
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListTriggeredPipelines_CancelledContext verifies the behavior of list triggered pipelines cancelled context.
func TestListTriggeredPipelines_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListTriggeredPipelines(ctx, client, ListTriggeredPipelinesInput{
		ProjectID: "1", ScheduleID: 1,
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListTriggeredPipelines_WithPagination verifies the behavior of list triggered pipelines with pagination.
func TestListTriggeredPipelines_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/pipeline_schedules/1/pipelines" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":100,"iid":10,"ref":"main","sha":"abc","status":"success","source":"schedule","web_url":"https://example.com/p/100"},
				{"id":101,"iid":11,"ref":"main","sha":"def","status":"failed","source":"schedule","web_url":"https://example.com/p/101"}
			]`, testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListTriggeredPipelines(context.Background(), client, ListTriggeredPipelinesInput{
		ProjectID: "1", ScheduleID: 1, Page: 2, PerPage: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Pipelines) != 2 {
		t.Fatalf("len(Pipelines) = %d, want 2", len(out.Pipelines))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// toOutput — all optional fields (owner, timestamps)
// ---------------------------------------------------------------------------.

// TestToOutput_AllOptionalFields verifies the behavior of to output all optional fields.
func TestToOutput_AllOptionalFields(t *testing.T) {
	out := FormatOutputMarkdown(Output{
		ID:           1,
		Description:  "Nightly",
		Ref:          "main",
		Cron:         "0 1 * * *",
		CronTimezone: "UTC",
		Active:       true,
		OwnerName:    "admin",
		NextRunAt:    "2026-03-08T01:00:00Z",
		CreatedAt:    "2026-01-01T00:00:00Z",
		UpdatedAt:    "2026-03-07T12:00:00Z",
	})

	for _, want := range []string{
		"## Pipeline Schedule #1",
		"| Description | Nightly |",
		"| Ref | main |",
		"| Cron | `0 1 * * *` |",
		"| Timezone | UTC |",
		"| Active | ✅ |",
		"| Next Run | 8 Mar 2026 01:00 UTC |",
		"| Owner | admin |",
		"| Created | 1 Jan 2026 00:00 UTC |",
		"| Updated | 7 Mar 2026 12:00 UTC |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q:\n%s", want, out)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_ZeroID verifies the behavior of format output markdown zero i d.
func TestFormatOutputMarkdown_ZeroID(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for zero ID, got %q", md)
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies the behavior of format output markdown minimal fields.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:          5,
		Description: "Weekly",
		Ref:         "develop",
		Cron:        "0 9 * * 1",
		Active:      false,
	})

	if !strings.Contains(md, "## Pipeline Schedule #5") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{
		"| Timezone |",
		"| Next Run |",
		"| Owner |",
		"| Created |",
		"| Updated |",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithSchedules verifies the behavior of format list markdown with schedules.
func TestFormatListMarkdown_WithSchedules(t *testing.T) {
	out := ListOutput{
		Schedules: []Output{
			{ID: 1, Description: "Nightly", Ref: "main", Cron: "0 1 * * *", Active: true, OwnerName: "admin"},
			{ID: 2, Description: "Weekly", Ref: "develop", Cron: "0 9 * * 1", Active: false, OwnerName: "user1"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Pipeline Schedules (2)",
		"| ID |",
		"| --- |",
		"| 1 |",
		"| 2 |",
		"Nightly",
		"Weekly",
		"admin",
		"user1",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No pipeline schedules found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatVariableMarkdown
// ---------------------------------------------------------------------------.

// TestFormatVariableMarkdown_WithType verifies the behavior of format variable markdown with type.
func TestFormatVariableMarkdown_WithType(t *testing.T) {
	md := FormatVariableMarkdown(VariableOutput{Key: "MY_VAR", Value: "hello", VariableType: "env_var"})

	for _, want := range []string{
		"## Pipeline Schedule Variable",
		"**Key**: MY_VAR",
		"**Value**: hello",
		"**Type**: env_var",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatVariableMarkdown_WithoutType verifies the behavior of format variable markdown without type.
func TestFormatVariableMarkdown_WithoutType(t *testing.T) {
	md := FormatVariableMarkdown(VariableOutput{Key: "K", Value: "V"})
	if strings.Contains(md, "**Type**") {
		t.Error("should not contain Type when empty")
	}
	if !strings.Contains(md, "**Key**: K") {
		t.Errorf("missing key:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatTriggeredPipelinesMarkdown
// ---------------------------------------------------------------------------.

// TestFormatTriggeredPipelinesMarkdown_WithData verifies the behavior of format triggered pipelines markdown with data.
func TestFormatTriggeredPipelinesMarkdown_WithData(t *testing.T) {
	out := TriggeredPipelinesListOutput{
		Pipelines: []TriggeredPipelineOutput{
			{ID: 100, IID: 10, Ref: "main", SHA: "abc", Status: "success", Source: "schedule", WebURL: "https://example.com/100"},
			{ID: 101, IID: 11, Ref: "main", SHA: "def", Status: "failed", Source: "schedule", WebURL: "https://example.com/101"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatTriggeredPipelinesMarkdown(out)

	for _, want := range []string{
		"## Triggered Pipelines (2)",
		"| ID |",
		"| --- |",
		"| [#100](https://example.com/100) |",
		"| [#101](https://example.com/101) |",
		"success",
		"failed",
		"schedule",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatTriggeredPipelinesMarkdown_Empty verifies the behavior of format triggered pipelines markdown empty.
func TestFormatTriggeredPipelinesMarkdown_Empty(t *testing.T) {
	md := FormatTriggeredPipelinesMarkdown(TriggeredPipelinesListOutput{})
	if !strings.Contains(md, "No triggered pipelines found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 11 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newSchedulesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_pipeline_schedule_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_pipeline_schedule_get", map[string]any{"project_id": "1", "schedule_id": 1}},
		{"create", "gitlab_pipeline_schedule_create", map[string]any{"project_id": "1", "description": "nightly", "ref": "main", "cron": "0 1 * * *"}},
		{"update", "gitlab_pipeline_schedule_update", map[string]any{"project_id": "1", "schedule_id": 1, "description": "updated"}},
		{"delete", "gitlab_pipeline_schedule_delete", map[string]any{"project_id": "1", "schedule_id": 1}},
		{"run", "gitlab_pipeline_schedule_run", map[string]any{"project_id": "1", "schedule_id": 1}},
		{"take_ownership", "gitlab_pipeline_schedule_take_ownership", map[string]any{"project_id": "1", "schedule_id": 1}},
		{"create_variable", "gitlab_pipeline_schedule_create_variable", map[string]any{"project_id": "1", "schedule_id": 1, "key": "K", "value": "V"}},
		{"edit_variable", "gitlab_pipeline_schedule_edit_variable", map[string]any{"project_id": "1", "schedule_id": 1, "key": "K", "value": "V2"}},
		{"delete_variable", "gitlab_pipeline_schedule_delete_variable", map[string]any{"project_id": "1", "schedule_id": 1, "key": "K"}},
		{"list_triggered_pipelines", "gitlab_pipeline_schedule_list_triggered_pipelines", map[string]any{"project_id": "1", "schedule_id": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newSchedulesMCPSession is an internal helper for the pipelineschedules package.
func newSchedulesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	scheduleJSON := `{"id":1,"description":"Nightly","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,"owner":{"username":"admin"}}`
	variableJSON := `{"key":"K","value":"V","variable_type":"env_var"}`

	handler := http.NewServeMux()

	// List pipeline schedules
	handler.HandleFunc("GET /api/v4/projects/1/pipeline_schedules", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+scheduleJSON+`]`)
	})

	// Get pipeline schedule
	handler.HandleFunc("GET /api/v4/projects/1/pipeline_schedules/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, scheduleJSON)
	})

	// Create pipeline schedule
	handler.HandleFunc("POST /api/v4/projects/1/pipeline_schedules", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, scheduleJSON)
	})

	// Update pipeline schedule
	handler.HandleFunc("PUT /api/v4/projects/1/pipeline_schedules/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, scheduleJSON)
	})

	// Delete pipeline schedule
	handler.HandleFunc("DELETE /api/v4/projects/1/pipeline_schedules/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Run (play) pipeline schedule
	handler.HandleFunc("POST /api/v4/projects/1/pipeline_schedules/1/play", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	// Take ownership
	handler.HandleFunc("POST /api/v4/projects/1/pipeline_schedules/1/take_ownership", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, scheduleJSON)
	})

	// Create variable
	handler.HandleFunc("POST /api/v4/projects/1/pipeline_schedules/1/variables", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, variableJSON)
	})

	// Edit variable
	handler.HandleFunc("PUT /api/v4/projects/1/pipeline_schedules/1/variables/K", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"K","value":"V2","variable_type":"env_var"}`)
	})

	// Delete variable
	handler.HandleFunc("DELETE /api/v4/projects/1/pipeline_schedules/1/variables/K", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, variableJSON)
	})

	// List triggered pipelines
	handler.HandleFunc("GET /api/v4/projects/1/pipeline_schedules/1/pipelines", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":100,"iid":10,"ref":"main","sha":"abc","status":"success","source":"schedule","web_url":"https://example.com/p/100"}]`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestPipelineScheduleGet_WithTimestamps covers the NextRunAt/CreatedAt/UpdatedAt
// != nil branches in toOutput by providing timestamps in the JSON response.
func TestPipelineScheduleGet_WithTimestamps(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedule1 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"Nightly","ref":"main","cron":"0 1 * * *",
				"cron_timezone":"UTC","active":true,"owner":{"username":"admin"},
				"next_run_at":"2026-06-15T01:00:00Z",
				"created_at":"2026-01-10T08:00:00Z",
				"updated_at":"2026-03-20T12:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "123", ScheduleID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.NextRunAt == "" {
		t.Error("expected non-empty NextRunAt")
	}
	if out.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if out.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
}
