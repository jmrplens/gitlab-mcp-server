package toolutil

import (
	"encoding/json"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// movedProjectRefusalMessage is the message find_project! (lib/api/helpers.rb
// at v19.4.1-ee) refuses a request with when it names a moved project by an
// old path and is not a GET: GitLab keeps a redirect from every path a project
// had and follows a GET through it, and an endpoint that looks the project up
// through find_project!, which most do, answers every other method 405 once
// it has found that the caller can read the project.
const movedProjectRefusalMessage = "Non GET methods are not allowed for moved projects"

// movedProjectDescription is what [ClassifyError] says about that refusal in
// place of the 405 sentence, which reads as a statement about the merge
// request, issue or branch the call was about, so a model looked for the
// reason there and never retried with the ID (issue 1133). It names
// project.get because that read, given the old path, is followed to the new
// location and answers both the ID and the current path.
//
// It is one literal rather than a concatenation, for the reason
// [UnattributedRequestMessage] gives.
const movedProjectDescription = "method not allowed: the project was named by a path it no longer has, since it or a group above it was renamed or transferred. GitLab follows a GET to the project's new location and refuses every other method, HEAD included, that names it by an old path. Retry with the numeric project ID or the current path, both of which project.get returns for the old path"

// projectsRoute is where the project segment of a REST path starts, matched
// anywhere in the path so an instance served under a relative URL root is
// read too.
const projectsRoute = "/api/v4/projects/"

// IsMovedProjectRefusal reports whether GitLab refused err's call because it
// named a moved project by an old path: a 405 whose body carries
// find_project!'s message, or a 405 answered to a HEAD request that names the
// project by a path.
//
// The HEAD is read by its request because its answer has no body to carry the
// message, and find_project! refuses it all the same, since its check is for
// a GET. Two of this server's reads are HEAD requests, the file metadata ones.
// project_moved? is false for a numeric ID, so a HEAD naming the project by
// one is never this refusal, and neither is any other 405: GitLab also
// answers 405 for a merge request that cannot be merged, a method a route
// does not have, and a token of the wrong kind to rotate itself.
//
// A handler that keys a hint on the 405 of a request other than a GET, HEAD
// included, that names a project by its :id checks this first, so that its
// hint does not explain a refusal GitLab gave before the route looked at
// anything the hint is about. A GET is followed through the redirect, so a
// 405 answered to one is never this refusal, and neither is one answered to
// a route that names no project.
func IsMovedProjectRefusal(err error) bool {
	glErr, ok := gitLabResponseOf(err)
	return ok && isMovedProjectRefusal(glErr)
}

// isMovedProjectRefusal is [IsMovedProjectRefusal] for a response already
// found, which [ClassifyError] holds.
func isMovedProjectRefusal(glErr *gl.ErrorResponse) bool {
	if answeredStatus(glErr) != http.StatusMethodNotAllowed {
		return false
	}
	return refusesMovedProject(glErr.Body) || headNamesProjectByPath(answeredRequest(glErr))
}

// refusesMovedProject reports whether body is a JSON object whose message is
// [movedProjectRefusalMessage]. It is read from the body rather than from
// ErrorResponse.Message, which carries client-go's {message: ...} rendering.
func refusesMovedProject(body []byte) bool {
	var answer struct {
		Message string `json:"message"`
	}
	return json.Unmarshal(body, &answer) == nil && answer.Message == movedProjectRefusalMessage
}

// headNamesProjectByPath reports whether req is a HEAD request whose project
// segment is a path rather than a numeric ID. client-go escapes the slashes
// of a path into that segment, and an ID has none, so an escaped slash is
// what tells them apart. A request that cannot be read, as in an error built
// by hand, is not one.
func headNamesProjectByPath(req *http.Request) bool {
	if req == nil || req.Method != http.MethodHead || req.URL == nil {
		return false
	}
	_, rest, _ := strings.Cut(req.URL.EscapedPath(), projectsRoute)
	project, _, _ := strings.Cut(rest, "/")
	return strings.Contains(strings.ToUpper(project), "%2F")
}
