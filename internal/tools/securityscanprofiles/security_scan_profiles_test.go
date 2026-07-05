// security_scan_profiles_test.go contains unit tests for GitLab security scan
// profile operations.
//
// The tests mock GitLab GraphQL mutations and queries with
// [testutil.GraphQLHandler], then call handlers directly to verify request
// payloads, validation, error wrapping, output conversion, markdown rendering,
// and ActionSpec metadata.
package securityscanprofiles

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// scanProfileInput parses the GraphQL input variables from r and fails the
// calling test if the request does not contain the expected input object.
func scanProfileInput(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	vars, err := testutil.ParseGraphQLVariables(r)
	if err != nil {
		t.Fatalf("ParseGraphQLVariables error: %v", err)
	}
	input, ok := vars["input"].(map[string]any)
	if !ok {
		t.Fatalf("GraphQL input = %#v, want map", vars["input"])
	}
	return input
}

// TestAttach_Success verifies that Attach resolves a scan-type identifier into
// a scan profile global ID and sends the project and group targets as GIDs.
func TestAttach_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityScanProfileAttach": func(w http.ResponseWriter, r *http.Request) {
			input := scanProfileInput(t, r)
			if got := input["securityScanProfileId"]; got != "gid://gitlab/Security::ScanProfile/dependency_scanning" {
				t.Errorf("securityScanProfileId = %v, want built-in scan-type GID", got)
			}
			if got := input["projectIds"]; !equalStrings(got, []string{"gid://gitlab/Project/7"}) {
				t.Errorf("projectIds = %v, want [gid://gitlab/Project/7]", got)
			}
			if got := input["groupIds"]; !equalStrings(got, []string{"gid://gitlab/Group/3"}) {
				t.Errorf("groupIds = %v, want [gid://gitlab/Group/3]", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityScanProfileAttach":{"errors":[]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Attach(context.Background(), client, AttachInput{
		SecurityScanProfileID: "dependency_scanning",
		ProjectIDs:            []int64{7},
		GroupIDs:              []int64{3},
	})
	if err != nil {
		t.Fatalf("Attach() unexpected error: %v", err)
	}
	if out.Status != "success" || out.SecurityScanProfileID != "dependency_scanning" {
		t.Errorf("Attach() output = %+v", out)
	}
	if len(out.ProjectIDs) != 1 || len(out.GroupIDs) != 1 {
		t.Errorf("Attach() echoed targets = %+v", out)
	}
}

// TestAttach_FullGIDPassthrough verifies that a caller-supplied global ID is
// sent verbatim rather than being wrapped a second time.
func TestAttach_FullGIDPassthrough(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityScanProfileAttach": func(w http.ResponseWriter, r *http.Request) {
			input := scanProfileInput(t, r)
			if got := input["securityScanProfileId"]; got != "gid://gitlab/Security::ScanProfile/42" {
				t.Errorf("securityScanProfileId = %v, want verbatim GID", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityScanProfileAttach":{"errors":[]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Attach(context.Background(), client, AttachInput{
		SecurityScanProfileID: "gid://gitlab/Security::ScanProfile/42",
		ProjectIDs:            []int64{1},
	})
	if err != nil {
		t.Fatalf("Attach() unexpected error: %v", err)
	}
}

// TestAttach_Validation verifies the required-field and target guards.
func TestAttach_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called on validation failure")
		w.WriteHeader(http.StatusOK)
	}))
	tests := []struct {
		name  string
		input AttachInput
	}{
		{"missing profile id", AttachInput{ProjectIDs: []int64{1}}},
		{"no targets", AttachInput{SecurityScanProfileID: "x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Attach(context.Background(), client, tc.input); err == nil {
				t.Fatal("Attach() expected validation error, got nil")
			}
		})
	}
}

// TestAttach_MutationError verifies that GraphQL payload errors are surfaced
// with an actionable hint.
func TestAttach_MutationError(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityScanProfileAttach": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityScanProfileAttach":{"errors":["not allowed"]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Attach(context.Background(), client, AttachInput{SecurityScanProfileID: "x", ProjectIDs: []int64{1}})
	if err == nil {
		t.Fatal("Attach() expected mutation error, got nil")
	}
	if !strings.Contains(err.Error(), "attach security scan profile") {
		t.Errorf("Attach() error = %v, want wrapped op prefix", err)
	}
}

// TestAttach_ContextCancelled verifies the context guard short-circuits before
// any request is dispatched.
func TestAttach_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with cancelled context")
		w.WriteHeader(http.StatusOK)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Attach(ctx, client, AttachInput{SecurityScanProfileID: "x", ProjectIDs: []int64{1}}); err == nil {
		t.Fatal("Attach() expected context error, got nil")
	}
}

// TestDetach_Success verifies that Detach sends the resolved profile and targets.
func TestDetach_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityScanProfileDetach": func(w http.ResponseWriter, r *http.Request) {
			input := scanProfileInput(t, r)
			if got := input["groupIds"]; !equalStrings(got, []string{"gid://gitlab/Group/9"}) {
				t.Errorf("groupIds = %v, want [gid://gitlab/Group/9]", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityScanProfileDetach":{"errors":[]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := Detach(context.Background(), client, DetachInput{
		SecurityScanProfileID: "dependency_scanning",
		GroupIDs:              []int64{9},
	})
	if err != nil {
		t.Fatalf("Detach() unexpected error: %v", err)
	}
	if out.Status != "success" || len(out.GroupIDs) != 1 {
		t.Errorf("Detach() output = %+v", out)
	}
}

// TestDetach_Validation verifies the required-field and target guards.
func TestDetach_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called on validation failure")
		w.WriteHeader(http.StatusOK)
	}))
	if _, err := Detach(context.Background(), client, DetachInput{SecurityScanProfileID: "x"}); err == nil {
		t.Fatal("Detach() expected validation error, got nil")
	}
	if _, err := Detach(context.Background(), client, DetachInput{ProjectIDs: []int64{1}}); err == nil {
		t.Fatal("Detach() expected validation error, got nil")
	}
}

// TestDetach_MutationError verifies that GraphQL payload errors surface with an
// actionable hint on the detach path.
func TestDetach_MutationError(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityScanProfileDetach": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityScanProfileDetach":{"errors":["not allowed"]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Detach(context.Background(), client, DetachInput{SecurityScanProfileID: "x", ProjectIDs: []int64{1}})
	if err == nil {
		t.Fatal("Detach() expected mutation error, got nil")
	}
	if !strings.Contains(err.Error(), "detach security scan profile") {
		t.Errorf("Detach() error = %v, want wrapped op prefix", err)
	}
}

// TestDetach_ContextCancelled verifies the context guard short-circuits before
// any request is dispatched.
func TestDetach_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with cancelled context")
		w.WriteHeader(http.StatusOK)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Detach(ctx, client, DetachInput{SecurityScanProfileID: "x", ProjectIDs: []int64{1}}); err == nil {
		t.Fatal("Detach() expected context error, got nil")
	}
}

// TestListProjectStatuses_ContextCancelled verifies the context guard
// short-circuits before any request is dispatched.
func TestListProjectStatuses_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called with cancelled context")
		w.WriteHeader(http.StatusOK)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListProjectStatuses(ctx, client, ListProjectStatusesInput{ProjectFullPath: "group/project"}); err == nil {
		t.Fatal("ListProjectStatuses() expected context error, got nil")
	}
}

// TestListProjectStatuses_Success verifies that statuses are mapped from the
// GraphQL response into the output structs.
func TestListProjectStatuses_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"scanProfileStatuses": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Fatalf("ParseGraphQLVariables error: %v", err)
			}
			if got := vars["fullPath"]; got != "group/project" {
				t.Errorf("fullPath = %v, want group/project", got)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"project":{"scanProfileStatuses":[
				{"status":"ACTIVE","scanProfile":{"id":"gid://gitlab/Security::ScanProfile/1","name":"Default","scanType":"dependency_scanning"}}
			]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ListProjectStatuses(context.Background(), client, ListProjectStatusesInput{ProjectFullPath: "group/project"})
	if err != nil {
		t.Fatalf("ListProjectStatuses() unexpected error: %v", err)
	}
	if len(out.Statuses) != 1 {
		t.Fatalf("ListProjectStatuses() statuses = %d, want 1", len(out.Statuses))
	}
	s := out.Statuses[0]
	if s.Status != "ACTIVE" || s.ScanProfile.ScanType != "dependency_scanning" {
		t.Errorf("ListProjectStatuses() status = %+v", s)
	}
}

// TestListProjectStatuses_Validation verifies the required-path guard.
func TestListProjectStatuses_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called on validation failure")
		w.WriteHeader(http.StatusOK)
	}))
	if _, err := ListProjectStatuses(context.Background(), client, ListProjectStatusesInput{ProjectFullPath: "  "}); err == nil {
		t.Fatal("ListProjectStatuses() expected validation error, got nil")
	}
}

// TestListProjectStatuses_NotFound verifies a null project surfaces a wrapped
// not-found error.
func TestListProjectStatuses_NotFound(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"scanProfileStatuses": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"project":null}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	if _, err := ListProjectStatuses(context.Background(), client, ListProjectStatusesInput{ProjectFullPath: "group/missing"}); err == nil {
		t.Fatal("ListProjectStatuses() expected not-found error, got nil")
	}
}

// TestFormatMutationMarkdown covers the attach/detach confirmation renderer.
func TestFormatMutationMarkdown(t *testing.T) {
	md := FormatMutationMarkdown(MutationOutput{
		Status: "success", Message: "Successfully attached security scan profile.",
		SecurityScanProfileID: "dependency_scanning",
		ProjectIDs:            []int64{7, 8}, GroupIDs: []int64{3},
	})
	for _, want := range []string{"Security Scan Profile", "dependency_scanning", "7, 8", "3"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListProjectStatusesMarkdown covers the populated and empty renders.
func TestFormatListProjectStatusesMarkdown(t *testing.T) {
	empty := FormatListProjectStatusesMarkdown(ListProjectStatusesOutput{ProjectFullPath: "g/p"})
	if !strings.Contains(empty, "No scan profile statuses found") {
		t.Errorf("empty markdown = %s", empty)
	}
	md := FormatListProjectStatusesMarkdown(ListProjectStatusesOutput{
		ProjectFullPath: "g/p",
		Statuses: []ScanProfileStatus{{
			Status: "ACTIVE", ScanProfile: ScanProfile{Name: "Default", ScanType: "sast"},
		}},
	})
	for _, want := range []string{"Scan Profile Statuses: g/p", "ACTIVE", "Default", "sast"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestActionSpecs_Metadata verifies the canonical action set, editions, and
// destructive/read-only classification.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("ActionSpecs() = %d specs, want 3", len(specs))
	}
	byName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, s := range specs {
		byName[s.Name] = s
		if s.Edition != "ultimate" {
			t.Errorf("action %q edition = %q, want ultimate", s.Name, s.Edition)
		}
		if s.OwnerPackage != "securityscanprofiles" {
			t.Errorf("action %q owner = %q", s.Name, s.OwnerPackage)
		}
	}
	if !byName["detach"].Destructive {
		t.Error("detach should be destructive")
	}
	if !byName["list_project_statuses"].ReadOnly {
		t.Error("list_project_statuses should be read-only")
	}
	if byName["attach"].Destructive {
		t.Error("attach should not be destructive")
	}
}

// equalStrings reports whether an any value decoded from JSON equals the
// expected slice of strings.
func equalStrings(got any, want []string) bool {
	list, ok := got.([]any)
	if !ok || len(list) != len(want) {
		return false
	}
	strs := make([]string, len(list))
	for i, v := range list {
		s, isStr := v.(string)
		if !isStr {
			return false
		}
		strs[i] = s
	}
	return slices.Equal(strs, want)
}
