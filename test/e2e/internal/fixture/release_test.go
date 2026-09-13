//go:build e2e

// release_test.go drives the release builder's GitLab-touching half against
// an instance of its own, since the shared stub answers neither tags nor
// releases and this is the only file that wants them.

package fixture

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// releaseStub answers the two endpoints createRelease calls and records the
// order it was asked in, which is the property the test is about.
type releaseStub struct {
	t *testing.T

	mu sync.Mutex
	// calls records each request as "METHOD path".
	calls []string
	// tagStatus and releaseStatus are what the two endpoints answer; zero
	// means the created status a real GitLab gives.
	tagStatus     int
	releaseStatus int
	// tagBody replaces what the tag endpoint echoes back, so a test can
	// give the refusal the message a real GitLab carries.
	tagBody map[string]any
	// releaseBody does the same for the release endpoint, which is how the
	// conflict a second attempt meets is given its real message.
	releaseBody map[string]any
	// readStatus is what the release read answers, for the path a conflict
	// sends the builder down.
	readStatus int
}

// newReleaseStub starts a stub and returns it beside a client pointed at it.
func newReleaseStub(t *testing.T) (*releaseStub, *gitlabclient.Client) {
	t.Helper()

	stub := &releaseStub{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/{id}/repository/tags", func(w http.ResponseWriter, r *http.Request) {
		sent := stub.record(r)
		body := stub.tagBody
		if body == nil {
			body = map[string]any{"name": sent["tag_name"]}
		}
		stub.answer(w, stub.tagStatus, body)
	})
	mux.HandleFunc("POST /api/v4/projects/{id}/releases", func(w http.ResponseWriter, r *http.Request) {
		sent := stub.record(r)
		body := stub.releaseBody
		if body == nil {
			body = map[string]any{"tag_name": sent["tag_name"], "name": sent["name"]}
		}
		stub.answer(w, stub.releaseStatus, body)
	})
	mux.HandleFunc("GET /api/v4/projects/{id}/releases/{tag}", func(w http.ResponseWriter, r *http.Request) {
		tag := stub.note(r)
		stub.answer(w, stub.readStatus, map[string]any{"tag_name": tag, "name": "Release " + tag})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("release stub: unexpected %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client, err := gitlabclient.NewClientWithTokenRetries(server.URL, "glpat-release-stub", false, true)
	if err != nil {
		t.Fatalf("building a client for the release stub: %v", err)
	}
	return stub, client
}

// record writes the request down under the stub's lock and returns the JSON
// body it carried, which is how client-go sends both of these.
func (s *releaseStub) record(r *http.Request) map[string]any {
	sent := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
		s.t.Errorf("release stub: %s %s carried no JSON body: %v", r.Method, r.URL.Path, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	return sent
}

// note writes down a request that carries no body and returns the tag it
// addressed, for the read the conflict path makes.
func (s *releaseStub) note(r *http.Request) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	return r.PathValue("tag")
}

// answer writes a JSON body with the status the test configured.
func (s *releaseStub) answer(w http.ResponseWriter, status int, body map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status == 0 {
		status = http.StatusCreated
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		s.t.Errorf("release stub: writing the answer: %v", err)
	}
}

// recorded returns the requests the stub answered.
func (s *releaseStub) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// TestCreateRelease_Answers_MakesTheTagBeforeTheRelease checks the builder
// asks for the tag first and reads the release off the second answer, which
// is what a group release listing then aggregates.
func TestCreateRelease_Answers_MakesTheTagBeforeTheRelease(t *testing.T) {
	stub, client := newReleaseStub(t)

	release, err := createRelease(t.Context(), client, 7, "v1.2.3", "main", "e2e: a test")
	if err != nil {
		t.Fatalf("createRelease() error = %v, want nil", err)
	}
	if release.TagName != "v1.2.3" || release.Name != "Release v1.2.3" {
		t.Errorf("createRelease() = %+v, want the tag v1.2.3 titled Release v1.2.3", release)
	}

	calls := stub.recorded()
	if len(calls) != 2 ||
		!strings.HasSuffix(calls[0], "/repository/tags") ||
		!strings.HasSuffix(calls[1], "/releases") {
		t.Errorf("the stub was asked %v, want the tag created before the release", calls)
	}
}

// TestCreateRelease_TagRefused_NamesTheTagAndSendsNoRelease checks that a
// refused tag ends the builder there: GitLab would answer the release with
// a 404 about the tag, so the error has to name the tag and the release
// must never be sent.
func TestCreateRelease_TagRefused_NamesTheTagAndSendsNoRelease(t *testing.T) {
	stub, client := newReleaseStub(t)
	stub.tagStatus = http.StatusBadRequest
	stub.tagBody = map[string]any{"message": "Bad Request"}

	_, err := createRelease(t.Context(), client, 7, "v1.2.3", "main", "e2e: a test")
	if err == nil || !strings.Contains(err.Error(), "v1.2.3") {
		t.Fatalf("createRelease() error = %v, want one naming the tag", err)
	}
	if calls := stub.recorded(); len(calls) != 1 || !strings.HasSuffix(calls[0], "/repository/tags") {
		t.Errorf("the stub was asked %v, want the tag alone", calls)
	}
}

// TestCreateRelease_TagAlreadyThere_CarriesOnToTheRelease checks the one
// refusal the pair tolerates. The whole pair is retried as one, so a second
// attempt meets the tag its first attempt made, and treating that as a
// failure would turn a retryable release error into a permanent one about
// the wrong object.
func TestCreateRelease_TagAlreadyThere_CarriesOnToTheRelease(t *testing.T) {
	stub, client := newReleaseStub(t)
	stub.tagStatus = http.StatusBadRequest
	stub.tagBody = map[string]any{"message": "Tag v1.2.3 already exists"}

	release, err := createRelease(t.Context(), client, 7, "v1.2.3", "main", "e2e: a test")
	if err != nil {
		t.Fatalf("createRelease() error = %v, want the tag that is already there to be accepted", err)
	}
	if release.TagName != "v1.2.3" {
		t.Errorf("createRelease() = %+v, want the release on the tag that was already there", release)
	}
	if calls := stub.recorded(); len(calls) != 2 || !strings.HasSuffix(calls[1], "/releases") {
		t.Errorf("the stub was asked %v, want the release sent after the refused tag", calls)
	}
}

// TestCreateRelease_ReleaseAlreadyThere_ReadsItBack checks the other half of
// the same retry: an attempt that created the release and lost the answer
// meets GitLab's 409 on the next one, and the release it asked for is read
// back rather than reported as a failure.
func TestCreateRelease_ReleaseAlreadyThere_ReadsItBack(t *testing.T) {
	stub, client := newReleaseStub(t)
	stub.tagStatus = http.StatusBadRequest
	stub.tagBody = map[string]any{"message": "Tag v1.2.3 already exists"}
	stub.releaseStatus = http.StatusConflict
	stub.releaseBody = map[string]any{"message": releaseAlreadyExists}

	release, err := createRelease(t.Context(), client, 7, "v1.2.3", "main", "e2e: a test")
	if err != nil {
		t.Fatalf("createRelease() error = %v, want the release that is already there to be read back", err)
	}
	if release.TagName != "v1.2.3" || release.Name != "Release v1.2.3" {
		t.Errorf("createRelease() = %+v, want the release that was already on the tag", release)
	}
	calls := stub.recorded()
	if len(calls) != 3 || !strings.HasPrefix(calls[2], http.MethodGet+" ") {
		t.Errorf("the stub was asked %v, want the release read after the refused create", calls)
	}
}

// TestCreateRelease_ReleaseAlreadyThereAndUnreadable_NamesTheTag checks that
// the conflict path reports the read it could not make, rather than the
// conflict that sent it there, since the read is what actually failed.
func TestCreateRelease_ReleaseAlreadyThereAndUnreadable_NamesTheTag(t *testing.T) {
	stub, client := newReleaseStub(t)
	stub.releaseStatus = http.StatusConflict
	stub.releaseBody = map[string]any{"message": releaseAlreadyExists}
	stub.readStatus = http.StatusForbidden

	_, err := createRelease(t.Context(), client, 7, "v1.2.3", "main", "e2e: a test")
	if err == nil || !strings.Contains(err.Error(), "v1.2.3") {
		t.Fatalf("createRelease() error = %v, want one naming the tag it could not read", err)
	}
}
