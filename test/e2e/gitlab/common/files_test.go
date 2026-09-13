//go:build e2e

// files_test.go covers a repository file through the server: the reads,
// which classify what they decode as text, image or binary and answer the
// content only for text, and the create, update and delete of one file.

package common

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The text file every surface reads, and the binary content the read has to
// refuse to decode into text.
const (
	textFileContent   = "package main\n\nfunc hello() string { return \"world\" }\n"
	binaryFileContent = "%PDF-1.4 fake binary content"
)

// The content categories the file reads answer with.
const (
	categoryText   = "text"
	categoryImage  = "image"
	categoryBinary = "binary"
)

// createFileBase64 creates a file through the server from base64 content,
// the way an image or a binary has to be sent, and checks the answer names
// the file on the branch it was created on.
func createFileBase64(e *harness.Env, s *harness.Session, project fixture.Project, path, content string) {
	e.T.Helper()

	created := harness.Do[files.FileInfoOutput](s, actionRepositoryFileCreate, map[string]any{
		"project_id": project.IDParam(), "file_path": path, "branch": project.DefaultBranch,
		"content": content, "encoding": "base64", "commit_message": "chore: add " + path,
	})
	if created.FilePath != path || created.Branch != project.DefaultBranch {
		e.T.Fatalf("file create answered %+v, want %s on %s", created, path, project.DefaultBranch)
	}
}

// assertFileGetClassifies reads the three files through the full read,
// which classifies each and decodes the text alone.
func assertFileGetClassifies(e *harness.Env, s *harness.Session, params map[string]any, textPath, imagePath, binaryPath string, textCommit fixture.Commit) {
	e.T.Helper()

	text := harness.Do[files.Output](s, actionRepositoryFileGet, withParams(params, map[string]any{"file_path": textPath}))
	if text.FileName != "hello.go" || text.ContentCategory != categoryText || text.Content != textFileContent || text.Size != int64(len(textFileContent)) || text.LastCommitID != textCommit.SHA {
		e.T.Errorf("file get of %s answered %+v, want the text file of %d bytes last written by %s", textPath, text, len(textFileContent), textCommit.ShortID)
	}
	image := harness.Do[files.Output](s, actionRepositoryFileGet, withParams(params, map[string]any{"file_path": imagePath}))
	if image.ContentCategory != categoryImage || image.Content != "" || image.Size == 0 {
		e.T.Errorf("file get of %s answered category %q with %d bytes of content, want an image with no decoded content", imagePath, image.ContentCategory, len(image.Content))
	}
	binary := harness.Do[files.Output](s, actionRepositoryFileGet, withParams(params, map[string]any{"file_path": binaryPath}))
	if binary.ContentCategory != categoryBinary || binary.Content != "" {
		e.T.Errorf("file get of %s answered category %q with %d bytes of content, want a binary with no decoded content", binaryPath, binary.ContentCategory, len(binary.Content))
	}
}

// assertFileRawReads reads the text and the image through the raw read,
// which classifies both and decodes the text alone.
func assertFileRawReads(e *harness.Env, s *harness.Session, params map[string]any, textPath, imagePath string) {
	e.T.Helper()

	rawText := harness.Do[files.RawOutput](s, actionRepositoryFileRaw, withParams(params, map[string]any{"file_path": textPath}))
	if rawText.ContentCategory != categoryText || rawText.Content != textFileContent || rawText.Size != len(textFileContent) {
		e.T.Errorf("raw read of %s answered %+v, want the text file's content", textPath, rawText)
	}
	rawImage := harness.Do[files.RawOutput](s, actionRepositoryFileRaw, withParams(params, map[string]any{"file_path": imagePath}))
	if rawImage.ContentCategory != categoryImage || rawImage.Content != "" || rawImage.Size == 0 {
		e.T.Errorf("raw read of %s answered %+v, want an image with a size and no decoded content", imagePath, rawImage)
	}
}

// assertFileDescribed reads the text file's metadata both ways and its
// blame, which attributes the one range to the commit that wrote it.
func assertFileDescribed(e *harness.Env, s *harness.Session, params map[string]any, textPath string, textCommit fixture.Commit) {
	e.T.Helper()

	metadata := harness.Do[files.MetaDataOutput](s, actionRepositoryFileMetadata, withParams(params, map[string]any{"file_path": textPath}))
	if metadata.FileName != "hello.go" || metadata.Size != int64(len(textFileContent)) || metadata.BlobID == "" || metadata.SHA256 == "" {
		e.T.Errorf("metadata of %s answered %+v, want the name, the size, a blob id and a content sha256", textPath, metadata)
	}
	rawMetadata := harness.Do[files.MetaDataOutput](s, actionRepositoryFileRawMetadata, withParams(params, map[string]any{"file_path": textPath}))
	if rawMetadata.FileName != "hello.go" || rawMetadata.Size != int64(len(textFileContent)) || rawMetadata.SHA256 != metadata.SHA256 {
		e.T.Errorf("raw metadata of %s answered %+v, want the same name, size and sha256 as the metadata read", textPath, rawMetadata)
	}

	blame := harness.Do[files.BlameOutput](s, actionRepositoryFileBlame, withParams(params, map[string]any{"file_path": textPath}))
	if blame.FilePath != textPath || len(blame.Ranges) == 0 || blame.Ranges[0].Commit.ID != textCommit.SHA {
		e.T.Errorf("blame of %s answered %+v, want a range attributed to %s", textPath, blame, textCommit.ShortID)
	}
}

// TestRepositoryFile_Reads_ClassifyTextImageAndBinary commits a text file
// beside an image and a binary created through the server from base64,
// then reads all three on every surface: the full read classifies each and
// decodes the text alone, the raw read does the same for text and image,
// and the metadata, raw metadata and blame reads describe the text file.
//
// Replaces: TestIndividual_Files, TestMeta_Files
func TestRepositoryFile_Reads_ClassifyTextImageAndBinary(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("filereads"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		dir := string(surface) + "/"
		textPath, imagePath, binaryPath := dir+"hello.go", dir+"pixel.png", dir+"report.pdf"

		textCommit := fixture.CommitFile(e, project, project.DefaultBranch, textPath, textFileContent, "chore: add "+textPath)
		createFileBase64(e, s, project, imagePath, fixture.PNGBase64(e))
		createFileBase64(e, s, project, binaryPath, base64.StdEncoding.EncodeToString([]byte(binaryFileContent)))
		params := map[string]any{"project_id": project.IDParam(), "ref": project.DefaultBranch}

		assertFileGetClassifies(e, s, params, textPath, imagePath, binaryPath, textCommit)
		assertFileRawReads(e, s, params, textPath, imagePath)
		assertFileDescribed(e, s, params, textPath, textCommit)
	})
}

// TestRepositoryFile_Lifecycle_CreateUpdateDelete creates a text file on
// every surface, reads it back, updates it, reads the new content back,
// deletes it and asserts the read afterwards is refused as not found.
//
// Replaces: TestIndividual_Files, TestMeta_Files, TestMeta_RepositoryFiles
func TestRepositoryFile_Lifecycle_CreateUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("filecrud"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		path := "crud-" + string(surface) + ".txt"
		write := map[string]any{"project_id": project.IDParam(), "file_path": path, "branch": project.DefaultBranch}
		read := map[string]any{"project_id": project.IDParam(), "file_path": path, "ref": project.DefaultBranch}

		created := harness.Do[files.FileInfoOutput](s, actionRepositoryFileCreate, withParams(write, map[string]any{
			"content": "initial content", "commit_message": "chore: create " + path,
		}))
		if created.FilePath != path || created.Branch != project.DefaultBranch {
			e.T.Fatalf("file create answered %+v, want %s on %s", created, path, project.DefaultBranch)
		}
		initial := harness.Do[files.Output](s, actionRepositoryFileGet, read)
		if initial.Content != "initial content" {
			e.T.Errorf("the created file reads back as %q, want the initial content", initial.Content)
		}

		updated := harness.Do[files.FileInfoOutput](s, actionRepositoryFileUpdate, withParams(write, map[string]any{
			"content": "updated content", "commit_message": "chore: update " + path,
		}))
		if updated.FilePath != path || updated.Branch != project.DefaultBranch {
			e.T.Errorf("file update answered %+v, want %s on %s", updated, path, project.DefaultBranch)
		}
		current := harness.Do[files.Output](s, actionRepositoryFileGet, read)
		if !strings.Contains(current.Content, "updated") {
			e.T.Errorf("the updated file reads back as %q, want the updated content", current.Content)
		}

		harness.DoVoid(s, actionRepositoryFileDelete, withParams(write, map[string]any{"commit_message": "chore: delete " + path}))
		refused := harness.Refused(s, actionRepositoryFileGet, read, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}
