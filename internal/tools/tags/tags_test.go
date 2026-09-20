// tags_test.go contains unit tests for GitLab tag operations (create, delete,
// get, list, signature, protected tag CRUD). Tests use httptest to mock the
// GitLab API and verify success, error, canceled-context, and markdown paths.
package tags

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Test endpoint paths and fixture values used across tag operation tests.
const (
	errExpEmptyProjectID = "expected error for empty project_id, got nil"
	errExpEmptyTagName   = "expected error for empty tag_name, got nil"
	errExpAPIFailure     = "expected error for API failure"
	pathRepoTags         = "/api/v4/projects/42/repository/tags"
	testTagName          = "v1.2.0"
	testTagMessage       = "Release v1.2.0"
	testTagV100          = "v1.0.0"
	testReleaseName      = "Release v1.0.0"
	testEmailAddr        = "test@example.com"
	testCRLURL           = "https://example.com/crl"

	errCancelledCtx = "expected error for canceled context"
	descMaintainers = "Maintainers"
	argProjectID    = "project_id"
	argTagName      = "tag_name"
	fmtNameWant     = "out.Name = %q, want %q"
	fmtListUnexpErr = "List() unexpected error: %v"
)

// TestTagCreate_Success verifies that Create creates an annotated tag and
// returns the correct name, target commit, and message. The mock returns
// HTTP 201 with a valid tag JSON response.
func TestTagCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathRepoTags {
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v1.2.0","target":"abc123def456","message":"Release v1.2.0","protected":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   testTagName,
		Ref:       "main",
		Message:   testTagMessage,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Name != testTagName {
		t.Errorf(fmtNameWant, out.Name, testTagName)
	}
	if out.Target != "abc123def456" {
		t.Errorf("out.Target = %q, want %q", out.Target, "abc123def456")
	}
	if out.Message != testTagMessage {
		t.Errorf("out.Message = %q, want %q", out.Message, testTagMessage)
	}
}

// TestTagCreate_InvalidRef verifies that Create returns an error when the
// ref (branch or commit) does not exist. The mock returns HTTP 404.
func TestTagCreate_InvalidRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Ref Not Found"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		TagName:   testTagV100,
		Ref:       "nonexistent-branch",
	})
	if err == nil {
		t.Fatal("Create() expected error for invalid ref, got nil")
	}
}

// decodeJSONBody reads a request body as a JSON object, reporting on the test
// goroutine and answering deterministically when it cannot, since an httptest
// handler may not abort the test.
func decodeJSONBody(t *testing.T, w http.ResponseWriter, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode request body: %v", err)
		http.Error(w, "undecodable request body", http.StatusBadRequest)
		return nil
	}
	return body
}

// TestTagCreate_RequestBody pins what Create asks GitLab to do: the tag name,
// the ref it points at, and the annotated message, each read back from the
// request body.
//
// No test read that body before, so all three could be read off one another —
// creating a tag named after its own ref, annotated with the ref — and the
// suite stayed green; the message guard was a live mutant for the same reason.
// The message is given with a literal backslash-n because that is what an MCP
// client's double-escaped input looks like, and it must arrive as a newline.
func TestTagCreate_RequestBody(t *testing.T) {
	cases := []struct {
		name  string
		input CreateInput
		want  map[string]any
	}{
		{
			name:  "an annotated tag carries the caller's message",
			input: CreateInput{ProjectID: "42", TagName: testTagName, Ref: "release-branch", Message: `Ship it\nand the line after`},
			want:  map[string]any{"tag_name": testTagName, "ref": "release-branch", "message": "Ship it\nand the line after"},
		},
		{
			name:  "a lightweight tag sends no message at all",
			input: CreateInput{ProjectID: "42", TagName: testTagV100, Ref: "9f8e7d6"},
			want:  map[string]any{"tag_name": testTagV100, "ref": "9f8e7d6"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != pathRepoTags {
					http.NotFound(w, r)
					return
				}
				body = decodeJSONBody(t, w, r)
				if body == nil {
					return
				}
				testutil.RespondJSON(w, http.StatusCreated, `{"name":"v1.2.0","target":"abc123def456"}`)
			}))

			if _, err := Create(context.Background(), client, c.input); err != nil {
				t.Fatalf("Create() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(body, c.want) {
				t.Errorf("request body = %#v, want %#v", body, c.want)
			}
		})
	}
}

// TestTagDelete_Success verifies that Delete removes a tag without error.
// The mock returns HTTP 204 No Content for the correct DELETE path.
func TestTagDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/repository/tags/v1.2.0" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	if err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		TagName:   testTagName,
	}); err != nil {
		t.Errorf("Delete() unexpected error: %v", err)
	}
}

// TestTagDelete_NotFound verifies that Delete returns an error when the
// target tag does not exist. The mock returns HTTP 404.
func TestTagDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Tag Not Found"}`)
	}))

	if err := Delete(context.Background(), client, DeleteInput{
		ProjectID: "42",
		TagName:   "nonexistent",
	}); err == nil {
		t.Fatal("Delete() expected error for non-existent tag, got nil")
	}
}

// TestTagList_Success verifies that List returns multiple tags with their
// attributes correctly mapped from the GitLab API response.
func TestTagList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoTags {
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v1.2.0","target":"abc123","message":null,"protected":false},{"name":"v1.1.0","target":"def456","message":null,"protected":false}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtListUnexpErr, err)
	}
	if len(out.Tags) != 2 {
		t.Errorf("len(out.Tags) = %d, want 2", len(out.Tags))
	}
	if out.Tags[0].Name != testTagName {
		t.Errorf("out.Tags[0].Name = %q, want %q", out.Tags[0].Name, testTagName)
	}
}

// TestTagList_Empty verifies that List handles an empty API response
// gracefully, returning zero tags without error.
func TestTagList_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("List() unexpected error for empty list: %v", err)
	}
	if len(out.Tags) != 0 {
		t.Errorf("len(out.Tags) = %d, want 0", len(out.Tags))
	}
}

// TestTagList_PaginationQueryParamsAndMetadata verifies that List forwards
// page and per_page query parameters and correctly parses pagination metadata
// (TotalItems, NextPage, PrevPage) from the GitLab response headers.
func TestTagList_PaginationQueryParamsAndMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoTags {
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Errorf("query param page = %q, want %q", got, "1")
			}
			if got := r.URL.Query().Get("per_page"); got != "3" {
				t.Errorf("query param per_page = %q, want %q", got, "3")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"v1.0.0","target":"aaa","message":"","protected":false}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "3", Total: "7", TotalPages: "3", NextPage: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", Page: 1, PerPage: 3})
	if err != nil {
		t.Fatalf(fmtListUnexpErr, err)
	}
	if out.Pagination.TotalItems != 7 {
		t.Errorf("Pagination.TotalItems = %d, want 7", out.Pagination.TotalItems)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("Pagination.NextPage = %d, want 2", out.Pagination.NextPage)
	}
	if out.Pagination.PrevPage != 0 {
		t.Errorf("Pagination.PrevPage = %d, want 0", out.Pagination.PrevPage)
	}
}

// TestTagGet_Success verifies that Get retrieves a single tag by name.
func TestTagGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTags+"/v1.0.0" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"name":"v1.0.0",
				"message":"Release v1.0.0",
				"target":"abc123",
				"commit":{
					"id":"abc123","short_id":"abc123d","title":"Release v1.0.0",
					"author_name":"Test","committed_date":"2026-03-01T10:00:00Z",
					"web_url":"https://gitlab.example.com/-/commit/abc123"
				},
				"protected":false
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   testTagV100,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Name != testTagV100 {
		t.Errorf(fmtNameWant, out.Name, testTagV100)
	}
	if out.Message != testReleaseName {
		t.Errorf("out.Message = %q, want %q", out.Message, testReleaseName)
	}
}

// TestTagGet_SuccessEnrichedFields verifies that Get maps the full nested commit
// object, the release note, and CreatedAt from the tag payload.
//
// No two values in the fixture agree and every timestamp is asserted by value,
// because the converter is straight-line assignment that no mutation or
// condition gate can judge: while the author and the committer shared a name
// and an email, and the commit's authored and created dates were the same
// instant, reading either field off its neighbor failed nothing.
func TestTagGet_SuccessEnrichedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTags+"/v2.0.0" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"name":"v2.0.0",
				"message":"Annotated tag",
				"target":"aaa111",
				"commit":{
					"id":"aaa111bbb222","short_id":"aaa111b","title":"feat",
					"message":"feat: new feature","author_name":"Ada Author","author_email":"ada@example.com",
					"committer_name":"Cid Committer","committer_email":"cid@example.com",
					"authored_date":"2026-06-15T08:00:00Z","committed_date":"2026-06-15T08:10:00Z",
					"created_at":"2026-06-15T07:50:00Z","web_url":"https://gitlab.example.com/-/commit/aaa111bbb222",
					"parent_ids":["zzz000"],"project_id":42,"status":"success",
					"trailers":{"Signed-off-by":"Dev"},"extended_trailers":{"Signed-off-by":"Dev"},
					"stats":{"additions":3,"deletions":1,"total":4}
				},
				"release":{"tag_name":"v2.0.0","description":"Second major release"},
				"protected":true,
				"created_at":"2026-06-15T08:30:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   "v2.0.0",
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Commit == nil {
		t.Fatal("out.Commit is nil, want commit object")
	}
	if out.Release == nil {
		t.Fatal("out.Release is nil, want release object")
	}
	stringChecks := map[string][2]string{
		"Name":                  {out.Name, "v2.0.0"},
		"Target":                {out.Target, "aaa111"},
		"Message":               {out.Message, "Annotated tag"},
		"CreatedAt":             {out.CreatedAt, "2026-06-15T08:30:00Z"},
		"Commit.ID":             {out.Commit.ID, "aaa111bbb222"},
		"Commit.ShortID":        {out.Commit.ShortID, "aaa111b"},
		"Commit.Title":          {out.Commit.Title, "feat"},
		"Commit.Message":        {out.Commit.Message, "feat: new feature"},
		"Commit.AuthorName":     {out.Commit.AuthorName, "Ada Author"},
		"Commit.AuthorEmail":    {out.Commit.AuthorEmail, "ada@example.com"},
		"Commit.CommitterName":  {out.Commit.CommitterName, "Cid Committer"},
		"Commit.CommitterEmail": {out.Commit.CommitterEmail, "cid@example.com"},
		"Commit.AuthoredDate":   {out.Commit.AuthoredDate, "2026-06-15T08:00:00Z"},
		"Commit.CommittedDate":  {out.Commit.CommittedDate, "2026-06-15T08:10:00Z"},
		"Commit.CreatedAt":      {out.Commit.CreatedAt, "2026-06-15T07:50:00Z"},
		"Release.TagName":       {out.Release.TagName, "v2.0.0"},
		"Release.Desc":          {out.Release.Description, "Second major release"},
	}
	for field, pair := range stringChecks {
		t.Run(field, func(t *testing.T) {
			if pair[0] != pair[1] {
				t.Errorf("%s = %q, want %q", field, pair[0], pair[1])
			}
		})
	}
	if !out.Protected {
		t.Error("out.Protected = false, want true")
	}
	if len(out.Commit.ParentIDs) != 1 || out.Commit.ParentIDs[0] != "zzz000" {
		t.Errorf("out.Commit.ParentIDs = %v, want [zzz000]", out.Commit.ParentIDs)
	}
}

// TestTagGet_CommitSubsetIgnoresUndocumentedFields verifies that the documented
// reference subset CommitOutput silently ignores commit fields the tags API does
// not document (web_url, status, project_id, trailers, stats, last_pipeline),
// providing version tolerance when GitLab returns a richer commit object.
func TestTagGet_CommitSubsetIgnoresUndocumentedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTags+"/v3.0.0" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"name":"v3.0.0","target":"ccc333","protected":false,
				"commit":{
					"id":"ccc333","short_id":"ccc333a","title":"chore",
					"web_url":"https://gitlab.example.com/-/commit/ccc333","status":"success",
					"project_id":99,"trailers":{"k":"v"},"extended_trailers":{"k":"v"},
					"stats":{"additions":1,"deletions":0,"total":1},
					"last_pipeline":{"id":7,"ref":"main","status":"success"}
				}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", TagName: "v3.0.0"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Commit == nil {
		t.Fatal("out.Commit is nil, want commit object")
	}
	if out.Commit.ID != "ccc333" {
		t.Errorf("out.Commit.ID = %q, want %q", out.Commit.ID, "ccc333")
	}
}

// TestTagGet_EmptyProjectID verifies Get refuses an empty project_id before it
// reaches GitLab: the mock fails the test if any request arrives, so the
// refusal cannot be something the server answered.
func TestTagGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Get(context.Background(), client, GetInput{TagName: testTagV100})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagGet_APIError verifies Get returns an error on API failure.
func TestTagGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Tag Not Found"}`)
	}))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		TagName:   "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetSignature tests
// ---------------------------------------------------------------------------.

// pathRepoTagSig identifies the path repo tag sig constant used by this package.
const pathRepoTagSig = "/api/v4/projects/42/repository/tags/v1.0.0/signature"

// TestTagGetSignature_Success pins the whole signature GetSignature builds: the
// certificate and its issuer carry different ids, subjects and key identifiers,
// and the output is compared as one value.
//
// Four of the twelve fields were asserted before, which left the certificate's
// own subject readable off its key identifier and the issuer's off the
// certificate's — straight-line assignment no gate can judge — and left the
// serial number, which GitLab sends as a number and this package prints, unread
// by anything driving the handler.
func TestTagGetSignature_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTagSig {
			testutil.RespondJSON(w, http.StatusOK, `{
				"signature_type": "X509",
				"verification_status": "verified",
				"x509_certificate": {
					"id": 11,
					"subject": "CN=Test",
					"subject_key_identifier": "abc123",
					"email": "test@example.com",
					"serial_number": 12345,
					"certificate_status": "good",
					"x509_issuer": {
						"id": 22,
						"subject": "CN=Issuer",
						"subject_key_identifier": "def456",
						"crl_url": "https://example.com/crl"
					}
				}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetSignature(context.Background(), client, SignatureInput{
		ProjectID: "42",
		TagName:   testTagV100,
	})
	if err != nil {
		t.Fatalf("GetSignature() unexpected error: %v", err)
	}

	want := SignatureOutput{
		SignatureType:      "X509",
		VerificationStatus: "verified",
		X509Certificate: X509CertificateOutput{
			ID:                   11,
			Subject:              "CN=Test",
			SubjectKeyIdentifier: "abc123",
			Email:                testEmailAddr,
			SerialNumber:         "12345",
			CertificateStatus:    "good",
			X509Issuer: X509IssuerOutput{
				ID:                   22,
				Subject:              "CN=Issuer",
				SubjectKeyIdentifier: "def456",
				CrlURL:               testCRLURL,
			},
		},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("signature = %#v, want %#v", out, want)
	}
}

// TestTagGetSignature_NoSerialNumber pins that a certificate GitLab sent no
// serial number for publishes an empty one rather than the "<nil>" that
// formatting a nil big.Int would print.
//
// The guard that decides it had only ever been evaluated true, so both gates
// reported it: inverting it changed nothing any test could see.
func TestTagGetSignature_NoSerialNumber(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTagSig {
			testutil.RespondJSON(w, http.StatusOK, `{
				"signature_type": "X509",
				"verification_status": "unverified",
				"x509_certificate": {"id": 11, "subject": "CN=Test", "serial_number": null}
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetSignature(context.Background(), client, SignatureInput{ProjectID: "42", TagName: testTagV100})
	if err != nil {
		t.Fatalf("GetSignature() unexpected error: %v", err)
	}
	if out.X509Certificate.SerialNumber != "" {
		t.Errorf("SerialNumber = %q, want empty", out.X509Certificate.SerialNumber)
	}
	if out.X509Certificate.Subject != "CN=Test" {
		t.Errorf("Subject = %q, want %q", out.X509Certificate.Subject, "CN=Test")
	}
}

// TestTagGetSignature_EmptyProjectID verifies GetSignature refuses an empty
// project_id before reaching GitLab.
func TestTagGetSignature_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetSignature(context.Background(), client, SignatureInput{TagName: testTagV100})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagGetSignature_EmptyTagName verifies GetSignature refuses an empty
// tag_name before reaching GitLab.
func TestTagGetSignature_EmptyTagName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetSignature(context.Background(), client, SignatureInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyTagName)
	}
}

// ---------------------------------------------------------------------------
// Protected Tags tests
// ---------------------------------------------------------------------------.

// pathProtectedTags identifies the path protected tags constant used by this package.
const pathProtectedTags = "/api/v4/projects/42/protected_tags"

// TestTagListProtected_Success verifies TagListProtected when success.
func TestTagListProtected_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedTags {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"name":"v*","create_access_levels":[{"id":1,"access_level":40,"access_level_description":"Maintainers"}]},
				{"name":"release-*","create_access_levels":[]}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("ListProtectedTags() unexpected error: %v", err)
	}
	if len(out.Tags) != 2 {
		t.Fatalf("len(out.Tags) = %d, want 2", len(out.Tags))
	}
	if out.Tags[0].Name != "v*" {
		t.Errorf("out.Tags[0].Name = %q, want %q", out.Tags[0].Name, "v*")
	}
	if len(out.Tags[0].CreateAccessLevels) != 1 {
		t.Fatalf("len(CreateAccessLevels) = %d, want 1", len(out.Tags[0].CreateAccessLevels))
	}
	if out.Tags[0].CreateAccessLevels[0].AccessLevel != 40 {
		t.Errorf("AccessLevel = %d, want 40", out.Tags[0].CreateAccessLevels[0].AccessLevel)
	}
}

// TestTagListProtected_EmptyProjectID verifies ListProtectedTags refuses an
// empty project_id before reaching GitLab.
func TestTagListProtected_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagGetProtected_Success verifies TagGetProtected when success.
func TestTagGetProtected_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedTags+"/v*" {
			testutil.RespondJSON(w, http.StatusOK, `{"name":"v*","create_access_levels":[{"id":1,"access_level":40,"access_level_description":"Maintainers"}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{ProjectID: "42", TagName: "v*"})
	if err != nil {
		t.Fatalf("GetProtectedTag() unexpected error: %v", err)
	}
	if out.Name != "v*" {
		t.Errorf(fmtNameWant, out.Name, "v*")
	}
}

// TestTagGetProtected_MapsEachAccessLevelIdentifier pins that every identifier
// on a create access level arrives under its own name, with the rule id, the
// access level and the three principal ids all different numbers.
//
// The converter is straight-line assignment, so no mutation or condition gate
// can judge it, and the only fixture that reached it through a handler left
// user, group and deploy key at zero: reading any of the five off its
// neighbor failed nothing.
func TestTagGetProtected_MapsEachAccessLevelIdentifier(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedTags+"/v*" {
			testutil.RespondJSON(w, http.StatusOK, `{"name":"v*","create_access_levels":[
				{"id":71,"access_level":40,"access_level_description":"Maintainers","user_id":5,"group_id":10,"deploy_key_id":3}
			]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{ProjectID: "42", TagName: "v*"})
	if err != nil {
		t.Fatalf("GetProtectedTag() unexpected error: %v", err)
	}

	want := ProtectedTagOutput{
		Name: "v*",
		CreateAccessLevels: []TagAccessLevelOutput{{
			ID:                     71,
			AccessLevel:            40,
			AccessLevelDescription: descMaintainers,
			UserID:                 5,
			GroupID:                10,
			DeployKeyID:            3,
		}},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("protected tag = %#v, want %#v", out, want)
	}
}

// TestTagGetProtected_EmptyProjectID verifies GetProtectedTag refuses an empty
// project_id before reaching GitLab.
func TestTagGetProtected_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{TagName: "v*"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagGetProtected_EmptyTagName verifies GetProtectedTag refuses an empty
// tag_name before reaching GitLab.
func TestTagGetProtected_EmptyTagName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyTagName)
	}
}

// TestTagProtect_Success verifies TagProtect when success.
func TestTagProtect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedTags {
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v*","create_access_levels":[{"id":1,"access_level":30,"access_level_description":"Developers + Maintainers"}]}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ProtectTag(context.Background(), client, ProtectTagInput{
		ProjectID:         "42",
		TagName:           "v*",
		CreateAccessLevel: 30,
	})
	if err != nil {
		t.Fatalf("ProtectTag() unexpected error: %v", err)
	}
	if out.Name != "v*" {
		t.Errorf(fmtNameWant, out.Name, "v*")
	}
	if len(out.CreateAccessLevels) != 1 {
		t.Fatalf("len(CreateAccessLevels) = %d, want 1", len(out.CreateAccessLevels))
	}
}

// TestTagProtect_TagNameMapsToSDKName verifies that the MCP tag_name input is
// forwarded to the GitLab API as the SDK `name` field (a deliberate rename: the
// MCP surface uses tag_name throughout for clarity).
func TestTagProtect_TagNameMapsToSDKName(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathProtectedTags {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v*","create_access_levels":[]}`)
			return
		}
		http.NotFound(w, r)
	}))

	if _, err := ProtectTag(context.Background(), client, ProtectTagInput{
		ProjectID: "42",
		TagName:   "v*",
	}); err != nil {
		t.Fatalf("ProtectTag() unexpected error: %v", err)
	}
	if !strings.Contains(body, `"name":"v*"`) {
		t.Errorf("request body = %q, want SDK name field carrying tag_name", body)
	}
}

// TestTagProtect_EmptyProjectID verifies ProtectTag refuses an empty
// project_id before reaching GitLab.
func TestTagProtect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ProtectTag(context.Background(), client, ProtectTagInput{TagName: "v*"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagProtect_EmptyName verifies ProtectTag refuses an empty tag_name
// before reaching GitLab.
func TestTagProtect_EmptyName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := ProtectTag(context.Background(), client, ProtectTagInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyTagName)
	}
}

// TestTagUnprotect_Success verifies TagUnprotect when success.
func TestTagUnprotect_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathProtectedTags+"/v*" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := UnprotectTag(context.Background(), client, UnprotectTagInput{ProjectID: "42", TagName: "v*"})
	if err != nil {
		t.Fatalf("UnprotectTag() unexpected error: %v", err)
	}
}

// TestTagUnprotect_EmptyProjectID verifies UnprotectTag refuses an empty
// project_id before reaching GitLab.
func TestTagUnprotect_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := UnprotectTag(context.Background(), client, UnprotectTagInput{TagName: "v*"})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
}

// TestTagUnprotect_EmptyTagName verifies UnprotectTag refuses an empty
// tag_name before reaching GitLab.
func TestTagUnprotect_EmptyTagName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := UnprotectTag(context.Background(), client, UnprotectTagInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpEmptyTagName)
	}
}

// ---------------------------------------------------------------------------
// Canceled context tests for ALL functions
// ---------------------------------------------------------------------------.

// TestTagCreate_CancelledContext verifies TagCreate when cancelled context.
func TestTagCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", TagName: "v0", Ref: "main"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagDelete_CancelledContext verifies TagDelete when cancelled context.
func TestTagDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", TagName: "v0"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagList_CancelledContext verifies TagList when cancelled context.
func TestTagList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagGet_CancelledContext verifies TagGet when cancelled context.
func TestTagGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", TagName: "v0"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagGetSignature_CancelledContext verifies TagGetSignature when cancelled context.
func TestTagGetSignature_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetSignature(ctx, client, SignatureInput{ProjectID: "42", TagName: "v0"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagListProtected_CancelledContext verifies TagListProtected when cancelled context.
func TestTagListProtected_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProtectedTags(ctx, client, ListProtectedTagsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagGetProtected_CancelledContext verifies TagGetProtected when cancelled context.
func TestTagGetProtected_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetProtectedTag(ctx, client, GetProtectedTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagProtect_CancelledContext verifies TagProtect when cancelled context.
func TestTagProtect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ProtectTag(ctx, client, ProtectTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestTagUnprotect_CancelledContext verifies TagUnprotect when cancelled context.
func TestTagUnprotect_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := UnprotectTag(ctx, client, UnprotectTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Empty ProjectID for Create and Delete
// ---------------------------------------------------------------------------.

// TestTagCreate_EmptyProjectID verifies Create refuses an empty project_id
// before reaching GitLab.
func TestTagCreate_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(context.Background(), client, CreateInput{TagName: "v0", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestTagDelete_EmptyProjectID verifies Delete refuses an empty project_id
// before reaching GitLab, which for a destructive action is the difference
// between refusing and deleting from somewhere else.
func TestTagDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(context.Background(), client, DeleteInput{TagName: "v0"})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// ---------------------------------------------------------------------------
// API error tests
// ---------------------------------------------------------------------------.

// TestTagList_APIError verifies TagList when API error.
func TestTagList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"500"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagList_EmptyProjectID verifies List validates project_id before calling GitLab.
func TestTagList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpEmptyProjectID)
	}
	if !strings.Contains(err.Error(), argProjectID) {
		t.Fatalf("error = %v, want project_id", err)
	}
}

// TestTagGetSignature_APIError verifies TagGetSignature when API error.
func TestTagGetSignature_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))
	_, err := GetSignature(context.Background(), client, SignatureInput{ProjectID: "42", TagName: "v0"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagListProtected_APIError verifies TagListProtected when API error.
func TestTagListProtected_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagGetProtected_APIError verifies TagGetProtected when API error.
func TestTagGetProtected_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))
	_, err := GetProtectedTag(context.Background(), client, GetProtectedTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagProtect_APIError verifies TagProtect when API error.
func TestTagProtect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	_, err := ProtectTag(context.Background(), client, ProtectTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// TestTagUnprotect_APIError verifies TagUnprotect when API error.
func TestTagUnprotect_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403"}`)
	}))
	err := UnprotectTag(context.Background(), client, UnprotectTagInput{ProjectID: "42", TagName: "v*"})
	if err == nil {
		t.Fatal(errExpAPIFailure)
	}
}

// ---------------------------------------------------------------------------
// ProtectTag with AllowedToCreate granular permissions
// ---------------------------------------------------------------------------.

// TestTagProtect_RequestBody pins the protection rule ProtectTag asks GitLab
// for, body and all, with one case per principal a permission entry can name.
//
// Driving one field at a time is what makes the four guards distinguishable: a
// permission entry is a block of ids, so an entry carrying all of them at once
// has no fixture where no two values agree, and the assertion this replaces —
// that the body mentioned each field name somewhere — passed with every guard
// inverted, because another entry in the same list supplied each name. The
// empty cases matter as much: an optional field GitLab was not asked about
// must be absent rather than sent as zero.
func TestTagProtect_RequestBody(t *testing.T) {
	cases := []struct {
		name  string
		input ProtectTagInput
		want  map[string]any
	}{
		{
			name:  "a role rule",
			input: ProtectTagInput{ProjectID: "42", TagName: "v*", CreateAccessLevel: 30},
			want:  map[string]any{"name": "v*", "create_access_level": float64(30)},
		},
		{
			name:  "no access level named at all",
			input: ProtectTagInput{ProjectID: "42", TagName: "v*"},
			want:  map[string]any{"name": "v*"},
		},
		{
			name:  "one user",
			input: ProtectTagInput{ProjectID: "42", TagName: "v*", AllowedToCreate: []TagPermission{{UserID: 5}}},
			want:  map[string]any{"name": "v*", "allowed_to_create": []any{map[string]any{"user_id": float64(5)}}},
		},
		{
			name:  "one group",
			input: ProtectTagInput{ProjectID: "42", TagName: "v*", AllowedToCreate: []TagPermission{{GroupID: 10}}},
			want:  map[string]any{"name": "v*", "allowed_to_create": []any{map[string]any{"group_id": float64(10)}}},
		},
		{
			name:  "one deploy key",
			input: ProtectTagInput{ProjectID: "42", TagName: "v*", AllowedToCreate: []TagPermission{{DeployKeyID: 3}}},
			want:  map[string]any{"name": "v*", "allowed_to_create": []any{map[string]any{"deploy_key_id": float64(3)}}},
		},
		{
			name:  "one access level",
			input: ProtectTagInput{ProjectID: "42", TagName: "v*", AllowedToCreate: []TagPermission{{AccessLevel: 40}}},
			want:  map[string]any{"name": "v*", "allowed_to_create": []any{map[string]any{"access_level": float64(40)}}},
		},
		{
			name: "several principals at once",
			input: ProtectTagInput{ProjectID: "42", TagName: "v*", AllowedToCreate: []TagPermission{
				{UserID: 5, AccessLevel: 30},
				{GroupID: 10},
				{DeployKeyID: 3},
			}},
			want: map[string]any{"name": "v*", "allowed_to_create": []any{
				map[string]any{"user_id": float64(5), "access_level": float64(30)},
				map[string]any{"group_id": float64(10)},
				map[string]any{"deploy_key_id": float64(3)},
			}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != pathProtectedTags {
					http.NotFound(w, r)
					return
				}
				body = decodeJSONBody(t, w, r)
				if body == nil {
					return
				}
				testutil.RespondJSON(w, http.StatusCreated, `{"name":"v*","create_access_levels":[]}`)
			}))

			out, err := ProtectTag(context.Background(), client, c.input)
			if err != nil {
				t.Fatalf("ProtectTag() unexpected error: %v", err)
			}
			if out.Name != "v*" {
				t.Errorf(fmtNameWant, out.Name, "v*")
			}
			if !reflect.DeepEqual(body, c.want) {
				t.Errorf("request body = %#v, want %#v", body, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ListProtectedTags with pagination
// ---------------------------------------------------------------------------.

// TestTagListProtected_Pagination verifies TagListProtected when pagination.
func TestTagListProtected_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedTags {
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Errorf("query param page = %q, want %q", got, "2")
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"v*","create_access_levels":[]}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "6", TotalPages: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{
		ProjectID: "42",
		Page:      2, PerPage: 5,
	})
	if err != nil {
		t.Fatalf("ListProtectedTags() unexpected error: %v", err)
	}
	if out.Pagination.Page != 2 {
		t.Errorf("Pagination.Page = %d, want 2", out.Pagination.Page)
	}
	if out.Pagination.TotalItems != 6 {
		t.Errorf("Pagination.TotalItems = %d, want 6", out.Pagination.TotalItems)
	}
}

// TestTagListProtected_KeysetOrderingParams verifies that order_by, sort, and
// keyset pagination parameters are forwarded onto the request query.
func TestTagListProtected_KeysetOrderingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProtectedTags {
			q := r.URL.Query()
			if got := q.Get("order_by"); got != "name" {
				t.Errorf("query param order_by = %q, want %q", got, "name")
			}
			if got := q.Get("sort"); got != "desc" {
				t.Errorf("query param sort = %q, want %q", got, "desc")
			}
			if got := q.Get("pagination"); got != "keyset" {
				t.Errorf("query param pagination = %q, want %q", got, "keyset")
			}
			if got := q.Get("page_token"); got != "tok123" {
				t.Errorf("query param page_token = %q, want %q", got, "tok123")
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v*","create_access_levels":[]}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProtectedTags(context.Background(), client, ListProtectedTagsInput{
		ProjectID:  "42",
		OrderBy:    "name",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok123",
	})
	if err != nil {
		t.Fatalf("ListProtectedTags() unexpected error: %v", err)
	}
	if len(out.Tags) != 1 {
		t.Fatalf("len(out.Tags) = %d, want 1", len(out.Tags))
	}
}

// TestTagList_KeysetParams verifies that the tag list forwards keyset pagination
// parameters (pagination, page_token) onto the request query.
func TestTagList_KeysetParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathRepoTags {
			q := r.URL.Query()
			if got := q.Get("pagination"); got != "keyset" {
				t.Errorf("query param pagination = %q, want %q", got, "keyset")
			}
			if got := q.Get("page_token"); got != "cursor9" {
				t.Errorf("query param page_token = %q, want %q", got, "cursor9")
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v1.0.0","target":"abc"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		Pagination: "keyset", PageToken: "cursor9",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Tags) != 1 {
		t.Fatalf("len(out.Tags) = %d, want 1", len(out.Tags))
	}
}

// ---------------------------------------------------------------------------
// Converter edge cases
// ---------------------------------------------------------------------------.

// TestToOutput_NilCommitAndZeroTime verifies ToOutput when nil commit and zero time.
func TestToOutput_NilCommitAndZeroTime(t *testing.T) {
	out := toOutput(&gl.Tag{Name: "v0.0.1", Target: "abc"})
	if out.Commit != nil {
		t.Errorf("out.Commit = %+v, want nil for nil commit", out.Commit)
	}
	if out.Release != nil {
		t.Errorf("out.Release = %+v, want nil for nil release", out.Release)
	}
	if out.CreatedAt != "" {
		t.Errorf("out.CreatedAt = %q, want empty for zero time", out.CreatedAt)
	}
}

// TestProtectedTagOutput_FromGLEmptyLevels verifies ProtectedTagOutput when from gl empty levels.
func TestProtectedTagOutput_FromGLEmptyLevels(t *testing.T) {
	out := protectedTagOutputFromGL(&gl.ProtectedTag{Name: "v*"})
	if len(out.CreateAccessLevels) != 0 {
		t.Errorf("len(CreateAccessLevels) = %d, want 0", len(out.CreateAccessLevels))
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// The guidance sections the five tag formatters close with, pinned once so
// each whole-output expectation names them rather than restating them.
const (
	tagCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'tag.delete' to remove this tag\n" +
		"- Use action 'release.create' to create a release from this tag\n"

	tagListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'tag.get' to see one tag in full\n" +
		"- Use action 'tag.create' to create a new tag\n"

	tagSignatureHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'tag.get' to see the tag this signature belongs to\n" +
		"- Use action 'tag.list' to browse all tags\n"

	protectedTagCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'tag.list_protected' to see all protected tags\n" +
		"- Use action 'tag.unprotect' to remove tag protection\n"

	protectedTagListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'tag.get_protected' to see one rule in full\n" +
		"- Use action 'tag.protect' to add a new protected tag\n"
)

// TestFormatOutputMarkdownString pins the whole card of an annotated tag: the
// tag object's own id, which differs from the commit's here, then the commit
// as a nested object.
func TestFormatOutputMarkdownString(t *testing.T) {
	got := FormatOutputMarkdownString(Output{
		Name:      testTagV100,
		Target:    "abc123",
		Protected: true,
		Message:   testReleaseName,
		Commit:    &CommitOutput{ID: "abc123def456", Message: "feat: init"},
		Release:   &ReleaseNoteOutput{TagName: testTagV100, Description: "Initial release"},
		CreatedAt: "2026-01-01T00:00:00Z",
	})

	want := "## Tag: v1.0.0\n\n" +
		"- **Protected**: ✅\n" +
		"- **Tag Object**: `abc123`\n" +
		"- **Message**: Release v1.0.0\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **Commit**:\n" +
		"  - **SHA**: `abc123def456`\n" +
		"- **Release**: Initial release\n" +
		tagCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdownString_LightweightTag pins that a tag whose target
// is its commit shows the id once: printing it twice under two labels invited
// a reader to treat the tag object as a second commit.
func TestFormatOutputMarkdownString_LightweightTag(t *testing.T) {
	got := FormatOutputMarkdownString(Output{
		Name:   testTagV100,
		Target: "abc123def456",
		Commit: &CommitOutput{ID: "abc123def456", Title: "Fix login", AuthorName: "Alice", CommittedDate: "2026-03-20T15:45:00Z"},
	})

	want := "## Tag: v1.0.0\n\n" +
		"- **Protected**: ❌\n" +
		"- **Commit**:\n" +
		"  - **SHA**: `abc123def456`\n" +
		"  - **Title**: Fix login\n" +
		"  - **Author**: Alice\n" +
		"  - **Committed**: 20 Mar 2026 15:45 UTC\n" +
		tagCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdownString_MultiLineMessage pins that an annotated tag
// message of several lines is quoted under its label rather than flattened
// onto the row, where its second paragraph used to become a line of the card.
func TestFormatOutputMarkdownString_MultiLineMessage(t *testing.T) {
	got := FormatOutputMarkdownString(Output{
		Name:    "v2",
		Target:  "abc",
		Message: "Release notes\n\n- one\n- two",
	})

	want := "## Tag: v2\n\n" +
		"- **Protected**: ❌\n" +
		"- **Tag Object**: `abc`\n" +
		"- **Message**:\n" +
		"  > Release notes\n" +
		"  >\n" +
		"  > - one\n" +
		"  > - two\n" +
		tagCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdownString_Minimal pins the card of a tag GitLab
// answered with nothing but a name and a target.
func TestFormatOutputMarkdownString_Minimal(t *testing.T) {
	got := FormatOutputMarkdownString(Output{Name: "v0", Target: "x"})

	want := "## Tag: v0\n\n" +
		"- **Protected**: ❌\n" +
		"- **Tag Object**: `x`\n" +
		tagCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdownString pins the whole tag listing. The commit column
// is the commit a tag resolves to rather than the tag object's own id, which
// for an annotated tag is a SHA no other action accepts.
//
// The third row is a tag GitLab answered with a commit object carrying no id:
// the column falls back to the target, which is the only other SHA there is.
// That half of the fallback had never been taken, so the condition deciding it
// was only ever evaluated one way.
func TestFormatListMarkdownString(t *testing.T) {
	got := FormatListMarkdownString(ListOutput{
		Tags: []Output{
			{Name: testTagV100, Target: "abc", Protected: true, Commit: &CommitOutput{ID: "c0ffee1"}},
			{Name: "v0.9.0", Target: "def", Protected: false},
			{Name: "v0.8.0", Target: "beef42", Protected: false, Commit: &CommitOutput{Title: "no id sent"}},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 3},
	})

	want := "## Tags (3)\n\n" +
		"| Name | Commit | Protected |\n" +
		"| --- | --- | --- |\n" +
		"| v1.0.0 | `c0ffee1` | ✅ |\n" +
		"| v0.9.0 | `def` | ❌ |\n" +
		"| v0.8.0 | `beef42` | ❌ |\n" +
		"\n3 items total\n" +
		tagListHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdownString_Empty pins the whole response of a project with
// no tags.
func TestFormatListMarkdownString_Empty(t *testing.T) {
	got := FormatListMarkdownString(ListOutput{})

	want := "No tags found.\n"

	if got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatSignatureMarkdownString pins the whole card of a signed tag: the
// certificate and its issuer as sections of their own.
func TestFormatSignatureMarkdownString(t *testing.T) {
	got := FormatSignatureMarkdownString(SignatureOutput{
		SignatureType:      "X509",
		VerificationStatus: "verified",
		X509Certificate: X509CertificateOutput{
			Subject:           "CN=Test",
			Email:             testEmailAddr,
			CertificateStatus: "good",
			SerialNumber:      "12345",
			X509Issuer: X509IssuerOutput{
				Subject: "CN=Issuer",
				CrlURL:  testCRLURL,
			},
		},
	})

	want := "## Tag Signature\n\n" +
		"- **Signature Type**: X509\n" +
		"- **Verification Status**: verified\n" +
		"\n### X.509 Certificate\n\n" +
		"- **Subject**: CN=Test\n" +
		"- **Email**: test@example.com\n" +
		"- **Status**: good\n" +
		"- **Serial Number**: `12345`\n" +
		"\n### Issuer\n\n" +
		"- **Subject**: CN=Issuer\n" +
		"- **CRL URL**: [https://example.com/crl](https://example.com/crl)\n" +
		tagSignatureHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatSignatureMarkdownString_Minimal pins that a signature GitLab sent
// no certificate for opens neither section: a heading with nothing beneath it
// reads as content that failed to render.
func TestFormatSignatureMarkdownString_Minimal(t *testing.T) {
	got := FormatSignatureMarkdownString(SignatureOutput{
		SignatureType:      "X509",
		VerificationStatus: "unverified",
	})

	want := "## Tag Signature\n\n" +
		"- **Signature Type**: X509\n" +
		"- **Verification Status**: unverified\n" +
		tagSignatureHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatProtectedTagMarkdownString pins the whole card of a protection
// rule: its create access levels as the nested collection they are, the role
// the number stands for beside GitLab's own description.
func TestFormatProtectedTagMarkdownString(t *testing.T) {
	cases := []struct {
		name  string
		input ProtectedTagOutput
		want  string
	}{
		{
			name: "a role rule",
			input: ProtectedTagOutput{
				Name:               "v*",
				CreateAccessLevels: []TagAccessLevelOutput{{ID: 1, AccessLevel: 40, AccessLevelDescription: descMaintainers}},
			},
			want: "## Protected Tag: v*\n\n" +
				"### Create Access Levels\n\n" +
				"| ID | Access Level | Description | User ID | Group ID | Deploy Key ID |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 1 | Maintainer | Maintainers | - | - | - |\n" +
				protectedTagCardHints,
		},
		{
			name: "one user",
			input: ProtectedTagOutput{
				Name:               "v*",
				CreateAccessLevels: []TagAccessLevelOutput{{ID: 1, AccessLevel: 40, AccessLevelDescription: descMaintainers, UserID: 5}},
			},
			want: "## Protected Tag: v*\n\n" +
				"### Create Access Levels\n\n" +
				"| ID | Access Level | Description | User ID | Group ID | Deploy Key ID |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 1 | Maintainer | Maintainers | 5 | - | - |\n" +
				protectedTagCardHints,
		},
		{
			name: "one group",
			input: ProtectedTagOutput{
				Name:               "release-*",
				CreateAccessLevels: []TagAccessLevelOutput{{ID: 2, AccessLevel: 30, AccessLevelDescription: "Developers", GroupID: 10}},
			},
			want: "## Protected Tag: release-*\n\n" +
				"### Create Access Levels\n\n" +
				"| ID | Access Level | Description | User ID | Group ID | Deploy Key ID |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 2 | Developer | Developers | - | 10 | - |\n" +
				protectedTagCardHints,
		},
		{
			name: "one deploy key",
			input: ProtectedTagOutput{
				Name:               "deploy-*",
				CreateAccessLevels: []TagAccessLevelOutput{{ID: 3, AccessLevel: 40, AccessLevelDescription: "Deploy Key", DeployKeyID: 7}},
			},
			want: "## Protected Tag: deploy-*\n\n" +
				"### Create Access Levels\n\n" +
				"| ID | Access Level | Description | User ID | Group ID | Deploy Key ID |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| 3 | Maintainer | Deploy Key | - | - | 7 |\n" +
				protectedTagCardHints,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FormatProtectedTagMarkdownString(c.input); got != c.want {
				t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

// TestFormatProtectedTagMarkdownString_Empty pins the whole card of a rule
// with no create access levels, which is a rule nobody may create the tag
// under.
func TestFormatProtectedTagMarkdownString_Empty(t *testing.T) {
	got := FormatProtectedTagMarkdownString(ProtectedTagOutput{Name: "release-*"})

	want := "## Protected Tag: release-*\n\n" +
		"No create access levels are defined, so no one may create this tag.\n" +
		protectedTagCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListProtectedTagsMarkdownString pins the whole protected-tag
// listing, each entry naming the principal it grants when it grants one.
func TestFormatListProtectedTagsMarkdownString(t *testing.T) {
	cases := []struct {
		name   string
		levels []TagAccessLevelOutput
		cell   string
	}{
		{"a role", []TagAccessLevelOutput{{AccessLevelDescription: descMaintainers}}, "Maintainers"},
		{"one user", []TagAccessLevelOutput{{AccessLevelDescription: "Developer", UserID: 5}}, "Developer (User #5)"},
		{"one group", []TagAccessLevelOutput{{AccessLevelDescription: descMaintainers, GroupID: 10}}, "Maintainers (Group #10)"},
		{"one deploy key", []TagAccessLevelOutput{{AccessLevelDescription: "Deploy Key", DeployKeyID: 7}}, "Deploy Key (Deploy Key #7)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FormatListProtectedTagsMarkdownString(ListProtectedTagsOutput{
				Tags:       []ProtectedTagOutput{{Name: "v*", CreateAccessLevels: c.levels}},
				Pagination: toolutil.PaginationOutput{TotalItems: 1},
			})

			want := "## Protected Tags (1)\n\n" +
				"| Name | Create Access Levels |\n" +
				"| --- | --- |\n" +
				"| v* | " + c.cell + " |\n" +
				"\n1 items total\n" +
				protectedTagListHints

			if got != want {
				t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// TestFormatIDCell verifies FormatIDCell.
func TestFormatIDCell(t *testing.T) {
	if got := formatIDCell(0); got != "-" {
		t.Errorf("formatIDCell(0) = %q, want %q", got, "-")
	}
	if got := formatIDCell(42); got != "42" {
		t.Errorf("formatIDCell(42) = %q, want %q", got, "42")
	}
}

// TestFormatAccessLevelSummary covers FormatAccessLevelSummary with table-driven subtests.
func TestFormatAccessLevelSummary(t *testing.T) {
	tests := []struct {
		name  string
		input TagAccessLevelOutput
		want  string
	}{
		{"plain", TagAccessLevelOutput{AccessLevelDescription: descMaintainers}, "Maintainers"},
		{"user", TagAccessLevelOutput{AccessLevelDescription: "Dev", UserID: 5}, "Dev (User #5)"},
		{"group", TagAccessLevelOutput{AccessLevelDescription: "Dev", GroupID: 10}, "Dev (Group #10)"},
		{"deploy_key", TagAccessLevelOutput{AccessLevelDescription: "Key", DeployKeyID: 3}, "Key (Deploy Key #3)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatAccessLevelSummary(tt.input); got != tt.want {
				t.Errorf("formatAccessLevelSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatListProtectedTagsMarkdownString_Empty pins the whole response of a
// project with no protected tags.
func TestFormatListProtectedTagsMarkdownString_Empty(t *testing.T) {
	got := FormatListProtectedTagsMarkdownString(ListProtectedTagsOutput{})

	want := "No protected tags found.\n"

	if got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownWrappers verifies the MCP CallToolResult wrappers carry
// exactly the Markdown the tag string formatters produce, which is what makes
// the whole-output expectations above cover the wrapped surfaces too.
func TestFormatMarkdownWrappers(t *testing.T) {
	tagOut := Output{Name: testTagV100, Target: "abc"}
	listOut := ListOutput{Tags: []Output{tagOut}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}
	signatureOut := SignatureOutput{SignatureType: "X509", VerificationStatus: "verified"}
	protectedOut := ProtectedTagOutput{Name: "v*"}
	protectedListOut := ListProtectedTagsOutput{Tags: []ProtectedTagOutput{protectedOut}, Pagination: toolutil.PaginationOutput{TotalItems: 1}}

	tests := []struct {
		name   string
		result *mcp.CallToolResult
		want   string
	}{
		{name: "tag", result: FormatOutputMarkdown(tagOut), want: FormatOutputMarkdownString(tagOut)},
		{name: "list", result: FormatListMarkdown(listOut), want: FormatListMarkdownString(listOut)},
		{name: "signature", result: FormatSignatureMarkdown(signatureOut), want: FormatSignatureMarkdownString(signatureOut)},
		{name: "protected", result: FormatProtectedTagMarkdown(protectedOut), want: FormatProtectedTagMarkdownString(protectedOut)},
		{name: "protected list", result: FormatListProtectedTagsMarkdown(protectedListOut), want: FormatListProtectedTagsMarkdownString(protectedListOut)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result == nil {
				t.Fatal("expected non-nil result")
			}
			content, ok := tt.result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content type = %T, want TextContent", tt.result.Content[0])
			}
			if content.Text != tt.want {
				t.Fatalf("wrapper mismatch:\ngot:\n%s\nwant:\n%s", content.Text, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// List with search/order/sort params
// ---------------------------------------------------------------------------.

// TestTagList_WithSearchOrderSort verifies TagList when with search order sort.
func TestTagList_WithSearchOrderSort(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathRepoTags {
			if got := r.URL.Query().Get("search"); got != "v1" {
				t.Errorf("search = %q, want %q", got, "v1")
			}
			if got := r.URL.Query().Get("order_by"); got != "version" {
				t.Errorf("order_by = %q, want %q", got, "version")
			}
			if got := r.URL.Query().Get("sort"); got != "desc" {
				t.Errorf("sort = %q, want %q", got, "desc")
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"v1.2.0","target":"abc","protected":false}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "v1",
		OrderBy:   "version",
		Sort:      "desc",
	})
	if err != nil {
		t.Fatalf(fmtListUnexpErr, err)
	}
	if len(out.Tags) != 1 {
		t.Fatalf("len(out.Tags) = %d, want 1", len(out.Tags))
	}
}

// ---------------------------------------------------------------------------
// RegisterTools + CallAllThroughMCP
// ---------------------------------------------------------------------------.

// tagRouteHandler returns an http.HandlerFunc that dispatches requests
// to canned tag API responses based on method and path.
func tagRouteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/signature"):
			testutil.RespondJSON(w, http.StatusOK, `{
				"signature_type":"X509","verification_status":"verified",
				"x509_certificate":{"id":1,"subject":"CN=T","subject_key_identifier":"a",
					"email":"t@t.com","serial_number":1,"certificate_status":"good",
					"x509_issuer":{"id":2,"subject":"CN=I","subject_key_identifier":"b"}}
			}`)

		case r.Method == http.MethodGet && strings.Contains(path, "/repository/tags/"):
			testutil.RespondJSON(w, http.StatusOK, `{"name":"v1.0.0","target":"abc","protected":false}`)

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/repository/tags"):
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v2.0.0","target":"def","protected":false}`)

		case r.Method == http.MethodDelete && strings.Contains(path, "/repository/tags/"):
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/repository/tags"):
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"v1.0.0","target":"abc","protected":false}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/protected_tags/v*"):
			testutil.RespondJSON(w, http.StatusOK, `{"name":"v*","create_access_levels":[]}`)

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/protected_tags"):
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"name":"v*","create_access_levels":[]}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/protected_tags"):
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"v*","create_access_levels":[]}`)

		case r.Method == http.MethodDelete && strings.Contains(path, "/protected_tags/"):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}
}

// newTagSpecsByTool constructs tag specs by tool test fixtures.
func newTagSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	client := testutil.NewTestClient(t, tagRouteHandler())
	return tagSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_Metadata verifies canonical metadata for tag actions.
func TestActionSpecs_Metadata(t *testing.T) {
	byTool := newTagSpecsByTool(t)

	if len(byTool) != 9 {
		t.Fatalf("len(byTool) = %d, want 9", len(byTool))
	}
	for toolName, spec := range byTool {
		if spec.OwnerPackage != "tags" {
			t.Fatalf("OwnerPackage for %s = %q, want tags", toolName, spec.OwnerPackage)
		}
	}

	list := byTool["gitlab_tag_list"]
	if list.Usage == "" || len(list.Aliases) == 0 || len(list.ParameterGuidance) == 0 {
		t.Fatalf("gitlab_tag_list metadata incomplete: usage=%q aliases=%d guidance=%d", list.Usage, len(list.Aliases), len(list.ParameterGuidance))
	}

	get := byTool["gitlab_tag_get"]
	if get.Usage == "" || len(get.Aliases) == 0 || get.ParameterGuidance["tag_name"].SemanticRole == "" {
		t.Fatalf("gitlab_tag_get metadata incomplete: usage=%q aliases=%d guidance(tag_name)=%q", get.Usage, len(get.Aliases), get.ParameterGuidance["tag_name"].SemanticRole)
	}

	create := byTool["gitlab_tag_create"]
	if create.Usage == "" || len(create.Aliases) == 0 || create.ParameterGuidance["ref"].SemanticRole == "" {
		t.Fatalf("gitlab_tag_create metadata incomplete: usage=%q aliases=%d guidance(ref)=%q", create.Usage, len(create.Aliases), create.ParameterGuidance["ref"].SemanticRole)
	}
}

// assertTagRouteSuccess checks tag route success invariants for tests.
func assertTagRouteSuccess(t *testing.T, specs map[string]toolutil.ActionSpec, name string, args map[string]any) {
	t.Helper()
	result, err := specs[name].Route.Handler(t.Context(), args)
	if err != nil {
		t.Fatalf("Route.Handler(%s) error: %v", name, err)
	}
	if result == nil {
		t.Fatalf("Route.Handler(%s) returned nil", name)
	}
}

// TestActionSpecs_CallAllRoutes validates tag routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	specs := newTagSpecsByTool(t)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_tag_get", map[string]any{argProjectID: "42", argTagName: testTagV100}},
		{"gitlab_tag_create", map[string]any{argProjectID: "42", argTagName: "v2.0.0", "ref": "main"}},
		{"gitlab_tag_delete", map[string]any{argProjectID: "42", argTagName: testTagV100}},
		{"gitlab_tag_list", map[string]any{argProjectID: "42"}},
		{"gitlab_tag_get_signature", map[string]any{argProjectID: "42", argTagName: testTagV100}},
		{"gitlab_tag_list_protected", map[string]any{argProjectID: "42"}},
		{"gitlab_tag_get_protected", map[string]any{argProjectID: "42", argTagName: "v*"}},
		{"gitlab_tag_protect", map[string]any{argProjectID: "42", argTagName: "v*"}},
		{"gitlab_tag_unprotect", map[string]any{argProjectID: "42", argTagName: "v*"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertTagRouteSuccess(t, specs, tt.name, tt.args)
		})
	}
}

// TestList_CancelledContext verifies that List returns the context's
// cancellation error when ctx is cancelled before the call. It targets
// the early-return guard at the top of List that checks ctx.Err() prior
// to performing any GitLab API request.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := List(ctx, client, ListInput{ProjectID: "42"}); err == nil {
		t.Fatal(errCancelledCtx)
	}
}

// TestActionSpecs_TagGetRoute verifies the canonical tag get route output.
func TestActionSpecs_TagGetRoute(t *testing.T) {
	const respJSON = `{"name":"v1.0.0","message":"Release v1.0.0","target":"abcdef","protected":false,"commit":{"id":"abcdef","created_at":"2026-01-01T00:00:00Z"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/repository/tags/v1.0.0") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := tagSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_tag_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "tag_name": "v1.0.0"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.Name != "v1.0.0" || out.Target != "abcdef" {
		t.Fatalf("tag output = %#v, want v1.0.0 target abcdef", out)
	}
}

// tagSpecsByTool supports tag specs by tool assertions in tags tests.
func tagSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
