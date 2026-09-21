// merge_trains_test.go contains unit tests for GitLab merge train operations.
// Tests use httptest to mock the GitLab Merge Trains API.
package mergetrains

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestListProjectMergeTrains validates the ListProjectMergeTrains handler.
// Covers a populated car asserted field for field, empty project_id validation,
// scope/sort query params, a refusal from GitLab, and empty results.
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
				t.Helper()
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/merge_trains")
				testutil.RespondJSONWithPagination(w, http.StatusOK, "["+populatedTrainJSON+"]",
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Trains) != 1 {
					t.Fatalf("got %d trains, want 1", len(out.Trains))
				}
				assertPopulatedMergeTrain(t, out.Trains[0])
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
				t.Helper()
				testutil.AssertQueryParam(t, r, "scope", "active")
				testutil.AssertQueryParam(t, r, "sort", "asc")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Trains) != 0 {
					t.Errorf("got %d trains, want 0", len(out.Trains))
				}
			},
		},
		{
			name:  "returns error on API 403",
			input: ListProjectInput{ProjectID: "42"},
			handler: func(_ *testing.T, w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
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
				t.Helper()
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
					t.Error("handler should not be called")
					http.Error(w, "handler should not be called", http.StatusInternalServerError)
					return
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

// populatedTrainJSON is one merge train car as GitLab sends it, with every
// value distinct from every other: the two identifiers of the car differ from
// the three of its merge request, its status differs from the merge request's
// state, and all five timestamps name a different day.
//
// The distinctness is the point. The fixture it replaced gave the car and its
// user the id 1 and gave the car's status and the merge request's state both
// "merged", so a converter reading either from its neighbor produced exactly
// the same output, a defect no mutation of a branch and no condition counter
// can see, because a straight-line assignment has neither.
const populatedTrainJSON = `{"id":1,` +
	`"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug",` +
	`"description":"Corrects the off-by-one","state":"opened",` +
	`"web_url":"https://gitlab.example.com/-/merge_requests/5",` +
	`"created_at":"2026-01-11T10:00:00Z","updated_at":"2026-01-12T10:00:00Z"},` +
	`"user":{"id":7,"username":"admin"},"pipeline":{"id":200},` +
	`"target_branch":"main","status":"merged","duration":120,` +
	`"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-16T10:00:00Z",` +
	`"merged_at":"2026-01-17T10:00:00Z"}`

// wantPopulatedMergeTrain is the whole [Output] populatedTrainJSON converts to.
func wantPopulatedMergeTrain() Output {
	return Output{
		ID: 1,
		MergeRequest: MergeRequestOutput{
			ID:          100,
			IID:         5,
			ProjectID:   42,
			Title:       "Fix bug",
			Description: "Corrects the off-by-one",
			State:       "opened",
			CreatedAt:   "2026-01-11T10:00:00Z",
			UpdatedAt:   "2026-01-12T10:00:00Z",
			WebURL:      "https://gitlab.example.com/-/merge_requests/5",
		},
		User:         &toolutil.BasicUserOutput{ID: 7, Username: "admin"},
		Pipeline:     &toolutil.PipelineOutput{ID: 200},
		TargetBranch: "main",
		Status:       "merged",
		Duration:     120,
		CreatedAt:    "2026-01-15T10:00:00Z",
		UpdatedAt:    "2026-01-16T10:00:00Z",
		MergedAt:     "2026-01-17T10:00:00Z",
	}
}

// assertPopulatedMergeTrain holds the converted car against the whole expected
// object rather than spot-checking a few of its fields, so a field read from
// the wrong source changes the comparison instead of passing through it. The
// spot-checking form this replaced never read the merge request's id, project
// id, title, description or state at all, and asked of the five timestamps only
// that they were not empty.
func assertPopulatedMergeTrain(t *testing.T, tr Output) {
	t.Helper()
	want := wantPopulatedMergeTrain()
	if reflect.DeepEqual(tr, want) {
		return
	}
	// Reported as JSON rather than with %+v, which prints the user and the
	// pipeline as addresses and so says nothing about the fields that differ.
	t.Fatalf("train =\n%s\nwant\n%s", mustJSON(t, tr), mustJSON(t, want))
}

// mustJSON renders a value for a failure message, or fails the test.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return string(encoded)
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
				t.Helper()
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/merge_trains/main")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[
					{"id":1,"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug","state":"merged","web_url":"https://gitlab.example.com/-/merge_requests/5"},"user":{"id":1,"username":"admin"},"pipeline":{"id":200},"target_branch":"main","status":"merged","duration":120,"created_at":"2026-01-15T10:00:00Z"}
				]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
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
				t.Helper()
				testutil.AssertQueryParam(t, r, "scope", "complete")
				testutil.AssertQueryParam(t, r, "sort", "desc")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
					testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			},
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
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
					t.Error("handler should not be called")
					http.Error(w, "handler should not be called", http.StatusInternalServerError)
					return
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
				t.Helper()
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/merge_trains/merge_requests/5")
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug","state":"merged"},"target_branch":"main","status":"merged","duration":60}`)
			},
			validate: func(t *testing.T, out Output) {
				t.Helper()
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
					t.Error("handler should not be called")
					http.Error(w, "handler should not be called", http.StatusInternalServerError)
					return
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
// Covers success, missing project_id, a zero and a negative merge_request_iid,
// and a 422 from GitLab. The optional fields are asserted by
// [TestAddMergeRequestToMergeTrain_Options], which compares the whole body
// rather than one key, so they are deliberately not exercised here.
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
			handler: addMergeTrainSuccessHandler(`[
					{"id":2,"merge_request":{"id":100,"iid":5,"project_id":42,"title":"Fix bug","state":"opened"},"target_branch":"main","status":"idle","duration":0}
				]`),
			validate: func(t *testing.T, out ListOutput) {
				t.Helper()
				if len(out.Trains) != 1 {
					t.Fatalf("got %d trains, want 1", len(out.Trains))
				}
				if out.Trains[0].Status != "idle" {
					t.Errorf("got status %q, want %q", out.Trains[0].Status, "idle")
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
					t.Error("handler should not be called")
					http.Error(w, "handler should not be called", http.StatusInternalServerError)
					return
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

func addMergeTrainSuccessHandler(body string) func(*testing.T, http.ResponseWriter, *http.Request) {
	return func(t *testing.T, w http.ResponseWriter, r *http.Request) {
		t.Helper()
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/projects/42/merge_trains/merge_requests/5")
		respondMergeTrainList(w, body)
	}
}

func respondMergeTrainList(w http.ResponseWriter, body string) {
	testutil.RespondJSONWithPagination(w, http.StatusOK, body, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
}

// ---------------------------------------------------------------------------
// The refusal each route hands a model
// ---------------------------------------------------------------------------.

// mergeTrainRefusal describes the refusal one route was written to give when
// GitLab turns the call down: the operation it reports under, the status its
// hint is keyed on, a second status that must therefore go unhinted, and the
// whole sentence the hint has to be.
type mergeTrainRefusal struct {
	operation   string
	hintStatus  int
	otherStatus int
	hint        string
	call        func(ctx context.Context, client *gitlabclient.Client) error
}

// mergeTrainRefusals pairs each route with the refusal it was written to give.
//
// The second status of each is another route's keyed status rather than an
// arbitrary one, since crossing the two constants is exactly the mistake four
// near-identical handlers invite: three of them key their hint on a 404 and the
// fourth, the only one that writes, keys it on a 400.
func mergeTrainRefusals() []mergeTrainRefusal {
	return []mergeTrainRefusal{
		{
			operation: "gitlab_list_project_merge_trains", hintStatus: http.StatusNotFound,
			otherStatus: http.StatusBadRequest,
			hint:        "verify project_id with project.get. Merge trains require Premium license",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := ListProjectMergeTrains(ctx, client, ListProjectInput{ProjectID: "42"})
				return err
			},
		},
		{
			operation: "gitlab_list_merge_request_in_merge_train", hintStatus: http.StatusNotFound,
			otherStatus: http.StatusBadRequest,
			hint:        "verify project_id and target_branch. Merge trains require Premium license",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := ListMergeRequestInMergeTrain(ctx, client, ListBranchInput{ProjectID: "42", TargetBranch: "main"})
				return err
			},
		},
		{
			operation: "gitlab_get_merge_request_on_merge_train", hintStatus: http.StatusNotFound,
			otherStatus: http.StatusBadRequest,
			hint:        "verify project_id and merge_request_iid. The MR must be on a merge train",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GetMergeRequestOnMergeTrain(ctx, client, GetInput{ProjectID: "42", MergeRequestID: 5})
				return err
			},
		},
		{
			operation: "gitlab_add_merge_request_to_merge_train", hintStatus: http.StatusBadRequest,
			otherStatus: http.StatusNotFound,
			hint:        "verify the MR is approved and pipeline passed. Merge trains require Premium license",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := AddMergeRequestToMergeTrain(ctx, client, AddInput{ProjectID: "42", MergeRequestID: 5})
				return err
			},
		},
	}
}

// mergeTrainRefusalFrom drives one route against an instance that answers code,
// and returns the error text the caller is handed.
func mergeTrainRefusalFrom(t *testing.T, route mergeTrainRefusal, code int) string {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, code, `{"message":"refused"}`)
	}))
	err := route.call(t.Context(), client)
	if err == nil {
		t.Fatalf("%s: GitLab answered %d and the handler returned no error", route.operation, code)
	}
	return err.Error()
}

// TestHandlers_RefusalReportsItsOwnOperationAndHint checks that each route
// reports a refusal under its own operation and, on the status its hint is
// keyed on, carries that route's own sentence.
//
// Why that matters: every error case in this package stopped at "an error came
// back", so the operation label, the status constant and the hint sentence
// could each be crossed with another route's and nothing would fail. None of
// the three is a branch, so neither gate can see it either. The whole sentence
// is held rather than a token of it, because the two halves of a hint cross
// separately: "verify project_id and target_branch" is true of the branch route
// and of no other, while "Merge trains require Premium license" is true of
// three, so an assertion on either alone leaves the rest of the sentence free
// to be a sibling's. What a model does next is this text, and the add route's
// is the sharpest: its 400 hint is what a model is told when GitLab refuses to
// enqueue the merge request.
func TestHandlers_RefusalReportsItsOwnOperationAndHint(t *testing.T) {
	for _, route := range mergeTrainRefusals() {
		t.Run(route.operation, func(t *testing.T) {
			hinted := mergeTrainRefusalFrom(t, route, route.hintStatus)
			if !strings.HasPrefix(hinted, route.operation+": ") {
				t.Errorf("refusal = %q, want it reported under %q", hinted, route.operation)
			}
			// The trailing colon is what ends the hint in the composed
			// message, so including it holds the sentence to its whole text
			// rather than to a prefix of it.
			wantHint := "Suggestion: " + route.hint + ":"
			if !strings.Contains(hinted, wantHint) {
				t.Errorf("refusal to a %d = %q, want it to carry %q", route.hintStatus, hinted, wantHint)
			}
		})
	}
}

// TestHandlers_RefusalOnAnotherStatusCarriesNoHint checks that each route's
// hint reaches a model only on the status it was keyed on.
//
// This is the other half of the pairing above, and the half that pins the
// status constant: three of these routes advise on a 404, which says the
// project, the branch or the merge request is not where the caller thinks, and
// the fourth advises on a 400, which says the merge request is not ready to be
// enqueued. Offered on the wrong status, each advises a model to re-check
// something GitLab never objected to.
func TestHandlers_RefusalOnAnotherStatusCarriesNoHint(t *testing.T) {
	for _, route := range mergeTrainRefusals() {
		t.Run(route.operation, func(t *testing.T) {
			unhinted := mergeTrainRefusalFrom(t, route, route.otherStatus)
			if !strings.HasPrefix(unhinted, route.operation+": ") {
				t.Errorf("refusal = %q, want it reported under %q", unhinted, route.operation)
			}
			if strings.Contains(unhinted, "Suggestion: ") {
				t.Errorf("refusal to a %d = %q, carrying a hint keyed on %d", route.otherStatus, unhinted, route.hintStatus)
			}
		})
	}
}

// mergeTrainGuard describes one input this package refuses before it calls
// GitLab: the refusal the caller must be handed, and the call that trips it.
type mergeTrainGuard struct {
	name string
	want error
	call func(ctx context.Context, client *gitlabclient.Client) error
}

// mergeTrainGuards pairs each input guard with the refusal it owes a model.
//
// The expectation is built by calling the same toolutil constructor the handler
// calls, so what is held is the arguments the handler passes it and not a copy
// of the sentence toolutil composes: the field name, and for the two int64
// guards the operation beside it. Those are the crossable pair. A rewording
// inside toolutil moves both sides together, which is right, because the
// wording is that package's business and is asserted there.
func mergeTrainGuards() []mergeTrainGuard {
	return []mergeTrainGuard{
		{
			name: "list_project without a project",
			want: toolutil.ErrFieldRequired("project_id"),
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := ListProjectMergeTrains(ctx, client, ListProjectInput{})
				return err
			},
		},
		{
			name: "list_branch without a project",
			want: toolutil.ErrFieldRequired("project_id"),
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := ListMergeRequestInMergeTrain(ctx, client, ListBranchInput{TargetBranch: "main"})
				return err
			},
		},
		{
			name: "list_branch without a target branch",
			want: toolutil.ErrFieldRequired("target_branch"),
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := ListMergeRequestInMergeTrain(ctx, client, ListBranchInput{ProjectID: "42"})
				return err
			},
		},
		{
			name: "get without a project",
			want: toolutil.ErrFieldRequired("project_id"),
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GetMergeRequestOnMergeTrain(ctx, client, GetInput{MergeRequestID: 5})
				return err
			},
		},
		{
			name: "get without a merge request",
			want: toolutil.ErrRequiredInt64("gitlab_get_merge_request_on_merge_train", "merge_request_iid"),
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GetMergeRequestOnMergeTrain(ctx, client, GetInput{ProjectID: "42"})
				return err
			},
		},
		{
			name: "add without a project",
			want: toolutil.ErrFieldRequired("project_id"),
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := AddMergeRequestToMergeTrain(ctx, client, AddInput{MergeRequestID: 5})
				return err
			},
		},
		{
			name: "add without a merge request",
			want: toolutil.ErrRequiredInt64("gitlab_add_merge_request_to_merge_train", "merge_request_iid"),
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := AddMergeRequestToMergeTrain(ctx, client, AddInput{ProjectID: "42", MergeRequestID: -1})
				return err
			},
		},
	}
}

// TestHandlers_RefusedInput_NamesTheFieldItRefusedFor checks that each guard
// names the field the caller left out, under the operation that refused, and
// that it refuses without asking GitLab anything.
//
// Why that matters: the seven guards are the same two calls repeated with a
// different argument, and the tests that drive them assert only that an error
// came back, so any guard could name any sibling's field and stay green. The
// field name is the whole content of the refusal: told "target_branch is
// required" for a missing project, a model supplies a branch it already sent
// and is refused again. The two int64 guards carry an operation as well, which
// is the other half of the same pair.
//
// The mock fails the test if it is reached, which is the claim the wantErr
// cases could not make: against a mock that answers, "an error came back" is
// satisfied just as well by a guard that was never there and a GitLab that
// refused the call.
func TestHandlers_RefusedInput_NamesTheFieldItRefusedFor(t *testing.T) {
	for _, guard := range mergeTrainGuards() {
		t.Run(guard.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
			err := guard.call(t.Context(), client)
			if err == nil {
				t.Fatal("the input reached the end of the handler unrefused")
			}
			if err.Error() != guard.want.Error() {
				t.Errorf("refusal = %q, want %q", err, guard.want)
			}
		})
	}
}

// TestListProjectMergeTrains_KeysetAndOrdering verifies order_by, sort, and
// keyset pagination (pagination/page_token) are forwarded as query parameters.
//
// Each parameter is asserted on its own line rather than in a loop: the
// assertions run inside the mock's handler, on the server's goroutine, where a
// subtest cannot be opened.
func TestListProjectMergeTrains_KeysetAndOrdering(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "sort", "desc")
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "tok")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))
	_, err := ListProjectMergeTrains(context.Background(), client, ListProjectInput{
		ProjectID:  "42",
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListMergeRequestInMergeTrain_KeysetAndOrdering verifies order_by and
// keyset pagination forwarding for the per-branch list handler, asserted one
// parameter per line for the reason its project-wide sibling states.
func TestListMergeRequestInMergeTrain_KeysetAndOrdering(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "order_by", "id")
		testutil.AssertQueryParam(t, r, "pagination", "keyset")
		testutil.AssertQueryParam(t, r, "page_token", "tok")
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
	}))
	_, err := ListMergeRequestInMergeTrain(context.Background(), client, ListBranchInput{
		ProjectID:    "42",
		TargetBranch: "main",
		OrderBy:      "id",
		Pagination:   "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestAddMergeRequestToMergeTrain_Options drives each optional field of the add
// request on its own and compares the whole body GitLab receives, which is the
// only shape that tells the four options apart.
//
// Setting them together and checking each key is present cannot: every one is a
// pointer written under a guard of its own, so two guards filling each other's
// field send exactly the same body when both flags are true. The empty case is
// the other half of the claim: client-go omits every option nobody set, so no
// value is sent that the caller never asked to send.
func TestAddMergeRequestToMergeTrain_Options(t *testing.T) {
	tests := []struct {
		name  string
		input AddInput
		want  map[string]any
	}{
		{
			name:  "no option sends an empty body",
			input: AddInput{ProjectID: "42", MergeRequestID: 5},
			want:  map[string]any{},
		},
		{
			name:  "auto_merge alone",
			input: AddInput{ProjectID: "42", MergeRequestID: 5, AutoMerge: true},
			want:  map[string]any{"auto_merge": true},
		},
		{
			name:  "squash alone",
			input: AddInput{ProjectID: "42", MergeRequestID: 5, Squash: true},
			want:  map[string]any{"squash": true},
		},
		{
			name:  "sha alone",
			input: AddInput{ProjectID: "42", MergeRequestID: 5, SHA: "abc123"},
			want:  map[string]any{"sha": "abc123"},
		},
		{
			// Deprecated in 17.11 and mirrored for 1:1 SDK fidelity: it is sent
			// under its own name and never folded into auto_merge, which is what
			// keeps the two distinguishable to a GitLab that still reads both.
			name:  "when_pipeline_succeeds alone",
			input: AddInput{ProjectID: "42", MergeRequestID: 5, WhenPipelineSucceeds: true},
			want:  map[string]any{"when_pipeline_succeeds": true},
		},
		{
			name: "every option together",
			input: AddInput{
				ProjectID: "42", MergeRequestID: 5,
				AutoMerge: true, SHA: "abc123", Squash: true, WhenPipelineSucceeds: true,
			},
			want: map[string]any{
				"auto_merge": true, "sha": "abc123", "squash": true, "when_pipeline_succeeds": true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody []byte
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
					http.Error(w, "read body", http.StatusInternalServerError)
					return
				}
				gotBody = body
				respondMergeTrainList(w, registerTrainsJSON)
			}))
			if _, err := AddMergeRequestToMergeTrain(context.Background(), client, tt.input); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertJSONBody(t, gotBody, tt.want)
		})
	}
}

// assertJSONBody compares a recorded request body with the whole object it is
// expected to be, so a key sent that no case asked for fails as loudly as a
// missing one.
func assertJSONBody(t *testing.T, body []byte, want map[string]any) {
	t.Helper()
	got := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("parse body %q: %v", body, err)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %v, want %v (raw: %s)", got, want, body)
	}
}

// TestToOutput_FullPipeline verifies a populated pipeline reaches the output as
// a nested object rather than as an id: its scalars, its user, its five
// timestamps, and the detailed status with its illustration image.
//
// It samples those fields rather than asserting the whole pipeline, because the
// conversion is [toolutil.NewPipelineOutput]'s and is held field by field
// there; what this test is about is that [toOutput] reaches for it at all. The
// comment used to say "mirrored in full", which the body never checked.
func TestToOutput_FullPipeline(t *testing.T) {
	created := mustParseTime(t, "2026-01-15T10:00:00Z")
	mt := &gl.MergeTrain{
		ID:           1,
		TargetBranch: "main",
		Status:       "merged",
		User:         &gl.BasicUser{ID: 7, Username: "admin", Name: "Admin", State: "active", AvatarURL: "a", WebURL: "w", CreatedAt: &created},
		Pipeline: &gl.Pipeline{
			ID: 200, IID: 3, ProjectID: 42, Status: "success", Source: "push", Ref: "main",
			Name: "build", SHA: "deadbeef", BeforeSHA: "cafe", Tag: true, YamlErrors: "",
			User: &gl.BasicUser{ID: 7, Username: "admin"}, UpdatedAt: &created, CreatedAt: &created,
			StartedAt: &created, FinishedAt: &created, CommittedAt: &created,
			Duration: 60, QueuedDuration: 5, Coverage: "90", WebURL: "https://gl/pipe/200",
			DetailedStatus: &gl.DetailedStatus{
				Icon: "icon", Text: "passed", Label: "passed", Group: "success",
				Tooltip: "ok", HasDetails: true, DetailsPath: "/p", Favicon: "fav",
				Illustration: gl.DetailedStatusIllustration{Image: "img.png"},
			},
		},
	}
	out := toOutput(mt)
	if out.User == nil || out.User.AvatarURL != "a" || out.User.CreatedAt == "" {
		t.Fatalf("user = %+v, want full BasicUser", out.User)
	}
	if out.Pipeline == nil {
		t.Fatal("pipeline is nil, want populated")
	}
	assertFullPipeline(t, out.Pipeline)
}

// assertFullPipeline verifies the sampled fields of a populated pipeline: six
// scalars, the user, the five timestamps and the detailed status.
func assertFullPipeline(t *testing.T, p *toolutil.PipelineOutput) {
	t.Helper()
	switch {
	case p.ID != 200, p.IID != 3, p.Source != "push", p.SHA != "deadbeef", p.Coverage != "90", p.WebURL != "https://gl/pipe/200":
		t.Errorf("pipeline scalars = %+v", p)
	}
	if p.User == nil || p.User.Username != "admin" {
		t.Errorf("pipeline.user = %+v, want admin", p.User)
	}
	switch {
	case p.StartedAt == "", p.FinishedAt == "", p.CommittedAt == "", p.UpdatedAt == "", p.CreatedAt == "":
		t.Errorf("pipeline timestamps missing = %+v", p)
	}
	ds := p.DetailedStatus
	if ds == nil || ds.Label != "passed" || ds.Illustration == nil || ds.Illustration.Image != "img.png" {
		t.Errorf("pipeline.detailed_status = %+v", ds)
	}
}

// mustParseTime parses an RFC 3339 timestamp or fails the test.
func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return parsed
}

// TestPipelineHelpers_NilBranches verifies the shape converters return nil for
// nil inputs and omit the illustration when its image is empty.
func TestPipelineHelpers_NilBranches(t *testing.T) {
	if toolutil.NewPipelineOutput(nil) != nil {
		t.Error("toolutil.NewPipelineOutput(nil) should be nil")
	}
	if toolutil.NewBasicUserOutput(nil) != nil {
		t.Error("toolutil.NewBasicUserOutput(nil) should be nil")
	}
	if toolutil.NewPipelineDetailedStatusOutput(nil) != nil {
		t.Error("toolutil.NewPipelineDetailedStatusOutput(nil) should be nil")
	}
	if toolutil.FormatTimePtr(nil) != "" {
		t.Error("toolutil.FormatTimePtr(nil) should be empty")
	}
	ds := toolutil.NewPipelineDetailedStatusOutput(&gl.DetailedStatus{Label: "passed"})
	if ds == nil || ds.Illustration != nil {
		t.Errorf("detailed_status = %+v, want non-nil with nil illustration", ds)
	}
}

// TestDecorateMergeTrainMeta_UnknownTool verifies the metadata decorator leaves
// the options untouched when the tool name is not in the metadata map.
func TestDecorateMergeTrainMeta_UnknownTool(t *testing.T) {
	opts := toolutil.ActionSpecOptions{Usage: "original"}
	decorateMergeTrainMeta(&opts, "gitlab_unknown_tool")
	if opts.Usage != "original" {
		t.Errorf("usage = %q, want unchanged 'original'", opts.Usage)
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
	if out.User != nil {
		t.Errorf("got user %+v, want nil for nil User", out.User)
	}
	if out.Pipeline != nil {
		t.Errorf("got pipeline %+v, want nil for nil Pipeline", out.Pipeline)
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

// listTableHead is the header and delimiter of the merge train table, and
// listHints the guidance section every listing closes with, written once so the
// whole-output expectations below name them rather than repeating them.
const (
	listTableHead = "| ID | MR | Title | Target Branch | Status | Pipeline | User | Duration |\n" +
		"| --- | --- | --- | --- | --- | --- | --- | --- |\n"
	listHints = "\n---\n💡 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'merge_train.get' to read one merge request's position on the train\n" +
		"- Use action 'merge_train.add' to add another merge request to the train\n"
)

// TestFormatListMarkdown validates Markdown formatting for merge train lists.
// Covers empty trains, trains with WebURL links, and trains without WebURL.
//
// Every case asserts the whole rendering rather than a substring of it. The
// substring form is what let the card next door open a table and write list
// rows into it while "| Status | merged |" still matched.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name:  "empty list returns no-results message",
			input: ListOutput{Trains: []Output{}},
			want:  "No merge trains found.\n",
		},
		{
			name: "renders table with WebURL link",
			input: ListOutput{
				Trains: []Output{
					{
						ID:           1,
						TargetBranch: "main",
						Status:       "merged",
						User:         &toolutil.BasicUserOutput{ID: 1, Username: "admin"},
						Pipeline:     &toolutil.PipelineOutput{ID: 200, Status: "success", WebURL: "https://gitlab.example.com/-/pipelines/200"},
						Duration:     120,
						MergeRequest: MergeRequestOutput{IID: 5, Title: "Fix bug", WebURL: "https://gitlab.example.com/-/merge_requests/5"},
					},
				},
			},
			want: "## Merge Trains (1)\n\n" + listTableHead +
				"| 1 | [!5](https://gitlab.example.com/-/merge_requests/5) | Fix bug | main | merged | " +
				"[#200](https://gitlab.example.com/-/pipelines/200) ✅ success | @admin | 120s |\n" + listHints,
		},
		{
			name: "renders MR without WebURL as plain text",
			input: ListOutput{
				Trains: []Output{
					{
						ID:           2,
						TargetBranch: "develop",
						Status:       "idle",
						User:         &toolutil.BasicUserOutput{ID: 2, Username: "dev"},
						Duration:     0,
						MergeRequest: MergeRequestOutput{IID: 10, Title: "Add feature"},
					},
				},
			},
			want: "## Merge Trains (1)\n\n" + listTableHead +
				"| 2 | !10 | Add feature | develop | idle |  | @dev | 0s |\n" + listHints,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatListMarkdown(tt.input); got != tt.want {
				t.Errorf("rendered =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// cardHints is the guidance section a merge train card closes with.
const cardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'merge_train.list_project' to see every merge train in the project\n" +
	"- Use action 'merge_train.list_branch' to see the rest of this branch's train\n" +
	"- Use action 'merge_train.add' to add another merge request to the train\n"

// TestFormatOutputMarkdown validates Markdown formatting for a single merge
// train entry: the card of one object, asserted whole.
//
// This is the formatter the audit proved broken. It used to open a
// "| Property | Value |" table and then write "- **ID**: 1" into it, which
// ended the table with no body and left every later "| Status | merged |" on
// the page as literal pipes — while a test asserting exactly that substring
// passed. Nothing here is a substring any more.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input Output
		want  string
	}{
		{
			name: "renders full output with all optional fields",
			input: Output{
				ID:           1,
				TargetBranch: "main",
				Status:       "merged",
				User:         &toolutil.BasicUserOutput{ID: 1, Username: "admin"},
				Pipeline:     &toolutil.PipelineOutput{ID: 200, Status: "success", WebURL: "https://gitlab.example.com/-/pipelines/200"},
				Duration:     120,
				CreatedAt:    "2026-01-15T10:00:00Z",
				UpdatedAt:    "2026-01-16T10:00:00Z",
				MergedAt:     "2026-01-17T10:00:00Z",
				MergeRequest: MergeRequestOutput{IID: 5, Title: "Fix bug", WebURL: "https://gitlab.example.com/-/merge_requests/5"},
			},
			want: "## Merge Train #1\n\n" +
				"- **ID**: 1\n" +
				"- **Status**: merged\n" +
				"- **Target Branch**: main\n" +
				"- **Merge Request**: [!5](https://gitlab.example.com/-/merge_requests/5) - Fix bug\n" +
				"- **User**: @admin\n" +
				"- **Pipeline**: [#200](https://gitlab.example.com/-/pipelines/200) ✅ success\n" +
				"- **Duration**: 120s\n" +
				"- **Created**: 15 Jan 2026 10:00 UTC\n" +
				"- **Updated**: 16 Jan 2026 10:00 UTC\n" +
				"- **Merged**: 17 Jan 2026 10:00 UTC\n" + cardHints,
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
			want: "## Merge Train #2\n\n" +
				"- **ID**: 2\n" +
				"- **Status**: idle\n" +
				"- **Target Branch**: develop\n" +
				"- **Merge Request**: !10 - Add feature\n" +
				"- **Duration**: 0s\n" + cardHints,
		},
		{
			name: "renders MR without WebURL as plain text",
			input: Output{
				ID:           3,
				TargetBranch: "main",
				Status:       "active",
				MergeRequest: MergeRequestOutput{IID: 7, Title: "Update docs"},
			},
			want: "## Merge Train #3\n\n" +
				"- **ID**: 3\n" +
				"- **Status**: active\n" +
				"- **Target Branch**: main\n" +
				"- **Merge Request**: !7 - Update docs\n" +
				"- **Duration**: 0s\n" + cardHints,
		},
		{
			// A car GitLab sent no merge request for has neither an IID nor a
			// title, and the card must say nothing rather than link "!0" to
			// nowhere: the row is the reader's way to the merge request, and a
			// link to a merge request that does not exist is worse than none.
			name: "car with no merge request writes no merge request row",
			input: Output{
				ID:           4,
				TargetBranch: "main",
				Status:       "idle",
			},
			want: "## Merge Train #4\n\n" +
				"- **ID**: 4\n" +
				"- **Status**: idle\n" +
				"- **Target Branch**: main\n" +
				"- **Duration**: 0s\n" + cardHints,
		},
		{
			// An IID with no title is a merge request, so the row stays and
			// carries the reference alone. This is the half of the guard that
			// separates "GitLab sent no merge request" from "GitLab sent one
			// whose title this response omits".
			name: "merge request with an IID and no title renders the bare reference",
			input: Output{
				ID:           5,
				TargetBranch: "main",
				Status:       "idle",
				MergeRequest: MergeRequestOutput{IID: 7, WebURL: "https://gitlab.example.com/-/merge_requests/7"},
			},
			want: "## Merge Train #5\n\n" +
				"- **ID**: 5\n" +
				"- **Status**: idle\n" +
				"- **Target Branch**: main\n" +
				"- **Merge Request**: [!7](https://gitlab.example.com/-/merge_requests/7)\n" +
				"- **Duration**: 0s\n" + cardHints,
		},
		{
			// The row is dropped only when there is nothing at all to show,
			// which is why the guard reads "no IID and no title" rather than
			// "no IID": a response that carries a title keeps it, and the
			// reference renders beside it as GitLab numbered it.
			name: "merge request with a title and no IID keeps the title",
			input: Output{
				ID:           6,
				TargetBranch: "main",
				Status:       "idle",
				MergeRequest: MergeRequestOutput{Title: "Untracked change"},
			},
			want: "## Merge Train #6\n\n" +
				"- **ID**: 6\n" +
				"- **Status**: idle\n" +
				"- **Target Branch**: main\n" +
				"- **Merge Request**: Untracked change\n" +
				"- **Duration**: 0s\n" + cardHints,
		},
		{
			// A pipeline object carrying no id is no pipeline: the row would
			// otherwise read "#0" and link to nowhere, which a reader deciding
			// whether the car can leave the train would follow.
			name: "pipeline object with no id writes no pipeline row",
			input: Output{
				ID:           7,
				TargetBranch: "main",
				Status:       "idle",
				MergeRequest: MergeRequestOutput{IID: 8, Title: "Retry"},
				Pipeline:     &toolutil.PipelineOutput{Status: "created"},
			},
			want: "## Merge Train #7\n\n" +
				"- **ID**: 7\n" +
				"- **Status**: idle\n" +
				"- **Target Branch**: main\n" +
				"- **Merge Request**: !8 - Retry\n" +
				"- **Duration**: 0s\n" + cardHints,
		},
		{
			// A pipeline whose status GitLab omitted renders as the link alone,
			// rather than as a link followed by the glyph for a status nobody
			// sent.
			name: "pipeline with no status renders the link alone",
			input: Output{
				ID:           8,
				TargetBranch: "main",
				Status:       "idle",
				MergeRequest: MergeRequestOutput{IID: 9, Title: "Ship"},
				Pipeline:     &toolutil.PipelineOutput{ID: 300, WebURL: "https://gitlab.example.com/-/pipelines/300"},
			},
			want: "## Merge Train #8\n\n" +
				"- **ID**: 8\n" +
				"- **Status**: idle\n" +
				"- **Target Branch**: main\n" +
				"- **Merge Request**: !9 - Ship\n" +
				"- **Pipeline**: [#300](https://gitlab.example.com/-/pipelines/300)\n" +
				"- **Duration**: 0s\n" + cardHints,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.input); got != tt.want {
				t.Errorf("rendered =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
