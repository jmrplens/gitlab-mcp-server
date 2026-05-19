// alertmanagement_test.go contains unit tests for the alert management MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package alertmanagement

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testFilename identifies the test filename constant used by this package.
const testFilename = "test.png"

// errMissingAlertIID identifies the err missing alert IID constant used by this package.
const errMissingAlertIID = "expected error for missing alert_iid"

// TestListMetricImages verifies ListMetricImages.
func TestListMetricImages(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/alert_management_alerts/5/metric_images" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"filename":"img.png","file_path":"/uploads/img.png","url":"https://example.com","url_text":"link"}]`)
	}))
	out, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{ProjectID: "1", AlertIID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(out.Images))
	}
	if out.Images[0].Filename != "img.png" {
		t.Errorf("expected img.png, got %s", out.Images[0].Filename)
	}
}

// TestListMetricImages_Error verifies ListMetricImages when error.
func TestListMetricImages_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{ProjectID: "1", AlertIID: 5})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateMetricImage verifies UpdateMetricImage.
func TestUpdateMetricImage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/alert_management_alerts/5/metric_images/10" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"filename":"img.png","url":"https://new.com","url_text":"updated"}`)
	}))
	url := "https://new.com"
	out, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 10, URL: &url})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.URL != "https://new.com" {
		t.Errorf("expected https://new.com, got %s", out.URL)
	}
}

// TestUpdateMetricImage_Error verifies UpdateMetricImage when error.
func TestUpdateMetricImage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUploadMetricImage verifies UploadMetricImage.
func TestUploadMetricImage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":20,"filename":"test.png","url":"https://uploaded.com"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("image-data"))
	out, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, ContentBase64: content, Filename: testFilename})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 20 {
		t.Errorf("expected ID 20, got %d", out.ID)
	}
}

// TestUploadMetricImage_Error verifies UploadMetricImage when error.
func TestUploadMetricImage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, ContentBase64: content, Filename: testFilename})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestDeleteMetricImage verifies DeleteMetricImage.
func TestDeleteMetricImage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/alert_management_alerts/5/metric_images/10" || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteMetricImage_Error verifies DeleteMetricImage when error.
func TestDeleteMetricImage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	err := DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListMetricImagesOutput{Images: []MetricImageItem{{ID: 1, Filename: "img.png", URL: "https://example.com"}}}
	md := FormatListMarkdown(out)
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatImageMarkdown verifies FormatImageMarkdown.
func TestFormatImageMarkdown(t *testing.T) {
	md := FormatImageMarkdown(MetricImageItem{ID: 1, Filename: testFilename})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestListMetricImages_MissingAlertIID verifies ListMetricImages when missing alert IID.
func TestListMetricImages_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{ProjectID: "1", AlertIID: 0})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestUpdateMetricImage_MissingAlertIID verifies UpdateMetricImage when missing alert IID.
func TestUpdateMetricImage_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 0, ImageID: 10})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestUpdateMetricImage_MissingImageID verifies UpdateMetricImage when missing image ID.
func TestUpdateMetricImage_MissingImageID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 0})
	if err == nil {
		t.Fatal("expected error for missing image_id")
	}
}

// TestUploadMetricImage_MissingAlertIID verifies UploadMetricImage when missing alert IID.
func TestUploadMetricImage_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 0, ContentBase64: content, Filename: testFilename})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestUploadMetricImage_FilePath_Success verifies upload with file_path.
func TestUploadMetricImage_FilePath_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"id":30,"filename":"metric.png","url":"https://uploaded.com"}`)
	}))
	tmpFile := t.TempDir() + "/metric.png"
	if err := os.WriteFile(tmpFile, []byte("fake-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, FilePathLocal: tmpFile, Filename: "metric.png"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 30 {
		t.Errorf("expected ID 30, got %d", out.ID)
	}
}

// TestUploadMetricImage_FilePathInvalid verifies local file validation errors.
func TestUploadMetricImage_FilePathInvalid(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{
		ProjectID:     "1",
		AlertIID:      5,
		FilePathLocal: t.TempDir() + "/missing.png",
		Filename:      "missing.png",
	})
	if err == nil {
		t.Fatal("expected file validation error")
	}
	if !strings.Contains(err.Error(), "gitlab_upload_alert_metric_image") {
		t.Fatalf("error = %v, want upload context", err)
	}
}

// TestUploadMetricImage_BothInputs verifies error when both file_path and content_base64 provided.
func TestUploadMetricImage_BothInputs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, FilePathLocal: "/tmp/x", ContentBase64: "dGVzdA==", Filename: "x.png"})
	if err == nil {
		t.Fatal("expected error when both file_path and content_base64 provided, got nil")
	}
}

// TestUploadMetricImage_NeitherInput verifies error when neither input provided.
func TestUploadMetricImage_NeitherInput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, Filename: "x.png"})
	if err == nil {
		t.Fatal("expected error when neither file_path nor content_base64 provided, got nil")
	}
}

// TestUploadMetricImage_InvalidBase64 verifies error for invalid base64.
func TestUploadMetricImage_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, ContentBase64: "!!!invalid!!!", Filename: "x.png"})
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// TestDeleteMetricImage_MissingAlertIID verifies DeleteMetricImage when missing alert IID.
func TestDeleteMetricImage_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 0, ImageID: 10})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestDeleteMetricImage_MissingImageID verifies DeleteMetricImage when missing image ID.
func TestDeleteMetricImage_MissingImageID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 0})
	if err == nil {
		t.Fatal("expected error for missing image_id")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// covImageJSON is a reusable JSON fixture for a single metric image.
const covImageJSON = `{"id":1,"filename":"img.png","file_path":"/uploads/img.png","url":"https://example.com","url_text":"link"}`

// ---------------------------------------------------------------------------
// ListMetricImages — with pagination params
// ---------------------------------------------------------------------------.

// TestListMetricImages_WithPagination verifies ListMetricImages when with pagination.
func TestListMetricImages_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/alert_management_alerts/5/metric_images" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[`+covImageJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{
		ProjectID: "1",
		AlertIID:  5,
		Page:      2,
		PerPage:   10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(out.Images))
	}
}

// ---------------------------------------------------------------------------
// UploadMetricImage — with optional URL and URLText
// ---------------------------------------------------------------------------.

// TestUploadMetricImage_WithOptionalFields verifies UploadMetricImage when with optional fields.
func TestUploadMetricImage_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, covImageJSON)
			return
		}
		http.NotFound(w, r)
	}))
	covURL := "https://example.com"
	covURLText := "link"
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	out, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{
		ProjectID:     "1",
		AlertIID:      5,
		ContentBase64: content,
		Filename:      "img.png",
		URL:           &covURL,
		URLText:       &covURLText,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", out.URL)
	}
	if out.URLText != "link" {
		t.Errorf("expected URLText link, got %s", out.URLText)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — empty images
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListMetricImagesOutput{})
	if !strings.Contains(md, "No metric images found") {
		t.Errorf("expected empty-state message, got:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestActionSpecs_Metadata verifies alert management action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "alertmanagement" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes validates all alert management canonical routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	specByTool := covAlertMgmtSpecsByTool(t, covAlertMgmtHandler())

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_alert_metric_images", map[string]any{"project_id": "1", "alert_iid": 5, "page": 0, "per_page": 0}},
		{"upload", "gitlab_upload_alert_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "content_base64": "ZGF0YQ==", "filename": "img.png", "url": "https://example.com", "url_text": "link"}},
		{"update", "gitlab_update_alert_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "image_id": 1, "url": "https://example.com", "url_text": "link"}},
		{"delete", "gitlab_delete_alert_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "image_id": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution error paths
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRouteErrors validates canonical route error paths.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	specByTool := covAlertMgmtSpecsByTool(t, covAlertMgmtErrorHandler())

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_list_alert_metric_images", map[string]any{"project_id": "1", "alert_iid": 5, "page": 0, "per_page": 0}},
		{"upload", "gitlab_upload_alert_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "content_base64": "ZGF0YQ==", "filename": "img.png", "url": "https://example.com", "url_text": "link"}},
		{"update", "gitlab_update_alert_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "image_id": 1, "url": "https://example.com", "url_text": "link"}},
		{"delete", "gitlab_delete_alert_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "image_id": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			if _, err := spec.Route.Handler(t.Context(), tt.args); err == nil {
				t.Fatalf("Route.Handler(%s) expected error", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// covAlertMgmtHandler supports cov alert mgmt handler assertions in alertmanagement tests.
func covAlertMgmtHandler() http.Handler {
	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/alert_management_alerts/5/metric_images", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covImageJSON+`]`)
	})

	handler.HandleFunc("POST /api/v4/projects/1/alert_management_alerts/5/metric_images", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covImageJSON)
	})

	handler.HandleFunc("PUT /api/v4/projects/1/alert_management_alerts/5/metric_images/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covImageJSON)
	})

	handler.HandleFunc("DELETE /api/v4/projects/1/alert_management_alerts/5/metric_images/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return handler
}

// covAlertMgmtErrorHandler supports cov alert mgmt error handler assertions in alertmanagement tests.
func covAlertMgmtErrorHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	})

	return handler
}

// covAlertMgmtSpecsByTool supports cov alert mgmt specs by tool assertions in alertmanagement tests.
func covAlertMgmtSpecsByTool(t *testing.T, handler http.Handler) map[string]toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}
	return specByTool
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.
