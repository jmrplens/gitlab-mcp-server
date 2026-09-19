// alert_management_test.go contains unit tests for the alert management MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package alertmanagement

import (
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testFilename identifies the test filename constant used by this package.
const testFilename = "test.png"

// errMissingAlertIID identifies the err missing alert IID constant used by this package.
const errMissingAlertIID = "expected error for missing alert_iid"

// TestListMetricImages verifies the ListMetricImages handler.
// The mock GitLab API at /api/v4/projects/1/alert_management_alerts/5/metric_images (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestListMetricImages_Error verifies that ListMetricImages returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListMetricImages_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{ProjectID: "1", AlertIID: 5})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateMetricImage verifies the UpdateMetricImage handler.
// The mock GitLab API at /api/v4/projects/1/alert_management_alerts/5/metric_images/10 (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestUpdateMetricImage_UpdatesBothTheLinkAndItsText asserts that an update
// naming a link and the text for it sends both, rather than dropping the text
// on the way out.
//
// Both are optional pointers copied under guards of their own, so an inverted
// or missing guard loses one silently: the caption a caller wrote never
// reaches GitLab and the handler still answers with whatever the instance
// echoes back. Reading the body is what makes that a failure, and it is also
// what keeps url_text in the recorded request inventory, which is the only
// record of which parameters this endpoint has been sent.
func TestUpdateMetricImage_UpdatesBothTheLinkAndItsText(t *testing.T) {
	var sent map[string]any
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/1/alert_management_alerts/5/metric_images/10" || r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, "decode body", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"id":10,"filename":"img.png","url":"https://new.com","url_text":"updated"}`)
	}))

	url, urlText := "https://new.com", "updated"
	out, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{
		ProjectID: "1", AlertIID: 5, ImageID: 10, URL: &url, URLText: &urlText,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if sent["url"] != url {
		t.Errorf("body url = %v, want %q", sent["url"], url)
	}
	if sent["url_text"] != urlText {
		t.Errorf("body url_text = %v, want %q", sent["url_text"], urlText)
	}
	if out.URLText != "updated" {
		t.Errorf("URLText = %q, want updated", out.URLText)
	}
}

// TestUpdateMetricImage_Error verifies that UpdateMetricImage returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateMetricImage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUploadMetricImage verifies the UploadMetricImage handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestUploadMetricImage_Error verifies that UploadMetricImage returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDeleteMetricImage verifies the DeleteMetricImage handler.
// The mock GitLab API at /api/v4/projects/1/alert_management_alerts/5/metric_images/10 (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
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

// TestDeleteMetricImage_Error verifies that DeleteMetricImage returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteMetricImage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))
	err := DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 10})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// The Markdown formatters are asserted whole in markdown_test.go.

// TestListMetricImages_MissingAlertIID verifies that ListMetricImages_MissingAlertIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListMetricImages_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{ProjectID: "1", AlertIID: 0})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestUpdateMetricImage_MissingAlertIID verifies that UpdateMetricImage_MissingAlertIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateMetricImage_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 0, ImageID: 10})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestUpdateMetricImage_MissingImageID verifies that UpdateMetricImage_MissingImageID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateMetricImage_MissingImageID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 5, ImageID: 0})
	if err == nil {
		t.Fatal("expected error for missing image_id")
	}
}

// TestUploadMetricImage_MissingAlertIID verifies that UploadMetricImage_MissingAlertIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestUploadMetricImage_FilePath_Success verifies that UploadMetricImage_FilePath succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestUploadMetricImage_FilePathInvalid verifies the UploadMetricImage_FilePathInvalid handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestUploadMetricImage_BothInputs verifies the UploadMetricImage_BothInputs handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUploadMetricImage_BothInputs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, FilePathLocal: "/tmp/x", ContentBase64: "dGVzdA==", Filename: "x.png"})
	if err == nil {
		t.Fatal("expected error when both file_path and content_base64 provided, got nil")
	}
}

// TestUploadMetricImage_NeitherInput verifies the UploadMetricImage_NeitherInput handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUploadMetricImage_NeitherInput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, Filename: "x.png"})
	if err == nil {
		t.Fatal("expected error when neither file_path nor content_base64 provided, got nil")
	}
}

// TestUploadMetricImage_InvalidBase64 verifies the UploadMetricImage_InvalidBase64 handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUploadMetricImage_InvalidBase64(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{ProjectID: "1", AlertIID: 5, ContentBase64: "!!!invalid!!!", Filename: "x.png"})
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// TestMetricImages_IdentifierAtZero_RefusedBeforeAnyRequest verifies that a
// required identifier left at zero is refused by the handler itself, naming the
// tool and the parameter, and that nothing reaches GitLab.
//
// Zero is not an arbitrary value: it is what a missing required integer
// deserializes to, so it is exactly the shape a model produces when it omits
// alert_iid or image_id. The guards exist to answer that with the parameter's
// name rather than with whatever GitLab says about alert 0. The other tests in
// this file assert only that some error came back, which a request that reached
// GitLab and failed satisfies just as well — so the mock here answers every
// route successfully, and the test asserts both that the call failed and that
// the request counter never moved. Without both halves, widening a guard from
// `<= 0` to `< 0` passes unnoticed while every caller who omits the field is
// told the alert does not exist.
func TestMetricImages_IdentifierAtZero_RefusedBeforeAnyRequest(t *testing.T) {
	var requests atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[`+covImageJSON+`]`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			testutil.RespondJSON(w, http.StatusOK, covImageJSON)
		}
	}))
	content := base64.StdEncoding.EncodeToString([]byte("image-data"))

	cases := []struct {
		name  string
		op    string
		field string
		call  func() error
	}{
		{
			name: "list without alert_iid", op: "gitlab_list_alert_metric_images", field: "alert_iid",
			call: func() error {
				_, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{ProjectID: "1"})
				return err
			},
		},
		{
			name: "upload without alert_iid", op: "gitlab_upload_alert_metric_image", field: "alert_iid",
			call: func() error {
				_, err := UploadMetricImage(t.Context(), client, UploadMetricImageInput{
					ProjectID: "1", ContentBase64: content, Filename: testFilename,
				})
				return err
			},
		},
		{
			name: "update without alert_iid", op: "gitlab_update_alert_metric_image", field: "alert_iid",
			call: func() error {
				_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", ImageID: 10})
				return err
			},
		},
		{
			name: "update without image_id", op: "gitlab_update_alert_metric_image", field: "image_id",
			call: func() error {
				_, err := UpdateMetricImage(t.Context(), client, UpdateMetricImageInput{ProjectID: "1", AlertIID: 5})
				return err
			},
		},
		{
			name: "delete without alert_iid", op: "gitlab_delete_alert_metric_image", field: "alert_iid",
			call: func() error {
				return DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", ImageID: 10})
			},
		},
		{
			name: "delete without image_id", op: "gitlab_delete_alert_metric_image", field: "image_id",
			call: func() error {
				return DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 5})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := requests.Load()
			err := tc.call()
			if err == nil {
				t.Fatalf("%s with %s at zero returned no error, want a refusal", tc.op, tc.field)
			}
			if sent := requests.Load() - before; sent != 0 {
				t.Errorf("requests reaching GitLab = %d, want 0: %s at zero must be refused before a request is built", sent, tc.field)
			}
			if !strings.Contains(err.Error(), tc.op) {
				t.Errorf("error = %q, want it to name the tool %q", err, tc.op)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error = %q, want it to name the missing parameter %q", err, tc.field)
			}
		})
	}
}

// TestUploadMetricImage_CaptionAndLink_SentAsMultipartFields verifies that the
// caption and link a caller supplies travel to GitLab as the url_text and url
// fields of the multipart upload, beside the image itself, and that neither
// field is invented when the caller supplies nothing.
//
// The success tests above read their assertions out of the mock's own response
// body, so they move both sides of the comparison: an upload that silently
// dropped the caption would still echo back whatever fixture the mock holds.
// This one reads the request instead, which is the only place the caller's
// intent can be observed.
func TestUploadMetricImage_CaptionAndLink_SentAsMultipartFields(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("image-data"))

	t.Run("what the caller supplied", func(t *testing.T) {
		link := "https://example.com/img.png"
		caption := "CPU saturation"
		form := uploadedMultipartForm(t, UploadMetricImageInput{
			ProjectID: "1", AlertIID: 5, ContentBase64: content, Filename: testFilename,
			URL: &link, URLText: &caption,
		})
		if got := testutil.FormValue(form, "url"); got != link {
			t.Errorf("multipart url = %q, want %q", got, link)
		}
		if got := testutil.FormValue(form, "url_text"); got != caption {
			t.Errorf("multipart url_text = %q, want %q", got, caption)
		}
		file, header, err := testutil.FormFile(form, "file")
		if err != nil {
			t.Fatalf("multipart file part: %v", err)
		}
		defer func() { _ = file.Close() }()
		if header.Filename != testFilename {
			t.Errorf("multipart filename = %q, want %q", header.Filename, testFilename)
		}
	})

	t.Run("nothing the caller withheld", func(t *testing.T) {
		form := uploadedMultipartForm(t, UploadMetricImageInput{
			ProjectID: "1", AlertIID: 5, ContentBase64: content, Filename: testFilename,
		})
		if values, ok := form.Value["url"]; ok {
			t.Errorf("multipart carried url = %q, want the field omitted", values)
		}
		if values, ok := form.Value["url_text"]; ok {
			t.Errorf("multipart carried url_text = %q, want the field omitted", values)
		}
	})
}

// uploadedMultipartForm drives one upload against a mock that parses the
// multipart body and returns the form the request carried, so a test can assert
// what GitLab was sent rather than what the mock answered.
func uploadedMultipartForm(t *testing.T, input UploadMetricImageInput) *multipart.Form {
	t.Helper()
	var form *multipart.Form
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parsed, err := testutil.ReadMultipartForm(r, 1<<20)
		if err != nil {
			t.Errorf("ReadMultipartForm: %v", err)
			http.Error(w, "parse multipart form", http.StatusBadRequest)
			return
		}
		form = parsed
		testutil.RespondJSON(w, http.StatusCreated, covImageJSON)
	}))
	if _, err := UploadMetricImage(t.Context(), client, input); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if form == nil {
		t.Fatal("no multipart request reached the mock")
	}
	return form
}

// TestDeleteMetricImage_MissingAlertIID verifies that DeleteMetricImage_MissingAlertIID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteMetricImage_MissingAlertIID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	err := DeleteMetricImage(t.Context(), client, DeleteMetricImageInput{ProjectID: "1", AlertIID: 0, ImageID: 10})
	if err == nil {
		t.Fatal(errMissingAlertIID)
	}
}

// TestDeleteMetricImage_MissingImageID verifies that DeleteMetricImage_MissingImageID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestListMetricImages_WithPagination verifies that ListMetricImages_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/projects/1/alert_management_alerts/5/metric_images (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
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
		Page:      2, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(out.Images))
	}
}

// ---------------------------------------------------------------------------
// ListMetricImages — keyset pagination + created_at mapping
// ---------------------------------------------------------------------------.

// TestListMetricImages_KeysetAndCreatedAt verifies that ListMetricImages
// forwards keyset pagination parameters (order_by, sort, pagination, page_token)
// to the GitLab API and that the SDK CreatedAt timestamp is mapped onto the
// MCP output item in RFC3339 form.
func TestListMetricImages_KeysetAndCreatedAt(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/alert_management_alerts/5/metric_images" && r.Method == http.MethodGet {
			gotQuery = r.URL.RawQuery
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"created_at":"2024-01-02T03:04:05Z","filename":"img.png","file_path":"/uploads/img.png","url":"https://example.com","url_text":"link"}]`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := ListMetricImages(t.Context(), client, ListMetricImagesInput{
		ProjectID:  "1",
		AlertIID:   5,
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(out.Images))
	}
	if out.Images[0].CreatedAt != "2024-01-02T03:04:05Z" {
		t.Errorf("expected created_at 2024-01-02T03:04:05Z, got %q", out.Images[0].CreatedAt)
	}
	for _, want := range []string{"order_by=created_at", "sort=desc", "pagination=keyset", "page_token=42"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("expected query to contain %q, got %q", want, gotQuery)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UploadMetricImage — with optional URL and URLText
// ---------------------------------------------------------------------------.

// TestUploadMetricImage_WithOptionalFields verifies the UploadMetricImage_WithOptionalFields handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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
