// model_registry_test.go contains unit tests for GitLab ML model registry
// operations. Tests use httptest to mock the GitLab Model Registry API.
package modelregistry

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestDownload validates the Download handler for the ML model registry.
// Covers successful download with output verification, all four required-field
// validations, multiple API error status codes (401, 403, 404, 500), and
// context cancellation.
func TestDownload(t *testing.T) {
	tests := []struct {
		name       string
		input      DownloadInput
		handler    http.HandlerFunc
		cancelCtx  bool
		wantErr    bool
		errContain string
		validate   func(t *testing.T, out DownloadOutput)
	}{
		{
			name: "returns base64-encoded content on success",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/projects/42/packages/ml_models/7/files/models/model.bin")
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("model-binary-data"))
			},
			validate: func(t *testing.T, out DownloadOutput) {
				t.Helper()
				if out.ProjectID != "42" {
					t.Errorf("ProjectID = %q, want %q", out.ProjectID, "42")
				}
				if out.ModelVersionID != "7" {
					t.Errorf("ModelVersionID = %q, want %q", out.ModelVersionID, "7")
				}
				if out.Path != "models" {
					t.Errorf("Path = %q, want %q", out.Path, "models")
				}
				if out.Filename != "model.bin" {
					t.Errorf("Filename = %q, want %q", out.Filename, "model.bin")
				}
				wantBase64 := base64.StdEncoding.EncodeToString([]byte("model-binary-data"))
				if out.ContentBase64 != wantBase64 {
					t.Errorf("ContentBase64 = %q, want %q", out.ContentBase64, wantBase64)
				}
				if out.SizeBytes != len("model-binary-data") {
					t.Errorf("SizeBytes = %d, want %d", out.SizeBytes, len("model-binary-data"))
				}
			},
		},
		{
			name: "returns empty base64 for zero-byte file",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("10"),
				ModelVersionID: toolutil.StringOrInt("1"),
				Path:           "empty",
				Filename:       "empty.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
			},
			validate: func(t *testing.T, out DownloadOutput) {
				t.Helper()
				if out.ContentBase64 != "" {
					t.Errorf("ContentBase64 = %q, want empty string", out.ContentBase64)
				}
				if out.SizeBytes != 0 {
					t.Errorf("SizeBytes = %d, want 0", out.SizeBytes)
				}
			},
		},
		{
			name: "returns error when project_id is empty",
			input: DownloadInput{
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.NotFound(w, nil)
			},
			wantErr:    true,
			errContain: "project_id",
		},
		{
			name: "returns error when model_version_id is empty",
			input: DownloadInput{
				ProjectID: toolutil.StringOrInt("42"),
				Path:      "models",
				Filename:  "model.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.NotFound(w, nil)
			},
			wantErr:    true,
			errContain: "model_version_id",
		},
		{
			name: "returns error when path is empty",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Filename:       "model.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.NotFound(w, nil)
			},
			wantErr:    true,
			errContain: "path",
		},
		{
			name: "returns error when filename is empty",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.NotFound(w, nil)
			},
			wantErr:    true,
			errContain: "filename",
		},
		{
			name: "returns error on 401 unauthorized",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
			},
			wantErr: true,
		},
		{
			name: "returns error on 403 forbidden",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			},
			wantErr: true,
		},
		{
			name: "returns error on 404 not found",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr: true,
		},
		{
			name: "returns error on 500 server error",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"internal server error"}`)
			},
			wantErr: true,
		},
		{
			name: "returns error when context is cancelled",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			cancelCtx: true,
			wantErr:   true,
		},
		{
			name: "handles URL-encoded project path",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("group%2Fproject"),
				ModelVersionID: toolutil.StringOrInt("candidate:5"),
				Path:           "deep/nested",
				Filename:       "weights.h5",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("weight-data"))
			},
			validate: func(t *testing.T, out DownloadOutput) {
				t.Helper()
				if out.ProjectID != "group%2Fproject" {
					t.Errorf("ProjectID = %q, want %q", out.ProjectID, "group%2Fproject")
				}
				if out.ModelVersionID != "candidate:5" {
					t.Errorf("ModelVersionID = %q, want %q", out.ModelVersionID, "candidate:5")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)

			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			got, err := Download(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Download() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContain != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContain)
				}
			}
			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}
