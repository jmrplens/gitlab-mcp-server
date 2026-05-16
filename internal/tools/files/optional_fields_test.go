// optional_fields_test.go covers optional repository file request fields.
package files

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

// TestFileCreate_OptionalFields covers optional fields in Create.
func TestFileCreate_OptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"file_path":"f.go","branch":"main"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID:     "1",
		Branch:        "main",
		CommitMessage: "add",
		Content:       "data",
		StartBranch:   "dev",
		Encoding:      "text",
		AuthorEmail:   "a@b.com",
		AuthorName:    "A",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFileUpdate_OptionalFields covers optional fields in Update.
func TestFileUpdate_OptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"file_path":"f.go","branch":"main"}`)
	})
	client := testutil.NewTestClient(t, mux)
	fm := true
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:       "1",
		FilePath:        "f.go",
		Branch:          "main",
		CommitMessage:   "up",
		Content:         "data",
		StartBranch:     "dev",
		Encoding:        "text",
		AuthorEmail:     "a@b.com",
		AuthorName:      "A",
		LastCommitID:    "abc",
		ExecuteFilemode: &fm,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFileDelete_OptionalFields covers optional fields in Delete.
func TestFileDelete_OptionalFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:     "1",
		FilePath:      "f.go",
		Branch:        "main",
		CommitMessage: "del",
		StartBranch:   "dev",
		AuthorEmail:   "a@b.com",
		AuthorName:    "A",
		LastCommitID:  "abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
