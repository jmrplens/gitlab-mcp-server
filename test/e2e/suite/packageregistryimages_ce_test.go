//go:build e2e && !enterprise

// packageregistryimages_ce_test.go covers the image-backed container
// registry actions of the gitlab_package meta-tool: registry_get,
// registry_tag_list, registry_tag_get, registry_tag_delete,
// registry_tag_delete_bulk, and registry_delete.
//
// The Docker compose stack exposes the GitLab container registry over plain
// HTTP on port 5050, but ships no Docker CLI, so the test pushes a minimal
// Docker v2 image purely over HTTP: it obtains a registry JWT from GitLab's
// /jwt/auth endpoint with basic auth, uploads a tiny config blob and an
// empty-tar gzip layer through the registry v2 blob upload protocol, and
// PUTs one manifest per tag.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/containerregistry"
)

// pkgImgManifestMediaType is the Docker v2 manifest media type the GitLab
// registry accepts for tag pushes.
const pkgImgManifestMediaType = "application/vnd.docker.distribution.manifest.v2+json"

// pkgImgRegistryBaseURL derives the registry endpoint from GITLAB_URL: the
// compose stack publishes the registry on the same host under port 5050
// (registry_external_url in docker-compose.yml).
func pkgImgRegistryBaseURL(t *testing.T) string {
	t.Helper()
	parsed, err := url.Parse(os.Getenv("GITLAB_URL"))
	requireNoError(t, err, "parse GITLAB_URL")
	return "http://" + parsed.Hostname() + ":5050"
}

// pkgImgRequest performs one raw HTTP request and returns the response
// status, headers, and body. The registry v2 protocol needs header access
// (the blob upload Location) that the JSON-oriented suite helpers do not
// expose.
func pkgImgRequest(ctx context.Context, t *testing.T, method, rawURL string, headers map[string]string, body []byte) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body)) //nolint:gosec // G704: e2e helper deliberately targets registry URLs on the trusted local Docker stack.
	requireNoError(t, err, "build registry HTTP request")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: e2e helper deliberately targets registry URLs on the trusted local Docker stack.
	requireNoError(t, err, "execute registry HTTP request")
	defer func() { _ = resp.Body.Close() }()
	payload, err := readAllRegistryBody(resp)
	requireNoError(t, err, "read registry HTTP response")
	return resp.StatusCode, resp.Header, payload
}

// readAllRegistryBody drains one registry response body so connections can
// be reused across the several protocol round-trips of a push.
func readAllRegistryBody(resp *http.Response) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(resp.Body)
	return buf.Bytes(), err
}

// pkgImgRegistryReachable reports whether the :5050 registry endpoint
// answers at all; without it the image-backed registry actions cannot be
// exercised and the test skips.
func pkgImgRegistryReachable(ctx context.Context, baseURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v2/", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// pkgImgJWT fetches a registry bearer token scoped for push and pull on one
// repository from GitLab's JWT auth endpoint using basic auth.
func pkgImgJWT(ctx context.Context, t *testing.T, repository string) string {
	t.Helper()
	token := os.Getenv("GITLAB_TOKEN")
	requireTruef(t, token != "", "GITLAB_TOKEN is required for the registry JWT flow")
	authURL := fmt.Sprintf("%s/jwt/auth?service=container_registry&scope=repository:%s:push,pull",
		os.Getenv("GITLAB_URL"), repository)
	basic := base64.StdEncoding.EncodeToString([]byte(sess.username + ":" + token))
	status, _, payload := pkgImgRequest(ctx, t, http.MethodGet, authURL, map[string]string{
		"Authorization": "Basic " + basic,
	}, nil)
	requireTruef(t, status == http.StatusOK, "jwt/auth: status %d, body %s", status, payload)
	var decoded struct {
		Token string `json:"token"`
	}
	requireNoError(t, json.Unmarshal(payload, &decoded), "decode jwt/auth response")
	requireTruef(t, decoded.Token != "", "expected non-empty registry JWT")
	return decoded.Token
}

// pkgImgUploadBlob pushes one blob through the registry v2 two-step upload
// (POST for an upload URL, PUT with the digest) and returns its digest.
func pkgImgUploadBlob(ctx context.Context, t *testing.T, registryURL, repository, jwt string, blob []byte) string {
	t.Helper()
	digest := "sha256:" + hex.EncodeToString(func() []byte { sum := sha256.Sum256(blob); return sum[:] }())

	status, headers, payload := pkgImgRequest(ctx, t, http.MethodPost,
		registryURL+"/v2/"+repository+"/blobs/uploads/",
		map[string]string{"Authorization": "Bearer " + jwt}, nil)
	requireTruef(t, status == http.StatusAccepted, "blob upload start: status %d, body %s", status, payload)
	location := headers.Get("Location")
	requireTruef(t, location != "", "expected blob upload Location header")

	separator := "?"
	if strings.Contains(location, "?") {
		separator = "&"
	}
	status, _, payload = pkgImgRequest(ctx, t, http.MethodPut, location+separator+"digest="+digest, map[string]string{
		"Authorization": "Bearer " + jwt,
		"Content-Type":  "application/octet-stream",
	}, blob)
	requireTruef(t, status == http.StatusCreated, "blob upload finish: status %d, body %s", status, payload)
	return digest
}

// pkgImgPushImage pushes one minimal single-layer image to the repository
// under the given tag. The variant string is embedded in the image config so
// different tags get distinct manifests (a shared manifest would make a
// single tag deletion ambiguous).
func pkgImgPushImage(ctx context.Context, t *testing.T, registryURL, repository, jwt, tag, variant string) {
	t.Helper()

	var tarBuf bytes.Buffer
	requireNoError(t, tar.NewWriter(&tarBuf).Close(), "build empty tar layer")
	var layerBuf bytes.Buffer
	gz := gzip.NewWriter(&layerBuf)
	_, err := gz.Write(tarBuf.Bytes())
	requireNoError(t, err, "gzip layer content")
	requireNoError(t, gz.Close(), "finish gzip layer")
	layer := layerBuf.Bytes()

	config := fmt.Appendf(nil,
		`{"architecture":"amd64","os":"linux","rootfs":{"type":"layers","diff_ids":[]},"config":{"Labels":{"e2e-variant":%q}}}`,
		variant)

	configDigest := pkgImgUploadBlob(ctx, t, registryURL, repository, jwt, config)
	layerDigest := pkgImgUploadBlob(ctx, t, registryURL, repository, jwt, layer)

	manifest := fmt.Sprintf(
		`{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":"application/vnd.docker.container.image.v1+json","size":%d,"digest":%q},"layers":[{"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","size":%d,"digest":%q}]}`,
		pkgImgManifestMediaType, len(config), configDigest, len(layer), layerDigest)

	status, _, payload := pkgImgRequest(ctx, t, http.MethodPut,
		registryURL+"/v2/"+repository+"/manifests/"+tag,
		map[string]string{
			"Authorization": "Bearer " + jwt,
			"Content-Type":  pkgImgManifestMediaType,
		}, []byte(manifest))
	requireTruef(t, status == http.StatusCreated, "manifest push %q: status %d, body %s", tag, status, payload)
}

// TestMeta_PackageRegistryImages exercises the image-backed container
// registry actions through the gitlab_package meta-tool.
//
// The test pushes two distinct single-layer images (tags e2e-one and
// e2e-two) to a fresh project's registry repository purely over HTTP, then
// walks registry_get, registry_tag_list, registry_tag_get, a single
// registry_tag_delete, a regex registry_tag_delete_bulk for the remaining
// tag, and finally registry_delete for the whole repository. The repository
// ID is discovered through the already-covered registry_list_project action.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only, needs :5050 registry). Surface: meta.
func TestMeta_PackageRegistryImages(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("registry image pushes need the compose registry on :5050 (Docker mode only)")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	registryURL := pkgImgRegistryBaseURL(t)
	if !pkgImgRegistryReachable(ctx, registryURL) {
		t.Skipf("container registry at %s is unreachable", registryURL)
	}

	proj := CreateProjectMeta(ctx, e2e, sess.meta)
	// The registry always addresses repositories by the lowercased project
	// full path regardless of the project's display casing.
	repository := strings.ToLower(proj.Path)
	jwt := pkgImgJWT(ctx, t, repository)
	pkgImgPushImage(ctx, t, registryURL, repository, jwt, "e2e-one", "one")
	pkgImgPushImage(ctx, t, registryURL, repository, jwt, "e2e-two", "two")

	var repositoryID int64

	t.Run("DiscoverRepository", func(t *testing.T) {
		out, err := callToolOn[containerregistry.RepositoryListOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "registry_list_project")
		requireTruef(t, len(out.Repositories) == 1, "expected 1 registry repository, got %d", len(out.Repositories))
		repositoryID = out.Repositories[0].ID
		requireTruef(t, repositoryID > 0, "expected repository ID > 0")
	})

	t.Run("RegistryGet", func(t *testing.T) {
		requireTruef(t, repositoryID > 0, "repositoryID not set")
		out, err := callToolOn[containerregistry.RepositoryOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_get",
			"params": map[string]any{
				"repository_id": repositoryID,
				"tags_count":    true,
			},
		})
		requireNoError(t, err, "registry_get")
		requireTruef(t, out.ID == repositoryID, "expected repository ID %d, got %d", repositoryID, out.ID)
		requireTruef(t, out.TagsCount == 2, "expected 2 tags, got %d", out.TagsCount)
	})

	t.Run("TagList", func(t *testing.T) {
		requireTruef(t, repositoryID > 0, "repositoryID not set")
		out, err := callToolOn[containerregistry.TagListOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_tag_list",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"repository_id": repositoryID,
			},
		})
		requireNoError(t, err, "registry_tag_list")
		requireTruef(t, len(out.Tags) == 2, "expected 2 tags, got %d", len(out.Tags))
	})

	t.Run("TagGet", func(t *testing.T) {
		requireTruef(t, repositoryID > 0, "repositoryID not set")
		out, err := callToolOn[containerregistry.TagOutput](ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_tag_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"repository_id": repositoryID,
				"tag_name":      "e2e-one",
			},
		})
		requireNoError(t, err, "registry_tag_get")
		requireTruef(t, out.Name == "e2e-one", "expected tag e2e-one, got %q", out.Name)
		requireTruef(t, out.Digest != "", "expected non-empty tag digest")
	})

	t.Run("TagDelete", func(t *testing.T) {
		requireTruef(t, repositoryID > 0, "repositoryID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_tag_delete",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"repository_id": repositoryID,
				"tag_name":      "e2e-one",
			},
		})
		requireNoError(t, err, "registry_tag_delete")
	})

	t.Run("TagDeleteBulk", func(t *testing.T) {
		requireTruef(t, repositoryID > 0, "repositoryID not set")
		// The bulk deletion is asynchronous (202); success of the scheduling
		// call is the contract being tested.
		err := callToolVoidOn(ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_tag_delete_bulk",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"repository_id":     repositoryID,
				"name_regex_delete": "e2e-.*",
			},
		})
		requireNoError(t, err, "registry_tag_delete_bulk")
	})

	t.Run("RegistryDelete", func(t *testing.T) {
		requireTruef(t, repositoryID > 0, "repositoryID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_package", map[string]any{
			"action": "registry_delete",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"repository_id": repositoryID,
			},
		})
		requireNoError(t, err, "registry_delete")
	})
}
