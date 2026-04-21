// civariables_test.go contains unit tests for the CI/CD variable MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package civariables

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const errExpectedErr = "expected error"

const errExpCancelledCtx = "expected error for canceled context"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// CI Variable List
// ---------------------------------------------------------------------------.

// TestCIVariableList_Success verifies the behavior of c i variable list success.
func TestCIVariableList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/variables" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"key":"DB_HOST","value":"localhost","variable_type":"env_var","protected":false,"masked":false,"hidden":false,"raw":false,"environment_scope":"*","description":"Database host"}
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
	if len(out.Variables) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(out.Variables))
	}
	if out.Variables[0].Key != "DB_HOST" {
		t.Errorf("key = %q, want %q", out.Variables[0].Key, "DB_HOST")
	}
	if out.Variables[0].Description != "Database host" {
		t.Errorf("description = %q, want %q", out.Variables[0].Description, "Database host")
	}
}

// TestCIVariableList_MissingProjectID verifies the behavior of c i variable list missing project i d.
func TestCIVariableList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestCIVariableList_CancelledContext verifies the behavior of c i variable list cancelled context.
func TestCIVariableList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CI Variable Get
// ---------------------------------------------------------------------------.

// TestCIVariableGet_Success verifies the behavior of c i variable get success.
func TestCIVariableGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/variables/DB_HOST" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"DB_HOST","value":"localhost","variable_type":"env_var","protected":true,"masked":false,"hidden":false,"raw":false,"environment_scope":"*"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "123",
		Key:       "DB_HOST",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Key != "DB_HOST" {
		t.Errorf("key = %q, want %q", out.Key, "DB_HOST")
	}
	if !out.Protected {
		t.Error("expected protected=true")
	}
}

// TestCIVariableGet_MissingFields validates c i variable get missing fields across multiple scenarios using table-driven subtests.
func TestCIVariableGet_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	tests := []struct {
		name  string
		input GetInput
	}{
		{"missing project_id", GetInput{Key: "K"}},
		{"missing key", GetInput{ProjectID: "1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Get(context.Background(), client, tc.input)
			if err == nil {
				t.Fatal(errExpectedErr)
			}
		})
	}
}

// TestCIVariableGet_CancelledContext verifies the behavior of c i variable get cancelled context.
func TestCIVariableGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "1", Key: "K"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CI Variable Create
// ---------------------------------------------------------------------------.

// TestCIVariableCreate_Success verifies the behavior of c i variable create success.
func TestCIVariableCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/variables" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"key":"API_KEY","value":"secret123","variable_type":"env_var","protected":true,"masked":true,"hidden":false,"raw":false,"environment_scope":"production"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "123",
		Key:       "API_KEY",
		Value:     "secret123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Key != "API_KEY" {
		t.Errorf("key = %q, want %q", out.Key, "API_KEY")
	}
}

// TestCIVariableCreate_MissingFields validates c i variable create missing fields across multiple scenarios using table-driven subtests.
func TestCIVariableCreate_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	tests := []struct {
		name  string
		input CreateInput
	}{
		{"missing project_id", CreateInput{Key: "K", Value: "V"}},
		{"missing key", CreateInput{ProjectID: "1", Value: "V"}},
		{"missing value", CreateInput{ProjectID: "1", Key: "K"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Create(context.Background(), client, tc.input)
			if err == nil {
				t.Fatal(errExpectedErr)
			}
		})
	}
}

// TestCIVariableCreate_CancelledContext verifies the behavior of c i variable create cancelled context.
func TestCIVariableCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{
		ProjectID: "1", Key: "K", Value: "V",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CI Variable Update
// ---------------------------------------------------------------------------.

// TestCIVariableUpdate_Success verifies the behavior of c i variable update success.
func TestCIVariableUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/variables/DB_HOST" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"DB_HOST","value":"db.prod.internal","variable_type":"env_var","protected":true,"masked":false,"hidden":false,"raw":false,"environment_scope":"production"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID: "123",
		Key:       "DB_HOST",
		Value:     "db.prod.internal",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "db.prod.internal" {
		t.Errorf("value = %q, want %q", out.Value, "db.prod.internal")
	}
}

// TestCIVariableUpdate_MissingFields validates c i variable update missing fields across multiple scenarios using table-driven subtests.
func TestCIVariableUpdate_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	tests := []struct {
		name  string
		input UpdateInput
	}{
		{"missing project_id", UpdateInput{Key: "K"}},
		{"missing key", UpdateInput{ProjectID: "1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Update(context.Background(), client, tc.input)
			if err == nil {
				t.Fatal(errExpectedErr)
			}
		})
	}
}

// TestCIVariableUpdate_CancelledContext verifies the behavior of c i variable update cancelled context.
func TestCIVariableUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{
		ProjectID: "1", Key: "K",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CI Variable Delete
// ---------------------------------------------------------------------------.

// TestCIVariableDelete_Success verifies the behavior of c i variable delete success.
func TestCIVariableDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/123/variables/DB_HOST" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "123", Key: "DB_HOST",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestCIVariableDelete_MissingFields validates c i variable delete missing fields across multiple scenarios using table-driven subtests.
func TestCIVariableDelete_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	tests := []struct {
		name  string
		input DeleteInput
	}{
		{"missing project_id", DeleteInput{Key: "K"}},
		{"missing key", DeleteInput{ProjectID: "1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Delete(context.Background(), client, tc.input)
			if err == nil {
				t.Fatal(errExpectedErr)
			}
		})
	}
}

// TestCIVariableDelete_CancelledContext verifies the behavior of c i variable delete cancelled context.
func TestCIVariableDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{
		ProjectID: "1", Key: "K",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// toOutput — auto-masking of masked/hidden variables
// ---------------------------------------------------------------------------.

// TestToOutput_AutoMasking verifies that toOutput redacts values for masked
// and hidden variables to prevent accidental secret exposure in JSON responses.
func TestToOutput_AutoMasking(t *testing.T) {
	tests := []struct {
		name      string
		masked    bool
		hidden    bool
		rawValue  string
		wantValue string
	}{
		{"unmasked variable exposes value", false, false, "secret123", "secret123"},
		{"masked variable redacts value", true, false, "secret123", "[masked]"},
		{"hidden variable redacts value", false, true, "secret123", "[masked]"},
		{"masked+hidden variable redacts value", true, true, "secret123", "[masked]"},
		{"masked with empty value still redacts", true, false, "", "[masked]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &gitlab.ProjectVariable{
				Key:    "TEST_KEY",
				Value:  tc.rawValue,
				Masked: tc.masked,
				Hidden: tc.hidden,
			}
			out := toOutput(v)
			if out.Value != tc.wantValue {
				t.Errorf("Value = %q, want %q", out.Value, tc.wantValue)
			}
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const (
	msgServerError = "server error"
	errExpectedAPI = "expected API error, got nil"
	testEnvScope   = "production"

	fmtEnvironmentScope = "EnvironmentScope = %q, want %q"
)

// ---------------------------------------------------------------------------
// List — API error, with pagination parameters
// ---------------------------------------------------------------------------.

// TestCIVariableList_APIError verifies the behavior of c i variable list a p i error.
func TestCIVariableList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCIVariableList_WithPagination verifies the behavior of c i variable list with pagination.
func TestCIVariableList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/variables" && r.Method == http.MethodGet {
			if r.URL.Query().Get("page") != "2" {
				t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"key":"VAR_A","value":"a","variable_type":"env_var","protected":false,"masked":false,"hidden":false,"raw":false,"environment_scope":"*"},
				{"key":"VAR_B","value":"b","variable_type":"file","protected":true,"masked":true,"hidden":false,"raw":true,"environment_scope":"staging"}
			]`, testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:       "42",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 2},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Variables) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(out.Variables))
	}
	if out.Variables[0].Key != "VAR_A" {
		t.Errorf("first key = %q, want %q", out.Variables[0].Key, "VAR_A")
	}
	if out.Variables[1].Protected != true {
		t.Error("expected second variable protected=true")
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// TestCIVariableList_EmptyResult verifies the behavior of c i variable list empty result.
func TestCIVariableList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/variables" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Variables) != 0 {
		t.Fatalf("expected 0 variables, got %d", len(out.Variables))
	}
}

// ---------------------------------------------------------------------------
// Get — API error, with environment_scope
// ---------------------------------------------------------------------------.

// TestCIVariableGet_APIError verifies the behavior of c i variable get a p i error.
func TestCIVariableGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCIVariableGet_WithEnvironmentScope verifies the behavior of c i variable get with environment scope.
func TestCIVariableGet_WithEnvironmentScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/10/variables/DB_URL" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"DB_URL","value":"postgres://prod","variable_type":"env_var","protected":true,"masked":true,"hidden":false,"raw":false,"environment_scope":"production","description":"Production DB"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:        "10",
		Key:              "DB_URL",
		EnvironmentScope: testEnvScope,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.EnvironmentScope != testEnvScope {
		t.Errorf(fmtEnvironmentScope, out.EnvironmentScope, testEnvScope)
	}
	if out.Description != "Production DB" {
		t.Errorf("Description = %q, want %q", out.Description, "Production DB")
	}
	if !out.Masked {
		t.Error("expected masked=true")
	}
}

// ---------------------------------------------------------------------------
// Create — API error, all optional fields
// ---------------------------------------------------------------------------.

// TestCIVariableCreate_APIError verifies the behavior of c i variable create a p i error.
func TestCIVariableCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCIVariableCreate_AllOptionalFields verifies the behavior of c i variable create all optional fields.
func TestCIVariableCreate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/variables" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"key":"SECRET_FILE","value":"/tmp/secret","variable_type":"file",
				"protected":true,"masked":true,"hidden":true,"raw":true,
				"environment_scope":"production","description":"Secret file for deploy"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	bTrue := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:        "1",
		Key:              "SECRET_FILE",
		Value:            "/tmp/secret",
		Description:      "Secret file for deploy",
		VariableType:     "file",
		Protected:        &bTrue,
		Masked:           &bTrue,
		MaskedAndHidden:  &bTrue,
		Raw:              &bTrue,
		EnvironmentScope: testEnvScope,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.VariableType != "file" {
		t.Errorf("VariableType = %q, want %q", out.VariableType, "file")
	}
	if !out.Protected {
		t.Error("expected protected=true")
	}
	if !out.Hidden {
		t.Error("expected hidden=true")
	}
	if !out.Raw {
		t.Error("expected raw=true")
	}
	if out.EnvironmentScope != testEnvScope {
		t.Errorf(fmtEnvironmentScope, out.EnvironmentScope, testEnvScope)
	}
}

// ---------------------------------------------------------------------------
// Update — API error, all optional fields
// ---------------------------------------------------------------------------.

// TestCIVariableUpdate_APIError verifies the behavior of c i variable update a p i error.
func TestCIVariableUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCIVariableUpdate_AllOptionalFields verifies the behavior of c i variable update all optional fields.
func TestCIVariableUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/variables/DB_HOST" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{
				"key":"DB_HOST","value":"db.prod","variable_type":"file",
				"protected":true,"masked":true,"hidden":false,"raw":true,
				"environment_scope":"staging","description":"Updated"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	bTrue := true
	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:        "1",
		Key:              "DB_HOST",
		Value:            "db.prod",
		Description:      "Updated",
		VariableType:     "file",
		Protected:        &bTrue,
		Masked:           &bTrue,
		Raw:              &bTrue,
		EnvironmentScope: "staging",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.VariableType != "file" {
		t.Errorf("VariableType = %q, want %q", out.VariableType, "file")
	}
	if out.EnvironmentScope != "staging" {
		t.Errorf(fmtEnvironmentScope, out.EnvironmentScope, "staging")
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, with environment_scope
// ---------------------------------------------------------------------------.

// TestCIVariableDelete_APIError verifies the behavior of c i variable delete a p i error.
func TestCIVariableDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCIVariableDelete_WithEnvironmentScope verifies the behavior of c i variable delete with environment scope.
func TestCIVariableDelete_WithEnvironmentScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/variables/DB_HOST" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:        "1",
		Key:              "DB_HOST",
		EnvironmentScope: "staging",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_EmptyKey verifies the behavior of format output markdown empty key.
func TestFormatOutputMarkdown_EmptyKey(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for empty key, got %q", md)
	}
}

// TestFormatOutputMarkdown_FullUnmasked verifies the behavior of format output markdown full unmasked.
func TestFormatOutputMarkdown_FullUnmasked(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "DB_HOST",
		Value:            "localhost",
		VariableType:     "env_var",
		Protected:        true,
		Masked:           false,
		Hidden:           false,
		Raw:              true,
		EnvironmentScope: testEnvScope,
		Description:      "Database host",
	})

	for _, want := range []string{
		"## Variable: DB_HOST",
		"| Type | env_var |",
		"| Protected | ✅ |",
		"| Masked | ❌ |",
		"| Raw | ✅ |",
		"| Environment Scope | production |",
		"| Description | Database host |",
		"| Value | localhost |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "| Hidden | ✅ |") {
		t.Error("should not contain Hidden line when hidden=false")
	}
}

// TestFormatOutputMarkdown_MaskedValue verifies the behavior of format output markdown masked value.
func TestFormatOutputMarkdown_MaskedValue(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "SECRET",
		Value:            "super-secret",
		VariableType:     "env_var",
		Masked:           true,
		EnvironmentScope: "*",
	})

	if !strings.Contains(md, "| Value | [masked] |") {
		t.Errorf("expected masked value placeholder:\n%s", md)
	}
	if strings.Contains(md, "super-secret") {
		t.Error("masked value should not appear in markdown")
	}
}

// TestFormatOutputMarkdown_HiddenValue verifies the behavior of format output markdown hidden value.
func TestFormatOutputMarkdown_HiddenValue(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "TOKEN",
		Value:            "",
		VariableType:     "env_var",
		Hidden:           true,
		EnvironmentScope: "*",
	})

	if !strings.Contains(md, "| Hidden | ✅ |") {
		t.Errorf("expected Hidden line:\n%s", md)
	}
	if !strings.Contains(md, "| Value | [masked] |") {
		t.Errorf("hidden variable should show [masked]:\n%s", md)
	}
}

// TestFormatOutputMarkdown_NoDescription verifies the behavior of format output markdown no description.
func TestFormatOutputMarkdown_NoDescription(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "SIMPLE",
		Value:            "val",
		VariableType:     "env_var",
		EnvironmentScope: "*",
	})

	if strings.Contains(md, "| Description |") {
		t.Error("should not contain Description when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithVariables verifies the behavior of format list markdown with variables.
func TestFormatListMarkdown_WithVariables(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "DB_HOST", VariableType: "env_var", Protected: false, Masked: false, EnvironmentScope: "*"},
			{Key: "API_KEY", VariableType: "env_var", Protected: true, Masked: true, EnvironmentScope: testEnvScope},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## CI/CD Variables (2)",
		"| Key |",
		"| --- |",
		"| DB_HOST |",
		"| API_KEY |",
		"env_var",
		testEnvScope,
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No CI/CD variables found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| Key |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_EscapesTableCells verifies the behavior of format list markdown escapes table cells.
func TestFormatListMarkdown_EscapesTableCells(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "MY|VAR", VariableType: "env_var", EnvironmentScope: "scope|test"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	// Pipe chars in key/scope should be escaped to not break the table
	if strings.Contains(md, "| MY|VAR |") {
		t.Errorf("pipe in key should be escaped:\n%s", md)
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
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 5 tools
// ---------------------------------------------------------------------------.

// requireToolCallSuccess is an internal helper for the civariables package.
func requireToolCallSuccess(t *testing.T, session *mcp.ClientSession, ctx context.Context, toolName string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", toolName, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", toolName, tc.Text)
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true", toolName)
	}
}

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newCIVariablesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_ci_variable_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_ci_variable_get", map[string]any{"project_id": "1", "key": "DB_HOST", "environment_scope": ""}},
		{"create", "gitlab_ci_variable_create", map[string]any{
			"project_id": "1", "key": "NEW_VAR", "value": "new-val",
			"description": "", "variable_type": "", "protected": false,
			"masked": false, "masked_and_hidden": false, "raw": false,
			"environment_scope": "",
		}},
		{"update", "gitlab_ci_variable_update", map[string]any{
			"project_id": "1", "key": "DB_HOST", "value": "new-host",
			"description": "", "variable_type": "", "protected": false,
			"masked": false, "raw": false, "environment_scope": "",
		}},
		{"delete", "gitlab_ci_variable_delete", map[string]any{"project_id": "1", "key": "DB_HOST", "environment_scope": ""}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			requireToolCallSuccess(t, session, ctx, tt.tool, tt.args)
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newCIVariablesMCPSession is an internal helper for the civariables package.
func newCIVariablesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	variableJSON := `{"key":"DB_HOST","value":"localhost","variable_type":"env_var","protected":false,"masked":false,"hidden":false,"raw":false,"environment_scope":"*","description":"Database host"}`

	handler := http.NewServeMux()

	// List variables
	handler.HandleFunc("GET /api/v4/projects/1/variables", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+variableJSON+`]`)
	})

	// Get variable
	handler.HandleFunc("GET /api/v4/projects/1/variables/DB_HOST", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, variableJSON)
	})

	// Create variable
	handler.HandleFunc("POST /api/v4/projects/1/variables", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"key":"NEW_VAR","value":"new-val","variable_type":"env_var","protected":false,"masked":false,"hidden":false,"raw":false,"environment_scope":"*"}`)
	})

	// Update variable
	handler.HandleFunc("PUT /api/v4/projects/1/variables/DB_HOST", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"DB_HOST","value":"new-host","variable_type":"env_var","protected":false,"masked":false,"hidden":false,"raw":false,"environment_scope":"*"}`)
	})

	// Delete variable
	handler.HandleFunc("DELETE /api/v4/projects/1/variables/DB_HOST", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
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
