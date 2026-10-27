// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// moved_project_test.go verifies IsMovedProjectRefusal, the reading of
// GitLab's refusal of a request that names a moved project by its old path.
package toolutil

import (
	"errors"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestIsMovedProjectRefusal_ReadsTheBodyOrAHEADByPath verifies which answers
// the predicate reads as GitLab refusing a moved project's old path.
//
// Two forms are: a 405 whose body carries find_project!'s message, and a 405
// answered to a HEAD request that names the project by a path, since GitLab
// refuses a HEAD the same way and its answer has no body to carry the
// message. Every other 405 is not, because GitLab answers a merge request that
// cannot be merged, a route a method does not have and a token of the wrong
// kind with 405 too, and project_moved? is false for a numeric ID, so a HEAD
// naming one is never this refusal.
//
// The requests are really made, through a real client-go client, so what the
// predicate is held to is client-go's own error value. One is made to an
// instance served under a relative URL root, whose project segment does not
// start the path. The hand-built errors are the shapes client-go never
// builds, which a handler's error can still be.
func TestIsMovedProjectRefusal_ReadsTheBodyOrAHEADByPath(t *testing.T) {
	answerUnder := func(root, method, path string, status int, body string) func(*testing.T) error {
		return func(t *testing.T) error {
			t.Helper()
			_, err := restAnswerUnder(t, root, method, path, status, body)
			return err
		}
	}
	answer := func(method, path string, status int, body string) func(*testing.T) error {
		return answerUnder("", method, path, status, body)
	}
	built := func(err error) func(*testing.T) error {
		return func(*testing.T) error { return err }
	}
	tests := []struct {
		name string
		err  func(*testing.T) error
		want bool
	}{
		{name: "a write naming the project by its old path", err: answer(http.MethodPut, movedProjectMergePath, http.StatusMethodNotAllowed, movedProjectBody), want: true},
		{name: "a HEAD naming the project by its old path", err: answer(http.MethodHead, movedProjectFilePath, http.StatusMethodNotAllowed, movedProjectBody), want: true},
		{name: "a HEAD naming the project by an old path escaped in lower case", err: answer(http.MethodHead, "projects/e2e-group%2fold-name/repository/files/README%2Emd", http.StatusMethodNotAllowed, ""), want: true},
		{name: "a HEAD naming the project by its old path under a relative URL root", err: answerUnder("/gitlab", http.MethodHead, movedProjectFilePath, http.StatusMethodNotAllowed, ""), want: true},
		{name: "a HEAD naming the project by its numeric ID", err: answer(http.MethodHead, numericProjectFilePath, http.StatusMethodNotAllowed, "")},
		{name: "a HEAD naming a group by its path", err: answer(http.MethodHead, "groups/e2e-group%2Fsub/projects", http.StatusMethodNotAllowed, "")},
		{name: "a merge request that cannot be merged, named by path", err: answer(http.MethodPut, movedProjectMergePath, http.StatusMethodNotAllowed, genericNotAllowedBody)},
		{name: "the message under another status", err: answer(http.MethodPost, movedProjectFilePath, http.StatusBadRequest, movedProjectBody)},
		{name: "a message that only starts with it", err: answer(http.MethodPut, movedProjectMergePath, http.StatusMethodNotAllowed,
			`{"message":"Non GET methods are not allowed for moved projects, and more"}`)},
		{name: "the message as a list", err: answer(http.MethodPut, movedProjectMergePath, http.StatusMethodNotAllowed,
			`{"message":["Non GET methods are not allowed for moved projects"]}`)},
		{name: "an error that carries no answer", err: built(errors.New("Non GET methods are not allowed for moved projects"))},
		{name: "a refusal built with its body and no response", err: built(&gl.ErrorResponse{StatusCode: http.StatusMethodNotAllowed, Body: []byte(movedProjectBody)}), want: true},
		{name: "a 405 built with neither a body nor a response", err: built(&gl.ErrorResponse{StatusCode: http.StatusMethodNotAllowed})},
		{name: "a HEAD built without its URL", err: built(&gl.ErrorResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Response:   &http.Response{StatusCode: http.StatusMethodNotAllowed, Request: &http.Request{Method: http.MethodHead}},
		})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.err(t)
			if got := IsMovedProjectRefusal(err); got != tt.want {
				t.Errorf("IsMovedProjectRefusal(%v) = %v, want %v", err, got, tt.want)
			}
		})
	}
}
