//go:build e2e

// stub_test.go is the GitLab the fixture tests run against: an httptest
// server answering the handful of endpoints the builders' pure halves call,
// with the delayed-deletion dance and the eventual-consistency answers a real
// one gives. It exists so the two-step delete, the waits and the sweep can be
// driven without a GitLab, on every push.

package fixture

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// stubToken is the credential the stub client sends; the stub never checks
// it.
const stubToken = "glpat-fixture-stub"

// deletionSuffix is what GitLab appends to the path of an object it has
// marked for deletion.
const deletionSuffix = "-deletion_scheduled-"

// stubObject is a project or a group as the stub keeps it.
type stubObject struct {
	ID     int64
	Name   string
	Path   string
	Marked bool
	// permanentRemoveUnsupported makes the object refuse a permanent-remove
	// with the message GitLab returns for a top-level group on an instance
	// with delayed deletion, which can only be marked, never purged, through
	// the API.
	permanentRemoveUnsupported bool
	// permanentRemoveStatus is the status that refusal carries; zero means
	// the 400 GitLab answers with, and a test sets another to prove the
	// tolerance reads the status and not only the words.
	permanentRemoveStatus int
}

// stubGitLab is the in-memory instance one test drives.
type stubGitLab struct {
	t *testing.T

	mu       sync.Mutex
	projects map[int64]*stubObject
	groups   map[int64]*stubObject
	users    map[int64]string
	// deletes records every DELETE the stub answered, as "kind id params".
	deletes []string
	// branchMisses is how many branch reads answer 404 before the branch
	// appears, which is how a stub says "not yet".
	branchMisses int
	// mergeStatuses and pipelineStatuses are answered one per read, the
	// last repeating.
	mergeStatuses    []string
	pipelineStatuses []string
	// pipelineFailures is how many pipeline reads answer 502 first.
	pipelineFailures int
	// enqueued is what the Sidekiq stats report, decremented per read.
	enqueued int64
	// runners is what the runner listing answers.
	runners []*gl.Runner
	// state is what the World's readers see, keyed by the request path.
	state map[string]any
	// graphqlAnswers are answered one per document, the last repeating,
	// each the raw body of a GraphQL response.
	graphqlAnswers []string
	// graphqlDocuments records every document the stub was sent.
	graphqlDocuments []string
	// hookEventAnswers are the raw bodies of the hook events listing,
	// answered one per read, the last repeating; a body that is not JSON is
	// answered as a 404, which is how a real GitLab answers before the
	// first delivery on some releases.
	hookEventAnswers []string

	server *httptest.Server
}

// newStubGitLab starts a stub and returns it beside a client pointed at it.
func newStubGitLab(t *testing.T) (*stubGitLab, *gitlabclient.Client) {
	t.Helper()

	stub := &stubGitLab{
		t:        t,
		projects: map[int64]*stubObject{},
		groups:   map[int64]*stubObject{},
		users:    map[int64]string{},
		state:    map[string]any{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects", stub.listProjects)
	mux.HandleFunc("/api/v4/projects/{id}", stub.project)
	mux.HandleFunc("/api/v4/groups", stub.listGroups)
	mux.HandleFunc("/api/v4/groups/{id}", stub.group)
	mux.HandleFunc("/api/v4/users", stub.listUsers)
	mux.HandleFunc("/api/v4/users/{id}", stub.user)
	mux.HandleFunc("/api/v4/sidekiq/job_stats", stub.sidekiq)
	mux.HandleFunc("/api/v4/runners/all", stub.listRunners)
	mux.HandleFunc("/api/v4/projects/{id}/repository/branches/{branch...}", stub.branch)
	mux.HandleFunc("/api/v4/projects/{id}/merge_requests/{iid}", stub.mergeRequest)
	mux.HandleFunc("/api/v4/projects/{id}/pipelines/{pipeline}", stub.pipeline)
	mux.HandleFunc("/api/v4/projects/{id}/issues/{iid}", stub.stateAnswer)
	mux.HandleFunc("/api/v4/projects/{id}/labels/{label}", stub.stateAnswer)
	mux.HandleFunc("/api/v4/projects/{id}/milestones/{milestone}", stub.stateAnswer)
	mux.HandleFunc("/api/v4/groups/{id}/hooks/{hook}/events", stub.hookEvents)
	mux.HandleFunc("/api/graphql", stub.graphql)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("stub GitLab: unexpected %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})
	stub.server = httptest.NewServer(mux)
	t.Cleanup(stub.server.Close)

	client, err := gitlabclient.NewClientWithTokenRetries(stub.server.URL, stubToken, false, true)
	if err != nil {
		t.Fatalf("building a client for the stub: %v", err)
	}
	return stub, client
}

// configure changes the stub's answers under its lock, so a test's setup
// and the handlers that read it are ordered the way the race detector can
// see.
func (s *stubGitLab) configure(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

// addProject registers a project with the stub.
func (s *stubGitLab) addProject(id int64, name, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[id] = &stubObject{ID: id, Name: name, Path: path}
}

// addGroup registers a group with the stub.
func (s *stubGitLab) addGroup(id int64, name, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[id] = &stubObject{ID: id, Name: name, Path: path}
}

// addTopLevelGroup registers a group whose permanent removal GitLab refuses,
// as it does for a top-level group on an instance with delayed deletion.
func (s *stubGitLab) addTopLevelGroup(id int64, name, path string) {
	s.addTopLevelGroupRefusing(id, name, path, http.StatusBadRequest)
}

// addTopLevelGroupRefusing is addTopLevelGroup with the refusal carried under
// the given status rather than GitLab's 400.
func (s *stubGitLab) addTopLevelGroupRefusing(id int64, name, path string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[id] = &stubObject{ID: id, Name: name, Path: path, permanentRemoveUnsupported: true, permanentRemoveStatus: status}
}

// addUser registers a user with the stub.
func (s *stubGitLab) addUser(id int64, username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[id] = username
}

// recordedDeletes returns what the stub was asked to delete, in order.
func (s *stubGitLab) recordedDeletes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.deletes...)
}

// remaining returns the paths of the projects and groups still there.
func (s *stubGitLab) remaining() (projects, groups []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		projects = append(projects, p.Path)
	}
	for _, g := range s.groups {
		groups = append(groups, g.Path)
	}
	return projects, groups
}

// writeJSON answers with a JSON body. A body that does not encode is a bug in
// the stub, answered as a 500 so the calling test sees it as an error.
func writeJSON(w http.ResponseWriter, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		http.Error(w, "stub: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// writeError answers the way GitLab refuses something.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}

// pathID reads a numeric path value.
func pathID(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id
}

// projectJSON is a project as the stub serializes it.
func projectJSON(p *stubObject) map[string]any {
	return map[string]any{
		"id": p.ID, "name": p.Name, "path_with_namespace": p.Path, "path": lastSegment(p.Path),
		"default_branch": "main", "visibility": "private", "description": "", "archived": false,
	}
}

// groupJSON is a group as the stub serializes it.
func groupJSON(g *stubObject) map[string]any {
	return map[string]any{"id": g.ID, "name": g.Name, "full_path": g.Path, "path": lastSegment(g.Path), "visibility": "private"}
}

// listProjects answers the project listing, filtered by search.
func (s *stubGitLab) listProjects(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	search := r.URL.Query().Get("search")
	var out []map[string]any
	for _, p := range s.projects {
		if strings.Contains(p.Name, search) || strings.Contains(p.Path, search) {
			out = append(out, projectJSON(p))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// listGroups answers the group listing, filtered by search.
func (s *stubGitLab) listGroups(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	search := r.URL.Query().Get("search")
	var out []map[string]any
	for _, g := range s.groups {
		if strings.Contains(g.Name, search) || strings.Contains(g.Path, search) {
			out = append(out, groupJSON(g))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// listUsers answers the user listing, filtered by search.
func (s *stubGitLab) listUsers(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	search := r.URL.Query().Get("search")
	var out []map[string]any
	for id, username := range s.users {
		if strings.Contains(username, search) {
			out = append(out, map[string]any{"id": id, "username": username, "name": username})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// user answers a user's deletion.
func (s *stubGitLab) user(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := pathID(r, "id")
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "stub: only DELETE is answered for users")
		return
	}
	if _, ok := s.users[id]; !ok {
		writeError(w, http.StatusNotFound, "404 User Not Found")
		return
	}
	delete(s.users, id)
	s.deletes = append(s.deletes, "user "+strconv.FormatInt(id, 10))
	w.WriteHeader(http.StatusNoContent)
}

// project answers reads and the two deletion steps for one project.
func (s *stubGitLab) project(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.object(w, r, "project", s.projects, projectJSON)
}

// group answers reads and the two deletion steps for one group.
func (s *stubGitLab) group(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.object(w, r, "group", s.groups, groupJSON)
}

// object is the delayed-deletion dance a real GitLab does: a plain DELETE
// marks and renames, a second plain DELETE is refused as already marked, and
// a permanent DELETE needs the renamed path.
func (s *stubGitLab) object(w http.ResponseWriter, r *http.Request, kind string, objects map[int64]*stubObject, serialize func(*stubObject) map[string]any) {
	id := pathID(r, "id")
	query := r.URL.Query()
	if r.Method == http.MethodDelete {
		s.deletes = append(s.deletes, fmt.Sprintf("%s %d %s", kind, id, query.Encode()))
	}
	obj, ok := objects[id]
	if !ok {
		writeError(w, http.StatusNotFound, "404 "+kind+" Not Found")
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, serialize(obj))
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "stub: unexpected method")
		return
	}
	if query.Get("permanently_remove") != "true" {
		if obj.Marked {
			writeError(w, http.StatusBadRequest, "Project has been already marked for deletion")
			return
		}
		obj.Marked = true
		obj.Path += deletionSuffix + strconv.FormatInt(id, 10)
		writeJSON(w, http.StatusAccepted, map[string]string{"message": "202 Accepted"})
		return
	}
	if obj.permanentRemoveUnsupported {
		status := obj.permanentRemoveStatus
		if status == 0 {
			status = http.StatusBadRequest
		}
		writeError(w, status, "`permanently_remove` option is only available for subgroups.")
		return
	}
	if !obj.Marked {
		writeError(w, http.StatusBadRequest, kind+" must be marked for deletion first")
		return
	}
	if query.Get("full_path") != obj.Path {
		writeError(w, http.StatusBadRequest, "full_path is incorrect")
		return
	}
	delete(objects, id)
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "202 Accepted"})
}

// sidekiq answers the job stats, with one fewer job enqueued per read.
func (s *stubGitLab) sidekiq(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enqueued := s.enqueued
	if s.enqueued > 0 {
		s.enqueued--
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": map[string]any{"processed": 1, "failed": 0, "enqueued": enqueued}})
}

// listRunners answers the instance runner listing.
func (s *stubGitLab) listRunners(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if got := r.URL.Query().Get("type"); got != "instance_type" {
		s.t.Errorf("runner listing asked for type %q, want instance_type", got)
	}
	writeJSON(w, http.StatusOK, s.runners)
}

// branch answers 404 until the configured misses are spent, then the
// branch.
func (s *stubGitLab) branch(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, _ := url.PathUnescape(r.PathValue("branch"))
	if s.branchMisses > 0 {
		s.branchMisses--
		writeError(w, http.StatusNotFound, "404 Branch Not Found")
		return
	}
	if answer, ok := s.state[r.URL.Path]; ok {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "protected": false, "commit": map[string]any{"id": "abc123"}})
}

// mergeRequest answers the next merge status, the last repeating.
func (s *stubGitLab) mergeRequest(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if answer, ok := s.state[r.URL.Path]; ok {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	status := nextStatus(&s.mergeStatuses)
	if status == "" {
		writeError(w, http.StatusNotFound, "404 Not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"iid": pathID(r, "iid"), "detailed_merge_status": status})
}

// pipeline answers the configured failures, then the next status.
func (s *stubGitLab) pipeline(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pipelineFailures > 0 {
		s.pipelineFailures--
		writeError(w, http.StatusBadGateway, "502 Bad Gateway")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": pathID(r, "pipeline"), "status": nextStatus(&s.pipelineStatuses)})
}

// stateAnswer answers whatever the test put under the request path.
func (s *stubGitLab) stateAnswer(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if answer, ok := s.state[r.URL.Path]; ok {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	writeError(w, http.StatusNotFound, "404 Not Found")
}

// graphql answers the next scripted body to any document, recording the
// document it was sent.
func (s *stubGitLab) graphql(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var document struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&document); err != nil {
		writeError(w, http.StatusBadRequest, "malformed document: "+err.Error())
		return
	}
	s.graphqlDocuments = append(s.graphqlDocuments, document.Query)
	body := nextStatus(&s.graphqlAnswers)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// hookEvents answers the next scripted hook events body, or a 404 for a
// body that is not JSON.
func (s *stubGitLab) hookEvents(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body := nextStatus(&s.hookEventAnswers)
	if !json.Valid([]byte(body)) {
		writeError(w, http.StatusNotFound, "404 Not Found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// nextStatus pops the next status of a sequence, keeping the last one.
func nextStatus(statuses *[]string) string {
	if len(*statuses) == 0 {
		return ""
	}
	status := (*statuses)[0]
	if len(*statuses) > 1 {
		*statuses = (*statuses)[1:]
	}
	return status
}

// statusError builds the structured error client-go returns for an HTTP
// refusal, with the request a real one carries so its message formats.
func statusError(code int, message string) error {
	request, _ := http.NewRequest(http.MethodGet, "https://gitlab.example/api/v4/projects/1", http.NoBody) //nolint:noctx // a fixture error needs a request shape, not a live request
	return &gl.ErrorResponse{
		Response: &http.Response{StatusCode: code, Request: request},
		Message:  message,
	}
}
