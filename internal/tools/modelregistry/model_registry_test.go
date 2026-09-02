// model_registry_test.go contains unit tests for GitLab ML model registry
// operations. Tests use httptest to mock the GitLab Model Registry API.
package modelregistry

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestDownload validates the Download handler for the ML model registry.
// Covers successful download with output verification, all four required-field
// validations, multiple API error status codes (401, 403, 404, 500), and
// context cancellation.
func TestDownload(t *testing.T) {
	tests := []downloadCase{
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
			runDownloadCase(t, tt)
		})
	}
}

type downloadCase struct {
	name       string
	input      DownloadInput
	handler    http.HandlerFunc
	cancelCtx  bool
	wantErr    bool
	errContain string
	validate   func(t *testing.T, out DownloadOutput)
}

func runDownloadCase(t *testing.T, tt downloadCase) {
	t.Helper()
	client := testutil.NewTestClient(t, tt.handler)
	got, err := Download(downloadCaseContext(tt), client, tt.input)
	assertDownloadCaseResult(t, got, err, tt)
}

func downloadCaseContext(tt downloadCase) context.Context {
	ctx := context.Background()
	if tt.cancelCtx {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	}
	return ctx
}

func assertDownloadCaseResult(t *testing.T, got DownloadOutput, err error, tt downloadCase) {
	t.Helper()
	if (err != nil) != tt.wantErr {
		t.Fatalf("Download() error = %v, wantErr %v", err, tt.wantErr)
	}
	if tt.errContain != "" && err != nil && !strings.Contains(err.Error(), tt.errContain) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContain)
	}
	if tt.validate != nil {
		tt.validate(t, got)
	}
}

// TestDownloadOutput_ReadError verifies downloadOutput returns reader failures
// instead of producing partial base64 content.
//
// The test injects a reader that always fails and expects a non-nil error,
// protecting ML model downloads from silently accepting corrupted file streams.
func TestDownloadOutput_ReadError(t *testing.T) {
	_, err := downloadOutput(DownloadInput{
		ProjectID:      toolutil.StringOrInt("42"),
		ModelVersionID: toolutil.StringOrInt("7"),
		Path:           "models",
		Filename:       "model.bin",
	}, failingReader{})
	if err == nil {
		t.Fatal("downloadOutput() error = nil, want read error")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

// TestDownloadOutput_FileOverTheCeiling_IsRefusedNotTruncated verifies that the
// file this action turns into base64 and copies into a JSON-RPC message is
// bounded, and that a file above the bound produces an error naming the way out
// rather than a partial file.
//
// The read was an unbounded io.ReadAll. The client-wide response ceiling bounds
// what it costs to read and not what the answer becomes, and a caller can start
// as many downloads as it likes. A prefix of a .safetensors loads no better
// than a prefix of a .tar.gz imports, so the refusal is the whole answer here.
// The exactly-at-the-ceiling case is present because an off-by-one in the limit
// reader would refuse a file that fits.
func TestDownloadOutput_FileOverTheCeiling_IsRefusedNotTruncated(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "small file is returned whole", size: 1024},
		{name: "file exactly at the ceiling is returned whole", size: maxModelFileBytes},
		{name: "file one byte over the ceiling is refused", size: maxModelFileBytes + 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			}
			out, err := downloadOutput(in, bytes.NewReader(make([]byte, tt.size)))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("downloadOutput(%d bytes) error = nil, want a refusal", tt.size)
				}
				if !strings.Contains(err.Error(), "download it from GitLab directly") {
					t.Errorf("downloadOutput(%d bytes) error = %v, want it to name the way out", tt.size, err)
				}
				// A refusal must not double as a partial answer: content that
				// looks like a whole file is worse than no content.
				if out.ContentBase64 != "" {
					t.Errorf("ContentBase64 length = %d, want no content alongside the refusal", len(out.ContentBase64))
				}
				return
			}
			if err != nil {
				t.Fatalf("downloadOutput(%d bytes) error = %v", tt.size, err)
			}
			if out.SizeBytes != tt.size {
				t.Errorf("SizeBytes = %d, want %d", out.SizeBytes, tt.size)
			}
		})
	}
}

// TestDownload_ResponseOverTheClientCeiling_NamesTheWayOut verifies that a file
// the client-wide response ceiling refuses to read produces the same actionable
// message as one this action refuses to encode.
//
// The SDK buffers the whole body during the call, so that ceiling fails the
// call rather than the later read, and it arrives as a transport error saying
// nothing about model files. An operator whose weights file is simply too big
// to travel this way otherwise sees only that something exceeded a maximum.
func TestDownload_ResponseOverTheClientCeiling_NamesTheWayOut(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 4096))
	})
	client := testutil.NewTestClient(t, handler)
	client.SetMaxResponseBytes(1024)

	_, err := Download(t.Context(), client, DownloadInput{
		ProjectID:      toolutil.StringOrInt("42"),
		ModelVersionID: toolutil.StringOrInt("7"),
		Path:           "models",
		Filename:       "model.bin",
	})
	if err == nil {
		t.Fatal("Download() error = nil, want an error for a response over the client ceiling")
	}
	if !strings.Contains(err.Error(), "download it from GitLab directly") {
		t.Errorf("Download() error = %v, want it to name the way out", err)
	}
}
