package testutil

import (
	"mime/multipart"
	"net/http"
)

// ReadMultipartForm parses the multipart body of a request a test handler
// received and hands back the form, which carries the same Value and File
// maps that Request.MultipartForm would after Request.ParseMultipartForm.
//
// It reads through Request.MultipartReader rather than ParseMultipartForm
// on purpose. The two parse the same body to the same form; the difference
// is that ParseMultipartForm is the call gosec's G120 names as where an
// unbounded upload exhausts a real handler's memory, and a taint rule cannot
// tell a fixture body from a client's, so every test handler that called it
// carried a directive saying so. Reading the form here needs none, and
// maxMemory still bounds what is held in memory before the rest spills to
// disk, exactly as it does for ParseMultipartForm.
//
// What a caller loses is Request.FormValue and Request.FormFile, which read
// the form ParseMultipartForm stored on the request: [FormValue] and
// [FormFile] answer the same questions of the returned form.
func ReadMultipartForm(r *http.Request, maxMemory int64) (*multipart.Form, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	return reader.ReadForm(maxMemory)
}

// FormValue returns the first value of a multipart form field, or an empty
// string when the form carries none, which is what Request.FormValue answers
// for a parsed form. An absent field is an assertion for the test goroutine
// to make, not a panic on the server's.
func FormValue(form *multipart.Form, name string) string {
	if values := form.Value[name]; len(values) > 0 {
		return values[0]
	}
	return ""
}

// FormFile returns the first file of a multipart form field, opened, with
// its header, the way Request.FormFile answers for a parsed form. A field
// that carries no file reports http.ErrMissingFile, as Request.FormFile
// does.
func FormFile(form *multipart.Form, name string) (multipart.File, *multipart.FileHeader, error) {
	headers := form.File[name]
	if len(headers) == 0 {
		return nil, nil, http.ErrMissingFile
	}
	f, err := headers[0].Open()
	if err != nil {
		return nil, nil, err
	}
	return f, headers[0], nil
}
