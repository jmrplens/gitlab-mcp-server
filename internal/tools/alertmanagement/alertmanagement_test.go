// alertmanagement_test.go contains unit tests for the alert management MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package alertmanagement

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const errExpectedErr = "expected error"

const fmtUnexpErr = "unexpected error: %v"

const testFilename = "test.png"

const errMissingAlertIID = "expected error for missing alert_iid"

// TestListMetricImages verifies the behavior of list metric images.
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

// TestListMetricImages_Error verifies the behavior of list metric images error.
func TestListMetricImages_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{ProjectID: "1", AlertIID: 5})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateMetricImage verifies the behavior of update metric image.
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

// TestUpdateMetricImage_Error verifies the behavior of update metric image error.
func TestUpdateMetricImage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUploadMetricImage verifies the behavior of upload metric image.
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

// TestUploadMetricImage_Error verifies the behavior of upload metric image error.
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

// TestDeleteMetricImage verifies the behavior of delete metric image.
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

// TestDeleteMetricImage_Error verifies the behavior of delete metric image error.
func TestDeleteMetricImage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	err := DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListMetricImagesOutput{Images: []MetricImageItem{{ID: 1, Filename: "img.png", URL: "https://example.com"}}}
	md := FormatListMarkdown(out)
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestFormatImageMarkdown verifies the behavior of format image markdown.
func TestFormatImageMarkdown(t *testing.T) {
	md := FormatImageMarkdown(MetricImageItem{ID: 1, Filename: testFilename})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// TestListMetricImages_MissingAlertIID verifies the behavior of list metric images missing alert i i d.
func TestListMetricImages_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{ProjectID: "1", AlertIID: 0})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestUpdateMetricImage_MissingAlertIID verifies the behavior of update metric image missing alert i i d.
func TestUpdateMetricImage_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 0, ImageID: 10})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestUpdateMetricImage_MissingImageID verifies the behavior of update metric image missing image i d.
func TestUpdateMetricImage_MissingImageID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 0})
	if err == nil {
		t.Fatal("expected error for missing image_id")
	}
}

// TestUploadMetricImage_MissingAlertIID verifies the behavior of upload metric image missing alert i i d.
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

// TestDeleteMetricImage_MissingAlertIID verifies the behavior of delete metric image missing alert i i d.
func TestDeleteMetricImage_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 0, ImageID: 10})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestDeleteMetricImage_MissingImageID verifies the behavior of delete metric image missing image i d.
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

// TestListMetricImages_WithPagination verifies the behavior of cov list metric images with pagination.
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

// TestUploadMetricImage_WithOptionalFields verifies the behavior of cov upload metric image with optional fields.
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

// TestFormatListMarkdown_Empty verifies the behavior of cov format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListMetricImagesOutput{})
	if !strings.Contains(md, "No metric images found") {
		t.Errorf("expected empty-state message, got:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterTools — MCP round-trip for all 4 tools (success)
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates cov register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := covNewAlertMgmtMCPSession(t)
	ctx := context.Background()

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
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — MCP round-trip for all 4 tools (error paths)
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCPError validates cov register tools call all through m c p error across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCPError(t *testing.T) {
	session := covNewAlertMgmtErrorMCPSession(t)
	ctx := context.Background()

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
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error: %v", tt.tool, err)
			}
			if !result.IsError {
				t.Fatalf("CallTool(%s) expected IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — MCP round-trip for all 4 actions
// ---------------------------------------------------------------------------.

// TestRegisterMeta_CallAllThroughMCP validates cov register meta call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterMeta_CallAllThroughMCP(t *testing.T) {
	session := covNewAlertMgmtMetaMCPSession(t)
	ctx := context.Background()

	actions := []struct {
		name   string
		action string
		params map[string]any
	}{
		{"list_metric_images", "list_metric_images", map[string]any{"project_id": "1", "alert_iid": 5}},
		{"upload_metric_image", "upload_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "content_base64": "ZGF0YQ==", "filename": "img.png"}},
		{"update_metric_image", "update_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "image_id": 1}},
		{"delete_metric_image", "delete_metric_image", map[string]any{"project_id": "1", "alert_iid": 5, "image_id": 1}},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "gitlab_alert_management",
				Arguments: map[string]any{
					"action": tt.action,
					"params": tt.params,
				},
			})
			if err != nil {
				t.Fatalf("CallTool(action=%s) error: %v", tt.action, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(action=%s) returned error: %s", tt.action, tc.Text)
					}
				}
				t.Fatalf("CallTool(action=%s) returned IsError=true", tt.action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterTools (success)
// ---------------------------------------------------------------------------.

// covNewAlertMgmtMCPSession is an internal helper for the alertmanagement package.
func covNewAlertMgmtMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

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

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterTools (error paths)
// ---------------------------------------------------------------------------.

// covNewAlertMgmtErrorMCPSession is an internal helper for the alertmanagement package.
func covNewAlertMgmtErrorMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
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

// ---------------------------------------------------------------------------
// Helper: MCP session for RegisterMeta
// ---------------------------------------------------------------------------.

// covNewAlertMgmtMetaMCPSession is an internal helper for the alertmanagement package.
func covNewAlertMgmtMetaMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

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

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)

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

// TestRegisterTools_DeleteConfirmDeclined covers the ConfirmAction early-return
// branch in the alert metric image delete handler when the user declines.
func TestRegisterTools_DeleteConfirmDeclined(t *testing.T) {
client := testutil.NewTestClient(t, http.NewServeMux())
server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
RegisterTools(server, client)

st, ct := mcp.NewInMemoryTransports()
ctx := context.Background()
if _, err := server.Connect(ctx, st, nil); err != nil {
t.Fatalf("server connect: %v", err)
}
mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
return &mcp.ElicitResult{Action: "decline"}, nil
},
})
session, err := mcpClient.Connect(ctx, ct, nil)
if err != nil {
t.Fatalf("client connect: %v", err)
}
t.Cleanup(func() { session.Close() })

result, err := session.CallTool(ctx, &mcp.CallToolParams{
Name:      "gitlab_delete_alert_metric_image",
Arguments: map[string]any{"project_id": "42", "alert_iid": 1, "image_id": 1},
})
if err != nil {
t.Fatalf("CallTool error: %v", err)
}
if result == nil {
t.Fatal("expected non-nil result for declined confirmation")
}
}
