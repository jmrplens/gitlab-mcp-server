// model_registry_test.go contains unit tests for GitLab ML model registry
// operations. Tests use httptest to mock the GitLab Model Registry API.
package modelregistry

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestDownload validates the Download handler for the ML model registry.
// Covers successful download with output verification, all four required-field
// validations, multiple API error status codes (401, 403, 404, 500), context
// cancellation, and a nested path with a percent-encoded project.
//
// Every case that asserts a refusal names the layer that refused. The four
// required-field cases and the cancelled one run against
// [testutil.ForbiddenHandler], so they fail if any request is issued at all,
// and each pins the exact error the guard returns: with the mock answering 404
// instead, all four passed on a word the 404 hint happens to carry
// ("verify project_id, model_version_id, path, and filename"), so every one of
// them stayed green with its guard deleted from the handler.
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
			name: "empty project_id is refused before any request",
			input: DownloadInput{
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			forbidRequest: true,
			wantErr:       true,
			errContain:    "project_id is required",
		},
		{
			name: "empty model_version_id is refused before any request",
			input: DownloadInput{
				ProjectID: toolutil.StringOrInt("42"),
				Path:      "models",
				Filename:  "model.bin",
			},
			forbidRequest: true,
			wantErr:       true,
			errContain:    "model_version_id is required",
		},
		{
			name: "empty path is refused before any request",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Filename:       "model.bin",
			},
			forbidRequest: true,
			wantErr:       true,
			errContain:    "path is required",
		},
		{
			name: "empty filename is refused before any request",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
			},
			forbidRequest: true,
			wantErr:       true,
			errContain:    "filename is required",
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
			wantErr:   true,
			errPrefix: wantDownloadOp,
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
			wantErr:   true,
			errPrefix: wantDownloadOp,
			// The identifier hint belongs to 404 alone: a forbidden read is not
			// a mistyped path, and telling a caller to check their arguments
			// sends them to correct something that is already right.
			errOmit: wantIdentifierHint,
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
			wantErr:    true,
			errPrefix:  wantDownloadOp,
			errContain: wantIdentifierHint,
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
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"internal server error"}`)
			},
			wantErr:   true,
			errPrefix: wantDownloadOp,
			errOmit:   wantIdentifierHint,
		},
		{
			name: "cancelled context is refused before any request",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           "models",
				Filename:       "model.bin",
			},
			cancelCtx:     true,
			forbidRequest: true,
			wantErr:       true,
			// The context error itself, unwrapped: the SDK would also refuse a
			// cancelled context, but it would arrive wrapped in this action's
			// operation label, so identity is what separates the early return
			// from a request that was built and then abandoned.
			errExactly: context.Canceled,
		},
		{
			name: "a nested path stays one route segment",
			input: DownloadInput{
				ProjectID:      toolutil.StringOrInt("group%2Fproject"),
				ModelVersionID: toolutil.StringOrInt("candidate:5"),
				Path:           "deep/nested",
				Filename:       "weights.h5",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				// The escaped path, not r.URL.Path: the server decodes %2F back
				// into a slash, so the decoded form cannot tell a nested path
				// from two extra route segments, which is the whole question
				// here.
				if got := r.URL.EscapedPath(); got != wantNestedEscapedPath {
					t.Errorf("escaped URL path = %q, want %q", got, wantNestedEscapedPath)
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("weight-data"))
			},
			validate: func(t *testing.T, out DownloadOutput) {
				t.Helper()
				want := DownloadOutput{
					ProjectID:      "group%2Fproject",
					ModelVersionID: "candidate:5",
					Path:           "deep/nested",
					Filename:       "weights.h5",
					ContentBase64:  base64.StdEncoding.EncodeToString([]byte("weight-data")),
					SizeBytes:      len("weight-data"),
				}
				if !reflect.DeepEqual(out, want) {
					t.Errorf("Download() = %+v, want %+v", out, want)
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

// wantIdentifierHint is the hint this action attaches to a 404 and to no other
// status, so a case can assert both that it is there and that it is not.
const wantIdentifierHint = "verify project_id, model_version_id, path, and filename"

// wantNestedEscapedPath is the request line a nested path and a
// percent-encoded project produce: the slash inside path is escaped so the
// package path stays one route segment, the percent of an already-encoded
// project id is escaped again, and the colon of a candidate version and the dot
// of a file name are left alone.
const wantNestedEscapedPath = "/api/v4/projects/group%252Fproject/packages/ml_models/candidate:5/files/deep%2Fnested/weights.h5"

// wantDownloadOp is the label every GitLab-side failure of this action opens
// with, spelled out rather than read from opDownloadModelPackage: a label the
// assertion takes from the value it is judging can be changed on one side and
// still satisfy it.
const wantDownloadOp = "download ml model package: "

// wantOverCeilingRefusal is the whole refusal a file above the ceiling
// produces, for a fixture whose path is "models" and whose file name is
// "model.bin". It is spelled out for the reason [wantDownloadOp] is, and
// because the two names are what a crossing at the wrap site exchanges.
const wantOverCeilingRefusal = wantDownloadOp + "unexpected error. Suggestion: " +
	"the file is too large to return in one MCP response; download it from GitLab directly " +
	"(GET /projects/:id/packages/ml_models/:model_version_id/files/:path/:filename): " +
	"model.bin exceeds the 32 MiB this action returns"

type downloadCase struct {
	name  string
	input DownloadInput
	// handler answers the mock GitLab; forbidRequest replaces it with one that
	// fails the test on any request, for a case asserting that the handler
	// refuses before it reaches GitLab.
	handler       http.HandlerFunc
	forbidRequest bool
	cancelCtx     bool
	wantErr       bool
	// errPrefix is the operation label the refusal must open with. A case
	// refused before the request is built carries none, since those errors
	// belong to the guard rather than to the call.
	errPrefix  string
	errContain string
	errOmit    string
	errExactly error
	validate   func(t *testing.T, out DownloadOutput)
}

func runDownloadCase(t *testing.T, tt downloadCase) {
	t.Helper()
	client := testutil.NewTestClient(t, downloadCaseHandler(t, tt))
	got, err := Download(downloadCaseContext(tt), client, tt.input)
	assertDownloadCaseResult(t, got, err, tt)
}

// downloadCaseHandler gives the case its own mock, or one that fails the test
// on any request at all when the case asserts a refusal that must never reach
// GitLab.
func downloadCaseHandler(t *testing.T, tt downloadCase) http.Handler {
	t.Helper()
	if tt.forbidRequest {
		return testutil.ForbiddenHandler(t)
	}
	return tt.handler
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
	if tt.errPrefix != "" && err != nil && !strings.HasPrefix(err.Error(), tt.errPrefix) {
		t.Errorf("error = %q, want it to open with %q", err.Error(), tt.errPrefix)
	}
	if tt.errContain != "" && err != nil && !strings.Contains(err.Error(), tt.errContain) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContain)
	}
	if tt.errOmit != "" && err != nil && strings.Contains(err.Error(), tt.errOmit) {
		t.Errorf("error = %q, want it not to carry %q", err.Error(), tt.errOmit)
	}
	// Identity rather than errors.Is: a wrapped context error would satisfy
	// errors.Is and would mean the request was built before it was abandoned.
	if tt.errExactly != nil && err != tt.errExactly { //nolint:errorlint // identity is the assertion
		t.Errorf("error = %v, want exactly %v", err, tt.errExactly)
	}
	if tt.validate != nil {
		tt.validate(t, got)
	}
}

// TestDownloadOutput_ReadError verifies downloadOutput returns reader failures
// instead of producing partial base64 content.
//
// The reader yields a prefix and only then fails, which is the shape a
// truncated network read takes and the only shape that can tell the claim
// apart: io.ReadAll returns the bytes it did read alongside the error, so a
// reader that yields nothing leaves nothing to drop and the test would pass
// however the failure were handled. The error names the read rather than the
// download, because those are two different ceilings a caller reacts to
// differently.
func TestDownloadOutput_ReadError(t *testing.T) {
	out, err := downloadOutput(DownloadInput{
		ProjectID:      toolutil.StringOrInt("42"),
		ModelVersionID: toolutil.StringOrInt("7"),
		Path:           "models",
		Filename:       "model.bin",
	}, &partialThenFailingReader{})
	if err == nil {
		t.Fatal("downloadOutput() error = nil, want read error")
	}
	if !strings.Contains(err.Error(), "read ml model package content") {
		t.Errorf("error = %v, want it to name the read operation", err)
	}
	if out.ContentBase64 != "" || out.SizeBytes != 0 {
		t.Errorf("output = %+v, want no content alongside the read failure", out)
	}
}

// partialThenFailingReader yields one short chunk and then fails on every
// later read.
type partialThenFailingReader struct{ yielded bool }

func (r *partialThenFailingReader) Read(p []byte) (int, error) {
	if r.yielded {
		return 0, errors.New("read failed")
	}
	r.yielded = true
	return copy(p, "partial-model-bytes"), nil
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
				// The whole sentence rather than the hint alone. Everything
				// else in it is an argument: the name comes from one of the
				// input's two strings, the operation from a package constant
				// and the ceiling from a shift on another. Each can be wrong
				// on its own while the hint stays exactly as it is, and the
				// hint is all this case used to read.
				if err.Error() != wantOverCeilingRefusal {
					t.Errorf("downloadOutput(%d bytes) error = %q, want %q", tt.size, err.Error(), wantOverCeilingRefusal)
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
	// The whole message cannot be pinned here, since its tail is whatever the
	// transport said about the body it stopped reading, but the label is this
	// action's own and opens it.
	if !strings.HasPrefix(err.Error(), wantDownloadOp) {
		t.Errorf("Download() error = %q, want it to open with %q", err.Error(), wantDownloadOp)
	}
}
