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

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// scanProfileInput parses the GraphQL input variables from r and records a test
// failure if the request does not contain the expected input object. It runs
// inside the httptest server goroutine, so it uses t.Errorf (not t.Fatal, which
// must only be called from the test goroutine) and returns nil on failure.
func scanProfileInput(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	vars, err := testutil.ParseGraphQLVariables(r)
	if err != nil {
		t.Errorf("ParseGraphQLVariables error: %v", err)
		return nil
	}
	input, ok := vars["input"].(map[string]any)
	if !ok {
		t.Errorf("GraphQL input = %#v, want map", vars["input"])
		return nil
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
		SecurityScanProfileID: "gid://gitlab/Security::ScanProfile/5",
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

// TestDetach_RejectsScanTypeName verifies that detach fails fast with an
// actionable error when given a scan-type name (or any non-numeric identifier)
// instead of the persisted profile's numeric ID, without dispatching a request.
func TestDetach_RejectsScanTypeName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for a non-numeric detach identifier")
		w.WriteHeader(http.StatusOK)
	}))
	for _, id := range []string{"dependency_scanning", "gid://gitlab/Security::ScanProfile/dependency_scanning", "gid://"} {
		t.Run(id, func(t *testing.T) {
			_, err := Detach(context.Background(), client, DetachInput{SecurityScanProfileID: id, ProjectIDs: []int64{1}})
			if err == nil {
				t.Fatalf("Detach(%q) expected validation error, got nil", id)
			}
			if !strings.Contains(err.Error(), "numeric ID") {
				t.Errorf("Detach(%q) error = %v, want persisted-numeric-ID hint", id, err)
			}
		})
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

	_, err := Detach(context.Background(), client, DetachInput{SecurityScanProfileID: "1", ProjectIDs: []int64{1}})
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
				t.Errorf("ParseGraphQLVariables error: %v", err)
				return
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

// mutationHints is the guidance section every mutation render closes with.
const mutationHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'security_scan_profile.list_project_statuses' to see which scan profiles a project carries now\n" +
	"- Use action 'vulnerability.list' to read the vulnerabilities a scan found\n"

// TestFormatMutationMarkdown covers the attach/detach confirmation renderer.
// The whole render is compared rather than a set of substrings: a substring
// assertion passes on a row that landed outside the block it was meant for.
func TestFormatMutationMarkdown(t *testing.T) {
	md := FormatMutationMarkdown(MutationOutput{
		Status: "success", Message: "Successfully attached security scan profile.",
		SecurityScanProfileID: "dependency_scanning",
		ProjectIDs:            []int64{7, 8}, GroupIDs: []int64{3},
	})

	want := "## Security Scan Profile\n\n" +
		"- **Result**: Successfully attached security scan profile.\n" +
		"- **Status**: success\n" +
		"- **Profile**: `dependency_scanning`\n" +
		"- **Projects**: 7, 8\n" +
		"- **Groups**: 3\n" +
		mutationHints

	if md != want {
		t.Errorf("FormatMutationMarkdown() =\n%s\nwant:\n%s", md, want)
	}
}

// TestFormatMutationMarkdown_NoTargets verifies that a confirmation naming no
// projects and no groups writes neither row: an absent value is never a label
// with nothing after it.
func TestFormatMutationMarkdown_NoTargets(t *testing.T) {
	md := FormatMutationMarkdown(MutationOutput{
		Status: "success", Message: "Successfully detached security scan profile.",
		SecurityScanProfileID: "42",
	})

	want := "## Security Scan Profile\n\n" +
		"- **Result**: Successfully detached security scan profile.\n" +
		"- **Status**: success\n" +
		"- **Profile**: `42`\n" +
		mutationHints

	if md != want {
		t.Errorf("FormatMutationMarkdown() =\n%s\nwant:\n%s", md, want)
	}
}

// TestFormatMutationMarkdown_HostileProfileID verifies that the identifier the
// caller supplied cannot open a heading, a list item or a raw tag: it is shown
// inside a code span, where nothing is Markdown.
func TestFormatMutationMarkdown_HostileProfileID(t *testing.T) {
	md := FormatMutationMarkdown(MutationOutput{
		Status:                "success",
		Message:               "Successfully attached security scan profile.",
		SecurityScanProfileID: "x\n## injected\n<a href=\"http://attacker.invalid\">x</a>",
	})

	want := "## Security Scan Profile\n\n" +
		"- **Result**: Successfully attached security scan profile.\n" +
		"- **Status**: success\n" +
		"- **Profile**: `x ## injected <a href=\"http://attacker.invalid\">x</a>`\n" +
		mutationHints

	if md != want {
		t.Errorf("FormatMutationMarkdown() =\n%s\nwant:\n%s", md, want)
	}
}

// TestFormatListProjectStatusesMarkdown covers the populated and empty
// renders. The profile ID is a column because it is what the detach action
// takes, and nothing else in the surface hands it to a reader.
func TestFormatListProjectStatusesMarkdown(t *testing.T) {
	empty := FormatListProjectStatusesMarkdown(ListProjectStatusesOutput{ProjectFullPath: "g/p"})
	if want := "No scan profile statuses found.\n"; empty != want {
		t.Errorf("empty markdown = %q, want %q", empty, want)
	}

	md := FormatListProjectStatusesMarkdown(ListProjectStatusesOutput{
		ProjectFullPath: "g/p",
		Statuses: []ScanProfileStatus{{
			Status: "ACTIVE", ScanProfile: ScanProfile{ID: "51", Name: "Default", ScanType: "sast"},
		}},
	})

	want := "## Scan Profile Statuses: g/p (1)\n\n" +
		"| ID | Scan Type | Profile | Status |\n" +
		"| --- | --- | --- | --- |\n" +
		"| `51` | sast | Default | ACTIVE |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_scan_profile.attach' to attach a scan profile to more projects or groups\n" +
		"- Use action 'security_scan_profile.detach' to detach one, naming the profile ID above\n"

	if md != want {
		t.Errorf("FormatListProjectStatusesMarkdown() =\n%s\nwant:\n%s", md, want)
	}
	if strings.Contains(md, "clickable [text](url) links") {
		t.Error("the list tells the model to keep links a table without links cannot have")
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
