// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package modelregistry

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// DownloadInput holds parameters for downloading a ML model package file.
type DownloadInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"       jsonschema:"Project ID or URL-encoded path,required"`
	ModelVersionID toolutil.StringOrInt `json:"model_version_id" jsonschema:"Model version ID. Either the numeric version ID from the model version URL or a candidate run ID prefixed with candidate: such as candidate:5,required"`
	Path           string               `json:"path,omitempty"   jsonschema:"Directory of the file within the model package (such as models or sub/dir). Leave it empty or pass . for a file at the package root"`
	Filename       string               `json:"filename"         jsonschema:"Name of the file to download,required"`
}

// DownloadOutput represents the downloaded ML model package file.
type DownloadOutput struct {
	toolutil.HintableOutput
	ProjectID      string `json:"project_id"`
	ModelVersionID string `json:"model_version_id"`
	Path           string `json:"path"`
	Filename       string `json:"filename"`
	ContentBase64  string `json:"content_base64"`
	SizeBytes      int    `json:"size_bytes"`
}

// Download retrieves a single file from a machine learning model
// package via the GitLab Model Registry API
// (GET /projects/:id/packages/ml_models/:model_version_id/files/(:path/):filename).
// Returns the raw bytes base64-encoded in ContentBase64, with
// SizeBytes describing the decoded size.
//
// The request carries the token as "Authorization: Bearer", which is the only
// header the route reads ([gitlabclient.Client.BearerToken]); a file at the
// package root is requested without a path segment, which client-go cannot
// write ([downloadFromPackageRoot]). The model version and the file name each
// reach the path as one segment whose dots are not escaped, so a value with a
// "." or ".." segment is refused before any request, naming the parameter;
// the path is normalized instead ([downloadModelFile]), and a dot segment in
// it that the normalization does not read as one is refused the same way
// ([rejectUnresolvedDotSegment]).
func Download(ctx context.Context, client *gitlabclient.Client, in DownloadInput) (DownloadOutput, error) {
	if err := ctx.Err(); err != nil {
		return DownloadOutput{}, err
	}
	if in.ProjectID.String() == "" {
		return DownloadOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.ModelVersionID.String() == "" {
		return DownloadOutput{}, toolutil.ErrFieldRequired("model_version_id")
	}
	if in.Filename == "" {
		return DownloadOutput{}, toolutil.ErrFieldRequired("filename")
	}
	if err := toolutil.RejectTraversalArg(opDownloadModelPackage, "model_version_id", in.ModelVersionID.String()); err != nil {
		return DownloadOutput{}, err
	}
	if err := toolutil.RejectTraversalArg(opDownloadModelPackage, "filename", in.Filename); err != nil {
		return DownloadOutput{}, err
	}
	if err := rejectUnresolvedDotSegment(in.Path); err != nil {
		return DownloadOutput{}, err
	}

	reader, err := downloadModelFile(ctx, client, in)
	if err != nil {
		return DownloadOutput{}, downloadError(client, err)
	}

	return downloadOutput(in, reader)
}

// downloadModelFile sends the one request a download makes: through client-go
// for a file inside a directory of the package, and as this package's own for
// a file at its root.
//
// The path is read the way a file system reads it ([path.Clean]): GitLab
// joins the path and the file name into the name it looks the file up by and
// stores none with a slash at either end, a doubled slash or a "." segment,
// so "/deep/nested/", "./deep/nested" and "deep//nested" all name
// deep/nested, where any of them sent as written names a file no upload
// creates. "" and "." are the root, the second being what the meta surface's
// file_path alias hands this action for a bare file name. Both requests
// carry the token as Bearer.
func downloadModelFile(ctx context.Context, client *gitlabclient.Client, in DownloadInput) (io.Reader, error) {
	dir := strings.Trim(path.Clean("/"+in.Path), "/")
	if dir == "" {
		return downloadFromPackageRoot(ctx, client, rootFilePath(in))
	}
	reader, _, err := client.GL().ModelRegistry.DownloadMachineLearningModelPackage(
		in.ProjectID.String(), in.ModelVersionID.String(), dir, in.Filename,
		gl.WithContext(ctx), client.BearerToken(),
	)
	return reader, err
}

// rejectUnresolvedDotSegment returns the refusal of the path parameter when
// one of its "/"-separated components is a "." or ".." segment in a form
// [path.Clean] does not resolve, before the path is normalized.
//
// path.Clean splits on "/" alone and resolves only a component written exactly
// "." or "..", which is what lets [downloadModelFile] accept "./deep" or
// "deep/../models" as the directory they name. A component that path.Clean
// keeps as a name can still carry a dot segment: one delimited by "\", which
// GitLab reads as a separator too ("a\..\b"), or one written percent-encoded
// ("%2e%2e", "..%5Cx"). Each such component is held to the rule the model
// version and the file name are held to ([toolutil.RejectTraversalArg]), so
// the refusal names path and the segment found.
func rejectUnresolvedDotSegment(dir string) error {
	for component := range strings.SplitSeq(dir, "/") {
		if component == "." || component == ".." {
			continue
		}
		if err := toolutil.RejectTraversalArg(opDownloadModelPackage, "path", component); err != nil {
			return err
		}
	}
	return nil
}

// rootFilePath is the route of a file at the root of a model package, the
// form GitLab 19.4.2 serves: files/<name>. client-go's
// DownloadMachineLearningModelPackage writes the path segment whatever its
// value: an empty path is sent as files//<name>, which GitLab serves only
// because its router tolerates the empty segment, and "." as files/./<name>,
// which it answers 404 because it looks up the file "./<name>" (see "The
// model package file path segment cannot be omitted" in
// docs/development/upstream-bugs.md). The values are escaped the way client-go
// escapes them on its own route, the project with gl.PathEscape and the model
// version and the file name with url.PathEscape, which leaves the colon of a
// candidate version and the dot of a file name alone.
func rootFilePath(in DownloadInput) string {
	return "projects/" + gl.PathEscape(in.ProjectID.String()) +
		"/packages/ml_models/" + url.PathEscape(in.ModelVersionID.String()) +
		"/files/" + url.PathEscape(in.Filename)
}

// downloadFromPackageRoot sends the GET of a root file and returns its body,
// read whole as client-go reads the nested one, so the client-wide response
// ceiling stops it the same way.
//
// It takes the route rather than the identifiers, as the raw helpers of the
// other packages do, so the one way client-go's NewRequest refuses a GET, a
// route url.PathUnescape cannot read, can be driven: rootFilePath escapes
// every value a caller supplies, so no real input reaches that branch.
func downloadFromPackageRoot(ctx context.Context, client *gitlabclient.Client, route string) (io.Reader, error) {
	req, err := client.GL().NewRequest(http.MethodGet, route, nil,
		[]gl.RequestOptionFunc{gl.WithContext(ctx), client.BearerToken()})
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	if _, err = client.GL().Do(req, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

// downloadError wraps what the download's request failed with: a body over
// the client's response ceiling with the way to fetch it from GitLab, a 401
// in a session that sends Bearer with the route's limit on OAuth tokens, and
// a 404 with the identifiers to check.
func downloadError(client *gitlabclient.Client, err error) error {
	if errors.Is(err, gitlabclient.ErrResponseTooLarge) {
		return toolutil.WrapErrWithHint(opDownloadModelPackage, err, errModelFileTooLarge)
	}
	if client.SendsBearer() && toolutil.IsHTTPStatus(err, http.StatusUnauthorized) {
		return toolutil.WrapErrWithHint(opDownloadModelPackage, err, errModelRouteRefusesOAuth)
	}
	return toolutil.WrapErrWithStatusHint(opDownloadModelPackage, err, http.StatusNotFound, "verify project_id, model_version_id, path, and filename")
}

// errModelRouteRefusesOAuth is the hint for a 401 from this route in a session
// whose client sends Bearer, which is an oauth HTTP mode session. The route
// resolves a personal access token or a CI job token sent as Bearer and
// nothing else, so a GitLab OAuth access token is refused there on every
// project with a 401 that says nothing about why (see "The ML model package
// route accepts neither PRIVATE-TOKEN nor a GitLab OAuth access token" in
// docs/development/upstream-bugs.md). A personal access token admitted in the
// same mode works, which is the way round it. It is one literal rather than a
// concatenation, since an operator in a constant initializer carries no
// statement a mutation run could credit a test with killing.
const errModelRouteRefusesOAuth = "GitLab's ML model package route authenticates only a personal access token or a CI job token, so it refuses a GitLab OAuth access token on every project; connect with a personal access token to download model files"

// maxModelFileBytes is the largest model package file this action returns in
// one response.
//
// The bound is on what the download becomes, not on what it costs to read: the
// file is buffered, base64-encoded a third larger again and copied into a
// JSON-RPC message, and the client-wide response ceiling caps only the read.
// A file above it is refused rather than truncated, for the reason a group
// export is: the next step for a model file is loading it, and a prefix of a
// .safetensors or a .gguf loads no more than a prefix of a .tar.gz imports.
//
// It is the group export's ceiling rather than the 1 MiB the jobs package
// applies to artifact content, because that one truncates and flags it, which
// is a sensible partial answer for a build log and not for a weights file, and
// because 1 MiB cuts through the middle of what this endpoint is usable for: a
// model card and a config.json are kilobytes, but a tokenizer.json for a large
// vocabulary reaches into the tens of MiB. A ceiling between that and the
// hundreds of MiB a weights file occupies separates the files a caller can use
// here from the ones they must fetch another way, and 32 MiB is in that gap.
// opDownloadModelPackage names this action in every error it wraps, so a
// reader sees one operation rather than three spellings of it.
const opDownloadModelPackage = "download ml model package"

const maxModelFileBytes = 32 << 20

// errModelFileTooLarge is the hint for a file this action will not carry,
// whichever ceiling stopped it: this one, or the client-wide response ceiling
// that stops the read before it reaches here.
const errModelFileTooLarge = "the file is too large to return in one MCP response; download it from GitLab directly, sending the token as Authorization: Bearer (GET /projects/:id/packages/ml_models/:model_version_id/files/:path/:filename, with :path/ left out for a file at the package root)"

// downloadOutput reads a model package file stream up to
// [maxModelFileBytes] and returns the bytes base64-encoded alongside the
// original input metadata.
func downloadOutput(in DownloadInput, reader io.Reader) (DownloadOutput, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxModelFileBytes+1))
	if err != nil {
		return DownloadOutput{}, toolutil.WrapErrWithMessage("read ml model package content", err)
	}
	if len(data) > maxModelFileBytes {
		return DownloadOutput{}, toolutil.WrapErrWithHint(
			opDownloadModelPackage,
			fmt.Errorf("%s exceeds the %d MiB this action returns", in.Filename, maxModelFileBytes>>20),
			errModelFileTooLarge,
		)
	}

	return DownloadOutput{
		ProjectID:      in.ProjectID.String(),
		ModelVersionID: in.ModelVersionID.String(),
		Path:           in.Path,
		Filename:       in.Filename,
		ContentBase64:  base64.StdEncoding.EncodeToString(data),
		SizeBytes:      len(data),
	}, nil
}
