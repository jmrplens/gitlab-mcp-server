package testutil

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// multipartRequest builds a POST carrying one text field and one file, the
// shape every upload fixture in the tree sends.
func multipartRequest(t *testing.T) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("operations", `{"query":"mutation"}`); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	part, err := w.CreateFormFile("file", "avatar.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, writeErr := part.Write([]byte("png bytes")); writeErr != nil {
		t.Fatalf("write part: %v", writeErr)
	}
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// TestReadMultipartForm_ReadsValuesAndFiles verifies that the form read
// through MultipartReader carries the field and the file the request sent,
// which is the contract every handler that used ParseMultipartForm relied on.
func TestReadMultipartForm_ReadsValuesAndFiles(t *testing.T) {
	form, err := ReadMultipartForm(multipartRequest(t), 1<<20)
	if err != nil {
		t.Fatalf("ReadMultipartForm: %v", err)
	}
	if got := FormValue(form, "operations"); got != `{"query":"mutation"}` {
		t.Errorf("FormValue(operations) = %q, want the field the request sent", got)
	}
	if got := FormValue(form, "absent"); got != "" {
		t.Errorf("FormValue(absent) = %q, want empty for a field the request did not send", got)
	}
	f, header, err := FormFile(form, "file")
	if err != nil {
		t.Fatalf("FormFile(file): %v", err)
	}
	defer func() { _ = f.Close() }()
	if header.Filename != "avatar.png" {
		t.Errorf("header.Filename = %q, want avatar.png", header.Filename)
	}
	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read file part: %v", err)
	}
	if string(content) != "png bytes" {
		t.Errorf("file content = %q, want the bytes the request sent", content)
	}
}

// TestFormFile_MissingFieldReportsErrMissingFile verifies that a field with no
// file answers the sentinel Request.FormFile would, so a handler ported from
// it keeps the same branch.
func TestFormFile_MissingFieldReportsErrMissingFile(t *testing.T) {
	form, err := ReadMultipartForm(multipartRequest(t), 1<<20)
	if err != nil {
		t.Fatalf("ReadMultipartForm: %v", err)
	}
	_, _, err = FormFile(form, "nope")
	if !errors.Is(err, http.ErrMissingFile) {
		t.Errorf("FormFile(nope) error = %v, want http.ErrMissingFile", err)
	}
}

// TestReadMultipartForm_NotMultipartFails verifies that a request without a
// multipart body is refused rather than read as an empty form, so a fixture
// that stops sending one fails its test instead of passing on nothing.
func TestReadMultipartForm_NotMultipartFails(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/upload", bytes.NewBufferString("plain"))
	req.Header.Set("Content-Type", "text/plain")
	if _, err := ReadMultipartForm(req, 1<<20); err == nil {
		t.Fatal("ReadMultipartForm accepted a body that is not multipart")
	}
}
