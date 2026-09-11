// pipeline_triggers_test.go contains unit tests for the pipeline trigger MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package pipelinetriggers

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpMissingProjectID identifies the err exp missing project ID constant used by this package.
const errExpMissingProjectID = "expected error for missing project_id"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestRunTrigger_ReadsArchived verifies the triggered pipeline carries, beside
// what client-go decoded, the archived flag lib/api/entities/ci/pipeline.rb
// sends and gl.Pipeline does not.
func TestRunTrigger_ReadsArchived(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"sha":"abc","ref":"main","status":"created","archived":true}`)
	}))

	out, err := RunTrigger(context.Background(), client, RunInput{ProjectID: "1", Ref: "main", Token: "tok123"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Archived {
		t.Errorf("RunTrigger() = %+v, want archived read off the captured answer", out)
	}
}

// TestRunTrigger_ACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds: GitLab's answer decodes for the SDK
// and not for the field read beside it, and the handler reports it rather
// than swallowing it.
func TestRunTrigger_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"sha":"abc","ref":"main","status":"created","archived":"not-a-bool"}`)
	}))

	_, err := RunTrigger(context.Background(), client, RunInput{ProjectID: "1", Ref: "main", Token: "tok123"})

	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("RunTrigger() error = %v, want the capture's decode failure", err)
	}
}

// ----------------------------------------------
// ListTriggers
// ----------------------------------------------.

// TestListTriggers_Success verifies ListTriggers when success.
func TestListTriggers_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/triggers", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(
			w, http.StatusOK,
			`[{"id":10,"description":"deploy","token":"abc123","owner":{"id":1,"name":"Admin"}}]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
		)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListTriggers(context.Background(), client, ListInput{
		ProjectID: "1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Triggers) != 1 {
		t.Fatalf("triggers = %d, want 1", len(out.Triggers))
	}
	if out.Triggers[0].ID != 10 {
		t.Errorf("id = %d, want 10", out.Triggers[0].ID)
	}
	if out.Triggers[0].Description != "deploy" {
		t.Errorf("description = %q, want %q", out.Triggers[0].Description, "deploy")
	}
	if out.Triggers[0].Token != "abc123" {
		t.Errorf("token = %q, want %q", out.Triggers[0].Token, "abc123")
	}
	if out.Triggers[0].Owner == nil {
		t.Fatal("expected owner object to be populated")
	}
	if out.Triggers[0].Owner.Name != "Admin" {
		t.Errorf("owner.name = %q, want %q", out.Triggers[0].Owner.Name, "Admin")
	}
	if out.Triggers[0].Owner.ID != 1 {
		t.Errorf("owner.id = %d, want 1", out.Triggers[0].Owner.ID)
	}
}

// TestListTriggers_MissingProjectID verifies ListTriggers when missing project ID.
func TestListTriggers_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListTriggers(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// ----------------------------------------------
// GetTrigger
// ----------------------------------------------.

// TestGetTrigger_Success verifies GetTrigger when success.
func TestGetTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"description":"deploy","token":"abc123"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "1", TriggerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 10 {
		t.Errorf("id = %d, want 10", out.ID)
	}
	if out.Description != "deploy" {
		t.Errorf("description = %q, want %q", out.Description, "deploy")
	}
}

// TestGetTrigger_MissingProjectID verifies GetTrigger when missing project ID.
func TestGetTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetTrigger(context.Background(), client, GetInput{TriggerID: 10})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// TestGetTrigger_MissingTriggerID verifies GetTrigger when missing trigger ID.
func TestGetTrigger_MissingTriggerID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing trigger_id")
	}
}

// ----------------------------------------------
// CreateTrigger
// ----------------------------------------------.

// TestCreateTrigger_Success verifies CreateTrigger when success.
func TestCreateTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/triggers", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":11,"description":"test trigger","token":"xyz789"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateTrigger(context.Background(), client, CreateInput{
		ProjectID:   "1",
		Description: "test trigger",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 11 {
		t.Errorf("id = %d, want 11", out.ID)
	}
	if out.Description != "test trigger" {
		t.Errorf("description = %q, want %q", out.Description, "test trigger")
	}
}

// TestCreateTrigger_MintedToken_IsRenderedInFull verifies that the one card
// that answers the call which mints a trigger token prints that token whole.
//
// It matters because the value is unobtainable anywhere else in a usable form
// once the get and the list cards stop printing it, so masking every card
// would have left the create action useless.
func TestCreateTrigger_MintedToken_IsRenderedInFull(t *testing.T) {
	const token = "glptt-mintedsecret0123456789"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/triggers", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":11,"description":"test trigger","token":"`+token+`"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateTrigger(context.Background(), client, CreateInput{ProjectID: "1", Description: "test trigger"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	want := "## Pipeline Trigger #11\n\n" +
		"- **ID**: 11\n" +
		"- **Description**: test trigger\n" +
		"- **Token**: `" + token + "`\n" +
		ptHintsOpening + ptStoreHint + ptUpdateHint + ptRunHint + ptDeleteHint

	if got := FormatTriggerMarkdown(out); got != want {
		t.Errorf("create card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestGetTrigger_ExistingToken_IsRenderedByPrefixOnly verifies that the card a
// get answers with prints the token's prefix and never the whole secret.
//
// It matters because GitLab returns the token from the get endpoint too, so
// printing it was this server's own choice, and the value reached the model as
// a usable credential on every read of a trigger somebody else created.
func TestGetTrigger_ExistingToken_IsRenderedByPrefixOnly(t *testing.T) {
	const token = "glptt-storedsecret0123456789"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"description":"deploy","token":"`+token+`"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "1", TriggerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != token {
		t.Errorf("structured output = %q, want the token GitLab sent kept for 1:1 parity", out.Token)
	}

	want := "## Pipeline Trigger #10\n\n" +
		"- **ID**: 10\n" +
		"- **Description**: deploy\n" +
		"- **Token**: `glptt-stor...`\n" +
		ptMaskedHints

	md := FormatTriggerMarkdown(out)
	if md != want {
		t.Errorf("get card mismatch:\ngot:\n%s\nwant:\n%s", md, want)
	}
	if strings.Contains(md, token) {
		t.Errorf("get card rendered the whole token:\n%s", md)
	}
}

// TestFormatListTriggersMarkdown_Tokens_AreRenderedByPrefixOnly verifies that a
// list of triggers writes no usable credential into its table.
//
// It matters because a list answers with every trigger a project has, so the
// old row printed one live token per line into text a model keeps in context.
func TestFormatListTriggersMarkdown_Tokens_AreRenderedByPrefixOnly(t *testing.T) {
	const tokenA = "glptt-aaaasecret0123456789"
	const tokenB = "glptt-bbbbsecret0123456789"
	got := FormatListTriggersMarkdown(ListOutput{
		Triggers: []Output{
			{ID: 1, Description: "A", Token: tokenA},
			{ID: 2, Description: "B", Token: tokenB},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	})

	want := "## Pipeline Triggers (2)\n\n" +
		ptListHeader +
		"| 1 | A | `glptt-aaaa...` |  |  | never |\n" +
		"| 2 | B | `glptt-bbbb...` |  |  | never |\n\n" +
		"Page 1 of 1 | 2 items total | 20 per page\n" +
		ptListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	for _, token := range []string{tokenA, tokenB} {
		t.Run(token, func(t *testing.T) {
			if strings.Contains(got, token) {
				t.Errorf("list rendered the whole token %q:\n%s", token, got)
			}
		})
	}
}

// TestMaskTriggerToken_TokenShapes_RevealNoMoreThanThePrefix verifies the
// masking rule itself: GitLab's marker plus four characters, an empty token
// rendered as nothing, and a token too short to spare four characters
// withheld whole rather than revealed by the arithmetic.
func TestMaskTriggerToken_TokenShapes_RevealNoMoreThanThePrefix(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{name: "prefixed token", token: "glptt-abcdef123456", want: "glptt-abcd..."},
		{name: "unprefixed token", token: "abcdef123456", want: "abcd..."},
		{name: "exactly the revealed length", token: "glptt-abcd", want: toolutil.RedactedPlaceholder},
		{name: "shorter than the revealed length", token: "abc", want: toolutil.RedactedPlaceholder},
		{name: "empty", token: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskTriggerToken(tc.token); got != tc.want {
				t.Errorf("maskTriggerToken(%q) = %q, want %q", tc.token, got, tc.want)
			}
		})
	}
}

// TestCreateTrigger_MissingDescription verifies CreateTrigger when missing description.
func TestCreateTrigger_MissingDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateTrigger(context.Background(), client, CreateInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

// ----------------------------------------------
// UpdateTrigger
// ----------------------------------------------.

// TestUpdateTrigger_Success verifies UpdateTrigger when success.
func TestUpdateTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"description":"updated","token":"abc123"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateTrigger(context.Background(), client, UpdateInput{
		ProjectID:   "1",
		TriggerID:   10,
		Description: "updated",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Description != "updated" {
		t.Errorf("description = %q, want %q", out.Description, "updated")
	}
}

// TestUpdateTrigger_MissingTriggerID verifies UpdateTrigger when missing trigger ID.
func TestUpdateTrigger_MissingTriggerID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateTrigger(context.Background(), client, UpdateInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing trigger_id")
	}
}

// ----------------------------------------------
// DeleteTrigger
// ----------------------------------------------.

// TestDeleteTrigger_Success verifies DeleteTrigger when success.
func TestDeleteTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/1/triggers/10", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteTrigger(context.Background(), client, DeleteInput{ProjectID: "1", TriggerID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteTrigger_MissingProjectID verifies DeleteTrigger when missing project ID.
func TestDeleteTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteTrigger(context.Background(), client, DeleteInput{TriggerID: 10})
	if err == nil {
		t.Fatal(errExpMissingProjectID)
	}
}

// ----------------------------------------------
// RunTrigger
// ----------------------------------------------.

// TestRunTrigger_Success verifies RunTrigger when success.
func TestRunTrigger_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":99,"sha":"abc","ref":"main","status":"created","web_url":"https://gl/p/1/-/pipelines/99"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1",
		Ref:       "main",
		Token:     "tok123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 99 {
		t.Errorf("id = %d, want 99", out.ID)
	}
	if out.Status != "created" {
		t.Errorf("status = %q, want %q", out.Status, "created")
	}
}

// TestRunTrigger_WithVariables verifies RunTrigger forwards CI/CD variables as
// a map[string]string to the SDK.
func TestRunTrigger_WithVariables(t *testing.T) {
	var gotBody string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		testutil.RespondJSON(w, http.StatusCreated, `{"id":100,"sha":"def","ref":"main","status":"created","web_url":"https://gl/p/1/-/pipelines/100"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1",
		Ref:       "main",
		Token:     "tok123",
		Variables: map[string]string{"ENV": "prod"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 100 {
		t.Errorf("id = %d, want 100", out.ID)
	}
	if !strings.Contains(gotBody, "ENV") || !strings.Contains(gotBody, "prod") {
		t.Errorf("request body = %q, want to carry variables[ENV]=prod", gotBody)
	}
}

// TestRunTrigger_FullPipeline verifies convertPipeline surfaces the nested user
// and detailed_status sub-objects plus the additive Pipeline fields.
func TestRunTrigger_FullPipeline(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":99,"iid":5,"project_id":1,"status":"running","source":"trigger",
			"ref":"main","name":"deploy","sha":"abc","before_sha":"def","tag":true,
			"yaml_errors":"","duration":42,"queued_duration":3,"coverage":"88.5",
			"web_url":"https://gl/p/1/-/pipelines/99",
			"user":{"id":7,"username":"bot","name":"Bot","state":"active"},
			"detailed_status":{"icon":"status_running","text":"running","label":"running","group":"running","tooltip":"running","has_details":true,"details_path":"/p","favicon":"/f","illustration":{"image":"/img"}},
			"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z",
			"started_at":"2026-01-01T01:00:00Z","finished_at":"2026-01-01T02:00:00Z",
			"committed_at":"2026-01-01T00:30:00Z"
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := RunTrigger(context.Background(), client, RunInput{ProjectID: "1", Ref: "main", Token: "tok"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.IID != 5 || out.ProjectID != 1 || out.Source != "trigger" || out.Name != "deploy" {
		t.Errorf("scalar fields not mapped: %+v", out)
	}
	if out.BeforeSHA != "def" || !out.Tag || out.Duration != 42 || out.QueuedDuration != 3 || out.Coverage != "88.5" {
		t.Errorf("additive fields not mapped: %+v", out)
	}
	if out.User == nil || out.User.Username != "bot" {
		t.Fatalf("user object not mapped: %+v", out.User)
	}
	assertDetailedStatusRunning(t, out.DetailedStatus)
	if out.StartedAt == "" || out.FinishedAt == "" || out.CommittedAt == "" || out.UpdatedAt == "" {
		t.Errorf("timestamps not mapped: %+v", out)
	}
}

// assertDetailedStatusRunning verifies the triggered-pipeline detailed_status
// object and its nested illustration sub-object were mapped from the API
// response. Extracted from TestRunTrigger_FullPipeline to keep that test below
// the gocyclo complexity threshold.
func assertDetailedStatusRunning(t *testing.T, ds *DetailedStatusOutput) {
	t.Helper()
	if ds == nil || ds.Label != "running" {
		t.Fatalf("detailed_status not mapped: %+v", ds)
	}
	if ds.Illustration == nil || ds.Illustration.Image != "/img" {
		t.Fatalf("detailed_status illustration not mapped: %+v", ds.Illustration)
	}
}

// TestRunTrigger_NullIllustration verifies version tolerance for the documented
// "illustration": null detailed_status payload. Per doc/api/pipeline_triggers.md
// the triggered-pipeline response renders illustration as null by default, and
// older GitLab versions omit the nested image entirely; the converter must leave
// DetailedStatus.Illustration nil rather than emit an empty object.
func TestRunTrigger_NullIllustration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":99,"status":"created","ref":"main","sha":"abc",
			"web_url":"https://gl/p/1/-/pipelines/99",
			"detailed_status":{"icon":"status_created","text":"created","label":"created","group":"created","tooltip":"created","has_details":true,"details_path":"/p","illustration":null,"favicon":"/f"}
		}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := RunTrigger(context.Background(), client, RunInput{ProjectID: "1", Ref: "main", Token: "tok"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.DetailedStatus == nil {
		t.Fatalf("detailed_status not mapped: %+v", out)
	}
	if out.DetailedStatus.Illustration != nil {
		t.Errorf("expected nil illustration for null payload, got %+v", out.DetailedStatus.Illustration)
	}
}

// TestListTriggers_OrderBySort verifies ListTriggers forwards order_by and sort
// to the API query.
func TestListTriggers_OrderBySort(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"description":"t","token":"a"}]`)
	}))
	_, err := ListTriggers(context.Background(), client, ListInput{
		ProjectID:  "1",
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "5",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=id", "sort=desc", "pagination=keyset", "page_token=5"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query = %q, want to contain %q", gotQuery, want)
			}
		})
	}
}

// TestDecoratePipelineTriggerMeta_UnknownTool verifies the decorator is a no-op
// for a tool with no metadata entry, leaving the generic options unchanged.
func TestDecoratePipelineTriggerMeta_UnknownTool(t *testing.T) {
	options := pipelineTriggerOptions("gitlab_unknown_tool")
	before := options.Usage
	toolutil.ApplyActionMeta(&options, pipelineTriggerActionMeta["gitlab_unknown_tool"])
	if options.Usage != before {
		t.Errorf("usage changed for unknown tool: %q", options.Usage)
	}
}

// TestRunTrigger_WithInputs verifies RunTrigger forwards typed pipeline inputs
// of every supported value kind without error.
func TestRunTrigger_WithInputs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/trigger/pipeline", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"id":101,"sha":"def","ref":"main","status":"created"}`)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1",
		Ref:       "main",
		Token:     "tok123",
		Inputs: map[string]any{
			"environment": "production",
			"replicas":    float64(3),
			"debug":       false,
			"regions":     []any{"us-east", "eu-west"},
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 101 {
		t.Errorf("id = %d, want 101", out.ID)
	}
}

// TestRunTrigger_InvalidInputArray verifies RunTrigger rejects an input array
// containing non-string elements.
func TestRunTrigger_InvalidInputArray(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1",
		Ref:       "main",
		Token:     "tok",
		Inputs:    map[string]any{"regions": []any{"us-east", 42}},
	})
	if err == nil {
		t.Fatal("expected error for non-string array element in inputs")
	}
}

// TestRunTrigger_InvalidInputType verifies RunTrigger rejects an input value of
// an unsupported type.
func TestRunTrigger_InvalidInputType(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1",
		Ref:       "main",
		Token:     "tok",
		Inputs:    map[string]any{"weird": map[string]any{"nested": true}},
	})
	if err == nil {
		t.Fatal("expected error for unsupported input value type")
	}
}

// TestBuildPipelineInputs_IntegerKinds verifies buildPipelineInputs accepts the
// int and int64 value kinds (which JSON decoding does not normally produce but
// programmatic callers may supply).
func TestBuildPipelineInputs_IntegerKinds(t *testing.T) {
	inputs, err := buildPipelineInputs(map[string]any{
		"a": int(1),
		"b": int64(2),
		"c": []string{"x", "y"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inputs) != 3 {
		t.Fatalf("len(inputs) = %d, want 3", len(inputs))
	}
}

// TestRunTrigger_MissingRef verifies RunTrigger when missing ref.
func TestRunTrigger_MissingRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RunTrigger(context.Background(), client, RunInput{ProjectID: "1", Token: "tok"})
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
}

// TestRunTrigger_MissingToken verifies RunTrigger when missing token.
func TestRunTrigger_MissingToken(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := RunTrigger(context.Background(), client, RunInput{ProjectID: "1", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

// ----------------------------------------------
// Markdown formatters
// ----------------------------------------------.

// The pieces the pipeline-trigger formatters close with. A card that showed a
// masked token points the reader at the structured output instead of at a run
// action it cannot supply a token for; the list table carries no link, so it
// does not ask the model to preserve any.
const (
	ptHintsOpening = "\n---\n\U0001F4A1 **Next steps:**\n"
	ptStoreHint    = "- Store the token securely. It cannot be retrieved later\n"
	ptUpdateHint   = "- Use the selected tool surface's pipeline-trigger update action with the same project_id and trigger_id to modify this trigger\n"
	ptRunHint      = "- Use the selected tool surface's pipeline-trigger run action with the same project_id, ref, and this token to execute a pipeline\n"
	ptPrefixHint   = "- Only the prefix of the token is shown here. Read the full value from this result's structured output, or use the pipeline-trigger create action to mint a new trigger, before calling the pipeline-trigger run action\n"
	ptDeleteHint   = "- Use the selected tool surface's pipeline-trigger delete action with the same project_id, trigger_id, and explicit confirm=true to remove this trigger\n"

	ptMaskedHints  = ptHintsOpening + ptUpdateHint + ptPrefixHint + ptDeleteHint
	ptNoTokenHints = ptHintsOpening + ptUpdateHint + ptRunHint + ptDeleteHint

	ptListHeader = "| ID | Description | Token | Owner | Last Used | Expires |\n" +
		"| --- | --- | --- | --- | --- | --- |\n"

	ptListHints = ptHintsOpening +
		"- The Token column shows each token's prefix only. Read the full value from this result's structured output when a pipeline-trigger run action needs one\n" +
		"- Use the selected tool surface's pipeline-trigger get action with the same project_id and trigger_id for full details\n" +
		"- Use the selected tool surface's pipeline-trigger create action with project_id to add a new pipeline trigger\n"

	ptRunCardHints = ptHintsOpening +
		"- Use the selected tool surface's pipeline get action with the returned id to monitor progress\n"
)

// TestFormatTriggerMarkdown pins the whole card a get answers with: the token
// masked to its prefix inside a code span, and the run hint replaced by the
// one that says where the full value is.
func TestFormatTriggerMarkdown(t *testing.T) {
	got := FormatTriggerMarkdown(Output{ID: 10, Description: "deploy", Token: "glptt-abc123def", Owner: &UserOutput{ID: 1, Name: "Admin"}, CreatedAt: "2026-01-01T00:00:00Z"})

	want := "## Pipeline Trigger #10\n\n" +
		"- **ID**: 10\n" +
		"- **Description**: deploy\n" +
		"- **Token**: `glptt-abc1...`\n" +
		"- **Owner**: Admin\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		ptMaskedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListTriggersMarkdown_Empty pins the whole response of a list with
// no triggers.
func TestFormatListTriggersMarkdown_Empty(t *testing.T) {
	if got, want := FormatListTriggersMarkdown(ListOutput{}), "No pipeline triggers found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListTriggersMarkdown_WithData pins the whole list document for one
// trigger, the Expires column included.
func TestFormatListTriggersMarkdown_WithData(t *testing.T) {
	got := FormatListTriggersMarkdown(ListOutput{
		Triggers: []Output{{ID: 1, Description: "test", Token: "glptt-tokvalue1234", ExpiresAt: "2027-03-04T00:00:00Z"}},
		Pagination: toolutil.PaginationOutput{
			Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1,
		},
	})

	want := "## Pipeline Triggers (1)\n\n" +
		ptListHeader +
		"| 1 | test | `glptt-tokv...` |  |  | 4 Mar 2027 00:00 UTC |\n\n" +
		"Page 1 of 1 | 1 items total | 20 per page\n" +
		ptListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatRunOutputMarkdown pins the whole card of a pipeline a trigger
// started, the status with the glyph its state earns.
func TestFormatRunOutputMarkdown(t *testing.T) {
	got := FormatRunOutputMarkdown(RunOutput{ID: 99, SHA: "abc", Ref: "main", Status: "created", WebURL: "https://gl/p/1"})

	want := "## Pipeline Triggered\n\n" +
		"- **Pipeline ID**: 99\n" +
		"- **SHA**: `abc`\n" +
		"- **Ref**: main\n" +
		"- **Status**: " + toolutil.PipelineStatusEmoji("created") + " created\n" +
		"- **URL**: [https://gl/p/1](https://gl/p/1)\n" +
		ptRunCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// ListTriggers — API error, canceled context
// ---------------------------------------------------------------------------.

// TestListTriggers_APIError verifies ListTriggers when API error.
func TestListTriggers_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListTriggers(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListTriggers_CancelledContext verifies ListTriggers when cancelled context.
func TestListTriggers_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListTriggers(ctx, client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListTriggers_WithPagination verifies ListTriggers when with pagination.
func TestListTriggers_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/1/triggers" {
			testutil.RespondJSONWithPagination(
				w, http.StatusOK,
				`[{"id":1,"description":"t1","token":"a"},{"id":2,"description":"t2","token":"b"}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListTriggers(context.Background(), client, ListInput{
		ProjectID: "1",
		Page:      2, PerPage: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Triggers) != 2 {
		t.Fatalf("len(Triggers) = %d, want 2", len(out.Triggers))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// GetTrigger — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGetTrigger_APIError verifies GetTrigger when API error.
func TestGetTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetTrigger_CancelledContext verifies GetTrigger when cancelled context.
func TestGetTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetTrigger(ctx, client, GetInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// CreateTrigger — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestCreateTrigger_APIError verifies CreateTrigger when API error.
func TestCreateTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := CreateTrigger(context.Background(), client, CreateInput{
		ProjectID: "1", Description: "test",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateTrigger_MissingProjectID verifies CreateTrigger when missing project ID.
func TestCreateTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := CreateTrigger(context.Background(), client, CreateInput{Description: "test"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestCreateTrigger_CancelledContext verifies CreateTrigger when cancelled context.
func TestCreateTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := CreateTrigger(ctx, client, CreateInput{ProjectID: "1", Description: "test"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// UpdateTrigger — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestUpdateTrigger_APIError verifies UpdateTrigger when API error.
func TestUpdateTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := UpdateTrigger(context.Background(), client, UpdateInput{
		ProjectID: "1", TriggerID: 10, Description: "updated",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateTrigger_MissingProjectID verifies UpdateTrigger when missing project ID.
func TestUpdateTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := UpdateTrigger(context.Background(), client, UpdateInput{TriggerID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdateTrigger_CancelledContext verifies UpdateTrigger when cancelled context.
func TestUpdateTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateTrigger(ctx, client, UpdateInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestUpdateTrigger_WithoutDescription verifies UpdateTrigger when without description.
func TestUpdateTrigger_WithoutDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/1/triggers/10" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"description":"original","token":"abc123"}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := UpdateTrigger(context.Background(), client, UpdateInput{
		ProjectID: "1", TriggerID: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Description != "original" {
		t.Errorf("description = %q, want %q", out.Description, "original")
	}
}

// ---------------------------------------------------------------------------
// DeleteTrigger — API error, missing trigger_id, canceled context
// ---------------------------------------------------------------------------.

// TestDeleteTrigger_APIError verifies DeleteTrigger when API error.
func TestDeleteTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteTrigger(context.Background(), client, DeleteInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteTrigger_MissingTriggerID verifies DeleteTrigger when missing trigger ID.
func TestDeleteTrigger_MissingTriggerID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := DeleteTrigger(context.Background(), client, DeleteInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing trigger_id")
	}
}

// TestDeleteTrigger_CancelledContext verifies DeleteTrigger when cancelled context.
func TestDeleteTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := DeleteTrigger(ctx, client, DeleteInput{ProjectID: "1", TriggerID: 10})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// RunTrigger — API error, missing project_id, canceled context
// ---------------------------------------------------------------------------.

// TestRunTrigger_APIError verifies RunTrigger when API error.
func TestRunTrigger_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1", Ref: "main", Token: "tok123",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRunTrigger_BadRequest verifies RunTrigger returns ref and CI lint guidance for 400 responses.
func TestRunTrigger_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := RunTrigger(context.Background(), client, RunInput{
		ProjectID: "1", Ref: "missing", Token: "tok123",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "gitlab_ci_lint") {
		t.Fatalf("error = %v, want CI lint hint", err)
	}
}

// TestRunTrigger_MissingProjectID verifies RunTrigger when missing project ID.
func TestRunTrigger_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := RunTrigger(context.Background(), client, RunInput{Ref: "main", Token: "tok"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestRunTrigger_CancelledContext verifies RunTrigger when cancelled context.
func TestRunTrigger_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := RunTrigger(ctx, client, RunInput{
		ProjectID: "1", Ref: "main", Token: "tok",
	})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatTriggerMarkdown — all optional fields, minimal fields
// ---------------------------------------------------------------------------.

// TestFormatTriggerMarkdown_AllFields pins the whole card of a trigger with
// every field GitLab sends set.
func TestFormatTriggerMarkdown_AllFields(t *testing.T) {
	got := FormatTriggerMarkdown(Output{
		ID:          10,
		Description: "deploy trigger",
		Token:       "glptt-abc123def456",
		Owner:       &UserOutput{ID: 1, Name: "Admin"},
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-06-01T00:00:00Z",
		LastUsed:    "2026-12-01T00:00:00Z",
	})

	want := "## Pipeline Trigger #10\n\n" +
		"- **ID**: 10\n" +
		"- **Description**: deploy trigger\n" +
		"- **Token**: `glptt-abc1...`\n" +
		"- **Owner**: Admin\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **Last Used**: 1 Dec 2026 00:00 UTC\n" +
		ptMaskedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatTriggerMarkdown_MinimalFields pins the card of a trigger GitLab
// sent no owner and no timestamps for, and the token too short to spare four
// characters withheld whole.
func TestFormatTriggerMarkdown_MinimalFields(t *testing.T) {
	got := FormatTriggerMarkdown(Output{
		ID:          5,
		Description: "minimal",
		Token:       "tok",
	})

	want := "## Pipeline Trigger #5\n\n" +
		"- **ID**: 5\n" +
		"- **Description**: minimal\n" +
		"- **Token**: `" + toolutil.RedactedPlaceholder + "`\n" +
		ptMaskedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatTriggerMarkdown_NoToken pins the card of a trigger GitLab answered
// with no token at all: no token row, no store-it hint, and the ordinary run
// hint.
func TestFormatTriggerMarkdown_NoToken(t *testing.T) {
	got := FormatTriggerMarkdown(Output{ID: 7, Description: "tokenless"})

	want := "## Pipeline Trigger #7\n\n" +
		"- **ID**: 7\n" +
		"- **Description**: tokenless\n" +
		ptNoTokenHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListTriggersMarkdown — detailed checks
// ---------------------------------------------------------------------------.

// TestFormatListTriggersMarkdown_DetailedContent pins the whole list document
// for two triggers, one of which GitLab never reported a last use for.
func TestFormatListTriggersMarkdown_DetailedContent(t *testing.T) {
	out := ListOutput{
		Triggers: []Output{
			{ID: 1, Description: "Trigger A", Token: "glptt-tokAsecretvalue", Owner: &UserOutput{Name: "admin"}, LastUsed: "2026-01-01T00:00:00Z"},
			{ID: 2, Description: "Trigger B", Token: "glptt-tokBsecretvalue", Owner: &UserOutput{Name: "user1"}, LastUsed: ""},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}

	want := "## Pipeline Triggers (2)\n\n" +
		ptListHeader +
		"| 1 | Trigger A | `glptt-tokA...` | admin | 1 Jan 2026 00:00 UTC | never |\n" +
		"| 2 | Trigger B | `glptt-tokB...` | user1 |  | never |\n\n" +
		"Page 1 of 1 | 2 items total | 20 per page\n" +
		ptListHints

	if got := FormatListTriggersMarkdown(out); got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatRunOutputMarkdown — without web URL, empty values
// ---------------------------------------------------------------------------.

// TestFormatRunOutputMarkdown_WithoutWebURL pins the card of a pipeline
// GitLab gave no address for: no URL row at all, rather than a label with
// nothing after it.
func TestFormatRunOutputMarkdown_WithoutWebURL(t *testing.T) {
	got := FormatRunOutputMarkdown(RunOutput{
		ID:     50,
		SHA:    "deadbeef",
		Ref:    "develop",
		Status: "pending",
	})

	want := "## Pipeline Triggered\n\n" +
		"- **Pipeline ID**: 50\n" +
		"- **SHA**: `deadbeef`\n" +
		"- **Ref**: develop\n" +
		"- **Status**: " + toolutil.PipelineStatusEmoji("pending") + " pending\n" +
		ptRunCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatRunOutputMarkdown_AllFields pins the whole card of a triggered
// pipeline with an address.
func TestFormatRunOutputMarkdown_AllFields(t *testing.T) {
	const url = "https://gl/p/1/-/pipelines/99"
	got := FormatRunOutputMarkdown(RunOutput{
		ID:        99,
		SHA:       "abc",
		Ref:       "main",
		Status:    "created",
		WebURL:    url,
		CreatedAt: "2026-06-01T00:00:00Z",
	})

	want := "## Pipeline Triggered\n\n" +
		"- **Pipeline ID**: 99\n" +
		"- **SHA**: `abc`\n" +
		"- **Ref**: main\n" +
		"- **Status**: " + toolutil.PipelineStatusEmoji("created") + " created\n" +
		"- **URL**: [" + url + "](" + url + ")\n" +
		ptRunCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatTriggerMarkdown_ExpiresAt pins that the expiry reaches the card.
// It is read off the captured response because the SDK does not model it, and
// a trigger shown without it reads as one that never expires.
func TestFormatTriggerMarkdown_ExpiresAt(t *testing.T) {
	withExpiry := "## Pipeline Trigger #1\n\n" +
		"- **ID**: 1\n" +
		"- **Description**: nightly\n" +
		"- **Expires At**: 5 May 2026 00:00 UTC\n" +
		ptNoTokenHints
	if got := FormatTriggerMarkdown(Output{ID: 1, Description: "nightly", ExpiresAt: "2026-05-05T00:00:00Z"}); got != withExpiry {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, withExpiry)
	}

	without := "## Pipeline Trigger #1\n\n" +
		"- **ID**: 1\n" +
		"- **Description**: nightly\n" +
		ptNoTokenHints
	if got := FormatTriggerMarkdown(Output{ID: 1, Description: "nightly"}); got != without {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, without)
	}
}

// TestPipelineTriggers_UnreadableCapturedExpiresAt verifies that every trigger
// handler reading expires_at off the captured answer returns an error rather
// than a half-filled trigger when GitLab sends it as something that is not a
// timestamp. The SDK ignores the key its own PipelineTrigger does not model, so
// the captured read is the only thing that can notice, and a trigger published
// without its expiry reads as one that never expires.
func TestPipelineTriggers_UnreadableCapturedExpiresAt(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"id":1,"description":"nightly","expires_at":"never"}]`)
			_, err := ListTriggers(context.Background(), client, ListInput{ProjectID: "42"})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"id":1,"description":"nightly","expires_at":"never"}`)
			_, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "42", TriggerID: 1})
			return err
		}},
		{Name: "create", Call: func() error {
			client := poisoned(`{"id":1,"description":"nightly","expires_at":"never"}`)
			_, err := CreateTrigger(context.Background(), client, CreateInput{ProjectID: "42", Description: "nightly"})
			return err
		}},
		{Name: "update", Call: func() error {
			client := poisoned(`{"id":1,"description":"nightly","expires_at":"never"}`)
			_, err := UpdateTrigger(context.Background(), client, UpdateInput{ProjectID: "42", TriggerID: 1, Description: "renamed"})
			return err
		}},
	})
}

// TestGet_WithAllTimestamps verifies convertTrigger covers UpdatedAt and LastUsed nil guards.
func TestGet_WithAllTimestamps(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":10,"description":"deploy","token":"abc",
			"owner":{"id":1,"name":"Admin"},
			"created_at":"2026-01-15T10:00:00Z",
			"updated_at":"2026-02-01T12:00:00Z",
			"last_used":"2026-03-01T08:30:00Z",
			"expires_at":"2026-12-31T23:59:59Z"
		}`)
	}))
	out, err := GetTrigger(context.Background(), client, GetInput{ProjectID: "42", TriggerID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.UpdatedAt == "" {
		t.Error("expected UpdatedAt to be set")
	}
	if out.LastUsed == "" {
		t.Error("expected LastUsed to be set")
	}
	if out.ExpiresAt == "" {
		t.Error("expected ExpiresAt to be set from the captured answer")
	}
}
