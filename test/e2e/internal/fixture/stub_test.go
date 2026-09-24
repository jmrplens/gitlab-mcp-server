//go:build e2e

// stub_test.go is the GitLab the fixture tests run against: an httptest
// server answering the handful of endpoints the builders' pure halves call,
// with the delayed-deletion dance and the eventual-consistency answers a real
// one gives. It exists so the two-step delete, the waits and the sweep can be
// driven without a GitLab, on every push.
//
// Beside it are the two ways a test reaches a whole builder rather than the
// half that takes a client: an Env pointed at the stub, and a child test
// process for the branch where a builder fails the test that called it.

package fixture

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
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
	// snippets is what the snippet listing answers. It holds nothing unless
	// a test adds to it, so every test that never thinks about snippets
	// still sees a sweep that finds none.
	snippets map[int64]stubSnippet
	// deletes records every DELETE the stub answered, as "kind id params".
	deletes []string
	// listings records every listing the stub answered, which is how a test
	// sees what a sweep searched by.
	listings []stubRequest
	// refusals is the status a request path is refused with, for a test of
	// what a sweep does when it cannot read a kind at all or cannot delete
	// one object of it. The listings and a user's deletion answer it.
	refusals map[string]int
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
	// graphqlDocuments records every document the stub was sent, and
	// graphqlVariables the variables each carried, in the same order.
	graphqlDocuments []string
	graphqlVariables []map[string]any
	// hookEventAnswers are the raw bodies of the hook events listing,
	// answered one per read, the last repeating; a body that is not JSON is
	// answered as a 404, which is how a real GitLab answers before the
	// first delivery on some releases.
	hookEventAnswers []string
	// scripted holds the answers the catch-all route gives, keyed by method
	// and path, consumed one per request with the last one repeating.
	scripted map[string][]scriptedAnswer
	// requests records what the catch-all route was sent, in order.
	requests []stubRequest

	server *httptest.Server
}

// stubSnippet is a snippet the listing answers with. A zero ProjectID is a
// personal snippet, which GitLab answers with a null project_id.
type stubSnippet struct {
	Title     string
	ProjectID int64
}

// scriptedAnswer is one answer the stub was told to give.
type scriptedAnswer struct {
	status int
	body   any
}

// stubRequest is a request the catch-all route answered, as a test reads it
// back: enough to assert what a builder put on the wire without asserting the
// exact bytes client-go chose to encode it as.
type stubRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// newStubGitLab starts a stub and returns it beside a client pointed at it.
func newStubGitLab(t *testing.T) (*stubGitLab, *gitlabclient.Client) {
	t.Helper()

	stub := &stubGitLab{
		t:        t,
		projects: map[int64]*stubObject{},
		groups:   map[int64]*stubObject{},
		users:    map[int64]string{},
		snippets: map[int64]stubSnippet{},
		state:    map[string]any{},
		scripted: map[string][]scriptedAnswer{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects", stub.listProjects)
	mux.HandleFunc("/api/v4/projects/{id}", stub.project)
	mux.HandleFunc("/api/v4/groups", stub.listGroups)
	mux.HandleFunc("/api/v4/groups/{id}", stub.group)
	mux.HandleFunc("/api/v4/users", stub.listUsers)
	mux.HandleFunc("/api/v4/users/{id}", stub.user)
	// A snippet's deletion is left to the script, under the catch-all
	// below: a sweep that deletes a snippet nothing scripted fails its test,
	// which is the strictness a sweep's test wants.
	mux.HandleFunc("/api/v4/snippets", stub.listSnippets)
	mux.HandleFunc("/api/v4/sidekiq/job_stats", stub.sidekiq)
	mux.HandleFunc("/api/v4/runners/all", stub.listRunners)
	mux.HandleFunc("/api/v4/projects/{id}/repository/branches/{branch...}", stub.branch)
	// Named on its own, because the branch pattern above ends in a wildcard
	// and ServeMux answers the bare collection path with a redirect to it,
	// which client-go follows as a read: a branch creation would come back
	// as a branch with no name rather than reaching the script.
	mux.HandleFunc("/api/v4/projects/{id}/repository/branches", stub.scriptedRoute)
	mux.HandleFunc("/api/v4/projects/{id}/merge_requests/{iid}", stub.mergeRequest)
	mux.HandleFunc("/api/v4/projects/{id}/pipelines/{pipeline}", stub.pipeline)
	mux.HandleFunc("/api/v4/projects/{id}/issues/{iid}", stub.stateAnswer)
	mux.HandleFunc("/api/v4/projects/{id}/labels/{label}", stub.stateAnswer)
	mux.HandleFunc("/api/v4/projects/{id}/milestones/{milestone}", stub.stateAnswer)
	mux.HandleFunc("/api/v4/groups/{id}/hooks/{hook}/events", stub.hookEvents)
	mux.HandleFunc("/api/graphql", stub.graphql)
	// Everything else under the API root is answered from the script. The
	// routes above are more specific patterns, so ServeMux still prefers
	// them; this one is what the recipe tests add an endpoint through, and
	// it keeps the catch-all's rule that an unasked-for request is a bug by
	// failing the test when nothing is scripted for it.
	mux.HandleFunc("/api/v4/", stub.scriptedRoute)
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

// addSnippet registers a snippet with the stub's listing: a personal one
// when projectID is zero, and otherwise one written in that project.
func (s *stubGitLab) addSnippet(id int64, title string, projectID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snippets[id] = stubSnippet{Title: title, ProjectID: projectID}
}

// recordedDeletes returns what the stub was asked to delete, in order.
func (s *stubGitLab) recordedDeletes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.deletes...)
}

// recordedListings returns the listings the stub answered, in order.
func (s *stubGitLab) recordedListings() []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubRequest(nil), s.listings...)
}

// recordListing notes one listing the stub is answering, and reports true
// when a test asked for that listing to be refused, having answered the
// refusal itself; the caller holds the lock.
func (s *stubGitLab) recordListing(w http.ResponseWriter, r *http.Request) bool {
	s.listings = append(s.listings, stubRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()})
	return s.refuse(w, r)
}

// refuse answers the refusal a test asked for on the request's path, and
// reports whether there was one; the caller holds the lock.
func (s *stubGitLab) refuse(w http.ResponseWriter, r *http.Request) bool {
	status, refused := s.refusals[r.URL.Path]
	if refused {
		writeError(w, status, strconv.Itoa(status)+" refused by the stub")
	}
	return refused
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

// listProjects answers the project listing, filtered by search, and a
// creation from the script, since what a created project is named is the
// test's to decide.
func (s *stubGitLab) listProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.scriptedRoute(w, r)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recordListing(w, r) {
		return
	}
	search := r.URL.Query().Get("search")
	var out []map[string]any
	for _, p := range s.projects {
		if strings.Contains(p.Name, search) || strings.Contains(p.Path, search) {
			out = append(out, projectJSON(p))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// listGroups answers the group listing, filtered by search, and a creation
// from the script, like the project listing.
func (s *stubGitLab) listGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.scriptedRoute(w, r)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recordListing(w, r) {
		return
	}
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
	if s.recordListing(w, r) {
		return
	}
	search := r.URL.Query().Get("search")
	var out []map[string]any
	for id, username := range s.users {
		if strings.Contains(username, search) {
			out = append(out, map[string]any{"id": id, "username": username, "name": username})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// listSnippets answers the snippet listing in ID order, with the null
// project_id GitLab gives a personal snippet, and a creation from the script,
// like the project listing.
//
// GitLab's snippet listing takes no search, so neither does this one: a
// sweep that searched it would be sending a parameter GitLab ignores, and the
// recorded query is what shows it did not.
func (s *stubGitLab) listSnippets(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.scriptedRoute(w, r)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recordListing(w, r) {
		return
	}
	out := []map[string]any{}
	for _, id := range slices.Sorted(maps.Keys(s.snippets)) {
		snippet := s.snippets[id]
		var projectID any
		if snippet.ProjectID != 0 {
			projectID = snippet.ProjectID
		}
		out = append(out, map[string]any{"id": id, "title": snippet.Title, "project_id": projectID})
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
	if s.refuse(w, r) {
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
	// An edit answers with the object, the way a real GitLab does. The
	// builders that edit one do it for the side effect rather than for the
	// answer, and an audit event is the side effect they are after.
	if r.Method == http.MethodPut {
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

// pipeline answers the configured failures, then the next status, or the
// whole pipeline a test put under the request path when it needs more of one
// than its status.
func (s *stubGitLab) pipeline(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pipelineFailures > 0 {
		s.pipelineFailures--
		writeError(w, http.StatusBadGateway, "502 Bad Gateway")
		return
	}
	if answer, ok := s.state[r.URL.Path]; ok {
		writeJSON(w, http.StatusOK, answer)
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
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&document); err != nil {
		writeError(w, http.StatusBadRequest, "malformed document: "+err.Error())
		return
	}
	s.graphqlDocuments = append(s.graphqlDocuments, document.Query)
	s.graphqlVariables = append(s.graphqlVariables, document.Variables)
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

// answers tells the stub what to answer one method and path with, in order,
// the last answer repeating.
func (s *stubGitLab) answers(method, path string, answers ...scriptedAnswer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scripted[method+" "+path] = answers
}

// recordedRequests returns what the scripted route was sent, in order.
func (s *stubGitLab) recordedRequests() []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubRequest(nil), s.requests...)
}

// The three shapes an answer takes: a body, an empty success, and a refusal.
func stubOK(body any) scriptedAnswer { return scriptedAnswer{status: http.StatusOK, body: body} }

func stubCreated(body any) scriptedAnswer {
	return scriptedAnswer{status: http.StatusCreated, body: body}
}

func stubNoContent() scriptedAnswer { return scriptedAnswer{status: http.StatusNoContent} }

func stubRefusal(status int, message string) scriptedAnswer {
	return scriptedAnswer{status: status, body: map[string]string{"message": message}}
}

// scriptedRoute answers from the script and records what it was sent.
//
// A request nothing scripted fails the test rather than answering a plausible
// empty object: a builder that reached an endpoint its recipe never meant to
// touch is exactly what these tests are for, and a silent 404 would read as
// the endpoint being absent.
func (s *stubGitLab) scriptedRoute(w http.ResponseWriter, r *http.Request) {
	recorded := stubRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: map[string]any{}}
	if r.Body != nil {
		if raw, err := io.ReadAll(r.Body); err == nil && json.Valid(raw) {
			// A body that is a JSON array or scalar decodes into nothing,
			// which is the empty map the request already carries.
			_ = json.Unmarshal(raw, &recorded.Body)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, recorded)

	key := r.Method + " " + r.URL.Path
	scripted, known := s.scripted[key]
	if !known || len(scripted) == 0 {
		s.t.Errorf("stub GitLab: nothing scripted for %s", key)
		writeError(w, http.StatusNotFound, "404 Not Found")
		return
	}
	answer := scripted[0]
	if len(scripted) > 1 {
		s.scripted[key] = scripted[1:]
	}
	if answer.status == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, answer.status, answer.body)
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
//
// The request is a literal rather than a built one: nothing sends it, and its
// message reads only the method and the URL.
func statusError(code int, message string) error {
	request := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Scheme: "https", Host: "gitlab.example", Path: "/api/v4/projects/1"},
	}
	return &gl.ErrorResponse{
		Response: &http.Response{StatusCode: code, Request: request},
		Message:  message,
	}
}

// detachedStub starts a stub and hands back an Env whose client is pointed
// at it, for a test that drives a builder whole rather than only the half
// that takes a client.
func detachedStub(t *testing.T) (*stubGitLab, *harness.Env) {
	t.Helper()
	stub, client := newStubGitLab(t)
	return stub, harness.NewDetached(t, client)
}

// isScopedName reports whether a name a builder made carries the prefix it
// was given and the run its Env belongs to, which is how the sweep recognizes
// what a run left behind.
func isScopedName(name, prefix string, e *harness.Env) bool {
	return strings.HasPrefix(name, prefix+"-") && strings.Contains(name, e.RunID())
}

// builderFatalEnv names the case [TestBuilderFatalCase_NamedByTheParent_RunsInAChildProcess]
// runs, and is set only by a parent test that expects that child to fail.
const builderFatalEnv = "E2E_FIXTURE_FATAL_CASE"

// builderFatalReturned is what a child prints when the builder it drove
// returned instead of ending its test, which is the one outcome a parent must
// tell apart from the failure it expects.
const builderFatalReturned = "the builder returned instead of failing its test"

// TestBuilderFatalCase_NamedByTheParent_RunsInAChildProcess runs the fatal
// case a parent test named, and skips when none did, which is every ordinary
// run.
func TestBuilderFatalCase_NamedByTheParent_RunsInAChildProcess(t *testing.T) {
	name := os.Getenv(builderFatalEnv)
	if name == "" {
		t.Skip("runs only as the child of a test that expects it to fail")
	}
	run, known := builderFatalCases[name]
	if !known {
		t.Fatalf("no fatal case is named %q", name)
	}
	run(t)
	t.Error(builderFatalReturned)
}

// runBuilderFatalCase runs one fatal case in a child test process and returns
// what the child printed, failing unless the child failed its test by ending
// it.
//
// A builder that cannot make what it was asked for calls t.Fatalf, which ends
// the test that called it, so the branch can be watched only from outside the
// process it ends. The child is this test binary asked for the one test
// above, and the coverage directory this process was given is handed on, so
// what the child executes is counted in the profile of the run that started
// it.
func runBuilderFatalCase(t *testing.T, name string) string {
	t.Helper()
	args := []string{"-test.run=^TestBuilderFatalCase_NamedByTheParent_RunsInAChildProcess$", "-test.count=1"}
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.gocoverdir=") {
			args = append(args, arg)
		}
	}
	// #nosec G204 G702 -- the binary is this test binary, and the arguments are
	// constants plus the coverage flag it was itself started with.
	cmd := exec.CommandContext(t.Context(), os.Args[0], args...)
	cmd.Env = append(os.Environ(), builderFatalEnv+"="+name)
	out, err := cmd.CombinedOutput()
	if _, exited := errors.AsType[*exec.ExitError](err); !exited {
		t.Fatalf("the %q case did not fail its child test (%v):\n%s", name, err, out)
	}
	if strings.Contains(string(out), builderFatalReturned) {
		t.Fatalf("in the %q case %s:\n%s", name, builderFatalReturned, out)
	}
	return string(out)
}

// fatalRefusal is the answer the fatal cases refuse a request with: one no
// retry policy of the fixture library waits through, so the builder fails on
// the first attempt.
func fatalRefusal() scriptedAnswer { return stubRefusal(http.StatusForbidden, "403 Forbidden") }

// builderFatalCases drive each builder into the branch that fails the test
// calling it, against a stub that refuses the one request the builder cannot
// do without.
var builderFatalCases = map[string]func(t *testing.T){
	"group board": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.configure(func() {
			stub.graphqlAnswers = []string{`{"data":{"createBoard":{"board":null,"errors":["Multiple boards are not available"]}}}`}
		})
		NewGroupBoard(e, Group{Path: "e2e-group"}, "board")
	},
	"environment": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodPost, "/api/v4/projects/2/environments", fatalRefusal())
		NewEnvironment(e, Project{ID: 2}, "env")
	},
	"deployment": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodPost, "/api/v4/projects/2/deployments", fatalRefusal())
		NewDeployment(e, Project{ID: 2, DefaultBranch: "main"}, Environment{ID: 5, Name: "env-run"}, "0123456789abcdef")
	},
	"group label": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodPost, "/api/v4/groups/1/labels", fatalRefusal())
		NewGroupLabel(e, Group{ID: 1}, "label")
	},
	"group milestone": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodPost, "/api/v4/groups/1/milestones", fatalRefusal())
		NewGroupMilestone(e, Group{ID: 1}, "milestone")
	},
	"pipeline": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodGet, "/api/v4/runners", stubOK([]map[string]any{{"id": 1, "description": "runner"}}))
		stub.answers(http.MethodPost, "/api/v4/projects/2/pipeline", fatalRefusal())
		NewPipeline(e, Project{ID: 2}, "main")
	},
	"pipeline without waiting": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodPost, "/api/v4/projects/2/pipeline", fatalRefusal())
		NewPipelineNoWait(e, Project{ID: 2}, "feature")
	},
	"pipeline job": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodGet, "/api/v4/projects/2/pipelines/77/jobs", fatalRefusal())
		ManualPipelineJobID(e, Project{ID: 2}, 77)
	},
	"project snippet": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodPost, "/api/v4/projects/2/snippets", fatalRefusal())
		NewProjectSnippet(e, Project{ID: 2})
	},
	"personal snippet": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodPost, "/api/v4/snippets", fatalRefusal())
		NewSnippet(e)
	},
	"world group": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		stub.answers(http.MethodPost, "/api/v4/groups", fatalRefusal())
		buildWorld(e, &World{})
	},
	"world project": func(t *testing.T) {
		t.Helper()
		stub, e := detachedStub(t)
		scriptWorldBuild(stub)
		stub.answers(http.MethodPost, "/api/v4/projects", fatalRefusal())
		buildWorld(e, &World{})
	},
	"world digest": func(t *testing.T) {
		t.Helper()
		shortWorldPipelineWaits(t)
		stub, e := detachedStub(t)
		scriptWorldBuild(stub)
		stub.configure(func() { delete(stub.state, "/api/v4/projects/2/labels/5") })
		buildWorld(e, &World{})
	},
}

// TestBuilders_Refused_FailTheirTestNamingWhatTheyWereMaking checks the
// branch every builder ends in when GitLab will not make what it asked for:
// the test that called it fails, and the failure names the object, the place
// it was to be made in and GitLab's reason, which is all a reader of a failed
// run has to go on.
func TestBuilders_Refused_FailTheirTestNamingWhatTheyWereMaking(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{name: "group board", want: []string{"creating a board in group e2e-group: GitLab refused the board: Multiple boards are not available"}},
		{name: "environment", want: []string{`creating environment "env-`, `in project 2: `, "403"}},
		{name: "deployment", want: []string{`creating a deployment of 01234567 into "env-run" in project 2: `, "403"}},
		{name: "group label", want: []string{`creating group label "label-`, `in group 1: `, "403"}},
		{name: "group milestone", want: []string{`creating group milestone "milestone-`, `in group 1: `, "403"}},
		{name: "pipeline", want: []string{`creating a pipeline on "main" in project 2: `, "403"}},
		{name: "pipeline without waiting", want: []string{`creating a pipeline on "feature" in project 2: `, "403"}},
		{name: "pipeline job", want: []string{"pipeline 77 in project 2 grew no job named " + ManualJobName + " within ", "403"}},
		{name: "project snippet", want: []string{`creating project snippet "snippet-`, `in project 2: `, "403"}},
		{name: "personal snippet", want: []string{`creating snippet "snippet-`, "403"}},
		{name: "world group", want: []string{"building the World: creating its group: ", "403"}},
		{name: "world project", want: []string{"building the World: creating its project: ", "403"}},
		{name: "world digest", want: []string{"building the World: reading it back for its digest: reading label 5 of project 2"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			out := runBuilderFatalCase(t, testCase.name)
			for _, want := range testCase.want {
				if !strings.Contains(out, want) {
					t.Errorf("the child printed:\n%s\nwant it to say %q", out, want)
				}
			}
		})
	}
	if len(cases) != len(builderFatalCases) {
		t.Errorf("%d cases are checked of the %d a child can run", len(cases), len(builderFatalCases))
	}
}
