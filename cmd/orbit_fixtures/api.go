// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// maxAnswerBytes bounds what one answer is read up to. The largest answer
// read here is a page of a repository tree, a hundred short entries.
const maxAnswerBytes = 8 << 20

// gitlabAPI makes the REST requests of a run against one instance's API root.
type gitlabAPI struct {
	// root is the API root, such as https://gitlab.com/api/v4, with no
	// trailing slash.
	root string
	// token is sent as PRIVATE-TOKEN on every request.
	token  string
	client *http.Client
}

// answer is what GitLab answered one request with. A status of 0 is a request
// that got no answer, err saying why.
type answer struct {
	status int
	body   []byte
	err    error
}

// ok reports whether the request was answered with a 2xx.
func (a answer) ok() bool { return a.status >= 200 && a.status < 300 }

// String is the answer as a message quotes it: GitLab's body, or why there
// was none.
func (a answer) String() string {
	if a.err != nil {
		return a.err.Error()
	}
	return string(a.body)
}

// do sends one request. path is appended to the API root as written, its
// escapes included: a project or group named by its full path carries every
// slash of the path as %2F, which GitLab requires.
func (api gitlabAPI) do(ctx context.Context, method, path string, body any) answer {
	target, err := url.Parse(api.root + path)
	if err != nil {
		return answer{err: fmt.Errorf("the request path %q does not parse: %w", path, err)}
	}
	var reader io.Reader
	if body != nil {
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return answer{err: marshalErr}
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return answer{err: err}
	}
	request.Header.Set("PRIVATE-TOKEN", api.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := api.client.Do(request)
	if err != nil {
		return answer{err: err}
	}
	defer response.Body.Close()
	read, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	if err != nil {
		return answer{status: response.StatusCode, err: fmt.Errorf("reading the answer: %w", err)}
	}
	return answer{status: response.StatusCode, body: read}
}

// decode reads the answer's body into out, and reports whether it held one
// JSON value of out's shape.
func (a answer) decode(out any) bool {
	return a.err == nil && json.Unmarshal(a.body, out) == nil
}
