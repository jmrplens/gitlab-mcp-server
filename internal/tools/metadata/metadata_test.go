// metadata_test.go contains unit tests for the GitLab metadata MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package metadata

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestGet verifies Get.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/metadata" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"version": "16.8.0",
			"revision": "abc123",
			"kas": {
				"enabled": true,
				"externalUrl": "wss://kas.example.com",
				"externalK8sProxyUrl": "https://kas.example.com/k8s-proxy",
				"version": "16.8.0-rc1"
			},
			"enterprise": true
		}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Version != "16.8.0" {
		t.Errorf("Version = %q, want 16.8.0", out.Version)
	}
	if out.Revision != "abc123" {
		t.Errorf("Revision = %q, want abc123", out.Revision)
	}
	if !out.Enterprise {
		t.Error("Enterprise = false, want true")
	}
	if !out.KAS.Enabled {
		t.Error("KAS.Enabled = false, want true")
	}
	if out.KAS.Version != "16.8.0-rc1" {
		t.Errorf("KAS.Version = %q, want 16.8.0-rc1", out.KAS.Version)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestFormatGetMarkdown verifies FormatGetMarkdown.
func TestFormatGetMarkdown(t *testing.T) {
	out := GetOutput{
		Version:    "16.8.0",
		Revision:   "abc123",
		Enterprise: true,
		KAS:        KASInfo{Enabled: true, Version: "16.8.0-rc1", ExternalURL: "wss://kas"},
	}
	md := FormatGetMarkdown(out)
	if !strings.Contains(md, "16.8.0") {
		t.Error("missing version")
	}
	if !strings.Contains(md, "abc123") {
		t.Error("missing revision")
	}
	if !strings.Contains(md, "KAS Enabled") {
		t.Error("missing KAS enabled")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// covCovMetaJSON identifies the cov cov meta JSON constant used by this package.
const covCovMetaJSON = `{"version":"17.0.0","revision":"abc123","kas":{"enabled":true,"external_url":"https://kas.example.com","external_k8s_proxy_url":"https://k8s.example.com","version":"17.0.0"},"enterprise":true}`

// TestGet_APIError_Coverage verifies the API error path for metadata lookup.
func TestGet_APIError_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success_Coverage verifies a successful metadata lookup with KAS and
// Enterprise fields.
func TestGet_Success_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covCovMetaJSON)
	}))
	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Version != "17.0.0" || !out.Enterprise || !out.KAS.Enabled {
		t.Errorf("unexpected: %+v", out)
	}
}

// TestFormatGetMarkdown_Full_Coverage verifies full metadata Markdown output.
func TestFormatGetMarkdown_Full_Coverage(t *testing.T) {
	out := GetOutput{
		Version:    "17.0.0",
		Revision:   "abc123",
		Enterprise: true,
		KAS: KASInfo{
			Enabled:     true,
			Version:     "17.0.0",
			ExternalURL: "https://kas.example.com",
		},
	}
	md := FormatGetMarkdown(out)
	if !strings.Contains(md, "17.0.0") || !strings.Contains(md, "abc123") || !strings.Contains(md, "kas.example.com") {
		t.Error("expected metadata in markdown")
	}
}

// TestFormatGetMarkdown_NoKAS_Coverage verifies FormatGetMarkdown when no kas coverage.
func TestFormatGetMarkdown_NoKAS_Coverage(t *testing.T) {
	md := FormatGetMarkdown(GetOutput{Version: "17.0.0"})
	if strings.Contains(md, "KAS Version") || strings.Contains(md, "KAS URL") {
		t.Error("should not show KAS details when empty")
	}
}

// TestActionSpecs_MetadataGet_Coverage verifies metadata action spec metadata
// and canonical route execution.
func TestActionSpecs_MetadataGet_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covCovMetaJSON)
	}))

	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	spec := specs[0]
	if spec.Name != "metadata_get" {
		t.Errorf("Name = %q, want metadata_get", spec.Name)
	}
	if spec.OwnerPackage != "metadata" {
		t.Errorf("OwnerPackage = %q, want metadata", spec.OwnerPackage)
	}
	if spec.IndividualTool.Name != "gitlab_get_metadata" {
		t.Errorf("IndividualTool.Name = %q, want gitlab_get_metadata", spec.IndividualTool.Name)
	}
	if !spec.ReadOnly || !spec.Idempotent || !spec.OpenWorld {
		t.Errorf("unexpected action semantics: read_only=%v idempotent=%v open_world=%v", spec.ReadOnly, spec.Idempotent, spec.OpenWorld)
	}
	if !slices.Contains(spec.Aliases, "gitlab version") {
		t.Fatalf("Aliases = %v, want gitlab version", spec.Aliases)
	}
	if !strings.Contains(spec.Usage, "Do not use this for application settings") {
		t.Fatalf("Usage = %q, want settings distinction", spec.Usage)
	}
	if !strings.Contains(spec.IndividualTool.Description, "Returns:") || !strings.Contains(spec.IndividualTool.Description, "See also:") {
		t.Fatalf("Description = %q, want Returns/See also guidance", spec.IndividualTool.Description)
	}

	result, err := spec.Route.Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("Route.Handler: %v", err)
	}
	out, ok := result.(GetOutput)
	if !ok {
		t.Fatalf("Route.Handler result = %T, want GetOutput", result)
	}
	if out.Version != "17.0.0" || !out.Enterprise || !out.KAS.Enabled {
		t.Errorf("unexpected route output: %+v", out)
	}
}
