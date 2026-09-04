// pipeline_schedules_test.go contains unit tests for the pipeline schedule MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package pipelineschedules

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpMissingProjectID identifies the err exp missing project ID constant used by this package.
const errExpMissingProjectID = "expected error for missing project_id"

// errExpZeroScheduleID identifies the err exp zero schedule ID constant used by this package.
const errExpZeroScheduleID = "expected error for zero schedule_id"

// errExpMissingKey identifies the err exp missing key constant used by this package.
const errExpMissingKey = "expected error for missing key"

const (
	// testPathSchedules identifies the test path schedules constant used by this package.
	testPathSchedules = "/api/v4/projects/123/pipeline_schedules"
	// testPathSchedule1 identifies the test path schedule 1 constant used by this package.
	testPathSchedule1 = "/api/v4/projects/123/pipeline_schedules/1"
	// testUpdatedDesc identifies the test updated desc constant used by this package.
	testUpdatedDesc = "Updated desc"
)

// ---------------------------------------------------------------------------
// Pipeline Schedule List
// ---------------------------------------------------------------------------.

// TestPipelineScheduleList_Success verifies PipelineScheduleList when success.
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
	if out.Schedules[0].Owner == nil || out.Schedules[0].Owner.Username != "admin" {
		t.Errorf("owner = %+v, want username %q", out.Schedules[0].Owner, "admin")
	}
}

// TestPipelineScheduleList_WithScope verifies PipelineScheduleList when with scope.
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

// TestPipelineScheduleList_KeysetAndSort verifies List forwards order_by, sort,
// and keyset pagination (pagination, page_token) query parameters.
func TestPipelineScheduleList_KeysetAndSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedules {
			q := r.URL.Query()
			if q.Get("order_by") != "id" {
				t.Errorf("order_by = %q, want id", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("sort = %q, want desc", q.Get("sort"))
			}
			if q.Get("pagination") != "keyset" {
				t.Errorf("pagination = %q, want keyset", q.Get("pagination"))
			}
			if q.Get("page_token") != "tok123" {
				t.Errorf("page_token = %q, want tok123", q.Get("page_token"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:  "123",
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPipelineScheduleList_MissingProjectID verifies PipelineScheduleList when missing project ID.
func TestPipelineScheduleList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestPipelineScheduleList_CancelledContext verifies PipelineScheduleList when cancelled context.
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

// assertLastPipelineDocumentedSubset verifies that a last_pipeline reference
// carries the documented fields (id, status, ref) and that the trimmed web_url
// field is not surfaced in the serialized output, per
// doc/api/pipeline_schedules.md.
func assertLastPipelineDocumentedSubset(t *testing.T, lp *LastPipelineOutput) {
	t.Helper()
	if lp == nil || lp.ID != 99 || lp.Status != "success" || lp.Ref != "main" {
		t.Errorf("last_pipeline = %+v, want id 99 status success ref main", lp)
	}
	blob, err := json.Marshal(lp)
	if err != nil {
		t.Fatalf("marshal last_pipeline: %v", err)
	}
	if strings.Contains(string(blob), "web_url") {
		t.Errorf("last_pipeline JSON unexpectedly contains web_url: %s", blob)
	}
}

// TestPipelineScheduleGet_Success verifies PipelineScheduleGet when success.
func TestPipelineScheduleGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedule1 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"Nightly build","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,
				"owner":{"id":7,"username":"admin","name":"Admin User","state":"active","avatar_url":"http://av","web_url":"http://u"},
				"last_pipeline":{"id":99,"sha":"abc","ref":"main","status":"success","web_url":"http://p"},
				"variables":[{"key":"DEPLOY_ENV","value":"prod","variable_type":"env_var"},null],
				"inputs":[{"name":"version","value":"1.2.3"},null]
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
	if out.Owner == nil || out.Owner.ID != 7 || out.Owner.Name != "Admin User" || out.Owner.State != "active" {
		t.Errorf("owner = %+v, want full user object", out.Owner)
	}
	assertLastPipelineDocumentedSubset(t, out.LastPipeline)
	if len(out.Variables) != 1 || out.Variables[0].Key != "DEPLOY_ENV" || out.Variables[0].VariableType != "env_var" {
		t.Errorf("variables = %+v, want one DEPLOY_ENV env_var", out.Variables)
	}
	if len(out.Inputs) != 1 || out.Inputs[0].Name != "version" || out.Inputs[0].Value != "1.2.3" {
		t.Errorf("inputs = %+v, want one version=1.2.3", out.Inputs)
	}
}

// TestPipelineScheduleGet_RawVariableSurfaced verifies that the documented
// variables[].raw boolean — which the SDK gl.PipelineVariable omits — is
// surfaced through the raw REST superset fetch on the get-schedule handler.
func TestPipelineScheduleGet_RawVariableSurfaced(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedule1 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"Nightly build","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,
				"variables":[{"key":"TOKEN","value":"$secret","variable_type":"env_var","raw":true}]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "123", ScheduleID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Variables) != 1 || !out.Variables[0].Raw {
		t.Fatalf("variables = %+v, want one variable with raw=true", out.Variables)
	}
	blob, err := json.Marshal(out.Variables[0])
	if err != nil {
		t.Fatalf("marshal variable: %v", err)
	}
	if !strings.Contains(string(blob), `"raw":true`) {
		t.Errorf("variable JSON missing raw=true: %s", blob)
	}
}

// TestPipelineScheduleGet_RawVariableAbsent_VersionTolerance verifies that a
// schedule whose variables[] entries omit the raw field (older GitLab versions)
// still decodes successfully, leaving raw at its false zero value and omitting it
// from the serialized output via the omitempty tag.
func TestPipelineScheduleGet_RawVariableAbsent_VersionTolerance(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedule1 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"Nightly build","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,
				"variables":[{"key":"DEPLOY_ENV","value":"prod","variable_type":"env_var"}]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "123", ScheduleID: 1})
	if err != nil {
		t.Fatalf("version-tolerant decode must not fail: %v", err)
	}
	if len(out.Variables) != 1 || out.Variables[0].Key != "DEPLOY_ENV" {
		t.Fatalf("variables = %+v, want one DEPLOY_ENV variable", out.Variables)
	}
	if out.Variables[0].Raw {
		t.Errorf("raw = true, want false when field absent")
	}
	blob, err := json.Marshal(out.Variables[0])
	if err != nil {
		t.Fatalf("marshal variable: %v", err)
	}
	if strings.Contains(string(blob), "raw") {
		t.Errorf("variable JSON unexpectedly contains raw when absent: %s", blob)
	}
}

// TestToOutput_SDKConverter_FullSchedule exercises the SDK-wrapper toOutput
// converter (still used by the list/create/update/take-ownership handlers, which
// return the gl.PipelineSchedule wrapper) across all of its sub-object and
// timestamp branches: owner, last_pipeline, variables, inputs, next_run_at,
// created_at, and updated_at. The get/run handlers now use the raw superset path
// (toOutputAPI), so this direct test keeps the SDK converter fully covered.
func TestToOutput_SDKConverter_FullSchedule(t *testing.T) {
	now := time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC)
	destroy := true
	s := &gitlab.PipelineSchedule{
		ID:           1,
		Description:  "Nightly",
		Ref:          "main",
		Cron:         "0 1 * * *",
		CronTimezone: "UTC",
		Active:       true,
		NextRunAt:    &now,
		CreatedAt:    &now,
		UpdatedAt:    &now,
		Owner:        &gitlab.User{ID: 7, Username: "admin", Name: "Admin User", State: "active"},
		LastPipeline: &gitlab.LastPipeline{ID: 99, SHA: "abc", Ref: "main", Status: "success"},
		Variables: []*gitlab.PipelineVariable{
			{Key: "DEPLOY_ENV", Value: "prod", VariableType: gitlab.EnvVariableType},
			nil,
		},
		Inputs: []*gitlab.PipelineInput{
			{Name: "version", Value: "1.2.3", Destroy: &destroy},
			nil,
		},
	}

	out := toOutput(s)

	if out.Owner == nil || out.Owner.ID != 7 {
		t.Errorf("owner = %+v, want id 7", out.Owner)
	}
	if out.LastPipeline == nil || out.LastPipeline.ID != 99 {
		t.Errorf("last_pipeline = %+v, want id 99", out.LastPipeline)
	}
	if len(out.Variables) != 1 || out.Variables[0].Key != "DEPLOY_ENV" {
		t.Errorf("variables = %+v, want one DEPLOY_ENV variable (nil skipped)", out.Variables)
	}
	if len(out.Inputs) != 1 || out.Inputs[0].Name != "version" {
		t.Errorf("inputs = %+v, want one version input (nil skipped)", out.Inputs)
	}
	if out.NextRunAt == "" || out.CreatedAt == "" || out.UpdatedAt == "" {
		t.Errorf("timestamps not formatted: %+v", out)
	}
}

// TestPipelineSchedule_GetZeroID verifies PipelineSchedule when get zero ID.
func TestPipelineSchedule_GetZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "123", ScheduleID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// TestPipelineScheduleGet_CancelledContext verifies PipelineScheduleGet when cancelled context.
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

// TestPipelineScheduleCreate_Success verifies PipelineScheduleCreate when success.
func TestPipelineScheduleCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedules && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"name":"version"`) {
				t.Errorf("create body missing inputs: %s", body)
			}
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":10,"description":"Weekly deploy","ref":"main","cron":"0 9 * * 1","cron_timezone":"UTC","active":true,
				"inputs":[{"name":"version","value":"1.0.0"}]
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	active := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:    "123",
		Description:  "Weekly deploy",
		Ref:          "main",
		Cron:         "0 9 * * 1",
		CronTimezone: "UTC",
		Active:       &active,
		Inputs:       []InputObject{{Name: "version", Value: "1.0.0"}},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("id = %d, want 10", out.ID)
	}
	if len(out.Inputs) != 1 || out.Inputs[0].Name != "version" {
		t.Errorf("inputs = %+v, want one version input", out.Inputs)
	}
}

// TestPipelineScheduleCreate_MissingFields covers PipelineScheduleCreate with table-driven subtests for missing fields.
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

// TestPipelineScheduleCreate_CancelledContext verifies PipelineScheduleCreate when cancelled context.
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

// TestPipelineScheduleUpdate_Success verifies PipelineScheduleUpdate when success.
func TestPipelineScheduleUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testPathSchedule1 && r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"name":"old_input"`) {
				t.Errorf("update body missing inputs: %s", body)
			}
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"description":"Updated desc","ref":"develop","cron":"0 2 * * *","cron_timezone":"UTC","active":false
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	destroy := true
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:   "123",
		ScheduleID:  1,
		Description: testUpdatedDesc,
		Inputs:      []InputObject{{Name: "old_input", Destroy: &destroy}},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Description != testUpdatedDesc {
		t.Errorf("description = %q, want %q", out.Description, testUpdatedDesc)
	}
}

// TestPipelineSchedule_UpdateZeroID verifies PipelineSchedule when update zero ID.
func TestPipelineSchedule_UpdateZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "123", ScheduleID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// TestPipelineScheduleUpdate_CancelledContext verifies PipelineScheduleUpdate when cancelled context.
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

// TestPipelineScheduleDelete_Success verifies PipelineScheduleDelete when success.
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

// TestPipelineSchedule_DeleteZeroID verifies PipelineSchedule when delete zero ID.
func TestPipelineSchedule_DeleteZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "123", ScheduleID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// TestPipelineScheduleDelete_CancelledContext verifies PipelineScheduleDelete when cancelled context.
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

// TestPipelineScheduleRun_Success verifies PipelineScheduleRun when success.
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

// TestPipelineSchedule_RunZeroID verifies PipelineSchedule when run zero ID.
func TestPipelineSchedule_RunZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := Run(context.Background(), client, RunInput{
		ProjectID: "123", ScheduleID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroScheduleID)
	}
}

// TestPipelineScheduleRun_CancelledContext verifies PipelineScheduleRun when cancelled context.
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

// scheduleJSON identifies the schedule JSON constant used by this package.
const scheduleJSON = `{"id":1,"description":"Nightly","ref":"main","cron":"0 1 * * *","cron_timezone":"UTC","active":true,"owner":{"username":"newowner"}}`

// TestTakeOwnership_Success verifies TakeOwnership when success.
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
	if out.Owner == nil || out.Owner.Username != "newowner" {
		t.Errorf("Owner = %+v, want username %q", out.Owner, "newowner")
	}
}

// TestTakeOwnership_MissingProjectID verifies TakeOwnership when missing project ID.
func TestTakeOwnership_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := TakeOwnership(context.Background(), client, TakeOwnershipInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestTakeOwnership_ZeroScheduleID verifies TakeOwnership when zero schedule ID.
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

// TestCreateVariable_Success verifies CreateVariable when success.
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

// TestCreateVariable_MissingKey verifies CreateVariable when missing key.
func TestCreateVariable_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ProjectID: "42", ScheduleID: 1, Value: "val",
	})
	if err == nil {
		t.Fatal(errExpMissingKey)
	}
}

// TestCreateVariable_MissingValue verifies CreateVariable when missing value.
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

// TestEditVariable_Success verifies EditVariable when success.
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

// TestEditVariable_MissingKey verifies EditVariable when missing key.
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

// TestDeleteVariable_Success verifies DeleteVariable when success.
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

// TestDeleteVariable_MissingKey verifies DeleteVariable when missing key.
func TestDeleteVariable_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	err := DeleteVariable(context.Background(), client, DeleteVariableInput{
		ProjectID: "42", ScheduleID: 1,
	})
	if err == nil {
		t.Fatal(errExpMissingKey)
	}
}

// TestDeleteVariable_APIError verifies DeleteVariable when API error.
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

// TestListTriggeredPipelines_Success verifies ListTriggeredPipelines when success.
func TestListTriggeredPipelines_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipeline_schedules/1/pipelines" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":100,"iid":10,"project_id":42,"ref":"main","sha":"abc","status":"success","source":"schedule","web_url":"https://example.com/p/100","created_at":"2016-08-11T11:28:34.085Z","updated_at":"2016-08-11T11:32:35.169Z"}
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
	p := out.Pipelines[0]
	if p.ID != 100 {
		t.Errorf("ID = %d, want 100", p.ID)
	}
	if p.Source != "schedule" {
		t.Errorf("Source = %q, want %q", p.Source, "schedule")
	}
	if p.ProjectID != 42 {
		t.Errorf("ProjectID = %d, want 42", p.ProjectID)
	}
	if p.CreatedAt != "2016-08-11T11:28:34Z" {
		t.Errorf("CreatedAt = %q, want 2016-08-11T11:28:34Z", p.CreatedAt)
	}
	if p.UpdatedAt != "2016-08-11T11:32:35Z" {
		t.Errorf("UpdatedAt = %q, want 2016-08-11T11:32:35Z", p.UpdatedAt)
	}
}

// TestListTriggeredPipelines_MissingProjectID verifies ListTriggeredPipelines when missing project ID.
func TestListTriggeredPipelines_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { /* no response body needed */ }))
	_, err := ListTriggeredPipelines(context.Background(), client, ListTriggeredPipelinesInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestListTriggeredPipelines_ZeroScheduleID verifies ListTriggeredPipelines when zero schedule ID.
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error
// ---------------------------------------------------------------------------.

// TestPipelineScheduleList_APIError verifies PipelineScheduleList when API error.
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

// TestPipelineScheduleGet_APIError verifies PipelineScheduleGet when API error.
func TestPipelineScheduleGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleGet_MissingProjectID verifies PipelineScheduleGet when missing project ID.
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

// TestPipelineScheduleCreate_APIError verifies PipelineScheduleCreate when API error.
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

// TestPipelineScheduleCreate_StatusErrorBranches verifies create status-specific hints.
func TestPipelineScheduleCreate_StatusErrorBranches(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		wantText   string
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, wantText: "check cron expression"},
		{name: "not found", statusCode: http.StatusNotFound, wantText: "gitlab_project_get"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, testCase.statusCode, `{"message":"failed"}`)
			}))
			_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Description: "d", Ref: "main", Cron: "0 * * * *"})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("error = %v, want %q", err, testCase.wantText)
			}
		})
	}
}

// TestPipelineScheduleCreate_WithOptionalFields verifies PipelineScheduleCreate when with optional fields.
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

// TestPipelineScheduleUpdate_APIError verifies PipelineScheduleUpdate when API error.
func TestPipelineScheduleUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleUpdate_NotFound verifies update not-found hints.
func TestPipelineScheduleUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), hintVerifyScheduleID) {
		t.Fatalf("error = %v, want schedule hint", err)
	}
}

// TestPipelineScheduleUpdate_MissingProjectID verifies PipelineScheduleUpdate when missing project ID.
func TestPipelineScheduleUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Update(context.Background(), client, UpdateInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPipelineScheduleUpdate_AllOptionalFields verifies PipelineScheduleUpdate when all optional fields.
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

// TestPipelineScheduleDelete_APIError verifies PipelineScheduleDelete when API error.
func TestPipelineScheduleDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleDelete_NotFound verifies delete not-found hints.
func TestPipelineScheduleDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), hintVerifyScheduleID) {
		t.Fatalf("error = %v, want schedule hint", err)
	}
}

// TestPipelineScheduleDelete_MissingProjectID verifies PipelineScheduleDelete when missing project ID.
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

// TestPipelineScheduleRun_APIError verifies PipelineScheduleRun when API error.
func TestPipelineScheduleRun_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Run(context.Background(), client, RunInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPipelineScheduleRun_StatusErrorBranches verifies run status-specific hints.
func TestPipelineScheduleRun_StatusErrorBranches(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		wantText   string
	}{
		{name: "rate limited", statusCode: http.StatusTooManyRequests, wantText: "rate-limited"},
		{name: "not found", statusCode: http.StatusNotFound, wantText: hintVerifyScheduleID},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, testCase.statusCode, `{"message":"failed"}`)
			}))
			_, err := Run(context.Background(), client, RunInput{ProjectID: "1", ScheduleID: 1})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("error = %v, want %q", err, testCase.wantText)
			}
		})
	}
}

// TestPipelineScheduleRun_MissingProjectID verifies PipelineScheduleRun when missing project ID.
func TestPipelineScheduleRun_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Run(context.Background(), client, RunInput{ScheduleID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestPipelineSchedule_RunGetAfterPlayFails verifies PipelineSchedule when run get after play fails.
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

// TestTakeOwnership_APIError verifies TakeOwnership when API error.
func TestTakeOwnership_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := TakeOwnership(context.Background(), client, TakeOwnershipInput{ProjectID: "1", ScheduleID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestTakeOwnership_CancelledContext verifies TakeOwnership when cancelled context.
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

// TestCreateVariable_APIError verifies CreateVariable when API error.
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

// TestCreateVariable_BadRequest verifies invalid variable key hints.
func TestCreateVariable_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad key"}`)
	}))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{ProjectID: "1", ScheduleID: 1, Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "variable key must match") {
		t.Fatalf("error = %v, want variable key hint", err)
	}
}

// TestCreateVariable_MissingProjectID verifies CreateVariable when missing project ID.
func TestCreateVariable_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ScheduleID: 1, Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreateVariable_ZeroScheduleID verifies CreateVariable when zero schedule ID.
func TestCreateVariable_ZeroScheduleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateVariable(context.Background(), client, CreateVariableInput{
		ProjectID: "1", Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal("expected error for zero schedule_id")
	}
}

// TestCreateVariable_CancelledContext verifies CreateVariable when cancelled context.
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

// TestCreateVariable_WithVariableType verifies CreateVariable when with variable type.
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

// TestEditVariable_APIError verifies EditVariable when API error.
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

// TestEditVariable_MissingProjectID verifies EditVariable when missing project ID.
func TestEditVariable_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := EditVariable(context.Background(), client, EditVariableInput{
		ScheduleID: 1, Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEditVariable_ZeroScheduleID verifies EditVariable when zero schedule ID.
func TestEditVariable_ZeroScheduleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := EditVariable(context.Background(), client, EditVariableInput{
		ProjectID: "1", Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal("expected error for zero schedule_id")
	}
}

// TestEditVariable_MissingValue verifies EditVariable when missing value.
func TestEditVariable_MissingValue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := EditVariable(context.Background(), client, EditVariableInput{
		ProjectID: "1", ScheduleID: 1, Key: "K",
	})
	if err == nil {
		t.Fatal("expected error for missing value")
	}
}

// TestEditVariable_CancelledContext verifies EditVariable when cancelled context.
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

// TestEditVariable_WithVariableType verifies EditVariable when with variable type.
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

// TestDeleteVariable_MissingProjectID verifies DeleteVariable when missing project ID.
func TestDeleteVariable_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteVariable(context.Background(), client, DeleteVariableInput{
		ScheduleID: 1, Key: "K",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDeleteVariable_ZeroScheduleID verifies DeleteVariable when zero schedule ID.
func TestDeleteVariable_ZeroScheduleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteVariable(context.Background(), client, DeleteVariableInput{
		ProjectID: "1", Key: "K",
	})
	if err == nil {
		t.Fatal("expected error for zero schedule_id")
	}
}

// TestDeleteVariable_CancelledContext verifies DeleteVariable when cancelled context.
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

// TestListTriggeredPipelines_APIError verifies ListTriggeredPipelines when API error.
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

// TestListTriggeredPipelines_CancelledContext verifies ListTriggeredPipelines when cancelled context.
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

// TestListTriggeredPipelines_WithPagination verifies ListTriggeredPipelines when with pagination.
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
		ProjectID: "1", ScheduleID: 1,
		Page: 2, PerPage: 2,
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

// TestListTriggeredPipelines_OrderByAndSort verifies ListTriggeredPipelines
// forwards the order_by and sort query parameters to the GitLab API.
func TestListTriggeredPipelines_OrderByAndSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipeline_schedules/1/pipelines" {
			q := r.URL.Query()
			if q.Get("order_by") != "id" {
				t.Errorf("order_by = %q, want id", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("sort = %q, want desc", q.Get("sort"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":100,"iid":10,"project_id":42,"ref":"main","sha":"abc","status":"success","source":"schedule","web_url":"https://example.com/p/100"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListTriggeredPipelines(context.Background(), client, ListTriggeredPipelinesInput{
		ProjectID: "42", ScheduleID: 1,
		OrderBy: "id", Sort: "desc",
	})
	if err != nil {
		t.Fatalf("ListTriggeredPipelines() error: %v", err)
	}
	if len(out.Pipelines) != 1 {
		t.Fatalf("len(Pipelines) = %d, want 1", len(out.Pipelines))
	}
}

// ---------------------------------------------------------------------------
// toOutput — all optional fields (owner, timestamps)
// ---------------------------------------------------------------------------.

// TestToOutput_AllOptionalFields verifies ToOutput when all optional fields.
func TestToOutput_AllOptionalFields(t *testing.T) {
	out := FormatOutputMarkdown(Output{
		ID:           1,
		Description:  "Nightly",
		Ref:          "main",
		Cron:         "0 1 * * *",
		CronTimezone: "UTC",
		Active:       true,
		Owner:        &OwnerOutput{Username: "admin"},
		LastPipeline: &LastPipelineOutput{ID: 99, Status: "success"},
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
		"| Last Pipeline |",
		"#99 (success)",
		"| Created | 1 Jan 2026 00:00 UTC |",
		"| Updated | 7 Mar 2026 12:00 UTC |",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("markdown missing %q:\n%s", want, out)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_ZeroID verifies FormatOutputMarkdown when zero ID.
func TestFormatOutputMarkdown_ZeroID(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for zero ID, got %q", md)
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies FormatOutputMarkdown when minimal fields.
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
		t.Run(absent, func(t *testing.T) {
			if strings.Contains(md, absent) {
				t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithSchedules verifies FormatListMarkdown when with schedules.
func TestFormatListMarkdown_WithSchedules(t *testing.T) {
	out := ListOutput{
		Schedules: []Output{
			{ID: 1, Description: "Nightly", Ref: "main", Cron: "0 1 * * *", Active: true, Owner: &OwnerOutput{Username: "admin"}},
			{ID: 2, Description: "Weekly", Ref: "develop", Cron: "0 9 * * 1", Active: false, Owner: &OwnerOutput{Username: "user1"}},
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
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

// TestFormatVariableMarkdown_WithType verifies FormatVariableMarkdown when with type.
func TestFormatVariableMarkdown_WithType(t *testing.T) {
	md := FormatVariableMarkdown(VariableOutput{Key: "MY_VAR", Value: "hello", VariableType: "env_var"})

	for _, want := range []string{
		"## Pipeline Schedule Variable",
		"**Key**: MY_VAR",
		"**Value**: hello",
		"**Type**: env_var",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatVariableMarkdown_WithoutType verifies FormatVariableMarkdown when without type.
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

// TestFormatTriggeredPipelinesMarkdown_WithData verifies FormatTriggeredPipelinesMarkdown when with data.
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatTriggeredPipelinesMarkdown_Empty verifies FormatTriggeredPipelinesMarkdown when empty.
func TestFormatTriggeredPipelinesMarkdown_Empty(t *testing.T) {
	md := FormatTriggeredPipelinesMarkdown(TriggeredPipelinesListOutput{})
	if !strings.Contains(md, "No triggered pipelines found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
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
