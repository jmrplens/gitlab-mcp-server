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
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
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
// The echoed confirmation is compared field by field, projects against groups:
// the two lists are the only record of what the call touched, and a swap
// between them is invisible to a length check.
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
	if out.Status != "success" || out.Message != "Successfully attached security scan profile." {
		t.Errorf("Attach() status/message = %q/%q, want success and the attach confirmation", out.Status, out.Message)
	}
	if out.SecurityScanProfileID != "dependency_scanning" {
		t.Errorf("Attach() echoed profile = %q, want the caller's own identifier", out.SecurityScanProfileID)
	}
	if !slices.Equal(out.ProjectIDs, []int64{7}) || !slices.Equal(out.GroupIDs, []int64{3}) {
		t.Errorf("Attach() echoed projects %v and groups %v, want [7] and [3]", out.ProjectIDs, out.GroupIDs)
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

// TestAttach_ScanTypeWithoutDefaultProfile_HintOffersTheDefaultProfiles
// verifies what a caller learns when it names a scan type GitLab defines no
// default profile for. GitLab's FindOrCreateService finds nothing for
// container_scanning and the mutation answers with the generic
// resource-not-available error, which names neither the value at fault nor
// the ones that would have worked. The name still reaches GitLab untouched,
// since whether a scan type has a default profile is GitLab's to say and can
// change in a release; what the handler adds is the list of names that do,
// the release each needs, and the three that are refused by name.
func TestAttach_ScanTypeWithoutDefaultProfile_HintOffersTheDefaultProfiles(t *testing.T) {
	var sent string
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityScanProfileAttach": func(w http.ResponseWriter, r *http.Request) {
			if input := scanProfileInput(t, r); input != nil {
				sent, _ = input["securityScanProfileId"].(string)
			}
			testutil.RespondJSON(w, http.StatusOK, `{"errors":[{"message":"The resource that you are attempting to access does not exist or you don't have permission to perform this action"}],"data":{"securityScanProfileAttach":null}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	_, err := Attach(context.Background(), client, AttachInput{SecurityScanProfileID: "container_scanning", ProjectIDs: []int64{1}})
	if err == nil {
		t.Fatal("Attach() error = nil, want GitLab's refusal of a scan type with no default profile")
	}
	if sent != "gid://gitlab/Security::ScanProfile/container_scanning" {
		t.Errorf("securityScanProfileId = %q, want the scan type wrapped as a global ID and left to GitLab to judge", sent)
	}
	for _, want := range []string{DefaultProfileNames, DefaultProfileFloors, RefusedByName} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Attach() error = %q, want it to carry %q", err.Error(), want)
			}
		})
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

// TestDetach_Success verifies that Detach sends the resolved profile and
// targets when a group is the only target named. Its confirmation is held to
// the detach wording and to each target list in its own field: attach and
// detach return the same type, so a confirmation naming the wrong operation
// reads as a successful attach, and a group echoed under project_ids tells a
// reader the call touched something it never touched.
func TestDetach_Success(t *testing.T) {
	handler := testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"securityScanProfileDetach": func(w http.ResponseWriter, r *http.Request) {
			input := scanProfileInput(t, r)
			if got := input["projectIds"]; !equalStrings(got, []string{}) {
				t.Errorf("projectIds = %v, want an empty list", got)
			}
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
	if out.Status != "success" || out.Message != "Successfully detached security scan profile." {
		t.Errorf("Detach() status/message = %q/%q, want success and the detach confirmation", out.Status, out.Message)
	}
	if out.SecurityScanProfileID != "gid://gitlab/Security::ScanProfile/5" {
		t.Errorf("Detach() echoed profile = %q, want the caller's own identifier", out.SecurityScanProfileID)
	}
	if len(out.ProjectIDs) != 0 || !slices.Equal(out.GroupIDs, []int64{9}) {
		t.Errorf("Detach() echoed projects %v and groups %v, want none and [9]", out.ProjectIDs, out.GroupIDs)
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

// TestDetach_IdentifierForms_DecideWhatReachesGitLab drives each shape of the
// detach identifier through the handler and asserts what GitLab is sent, or
// that nothing is sent at all. The digits at both ends of the range and the
// padded value are here because the guard scans runes and the resolver trims:
// a boundary off by one would refuse a profile whose ID contains a 0 or a 9,
// and a lost trim would send GitLab an identifier with spaces inside it.
func TestDetach_IdentifierForms_DecideWhatReachesGitLab(t *testing.T) {
	tests := []struct {
		name string
		id   string
		// sent is the securityScanProfileId GitLab receives; empty means the
		// guard must refuse the identifier before a request exists.
		sent string
	}{
		{"digits at both ends of the range", "90", "gid://gitlab/Security::ScanProfile/90"},
		{"surrounding whitespace trimmed off", "  42  ", "gid://gitlab/Security::ScanProfile/42"},
		{"global ID with a numeric tail", "gid://gitlab/Security::ScanProfile/109", "gid://gitlab/Security::ScanProfile/109"},
		{"a global ID with no authority and no type", "gid:///5", ""},
		{"a global ID whose authority is not gitlab", "gid://elsewhere/Security::ScanProfile/5", ""},
		{"a global ID carrying an id but no type", "gid://gitlab/5", ""},
		{"a global ID whose type is the empty string", "gid://gitlab//5", ""},
		{"a character below the digits", "4-2", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Errorf("Detach(%q) dispatched a request the guard should have refused", tc.id)
				w.WriteHeader(http.StatusOK)
			})
			if tc.sent != "" {
				handler = testutil.GraphQLHandler(map[string]http.HandlerFunc{
					"securityScanProfileDetach": func(w http.ResponseWriter, r *http.Request) {
						input := scanProfileInput(t, r)
						if got := input["securityScanProfileId"]; got != tc.sent {
							t.Errorf("securityScanProfileId = %v, want %q", got, tc.sent)
						}
						testutil.RespondGraphQL(w, http.StatusOK, `{"securityScanProfileDetach":{"errors":[]}}`)
					},
				})
			}
			client := testutil.NewTestClient(t, handler)

			_, err := Detach(context.Background(), client, DetachInput{SecurityScanProfileID: tc.id, ProjectIDs: []int64{1}})
			if tc.sent == "" {
				if err == nil {
					t.Fatalf("Detach(%q) expected validation error, got nil", tc.id)
				}
				return
			}
			if err != nil {
				t.Fatalf("Detach(%q) unexpected error: %v", tc.id, err)
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
// GraphQL response into the output structs. Every value in the fixture differs
// from every other, and the whole slice is compared rather than one field:
// the ID is what detach takes and the name is prose, so a converter reading
// one where it meant the other hands a model an identifier that does not
// exist. The path is asserted on both sides because it is sent trimmed and
// echoed back as the heading of the render.
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
				{"status":"ACTIVE","scanProfile":{"id":"gid://gitlab/Security::ScanProfile/31","name":"Nightly deep scan","scanType":"dependency_scanning"}},
				{"status":"NOT_CONFIGURED","scanProfile":{"id":"gid://gitlab/Security::ScanProfile/48","name":"Baseline secrets","scanType":"secret_detection"}}
			]}}`)
		},
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ListProjectStatuses(context.Background(), client, ListProjectStatusesInput{ProjectFullPath: "  group/project  "})
	if err != nil {
		t.Fatalf("ListProjectStatuses() unexpected error: %v", err)
	}
	if out.ProjectFullPath != "group/project" {
		t.Errorf("ListProjectStatuses() path = %q, want the trimmed group/project", out.ProjectFullPath)
	}
	want := []ScanProfileStatus{
		{Status: "ACTIVE", ScanProfile: ScanProfile{
			ID: "gid://gitlab/Security::ScanProfile/31", Name: "Nightly deep scan", ScanType: "dependency_scanning",
		}},
		{Status: "NOT_CONFIGURED", ScanProfile: ScanProfile{
			ID: "gid://gitlab/Security::ScanProfile/48", Name: "Baseline secrets", ScanType: "secret_detection",
		}},
	}
	if !slices.Equal(out.Statuses, want) {
		t.Errorf("ListProjectStatuses() statuses =\n%+v\nwant:\n%+v", out.Statuses, want)
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

// TestActionSpecs_ActionIDs_NameActionsThatExist holds the domain.action
// constants the markdown hints and the related lists are built from against
// the spec names they claim to address. Nothing in the repository validates a
// related_actions entry or a hint target, so a misspelling passes every gate
// and answers a model "unknown action" the moment it follows the hint.
func TestActionSpecs_ActionIDs_NameActionsThatExist(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	own := make(map[string]bool, len(specs))
	for _, s := range specs {
		own["security_scan_profile."+s.Name] = true
	}
	for _, id := range []string{actionAttach, actionDetach, actionListProjectStatuses} {
		t.Run(id, func(t *testing.T) {
			if !own[id] {
				t.Errorf("constant %q names no action this package registers (registered: %v)", id, own)
			}
		})
	}
	// The two foreign IDs are the only actions outside this package the specs
	// and hints are allowed to send a model to; both are checked against the
	// catalog by hand, since nothing here can see it.
	neighbors := map[string]bool{actionProjectGet: true, actionVulnList: true}
	for _, s := range specs {
		for _, rel := range s.RelatedActions {
			if !own[rel] && !neighbors[rel] {
				t.Errorf("action %q relates to %q, which is neither ours nor a declared neighbor", s.Name, rel)
			}
		}
	}
}

// scanTypesRefusedByName are the SecurityScanProfileType values GitLab defines
// no default profile for, each with the evidence, read from GitLab's source at
// the pinned 19.5.0-pre (5041f73d695) and at every stable branch from 18-7 to
// 19-4. Attach refuses them by name, so [DefaultProfileNames] leaves them out
// and the description names them instead. A value added here that the enum no
// longer holds fails the test below, so the table cannot outlive its reason.
var scanTypesRefusedByName = map[string]string{
	"container_scanning": "Security::DefaultScanProfilesHelper builds no container scanning profile at any release from 18.7 to 19.5",
	"business_logic":     "added to the enum at 19.5 as a scan run by an AI flow (Enums::Security::FLOW_BACKED_SCAN_TYPES), with no default profile",
}

// presetsOfScanType are the scan types whose default profiles are named by
// preset key rather than by scan type: GitLab 19.4 builds one Triage and
// Remediation profile per entry of
// Security::ScanProfiles::Configuration::Defaults::TriageAndRemediation::PRESETS
// and keys each as triage_and_remediation_<preset>, so the bare scan type
// matches none of them.
var presetsOfScanType = map[string][]string{
	"triage_and_remediation": {"conservative", "standard", "proactive"},
}

// listedValues splits a sentence fragment listing values, "a, b, or c", into
// the values it names, sorted.
func listedValues(fragment string) []string {
	fragment = strings.ReplaceAll(fragment, ", or ", ", ")
	values := strings.Split(fragment, ", ")
	slices.Sort(values)
	return values
}

// pinnedEnum returns the values the pinned GitLab schema declares for the enum
// named typeName, sorted.
func pinnedEnum(t *testing.T, typeName string) []string {
	t.Helper()
	schema, err := graphqlschema.Schema()
	if err != nil {
		t.Fatalf("graphqlschema.Schema() error: %v", err)
	}
	definition, ok := schema.Types[typeName]
	if !ok || len(definition.EnumValues) == 0 {
		t.Fatalf("the pinned schema declares no enum %s", typeName)
	}
	values := make([]string, 0, len(definition.EnumValues))
	for _, value := range definition.EnumValues {
		values = append(values, value.Name)
	}
	slices.Sort(values)
	return values
}

// TestDefaultProfileNames_AreTheSchemaScanTypesThatHaveADefaultProfile holds
// the names attach is described as taking to the SecurityScanProfileType enum
// of the pinned schema. The two differ on purpose, by the scan types GitLab
// builds no default profile for and by the presets one scan type is offered
// through, and both differences are declared in the tables above. What the
// test catches is everything else: a scan type GitLab adds at a re-pin, which
// fails here until someone decides whether attach takes it by name, and a
// name the list offers that the enum never had. The list used to offer
// container_scanning, which no GitLab release builds a default profile for,
// and it left out the three names GitLab 19.2 and 19.4 added.
func TestDefaultProfileNames_AreTheSchemaScanTypesThatHaveADefaultProfile(t *testing.T) {
	enum := pinnedEnum(t, "SecurityScanProfileType")
	var want []string
	for _, value := range enum {
		scanType := strings.ToLower(value)
		if _, refused := scanTypesRefusedByName[scanType]; refused {
			continue
		}
		presets, byPreset := presetsOfScanType[scanType]
		if !byPreset {
			want = append(want, scanType)
			continue
		}
		for _, preset := range presets {
			want = append(want, scanType+"_"+preset)
		}
	}
	slices.Sort(want)
	if got := listedValues(DefaultProfileNames); !slices.Equal(got, want) {
		t.Errorf("DefaultProfileNames lists %v, want the pinned SecurityScanProfileType %v less the declared refusals, with the declared presets: %v", got, enum, want)
	}

	declared := slices.Concat(slices.Collect(maps.Keys(scanTypesRefusedByName)), slices.Collect(maps.Keys(presetsOfScanType)))
	for _, scanType := range declared {
		t.Run(scanType, func(t *testing.T) {
			if !slices.Contains(enum, strings.ToUpper(scanType)) {
				t.Errorf("%q is declared, but the pinned SecurityScanProfileType %v no longer has it", scanType, enum)
			}
		})
	}
}

// TestAttach_ServedText_OffersTheDefaultProfilesAndNamesTheRefusals verifies
// that the two texts a model reads before an attach, the input's description
// as the built schema serves it and the usage line, offer the list the test
// above holds to the schema, the release each name needs, and the scan types
// attach refuses by name; the hint a refused attach carries is held by
// TestAttach_ScanTypeWithoutDefaultProfile_HintOffersTheDefaultProfiles, and
// the meta group description by a test in internal/tools. The description is
// a struct tag and cannot be built from the constants, so it is the text that
// could drift on its own.
func TestAttach_ServedText_OffersTheDefaultProfilesAndNamesTheRefusals(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	var attach toolutil.ActionSpec
	for _, spec := range ActionSpecs(client) {
		if spec.Name == "attach" {
			attach = spec
		}
	}
	properties, _ := attach.Route.InputSchema["properties"].(map[string]any)
	property, _ := properties["security_scan_profile_id"].(map[string]any)
	description, _ := property["description"].(string)

	texts := map[string]string{"description": description, "usage": attach.Usage}
	for name, text := range texts {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{DefaultProfileNames, DefaultProfileFloors, RefusedByName} {
				if !strings.Contains(text, want) {
					t.Errorf("%s = %q, want it to carry %q", name, text, want)
				}
			}
		})
	}
}

// TestDefaultProfileSentences_HeldToTheScanTypeTables holds the two sentences
// every text offering the default profile names adds to the tables the test
// of the names reads: every scan type refused, and the bare type of every
// preset family, is named in the refusal, and every name offered past
// secret_detection, which is as old as scan profiles, is given its release,
// so a name added to the list without a floor fails here.
func TestDefaultProfileSentences_HeldToTheScanTypeTables(t *testing.T) {
	for scanType := range scanTypesRefusedByName {
		t.Run("refused "+scanType, func(t *testing.T) {
			if !strings.Contains(RefusedByName, scanType+" ") {
				t.Errorf("RefusedByName = %q, want it to name %s", RefusedByName, scanType)
			}
		})
	}
	for scanType := range presetsOfScanType {
		t.Run("bare "+scanType, func(t *testing.T) {
			if !strings.Contains(RefusedByName, "bare "+scanType+" ") {
				t.Errorf("RefusedByName = %q, want it to name the bare %s", RefusedByName, scanType)
			}
			if !strings.Contains(DefaultProfileFloors, scanType+" presets need ") {
				t.Errorf("DefaultProfileFloors = %q, want it to give the %s presets their release", DefaultProfileFloors, scanType)
			}
		})
	}
	for _, name := range listedValues(DefaultProfileNames) {
		if name == "secret_detection" || presetFamily(name) != "" {
			continue
		}
		t.Run("floor "+name, func(t *testing.T) {
			if !strings.Contains(DefaultProfileFloors, name+" needs ") {
				t.Errorf("DefaultProfileFloors = %q, want it to say which release %s needs", DefaultProfileFloors, name)
			}
		})
	}
}

// presetFamily returns the scan type a default profile name is a preset of,
// or "" for a name that is a scan type itself.
func presetFamily(name string) string {
	for scanType, presets := range presetsOfScanType {
		for _, preset := range presets {
			if name == scanType+"_"+preset {
				return scanType
			}
		}
	}
	return ""
}

// TestDescriptionList_StatusesAreTheSchemaEnum holds the statuses the list
// tool's description names to the ScanProfileStatus enum client-go's statuses
// query decodes, so a status GitLab adds is a failure here rather than a
// value the description never mentions.
func TestDescriptionList_StatusesAreTheSchemaEnum(t *testing.T) {
	_, rest, found := strings.Cut(descriptionList, "profile status (")
	listed, _, closed := strings.Cut(rest, ")")
	if !found || !closed {
		t.Fatalf("descriptionList = %q, want it to list the statuses in parentheses after %q", descriptionList, "profile status")
	}
	if got, want := listedValues(listed), pinnedEnum(t, "ScanProfileStatus"); !slices.Equal(got, want) {
		t.Errorf("descriptionList names the statuses %v, want the pinned ScanProfileStatus %v", got, want)
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
