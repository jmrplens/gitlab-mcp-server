// packages_stream_test.go contains unit tests for streaming package downloads.
package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// testOctetStream identifies the test octet stream constant used by this package.
	testOctetStream = "application/octet-stream"
	// testOutputBin identifies the test output bin constant used by this package.
	testOutputBin = "out.bin"
	// testAppBin identifies the test app bin constant used by this package.
	testAppBin = "app.bin"
	// testPkgVersion identifies the test pkg version constant used by this package.
	testPkgVersion = "1.0.0"
	// headerContentType identifies the header content type constant used by this package.
	headerContentType = "Content-Type"
)

// testStreamServer creates a handler that serves streaming downloads.
func testStreamServer(t *testing.T, fileBody string, statusCode int) http.HandlerFunc {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/packages/generic/"):
			w.Header().Set(headerContentType, testOctetStream)
			w.Header().Set("Content-Length", strconv.Itoa(len(fileBody)))
			w.WriteHeader(statusCode)
			w.Write([]byte(fileBody))
		default:
			http.NotFound(w, r)
		}
	})
}

// TestStreamDownloadPackageFile_Success verifies StreamDownloadPackageFile when success.
func TestStreamDownloadPackageFile_Success(t *testing.T) {
	fileBody := strings.Repeat("streaming-data-block-", 1000)
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))

	outPath := filepath.Join(t.TempDir(), testOutputBin)
	out, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     outPath,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.OutputPath != outPath {
		t.Errorf("OutputPath = %q, want %q", out.OutputPath, outPath)
	}
	if out.Size != int64(len(fileBody)) {
		t.Errorf("Size = %d, want %d", out.Size, len(fileBody))
	}
	data, _ := os.ReadFile(outPath)
	if string(data) != fileBody {
		t.Error("downloaded content does not match")
	}
	expectedSHA := fmt.Sprintf("%x", sha256.Sum256([]byte(fileBody)))
	if out.SHA256 != expectedSHA {
		t.Errorf("SHA256 = %q, want %q", out.SHA256, expectedSHA)
	}
}

// makeDirs creates each directory or fails the test, so a containment fixture
// reads as one line instead of a loop that asserts.
func makeDirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatalf("Mkdir(%q) error = %v", dir, err)
		}
	}
}

// confineDownloadRoots points the default local-path allow-list roots (the
// working directory and the OS temp directory) at dir, so a sibling of dir is
// genuinely outside every allowed destination.
func confineDownloadRoots(t *testing.T, dir string) {
	t.Helper()
	testutil.IsolateTempDir(t, dir)
	t.Chdir(dir)
}

// TestDownload_UnusableOutputPath_RefusedBeforeGitLabIsAsked verifies a
// download whose destination cannot hold a file is refused while the
// destination is still being prepared, before any byte is requested: a parent
// that is a file rather than a directory, and a destination that is itself a
// directory.
//
// Which stage refuses the file-as-parent case is a platform property, so the
// case accepts either. On Unix the path resolution itself fails, because a
// component that is not a directory is ENOTDIR. Windows resolves that path and
// refuses at the mkdir instead, which means the create branch this test was
// first written to prove unreachable is in fact reached there. The invariant
// the name states holds either way, and the handler is what enforces it:
// GitLab is never asked.
func TestDownload_UnusableOutputPath_RefusedBeforeGitLabIsAsked(t *testing.T) {
	root := t.TempDir()
	confineDownloadRoots(t, root)

	fileAsParent := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(fileAsParent, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	existingDir := filepath.Join(root, "already-a-directory")
	makeDirs(t, existingDir)

	for _, testCase := range []struct {
		name    string
		path    string
		wantAny []string
	}{
		{
			name:    "parent is a file",
			path:    filepath.Join(fileAsParent, "out.bin"),
			wantAny: []string{"resolve output path", "create output directory"},
		},
		{
			name:    "destination is a directory",
			path:    existingDir,
			wantAny: []string{"already exists and is not a regular file"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Errorf("package file was requested for an unusable output_path %q", testCase.path)
				w.WriteHeader(http.StatusForbidden)
			}))
			_, err := downloadTo(t, client, testCase.path)
			if err == nil || !containsAny(err.Error(), testCase.wantAny) {
				t.Errorf("Download(%q) error = %v, want one naming any of %q", testCase.path, err, testCase.wantAny)
			}
		})
	}
}

// containsAny reports whether s contains any of the substrings, which lets a
// case accept the several messages one refusal is spelled with across
// platforms without weakening into a bare "an error happened".
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// TestDownload_OutputPathOutsideAllowedDirs_Rejected verifies that a download
// destination is confined to the allow-listed roots: a path outside them, a
// parent-traversal escape and a symlinked parent are all refused, nothing is
// created on the way, and the package is never even requested. The one
// legitimate destination inside the workspace still works.
func TestDownload_OutputPathOutsideAllowedDirs_Rejected(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	linkDir := filepath.Join(allowed, "escape")
	symlinked := os.Symlink(outside, linkDir) == nil
	confineDownloadRoots(t, allowed)

	tests := []struct {
		name    string
		path    string
		wantErr bool
		skip    bool
	}{
		{name: "destination inside the workspace is written", path: filepath.Join(allowed, "artifact.bin")},
		{name: "destination outside every allowed root is refused", path: filepath.Join(outside, ".gitlab-mcp-server.env"), wantErr: true},
		{name: "parent traversal out of the workspace is refused", path: filepath.Join(allowed, "..", "home", "stolen.bin"), wantErr: true},
		{name: "symlinked parent directory pointing outside is refused", path: filepath.Join(linkDir, "planted.env"), wantErr: true, skip: !symlinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			if tt.wantErr {
				assertDownloadRefused(t, tt.path, outside)
				return
			}
			assertDownloadWritten(t, tt.path)
		})
	}
}

// assertDownloadRefused runs a download to path and asserts it is refused
// without reaching the package endpoint and without creating anything, at the
// destination or under outside.
func assertDownloadRefused(t *testing.T, path, outside string) {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("package file was requested for a refused output_path %q", path)
		w.WriteHeader(http.StatusForbidden)
	}))

	if _, err := downloadTo(t, client, path); err == nil {
		t.Fatalf("Download(%q) error = nil, want refusal", path)
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("os.Lstat(%q) error = %v after refusal, want nothing written", path, statErr)
	}
	escaped := filepath.Join(outside, filepath.Base(path))
	if _, statErr := os.Lstat(escaped); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("os.Lstat(%q) error = %v, want no file outside the allowed roots", escaped, statErr)
	}
}

// assertDownloadWritten runs a download to path and asserts it reaches the
// package endpoint and lands on disk.
func assertDownloadWritten(t *testing.T, path string) {
	t.Helper()
	var served atomic.Bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Store(true)
		w.Header().Set(headerContentType, testOctetStream)
		_, _ = w.Write([]byte("payload"))
	}))

	if _, err := downloadTo(t, client, path); err != nil {
		t.Fatalf("Download(%q) error = %v, want success", path, err)
	}
	if !served.Load() {
		t.Error("the legitimate download did not reach the package endpoint")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("os.Stat(%q) error = %v, want the file written", path, statErr)
	}
}

// downloadTo issues one package download to path against client.
func downloadTo(t *testing.T, client *gitlabclient.Client, path string) (DownloadOutput, error) {
	t.Helper()
	return Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     path,
	})
}

// TestStreamDownloadPackageFile_CreatesDirectory verifies StreamDownloadPackageFile creates directory.
func TestStreamDownloadPackageFile_CreatesDirectory(t *testing.T) {
	fileBody := "hello-stream"
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))

	outPath := filepath.Join(t.TempDir(), "sub", "deep", testOutputBin)
	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     outPath,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if _, statErr := os.Stat(outPath); os.IsNotExist(statErr) {
		t.Error("expected output file to be created, but it does not exist")
	}
}

// TestStreamDownloadPackageFile_WithProgressToken verifies Download wraps the
// streaming writer when the MCP request includes a progress token.
func TestStreamDownloadPackageFile_WithProgressToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fileBody := strings.Repeat("stream-progress-block-", 4096)
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))
	outPath := filepath.Join(t.TempDir(), testOutputBin)

	server := mcp.NewServer(&mcp.Implementation{Name: "package-download-test", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "package_download_with_progress"}, func(ctx context.Context, req *mcp.CallToolRequest, input DownloadInput) (*mcp.CallToolResult, DownloadOutput, error) {
		out, err := Download(ctx, req, client, input)
		if err != nil {
			return nil, DownloadOutput{}, err
		}
		return &mcp.CallToolResult{}, out, nil
	})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	progressSeen := make(chan struct{}, 1)
	var once sync.Once
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "package-download-client"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, _ *mcp.ProgressNotificationClientRequest) {
			once.Do(func() { progressSeen <- struct{}{} })
		},
	})
	clientSession, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		_ = serverSession.Wait()
	})

	_, err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "package_download_with_progress",
		Arguments: map[string]any{
			"project_id":      "42",
			"package_name":    testPackageName,
			"package_version": testPkgVersion,
			"file_name":       testAppBin,
			"output_path":     outPath,
		},
		Meta: mcp.Meta{"progressToken": "package-download-progress-token"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}

	select {
	case <-progressSeen:
	case <-ctx.Done():
		t.Fatal("timed out waiting for progress notification")
	}
}

// TestStreamDownloadPackageFile_ContextCancelled verifies StreamDownloadPackageFile when context cancelled.
func TestStreamDownloadPackageFile_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testStreamServer(t, "data", http.StatusOK))

	ctx := testutil.CancelledCtx(t)

	_, err := Download(ctx, nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     filepath.Join(t.TempDir(), testOutputBin),
	})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TestStreamDownloadPackageFile_APIError verifies StreamDownloadPackageFile when API error.
func TestStreamDownloadPackageFile_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"Package Not Found"}`)
	}))

	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     filepath.Join(t.TempDir(), testOutputBin),
	})
	if err == nil {
		t.Fatal("expected error for API error, got nil")
	}
}

// TestComputeSHA256_ViaToolutil verifies ComputeSHA256 when via toolutil.
func TestComputeSHA256_ViaToolutil(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.bin")
	os.WriteFile(f, []byte("hello"), 0o600)

	hash, err := toolutil.ComputeSHA256(f)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte("hello")))
	if hash != expected {
		t.Errorf("SHA256 = %q, want %q", hash, expected)
	}
}

// TestStreamDownload_UnwritablePath verifies that a download whose output path
// has a regular file where a directory belongs is refused.
//
// The refusal comes from the confinement rather than from opening the file:
// toolutil.WriteDownloadOutputFile resolves the longest existing prefix and
// gets ENOTDIR, so neither MkdirAll nor the create below it ever runs. The
// comment here used to say os.Create failed, which gobco refutes: that arm is
// never taken. TestDownload_UnusableOutputPath_RefusedBeforeGitLabIsAsked states the
// same rule with the message it is actually refused by.
func TestStreamDownload_UnwritablePath(t *testing.T) {
	client := testutil.NewTestClient(t, testStreamServer(t, "data", http.StatusOK))

	blocker := filepath.Join(t.TempDir(), "blocker")
	os.WriteFile(blocker, []byte("x"), 0o600)
	badPath := filepath.Join(blocker, "sub", testOutputBin)

	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     badPath,
	})
	if err == nil {
		t.Fatal("expected error for unwritable output path, got nil")
	}
}

// TestStreamDownload_OutputPathIsDirectory verifies streamDownloadPackageFile
// refuses an output path that already exists as something other than a regular
// file, before it opens anything. A directory is the harmless instance of that
// rule; a symlink is the one it exists for.
func TestStreamDownload_OutputPathIsDirectory(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Download(context.Background(), nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error when output_path is a directory")
	}
	if !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("error = %q, want the not-a-regular-file refusal", err.Error())
	}
}

// ----- branch coverage -----

// TestStreamDownload_DeadBranches documents the one condition inside
// streamDownloadPackageFile that no public call path evaluates both ways,
// which gobco reports:
//
//   - FormatPackageURL error: reached, by the invalid-file-name cases of
//     TestDownload_FileNameShapes. What is unreachable is the arm below it
//     that leaves the hint empty: for a string project id, parseID accepts
//     whatever it is given, so ErrInvalidFileName is the only error the call
//     can return and errors.Is is never false there.
//
// The request construction failure no input reaches is reached through the
// newDownloadRequest seam instead, by
// TestDownload_RequestCannotBeBuilt_NothingIsWritten. The branches that
// prepare the destination, flush the file and put it in place used to be
// listed here as unreachable too; they now live in
// toolutil.WriteDownloadOutputFile, whose own tests stage each of them.
//
// We assert the documented contract below: a happy-path download streams
// the payload to disk and reports its size and checksum.
func TestStreamDownload_DeadBranches(t *testing.T) {
	fileBody := "dead-branch-fixture"
	client := testutil.NewTestClient(t, testStreamServer(t, fileBody, http.StatusOK))

	outPath := filepath.Join(t.TempDir(), "dead-branches.bin")
	size, checksum, err := streamDownloadPackageFile(
		context.Background(),
		nil,
		client,
		DownloadInput{
			ProjectID:      "42",
			PackageName:    testPackageName,
			PackageVersion: testPkgVersion,
			FileName:       testAppBin,
			OutputPath:     outPath,
		},
	)
	if err != nil {
		t.Fatalf("streamDownloadPackageFile() error = %v", err)
	}
	if size != int64(len(fileBody)) {
		t.Fatalf("size = %d, want %d", size, len(fileBody))
	}
	expected := sha256.Sum256([]byte(fileBody))
	want := hex.EncodeToString(expected[:])
	if checksum != want {
		t.Fatalf("checksum = %q, want %q", checksum, want)
	}
}

// TestDownload_ContextCancelledMidFlight_AbandonsTheRequest verifies that the
// download request carries the caller's context, so the action deadline and
// an abandoned HTTP POST end a transfer nothing else bounds.
//
// The download builds its request by hand through NewRequest, which takes the
// context as a request option like every other client-go call and falls back
// to context.Background() without one; it was built with none. The context is
// cancelled once the GET has arrived, since the guard at the top of Download
// answers one cancelled up front before any request exists.
func TestDownload_ContextCancelledMidFlight_AbandonsTheRequest(t *testing.T) {
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerContentType, testOctetStream)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("package-bytes"))
	})

	_, err := Download(ctx, nil, client, DownloadInput{
		ProjectID:      "42",
		PackageName:    testPackageName,
		PackageVersion: testPkgVersion,
		FileName:       testAppBin,
		OutputPath:     filepath.Join(t.TempDir(), testOutputBin),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Download() error = %v after the context was cancelled mid-flight, want context.Canceled", err)
	}
}

// previousRelease is what a destination held before a download that fails,
// which it must still hold afterwards.
const previousRelease = "the previous release, which a failed download must not touch\n"

// packageBody is the file a mock serves, long enough that half of it is
// several writes and a flush on its own.
var packageBody = strings.Repeat("package-file-block-", 512)

// serveHalfThen answers the package GET with a Content-Length for the whole
// body, sends half of it and flushes, so the download has written bytes by
// the time then decides how the response ends.
func serveHalfThen(then func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerContentType, testOctetStream)
		w.Header().Set("Content-Length", strconv.Itoa(len(packageBody)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(packageBody[:len(packageBody)/2]))
		// A failed flush still ends the response short of its declared
		// length, so the download is interrupted either way.
		_ = http.NewResponseController(w).Flush()
		then(w, r)
	}
}

// interruption is one way a package download can end before its body does,
// with the context and client that stage it.
type interruption struct {
	name  string
	stage func(t *testing.T) (context.Context, *gitlabclient.Client)
	check func(t *testing.T, err error)
}

// wantContextCanceled fails unless err carries context.Canceled.
func wantContextCanceled(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Download() error = %v, want context.Canceled", err)
	}
}

// wantMessage returns a check that fails unless the error names want.
func wantMessage(want string) func(t *testing.T, err error) {
	return func(t *testing.T, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Download() error = %v, want one naming %q", err, want)
		}
	}
}

// interruptions lists the four ways a download ends early that
// TestDownload_Interrupted_LeavesOutputPathAsItWas holds to one outcome.
func interruptions() []interruption {
	return []interruption{
		{
			name: "GitLab answers an error",
			stage: func(t *testing.T) (context.Context, *gitlabclient.Client) {
				t.Helper()
				return context.Background(), testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Package Not Found"}`)
				}))
			},
			check: wantMessage("404"),
		},
		{
			name: "the body is cut short",
			stage: func(t *testing.T) (context.Context, *gitlabclient.Client) {
				t.Helper()
				// ErrAbortHandler closes the connection without the rest of
				// the declared length, which the client reads as an
				// unexpected end of the body, and net/http does not log it.
				return context.Background(), testutil.NewTestClient(t, serveHalfThen(func(http.ResponseWriter, *http.Request) {
					panic(http.ErrAbortHandler)
				}))
			},
			check: wantMessage("unexpected EOF"),
		},
		{
			name: "the call is cancelled once the request arrives",
			stage: func(t *testing.T) (context.Context, *gitlabclient.Client) {
				t.Helper()
				return testutil.CancelOnArrival(t, testStreamServer(t, packageBody, http.StatusOK))
			},
			check: wantContextCanceled,
		},
		{
			name: "the call is cancelled partway through the body",
			stage: func(t *testing.T) (context.Context, *gitlabclient.Client) {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				return ctx, testutil.NewTestClient(t, serveHalfThen(func(_ http.ResponseWriter, r *http.Request) {
					cancel()
					// Held until the client abandons the request, so the
					// cancellation and not the end of the response is what
					// ends the body; bounded so a download that ignored the
					// context fails in seconds rather than hanging. How much
					// of the flushed half the client wrote before it saw the
					// cancellation is the scheduler's call; the cut body
					// above is the case where every byte of it is written.
					select {
					case <-r.Context().Done():
					case <-time.After(5 * time.Second):
					}
				}))
			},
			check: wantContextCanceled,
		},
	}
}

// TestDownload_Interrupted_LeavesOutputPathAsItWas verifies that a package
// download that ends before its body does leaves output_path exactly as it
// was, absent if it was absent and holding its previous content if it held
// one, and leaves no temporary file beside it.
//
// The download used to create output_path first and stream into it, so every
// one of these left a file that looked like a download and was not one: empty
// after an error answer or a cancellation on arrival, truncated after a cut
// body or a cancellation partway, and a previous file at that path destroyed
// in all four, since it was truncated before a byte was requested. The case
// that cancels partway is the one the handler context makes routine: an
// abandoned call or the action deadline now ends a transfer where it stands.
func TestDownload_Interrupted_LeavesOutputPathAsItWas(t *testing.T) {
	for _, tc := range interruptions() {
		t.Run(tc.name, func(t *testing.T) {
			for _, existed := range []bool{false, true} {
				name := "output_path absent"
				if existed {
					name = "output_path held a previous file"
				}
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					outPath := filepath.Join(dir, testOutputBin)
					if existed {
						if err := os.WriteFile(outPath, []byte(previousRelease), 0o600); err != nil {
							t.Fatalf("WriteFile(%q) error = %v", outPath, err)
						}
					}
					ctx, client := tc.stage(t)

					_, err := Download(ctx, nil, client, DownloadInput{
						ProjectID:      "42",
						PackageName:    testPackageName,
						PackageVersion: testPkgVersion,
						FileName:       testAppBin,
						OutputPath:     outPath,
					})
					tc.check(t, err)

					assertOutputPathAsItWas(t, outPath, existed)
					assertOnlyEntries(t, dir, existed)
				})
			}
		})
	}
}

// assertOutputPathAsItWas fails unless outPath is still absent, when it was,
// or still holds previousRelease, when it held that.
func assertOutputPathAsItWas(t *testing.T, outPath string, existed bool) {
	t.Helper()
	got, err := os.ReadFile(outPath)
	if !existed {
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("os.ReadFile(%q) = %d bytes, %v, want output_path still absent", outPath, len(got), err)
		}
		return
	}
	if err != nil || string(got) != previousRelease {
		t.Errorf("os.ReadFile(%q) = %d bytes, %v, want the previous file's %d bytes untouched", outPath, len(got), err, len(previousRelease))
	}
}

// assertOnlyEntries fails when dir holds anything besides the output file it
// held before the download, which is where a temporary file left behind would
// show.
func assertOnlyEntries(t *testing.T, dir string, existed bool) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) error = %v", dir, err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	want := 0
	if existed {
		want = 1
	}
	if len(names) != want || (existed && names[0] != testOutputBin) {
		t.Errorf("os.ReadDir(%q) = %q, want only the output file it held before", dir, names)
	}
}

// TestDownload_RequestCannotBeBuilt_NothingIsWritten verifies that a download
// whose request cannot be built is refused with the construction error before
// GitLab is asked and before anything is created at output_path. No input
// reaches this, since FormatPackageURL escapes everything it interpolates, so
// the construction is replaced for the test.
func TestDownload_RequestCannotBeBuilt_NothingIsWritten(t *testing.T) {
	original := newDownloadRequest
	newDownloadRequest = func(context.Context, *gitlabclient.Client, string) (*retryablehttp.Request, error) {
		return nil, errors.New("malformed request path")
	}
	t.Cleanup(func() { newDownloadRequest = original })

	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	outPath := filepath.Join(t.TempDir(), "sub", testOutputBin)

	_, err := downloadTo(t, client, outPath)
	if err == nil || !strings.Contains(err.Error(), "create download request: malformed request path") {
		t.Errorf("Download() error = %v, want the construction error", err)
	}
	if _, statErr := os.Lstat(filepath.Dir(outPath)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("os.Lstat(%q) error = %v, want nothing created", filepath.Dir(outPath), statErr)
	}
}
