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
	"strconv"
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
			// A personal access token client sends this route the token as
			// Bearer, which the route reads, so a 401 here is about the token
			// itself and not about the route's limit on OAuth tokens.
			errOmit: wantBearerLimitHint,
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
	"the file is too large to return in one MCP response; download it from GitLab directly, " +
	"sending the token as Authorization: Bearer " +
	"(GET /projects/:id/packages/ml_models/:model_version_id/files/:path/:filename, " +
	"with :path/ left out for a file at the package root): " +
	"model.bin exceeds the 32 MiB this action returns"

// wantBearerLimitHint is what a 401 from this route says in a session whose
// client sends Bearer: the route takes a personal access token or a CI job
// token and nothing else, so a GitLab OAuth access token is refused on every
// project, which no ordinary reading of a 401 tells a caller.
const wantBearerLimitHint = "GitLab's ML model package route authenticates only a personal access token " +
	"or a CI job token, so it refuses a GitLab OAuth access token on every project; " +
	"connect with a personal access token to download model files"

// wantRootEscapedPath is the request line a file at the package root
// produces: the file name straight after files/, with no path segment. GitLab
// 19.4.2 serves the root file there and answers 404 to files/./model.bin,
// since it joins the path and the file name into the name it looks up.
const wantRootEscapedPath = "/api/v4/projects/42/packages/ml_models/7/files/model.bin"

// TestDownload_FileAtThePackageRoot_IsRequestedWithoutAPathSegment verifies
// that a file stored at the root of a model package can be downloaded, which
// it could not be while path was required.
//
// The empty path, ".", and the two forms of either a caller writes with a
// slash all name the root, and each is requested as files/<name>. That is the
// form GitLab serves: "." passed through, as client-go's method would send it,
// is files/./model.bin, which GitLab answers 404 because it looks up the file
// "./model.bin". "." is also what the meta surface's file_path alias hands
// this action for a bare file name, so the root has to be read there too.
func TestDownload_FileAtThePackageRoot_IsRequestedWithoutAPathSegment(t *testing.T) {
	for _, path := range []string{"", ".", "/", "./"} {
		t.Run("path "+strconv.Quote(path), func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				if got := r.URL.EscapedPath(); got != wantRootEscapedPath {
					t.Errorf("escaped URL path = %q, want %q", got, wantRootEscapedPath)
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("root-model-data"))
			}))

			out, err := Download(t.Context(), client, DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           path,
				Filename:       "model.bin",
			})
			if err != nil {
				t.Fatalf("Download() error: %v", err)
			}
			want := DownloadOutput{
				ProjectID:      "42",
				ModelVersionID: "7",
				Path:           path,
				Filename:       "model.bin",
				ContentBase64:  base64.StdEncoding.EncodeToString([]byte("root-model-data")),
				SizeBytes:      len("root-model-data"),
			}
			if !reflect.DeepEqual(out, want) {
				t.Errorf("Download() = %+v, want %+v", out, want)
			}
		})
	}
}

// TestDownload_PathWrittenWithSlashes_NamesTheSameDirectory verifies that a
// nested path a caller writes with a leading or trailing slash reaches the
// directory it names. GitLab joins the path and the file name into the name
// it looks the file up by, so "/deep/nested/" sent as written would look for
// "/deep/nested//weights.h5", which no upload ever creates. The other
// spellings a caller writes for the same directory, "./deep/nested", a
// doubled slash and a "." segment, are read as it too: "./deep/nested" used to
// be sent as it was, and GitLab looked up "./deep/nested/weights.h5" and
// answered 404, the failure it gives files/./<name> at the root.
func TestDownload_PathWrittenWithSlashes_NamesTheSameDirectory(t *testing.T) {
	const want = "/api/v4/projects/42/packages/ml_models/7/files/deep%2Fnested/weights.h5"
	for _, path := range []string{"/deep/nested/", "./deep/nested", "deep//nested", "deep/./nested"} {
		t.Run(path, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.EscapedPath(); got != want {
					t.Errorf("escaped URL path = %q, want %q", got, want)
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("weight-data"))
			}))

			out, err := Download(t.Context(), client, DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           path,
				Filename:       "weights.h5",
			})
			if err != nil {
				t.Fatalf("Download() error: %v", err)
			}
			// The output echoes the caller's own spelling: it reports what
			// was asked for, and the request line above is where the reading
			// shows.
			if out.Path != path {
				t.Errorf("Path = %q, want the caller's %q", out.Path, path)
			}
		})
	}
}

// TestDownload_SendsTheTokenAsBearer pins the header this route is
// authenticated by.
//
// GitLab's ML model package API declares the route authenticated by a
// personal access token or a job token sent through the Bearer header and
// reads no other: the same personal access token gets 200 as Bearer and 404
// "Project Not Found" as PRIVATE-TOKEN on a private project (GitLab 19.4.2),
// so the download of every file of a private project's model read as a file
// that does not exist. Both request forms are pinned, the nested path through
// client-go and the root through this package's own request.
func TestDownload_SendsTheTokenAsBearer(t *testing.T) {
	for _, path := range []string{"models", ""} {
		t.Run("path "+strconv.Quote(path), func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("model-binary-data"))
			}))

			if _, err := Download(t.Context(), client, DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           path,
				Filename:       "model.bin",
			}); err != nil {
				t.Fatalf("Download() error: %v", err)
			}
		})
	}
}

// TestDownload_BearerSessionRefused_NamesTheRouteLimit verifies what a 401
// from this route says in a session whose client sends Bearer, which is an
// oauth HTTP mode session.
//
// There a GitLab OAuth access token is refused on every project: the route
// resolves a personal access token or a job token, and an OAuth token is
// neither. GitLab answers it 401 with nothing in the body that says why, so the
// refusal names the limit and the way round it. The 404 of a session that
// reached a project it cannot read keeps the identifier hint, since that one
// is not about the token.
func TestDownload_BearerSessionRefused_NamesTheRouteLimit(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		path    string
		want    string
		without string
	}{
		{name: "401 on a nested file", status: http.StatusUnauthorized, path: "models", want: wantBearerLimitHint, without: wantIdentifierHint},
		{name: "401 on a root file", status: http.StatusUnauthorized, path: "", want: wantBearerLimitHint, without: wantIdentifierHint},
		{name: "404 keeps the identifier hint", status: http.StatusNotFound, path: "models", want: wantIdentifierHint, without: wantBearerLimitHint},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewOAuthTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, `{"message":"`+http.StatusText(tt.status)+`"}`)
			}))

			_, err := Download(t.Context(), client, DownloadInput{
				ProjectID:      toolutil.StringOrInt("42"),
				ModelVersionID: toolutil.StringOrInt("7"),
				Path:           tt.path,
				Filename:       "model.bin",
			})
			if err == nil {
				t.Fatal("Download() error = nil, want a refusal")
			}
			if !strings.HasPrefix(err.Error(), wantDownloadOp) {
				t.Errorf("error = %q, want it to open with %q", err.Error(), wantDownloadOp)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to carry %q", err.Error(), tt.want)
			}
			if strings.Contains(err.Error(), tt.without) {
				t.Errorf("error = %q, want it not to carry %q", err.Error(), tt.without)
			}
		})
	}
}

// TestDownloadFromPackageRoot_PathNewRequestRefuses_IsAnError verifies that
// the root request returns what client-go's NewRequest refuses rather than
// sending anything. downloadFromPackageRoot takes the path rather than the
// identifiers so this branch can be driven: rootFilePath escapes every value a
// caller supplies, so no real input reaches it.
func TestDownloadFromPackageRoot_PathNewRequestRefuses_IsAnError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	reader, err := downloadFromPackageRoot(t.Context(), client, "projects/%zz/packages/ml_models/7/files/model.bin")
	if err == nil {
		t.Fatal("downloadFromPackageRoot() error = nil, want the unreadable path refused")
	}
	if reader != nil {
		t.Errorf("reader = %v, want none alongside the error", reader)
	}
}

// TestDownloadInput_Path_IsOptional verifies that path is not required by the
// schema every surface serves, which is read from the jsonschema tag's
// required marker. GitLab's route writes the path as an optional group,
// (*path/):file_name, and a file at the package root has none.
func TestDownloadInput_Path_IsOptional(t *testing.T) {
	field, ok := reflect.TypeFor[DownloadInput]().FieldByName("Path")
	if !ok {
		t.Fatal("DownloadInput has no Path field")
	}
	if tag := field.Tag.Get("jsonschema"); strings.HasSuffix(tag, ",required") {
		t.Errorf("Path jsonschema tag = %q, want it without the required marker", tag)
	}
}

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
// The root file's request is this package's own rather than client-go's, and
// is held to the same answer.
func TestDownload_ResponseOverTheClientCeiling_NamesTheWayOut(t *testing.T) {
	for _, path := range []string{"models", ""} {
		t.Run("path "+strconv.Quote(path), func(t *testing.T) {
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
				Path:           path,
				Filename:       "model.bin",
			})
			if err == nil {
				t.Fatal("Download() error = nil, want an error for a response over the client ceiling")
			}
			if !strings.Contains(err.Error(), "download it from GitLab directly") {
				t.Errorf("Download() error = %v, want it to name the way out", err)
			}
			// The whole message cannot be pinned here, since its tail is
			// whatever the transport said about the body it stopped reading,
			// but the label is this action's own and opens it.
			if !strings.HasPrefix(err.Error(), wantDownloadOp) {
				t.Errorf("Download() error = %q, want it to open with %q", err.Error(), wantDownloadOp)
			}
		})
	}
}
