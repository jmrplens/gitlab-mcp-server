// group_relations_test.go contains unit tests for the group relation list
// handlers added with client-go v2.41.0: SharedWithList, InvitedList, and
// TransferLocationsList. Tests use httptest to mock GitLab API responses.
package groups

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// canceledContext returns an already-canceled context to exercise the early
// ctx.Err() guard in the relation list handlers.
func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestGroupRelationMetadata_Discoverability locks in the model-facing discovery
// metadata for the relation tools added with client-go v2.41.0. It guards
// against a regression back to the generic "Use to execute ... domain action"
// placeholder and ensures each tool carries natural-language aliases, related
// canonical actions, and a cross-referencing description so models know when
// and how to choose them.
func TestGroupRelationMetadata_Discoverability(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := groupSpecsByTool(t, ActionSpecs(client))

	cases := []struct {
		tool          string
		aliasContains string
		related       string
		descContains  string
	}{
		{"gitlab_group_shared_with_list", "shared", "group.invited_groups", "shared with a GitLab group"},
		{"gitlab_group_invited_list", "invited", "group.shared_with", "invited to a GitLab group"},
		{"gitlab_group_transfer_locations", "transfer", "group.transfer_project", "candidate parent groups"},
		{"gitlab_group_transfer_project", "transfer", "group.transfer_locations", "namespace"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			spec, ok := byTool[tc.tool]
			if !ok {
				t.Fatalf("missing spec for %s", tc.tool)
			}
			if spec.Usage == "" || strings.Contains(spec.Usage, "Use to execute") {
				t.Errorf("%s has generic/empty Usage: %q", tc.tool, spec.Usage)
			}
			if !hasAliasContaining(spec.Aliases, tc.aliasContains) {
				t.Errorf("%s aliases %v missing phrase %q", tc.tool, spec.Aliases, tc.aliasContains)
			}
			if !slices.Contains(spec.RelatedActions, tc.related) {
				t.Errorf("%s related %v missing %q", tc.tool, spec.RelatedActions, tc.related)
			}
			if !strings.Contains(spec.IndividualTool.Description, "See also") || !strings.Contains(spec.IndividualTool.Description, tc.descContains) {
				t.Errorf("%s description weak: %q", tc.tool, spec.IndividualTool.Description)
			}
		})
	}
}

func hasAliasContaining(aliases []string, sub string) bool {
	for _, a := range aliases {
		if strings.Contains(a, sub) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// SharedWithList
// ---------------------------------------------------------------------------.

// TestSharedWithList_Success verifies that SharedWithList returns the groups shared with a group.
// The test exercises the GET groups/:id/groups/shared path.
// It asserts the returned output contains the mocked shared group.
func TestSharedWithList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/groups/shared", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":11,"name":"Shared","path":"shared","full_path":"shared","visibility":"private","web_url":"https://gitlab.example.com/groups/shared"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := SharedWithList(context.Background(), client, SharedWithListInput{GroupID: toolutil.StringOrInt("7")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != 11 {
		t.Fatalf("expected shared group 11, got %+v", out.Groups)
	}
}

// TestSharedWithList_AllFilters verifies that every optional filter is wired into the request.
// The test sets pagination, search, min access level, visibility, order, and sort.
// It asserts the query string carries each filter and the call succeeds.
func TestSharedWithList_AllFilters(t *testing.T) {
	var query string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/groups/shared", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "2", PerPage: "10"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := SharedWithList(context.Background(), client, SharedWithListInput{
		GroupID:              toolutil.StringOrInt("7"),
		Search:               "team",
		MinAccessLevel:       30,
		Visibility:           "private",
		OrderBy:              "name",
		Sort:                 "desc",
		SkipGroups:           []int64{11, 12},
		WithCustomAttributes: true,
		PaginationInput:      toolutil.PaginationInput{Page: 2, PerPage: 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"search=team", "min_access_level=30", "visibility=private", "order_by=name", "sort=desc", "skip_groups=", "with_custom_attributes=true", "page=2", "per_page=10"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q missing %q", query, want)
		}
	}
}

// TestSharedWithList_APIError verifies that an upstream error is wrapped with a hint.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestSharedWithList_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/groups/shared", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := SharedWithList(context.Background(), client, SharedWithListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestSharedWithList_CanceledContext verifies the early ctx.Err() guard.
// The test passes an already-canceled context.
// It asserts an error is returned before any request.
func TestSharedWithList_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := SharedWithList(canceledContext(), client, SharedWithListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

// TestSharedWithList_MissingGroupID verifies that SharedWithList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestSharedWithList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := SharedWithList(context.Background(), client, SharedWithListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvitedList
// ---------------------------------------------------------------------------.

// TestInvitedList_Success verifies that InvitedList returns the groups invited to a group.
// The test exercises the GET groups/:id/invited_groups path.
// It asserts the returned output contains the mocked invited group.
func TestInvitedList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/invited_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":12,"name":"Invited","path":"invited","full_path":"invited","visibility":"private","web_url":"https://gitlab.example.com/groups/invited"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := InvitedList(context.Background(), client, InvitedListInput{
		GroupID:  toolutil.StringOrInt("7"),
		Relation: []string{"direct"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != 12 {
		t.Fatalf("expected invited group 12, got %+v", out.Groups)
	}
}

// TestInvitedList_AllFilters verifies that every optional filter is wired into the request.
// The test sets pagination, search, min access level, and relation.
// It asserts the query string carries each filter.
func TestInvitedList_AllFilters(t *testing.T) {
	var query string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/invited_groups", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "3", PerPage: "5"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := InvitedList(context.Background(), client, InvitedListInput{
		GroupID:              toolutil.StringOrInt("7"),
		Search:               "guests",
		MinAccessLevel:       20,
		Relation:             []string{"direct", "inherited"},
		WithCustomAttributes: true,
		PaginationInput:      toolutil.PaginationInput{Page: 3, PerPage: 5},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"search=guests", "min_access_level=20", "relation=", "with_custom_attributes=true", "page=3", "per_page=5"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q missing %q", query, want)
		}
	}
}

// TestInvitedList_APIError verifies that an upstream error is wrapped.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestInvitedList_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/invited_groups", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := InvitedList(context.Background(), client, InvitedListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestInvitedList_CanceledContext verifies the early ctx.Err() guard.
// The test passes an already-canceled context.
// It asserts an error is returned before any request.
func TestInvitedList_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := InvitedList(canceledContext(), client, InvitedListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

// TestInvitedList_MissingGroupID verifies that InvitedList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestInvitedList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := InvitedList(context.Background(), client, InvitedListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TransferLocationsList
// ---------------------------------------------------------------------------.

// TestTransferLocationsList_Success verifies that TransferLocationsList returns candidate parent groups.
// The test exercises the GET groups/:id/transfer_locations path.
// It asserts the returned output contains the mocked location.
func TestTransferLocationsList_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/transfer_locations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":99,"name":"Target","full_name":"Target Group","full_path":"target","web_url":"https://gitlab.example.com/groups/target"}]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "1", Page: "1", PerPage: "20"})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{GroupID: toolutil.StringOrInt("7")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Locations) != 1 || out.Locations[0].ID != 99 {
		t.Fatalf("expected location 99, got %+v", out.Locations)
	}
	if out.Locations[0].FullPath != "target" {
		t.Errorf("expected full_path target, got %s", out.Locations[0].FullPath)
	}
}

// TestTransferLocationsList_AllFilters verifies that search and pagination are wired into the request.
// The test sets search plus page/per_page.
// It asserts the query string carries each value.
func TestTransferLocationsList_AllFilters(t *testing.T) {
	var query string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/transfer_locations", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
			testutil.PaginationHeaders{TotalPages: "1", Total: "0", Page: "2", PerPage: "15"})
	})
	client := testutil.NewTestClient(t, mux)

	_, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{
		GroupID:         toolutil.StringOrInt("7"),
		Search:          "org",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 15},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"search=org", "page=2", "per_page=15"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q missing %q", query, want)
		}
	}
}

// TestTransferLocationsList_APIError verifies that an upstream error is wrapped.
// The test makes the mock return 404.
// It asserts a non-nil error is returned.
func TestTransferLocationsList_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/7/transfer_locations", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

// TestTransferLocationsList_CanceledContext verifies the early ctx.Err() guard.
// The test passes an already-canceled context.
// It asserts an error is returned before any request.
func TestTransferLocationsList_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := TransferLocationsList(canceledContext(), client, TransferLocationsListInput{GroupID: toolutil.StringOrInt("7")})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

// TestTransferLocationsList_MissingGroupID verifies that TransferLocationsList validates group_id.
// The test exercises the input guard before any API call.
// It asserts the error names the missing group_id field.
func TestTransferLocationsList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := TransferLocationsList(context.Background(), client, TransferLocationsListInput{})
	if err == nil || !strings.Contains(err.Error(), "group_id is required") {
		t.Fatalf("expected group_id required error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Markdown
// ---------------------------------------------------------------------------.

// TestFormatTransferLocationsListMarkdown verifies the transfer-locations markdown formatter.
// The test exercises rendering of a populated location list.
// It asserts the rendered Markdown contains a clickable name link.
func TestFormatTransferLocationsListMarkdown(t *testing.T) {
	out := TransferLocationsListOutput{Locations: []TransferLocationOutput{
		{ID: 99, Name: "Target", FullPath: "target", WebURL: "https://gitlab.example.com/groups/target"},
	}}
	md := FormatTransferLocationsListMarkdown(out)
	if !strings.Contains(md, "[Target](https://gitlab.example.com/groups/target)") {
		t.Errorf("expected clickable link in markdown, got: %s", md)
	}
}

// TestFormatTransferLocationsListMarkdown_Empty verifies the empty-state rendering.
// The test exercises rendering of an empty location list.
// It asserts the empty-state message is present.
func TestFormatTransferLocationsListMarkdown_Empty(t *testing.T) {
	md := FormatTransferLocationsListMarkdown(TransferLocationsListOutput{})
	if !strings.Contains(md, "No transfer locations available") {
		t.Errorf("expected empty-state message, got: %s", md)
	}
}

// ---------------------------------------------------------------------------
// Create/Update new options (client-go v2.41.0)
// ---------------------------------------------------------------------------.

// TestCreate_NewOptions verifies that the v2.41.0 group create options are sent to the API.
// The test inspects the request body for the new boolean flags.
// It asserts each new flag appears in the marshaled request.
func TestCreate_NewOptions(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body = string(buf)
		testutil.RespondJSON(w, http.StatusCreated,
			`{"id":1,"name":"G","path":"g","full_path":"g","visibility":"private","web_url":"https://x/groups/g"}`)
	})
	client := testutil.NewTestClient(t, mux)

	enabled := true
	orgID := int64(42)
	limit := int64(100)
	interval := int64(300)
	_, err := Create(context.Background(), client, CreateInput{
		Name:                         "G",
		OrganizationID:               &orgID,
		MathRenderingLimitsEnabled:   &enabled,
		WebBasedCommitSigningEnabled: &enabled,
		AllowPersonalSnippets:        &enabled,
		UniqueProjectDownloadLimit:   &limit,
		UniqueProjectDownloadLimitIntervalInSeconds: &interval,
		UniqueProjectDownloadLimitAllowlist:         []string{"trusted-user"},
		UniqueProjectDownloadLimitAlertlist:         []int64{1, 2},
		AutoBanUserOnExcessiveProjectsDownload:      &enabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, field := range []string{
		"organization_id", "math_rendering_limits_enabled", "web_based_commit_signing_enabled", "allow_personal_snippets",
		"unique_project_download_limit", "unique_project_download_limit_interval_in_seconds",
		"unique_project_download_limit_allowlist", "unique_project_download_limit_alertlist",
		"auto_ban_user_on_excessive_projects_download",
	} {
		if !strings.Contains(body, field) {
			t.Errorf("expected %s in request body, got: %s", field, body)
		}
	}
}

// TestUpdate_NewOptions verifies that the v2.41.0 group update options are sent to the API.
// The test inspects the request body for the new boolean flags.
// It asserts each new flag appears in the marshaled request.
func TestUpdate_NewOptions(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/groups/5", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body = string(buf)
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":5,"name":"G","path":"g","full_path":"g","visibility":"private","web_url":"https://x/groups/g"}`)
	})
	client := testutil.NewTestClient(t, mux)

	enabled := false
	_, err := Update(context.Background(), client, UpdateInput{
		GroupID:                      toolutil.StringOrInt("5"),
		MathRenderingLimitsEnabled:   &enabled,
		WebBasedCommitSigningEnabled: &enabled,
		AllowPersonalSnippets:        &enabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, field := range []string{"math_rendering_limits_enabled", "web_based_commit_signing_enabled", "allow_personal_snippets"} {
		if !strings.Contains(body, field) {
			t.Errorf("expected %s in request body, got: %s", field, body)
		}
	}
}
