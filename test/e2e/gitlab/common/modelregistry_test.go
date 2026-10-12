//go:build e2e

// modelregistry_test.go covers the model registry's one action, the download
// of a model version file: refused as not found on every surface for a project
// that does not exist, and read back byte for byte from a private project's
// model version, for a file at the root of its package and for a nested one.

package common

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/modelregistry"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The project and the model version the download names, neither of which
// a fresh instance has.
const (
	missingModelProjectID = "999999"
	missingModelVersionID = "999999"
)

// TestModelRegistry_Download_MissingProjectRefused asks every surface for a
// model version file of a project that does not exist, and reads the
// refusal, which names the parameters to verify.
//
// Replaces: TestMeta_ModelRegistry
func TestModelRegistry_Download_MissingProjectRefused(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		refused := harness.Refused(s, actionModelRegistryDownload, map[string]any{
			"project_id": missingModelProjectID, "model_version_id": missingModelVersionID, "path": "/", "filename": "model.bin",
		}, harness.FailureNotFound)
		assertMentions(e, "the download of a model version from a project that does not exist", refused, "model_version_id")
	})
}

// The two files the private project's model version holds: one at the root of
// its package and one under sub/dir, each with content of its own so a
// download that reaches the other file is caught.
const (
	modelRootFile      = "model.bin"
	modelRootContent   = "root-model-file-content\n"
	modelNestedDir     = "sub/dir"
	modelNestedFile    = "w.bin"
	modelNestedContent = "nested-model-file-content\n"
)

// TestModelRegistry_Download_PrivateProjectFiles_AtTheRootAndNested downloads
// both files of a private project's model version on every surface, and holds
// each answer to the bytes uploaded.
//
// Two defects kept this from working. GitLab's model package route reads a
// token only from the Bearer header, so the PRIVATE-TOKEN this server sent
// left the request unauthenticated and a private project answered 404 for
// every file; and path was required, while a file at the package root has
// none, and the one spelling of the root client-go can send, files/./name, is
// answered 404 too. The root is asked for as an empty path and as ".", the
// second being what the meta surface's file_path alias hands the action for a
// bare file name.
func TestModelRegistry_Download_PrivateProjectFiles_AtTheRootAndNested(t *testing.T) {
	e := harness.New(t)
	project := fixture.NewProject(e, fixture.WithNamePrefix("ml"))
	versionID := createModelVersion(e, project)
	uploadModelFile(e, project, versionID, modelRootFile, modelRootContent)
	uploadModelFile(e, project, versionID, path.Join(modelNestedDir, modelNestedFile), modelNestedContent)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		assertModelDownload(e, s, project, versionID, "", modelRootFile, modelRootContent)
		assertModelDownload(e, s, project, versionID, ".", modelRootFile, modelRootContent)
		assertModelDownload(e, s, project, versionID, modelNestedDir, modelNestedFile, modelNestedContent)
	})
}

// assertModelDownload downloads one file of the model version, leaving path
// out of the call when it is empty, and holds the answer to want.
func assertModelDownload(e *harness.Env, s *harness.Session, project fixture.Project, versionID, dir, filename, want string) {
	e.T.Helper()
	params := map[string]any{
		"project_id": project.IDParam(), "model_version_id": versionID, "filename": filename,
	}
	if dir != "" {
		params["path"] = dir
	}
	out := harness.Do[modelregistry.DownloadOutput](s, actionModelRegistryDownload, params)
	got, err := base64.StdEncoding.DecodeString(out.ContentBase64)
	if err != nil {
		e.T.Errorf("path %q, file %s: content_base64 does not decode: %v", dir, filename, err)
		return
	}
	if string(got) != want {
		e.T.Errorf("path %q, file %s: downloaded %q, want %q", dir, filename, got, want)
	}
	if out.SizeBytes != len(want) {
		e.T.Errorf("path %q, file %s: size_bytes = %d, want %d", dir, filename, out.SizeBytes, len(want))
	}
}

// createModelVersion creates a model and a version of it in project through
// GraphQL, the only API that creates either, and returns the version's
// numeric ID, which is what the download route takes.
func createModelVersion(e *harness.Env, project fixture.Project) string {
	e.T.Helper()

	const createModel = `mutation($projectPath: ID!, $name: String!) {
		mlModelCreate(input: {projectPath: $projectPath, name: $name}) { model { id } errors }
	}`
	var model struct {
		MlModelCreate struct {
			Model  *struct{ ID string } `json:"model"`
			Errors []string             `json:"errors"`
		} `json:"mlModelCreate"`
	}
	modelMutation(e, "mlModelCreate", createModel, map[string]any{
		"projectPath": project.Path, "name": e.Name("model"),
	}, &model)
	if model.MlModelCreate.Model == nil || len(model.MlModelCreate.Errors) > 0 {
		e.T.Fatalf("mlModelCreate created no model: %v", model.MlModelCreate.Errors)
	}

	const createVersion = `mutation($projectPath: ID!, $modelId: MlModelID!) {
		mlModelVersionCreate(input: {projectPath: $projectPath, modelId: $modelId, version: "1.0.0"}) { modelVersion { id } errors }
	}`
	var version struct {
		MlModelVersionCreate struct {
			ModelVersion *struct{ ID string } `json:"modelVersion"`
			Errors       []string             `json:"errors"`
		} `json:"mlModelVersionCreate"`
	}
	modelMutation(e, "mlModelVersionCreate", createVersion, map[string]any{
		"projectPath": project.Path, "modelId": model.MlModelCreate.Model.ID,
	}, &version)
	if version.MlModelVersionCreate.ModelVersion == nil || len(version.MlModelVersionCreate.Errors) > 0 {
		e.T.Fatalf("mlModelVersionCreate created no version: %v", version.MlModelVersionCreate.Errors)
	}
	// The global ID ends in the numeric one: gid://gitlab/Ml::ModelVersion/1.
	return path.Base(version.MlModelVersionCreate.ModelVersion.ID)
}

// modelMutation sends one model registry mutation and decodes the data half
// of GitLab's answer into into, failing the test with GitLab's reason when it
// refused the document.
//
// client-go's GraphQL.Do decodes the whole answer, the data and errors halves
// together, and returns no error for a document GitLab refused with a 200, so
// a struct shaped like the data alone reads nothing: on CE 19.4.2 the model
// was created and the scenario reported that it was not.
func modelMutation(e *harness.Env, name, document string, variables map[string]any, into any) {
	e.T.Helper()
	var answer struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if _, err := e.Client().GL().GraphQL.Do(gl.GraphQLQuery{Query: document, Variables: variables}, &answer, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("%s: %v", name, err)
	}
	if len(answer.Errors) > 0 {
		e.T.Fatalf("%s was refused: %+v", name, answer.Errors)
	}
	if err := json.Unmarshal(answer.Data, into); err != nil {
		e.T.Fatalf("%s answered data that does not decode: %v", name, err)
	}
}

// uploadModelFile puts content at filePath in the model version's package.
// The route takes the file as the raw body of a PUT, which client-go has no
// method for, and reads the token from the Bearer header alone, as the
// download does.
func uploadModelFile(e *harness.Env, project fixture.Project, versionID, filePath, content string) {
	e.T.Helper()
	client := e.Client()
	route := fmt.Sprintf("projects/%d/packages/ml_models/%s/files/%s", project.ID, versionID, filePath)
	// Built as a GET so client-go encodes no body, then turned into the PUT
	// with the file as its body, as client-go's own generic package upload
	// does.
	req, err := client.GL().NewRequest(http.MethodGet, route, nil,
		[]gl.RequestOptionFunc{gl.WithContext(e.Ctx), client.BearerToken()})
	if err != nil {
		e.T.Fatalf("building the upload of %s: %v", filePath, err)
	}
	req.Method = http.MethodPut
	if err = req.SetBody([]byte(content)); err != nil {
		e.T.Fatalf("setting the body of the upload of %s: %v", filePath, err)
	}
	if _, err = client.GL().Do(req, nil); err != nil {
		e.T.Fatalf("uploading %s: %v", filePath, err)
	}
}
